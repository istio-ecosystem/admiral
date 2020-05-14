#!/bin/bash

echo "**********Installing rollouts *********"
kubectl create namespace argo-rollouts
kubectl apply -n argo-rollouts -f https://raw.githubusercontent.com/argoproj/argo-rollouts/stable/manifests/install.yaml
kubectl rollout status deployment argo-rollouts -n argo-rollouts
echo "****Rollouts installed*******"