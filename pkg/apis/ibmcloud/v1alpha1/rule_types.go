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

	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"
)

// Rule is the Schema for the rules API
// +k8s:openapi-gen=true
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
type Rule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RuleSpec   `json:"spec,omitempty"`
	Status RuleStatus `json:"status,omitempty"`
}

// RuleList contains a list of Rule
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rule `json:"items"`
}

// RuleSpec defines the desired state of Rule
type RuleSpec struct {
	// Rule name. Override metadata.name.
	// +optional
	Name string `json:"name,omitempty"`
	// Name of the trigger the Rule applies to
	Trigger string `json:"trigger"`
	// Name of the action the rule applies to
	Function string `json:"function"`

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

// RuleStatus defines the observed state of Rule
type RuleStatus struct {
	resv1.ResourceStatus `json:",inline"`

	// Last synced generation. Set by the system
	Generation int64 `json:"generation"`
}

func init() {
	SchemeBuilder.Register(&Rule{}, &RuleList{})
}

// GetStatus returns the function status
func (r *Rule) GetStatus() resv1.Status {
	return &r.Status
}
