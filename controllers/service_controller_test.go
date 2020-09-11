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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
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
			Message: "value and ValueFrom properties are mutually exclusive (for hello variable)",
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

	t.Run("create failed", func(t *testing.T) {
		scheme := schemas(t)
		objects := []runtime.Object{
			&ibmcloudv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Status:     ibmcloudv1beta1.ServiceStatus{Plan: "Lite"},
				Spec:       ibmcloudv1beta1.ServiceSpec{Plan: "Lite"},
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
				Message: "failed",
				Plan:    "Lite",
			},
			Spec: ibmcloudv1beta1.ServiceSpec{Plan: "Lite"},
		}, r.Client.(MockClient).LastStatusUpdate())
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
}
