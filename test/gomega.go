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

package test

import (
	"fmt"

	"github.com/apache/incubator-openwhisk-client-go/whisk"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	Ω "github.com/onsi/gomega"

	context "github.com/ibm/cloud-operators/pkg/context"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"
)

// StartTestManager starts the manager
func StartTestManager(mgr manager.Manager) chan struct{} {
	stop := make(chan struct{})
	go func() {
		Ω.Expect(mgr.Start(stop)).NotTo(Ω.HaveOccurred())
	}()
	return stop
}

// GetObject gets the object from the store
func GetObject(context context.Context, obj runtime.Object) func() runtime.Object {
	return func() runtime.Object {
		key, err := client.ObjectKeyFromObject(obj)
		if err != nil {
			return nil
		}
		if err := context.Client().Get(context, key, obj); err != nil {
			return nil
		}
		return obj
	}
}

// GetState gets the object status from the store
func GetState(context context.Context, obj runtime.Object) func() string {
	return func() string {
		if obj := GetObject(context, obj)(); obj != nil {
			if status := resv1.GetStatus(obj); status != nil {
				return status.GetState()
			}
		}
		return ""
	}
}

// GetAction tries to get the action.
func GetAction(client *whisk.Client, actionName string) func() (*whisk.Action, error) {
	return func() (*whisk.Action, error) {
		action, _, err := client.Actions.Get(actionName, true)
		if err == nil {
			return action, nil
		}
		return nil, err
	}
}

// GetPackage tries to get the package.
func GetPackage(client *whisk.Client, pkgName string) func() (*whisk.Package, error) {
	return func() (*whisk.Package, error) {
		pkg, _, err := client.Packages.Get(pkgName)
		if err == nil {
			return pkg, nil
		}
		return nil, err
	}
}

// ActionInvocation invokes the given action, dropping the response for gomega compatibility
func ActionInvocation(wskclient *whisk.Client, actionName string, payload interface{}) (map[string]interface{}, error) {
	result, _, err := wskclient.Actions.Invoke(actionName, payload, true, true)
	return result, err
}

// CompositionInvocation invokes the given action
// func CompositionInvocation(client *ow.CompositionClient, name string, payload interface{}) (map[string]interface{}, error) {
// 	result, _, err := client.Invoke(name, payload)
// 	return result, err
// }

// GetActivation tries to get activations for the action.
func GetActivation(client *whisk.Client, actionName string) func() (*whisk.Activation, error) {
	return func() (*whisk.Activation, error) {
		activations, _, err := client.Activations.List(&whisk.ActivationListOptions{Since: ts})
		if err != nil {
			return nil, err
		}
		for _, activation := range activations {
			if activation.Name == actionName {
				return &activation, nil
			}
		}
		return nil, fmt.Errorf("No activation found for %s", actionName)
	}
}

// Result extracts response.result
func Result(httpResponse map[string]interface{}) map[string]interface{} {
	if httpResponse == nil {
		return nil
	}
	if response, ok := httpResponse["response"]; ok {
		if result, ok := response.(map[string]interface{})["result"]; ok {
			return result.(map[string]interface{})
		}
	}

	return nil
}
