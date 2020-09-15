#!/bin/bash

[ $# -lt 2 ] && { echo "Usage: $0 <k8s_version> <vm_driver>" ; exit 1; }

k8s_version=$1
vm_driver=$2

minikube start --memory=2048 --cpus=2 --kubernetes-version=$k8s_version --vm-driver --vm-driver=none
export KUBECONFIG=${HOME}/.kube/config