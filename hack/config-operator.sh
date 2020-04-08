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

if [[ -z "${IC_APIKEY}" ]]; then
  echo "*** Generating new APIKey"
  IC_APIKEY=$(ibmcloud iam api-key-create icop-key -d "Key for IBM Cloud Operator" | grep "API Key" | awk '{ print $3 }')
fi
IC_TARGET=$(ibmcloud target) \
IC_ORG=$(echo "$IC_TARGET" | grep Org | awk '{print $2}')  \
IC_USER=$(echo "$IC_TARGET" | grep User | awk '{print $2}')  \
IC_SPACE=$(echo "$IC_TARGET" | grep Space | awk '{print $2}') \
IC_REGION=$(echo "$IC_TARGET" | grep Region | awk '{print $2}') \
IC_GROUP=$(echo "$IC_TARGET" | grep 'Resource' | awk '{print $3}')
B64_APIKEY=$(echo -n $IC_APIKEY | base64)
B64_REGION=$(echo -n $IC_REGION | base64)

IC_GROUP_QUERY=$(ibmcloud resource group "$IC_GROUP") \
IC_GROUP_ID=$(echo "$IC_GROUP_QUERY" | grep ID | grep -v Account | awk '{print $2}')


cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: secret-ibm-cloud-operator
  labels:
    seed.ibm.com/ibmcloud-token: "apikey"
    app.kubernetes.io/name: ibmcloud-operator
  namespace: default
type: Opaque
data:
  api-key: $B64_APIKEY
  region: $B64_REGION
EOF

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-ibm-cloud-operator
  namespace: default
  labels:
    app.kubernetes.io/name: ibmcloud-operator
data:
  org: "${IC_ORG}"
  region: "${IC_REGION}"
  resourcegroup: "${IC_GROUP}"
  resourcegroupid: "${IC_GROUP_ID}"
  space: "${IC_SPACE}"
  user: "${IC_USER}"
EOF
