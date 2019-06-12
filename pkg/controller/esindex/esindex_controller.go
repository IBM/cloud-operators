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

package esindex

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

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

// IndexCreate for rest call body to elasticsearch
type IndexCreate struct {
	Settings struct {
		NumberOfShards   int32 `json:"number_of_shards,omitempty"`
		NumberOfReplicas int32 `json:"number_of_replicas,omitempty"`
	} `json:"settings,omitempty"`
	Mappings map[string]interface{} `json:"mappings,omitempty"`
}

// reconcileContext
type reconcileContext struct {
	context.Context
	cl        client.Client
	namespace string
	name      string
}

// retryInterval is the waiting time before next retry
const (
	retryInterval time.Duration = time.Second * 20
	pingInterval  time.Duration = time.Second * 80
)

const (
	// ResourceStateCreated indicates a resource is in a created state
	ResourceStateCreated string = "Created"
	// ResourceStatePending indicates a resource is in a pending state
	ResourceStatePending string = "Pending"
	// ResourceStateFailed indicates a resource is in a failed state
	ResourceStateFailed string = "Failed"
	// ResourceStateUnknown indicates a resource is in a unknown state
	ResourceStateUnknown string = "Unknown"
	// ResourceStateDeleting indicates a resource is being deleted
	ResourceStateDeleting string = "Deleting"
	// ResourceStateOnline indicates a resource has been fully synchronized and online
	ResourceStateOnline string = "Online"
	// ResourceStateBinding indicates a resource such as a cloud service is being bound
	ResourceStateBinding string = "Binding"
)

var logt = logf.Log.WithName("esindex")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new EsIndex Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this ibmcloud.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileEsIndex{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("esindex-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to EsIndex
	err = c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.EsIndex{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileEsIndex{}

// ReconcileEsIndex reconciles a EsIndex object
type ReconcileEsIndex struct {
	client.Client
	scheme *runtime.Scheme
}

const esindexFinalizer = "esindex.cloud-operators.ibm.com"

// ContainsFinalizer checks if the instance contains streams finalizer
func ContainsFinalizer(instance *ibmcloudv1alpha1.EsIndex) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, esindexFinalizer) {
			return true
		}
	}
	return false
}

// ContainsOwnerReference checks if the instance contains streams finalizer
func ContainsOwnerReference(instance *ibmcloudv1alpha1.EsIndex) bool {
	if len(instance.ObjectMeta.OwnerReferences) > 0 {
		return true
	}
	return false
}

// DeleteFinalizer delete esindex finalizer
func DeleteFinalizer(instance *ibmcloudv1alpha1.EsIndex) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == esindexFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}

// Reconcile reads that state of the cluster for a EsIndex object and makes changes based on the state read
// and what is in the EsIndex.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=esindices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=esindices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=esindices/finalizers,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileEsIndex) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the EsIndex instance
	instance := &ibmcloudv1alpha1.EsIndex{}
	context := context.Background()
	err := r.Get(context, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// handle deletion
	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted
		if ContainsFinalizer(instance) {
			logt.Info("delete elastic search index", "name", instance.ObjectMeta.Name)
			result, err := r.deleteIndex(instance)

			if (err != nil && result.ErrorType == ErrorTypeEsURINotFound) ||
				(err == nil && result.StatusCode == 200) ||
				(result.StatusCode == 404) {
				logt.Info("remove k8s object", "name", instance.ObjectMeta.Name)
				instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
				if err := r.Update(context, instance); err != nil {
					logt.Error(err, "removing finalizers", "name", instance.ObjectMeta.Name)
					return reconcile.Result{Requeue: true}, nil
				}
				updateInstanceStatus(instance, ResourceStateDeleting, "Deleted the index on ElasticSearch.", instance.Status.Generation)
				r.Status().Update(context, instance)
				return reconcile.Result{}, nil
			}
			logt.Error(err, "delete elastic search index", "name", instance.ObjectMeta.Name)
			updateInstanceStatus(instance, ResourceStateDeleting, fmt.Sprintf("Deleting ... encountered an error and will retry. %v", err), instance.Status.Generation)
			r.Status().Update(context, instance)
			return reconcile.Result{RequeueAfter: retryInterval}, err

		}
		return reconcile.Result{}, nil
	}

	var needUpdate = false
	// add finalizer if not exist already
	if !ContainsFinalizer(instance) {
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, esindexFinalizer)
		needUpdate = true
	}
	// add owner reference to the instance if not exist already
	if !ContainsOwnerReference(instance) {
		if err := r.setCRDOwnerReference(instance); err != nil {
			logt.Error(err, "setCRDOwnerReference", "name", instance.ObjectMeta.Name)
		} else {
			needUpdate = true
		}
	}
	if needUpdate {
		r.Update(context, instance)
	}

	// handle creation
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.EsIndexStatus{}) {

		logt.Info("create index instance", "name", instance.ObjectMeta.Name, "indexName", instance.Spec.IndexName)
		resp, err := r.createIndex(instance)
		if err != nil {
			logt.Error(err, "createIndex", "indexName", instance.Spec.IndexName)
			if resp.ErrorType != "" && resp.ErrorType == ErrorTypeEsURINotFound {
				r.Update(context, instance)
				updateInstanceStatus(instance, ResourceStatePending, fmt.Sprintf("ElasticSearch credentials not found. Will retry shortly. %v", err), instance.Status.Generation)
				logt.Info("createIndex error", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
				r.Status().Update(context, instance)
				return reconcile.Result{}, nil
			}
			updateInstanceStatus(instance, ResourceStatePending, fmt.Sprintf("An error occurred and will retry shortly. %v", err), instance.Status.Generation)
			logt.Info("createIndex error", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
			r.Status().Update(context, instance)
			return reconcile.Result{}, nil
		}
		logt.Info("elasticsearch statusCode="+strconv.Itoa(resp.StatusCode), "response", resp.Body)
		if resp.StatusCode == 200 {
			now := fmt.Sprintf("%v", time.Now())
			updateInstanceStatus(instance, ResourceStateOnline, "Created successfully at "+now+". "+resp.Body, instance.ObjectMeta.Generation)
			r.Status().Update(context, instance)
			logt.Info("update state to Online", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
			return reconcile.Result{}, nil
		}
		if resp.StatusCode == 400 && instance.Spec.BindOnly == false {
			if strings.Contains(resp.Body, "resource_already_exists_exception") {
				updateInstanceStatus(instance, ResourceStateFailed, "You may try to set Spec.BindOnly to true. "+resp.Body, instance.ObjectMeta.Generation)
			} else {
				updateInstanceStatus(instance, ResourceStateFailed, "Bad request.  "+resp.Body, instance.ObjectMeta.Generation)
			}
			logt.Info("update status", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
			r.Status().Update(context, instance)
			return reconcile.Result{}, nil
		}
		// status code != 200 or != 400, set for retry
		updateInstanceStatus(instance, ResourceStatePending, "Encountered an error and will retry shortly. "+resp.Body, instance.ObjectMeta.Generation)
		logt.Info("update status", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
		r.Status().Update(context, instance)
		return reconcile.Result{}, nil
	}

	// handle retires
	if instance.Status.State == ResourceStatePending ||
		(instance.Status.State == ResourceStateFailed && instance.Spec.BindOnly == true) {
		//check elastic search, if index exists then change state to Online; otherwise create it
		getResult, err := r.getIndex(instance)
		if err != nil {
			logt.Error(err, "getIndex returned error", "retry", instance.ObjectMeta.Name)
			return reconcile.Result{RequeueAfter: retryInterval}, nil
		}
		if getResult.StatusCode == 200 { //index exists, update state to Online
			updateInstanceStatus(instance, ResourceStateOnline, "Online. "+getResult.Body, instance.ObjectMeta.Generation)
			logt.Info("getIndex returned 200", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
			r.Status().Update(context, instance)
			return reconcile.Result{}, nil
		}
		if getResult.StatusCode == 404 { //index not found on elastic search, call create
			resp, err := r.createIndex(instance)
			if err != nil {
				updateInstanceStatus(instance, instance.Status.State, fmt.Sprintf("%v", err), instance.ObjectMeta.Generation)
				logt.Info("createIndex returned error", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
				r.Status().Update(context, instance)
				return reconcile.Result{}, nil
			}
			if resp.StatusCode == 200 {
				now := fmt.Sprintf("%v", time.Now())
				updateInstanceStatus(instance, ResourceStateOnline, "Created successfully at "+now+". "+resp.Body, instance.ObjectMeta.Generation)
				logt.Info("createIndex returned 200", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
				r.Status().Update(context, instance)
				return reconcile.Result{}, nil
			}
			updateInstanceStatus(instance, ResourceStatePending, resp.Body, instance.ObjectMeta.Generation)
			r.Status().Update(context, instance)
			return reconcile.Result{}, nil
		}
		// return code is not 200 or 404
		updateInstanceStatus(instance, ResourceStatePending, "Error from ElasticSearch. "+getResult.Body, instance.Status.Generation)
		logt.Info("getIndex returned error", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
		r.Status().Update(context, instance)
		return reconcile.Result{}, nil
	}

	//monitor index existence on the remote service
	if instance.Status.State == ResourceStateOnline {
		getResult, err := r.getIndex(instance)
		if err != nil { // ResourceStateUnknown, update state and ping again
			updateInstanceStatus(instance, ResourceStateUnknown, getResult.Body, instance.Status.Generation)
			logt.Error(err, "ping remote index error", "indexname", instance.Spec.IndexName)
			r.Status().Update(context, instance)
			return reconcile.Result{}, nil
		}
		if getResult.StatusCode == 200 { //index exists, do nothing
			return reconcile.Result{RequeueAfter: pingInterval}, nil
		}
		if getResult.StatusCode == 404 { //not found, set state to Pending for reconcile to create it
			updateInstanceStatus(instance, ResourceStatePending, getResult.Body, instance.Status.Generation)
			logt.Info("ping remote index returned 404", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
			r.Status().Update(context, instance)
			return reconcile.Result{}, nil
		}
		updateInstanceStatus(instance, ResourceStateUnknown, getResult.Body, instance.Status.Generation)
		logt.Info("ping remote index returned "+strconv.Itoa(getResult.StatusCode), "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
		r.Status().Update(context, instance)
		return reconcile.Result{}, nil
	}

	if instance.Status.State == ResourceStateUnknown {
		getResult, err := r.getIndex(instance)
		if err != nil { // ResourceStateUnknown, requeue event to ping again
			logt.Error(err, "ping remote index error", "indexname", instance.Spec.IndexName)
			return reconcile.Result{RequeueAfter: pingInterval}, nil
		}
		if getResult.StatusCode == 200 { //
			updateInstanceStatus(instance, ResourceStateOnline, getResult.Body, instance.Status.Generation)
			logt.Info("getIndex returned 200", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
			r.Status().Update(context, instance)
			return reconcile.Result{}, nil
		}
		if getResult.StatusCode == 404 { //not found, set state to Pending for reconcile to create it
			updateInstanceStatus(instance, ResourceStatePending, getResult.Body, instance.Status.Generation)
			logt.Info("ping remote index returned 404", "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
			r.Status().Update(context, instance)
			return reconcile.Result{}, nil
		}
		logt.Info("ping remote index returned "+strconv.Itoa(getResult.StatusCode), "name", instance.ObjectMeta.Name, "state", instance.Status.State, "message", instance.Status.Message)
	}
	return reconcile.Result{RequeueAfter: pingInterval}, nil

}

// updateInstanceStatus sets instance status
func updateInstanceStatus(obj *ibmcloudv1alpha1.EsIndex, state string, msg string, generation int64) {
	obj.Status.State = state
	obj.Status.Message = msg
	obj.Status.Generation = generation
}
