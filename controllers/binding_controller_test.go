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
)

func mustLoadObject(t *testing.T, file string, obj runtime.Object, meta *metav1.ObjectMeta) {
	t.Helper()
	buf, err := ioutil.ReadFile(file)
	if err != nil {
		t.Fatalf("Error while reading template %q: %v", file, err)
	}
	err = yaml.Unmarshal(buf, obj)
	if err != nil {
		t.Fatalf("Error while unmarshaling template %q: %v", file, err)
	}
	meta.Namespace = testNamespace
}

func TestBinding(t *testing.T) {
	const (
		servicefile = "testdata/translator-2.yaml"
		bindingfile = "testdata/translator-binding.yaml"
	)

	var service ibmcloudv1beta1.Service
	mustLoadObject(t, servicefile, &service, &service.ObjectMeta)
	var binding ibmcloudv1beta1.Binding
	mustLoadObject(t, bindingfile, &binding, &binding.ObjectMeta)

	ready := t.Run("should be ready", func(t *testing.T) {
		ctx := context.TODO()

		err := k8sClient.Create(ctx, &service)
		require.NoError(t, err)

		// make sure service is online
		require.Eventually(t, verifyStatus(ctx, t, service.ObjectMeta, new(ibmcloudv1beta1.Service), serviceStateOnline), defaultWait, defaultTick)

		// now test creation of binding
		err = k8sClient.Create(ctx, &binding)
		require.NoError(t, err)

		// check binding is online
		require.Eventually(t, verifyStatus(ctx, t, binding.ObjectMeta, new(ibmcloudv1beta1.Binding), bindingStateOnline), defaultWait, defaultTick)

		// check secret is created
		err = getObject(ctx, binding.ObjectMeta, &corev1.Secret{})
		assert.NoError(t, err)
	})
	if !ready {
		t.FailNow()
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
