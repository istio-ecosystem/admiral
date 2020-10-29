#!/bin/bash

[ $# -lt 3 ] && { echo "Usage: $0 <source_app> <source_ns> <destination_app>" ; exit 1; }

source=$1
source_ns=$2
dest=$3

#Deploy the grpc client pod to run requests against the grpc endpoint
kubectl apply -f ./grpc-client.yaml -n $source_ns

kubectl rollout status deploy grpc-client -n $source_ns

sleep 15

kubectl get pods -n $source_ns

kubectl logs --namespace=$source_ns $(kubectl get pod -l "app=$source" --namespace=$source_ns -o jsonpath='{.items[0].metadata.name}') -c $source

#Test, expecting to expect the grpc client to complete the requests with 100% success
output=$(kubectl logs --namespace=$source_ns $(kubectl get pod -l "app=$source" --namespace=$source_ns -o jsonpath='{.items[0].metadata.name}') -c $source | grep '"good": 10')

if [[ "$output" == *"good"* ]]; then
  echo "PASS"
  exit 0
else
  echo "FAIL" . $output
  exit 1
fi
