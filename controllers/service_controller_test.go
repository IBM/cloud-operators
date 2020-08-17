package controllers

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	bxcontroller "github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
	"github.com/ibm/cloud-operators/internal/ibmcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"k8s.io/apimachinery/pkg/api/errors"
)

func TestService(t *testing.T) {
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
	return getServiceInstance(instances, service.Status.InstanceID)
}

func TestServiceV1Alpha1Compat(t *testing.T) {
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
