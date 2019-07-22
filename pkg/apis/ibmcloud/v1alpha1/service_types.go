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
	icv1 "github.com/ibm/cloud-operators/pkg/lib/ibmcloud/v1"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceSpec defines the desired state of Service
type ServiceSpec struct {
	ServiceClass     string `json:"serviceClass"`
	Plan             string `json:"plan"`
	ServiceClassType string `json:"serviceClassType,omitempty"`
	// +optional
	ExternalName string `json:"externalName,omitempty"`
	// Parameters []keyvaluev1.KeyValue `json:"parameters,omitempty"`
	Context icv1.ResourceContext `json:"context,omitempty"`
}

// ServiceStatus defines the observed state of Service
type ServiceStatus struct {
	resv1.ResourceStatus `json:",inline"`
	Generation           int64 `json:"generation,omitempty"`

	ServiceClass     string               `json:"serviceClass"`
	ServiceClassType string               `json:"serviceClassType"`
	Plan             string               `json:"plan"`
	InstanceID       string               `json:"instanceId,omitempty"`
	ExternalName     string               `json:"externalName,omitempty"`
	Context          icv1.ResourceContext `json:"context,omitempty"`
	Binding          string               `json:"binding,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Service is the Schema for the services API
// +k8s:openapi-gen=true
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
type Service struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceSpec   `json:"spec,omitempty"`
	Status ServiceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceList contains a list of Service
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Service `json:"items"`
}

// GetStatus returns the service status
func (s *Service) GetStatus() resv1.Status {
	return &s.Status
}

func init() {
	SchemeBuilder.Register(&Service{}, &ServiceList{})
}
