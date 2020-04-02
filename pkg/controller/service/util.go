/*
 * Copyright 2019 IBM Corporation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package service

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	bx "github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/api/account/accountv2"
	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/catalog"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	bxcontroller "github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/crn"
	bxendpoints "github.com/IBM-Cloud/bluemix-go/endpoints"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/session"
	ibmcloudv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	rcontext "github.com/ibm/cloud-operators/pkg/context"
	icv1 "github.com/ibm/cloud-operators/pkg/lib/ibmcloud/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const aliasPlan = "alias"

const seedInstall = "ibm-cloud-operator"
const seedSecret = "secret-ibm-cloud-operator"
const seedDefaults = "config-ibm-cloud-operator"
const seedTokens = "secret-ibm-cloud-operator-tokens"

var icoDefaultNamespace string

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
	Context          icv1.ResourceContext
}

func getBxConfig(r client.Client, instance *ibmcloudv1alpha1.Service) (bx.Config, error) {
	c := bx.Config{
		EndpointLocator: bxendpoints.NewEndpointLocator("us-south"), // TODO: hard wired to us-south!!
	}

	secretName := seedSecret
	secretNameSpace := instance.ObjectMeta.Namespace

	secret := &v1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: secretNameSpace}, secret)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			namespace := getDefaultNamespace(r)
			if namespace != "default" {
				secretName = secretNameSpace + "-" + secretName
			}
			err = r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
			if err != nil {
				logt.Info("Unable to get secret in namespace", namespace, err)
				return c, err
			}
		} else {
			logt.Info("Unable to get secret", "Error", err)
			return c, err
		}
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

func getDefaultNamespace(r client.Client) string {
	if icoDefaultNamespace == "default" {
		return icoDefaultNamespace
	}
	cm := &v1.ConfigMap{}
	cmName := seedInstall
	cmNamespace := os.Getenv("CONTROLLER_NAMESPACE")

	err := r.Get(context.Background(), types.NamespacedName{Namespace: cmNamespace, Name: cmName}, cm)
	if err != nil {
		icoDefaultNamespace = "default"
		return icoDefaultNamespace
	}

	// There exists an ico-management configmap in the controller namespace
	icoDefaultNamespace = cm.Data["namespace"]
	return icoDefaultNamespace
}

func getIBMCloudDefaultContext(r client.Client, instance *ibmcloudv1alpha1.Service) (icv1.ResourceContext, error) {
	// If the object already has the context set in its Status, then we don't read from the configmap
	if !reflect.DeepEqual(instance.Status.Context, icv1.ResourceContext{}) {
		return instance.Status.Context, nil
	}

	cm := &v1.ConfigMap{}
	cmName := seedDefaults
	cmNameSpace := instance.ObjectMeta.Namespace

	err := r.Get(context.Background(), types.NamespacedName{Namespace: cmNameSpace, Name: cmName}, cm)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			namespace := getDefaultNamespace(r)
			if namespace != "default" {
				cmName = cmNameSpace + "-" + cmName
			}
			err = r.Get(context.TODO(), types.NamespacedName{Name: cmName, Namespace: namespace}, cm)
			if err != nil {
				logt.Info("Failed to find ConfigMap in namespace (in Service)", namespace, err)
				return icv1.ResourceContext{}, err
			}
		} else {
			logt.Info("Failed to find ConfigMap in namespace (in Service)", cmNameSpace, err)
			return icv1.ResourceContext{}, err
		}

	}
	ibmCloudContext := getIBMCloudContext(instance, cm)
	return ibmCloudContext, nil
}

func getIBMCloudContext(instance *ibmcloudv1alpha1.Service, cm *v1.ConfigMap) icv1.ResourceContext {
	if (icv1.ResourceContext{}) == instance.Spec.Context {
		newContext := icv1.ResourceContext{
			Org:             cm.Data["org"],
			Space:           cm.Data["space"],
			Region:          cm.Data["region"],
			ResourceGroup:   cm.Data["resourcegroup"],
			ResourceGroupID: cm.Data["resourcegroupid"],
		}
		return newContext
	}
	if instance.Spec.Context.ResourceGroup == "" {
		instance.Spec.Context.ResourceGroup = cm.Data["resourcegroup"]
	}
	if instance.Spec.Context.ResourceGroupID == "" {
		instance.Spec.Context.ResourceGroupID = cm.Data["resourcegroupid"]
	}
	return instance.Spec.Context
}

func getIamToken(r client.Client, instance *ibmcloudv1alpha1.Service) (string, string, string, string, error) {
	secretName := seedTokens
	secretNameSpace := instance.ObjectMeta.Namespace

	secret := &v1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: secretNameSpace}, secret)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			namespace := getDefaultNamespace(r)
			if namespace != "default" {
				secretName = secretNameSpace + "-" + secretName
			}
			err = r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
			if err != nil {
				logt.Info("Unable to get secret in namespace", namespace, err)
				return "", "", "", "", err
			}
		} else {
			logt.Info("Unable to get secret", "Error", err)
			return "", "", "", "", err
		}
	}

	return string(secret.Data["access_token"]), string(secret.Data["refresh_token"]), string(secret.Data["uaa_refresh_token"]), string(secret.Data["uaa_token"]), nil
}

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

	return getIBMCloudInfoHelper(r, &bxConfig, ibmCloudContext, instance)
}

func getIBMCloudInfoHelper(r client.Client, config *bx.Config, nctx icv1.ResourceContext, instance *ibmcloudv1alpha1.Service) (*IBMCloudInfo, error) {
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
			Context: useCtx,
		}
		return &ibmCloudInfo, nil

	} else {
		IAMAccessToken, IAMRefreshToken, UAAAccessToken, UAARefreshToken, err := getIamToken(r, instance)
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

		// managementClient, err := management.New(sess)
		// if err != nil {
		// 	return nil, err
		// }
		// var resourceGroupID string
		// resGrpAPI := managementClient.ResourceGroup()
		// if useCtx.ResourceGroup == "" {

		// 	resourceGroupQuery := management.ResourceGroupQuery{
		// 		Default: true,
		// 	}

		// 	grpList, err := resGrpAPI.List(&resourceGroupQuery)

		// 	if err != nil {
		// 		return nil, err
		// 	}

		// 	resourceGroupID = grpList[0].ID

		// } else {
		// 	grp, err := resGrpAPI.FindByName(nil, useCtx.ResourceGroup)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// 	resourceGroupID = grp[0].ID
		// }

		// if useCtx.ResourceLocation == "" {
		// 	useCtx.ResourceLocation = useCtx.Region
		// }

		ibmCloudInfo := IBMCloudInfo{
			//BXClient:         bxclient,         // MccpServiceAPI
			ResourceClient: controllerClient, // IAMServiceAPI
			CatalogClient:  catalogClient,
			//Account:          myAccount, // *Account
			//Org:              myorg,     //*Organization
			//Space:            myspace,
			//Region:           regionList,
			ResourceGroupID:  useCtx.ResourceGroupID,
			ResourceLocation: useCtx.ResourceLocation,
			Session:          sess,
			ServiceClass:     servicename,
			ServiceClassType: servicetype,
			ServicePlan:      serviceplan,
			ServicePlanID:    servicePlanID,
			TargetCrn:        catalogCRN,
			Context:          useCtx,
		}
		return &ibmCloudInfo, nil
	}
}

// GetServiceInstanceFromObj from bx given context and resource
func GetServiceInstanceFromObj(scontext rcontext.Context, obj runtime.Object) (models.ServiceInstance, error) {
	service := obj.(*ibmcloudv1alpha1.Service)
	externalName := getExternalName(service)

	ibmCloudInfo, err := GetIBMCloudInfo(scontext.Client(), service)
	if err != nil {
		return models.ServiceInstance{}, err
	}

	// Service instance is not CF
	controllerClient := ibmCloudInfo.ResourceClient
	resServiceInstanceAPI := controllerClient.ResourceServiceInstance()
	serviceInstanceQuery := bxcontroller.ServiceInstanceQuery{
		ResourceGroupID: ibmCloudInfo.ResourceGroupID,
		ServicePlanID:   ibmCloudInfo.ServicePlanID,
		Name:            externalName,
	}

	instances, err := resServiceInstanceAPI.ListInstances(serviceInstanceQuery)
	return GetServiceInstance(instances, service.Status.InstanceID)
}

// GetServiceInstanceFromObjCF from bx given context and resource in a CF context
func GetServiceInstanceFromObjCF(scontext rcontext.Context, obj runtime.Object) (*mccpv2.ServiceInstance, error) {
	service := obj.(*ibmcloudv1alpha1.Service)
	externalName := getExternalName(service)

	ibmCloudInfo, err := GetIBMCloudInfo(scontext.Client(), service)
	if err != nil {
		return &mccpv2.ServiceInstance{}, err
	}

	serviceInstanceAPI := ibmCloudInfo.BXClient.ServiceInstances()
	return serviceInstanceAPI.FindByName(externalName)
}
