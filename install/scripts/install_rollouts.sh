#!/bin/bash

if [ -z "$KUBECONFIG" ]
then
    echo "\$KUBECONFIG is not set"
    exit 1
elif [[ $KUBECONFIG == *"ppd"* || $KUBECONFIG == *"prd"* || $KUBECONFIG == *"prod"* ]]
then
    echo "\$KUBECONFIG is not for the dev cluster"
    exit 1
fi

echo "**********Installing rollouts *********"
kubectl create namespace argo-rollouts
kubectl apply -n argo-rollouts -f https://raw.githubusercontent.com/argoproj/argo-rollouts/stable/manifests/install.yaml
kubectl rollout status deployment argo-rollouts -n argo-rollouts
echo "****Rollouts installed*******"
