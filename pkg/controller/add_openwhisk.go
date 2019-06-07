/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"github.com/ibm/cloud-operators/pkg/controller/openwhisk/auth"
	"github.com/ibm/cloud-operators/pkg/controller/openwhisk/function"
	"github.com/ibm/cloud-operators/pkg/controller/openwhisk/invocation"
	"github.com/ibm/cloud-operators/pkg/controller/openwhisk/pkg"
	"github.com/ibm/cloud-operators/pkg/controller/openwhisk/rule"
	"github.com/ibm/cloud-operators/pkg/controller/openwhisk/trigger"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, function.Add, pkg.Add, rule.Add, trigger.Add,
		invocation.Add, auth.Add)
}
