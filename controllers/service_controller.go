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
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
	"github.com/ibm/cloud-operators/internal/config"
	"github.com/ibm/cloud-operators/internal/ibmcloud/cfservice"
	"github.com/ibm/cloud-operators/internal/ibmcloud/resource"
)

const (
	serviceFinalizer = "service.ibmcloud.ibm.com"
	instanceIDKey    = "ibmcloud.ibm.com/instanceId"
	aliasPlan        = "alias"
)

const (
	// serviceStatePending indicates a resource is in a pending state
	serviceStatePending string = "Pending"
	// serviceStateFailed indicates a resource is in a failed state
	serviceStateFailed string = "Failed"
	// serviceStateOnline indicates a resource has been fully synchronized and online
	serviceStateOnline string = "Online"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	CreateCFServiceInstance         cfservice.InstanceCreator
	CreateResourceServiceInstance   resource.ServiceInstanceCreator
	DeleteCFServiceInstance         cfservice.InstanceDeleter
	DeleteResourceServiceInstance   resource.ServiceInstanceDeleter
	GetCFServiceInstance            cfservice.InstanceGetter
	GetIBMCloudInfo                 IBMCloudInfoGetter
	GetResourceServiceAliasInstance resource.ServiceAliasInstanceGetter
	GetResourceServiceInstanceState resource.ServiceInstanceStatusGetter
	UpdateResourceServiceInstance   resource.ServiceInstanceUpdater
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ibmcloudv1beta1.Service{}).
		Complete(r)
}

// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=services/status,verbs=get;update;patch

// Reconcile reads the state of the cluster for a Service object and makes changes based on the state read
// and what is in the Service.Spec.
// Automatically generate RBAC rules to allow the Controller to read and write Deployments.
func (r *ServiceReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logt := r.Log.WithValues("service", request.NamespacedName)

	// Fetch the Service instance
	instance := &ibmcloudv1beta1.Service{}
	err := r.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Enforce immutability, restore the spec if it has changed
	if specChanged(instance) {
		logt.Info("Spec is immutable", "Restoring", instance.ObjectMeta.Name)
		instance.Spec.Plan = instance.Status.Plan
		instance.Spec.ExternalName = instance.Status.ExternalName
		instance.Spec.ServiceClass = instance.Status.ServiceClass
		instance.Spec.ServiceClassType = instance.Status.ServiceClassType
		instance.Spec.Context = instance.Status.Context
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	var (
		resourceContext ibmcloudv1beta1.ResourceContext
		session         *session.Session
		resourceGroupID,
		serviceClassType,
		servicePlanID,
		spaceID,
		targetCRN string
	)
	{
		ibmCloudInfo, err := r.GetIBMCloudInfo(logt, r.Client, instance)
		if err != nil {
			// If secrets have already been deleted and we are in a deletion flow, just delete the finalizers
			// to not prevent object from finalizing. This would cause orphaned service in IBM Cloud.
			if k8sErrors.IsNotFound(err) && containsServiceFinalizer(instance) &&
				!instance.ObjectMeta.DeletionTimestamp.IsZero() {
				logt.Info("Cannot get IBMCloud related secrets and configmaps, just remove finalizers", "in deletion", err.Error())
				instance.ObjectMeta.Finalizers = deleteServiceFinalizer(instance)
				if err := r.Update(ctx, instance); err != nil {
					logt.Error(err, "Error removing finalizers in deletion")
					// TODO(johnstarich): Shouldn't this be a failure so it can be requeued?
					// Also, should the status be updated to include this failure message?
				}
				return ctrl.Result{}, nil
			}
			logt.Error(err, "Failed to get IBM Cloud info for service")
			return r.updateStatusError(instance, serviceStateFailed, err)
		}
		resourceContext = ibmCloudInfo.Context
		resourceGroupID = ibmCloudInfo.ResourceGroupID
		serviceClassType = ibmCloudInfo.ServiceClassType
		session = ibmCloudInfo.Session
		targetCRN = ibmCloudInfo.TargetCrn
		if ibmCloudInfo.Space != nil {
			spaceID = ibmCloudInfo.Space.GUID
		}
		if ibmCloudInfo.BxPlan != nil {
			servicePlanID = ibmCloudInfo.BxPlan.GUID
		} else {
			servicePlanID = ibmCloudInfo.ServicePlanID
		}
		logt = logt.WithValues("User", ibmCloudInfo.Context.User)
	}

	// Set the Status field for the first time
	if reflect.DeepEqual(instance.Status, ibmcloudv1beta1.ServiceStatus{}) {
		instance.Status.State = serviceStatePending
		instance.Status.Message = "Processing Resource"
		//setStatusFieldsFromSpec(instance, ibmCloudInfo)
		if err := r.Status().Update(ctx, instance); err != nil {
			logt.Info("Failed setting status for the first time", "error", err.Error())
			return ctrl.Result{}, err
		}
	}

	// Delete if necessary
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Instance is not being deleted, add the finalizer if not present
		if !containsServiceFinalizer(instance) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, serviceFinalizer)
			if err := r.Update(ctx, instance); err != nil {
				logt.Error(err, "Error adding finalizer", "service", instance.ObjectMeta.Name)
				// TODO(johnstarich): Shouldn't this update the status with the failure message?
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsServiceFinalizer(instance) {
			err := r.deleteService(session, logt, instance, serviceClassType)
			if err != nil {
				logt.Error(err, "Error deleting resource", "service", instance.ObjectMeta.Name)
				// TODO(johnstarich): Shouldn't this return the error so it will be logged?
				return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = deleteServiceFinalizer(instance)
			err = r.Update(ctx, instance)
			if err != nil {
				logt.Error(err, "Error removing finalizers")
			}
			return ctrl.Result{}, err
		}
	}

	/*
		There is a representation invariant that is maintained by this code.
		When the Status.InstanceID is set, then the Plan, ServiceClass are also set in the Status,
		and together they point to a service in Bluemix that has been deployed and that
		is managed by this controller.

		The job of the Reconciler is to maintain this invariant.
	*/

	/*
		In the following code, we first check if the serviceClassType is CF or not.
		In both cases, we first check if the InstanceID has been set in Status.
		If not, we try to create the service on Bluemix. If InstanceID has been set,
		then we verify that the service still exists on Bluemix and recreate it if necessary.

		For non-CF resources, before creating we set the InstanceID to "IN PROGRESS".
		This is to mitigate a potential data race that could cause the service to
		be created more than once on Bluemix (with the same name, but different InstanceIDs).
		CF services do not allow multiple services with the same name, so this is not needed.

		When the service is created (or recreated), we update the Status fields to reflect
		the external state. If this update fails (because the underlying etcd instance was modified),
		then we restore the invariant by deleting the external resource that was created.
		In this case, another run of the Reconcile method will make the external state consistent with
		the updated spec.
	*/

	externalName := getExternalName(instance)
	params, err := r.getParams(ctx, instance)
	if err != nil {
		logt.Error(err, "Instance has problems with its parameters", "service", instance.ObjectMeta.Name)
		return r.updateStatusError(instance, serviceStateFailed, err)
	}
	tags := getTags(instance)
	logt.Info("ServiceInstance ", "name", externalName, "tags", tags)

	if serviceClassType == "CF" {
		logt.Info("ServiceInstance is CF", "instance", instance.ObjectMeta.Name)
		if instance.Status.InstanceID == "" { // ServiceInstance has not been created on Bluemix
			// check if using the alias plan, in that case we need to use the existing instance
			if isAlias(instance) {
				logt.Info("Using `Alias` plan, checking if instance exists")

				instanceID, _, err := r.GetCFServiceInstance(session, externalName)
				if err != nil {
					logt.Error(err, "Instance ", instance.ObjectMeta.Name, " with `Alias` plan does not exists")
					return r.updateStatusError(instance, serviceStateFailed, err)
				}
				return r.updateStatus(session, logt, instance, resourceContext, instanceID, serviceStateOnline, serviceClassType)
			}
			// Service is not Alias
			logt.Info("Creating", "instance", instance.ObjectMeta.Name, "service class", instance.Spec.ServiceClass)
			guid, state, err := r.CreateCFServiceInstance(session, externalName, servicePlanID, spaceID, params, tags)
			if err != nil {
				return r.updateStatusError(instance, serviceStateFailed, err)
			}
			return r.updateStatus(session, logt, instance, resourceContext, guid, state, serviceClassType)
		}
		// ServiceInstance was previously created, verify that it is still there
		logt.Info("CF ServiceInstance ", "should already exists, verifying", instance.ObjectMeta.Name)
		_, state, err := r.GetCFServiceInstance(session, externalName)
		if err != nil && !isAlias(instance) {
			if _, notFound := err.(cfservice.NotFoundError); notFound {
				logt.Info("Recreating ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)

				guid, state, err := r.CreateCFServiceInstance(session, externalName, servicePlanID, spaceID, params, tags)
				if err != nil {
					return r.updateStatusError(instance, serviceStateFailed, err)
				}
				return r.updateStatus(session, logt, instance, resourceContext, guid, state, serviceClassType)
			}
			return r.updateStatusError(instance, serviceStateFailed, err)
		} else if err != nil && isAlias(instance) {
			// reset the service instance ID, since it's gone
			instance.Status.InstanceID = ""
			return r.updateStatusError(instance, serviceStatePending, err)
		}

		logt.Info("ServiceInstance ", "exists", instance.ObjectMeta.Name)

		// Verification was successful, service exists, update the status if necessary
		return r.updateStatus(session, logt, instance, resourceContext, instance.Status.InstanceID, state, serviceClassType)

	}

	// resource is not CF
	createServiceInstance := func() (id, state string, err error) {
		return r.CreateResourceServiceInstance(session, externalName, servicePlanID, resourceGroupID, targetCRN, params, tags)
	}

	if instance.Status.InstanceID == "" { // ServiceInstance has not been created on Bluemix
		// check if using the alias plan, in that case we need to use the existing instance
		if isAlias(instance) {
			logt := logt.WithValues("Name", instance.ObjectMeta.Name)
			logt.Info("Using `Alias` plan, checking if instance exists")

			// check if there is an annotation for service ID
			instanceID := instance.ObjectMeta.GetAnnotations()[instanceIDKey]

			id, state, err := r.GetResourceServiceAliasInstance(session, instanceID, resourceGroupID, servicePlanID, externalName, logt)
			if _, notFound := err.(resource.NotFoundError); notFound {
				return r.updateStatusError(instance, serviceStateFailed, errors.Wrapf(err, "no service instances with name %s found for alias plan", instance.ObjectMeta.Name))
			}
			if err != nil {
				return r.updateStatusError(instance, serviceStateFailed, errors.Wrapf(err, "failed to resolve Alias plan instance %s", instance.ObjectMeta.Name))
			}
			return r.updateStatus(session, logt, instance, resourceContext, id, state, serviceClassType)
		}

		// Create the instance, service is not alias
		instance.Status.InstanceID = inProgress
		if err := r.Status().Update(ctx, instance); err != nil {
			logt.Info("Error updating InstanceID to be in progress", "Error", err.Error())
			return ctrl.Result{}, err
		}

		logt.Info("Creating ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
		id, state, err := createServiceInstance()
		if err != nil {
			return r.updateStatusError(instance, serviceStateFailed, err)
		}
		return r.updateStatus(session, logt, instance, resourceContext, id, state, serviceClassType)
	}

	// ServiceInstance was previously created, verify that it is still there
	logt.Info("ServiceInstance ", "should already exists, verifying", instance.ObjectMeta.Name)

	state, err := r.GetResourceServiceInstanceState(session, resourceGroupID, servicePlanID, externalName, instance.Status.InstanceID)
	if _, ok := err.(resource.NotFoundError); ok { // Need to recreate it!
		if !isAlias(instance) {
			logt.Info("Recreating ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
			instance.Status.InstanceID = inProgress
			if err := r.Status().Update(ctx, instance); err != nil {
				logt.Info("Error updating instanceID to be in progress", "Error", err.Error())
				return ctrl.Result{}, err
			}
			id, state, err := createServiceInstance()
			if err != nil {
				return r.updateStatusError(instance, serviceStateFailed, err)
			}
			return r.updateStatus(session, logt, instance, resourceContext, id, state, serviceClassType)
		}
		instance.Status.InstanceID = ""
		return r.updateStatusError(instance, serviceStatePending, fmt.Errorf("aliased service instance no longer exists"))
	}
	if err != nil {
		return r.updateStatusError(instance, serviceStatePending, err)
	}

	logt.Info("ServiceInstance ", "exists", instance.ObjectMeta.Name)

	// Update Params and Tags if they have changed
	if tagsOrParamsChanged(instance) {
		logt.Info("ServiceInstance ", "updating tags and/or parameters", instance.ObjectMeta.Name)
		state, err = r.UpdateResourceServiceInstance(session, instance.Status.InstanceID, externalName, servicePlanID, params, tags)
		if err != nil {
			logt.Info("Error updating tags and/or parameters", "Error", err.Error())
			return r.updateStatusError(instance, serviceStateFailed, err)
		}
	}

	// Verification was successful, service exists, update the status if necessary
	return r.updateStatus(session, logt, instance, resourceContext, instance.Status.InstanceID, state, serviceClassType)
}

func specChanged(instance *ibmcloudv1beta1.Service) bool {
	if reflect.DeepEqual(instance.Status, ibmcloudv1beta1.ServiceStatus{}) { // Object does not have a status field yet
		return false
	}
	// If the Plan has not been set, then there is no need to test is spec has changed, Object has not been fully initialized yet
	if instance.Status.Plan == "" {
		return false
	}
	if instance.Spec.ExternalName != instance.Status.ExternalName {
		return true
	}
	if instance.Spec.Plan != instance.Status.Plan {
		return true
	}
	if instance.Spec.ServiceClass != instance.Status.ServiceClass {
		return true
	}
	if instance.Spec.ServiceClassType != instance.Status.ServiceClassType {
		return true
	}

	if !reflect.DeepEqual(instance.Spec.Context, instance.Status.Context) {
		return true
	}

	return false
}

// containsServiceFinalizer checks if the instance contains service finalizer
func containsServiceFinalizer(instance *ibmcloudv1beta1.Service) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, serviceFinalizer) {
			return true
		}
	}
	return false
}

// deleteServiceFinalizer delete service finalizer
func deleteServiceFinalizer(instance *ibmcloudv1beta1.Service) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == serviceFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}

func (r *ServiceReconciler) updateStatusError(instance *ibmcloudv1beta1.Service, state string, err error) (ctrl.Result, error) {
	logt := r.Log.WithValues("namespacedname", instance.Namespace+"/"+instance.Name)
	message := err.Error()
	logt.Error(err, "Updating status with error")
	if strings.Contains(message, "no such host") {
		// This means that the IBM Cloud server is under too much pressure, we need to backup
		return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 5}, nil

	}
	if instance.Status.State != state {
		instance.Status.State = state
		instance.Status.Message = message
		if err := r.Status().Update(context.Background(), instance); err != nil {
			logt.Info("Error updating status", state, err.Error())
			return ctrl.Result{}, err
		}
		//return ctrl.Result{}, nil
	}
	return ctrl.Result{Requeue: true, RequeueAfter: config.Get().SyncPeriod}, nil
}

func (r *ServiceReconciler) deleteService(session *session.Session, logt logr.Logger, instance *ibmcloudv1beta1.Service, serviceClassType string) error {
	if isAlias(instance) {
		logt.Info("Aliased service will not be deleted", "Name", instance.Name)
		return nil
	}
	if instance.Status.InstanceID == "" {
		return nil // Nothing to do here, service was not intialized
	}
	if serviceClassType == "CF" {
		logt.Info("Deleting ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
		err := r.DeleteCFServiceInstance(session, instance.Status.InstanceID, logt)
		if err != nil {
			return err
		}

	} else { // Resource is not CF
		logt.Info("Deleting ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
		err := r.DeleteResourceServiceInstance(session, instance.Status.InstanceID, logt)
		if err != nil {
			return err
		}
	}
	return nil
}

func getExternalName(instance *ibmcloudv1beta1.Service) string {
	if instance.Spec.ExternalName != "" {
		return instance.Spec.ExternalName
	}
	return instance.Name
}

func (r *ServiceReconciler) getParams(ctx context.Context, instance *ibmcloudv1beta1.Service) (map[string]interface{}, error) {
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

// paramToJSON converts variable value to JSON value
func (r *ServiceReconciler) paramToJSON(ctx context.Context, p ibmcloudv1beta1.Param, namespace string) (interface{}, error) {
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
func (r *ServiceReconciler) paramValueToJSON(ctx context.Context, valueFrom ibmcloudv1beta1.ParamSource, namespace string) (interface{}, error) {
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

func getTags(instance *ibmcloudv1beta1.Service) []string {
	return instance.Spec.Tags
}

func isAlias(instance *ibmcloudv1beta1.Service) bool {
	return strings.ToLower(instance.Spec.Plan) == aliasPlan
}

func (r *ServiceReconciler) updateStatus(session *session.Session, logt logr.Logger, instance *ibmcloudv1beta1.Service, resourceContext ibmcloudv1beta1.ResourceContext, instanceID, instanceState, serviceClassType string) (ctrl.Result, error) {
	r.Log.Info("the instance state", "is:", instanceState)
	state := getState(instanceState)
	if instance.Status.State != state || instance.Status.InstanceID != instanceID || tagsOrParamsChanged(instance) {
		instance.Status.State = state
		instance.Status.Message = state
		instance.Status.InstanceID = instanceID
		instance.Status.DashboardURL = getDashboardURL(instance.Spec.ServiceClass, instanceID)
		setStatusFieldsFromSpec(instance, resourceContext)
		err := r.Status().Update(context.Background(), instance)
		if err != nil {
			r.Log.Info("Failed to update online status, will delete external resource ", instance.ObjectMeta.Name, err.Error())
			errD := r.deleteService(session, logt, instance, serviceClassType)
			if errD != nil {
				r.Log.Info("Failed to delete external resource, operator state and external resource might be in an inconsistent state", instance.ObjectMeta.Name, errD.Error())
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{Requeue: true, RequeueAfter: config.Get().SyncPeriod}, nil
}

func getState(serviceInstanceState string) string {
	if serviceInstanceState == "succeeded" || serviceInstanceState == "active" || serviceInstanceState == "provisioned" {
		return "Online"
	}
	return serviceInstanceState
}

func setStatusFieldsFromSpec(instance *ibmcloudv1beta1.Service, resourceContext ibmcloudv1beta1.ResourceContext) {
	instance.Status.Plan = instance.Spec.Plan
	instance.Status.ExternalName = instance.Spec.ExternalName
	instance.Status.ServiceClass = instance.Spec.ServiceClass
	instance.Status.ServiceClassType = instance.Spec.ServiceClassType
	instance.Status.Parameters = instance.Spec.Parameters
	instance.Status.Tags = instance.Spec.Tags
	instance.Status.Context = resourceContext
	instance.Spec.Context = resourceContext
}

func tagsOrParamsChanged(instance *ibmcloudv1beta1.Service) bool {
	return !reflect.DeepEqual(instance.Spec.Parameters, instance.Status.Parameters) || !reflect.DeepEqual(instance.Spec.Tags, instance.Status.Tags)
}

func getDashboardURL(serviceClass, crn string) string {
	url := "https://cloud.ibm.com/services/" + serviceClass + "/"
	crn = strings.Replace(crn, ":", "%3A", -1)
	crn = strings.Replace(crn, "/", "%2F", -1)
	return url + crn
}
