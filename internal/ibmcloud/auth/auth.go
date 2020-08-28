package auth

import (
	"net/http"
	"strings"

	"github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/authentication"
	"github.com/IBM-Cloud/bluemix-go/endpoints"
	"github.com/IBM-Cloud/bluemix-go/rest"
)

type Authenticator func(apiKey, region string) (Credentials, error)

type Credentials struct {
	IAMAccessToken  string
	IAMRefreshToken string
	UAAAccessToken  string
	UAARefreshToken string
}

type InvalidConfigError struct {
	error
}

type authenticator struct {
	client *http.Client
}

func New(client *http.Client) Authenticator {
	return authenticator{
		client: client,
	}.Authenticate
}

func (a authenticator) Authenticate(apiKey, region string) (Credentials, error) {
	config := &bluemix.Config{
		EndpointLocator: endpoints.NewEndpointLocator(region),
	}

	iamAuth, err := authentication.NewIAMAuthRepository(config, &rest.Client{HTTPClient: a.client})
	if err != nil {
		return Credentials{}, InvalidConfigError{err}
	}
	err = iamAuth.AuthenticateAPIKey(apiKey)
	return Credentials{
		IAMAccessToken:  config.IAMAccessToken,
		IAMRefreshToken: config.IAMRefreshToken,
		UAAAccessToken:  config.UAAAccessToken,
		UAARefreshToken: config.UAARefreshToken,
	}, err
}

func (c *Credentials) UnmarshalSecret(secretData map[string][]byte) {
	c.IAMAccessToken = string(secretData["access_token"])
	c.IAMRefreshToken = string(secretData["refresh_token"])
	c.UAAAccessToken = string(secretData["uaa_token"])
	c.UAARefreshToken = string(secretData["uaa_refresh_token"])
}

func (c *Credentials) MarshalSecret() map[string][]byte {
	return map[string][]byte{
		"access_token":      []byte(c.IAMAccessToken),
		"refresh_token":     []byte(c.IAMRefreshToken),
		"uaa_token":         []byte(strings.Replace(c.UAAAccessToken, "Bearer ", "bearer ", 1)),
		"uaa_refresh_token": []byte(c.UAARefreshToken),
	}
}
