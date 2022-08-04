#!/bin/bash

[ $# -lt 3 ] && { echo "Usage: $0 <source_ns> <envoy_version> <install_dir>" ; exit 1; }

deploy_and_test() {
  source_ns=$1
  envoy_version=$2
  install_dir=$3

  #Install the routing policy

  kubectl apply -f $install_dir/yaml/rp.yaml -n $source_ns


  sleep 15

   #Test, expecting envoy filter with correct version to be present in istio-system namespace
  output=($(kubectl get envoyfilters.networking.istio.io -n istio-system | grep dynamicrouting| grep $envoy_version))


  if [[ "$output" != "" ]]; then
    echo "PASS"
    return 0
  else
    echo "FAIL" . $output
    kubectl get envoyfilters -n istio-system
    kubectl get pod --all-namespaces
    kubectl get dependencies --all-namespaces
    return 1
  fi
  kubectl delete -f $install_dir/yaml/rp.yaml -n $source_ns
}

export -f deploy_and_test
timeout 90s bash -c "until deploy_and_test $1 $2 $3; do sleep 2; done"
if [[ $? -eq 124 ]]
then
  exit 1
fi


