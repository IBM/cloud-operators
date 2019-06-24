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

IC_APIKEY=$(ibmcloud iam api-key-create icop-key -d "Key for IBM Cloud Operator" | grep "API Key" | awk '{ print $3 }')
IC_TARGET=$(ibmcloud target) \
IC_ORG=$(echo "$IC_TARGET" | grep Org | awk '{print $2}')  \
IC_SPACE=$(echo "$IC_TARGET" | grep Space | awk '{print $2}') \
IC_REGION=$(echo "$IC_TARGET" | grep Region | awk '{print $2}') \
IC_GROUP=$(echo "$IC_TARGET" | grep 'Resource' | awk '{print $3}')
B64_APIKEY=$(echo -n $IC_APIKEY | base64)
B64_REGION=$(echo -n $IC_REGION | base64)

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: seed-secret
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
  name: seed-defaults
  namespace: default
  labels:
    app.kubernetes.io/name: ibmcloud-operator
data:
  org: $IC_ORG
  region: $IC_REGION
  resourceGroup: $IC_GROUP
  space: $IC_SPACE
EOF

