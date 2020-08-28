package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev2/managementv2"
	"github.com/IBM-Cloud/bluemix-go/authentication"
	"github.com/IBM-Cloud/bluemix-go/endpoints"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/rest"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
	"github.com/ibm/cloud-operators/internal/config"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	testCfg config.IBMCloud
)

const (
	defaultWait = 60 * time.Second
	defaultTick = 5 * time.Second
)

func setup() error {
	testCfg = config.MustGetIBMCloud()
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
		return errors.Wrap(err, "Failed to open IBM Cloud session")
	}
	if testCfg.ResourceGroupID == "" {
		testCfg.ResourceGroupID, testCfg.ResourceGroupName, err = getResourceGroup(sess, testCfg.AccountID, testCfg.ResourceGroupName)
		if err != nil {
			return errors.Wrap(err, "Failed to resolve resource group name. Did you set account ID too?")
		}
	}
	if testCfg.UAAAccessToken == "" || testCfg.UAARefreshToken == "" {
		testCfg.UAAAccessToken, testCfg.UAARefreshToken, err = getAuthTokens(sess)
		if err != nil {
			err = errors.Wrap(err, "Failed to fetch UAA tokens")
			println("FAILED CF SETUP\n\n", err.Error()) // TODO Restore once we add int tests for CF services and switch Travis API key from service ID to functional user
		}
	}
	return nil
}

func getResourceGroup(sess *session.Session, accountID, resourceGroupName string) (id, name string, err error) {
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
		groups, err = management.ResourceGroup().FindByName(
			&managementv2.ResourceGroupQuery{AccountID: accountID},
			resourceGroupName,
		)
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

func schemas(t *testing.T) *runtime.Scheme {
	scheme, err := ibmcloudv1beta1.SchemeBuilder.Build()
	require.NoError(t, err)
	require.NoError(t, corev1.SchemeBuilder.AddToScheme(scheme))
	return scheme
}

func testLogger(t *testing.T) logr.Logger {
	opts := []zap.Option{
		zap.AddCaller(),
		zap.AddCallerSkip(1),
	}
	return zapr.NewLogger(zaptest.NewLogger(t, zaptest.WrapOptions(opts...)))
}
