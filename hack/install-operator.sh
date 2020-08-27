#!/usr/bin/env bash
# Back-compatible installer. Simply runs the new installer instead.

echo 'WARNING: This installer is deprecated and may be removed in a future version.' >&2
echo 'Please see here for the most up-to-date install script: https://github.com/ibm/cloud-operators' >&2
echo >&2
LATEST_V0_1=0.1.11
echo "Installing v${LATEST_V0_1}..." >&2
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh | bash -s -- -v "$LATEST_V0_1" install
