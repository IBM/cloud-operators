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

package topic

import (
	"context"
	"reflect"
	"strings"
	"time"

	ibmcloudv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	rcontext "github.com/ibm/cloud-operators/pkg/context"
	binding "github.com/ibm/cloud-operators/pkg/controller/binding"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var logt = logf.Log.WithName("topic")

const topicFinalizer = "topic.ibmcloud.ibm.com"

// ContainsFinalizer checks if the instance contains service finalizer
func ContainsFinalizer(instance *ibmcloudv1alpha1.Topic) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, topicFinalizer) {
			return true
		}
	}
	return false
}

// DeleteFinalizer delete service finalizer
func DeleteFinalizer(instance *ibmcloudv1alpha1.Topic) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == topicFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}

// Add creates a new Topic Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this ibmcloud.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileTopic{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("topic-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Topic
	err = c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.Topic{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create
	// Uncomment watch a Deployment created by Binding - change this for objects you create
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &ibmcloudv1alpha1.Binding{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileTopic{}

// ReconcileTopic reconciles a Topic object
type ReconcileTopic struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Topic object and makes changes based on the state read
// and what is in the Topic.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=topics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=topics/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=topics/finalizers,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileTopic) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := rcontext.New(r.Client, request)

	// Fetch the Topic instance
	instance := &ibmcloudv1alpha1.Topic{}
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
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.BindingStatus{}) {
		instance.Status.State = "Pending"
		instance.Status.Message = "Processing Resource"
		if err := r.Status().Update(context.Background(), instance); err != nil {
			return reconcile.Result{}, nil
		}
	}

	bindingNamespace := instance.Namespace
	if instance.Spec.BindingFrom.Namespace != "" {
		bindingNamespace = instance.Spec.BindingFrom.Namespace
	}
	bindingInstance, err := binding.GetBinding(r, instance.Spec.BindingFrom.Name, bindingNamespace)
	if err != nil {
		if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
			// In this case it is enough to simply remove the finalizer:
			instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
			if err := r.Update(context.Background(), instance); err != nil {
				logt.Info("Error removing finalizers", "in deletion", err.Error())
				// No further action required, object was modified, another reconcile will finish the job.
			}
			return reconcile.Result{}, nil
		}
		logt.Info("Binding not found", instance.Spec.BindingFrom.Name, bindingNamespace)
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
	}

	if err := controllerutil.SetControllerReference(bindingInstance, instance, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	secret, err := binding.GetSecret(r, bindingInstance)
	if err != nil {
		logt.Info("Secret not found", instance.Spec.BindingFrom.Name, bindingNamespace)
		if bindingInstance.Status.State == "Online" {
			return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
		} else {
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}
	}

	kafkaAdminURL, apiKey, err := getKafkaAdminInfo(instance, secret)
	if err != nil {
		logt.Info("Kafka admin URL and/or APIKey not found", instance.Name, err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}

	// Delete if necessary
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Instance is not being deleted, add the finalizer if not present
		if !ContainsFinalizer(instance) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, topicFinalizer)
			if err := r.Update(context.Background(), instance); err != nil {
				logt.Info("Error adding finalizer", instance.Name, err.Error())
				return reconcile.Result{}, nil
			}
		}
	} else {
		// The object is being deleted
		if ContainsFinalizer(instance) {
			result, err := deleteTopic(kafkaAdminURL, apiKey, instance)
			if err != nil {
				logt.Info("Error deleting topic", "in deletion", err.Error())
				return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
			}
			if result.StatusCode == 202 || result.StatusCode == 404 { // deletion succeeded or topic does not exist
				// remove our finalizer from the list and update it.
				instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
				if err := r.Update(context.Background(), instance); err != nil {
					logt.Info("Error removing finalizers", "in deletion", err.Error())
				}
				return reconcile.Result{}, nil
			}
		}
	}

	logt.Info("Getting", "topic", instance.Name)
	result, err := getTopic(kafkaAdminURL, apiKey, instance)
	logt.Info("Result of Get", instance.Name, result)
	if result.StatusCode == 405 && strings.Contains(result.Body, "Method Not Allowed") {
		logt.Info("Trying to create", "topic", instance.Name)
		// This must be a CF Messagehub, does not support GET, test by trying to create
		result, err = createTopic(ctx, kafkaAdminURL, apiKey, instance)
		logt.Info("Creation result", instance.Name, result)
		if result.StatusCode == 200 { // Success
			logt.Info("Topic created", "success", instance.Name)
			return r.updateStatusOnline(instance)
		} else if strings.Contains(result.Body, "already exists") {
			logt.Info("Topic already exists", "success", instance.Name)
			return r.updateStatusOnline(instance)
		}
		if err != nil {
			logt.Info("Topic creation", "error", err.Error())
			return r.updateStatusError(instance, "Failed", err.Error())
		}
		logt.Info("Topic creation error", instance.Name, result.Body)
		return r.updateStatusError(instance, "Failed", result.Body)

	} else if result.StatusCode == 200 {
		// TODO: check that the configuration is the same
		// TODO: status online and return
		return r.updateStatusOnline(instance)

	} else if result.StatusCode == 404 && strings.Contains(result.Body, "unable to get topic details") {
		// Need to create the topic
		result, err = createTopic(ctx, kafkaAdminURL, apiKey, instance)
		if err != nil {
			logt.Info("Topic creation error", instance.Name, err.Error())
			return r.updateStatusError(instance, "Failed", err.Error())
		}
		if result.StatusCode != 200 {
			logt.Info("Topic creation error", instance.Name, result.StatusCode)
			return r.updateStatusError(instance, "Failed", result.Body)
		}
		return r.updateStatusOnline(instance)
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileTopic) updateStatusError(instance *ibmcloudv1alpha1.Topic, state string, message string) (reconcile.Result, error) {
	logt.Info(message)
	if strings.Contains(message, "dial tcp: lookup iam.cloud.ibm.com: no such host") || strings.Contains(message, "dial tcp: lookup login.ng.bluemix.net: no such host") {
		// This means that the IBM Cloud server is under too much pressure, we need to back up
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

func (r *ReconcileTopic) updateStatusOnline(instance *ibmcloudv1alpha1.Topic) (reconcile.Result, error) {
	instance.Status.State = "Online"
	instance.Status.Message = "Online"
	err := r.Status().Update(context.Background(), instance)
	if err != nil {
		logt.Info("Failed to update online status, will delete external resource ", instance.ObjectMeta.Name, err.Error())
		// TODO
		//err = r.deleteCredentials(instance, ibmCloudInfo)
		if err != nil {
			logt.Info("Failed to delete external resource, operator state and external resource might be in an inconsistent state", instance.ObjectMeta.Name, err.Error())
		}
	}

	return reconcile.Result{Requeue: true, RequeueAfter: time.Minute * 30}, nil
}
