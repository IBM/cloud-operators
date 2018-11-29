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
	"io/ioutil"
	"time"

	yaml2 "github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	rcontext "github.com/ibm/cloud-operators/pkg/context"
)

// PostInNs the object in the context namespace
func PostInNs(context rcontext.Context, obj runtime.Object, async bool, delay time.Duration) runtime.Object {
	obj.(metav1.ObjectMetaAccessor).GetObjectMeta().SetNamespace(context.Namespace())
	return post(context, obj, async, delay)
}

// Post the object
func post(context rcontext.Context, obj runtime.Object, async bool, delay time.Duration) runtime.Object {
	done := make(chan bool)

	go func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		err := context.Client().Create(context, obj)
		if err != nil {
			panic(err)
		}
		done <- true
	}()

	if !async {
		<-done
	}
	return obj
}

func deleteObject(context rcontext.Context, obj runtime.Object, async bool) {
	done := make(chan bool)

	go func() {
		err := context.Client().Delete(context, obj)
		if err != nil {
			panic(err)
		}
		done <- true
	}()

	if !async {
		<-done
	}
}

// LoadObject loads the YAML spec into obj
func LoadObject(filename string, obj runtime.Object) runtime.Object {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	yaml2.Unmarshal(bytes, obj)
	return obj
}
