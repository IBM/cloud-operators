#!/bin/bash
#
# A script to fix CRD generations

# Note: script depends on line numbers so if anything is changed in the CRD generation it needs to be adjusted.

SCRIPTDIR=$(cd "$(dirname "${BASH_SOURCE[0]}" )" && pwd)
sed -i.bak "67s/.*//" $SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_service.yaml


# remove the .bak as they create issues with the releases
rm $SCRIPTDIR/../config/crds/*.yaml.bak
