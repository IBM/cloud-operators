#!/usr/bin/env bash
#
# Copyright 2020 IBM Corp. All Rights Reserved.
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

#
# install-operator.sh [ACTION]
#
# This script installs the IBM Cloud Operator from the latest release.
# It attempts to pick up as much as it can from the 'ibmcloud' CLI target context when configuring the operator.
# If an API key is not provided, one is generated.
#

# Exit if any statement has a non-zero exit code
set -e
# Fail a pipe if any of the commands fail
set -o pipefail

TMPDIR=$(mktemp -d)
trap "set -x; rm -rf '$TMPDIR'" EXIT

# error prints the arguments to stderr. If printing to a TTY, adds red color.
error() {
    if [[ -t 2 ]]; then
        printf '\033[1;31m%s\033[m\n' "$*" >&2
    else
        echo "$*" >&2
    fi
}

# json_grep assumes stdin is an indented JSON blob, then looks for a matching JSON key for $1.
# The value must be a string type.
#
# The first match is printed to stdout.
json_grep() {
    local pattern="\"$1\": *\"(.*)\",?\$"
    declare -i max_matches=${2:-1} matches=0
    while read -r line; do
        if [[ "$line" =~ $pattern ]]; then
            printf "${BASH_REMATCH[1]}"
            if (( max_matches == -1 || max_matches > 1 )); then
                echo # add new line
            fi
            matches+=1
            if (( matches >= max_matches && max_matches != -1 )); then
                break
            fi
        fi
    done
}

# json_grep_after runs json_grep for $2 only after finding a line matching $1
json_grep_after() {
    local after=$1
    local pattern=$2
    while read -r line; do
        if [[ "$line" =~ "$after" ]]; then
            json_grep "$pattern"
            break
        fi
    done
}

# fetch_assets retrieves the given URLs and saves them to a temporary directory. The directory is printed to stdout.
fetch_assets() {
    local file_urls=("$@")
    if [[ -n "$DEBUG_OUT" ]]; then
        # Use custom assets directory for debugging purposes.
        printf "$DEBUG_OUT"
        return
    fi

    pushd "$TMPDIR" >/dev/null
    xargs -P 10 -n1 curl --silent --location --remote-name <<<"${file_urls[@]}"
    echo "Downloaded:" >&2
    ls "$TMPDIR" >&2
    popd >/dev/null

    printf "$TMPDIR"
}

## Validate args

ACTION=${1:-apply}
case "$ACTION" in
    apply | delete) ;;
    *)
        echo "Invalid action: $ACTION" >&2
        echo "Valid actions: delete"
        exit 2
esac


## Ensure API key Secret and operator ConfigMap are set up

if ! kubectl get secret -n default secret-ibm-cloud-operator; then
    if [[ -z "$IBMCLOUD_API_KEY" ]]; then
        key_output=$(ibmcloud iam api-key-create ibmcloud-operator-key -d "Key for IBM Cloud Operator" --output json)
        IBMCLOUD_API_KEY=$(json_grep apikey <<<"$key_output")
    fi
    target=$(ibmcloud target --output json)
    b64_region=$(json_grep_after region name <<<"$target" | base64)
    b64_apikey=$(printf "$IBMCLOUD_API_KEY" | base64)

    kubectl apply -f - <<EOT
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
  api-key: $b64_apikey
  region: $b64_region
EOT
fi

if ! kubectl get configmap -n default config-ibm-cloud-operator; then
    target=$(ibmcloud target --output json)
    region=$(json_grep_after region name <<<"$target")
    org=$(json_grep_after org name <<<"$target")
    space=$(json_grep_after space name <<<"$target")
    resource_group=$(json_grep_after resource_group name <<<"$target")
    resource_group_id=$(json_grep_after resource_group guid <<<"$target")
    user=$(json_grep_after user display_name <<<"$target")

    kubectl apply -f - <<EOT
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-ibm-cloud-operator
  namespace: default
  labels:
    app.kubernetes.io/name: ibmcloud-operator
data:
  org: "${org}"
  region: "${region}"
  resourcegroup: "${resource_group}"
  resourcegroupid: "${resource_group_id}"
  space: "${space}"
  user: "${user}"
EOT
fi

## Install ibmcloud-operators

latest=$(curl -H 'Accept: application/vnd.github.v3+json' https://api.github.com/repos/IBM/cloud-operators/releases/latest)
urls=$(json_grep browser_download_url -1 <<<"$latest")
file_urls=()
while read -r url; do
    if ! [[ "$url" =~ package.yaml|clusterserviceversion.yaml ]]; then
        file_urls+=("$url")
    fi
done <<<"$urls"

assets=$(fetch_assets "${file_urls[@]}")
set +x

if [[ "$ACTION" == apply ]]; then
    # Apply specially prefixed resources first. Typically these are namespaces and services.
    for f in "$assets"/*; do
        case "$(basename "$f")" in
            ~g_* | g_*)
                echo "Installing pre-requisite resource: $f"
                kubectl apply -f "$f"
                rm "$f"  # Do not reprocess
                ;;
        esac
    done
fi

kubectl "$ACTION" -f "$assets"
