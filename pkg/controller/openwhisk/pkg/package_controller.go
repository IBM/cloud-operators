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

package pkg

import (
	"encoding/json"
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
	"github.com/ibm/cloud-operators/pkg/lib/secret"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"

	openwhiskv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	ow "github.com/ibm/cloud-operators/pkg/controller/openwhisk/common"
)

var clog = logf.Log

// Add creates a new Package Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePackage{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("package-controller", mgr, controller.Options{MaxConcurrentReconciles: 32, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Package
	err = c.Watch(&source.Kind{Type: &openwhiskv1alpha1.Package{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePackage{}

// ReconcilePackage reconciles a Package object
type ReconcilePackage struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Package object and makes changes based on the state read
// and what is in the Package.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=openwhisk.seed.ibm.com,resources=packages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openwhisk.seed.ibm.com,resources=packages/status,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcilePackage) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	context := context.New(r.Client, request)

	// Fetch the Package instance
	pkg := &openwhiskv1alpha1.Package{}
	err := r.Get(context, request.NamespacedName, pkg)
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
	if pkg.GetDeletionTimestamp() != nil {
		return r.finalize(context, pkg)
	}

	log := clog.WithValues("namespace", pkg.Namespace, "name", pkg.Name)

	// Check generation
	currentGeneration := pkg.Generation
	syncedGeneration := pkg.Status.Generation
	if currentGeneration != 0 && syncedGeneration >= currentGeneration {
		// condition generation matches object generation. Nothing to do
		log.Info("package up-to-date")
		return reconcile.Result{}, nil
	}

	// Check Finalizer is set
	if !resv1.HasFinalizer(pkg, ow.Finalizer) {
		pkg.SetFinalizers(append(pkg.GetFinalizers(), ow.Finalizer))

		if err := r.Update(context, pkg); err != nil {
			log.Info("setting finalizer failed. (retrying)", "error", err)
			return reconcile.Result{}, err
		}
	}

	// Make sure status is Pending
	if err := ow.SetStatusToPending(context, r.Client, pkg, "deploying"); err != nil {
		return reconcile.Result{}, err
	}

	retry, err := r.updatePackage(context, pkg)
	if err != nil {
		if !retry {
			log.Error(err, "deployment failed")

			// Non recoverable error.
			pkg.Status.Generation = currentGeneration
			pkg.Status.State = resv1.ResourceStateFailed
			pkg.Status.Message = fmt.Sprintf("%v", err)
			if err := resv1.PutStatusAndEmit(context, pkg); err != nil {
				log.Info("failed to set status. (retrying)", "error", err)
			}
			return reconcile.Result{}, nil
		}
		log.Error(err, "deployment failed (retrying)", "error", err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcilePackage) updatePackage(context context.Context, obj *openwhiskv1alpha1.Package) (bool, error) {
	log := clog.WithValues("namespace", obj.Namespace, "name", obj.Name)

	pkg := obj.Spec
	if pkg.Service != "" || pkg.Bind != "" {
		return r.updateBinding(context, obj)
	}

	wpkg := &whisk.Package{}
	wpkg.Name = obj.Name
	if pkg.Name != "" {
		wpkg.Name = pkg.Name
	}

	log.Info("preparing package")

	if wpkg.Name != "default" {
		wpkg.Namespace = "_"
		wpkg.Publish = pkg.Publish

		// parametersFrom
		paramsFromArr, retry, err := ow.ConvertParametersFrom(context, obj, pkg.ParametersFrom)
		if err != nil || retry {
			return retry, err
		}

		if len(paramsFromArr) > 0 {
			wpkg.Parameters = paramsFromArr
		}

		// params
		keyValArr, retry, err := ow.ConvertKeyValues(context, obj, pkg.Parameters, "parameters")
		if err != nil || retry {
			return retry, err
		}

		// if we have successfully parser valid key/value parameters
		if len(keyValArr) > 0 {
			wpkg.Parameters = append(wpkg.Parameters, keyValArr...)
		}

		// annotations
		keyValArr, retry, err = ow.ConvertKeyValues(context, obj, pkg.Annotations, "annotations")
		if err != nil || retry {
			return retry, err
		}

		// if we have successfully parser valid key/value parameters
		if len(keyValArr) > 0 {
			wpkg.Annotations = keyValArr
		}

		wskclient, err := ow.NewWskClient(context, obj.Spec.ContextFrom)
		if err != nil {
			return true, fmt.Errorf("error creating Cloud Function client %v. (Retrying)", err)
		}

		_, resp, err := wskclient.Packages.Insert(wpkg, true)

		if err != nil {
			log.Error(err, "package creation failed")
			if ow.ShouldRetry(resp, err) {
				return true, err
			}

			return false, fmt.Errorf("error deploying package: %v", err)
		}

		log.Info("package created")
	}

	obj.Status.Generation = obj.Generation
	obj.Status.State = resv1.ResourceStateOnline
	obj.Status.Message = time.Now().Format(time.RFC850)

	return false, resv1.PutStatusAndEmit(context, obj)
}

func (r *ReconcilePackage) updateBinding(context context.Context, obj *openwhiskv1alpha1.Package) (bool, error) {
	log := clog.WithValues("namespace", obj.Namespace, "name", obj.Name)

	pkg := obj.Spec

	wpkg := &whisk.BindingPackage{}
	wpkg.Name = obj.Name
	if pkg.Name != "" {
		wpkg.Name = pkg.Name
	}
	wpkg.Namespace = "_"

	log.Info("preparing package binding")

	// TODO: issue #335
	qName, err := ow.ParseQualifiedName(pkg.Bind, "_")
	if err != nil {
		return false, fmt.Errorf("invalid binding name %s", pkg.Bind)
	}

	wpkg.Binding = whisk.Binding{Namespace: qName.Namespace, Name: qName.EntityName}
	wpkg.Publish = pkg.Publish

	// --- Parameters

	if pkg.Service != "" {
		keys, err := getServiceKeys(context, pkg.Service)
		if err != nil {
			return true, fmt.Errorf("error getting service keys: %v. (Retrying)", err)
		}
		keyValArr, err := ow.ToKeyValueArrFromMap(keys)
		if err != nil {
			return true, nil
		}
		if len(keyValArr) > 0 {
			wpkg.Parameters = keyValArr
		}
	}

	// Additional parameters
	keyValArr, retry, err := ow.ConvertKeyValues(context, obj, pkg.Parameters, "parameters")
	if err != nil || retry {
		log.Info("error converting parameters", "error", err)
		return retry, err
	}

	// if we have successfully parser valid key/value parameters
	if len(keyValArr) > 0 {
		wpkg.Parameters = append(wpkg.Parameters, keyValArr...)
	}

	// annotations
	keyValArr, retry, err = ow.ConvertKeyValues(context, obj, pkg.Annotations, "annotations")
	if err != nil || retry {
		return retry, err
	}

	// if we have successfully parser valid key/value parameters
	if len(keyValArr) > 0 {
		wpkg.Annotations = keyValArr
	}

	wskclient, err := ow.NewWskClient(context, obj.Spec.ContextFrom)
	if err != nil {
		return true, fmt.Errorf("error creating Cloud Function client %v. (Retrying)", err)
	}
	// Deploy package binding
	_, resp, err := wskclient.Packages.Insert(wpkg, true)

	if err != nil {
		if ow.ShouldRetry(resp, err) {
			return true, err
		}

		return false, fmt.Errorf("error deploying package: %v", err)
	}

	obj.Status.Generation = obj.Generation
	obj.Status.State = resv1.ResourceStateOnline
	obj.Status.Message = time.Now().Format(time.RFC850)

	return false, resv1.PutStatusAndEmit(context, obj)
}

func getServiceKeys(context context.Context, serviceName string) (interface{}, error) {
	secretName := fmt.Sprintf("binding-%s", serviceName)
	value, err := secret.GetSecret(context, secretName, true)
	if err != nil {
		return nil, err
	}

	bnd := value.Data["binding"]
	var js interface{}
	err = json.Unmarshal(bnd, &js)
	return js, err
}

func (r *ReconcilePackage) finalize(context context.Context, obj *openwhiskv1alpha1.Package) (reconcile.Result, error) {
	pkg := obj.Spec
	name := obj.Name
	if pkg.Name != "" {
		name = pkg.Name
	}

	wskclient, err := ow.NewWskClient(context, obj.Spec.ContextFrom)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("Error creating Cloud Function client %v. (Retrying)", err)
	}

	if _, err := wskclient.Packages.Delete(name); err != nil {
		if ow.ShouldRetryFinalize(err) {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, resv1.RemoveFinalizerAndPut(context, obj, ow.Finalizer)
}
