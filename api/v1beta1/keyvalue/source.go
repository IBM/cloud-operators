package keyvalue

import (
	v1 "k8s.io/api/core/v1"
)

// +k8s:deepcopy-gen=true

// Source represents a source for the value of a KeyValue.
type Source struct {
	// Selects a key of a ConfigMap.
	// +optional
	ConfigMapKeyRef *v1.ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`

	// Selects a key of a secret in the resource namespace
	// +optional
	SecretKeyRef *v1.SecretKeySelector `json:"secretKeyRef,omitempty"`
}
