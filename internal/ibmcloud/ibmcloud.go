package ibmcloud

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/api/account/accountv2"
	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/catalog"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/IBM-Cloud/bluemix-go/endpoints"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/go-logr/logr"
	ibmcloudv1 "github.com/ibm/cloud-operators/api/v1"
	"github.com/ibm/cloud-operators/internal/config"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	aliasPlan    = "alias"
	icoConfigMap = "ibmcloud-operator-config"
	seedSecret   = "ibmcloud-operator-secret"
	seedDefaults = "ibmcloud-operator-defaults"
	seedTokens   = "ibmcloud-operator-tokens"
)

// Info kept all the needed client API resource and instance Info
type Info struct {
	Session          *session.Session
	BXClient         mccpv2.MccpServiceAPI
	ResourceClient   controller.ResourceControllerAPI
	CatalogClient    catalog.ResourceCatalogAPI
	Account          *accountv2.Account
	Org              *mccpv2.Organization
	Region           *models.Region
	Space            *mccpv2.Space
	ResourceGroupID  string
	ResourceLocation string
	InstanceName     string
	InstanceID       string
	InstanceCRN      crn.CRN
	InstanceAlias    string
	ServiceClass     string
	ServiceClassType string
	ServicePlan      string
	BxPlan           *mccpv2.ServicePlan
	ServicePlanID    string
	TargetCrn        string
	Token            string
	Context          ibmcloudv1.ResourceContext
}

// GetInfo initializes sessions and sets up a struct to faciliate making calls to bx
func GetInfo(logt logr.Logger, r client.Client, instance *ibmcloudv1.Service) (*Info, error) {
	bxConfig, err := getBxConfig(logt, r, instance)
	if err != nil {
		return nil, err
	}

	ibmCloudContext, err := getIBMCloudDefaultContext(logt, r, instance)
	if err != nil {
		return nil, err
	}

	return getInfoHelper(logt, r, &bxConfig, ibmCloudContext, instance)
}

func getInfoHelper(logt logr.Logger, r client.Client, config *bluemix.Config, nctx ibmcloudv1.ResourceContext, instance *ibmcloudv1.Service) (*Info, error) {
	servicename := instance.Spec.ServiceClass
	servicetype := instance.Spec.ServiceClassType
	serviceplan := instance.Spec.Plan

	useCtx := nctx
	logt.Info("Context", "is", useCtx)
	if useCtx.ResourceLocation == "" {
		useCtx.ResourceLocation = useCtx.Region
	}

	sess, err := session.New(config)
	if err != nil {
		return nil, err
	}

	if servicetype == "CF" {
		bxclient, err := mccpv2.New(sess)
		if err != nil {
			return nil, err
		}

		orgAPI := bxclient.Organizations()
		myorg, err := orgAPI.FindByName(useCtx.Org, useCtx.Region)
		if err != nil {
			return nil, err
		}

		regionAPI := bxclient.Regions()
		regionList, err := regionAPI.FindRegionByName(sess.Config.Region)
		if err != nil {
			return nil, err
		}

		spaceAPI := bxclient.Spaces()
		myspace, err := spaceAPI.FindByNameInOrg(myorg.GUID, useCtx.Space, sess.Config.Region)
		if err != nil {
			return nil, err
		}

		servicePlan := &mccpv2.ServicePlan{}
		if strings.ToLower(instance.Spec.Plan) != aliasPlan {
			serviceOfferingAPI := bxclient.ServiceOfferings()
			myserviceOff, err := serviceOfferingAPI.FindByLabel(servicename)
			if err != nil {
				return nil, err
			}

			servicePlanAPI := bxclient.ServicePlans()
			servicePlan, err = servicePlanAPI.FindPlanInServiceOffering(myserviceOff.GUID, serviceplan)
			if err != nil {
				return nil, err
			}
		}

		return &Info{
			BXClient:         bxclient,
			Org:              myorg,
			Space:            myspace,
			Region:           regionList,
			ResourceLocation: useCtx.ResourceLocation,
			Session:          sess,
			ServiceClass:     servicename,
			ServiceClassType: servicetype,
			BxPlan:           servicePlan,
			Context:          useCtx,
		}, nil
	}

	IAMAccessToken, IAMRefreshToken, UAAAccessToken, UAARefreshToken, err := getIamToken(logt, r, instance)
	if err == nil {
		config.IAMAccessToken = IAMAccessToken
		config.IAMRefreshToken = IAMRefreshToken
		config.UAAAccessToken = UAAAccessToken
		config.UAARefreshToken = UAARefreshToken
	}

	controllerClient, err := controller.New(sess)

	if err != nil {
		return nil, err
	}

	catalogClient, err := catalog.New(sess)
	if err != nil {
		return nil, err
	}

	resCatalogAPI := catalogClient.ResourceCatalog()

	service, err := resCatalogAPI.FindByName(servicename, true)
	if err != nil {
		return nil, err
	}

	servicePlanID := ""
	catalogCRN := ""
	if strings.ToLower(instance.Spec.Plan) != aliasPlan {
		servicePlanID, err = resCatalogAPI.GetServicePlanID(service[0], serviceplan)
		if err != nil {
			return nil, err
		}
		if servicePlanID == "" {
			_, err := resCatalogAPI.GetServicePlan(serviceplan)
			if err != nil {
				return nil, err
			}
			servicePlanID = serviceplan
		}

		deployments, err := resCatalogAPI.ListDeployments(servicePlanID)
		if err != nil {
			return nil, err
		}

		if len(deployments) == 0 {
			return nil, fmt.Errorf("Failed: No deployment found for service plan : %s", serviceplan)
		}

		supportedDeployments := []models.ServiceDeployment{}
		supportedLocations := make(map[string]bool)
		for _, d := range deployments {
			if d.Metadata.RCCompatible {
				deploymentLocation := d.Metadata.Deployment.Location
				supportedLocations[deploymentLocation] = true
				if deploymentLocation == useCtx.Region {
					supportedDeployments = append(supportedDeployments, d)
				}
			}
		}

		if len(supportedDeployments) == 0 {
			locationList := make([]string, 0, len(supportedLocations))
			for l := range supportedLocations {
				locationList = append(locationList, l)
			}
			return nil, fmt.Errorf("Failed: No deployment found for service plan %s at location %s. Valid location(s) are: %q.\nUse service instance example if the service is a Cloud Foundry service", serviceplan, useCtx.Region, locationList)
		}
		catalogCRN = supportedDeployments[0].CatalogCRN
	}

	return &Info{
		ResourceClient:   controllerClient, // IAMServiceAPI
		CatalogClient:    catalogClient,
		ResourceGroupID:  useCtx.ResourceGroupID,
		ResourceLocation: useCtx.ResourceLocation,
		Session:          sess,
		ServiceClass:     servicename,
		ServiceClassType: servicetype,
		ServicePlan:      serviceplan,
		ServicePlanID:    servicePlanID,
		TargetCrn:        catalogCRN,
		Context:          useCtx,
	}, nil
}

func getBxConfig(logt logr.Logger, r client.Client, instance *ibmcloudv1.Service) (bluemix.Config, error) {
	secretName := seedSecret
	secretNameSpace := instance.ObjectMeta.Namespace

	secret := &v1.Secret{}

	err := getConfigOrSecret(logt, r, secretNameSpace, secretName, secret)
	if err != nil {
		logt.Info("Unable to get IBM Cloud Operator secret in namespace", secretNameSpace, err)
		return bluemix.Config{}, err
	}

	APIKey := string(secret.Data["api-key"])

	regionb, ok := secret.Data["region"]
	if !ok {
		logt.Info("Setting default region to us-south")
		regionb = []byte("us-south")
	}
	region := string(regionb)
	c := bluemix.Config{
		EndpointLocator: endpoints.NewEndpointLocator(region),
	}
	c.Region = region
	c.BluemixAPIKey = APIKey

	return c, nil
}

func getIBMCloudDefaultContext(logt logr.Logger, r client.Client, instance *ibmcloudv1.Service) (ibmcloudv1.ResourceContext, error) {
	// If the object already has the context set in its Status, then we don't read from the configmap
	if !reflect.DeepEqual(instance.Status.Context, ibmcloudv1.ResourceContext{}) {
		return instance.Status.Context, nil
	}

	cm := &v1.ConfigMap{}
	cmName := seedDefaults
	cmNameSpace := instance.ObjectMeta.Namespace

	err := getConfigOrSecret(logt, r, cmNameSpace, cmName, cm)
	if err != nil {
		logt.Info("Unable to get IBM Cloud Operator configmap in namespace", cmNameSpace, err)
		return ibmcloudv1.ResourceContext{}, err
	}

	ibmCloudContext := getIBMCloudContext(instance, cm)
	return ibmCloudContext, nil
}

func getIamToken(logt logr.Logger, r client.Client, instance *ibmcloudv1.Service) (string, string, string, string, error) {
	secretName := seedTokens
	secretNameSpace := instance.ObjectMeta.Namespace

	secret := &v1.Secret{}
	err := getConfigOrSecret(logt, r, secretNameSpace, secretName, secret)
	if err != nil {
		logt.Info("Unable to get IBM Cloud Operator IAM token in namespace", secretNameSpace, err)
		return "", "", "", "", err
	}

	return string(secret.Data["access_token"]), string(secret.Data["refresh_token"]), string(secret.Data["uaa_refresh_token"]), string(secret.Data["uaa_token"]), nil
}

func getConfigOrSecret(logt logr.Logger, r client.Client, instanceNamespace string, objName string, obj client.Object) error {
	defaultNamespace, isManagement := getDefaultNamespace(logt, r)
	if isManagement {
		objName = instanceNamespace + "-" + objName
		err := r.Get(context.TODO(), types.NamespacedName{Name: objName, Namespace: defaultNamespace}, obj)
		if err != nil {
			logt.Info("Unable to find secret or config in namespace", objName, defaultNamespace)
			return err
		}
		return nil
	}
	err := r.Get(context.TODO(), types.NamespacedName{Name: objName, Namespace: instanceNamespace}, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			err = r.Get(context.TODO(), types.NamespacedName{Name: objName, Namespace: defaultNamespace}, obj)
			if err != nil {
				logt.Info("Unable to find secret or config in same namespace or default namespace", "secret or config", objName, "namespace", instanceNamespace, "default namespace", defaultNamespace)
				return err
			}
			return nil
		}
		logt.Error(err, "Unknown error getting config or secret", "name", objName, "namespace", instanceNamespace)
		return err
	}
	return nil
}

func getIBMCloudContext(instance *ibmcloudv1.Service, cm *v1.ConfigMap) ibmcloudv1.ResourceContext {
	if (ibmcloudv1.ResourceContext{}) == instance.Spec.Context {
		newContext := ibmcloudv1.ResourceContext{
			Org:             cm.Data["org"],
			Space:           cm.Data["space"],
			Region:          cm.Data["region"],
			ResourceGroupID: cm.Data["resourcegroupid"],
			User:            cm.Data["user"],
		}
		return newContext
	}
	if instance.Spec.Context.Org == "" {
		instance.Spec.Context.Org = cm.Data["org"]
	}
	if instance.Spec.Context.Space == "" {
		instance.Spec.Context.Space = cm.Data["space"]
	}
	if instance.Spec.Context.Region == "" {
		instance.Spec.Context.Region = cm.Data["region"]
	}
	if instance.Spec.Context.ResourceGroupID == "" {
		instance.Spec.Context.ResourceGroupID = cm.Data["resourcegroupid"]
	}
	if instance.Spec.Context.User == "" {
		instance.Spec.Context.User = cm.Data["user"]
	}
	return instance.Spec.Context
}

func getDefaultNamespace(logt logr.Logger, r client.Client) (string, bool) {
	cm := &v1.ConfigMap{}
	err := r.Get(context.Background(), types.NamespacedName{Namespace: config.Get().ControllerNamespace, Name: icoConfigMap}, cm)
	if err != nil {
		return "default", false
	}

	// There exists an ico-management configmap in the controller namespace
	logt.Info("Found ibmcloud-operator-config in own namespace, using that namespace for configuration", "own namespace", config.Get().ControllerNamespace, "management namespace", cm.Data["namespace"])
	return cm.Data["namespace"], true
}
