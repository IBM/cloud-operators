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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	keyvaluev1 "github.com/ibm/cloud-operators/pkg/lib/keyvalue/v1"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"
)

// Invocation is the Schema for the invocations API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Invocation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InvocationSpec   `json:"spec,omitempty"`
	Status InvocationStatus `json:"status,omitempty"`
}

// InvocationList contains a list of Invocation
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type InvocationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Invocation `json:"items"`
}

// InvocationSpec defines the desired state of Invocation
type InvocationSpec struct {
	// defines the name of function to invoke (eg. `/whisk.system/utils/echo` or `myfunction`)
	// Invokes the function in the invocation context when the name is not fully qualified
	Function string `json:"function"`

	// Defines the list of parameters to use for the invocation
	// +optional
	Parameters []keyvaluev1.KeyValue `json:"parameters,omitempty"`

	// Defines where to store the invocation result. Discard the result when not specified.
	// +optional
	To *InvocationTarget `json:"to,omitempty"`

	// Defines the function to invoke when this resource is deleted.
	// +optional
	Finalizer *InvocationFinalizer `json:"finalizer,omitempty"`

	// Reference to a secret representing where to deploy this entity
	// Default is `seed-default-owprops`
	// The secret must defines these fields:
	// apihost (string) : The OpenWhisk host
	// auth (string): the authorization key
	// cert (string):  the client certificate (optional)
	// insecure (bool):  Whether or not to bypass certificate checking (optional, default is false)
	// +optional
	ContextFrom *v1.SecretEnvSource `json:"contextFrom,omitempty"`
}

// InvocationStatus defines the observed state of Invocation
type InvocationStatus struct {
	resv1.ResourceStatus `json:",inline"`

	// Last synced generation. Set by the system
	Generation int64 `json:"generation"`
}

// InvocationFinalizer defines the function to invoke when deleting the function invocation
type InvocationFinalizer struct {
	// Function defines the name of the function to invoke (eg. `/whisk.system/utils/echo` or `myfunction`)
	// Invokes the function in the invocation context when the name is not fully qualified
	Function string `json:"function"`

	// Parameters defines the list of parameters to use for the invocation
	// +optional
	Parameters []keyvaluev1.KeyValue `json:"parameters,omitempty"`
}

// InvocationTarget represents where to store the invocation result
type InvocationTarget struct {
	// Selects a key of a ConfigMap.
	// +optional
	ConfigMapKeyRef *v1.ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`

	// Selects a key of a secret in the invocation namespace
	// +optional
	SecretKeyRef *v1.SecretKeySelector `json:"secretKeyRef,omitempty"`

	// JSONPath template selecting parts of the invocation result to store.
	// Default is "{@.response.result}"
	// More info: https://kubernetes.io/docs/reference/kubectl/jsonpath/
	Projection *string `json:"projection,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Invocation{}, &InvocationList{})
}

// GetStatus returns the function status
func (r *Invocation) GetStatus() resv1.Status {
	return &r.Status
}
