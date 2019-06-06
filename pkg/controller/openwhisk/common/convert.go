/*
 * Copyright 2017-2018 IBM Corporation
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

package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/apache/incubator-openwhisk-client-go/whisk"
	"k8s.io/apimachinery/pkg/runtime"

	context "github.com/ibm/cloud-operators/pkg/context"
	kv "github.com/ibm/cloud-operators/pkg/lib/keyvalue/v1"
	"github.com/ibm/cloud-operators/pkg/lib/secret"
	"github.com/ibm/cloud-operators/pkg/util"

	openwhiskv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
)

// ConvertKeyValues convert key value array to whisk key values
func ConvertKeyValues(ctx context.Context, obj runtime.Object, params []kv.KeyValue, what string) (whisk.KeyValueArr, bool, error) {
	keyValArr, err := ToKeyValueArr(ctx, params)
	if err != nil {
		if strings.Contains(err.Error(), "Missing") {
			return nil, true, fmt.Errorf("%v (Retrying)", err)
		}
		return nil, false, fmt.Errorf("Error converting %s: %v", what, err)
	}
	return keyValArr, false, nil
}

// ToKeyValueArr converts a list of key-value pairs to Whisk format
func ToKeyValueArr(ctx context.Context, vars []kv.KeyValue) (whisk.KeyValueArr, error) {
	keyValueArr := make(whisk.KeyValueArr, 0)
	for _, v := range vars {
		var keyVal whisk.KeyValue
		keyVal.Key = v.Name

		value, err := v.ToJSON(ctx)
		if err != nil {
			return nil, err
		}

		if value != nil {
			keyVal.Value = value
			keyValueArr = append(keyValueArr, keyVal)
		}
	}

	return keyValueArr, nil
}

// ToKeyValueArrFromMap converts raw JSON to whisk param format
func ToKeyValueArrFromMap(m interface{}) (whisk.KeyValueArr, error) {
	obj, ok := m.(map[string]interface{})
	if !ok {
		return nil, errors.New("error: JSON value is not an object")
	}
	keyValueArr := make(whisk.KeyValueArr, 0)

	for key := range obj {
		var keyVal whisk.KeyValue
		keyVal.Key = key
		keyVal.Value = obj[key]
		keyValueArr = append(keyValueArr, keyVal)
	}

	return keyValueArr, nil
}

// GetValueString gets the string value for the key
func GetValueString(keyValueArr whisk.KeyValueArr, key string) (string, error) {
	value := keyValueArr.GetValue(key)
	if str, ok := value.(string); ok {
		return str, nil
	}
	return "", fmt.Errorf("missing string value '%v' for key '%s'", value, key)
}

// ConvertParametersFrom converts parameters sources to whisk key value pairs
func ConvertParametersFrom(ctx context.Context, obj runtime.Object, params []openwhiskv1alpha1.ParametersFromSource) (whisk.KeyValueArr, bool, error) {
	keyValueArr := make(whisk.KeyValueArr, 0)

	for _, source := range params {
		if source.ConfigMapKeyRef != nil {
			cm, err := util.GetConfigMap(ctx, source.ConfigMapKeyRef.Name, true)
			if err != nil {
				// Recoverable
				return nil, true, fmt.Errorf("Missing configmap %s", source.ConfigMapKeyRef.Name)
			}
			if cm.Data != nil {
				for k, v := range cm.Data {
					var keyVal whisk.KeyValue
					keyVal.Key = k

					value := toJSONFromString(v)

					if value != nil {
						keyVal.Value = value
						keyValueArr = append(keyValueArr, keyVal)
					}
				}
			}
		}
		if source.SecretKeyRef != nil {
			sc, err := secret.GetSecret(ctx, source.SecretKeyRef.Name, true)
			if err != nil {
				// Recoverable
				return nil, true, fmt.Errorf("Missing secret %s", source.SecretKeyRef.Name)

			}
			if sc.Data != nil {
				for k, v := range sc.Data {
					var keyVal whisk.KeyValue
					keyVal.Key = k

					value := toJSONFromString(string(v))
					if value != nil {
						keyVal.Value = value
						keyValueArr = append(keyValueArr, keyVal)
					}
				}
			}
		}
	}

	return keyValueArr, false, nil
}

func toJSONFromString(content string) interface{} {
	var data interface{}

	dc := json.NewDecoder(strings.NewReader(content))
	dc.UseNumber()
	if err := dc.Decode(&data); err != nil {
		// Just return the content in order to support unquoted string value
		// In the future we might want to implement some heuristic to detect the user intention
		// Maybe if content start with '{' or '[' then the intent might be to specify a JSON and it is invalid

		return content
	}
	if dc.More() {
		// Not a valid JSON. Interpret as unquoted string value
		return content
	}
	return data
}

func isJSON(str string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(str), &js) == nil
}
