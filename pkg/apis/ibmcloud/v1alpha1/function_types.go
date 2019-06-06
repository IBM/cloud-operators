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

// Function is the Schema for the functions API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Function struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FunctionSpec   `json:"spec,omitempty"`
	Status FunctionStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FunctionList contains a list of Function
type FunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Function `json:"items"`
}

// FunctionSpec represents the specification for Function resources
type FunctionSpec struct {
	// Action name. Override metadata.name. Does not include the package name (see below)
	// +optional
	Name *string `json:"name,omitempty"`
	// Action package name. Add it to the default package when not specified
	// +optional
	Package *string `json:"package,omitempty"`
	// The location of the code to deploy. Support `http(s)` and `file` protocols.
	// +optional
	CodeURI *string `json:"codeURI,omitempty"`
	// The inline code to deploy.
	// +optional
	Code *string `json:"code,omitempty"`
	// Runtime name and optional version.
	// More info: https://github.com/apache/incubator-openwhisk/blob/master/docs/actions.md#languages-and-runtimes
	//
	// Support these runtimes (not an exhaustive live):
	//
	// Runtime	| image name	| Description\n
	// -------- | --------- | ----------\n
	// app | N/A | runs a function composition\n
	// sequence | N/A | runs a function sequence\n
	// nodejs | nodejsaction:latest | Latest NodeJS runtime\n
	// nodejs:6	| nodejs6action:latest | Latest NodeJS 6 runtime\n
	// java	| java8action:latest | Latest Java language runtime\n
	// python:2	| python2action:latest | Latest Python 2 language runtime\n
	// python:3	| python3action:latest | Latest Python 3 language runtime\n
	// swift | swiftaction:latest | Latest Swift 2 language runtime\n
	// swift:3	| swift3action:latest | Latest Swift 3 language runtime\n
	// swift:3.1.1 | action-swift-v3.1.1:latest | Latest Swift 3.1.1 language runtime\n
	// php:7.1 | action-php-v7.1:latest	| Latest PHP language runtime\n
	//
	// +optional
	Runtime string `json:"runtime"`
	// Docker image identifier (in dockerhub). More info: https://github.com/apache/incubator-openwhisk/blob/master/docs/actions-docker.md
	// +optional
	Docker string `json:"docker,omitempty"`
	// Run the action as native. More info: https://github.com/apache/incubator-openwhisk/blob/master/docs/actions-docker.md#creating-native-actions
	// +optional
	Native bool `json:"native,omitempty"`
	// List of key/value input parameters
	// +optional
	Parameters []keyvaluev1.KeyValue `json:"parameters,omitempty"`
	// Sets the action limits. More info: https://github.com/apache/incubator-openwhisk/blob/master/docs/reference.md#system-limits
	// +optional
	Limits *Limits `json:"limits,omitempty"`
	// The name of the action entry point (function or fully-qualified method name when applicable)
	// +optional
	Main *string `json:"main,omitempty"`
	// Comma separated sequence of actions. Only valid when `runtime` is `sequence`
	// +optional
	Functions *string `json:"functions,omitempty"`
	// Turns the function into a "web action" causing it to return HTTP content
	// without use of an API Gateway. More info: https://github.com/apache/incubator-openwhisk/blob/master/docs/webactions.md
	// +optional
	WebExport bool `json:"webExport,omitempty"`
	// Indicates if the function is able to consume the raw contents within
	// the body of an HTTP request. Only valid when `webExport` is `true`.
	// More info: https://github.com/apache/incubator-openwhisk/blob/master/docs/webactions.md#raw-http-handling
	// +optional
	RawHTTP bool `json:"rawHTTP,omitempty"`
	// List of key/value annotations
	// +optional
	Annotations []keyvaluev1.KeyValue `json:"annotations,omitempty"`
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

// Limits is used to express function resources constraints such as memory limits or timeout
type Limits struct {
	// The per-invocation Action timeout. Default unit is assumed to be milliseconds (ms).
	// +optional
	Timeout int `json:"timeout,omitempty"`

	// The per-Action memory. Default unit is assumed to be in megabytes (MB).
	// +optional
	Memory int `json:"memory,omitempty"`

	// The action log size. Default unit is assumed to be in megabytes (MB).
	// +optional
	LogSize int `json:"logSize,omitempty"`
}

// FunctionStatus defines the observed state of Function
type FunctionStatus struct {
	resv1.ResourceStatus `json:",inline"`

	// Last synced generation. Set by the system
	Generation int64 `json:"generation"`
}

// GetStatus returns the function status
func (r *Function) GetStatus() resv1.Status {
	return &r.Status
}

func init() {
	SchemeBuilder.Register(&Function{}, &FunctionList{})
}
