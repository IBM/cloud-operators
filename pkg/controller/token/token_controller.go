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

package token

import (
	"context"
	"net/http"
	"strings"
	"time"

	bx "github.com/IBM-Cloud/bluemix-go"
	bxauth "github.com/IBM-Cloud/bluemix-go/authentication"
	bxendpoints "github.com/IBM-Cloud/bluemix-go/endpoints"
	bxrest "github.com/IBM-Cloud/bluemix-go/rest"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var logt = log.Log.WithName("iam-token")

// Add creates a new Token Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileToken{Client: mgr.GetClient(), scheme: mgr.GetScheme(), httpClient: http.DefaultClient}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("token-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Token
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return labelFilter(e.Meta.GetLabels()) },
		UpdateFunc: func(e event.UpdateEvent) bool { return labelFilter(e.MetaNew.GetLabels()) },
		DeleteFunc: func(e event.DeleteEvent) bool { return false },
	})
	if err != nil {
		return err
	}

	return nil
}

func labelFilter(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	value, ok := labels["seed.ibm.com/ibmcloud-token"]
	if !ok {
		return false
	}
	return value == "apikey"
}

var _ reconcile.Reconciler = &ReconcileToken{}

// ReconcileToken reconciles a Token object
type ReconcileToken struct {
	client.Client
	scheme     *runtime.Scheme
	httpClient *http.Client
}

// Reconcile computes IAM and UAA tokens
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch
func (r *ReconcileToken) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logt.Info("reconciling IBM cloud IAM tokens", "secretRef", request.Name)
	context := context.Background()

	secret := &corev1.Secret{}
	err := r.Get(context, request.NamespacedName, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			logt.Info("object not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logt.Info("object cannot be read", "error", err)
		return reconcile.Result{}, err
	}

	if secret.DeletionTimestamp != nil {
		// Secret is being deleted... nothing to do.
		return reconcile.Result{}, nil
	}

	apikeyb, ok := secret.Data["api-key"]
	if !ok {
		logt.Info("missing api-key key in secret", "Namespace", secret.Namespace, "Name", secret.Name)
		return reconcile.Result{}, nil
	}

	regionb, ok := secret.Data["region"]
	if !ok {
		logt.Info("set default region to us-south")
		regionb = []byte("us-south")
	}
	region := string(regionb)

	config := bx.Config{
		EndpointLocator: bxendpoints.NewEndpointLocator(region),
	}

	auth, err := bxauth.NewIAMAuthRepository(&config, &bxrest.Client{HTTPClient: r.httpClient})
	if err != nil {
		// Invalid region. Do not requeue
		logt.Info("no endpoint found for region", "region", region)
		return reconcile.Result{}, nil
	}

	logt.Info("authenticating...")
	if err := auth.AuthenticateAPIKey(string(apikeyb)); err != nil {
		// TODO: check BX Error
		logt.Info("authentication failed", "error", err)
		return reconcile.Result{}, err // requeue
	}
	tokensRef := secret.Name + "-tokens"
	logt.Info("creating tokens secret", "name", tokensRef)

	tokens := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tokensRef,
			Namespace: secret.Namespace,
		},
		Data: map[string][]byte{
			"access_token":      []byte(config.IAMAccessToken),
			"refresh_token":     []byte(config.IAMRefreshToken),
			"uaa_token":         []byte(strings.Replace(config.UAAAccessToken, "B", "b", 1)),
			"uaa_refresh_token": []byte(config.UAARefreshToken),
		},
	}

	r.Delete(context, tokens)

	if err := r.Create(context, tokens); err != nil {
		logt.Error(err, "failed to update secret (retrying)")
		return reconcile.Result{}, err
	}
	logt.Info("secret created", "name", tokensRef)
	return reconcile.Result{RequeueAfter: 10 * time.Minute}, nil
}
