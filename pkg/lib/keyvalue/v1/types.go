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
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	rcontext "github.com/ibm/cloud-operators/pkg/context"
	"github.com/ibm/cloud-operators/pkg/lib/secret"
	"github.com/ibm/cloud-operators/pkg/util"
)

// KeyValue represents a key-value pair
type KeyValue struct {
	// Name representing the key.
	Name string `json:"name"`

	// A parameter may have attributes (e.g. message hub topic might have partitions)
	// +optional
	Attributes map[string]runtime.RawExtension `json:"attributes,omitempty"`

	// Mutual exclusive: no more than one of the following may be specified.

	// Defaults to null.
	// +optional
	Value *runtime.RawExtension `json:"value,omitempty"`

	// Source for the value. Cannot be used if value is not empty.
	// +optional
	ValueFrom *KeyValueSource `json:"valueFrom,omitempty"`
}

// KeyValueSource represents a source for the value of a KeyValue.
type KeyValueSource struct {
	// Selects a key of a ConfigMap.
	// +optional
	ConfigMapKeyRef *v1.ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`

	// Selects a key of a secret in the resource namespace
	// +optional
	SecretKeyRef *v1.SecretKeySelector `json:"secretKeyRef,omitempty"`
}

// ToJSON converts variable value to JSON value
func (v *KeyValue) ToJSON(ctx rcontext.Context) (interface{}, error) {
	if v.Value != nil && v.ValueFrom != nil {
		return nil, fmt.Errorf("Value and ValueFrom properties are mutually exclusive (for %s variable)", v.Name)
	}

	valueFrom := v.ValueFrom
	if valueFrom != nil {
		return ValueToJSON(ctx, *valueFrom)
	}

	if v.Value == nil {
		return nil, nil
	}
	return toJSONFromRaw(v.Value)
}

// ValueToJSON takes a KeyValueSource and resolves its value
func ValueToJSON(ctx rcontext.Context, valueFrom KeyValueSource) (interface{}, error) {
	if valueFrom.SecretKeyRef != nil {
		data, err := secret.GetSecretValue(ctx, valueFrom.SecretKeyRef.Name, valueFrom.SecretKeyRef.Key, true)
		if err != nil {
			// Recoverable
			return nil, fmt.Errorf("Missing secret %s", valueFrom.SecretKeyRef.Name)
		}
		return toJSONFromString(string(data))
	} else if valueFrom.ConfigMapKeyRef != nil {
		data, err := util.GetConfigMapValue(ctx, valueFrom.ConfigMapKeyRef.Name, valueFrom.ConfigMapKeyRef.Key, true)
		if err != nil {
			// Recoverable
			return nil, fmt.Errorf("Missing configmap %s", valueFrom.ConfigMapKeyRef.Name)
		}
		return toJSONFromString(data)
	}
	return nil, fmt.Errorf("Missing secretKeyRef or configMapKeyRef")
}

func toJSONFromRaw(content *runtime.RawExtension) (interface{}, error) {
	var data interface{}

	if err := json.Unmarshal(content.Raw, &data); err != nil {
		return nil, err
	}

	return data, nil
}

func toJSONFromString(content string) (interface{}, error) {
	var data interface{}

	dc := json.NewDecoder(strings.NewReader(content))
	dc.UseNumber()
	if err := dc.Decode(&data); err != nil {
		// Just return the content in order to support unquoted string value
		// In the future we might want to implement some heuristic to detect the user intention
		// Maybe if content start with '{' or '[' then the intent might be to specify a JSON and it is invalid

		return content, nil
	}
	if dc.More() {
		// Not a valid JSON. Interpret as unquoted string value
		return content, nil
	}
	return data, nil
}

func isJSON(str string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(str), &js) == nil
}
