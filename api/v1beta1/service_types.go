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

package v1beta1

import (
	"github.com/ibm/cloud-operators/api/v1beta1/keyvalue"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceSpec defines the desired state of Service
type ServiceSpec struct {
	ServiceClass string `json:"serviceClass"`
	Plan         string `json:"plan"`
	// +optional
	ServiceClassType string `json:"serviceClassType,omitempty"`
	// +optional
	ExternalName string `json:"externalName,omitempty"`
	// +optional
	Parameters []keyvalue.KeyValue `json:"parameters,omitempty"`
	// +optional
	Tags []string `json:"tags,omitempty"`
	// +optional
	Context ServiceContext `json:"context,omitempty"`
}

// ServiceStatus defines the observed state of Service
type ServiceStatus struct {
	metav1.TypeMeta `json:",inline"`
	Generation      int64 `json:"generation,omitempty"`

	ServiceClass     string `json:"serviceClass"`
	ServiceClassType string `json:"serviceClassType"`
	Plan             string `json:"plan"`
	// +optional
	InstanceID string `json:"instanceId,omitempty"`
	// +optional
	ExternalName string `json:"externalName,omitempty"`
	// +optional
	Context ServiceContext `json:"context,omitempty"`
	// +optional
	Parameters []keyvalue.KeyValue `json:"parameters,omitempty"`
	// +optional
	Tags []string `json:"tags,omitempty"`
	// +optional
	DashboardURL string `json:"dashboardURL,omitempty"`
}

// +kubebuilder:object:root=true

// Service is the Schema for the services API
type Service struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceSpec   `json:"spec,omitempty"`
	Status ServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceList contains a list of Service
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Service `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Service{}, &ServiceList{})
}

type ServiceContext struct {
	// +optional
	Org string `json:"org,omitempty"`
	// +optional
	Space string `json:"space,omitempty"`
	// +optional
	Region string `json:"region,omitempty"`
	// +optional
	ResourceGroup string `json:"resourcegroup,omitempty"`
	// +optional
	ResourceGroupID string `json:"resourcegroupid,omitempty"`
	// +optional
	ResourceLocation string `json:"resourcelocation,omitempty"`
	// +optional
	User string `json:"user,omitempty"`
}
