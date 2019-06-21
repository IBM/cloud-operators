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

	"github.com/ibm/cloud-operators/pkg/lib/event"
	"k8s.io/apimachinery/pkg/runtime"

	rcontext "github.com/ibm/cloud-operators/pkg/context"
)

// PutAndEmit updates the object stus on the server and emits an event (on the same server) recording current object state
func PutAndEmit(ctx rcontext.Context, obj runtime.Object) error {
	if err := ctx.Client().Status().Update(ctx, obj); err != nil {
		return err
	}

	return EmitStatusEvent(ctx, obj)
}

// PutStatusAndEmit updates the object status on the server and emits an event (on the same server) recording current object state
func PutStatusAndEmit(ctx rcontext.Context, obj runtime.Object) error {
	if err := ctx.Client().Status().Update(ctx, obj); err != nil {
		return err
	}

	return EmitStatusEvent(ctx, obj)
}

// EmitStatusEvent creates an event representing the current object status
func EmitStatusEvent(ctx rcontext.Context, obj runtime.Object) error {
	status := GetStatus(obj)
	meta := ObjectMeta(obj)
	kind := Kind(obj)
	result, err := event.CreateEvent(ctx, meta, status.GetState(), status.GetMessage(), kind, "")
	if err != nil {
		return fmt.Errorf("Error creating event: %v, result: %v", err, result)
	}
	return nil
}
