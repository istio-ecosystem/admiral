#!/bin/bash

[ $# -lt 3 ] && { echo "Usage: $0 <source_app> <source_ns> <destination_app>" ; exit 1; }

deploy_and_test() {
  source=$1
  source_ns=$2
  dest=$3
  install_dir=$4

  #Deploy the grpc client pod to run requests against the grpc endpoint
  kubectl apply -f $install_dir/yaml/grpc-client.yaml -n $source_ns

  kubectl rollout status deploy grpc-client -n $source_ns

  sleep 15

   #Test, expecting to expect the grpc client to complete the requests with 100% success
  output=($(kubectl logs --namespace=$source_ns $(kubectl get pod -l "app=$source" --namespace=$source_ns -o jsonpath='{.items[0].metadata.name}') -c $source | egrep -o '"good": ([0-9]+)'))

  if [[ "${output[1]}" -gt 0 ]]; then
    echo "PASS"
    return 0
  else
    echo "FAIL" . $output
    kubectl get serviceentry -n admiral-sync
    kubectl get pods -n $source_ns
    kubectl logs --namespace=$source_ns $(kubectl get pod -l "app=$source" --namespace=$source_ns -o jsonpath='{.items[0].metadata.name}') -c $source
    return 1
  fi
  kubectl delete deploy grpc-client -n $source_ns
}

export -f deploy_and_test
timeout 90s bash -c "until deploy_and_test $1 $2 $3 $4; do sleep 2; done"
if [[ $? -eq 124 ]]
then
  exit 1
fi


