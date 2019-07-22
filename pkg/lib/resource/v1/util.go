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
	"fmt"
	"strconv"

	"github.com/ibm/cloud-operators/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// ObjectMeta gets the resource ObjectMeta field
func ObjectMeta(obj runtime.Object) metav1.Object {
	return obj.(metav1.ObjectMetaAccessor).GetObjectMeta()
}

// Kind gets the resource kind
func Kind(obj runtime.Object) string {
	return obj.GetObjectKind().GroupVersionKind().Kind
}

// StatusAccessor gets access to Status
type StatusAccessor interface {
	GetStatus() Status
}

// Status represent a minimal set of status state
type Status interface {
	GetState() string
	SetState(state string)
	GetMessage() string
	SetMessage(message string)
}

func (r *ResourceStatus) GetStatus() Status {
	return r
}

func (r *ResourceStatus) GetState() string {
	return r.State
}
func (r *ResourceStatus) SetState(state string) {
	r.State = state
}

func (r *ResourceStatus) GetMessage() string {
	return r.Message
}

func (r *ResourceStatus) SetMessage(message string) {
	r.Message = message
}

// GetStatus gets the resource status field (if any)
func GetStatus(obj runtime.Object) Status {
	return obj.(StatusAccessor).GetStatus()
}

// SetStatus updates the object status. Returns the same object to enable call chaining
func SetStatus(obj runtime.Object, state string, format string, a ...interface{}) runtime.Object {
	status := GetStatus(obj)
	if len(a) == 0 {
		status.SetMessage(format)
	} else {
		status.SetMessage(fmt.Sprintf(format, a...))
	}
	status.SetState(state)
	return obj
}

// SeedGeneration gets the resource generation
func SeedGeneration(obj runtime.Object) int64 {
	if val, ok := ObjectMeta(obj).GetAnnotations()["seed-generation"]; ok {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}

	return 0
}

// Conditions returns resource list of conditions
func Conditions(obj runtime.Object) []Condition {
	if conditions := util.GetField(obj, "Conditions"); conditions != nil {
		return conditions.([]Condition)
	}
	return make([]Condition, 0)
}

// GetCondition returns the condition with the provided type.
func GetCondition(obj runtime.Object, condType string) *Condition {
	if conditions := Conditions(obj); conditions != nil {
		for _, condition := range conditions {
			if condition.Type == condType {
				return &condition
			}
		}
	}
	return nil
}

// SetCondition updates the resource condition to include the provided condition. If the condition that
// we are about to add already exists and has the same status then we are not going to update.
func SetCondition(obj runtime.Object, condition *Condition) runtime.Object {
	currentCond := GetCondition(obj, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status {
		return obj
	}
	newConditions := filterOutCondition(Conditions(obj), condition.Type)
	util.SetField(obj, "Conditions", append(newConditions, *condition))
	return obj
}

// RemoveCondition removes the condition with the provided type.
func RemoveCondition(obj runtime.Object, condType string) runtime.Object {
	util.SetField(obj, "Conditions", filterOutCondition(Conditions(obj), condType))
	return obj
}

// filterOutCondition returns a new slice of conditions without conditions with the provided type.
func filterOutCondition(conditions []Condition, condType string) []Condition {
	var newConditions []Condition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}
