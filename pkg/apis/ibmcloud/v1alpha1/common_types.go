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

package v1alpha1

import v1 "k8s.io/api/core/v1"

// ParametersFromSource represents a source for the value of parameters
type ParametersFromSource struct {
	// Selects a key of a ConfigMap.
	// +optional
	ConfigMapKeyRef *ConfigMapKeyReference `json:"configMapKeyRef,omitempty"`

	// Selects a key of a secret in the resource namespace
	// +optional
	SecretKeyRef *SecretKeyReference `json:"secretKeyRef,omitempty"`
}

// ConfigMapKeyReference selects a ConfigMap and optionally a key from it.
type ConfigMapKeyReference struct {
	// The ConfigMap to select from.
	v1.LocalObjectReference `json:",inline"`

	// // The key to select.
	// // +optional
	// Key *string `json:"key,omitempty"`
}

// SecretKeyReference selects a secret and optionally a key from it.
type SecretKeyReference struct {
	// The name of the secret in the pod's namespace to select from.
	v1.LocalObjectReference `json:",inline"`

	// // The key of the secret to select from.  Must be a valid secret key.
	// // +optional
	// Key *string `json:"key"`
}
