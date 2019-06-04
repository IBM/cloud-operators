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

if [ -n "${SKIP_K8S_TOOLS}" ]; then
    exit 0
fi

if [ -z ${KUBECTL_VERSION+x} ]; then
    KUBECTL_VERSION=v1.10.0
fi

if [ -z ${KB_VERSION+x} ]; then
    KB_VERSION=1.0.5
fi

if [ -z ${KUSTOMIZE_VERSION+x} ]; then
    KUSTOMIZE_VERSION=1.0.10
fi

echo "installing kubectl"
curl -LO https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl
chmod +x ./kubectl
sudo mv ./kubectl /usr/local/bin/

echo "installing kubebuilder"
curl -SL "https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KB_VERSION}/kubebuilder_${KB_VERSION}_linux_amd64.tar.gz" | tar xz
sudo mv kubebuilder_${KB_VERSION}_linux_amd64 /usr/local/kubebuilder
export KUBEBUILDER_ASSETS=/usr/local/kubebuilder/bin
sudo ln -s ${KUBEBUILDER_ASSETS}/kubebuilder /usr/local/bin/kubebuilder
kubebuilder version

echo "installing kustomize"
curl -OL "https://github.com/kubernetes-sigs/kustomize/releases/download/v${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_linux_amd64"
sudo chmod +x kustomize_${KUSTOMIZE_VERSION}_linux_amd64
sudo mv kustomize_${KUSTOMIZE_VERSION}_linux_amd64 /usr/local/bin/kustomize
kustomize version