#!/bin/bash

[ $# -eq 0 ] && { echo "Usage: $0 <install_dir>" ; exit 1; }

install_dir=$1

#verify KUBECONFIG is set
if [ -z "$KUBECONFIG" ]
then
    echo "\$KUBECONFIG is not set"
    exit 1
elif [[ $KUBECONFIG == *"ppd"* || $KUBECONFIG == *"prd"* || $KUBECONFIG == *"prod"* ]]
then
    echo "\$KUBECONFIG is not for a dev cluster"
    exit 1
fi

#Install admiral

kubectl delete svc -n admiral admiral-http
kubectl delete svc -n admiral admiral-metrics
kubectl apply -f $install_dir/yaml/remotecluster.yaml
kubectl apply -f $install_dir/yaml/demosinglecluster.yaml

#Verify admiral is running
kubectl rollout status deployment admiral -n admiral
