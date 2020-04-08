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

// ResourceContext defines the CloudFoundry context and resource group
type ResourceContext struct {
	// +optional
	Org string `json:"org,omitempty"`
	// +optional
	Space string `json:"space,omitempty"`
	// +optional
	Region string `json:"region,omitempty"`
	// +optional
	ResourceGroup string `json:"resourcegroup,omitempty"`
	// +optional
	ResourceGroupID string `json:"resourcegroupid,omitempty"`
	// +optional
	ResourceLocation string `json:"resourcelocation,omitempty"`
	// +optional
	User string `json:"user,omitempty"`
}

// BindingFrom specifies the binding object
// Used by those objects that refer to Binding
type BindingFrom struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}
