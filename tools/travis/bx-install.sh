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

if [ -n "${INSTALL_IBMCLOUD}" ]; then
    exit 0
fi

echo "installing ibmcloud"
curl -fsSL https://clis.ng.bluemix.net/install/linux | sh

echo "installing Cloud Functions plugin"
ibmcloud plugin install Cloud-Functions -r Bluemix -f
