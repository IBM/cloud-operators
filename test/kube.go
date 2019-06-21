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

package test

import (
	"strings"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc" //
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// SetupKubeOrDie setups Kube for testing
func SetupKubeOrDie(restCfg *rest.Config, stem string) string {
	clientset := GetClientsetOrDie(restCfg)

	namespace := CreateNamespaceOrDie(clientset.CoreV1().Namespaces(), stem)
	ConfigureSeedDefaults(clientset.CoreV1().ConfigMaps(namespace))
	ConfigureSeedSecret(clientset.CoreV1().Secrets(namespace))

	return namespace
}

// GetClientsetOrDie gets a Kube clientset for KUBECONFIG
func GetClientsetOrDie(restCfg *rest.Config) *kubernetes.Clientset {
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		panic(err)
	}
	return clientset
}

// GetContextNamespaceOrDie returns the current namespace context or "default"
func GetContextNamespaceOrDie() string {
	loader := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := loader.Load()
	if err == nil {
		if context, ok := config.Contexts[config.CurrentContext]; ok {
			if context.Namespace != "" {
				return context.Namespace
			}
		}
	}
	return "default"
}

// EnsureNamespaceOrDie makes sure the given namespace exists.
func EnsureNamespaceOrDie(namespaces corev1.NamespaceInterface, namespace string) {
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err := namespaces.Create(ns)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			panic(err)
		}
	}
}

// CreateNamespaceOrDie creates a new unique namespace from stem
func CreateNamespaceOrDie(namespaces corev1.NamespaceInterface, stem string) string {
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: stem}}
	ns, err := namespaces.Create(ns)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			panic(err)
		}
	}
	return ns.Name
}

// ConfigureSeedDefaults sets seed-defaults
func ConfigureSeedDefaults(configmaps corev1.ConfigMapInterface) {
	config := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "seed-defaults",
		},
		Data: map[string]string{
			"org":           org,
			"space":         space,
			"region":        region,
			"resourceGroup": resourceGroup,
		},
	}
	configmaps.Create(config)
}

// ConfigureSeedSecret sets seed-secret and seed-secret-tokens
func ConfigureSeedSecret(secrets corev1.SecretInterface) {
	config := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "seed-secret",
		},
		Data: map[string][]byte{
			"api-key": []byte(apikey),
		},
	}
	secrets.Create(config)

	config = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "seed-secret-tokens",
		},
		Data: map[string][]byte{
			"uaa_token":         []byte(uaaAccessToken),
			"uaa_refresh_token": []byte(uaaRefreshToken),
		},
	}
	secrets.Create(config)
}

// DeleteNamespace deletes a kube namespace. Wait for all resources to be really gone.
func DeleteNamespace(namespaces corev1.NamespaceInterface, namespace string) {
	namespaces.Delete(namespace, &metav1.DeleteOptions{})

	// Wait until it's really gone
	for retry := 60; retry >= 60; retry-- {
		_, err := namespaces.Get(namespace, metav1.GetOptions{})
		if err != nil {
			return
		}
		time.Sleep(time.Second)
	}
}
