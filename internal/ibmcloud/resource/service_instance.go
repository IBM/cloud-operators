package resource

import (
	"fmt"
	"strings"

	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/bmxerror"
	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/go-logr/logr"
)

type NotFoundError struct {
	Err error
}

func (n NotFoundError) Error() string {
	return n.Err.Error()
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
	if err != nil {
		return "", "", err
	}
	return serviceInstance.ID, serviceInstance.LastOperation.State, nil
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
	return "", NotFoundError{fmt.Errorf("not found")}
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

type ServiceInstanceDeleter func(session *session.Session, instanceID string, logt logr.Logger) error

var _ ServiceInstanceDeleter = DeleteServiceInstance

func DeleteServiceInstance(session *session.Session, instanceID string, logt logr.Logger) error {
	controllerClient, err := controller.New(session)
	if err != nil {
		return err
	}
	resServiceInstanceAPI := controllerClient.ResourceServiceInstance()
	err = resServiceInstanceAPI.DeleteInstance(instanceID, true)
	bmxerr, ok := err.(bmxerror.Error)
	if !ok {
		return err
	}

	if bmxerr.Code() == "410" { // Not Found
		logt.Info("Resource not found, nothing to to", "ServiceInstance", err.Error())
		return nil // Nothing to do here, service not found
	}
	if strings.Contains(err.Error(), "cannot be found") { // Not Found
		logt.Info("Resource not found, nothing to to", "ServiceInstance", err.Error())
		return nil // Nothing to do here, service not found
	}
	if strings.Contains(err.Error(), "Request failed with status code: 410") { // Not Found
		logt.Info("Resource not found, nothing to to", "ServiceInstance", err.Error())
		return nil // Nothing to do here, service not found
	}
	if strings.Contains(err.Error(), "Instance is pending reclamation") { // Not Found
		logt.Info("Resource not found, nothing to to", "ServiceInstance", err.Error())
		return nil // Nothing to do here, service not found
	}
	return err
}

type ServiceAliasInstanceGetter func(session *session.Session, instanceID, resourceGroupID, servicePlanID, externalName string, logt logr.Logger) (id, state string, err error)

var _ ServiceAliasInstanceGetter = GetServiceAliasInstance

// GetServiceAliasInstance retrieves a service instance ID and state with optional 'instanceID'. instanceID is only used to resolve name conflicts.
func GetServiceAliasInstance(session *session.Session, instanceID, resourceGroupID, servicePlanID, externalName string, logt logr.Logger) (id, state string, err error) {
	controllerClient, err := controller.New(session)
	if err != nil {
		return "", "", err
	}

	resServiceInstanceAPI := controllerClient.ResourceServiceInstance()
	serviceInstances, err := resServiceInstanceAPI.ListInstances(controller.ServiceInstanceQuery{
		// Warning: Do not add the ServiceID to this query
		ResourceGroupID: resourceGroupID,
		ServicePlanID:   servicePlanID,
		Name:            externalName,
	})
	if err != nil {
		return "", "", err
	}

	if len(serviceInstances) == 0 {
		return "", "", NotFoundError{fmt.Errorf("no service instances found for Alias plan")}
	}

	// if only one instance with that name is found, then instanceID is not required, but if present it should match the ID
	if len(serviceInstances) == 1 {
		logt.Info("Found 1 service instance for `Alias` plan:", "InstanceID", serviceInstances[0].ID)
		if instanceID != "" { // check matches ID
			if instanceID != serviceInstances[0].ID {
				return "", "", NotFoundError{fmt.Errorf("instance ID %s does not match instance ID %s found", instanceID, serviceInstances[0].ID)}
			}
		}
		return serviceInstances[0].ID, serviceInstances[0].State, nil
	}

	// if there is more then 1 service instance with the same name, then the instance ID annotation must be present
	logt.Info("Multiple service instances for `Alias` plan and instance")
	if instanceID != "" {
		var serviceInstance *models.ServiceInstance
		for _, instance := range serviceInstances {
			if instance.ID == instanceID {
				instanceCopy := instance
				serviceInstance = &instanceCopy
				break
			}
		}
		if serviceInstance == nil {
			return "", "", NotFoundError{fmt.Errorf("multiple services instances found, but none matched instance ID %s", instanceID)}
		}

		if serviceInstance.ServiceID == "" {
			return "", "", NotFoundError{fmt.Errorf("could not find matching instance with instance ID %s", instanceID)}
		}
		logt.Info("Found service instances for `Alias` plan and instance", "InstanceID", instanceID)
		return serviceInstance.ID, serviceInstance.State, nil
	}
	return "", "", fmt.Errorf("multiple instance with same name found, and plan `Alias` requires `ibmcloud.ibm.com/instanceId` annotation")
}
