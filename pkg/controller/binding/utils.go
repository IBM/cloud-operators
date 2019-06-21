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

package binding

import (
	"context"

	ibmcloudv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetSecret takes a name and namespace for a Binding and returns the corresponding secret
func GetSecret(r client.Client, binding *ibmcloudv1alpha1.Binding) (*corev1.Secret, error) {
	secretName := binding.Name
	if binding.Spec.SecretName != "" {
		secretName = binding.Spec.SecretName
	}
	secretInstance := &corev1.Secret{}
	err := r.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: binding.ObjectMeta.Namespace}, secretInstance)
	if err != nil {
		return &corev1.Secret{}, err
	}
	return secretInstance, nil
}

// GetBinding takes a name and namespace and returns the corresponding Binding object
func GetBinding(r client.Client, bindingName string, bindingNamespace string) (*ibmcloudv1alpha1.Binding, error) {
	bindingInstance := &ibmcloudv1alpha1.Binding{}
	err := r.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingNamespace}, bindingInstance)
	if err != nil {
		return &ibmcloudv1alpha1.Binding{}, err
	}
	return bindingInstance, nil
}
