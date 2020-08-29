package config

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

type IBMCloud struct {
	Config
	ResourceGroupID string
	UAAAccessToken  string
	UAARefreshToken string
	UserDisplayName string
}

func setIfEmpty(dest *string, src string) {
	if *dest == "" {
		*dest = src
	}
}

func MustGetIBMCloud() IBMCloud {
	var config IBMCloud
	config.Config = Get()

	bxConfig, ok, err := readBluemixConfig()
	if err != nil {
		panic(err)
	}
	if ok {
		setIfEmpty(&config.Region, bxConfig.Region)
		setIfEmpty(&config.ResourceGroupID, bxConfig.ResourceGroup.GUID)
		setIfEmpty(&config.ResourceGroupName, bxConfig.ResourceGroup.Name)
		setIfEmpty(&config.UserDisplayName, bxConfig.UserDisplayName)
	}

	cfConfig, ok, err := readCFConfig()
	if err != nil {
		panic(err)
	}
	if ok {
		setIfEmpty(&config.Org, cfConfig.OrganizationFields.Name)
		setIfEmpty(&config.Space, cfConfig.SpaceFields.Name)
		setIfEmpty(&config.UAAAccessToken, cfConfig.AccessToken)
		setIfEmpty(&config.UAARefreshToken, cfConfig.RefreshToken)
	}

	for name, s := range map[string]string{
		"API key": config.APIKey,
		"Org":     config.Org,
		"Region":  config.Region,
		"Space":   config.Space,
	} {
		if s == "" {
			panic(errors.Errorf("IBM Cloud config missing value for %q.\n\nTry setting the ibmcloud CLI target or using the environment variable.", name))
		}
	}
	return config
}

type bluemixConfig struct {
	IAMToken      string
	Region        string
	ResourceGroup struct {
		Name string
		GUID string
	}
	UserDisplayName string
	UserEmail       string
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
	if err != nil {
		return config, false, err
	}

	token, err := decodeToken(config.IAMToken)
	if err != nil {
		return config, false, err
	}

	config.UserEmail = token.UserEmail
	config.UserDisplayName = token.UserEmail
	if config.UserDisplayName == "" {
		config.UserDisplayName = token.Subject
	}
	return config, true, nil
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

type iamToken struct {
	UserEmail string `json:"email"`
	Subject   string `json:"sub"`
}

func decodeToken(tokenStr string) (token iamToken, err error) {
	// lifted from bluemix-cli decode implementation
	encodedParts := strings.Split(tokenStr, ".")
	if len(encodedParts) < 3 {
		return
	}

	tokenJSON, err := base64.RawURLEncoding.DecodeString(encodedParts[1])
	if err != nil {
		return
	}
	err = json.Unmarshal(tokenJSON, &token)
	return
}
