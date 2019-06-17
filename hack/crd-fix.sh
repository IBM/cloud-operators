#!/bin/bash
#
# A script to fix CRD generations

# Note: script depends on line numbers so if anything is changed in the CRD generation it needs to be adjusted.

SCRIPTDIR=$(cd "$(dirname "${BASH_SOURCE[0]}" )" && pwd)

sed -i.bak  "137s/.*//" $SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_function.yaml
sed -i.bak  "69s/.*//"  $SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_invocation.yaml
sed -i.bak  "107s/.*//" $SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_invocation.yaml
sed -i.bak  "95s/.*//"  $SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_package.yaml
sed -i.bak  "95s/.*//"  $SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_trigger.yaml


# remove the .bak as they create issues with the releases
rm $SCRIPTDIR/../config/crds/*.yaml.bak
