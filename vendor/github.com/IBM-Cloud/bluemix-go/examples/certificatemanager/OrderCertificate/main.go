package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/IBM-Cloud/bluemix-go"
	v "github.com/IBM-Cloud/bluemix-go/api/certificatemanager"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/IBM-Cloud/bluemix-go/trace"
)

func main() {

	c := new(bluemix.Config)

	trace.Logger = trace.NewLogger("true")

	var InstanceID string
	flag.StringVar(&InstanceID, "InstanceID", "", "Id of Instance")
	sess, err := session.New(c)
	if err != nil {
		log.Fatal(err)
	}

	if err != nil {
		log.Fatal(err)
	}
	orderdata := models.CertificateOrderData{
		Name:                   "Test",
		Description:            "Test Certificate",
		Domains:                []string{"ca"},
		DomainValidationMethod: "dns-01",
		DNSProviderInstanceCrn: "",
		Issuer:                 "",
		Algorithm:              "",
		KeyAlgorithm:           "",
	}

	certClient, err := v.New(sess)
	if err != nil {
		log.Fatal(err)
	}
	certificateAPI := certClient.Certificate()

	out, err := certificateAPI.OrderCertificate(InstanceID, orderdata)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("out=", out)
}
