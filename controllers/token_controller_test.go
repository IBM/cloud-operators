package controllers

import (
	"context"
	"fmt"
	"k8s.io/apiserver/pkg/storage/testresource"
	"testing"
	"time"

	"github.com/ibm/cloud-operators/internal/ibmcloud/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake" // nolint: staticcheck
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
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
		secretName   = "ibmcloud-operator-secret"
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
		err := k8sClient.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: "ibmcloud-operator-tokens"}, &secret)
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
	objects := []client.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "ibmcloud-operator-secret"},
			Data: map[string][]byte{
				"api-key": []byte(`bogus key`),
			},
		},
	}
	r := &TokenReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
		Log:    testLogger(t),
		Scheme: scheme,
		Authenticate: func(apiKey, region string) (auth.Credentials, error) {
			return auth.Credentials{}, fmt.Errorf("failure")
		},
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "ibmcloud-operator-secret"},
	})
	assert.EqualError(t, err, "failure")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestTokenFailedSecretLookup(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	r := &TokenReconciler{
		Client:       fake.NewClientBuilder().WithScheme(scheme).Build(),
		Log:          testLogger(t),
		Scheme:       scheme,
		Authenticate: nil, // should not be called
	}

	t.Run("not found", func(t *testing.T) {
		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "ibmcloud-operator-secret"},
		})
		assert.NoError(t, err, "Don't retry (return err) if secret no longer exists")
		assert.Equal(t, ctrl.Result{}, result)
	})

	r.Client = fake.NewClientBuilder().Build() // fail to read the type Secret
	t.Run("failed to read secret", func(t *testing.T) {
		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "ibmcloud-operator-secret"},
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
	objects := []client.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "ibmcloud-operator-secret",
				DeletionTimestamp: now,
			},
		},
	}
	r := &TokenReconciler{
		Client:       fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
		Log:          testLogger(t),
		Scheme:       scheme,
		Authenticate: nil, // should not be called
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "ibmcloud-operator-secret"},
	})
	assert.NoError(t, err, "Don't retry (return err) if secret is deleting")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestTokenAPIKeyIsMissing(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	objects := []client.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "ibmcloud-operator-secret"},
			Data:       nil, // no API key
		},
	}
	r := &TokenReconciler{
		Client:       fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
		Log:          testLogger(t),
		Scheme:       scheme,
		Authenticate: nil, // should not be called
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "ibmcloud-operator-secret"},
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
	objects := []client.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "ibmcloud-operator-secret"},
			Data: map[string][]byte{
				"api-key": []byte(apiKey),
				"region":  []byte(region),
			},
		},
	}
	r := &TokenReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
		Log:    testLogger(t),
		Scheme: scheme,
		Authenticate: func(actualAPIKey, actualRegion string) (auth.Credentials, error) {
			assert.Equal(t, apiKey, actualAPIKey)
			assert.Equal(t, region, actualRegion)
			return auth.Credentials{}, auth.InvalidConfigError{Err: fmt.Errorf("Invalid region")}
		},
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "ibmcloud-operator-secret"},
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
	objects := []client.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "ibmcloud-operator-secret"},
			Data: map[string][]byte{
				"api-key": []byte(apiKey),
				"region":  []byte(region),
			},
		},
	}
	var r *TokenReconciler
	r = &TokenReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),

		Log:    testLogger(t),
		Scheme: scheme,
		Authenticate: func(actualAPIKey, actualRegion string) (auth.Credentials, error) {
			assert.Equal(t, apiKey, actualAPIKey)
			assert.Equal(t, region, actualRegion)
			r.Client = fake.NewClientBuilder().Build() // trigger later failure of r.Client.Delete
			return auth.Credentials{
				IAMAccessToken: accessToken,
			}, nil
		},
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "ibmcloud-operator-secret"},
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
		ObjectMeta: metav1.ObjectMeta{Name: "ibmcloud-operator-tokens"},
		Data: map[string][]byte{
			"access_token": []byte("old " + accessToken),
		},
	}
	objects := []client.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "ibmcloud-operator-secret"},
			Data: map[string][]byte{
				"api-key": []byte(apiKey),
				"region":  []byte(region),
			},
		},
		tokensSecret,
	}
	r := &TokenReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
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
		result, err = r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "ibmcloud-operator-secret"},
		})
		return err != nil
	}, 5*time.Second, 10*time.Millisecond)
	assert.Error(t, err)
	assert.True(t, k8sErrors.IsAlreadyExists(err))
	assert.Equal(t, ctrl.Result{}, result)
}

func TestShouldProcessSecret(t *testing.T) {
	t.Parallel()

	t.Run("normal secret", func(t *testing.T) {
		assert.True(t, shouldProcessSecret(&metav1.ObjectMeta{Name: "ibmcloud-operator-secret"}))
	})

	t.Run("management namespace secret", func(t *testing.T) {
		assert.True(t, shouldProcessSecret(&metav1.ObjectMeta{Name: "mynamespace-ibmcloud-operator-secret"}))
	})
}

func TestTokenSetupWithManager(t *testing.T) {
	t.Parallel()
	mgr := &mockManager{T: t}
	options := controller.Options{MaxConcurrentReconciles: 1}

	err := (&TokenReconciler{}).SetupWithManager(mgr, options)
	assert.NoError(t, err)
}

func TestTokenEventsFilter(t *testing.T) {
	t.Parallel()

	filter := eventsFilter()
	//shouldProcessEvent := &metav1.ObjectMeta{Name: icoSecretName}
	shouldProcessEvent := &testresource.TestResource{ObjectMeta: metav1.ObjectMeta{Name: icoSecretName}}
	shouldNotProcessEvent := &testresource.TestResource{ObjectMeta: metav1.ObjectMeta{}}
	assert.True(t, filter.CreateFunc(event.CreateEvent{shouldProcessEvent}))
	assert.False(t, filter.CreateFunc(event.CreateEvent{shouldNotProcessEvent}))
	assert.True(t, filter.DeleteFunc(event.DeleteEvent{shouldProcessEvent, true}))
	assert.False(t, filter.DeleteFunc(event.DeleteEvent{shouldNotProcessEvent, false}))
	assert.True(t, filter.UpdateFunc(event.UpdateEvent{shouldProcessEvent, shouldProcessEvent}))
	assert.False(t, filter.UpdateFunc(event.UpdateEvent{shouldNotProcessEvent, shouldNotProcessEvent}))
}
