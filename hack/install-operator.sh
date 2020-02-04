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

RELEASE="latest/"

which curl > /dev/null 2>&1 || (echo "Install curl first before running install-operator.sh" && exit 1)

if ! curl -s https://github.com/ > /dev/null
then
  echo "GitHub is down, or having issues. You won't be able to pull the master.zip from the repository."
  exit 1
fi

which unzip > /dev/null 2>&1 || (echo "Install unzip first before running install-operator.sh" && exit 1)

# check if running piped from curl
if [ -z ${BASH_SOURCE} ]; then
  echo "* Downloading install yaml..."
  rm -rf /tmp/ibm-operators && mkdir -p /tmp/ibm-operators
  cd /tmp/ibm-operators
  curl -sLJO https://github.com/IBM/cloud-operators/archive/master.zip
  unzip -qq cloud-operators-master.zip
  cd cloud-operators-master
  SCRIPTS_HOME=${PWD}/hack
else
  SCRIPTS_HOME=$(dirname ${BASH_SOURCE})
fi

# configure the operator
${SCRIPTS_HOME}/config-operator.sh

# install the operator
kubectl apply -f ${SCRIPTS_HOME}/../releases/${RELEASE}
