#!/bin/bash
#
# Copyright 2019 IBM Corp. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${SCRIPT_ROOT}; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ${GOPATH}/src/k8s.io/code-generator)}

vendor/k8s.io/code-generator/generate-groups.sh \
  deepcopy \
  github.com/ibm/cloud-operators/pkg/lib/resource/v1 \
  github.com/ibm/cloud-operators/pkg/lib \
  "resource:v1" \
  --output-base "${SCRIPT_ROOT}/../../.." \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt  

vendor/k8s.io/code-generator/generate-groups.sh \
  deepcopy \
  github.com/ibm/cloud-operators/pkg/lib/keyvalue/v1 \
  github.com/ibm/cloud-operators/pkg/lib \
  "keyvalue:v1" \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt   
