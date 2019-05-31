#!/bin/bash
#
# Copyright 2017-2019 IBM Corporation
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

SEED="latest/"

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

# check if running piped from curl
if [ -z ${BASH_SOURCE} ]; then
  echo "* Downloading install yaml..."
  rm -rf /tmp/ibm-operators && mkdir -p /tmp/ibm-operators
  cd /tmp/ibm-operators
  curl -sLJO https://${IBM_GITHUB_TOKEN}@github.ibm.com/seed/cloud-operators/archive/master.zip
  unzip -qq cloud-operators-master.zip
  cd cloud-operators-master
  SCRIPTS_HOME=${PWD}/hack
else
  SCRIPTS_HOME=$(dirname ${BASH_SOURCE})
fi

# install the operator
kubectl apply -f ${SCRIPTS_HOME}/../releases/${SEED}


# Add pull secret
# TODO - REMOVE THIS BEFORE PUSHING IT TO PUBLIC GITHUB !!!!!
X=eyJhdXRocyI6eyJyZWdpc3RyeS5uZy5ibHVlbWl4Lm5ldCI6eyJ1c2VybmFtZSI6InRva2VuIiwicGFzc3dvcmQiOiJleUpoYkdjaU9pSklVekkxTmlJc0luUjVjQ0k2SWtwWFZDSjkuZXlKcWRHa2lPaUkxTWpjNU1tSmhOaTFsTXpnNUxUVmhaamt0WW1ObE5pMHlZVGd4WW1NNE16VXdOemNpTENKcGMzTWlPaUp5WldkcGMzUnllUzV1Wnk1aWJIVmxiV2w0TG01bGRDSjkuNjYzT0o0YmFaR1FPUzZxcy1RVXNQdEFJbHVGYkRnTUI5WFZBb2ptZzFlcyIsImVtYWlsIjoic2VlZEB1cy5pYm0uY29tIiwiYXV0aCI6ImRHOXJaVzQ2WlhsS2FHSkhZMmxQYVVwSlZYcEpNVTVwU1hOSmJsSTFZME5KTmtscmNGaFdRMG81TG1WNVNuRmtSMnRwVDJsSk1VMXFZelZOYlVwb1Rta3hiRTE2WnpWTVZGWm9XbXByZEZsdFRteE9hVEI1V1ZSbmVGbHRUVFJOZWxWM1RucGphVXhEU25Cak0wMXBUMmxLZVZwWFpIQmpNMUo1WlZNMWRWcDVOV2xpU0Zac1lsZHNORXh0Tld4a1EwbzVMalkyTTA5S05HSmhXa2RSVDFNMmNYTXRVVlZ6VUhSQlNXeDFSbUpFWjAxQ09WaFdRVzlxYldjeFpYTT0ifX19
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: ibmcloud-operator-pullsecret
  namespace: ibmcloud-operators
  labels:
    app.kubernetes.io/name: ibmcloud-operator
type: kubernetes.io/dockerconfigjson  
data:
  .dockerconfigjson: $X
EOF