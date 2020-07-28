/*
 * Copyright 2020 IBM Corporation
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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	iam "github.com/IBM-Cloud/bluemix-go/api/iam/iamv1"
	bxcontroller "github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/utils"
	"github.com/go-logr/logr"
	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
	"github.com/ibm/cloud-operators/internal/config"
	"github.com/ibm/cloud-operators/internal/ibmcloud"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	syncPeriod = config.GetSyncPeriod()
)

const (
	bindingFinalizer = "binding.ibmcloud.ibm.com"
	inProgress       = "IN PROGRESS"
	notFound         = "Not Found"
	idkey            = "ibmcloud.ibm.com/keyId"
)

// BindingReconciler reconciles a Binding object
type BindingReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func (r *BindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ibmcloudv1beta1.Binding{}).
		Complete(r)
}

// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=bindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=bindings/status,verbs=get;update;patch

func (r *BindingReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logt := r.Log.WithValues("binding", request.NamespacedName)

	// Fetch the Binding instance
	instance := &ibmcloudv1beta1.Binding{}
	err := r.Get(context.Background(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Set the Status field for the first time
	if reflect.DeepEqual(instance.Status, ibmcloudv1beta1.BindingStatus{}) {
		instance.Status.State = "Pending"
		instance.Status.Message = "Processing Resource"
		if err := r.Status().Update(context.Background(), instance); err != nil {
			logt.Info("Binding could not update Status", instance.Name, err.Error())
			return ctrl.Result{}, nil
		}
	}

	// First, make sure that there is a current service InstanceID
	// Obtain the serviceInstance corresponding to this Binding object
	serviceInstance, err := r.getServiceInstance(instance)
	if err != nil {
		logt.Info("Binding could not read service", instance.Spec.ServiceName, err.Error())
		// We could not find a parent service. However, if this instance is marked for deletion, delete it anyway
		if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
			// In this case it is enough to simply remove the finalizer:
			// the credentials do not exist on the cloud, since the service cannot be found.
			// Also by removing the Binding instance, any correponding secret will also be deleted by Kubernetes.
			instance.ObjectMeta.Finalizers = deleteFinalizer(instance)
			if err := r.Update(context.Background(), instance); err != nil {
				logt.Info("Error removing finalizers", "in deletion", err.Error())
				// No further action required, object was modified, another reconcile will finish the job.
			}
			return ctrl.Result{}, nil
		}

		// In case there previously existed a service instance and it's now gone, reset the state of the resource
		if instance.Status.KeyInstanceID != "" {
			return r.resetResource(instance)
		}

		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil //Requeue fast
	}

	// Set an owner reference if service and binding are in the same namespace
	if serviceInstance.Namespace == instance.Namespace {
		if err := controllerutil.SetControllerReference(serviceInstance, instance, r.Scheme); err != nil {
			logt.Info("Binding could not update constroller reference", instance.Name, err.Error())
			return ctrl.Result{}, err
		}

		if err := r.Update(context.Background(), instance); err != nil {
			logt.Info("Error setting controller reference", instance.Name, err.Error())
			return ctrl.Result{}, nil
		}
	}

	// If the service has not been initialized fully yet, then requeue
	if serviceInstance.Status.InstanceID == "" || serviceInstance.Status.InstanceID == inProgress {
		// The parent service has not been initialized fully yet
		logt.Info("Parent service", "not yet initialized", instance.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil //Requeue fast
	}

	ibmCloudInfo, err := ibmcloud.GetInfo(logt, r.Client, serviceInstance)
	if err != nil {
		logt.Info("Unable to get", "ibmcloudInfo", instance.Name)
		if errors.IsNotFound(err) && containsFinalizer(instance) &&
			!instance.ObjectMeta.DeletionTimestamp.IsZero() {
			logt.Info("Cannot get IBMCloud related secrets and configmaps, just remove finalizers", "in deletion", err.Error())
			instance.ObjectMeta.Finalizers = deleteFinalizer(instance)
			if err := r.Update(context.Background(), instance); err != nil {
				logt.Info("Error removing finalizers", "in deletion", err.Error())
			}
			return ctrl.Result{}, nil
		}
		return r.updateStatusError(instance, "Pending", err)
	}

	// Delete if necessary
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Instance is not being deleted, add the finalizer if not present
		if !containsFinalizer(instance) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, bindingFinalizer)
			if err := r.Update(context.Background(), instance); err != nil {
				logt.Info("Error adding finalizer", instance.Name, err.Error())
				return ctrl.Result{}, nil
			}
		}
	} else {
		// The object is being deleted
		if containsFinalizer(instance) {
			logt.Info("Resource marked for deletion", "in deletion", instance.Name)
			err := r.deleteCredentials(instance, ibmCloudInfo)
			if err != nil {
				logt.Info("Error deleting credentials", "in deletion", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = deleteFinalizer(instance)
			if err := r.Update(context.Background(), instance); err != nil {
				logt.Info("Error removing finalizers", "in deletion", err.Error())
			}
			return ctrl.Result{}, nil
		}
	}

	if instance.Status.InstanceID == "" { // The service Instance ID has not been initialized yet
		instance.Status.InstanceID = serviceInstance.Status.InstanceID

	} else { // The service Instance ID has been set, verify that it is current
		if instance.Status.InstanceID != serviceInstance.Status.InstanceID {
			logt.Info("ServiceKey", "Service parent", "has changed")
			err := r.deleteCredentials(instance, ibmCloudInfo)
			if err != nil {
				logt.Info("Error deleting credentials", "in deletion", err.Error())
				return r.updateStatusError(instance, "Failed", err)
			}
			instance.Status.InstanceID = serviceInstance.Status.InstanceID
		}
	}

	// Now instance.Status.IntanceID has been set properly
	if instance.Status.KeyInstanceID == "" { // The KeyInstanceID has not been set, need to create the key
		instance.Status.KeyInstanceID = inProgress
		if err := r.Status().Update(context.Background(), instance); err != nil {
			logt.Info("Error updating KeyInstanceID to be in progress", "Error", err.Error())
			return ctrl.Result{}, nil
		}

		var keyInstanceID string
		var keyContents map[string]interface{}

		if instance.Spec.Alias != "" {
			keyInstanceID, keyContents, err = getAliasCredentials(logt, instance, ibmCloudInfo)
			if err != nil {
				logt.Info("Error retrieving alias credentials", instance.Name, err.Error())
				return r.updateStatusError(instance, "Pending", err)
			}
		} else {
			keyInstanceID, keyContents, err = r.createCredentials(ctx, instance, ibmCloudInfo)
			if err != nil {
				logt.Info("Error creating credentials", instance.Name, err.Error())
				if strings.Contains(err.Error(), "still in progress") {
					return r.updateStatusError(instance, "Pending", err)
				}
				return r.updateStatusError(instance, "Failed", err)
			}
		}
		instance.Status.KeyInstanceID = keyInstanceID

		// Now create the secret
		err = r.createSecret(instance, keyContents)

		if err != nil {
			logt.Info("Error creating secret", instance.Name, err.Error())
			return r.updateStatusError(instance, "Failed", err)
		}

		return r.updateStatusOnline(instance, serviceInstance, ibmCloudInfo)
	}

	// The KeyInstanceID has been set (or is still inProgress), verify that the key and secret still exist
	logt.Info("ServiceInstance Key", "should already exist, verifying", instance.ObjectMeta.Name)
	var keyInstanceID string
	var keyContents map[string]interface{}
	if instance.Spec.Alias != "" {
		_, keyContents, err = getAliasCredentials(logt, instance, ibmCloudInfo)
		if err != nil && strings.Contains(err.Error(), notFound) {
			return r.resetResource(instance)
		} else if err != nil {
			return r.updateStatusError(instance, "Failed", err)
		}
	} else {
		_, keyContents, err = getCredentials(logt, instance, ibmCloudInfo)
		if err != nil && strings.Contains(err.Error(), notFound) {
			logt.Info("ServiceInstance Key does not exist", "Recreating", instance.ObjectMeta.Name)
			keyInstanceID, keyContents, err = r.createCredentials(ctx, instance, ibmCloudInfo)
			if err != nil {
				return r.updateStatusError(instance, "Failed", err)
			}
			instance.Status.KeyInstanceID = keyInstanceID
		}
	}
	secret, err := getSecret(r, instance)
	if err != nil {
		logt.Info("Secret does not exist", "Recreating", getSecretName(instance))
		err = r.createSecret(instance, keyContents)
		if err != nil {
			logt.Info("Error creating secret", instance.Name, err.Error())
			return r.updateStatusError(instance, "Failed", err)
		}
		return r.updateStatusOnline(instance, serviceInstance, ibmCloudInfo)
	}

	// The secret exists, make sure it has the right content
	changed, err := keyContentsChanged(keyContents, secret)
	if err != nil {
		logt.Info("Error checking if key contents have changed", instance.Name, err.Error())
		return r.updateStatusError(instance, "Failed", err)
	}
	if instance.Status.KeyInstanceID != secret.Annotations["service-key-id"] || changed { // Warning: the deep comparison may not be needed, the key is probably enough
		err := r.deleteSecret(instance)
		if err != nil {
			logt.Info("Error deleting secret before recreating", instance.Name, err.Error())
			return r.updateStatusError(instance, "Failed", err)
		}
		err = r.createSecret(instance, keyContents)
		if err != nil {
			logt.Info("Error re-creating secret", instance.Name, err.Error())
			return r.updateStatusError(instance, "Failed", err)
		}
		return r.updateStatusOnline(instance, serviceInstance, ibmCloudInfo)
	}
	return r.updateStatusOnline(instance, serviceInstance, ibmCloudInfo)
}

func (r *BindingReconciler) getServiceInstance(instance *ibmcloudv1beta1.Binding) (*ibmcloudv1beta1.Service, error) {
	serviceNameSpace := instance.ObjectMeta.Namespace
	if instance.Spec.ServiceNamespace != "" {
		serviceNameSpace = instance.Spec.ServiceNamespace
	}
	serviceInstance := &ibmcloudv1beta1.Service{}
	err := r.Get(context.Background(), types.NamespacedName{Name: instance.Spec.ServiceName, Namespace: serviceNameSpace}, serviceInstance)
	if err != nil {
		return &ibmcloudv1beta1.Service{}, err
	}
	return serviceInstance, nil
}

func (r *BindingReconciler) resetResource(instance *ibmcloudv1beta1.Binding) (ctrl.Result, error) {
	instance.Status.State = "Pending"
	instance.Status.Message = "Processing Resource"
	instance.Status.InstanceID = ""
	instance.Status.KeyInstanceID = ""

	// If a secret exists that corresponds to this Binding, then delete it
	err := r.deleteSecret(instance)
	if err != nil {
		r.Log.Info("Unable to delete", "secret", instance.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: syncPeriod}, nil
	}

	instance.Status.SecretName = ""
	if err := r.Status().Update(context.Background(), instance); err != nil {
		r.Log.Info("Binding could not reset Status", instance.Name, err.Error())
		return ctrl.Result{}, nil
	}
	return ctrl.Result{Requeue: true, RequeueAfter: syncPeriod}, nil
}

func (r *BindingReconciler) updateStatusError(instance *ibmcloudv1beta1.Binding, state string, err error) (ctrl.Result, error) {
	message := err.Error()
	r.Log.Info(message)

	if strings.Contains(message, "no such host") {
		r.Log.Info("No such host", instance.Name, message)
		// This means that the IBM Cloud server is under too much pressure, we need to backup
		return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 5}, nil

	}

	if instance.Status.State != state {
		instance.Status.State = state
		instance.Status.Message = message
		if err := r.Status().Update(context.Background(), instance); err != nil {
			r.Log.Info("Error updating status", state, err.Error())
			return ctrl.Result{}, nil
		}
	}
	return ctrl.Result{Requeue: true, RequeueAfter: syncPeriod}, nil
}

// deleteCredentials also deletes the corresponding secret
func (r *BindingReconciler) deleteCredentials(instance *ibmcloudv1beta1.Binding, ibmCloudInfo *ibmcloud.Info) error {
	r.Log.WithValues("User", ibmCloudInfo.Context.User).Info("Deleting", "credentials", instance.ObjectMeta.Name)

	if instance.Spec.Alias == "" { // Delete only if it not alias
		if ibmCloudInfo.ServiceClassType == "CF" { // service type is CF
			serviceKeys := ibmCloudInfo.BXClient.ServiceKeys()
			err := serviceKeys.Delete(instance.Status.KeyInstanceID)
			if err != nil && !strings.Contains(err.Error(), "410") && !strings.Contains(err.Error(), "404") { // we do not propagate an error if the service or credential no longer exist
				return err
			}

		} else { // service type is not CF
			resServiceKeyAPI := ibmCloudInfo.ResourceClient.ResourceServiceKey()
			err := resServiceKeyAPI.DeleteKey(instance.Status.KeyInstanceID)
			if err != nil && !strings.Contains(err.Error(), "410") && !strings.Contains(err.Error(), "404") { // we do not propagate an error if the service or credential no longer exist
				return err
			}
		}
	}
	return r.deleteSecret(instance)
}

func getAliasCredentials(logt logr.Logger, instance *ibmcloudv1beta1.Binding, ibmCloudInfo *ibmcloud.Info) (string, map[string]interface{}, error) {
	logt.Info("Getting", " alias credentials", instance.ObjectMeta.Name)
	name := instance.Spec.Alias

	if ibmCloudInfo.ServiceClassType == "CF" { // service type is CF
		return getCFCredentials(logt, instance, ibmCloudInfo, name)
	}

	// service type is not CF
	keyid, annotationFound := instance.ObjectMeta.GetAnnotations()[idkey]
	if !annotationFound {
		return "", nil, fmt.Errorf("Alias credential does not have %s annotation", idkey)
	}
	resServiceKeyAPI := ibmCloudInfo.ResourceClient.ResourceServiceKey()
	key, err := resServiceKeyAPI.GetKey(keyid)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", nil, fmt.Errorf(notFound)
		}
		return "", nil, err
	}

	if key.Name != name { // alias name and keyid annotations are inconsistent
		return "", nil, fmt.Errorf("Alias credential name and keyid do not match")
	}

	_, contentsContainRedacted := key.Credentials["REDACTED"]
	if contentsContainRedacted {
		return "", nil, fmt.Errorf(notFound)
	}

	return key.ID, key.Credentials, nil
}

func (r *BindingReconciler) createCredentials(ctx context.Context, instance *ibmcloudv1beta1.Binding, ibmCloudInfo *ibmcloud.Info) (string, map[string]interface{}, error) {
	var keyContents map[string]interface{}
	var keyInstanceID string
	r.Log.WithValues("User", ibmCloudInfo.Context.User).Info("Creating", "credentials", instance.ObjectMeta.Name)
	parameters, err := r.getParams(ctx, instance)
	if err != nil {
		r.Log.Error(err, "Instance ", instance.ObjectMeta.Name, " has problems with its parameters")
		return "", nil, err
	}
	if ibmCloudInfo.ServiceClassType == "CF" { // service type is CF
		serviceKeys := ibmCloudInfo.BXClient.ServiceKeys()
		key, err := serviceKeys.Create(instance.Status.InstanceID, instance.ObjectMeta.Name, parameters)
		if err != nil {
			return "", nil, err
		}
		keyInstanceID = key.Metadata.GUID
		keyContents = key.Entity.Credentials

	} else { // service type is not CF
		resServiceInstanceAPI := ibmCloudInfo.ResourceClient.ResourceServiceInstance()
		serviceInstanceModel, err := resServiceInstanceAPI.GetInstance(instance.Status.InstanceID)
		if err != nil {
			return "", nil, err
		}
		resCatalogAPI := ibmCloudInfo.CatalogClient.ResourceCatalog()
		serviceresp, err := resCatalogAPI.Get(serviceInstanceModel.ServiceID, true)
		if err != nil {
			return "", nil, err
		}

		iamClient, err := iam.New(ibmCloudInfo.Session)
		if err != nil {
			return "", nil, err
		}

		serviceRolesAPI := iamClient.ServiceRoles()
		var roles []models.PolicyRole

		if serviceresp.Name == "" {
			roles, err = serviceRolesAPI.ListSystemDefinedRoles()
		} else {
			roles, err = serviceRolesAPI.ListServiceRoles(serviceresp.Name)
		}
		if err != nil {
			return "", nil, err
		}

		var roleID crn.CRN

		if instance.Spec.Role != "" {
			roleMatch, err := utils.FindRoleByName(roles, instance.Spec.Role)
			if err != nil {
				return "", nil, err
			}
			roleID = roleMatch.ID
		} else {
			if len(roles) == 0 {
				return "", nil, fmt.Errorf("The service has no roles defined for its bindings")
			}
			managerRole, err := getManagerRole(roles)
			if err != nil {
				// No Manager role found
				roleID = roles[0].ID
			} else {
				roleID = managerRole.ID
			}
		}

		parameters["role_crn"] = roleID

		resServiceKeyAPI := ibmCloudInfo.ResourceClient.ResourceServiceKey()
		params := bxcontroller.CreateServiceKeyRequest{
			Name:       instance.ObjectMeta.Name,
			SourceCRN:  serviceInstanceModel.Crn,
			Parameters: parameters,
		}

		keyresp, err := resServiceKeyAPI.CreateKey(params)
		if err != nil {
			return "", nil, err
		}

		keyInstanceID = keyresp.ID
		keyContents = keyresp.Credentials
	}
	return keyInstanceID, keyContents, nil
}

func (r *BindingReconciler) createSecret(instance *ibmcloudv1beta1.Binding, keyContents map[string]interface{}) error {
	r.Log.Info("Creating ", "secret", instance.ObjectMeta.Name)
	datamap, err := processKey(keyContents)
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: getSecretName(instance),
			Annotations: map[string]string{
				"service-instance-id": instance.Status.InstanceID,
				"service-key-id":      instance.Status.KeyInstanceID,
				"bindingFromName":     instance.Spec.ServiceName,
			},
			Namespace: instance.Namespace,
		},
		Data: datamap,
	}
	if err := controllerutil.SetControllerReference(instance, secret, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(context.Background(), secret); err != nil {
		return err
	}
	return nil
}

func (r *BindingReconciler) updateStatusOnline(instance *ibmcloudv1beta1.Binding, serviceInstance *ibmcloudv1beta1.Service, ibmCloudInfo *ibmcloud.Info) (ctrl.Result, error) {
	instance.Status.State = "Online"
	instance.Status.Message = "Online"
	instance.Status.SecretName = getSecretName(instance)
	err := r.Status().Update(context.Background(), instance)
	if err != nil {
		r.Log.Info("Failed to update online status, will delete external resource ", instance.ObjectMeta.Name, err.Error())
		err = r.deleteCredentials(instance, ibmCloudInfo)
		if err != nil {
			r.Log.Info("Failed to delete external resource, operator state and external resource might be in an inconsistent state", instance.ObjectMeta.Name, err.Error())
		}
	}

	return ctrl.Result{Requeue: true, RequeueAfter: syncPeriod}, nil
}

func getCredentials(logt logr.Logger, instance *ibmcloudv1beta1.Binding, ibmCloudInfo *ibmcloud.Info) (string, map[string]interface{}, error) {
	logt.Info("Getting", "credentials", instance.ObjectMeta.Name)

	if ibmCloudInfo.ServiceClassType == "CF" { // service type is CF
		return getCFCredentials(logt, instance, ibmCloudInfo, instance.Name)
	}

	// service type is not CF
	resServiceKeyAPI := ibmCloudInfo.ResourceClient.ResourceServiceKey()
	if instance.Status.KeyInstanceID != "" && instance.Status.KeyInstanceID != inProgress { // There is a valid KeyInstanceID
		keyresp, err := resServiceKeyAPI.GetKey(instance.Status.KeyInstanceID)
		if err != nil && strings.Contains(err.Error(), "404") {
			return "", nil, fmt.Errorf(notFound)
		} else if err != nil {
			return "", nil, err
		}
		_, contentsContainRedacted := keyresp.Credentials["REDACTED"]
		if contentsContainRedacted {
			return "", nil, fmt.Errorf(notFound)
		}
		return keyresp.ID, keyresp.Credentials, nil
	}

	return "", nil, fmt.Errorf(notFound)
}

func getSecretName(instance *ibmcloudv1beta1.Binding) string {
	secretName := instance.ObjectMeta.Name
	if instance.Spec.SecretName != "" {
		secretName = instance.Spec.SecretName
	}
	return secretName
}

func keyContentsChanged(keyContents map[string]interface{}, secret *corev1.Secret) (bool, error) {
	newContent, err := processKey(keyContents)
	if err != nil {
		return false, err
	}
	return !reflect.DeepEqual(newContent, secret.Data), nil
}

func (r *BindingReconciler) deleteSecret(instance *ibmcloudv1beta1.Binding) error {
	r.Log.Info("Deleting ", "secret", instance.Status.SecretName)
	secret, err := getSecret(r, instance)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil //secret does not exist, nothing to delete
		}
		return err
	}
	if err = r.Delete(context.Background(), secret); err != nil {
		return err
	}
	return nil
}

func getCFCredentials(logt logr.Logger, instance *ibmcloudv1beta1.Binding, ibmCloudInfo *ibmcloud.Info, name string) (string, map[string]interface{}, error) {
	logt.Info("Getting", "CF credentials", name)
	serviceKeys := ibmCloudInfo.BXClient.ServiceKeys()

	myRetrievedKeys, err := serviceKeys.FindByName(instance.Status.InstanceID, name)
	if err != nil {
		if strings.Contains(err.Error(), "doesn't exist") {
			return "", nil, fmt.Errorf(notFound)
		}
		return "", nil, err
	}
	_, contentsContainRedacted := myRetrievedKeys.Credentials["REDACTED"]
	if contentsContainRedacted {
		return "", nil, fmt.Errorf(notFound)
	}

	return myRetrievedKeys.GUID, myRetrievedKeys.Credentials, nil
}

func (r *BindingReconciler) getParams(ctx context.Context, instance *ibmcloudv1beta1.Binding) (map[string]interface{}, error) {
	params := make(map[string]interface{})

	for _, p := range instance.Spec.Parameters {
		val, err := r.paramToJSON(ctx, p, instance.Namespace)
		if err != nil {
			return params, err
		}
		params[p.Name] = val
	}
	return params, nil
}

func getManagerRole(roles []models.PolicyRole) (models.PolicyRole, error) {
	for _, role := range roles {
		if role.DisplayName == "Manager" {
			return role, nil
		}
	}
	return models.PolicyRole{}, fmt.Errorf("No Manager role found")
}

func processKey(keyContents map[string]interface{}) (map[string][]byte, error) {
	ret := make(map[string][]byte)
	for k, v := range keyContents {
		keyString := strings.Replace(k, " ", "_", -1)
		// need to re-marshal as json might have complex types, which need to be flattened in strings
		jString, err := json.Marshal(v)
		if err != nil {
			return ret, err
		}
		// need to remove quotes from flattened objects
		strVal := strings.TrimPrefix(string(jString), "\"")
		strVal = strings.TrimSuffix(strVal, "\"")
		ret[keyString] = []byte(strVal)
	}
	return ret, nil
}

// paramToJSON converts variable value to JSON value
func (r *BindingReconciler) paramToJSON(ctx context.Context, p ibmcloudv1beta1.Param, namespace string) (interface{}, error) {
	if p.Value != nil && p.ValueFrom != nil {
		return nil, fmt.Errorf("Value and ValueFrom properties are mutually exclusive (for %s variable)", p.Name)
	}

	valueFrom := p.ValueFrom
	if valueFrom != nil {
		return r.paramValueToJSON(ctx, *valueFrom, namespace)
	}

	if p.Value == nil {
		return nil, nil
	}
	return paramToJSONFromRaw(p.Value)
}

// paramValueToJSON takes a ParamSource and resolves its value
func (r *BindingReconciler) paramValueToJSON(ctx context.Context, valueFrom ibmcloudv1beta1.ParamSource, namespace string) (interface{}, error) {
	if valueFrom.SecretKeyRef != nil {
		data, err := r.getKubeSecretValue(ctx, valueFrom.SecretKeyRef.Name, valueFrom.SecretKeyRef.Key, true, namespace)
		if err != nil {
			// Recoverable
			return nil, fmt.Errorf("Missing secret %s", valueFrom.SecretKeyRef.Name)
		}
		return paramToJSONFromString(string(data))
	} else if valueFrom.ConfigMapKeyRef != nil {
		data, err := r.getConfigMapValue(ctx, valueFrom.ConfigMapKeyRef.Name, valueFrom.ConfigMapKeyRef.Key, true, namespace)
		if err != nil {
			// Recoverable
			return nil, fmt.Errorf("Missing configmap %s", valueFrom.ConfigMapKeyRef.Name)
		}
		return paramToJSONFromString(data)
	}
	return nil, fmt.Errorf("Missing secretKeyRef or configMapKeyRef")
}

func paramToJSONFromRaw(content *ibmcloudv1beta1.ParamValue) (interface{}, error) {
	var data interface{}

	if err := json.Unmarshal(content.RawMessage, &data); err != nil {
		return nil, err
	}

	return data, nil
}

func paramToJSONFromString(content string) (interface{}, error) {
	var data interface{}

	dc := json.NewDecoder(strings.NewReader(content))
	dc.UseNumber()
	if err := dc.Decode(&data); err != nil {
		// Just return the content in order to support unquoted string value
		// In the future we might want to implement some heuristic to detect the user intention
		// Maybe if content start with '{' or '[' then the intent might be to specify a JSON and it is invalid

		return content, nil
	}
	if dc.More() {
		// Not a valid JSON. Interpret as unquoted string value
		return content, nil
	}
	return data, nil
}
