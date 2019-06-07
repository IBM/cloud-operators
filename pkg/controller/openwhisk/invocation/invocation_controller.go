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

package invocation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	context "github.com/ibm/cloud-operators/pkg/context"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"

	openwhiskv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	ow "github.com/ibm/cloud-operators/pkg/controller/openwhisk/common"
)

var clog = logf.Log

// Add creates a new Invocation Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileInvocation{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("invocation-controller", mgr, controller.Options{MaxConcurrentReconciles: 256, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Invocation
	err = c.Watch(&source.Kind{Type: &openwhiskv1alpha1.Invocation{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileInvocation{}

// ReconcileInvocation reconciles a Invocation object
type ReconcileInvocation struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Invocation object and makes changes based on the state read
// and what is in the Invocation.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=invocations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=invocations/status,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileInvocation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	context := context.New(r.Client, request)

	// Fetch the Function instance
	invocation := &openwhiskv1alpha1.Invocation{}
	err := r.Get(context, request.NamespacedName, invocation)
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
	if invocation.GetDeletionTimestamp() != nil {
		return r.finalize(context, invocation)
	}

	log := clog.WithValues("namespace", invocation.Namespace, "name", invocation.Name)

	// Check generation
	currentGeneration := invocation.Generation
	syncedGeneration := invocation.Status.Generation
	if currentGeneration != 0 && syncedGeneration >= currentGeneration {
		// condition generation matches object generation. Nothing to do
		log.Info("invocation up-to-date")
		return reconcile.Result{}, nil
	}

	// Check Finalizer is set (but only if has a finalizer function)
	if invocation.Spec.Finalizer != nil {
		if !resv1.HasFinalizer(invocation, ow.Finalizer) {
			invocation.SetFinalizers(append(invocation.GetFinalizers(), ow.Finalizer))

			if err := r.Update(context, invocation); err != nil {
				log.Info("setting finalizer failed. (retrying)", "error", err)
				return reconcile.Result{}, err
			}
		}
	} else {
		if resv1.HasFinalizer(invocation, ow.Finalizer) {
			if err := resv1.RemoveFinalizerAndPut(context, invocation, ow.Finalizer); err != nil {
				log.Info("setting finalizer failed. (retrying)", "error", err)
				return reconcile.Result{}, err
			}
		}
	}

	// Make sure status is Pending
	if err := ow.SetStatusToPending(context, r.Client, invocation, "invoking"); err != nil {
		return reconcile.Result{}, err
	}

	retry, err := r.run(context, invocation)
	if err != nil {
		if !retry {
			log.Error(err, "deployment failed")

			// Non recoverable error.
			invocation.Status.Generation = currentGeneration
			invocation.Status.State = resv1.ResourceStateFailed
			invocation.Status.Message = fmt.Sprintf("%v", err)
			if err := resv1.PutStatusAndEmit(context, invocation); err != nil {
				log.Info("failed to set status. (retrying)", "error", err)
			}
			return reconcile.Result{}, nil
		}
		log.Error(err, "invocation failed (retrying)", "error", err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileInvocation) run(context context.Context, invocation *openwhiskv1alpha1.Invocation) (bool, error) {
	log := clog.WithValues("namespace", invocation.Namespace, "name", invocation.Name)

	log.Info("preparing invocation invocation")

	qualifiedName, err := ow.ParseQualifiedName(invocation.Spec.Function, "_")
	if err != nil {
		return false, err
	}

	// params
	keyValArr, retry, err := ow.ConvertKeyValues(context, invocation, invocation.Spec.Parameters, "parameters")
	if err != nil || retry {
		return retry, err
	}

	// validate jsonpath
	var projection *jsonpath.JSONPath
	if invocation.Spec.To != nil && invocation.Spec.To.Projection != nil {
		projection, err = parseJSONPath("projection", *invocation.Spec.To.Projection)
		if err != nil {
			return false, err // not recoverable
		}
	}

	wskclient, err := ow.NewWskClient(context, invocation.Spec.ContextFrom)
	if err != nil {
		return true, fmt.Errorf("error creating Cloud Function client %v. (retrying)", err)
	}

	parameters := make(map[string]interface{})
	for _, keyVal := range keyValArr {
		parameters[keyVal.Key] = keyVal.Value
	}

	wskclient.Namespace = qualifiedName.Namespace

	log.Info("invoking")
	result, resp, err := wskclient.Actions.Invoke(qualifiedName.EntityName, parameters, true, false)
	// No need to close body response.
	if err != nil {
		if resp.StatusCode == 502 {
			message, err := getApplicationErrorMessage(resp.Body)
			if err != nil {
				log.Info("application error message missing", "error", err)
				return true, fmt.Errorf("error invoking action: %v", err)
			}
			return true, fmt.Errorf("app error: %s", *message)
		}

		return true, err
	}

	if invocation.Spec.To != nil {
		retry, err := r.store(context, invocation, projection, result)
		if err != nil {
			return retry, err
		}
	}

	log.Info("invocation succeeded")

	invocation.Status.Generation = invocation.Generation
	invocation.Status.State = resv1.ResourceStateOnline
	invocation.Status.Message = time.Now().Format(time.RFC850)

	return false, resv1.PutStatusAndEmit(context, invocation)
}

func (r *ReconcileInvocation) store(context context.Context, invocation *openwhiskv1alpha1.Invocation, projection *jsonpath.JSONPath, result map[string]interface{}) (bool, error) {
	log := clog.WithValues("namespace", invocation.Namespace, "name", invocation.Name)

	to := *invocation.Spec.To
	if to.ConfigMapKeyRef == nil && to.SecretKeyRef == nil {
		log.Info("result discarded (to is empty).")
		return false, nil
	}

	var actual []byte
	var err error
	if projection != nil {
		buf := new(bytes.Buffer)
		if err := projection.Execute(buf, result); err != nil {
			return false, err
		}
		actual = buf.Bytes()
	} else {
		response, ok := result["response"]
		if !ok {
			return false, fmt.Errorf("missing response in %v", result)
		}
		actresult, ok := response.(map[string]interface{})["result"]
		if !ok {
			return false, fmt.Errorf("missing result in %v", response)
		}
		actual, err = json.Marshal(actresult)
		if err != nil {
			return false, err
		}
	}

	if to.ConfigMapKeyRef != nil {
		name := to.ConfigMapKeyRef.LocalObjectReference.Name
		key := types.NamespacedName{Namespace: context.Namespace(), Name: name}

		cm := v1.ConfigMap{}
		err := context.Client().Get(context, key, &cm)
		if err != nil {
			if to.ConfigMapKeyRef.Optional != nil && !*to.ConfigMapKeyRef.Optional {
				return false, err
			}
			cm = v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: context.Namespace(),
				},
			}
		}
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data[to.ConfigMapKeyRef.Key] = string(actual)

		if err == nil {
			if err := context.Client().Update(context, &cm); err != nil {
				return true, err
			}
		} else {
			if err := context.Client().Create(context, &cm); err != nil {
				return true, err
			}
		}
	}

	if to.SecretKeyRef != nil {
		name := to.SecretKeyRef.LocalObjectReference.Name
		key := types.NamespacedName{Namespace: context.Namespace(), Name: name}

		secret := v1.Secret{}
		err := context.Client().Get(context, key, &secret)
		if err != nil {
			if to.SecretKeyRef.Optional != nil && !*to.SecretKeyRef.Optional {
				return false, err
			}
			secret = v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: context.Namespace(),
				},
			}
		}
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		secret.Data[to.SecretKeyRef.Key] = actual

		if err == nil {
			if err := context.Client().Update(context, &secret); err != nil {
				return true, err
			}
		} else {
			if err := context.Client().Create(context, &secret); err != nil {
				return true, err
			}
		}
	}

	return false, nil
}

func parseJSONPath(name, template string) (*jsonpath.JSONPath, error) {
	j := jsonpath.New(name)
	if err := j.Parse(template); err != nil {
		return nil, err
	}
	return j, nil
}

func getApplicationErrorMessage(body io.Reader) (*string, error) {
	bytes, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, err
	}

	var jr interface{}
	err = json.Unmarshal(bytes, &jr)
	if err != nil {
		return nil, err
	}

	response, ok := jr.(map[string]interface{})["response"]
	if !ok {
		return nil, fmt.Errorf("missing response in %v", jr)
	}
	result, ok := response.(map[string]interface{})["result"]
	if !ok {
		return nil, fmt.Errorf("missing result in %v", response)
	}
	errorf, ok := result.(map[string]interface{})["error"]
	if !ok {
		return nil, fmt.Errorf("missing error in %v", result)
	}
	errormap, ok := errorf.(map[string]interface{})
	if ok {
		if message, ok := errormap["message"]; ok {
			str := message.(string)
			return &str, nil
		}
	}

	bytes, err = json.Marshal(errorf)
	if err != nil {
		return nil, fmt.Errorf("internal error %v", err)
	}

	str := string(bytes)
	return &str, nil
}

func (r *ReconcileInvocation) finalize(context context.Context, invocation *openwhiskv1alpha1.Invocation) (reconcile.Result, error) {
	log := clog.WithValues("namespace", invocation.Namespace, "name", invocation.Name)

	if invocation.Spec.Finalizer == nil {
		return reconcile.Result{}, nil
	}
	finalizer := *invocation.Spec.Finalizer

	qualifiedName, err := ow.ParseQualifiedName(finalizer.Function, "_")
	if err != nil {
		// TODO: set status
		// Not recoverable
		log.Error(err, "invalid finalizer function name")
		return reconcile.Result{}, nil
	}

	// params
	keyValArr, retry, err := ow.ConvertKeyValues(context, invocation, finalizer.Parameters, "parameters")
	if err != nil || retry {
		log.Error(err, "invalid parameters function name")
		return reconcile.Result{Requeue: retry}, nil
	}

	wskclient, err := ow.NewWskClient(context, invocation.Spec.ContextFrom)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error creating Cloud Function client %v. (retrying)", err)
	}

	parameters := make(map[string]interface{})
	for _, keyVal := range keyValArr {
		parameters[keyVal.Key] = keyVal.Value
	}

	wskclient.Namespace = qualifiedName.Namespace

	log.Info("invoking finalizer")
	_, resp, err := wskclient.Actions.Invoke(qualifiedName.EntityName, parameters, true, true)

	if err != nil {
		if resp.StatusCode == 502 {
			message, err := getApplicationErrorMessage(resp.Body)
			if err != nil {
				log.Info("application error message missing", "error", err)
				return reconcile.Result{}, fmt.Errorf("error invoking action: %v", err)
			}
			return reconcile.Result{}, fmt.Errorf("app error: %s", *message)
		}

		return reconcile.Result{}, err // retry
	}

	return reconcile.Result{}, resv1.RemoveFinalizerAndPut(context, invocation, ow.Finalizer)
}
