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

function tb::run() {
    u::begin_testcase "should deploy sample translator binding"

    kubectl apply -f binding-translator.yaml 
    object::wait_binding_online binding-translator 100
    object::check_resource_created secret binding-translator

    u::end_testcase 
}

function tb::cleanup() {
    kubectl delete -f binding-translator.yaml 
}