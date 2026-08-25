/*
 * Copyright 2020 IBM Corporation
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

package controllers

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/ibm/cloud-operators/internal/ibmcloud/auth"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	icoSecretName = "ibmcloud-operator-secret"
	icoTokensName = "ibmcloud-operator-tokens"
)

// TokenReconciler reconciles a Token object
type TokenReconciler struct {
	client.Client
	Log          logr.Logger
	Scheme       *runtime.Scheme
	Authenticate auth.Authenticator
}

// Reconcile computes IAM and UAA tokens
func (r *TokenReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logt := r.Log.WithValues("token", request.NamespacedName)

	logt.Info("reconciling IBM cloud IAM tokens", "secretRef", request.Name)

	secret := &corev1.Secret{}
	err := r.Get(ctx, request.NamespacedName, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			logt.Info("object not found")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logt.Info("object cannot be read", "error", err)
		return ctrl.Result{}, err
	}

	if secret.DeletionTimestamp != nil {
		// Secret is being deleted... nothing to do.
		return ctrl.Result{}, nil
	}

	apikeyb, ok := secret.Data["api-key"]
	if !ok {
		logt.Info("missing api-key key in secret", "Namespace", secret.Namespace, "Name", secret.Name)
		return ctrl.Result{}, nil
	}

	regionb, ok := secret.Data["region"]
	if !ok {
		logt.Info("set default region to us-south")
		regionb = []byte("us-south")
	}
	region := string(regionb)

	logt.Info("authenticating...")
	creds, err := r.Authenticate(string(apikeyb), string(regionb))
	if _, ok := err.(auth.InvalidConfigError); ok {
		// Invalid region. Do not requeue
		logt.Error(err, "failed to create auth client", "region", region)
		return ctrl.Result{}, nil
	}
	if err != nil {
		// TODO: check BX Error
		logt.Error(err, "authentication failed")
		return ctrl.Result{}, err // requeue
	}

	tokensRef := strings.TrimSuffix(secret.Name, icoSecretName) + icoTokensName // need to trim suffix, since management namespace could be the prefix
	logt.Info("creating tokens secret", "name", tokensRef)

	tokens := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tokensRef,
			Namespace: secret.Namespace,
		},
		Data: creds.MarshalSecret(),
	}

	// TODO(johnstarich) switch to CreateOrUpdate for atomic replace behavior
	err = r.Delete(ctx, tokens)
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, tokens); err != nil {
		logt.Error(err, "failed to update secret (retrying)")
		return ctrl.Result{}, err
	}
	logt.Info("secret created", "name", tokensRef)
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

func (r *TokenReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&corev1.Secret{}).
		WithEventFilter(eventsFilter()).
		Complete(r)
}

func eventsFilter() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return shouldProcessSecret(e.Object) },
		DeleteFunc: func(e event.DeleteEvent) bool { return shouldProcessSecret(e.Object) },
		UpdateFunc: func(e event.UpdateEvent) bool { return shouldProcessSecret(e.ObjectNew) },
	}
}

func shouldProcessSecret(meta metav1.Object) bool {
	return meta.GetName() == icoSecretName || strings.HasSuffix(meta.GetName(), "-"+icoSecretName)
}
