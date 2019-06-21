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

package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"

	rcontext "github.com/ibm/cloud-operators/pkg/context"
)

var slog = log.Log

// EnsureFinalizerAndPut makes sure obj contains finalizer. If not update obj and server
func EnsureFinalizerAndPut(ctx rcontext.Context, client client.Client, obj runtime.Object, finalizer string) error {
	meta := ObjectMeta(obj)
	log := slog.WithValues("Namespace", meta.GetNamespace(), "Name", meta.GetName())

	if !HasFinalizer(obj, finalizer) {
		addFinalizer(obj, finalizer)
		if err := client.Update(ctx, obj); err != nil {
			log.Info("error setting finalizer", "error", err)
			return err
		}
	}
	return nil
}

// RemoveFinalizerAndPut removes finalizer from obj (if present). Update obj and server when needed
func RemoveFinalizerAndPut(ctx rcontext.Context, obj runtime.Object, finalizer string) error {
	meta := ObjectMeta(obj)
	log := slog.WithValues("Namespace", meta.GetNamespace(), "Name", meta.GetName())

	if HasFinalizer(obj, finalizer) {
		RemoveFinalizer(obj, finalizer)
		if err := ctx.Client().Update(ctx, obj); err != nil {
			log.Info("error setting finalizer", "error", err)
			return err
		}
	}
	return nil
}
