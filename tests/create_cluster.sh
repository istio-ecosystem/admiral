#!/bin/bash

[ $# -lt 1 ] && { echo "Usage: $0 <k8s_version> "; exit 1; }

k8s_version=$1
driver="--vm-driver=virtualbox"

# apple silicon does not support virtualbox
if [[ "$OSTYPE" == "darwin"* && $(uname -m) == 'arm64' ]]; then
  driver="--driver=docker"
fi

echo "Creating K8s cluster with version: $k8s_version"

if [[ $IS_LOCAL == "false" ]]; then
  sudo apt-get update
  sudo apt-get install -y conntrack
  sudo sysctl fs.protected_regular=0
  sudo -E minikube start --vm-driver=none --kubernetes-version=$k8s_version
else
  echo "Creating K8s cluster with virtualbox vm driver"
  minikube start --memory=4096 --cpus=4 --kubernetes-version=$k8s_version $driver
  #label node for locality load balancing
  kubectl label nodes minikube --overwrite failure-domain.beta.kubernetes.io/region=us-west-2
fi
