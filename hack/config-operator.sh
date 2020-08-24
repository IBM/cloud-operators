#!/usr/bin/env bash
# Back-compatible installer. Simply runs the new installer instead.

echo 'WARNING: This installer is deprecated and may be removed in future version.' >&2
echo 'Please see here for the most up-to-date install script: https://github.com/ibm/cloud-operators' >&2
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh | bash
