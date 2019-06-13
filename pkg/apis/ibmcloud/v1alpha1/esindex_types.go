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

package v1alpha1

import (
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:object:root=true

// EsIndexSpec defines the desired state of EsIndex
type EsIndexSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of Index to be created on elastic search
	IndexName string `json:"indexName"`

	// Secret name that holds the elastic search access credentials
	BindingFrom v1.SecretEnvSource `json:"bindingFrom"`

	// Bind to an existing topic if true, default value false
	// +optional
	BindOnly bool `json:"bindOnly,omitempty"`

	// Number of shards, default value 1
	// +optional
	NumberOfShards int32 `json:"numberOfShards,omitempty"`

	// Number of replicas, default value 1
	// +optional
	NumberOfReplicas int32 `json:"numberOfReplicas,omitempty"`
}

// +kubebuilder:object:root=true

// EsIndexStatus defines the observed state of EsIndex
type EsIndexStatus struct {
	resv1.ResourceStatus `json:",inline"`
	Generation           int64 `json:"generation"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EsIndex is the Schema for the esindices API
// +k8s:openapi-gen=true
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
type EsIndex struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EsIndexSpec   `json:"spec,omitempty"`
	Status EsIndexStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EsIndexList contains a list of EsIndex
type EsIndexList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EsIndex `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EsIndex{}, &EsIndexList{})
}

// GetStatus returns the binding status
func (e *EsIndex) GetStatus() resv1.Status {
	return &e.Status
}
