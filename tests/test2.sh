#!/bin/bash

[ $# -lt 3 ] && { echo "Usage: $0 <source_app> <source_ns> <destination_app>" ; exit 1; }

source=$1
source_ns=$2
dest=$3

#Test
output=$(kubectl exec --namespace=$source_ns -it $(kubectl get pod -l "app=$source" --namespace=$source_ns -o jsonpath='{.items[0].metadata.name}') -c $source -- curl -v "http://default.$dest.global" && echo "")

if [[ "$output" == *"Admiral"* ]]; then
  echo "PASS"
  exit 0
else
  echo "FAIL" . $output
  exit 1
fi
