package servicekey

import (
	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	"github.com/IBM-Cloud/bluemix-go/session"
)

type Creator func(session *session.Session, serviceInstanceGUID string, keyName string, params map[string]interface{}) (guid string, credentials map[string]interface{}, err error)

var _ Creator = Create // Create must fit Creator type

func Create(session *session.Session, serviceInstanceGUID string, keyName string, params map[string]interface{}) (guid string, credentials map[string]interface{}, err error) {
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
