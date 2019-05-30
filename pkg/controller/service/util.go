package service

import (
	"context"
	"fmt"

	bx "github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/api/account/accountv2"
	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/catalog"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/management"
	"github.com/IBM-Cloud/bluemix-go/crn"
	bxendpoints "github.com/IBM-Cloud/bluemix-go/endpoints"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/session"
	ibmcloudv1alpha1 "github.com/ibm/ibm-cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	icv1 "github.com/ibm/ibm-cloud-operators/pkg/lib/ibmcloud/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IBMCloudInfo kept all the needed client API resource and instance Info
type IBMCloudInfo struct {
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
}

func getBxConfig(r client.Client, instance *ibmcloudv1alpha1.Service) (bx.Config, error) {
	// TODO - Fix this, need to go to namespace ibmcloud-operators if the base namespace doesn't have seed-secrets
	secretName := "seed-secret"
	secretNameSpace := "ibmcloud-operators"
	if instance.ObjectMeta.Namespace != "" {
		secretNameSpace = instance.ObjectMeta.Namespace
	}

	c := bx.Config{
		EndpointLocator: bxendpoints.NewEndpointLocator("us-south"), // TODOMV: hard wired to us-south!!
	}

	secret := &v1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: secretNameSpace}, secret)
	if err != nil {
		logt.Info("Unable to get secret", "Error", err)
		return c, err
	}

	APIKey := string(secret.Data["api-key"])

	regionb, ok := secret.Data["region"]
	if !ok {
		logt.Info("set default region to us-south")
		regionb = []byte("us-south")
	}
	region := string(regionb)
	c.Region = region
	c.BluemixAPIKey = APIKey

	return c, nil
}

func getIBMCloudDefaultContext(r client.Client, instance *ibmcloudv1alpha1.Service) (icv1.ResourceContext, error) {
	cm := &v1.ConfigMap{}
	cmName := "seed-defaults"
	cmNameSpace := "ibmcloud-operators"
	if instance.ObjectMeta.Namespace != "" {
		cmNameSpace = instance.ObjectMeta.Namespace
	}

	err := r.Get(context.TODO(), types.NamespacedName{Namespace: cmNameSpace, Name: cmName}, cm)
	if err != nil {
		logt.Info("Failed to find ConfigMap in namespace (in Service)", cmNameSpace, err)
		return icv1.ResourceContext{}, err
	}
	ibmCloudContext := getIBMCloudContext(instance, cm)
	return ibmCloudContext, nil
}

func getIBMCloudContext(instance *ibmcloudv1alpha1.Service, cm *v1.ConfigMap) icv1.ResourceContext {
	if (icv1.ResourceContext{}) == instance.Spec.Context {
		resourceGroup := cm.Data["resourcegroup"]
		if resourceGroup == "" {
			resourceGroup = "default"
		}
		newContext := icv1.ResourceContext{
			Org:           cm.Data["org"],
			Space:         cm.Data["space"],
			Region:        cm.Data["region"],
			ResourceGroup: resourceGroup,
		}

		return newContext
	} else if instance.Spec.Context.ResourceGroup == "" {
		instance.Spec.Context.ResourceGroup = "default"
		return instance.Spec.Context
	}

	return instance.Spec.Context
}

// func getIamToken(r client.Client, instance *ibmcloudv1alpha1.Service) (string, error) {
// 	secretName := "seed-secret-tokens"
// 	secretNameSpace := "ibmcloud-operators"
// 	if instance.ObjectMeta.Namespace != "" {
// 		secretNameSpace = instance.ObjectMeta.Namespace
// 	}

// 	secret := &v1.Secret{}
// 	err := r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: secretNameSpace}, secret)
// 	if err != nil {
// 		logt.Info("Unable to get secret-secret-tokens", "Error", err)
// 		return "", err
// 	}

// 	return string(secret.Data["access_token"]), nil
// }

// GetIBMCloudInfo initializes sessions and sets up a struct to faciliate making calls to bx
func GetIBMCloudInfo(r client.Client, instance *ibmcloudv1alpha1.Service) (*IBMCloudInfo, error) {
	// Get Bx Config
	bxConfig, err := getBxConfig(r, instance)
	if err != nil {
		return nil, err
	}

	ibmCloudContext, err := getIBMCloudDefaultContext(r, instance)
	if err != nil {
		return nil, err
	}

	// TODO - REMOVE IF NOT NEEDED!!!!
	// token, err := getIamToken(r, instance)
	// if err != nil {
	// 	return nil, err
	// }
	return getIBMCloudInfoHelper(&bxConfig, ibmCloudContext, instance)
}

func getIBMCloudInfoHelper(config *bx.Config, nctx icv1.ResourceContext, instance *ibmcloudv1alpha1.Service) (*IBMCloudInfo, error) {
	servicename := instance.Spec.ServiceClass
	servicetype := instance.Spec.ServiceClassType
	serviceplan := instance.Spec.Plan

	useCtx := nctx
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

		// accClient, err := accountv2.New(sess)
		// if err != nil {
		// 	return nil, err
		// }

		// logt.Info("here4")

		// accountAPI := accClient.Accounts()
		// myAccount, err := accountAPI.FindByOrg(myorg.GUID, useCtx.Region)
		// if err != nil {
		// 	return nil, err
		// }

		serviceOfferingAPI := bxclient.ServiceOfferings()
		myserviceOff, err := serviceOfferingAPI.FindByLabel(servicename)
		if err != nil {
			return nil, err
		}

		servicePlanAPI := bxclient.ServicePlans()
		servicePlan, err := servicePlanAPI.FindPlanInServiceOffering(myserviceOff.GUID, serviceplan)
		if err != nil {
			return nil, err
		}

		ibmCloudInfo := IBMCloudInfo{
			BXClient: bxclient, // MccpServiceAPI
			//ResourceClient:   controllerClient, // IAMServiceAPI
			//CatalogClient:    catalogClient,
			//Account: myAccount, // *Account
			Org:    myorg, //*Organization
			Space:  myspace,
			Region: regionList,
			//ResourceGroupID:  resourceGroupID,
			ResourceLocation: useCtx.ResourceLocation,
			Session:          sess,
			ServiceClass:     servicename,
			ServiceClassType: servicetype,
			BxPlan:           servicePlan,
			//ServicePlanID:    servicePlanID,
			//TargetCrn:        supportedDeployments[0].CatalogCRN,
		}
		return &ibmCloudInfo, nil

	} else {

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

		servicePlanID, err := resCatalogAPI.GetServicePlanID(service[0], serviceplan)
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

		managementClient, err := management.New(sess)
		if err != nil {
			return nil, err
		}
		var resourceGroupID string
		resGrpAPI := managementClient.ResourceGroup()
		if useCtx.ResourceGroup == "" {

			resourceGroupQuery := management.ResourceGroupQuery{
				Default: true,
			}

			grpList, err := resGrpAPI.List(&resourceGroupQuery)
			for _, gp := range grpList {
				logt.Info("Deployment", "name", gp.Name)
			}
			if err != nil {
				return nil, err
			}

			resourceGroupID = grpList[0].ID

		} else {
			grp, err := resGrpAPI.FindByName(nil, useCtx.ResourceGroup)
			if err != nil {
				return nil, err
			}
			resourceGroupID = grp[0].ID
		}

		if useCtx.ResourceLocation == "" {
			useCtx.ResourceLocation = useCtx.Region
		}

		ibmCloudInfo := IBMCloudInfo{
			//BXClient:         bxclient,         // MccpServiceAPI
			ResourceClient: controllerClient, // IAMServiceAPI
			CatalogClient:  catalogClient,
			//Account:          myAccount, // *Account
			//Org:              myorg,     //*Organization
			//Space:            myspace,
			//Region:           regionList,
			ResourceGroupID:  resourceGroupID,
			ResourceLocation: useCtx.ResourceLocation,
			Session:          sess,
			ServiceClass:     servicename,
			ServiceClassType: servicetype,
			ServicePlan:      serviceplan,
			ServicePlanID:    servicePlanID,
			TargetCrn:        supportedDeployments[0].CatalogCRN,
		}
		return &ibmCloudInfo, nil
	}
}
