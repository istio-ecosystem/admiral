#!/bin/bash

[ $# -lt 2 ] && { echo "Usage: $0 <k8s_version> <vm_driver>" ; exit 1; }

k8s_version=$1
vm_driver=$2

minikube start --memory=8192 --cpus=4 --kubernetes-version=$k8s_version --vm-driver $vm_driver