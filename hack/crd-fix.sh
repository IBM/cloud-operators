#!/bin/bash
#
# A script to fix CRD generations

SCRIPTDIR=$(cd "$(dirname "${BASH_SOURCE[0]}" )" && pwd)
FILE_PATH=$SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_function.yaml
sed -i .bak "130s/.*//" $SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_function.yaml
sed -i .bak "62s/.*//" $SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_invocation.yaml
sed -i .bak "100s/.*//" $SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_invocation.yaml
sed -i .bak "88s/.*//" $SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_package.yaml
sed -i .bak "88s/.*//" $SCRIPTDIR/../config/crds/ibmcloud_v1alpha1_trigger.yaml

# remove the .bak as they create issues with the releases
rm $SCRIPTDIR/../config/crds/*.yaml.bak
