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

package v1

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	context "github.com/ibm/cloud-operators/pkg/context"
)

// NsDefaultConfigMap is the config map used to store namespace defaults
const NsDefaultConfigMap = "seed-defaults"

// GetContext returns default context if the input context is empty
func GetContext(ctx context.Context, context ResourceContext) (ResourceContext, error) {
	if context.Org == "" || context.Space == "" || context.Region == "" {
		return GetNsDefaults(ctx, context)
	}
	return context, nil
}

// GetNsDefaults provides namespace defaults such as org and space
func GetNsDefaults(ctx context.Context, context ResourceContext) (ResourceContext, error) {
	var cm v1.ConfigMap
	err := ctx.Client().Get(ctx, types.NamespacedName{Namespace: ctx.Namespace(), Name: NsDefaultConfigMap}, &cm)
	if err != nil {
		return ResourceContext{}, err
	}
	defaults := ResourceContext{
		Org:              cm.Data["org"],
		Space:            cm.Data["space"],
		Region:           cm.Data["region"],
		ResourceGroup:    context.ResourceGroup,
		ResourceLocation: context.ResourceLocation,
	}
	return defaults, nil
}
