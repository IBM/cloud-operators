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

	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/go-logr/logr"
	ibmcloudv1 "github.com/ibm/cloud-operators/api/v1"
	"github.com/ibm/cloud-operators/internal/config"
	"github.com/ibm/cloud-operators/internal/ibmcloud"
	"github.com/ibm/cloud-operators/internal/ibmcloud/cfservice"
	"github.com/ibm/cloud-operators/internal/ibmcloud/iam"
	"github.com/ibm/cloud-operators/internal/ibmcloud/resource"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const (
	bindingFinalizer = "binding.ibmcloud.ibm.com"
	inProgress       = "IN PROGRESS"
	notFound         = "Not Found"
	idkey            = "ibmcloud.ibm.com/keyId"
	requeueFast      = 10 * time.Second
)

const (
	// bindingStatePending indicates a resource is in a pending state
	bindingStatePending string = "Pending"
	// bindingStateFailed indicates a resource is in a failed state
	bindingStateFailed string = "Failed"
	// bindingStateOnline indicates a resource has been fully synchronized and online
	bindingStateOnline string = "Online"
)

// BindingReconciler reconciles a Binding object
type BindingReconciler struct {
	client.Client
	Log    logr.Logger
	scheme *runtime.Scheme

	CreateResourceServiceKey   resource.KeyCreator
	CreateCFServiceKey         cfservice.KeyCreator
	DeleteResourceServiceKey   resource.KeyDeleter
	DeleteCFServiceKey         cfservice.KeyDeleter
	GetIBMCloudInfo            IBMCloudInfoGetter
	GetResourceServiceKey      resource.KeyGetter
	GetServiceInstanceCRN      resource.ServiceInstanceCRNGetter
	GetCFServiceKeyCredentials cfservice.KeyGetter
	GetServiceName             resource.ServiceNameGetter
	GetServiceRoleCRN          iam.ServiceRolesGetter
	SetControllerReference     OwnerReferenceSetter
	SetOwnerReference          OwnerReferenceSetter
}

type OwnerReferenceSetter func(owner, owned metav1.Object, scheme *runtime.Scheme) error

type IBMCloudInfoGetter func(logt logr.Logger, r client.Client, instance *ibmcloudv1.Service) (*ibmcloud.Info, error)

func (r *BindingReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&ibmcloudv1.Binding{}).
		Complete(r)
}

func (r *BindingReconciler) Scheme() *runtime.Scheme {
	return r.scheme
}

// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=bindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=bindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=bindings/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=services/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads the state of the cluster for a Binding object and makes changes based on the state read
// and what is in the Binding.Spec.
// Automatically generates RBAC rules to allow the Controller to read and write Deployments.
func (r *BindingReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logt := r.Log.WithValues("binding", request.NamespacedName)

	// Fetch the Binding instance
	instance := &ibmcloudv1.Binding{}
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
	if reflect.DeepEqual(instance.Status, ibmcloudv1.BindingStatus{}) {
		instance.Status.State = bindingStatePending
		instance.Status.Message = "Processing Resource"
		if err := r.Status().Update(ctx, instance); err != nil {
			logt.Info("Binding could not update Status", instance.Name, err.Error())
			// TODO(johnstarich): Shouldn't this be a failure so it can be requeued?
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
			instance.ObjectMeta.Finalizers = deleteBindingFinalizer(instance)
			if err := r.Update(ctx, instance); err != nil {
				logt.Info("Error removing finalizers", "in deletion", err.Error())
				// No further action required, object was modified, another reconcile will finish the job.
			}
			return ctrl.Result{}, nil
		}

		// In case there previously existed a service instance and it's now gone, reset the state of the resource
		if instance.Status.KeyInstanceID != "" {
			return r.resetResource(instance)
		}

		return ctrl.Result{Requeue: true, RequeueAfter: requeueFast}, nil
	}

	// Set an owner reference if service and binding are in the same namespace
	if serviceInstance.Namespace == instance.Namespace {
		if err := r.SetOwnerReference(serviceInstance, instance, r.Scheme()); err != nil {
			logt.Info("Binding could not update owner reference", instance.Name, err.Error())
			return ctrl.Result{}, err
		}

		if err := r.Update(ctx, instance); err != nil {
			logt.Info("Error setting owner reference", instance.Name, err.Error())
			return ctrl.Result{}, nil
		}
	}

	// If the service has not been initialized fully yet, then requeue
	if serviceInstance.Status.InstanceID == "" || serviceInstance.Status.InstanceID == inProgress {
		// The parent service has not been initialized fully yet
		logt.Info("Parent service", "not yet initialized", instance.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: requeueFast}, nil
	}

	var serviceClassType string
	var session *session.Session
	{
		ibmCloudInfo, err := r.GetIBMCloudInfo(logt, r.Client, serviceInstance)
		if err != nil {
			logt.Info("Unable to get IBM Cloud info", "ibmcloudInfo", instance.Name)
			if errors.IsNotFound(err) && containsBindingFinalizer(instance) &&
				!instance.ObjectMeta.DeletionTimestamp.IsZero() {
				logt.Info("Cannot get IBMCloud related secrets and configmaps, just remove finalizers", "in deletion", err.Error())
				instance.ObjectMeta.Finalizers = deleteBindingFinalizer(instance)
				if err := r.Update(ctx, instance); err != nil {
					logt.Info("Error removing finalizers", "in deletion", err.Error())
				}
				return ctrl.Result{}, nil
			}
			return r.updateStatusError(instance, bindingStatePending, err)
		}
		logt = logt.WithValues("User", ibmCloudInfo.Context.User)
		serviceClassType = ibmCloudInfo.ServiceClassType
		session = ibmCloudInfo.Session
	}

	// Delete if necessary
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Instance is not being deleted, add the finalizer if not present
		if !containsBindingFinalizer(instance) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, bindingFinalizer)
			if err := r.Update(ctx, instance); err != nil {
				logt.Info("Error adding finalizer", instance.Name, err.Error())
				return ctrl.Result{}, nil
			}
		}
	} else {
		// The object is being deleted
		if containsBindingFinalizer(instance) {
			logt.Info("Resource marked for deletion", "in deletion", instance.Name)
			err := r.deleteCredentials(session, instance, serviceClassType)
			if err != nil {
				logt.Info("Error deleting credentials", "in deletion", err.Error())
				return ctrl.Result{Requeue: true, RequeueAfter: requeueFast}, nil
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = deleteBindingFinalizer(instance)
			if err := r.Update(ctx, instance); err != nil {
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
			err := r.deleteCredentials(session, instance, serviceClassType)
			if err != nil {
				logt.Info("Error deleting credentials", "in deletion", err.Error())
				return r.updateStatusError(instance, bindingStateFailed, err)
			}
			instance.Status.InstanceID = serviceInstance.Status.InstanceID
		}
	}

	// Now instance.Status.IntanceID has been set properly
	if instance.Status.KeyInstanceID == "" { // The KeyInstanceID has not been set, need to create the key
		instance.Status.KeyInstanceID = inProgress
		if err := r.Status().Update(ctx, instance); err != nil {
			logt.Info("Error updating KeyInstanceID to be in progress", "Error", err.Error())
			// TODO(johnstarich): Shouldn't this be a failure so it can be requeued?
			return ctrl.Result{}, nil
		}

		var keyInstanceID string
		var keyContents map[string]interface{}

		if instance.Spec.Alias != "" {
			keyInstanceID, keyContents, err = r.getAliasCredentials(logt, session, instance, serviceClassType)
			if err != nil {
				logt.Info("Error retrieving alias credentials", instance.Name, err.Error())
				return r.updateStatusError(instance, bindingStatePending, err)
			}
		} else {
			keyInstanceID, keyContents, err = r.createCredentials(ctx, session, instance, serviceClassType)
			if err != nil {
				logt.Info("Error creating credentials", instance.Name, err.Error())
				if strings.Contains(err.Error(), "still in progress") {
					return r.updateStatusError(instance, bindingStatePending, err)
				}
				return r.updateStatusError(instance, bindingStateFailed, err)
			}
		}
		instance.Status.KeyInstanceID = keyInstanceID

		// Now create the secret
		err = r.createSecret(instance, keyContents)

		if err != nil {
			logt.Info("Error creating secret", instance.Name, err.Error())
			return r.updateStatusError(instance, bindingStateFailed, err)
		}

		return r.updateStatusOnline(session, instance)
	}

	// The KeyInstanceID has been set (or is still inProgress), verify that the key and secret still exist
	logt.Info("ServiceInstance Key", "should already exist, verifying", instance.ObjectMeta.Name)
	var keyInstanceID string
	var keyContents map[string]interface{}
	if instance.Spec.Alias != "" {
		_, keyContents, err = r.getAliasCredentials(logt, session, instance, serviceClassType)
		if err != nil && strings.Contains(err.Error(), notFound) {
			return r.resetResource(instance)
		} else if err != nil {
			return r.updateStatusError(instance, bindingStateFailed, err)
		}
	} else {
		_, keyContents, err = r.getCredentials(logt, session, instance, serviceClassType)
		if err != nil && strings.Contains(err.Error(), notFound) {
			logt.Info("ServiceInstance Key does not exist", "Recreating", instance.ObjectMeta.Name)
			keyInstanceID, keyContents, err = r.createCredentials(ctx, session, instance, serviceClassType)
			if err != nil {
				return r.updateStatusError(instance, bindingStateFailed, err)
			}
			instance.Status.KeyInstanceID = keyInstanceID
		} else if err != nil {
			logt.Error(err, "Failed to fetch credentials") // TODO(johnstarich): should this fail and requeue?
		}
	}
	secret, err := getSecret(r, instance)
	if err != nil {
		logt.Info("Secret does not exist", "Recreating", getSecretName(instance))
		err = r.createSecret(instance, keyContents)
		if err != nil {
			logt.Info("Error creating secret", instance.Name, err.Error())
			return r.updateStatusError(instance, bindingStateFailed, err)
		}
		return r.updateStatusOnline(session, instance)
	}

	// The secret exists, make sure it has the right content
	changed, err := keyContentsChanged(keyContents, secret)
	if err != nil {
		logt.Info("Error checking if key contents have changed", instance.Name, err.Error())
		return r.updateStatusError(instance, bindingStateFailed, err)
	}
	instanceIDMismatch := instance.Status.KeyInstanceID != secret.Annotations["service-key-id"]
	if instanceIDMismatch || changed { // Warning: the deep comparison may not be needed, the key is probably enough
		logt.Info("Updating secret", "key contents changed", changed, "status key ID and annotation mismatch", instanceIDMismatch)
		err := r.deleteSecret(instance)
		if err != nil {
			logt.Info("Error deleting secret before recreating", instance.Name, err.Error())
			return r.updateStatusError(instance, bindingStateFailed, err)
		}
		err = r.createSecret(instance, keyContents)
		if err != nil {
			logt.Info("Error re-creating secret", instance.Name, err.Error())
			return r.updateStatusError(instance, bindingStateFailed, err)
		}
		return r.updateStatusOnline(session, instance)
	}
	return r.updateStatusOnline(session, instance)
}

func (r *BindingReconciler) getServiceInstance(instance *ibmcloudv1.Binding) (*ibmcloudv1.Service, error) {
	serviceNameSpace := instance.ObjectMeta.Namespace
	if instance.Spec.ServiceNamespace != "" {
		serviceNameSpace = instance.Spec.ServiceNamespace
	}
	serviceInstance := &ibmcloudv1.Service{}
	err := r.Get(context.Background(), types.NamespacedName{Name: instance.Spec.ServiceName, Namespace: serviceNameSpace}, serviceInstance)
	if err != nil {
		return &ibmcloudv1.Service{}, err
	}
	return serviceInstance, nil
}

func (r *BindingReconciler) resetResource(instance *ibmcloudv1.Binding) (ctrl.Result, error) {
	instance.Status.State = bindingStatePending
	instance.Status.Message = "Processing Resource"
	instance.Status.InstanceID = ""
	instance.Status.KeyInstanceID = ""

	// If a secret exists that corresponds to this Binding, then delete it
	err := r.deleteSecret(instance)
	if err != nil {
		r.Log.Info("Unable to delete", "secret", instance.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: config.Get().SyncPeriod}, nil
	}

	instance.Status.SecretName = ""
	if err := r.Status().Update(context.Background(), instance); err != nil {
		r.Log.Info("Binding could not reset Status", instance.Name, err.Error())
		// TODO(johnstarich): Shouldn't this be a failure so it can be requeued?
		return ctrl.Result{}, nil
	}
	return ctrl.Result{Requeue: true, RequeueAfter: config.Get().SyncPeriod}, nil
}

func (r *BindingReconciler) updateStatusError(instance *ibmcloudv1.Binding, state string, err error) (ctrl.Result, error) {
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
	return ctrl.Result{Requeue: true, RequeueAfter: config.Get().SyncPeriod}, nil
}

// deleteCredentials also deletes the corresponding secret
func (r *BindingReconciler) deleteCredentials(session *session.Session, instance *ibmcloudv1.Binding, serviceClassType string) error {
	r.Log.Info("Deleting", "credentials", instance.ObjectMeta.Name)

	if instance.Spec.Alias == "" { // Delete only if it not alias
		if serviceClassType == "CF" { // service type is CF
			err := r.DeleteCFServiceKey(session, instance.Status.KeyInstanceID)
			if err != nil {
				return err
			}
		} else { // service type is not CF
			err := r.DeleteResourceServiceKey(session, instance.Status.KeyInstanceID)
			if err != nil {
				return err
			}
		}
	}
	return r.deleteSecret(instance)
}

func (r *BindingReconciler) getAliasCredentials(logt logr.Logger, session *session.Session, instance *ibmcloudv1.Binding, serviceClassType string) (string, map[string]interface{}, error) {
	logt.Info("Getting", " alias credentials", instance.ObjectMeta.Name)
	name := instance.Spec.Alias

	if serviceClassType == "CF" { // service type is CF
		return r.getCFCredentials(logt, session, instance, name)
	}

	// service type is not CF
	keyid, annotationFound := instance.ObjectMeta.GetAnnotations()[idkey]
	if !annotationFound {
		return "", nil, fmt.Errorf("Alias credential does not have %s annotation", idkey)
	}
	guid, keyName, credentials, err := r.GetResourceServiceKey(session, keyid)
	if err != nil {
		return "", nil, err
	}

	if keyName != name { // alias name and keyid annotations are inconsistent
		return "", nil, fmt.Errorf("alias credential name and keyid do not match. Key name: %q, Alias name: %q", keyName, name)
	}

	_, contentsContainRedacted := credentials["REDACTED"]
	if contentsContainRedacted {
		return "", nil, fmt.Errorf(notFound)
	}

	return guid, credentials, nil
}

func (r *BindingReconciler) createCredentials(ctx context.Context, session *session.Session, instance *ibmcloudv1.Binding, serviceClassType string) (string, map[string]interface{}, error) {
	r.Log.Info("Creating", "credentials", instance.ObjectMeta.Name)
	parameters, err := r.getParams(ctx, instance)
	if err != nil {
		r.Log.Error(err, "Instance ", instance.ObjectMeta.Name, " has problems with its parameters")
		return "", nil, err
	}
	if serviceClassType == "CF" { // service type is CF
		return r.CreateCFServiceKey(session, instance.Status.InstanceID, instance.ObjectMeta.Name, parameters)
	}
	// service type is not CF
	return r.getResourceServiceCredentials(session, instance, parameters)
}

func (r *BindingReconciler) getResourceServiceCredentials(session *session.Session, instance *ibmcloudv1.Binding, parameters map[string]interface{}) (string, map[string]interface{}, error) {
	instanceCRN, serviceID, err := r.GetServiceInstanceCRN(session, instance.Status.InstanceID)
	if err != nil {
		return "", nil, err
	}
	serviceName, err := r.GetServiceName(session, serviceID)
	if err != nil {
		return "", nil, err
	}

	parameters["role_crn"], err = r.GetServiceRoleCRN(session, serviceName, instance.Spec.Role)
	if err != nil {
		return "", nil, err
	}

	return r.CreateResourceServiceKey(session, instance.ObjectMeta.Name, instanceCRN, parameters)
}

func (r *BindingReconciler) createSecret(instance *ibmcloudv1.Binding, keyContents map[string]interface{}) error {
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
	if err := r.SetControllerReference(instance, secret, r.Scheme()); err != nil {
		return err
	}
	if err := r.Create(context.Background(), secret); err != nil {
		return err
	}
	return nil
}

func (r *BindingReconciler) updateStatusOnline(session *session.Session, instance *ibmcloudv1.Binding) (ctrl.Result, error) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		currentBindingInstance := &ibmcloudv1.Binding{}
		err := r.Get(context.Background(), types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, currentBindingInstance)
		if err != nil {
			r.Log.Error(err, "Failed to fetch binding instance", "namespace", instance.Namespace, "name", instance.Name)
			return err
		}
		currentBindingInstance.Status.State = bindingStateOnline
		currentBindingInstance.Status.Message = bindingStateOnline
		currentBindingInstance.Status.SecretName = getSecretName(currentBindingInstance)
		currentBindingInstance.Status.KeyInstanceID = instance.Status.KeyInstanceID
		return r.Status().Update(context.Background(), currentBindingInstance)
	})
	if err != nil {
		r.Log.Error(err, "Failed to update binding instance after retry", "namespace", instance.Namespace, "name", instance.Name)
		return ctrl.Result{Requeue: true}, err
	}
	return ctrl.Result{Requeue: true, RequeueAfter: config.Get().SyncPeriod}, nil
}

func (r *BindingReconciler) getCredentials(logt logr.Logger, session *session.Session, instance *ibmcloudv1.Binding, serviceClassType string) (string, map[string]interface{}, error) {
	logt.Info("Getting", "credentials", instance.ObjectMeta.Name)

	if serviceClassType == "CF" { // service type is CF
		return r.getCFCredentials(logt, session, instance, instance.Name)
	}

	// service type is not CF
	if instance.Status.KeyInstanceID != "" && instance.Status.KeyInstanceID != inProgress { // There is a valid KeyInstanceID
		guid, _, credentials, err := r.GetResourceServiceKey(session, instance.Status.KeyInstanceID)
		return guid, credentials, err
	}

	return "", nil, fmt.Errorf(notFound)
}

func getSecretName(instance *ibmcloudv1.Binding) string {
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

func (r *BindingReconciler) deleteSecret(instance *ibmcloudv1.Binding) error {
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

func (r *BindingReconciler) getCFCredentials(logt logr.Logger, session *session.Session, instance *ibmcloudv1.Binding, name string) (string, map[string]interface{}, error) {
	logt.Info("Getting", "CF credentials", name)
	return r.GetCFServiceKeyCredentials(session, instance.Status.InstanceID, name)
}

func (r *BindingReconciler) getParams(ctx context.Context, instance *ibmcloudv1.Binding) (map[string]interface{}, error) {
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
func (r *BindingReconciler) paramToJSON(ctx context.Context, p ibmcloudv1.Param, namespace string) (interface{}, error) {
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
func (r *BindingReconciler) paramValueToJSON(ctx context.Context, valueFrom ibmcloudv1.ParamSource, namespace string) (interface{}, error) {
	if valueFrom.SecretKeyRef != nil {
		data, err := getKubeSecretValue(ctx, r, r.Log, valueFrom.SecretKeyRef.Name, valueFrom.SecretKeyRef.Key, true, namespace)
		if err != nil {
			// Recoverable
			return nil, fmt.Errorf("Missing secret %s", valueFrom.SecretKeyRef.Name)
		}
		return paramToJSONFromString(string(data))
	} else if valueFrom.ConfigMapKeyRef != nil {
		data, err := getConfigMapValue(ctx, r, r.Log, valueFrom.ConfigMapKeyRef.Name, valueFrom.ConfigMapKeyRef.Key, true, namespace)
		if err != nil {
			// Recoverable
			return nil, fmt.Errorf("Missing configmap %s", valueFrom.ConfigMapKeyRef.Name)
		}
		return paramToJSONFromString(data)
	}
	return nil, fmt.Errorf("Missing secretKeyRef or configMapKeyRef")
}

func paramToJSONFromRaw(content *ibmcloudv1.ParamValue) (interface{}, error) {
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

// containsBindingFinalizer checks if the instance contains service finalizer
func containsBindingFinalizer(instance *ibmcloudv1.Binding) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, bindingFinalizer) {
			return true
		}
	}
	return false
}

// deleteBindingFinalizer delete service finalizer
func deleteBindingFinalizer(instance *ibmcloudv1.Binding) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == bindingFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}
