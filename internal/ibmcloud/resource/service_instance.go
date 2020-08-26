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
