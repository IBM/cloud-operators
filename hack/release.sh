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

command -v goreleaser >/dev/null 2>&1 || { echo "goreleaser (https://goreleaser.com/) is not installed. Aborting."; exit 1; }

TAG=$1
if [[ $TAG == "" ]]; then
  echo "usage: release.sh <tag>"
  exit 1
fi

echo Creating tag $TAG
git tag -a $TAG -m "Release $TAG"

goreleaser --rm-dist