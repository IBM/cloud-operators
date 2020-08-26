package servicekey

import (
	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	"github.com/IBM-Cloud/bluemix-go/session"
)

type ServiceKeys interface {
	Create(session *session.Session, serviceInstanceGUID string, keyName string, params map[string]interface{}) (guid string, credentials map[string]interface{}, err error)
}

type serviceKeys struct {
}

func New() ServiceKeys {
	return &serviceKeys{}
}

func (s *serviceKeys) Create(session *session.Session, serviceInstanceGUID string, keyName string, params map[string]interface{}) (guid string, credentials map[string]interface{}, err error) {
	bxClient, err := mccpv2.New(session)
	if err != nil {
		return "", nil, err
	}
	serviceKeys := bxClient.ServiceKeys()
	key, err := serviceKeys.Create(serviceInstanceGUID, keyName, params)
	if err != nil {
		return "", nil, err
	}
	return key.Metadata.GUID, key.Entity.Credentials, nil
}
