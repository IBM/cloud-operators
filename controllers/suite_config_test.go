package controllers

import (
	"context"
	"os"
	"time"

	"github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev2/managementv2"
	"github.com/IBM-Cloud/bluemix-go/authentication"
	"github.com/IBM-Cloud/bluemix-go/endpoints"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/rest"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	org             = os.Getenv("BLUEMIX_ORG")
	space           = os.Getenv("BLUEMIX_SPACE")
	region          = os.Getenv("BLUEMIX_REGION")
	resourceGroup   = os.Getenv("BLUEMIX_RESOURCE_GROUP")
	resourceGroupID = ""
	apiKey          = os.Getenv("BLUEMIX_API_KEY")
	uaaAccessToken  = ""
	uaaRefreshToken = ""
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
			"api-key": []byte(apiKey),
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
	if apiKey == "" {
		return errors.New("set BLUEMIX_API_KEY to run tests")
	}

	if region == "" {
		return errors.New("set BLUEMIX_REGION to run tests")
	}

	sess, err := session.New(&bluemix.Config{
		EndpointLocator: endpoints.NewEndpointLocator(region),
		Region:          region,
		BluemixAPIKey:   apiKey,
	})
	if err != nil {
		return err
	}
	resourceGroupID, resourceGroup, err = getResourceGroup(sess, resourceGroup)
	if err != nil {
		return err
	}
	uaaAccessToken, uaaRefreshToken, err = getAuthTokens(sess)
	if err != nil {
		return err
	}

	/*
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
	*/

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

/*
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
*/

func getResourceGroup(sess *session.Session, resourceGroupName string) (id, name string, err error) {
	management, err := managementv2.New(sess)
	if err != nil {
		return "", "", err
	}
	var groups []models.ResourceGroupv2
	if resourceGroupName == "" {
		groups, err = management.ResourceGroup().List(&managementv2.ResourceGroupQuery{
			Default: true,
		})
	} else {
		groups, err = management.ResourceGroup().FindByName(&managementv2.ResourceGroupQuery{}, resourceGroupName)
	}
	if err != nil {
		return "", "", err
	}
	if len(groups) != 1 {
		return "", "", errors.Errorf("failed to look up resource group ID: Expected 1 resource group by name %q, found %d: %v", resourceGroupName, len(groups), groups)
	}
	group := groups[0]
	return group.ID, group.Name, nil
}

func getAuthTokens(sess *session.Session) (uaaAccessToken, uaaRefreshToken string, err error) {
	config := sess.Config.Copy()
	tokenRefreher, err := authentication.NewUAARepository(config, &rest.Client{HTTPClient: config.HTTPClient})
	if err != nil {
		return "", "", err
	}
	err = authentication.PopulateTokens(tokenRefreher, config)
	if err != nil {
		return "", "", err
	}
	if config.UAAAccessToken == "" || config.UAARefreshToken == "" {
		return "", "", errors.New("Fetching UAA tokens failed")
	}
	return config.UAAAccessToken, config.UAARefreshToken, nil
}
