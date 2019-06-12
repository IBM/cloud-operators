#!/bin/bash
#
# Copyright 2017-2019 IBM Corporation
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

# Modified t
#SCRIPTS_HOME=$(dirname ${BASH_SOURCE})

# Delete the CRDs first, so controllers clean up their resources
#cd ${SCRIPTS_HOME}/../releases/latest 
#ls | grep v1alpha1 | xargs -I {} kubectl delete --wait -f {}

# TODO - it would be best to label each CRD - can it be done from kubebuilder?
kubectl delete --wait crd bindings.ibmcloud.ibm.com
#kubectl delete --wait crd buckets.ibmcloud.ibm.com
kubectl delete --wait crd services.ibmcloud.ibm.com
#kubectl delete --wait crd topics.ibmcloud.ibm.com
kubectl delete --wait crd functions.ibmcloud.ibm.com 
kubectl delete --wait crd invocations.ibmcloud.ibm.com
kubectl delete --wait crd packages.ibmcloud.ibm.com 
kubectl delete --wait crd rules.ibmcloud.ibm.com 
kubectl delete --wait crd triggers.ibmcloud.ibm.com
kubectl delete --wait crd esindices.ibmcloud.ibm.com


# Delete all clusterwide resources for the operator
kubectl delete clusterrole,clusterrolebinding -l app.kubernetes.io/name=ibmcloud-operator  

# delete all namespaced resources
kubectl delete ns -l app.kubernetes.io/name=ibmcloud-operator

# delete secrets/config maps left
kubectl delete secrets,configmaps -n default -l app.kubernetes.io/name=ibmcloud-operator