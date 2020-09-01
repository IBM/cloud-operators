package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
	"github.com/ibm/cloud-operators/internal/config"
	"github.com/ibm/cloud-operators/internal/ibmcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestBindingSetOwnerReferenceFailed(t *testing.T) {
	t.Parallel()
	t.Run("setting owner reference failed", func(t *testing.T) {
		scheme := schemas(t)
		const namespace = "mynamespace"
		objects := []runtime.Object{
			&ibmcloudv1beta1.Binding{
				TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{Name: "mybinding", Namespace: namespace},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: "myservice",
				},
			},
			&ibmcloudv1beta1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: namespace},
			},
		}
		r := &BindingReconciler{
			Client: fake.NewFakeClientWithScheme(scheme, objects...),
			Log:    testLogger(t),
			Scheme: scheme,

			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return fmt.Errorf("failed")
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "mybinding", Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.EqualError(t, err, "failed")
	})

	t.Run("binding update failed", func(t *testing.T) {
		scheme := schemas(t)
		const namespace = "mynamespace"
		objects := []runtime.Object{
			&ibmcloudv1beta1.Binding{
				TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{Name: "mybinding", Namespace: namespace},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1beta1.BindingStatus{
					State: bindingStateOnline,
				},
			},
			&ibmcloudv1beta1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: namespace},
			},
		}
		client := newMockClient(
			fake.NewFakeClientWithScheme(scheme, objects...),
			MockConfig{
				UpdateErr: fmt.Errorf("failed"),
			})
		r := &BindingReconciler{
			Client: client,
			Log:    testLogger(t),
			Scheme: scheme,

			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "mybinding", Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Binding{
			TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: "mybinding", Namespace: namespace},
			Spec: ibmcloudv1beta1.BindingSpec{
				ServiceName: "myservice",
			},
			Status: ibmcloudv1beta1.BindingStatus{
				State: bindingStateOnline,
			},
		}, client.LastUpdate())
	})
}

func TestBindingServiceIsNotReady(t *testing.T) {
	t.Parallel()
	t.Run("empty instance ID", func(t *testing.T) {
		scheme := schemas(t)
		const namespace = "mynamespace"
		objects := []runtime.Object{
			&ibmcloudv1beta1.Binding{
				TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{Name: "mybinding", Namespace: namespace},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: "myservice",
				},
			},
			&ibmcloudv1beta1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					InstanceID: "",
				},
			},
		}
		r := &BindingReconciler{
			Client: fake.NewFakeClientWithScheme(scheme, objects...),
			Log:    testLogger(t),
			Scheme: scheme,

			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "mybinding", Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: requeueFast,
		}, result)
		assert.NoError(t, err)
	})

	t.Run("status instance ID is in progress", func(t *testing.T) {
		scheme := schemas(t)
		const namespace = "mynamespace"
		objects := []runtime.Object{
			&ibmcloudv1beta1.Binding{
				TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{Name: "mybinding", Namespace: namespace},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: "myservice",
				},
			},
			&ibmcloudv1beta1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					InstanceID: inProgress,
				},
			},
		}
		r := &BindingReconciler{
			Client: fake.NewFakeClientWithScheme(scheme, objects...),
			Log:    testLogger(t),
			Scheme: scheme,

			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "mybinding", Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: requeueFast,
		}, result)
		assert.NoError(t, err)
	})
}

func TestBindingGetIBMCloudInfoFailed(t *testing.T) {
	t.Parallel()
	now := metav1.Now().Rfc3339Copy() // low-resolution time
	scheme := schemas(t)
	const (
		namespace      = "mynamespace"
		bindingName    = "mybinding"
		serviceName    = "myservice"
		someInstanceID = "some-instance-id"
	)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              bindingName,
				Namespace:         namespace,
				DeletionTimestamp: &now,
				Finalizers:        []string{bindingFinalizer},
			},
			Spec:   ibmcloudv1beta1.BindingSpec{ServiceName: serviceName},
			Status: ibmcloudv1beta1.BindingStatus{State: bindingStateFailed},
		},
		&ibmcloudv1beta1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1beta1.ServiceStatus{
				InstanceID: someInstanceID,
			},
		},
	}

	t.Run("not found error", func(t *testing.T) {
		var r *BindingReconciler
		r = &BindingReconciler{
			Client: newMockClient(fake.NewFakeClientWithScheme(scheme, objects...), MockConfig{}),
			Log:    testLogger(t),
			Scheme: scheme,

			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				r.Client = newMockClient( // swap out client so next update fails
					fake.NewFakeClientWithScheme(scheme, objects...),
					MockConfig{UpdateErr: fmt.Errorf("failed")},
				)
				return nil, errors.NewNotFound(ctrl.GroupResource{Group: "ibmcloud.ibm.com", Resource: "secret"}, "secret-ibm-cloud-operator")
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              bindingName,
				Namespace:         namespace,
				DeletionTimestamp: &now,
				Finalizers:        nil, // attempt to remove finalizers
			},
			Spec:   ibmcloudv1beta1.BindingSpec{ServiceName: serviceName},
			Status: ibmcloudv1beta1.BindingStatus{State: bindingStateFailed},
		}, r.Client.(MockClient).LastUpdate())
		assert.Equal(t, nil, r.Client.(MockClient).LastStatusUpdate())
	})

	t.Run("other error", func(t *testing.T) {
		r := &BindingReconciler{
			Client: newMockClient(fake.NewFakeClientWithScheme(scheme, objects...), MockConfig{}),
			Log:    testLogger(t),
			Scheme: scheme,

			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetIBMCloudInfo: func(logt logr.Logger, r client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return nil, fmt.Errorf("failed")
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              bindingName,
				Namespace:         namespace,
				DeletionTimestamp: &now,
				Finalizers:        []string{bindingFinalizer},
			},
			Spec: ibmcloudv1beta1.BindingSpec{ServiceName: serviceName},
			Status: ibmcloudv1beta1.BindingStatus{
				State:   bindingStatePending,
				Message: "failed",
			},
		}, r.Client.(MockClient).LastStatusUpdate())
	})
}

func TestBindingDeletesWithFinalizerFailed(t *testing.T) {
	t.Parallel()
	now := metav1.Now().Rfc3339Copy() // low-resolution time

	t.Run("deleting credentials failed", func(t *testing.T) {
		scheme := schemas(t)
		const (
			namespace      = "mynamespace"
			secretName     = "mysecret"
			bindingName    = "mybinding"
			serviceName    = "myservice"
			someInstanceID = "some-instance-id"
		)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              bindingName,
					Namespace:         namespace,
					DeletionTimestamp: &now,
					Finalizers:        []string{bindingFinalizer},
				},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: serviceName,
					Alias:       "some-binding-alias", // use alias plan to mock fewer dependencies during delete creds
					SecretName:  secretName,
				},
			},
			&ibmcloudv1beta1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					InstanceID: someInstanceID,
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			},
		}
		fakeClient := newMockClient(
			fake.NewFakeClientWithScheme(scheme, objects...),
			MockConfig{DeleteErr: fmt.Errorf("failed")},
		)
		r := &BindingReconciler{
			Client: fakeClient,
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, r client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: requeueFast,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &corev1.Secret{
			TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
		}, fakeClient.LastDelete())
	})

	t.Run("removing finalizer failed", func(t *testing.T) {
		scheme := schemas(t)
		const (
			namespace      = "mynamespace"
			secretName     = "mysecret"
			bindingName    = "mybinding"
			serviceName    = "myservice"
			someInstanceID = "some-instance-id"
		)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              bindingName,
					Namespace:         namespace,
					DeletionTimestamp: &now,
					Finalizers:        []string{bindingFinalizer},
				},
				Spec: ibmcloudv1beta1.BindingSpec{
					ServiceName: serviceName,
					Alias:       "some-binding-alias", // use alias plan to mock fewer dependencies during delete creds
					SecretName:  secretName,
				},
				Status: ibmcloudv1beta1.BindingStatus{State: bindingStatePending},
			},
			&ibmcloudv1beta1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					InstanceID: someInstanceID,
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			},
		}
		var r *BindingReconciler
		r = &BindingReconciler{
			Client: fake.NewFakeClientWithScheme(scheme, objects...),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				r.Client = newMockClient( // swap out client so next update fails
					fake.NewFakeClientWithScheme(scheme, objects...),
					MockConfig{UpdateErr: fmt.Errorf("failed")},
				)
				return &ibmcloud.Info{}, nil
			},
			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              bindingName,
				Namespace:         namespace,
				DeletionTimestamp: &now,
				Finalizers:        nil, // attempt to remove finalizers
				ResourceVersion:   "1",
			},
			Spec: ibmcloudv1beta1.BindingSpec{
				ServiceName: serviceName,
				Alias:       "some-binding-alias",
				SecretName:  secretName,
			},
			Status: ibmcloudv1beta1.BindingStatus{State: bindingStatePending},
		}, r.Client.(MockClient).LastUpdate())
	})
}

func TestBindingDeletesMissingFinalizerFailed(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	const (
		namespace      = "mynamespace"
		secretName     = "mysecret"
		bindingName    = "mybinding"
		serviceName    = "myservice"
		someInstanceID = "some-instance-id"
	)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              bindingName,
				Namespace:         namespace,
				DeletionTimestamp: nil, // not deleting
				Finalizers:        nil, // AND missing finalizer
			},
			Spec:   ibmcloudv1beta1.BindingSpec{ServiceName: serviceName},
			Status: ibmcloudv1beta1.BindingStatus{State: bindingStatePending},
		},
		&ibmcloudv1beta1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1beta1.ServiceStatus{
				InstanceID: someInstanceID,
			},
		},
	}
	var r *BindingReconciler
	r = &BindingReconciler{
		Client: fake.NewFakeClientWithScheme(scheme, objects...),
		Log:    testLogger(t),
		Scheme: scheme,

		GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
			r.Client = newMockClient( // swap out client so next update fails
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{UpdateErr: fmt.Errorf("failed")},
			)
			return &ibmcloud.Info{}, nil
		},
		SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
			return nil
		},
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
	})
	assert.Equal(t, ctrl.Result{}, result)
	assert.NoError(t, err)
	assert.Equal(t, &ibmcloudv1beta1.Binding{
		TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            bindingName,
			Namespace:       namespace,
			Finalizers:      []string{bindingFinalizer}, // added a finalizer
			ResourceVersion: "1",
		},
		Spec:   ibmcloudv1beta1.BindingSpec{ServiceName: serviceName},
		Status: ibmcloudv1beta1.BindingStatus{State: bindingStatePending},
	}, r.Client.(MockClient).LastUpdate())
}

func TestBindingDeleteMismatchedServiceIDsSecretFailed(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	const (
		namespace      = "mynamespace"
		secretName     = "mysecret"
		bindingName    = "mybinding"
		serviceName    = "myservice"
		someInstanceID = "some-instance-id"
	)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       bindingName,
				Namespace:  namespace,
				Finalizers: []string{bindingFinalizer},
			},
			Spec: ibmcloudv1beta1.BindingSpec{
				ServiceName: serviceName,
				Alias:       "some-binding-alias", // use alias plan to mock fewer dependencies during delete creds
				SecretName:  secretName,
			},
			Status: ibmcloudv1beta1.BindingStatus{
				State:      bindingStatePending,
				InstanceID: "a-deleted-instance-id",
				SecretName: secretName,
			},
		},
		&ibmcloudv1beta1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1beta1.ServiceStatus{
				InstanceID: someInstanceID,
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
		},
	}
	var r *BindingReconciler
	r = &BindingReconciler{
		Client: fake.NewFakeClientWithScheme(scheme, objects...),
		Log:    testLogger(t),
		Scheme: scheme,

		GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
			r.Client = newMockClient( // swap out client so next delete fails
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{DeleteErr: fmt.Errorf("failed")},
			)
			return &ibmcloud.Info{}, nil
		},
		SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
			return nil
		},
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
	})
	assert.Equal(t, ctrl.Result{
		Requeue:      true,
		RequeueAfter: config.Get().SyncPeriod,
	}, result)
	assert.NoError(t, err)
	assert.Equal(t, &ibmcloudv1beta1.Binding{
		TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            bindingName,
			Namespace:       namespace,
			Finalizers:      []string{bindingFinalizer},
			ResourceVersion: "1",
		},
		Spec: ibmcloudv1beta1.BindingSpec{
			ServiceName: serviceName,
			Alias:       "some-binding-alias",
			SecretName:  secretName,
		},
		Status: ibmcloudv1beta1.BindingStatus{
			State:      bindingStateFailed, // should move to failed state
			Message:    "failed",
			InstanceID: "a-deleted-instance-id",
			SecretName: secretName,
		},
	}, r.Client.(MockClient).LastStatusUpdate())
	assert.Equal(t, &corev1.Secret{
		TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
	}, r.Client.(MockClient).LastDelete())
}

func TestBindingSetKeyInstanceFailed(t *testing.T) {
	t.Parallel()

	scheme := schemas(t)
	const (
		namespace       = "mynamespace"
		aliasTargetName = "myBindingToAlias"
		secretName      = "mysecret"
		bindingName     = "mybinding"
		serviceName     = "myservice"
		someInstanceID  = "some-instance-id"
	)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       bindingName,
				Namespace:  namespace,
				Finalizers: []string{bindingFinalizer},
			},
			Spec: ibmcloudv1beta1.BindingSpec{
				ServiceName: serviceName,
				SecretName:  secretName,
			},
			Status: ibmcloudv1beta1.BindingStatus{
				State:      bindingStatePending,
				InstanceID: someInstanceID,
				SecretName: secretName,
			},
		},
		&ibmcloudv1beta1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       aliasTargetName,
				Namespace:  namespace,
				Finalizers: []string{bindingFinalizer},
			},
			Spec: ibmcloudv1beta1.BindingSpec{
				ServiceName: serviceName,
				SecretName:  secretName,
			},
			Status: ibmcloudv1beta1.BindingStatus{
				State:      bindingStatePending,
				InstanceID: someInstanceID,
				SecretName: secretName,
			},
		},
		&ibmcloudv1beta1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1beta1.ServiceStatus{
				InstanceID: someInstanceID,
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
		},
	}

	for _, tc := range []struct {
		description         string
		fakeClient          MockConfig
		isAlias             bool
		instanceIDKey       bool
		createServiceKeyErr error
		expectResult        ctrl.Result
		expectState         string
		expectMessage       string
	}{
		{
			description: "update status online",
			fakeClient:  MockConfig{},
			expectResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			},
			expectState:   bindingStateOnline,
			expectMessage: bindingStateOnline,
		},
		{
			description:  "fail to update key instance ID to inProgress",
			fakeClient:   MockConfig{StatusUpdateErr: fmt.Errorf("failed")},
			expectResult: ctrl.Result{},
			expectState:  bindingStatePending,
		},
		{
			description: "missing alias instanceID annotation",
			isAlias:     true,
			fakeClient:  MockConfig{},
			expectResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			},
			expectState: bindingStatePending,
		},
		{
			description:   "update alias online",
			isAlias:       true,
			instanceIDKey: true,
			fakeClient:    MockConfig{},
			expectResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			},
			expectState:   bindingStateOnline,
			expectMessage: bindingStateOnline,
		},
		{
			description:         "fail to create credentials",
			fakeClient:          MockConfig{},
			createServiceKeyErr: fmt.Errorf("failed"),
			expectResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			},
			expectState:   bindingStateFailed,
			expectMessage: "failed",
		},
		{
			description:         "fail to create credentials - still in progress",
			fakeClient:          MockConfig{},
			createServiceKeyErr: fmt.Errorf("still in progress"),
			expectResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			},
			expectState: bindingStatePending,
		},
		{
			description: "fail to create secret",
			fakeClient:  MockConfig{CreateErr: fmt.Errorf("failed")},
			expectResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			},
			expectState:   bindingStateFailed,
			expectMessage: "failed",
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			var testObjects []runtime.Object
			for _, obj := range objects {
				if binding, ok := obj.(*ibmcloudv1beta1.Binding); ok && binding.Name != aliasTargetName {
					binding = binding.DeepCopy()
					if tc.instanceIDKey {
						binding.Annotations = map[string]string{idkey: someInstanceID}
					}
					if tc.isAlias {
						binding.Spec.Alias = aliasTargetName
					}
					obj = binding
				}
				testObjects = append(testObjects, obj)
			}

			r := &BindingReconciler{
				Client: newMockClient(
					fake.NewFakeClientWithScheme(scheme, testObjects...),
					tc.fakeClient,
				),
				Log:    testLogger(t),
				Scheme: scheme,

				GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
					return &ibmcloud.Info{}, nil
				},
				SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
					return nil
				},
				GetServiceInstanceCRN: func(session *session.Session, instanceID string) (crn.CRN, string, error) {
					return crn.CRN{}, "", nil
				},
				GetServiceName: func(session *session.Session, serviceID string) (string, error) {
					return "", nil
				},
				GetServiceRoleCRN: func(session *session.Session, serviceName, roleName string) (crn.CRN, error) {
					return crn.CRN{}, nil
				},
				CreateResourceServiceKey: func(session *session.Session, name string, crn crn.CRN, parameters map[string]interface{}) (string, map[string]interface{}, error) {
					return "", nil, tc.createServiceKeyErr
				},
				GetResourceServiceKey: func(session *session.Session, keyID string) (string, string, map[string]interface{}, error) {
					return "", aliasTargetName, nil, nil
				},
			}

			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
			})
			assert.Equal(t, tc.expectResult, result)
			assert.NoError(t, err)

			update := r.Client.(MockClient).LastStatusUpdate()
			require.IsType(t, &ibmcloudv1beta1.Binding{}, update)
			status := update.(*ibmcloudv1beta1.Binding).Status
			assert.Equal(t, tc.expectState, status.State)
			assert.Equal(t, tc.expectMessage, status.Message)
		})
	}
}
