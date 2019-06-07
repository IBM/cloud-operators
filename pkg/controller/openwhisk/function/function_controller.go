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

package function

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"path"
	"strings"
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
	ic "github.com/ibm/cloud-operators/pkg/lib/ibmcloud"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"

	openwhiskv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	ow "github.com/ibm/cloud-operators/pkg/controller/openwhisk/common"
)

var clog = logf.Log

// Add creates a new Function Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileFunction{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("function-controller", mgr, controller.Options{MaxConcurrentReconciles: 32, Reconciler: r})

	if err != nil {
		return err
	}

	// Watch for changes to Function
	err = c.Watch(&source.Kind{Type: &openwhiskv1alpha1.Function{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileFunction{}

// ReconcileFunction reconciles a Function object
type ReconcileFunction struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Function object and makes changes based on the state read
// and what is in the Function.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=functions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=functions/status,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileFunction) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	context := context.New(r.Client, request)

	// Fetch the Function instance
	function := &openwhiskv1alpha1.Function{}
	err := r.Get(context, request.NamespacedName, function)
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
	if function.GetDeletionTimestamp() != nil {
		return r.finalize(context, function)
	}

	log := clog.WithValues("namespace", function.Namespace, "name", function.Name)

	// Check generation
	currentGeneration := function.Generation
	syncedGeneration := function.Status.Generation
	if currentGeneration != 0 && syncedGeneration >= currentGeneration {
		// condition generation matches object generation. Nothing to do
		log.Info("function up-to-date")
		return reconcile.Result{}, nil
	}

	// Check Finalizer is set
	if !resv1.HasFinalizer(function, ow.Finalizer) {
		function.SetFinalizers(append(function.GetFinalizers(), ow.Finalizer))

		if err := r.Update(context, function); err != nil {
			log.Info("setting finalizer failed. (retrying)", "error", err)
			return reconcile.Result{}, err
		}
	}

	// Make sure status is Pending
	if err := ow.SetStatusToPending(context, r.Client, function, "deploying"); err != nil {
		return reconcile.Result{}, err
	}

	retry, err := r.updateAction(context, function)
	if err != nil {
		if !retry {
			log.Error(err, "deployment failed")

			// Non recoverable error.
			function.Status.Generation = currentGeneration
			function.Status.State = resv1.ResourceStateFailed
			function.Status.Message = fmt.Sprintf("%v", err)
			if err := resv1.PutStatusAndEmit(context, function); err != nil {
				log.Info("failed to set status. (retrying)", "error", err)
			}
			return reconcile.Result{}, nil
		}
		log.Error(err, "deployment failed (retrying)", "error", err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileFunction) updateAction(context context.Context, obj *openwhiskv1alpha1.Function) (bool, error) {
	log := clog.WithValues("namespace", obj.Namespace, "name", obj.Name)

	// Can now reconcile!
	action := obj.Spec

	name := ow.ResolveFunctionName(obj.Name, action.Package, action.Name)
	log.Info("deploying action")

	log.V(3).Info("acquiring OpenWhisk credentials")
	wskclient, err := ow.NewWskClient(context, obj.Spec.ContextFrom)
	if err != nil {
		return true, fmt.Errorf("error creating Cloud Function client %v. (retrying)", err)
	}

	if action.Package != nil && *action.Package != "default" {
		// Check package exist
		_, response, err := wskclient.Packages.Get(*action.Package)
		if err != nil || response.StatusCode != 200 {
			return true, fmt.Errorf("package %s not found. (retrying)", *action.Package)
		}
	}

	log.V(3).Info("preparing action from spec")
	runtime := action.Runtime

	wskaction := new(whisk.Action)
	wskaction.Exec = new(whisk.Exec)
	wskaction.Exec.Kind = runtime
	wskaction.Name = name

	if action.Main != nil {
		wskaction.Exec.Main = *action.Main
	}

	if runtime == "sequence" && action.Parameters != nil {
		return false, fmt.Errorf("unexpected parameters when runtime is sequence")
	}

	// params
	keyValArr, retry, err := ow.ConvertKeyValues(context, obj, action.Parameters, "parameters")
	if err != nil || retry {
		return retry, err
	}

	// if we have successfully parser valid key/value parameters
	if len(keyValArr) > 0 {
		wskaction.Parameters = keyValArr
	}

	// annotations
	keyValArr, retry, err = ow.ConvertKeyValues(context, obj, action.Annotations, "annotations")
	if err != nil || retry {
		return retry, err
	}

	// webExport and rawHTTP. Silently override matching annotations
	if action.WebExport || action.RawHTTP {
		annot := "yes"
		if action.RawHTTP {
			annot = "raw"
		}
		keyValArr, _ = ow.WebAction(annot, keyValArr, false)
	}

	// if we have successfully parser valid key/value parameters
	if len(keyValArr) > 0 {
		wskaction.Annotations = keyValArr
	}

	// Action.Limits
	limits := action.Limits
	if limits != nil && (limits.LogSize != 0 || limits.Memory != 0 || limits.Timeout != 0) {
		wsklimits := new(whisk.Limits)
		if limits.Timeout != 0 {
			wsklimits.Timeout = &limits.Timeout
		}
		if limits.Memory != 0 {
			wsklimits.Memory = &limits.Memory
		}
		if limits.LogSize != 0 {
			wsklimits.Logsize = &limits.LogSize
		}
		wskaction.Limits = wsklimits
	}

	if runtime == "sequence" {
		if action.CodeURI != nil || action.Code != nil || action.Docker != "" || action.Native {
			return false, fmt.Errorf("unexpected code, codeURI, docker or native property when runtime is sequence")
		}

		if action.Functions == nil {
			return false, fmt.Errorf("missing functions property")
		}

		actions := strings.Split(*action.Functions, ",")
		components := make([]string, len(actions))
		for i, actionName := range actions {
			actionName = strings.TrimSpace(actionName)

			qname, err := ow.ParseQualifiedName(actionName, "_")
			if err != nil {
				return false, fmt.Errorf("Malformed action name: %s", actionName)
			}

			components[i] = fmt.Sprintf("/%s/%s", qname.Namespace, qname.EntityName)
			log.V(5).Info("adding component %s to function %s", components[i], wskaction.Name)
		}

		wskaction.Exec.Components = components
	} else {
		if action.CodeURI != nil {
			if *action.CodeURI == "" {
				return false, fmt.Errorf("codeURI is empty")
			}

			if action.Code != nil {
				return false, fmt.Errorf("codeURI and code are mutually exclusive")
			}

			// TODO: set status
			log.Info("downloading code", "URI", *action.CodeURI)

			dat, erRead := ic.Read(context, *action.CodeURI)
			if erRead != nil {
				return false, fmt.Errorf("Error reading %s : %v", *action.CodeURI, erRead)
			}

			code := string(dat)
			ext := path.Ext(*action.CodeURI)
			if ext == ".zip" || ext == ".jar" {
				code = base64.StdEncoding.EncodeToString([]byte(dat))
			}
			wskaction.Exec.Code = &code
		} else if action.Code != nil {
			if *action.Code == "" {
				return false, fmt.Errorf("code is empty")
			}

			if action.Docker != "" || action.Native {
				if action.Code, err = zipCode(*action.Code); err != nil {
					// something's wrong with the spec. Don't retry
					return false, err
				}
			}

			wskaction.Exec.Code = action.Code
		} else {
			return false, fmt.Errorf("missing codeURI or code")
		}

		if action.Docker != "" || action.Native {
			wskaction.Exec.Kind = "blackbox"
			if action.Native {
				wskaction.Exec.Image = "openwhisk/dockerskeleton"
			} else {
				wskaction.Exec.Image = action.Docker
			}
		}
	}

	log.Info("calling wsk action update")

	_, resp, err := wskclient.Actions.Insert(wskaction, true)

	if err != nil {
		if ow.ShouldRetry(resp, err) {
			return true, err
		}

		return false, fmt.Errorf("Error deploying action: %v", err)
	}

	log.Info("deployment done")

	obj.Status.Generation = obj.Generation
	obj.Status.State = resv1.ResourceStateOnline
	obj.Status.Message = time.Now().Format(time.RFC850)

	return false, resv1.PutStatusAndEmit(context, obj)
}

func (r *ReconcileFunction) finalize(context context.Context, obj *openwhiskv1alpha1.Function) (reconcile.Result, error) {
	log := clog.WithValues("namespace", obj.Namespace, "name", obj.Name)
	log.Info("finalizing")

	name := ow.ResolveFunctionName(obj.Name, obj.Spec.Package, obj.Spec.Name)

	wskclient, err := ow.NewWskClient(context, obj.Spec.ContextFrom)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("Error creating Cloud Function client %v. (retrying)", err)
	}

	if _, err := wskclient.Actions.Delete(name); err != nil {
		if ow.ShouldRetryFinalize(err) {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, resv1.RemoveFinalizerAndPut(context, obj, ow.Finalizer)
}

func zipCode(code string) (*string, error) {
	// Create a buffer to write our archive to.
	buf := new(bytes.Buffer)

	// Create a new zip archive.
	w := zip.NewWriter(buf)

	f, err := w.Create("exec")
	if err != nil {
		return nil, err
	}

	_, err = f.Write([]byte(code))
	if err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return &encoded, nil
}
