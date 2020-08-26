package resourceservicekey

import (
	"strings"

	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/session"
)

type Deleter func(session *session.Session, keyID string) error

var _ Deleter = Delete

func Delete(session *session.Session, keyID string) error {
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
