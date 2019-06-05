/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	bxcontroller "github.com/IBM-Cloud/bluemix-go/api/resource/resourcev1/controller"
	ibmcloudv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
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
	c, err := controller.New("service-controller", mgr, controller.Options{Reconciler: r})
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
func (r *ReconcileService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Service instance
	instance := &ibmcloudv1alpha1.Service{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
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
		setStatusFieldsFromSpec(instance)
		if err := r.Update(context.TODO(), instance); err != nil {
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
		if err := r.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, nil
		}
	}

	ibmCloudInfo, err := GetIBMCloudInfo(r.Client, instance)
	if err != nil {
		return r.updateStatus(instance, "Failed", err)
	}

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
			err := r.deleteService(ibmCloudInfo, instance)
			if err != nil {
				logt.Info("Error deleting resource", instance.ObjectMeta.Name, err.Error())
				return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
			if err := r.Update(context.TODO(), instance); err != nil {
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

	if ibmCloudInfo.ServiceClassType == "CF" {
		logt.Info("ServiceInstance ", "is CF", instance.ObjectMeta.Name)
		serviceInstanceAPI := ibmCloudInfo.BXClient.ServiceInstances()

		if instance.Status.InstanceID == "" { // ServiceInstance has not been created on Bluemix
			logt.Info("Creating ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
			serviceInstance, err := serviceInstanceAPI.Create(mccpv2.ServiceInstanceCreateRequest{
				Name:      instance.ObjectMeta.Name,
				PlanGUID:  ibmCloudInfo.BxPlan.GUID,
				SpaceGUID: ibmCloudInfo.Space.GUID,
			})
			if err != nil {
				return r.updateStatus(instance, "Failed", err)
			}
			return r.updateStatusOnline(instance, ibmCloudInfo, serviceInstance.Metadata.GUID)
		}
		// ServiceInstance was previously created, verify that it is still there
		logt.Info("CF ServiceInstance ", "should already exists, verifying", instance.ObjectMeta.Name)
		_, err := serviceInstanceAPI.FindByName(instance.ObjectMeta.Name)
		if err != nil {
			if strings.Contains(err.Error(), "doesn't exist") {
				logt.Info("Recreating ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
				serviceInstance, err := serviceInstanceAPI.Create(mccpv2.ServiceInstanceCreateRequest{
					Name:      instance.ObjectMeta.Name,
					PlanGUID:  ibmCloudInfo.BxPlan.GUID,
					SpaceGUID: ibmCloudInfo.Space.GUID,
				})
				if err != nil {
					return r.updateStatus(instance, "Failed", err)
				}
				return r.updateStatusOnline(instance, ibmCloudInfo, serviceInstance.Metadata.GUID)
			}
			return r.updateStatus(instance, "Failed", err)
		}
		return r.updateStatusOnline(instance, ibmCloudInfo, instance.Status.InstanceID)

	} else { // Resource is not CF
		controllerClient, err := bxcontroller.New(ibmCloudInfo.Session)
		if err != nil {
			return r.updateStatus(instance, "Pending", err)
		}

		resServiceInstanceAPI := controllerClient.ResourceServiceInstance()
		var serviceInstancePayload = bxcontroller.CreateServiceInstanceRequest{
			Name:            instance.ObjectMeta.Name,
			ServicePlanID:   ibmCloudInfo.ServicePlanID,
			ResourceGroupID: ibmCloudInfo.ResourceGroupID,
			TargetCrn:       ibmCloudInfo.TargetCrn,
		}

		if instance.Status.InstanceID == "" { // ServiceInstance has not been created on Bluemix
			// Create the instance
			logt.Info("Creating ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
			instance.Status.InstanceID = "IN PROGRESS"
			if err := r.Update(context.TODO(), instance); err != nil {
				logt.Info("Error updating instanceID to be in progress", "Error", err.Error())
				return reconcile.Result{}, nil
			}
			serviceInstance, err := resServiceInstanceAPI.CreateInstance(serviceInstancePayload)
			if err != nil {
				return r.updateStatus(instance, "Failed", err)
			}
			return r.updateStatusOnline(instance, ibmCloudInfo, serviceInstance.ID)
		}

		// ServiceInstance was previously created, verify that it is still there
		logt.Info("ServiceInstance ", "should already exists, verifying", instance.ObjectMeta.Name)

		serviceInstanceQuery := bxcontroller.ServiceInstanceQuery{
			ResourceGroupID: ibmCloudInfo.ResourceGroupID,
			//ServiceID:       instance.Status.InstanceID,
			ServicePlanID: ibmCloudInfo.ServicePlanID,
			Name:          instance.ObjectMeta.Name,
		}

		serviceInstances, err := resServiceInstanceAPI.ListInstances(serviceInstanceQuery)
		if err != nil {
			return r.updateStatus(instance, "Pending", err)
		}

		if len(serviceInstances) == 0 { // Need to recreate it!
			logt.Info("Recreating ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
			instance.Status.InstanceID = "IN PROGRESS"
			if err := r.Update(context.TODO(), instance); err != nil {
				logt.Info("Error updating instanceID to be in progress", "Error", err.Error())
				return reconcile.Result{}, nil
			}
			serviceInstance, err := resServiceInstanceAPI.CreateInstance(serviceInstancePayload)
			if err != nil {
				return r.updateStatus(instance, "Failed", err)
			}
			return r.updateStatusOnline(instance, ibmCloudInfo, serviceInstance.ID)

		}
		logt.Info("ServiceInstance ", "exists", instance.ObjectMeta.Name)
		if instance.Status.State != "Online" {
			return r.updateStatusOnline(instance, ibmCloudInfo, instance.Status.InstanceID)
		}
	}

	return reconcile.Result{Requeue: true, RequeueAfter: time.Minute * 1}, nil
}

func (r *ReconcileService) updateStatusOnline(instance *ibmcloudv1alpha1.Service, ibmCloudInfo *IBMCloudInfo, instanceID string) (reconcile.Result, error) {
	instance.Status.State = "Online"
	instance.Status.Message = "Online"
	instance.Status.InstanceID = instanceID
	setStatusFieldsFromSpec(instance)
	err := r.Update(context.TODO(), instance)
	if err != nil {
		logt.Info("Failed to update online status, will delete external resource ", instance.ObjectMeta.Name, err.Error())
		err = r.deleteService(ibmCloudInfo, instance)
		if err != nil {
			logt.Info("Failed to delete external resource, operator state and external resource might be in an inconsistent state", instance.ObjectMeta.Name, err.Error())
		}
	}
	return reconcile.Result{}, nil
}

func setStatusFieldsFromSpec(instance *ibmcloudv1alpha1.Service) {
	instance.Status.Plan = instance.Spec.Plan
	instance.Status.ExternalName = instance.Spec.ExternalName
	instance.Status.ServiceClass = instance.Spec.ServiceClass
	instance.Status.ServiceClassType = instance.Spec.ServiceClassType
}

func (r *ReconcileService) updateStatus(instance *ibmcloudv1alpha1.Service, state string, err error) (reconcile.Result, error) {
	message := err.Error()
	logt.Info(message)
	instance.Status.State = state
	instance.Status.Message = message
	if err := r.Update(context.TODO(), instance); err != nil {
		logt.Info("Error updating status", state, err.Error())
	}
	return reconcile.Result{}, err
}

func (r *ReconcileService) deleteService(ibmCloudInfo *IBMCloudInfo, instance *ibmcloudv1alpha1.Service) error {
	if instance.Status.InstanceID == "" || instance.Status.InstanceID == "IN PROGRESS" {
		return nil // Nothing to do here, service was not intialized
	}
	if ibmCloudInfo.ServiceClassType == "CF" {
		logt.Info("Deleting ", instance.ObjectMeta.Name, instance.Spec.ServiceClass)
		serviceInstanceAPI := ibmCloudInfo.BXClient.ServiceInstances()
		err := serviceInstanceAPI.Delete(instance.Status.InstanceID)
		if err != nil {
			if strings.Contains(err.Error(), "could not be found") {
				return nil // Nothing to do here, service not found
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
