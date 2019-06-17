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

BLUEMIX_RESOURCE_GROUP="${BLUEMIX_RESOURCE_GROUP:-default}"

if [ -z $BLUEMIX_API_KEY ]; then
    echo "missing BLUEMIX_API_KEY. Aborting"
    exit 1
fi

if [ -z $BLUEMIX_ORG ]; then
    echo "missing BLUEMIX_ORG. Aborting"
    exit 1
fi

if [ -z $BLUEMIX_SPACE ]; then
    echo "missing BLUEMIX_SPACE. Aborting"
    exit 1
fi

if [ -z $BLUEMIX_REGION ]; then
    echo "missing BLUEMIX_REGION. Aborting"
    exit 1
fi

bx login -a api.ng.bluemix.net --apikey ${BLUEMIX_API_KEY} -o $BLUEMIX_ORG

if ! bx account space $BLUEMIX_SPACE ; then
    bx account space-create $BLUEMIX_SPACE
fi
bx target -s $BLUEMIX_SPACE -g $BLUEMIX_RESOURCE_GROUP

# set +e

actions=($(bx wsk action list))

len=${#actions[@]}
for (( i=1; i<len; i+=3 ))
do
    bx wsk action delete ${actions[$i]}
done

pkgs=($(bx wsk package list))
len=${#pkgs[@]}
for (( i=1; i<len; i+=2 ))
do
    bx wsk package delete ${pkgs[$i]}
done

rules=($(bx wsk rule list))
len=${#rules[@]}
for (( i=1; i<len; i+=3 ))
do
    bx wsk rule delete ${rules[$i]}
done

triggers=($(bx wsk trigger list))
len=${#triggers[@]}
for (( i=1; i<len; i+=2 ))
do
    bx wsk trigger delete ${triggers[$i]}
done
