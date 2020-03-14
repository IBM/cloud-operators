package main

import (
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

	var InstanceID = "crn:v1:bluemix:public:cloudcerts:us-south:a/4448261269a14562b839e0a3019ed980:f352ce16-97c6-436c-a7b2-0f9bbe3fecf1::"
	/*string
	flag.StringVar(&InstanceID, "InstanceID", "", "Id of Instance")*/
	sess, err := session.New(c)
	if err != nil {
		log.Fatal(err)
	}

	if err != nil {
		log.Fatal(err)
	}
	data := models.Data{
		Content:                 "-----BEGIN CERTIFICATE-----\r\nMIIDsTCCApmgAwIBAgIUWzsBehxAkgLLYBUZEUpSjHkIaMowDQYJKoZIhvcNAQEL\r\nBQAwbzEMMAoGA1UEBhMDVVNBMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQH\r\nEw1TYW4gRnJhbmNpc2NvMQ0wCwYDVQQKEwRldGNkMRYwFAYDVQQLEw1ldGNkIFNl\r\nY3VyaXR5MQswCQYDVQQDEwJjYTAeFw0xNzExMTUxODAyMDBaFw0yNzExMTMxODAy\r\nMDBaMG8xDDAKBgNVBAYTA1VTQTETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UE\r\nBxMNU2FuIEZyYW5jaXNjbzENMAsGA1UEChMEZXRjZDEWMBQGA1UECxMNZXRjZCBT\r\nZWN1cml0eTELMAkGA1UEAxMCY2EwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEK\r\nAoIBAQCxjHVNtcCSCz1w9AiN7zAql0ZsPN6MNQWJ2j3iPCvmy9oi0wqSfYXTs+xw\r\nY4Q+j0dfA54+PcyIOSBQCZBeLLIwCaXN+gLkMxYEWCCVgWYUa6UY+NzPKRCfkbwG\r\noE2Ilv3R1FWIpMqDVE2rLmTb3YxSiw460Ruv4l16kodEzfs4BRcqrEiobBwaIMLd\r\n0rDJju7Q2TcioNji+HFoXV2aLN58LDgKO9AqszXxW88IKwUspfGBcsA4Zti/OHr+\r\nW+i/VxsxnQSJiAoKYbv9SkS8fUWw2hQ9SBBCKqE3jLzI71HzKgjS5TiQVZJaD6oK\r\ncw8FjexOELZd4r1+/p+nQdKqwnb5AgMBAAGjRTBDMA4GA1UdDwEB/wQEAwIBBjAS\r\nBgNVHRMBAf8ECDAGAQH/AgECMB0GA1UdDgQWBBRLfPxmhlZix1eTdBMAzMVlAnOV\r\ngTANBgkqhkiG9w0BAQsFAAOCAQEAeT2NfOt3WsBLUVcnyGMeVRQ0gXazxJXD/Z+3\r\n2RF3KClqBLuGmPUZVl0FU841J6hLlwNjS33mye7k2OHrjJcouElbV3Olxsgh/EV0\r\nJ7b7Wf4zWYHFNZz/VxwGHunsEZ+SCXUzU8OiMrEcHkOVzhtbC2veVPJzrESqd88z\r\nm1MseGW636VIcrg4fYRS9EebRPFvlwfymMd+bqLky9KsUbjNupYd/TlhpAudrIzA\r\nwO9ZUDb/0P44iOo+xURCoodxDTM0vvfZ8eJ6VZ/17HIf/a71kvk1oMqEhf060nmF\r\nIxnbK6iUqqhV8DLE1869vpFvgbDdOxP7BeabN5FXEnZFDTLDqg==\r\n-----END CERTIFICATE-----",
		Privatekey:              "",
		IntermediateCertificate: "",
	}

	importdata := models.CertificateImportData{
		Name:        "Test",
		Description: "Test Certificate",
		Data:        data,
	}

	certClient, err := v.New(sess)
	if err != nil {
		log.Fatal(err)
	}
	certificateAPI := certClient.Certificate()

	out, err := certificateAPI.ImportCertificate(InstanceID, importdata)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("out=", out)
}
