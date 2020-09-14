package cfservice

import (
	"strings"

	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/go-logr/logr"
)

type NotFoundError struct {
	Err error
}

func (e NotFoundError) Error() string {
	return e.Err.Error()
}

type InstanceGetter func(session *session.Session, name string) (guid, state string, err error)

var _ InstanceGetter = GetInstance

func GetInstance(session *session.Session, name string) (guid, state string, err error) {
	bxClient, err := mccpv2.New(session)
	if err != nil {
		return "", "", err
	}
	serviceInstance, err := bxClient.ServiceInstances().FindByName(name)
	if err != nil {
		if strings.Contains(err.Error(), "doesn't exist") {
			err = NotFoundError{Err: err}
		}
		return "", "", err
	}
	return serviceInstance.GUID, serviceInstance.LastOperation.State, nil
}

type InstanceCreator func(session *session.Session, externalName, planID, spaceID string, params map[string]interface{}, tags []string) (guid, state string, err error)

var _ InstanceCreator = CreateInstance

func CreateInstance(session *session.Session, externalName, planID, spaceID string, params map[string]interface{}, tags []string) (guid, state string, err error) {
	bxClient, err := mccpv2.New(session)
	if err != nil {
		return "", "", err
	}
	serviceInstance, err := bxClient.ServiceInstances().Create(mccpv2.ServiceInstanceCreateRequest{
		Name:      externalName,
		PlanGUID:  planID,
		SpaceGUID: spaceID,
		Params:    params,
		Tags:      tags,
	})
	if err != nil {
		return "", "", err
	}
	return serviceInstance.Metadata.GUID, serviceInstance.Entity.LastOperation.State, nil
}

type InstanceDeleter func(session *session.Session, instanceID string, logt logr.Logger) error

var _ InstanceDeleter = DeleteInstance

func DeleteInstance(session *session.Session, instanceID string, logt logr.Logger) error {
	bxClient, err := mccpv2.New(session)
	if err != nil {
		return err
	}

	serviceInstanceAPI := bxClient.ServiceInstances()
	err = serviceInstanceAPI.Delete(instanceID, true, true)                                  // async, recursive (i.e. delete credentials)
	if err != nil && strings.Contains(err.Error(), "Request failed with status code: 410") { // Not Found
		logt.Info("Resource not found, nothing to to", "ServiceInstance", err.Error())
		return nil // Nothing to do here, service not found
	}
	return err
}
