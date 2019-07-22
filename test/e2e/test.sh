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

set -e


ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")"/../.. && pwd)
cd $ROOT

KUBE_ENV=${KUBE_ENV:=default}

source hack/lib/object.sh
source hack/lib/utils.sh

if [ -z "$TRAVIS" ]
then
  u::header  "Not running in Travis, creating test branch..."
  git checkout -b e2e-test
fi  
export TAG=e2e-test
python hack/package.py v${TAG}
make docker-build
make docker-push

u::header "installing CRDs, operators and secrets"

hack/install-operator.sh
object::wait_operator_ready

cd $ROOT/test/e2e

source ./test-service-translator.sh
source ./test-binding-translator.sh

function cleanup() {
  set +e
  u::header "cleaning up..."

  $ROOT/hack/uninstall-operator.sh
  if [ -z "$TRAVIS" ]
  then
    u::header  "Not running in Travis, removing test branch..."
    # do some cleanup
    git checkout -f
    git checkout -
    git branch -D e2e-test
    rm git-rev 2> /dev/null
    rm  -r olm/ve2e-test/ 2> /dev/null
    rm -r releases/ve2e-test/ 2> /dev/null
  fi  
}
trap cleanup EXIT

u::header "running tests"

ts::run
tb::run

u::report_and_exit