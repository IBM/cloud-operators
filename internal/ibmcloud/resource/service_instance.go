package resource

import (
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/IBM-Cloud/bluemix-go/session"
)

type ServiceInstanceCRNGetter func(session *session.Session, instanceID string) (instanceCRN crn.CRN, serviceID string, err error)

var _ ServiceInstanceCRNGetter = GetServiceInstanceCRN

func GetServiceInstanceCRN(session *session.Session, instanceID string) (instanceCRN crn.CRN, serviceID string, err error) {
	controllerClient, err := controller.New(session)
	if err != nil {
		return crn.CRN{}, "", err
	}
	resServiceInstanceAPI := controllerClient.ResourceServiceInstance()
	model, err := resServiceInstanceAPI.GetInstance(instanceID)
	return model.Crn, model.ServiceID, err
}

type ServiceInstanceCreator func(session *session.Session, externalName, servicePlanID, resourceGroupID, targetCrn string, params map[string]interface{}, tags []string) (id, state string, err error)

var _ ServiceInstanceCreator = CreateServiceInstance

func CreateServiceInstance(session *session.Session, externalName, servicePlanID, resourceGroupID, targetCrn string, params map[string]interface{}, tags []string) (id, state string, err error) {
	controllerClient, err := controller.New(session)
	if err != nil {
		return "", "", err
	}
	resServiceInstanceAPI := controllerClient.ResourceServiceInstance()
	serviceInstance, err := resServiceInstanceAPI.CreateInstance(controller.CreateServiceInstanceRequest{
		Name:            externalName,
		ServicePlanID:   servicePlanID,
		ResourceGroupID: resourceGroupID,
		TargetCrn:       targetCrn,
		Parameters:      params,
		Tags:            tags,
	})
	return serviceInstance.ID, serviceInstance.LastOperation.State, err
}
