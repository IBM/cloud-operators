package servicekey

import (
	"fmt"
	"strings"

	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	"github.com/IBM-Cloud/bluemix-go/session"
)

const (
	notFound = "Not Found"
)

type Getter func(session *session.Session, serviceInstanceGUID string, keyName string) (guid string, credentials map[string]interface{}, err error)

var _ Getter = Get

func Get(session *session.Session, serviceInstanceGUID string, keyName string) (guid string, credentials map[string]interface{}, err error) {
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
