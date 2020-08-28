package resource

import (
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/catalog"
	"github.com/IBM-Cloud/bluemix-go/session"
)

type ServiceNameGetter func(session *session.Session, serviceID string) (string, error)

func GetServiceName(session *session.Session, serviceID string) (string, error) {
	catalogClient, err := catalog.New(session)
	if err != nil {
		return "", err
	}
	resCatalogAPI := catalogClient.ResourceCatalog()
	serviceresp, err := resCatalogAPI.Get(serviceID, true)
	return serviceresp.Name, err
}
