package controllers

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"time"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	org             = ""
	space           = ""
	region          = ""
	resourceGroup   = ""
	resourceGroupID = ""
	apikey          = os.Getenv("BLUEMIX_API_KEY")
	//auth            = os.Getenv("OW_AUTH")
	//apihost         = os.Getenv("OW_APIHOST")
	uaaAccessToken  = ""
	uaaRefreshToken = ""
	//ts              = time.Now().Unix()
)

const (
	defaultWait = 15 * time.Second
	defaultTick = defaultWait / 5
)

func setup() error {
	if err := setupAuth(); err != nil {
		return err
	}
	return setupConfigs()
}

func setupConfigs() error {
	ctx := context.Background()

	err := k8sClient.Create(ctx, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config-ibm-cloud-operator",
			Namespace: testNamespace,
		},
		Data: map[string]string{
			"org":             org,
			"space":           space,
			"region":          region,
			"resourcegroup":   resourceGroup,
			"resourcegroupid": resourceGroupID,
		},
	})
	if err != nil {
		return err
	}

	err = k8sClient.Create(ctx, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-ibm-cloud-operator",
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"api-key": []byte(apikey),
		},
	})
	if err != nil {
		return err
	}

	return k8sClient.Create(ctx, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-ibm-cloud-operator-tokens",
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"uaa_token":         []byte(uaaAccessToken),
			"uaa_refresh_token": []byte(uaaRefreshToken),
		},
	})
}

func setupAuth() error {
	// TODO remove globals, use config object instead
	if apikey == "" {
		return errors.New("set BLUEMIX_API_KEY to run tests")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	bxConfig, err := ioutil.ReadFile(home + "/.bluemix/config.json")
	if err == nil {
		err := setupBluemixAuth(bxConfig)
		if err != nil {
			return err
		}
	}
	cfConfig, err := ioutil.ReadFile(home + "/.bluemix/.cf/config.json")
	if err == nil {
		err := setupCFAuth(cfConfig)
		if err != nil {
			return err
		}
	}

	for name, s := range map[string]string{
		"org":             org,
		"space":           space,
		"region":          region,
		"uaaAccessToken":  uaaAccessToken,
		"uaaRefreshToken": uaaRefreshToken,
		"resourceGroupID": resourceGroupID,
	} {
		if s == "" {
			return errors.Errorf("Current ibmcloud target does not have a value for %q", name)
		}
	}
	return nil
}

func setupBluemixAuth(configBytes []byte) error {
	// TODO unmarshal into struct
	var config map[string]interface{}
	err := json.Unmarshal(configBytes, &config)
	if err != nil {
		return err
	}

	region = config["Region"].(string)
	if rgfields, ok := config["ResourceGroup"]; ok {
		resourceGroup = rgfields.(map[string]interface{})["Name"].(string)
		resourceGroupID = rgfields.(map[string]interface{})["GUID"].(string)
	}
	return nil
}

func setupCFAuth(configBytes []byte) error {
	// TODO unmarshal into struct
	var config map[string]interface{}
	err := json.Unmarshal(configBytes, &config)
	if err != nil {
		return err
	}

	if orgfields, ok := config["OrganizationFields"]; ok {
		org = orgfields.(map[string]interface{})["Name"].(string)
	}

	if spacefields, ok := config["SpaceFields"]; ok {
		space = spacefields.(map[string]interface{})["Name"].(string)
	}

	if accessToken, ok := config["AccessToken"]; ok {
		uaaAccessToken = accessToken.(string)
	}

	if refreshToken, ok := config["RefreshToken"]; ok {
		uaaRefreshToken = refreshToken.(string)
	}
	return nil
}
