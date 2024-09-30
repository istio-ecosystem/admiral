#!/bin/bash -e

## This test verifies if the Client Discovery (Mesh egress works as expected) works
## Validates the mesh endpoints for greeting are exported to job and numaflow types as mesh clients

#Test
output=$(kubectl get se stage.greeting.global-se -n admiral-sync -o yaml | grep sample-job)

if [[ "$output" == *"sample-job"* ]]; then
  echo "PASS"
else
  echo "FAIL"
  echo $output
  exit 1
fi

output=$(kubectl get se stage.greeting.global-se -n admiral-sync -o yaml | grep sample-monovertex)

if [[ "$output" == *"sample-monovertex"* ]]; then
  echo "PASS"
else
  echo "FAIL"
  echo $output
  exit 1
fi

exit 0
