package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/ghodss/yaml"
	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
	"github.com/ibm/cloud-operators/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
	if testing.Short() {
		t.SkipNow()
	}

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

func TestBindingFailedLookup(t *testing.T) {
	scheme := schemas(t)
	r := &BindingReconciler{
		Client: fake.NewFakeClientWithScheme(scheme),
		Log:    testLogger(t),
		Scheme: scheme,
	}

	t.Run("not found", func(t *testing.T) {
		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "mybinding"},
		})
		assert.NoError(t, err, "Don't retry (return err) if binding does not exist")
		assert.Equal(t, ctrl.Result{}, result)
	})

	r.Client = fake.NewFakeClientWithScheme(runtime.NewScheme()) // fail to read the type Binding
	t.Run("failed to read binding", func(t *testing.T) {
		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "mybinding"},
		})
		assert.Error(t, err)
		assert.False(t, k8sErrors.IsNotFound(err))
		assert.Equal(t, ctrl.Result{}, result)
	})
}

func TestBindingFailInitialStatus(t *testing.T) {
	scheme := schemas(t)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Binding{
			ObjectMeta: metav1.ObjectMeta{Name: "mybinding"},
			Status:     ibmcloudv1beta1.BindingStatus{}, // empty
		},
	}
	client := fake.NewFakeClientWithScheme(scheme, objects...)
	client = newMockClient(client, MockConfig{
		StatusUpdateErr: fmt.Errorf("failed"),
	})
	r := &BindingReconciler{
		Client: client,
		Log:    testLogger(t),
		Scheme: scheme,
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "mybinding"},
	})
	assert.NoError(t, err, "Don't retry (return err) if binding does not exist")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestBindingFailGetServiceInstance(t *testing.T) {
	now := metav1.Now().Rfc3339Copy() // low-resolution time
	for _, tc := range []struct {
		description        string
		binding            *ibmcloudv1beta1.Binding
		fakeClient         *MockConfig
		expectUpdate       *ibmcloudv1beta1.Binding
		expectStatusUpdate *ibmcloudv1beta1.Binding
		expectResult       ctrl.Result
	}{
		{
			description: "no service instance",
			binding: &ibmcloudv1beta1.Binding{
				TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{Name: "mybinding"},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: "myservice",
				},
			},
			expectResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: requeueFast,
			},
		},
		{
			description: "binding is deleting",
			binding: &ibmcloudv1beta1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mybinding",
					DeletionTimestamp: &now,
					Finalizers:        []string{bindingFinalizer},
				},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1beta1.BindingStatus{State: bindingStateOnline},
			},
			fakeClient: &MockConfig{},
			expectUpdate: &ibmcloudv1beta1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mybinding",
					DeletionTimestamp: &now,
				},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1beta1.BindingStatus{State: bindingStateOnline},
			},
			expectResult: ctrl.Result{},
		},
		{
			description: "binding is deleting but update fails",
			binding: &ibmcloudv1beta1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mybinding",
					DeletionTimestamp: &now,
					Finalizers:        []string{bindingFinalizer},
				},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1beta1.BindingStatus{State: bindingStateOnline},
			},
			fakeClient: &MockConfig{UpdateErr: fmt.Errorf("failed")},
			expectUpdate: &ibmcloudv1beta1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mybinding",
					DeletionTimestamp: &now,
				},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1beta1.BindingStatus{State: bindingStateOnline},
			},
			expectResult: ctrl.Result{},
		},
		{
			description: "binding is deleting and status service instance is set",
			binding: &ibmcloudv1beta1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "mybinding",
					Finalizers: []string{bindingFinalizer},
				},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1beta1.BindingStatus{
					State:         bindingStateOnline,
					KeyInstanceID: "myinstance",
				},
			},
			fakeClient: &MockConfig{},
			expectStatusUpdate: &ibmcloudv1beta1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "mybinding",
					Finalizers: []string{bindingFinalizer},
				},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1beta1.BindingStatus{
					State:   bindingStatePending,
					Message: "Processing Resource",
				},
			},
			expectResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			scheme := schemas(t)
			r := &BindingReconciler{
				Client: fake.NewFakeClientWithScheme(scheme, tc.binding),
				Log:    testLogger(t),
				Scheme: scheme,
			}
			if tc.fakeClient != nil {
				r.Client = newMockClient(r.Client, *tc.fakeClient)
			}

			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "mybinding"},
			})
			assert.NoError(t, err)
			assert.Equal(t, tc.expectResult, result)
			if tc.expectUpdate != nil {
				assert.Equal(t, tc.expectUpdate, r.Client.(MockClient).LastUpdate(), "Binding update should be equal")
			}
			if tc.expectStatusUpdate != nil {
				assert.Equal(t, tc.expectStatusUpdate, r.Client.(MockClient).LastStatusUpdate(), "Binding status update should be equal")
			}
		})
	}
}
