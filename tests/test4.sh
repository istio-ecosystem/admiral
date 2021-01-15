#!/bin/bash

[ $# -lt 2 ] && { echo "Usage: $0 <source_app> <source_ns>" ; exit 1; }

source=$1
source_ns=$2

#Delete the depolyment
kubectl delete deploy $1 -n $2

#wait for admiral to delete the service entry
sleep 5

#Test, expecting to expect the grpc client to complete the requests with 100% success
output=($(kubectl get se --namespace=admiral-sync | grep "stage.$1.global" | wc -l))

if [[ "${output}" -gt 0 ]]; then
  echo "FAIL"
  kubectl get se --namespace=admiral-sync
  exit 1
else
  echo "PASS"
  exit 0
fi
