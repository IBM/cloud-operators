package controllers

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/zapr"
	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
	"github.com/ibm/cloud-operators/internal/ibmcloud/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	scheme, err := ibmcloudv1beta1.SchemeBuilder.Build()
	require.NoError(t, err)
	require.NoError(t, corev1.SchemeBuilder.AddToScheme(scheme))
	client := fake.NewFakeClientWithScheme(scheme,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "secret",
			},
			Data: map[string][]byte{
				"api-key": []byte(`bogus key`),
			},
		},
	)
	tokenReconciler := &TokenReconciler{
		Client: client,
		Log:    zapr.NewLogger(zaptest.NewLogger(t)),
		Scheme: scheme,
		Authenticate: func(apiKey, region string) (auth.Credentials, error) {
			return auth.Credentials{}, fmt.Errorf("failure")
		},
	}

	result, err := tokenReconciler.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "secret",
		},
	})
	assert.EqualError(t, err, "failure")
	assert.Equal(t, ctrl.Result{}, result)
}
