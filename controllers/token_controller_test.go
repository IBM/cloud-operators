package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ibm/cloud-operators/internal/ibmcloud/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	// setTokenHTTPClient sets the test's authenticator, then restores it when the test ends
	setTokenHTTPClient func(testing.TB, auth.Authenticator)
)

func TestToken(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create the secret object and expect the Reconcile
	const (
		secretName   = "dummyapikey"
		secretAPIKey = "VExS246avaUT6MXZ56SH_I-AeWo_-JmW0u79Jd8LiBH" // nolint:gosec // Fake API key
	)

	setTokenHTTPClient(t, func(apiKey, region string) (auth.Credentials, error) {
		return auth.Credentials{
			IAMAccessToken: "Bearer dummytoken",
		}, nil
	})

	instance := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
			Labels: map[string]string{
				"seed.ibm.com/ibmcloud-token": "apikey",
			},
		},
		Data: map[string][]byte{
			"api-key": []byte(secretAPIKey),
		},
	}
	err := k8sClient.Create(context.TODO(), instance)
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, k8sClient.Delete(context.TODO(), instance))
	}()

	var secret corev1.Secret
	assert.Eventually(t, func() bool {
		err := k8sClient.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: "dummyapikey-tokens"}, &secret)
		if err != nil {
			t.Log("Failed to get secret:", err)
			return false
		}

		_, ok := secret.Data["access_token"]
		return ok
	}, defaultWait, defaultTick)

	assert.Equal(t, "Bearer dummytoken", string(secret.Data["access_token"]))
	assert.Contains(t, secret.Data, "refresh_token")
	assert.Contains(t, secret.Data, "uaa_token")
	assert.Contains(t, secret.Data, "uaa_refresh_token")
}

func TestTokenFailedAuth(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	objects := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secret-ibm-cloud-operator"},
			Data: map[string][]byte{
				"api-key": []byte(`bogus key`),
			},
		},
	}
	r := &TokenReconciler{
		Client: fake.NewFakeClientWithScheme(scheme, objects...),
		Log:    testLogger(t),
		Scheme: scheme,
		Authenticate: func(apiKey, region string) (auth.Credentials, error) {
			return auth.Credentials{}, fmt.Errorf("failure")
		},
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "secret-ibm-cloud-operator"},
	})
	assert.EqualError(t, err, "failure")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestTokenFailedSecretLookup(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	r := &TokenReconciler{
		Client:       fake.NewFakeClientWithScheme(scheme),
		Log:          testLogger(t),
		Scheme:       scheme,
		Authenticate: nil, // should not be called
	}

	t.Run("not found", func(t *testing.T) {
		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "secret-ibm-cloud-operator"},
		})
		assert.NoError(t, err, "Don't retry (return err) if secret no longer exists")
		assert.Equal(t, ctrl.Result{}, result)
	})

	r.Client = fake.NewFakeClientWithScheme(runtime.NewScheme()) // fail to read the type Secret
	t.Run("failed to read secret", func(t *testing.T) {
		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "secret-ibm-cloud-operator"},
		})
		assert.Error(t, err)
		assert.False(t, k8sErrors.IsNotFound(err))
		assert.Equal(t, ctrl.Result{}, result)
	})
}

func TestTokenSecretIsDeleting(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	now := metav1Now(t)
	objects := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "secret-ibm-cloud-operator",
				DeletionTimestamp: now,
			},
		},
	}
	r := &TokenReconciler{
		Client:       fake.NewFakeClientWithScheme(scheme, objects...),
		Log:          testLogger(t),
		Scheme:       scheme,
		Authenticate: nil, // should not be called
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "secret-ibm-cloud-operator"},
	})
	assert.NoError(t, err, "Don't retry (return err) if secret is deleting")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestTokenAPIKeyIsMissing(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	objects := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secret-ibm-cloud-operator"},
			Data:       nil, // no API key
		},
	}
	r := &TokenReconciler{
		Client:       fake.NewFakeClientWithScheme(scheme, objects...),
		Log:          testLogger(t),
		Scheme:       scheme,
		Authenticate: nil, // should not be called
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "secret-ibm-cloud-operator"},
	})
	assert.NoError(t, err, "Don't retry (return err) if secret does not contain an api-key entry")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestTokenAuthInvalidConfig(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	const (
		apiKey = "some API key"
		region = "some invalid region"
	)
	objects := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secret-ibm-cloud-operator"},
			Data: map[string][]byte{
				"api-key": []byte(apiKey),
				"region":  []byte(region),
			},
		},
	}
	r := &TokenReconciler{
		Client: fake.NewFakeClientWithScheme(scheme, objects...),
		Log:    testLogger(t),
		Scheme: scheme,
		Authenticate: func(actualAPIKey, actualRegion string) (auth.Credentials, error) {
			assert.Equal(t, apiKey, actualAPIKey)
			assert.Equal(t, region, actualRegion)
			return auth.Credentials{}, auth.InvalidConfigError{Err: fmt.Errorf("Invalid region")}
		},
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "secret-ibm-cloud-operator"},
	})
	assert.NoError(t, err, "Don't retry (return err) if secret region is invalid")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestTokenDeleteFailed(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	const (
		apiKey      = "some API key"
		region      = "some invalid region"
		accessToken = "some access token"
	)
	objects := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secret-ibm-cloud-operator"},
			Data: map[string][]byte{
				"api-key": []byte(apiKey),
				"region":  []byte(region),
			},
		},
	}
	var r *TokenReconciler
	r = &TokenReconciler{
		Client: fake.NewFakeClientWithScheme(scheme, objects...),
		Log:    testLogger(t),
		Scheme: scheme,
		Authenticate: func(actualAPIKey, actualRegion string) (auth.Credentials, error) {
			assert.Equal(t, apiKey, actualAPIKey)
			assert.Equal(t, region, actualRegion)
			r.Client = fake.NewFakeClientWithScheme(runtime.NewScheme()) // trigger later failure of r.Client.Delete
			return auth.Credentials{
				IAMAccessToken: accessToken,
			}, nil
		},
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "secret-ibm-cloud-operator"},
	})
	assert.Error(t, err)
	assert.False(t, k8sErrors.IsNotFound(err))
	assert.Equal(t, ctrl.Result{}, result)
}

func TestTokenRaceCreateFailed(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	const (
		apiKey      = "some API key"
		region      = "some invalid region"
		accessToken = "some access token"
	)
	tokensSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "secret-ibm-cloud-operator-tokens"},
		Data: map[string][]byte{
			"access_token": []byte("old " + accessToken),
		},
	}
	objects := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secret-ibm-cloud-operator"},
			Data: map[string][]byte{
				"api-key": []byte(apiKey),
				"region":  []byte(region),
			},
		},
		tokensSecret,
	}
	r := &TokenReconciler{
		Client: fake.NewFakeClientWithScheme(scheme, objects...),
		Log:    testLogger(t),
		Scheme: scheme,
		Authenticate: func(actualAPIKey, actualRegion string) (auth.Credentials, error) {
			assert.Equal(t, apiKey, actualAPIKey)
			assert.Equal(t, region, actualRegion)
			return auth.Credentials{
				IAMAccessToken: accessToken,
			}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// re-create the secret constantly during the test to trigger race condition
				_ = r.Client.Create(context.Background(), tokensSecret)
			}
		}
	}()
	defer cancel()

	var result ctrl.Result
	var err error
	require.Eventually(t, func() bool {
		result, err = r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "secret-ibm-cloud-operator"},
		})
		return err != nil
	}, 5*time.Second, 10*time.Millisecond)
	assert.Error(t, err)
	assert.True(t, k8sErrors.IsAlreadyExists(err))
	assert.Equal(t, ctrl.Result{}, result)
}
