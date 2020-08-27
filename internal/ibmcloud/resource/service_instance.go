package resource

import (
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/IBM-Cloud/bluemix-go/session"
)

type NotFoundError struct{}

func (e NotFoundError) Error() string {
	return "not found"
}

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

type ServiceInstanceStatusGetter func(session *session.Session, resourceGroupID, servicePlanID, externalName, instanceID string) (state string, err error)

var _ ServiceInstanceStatusGetter = GetServiceInstanceState

func GetServiceInstanceState(session *session.Session, resourceGroupID, servicePlanID, externalName, instanceID string) (state string, err error) {
	controllerClient, err := controller.New(session)
	if err != nil {
		return "", err
	}

	resServiceInstanceAPI := controllerClient.ResourceServiceInstance()
	serviceInstances, err := resServiceInstanceAPI.ListInstances(controller.ServiceInstanceQuery{
		// Warning: Do not add the ServiceID to this query
		ResourceGroupID: resourceGroupID,
		ServicePlanID:   servicePlanID,
		Name:            externalName,
	})
	if err != nil {
		return "", err
	}

	for _, instance := range serviceInstances {
		if instance.ID == instanceID {
			return instance.State, nil
		}
	}
	return "", NotFoundError{}
}

type ServiceInstanceUpdater func(session *session.Session, serviceInstanceID, externalName, servicePlanID string, params map[string]interface{}, tags []string) (state string, err error)

var _ ServiceInstanceUpdater = UpdateServiceInstance

func UpdateServiceInstance(session *session.Session, serviceInstanceID, externalName, servicePlanID string, params map[string]interface{}, tags []string) (state string, err error) {
	controllerClient, err := controller.New(session)
	if err != nil {
		return "", err
	}

	resServiceInstanceAPI := controllerClient.ResourceServiceInstance()
	serviceInstance, err := resServiceInstanceAPI.UpdateInstance(serviceInstanceID, controller.UpdateServiceInstanceRequest{
		Name:          externalName,
		ServicePlanID: servicePlanID,
		Parameters:    params,
		Tags:          tags,
	})
	return serviceInstance.State, err
}
