#!/bin/bash

[ $# -lt 2 ] && { echo "Usage: $0 <k8s_version> <vm_driver>" ; exit 1; }

k8s_version=$1
vm_driver=$2
printenv IS_LOCAL
#minikube start --memory=8192 --cpus=4 --kubernetes-version=$k8s_version --vm-driver "virtualbox"
#sudo -E minikube start --vm-driver=none --cpus 2 --memory 2048 --kubernetes-version=$k8s_version &> $HOME/minikube.log 2>&1 < /dev/null
#
if [[ $IS_LOCAL == "false" ]]; then
  sudo -E minikube start --vm-driver=none --cpus 2 --memory 2048 --kubernetes-version=$k8s_version &> $HOME/minikube.log 2>&1 < /dev/null
else
  minikube start --memory=8192 --cpus=4 --kubernetes-version=$k8s_version --vm-driver "virtualbox"
fi

export KUBECONFIG=${HOME}/.kube/config