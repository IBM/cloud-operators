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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	Ω "github.com/onsi/gomega"

	context "github.com/ibm/cloud-operators/pkg/context"
	resv1 "github.com/ibm/cloud-operators/pkg/types/apis/resource/v1"
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
