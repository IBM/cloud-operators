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

package event

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rcontext "github.com/ibm/cloud-operators/pkg/context"
)

// CreateEvent takes the representation of a event and creates it.
func CreateEvent(ctx rcontext.Context, obj metav1.Object, reason, message, kind, component string) (result *v1.Event, err error) {
	event := makeEvent(reason, message, kind, component, obj)
	if err := ctx.Client().Create(ctx, event); err != nil {
		return nil, err
	}
	return event, err
}

func makeEvent(reason, message, kind, component string, metadata metav1.Object) *v1.Event {
	involvedObject := v1.ObjectReference{
		Kind:       kind,
		Name:       metadata.GetName(),
		Namespace:  metadata.GetNamespace(),
		UID:        metadata.GetUID(),
		APIVersion: "v1",
		//FieldPath:  "spec.containers[2]", // this might be used for further details
	}
	eventTime := metav1.Now()
	event := &v1.Event{
		Reason:  reason,
		Message: message,
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    involvedObject.Namespace,
			GenerateName: involvedObject.Name,
		},
		InvolvedObject: involvedObject,
		Source: v1.EventSource{
			Component: component,
		},
		Count:          1,
		FirstTimestamp: eventTime,
		LastTimestamp:  eventTime,
		Type:           v1.EventTypeNormal,
	}
	return event
}
