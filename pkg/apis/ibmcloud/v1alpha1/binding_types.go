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

import (
	keyvaluev1 "github.com/ibm/cloud-operators/pkg/lib/keyvalue/v1"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BindingSpec defines the desired state of Binding
type BindingSpec struct {
	ServiceName string `json:"serviceName"`
	// +optional
	ServiceNamespace string `json:"serviceNamespace,omitempty"`
	// +optional
	Alias string `json:"alias,omitempty"`
	// +optional
	SecretName string `json:"secretName,omitempty"`
	// +optional
	Role string `json:"role,omitempty"`
	// +optional
	Parameters []keyvaluev1.KeyValue `json:"parameters,omitempty"`
}

// BindingStatus defines the observed state of Binding
type BindingStatus struct {
	resv1.ResourceStatus `json:",inline"`
	// +optional
	Generation int64 `json:"generation,omitempty"`
	// +optional
	InstanceID string `json:"instanceId,omitempty"`
	// +optional
	KeyInstanceID string `json:"keyInstanceId,omitempty"`
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Binding is the Schema for the bindings API
// +k8s:openapi-gen=true
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
type Binding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BindingSpec   `json:"spec,omitempty"`
	Status BindingStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BindingList contains a list of Binding
type BindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Binding `json:"items"`
}

// GetStatus returns the binding status
func (s *Binding) GetStatus() resv1.Status {
	return &s.Status
}

func init() {
	SchemeBuilder.Register(&Binding{}, &BindingList{})
}
