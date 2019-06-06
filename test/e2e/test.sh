#!/bin/bash
#
# Copyright 2017-2018 IBM Corporation
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

set -e


ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")"/../.. && pwd)
cd $ROOT

KUBE_ENV=${KUBE_ENV:=default}

source hack/lib/object.sh
source hack/lib/utils.sh

if [ "${KUBE_ENV}" = "local" ]; then
    u::header "building docker image"
    docker build . -t local/openwhisk-operator
fi

u::header "installing CRDs, operators and secrets"

kustomize build config/crds | kubectl apply -f -
kustomize build config/${KUBE_ENV} | kubectl apply -f -

cd $ROOT/test/e2e

source ./test-hello.sh
source ./test-doc.sh

function cleanup() {
  set +e
  u::header "cleaning up..."

  # td::cleanup
  kubectl delete secret seed-defaults-owprops
}
trap cleanup EXIT

. ./wskprops-secrets.sh

u::header "running tests"

td::run
th::run

u::report_and_exit