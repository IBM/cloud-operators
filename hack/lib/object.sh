#!/usr/bin/env bash
#
# Copyright 2019 IBM Corporation
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


# wait for the operator to be ready
function object::wait_operator_ready() {
    printf "Checking operator status .."
    until [ "$(kubectl -n ibmcloud-operators get po | grep 'Running' | awk '{print $3}')" == "Running" ]; do
        printf "."
        sleep 2
    done
    printf $CHECKMARK
    echo ""
}

# wait for resource to be online
function object::wait_resource_online() {
    local kind="$1"
    local name="$2"
    local retries="$3"

    printf "waiting for $kind $name to be online ."
    local i
    for i in $(seq 1 "$retries"); do
        if [ "$(kubectl get $kind $name -o=jsonpath='{.status.state}')" == "Online" ]; then
            printf $CHECKMARK
            echo ""
            return 0
        fi
        printf "."
        sleep 2
    done

    printf "timeout $CROSSMARK"
    echo ""
    return 1
}

# checks that a given resource has been created
function object::check_resource_created() {
    local kind="$1"
    local name="$2"

    set +e
    kubectl get $kind $name  > /dev/null 2>&1 
    rc=$? 
    set -e  
   
    if [ $rc -eq 0 ]
    then
        printf $CHECKMARK
        echo ""
        return 0
    fi

    printf $CROSSMARK
    echo ""
    return 1
}    

# wait for function to be online
function object::wait_function_online() {
    local name="$1"
    local retries="$2"

    object::wait_resource_online "functions.ibmcloud.ibm.com" $name $retries
}

# wait for package to be online
function object::wait_package_online() {
    local name="$1"
    local retries="$2"

    object::wait_resource_online "packages.ibmcloud.ibm.com" $name $retries
}

# wait for trigger to be online
function object::wait_trigger_online() {
    local name="$1"
    local retries="$2"

    object::wait_resource_online "triggers.ibmcloud.ibm.com" $name $retries
}

# wait for rule to be online
function object::wait_rule_online() {
    local name="$1"
    local retries="$2"

    object::wait_resource_online "rules.ibmcloud.ibm.com" $name $retries
}

# wait for service to be online
function object::wait_service_online() {
    local name="$1"
    local retries="$2"

    object::wait_resource_online "services.ibmcloud.ibm.com" $name $retries
}

# wait for binding to be online
function object::wait_binding_online() {
    local name="$1"
    local retries="$2"

    object::wait_resource_online "bindings.ibmcloud.ibm.com" $name $retries
}
