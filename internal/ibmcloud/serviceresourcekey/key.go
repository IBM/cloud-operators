package serviceresourcekey

import (
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/IBM-Cloud/bluemix-go/session"
)

type Creator func(session *session.Session, name string, crn crn.CRN, parameters map[string]interface{}) (id string, credentials map[string]interface{}, err error)

var _ Creator = Create

func Create(session *session.Session, name string, crn crn.CRN, parameters map[string]interface{}) (id string, credentials map[string]interface{}, err error) {
	controllerClient, err := controller.New(session)
	if err != nil {
		return "", nil, err
	}

	resServiceKeyAPI := controllerClient.ResourceServiceKey()
	params := controller.CreateServiceKeyRequest{
		Name:       name,
		SourceCRN:  crn,
		Parameters: parameters,
	}
	keyresp, err := resServiceKeyAPI.CreateKey(params)
	return keyresp.ID, keyresp.Credentials, err
}
