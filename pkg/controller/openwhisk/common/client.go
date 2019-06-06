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
	"errors"
	"fmt"
	"net/url"
	"strings"

	"k8s.io/api/core/v1"

	"github.com/apache/incubator-openwhisk-client-go/whisk"

	context "github.com/ibm/cloud-operators/pkg/context"
	"github.com/ibm/cloud-operators/pkg/lib/secret"
)

// --- Authentication

// WskConfig struct
type WskConfig struct {
	WskCliAuthKey   string   `json:"WskCliAuthKey"`
	WskCliNamespace string   `json:"WskCliNamespace"`
	WskCliAPIHost   string   `json:"WskCliApiHost"`
	WskCliInsecure  bool     `json:"WskCliInsecure"`
	WskCliCert      string   `json:"WskCliCert"`
	WskCliBaseURL   *url.URL `json:"WskBaseURL"`
}

// NewWskClient creates a new whisk.Client
func NewWskClient(ctx context.Context, owctx *v1.SecretEnvSource) (*whisk.Client, error) {
	secretName := "seed-defaults-owprops"
	if owctx != nil && owctx.Name != "" {
		secretName = owctx.Name
	}

	config, err := GetWskPropertiesFromSecret(ctx, secretName)
	if err != nil {
		return nil, err
	}

	configInput := &whisk.Config{
		AuthToken: string(config.WskCliAuthKey),
		Namespace: "_",
		Host:      string(config.WskCliAPIHost),
		Version:   "v1",
		BaseURL:   config.WskCliBaseURL,
		Cert:      config.WskCliCert,
		// Key:       key.Value,

		Insecure: config.WskCliInsecure, // true if you want to ignore certificate signing
	}

	client, err := whisk.NewClient(nil, configInput)
	return client, err
}

// GetWskPropertiesFromSecret returns WskConfig from secret
func GetWskPropertiesFromSecret(ctx context.Context, secretName string) (*WskConfig, error) {
	var config WskConfig

	secret, err := secret.GetSecret(ctx, secretName, true)
	if err != nil {
		return nil, err
	}

	apihostb, ok := secret.Data["apihost"]
	if !ok || string(apihostb) == "" {
		return nil, fmt.Errorf("missing 'apihost' key in seed-defaults-owprops secret")
	}
	authb, ok := secret.Data["auth"]
	if !ok || string(authb) == "" {
		return nil, fmt.Errorf("missing 'auth' key in seed-defaults-owprops secret")
	}
	config.WskCliInsecure = false
	if insecureb, ok := secret.Data["insecure"]; ok {
		config.WskCliInsecure = string(insecureb) == "true"
	}
	config.WskCliCert = ""
	if certb, ok := secret.Data["cert"]; ok {
		config.WskCliCert = string(certb)
	}

	config.WskCliAuthKey = string(authb)
	config.WskCliAPIHost = string(apihostb)

	baseURL, err := whisk.GetURLBase(config.WskCliAPIHost, "/api")
	if err != nil {
		return nil, err
	}
	config.WskCliBaseURL = baseURL

	return &config, nil
}

// ResolveFunctionName returns the full name of a function
func ResolveFunctionName(objname string, pkg *string, name *string) string {
	actualName := objname
	if name != nil {
		actualName = *name
	}

	if pkg != nil && *pkg != "default" {
		return strings.Join([]string{*pkg, actualName}, "/")
	}
	return actualName
}

// QualifiedName is a namespace-qualified name
type QualifiedName struct {
	Namespace  string
	EntityName string
}

// from go whisk cli

/*
 Parse a (possibly fully qualified) resource name into namespace and name components. If the given qualified name isNone,
 then this is a default qualified name and it is resolved from properties. If the namespace is missing from the qualified
 name, the namespace is also resolved from the property file.

 Return a qualifiedName struct

 Examples:
	   foo => qName {namespace: "_", entityName: foo}
	   pkg/foo => qName {namespace: "_", entityName: pkg/foo}
	   /ns/foo => qName {namespace: ns, entityName: foo}
	   /ns/pkg/foo => qName {namespace: ns, entityName: pkg/foo}
*/

// ParseQualifiedName returns a qualified name
func ParseQualifiedName(name string, defaultNamespace string) (QualifiedName, error) {
	var qualifiedName QualifiedName

	// If name has a preceding delimiter (/), it contains a namespace. Otherwise the name does not specify a namespace,
	// so default the namespace to the namespace value set in the properties file; if that is not set, use "_"
	if strings.HasPrefix(name, "/") {
		parts := strings.Split(name, "/")
		qualifiedName.Namespace = parts[1]

		if len(parts) < 2 || len(parts) > 4 {
			err := errors.New("A valid qualified name was not detected")
			return qualifiedName, err
		}

		for i := 1; i < len(parts); i++ {
			if len(parts[i]) == 0 || parts[i] == "." {
				err := errors.New("A valid qualified name was not detected")
				return qualifiedName, err
			}
		}

		qualifiedName.EntityName = strings.Join(parts[2:], "/")
	} else {
		if len(name) == 0 || name == "." {
			err := errors.New("A valid qualified name was not detected")
			return qualifiedName, err
		}

		qualifiedName.EntityName = name
		if defaultNamespace == "" {
			defaultNamespace = "_"
		}
		qualifiedName.Namespace = defaultNamespace
	}

	return qualifiedName, nil
}

// JoinQualifiedName returns a fully qualified entity name
func JoinQualifiedName(qname QualifiedName) string {
	return fmt.Sprintf("/%s/%s", qname.Namespace, qname.EntityName)
}

// --- Web annotation helpers

const webExportAnnot = "web-export"
const rawHTTPAnnot = "raw-http"

// const finalAnnot = "final"

// WebAction adds web annotations
func WebAction(webMode string, annotations whisk.KeyValueArr, fetch bool) (whisk.KeyValueArr, error) {
	switch strings.ToLower(webMode) {
	case "yes":
		fallthrough
	case "true":
		return webActionAnnotations(fetch, annotations, addWebAnnotations)
	case "no":
		fallthrough
	case "false":
		return webActionAnnotations(fetch, annotations, deleteWebAnnotations)
	case "raw":
		return webActionAnnotations(fetch, annotations, addRawAnnotations)
	default:
		return nil, errors.New(webMode)
	}
}

type webActionAnnotationMethod func(annotations whisk.KeyValueArr) whisk.KeyValueArr

func webActionAnnotations(
	fetchAnnotations bool,
	annotations whisk.KeyValueArr,
	webActionAnnotationMethod webActionAnnotationMethod) (whisk.KeyValueArr, error) {
	if annotations != nil || !fetchAnnotations {
		annotations = webActionAnnotationMethod(annotations)
	}

	return annotations, nil
}

func addWebAnnotations(annotations whisk.KeyValueArr) whisk.KeyValueArr {
	annotations = deleteWebAnnotationKeys(annotations)
	annotations = AddKeyValue(webExportAnnot, true, annotations)
	annotations = AddKeyValue(rawHTTPAnnot, false, annotations)
	//	annotations = AddKeyValue(finalAnnot, true, annotations)

	return annotations
}

func deleteWebAnnotations(annotations whisk.KeyValueArr) whisk.KeyValueArr {
	annotations = deleteWebAnnotationKeys(annotations)
	annotations = AddKeyValue(webExportAnnot, false, annotations)
	annotations = AddKeyValue(rawHTTPAnnot, false, annotations)
	//	annotations = AddKeyValue(finalAnnot, false, annotations)

	return annotations
}

func addRawAnnotations(annotations whisk.KeyValueArr) whisk.KeyValueArr {
	annotations = deleteWebAnnotationKeys(annotations)
	annotations = AddKeyValue(webExportAnnot, true, annotations)
	annotations = AddKeyValue(rawHTTPAnnot, true, annotations)
	//	annotations = AddKeyValue(finalAnnot, true, annotations)

	return annotations
}

func deleteWebAnnotationKeys(annotations whisk.KeyValueArr) whisk.KeyValueArr {
	annotations = deleteKey(webExportAnnot, annotations)
	annotations = deleteKey(rawHTTPAnnot, annotations)
	//	annotations = deleteKey(finalAnnot, annotations)

	return annotations
}

// AddKeyValue adds a key-value pair to the given array
func AddKeyValue(key string, value interface{}, keyValueArr whisk.KeyValueArr) whisk.KeyValueArr {
	keyValue := whisk.KeyValue{
		Key:   key,
		Value: value,
	}

	return append(keyValueArr, keyValue)
}

func deleteKey(key string, keyValueArr whisk.KeyValueArr) whisk.KeyValueArr {
	for i := 0; i < len(keyValueArr); i++ {
		if keyValueArr[i].Key == key {
			keyValueArr = append(keyValueArr[:i], keyValueArr[i+1:]...)
			break
		}
	}

	return keyValueArr
}
