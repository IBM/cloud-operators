#!/usr/bin/env bash
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


if [ -z ${KUBECTL_VERSION+x} ]; then
    KUBECTL_VERSION=v1.10.0
fi

if [ -z ${MINIKUBE_VERSION+x} ]; then
    MINIKUBE_VERSION=latest
fi

export MINIKUBE_WANTUPDATENOTIFICATION=false
export MINIKUBE_WANTREPORTERRORPROMPT=false
export CHANGE_MINIKUBE_NONE_USER=true
export MINIKUBE_HOME=$HOME

echo "installing kubectl"
curl -LO https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl
chmod +x ./kubectl
sudo mv ./kubectl /usr/local/bin/

echo "installing minikube"
curl -Lo minikube https://storage.googleapis.com/minikube/releases/${MINIKUBE_VERSION}/minikube-linux-amd64
chmod +x minikube
sudo mv minikube /usr/local/bin/

echo "starting minikube"
export KUBECONFIG=$HOME/.kube/config
sudo -E minikube start --vm-driver=none --bootstrapper=localkube --kubernetes-version=${KUBECTL_VERSION}

echo "update context"
# minikube update-context

echo "waiting minikube to be ready"
JSONPATH='{range .items[*]}{@.metadata.name}:{range @.status.conditions[*]}{@.type}={@.status};{end}{end}'; 

until kubectl get nodes -o jsonpath="$JSONPATH" 2>&1 | grep -q "Ready=True"; 
do 
  sleep 1; 
done
