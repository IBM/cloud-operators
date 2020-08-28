package resource

import (
	"fmt"
	"strings"

	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/IBM-Cloud/bluemix-go/session"
)

const (
	notFound = "Not Found"
)

type KeyCreator func(session *session.Session, name string, crn crn.CRN, parameters map[string]interface{}) (id string, credentials map[string]interface{}, err error)

var _ KeyCreator = CreateKey

func CreateKey(session *session.Session, name string, crn crn.CRN, parameters map[string]interface{}) (id string, credentials map[string]interface{}, err error) {
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

type KeyDeleter func(session *session.Session, keyID string) error

var _ KeyDeleter = DeleteKey

func DeleteKey(session *session.Session, keyID string) error {
	controllerClient, err := controller.New(session)
	if err != nil {
		return err
	}

	resServiceKeyAPI := controllerClient.ResourceServiceKey()
	err = resServiceKeyAPI.DeleteKey(keyID)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "410") && !strings.Contains(err.Error(), "404") {
		// we do not propagate an error if the service or credential no longer exist
		return err
	}
	return nil
}

type KeyGetter func(session *session.Session, keyID string) (guid, name string, credentials map[string]interface{}, err error)

var _ KeyGetter = GetKey

func GetKey(session *session.Session, keyID string) (guid, name string, credentials map[string]interface{}, err error) {
	controllerClient, err := controller.New(session)
	if err != nil {
		return "", "", nil, err
	}

	resServiceKeyAPI := controllerClient.ResourceServiceKey()
	keyresp, err := resServiceKeyAPI.GetKey(keyID)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return "", "", nil, fmt.Errorf(notFound)
		} else if strings.Contains(err.Error(), "not found") {
			return "", "", nil, fmt.Errorf(notFound)
		}
		return "", "", nil, err
	}
	_, contentsContainRedacted := keyresp.Credentials["REDACTED"]
	if contentsContainRedacted {
		return "", "", nil, fmt.Errorf(notFound)
	}
	return keyresp.ID, keyresp.Name, keyresp.Credentials, nil
}
