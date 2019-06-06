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

package auth

import (
	"strings"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	context "github.com/ibm/cloud-operators/pkg/context"
	"github.com/ibm/cloud-operators/pkg/lib/secret"
)

var clog = logf.Log

// SecretSuffix is the suffix appended to the original configmap name
const SecretSuffix = "-owprops"

// UAAConfig is a struct for parsing the Cloud Foundry config.json
type UAAConfig struct {
	Target       string
	AccessToken  string
	RefreshToken string
}

// Add creates a new Auth Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileAuth{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("auth-controller", mgr, controller.Options{MaxConcurrentReconciles: 4, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to configmap
	err = c.Watch(&source.Kind{Type: &v1.ConfigMap{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileAuth{}

// ReconcileAuth reconciles a Auth object
type ReconcileAuth struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Auth object and makes changes based on the state read
// and what is in the Auth.Spec
// +kubebuilder:rbac:groups=,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=events,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileAuth) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	context := context.New(r.Client, request)

	// Fetch the Function instance
	cm := &v1.ConfigMap{}
	err := r.Get(context, request.NamespacedName, cm)
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
	if cm.GetDeletionTimestamp() != nil {
		return reconcile.Result{}, nil
	}

	log := clog.WithValues("namespace", cm.Namespace, "name", cm.Name)

	if !isIBMCloudContext(cm) {
		return reconcile.Result{}, nil
	}
	secretName := cm.Name + SecretSuffix

	_, err = secret.GetSecret(context, secretName, true)
	if err != nil {
		log.Info("creating secret", "name", secretName)

		apikeySecret, err := secret.GetSecret(context, "seed-secret-tokens", true) // TODO: generalize
		if err != nil {
			return reconcile.Result{}, err
		}

		accessToken := string(apikeySecret.Data["uaa_token"])
		refreshToken := string(apikeySecret.Data["uaa_refresh_token"])

		log.Info("retrieving openwhisk auth")

		ns, _, err := AuthenticateUserWithWsk("openwhisk.ng.bluemix.net", strings.TrimPrefix(string(accessToken), "bearer "), string(refreshToken), false)
		if err != nil {
			return reconcile.Result{}, err // retry
		}

		org := cm.Data["org"]
		space := cm.Data["space"]

		auth, err := FindAuthKey(ns, org, space)
		if err != nil {
			return reconcile.Result{}, err
		}
		log.Info("openwhisk key found")

		if err := r.Create(context, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cm.Namespace,
				Name:      secretName,
			},
			Data: map[string][]byte{
				"apihost": []byte("openwhisk.ng.bluemix.net"), // TODO
				"auth":    []byte(auth),
			},
		}); err != nil {
			return reconcile.Result{}, err
		}

	}

	return reconcile.Result{}, nil
}

func isIBMCloudContext(cm *v1.ConfigMap) bool {
	// TODO: use label!

	_, hasOrg := cm.Data["org"]
	_, hasSpace := cm.Data["space"]
	_, hasRegion := cm.Data["region"]
	return hasOrg && hasSpace && hasRegion

}
