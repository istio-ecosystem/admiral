#!/bin/bash

[ $# -eq 0 ] && { echo "Usage: $0 <install_dir>" ; exit 1; }

install_dir=$1

#verify KUBECONFIG is set
if [ -z "$KUBECONFIG" ]
then
      echo "\$KUBECONFIG is not set"
      exit 1;
fi

#Install test services

kubectl apply -f $install_dir/yaml/sample.yaml
kubectl apply -f $install_dir/yaml/sample-greeting-rollout-bluegreen.yaml

#Install the dependency CR

kubectl apply -f $install_dir/yaml/sample_dep.yaml

#wait for the deployments to come up
kubectl rollout status deployment greeting -n sample
kubectl rollout status deployment webapp -n sample


kubectl rollout status deployment webapp -n sample-rollout-bluegreen

#Wait for admiral's magic
sleep 5

#Verify that admiral created service names for 'greeting' service
num_ses=$(kubectl get serviceentry -n admiral-sync | grep "global-se" -c)

if [ -z "$num_ses" ] || [ $num_ses -lt 1 ]
then
      echo "No service entries created"
      exit 1;
else
      echo "Admiral did it's magic!"
      exit 0
fi