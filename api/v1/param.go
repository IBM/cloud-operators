package v1

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
)

// Param represents a key-value pair
type Param struct {
	// Name representing the key.
	Name string `json:"name"`

	// A parameter may have attributes (e.g. message hub topic might have partitions)
	// +optional
	Attributes map[string]ParamValue `json:"attributes,omitempty"`

	// Mutual exclusive: no more than one of the following may be specified.

	// Defaults to null.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Value *ParamValue `json:"value,omitempty"`

	// Source for the value. Cannot be used if value is not empty.
	// +optional
	ValueFrom *ParamSource `json:"valueFrom,omitempty"`
}

// ParamSource represents a source for the value of a Param.
type ParamSource struct {
	// Selects a key of a ConfigMap.
	// +optional
	ConfigMapKeyRef *v1.ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`

	// Selects a key of a secret in the resource namespace
	// +optional
	SecretKeyRef *v1.SecretKeySelector `json:"secretKeyRef,omitempty"`
}

type ParamValue struct {
	json.RawMessage `json:"-"`
}
