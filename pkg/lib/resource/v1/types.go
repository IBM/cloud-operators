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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ResourceStateCreated indicates a resource is in a created state
	ResourceStateCreated string = "Created"
	// ResourceStatePending indicates a resource is in a pending state
	ResourceStatePending string = "Pending"
	// ResourceStateStopped indicates a resource is in a running state
	ResourceStateStopped string = "Stopped"
	// ResourceStateFailed indicates a resource is in a failed state
	ResourceStateFailed string = "Failed"
	// ResourceStateUnknown indicates a resource is in a unknown state
	ResourceStateUnknown string = "Unknown"
	// ResourceStateDeleting indicates a resource is being deleted
	ResourceStateDeleting string = "Deleting"
	// ResourceStateOnline indicates a resource has been fully synchronized and online
	ResourceStateOnline string = "Online"
	// ResourceStateWaiting indicates a resource is in a waiting state, e.g. waiting for dependencies
	ResourceStateWaiting string = "Waiting"
	// ResourceStateRetrying indicates a resource failed to provision for external reasons. Retrying later on.
	ResourceStateRetrying string = "Retrying"
	// ResourceStateBinding indicates a resource such as a cloud service is being bound
	ResourceStateBinding string = "Binding"
)

// Resource is the base struct for custom resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
type Resource struct {
	metav1.TypeMeta `json:",inline"`

	// Example of condition list
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// Conditions []Condition  `json:"conditions,omitempty"`
}

// ResourceList is the base struct for custom list types
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
}

// Spec provides embeddable Spec
type Spec struct {
}

// ResourceStatus defines the status for each resource
type ResourceStatus struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// Condition is the base struct for representing resource conditions
type Condition struct {
	// Type of condition, e.g Complete or Failed.
	Type string `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,3,opt,name=lastTransitionTime"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,5,opt,name=message"`
}
