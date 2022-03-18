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
kubectl apply -f $install_dir/yaml/gtp_failover.yaml
kubectl apply -f $install_dir/yaml/sample-greeting-rollout-bluegreen.yaml
kubectl apply -f $install_dir/yaml/sample-greeting-rollout-canary.yaml
kubectl apply -f $install_dir/yaml/grpc.yaml

#Install the dependency CR

kubectl apply -f $install_dir/yaml/sample_dep.yaml

#wait for the deployments to come up
kubectl rollout status deployment greeting -n sample
kubectl rollout status deployment webapp -n sample


kubectl rollout status deployment webapp -n sample-rollout-bluegreen

checkRolloutStatus() {
  rolloutName=$1
  namespace=$2
  status=$(kubectl get rollout -n $2 $1 -o jsonpath="{.status.readyReplicas}")

  if [[ "$status" == "1" ]]; then
    return 0
  else
    echo "Waiting rollout $1 in $2  namespace is not in Running phase $status"
    return 1
  fi
}

export -f checkRolloutStatus 

timeout 180s bash -c "until checkRolloutStatus greeting sample-rollout-bluegreen ; do sleep 10; done"
if [[ $? -eq 124 ]]
  then
    exit 1
fi
# Update BlueGreen Rollout with new preview release
kubectl apply -f $install_dir/yaml/greeting_preview.yaml

#Verify that admiral created service names for 'greeting' service
checkse() {
  identity=$1
  num_ses=$(kubectl get serviceentry -n admiral-sync | grep $1 -c)

  if [ -z "$num_ses" ] || [ $num_ses -lt 1 ]
  then
        echo "No service entries created for $identity workload"
        return 1;
  else
        echo "Admiral did it's magic for $identity workload"
        return 0
  fi
}
export -f checkse
for identity in webapp greeting greeting.canary greeting.bluegreen grpc-server; do
  timeout 180s bash -c "until checkse $identity; do sleep 10; done"
  if [[ $? -eq 124 ]]
  then
    exit 1
  fi
done