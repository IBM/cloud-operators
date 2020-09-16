package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	bxcontroller "github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
	"github.com/ibm/cloud-operators/internal/config"
	"github.com/ibm/cloud-operators/internal/ibmcloud"
	"github.com/ibm/cloud-operators/internal/ibmcloud/cfservice"
	"github.com/ibm/cloud-operators/internal/ibmcloud/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
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

func TestService(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	ready := t.Run("should be ready", func(t *testing.T) {
		for _, specfile := range []string{
			"translator.yaml",
			"cos.yaml",
			"messagehub.yaml",
			//"geoCF.yaml",
		} {
			t.Run(specfile, func(t *testing.T) {
				service := new(ibmcloudv1beta1.Service)
				mustLoadObject(t, filepath.Join("testdata", specfile), service, &service.ObjectMeta)
				ctx := context.TODO()
				logger := zapr.NewLogger(zaptest.NewLogger(t))

				require.NoError(t, k8sClient.Create(ctx, service))
				require.Eventually(t, verifyStatus(ctx, t, service.ObjectMeta, service, serviceStateOnline), defaultWait, defaultTick)

				if service.Spec.ServiceClassType == "CF" {
					// get instance directly from bx to make sure is there
					bxsvc, err := getServiceInstanceFromObjCF(logger, service)
					require.NoError(t, err)
					assert.Equal(t, service.ObjectMeta.Name, bxsvc.Name)

					// test delete
					serviceCopy := service.DeepCopyObject().(*ibmcloudv1beta1.Service)
					require.NoError(t, k8sClient.Delete(ctx, service))
					require.Eventually(t, func() bool {
						err := getObject(ctx, service.ObjectMeta, service)
						return errors.IsNotFound(err)
					}, defaultWait, defaultTick)

					_, err = getServiceInstanceFromObjCF(logger, serviceCopy)
					assert.NoError(t, err)
				} else {
					// get instance directly from bx to make sure is there
					bxsvc, err := getServiceInstanceFromObj(logger, service)
					require.NoError(t, err)
					assert.Equal(t, service.ObjectMeta.Name, bxsvc.Name)

					// test delete
					serviceCopy := service.DeepCopyObject().(*ibmcloudv1beta1.Service)
					require.NoError(t, k8sClient.Delete(ctx, service))
					require.Eventually(t, func() bool {
						err := getObject(ctx, service.ObjectMeta, service)
						return errors.IsNotFound(err)
					}, defaultWait, defaultTick)

					_, err = getServiceInstanceFromObj(logger, serviceCopy)
					assert.True(t, ibmcloud.IsNotFound(err), "Expect service to be deleted")
				}
			})
		}
	})
	if !ready {
		t.FailNow()
	}

	t.Run("should be ready - alias plan", func(t *testing.T) {
		const (
			specfile  = "testdata/translator.yaml"
			aliasfile = "testdata/translator-alias.yaml"
		)

		service, alias := new(ibmcloudv1beta1.Service), new(ibmcloudv1beta1.Service)
		mustLoadObject(t, specfile, service, &service.ObjectMeta)
		mustLoadObject(t, aliasfile, alias, &alias.ObjectMeta)
		logger := zapr.NewLogger(zaptest.NewLogger(t))
		ctx := context.TODO()

		require.NoError(t, k8sClient.Create(ctx, service))
		require.NoError(t, k8sClient.Create(ctx, alias))

		require.Eventually(t, verifyStatus(ctx, t, service.ObjectMeta, service, serviceStateOnline), defaultWait, defaultTick)
		require.Eventually(t, verifyStatus(ctx, t, alias.ObjectMeta, alias, serviceStateOnline), defaultWait*2, defaultTick)

		// test delete
		serviceCopy := service.DeepCopyObject().(*ibmcloudv1beta1.Service)
		require.NoError(t, k8sClient.Delete(ctx, service))
		require.Eventually(t, func() bool {
			err := getObject(ctx, service.ObjectMeta, service)
			return errors.IsNotFound(err)
		}, defaultWait, defaultTick)

		require.NoError(t, k8sClient.Delete(ctx, alias))
		require.Eventually(t, func() bool {
			err := getObject(ctx, alias.ObjectMeta, alias)
			return errors.IsNotFound(err)
		}, defaultWait, defaultTick)

		_, err := getServiceInstanceFromObj(logger, serviceCopy)
		assert.True(t, ibmcloud.IsNotFound(err))
	})

	t.Run("should fail", func(t *testing.T) {
		const (
			specfile = "testdata/translator-wrong-plan.yaml"
		)
		service := new(ibmcloudv1beta1.Service)
		mustLoadObject(t, specfile, service, &service.ObjectMeta)
		ctx := context.TODO()

		require.NoError(t, k8sClient.Create(ctx, service))
		assert.Eventually(t, verifyStatus(ctx, t, service.ObjectMeta, service, serviceStateFailed), defaultWait, defaultTick)
	})
}

// getServiceInstanceFromObjCF from bx given context and resource in a CF context
func getServiceInstanceFromObjCF(logt logr.Logger, service *ibmcloudv1beta1.Service) (*mccpv2.ServiceInstance, error) {
	externalName := getExternalName(service)

	ibmCloudInfo, err := ibmcloud.GetInfo(logt, k8sClient, service)
	if err != nil {
		return &mccpv2.ServiceInstance{}, err
	}

	serviceInstanceAPI := ibmCloudInfo.BXClient.ServiceInstances()
	return serviceInstanceAPI.FindByName(externalName)
}

// getServiceInstanceFromObj from bx given context and resource
func getServiceInstanceFromObj(logt logr.Logger, service *ibmcloudv1beta1.Service) (models.ServiceInstance, error) {
	externalName := getExternalName(service)

	ibmCloudInfo, err := ibmcloud.GetInfo(logt, k8sClient, service)
	if err != nil {
		return models.ServiceInstance{}, err
	}

	// Service instance is not CF
	controllerClient := ibmCloudInfo.ResourceClient
	resServiceInstanceAPI := controllerClient.ResourceServiceInstance()
	serviceInstanceQuery := bxcontroller.ServiceInstanceQuery{
		ResourceGroupID: ibmCloudInfo.ResourceGroupID,
		ServicePlanID:   ibmCloudInfo.ServicePlanID,
		Name:            externalName,
	}

	instances, err := resServiceInstanceAPI.ListInstances(serviceInstanceQuery)
	if err != nil {
		return models.ServiceInstance{}, err
	}
	for _, instance := range instances {
		if instance.ID == service.Status.InstanceID {
			return instance, nil
		}
	}
	return models.ServiceInstance{}, fmt.Errorf("not found")
}

func TestServiceV1Alpha1Compat(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	service := new(ibmcloudv1beta1.Service)
	mustLoadObject(t, filepath.Join("testdata", "translator-v1alpha1.yaml"), service, &service.ObjectMeta)
	ctx := context.TODO()
	logger := zapr.NewLogger(zaptest.NewLogger(t))

	require.NoError(t, k8sClient.Create(ctx, service))
	require.Eventually(t, verifyStatus(ctx, t, service.ObjectMeta, service, serviceStateOnline), defaultWait, defaultTick)

	// get instance directly from bx to make sure is there
	bxsvc, err := getServiceInstanceFromObj(logger, service)
	require.NoError(t, err)
	assert.Equal(t, service.ObjectMeta.Name, bxsvc.Name)

	// test delete
	serviceCopy := service.DeepCopyObject().(*ibmcloudv1beta1.Service)
	require.NoError(t, k8sClient.Delete(ctx, service))
	require.Eventually(t, func() bool {
		err := getObject(ctx, service.ObjectMeta, service)
		return errors.IsNotFound(err)
	}, defaultWait, defaultTick)

	_, err = getServiceInstanceFromObj(logger, serviceCopy)
	assert.True(t, ibmcloud.IsNotFound(err), "Expect service to be deleted")
}

func TestServiceLoadServiceFailed(t *testing.T) {
	t.Parallel()
	const (
		serviceName = "myservice"
		namespace   = "mynamespace"
	)

	t.Run("not found error", func(t *testing.T) {
		scheme := schemas(t)
		objects := []runtime.Object{}
		r := &ServiceReconciler{
			Client: fake.NewFakeClientWithScheme(scheme, objects...),
			Log:    testLogger(t),
			Scheme: scheme,
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.NoError(t, err)
	})

	t.Run("other error", func(t *testing.T) {
		scheme := runtime.NewScheme()
		objects := []runtime.Object{}
		r := &ServiceReconciler{
			Client: fake.NewFakeClientWithScheme(scheme, objects...),
			Log:    testLogger(t),
			Scheme: scheme,
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.Error(t, err)
		assert.False(t, k8sErrors.IsNotFound(err))
	})
}

func TestServiceSpecChangedAndUpdateFailed(t *testing.T) {
	t.Parallel()
	const (
		serviceName = "myservice"
		namespace   = "mynamespace"
	)

	scheme := schemas(t)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1beta1.ServiceStatus{
				Plan: "Lite",
			},
		},
	}
	r := &ServiceReconciler{
		Client: newMockClient(
			fake.NewFakeClientWithScheme(scheme, objects...),
			MockConfig{UpdateErr: fmt.Errorf("failed")},
		),
		Log:    testLogger(t),
		Scheme: scheme,
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
	})
	assert.Equal(t, ctrl.Result{}, result)
	assert.EqualError(t, err, "failed")
}

func TestServiceGetIBMCloudInfoFailed(t *testing.T) {
	t.Parallel()
	const (
		serviceName = "myservice"
		namespace   = "mynamespace"
	)

	now := metav1Now(t)
	scheme := schemas(t)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:              serviceName,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        []string{serviceFinalizer},
			},
			Status: ibmcloudv1beta1.ServiceStatus{Plan: "Lite"},
			Spec:   ibmcloudv1beta1.ServiceSpec{Plan: "Lite"},
		},
	}

	t.Run("not found error", func(t *testing.T) {
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{UpdateErr: fmt.Errorf("failed")},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return nil, errors.NewNotFound(ctrl.GroupResource{Group: "ibmcloud.ibm.com", Resource: "secret"}, "secret-ibm-cloud-operator")
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              serviceName,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        nil, // attempt to remove finalizers
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				//State: serviceStateFailed, // TODO this state should be set!
				Plan: "Lite",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{Plan: "Lite"},
		}, r.Client.(MockClient).LastUpdate())
		assert.Equal(t, nil, r.Client.(MockClient).LastStatusUpdate())
	})

	t.Run("other error", func(t *testing.T) {
		fakeClient := newMockClient(
			fake.NewFakeClientWithScheme(scheme, objects...),
			MockConfig{},
		)
		r := &ServiceReconciler{
			Client: fakeClient,
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, r client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return nil, fmt.Errorf("failed")
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              serviceName,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        []string{serviceFinalizer},
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				State:   serviceStateFailed,
				Message: "failed",
				Plan:    "Lite",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{Plan: "Lite"},
		}, fakeClient.LastStatusUpdate())
	})
}

func TestServiceFirstStatusFailed(t *testing.T) {
	t.Parallel()
	const (
		serviceName = "myservice"
		namespace   = "mynamespace"
	)

	scheme := schemas(t)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status:     ibmcloudv1beta1.ServiceStatus{},
		},
	}
	r := &ServiceReconciler{
		Client: newMockClient(
			fake.NewFakeClientWithScheme(scheme, objects...),
			MockConfig{StatusUpdateErr: fmt.Errorf("failed")},
		),
		Log:    testLogger(t),
		Scheme: scheme,

		GetIBMCloudInfo: func(logt logr.Logger, r client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
			return &ibmcloud.Info{}, nil
		},
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
	})
	assert.Equal(t, ctrl.Result{}, result)
	assert.EqualError(t, err, "failed")
}

func TestServiceEnsureFinalizerFailed(t *testing.T) {
	t.Parallel()
	const (
		serviceName = "myservice"
		namespace   = "mynamespace"
	)

	scheme := schemas(t)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:              serviceName,
				Namespace:         namespace,
				DeletionTimestamp: nil, // not deleting
				Finalizers:        nil, // AND missing finalizer
			},
			Status: ibmcloudv1beta1.ServiceStatus{Plan: "Lite"},
			Spec:   ibmcloudv1beta1.ServiceSpec{Plan: "Lite"},
		},
	}
	var r *ServiceReconciler
	r = &ServiceReconciler{
		Client: fake.NewFakeClientWithScheme(scheme, objects...),
		Log:    testLogger(t),
		Scheme: scheme,

		GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
			r.Client = newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{UpdateErr: fmt.Errorf("failed")},
			)
			return &ibmcloud.Info{}, nil
		},
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
	})
	assert.Equal(t, ctrl.Result{}, result)
	assert.EqualError(t, err, "failed")
	assert.Equal(t, &ibmcloudv1beta1.Service{
		TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:       serviceName,
			Namespace:  namespace,
			Finalizers: []string{serviceFinalizer},
		},
		Status: ibmcloudv1beta1.ServiceStatus{
			Plan: "Lite",
		},
		Spec: ibmcloudv1beta1.ServiceSpec{Plan: "Lite"},
	}, r.Client.(MockClient).LastUpdate())
}

func TestServiceDeletingFailed(t *testing.T) {
	t.Parallel()
	const (
		serviceName = "myservice"
		namespace   = "mynamespace"
	)

	t.Run("service delete failed", func(t *testing.T) {
		scheme := schemas(t)
		now := metav1Now(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:              serviceName,
					Namespace:         namespace,
					DeletionTimestamp: now,
					Finalizers:        []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{Plan: "Lite", InstanceID: "myinstanceid"},
				Spec:   ibmcloudv1beta1.ServiceSpec{Plan: "Lite"},
			},
		}

		var r *ServiceReconciler
		r = &ServiceReconciler{
			Client: fake.NewFakeClientWithScheme(scheme, objects...),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				r.Client = newMockClient(
					fake.NewFakeClientWithScheme(scheme, objects...),
					MockConfig{},
				)
				return &ibmcloud.Info{}, nil
			},
			DeleteResourceServiceInstance: func(session *session.Session, instanceID string, logt logr.Logger) error {
				return fmt.Errorf("failed")
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: 10 * time.Second,
		}, result)
		assert.NoError(t, err)
	})

	t.Run("update failed", func(t *testing.T) {
		scheme := schemas(t)
		now := metav1Now(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:              serviceName,
					Namespace:         namespace,
					DeletionTimestamp: now,
					Finalizers:        []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{Plan: "Lite"},
				Spec:   ibmcloudv1beta1.ServiceSpec{Plan: "Lite"},
			},
		}

		var r *ServiceReconciler
		r = &ServiceReconciler{
			Client: fake.NewFakeClientWithScheme(scheme, objects...),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				r.Client = newMockClient(
					fake.NewFakeClientWithScheme(scheme, objects...),
					MockConfig{UpdateErr: fmt.Errorf("failed")},
				)
				return &ibmcloud.Info{}, nil
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.EqualError(t, err, "failed")
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              serviceName,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        nil, // attempt to remove finalizers
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				Plan: "Lite",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{Plan: "Lite"},
		}, r.Client.(MockClient).LastUpdate())
	})
}

func TestServiceGetParamsFailed(t *testing.T) {
	t.Parallel()
	const (
		serviceName = "myservice"
		namespace   = "mynamespace"
	)

	scheme := schemas(t)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1beta1.ServiceStatus{
				Plan: "Lite",
				Parameters: []ibmcloudv1beta1.Param{
					{
						Name:      "hello",
						Value:     &ibmcloudv1beta1.ParamValue{RawMessage: json.RawMessage(`"world"`)},
						ValueFrom: &ibmcloudv1beta1.ParamSource{},
					},
				},
			},
			Spec: ibmcloudv1beta1.ServiceSpec{
				Plan: "Lite",
				Parameters: []ibmcloudv1beta1.Param{
					{
						Name:      "hello",
						Value:     &ibmcloudv1beta1.ParamValue{RawMessage: json.RawMessage(`"world"`)},
						ValueFrom: &ibmcloudv1beta1.ParamSource{},
					},
				},
			},
		},
	}
	r := &ServiceReconciler{
		Client: newMockClient(
			fake.NewFakeClientWithScheme(scheme, objects...),
			MockConfig{},
		),
		Log:    testLogger(t),
		Scheme: scheme,

		GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
			return &ibmcloud.Info{}, nil
		},
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
	})
	assert.Equal(t, ctrl.Result{
		Requeue:      true,
		RequeueAfter: config.Get().SyncPeriod,
	}, result)
	assert.NoError(t, err)
	assert.Equal(t, &ibmcloudv1beta1.Service{
		TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:       serviceName,
			Namespace:  namespace,
			Finalizers: []string{serviceFinalizer},
		},
		Status: ibmcloudv1beta1.ServiceStatus{
			State:   serviceStateFailed,
			Message: "Value and ValueFrom properties are mutually exclusive (for hello variable)",
			Plan:    "Lite",
			Parameters: []ibmcloudv1beta1.Param{
				{
					Name:      "hello",
					Value:     &ibmcloudv1beta1.ParamValue{RawMessage: json.RawMessage(`"world"`)},
					ValueFrom: &ibmcloudv1beta1.ParamSource{},
				},
			},
		},
		Spec: ibmcloudv1beta1.ServiceSpec{
			Plan: "Lite",
			Parameters: []ibmcloudv1beta1.Param{
				{
					Name:      "hello",
					Value:     &ibmcloudv1beta1.ParamValue{RawMessage: json.RawMessage(`"world"`)},
					ValueFrom: &ibmcloudv1beta1.ParamSource{},
				},
			},
		},
	}, r.Client.(MockClient).LastStatusUpdate())
}

func TestServiceEnsureCFServiceExists(t *testing.T) {
	t.Parallel()
	const (
		serviceName = "myservice"
		namespace   = "mynamespace"
	)

	t.Run("create - empty service ID", func(t *testing.T) {
		scheme := schemas(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status:     ibmcloudv1beta1.ServiceStatus{Plan: "Lite", ServiceClass: "service-name"},
				Spec:       ibmcloudv1beta1.ServiceSpec{Plan: "Lite", ServiceClass: "service-name"},
			},
		}
		var createErr error
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{
					ServiceClassType: "CF",
				}, nil
			},
			CreateCFServiceInstance: func(session *session.Session, externalName, planID, spaceID string, params map[string]interface{}, tags []string) (guid string, state string, err error) {
				return "guid", "state", createErr
			},
		}

		t.Run("success", func(t *testing.T) {
			createErr = nil
			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})
			assert.Equal(t, ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			}, result)
			assert.NoError(t, err)
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        "state",
					Message:      "state",
					Plan:         "Lite",
					InstanceID:   "guid",
					DashboardURL: "https://cloud.ibm.com/services/service-name/guid",
					ServiceClass: "service-name",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{Plan: "Lite", ServiceClass: "service-name"},
			}, r.Client.(MockClient).LastStatusUpdate())
		})

		t.Run("failed", func(t *testing.T) {
			createErr = fmt.Errorf("failed")
			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})
			assert.Equal(t, ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			}, result)
			assert.NoError(t, err)
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        serviceStateFailed,
					Message:      "failed",
					Plan:         "Lite",
					ServiceClass: "service-name",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{Plan: "Lite", ServiceClass: "service-name"},
			}, r.Client.(MockClient).LastStatusUpdate())
		})
	})

	t.Run("create alias success", func(t *testing.T) {
		scheme := schemas(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
					InstanceID:   "guid",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
			},
		}
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{
					ServiceClassType: "CF",
				}, nil
			},
			CreateCFServiceInstance: func(session *session.Session, externalName, planID, spaceID string, params map[string]interface{}, tags []string) (guid string, state string, err error) {
				return "", "", fmt.Errorf("failed")
			},
			GetCFServiceInstance: func(session *session.Session, name string) (guid string, state string, err error) {
				return "guid", "state", nil
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       serviceName,
				Namespace:  namespace,
				Finalizers: []string{serviceFinalizer},
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				State:        "state", // TODO(johnstarich) This isn't a known state, right? We should have predictable states here.
				Message:      "state",
				DashboardURL: "https://cloud.ibm.com/services/service-name/guid",
				Plan:         aliasPlan,
				ServiceClass: "service-name",
				InstanceID:   "guid",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{
				Plan:         aliasPlan,
				ServiceClass: "service-name",
			},
		}, r.Client.(MockClient).LastStatusUpdate())
	})

	t.Run("ensure alias - empty instance ID", func(t *testing.T) {
		scheme := schemas(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
					InstanceID:   "", // no instance ID set
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
			},
		}
		var getInstanceErr error
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{
					ServiceClassType: "CF",
				}, nil
			},
			GetCFServiceInstance: func(session *session.Session, name string) (guid string, state string, err error) {
				return "guid", "state", getInstanceErr
			},
		}

		t.Run("success", func(t *testing.T) {
			getInstanceErr = nil
			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})
			assert.Equal(t, ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			}, result)
			assert.NoError(t, err)
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        serviceStateOnline,
					Message:      serviceStateOnline,
					Plan:         aliasPlan,
					ServiceClass: "service-name",
					InstanceID:   "guid",
					DashboardURL: "https://cloud.ibm.com/services/service-name/guid",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
			}, r.Client.(MockClient).LastStatusUpdate())
		})

		t.Run("failed", func(t *testing.T) {
			getInstanceErr = fmt.Errorf("failed")
			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})
			assert.Equal(t, ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			}, result)
			assert.NoError(t, err)
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        serviceStateFailed,
					Message:      "failed",
					Plan:         aliasPlan,
					ServiceClass: "service-name",
					InstanceID:   "",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
			}, r.Client.(MockClient).LastStatusUpdate())
		})
	})

	t.Run("get instance failed - not found", func(t *testing.T) {
		scheme := schemas(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:         "Lite",
					ServiceClass: "service-name",
					InstanceID:   "guid",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         "Lite",
					ServiceClass: "service-name",
				},
			},
		}
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{
					ServiceClassType: "CF",
				}, nil
			},
			CreateCFServiceInstance: func(session *session.Session, externalName, planID, spaceID string, params map[string]interface{}, tags []string) (guid string, state string, err error) {
				return "guid", "state", nil
			},
			GetCFServiceInstance: func(session *session.Session, name string) (guid string, state string, err error) {
				return "", "", cfservice.NotFoundError{Err: fmt.Errorf("failed")}
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       serviceName,
				Namespace:  namespace,
				Finalizers: []string{serviceFinalizer},
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				State:        "state",
				Message:      "state",
				Plan:         "Lite",
				InstanceID:   "guid",
				ServiceClass: "service-name",
				DashboardURL: "https://cloud.ibm.com/services/service-name/guid",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{Plan: "Lite", ServiceClass: "service-name"},
		}, r.Client.(MockClient).LastStatusUpdate())
	})

	t.Run("get instance failed - not found, create failed", func(t *testing.T) {
		scheme := schemas(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:         "Lite",
					ServiceClass: "service-name",
					InstanceID:   "guid",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         "Lite",
					ServiceClass: "service-name",
				},
			},
		}
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{
					ServiceClassType: "CF",
				}, nil
			},
			CreateCFServiceInstance: func(session *session.Session, externalName, planID, spaceID string, params map[string]interface{}, tags []string) (guid string, state string, err error) {
				return "", "", fmt.Errorf("failed")
			},
			GetCFServiceInstance: func(session *session.Session, name string) (guid string, state string, err error) {
				return "", "", cfservice.NotFoundError{Err: fmt.Errorf("failed")}
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       serviceName,
				Namespace:  namespace,
				Finalizers: []string{serviceFinalizer},
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				State:        serviceStateFailed,
				Message:      "failed",
				Plan:         "Lite",
				InstanceID:   "guid",
				ServiceClass: "service-name",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{Plan: "Lite", ServiceClass: "service-name"},
		}, r.Client.(MockClient).LastStatusUpdate())
	})

	t.Run("get instance failed - other error", func(t *testing.T) {
		scheme := schemas(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:         "Lite",
					ServiceClass: "service-name",
					InstanceID:   "guid",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         "Lite",
					ServiceClass: "service-name",
				},
			},
		}
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{
					ServiceClassType: "CF",
				}, nil
			},
			GetCFServiceInstance: func(session *session.Session, name string) (guid string, state string, err error) {
				return "", "", fmt.Errorf("failed")
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       serviceName,
				Namespace:  namespace,
				Finalizers: []string{serviceFinalizer},
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				State:        serviceStateFailed,
				Message:      "failed",
				Plan:         "Lite",
				InstanceID:   "guid",
				ServiceClass: "service-name",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{Plan: "Lite", ServiceClass: "service-name"},
		}, r.Client.(MockClient).LastStatusUpdate())
	})

	t.Run("ensure alias - instance does not exist", func(t *testing.T) {
		scheme := schemas(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
					InstanceID:   "some-instance-id", // instance ID set
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
			},
		}
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{
					ServiceClassType: "CF",
				}, nil
			},
			GetCFServiceInstance: func(session *session.Session, name string) (guid string, state string, err error) {
				return "", "", cfservice.NotFoundError{Err: fmt.Errorf("failed")}
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       serviceName,
				Namespace:  namespace,
				Finalizers: []string{serviceFinalizer},
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				State:        serviceStatePending,
				Message:      "failed",
				Plan:         aliasPlan,
				ServiceClass: "service-name",
				InstanceID:   "", // instance ID should be deleted
			},
			Spec: ibmcloudv1beta1.ServiceSpec{
				Plan:         aliasPlan,
				ServiceClass: "service-name",
			},
		}, r.Client.(MockClient).LastStatusUpdate())
	})
}

func TestServiceEnsureResourceServiceInstance(t *testing.T) {
	t.Parallel()
	const (
		serviceName = "myservice"
		namespace   = "mynamespace"
	)

	t.Run("alias", func(t *testing.T) {
		scheme := schemas(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
			},
		}

		t.Run("success", func(t *testing.T) {
			r := &ServiceReconciler{
				Client: newMockClient(
					fake.NewFakeClientWithScheme(scheme, objects...),
					MockConfig{},
				),
				Log:    testLogger(t),
				Scheme: scheme,

				GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
					return &ibmcloud.Info{}, nil
				},
				GetResourceServiceAliasInstance: func(session *session.Session, instanceID, resourceGroupID, servicePlanID, externalName string, logt logr.Logger) (id string, state string, err error) {
					return "guid", "state", nil
				},
			}
			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})

			assert.Equal(t, ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			}, result)
			assert.NoError(t, err)
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        "state",
					Message:      "state",
					Plan:         aliasPlan,
					ServiceClass: "service-name",
					InstanceID:   "guid",
					DashboardURL: "https://cloud.ibm.com/services/service-name/guid",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
			}, r.Client.(MockClient).LastStatusUpdate())
		})

		t.Run("not found", func(t *testing.T) {
			r := &ServiceReconciler{
				Client: newMockClient(
					fake.NewFakeClientWithScheme(scheme, objects...),
					MockConfig{},
				),
				Log:    testLogger(t),
				Scheme: scheme,

				GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
					return &ibmcloud.Info{}, nil
				},
				GetResourceServiceAliasInstance: func(session *session.Session, instanceID, resourceGroupID, servicePlanID, externalName string, logt logr.Logger) (id string, state string, err error) {
					return "", "", resource.NotFoundError{Err: fmt.Errorf("failed")}
				},
			}
			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})

			assert.Equal(t, ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			}, result)
			assert.NoError(t, err)
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        serviceStateFailed,
					Message:      "no service instances with name myservice found for alias plan: failed",
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
			}, r.Client.(MockClient).LastStatusUpdate())
		})

		t.Run("other error", func(t *testing.T) {
			r := &ServiceReconciler{
				Client: newMockClient(
					fake.NewFakeClientWithScheme(scheme, objects...),
					MockConfig{},
				),
				Log:    testLogger(t),
				Scheme: scheme,

				GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
					return &ibmcloud.Info{}, nil
				},
				GetResourceServiceAliasInstance: func(session *session.Session, instanceID, resourceGroupID, servicePlanID, externalName string, logt logr.Logger) (id string, state string, err error) {
					return "", "", fmt.Errorf("failed")
				},
			}
			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})

			assert.Equal(t, ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			}, result)
			assert.NoError(t, err)
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        serviceStateFailed,
					Message:      "failed to resolve Alias plan instance myservice: failed",
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         aliasPlan,
					ServiceClass: "service-name",
				},
			}, r.Client.(MockClient).LastStatusUpdate())
		})
	})

	t.Run("non-alias", func(t *testing.T) {
		scheme := schemas(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:         "Lite",
					ServiceClass: "service-name",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         "Lite",
					ServiceClass: "service-name",
				},
			},
		}

		t.Run("success", func(t *testing.T) {
			r := &ServiceReconciler{
				Client: newMockClient(
					fake.NewFakeClientWithScheme(scheme, objects...),
					MockConfig{},
				),
				Log:    testLogger(t),
				Scheme: scheme,

				GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
					return &ibmcloud.Info{}, nil
				},
				CreateResourceServiceInstance: func(session *session.Session, externalName, servicePlanID, resourceGroupID, targetCrn string, params map[string]interface{}, tags []string) (id string, state string, err error) {
					return "id", "state", nil
				},
			}

			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})
			assert.Equal(t, ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			}, result)
			assert.NoError(t, err)
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        "state",
					Message:      "state",
					Plan:         "Lite",
					ServiceClass: "service-name",
					InstanceID:   "id",
					DashboardURL: "https://cloud.ibm.com/services/service-name/id",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         "Lite",
					ServiceClass: "service-name",
				},
			}, r.Client.(MockClient).LastStatusUpdate())
		})

		t.Run("update status failed", func(t *testing.T) {
			r := &ServiceReconciler{
				Client: newMockClient(
					fake.NewFakeClientWithScheme(scheme, objects...),
					MockConfig{StatusUpdateErr: fmt.Errorf("failed")},
				),
				Log:    testLogger(t),
				Scheme: scheme,

				GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
					return &ibmcloud.Info{}, nil
				},
			}

			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})
			assert.Equal(t, ctrl.Result{}, result)
			assert.EqualError(t, err, "failed")
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        "",
					Message:      "",
					Plan:         "Lite",
					ServiceClass: "service-name",
					InstanceID:   inProgress,
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         "Lite",
					ServiceClass: "service-name",
				},
			}, r.Client.(MockClient).LastStatusUpdate())
		})

		t.Run("create failed", func(t *testing.T) {
			r := &ServiceReconciler{
				Client: newMockClient(
					fake.NewFakeClientWithScheme(scheme, objects...),
					MockConfig{},
				),
				Log:    testLogger(t),
				Scheme: scheme,

				GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
					return &ibmcloud.Info{}, nil
				},
				CreateResourceServiceInstance: func(session *session.Session, externalName, servicePlanID, resourceGroupID, targetCrn string, params map[string]interface{}, tags []string) (id string, state string, err error) {
					return "", "", fmt.Errorf("failed")
				},
			}

			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})
			assert.Equal(t, ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			}, result)
			assert.NoError(t, err)
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        serviceStateFailed,
					Message:      "failed",
					Plan:         "Lite",
					ServiceClass: "service-name",
					InstanceID:   inProgress,
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         "Lite",
					ServiceClass: "service-name",
				},
			}, r.Client.(MockClient).LastStatusUpdate())
		})
	})
}

func TestServiceVerifyExists(t *testing.T) {
	t.Parallel()
	const (
		namespace   = "mynamespace"
		serviceName = "myservice"
	)
	scheme := schemas(t)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1beta1.ServiceStatus{
				Plan:         "Lite",
				ServiceClass: "service-name",
				InstanceID:   "myinstanceid",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{
				Plan:         "Lite",
				ServiceClass: "service-name",
			},
		},
	}
	aliasObjects := []runtime.Object{
		&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1beta1.ServiceStatus{
				Plan:         aliasPlan,
				ServiceClass: "service-name",
				InstanceID:   "myinstanceid",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{
				Plan:         aliasPlan,
				ServiceClass: "service-name",
			},
		},
	}

	t.Run("success", func(t *testing.T) {
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			GetResourceServiceInstanceState: func(session *session.Session, resourceGroupID, servicePlanID, externalName, instanceID string) (state string, err error) {
				return "state", nil
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       serviceName,
				Namespace:  namespace,
				Finalizers: []string{serviceFinalizer},
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				State:        "state",
				Message:      "state",
				Plan:         "Lite",
				ServiceClass: "service-name",
				InstanceID:   "myinstanceid",
				DashboardURL: "https://cloud.ibm.com/services/service-name/myinstanceid",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{
				Plan:         "Lite",
				ServiceClass: "service-name",
			},
		}, r.Client.(MockClient).LastStatusUpdate())
	})

	t.Run("not found non-alias - recreate service", func(t *testing.T) {
		var createErr error
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			GetResourceServiceInstanceState: func(session *session.Session, resourceGroupID, servicePlanID, externalName, instanceID string) (state string, err error) {
				return "", resource.NotFoundError{Err: fmt.Errorf("failed")}
			},
			CreateResourceServiceInstance: func(session *session.Session, externalName, servicePlanID, resourceGroupID, targetCrn string, params map[string]interface{}, tags []string) (id string, state string, err error) {
				return "id", "state", createErr
			},
		}

		t.Run("success", func(t *testing.T) {
			createErr = nil
			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})
			assert.Equal(t, ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			}, result)
			assert.NoError(t, err)
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        "state",
					Message:      "state",
					Plan:         "Lite",
					ServiceClass: "service-name",
					InstanceID:   "id",
					DashboardURL: "https://cloud.ibm.com/services/service-name/id",
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         "Lite",
					ServiceClass: "service-name",
				},
			}, r.Client.(MockClient).LastStatusUpdate())
		})

		t.Run("create error", func(t *testing.T) {
			createErr = fmt.Errorf("failed")
			result, err := r.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
			})
			assert.Equal(t, ctrl.Result{
				Requeue:      true,
				RequeueAfter: config.Get().SyncPeriod,
			}, result)
			assert.NoError(t, err)
			assert.Equal(t, &ibmcloudv1beta1.Service{
				TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:       serviceName,
					Namespace:  namespace,
					Finalizers: []string{serviceFinalizer},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					State:        serviceStateFailed,
					Message:      "failed",
					Plan:         "Lite",
					ServiceClass: "service-name",
					InstanceID:   inProgress,
				},
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         "Lite",
					ServiceClass: "service-name",
				},
			}, r.Client.(MockClient).LastStatusUpdate())
		})
	})

	t.Run("not found non-alias - status update failed", func(t *testing.T) {
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{StatusUpdateErr: fmt.Errorf("failed")},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			GetResourceServiceInstanceState: func(session *session.Session, resourceGroupID, servicePlanID, externalName, instanceID string) (state string, err error) {
				return "", resource.NotFoundError{Err: fmt.Errorf("some other error")}
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{}, result)
		assert.EqualError(t, err, "failed")
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       serviceName,
				Namespace:  namespace,
				Finalizers: []string{serviceFinalizer},
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				State:        "",
				Message:      "",
				Plan:         "Lite",
				ServiceClass: "service-name",
				InstanceID:   inProgress,
			},
			Spec: ibmcloudv1beta1.ServiceSpec{
				Plan:         "Lite",
				ServiceClass: "service-name",
			},
		}, r.Client.(MockClient).LastStatusUpdate())
	})

	t.Run("not found alias", func(t *testing.T) {
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, aliasObjects...),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			GetResourceServiceInstanceState: func(session *session.Session, resourceGroupID, servicePlanID, externalName, instanceID string) (state string, err error) {
				return "", resource.NotFoundError{Err: fmt.Errorf("failed")}
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       serviceName,
				Namespace:  namespace,
				Finalizers: []string{serviceFinalizer},
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				State:        serviceStatePending,
				Message:      "aliased service instance no longer exists",
				Plan:         aliasPlan,
				ServiceClass: "service-name",
				InstanceID:   "",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{
				Plan:         aliasPlan,
				ServiceClass: "service-name",
			},
		}, r.Client.(MockClient).LastStatusUpdate())
	})

	t.Run("other error", func(t *testing.T) {
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, objects...),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,

			GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
				return &ibmcloud.Info{}, nil
			},
			GetResourceServiceInstanceState: func(session *session.Session, resourceGroupID, servicePlanID, externalName, instanceID string) (state string, err error) {
				return "", fmt.Errorf("failed")
			},
		}

		result, err := r.Reconcile(ctrl.Request{
			NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
		})
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: config.Get().SyncPeriod,
		}, result)
		assert.NoError(t, err)
		assert.Equal(t, &ibmcloudv1beta1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:       serviceName,
				Namespace:  namespace,
				Finalizers: []string{serviceFinalizer},
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				State:        serviceStatePending,
				Message:      "failed",
				Plan:         "Lite",
				ServiceClass: "service-name",
				InstanceID:   "myinstanceid",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{
				Plan:         "Lite",
				ServiceClass: "service-name",
			},
		}, r.Client.(MockClient).LastStatusUpdate())
	})
}

func TestServiceUpdateTagsOrParamsFailed(t *testing.T) {
	t.Parallel()
	const (
		namespace   = "mynamespace"
		serviceName = "myservice"
	)
	scheme := schemas(t)
	objects := []runtime.Object{
		&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
			Status: ibmcloudv1beta1.ServiceStatus{
				Plan:         "Lite",
				ServiceClass: "service-name",
				InstanceID:   "myinstanceid",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{
				Plan:         "Lite",
				ServiceClass: "service-name",
				Tags:         []string{"somethingNew"},
			},
		},
	}

	r := &ServiceReconciler{
		Client: newMockClient(
			fake.NewFakeClientWithScheme(scheme, objects...),
			MockConfig{},
		),
		Log:    testLogger(t),
		Scheme: scheme,

		GetIBMCloudInfo: func(logt logr.Logger, _ client.Client, instance *ibmcloudv1beta1.Service) (*ibmcloud.Info, error) {
			return &ibmcloud.Info{}, nil
		},
		GetResourceServiceInstanceState: func(session *session.Session, resourceGroupID, servicePlanID, externalName, instanceID string) (state string, err error) {
			return "state", nil
		},
		UpdateResourceServiceInstance: func(session *session.Session, serviceInstanceID, externalName, servicePlanID string, params map[string]interface{}, tags []string) (state string, err error) {
			return "", fmt.Errorf("failed")
		},
	}

	result, err := r.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace},
	})
	assert.Equal(t, ctrl.Result{
		Requeue:      true,
		RequeueAfter: config.Get().SyncPeriod,
	}, result)
	assert.NoError(t, err)
	assert.Equal(t, &ibmcloudv1beta1.Service{
		TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "ibmcloud.ibm.com/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:       serviceName,
			Namespace:  namespace,
			Finalizers: []string{serviceFinalizer},
		},
		Status: ibmcloudv1beta1.ServiceStatus{
			State:        serviceStateFailed,
			Message:      "failed",
			Plan:         "Lite",
			ServiceClass: "service-name",
			InstanceID:   "myinstanceid",
		},
		Spec: ibmcloudv1beta1.ServiceSpec{
			Plan:         "Lite",
			ServiceClass: "service-name",
			Tags:         []string{"somethingNew"},
		},
	}, r.Client.(MockClient).LastStatusUpdate())
}

func TestSpecChanged(t *testing.T) {
	t.Parallel()
	const (
		something     = "something"
		somethingElse = "something else"
	)
	for _, tc := range []struct {
		description   string
		instance      ibmcloudv1beta1.Service
		expectChanged bool
	}{
		{
			description:   "empty object",
			instance:      ibmcloudv1beta1.Service{},
			expectChanged: false,
		},
		{
			description: "missing status plan",
			instance: ibmcloudv1beta1.Service{
				Spec:   ibmcloudv1beta1.ServiceSpec{ExternalName: something},
				Status: ibmcloudv1beta1.ServiceStatus{ExternalName: something},
			},
			expectChanged: false,
		},
		{
			description: "mismatched external name",
			instance: ibmcloudv1beta1.Service{
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         something,
					ExternalName: something,
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:         something,
					ExternalName: somethingElse,
				},
			},
			expectChanged: true,
		},
		{
			description: "mismatched plan",
			instance: ibmcloudv1beta1.Service{
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan: something,
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan: somethingElse,
				},
			},
			expectChanged: true,
		},
		{
			description: "mismatched service class",
			instance: ibmcloudv1beta1.Service{
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:         something,
					ServiceClass: something,
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:         something,
					ServiceClass: somethingElse,
				},
			},
			expectChanged: true,
		},
		{
			description: "mismatched service class type",
			instance: ibmcloudv1beta1.Service{
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:             something,
					ServiceClassType: something,
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:             something,
					ServiceClassType: somethingElse,
				},
			},
			expectChanged: true,
		},
		{
			description: "mismatched context",
			instance: ibmcloudv1beta1.Service{
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:    something,
					Context: ibmcloudv1beta1.ResourceContext{User: something},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:    something,
					Context: ibmcloudv1beta1.ResourceContext{User: somethingElse},
				},
			},
			expectChanged: true,
		},
		{
			description: "matching contexts",
			instance: ibmcloudv1beta1.Service{
				Spec: ibmcloudv1beta1.ServiceSpec{
					Plan:    something,
					Context: ibmcloudv1beta1.ResourceContext{User: somethingElse},
				},
				Status: ibmcloudv1beta1.ServiceStatus{
					Plan:    something,
					Context: ibmcloudv1beta1.ResourceContext{User: somethingElse},
				},
			},
			expectChanged: false,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			assert.Equal(t, tc.expectChanged, specChanged(&tc.instance))
		})
	}
}

func TestDeleteServiceFinalizer(t *testing.T) {
	t.Parallel()
	t.Run("no finalizer found", func(t *testing.T) {
		finalizers := []string(nil)
		assert.Equal(t, finalizers, deleteServiceFinalizer(&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{Finalizers: finalizers},
		}))
	})

	t.Run("one other finalizer found", func(t *testing.T) {
		finalizers := []string{"not-service-finalizer"}
		assert.Equal(t, finalizers, deleteServiceFinalizer(&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{Finalizers: finalizers},
		}))
	})

	t.Run("one finalizer found", func(t *testing.T) {
		finalizers := []string{serviceFinalizer}
		assert.Equal(t, []string(nil), deleteServiceFinalizer(&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{Finalizers: finalizers},
		}))
	})

	t.Run("multiple finalizers found", func(t *testing.T) {
		finalizers := []string{serviceFinalizer, serviceFinalizer}
		assert.Equal(t, []string(nil), deleteServiceFinalizer(&ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{Finalizers: finalizers},
		}))
	})
}

func TestServiceParamToJSON(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		description string
		param       ibmcloudv1beta1.Param
		expectJSON  map[string]interface{}
		expectErr   string
	}{
		{
			description: "error: value and valueFrom both set",
			param: ibmcloudv1beta1.Param{
				Name:      "myvalue",
				Value:     &ibmcloudv1beta1.ParamValue{},
				ValueFrom: &ibmcloudv1beta1.ParamSource{},
			},
			expectErr: "Value and ValueFrom properties are mutually exclusive (for myvalue variable)",
		},
		{
			description: "empty valueFrom error",
			param: ibmcloudv1beta1.Param{
				Name:      "myvalue",
				ValueFrom: &ibmcloudv1beta1.ParamSource{},
			},
			expectErr: "Missing secretKeyRef or configMapKeyRef",
		},
		{
			description: "empty value error",
			param: ibmcloudv1beta1.Param{
				Name:  "myvalue",
				Value: &ibmcloudv1beta1.ParamValue{},
			},
			expectErr: "unexpected end of JSON input",
		},
		{
			description: "value happy path",
			param: ibmcloudv1beta1.Param{
				Name: "myvalue",
				Value: &ibmcloudv1beta1.ParamValue{
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
			param:       ibmcloudv1beta1.Param{Name: "myvalue"},
			expectJSON:  nil,
			expectErr:   "",
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			r := &ServiceReconciler{}
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

func TestServiceParamValueToJSON(t *testing.T) {
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

	objects := []runtime.Object{
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
		valueFrom   ibmcloudv1beta1.ParamSource
		expectJSON  interface{}
		expectErr   string
	}{
		{
			description: "no value error",
			expectErr:   "Missing secretKeyRef or configMapKeyRef",
		},
		{
			description: "secret ref success",
			valueFrom: ibmcloudv1beta1.ParamSource{
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
			valueFrom: ibmcloudv1beta1.ParamSource{
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
			valueFrom: ibmcloudv1beta1.ParamSource{
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
			valueFrom: ibmcloudv1beta1.ParamSource{
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
			valueFrom: ibmcloudv1beta1.ParamSource{
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
			valueFrom: ibmcloudv1beta1.ParamSource{
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
			r := &ServiceReconciler{
				Client: fake.NewFakeClientWithScheme(scheme, objects...),
				Log:    testLogger(t),
				Scheme: scheme,
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

func TestServiceUpdateStatusFailed(t *testing.T) {
	t.Parallel()
	const (
		namespace   = "mynamespace"
		serviceName = "myservice"
	)
	scheme := schemas(t)
	instance := &ibmcloudv1beta1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
		Status: ibmcloudv1beta1.ServiceStatus{
			Plan:         "Lite",
			ServiceClass: "service-name",
			InstanceID:   "myinstanceid",
		},
		Spec: ibmcloudv1beta1.ServiceSpec{
			Plan:         "Lite",
			ServiceClass: "service-name",
		},
	}

	r := &ServiceReconciler{
		Client: newMockClient(
			fake.NewFakeClientWithScheme(scheme, instance),
			MockConfig{StatusUpdateErr: fmt.Errorf("failed")},
		),
		Log:    testLogger(t),
		Scheme: scheme,

		DeleteResourceServiceInstance: func(session *session.Session, instanceID string, logt logr.Logger) error {
			return fmt.Errorf("failed to delete") // only gets logged, no error handling
		},
	}

	result, err := r.updateStatus(nil, r.Log, instance, ibmcloudv1beta1.ResourceContext{}, "myinstanceid", "state", "")
	assert.Equal(t, ctrl.Result{}, result)
	assert.EqualError(t, err, "failed")
	assert.Equal(t, &ibmcloudv1beta1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
		Status: ibmcloudv1beta1.ServiceStatus{
			State:        "state",
			Message:      "state",
			Plan:         "Lite",
			ServiceClass: "service-name",
			InstanceID:   "myinstanceid",
			DashboardURL: "https://cloud.ibm.com/services/service-name/myinstanceid",
		},
		Spec: ibmcloudv1beta1.ServiceSpec{
			Plan:         "Lite",
			ServiceClass: "service-name",
		},
	}, r.Client.(MockClient).LastStatusUpdate())
}

func TestServiceUpdateStatusError(t *testing.T) {
	t.Parallel()
	const (
		namespace   = "mynamespace"
		serviceName = "myservice"
	)
	scheme := schemas(t)
	instance := &ibmcloudv1beta1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
		Status: ibmcloudv1beta1.ServiceStatus{
			Plan:         "Lite",
			ServiceClass: "service-name",
			InstanceID:   "myinstanceid",
		},
		Spec: ibmcloudv1beta1.ServiceSpec{
			Plan:         "Lite",
			ServiceClass: "service-name",
		},
	}

	t.Run("no such host", func(t *testing.T) {
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, instance),
				MockConfig{},
			),
			Log:    testLogger(t),
			Scheme: scheme,
		}

		result, err := r.updateStatusError(instance, "state", fmt.Errorf("no such host"))
		assert.Equal(t, ctrl.Result{
			Requeue:      true,
			RequeueAfter: 5 * time.Minute,
		}, result)
		assert.NoError(t, err)
	})

	t.Run("status update failed", func(t *testing.T) {
		r := &ServiceReconciler{
			Client: newMockClient(
				fake.NewFakeClientWithScheme(scheme, instance),
				MockConfig{StatusUpdateErr: fmt.Errorf("failed")},
			),
			Log:    testLogger(t),
			Scheme: scheme,
		}

		result, err := r.updateStatusError(instance, "state", fmt.Errorf("some error"))
		assert.Equal(t, ctrl.Result{}, result)
		assert.EqualError(t, err, "failed")
		assert.Equal(t, &ibmcloudv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: namespace,
			},
			Status: ibmcloudv1beta1.ServiceStatus{
				State:        "state",
				Message:      "some error",
				Plan:         "Lite",
				ServiceClass: "service-name",
				InstanceID:   "myinstanceid",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{
				Plan:         "Lite",
				ServiceClass: "service-name",
			},
		}, r.Client.(MockClient).LastStatusUpdate())
	})
}
