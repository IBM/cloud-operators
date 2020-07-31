package controllers

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/ghodss/yaml"
	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func mustLoadObject(t *testing.T, file string, obj runtime.Object) {
	buf, err := ioutil.ReadFile(file)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(buf, obj))
}

func TestServiceBinding(t *testing.T) {
	const (
		servicefile = "testdata/translator.yaml"
		bindingfile = "testdata/translator-binding.yaml"
	)

	var service ibmcloudv1beta1.Service
	mustLoadObject(t, servicefile, &service)
	service.Namespace = testNamespace
	service.GenerateName = testNameStem
	var binding ibmcloudv1beta1.Binding
	mustLoadObject(t, bindingfile, &binding)
	binding.Namespace = testNamespace
	service.GenerateName = testNameStem

	ready := t.Run("should be ready", func(t *testing.T) {
		ctx := context.TODO()

		err := k8sClient.Create(ctx, &service)
		require.NoError(t, err)
		t.Log("Service name & namespace", service.Name, service.Namespace)

		// make sure service is online
		var state string
		require.Eventually(t, func() bool {
			var fetched ibmcloudv1beta1.Service
			err := getObject(ctx, service.ObjectMeta, &fetched)
			t.Logf("Checking state: %v %#v", err, fetched.Status)
			return err == nil && fetched.Status.State != ""
		}, defaultWait, defaultTick)
		assert.Equal(t, bindingStateOnline, state)

		// now test creation of binding
		err = k8sClient.Create(ctx, &binding)
		require.NoError(t, err)

		// check binding is online
		require.Eventually(t, func() bool {
			var fetched ibmcloudv1beta1.Binding
			err := getObject(ctx, binding.ObjectMeta, &fetched)
			return err == nil && fetched.Status.State == bindingStateOnline
		}, defaultWait, defaultTick)

		// check secret is created
		err = getObject(ctx, binding.ObjectMeta, &corev1.Secret{})
		assert.NoError(t, err)
	})
	if !ready {
		return
	}

	t.Run("should delete", func(t *testing.T) {
		ctx := context.TODO()

		// delete binding
		require.NoError(t, k8sClient.Delete(ctx, &binding))

		// test secret is deleted
		assert.Eventually(t, func() bool {
			err := getObject(ctx, binding.ObjectMeta, &corev1.Secret{})
			return errors.IsNotFound(err)
		}, defaultWait, defaultTick)

		// delete service & return when done
		require.NoError(t, k8sClient.Delete(ctx, &service))

		assert.Eventually(t, func() bool {
			err := getObject(ctx, service.ObjectMeta, &ibmcloudv1beta1.Service{})
			return errors.IsNotFound(err)
		}, defaultWait, defaultTick)
	})
}

func getObject(ctx context.Context, meta metav1.ObjectMeta, v runtime.Object) error {
	return k8sClient.Get(ctx, types.NamespacedName{
		Name:      meta.Name,
		Namespace: meta.Namespace,
	}, v)
}
