#!/bin/bash
# Licensed to the Apache Software Foundation (ASF) under one or more contributor
# license agreements; and to You under the Apache License, Version 2.0.

set -x

if [ -z "${INSTALL_DIND}" ]; then
    exit 0
fi

# Install kubernetes-dind-cluster and boot it
wget https://cdn.rawgit.com/kubernetes-sigs/kubeadm-dind-cluster/master/fixed/dind-cluster-v$TRAVIS_KUBE_VERSION.sh -O $HOME/dind-cluster.sh && chmod +x $HOME/dind-cluster.sh && USE_HAIRPIN=true $HOME/dind-cluster.sh up

