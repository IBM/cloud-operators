package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	ibmcloudv1 "github.com/ibm/cloud-operators/api/v1"
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
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake" // nolint: staticcheck
	"sigs.k8s.io/controller-runtime/pkg/controller"
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

	var service ibmcloudv1.Service
	mustLoadObject(t, servicefile, &service, &service.ObjectMeta)
	var binding ibmcloudv1.Binding
	mustLoadObject(t, bindingfile, &binding, &binding.ObjectMeta)

	ready := t.Run("should be ready", func(t *testing.T) {
		ctx := context.TODO()

		err := k8sClient.Create(ctx, &service)
		require.NoError(t, err)

		// make sure service is online
		require.Eventually(t, verifyStatus(ctx, t, service.ObjectMeta, new(ibmcloudv1.Service), serviceStateOnline), defaultWait, defaultTick)

		// now test creation of binding
		err = k8sClient.Create(ctx, &binding)
		require.NoError(t, err)

		// check binding is online
		require.Eventually(t, verifyStatus(ctx, t, binding.ObjectMeta, new(ibmcloudv1.Binding), bindingStateOnline), defaultWait, defaultTick)
		{ // check binding owner reference
			var newBinding ibmcloudv1.Binding
			require.NoError(t, getObject(ctx, binding.ObjectMeta, &newBinding))
			for i := range newBinding.OwnerReferences {
				newBinding.OwnerReferences[i].UID = ""
			}
			assert.Equal(t, []metav1.OwnerReference{
				{
					APIVersion:         "ibmcloud.ibm.com/v1",
					Kind:               "Service",
					Name:               "test-translator-2",
					Controller:         nil, // controller must be nil. See https://github.com/IBM/cloud-operators/issues/222
					BlockOwnerDeletion: nil,
				},
			}, newBinding.OwnerReferences)
		}

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
			err := getObject(ctx, service.ObjectMeta, &ibmcloudv1.Service{})
			return errors.IsNotFound(err)
		}, defaultWait, defaultTick)
	})
}

func TestBindingFailedLookup(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	fake.NewClientBuilder().WithScheme(scheme)
	r := &BindingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Log:    testLogger(t),
		scheme: scheme,
	}

	t.Run("not found", func(t *testing.T) {
		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "mybinding"},
		})
		assert.NoError(t, err, "Don't retry (return err) if binding does not exist")
		assert.Equal(t, ctrl.Result{}, result)
	})

	r.Client = fake.NewClientBuilder().WithScheme(scheme).Build() // fail to read the type Binding
	t.Run("failed to read binding", func(t *testing.T) {
		result, err := r.Reconcile(context.Background(), ctrl.Request{
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
	objects := []client.Object{
		&ibmcloudv1.Binding{
			ObjectMeta: metav1.ObjectMeta{Name: "mybinding"},
			Status:     ibmcloudv1.BindingStatus{}, // empty
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	client = newMockClient(client, MockConfig{
		StatusUpdateErr: fmt.Errorf("failed"),
	})
	r := &BindingReconciler{
		Client: client,
		Log:    testLogger(t),
		scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "mybinding"},
	})
	assert.NoError(t, err, "Don't retry (return err) if binding does not exist")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestBindingFailGetServiceInstance(t *testing.T) {
	t.Parallel()
	now := metav1Now(t)
	for _, tc := range []struct {
		description        string
		binding            *ibmcloudv1.Binding
		fakeClient         *MockConfig
		expectUpdate       *ibmcloudv1.Binding
		expectStatusUpdate *ibmcloudv1.Binding
		expectResult       ctrl.Result
	}{
		{
			description: "no service instance",
			binding: &ibmcloudv1.Binding{
				TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "mybinding"},
				Spec: ibmcloudv1.BindingSpec{
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
			binding: &ibmcloudv1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mybinding",
					DeletionTimestamp: now,
					Finalizers:        []string{bindingFinalizer},
				},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1.BindingStatus{State: bindingStateOnline},
			},
			fakeClient: &MockConfig{},
			expectUpdate: &ibmcloudv1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mybinding",
					DeletionTimestamp: now,
				},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1.BindingStatus{State: bindingStateOnline},
			},
			expectResult: ctrl.Result{},
		},
		{
			description: "binding is deleting but update fails",
			binding: &ibmcloudv1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mybinding",
					DeletionTimestamp: now,
					Finalizers:        []string{bindingFinalizer},
				},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1.BindingStatus{State: bindingStateOnline},
			},
			fakeClient: &MockConfig{UpdateErr: fmt.Errorf("failed")},
			expectUpdate: &ibmcloudv1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mybinding",
					DeletionTimestamp: now,
				},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1.BindingStatus{State: bindingStateOnline},
			},
			expectResult: ctrl.Result{},
		},
		{
			description: "binding is deleting and status service instance is set",
			binding: &ibmcloudv1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "mybinding",
					Finalizers: []string{bindingFinalizer},
				},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1.BindingStatus{
					State:         bindingStateOnline,
					KeyInstanceID: "myinstance",
				},
			},
			fakeClient: &MockConfig{},
			expectStatusUpdate: &ibmcloudv1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "mybinding",
					Finalizers: []string{bindingFinalizer},
				},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1.BindingStatus{
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
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.binding).Build(),
				Log:    testLogger(t),
				scheme: scheme,
			}
			if tc.fakeClient != nil {
				r.Client = newMockClient(r.Client, *tc.fakeClient)
			}

			result, err := r.Reconcile(context.Background(), ctrl.Request{
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
		objects := []client.Object{
			&ibmcloudv1.Binding{
				TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "mybinding", Namespace: namespace},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: "myservice",
				},
			},
			&ibmcloudv1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: namespace},
			},
		}
		r := &BindingReconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
			Log:    testLogger(t),
			scheme: scheme,

			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return fmt.Errorf("failed")
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "mybinding", Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.EqualError(t, err, "failed")
	})

	t.Run("binding update failed", func(t *testing.T) {
		scheme := schemas(t)
		const namespace = "mynamespace"
		objects := []client.Object{
			&ibmcloudv1.Binding{
				TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "mybinding", Namespace: namespace},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: "myservice",
				},
				Status: ibmcloudv1.BindingStatus{
					State: bindingStateOnline,
				},
			},
			&ibmcloudv1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: namespace},
			},
		}
		client := newMockClient(
			fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
			MockConfig{
				UpdateErr: fmt.Errorf("failed"),
			})
		r := &BindingReconciler{
			Client: client,
			Log:    testLogger(t),
			scheme: scheme,

			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "mybinding", Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1.Binding{
			TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "mybinding", Namespace: namespace},
			Spec: ibmcloudv1.BindingSpec{
				ServiceName: "myservice",
			},
			Status: ibmcloudv1.BindingStatus{
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
		objects := []client.Object{
			&ibmcloudv1.Binding{
				TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "mybinding", Namespace: namespace},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: "myservice",
				},
			},
			&ibmcloudv1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: namespace},
				Status: ibmcloudv1.ServiceStatus{
					InstanceID: "",
				},
			},
		}
		r := &BindingReconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
			Log:    testLogger(t),
			scheme: scheme,

			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
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
		objects := []client.Object{
			&ibmcloudv1.Binding{
				TypeMeta:   metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "mybinding", Namespace: namespace},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: "myservice",
				},
			},
			&ibmcloudv1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: namespace},
				Status: ibmcloudv1.ServiceStatus{
					InstanceID: inProgress,
				},
			},
		}
		r := &BindingReconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
			Log:    testLogger(t),
			scheme: scheme,

			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
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
	now := metav1Now(t)
	scheme := schemas(t)
	const (
		namespace      = "mynamespace"
		bindingName    = "mybinding"
		serviceName    = "myservice"
		someInstanceID = "some-instance-id"
	)
	objects := []client.Object{
		&ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              bindingName,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        []string{bindingFinalizer},
			},
			Spec:   ibmcloudv1.BindingSpec{ServiceName: serviceName},
			Status: ibmcloudv1.BindingStatus{State: bindingStateFailed},
		},
		&ibmcloudv1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1.ServiceStatus{
				InstanceID: someInstanceID,
			},
		},
	}

	t.Run("not found error", func(t *testing.T) {
		var r *BindingReconciler
		r = &BindingReconciler{
			Client: newMockClient(fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(), MockConfig{}),
			Log:    testLogger(t),
			scheme: scheme,

			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				r.Client = newMockClient( // swap out client so next update fails
					fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
					MockConfig{UpdateErr: fmt.Errorf("failed")},
				)
				return nil, errors.NewNotFound(ctrl.GroupResource{Group: "ibmcloud.ibm.com", Resource: "secret"}, "ibmcloud-operator-secret")
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              bindingName,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        nil, // attempt to remove finalizers
			},
			Spec:   ibmcloudv1.BindingSpec{ServiceName: serviceName},
			Status: ibmcloudv1.BindingStatus{State: bindingStateFailed},
		}, r.Client.(MockClient).LastUpdate())
		assert.Equal(t, nil, r.Client.(MockClient).LastStatusUpdate())
	})

	t.Run("other error", func(t *testing.T) {
		r := &BindingReconciler{
			Client: newMockClient(fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(), MockConfig{}),
			Log:    testLogger(t),
			scheme: scheme,

			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetIBMCloudInfo: func(logt logr.Logger, r client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				return nil, fmt.Errorf("failed")
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              bindingName,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        []string{bindingFinalizer},
			},
			Spec: ibmcloudv1.BindingSpec{ServiceName: serviceName},
			Status: ibmcloudv1.BindingStatus{
				State:   bindingStatePending,
				Message: "failed",
			},
		}, r.Client.(MockClient).LastStatusUpdate())
	})
}

func TestBindingDeletesWithFinalizerFailed(t *testing.T) {
	t.Parallel()
	now := metav1Now(t)

	t.Run("deleting credentials failed", func(t *testing.T) {
		scheme := schemas(t)
		const (
			namespace      = "mynamespace"
			secretName     = "mysecret"
			bindingName    = "mybinding"
			serviceName    = "myservice"
			someInstanceID = "some-instance-id"
		)
		objects := []client.Object{
			&ibmcloudv1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              bindingName,
					Namespace:         namespace,
					DeletionTimestamp: now,
					Finalizers:        []string{bindingFinalizer},
				},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: serviceName,
					Alias:       "some-binding-alias", // use alias plan to mock fewer dependencies during delete creds
					SecretName:  secretName,
				},
			},
			&ibmcloudv1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1.ServiceStatus{
					InstanceID: someInstanceID,
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			},
		}
		fakeClient := newMockClient(
			fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
			MockConfig{DeleteErr: fmt.Errorf("failed")},
		)
		r := &BindingReconciler{
			Client: fakeClient,
			Log:    testLogger(t),
			scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, r client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
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
		objects := []client.Object{
			&ibmcloudv1.Binding{
				TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              bindingName,
					Namespace:         namespace,
					DeletionTimestamp: now,
					Finalizers:        []string{bindingFinalizer},
				},
				Spec: ibmcloudv1.BindingSpec{
					ServiceName: serviceName,
					Alias:       "some-binding-alias", // use alias plan to mock fewer dependencies during delete creds
					SecretName:  secretName,
				},
				Status: ibmcloudv1.BindingStatus{State: bindingStatePending},
			},
			&ibmcloudv1.Service{
				TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1.ServiceStatus{
					InstanceID: someInstanceID,
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			},
		}
		var r *BindingReconciler
		r = &BindingReconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
			Log:    testLogger(t),
			scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				r.Client = newMockClient( // swap out client so next update fails
					fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
					MockConfig{UpdateErr: fmt.Errorf("failed")},
				)
				return &ibmcloud.Info{}, nil
			},
			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              bindingName,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        nil, // attempt to remove finalizers
				ResourceVersion:   "1",
			},
			Spec: ibmcloudv1.BindingSpec{
				ServiceName: serviceName,
				Alias:       "some-binding-alias",
				SecretName:  secretName,
			},
			Status: ibmcloudv1.BindingStatus{State: bindingStatePending},
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
	objects := []client.Object{
		&ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              bindingName,
				Namespace:         namespace,
				DeletionTimestamp: nil, // not deleting
				Finalizers:        nil, // AND missing finalizer
			},
			Spec:   ibmcloudv1.BindingSpec{ServiceName: serviceName},
			Status: ibmcloudv1.BindingStatus{State: bindingStatePending},
		},
		&ibmcloudv1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1.ServiceStatus{
				InstanceID: someInstanceID,
			},
		},
	}
	var r *BindingReconciler
	r = &BindingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
		Log:    testLogger(t),
		scheme: scheme,

		GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
			r.Client = newMockClient( // swap out client so next update fails
				fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
				MockConfig{UpdateErr: fmt.Errorf("failed")},
			)
			return &ibmcloud.Info{}, nil
		},
		SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
			return nil
		},
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
	})
	assert.Equal(t, ctrl.Result{}, result)
	assert.NoError(t, err)
	assert.Equal(t, &ibmcloudv1.Binding{
		TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            bindingName,
			Namespace:       namespace,
			Finalizers:      []string{bindingFinalizer}, // added a finalizer
			ResourceVersion: "1",
		},
		Spec:   ibmcloudv1.BindingSpec{ServiceName: serviceName},
		Status: ibmcloudv1.BindingStatus{State: bindingStatePending},
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
	objects := []client.Object{
		&ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       bindingName,
				Namespace:  namespace,
				Finalizers: []string{bindingFinalizer},
			},
			Spec: ibmcloudv1.BindingSpec{
				ServiceName: serviceName,
				Alias:       "some-binding-alias", // use alias plan to mock fewer dependencies during delete creds
				SecretName:  secretName,
			},
			Status: ibmcloudv1.BindingStatus{
				State:      bindingStatePending,
				InstanceID: "a-deleted-instance-id",
				SecretName: secretName,
			},
		},
		&ibmcloudv1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1.ServiceStatus{
				InstanceID: someInstanceID,
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
		},
	}
	var r *BindingReconciler
	r = &BindingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
		Log:    testLogger(t),
		scheme: scheme,

		GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
			r.Client = newMockClient( // swap out client so next delete fails
				fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
				MockConfig{DeleteErr: fmt.Errorf("failed")},
			)
			return &ibmcloud.Info{}, nil
		},
		SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
			return nil
		},
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
	})
	assert.Equal(t, ctrl.Result{
		Requeue:      true,
		RequeueAfter: config.Get().SyncPeriod,
	}, result)
	assert.NoError(t, err)
	assert.Equal(t, &ibmcloudv1.Binding{
		TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            bindingName,
			Namespace:       namespace,
			Finalizers:      []string{bindingFinalizer},
			ResourceVersion: "1",
		},
		Spec: ibmcloudv1.BindingSpec{
			ServiceName: serviceName,
			Alias:       "some-binding-alias",
			SecretName:  secretName,
		},
		Status: ibmcloudv1.BindingStatus{
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
	objects := []client.Object{
		&ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       bindingName,
				Namespace:  namespace,
				Finalizers: []string{bindingFinalizer},
			},
			Spec: ibmcloudv1.BindingSpec{
				ServiceName: serviceName,
				SecretName:  secretName,
			},
			Status: ibmcloudv1.BindingStatus{
				State:      bindingStatePending,
				InstanceID: someInstanceID,
				SecretName: secretName,
			},
		},
		&ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       aliasTargetName,
				Namespace:  namespace,
				Finalizers: []string{bindingFinalizer},
			},
			Spec: ibmcloudv1.BindingSpec{
				ServiceName: serviceName,
				SecretName:  secretName,
			},
			Status: ibmcloudv1.BindingStatus{
				State:      bindingStatePending,
				InstanceID: someInstanceID,
				SecretName: secretName,
			},
		},
		&ibmcloudv1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1.ServiceStatus{
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
			var testObjects []client.Object
			for _, obj := range objects {
				if binding, ok := obj.(*ibmcloudv1.Binding); ok && binding.Name != aliasTargetName {
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
					fake.NewClientBuilder().WithScheme(scheme).WithObjects(testObjects...).Build(),
					tc.fakeClient,
				),
				Log:    testLogger(t),
				scheme: scheme,

				GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
					return &ibmcloud.Info{}, nil
				},
				SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
					return nil
				},
				SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
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

			result, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
			})
			assert.Equal(t, tc.expectResult, result)
			assert.NoError(t, err)

			update := r.Client.(MockClient).LastStatusUpdate()
			require.IsType(t, &ibmcloudv1.Binding{}, update)
			status := update.(*ibmcloudv1.Binding).Status
			assert.Equal(t, tc.expectState, status.State)
			assert.Equal(t, tc.expectMessage, status.Message)
		})
	}
}

func TestBindingEnsureCredentialsFailed(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	const (
		namespace         = "mynamespace"
		secretName        = "mysecret"
		bindingName       = "mybinding"
		serviceName       = "myservice"
		someInstanceID    = "some-instance-id"
		someKeyInstanceID = "some-key-instance-id"
	)
	objects := []client.Object{
		&ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       bindingName,
				Namespace:  namespace,
				Finalizers: []string{bindingFinalizer},
			},
			Spec: ibmcloudv1.BindingSpec{
				ServiceName: serviceName,
				SecretName:  secretName,
			},
			Status: ibmcloudv1.BindingStatus{
				State:         bindingStatePending,
				InstanceID:    someInstanceID,
				SecretName:    secretName,
				KeyInstanceID: someKeyInstanceID,
			},
		},
		&ibmcloudv1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1.ServiceStatus{
				InstanceID: someInstanceID,
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
		},
	}

	r := &BindingReconciler{
		Client: newMockClient(
			fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
			MockConfig{},
		),
		Log:    testLogger(t),
		scheme: scheme,

		GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
			return &ibmcloud.Info{}, nil
		},
		SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
			return nil
		},
		GetResourceServiceKey: func(session *session.Session, keyID string) (string, string, map[string]interface{}, error) {
			return "", "", nil, fmt.Errorf(notFound)
		},
		GetServiceInstanceCRN: func(session *session.Session, instanceID string) (instanceCRN crn.CRN, serviceID string, err error) {
			return crn.CRN{}, "", nil
		},
		GetServiceName: func(session *session.Session, serviceID string) (string, error) {
			return "", nil
		},
		GetServiceRoleCRN: func(session *session.Session, serviceName, roleName string) (crn.CRN, error) {
			return crn.CRN{}, nil
		},
		CreateResourceServiceKey: func(session *session.Session, name string, crn crn.CRN, parameters map[string]interface{}) (string, map[string]interface{}, error) {
			return "", nil, fmt.Errorf("failed")
		},
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
	})
	assert.Equal(t, ctrl.Result{
		Requeue:      true,
		RequeueAfter: config.Get().SyncPeriod,
	}, result)
	assert.NoError(t, err)
	update := r.Client.(MockClient).LastStatusUpdate()
	require.IsType(t, &ibmcloudv1.Binding{}, update)
	status := update.(*ibmcloudv1.Binding).Status
	assert.Equal(t, bindingStateFailed, status.State)
	assert.Equal(t, "failed", status.Message)
}

func TestBindingEnsureAliasCredentialsFailed(t *testing.T) {
	t.Parallel()
	const (
		namespace         = "mynamespace"
		aliasTargetName   = "myBindingToAlias"
		secretName        = "mysecret"
		bindingName       = "mybinding"
		serviceName       = "myservice"
		someInstanceID    = "some-instance-id"
		someKeyInstanceID = "some-key-instance-id"
	)
	objects := []client.Object{
		&ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:        bindingName,
				Namespace:   namespace,
				Finalizers:  []string{bindingFinalizer},
				Annotations: map[string]string{idkey: someInstanceID},
			},
			Spec: ibmcloudv1.BindingSpec{
				ServiceName: serviceName,
				SecretName:  secretName,
				Alias:       aliasTargetName,
			},
			Status: ibmcloudv1.BindingStatus{
				State:         bindingStatePending,
				InstanceID:    someInstanceID,
				SecretName:    secretName,
				KeyInstanceID: someKeyInstanceID,
			},
		},
		&ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       aliasTargetName,
				Namespace:  namespace,
				Finalizers: []string{bindingFinalizer},
			},
			Spec: ibmcloudv1.BindingSpec{
				ServiceName: serviceName,
				SecretName:  secretName,
			},
			Status: ibmcloudv1.BindingStatus{
				State:         bindingStatePending,
				InstanceID:    someInstanceID,
				SecretName:    secretName,
				KeyInstanceID: someKeyInstanceID,
			},
		},
		&ibmcloudv1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1.ServiceStatus{
				InstanceID: someInstanceID,
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
		},
	}

	t.Run("reset if aliased creds don't exist", func(t *testing.T) {
		scheme := schemas(t)
		r := &BindingReconciler{
			Client: newMockClient(
				fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
				MockConfig{},
			),
			Log:    testLogger(t),
			scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetResourceServiceKey: func(session *session.Session, keyID string) (string, string, map[string]interface{}, error) {
				return "", "", nil, fmt.Errorf(notFound)
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		update := r.Client.(MockClient).LastStatusUpdate()
		require.IsType(t, &ibmcloudv1.Binding{}, update)
		status := update.(*ibmcloudv1.Binding).Status
		assert.Equal(t, bindingStatePending, status.State)
		assert.Equal(t, "Processing Resource", status.Message)
	})

	t.Run("other error", func(t *testing.T) {
		scheme := schemas(t)
		r := &BindingReconciler{
			Client: newMockClient(
				fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
				MockConfig{},
			),
			Log:    testLogger(t),
			scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetResourceServiceKey: func(session *session.Session, keyID string) (string, string, map[string]interface{}, error) {
				return "", "", nil, fmt.Errorf("failed")
			},
			GetServiceInstanceCRN: func(session *session.Session, instanceID string) (instanceCRN crn.CRN, serviceID string, err error) {
				return crn.CRN{}, "", nil
			},
			GetServiceName: func(session *session.Session, serviceID string) (string, error) {
				return "", nil
			},
			GetServiceRoleCRN: func(session *session.Session, serviceName, roleName string) (crn.CRN, error) {
				return crn.CRN{}, nil
			},
			CreateResourceServiceKey: func(session *session.Session, name string, crn crn.CRN, parameters map[string]interface{}) (string, map[string]interface{}, error) {
				return "", nil, fmt.Errorf("failed")
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		update := r.Client.(MockClient).LastStatusUpdate()
		require.IsType(t, &ibmcloudv1.Binding{}, update)
		status := update.(*ibmcloudv1.Binding).Status
		assert.Equal(t, bindingStateFailed, status.State)
		assert.Equal(t, "failed", status.Message)
	})
}

func TestBindingEnsureSecretFailed(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	const (
		namespace         = "mynamespace"
		secretName        = "mysecret"
		bindingName       = "mybinding"
		serviceName       = "myservice"
		someInstanceID    = "some-instance-id"
		someKeyInstanceID = "some-key-instance-id"
	)
	objects := []client.Object{
		&ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       bindingName,
				Namespace:  namespace,
				Finalizers: []string{bindingFinalizer},
			},
			Spec: ibmcloudv1.BindingSpec{
				ServiceName: serviceName,
				SecretName:  secretName,
			},
			Status: ibmcloudv1.BindingStatus{
				State:         bindingStatePending,
				InstanceID:    someInstanceID,
				SecretName:    secretName,
				KeyInstanceID: someKeyInstanceID,
			},
		},
		&ibmcloudv1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1.ServiceStatus{
				InstanceID: someInstanceID,
			},
		},
	}

	t.Run("recreate secret success", func(t *testing.T) {
		r := &BindingReconciler{
			Client: newMockClient(
				fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
				MockConfig{},
			),
			Log:    testLogger(t),
			scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetResourceServiceKey: func(session *session.Session, keyID string) (string, string, map[string]interface{}, error) {
				return "", "", nil, nil
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					"service-instance-id": someInstanceID,
					"service-key-id":      someKeyInstanceID,
					"bindingFromName":     serviceName,
				},
			},
			Data: map[string][]byte{}, // TODO(johnstarich): validate key contents
		}, r.Client.(MockClient).LastCreate())

		update := r.Client.(MockClient).LastStatusUpdate()
		require.IsType(t, &ibmcloudv1.Binding{}, update)
		status := update.(*ibmcloudv1.Binding).Status
		assert.Equal(t, bindingStateOnline, status.State)
		assert.Equal(t, bindingStateOnline, status.Message)
	})

	t.Run("recreate secret failure", func(t *testing.T) {
		r := &BindingReconciler{
			Client: newMockClient(
				fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
				MockConfig{CreateErr: fmt.Errorf("failed")},
			),
			Log:    testLogger(t),
			scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetResourceServiceKey: func(session *session.Session, keyID string) (string, string, map[string]interface{}, error) {
				return "", "", nil, nil
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					"service-instance-id": someInstanceID,
					"service-key-id":      someKeyInstanceID,
					"bindingFromName":     serviceName,
				},
			},
			Data: map[string][]byte{}, // TODO(johnstarich): validate key contents
		}, r.Client.(MockClient).LastCreate())

		update := r.Client.(MockClient).LastStatusUpdate()
		require.IsType(t, &ibmcloudv1.Binding{}, update)
		status := update.(*ibmcloudv1.Binding).Status
		assert.Equal(t, bindingStateFailed, status.State)
		assert.Equal(t, "failed", status.Message)
	})
}

func TestBindingEnsureKeyContentsFailed(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	const (
		namespace         = "mynamespace"
		secretName        = "mysecret"
		bindingName       = "mybinding"
		serviceName       = "myservice"
		someInstanceID    = "some-instance-id"
		someKeyInstanceID = "some-key-instance-id"
	)
	objects := []client.Object{
		&ibmcloudv1.Binding{
			TypeMeta: metav1.TypeMeta{Kind: "Binding", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       bindingName,
				Namespace:  namespace,
				Finalizers: []string{bindingFinalizer},
			},
			Spec: ibmcloudv1.BindingSpec{
				ServiceName: serviceName,
				SecretName:  secretName,
			},
			Status: ibmcloudv1.BindingStatus{
				State:         bindingStatePending,
				InstanceID:    someInstanceID,
				SecretName:    secretName,
				KeyInstanceID: someKeyInstanceID,
			},
		},
		&ibmcloudv1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1.ServiceStatus{
				InstanceID: someInstanceID,
			},
		},
	}

	t.Run("update key contents success", func(t *testing.T) {
		keyContents := map[string]interface{}{
			"hello": "world",
		}
		testObjects := append(
			objects,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
					Annotations: map[string]string{
						"service-key-id": "some-old-service-key-id",
					},
				},
				Data: map[string][]byte{
					"hello": []byte("world"),
				},
			},
		)
		r := &BindingReconciler{
			Client: newMockClient(
				fake.NewClientBuilder().WithScheme(scheme).WithObjects(testObjects...).Build(),
				MockConfig{},
			),
			Log:    testLogger(t),
			scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetResourceServiceKey: func(session *session.Session, keyID string) (string, string, map[string]interface{}, error) {
				return "", "", keyContents, nil
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)

		assert.Equal(t, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					"service-instance-id": someInstanceID,
					"service-key-id":      someKeyInstanceID,
					"bindingFromName":     serviceName,
				},
			},
			Data: map[string][]byte{
				"hello": []byte("world"),
			},
		}, r.Client.(MockClient).LastCreate())

		update := r.Client.(MockClient).LastStatusUpdate()
		require.IsType(t, &ibmcloudv1.Binding{}, update)
		status := update.(*ibmcloudv1.Binding).Status
		assert.Equal(t, bindingStateOnline, status.State)
		assert.Equal(t, bindingStateOnline, status.Message)
	})

	t.Run("key is up to date", func(t *testing.T) {
		keyContents := map[string]interface{}{
			"hello": "world",
		}
		testObjects := append(
			objects,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
					Annotations: map[string]string{
						"service-key-id": someKeyInstanceID,
					},
				},
				Data: map[string][]byte{
					"hello": []byte("world"),
				},
			},
		)
		r := &BindingReconciler{
			Client: newMockClient(
				fake.NewClientBuilder().WithScheme(scheme).WithObjects(testObjects...).Build(),
				MockConfig{},
			),
			Log:    testLogger(t),
			scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetResourceServiceKey: func(session *session.Session, keyID string) (string, string, map[string]interface{}, error) {
				return "", "", keyContents, nil
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)

		assert.Nil(t, r.Client.(MockClient).LastCreate())

		update := r.Client.(MockClient).LastStatusUpdate()
		require.IsType(t, &ibmcloudv1.Binding{}, update)
		status := update.(*ibmcloudv1.Binding).Status
		assert.Equal(t, bindingStateOnline, status.State)
		assert.Equal(t, bindingStateOnline, status.Message)
	})

	t.Run("update key contents delete failed", func(t *testing.T) {
		keyContents := map[string]interface{}{
			"hello": "world",
		}
		testObjects := append(
			objects,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			},
		)
		r := &BindingReconciler{
			Client: newMockClient(
				fake.NewClientBuilder().WithScheme(scheme).WithObjects(testObjects...).Build(),
				MockConfig{DeleteErr: fmt.Errorf("failed")},
			),
			Log:    testLogger(t),
			scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetResourceServiceKey: func(session *session.Session, keyID string) (string, string, map[string]interface{}, error) {
				return "", "", keyContents, nil
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)

		assert.Equal(t, &corev1.Secret{
			TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
		}, r.Client.(MockClient).LastDelete())

		update := r.Client.(MockClient).LastStatusUpdate()
		require.IsType(t, &ibmcloudv1.Binding{}, update)
		status := update.(*ibmcloudv1.Binding).Status
		assert.Equal(t, bindingStateFailed, status.State)
		assert.Equal(t, "failed", status.Message)
	})

	t.Run("update key contents create failed", func(t *testing.T) {
		keyContents := map[string]interface{}{
			"hello": "world",
		}
		testObjects := append(
			objects,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			},
		)
		r := &BindingReconciler{
			Client: newMockClient(
				fake.NewClientBuilder().WithScheme(scheme).WithObjects(testObjects...).Build(),
				MockConfig{CreateErr: fmt.Errorf("failed")},
			),
			Log:    testLogger(t),
			scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			SetControllerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			SetOwnerReference: func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
				return nil
			},
			GetResourceServiceKey: func(session *session.Session, keyID string) (string, string, map[string]interface{}, error) {
				return "", "", keyContents, nil
			},
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)

		assert.Equal(t, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					"service-instance-id": someInstanceID,
					"service-key-id":      someKeyInstanceID,
					"bindingFromName":     serviceName,
				},
			},
			Data: map[string][]byte{
				"hello": []byte("world"),
			},
		}, r.Client.(MockClient).LastCreate())

		update := r.Client.(MockClient).LastStatusUpdate()
		require.IsType(t, &ibmcloudv1.Binding{}, update)
		status := update.(*ibmcloudv1.Binding).Status
		assert.Equal(t, bindingStateFailed, status.State)
		assert.Equal(t, "failed", status.Message)
	})
}

func TestBindingResetResource(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	const (
		secretName = "mysecret"
		namespace  = "mynamespace"
	)
	binding := &ibmcloudv1.Binding{
		ObjectMeta: metav1.ObjectMeta{Name: "mybinding", Namespace: namespace},
		Spec: ibmcloudv1.BindingSpec{
			SecretName: secretName,
		},
	}
	secret := &corev1.Secret{
		TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
	}

	t.Run("happy path", func(t *testing.T) {
		client := newMockClient(
			fake.NewClientBuilder().WithScheme(scheme).WithObjects(binding, secret).Build(),
			MockConfig{},
		)
		r := &BindingReconciler{
			Client: client,
			Log:    testLogger(t),
			scheme: scheme,
		}

		result, err := r.resetResource(binding)
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, secret, client.LastDelete())
		assert.Equal(t, binding, client.LastStatusUpdate())
	})

	t.Run("fail delete secret", func(t *testing.T) {
		client := newMockClient(
			fake.NewClientBuilder().WithScheme(scheme).WithObjects(binding, secret).Build(),
			MockConfig{DeleteErr: fmt.Errorf("failed")},
		)
		r := &BindingReconciler{
			Client: client,
			Log:    testLogger(t),
			scheme: scheme,
		}

		result, err := r.resetResource(binding)
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, secret, client.LastDelete())
	})

	t.Run("fail update status", func(t *testing.T) {
		client := newMockClient(
			fake.NewClientBuilder().WithScheme(scheme).WithObjects(binding, secret).Build(),
			MockConfig{StatusUpdateErr: fmt.Errorf("failed")},
		)
		r := &BindingReconciler{
			Client: client,
			Log:    testLogger(t),
			scheme: scheme,
		}

		result, err := r.resetResource(binding)
		assert.Equal(t, ctrl.Result{}, result)
		assert.NoError(t, err)
		assert.Equal(t, binding, client.LastStatusUpdate())
	})
}

func TestBindingUpdateStatusError(t *testing.T) {
	for _, tc := range []struct {
		description       string
		initialState      string
		initialMessage    string
		state             string
		err               error
		updateStatusError error
		expectState       string
		expectMessage     string
		expectResult      ctrl.Result
	}{
		{
			description:   "happy path",
			initialState:  bindingStatePending,
			state:         bindingStateFailed,
			err:           fmt.Errorf("failed"),
			expectState:   bindingStateFailed,
			expectMessage: "failed",
			expectResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			},
		},
		{
			description:  "no such host error",
			initialState: bindingStatePending,
			state:        bindingStateFailed,
			err:          fmt.Errorf("no such host"),
			expectResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: 5 * time.Minute,
			},
		},
		{
			description:    "happy path - same state",
			initialState:   bindingStatePending,
			initialMessage: "old message",
			state:          bindingStatePending,
			err:            fmt.Errorf("failed"),
			expectResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			},
		},
		{
			description:       "status updated failed",
			initialState:      bindingStatePending,
			state:             bindingStateFailed,
			err:               fmt.Errorf("failed"),
			updateStatusError: fmt.Errorf("failed status"),
			expectState:       bindingStateFailed,
			expectMessage:     "failed",
			expectResult:      ctrl.Result{},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			scheme := schemas(t)
			binding := &ibmcloudv1.Binding{
				Status: ibmcloudv1.BindingStatus{
					State:   tc.initialState,
					Message: tc.initialMessage,
				},
			}
			client := newMockClient(
				fake.NewClientBuilder().WithScheme(scheme).Build(),
				MockConfig{StatusUpdateErr: tc.updateStatusError},
			)
			r := &BindingReconciler{
				Client: client,
				Log:    testLogger(t),
				scheme: scheme,
			}

			result, err := r.updateStatusError(binding, tc.state, tc.err)
			assert.Equal(t, tc.expectResult, result)
			assert.NoError(t, err)
			var expectBinding runtime.Object
			if tc.expectState != "" {
				expectBinding = &ibmcloudv1.Binding{
					Status: ibmcloudv1.BindingStatus{
						State:   tc.expectState,
						Message: tc.expectMessage,
					},
				}
			}
			assert.Equal(t, expectBinding, client.LastStatusUpdate())
		})
	}
}

func TestBindingParamToJSON(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		description string
		param       ibmcloudv1.Param
		expectJSON  map[string]interface{}
		expectErr   string
	}{
		{
			description: "error: value and valueFrom both set",
			param: ibmcloudv1.Param{
				Name:      "myvalue",
				Value:     &ibmcloudv1.ParamValue{},
				ValueFrom: &ibmcloudv1.ParamSource{},
			},
			expectErr: "Value and ValueFrom properties are mutually exclusive (for myvalue variable)",
		},
		{
			description: "empty valueFrom error",
			param: ibmcloudv1.Param{
				Name:      "myvalue",
				ValueFrom: &ibmcloudv1.ParamSource{},
			},
			expectErr: "Missing secretKeyRef or configMapKeyRef",
		},
		{
			description: "empty value error",
			param: ibmcloudv1.Param{
				Name:  "myvalue",
				Value: &ibmcloudv1.ParamValue{},
			},
			expectErr: "unexpected end of JSON input",
		},
		{
			description: "value happy path",
			param: ibmcloudv1.Param{
				Name: "myvalue",
				Value: &ibmcloudv1.ParamValue{
					RawMessage: json.RawMessage(`{"hello": true, "world": {"!": 1}}`),
				},
			},
			expectJSON: map[string]interface{}{
				"hello": true,
				"world": map[string]interface{}{
					"!": 1.0,
				},
			},
		},
		{
			description: "neither value nor valueFrom set",
			param:       ibmcloudv1.Param{Name: "myvalue"},
			expectJSON:  nil,
			expectErr:   "",
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			r := &BindingReconciler{}
			j, err := r.paramToJSON(context.TODO(), tc.param, "someNamespace")
			if tc.expectErr != "" {
				assert.EqualError(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			if tc.expectJSON == nil {
				assert.Nil(t, j)
			} else {
				assert.Equal(t, tc.expectJSON, j)
			}
		})
	}
}

func TestBindingParamValueToJSON(t *testing.T) {
	t.Parallel()
	const (
		secretName     = "secretName"
		secretKey      = "mykey"
		secretValue    = "myvalue"
		configMapName  = "configMapName"
		configMapKey   = "mykey"
		configMapValue = "myvalue"
		namespace      = "mynamespace"
	)

	objects := []client.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			Data: map[string][]byte{
				secretKey: []byte(secretValue),
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: configMapName, Namespace: namespace},
			Data: map[string]string{
				configMapKey: configMapValue,
			},
		},
	}

	for _, tc := range []struct {
		description string
		valueFrom   ibmcloudv1.ParamSource
		expectJSON  interface{}
		expectErr   string
	}{
		{
			description: "no value error",
			expectErr:   "Missing secretKeyRef or configMapKeyRef",
		},
		{
			description: "secret ref success",
			valueFrom: ibmcloudv1.ParamSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: secretKey,
				},
			},
			expectJSON: secretValue,
		},
		{
			description: "secret ref name failure",
			valueFrom: ibmcloudv1.ParamSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "wrong-secret-name",
					},
					Key: secretKey,
				},
			},
			expectErr: "Missing secret wrong-secret-name",
		},
		{
			description: "secret ref key failure",
			valueFrom: ibmcloudv1.ParamSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: "wrong-key-name",
				},
			},
			expectJSON: "",
		},
		{
			description: "configmap ref success",
			valueFrom: ibmcloudv1.ParamSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
					Key: configMapKey,
				},
			},
			expectJSON: configMapValue,
		},
		{
			description: "configmap ref name failure",
			valueFrom: ibmcloudv1.ParamSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "wrong-configmap-name",
					},
					Key: configMapKey,
				},
			},
			expectErr: "Missing configmap wrong-configmap-name",
		},
		{
			description: "configmap ref key failure",
			valueFrom: ibmcloudv1.ParamSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
					Key: "wrong-key-name",
				},
			},
			expectJSON: "",
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			scheme := schemas(t)
			r := &BindingReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
				Log:    testLogger(t),
				scheme: scheme,
			}

			j, err := r.paramValueToJSON(context.TODO(), tc.valueFrom, namespace)
			if tc.expectErr != "" {
				assert.EqualError(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expectJSON, j)
		})
	}
}

func TestParamToJSONFromString(t *testing.T) {
	t.Parallel()
	t.Run("unmarshal happy path", func(t *testing.T) {
		j, err := paramToJSONFromString(`{
			"hello": 1,
			"world": 1.234567890987654321e1000,
			"!": false
		}`)
		assert.NoError(t, err)
		assert.Equal(t, map[string]interface{}{
			"hello": json.Number("1"),
			"world": json.Number("1.234567890987654321e1000"), // large precision is kept identically as a Number type
			"!":     false,
		}, j)
	})

	t.Run("too many JSON items", func(t *testing.T) {
		const contents = `
{ "hello": "abc" }
{ "world": "123" }
`
		j, err := paramToJSONFromString(contents)
		assert.NoError(t, err)
		assert.Equal(t, contents, j)
	})

	t.Run("invalid JSON is not parsed", func(t *testing.T) {
		const contents = `this is not JSON`
		j, err := paramToJSONFromString(contents)
		assert.NoError(t, err)
		assert.Equal(t, contents, j)
	})
}

func TestDeleteBindingFinalizer(t *testing.T) {
	t.Parallel()
	t.Run("no finalizer found", func(t *testing.T) {
		finalizers := []string(nil)
		assert.Equal(t, finalizers, deleteBindingFinalizer(&ibmcloudv1.Binding{
			ObjectMeta: metav1.ObjectMeta{Finalizers: finalizers},
		}))
	})

	t.Run("one other finalizer found", func(t *testing.T) {
		finalizers := []string{"not-binding-finalizer"}
		assert.Equal(t, finalizers, deleteBindingFinalizer(&ibmcloudv1.Binding{
			ObjectMeta: metav1.ObjectMeta{Finalizers: finalizers},
		}))
	})

	t.Run("one finalizer found", func(t *testing.T) {
		finalizers := []string{bindingFinalizer}
		assert.Equal(t, []string(nil), deleteBindingFinalizer(&ibmcloudv1.Binding{
			ObjectMeta: metav1.ObjectMeta{Finalizers: finalizers},
		}))
	})

	t.Run("multiple finalizers found", func(t *testing.T) {
		finalizers := []string{bindingFinalizer, bindingFinalizer}
		assert.Equal(t, []string(nil), deleteBindingFinalizer(&ibmcloudv1.Binding{
			ObjectMeta: metav1.ObjectMeta{Finalizers: finalizers},
		}))
	})
}

func TestBindingDeleteCredentials(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	binding := &ibmcloudv1.Binding{
		ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: "mynamespace"},
		Spec: ibmcloudv1.BindingSpec{
			Alias: "", // not an alias, so should delete IBM Cloud resources
		},
	}
	secret := &corev1.Secret{
		TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: "mynamespace"},
	}
	objects := []client.Object{
		binding,
		&ibmcloudv1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: "mynamespace"},
			Spec:       ibmcloudv1.ServiceSpec{},
		},
		secret,
	}

	for _, tc := range []struct {
		description      string
		serviceClassType string
		cfErr            error
		resourceErr      error
		expectDelete     runtime.Object
		deleteErr        error
		expectErr        string
	}{
		{
			description:  "delete resource service",
			resourceErr:  nil,
			expectDelete: secret,
		},
		{
			description:      "delete CF service",
			serviceClassType: "CF",
			cfErr:            nil,
			expectDelete:     secret,
		},
		{
			description: "fail delete resource service",
			resourceErr: fmt.Errorf("failed"),
			expectErr:   "failed",
		},
		{
			description:      "fail delete CF service",
			serviceClassType: "CF",
			cfErr:            fmt.Errorf("failed"),
			expectErr:        "failed",
		},
		{
			description:  "fail delete secret",
			deleteErr:    fmt.Errorf("failed"),
			expectErr:    "failed",
			expectDelete: secret,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			client := newMockClient(
				fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
				MockConfig{DeleteErr: tc.deleteErr},
			)
			r := &BindingReconciler{
				Client: client,
				Log:    testLogger(t),
				scheme: scheme,

				DeleteCFServiceKey: func(session *session.Session, serviceKeyGUID string) error {
					return tc.cfErr
				},
				DeleteResourceServiceKey: func(session *session.Session, serviceKeyGUID string) error {
					return tc.resourceErr
				},
			}
			err := r.deleteCredentials(nil, binding, tc.serviceClassType)
			assert.Equal(t, tc.expectDelete, client.LastDelete())
			if tc.expectErr != "" {
				assert.EqualError(t, err, tc.expectErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestBindingUpdateStatusOnlineFailedWithConflictError(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	binding := &ibmcloudv1.Binding{
		ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: "mynamespace"},
		Spec:       ibmcloudv1.BindingSpec{},
	}
	service := &ibmcloudv1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: "mynamespace"},
		Spec:       ibmcloudv1.ServiceSpec{},
	}

	errChan := make(chan error, retry.DefaultRetry.Steps)

	conflictErr := k8sErrors.StatusError{
		ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Code:   409,
			Reason: metav1.StatusReasonConflict,
		},
	}

	client := newMockClient(
		fake.NewClientBuilder().WithScheme(scheme).WithObjects(binding, service).Build(),
		MockConfig{ErrChan: errChan},
	)
	r := &BindingReconciler{
		Client: client,
		Log:    testLogger(t),
		scheme: scheme,

		DeleteResourceServiceKey: func(session *session.Session, keyID string) error {
			return fmt.Errorf("failed")
		},
	}

	//It return conflict error at first so retry will be triggered and succeed with no error and the function will succeed
	errChan <- &conflictErr
	errChan <- nil
	result, err := r.updateStatusOnline(nil, binding)
	assert.Equal(t, ctrl.Result{
		Requeue:      true,
		RequeueAfter: config.Get().SyncPeriod,
	}, result)
	assert.NoError(t, err)

	//It keeps returning conflict error so retry will fail
	for i := 0; i < retry.DefaultRetry.Steps; i++ {
		errChan <- &conflictErr

	}
	result, err = r.updateStatusOnline(nil, binding)
	assert.Equal(t, ctrl.Result{
		Requeue: true,
	}, result)
	assert.Error(t, err)
}

func TestBindingUpdateStatusOnlineFailedWithOtherUpdateErrror(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	binding := &ibmcloudv1.Binding{
		ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: "mynamespace"},
		Spec:       ibmcloudv1.BindingSpec{},
	}
	service := &ibmcloudv1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: "mynamespace"},
		Spec:       ibmcloudv1.ServiceSpec{},
	}

	errChan := make(chan error, retry.DefaultRetry.Steps)

	errChan <- fmt.Errorf("status failed")
	client := newMockClient(
		fake.NewClientBuilder().WithScheme(scheme).WithObjects(binding, service).Build(),
		MockConfig{ErrChan: errChan},
	)
	r := &BindingReconciler{
		Client: client,
		Log:    testLogger(t),
		scheme: scheme,
	}

	result, err := r.updateStatusOnline(nil, binding)
	assert.Equal(t, ctrl.Result{
		Requeue: true,
	}, result)
	assert.EqualError(t, err, "status failed")
}

func TestBindingUpdateStatusOnlineFailedWithGetError(t *testing.T) {
	t.Parallel()
	scheme := schemas(t)
	binding := &ibmcloudv1.Binding{
		ObjectMeta: metav1.ObjectMeta{Name: "myservice", Namespace: "mynamespace"},
		Spec:       ibmcloudv1.BindingSpec{},
	}
	errChan := make(chan error, retry.DefaultRetry.Steps)
	//there will be no error when do updating
	errChan <- nil
	client := newMockClient(
		// the service and binding does not add so Get will return error
		fake.NewClientBuilder().WithScheme(scheme).Build(),
		MockConfig{ErrChan: errChan},
	)
	r := &BindingReconciler{
		Client: client,
		Log:    testLogger(t),
		scheme: scheme,
	}

	result, err := r.updateStatusOnline(nil, binding)
	assert.Equal(t, ctrl.Result{
		Requeue: true,
	}, result)
	assert.Error(t, err)
	assert.Equal(t, true, k8sErrors.IsNotFound(err))
}

func TestBindingSetupWithManager(t *testing.T) {
	t.Parallel()
	mgr := &mockManager{T: t}
	options := controller.Options{MaxConcurrentReconciles: 1}

	err := (&BindingReconciler{}).SetupWithManager(mgr, options)
	assert.NoError(t, err)
}
