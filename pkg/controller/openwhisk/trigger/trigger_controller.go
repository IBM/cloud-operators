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

package trigger

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/apache/incubator-openwhisk-client-go/whisk"

	context "github.com/ibm/cloud-operators/pkg/context"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"

	openwhiskv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	ow "github.com/ibm/cloud-operators/pkg/controller/openwhisk/common"
)

var clog = logf.Log

// Add creates a new Trigger Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileTrigger{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("trigger-controller", mgr, controller.Options{MaxConcurrentReconciles: 32, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Trigger
	err = c.Watch(&source.Kind{Type: &openwhiskv1alpha1.Trigger{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileTrigger{}

// ReconcileTrigger reconciles a Trigger object
type ReconcileTrigger struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Trigger object and makes changes based on the state read
// and what is in the Trigger.Spec
// Automatically generate RBAC triggers to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=triggers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=triggers/status,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileTrigger) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	context := context.New(r.Client, request)

	// Fetch the Function instance
	trigger := &openwhiskv1alpha1.Trigger{}
	err := r.Get(context, request.NamespacedName, trigger)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Reconcile or finalize?
	if trigger.GetDeletionTimestamp() != nil {
		return r.finalize(context, trigger)
	}

	log := clog.WithValues("namespace", trigger.Namespace, "name", trigger.Name)

	// Check generation
	currentGeneration := trigger.Generation
	syncedGeneration := trigger.Status.Generation
	if currentGeneration != 0 && syncedGeneration >= currentGeneration {
		// condition generation matches object generation. Nothing to do
		log.Info("trigger up-to-date")
		return reconcile.Result{}, nil
	}

	// Check Finalizer is set
	if !resv1.HasFinalizer(trigger, ow.Finalizer) {
		trigger.SetFinalizers(append(trigger.GetFinalizers(), ow.Finalizer))

		if err := r.Update(context, trigger); err != nil {
			log.Info("setting finalizer failed. (retrying)", "error", err)
			return reconcile.Result{}, err
		}
	}

	// Make sure status is Pending
	if err := ow.SetStatusToPending(context, r.Client, trigger, "deploying"); err != nil {
		return reconcile.Result{}, err
	}

	retry, err := r.updateTrigger(context, trigger)
	if err != nil {
		if !retry {
			log.Error(err, "deployment failed")

			// Non recoverable error.
			trigger.Status.Generation = currentGeneration
			trigger.Status.State = resv1.ResourceStateFailed
			trigger.Status.Message = fmt.Sprintf("%v", err)
			if err := resv1.PutStatusAndEmit(context, trigger); err != nil {
				log.Info("failed to set status. (retrying)", "error", err)
			}
			return reconcile.Result{}, nil
		}
		log.Error(err, "deployment failed (retrying)", "error", err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileTrigger) updateTrigger(context context.Context, obj *openwhiskv1alpha1.Trigger) (bool, error) {
	log := clog.WithValues("namespace", obj.Namespace, "name", obj.Name)

	trigger := obj.Spec
	wsktrigger := new(whisk.Trigger)
	wsktrigger.Name = obj.Name
	if trigger.Name != "" {
		wsktrigger.Name = trigger.Name
	}

	log.Info("deploying trigger")

	pub := false
	wsktrigger.Publish = &pub

	var err error

	wskclient, err := ow.NewWskClient(context, obj.Spec.ContextFrom)
	if err != nil {
		return true, fmt.Errorf("error creating Cloud Function client %v. (retrying)", err)
	}

	feedName := trigger.Feed
	isFeed := feedName != ""

	// convert parameters
	params, retry, err := ow.ConvertKeyValues(context, obj, trigger.Parameters, "parameters")
	if err != nil || retry {
		// parameters not all resolved... retry
		return retry, err
	}

	deleteTrigger(wskclient, obj, wsktrigger.Name, params)

	log.Info("preparing trigger")

	// annotations
	annotations, retry, err := ow.ConvertKeyValues(context, obj, trigger.Annotations, "annotations")
	if err != nil || retry {
		return retry, err
	}

	var feedQName ow.QualifiedName
	if isFeed {
		feedQName, err = ow.ParseQualifiedName(feedName, "_")
		if err != nil {
			resv1.SetStatus(obj, resv1.ResourceStateFailed, "[%s] invalid feed name %s: %v", obj.Name, feedName, err)
			return false, err // not recoverable
		}

		keyVal := whisk.KeyValue{
			Key:   "feed",
			Value: ow.JoinQualifiedName(feedQName), // Full name
		}

		annotations = append(annotations, keyVal)
	}

	// if we have successfully parser valid key/value annotations
	if len(annotations) > 0 {
		wsktrigger.Annotations = annotations
	}

	if !isFeed && len(params) > 0 {
		// Add parameter to the trigger
		wsktrigger.Parameters = params
	}

	log.Info("calling wsk trigger update")

	_, resp, err := wskclient.Triggers.Insert(wsktrigger, true)

	if err != nil {
		log.Error(err, "failed to deploy trigger", "response", resp)
		// if ow.ShouldRetry(context, resp, err) {
		return true, fmt.Errorf("[%s] failed to deploy trigger (%v). (retrying)", obj.Name, err)
		//	}

		// resv1.SetStatus(obj, resv1.ResourceStateFailed, "Error deploying trigger: %v", err)
		// resv1.PutAndEmit(obj, context.ResourceClient())
		// return false, err
	}

	log.Info("wsk trigger update success")

	// Run feed action (if needed)
	if isFeed {
		log.Info("preparing feed parameters")

		feedparams := make(map[string]interface{})

		for _, kv := range params {
			feedparams[kv.Key] = kv.Value
		}

		// Create additional parameters specific to the feed
		feedparams["authKey"] = wskclient.AuthToken
		feedparams["lifecycleEvent"] = "CREATE"
		feedparams["triggerName"] = "/" + wskclient.Namespace + "/" + wsktrigger.Name

		namespace := wskclient.Namespace
		wskclient.Namespace = feedQName.Namespace

		log.Info("feed info", "namespace", feedQName.Namespace, "name", feedQName.EntityName, "params", feedparams)

		_, resp, err := wskclient.Actions.Invoke(feedQName.EntityName, feedparams, true, false)

		wskclient.Namespace = namespace

		if err != nil {
			log.Error(err, "error creating feed (retrying)", "response", resp)

			// Remove the created trigger

			deleteTrigger(wskclient, obj, wsktrigger.Name, params)
			// if err != nil {
			// 	log.Info("[%s] failed to delete trigger (response: %v) (error: %v) (ignored)", obj.Name, resp, err)
			// }

			return true, err // retrying
		}
	}

	log.Info("deployment done")

	obj.Status.Generation = obj.Generation
	obj.Status.State = resv1.ResourceStateOnline
	obj.Status.Message = time.Now().Format(time.RFC850)

	return false, resv1.PutStatusAndEmit(context, obj)
}

func (r *ReconcileTrigger) finalize(context context.Context, obj *openwhiskv1alpha1.Trigger) (reconcile.Result, error) {
	trigger := obj.Spec
	name := obj.Name
	if trigger.Name != "" {
		name = trigger.Name
	}

	wskclient, err := ow.NewWskClient(context, obj.Spec.ContextFrom)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("[%s] error creating Cloud Function client %v. (Retrying)", obj.Name, err)
	}

	if trigger.Feed == "" {
		if _, _, err := wskclient.Triggers.Delete(name); err != nil {
			if ow.ShouldRetryFinalize(err) {
				return reconcile.Result{}, err
			}
		}
	} else {

		// convert parameters
		params, _, err := ow.ConvertKeyValues(context, obj, trigger.Parameters, "parameters")

		if err != nil {
			// parameters not all resolved... ignore
			params = make(whisk.KeyValueArr, 0)
		}

		deleteTrigger(wskclient, obj, name, params)
	}

	return reconcile.Result{}, resv1.RemoveFinalizerAndPut(context, obj, ow.Finalizer)
}
