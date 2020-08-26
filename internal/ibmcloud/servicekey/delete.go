package servicekey

import (
	"strings"

	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	"github.com/IBM-Cloud/bluemix-go/session"
)

type Deleter func(session *session.Session, serviceKeyGUID string) error

var _ Deleter = Delete // Delete must fit Deleter type

func Delete(session *session.Session, serviceKeyGUID string) error {
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
