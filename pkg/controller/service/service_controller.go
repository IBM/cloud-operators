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
	"reflect"
	"strings"
	"time"

	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	bxcontroller "github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	"github.com/IBM-Cloud/bluemix-go/models"
	ibmcloudv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var logt = logf.Log.WithName("service")

const serviceFinalizer = "service.ibmcloud.ibm.com"
const selfHealingKey = "ibmcloud.ibm.com/self-healing"
const instanceIDKey = "ibmcloud.ibm.com/instanceId"

// ContainsFinalizer checks if the instance contains service finalizer
func ContainsFinalizer(instance *ibmcloudv1alpha1.Service) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, serviceFinalizer) {
			return true
		}
	}
	return false
}

// DeleteFinalizer delete service finalizer
func DeleteFinalizer(instance *ibmcloudv1alpha1.Service) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == serviceFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}

// Add creates a new Service Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this ibmcloud.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileService{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("service-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: 30})
	if err != nil {
		return err
	}

	// Watch for changes to Service
	err = c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.Service{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileService{}

// ReconcileService reconciles a Service object
type ReconcileService struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Service object and makes changes based on the state read
// and what is in the Service.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=services/status,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Service instance
	instance := &ibmcloudv1alpha1.Service{}
	err := r.Get(context.Background(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Set the Status field for the first time
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.ServiceStatus{}) {
		instance.Status.State = "Pending"
		instance.Status.Message = "Processing Resource"
		setStatusFieldsFromSpec(instance, nil)
		if err := r.Status().Update(context.Background(), instance); err != nil {
			logt.Info(err.Error())
			return reconcile.Result{}, nil
		}
	}

	// Enforce immutability, restore the spec if it has changed
	if specChanged(instance) {
		logt.Info("Spec is immutable", "Restoring", instance.ObjectMeta.Name)
		instance.Spec.Plan = instance.Status.Plan
		instance.Spec.ExternalName = instance.Status.ExternalName
		instance.Spec.ServiceClass = instance.Status.ServiceClass
		instance.Spec.ServiceClassType = instance.Status.ServiceClassType
		if err := r.Update(context.Background(), instance); err != nil {
			return reconcile.Result{}, nil
		}
	}

	ibmCloudInfo, err := GetIBMCloudInfo(r.Client, instance)
	if err != nil {
		return r.updateStatusError(instance, "Failed", err)
	}

	// check is the self-healing annotation is declared
	selfHealing := isSelfHealing(instance)

	// Delete if necessary
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Instance is not being deleted, add the finalizer if not present
		if !ContainsFinalizer(instance) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, serviceFinalizer)
			if err := r.Update(context.Background(), instance); err != nil {
				return reconcile.Result{}, nil
			}
		}
	} else {
		// The object is being deleted
		if ContainsFinalizer(instance) {
			// service should only be deleted if NOT using plan `Alias`
			if strings.ToLower(instance.Spec.Plan) != aliasPlan {
				err := r.deleteService(ibmCloudInfo, instance)
				if err != nil {
					logt.Info("Error deleting resource", instance.ObjectMeta.Name, err.Error())
					return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
				}
			} else {
				logt.Info("Service is using the `Alias` plan, since it is not managed by the operator it will not be deleted", "instance name:", instance.ObjectMeta.Name)
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
			if err := r.Update(context.Background(), instance); err != nil {
				logt.Info("Error removing finalizers", "in deletion", err.Error())
			}
			return reconcile.Result{}, nil
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

	if ibmCloudInfo.ServiceClassType == "CF" {
		logt.Info("ServiceInstance ", "is CF", instance.ObjectMeta.Name)
		serviceInstanceAPI := ibmCloudInfo.BXClient.ServiceInstances()
		if instance.Status.InstanceID == "" { // ServiceInstance has not been created on Bluemix
			// check if using the alias plan, in that case we need to use the existing instance
			if strings.ToLower(instance.Spec.Plan) == aliasPlan {
				logt.Info("Using `Alias` plan, checking if instance exists")
				// TODO - should use external name if defined
				serviceInstance, err := serviceInstanceAPI.FindByName(instance.ObjectMeta.Name)
				if err != nil {
					logt.Error(err, "Instance ", instance.ObjectMeta.Name, " with `Alias` plan does not exists")
					return r.updateStatusError(instance, "Failed", err)
				} else {
					return r.updateStatus(instance, ibmCloudInfo, serviceInstance.GUID, resv1.ResourceStateOnline)
				}
			}

			logt.Info("Creating ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
			serviceInstance, err := serviceInstanceAPI.Create(mccpv2.ServiceInstanceCreateRequest{
				Name:      externalName,
				PlanGUID:  ibmCloudInfo.BxPlan.GUID,
				SpaceGUID: ibmCloudInfo.Space.GUID,
			})
			if err != nil {
				return r.updateStatusError(instance, "Failed", err)
			}
			return r.updateStatus(instance, ibmCloudInfo, serviceInstance.Metadata.GUID, serviceInstance.Entity.LastOperation.State)
		}
		// ServiceInstance was previously created, verify that it is still there
		logt.Info("CF ServiceInstance ", "should already exists, verifying", instance.ObjectMeta.Name)
		serviceInstance, err := serviceInstanceAPI.FindByName(externalName)
		if err != nil {
			if strings.Contains(err.Error(), "doesn't exist") && selfHealing {
				logt.Info("Recreating ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
				serviceInstance, err := serviceInstanceAPI.Create(mccpv2.ServiceInstanceCreateRequest{
					Name:      externalName,
					PlanGUID:  ibmCloudInfo.BxPlan.GUID,
					SpaceGUID: ibmCloudInfo.Space.GUID,
				})
				if err != nil {
					return r.updateStatusError(instance, "Failed", err)
				}
				return r.updateStatus(instance, ibmCloudInfo, serviceInstance.Metadata.GUID, serviceInstance.Entity.LastOperation.State)
			}
			return r.updateStatusError(instance, "Failed", err)
		}

		logt.Info("ServiceInstance ", "exists", instance.ObjectMeta.Name)
		// Verfication was successful, service exists, update the status if necessary
		return r.updateStatus(instance, ibmCloudInfo, instance.Status.InstanceID, serviceInstance.LastOperation.State)

	} else { // resource is not CF
		controllerClient, err := bxcontroller.New(ibmCloudInfo.Session)
		if err != nil {
			return r.updateStatusError(instance, "Pending", err)
		}

		resServiceInstanceAPI := controllerClient.ResourceServiceInstance()
		var serviceInstancePayload = bxcontroller.CreateServiceInstanceRequest{
			Name:            externalName,
			ServicePlanID:   ibmCloudInfo.ServicePlanID,
			ResourceGroupID: ibmCloudInfo.ResourceGroupID,
			TargetCrn:       ibmCloudInfo.TargetCrn,
		}

		if instance.Status.InstanceID == "" { // ServiceInstance has not been created on Bluemix
			// check if using the alias plan, in that case we need to use the existing instance
			if strings.ToLower(instance.Spec.Plan) == aliasPlan {
				logt.Info("Using `Alias` plan, checking if instance exists")
				serviceInstanceQuery := bxcontroller.ServiceInstanceQuery{
					// Warning: Do not add the ServiceID to this query
					ResourceGroupID: ibmCloudInfo.ResourceGroupID,
					ServicePlanID:   ibmCloudInfo.ServicePlanID,
					Name:            instance.ObjectMeta.Name,
				}

				serviceInstances, err := resServiceInstanceAPI.ListInstances(serviceInstanceQuery)
				if err != nil {
					return r.updateStatusError(instance, "Pending", err)
				}
				if len(serviceInstances) == 0 {
					return r.updateStatusError(instance, "Failed", fmt.Errorf("No service instances with name %s found for alias plan", instance.ObjectMeta.Name))
				}

				// check if there is an annotation for service ID
				serviceID, annotationFound := instance.ObjectMeta.GetAnnotations()[instanceIDKey]

				// if only one instance with that name is found, then instanceID is not required, but if present it should match the ID
				if len(serviceInstances) == 1 {
					logt.Info("Found 1 service instance for `Alias` plan:", "Name", instance.ObjectMeta.Name, "InstanceID", serviceInstances[0].ID)
					if annotationFound { // check matches ID
						if serviceID != serviceInstances[0].ID {
							return r.updateStatusError(instance, "Failed", fmt.Errorf("service ID annotation %s for instance %s does not match instance ID %s found", serviceID, instance.ObjectMeta.Name, serviceInstances[0].ID))
						}
					}
					return r.updateStatus(instance, ibmCloudInfo, serviceInstances[0].ID, serviceInstances[0].State)
				}

				// if there is more then 1 service instance with the same name, then the instance ID annotation must be present
				logt.Info("Multiple service instances for `Alias` plan and instance", "Name", instance.ObjectMeta.Name)
				if annotationFound {
					serviceInstance, err := GetServiceInstance(serviceInstances, serviceID)
					if err != nil {
						r.updateStatusError(instance, "Failed", err)
					}
					if serviceInstance.ServiceID == "" {
						return r.updateStatusError(instance, "Failed", fmt.Errorf("Could not find matching instance with name %s and serviceID %s", instance.ObjectMeta.Name, serviceID))
					}
					logt.Info("Found service instances for `Alias` plan and instance", "Name", instance.ObjectMeta.Name, "InstanceID", serviceID)
					return r.updateStatus(instance, ibmCloudInfo, serviceInstance.ID, serviceInstance.State)
				} else {
					return r.updateStatusError(instance, "Failed", fmt.Errorf("multiple instance with same name found, and plan `Alias` requires `ibmcloud.ibm.com/instanceId` annotation for service %s", instance.ObjectMeta.Name))
				}
			}

			// Create the instance
			logt.Info("Creating ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
			instance.Status.InstanceID = "IN PROGRESS"
			if err := r.Status().Update(context.Background(), instance); err != nil {
				logt.Info("Error updating instanceID to be in progress", "Error", err.Error())
				return reconcile.Result{}, nil
			}
			serviceInstance, err := resServiceInstanceAPI.CreateInstance(serviceInstancePayload)
			if err != nil {
				return r.updateStatusError(instance, "Failed", err)
			}
			return r.updateStatus(instance, ibmCloudInfo, serviceInstance.ID, serviceInstance.State)
		}

		// ServiceInstance was previously created, verify that it is still there
		logt.Info("ServiceInstance ", "should already exists, verifying", instance.ObjectMeta.Name)

		serviceInstanceQuery := bxcontroller.ServiceInstanceQuery{
			// Warning: Do not add the ServiceID to this query
			ResourceGroupID: ibmCloudInfo.ResourceGroupID,
			ServicePlanID:   ibmCloudInfo.ServicePlanID,
			Name:            externalName,
		}

		serviceInstances, err := resServiceInstanceAPI.ListInstances(serviceInstanceQuery)
		if err != nil {
			return r.updateStatusError(instance, "Pending", err)
		}

		serviceInstance, err := GetServiceInstance(serviceInstances, instance.Status.InstanceID)
		if err != nil && strings.Contains(err.Error(), "not found") && selfHealing { // Need to recreate it!
			logt.Info("Recreating ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
			instance.Status.InstanceID = "IN PROGRESS"
			if err := r.Status().Update(context.Background(), instance); err != nil {
				logt.Info("Error updating instanceID to be in progress", "Error", err.Error())
				return reconcile.Result{}, nil
			}
			serviceInstance, err := resServiceInstanceAPI.CreateInstance(serviceInstancePayload)
			if err != nil {
				return r.updateStatusError(instance, "Failed", err)
			}
			return r.updateStatus(instance, ibmCloudInfo, serviceInstance.ID, serviceInstance.State)

		}
		logt.Info("ServiceInstance ", "exists", instance.ObjectMeta.Name)

		// Verification was successful, service exists, update the status if necessary
		return r.updateStatus(instance, ibmCloudInfo, instance.Status.InstanceID, serviceInstance.State)

	}
}

func getExternalName(instance *ibmcloudv1alpha1.Service) string {
	if instance.Spec.ExternalName != "" {
		return instance.Spec.ExternalName
	}
	return instance.Name
}

// GetServiceInstance gets the instance with given ID
func GetServiceInstance(instances []models.ServiceInstance, ID string) (models.ServiceInstance, error) {
	for _, instance := range instances {
		if instance.ID == ID {
			return instance, nil
		}
	}
	return models.ServiceInstance{}, fmt.Errorf("not found")
}

func (r *ReconcileService) updateStatus(instance *ibmcloudv1alpha1.Service, ibmCloudInfo *IBMCloudInfo, instanceID string, instanceState string) (reconcile.Result, error) {
	state := getState(instanceState)
	instance.Status.State = state
	instance.Status.Message = state
	instance.Status.InstanceID = instanceID
	setStatusFieldsFromSpec(instance, ibmCloudInfo)
	err := r.Status().Update(context.Background(), instance)
	if err != nil && isSelfHealing(instance) {
		logt.Info("Failed to update online status, will delete external resource ", instance.ObjectMeta.Name, err.Error())
		err = r.deleteService(ibmCloudInfo, instance)
		if err != nil {
			logt.Info("Failed to delete external resource, operator state and external resource might be in an inconsistent state", instance.ObjectMeta.Name, err.Error())
		}
	}
	return reconcile.Result{Requeue: true, RequeueAfter: time.Minute * 30}, nil
}

func getState(serviceInstanceState string) string {
	if serviceInstanceState == "succeeded" || serviceInstanceState == "active" || serviceInstanceState == "provisioned" {
		return "Online"
	}
	return serviceInstanceState
}

func setStatusFieldsFromSpec(instance *ibmcloudv1alpha1.Service, ibmCloudInfo *IBMCloudInfo) {
	instance.Status.Plan = instance.Spec.Plan
	instance.Status.ExternalName = instance.Spec.ExternalName
	instance.Status.ServiceClass = instance.Spec.ServiceClass
	instance.Status.ServiceClassType = instance.Spec.ServiceClassType
	if ibmCloudInfo != nil {
		instance.Status.Context = ibmCloudInfo.Context
	}
}

func (r *ReconcileService) updateStatusError(instance *ibmcloudv1alpha1.Service, state string, err error) (reconcile.Result, error) {
	message := err.Error()
	logt.Info(message)
	if strings.Contains(message, "dial tcp: lookup iam.cloud.ibm.com: no such host") || strings.Contains(message, "dial tcp: lookup login.ng.bluemix.net: no such host") {
		// This means that the IBM Cloud server is under too much pressure, we need to backup
		if instance.Status.State != state {
			instance.Status.Message = "Temporarily lost connection to server"
			instance.Status.State = "Pending"
			if err := r.Status().Update(context.Background(), instance); err != nil {
				logt.Info("Error updating status", state, err.Error())
			}
		}
		return reconcile.Result{Requeue: true, RequeueAfter: time.Minute * 3}, nil
	}
	if instance.Status.State != state {
		instance.Status.State = state
		instance.Status.Message = message
		if err := r.Status().Update(context.Background(), instance); err != nil {
			logt.Info("Error updating status", state, err.Error())
			return reconcile.Result{}, nil
		}
	}
	return reconcile.Result{Requeue: true, RequeueAfter: time.Minute * 3}, nil
}

func (r *ReconcileService) deleteService(ibmCloudInfo *IBMCloudInfo, instance *ibmcloudv1alpha1.Service) error {
	if instance.Status.InstanceID == "" || instance.Status.InstanceID == "IN PROGRESS" {
		return nil // Nothing to do here, service was not intialized
	}
	if ibmCloudInfo.ServiceClassType == "CF" {
		logt.Info("Deleting ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
		serviceInstanceAPI := ibmCloudInfo.BXClient.ServiceInstances()
		err := serviceInstanceAPI.Delete(instance.Status.InstanceID, true, true) // async, recursive (i.e. delete credentials)
		if err != nil {
			if strings.Contains(err.Error(), "could not be found") {
				return nil // Nothing to do here, service not found
			}
			if strings.Contains(err.Error(), "410") {
				return nil
			}
			return err
		}

	} else { // Resource is not CF
		logt.Info("Deleting ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
		controllerClient, err := bxcontroller.New(ibmCloudInfo.Session)
		if err != nil {
			logt.Info("Deletion error", "ServiceInstance", err.Error())
			return err
		}
		resServiceInstanceAPI := controllerClient.ResourceServiceInstance()

		err = resServiceInstanceAPI.DeleteInstance(instance.Status.InstanceID, true)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return nil // Nothing to do here, service not found
			}
			if strings.Contains(err.Error(), "410") {
				return nil
			}
			return err
		}
	}
	return nil
}

func specChanged(instance *ibmcloudv1alpha1.Service) bool {
	if instance.Status.Plan == "" { // Object has not been fully created yet
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
	return false
}

// check if self healing is enabled
func isSelfHealing(instance *ibmcloudv1alpha1.Service) bool {
	selfHealing := false
	v, ok := instance.ObjectMeta.GetAnnotations()[selfHealingKey]
	if ok {
		if v == "enabled" {
			logt.Info("Found annotation ", selfHealingKey, "=", v, " self healing is enabled")
			selfHealing = true
		} else {
			logt.Info("Found annotation ", selfHealingKey, "=", v, " but self healing is NOT enabled")
		}
	} else {
		logt.Info("Annotation ", selfHealingKey, " not found - self Healing is NOT enabled")
	}
	// check if using the alias plan - in this case self healing should be disabled but print a warning
	if (strings.ToLower(instance.Spec.Plan) == aliasPlan) && selfHealing {
		logt.Info("Warning: self healing annotation for cannot be used witb Alias plan. Setting self healing to false.", "instanceName", instance.ObjectMeta.Name)
		selfHealing = false
	}
	return selfHealing
}
