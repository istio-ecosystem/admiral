#!/bin/bash

[ $# -lt 1 ] && { echo "Usage: $0 <k8s_version>" ; exit 1; }

k8s_version=$1

if [[ $IS_LOCAL == "false" ]]; then
  sudo -E minikube start --vm-driver=none --cpus 6 --memory 6144 --kubernetes-version=$k8s_version &> $HOME/minikube.log 2>&1 < /dev/null
else
  minikube start --memory=4096 --cpus=4 --kubernetes-version=$k8s_version --vm-driver "virtualbox"
  #label node for locality load balancing
  kubectl label nodes minikube failure-domain.beta.kubernetes.io/region=us-west-2
fi