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
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// EnsureFinalizer makes sure the given object has the given finalizer.
// Return true if finalizer has been added
func EnsureFinalizer(obj runtime.Object, name string) bool {
	if HasFinalizer(obj, name) {
		return false
	}
	addFinalizer(obj, name)
	return true
}

func addFinalizer(obj runtime.Object, name string) runtime.Object {
	ObjectMeta(obj).SetFinalizers(append(ObjectMeta(obj).GetFinalizers(), name))
	return obj
}

// RemoveFinalizer clears the given finalizer from the list of the obj finalizers.
// Return true if finalizer has been removed
func RemoveFinalizer(obj runtime.Object, name string) bool {
	if !HasFinalizer(obj, name) {
		return false
	}

	finalizers := make([]string, 0)
	for _, finalizer := range ObjectMeta(obj).GetFinalizers() {
		if finalizer != name {
			finalizers = append(finalizers, finalizer)
		}
	}

	ObjectMeta(obj).SetFinalizers(finalizers)
	return true
}

// HasFinalizer returns true if the resource has the given finalizer name
func HasFinalizer(obj runtime.Object, name string) bool {
	finalizers := ObjectMeta(obj).GetFinalizers()
	for _, finalizer := range finalizers {
		if finalizer == name {
			return true
		}
	}
	return false
}
