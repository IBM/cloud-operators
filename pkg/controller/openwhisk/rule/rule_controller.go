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

package rule

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

// Add creates a new Rule Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRule{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("rule-controller", mgr, controller.Options{MaxConcurrentReconciles: 32, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Function
	err = c.Watch(&source.Kind{Type: &openwhiskv1alpha1.Rule{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileRule{}

// ReconcileRule reconciles a Rule object
type ReconcileRule struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Rule object and makes changes based on the state read
// and what is in the Rule.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=rules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=rules/status,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileRule) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	context := context.New(r.Client, request)

	// Fetch the Function instance
	rule := &openwhiskv1alpha1.Rule{}
	err := r.Get(context, request.NamespacedName, rule)
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
	if rule.GetDeletionTimestamp() != nil {
		return r.finalize(context, rule)
	}

	log := clog.WithValues("namespace", rule.Namespace, "name", rule.Name)

	// Check generation
	currentGeneration := rule.Generation
	syncedGeneration := rule.Status.Generation
	if currentGeneration != 0 && syncedGeneration >= currentGeneration {
		// condition generation matches object generation. Nothing to do
		log.Info("function up-to-date")
		return reconcile.Result{}, nil
	}

	// Check Finalizer is set
	if !resv1.HasFinalizer(rule, ow.Finalizer) {
		rule.SetFinalizers(append(rule.GetFinalizers(), ow.Finalizer))

		if err := r.Update(context, rule); err != nil {
			log.Info("setting finalizer failed. (retrying)", "error", err)
			return reconcile.Result{}, err
		}
	}

	// Make sure status is Pending
	if err := ow.SetStatusToPending(context, r.Client, rule, "deploying"); err != nil {
		return reconcile.Result{}, err
	}

	retry, err := r.updateRule(context, rule)
	if err != nil {
		if !retry {
			log.Error(err, "deployment failed")

			// Non recoverable error.
			rule.Status.Generation = currentGeneration
			rule.Status.State = resv1.ResourceStateFailed
			rule.Status.Message = fmt.Sprintf("%v", err)
			if err := resv1.PutStatusAndEmit(context, rule); err != nil {
				log.Info("failed to set status. (retrying)", "error", err)
			}
			return reconcile.Result{}, nil
		}
		log.Error(err, "deployment failed (retrying)", "error", err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileRule) updateRule(context context.Context, obj *openwhiskv1alpha1.Rule) (bool, error) {
	log := clog.WithValues("namespace", obj.Namespace, "name", obj.Name)

	rule := obj.Spec

	wskrule := new(whisk.Rule)
	wskrule.Name = obj.Name

	if rule.Name != "" {
		wskrule.Name = rule.Name
	}

	log.Info("deploying rule")

	pub := false
	wskrule.Publish = &pub

	triggerQName, err := ow.ParseQualifiedName(rule.Trigger, "_")
	if err != nil {
		resv1.SetStatus(obj, resv1.ResourceStateFailed, "Malformed trigger name: %s", rule.Trigger)
		return false, err
	}

	wskrule.Trigger = fmt.Sprintf("/%s/%s", triggerQName.Namespace, triggerQName.EntityName)

	actionQName, err := ow.ParseQualifiedName(rule.Function, "_")
	if err != nil {
		resv1.SetStatus(obj, resv1.ResourceStateFailed, "Malformed rule action name: %s", rule.Function)
		return false, nil
	}
	wskrule.Action = fmt.Sprintf("/%s/%s", actionQName.Namespace, actionQName.EntityName)

	log.Info("acquiring OpenWhisk credentials")

	wskclient, err := ow.NewWskClient(context, obj.Spec.ContextFrom)
	if err != nil {
		return true, fmt.Errorf("Error creating Cloud Function client %v. (Retrying)", err)
	}

	log.Info("calling wsk rule update")

	_, resp, err := wskclient.Rules.Insert(wskrule, true)

	if err != nil {
		log.Info("[%s] wsk rule update response: %v", obj.Name, resp)
		log.Info("[%s] wsk rule update failed: %v (Retyring)", obj.Name, err)

		// if ow.ShouldRetry(context, resp, err) {
		return true, err
		// }

		// return false, fmt.Errorf("Error deploying rule: %v", err)
	}

	log.Info("deployment done")

	obj.Status.Generation = obj.Generation
	obj.Status.State = resv1.ResourceStateOnline
	obj.Status.Message = time.Now().Format(time.RFC850)

	return false, resv1.PutStatusAndEmit(context, obj)
}

func (r *ReconcileRule) finalize(context context.Context, obj *openwhiskv1alpha1.Rule) (reconcile.Result, error) {
	rule := obj.Spec
	name := obj.Name
	if rule.Name != "" {
		name = rule.Name
	}

	wskclient, err := ow.NewWskClient(context, obj.Spec.ContextFrom)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("Error creating Cloud Function client %v. (retrying)", err)
	}

	if _, err := wskclient.Rules.Delete(name); err != nil {
		if ow.ShouldRetryFinalize(err) {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, resv1.RemoveFinalizerAndPut(context, obj, ow.Finalizer)
}
