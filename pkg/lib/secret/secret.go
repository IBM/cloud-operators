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

package secret

import (
	"fmt"

	"k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"

	rcontext "github.com/ibm/cloud-operators/pkg/context"
)

var slog = log.Log.WithName("secret")

// GetSecret gets a kubernetes secret
func GetSecret(context rcontext.Context, secretname string, fallback bool) (*v1.Secret, error) {
	namespace := context.Namespace()
	log := slog.WithName(fmt.Sprintf("%s/%s", namespace, secretname))
	log.V(5).Info("getting secret")

	var secret v1.Secret
	if err := context.Client().Get(context, client.ObjectKey{Namespace: namespace, Name: secretname}, &secret); err != nil {
		if namespace != "default" && fallback {
			if err := context.Client().Get(context, client.ObjectKey{Namespace: "default", Name: secretname}, &secret); err != nil {
				log.V(5).Info("secret not found")
				return nil, err
			}
		} else {
			log.V(5).Info("secret not found")
			return nil, err
		}
	}
	log.V(5).Info("secret found")
	return &secret, nil
}

// HasSecret checks if a secret exists
func HasSecret(context rcontext.Context, name string, fallback bool) bool {
	slog.Info("Checking secret %s exist", name)
	_, err := GetSecret(context, name, fallback)
	return err != nil
}

// GetSecretValue gets the value of a secret in the given namespace. If not found and fallback is true, check default namespace
func GetSecretValue(context rcontext.Context, name string, key string, fallback bool) ([]byte, error) {
	secret, err := GetSecret(context, name, fallback)
	if err != nil {
		return nil, err
	}

	return secret.Data[key], nil
}
