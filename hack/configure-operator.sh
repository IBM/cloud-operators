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
# configure-operator.sh
#
# By default, this script installs the IBM Cloud Operator from the latest release.
# It attempts to pick up as much as it can from the 'ibmcloud' CLI target context when configuring the operator.
# If an API key is not provided, one is generated.
#
# For full usage information, run with the -h flag provided.

# Exit if any statement has a non-zero exit code
set -e
# Fail a pipe if any of the commands fail
set -o pipefail
# Enable advanced pattern matching. Used in trim()
shopt -s extglob

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

VALID_ACTIONS="install, remove, store-creds"
usage() {
    cat >&2 <<EOT
Usage: $(basename "$0") [-h] [-v VERSION] [ACTION]

    -h            Shows this help message.
    -v VERSION    Uses the given semantic version (e.g. 1.2.3) to install or uninstall. Default is latest.

    ACTION        What action to perform. Options: $VALID_ACTIONS. Default is store-creds.

EOT
}

# trim trims whitespace characters from both ends of the passed args. If no args, uses stdin.
trim() {
    local s="$*"
    if [[ -z "$s" ]]; then
        s=$(cat -)  # if no args, read from stdin
    fi
    s="${s##*( )}"
    s="${s%%*( )}"
    printf '%s' "$s"
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

# valid_semver returns 0 if $1 is a valid semver number. Only allows MAJOR.MINOR.PATCH format.
valid_semver() {
    local version=$1
    local semver_pattern='^([0-9]+)\.([0-9]+)\.([0-9]+)$'
    if [[ "$version" =~ $semver_pattern ]]; then
        return 0
    fi
    return 1
}

# compare_semver prints 0 if the semver $1 is equal to $2, -1 for $1 < $2, and 1 for $1 > $2
compare_semver() {
    local semver_pattern='([0-9]+)\.([0-9]+)\.([0-9]+)'
    local semver1=(0 0 0)
    local semver2=(0 0 0)
    if [[ "$1" =~ $semver_pattern ]]; then
        semver1=("${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}")
    fi
    if [[ "$2" =~ $semver_pattern ]]; then
        semver2=("${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}")
    fi

    if (( ${semver1[0]} != ${semver2[0]} )); then
        if (( ${semver1[0]} > ${semver2[0]} )); then
            echo 1
            return
        fi
        echo -1
        return
    fi
    if (( ${semver1[1]} != ${semver2[1]} )); then
        if (( ${semver1[1]} > ${semver2[1]} )); then
            echo 1
            return
        fi
        echo -1
        return
    fi
    if (( ${semver1[2]} != ${semver2[2]} )); then
        if (( ${semver1[2]} > ${semver2[2]} )); then
            echo 1
            return
        fi
        echo -1
        return
    fi
    echo 0
}

# store_creds ensures an API key Secret and operator ConfigMap are set up
store_creds() {
    if [[ -z "$IBMCLOUD_API_KEY" ]]; then
        local key_output=$(ibmcloud iam api-key-create ibmcloud-operator-key -d "Key for IBM Cloud Operator" --output json)
        IBMCLOUD_API_KEY=$(json_grep apikey <<<"$key_output")
    fi
    IBMCLOUD_API_KEY=$(trim "$IBMCLOUD_API_KEY")
    local target=$(ibmcloud target --output json)
    local region=$(json_grep_after region name <<<"$target")
    if [[ -z "$region" ]]; then
        error 'Region must be set. Run `ibmcloud target -r $region` and try again.'
        return 2
    fi
    local b64_region=$(printf "$region" | base64)
    local b64_apikey=$(printf "$IBMCLOUD_API_KEY" | base64)

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

    local org=$(json_grep_after org name <<<"$target")
    local space=$(json_grep_after space name <<<"$target")
    local resource_group=$(json_grep_after resource_group name <<<"$target")
    local resource_group_id=$(json_grep_after resource_group guid <<<"$target")
    if [[ -z "$resource_group_id" ]]; then
        error 'Resource group must be set. Run `ibmcloud target -g $resource_group` and try again.'
        return 2
    fi
    local user=$(json_grep_after user display_name <<<"$target")

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
}

# release_action installs or uninstalls the given version
# First arg is the action (apply, delete) and second arg is the semantic version
release_action() {
    local action=$1
    local version=$2

    local download_version=$version
    if [[ "$version" != latest ]]; then
        download_version="tags/v${version}"
    fi
    local release=$(curl -H 'Accept: application/vnd.github.v3+json' "https://api.github.com/repos/IBM/cloud-operators/releases/$download_version")
    local urls=$(json_grep browser_download_url -1 <<<"$release")
    local file_urls=()
    while read -r url; do
        if ! [[ "$url" =~ package.yaml|clusterserviceversion.yaml ]]; then
            file_urls+=("$url")
        fi
    done <<<"$urls"

    local assets=$(fetch_assets "${file_urls[@]}")

    if [[ "$action" == apply ]]; then
        # Apply specially prefixed resources first. Typically these are namespaces and services.
        for f in "$assets"/*; do
            case "$(basename "$f")" in
                ~g_* | g_*)
                    echo "Installing pre-requisite resource: $f"
                    kubectl apply -f "$f"
                    rm "$f"  # Do not reprocess
                    ;;
                monitoring.*)
                    if ! kubectl apply -f "$f"; then
                        # Bypass failures on missing Prometheus Operator CRDs
                        error Failed to install monitoring, skipping...
                        error Install the Prometheus Operator and re-run this script to include monitoring.
                    fi
                    rm "$f"  # Do not reprocess
                    ;;
            esac
        done
    fi

    kubectl "$action" -f "$assets"
}


## Validate args

VERSION=latest
while getopts "hv:" opt; do
    case "$opt" in 
        h)
            usage
            exit 0
            ;;
        v)
            if ! valid_semver "$OPTARG"; then
                error "Invalid semver: $OPTARG"
                usage
                exit 2
            fi
            VERSION=$OPTARG
            ;;
    esac
done
shift $((OPTIND-1))

ACTION=${1:-store-creds}

## If version is pre-0.2.x, then run the old install scripts directly and exit.

if [[ "$VERSION" != latest && "$(compare_semver "$VERSION" 0.2.0)" == -1 ]]; then
    # This back-compatible installer runs in the style of v0.1.x's installer, but pulls v0.1.x's source code instead of latest.
    pushd "$TMPDIR"
    download_version=0.1.11
    tmpzip="${TMPDIR}/v${download_version}.zip"
    curl -sL "https://github.com/IBM/cloud-operators/archive/v${download_version}.zip" > "$tmpzip"
    unzip -qq "$tmpzip"
    pushd "cloud-operators-${download_version}"
    case "$ACTION" in
        store-creds)
            ./hack/config-operator.sh
            ;;
        install)
            ./hack/config-operator.sh
            kubectl apply -f "./releases/v${VERSION}"
            ;;
        remove)
            ./hack/uninstall-operator.sh
            ;;
    esac
    exit 0
fi

## Run the selected action

case "$ACTION" in
    store-creds)
        store_creds
        ;;
    install)
        # Only run for vanilla Kubernetes. OpenShift uses Operator Hub installer.
        store_creds
        release_action apply "$VERSION"
        ;;
    remove)
        release_action delete "$VERSION"
        ;;
    *)
        echo "Invalid action: $ACTION" >&2
        echo "Valid actions: $VALID_ACTIONS"
        exit 2
esac

