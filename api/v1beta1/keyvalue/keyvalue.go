package keyvalue

import (
	"encoding/json"
)

// +k8s:deepcopy-gen=true

// KeyValue represents a key-value pair
type KeyValue struct {
	// Name representing the key.
	Name string `json:"name"`

	// A parameter may have attributes (e.g. message hub topic might have partitions)
	// +optional
	Attributes map[string]json.RawMessage `json:"attributes,omitempty"`

	// Mutual exclusive: no more than one of the following may be specified.

	// Defaults to null.
	// +optional
	Value json.RawMessage `json:"value,omitempty"`

	// Source for the value. Cannot be used if value is not empty.
	// +optional
	ValueFrom *Source `json:"valueFrom,omitempty"`
}
