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
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	context "github.com/ibm/cloud-operators/pkg/context"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"
)

var slog = logf.Log.WithName("openwhisk")

// SetStatusToPending sets the status to Pending
func SetStatusToPending(context context.Context, client client.Client, obj runtime.Object, format string, args ...interface{}) error {
	status := resv1.GetStatus(obj)
	if status == nil || status.GetState() != resv1.ResourceStatePending {
		resv1.SetStatus(obj, resv1.ResourceStatePending, format, args...)
		if err := client.Update(context, obj); err != nil {
			slog.Info("failed setting status (retrying)", "error", err)
			return err
		}
	}
	return nil
}

// ShouldRetry returns true when the error is recoverable
func ShouldRetry(resp *http.Response, err error) bool {
	return true
}

// ShouldRetryFinalize returns true when the error is recoverable
func ShouldRetryFinalize(err error) bool {
	if strings.Contains(err.Error(), "The requested resource does not exist.") {
		return false
	}
	slog.Info("could not finalize entity deletion. (retrying)", "error", err)
	return true
}
