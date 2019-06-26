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
	icv1 "github.com/ibm/cloud-operators/pkg/lib/ibmcloud/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BucketSpec defines the desired state of Bucket
type BucketSpec struct {
	BindingFrom    v1.SecretEnvSource   `json:"bindingFrom"`
	Resiliency     string               `json:"resiliency,omitempty"`   // Default to regional
	Location       string               `json:"location,omitempty"`     // Default to us-south
	BucketType     string               `json:"bucketType,omitempty"`   // Default to public
	StorageClass   string               `json:"storageClass,omitempty"` // Default to standard
	CORSRules      CORSRule             `json:"corsRules,omitempty"`
	KeepIfNotEmpty bool                 `json:"keepIfNotEmpty,omitempty"` // Default to true
	Context        icv1.ResourceContext `json:"context,omitempty"`
	BindOnly       bool                 `json:"bindOnly,omitempty"` // Default to false
	// ExternalInstanceID string `json:"externalInstanceId,omitempty"`
	RetentionPolicy RetentionPolicy `json:"retentionPolicy,omitempty"`
}

type CORSRule struct {
	AllowedOrigin  string   `json:"allowedOrigin,omitempty"`  // Default to *
	AllowedHeader  string   `json:"allowedHeader,omitempty"`  // Default to *
	AllowedMethods []string `json:"allowedMethods,omitempty"` // Default to Post, Get, Put
}

type RetentionPolicy struct {
	MinimumRetentionDay int `json:"minimumRetentionDay,omitempty"`
	MaximumRetentionDay int `json:"maximumRetentionDay,omitempty"`
	DefaultRetentionDay int `json:"defaultRetentionDay,omitempty"`
}

// BucketStatus defines the observed state of Bucket
type BucketStatus struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Bucket is the Schema for the buckets API
// +k8s:openapi-gen=true
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
type Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BucketSpec   `json:"spec,omitempty"`
	Status BucketStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BucketList contains a list of Bucket
type BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Bucket{}, &BucketList{})
}
