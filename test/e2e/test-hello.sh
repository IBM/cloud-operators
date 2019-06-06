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

function th::run() {
    u::begin_testcase "should deploy the action hello in a package"

    kubectl apply -f hello.yaml >> /dev/null
    object::wait_function_online hello-world 10

    result=$(ibmcloud wsk action invoke -br hello-world-package/hello-world -p name John -p place Yorktown)
    u::assert_equal '{    "greeting": "Hello, John from Yorktown"}' "$result"

    u::end_testcase 
}