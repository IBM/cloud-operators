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

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Package is the Schema for the packages API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Package struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PackageSpec   `json:"spec,omitempty"`
	Status PackageStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PackageList contains a list of Package
type PackageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Package `json:"items"`
}

// PackageSpec defines the desired state of Package
type PackageSpec struct {
	// Package name. Override metadata.name. `default` is reserved.
	// +optional
	Name string `json:"name,omitempty"`

	// Package visibility; `true` for `shared`, `false` for `private`
	// +optional
	Publish *bool `json:"publish,omitempty"`

	// Name of the package for which a binding should be created
	// +optional
	Bind string `json:"bind,omitempty"`

	// indicates a cloud service resource which you want to bind to.
	// This feature provides automatic injection of service keys into the binding parameters (for example user, password, urls)
	// +optional
	Service string `json:"service,omitempty"`

	// List of key/value annotations
	// +optional
	Annotations []keyvaluev1.KeyValue `json:"annotations,omitempty"`

	// List of key/value input parameters
	// +optional
	Parameters []keyvaluev1.KeyValue `json:"parameters,omitempty"`

	// List of key/value input parameters coming from a Secret or ConfigMap
	// When multiple sources are specified, all key/value pairs are merged
	// into a single set of key/value pairs, from the first source to the last source
	// Duplicates are handled by overriding the previous key/value pair.
	// The parameters property is applied last
	// +optional
	ParametersFrom []ParametersFromSource `json:"parametersFrom,omitempty"`

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

// PackageStatus defines the observed state of Package
type PackageStatus struct {
	resv1.ResourceStatus `json:",inline"`

	// Last synced generation. Set by the system
	Generation int64 `json:"generation"`
}

func init() {
	SchemeBuilder.Register(&Package{}, &PackageList{})
}

// GetStatus returns the function status
func (r *Package) GetStatus() resv1.Status {
	return &r.Status
}
