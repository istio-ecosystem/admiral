#!/bin/bash -e

## This test verifies if the DependencyProxy created for the httpbin service
## to proxy the request through a "proxy" mesh service.
## The curl should validate httpbin external service is reachable
## through the proxy's virtualservice host

## Request Flow:
## GET http://stage.httpbin.foo/api/uuid =>
##     Istio intercepts and routes to VS host stage.httpbin.foo =>
##     VS host stage.httpbin.foo has a destination of SE host stage.httpbin.global:80 =>
##     SE host stage.httpbin.global:80 forwards to proxy.svc.cluster.local:80 =>
##     proxy.svc.cluster.local:80 which is an nginx gateway proxy, forwards the request to https://httpbin.org/uuid

[ $# -lt 3 ] && { echo "Usage: $0 <source_app> <source_ns> <destination_vs_host>" ; exit 1; }

source=$1
source_ns=$2
dest=$3

#Test
output=$(kubectl exec --namespace=$source_ns -it $(kubectl get pod -l "app=$source" --namespace=$source_ns -o jsonpath='{.items[0].metadata.name}') -c $source -- curl -v --resolve $dest:80:240.0.0.1 "http://$dest/api/uuid" && echo "")

if [[ "$output" == *"uuid"* ]]; then
  echo "PASS"
  exit 0
else
  echo "FAIL"
  echo $output
  exit 1
fi
