package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev2/managementv2"
	"github.com/IBM-Cloud/bluemix-go/authentication"
	"github.com/IBM-Cloud/bluemix-go/endpoints"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/rest"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/kelseyhightower/envconfig"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	testCfg testConfig
)

type testConfig struct {
	APIKey            string `envconfig:"bluemix_api_key"`
	Org               string `envconfig:"bluemix_org"`
	Region            string `envconfig:"bluemix_region"`
	ResourceGroupID   string
	ResourceGroupName string `envconfig:"bluemix_resource_group"`
	Space             string `envconfig:"bluemix_space"`
	UAAAccessToken    string
	UAARefreshToken   string
}

func init() {
	bxConfig, ok, err := readBluemixConfig()
	if err != nil {
		panic(err)
	}
	if ok {
		testCfg.Region = bxConfig.Region
		testCfg.ResourceGroupID = bxConfig.ResourceGroup.GUID
		testCfg.ResourceGroupName = bxConfig.ResourceGroup.Name
	}

	cfConfig, ok, err := readCFConfig()
	if err != nil {
		panic(err)
	}
	if ok {
		testCfg.Org = cfConfig.OrganizationFields.Name
		testCfg.Space = cfConfig.SpaceFields.Name
		testCfg.UAAAccessToken = cfConfig.AccessToken
		testCfg.UAARefreshToken = cfConfig.RefreshToken
	}

	envconfig.MustProcess("", &testCfg) // must be last: envconfig overrides bx config

	for name, s := range map[string]string{
		"API key": testCfg.APIKey,
		"Org":     testCfg.Org,
		"Region":  testCfg.Region,
		"Space":   testCfg.Space,
	} {
		if s == "" {
			panic(fmt.Sprintf("Test config missing value for %q.\n\nTry setting the ibmcloud CLI target or using the environment variable.", name))
		}
	}
}

const (
	defaultWait = 60 * time.Second
	defaultTick = 5 * time.Second
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
			"org":             testCfg.Org,
			"space":           testCfg.Space,
			"region":          testCfg.Region,
			"resourcegroup":   testCfg.ResourceGroupName,
			"resourcegroupid": testCfg.ResourceGroupID,
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
			"api-key": []byte(testCfg.APIKey),
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
			"uaa_token":         []byte(testCfg.UAAAccessToken),
			"uaa_refresh_token": []byte(testCfg.UAARefreshToken),
		},
	})
}

func setupAuth() error {
	// see init() for variables with hard requirements (i.e. don't need to be checked here)

	sess, err := session.New(&bluemix.Config{
		EndpointLocator: endpoints.NewEndpointLocator(testCfg.Region),
		Region:          testCfg.Region,
		BluemixAPIKey:   testCfg.APIKey,
	})
	if err != nil {
		return err
	}
	if testCfg.ResourceGroupID == "" {
		testCfg.ResourceGroupID, testCfg.ResourceGroupName, err = getResourceGroup(sess, testCfg.ResourceGroupName)
		if err != nil {
			return err
		}
	}
	if testCfg.UAAAccessToken == "" || testCfg.UAARefreshToken == "" {
		testCfg.UAAAccessToken, testCfg.UAARefreshToken, err = getAuthTokens(sess)
		if err != nil {
			return err
		}
	}
	return nil
}

type bluemixConfig struct {
	Region        string
	ResourceGroup struct {
		Name string
		GUID string
	}
}

func getBluemixHome() (string, error) {
	home := os.Getenv("IBMCLOUD_HOME")
	if home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bluemix"), nil
}

// readBluemixConfig attempts to read the ibmcloud CLI config file
// If file does not exist, then 'ok' is set to false and 'err' is nil.
func readBluemixConfig() (config bluemixConfig, ok bool, err error) {
	bxHome, err := getBluemixHome()
	if err != nil {
		return config, false, err
	}
	buf, err := ioutil.ReadFile(filepath.Join(bxHome, "config.json"))
	if os.IsNotExist(err) {
		return config, false, nil
	}
	if err != nil {
		return config, false, err
	}
	err = json.Unmarshal(buf, &config)
	return config, err == nil, err
}

type cfConfig struct {
	OrganizationFields struct {
		Name string
	}
	SpaceFields struct {
		Name string
	}
	AccessToken  string
	RefreshToken string
}

// readCFConfig attempts to read the CF CLI config file
// If file does not exist, then 'ok' is set to false and 'err' is nil.
func readCFConfig() (config cfConfig, ok bool, err error) {
	bxHome, err := getBluemixHome()
	if err != nil {
		return config, false, err
	}
	buf, err := ioutil.ReadFile(filepath.Join(bxHome, ".cf", "config.json"))
	if os.IsNotExist(err) {
		return config, false, nil
	}
	if err != nil {
		return config, false, err
	}
	err = json.Unmarshal(buf, &config)
	return config, err == nil, err
}

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
