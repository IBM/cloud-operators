package certificatemanager

import (
	"fmt"
	"net/url"

	"github.com/IBM-Cloud/bluemix-go/client"
	"github.com/IBM-Cloud/bluemix-go/models"
)

//Certificate Interface
type Certificate interface {
	ImportCertificate(InstanceID string, importData models.CertificateImportData) (models.CertificateInfo, error)
	OrderCertificate(InstanceID string, orderData models.CertificateOrderData) (models.CertificateInfo, error)
	RenewCertificate(CertID string, RenewData models.CertificateRenewData) (models.CertificateInfo, error)
	GetMetaData(CertID string) (models.CertificateInfo, error)
	GetCertData(CertID string) (models.CertificateGetData, error)
	DeleteCertificate(CertID string) error
	UpdateCertificateMetaData(CertID string, updateData models.CertificateMetadataUpdate) error
	ReimportCertificate(CertID string, reimportData models.CertificateReimportData) (models.CertificateInfo, error)
}

//Certificates client struct
type Certificates struct {
	client *client.Client
}

func newCertificateAPI(c *client.Client) Certificate {
	return &Certificates{
		client: c,
	}
}

//ImportCertificate ..
func (r *Certificates) ImportCertificate(InstanceID string, importData models.CertificateImportData) (models.CertificateInfo, error) {
	certInfo := models.CertificateInfo{}
	_, err := r.client.Post(fmt.Sprintf("/api/v3/%s/certificates/import", url.QueryEscape(InstanceID)), importData, &certInfo)
	if err != nil {
		return certInfo, err
	}
	return certInfo, err
}

//OrderCertificate ...
func (r *Certificates) OrderCertificate(InstanceID string, orderdata models.CertificateOrderData) (models.CertificateInfo, error) {
	certInfo := models.CertificateInfo{}
	_, err := r.client.Post(fmt.Sprintf("/api/v1/%s/certificates/order", url.QueryEscape(InstanceID)), orderdata, &certInfo)
	if err != nil {
		return certInfo, err
	}
	return certInfo, err
}

//RenewCertificate ...
func (r *Certificates) RenewCertificate(CertID string, renewdata models.CertificateRenewData) (models.CertificateInfo, error) {
	certInfo := models.CertificateInfo{}
	_, err := r.client.Post(fmt.Sprintf("/api/v1/certificate/%s/renew", url.QueryEscape(CertID)), renewdata, &certInfo)
	if err != nil {
		return certInfo, err
	}
	return certInfo, err
}

//GetMetaData ...
func (r *Certificates) GetMetaData(CertID string) (models.CertificateInfo, error) {
	certInfo := models.CertificateInfo{}
	_, err := r.client.Get(fmt.Sprintf("/api/v1/certificate/%s/metadata", url.QueryEscape(CertID)), &certInfo)
	if err != nil {
		return certInfo, err
	}
	return certInfo, err
}

//GetCertData ...
func (r *Certificates) GetCertData(CertID string) (models.CertificateGetData, error) {
	certInfo := models.CertificateGetData{}
	_, err := r.client.Get(fmt.Sprintf("/api/v2/certificate/%s", url.QueryEscape(CertID)), &certInfo)
	if err != nil {
		return certInfo, err
	}
	return certInfo, err
}

// DeleteCertificate ...
func (r *Certificates) DeleteCertificate(CertID string) error {
	_, err := r.client.Delete(fmt.Sprintf("/api/v2/certificate/%s", url.QueryEscape(CertID)))
	return err
}

// UpdateCertificateMetaData ...
func (r *Certificates) UpdateCertificateMetaData(CertID string, updatemetaData models.CertificateMetadataUpdate) error {
	_, err := r.client.Post(fmt.Sprintf("/api/v3/certificate/%s", url.QueryEscape(CertID)), updatemetaData, nil)
	return err
}

// ReimportCertificate ...
func (r *Certificates) ReimportCertificate(CertID string, reimportData models.CertificateReimportData) (models.CertificateInfo, error) {
	certInfo := models.CertificateInfo{}
	_, err := r.client.Put(fmt.Sprintf("/api/v1/certificate/%s", url.QueryEscape(CertID)), reimportData, &certInfo)
	if err != nil {
		return certInfo, err
	}
	return certInfo, err
}
