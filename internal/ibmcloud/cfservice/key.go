package cfservice

import (
	"fmt"
	"strings"

	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	"github.com/IBM-Cloud/bluemix-go/session"
)

const (
	notFound = "Not Found"
)

type KeyGetter func(session *session.Session, serviceInstanceGUID string, keyName string) (guid string, credentials map[string]interface{}, err error)

var _ KeyGetter = GetKey

func GetKey(session *session.Session, serviceInstanceGUID string, keyName string) (guid string, credentials map[string]interface{}, err error) {
	bxClient, err := mccpv2.New(session)
	if err != nil {
		return "", nil, err
	}
	serviceKeys := bxClient.ServiceKeys()
	key, err := serviceKeys.FindByName(serviceInstanceGUID, keyName)
	if err != nil {
		if strings.Contains(err.Error(), "doesn't exist") {
			return "", nil, fmt.Errorf(notFound)
		}
		return "", nil, err
	}
	_, contentsContainRedacted := key.Credentials["REDACTED"]
	if contentsContainRedacted {
		return "", nil, fmt.Errorf(notFound)
	}

	return key.GUID, key.Credentials, nil
}

type KeyCreator func(session *session.Session, serviceInstanceGUID string, keyName string, params map[string]interface{}) (guid string, credentials map[string]interface{}, err error)

var _ KeyCreator = CreateKey

func CreateKey(session *session.Session, serviceInstanceGUID string, keyName string, params map[string]interface{}) (guid string, credentials map[string]interface{}, err error) {
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

type KeyDeleter func(session *session.Session, serviceKeyGUID string) error

var _ KeyDeleter = DeleteKey

func DeleteKey(session *session.Session, serviceKeyGUID string) error {
	bxClient, err := mccpv2.New(session)
	if err != nil {
		return err
	}
	serviceKeys := bxClient.ServiceKeys()
	err = serviceKeys.Delete(serviceKeyGUID)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "410") && !strings.Contains(err.Error(), "404") {
		// we do not propagate an error if the service or credential no longer exist
		return err
	}
	return nil
}
