#!/bin/bash

[ $# -eq 0 ] && { echo "Usage: $0 <install_dir>" ; exit 1; }

install_dir=$1

#verify KUBECONFIG is set
if [ -z "$KUBECONFIG" ]
then
      echo "\$KUBECONFIG is not set"
      exit 1;
fi

kubectl delete svc prometheus -n istio-system
kubectl apply -f $install_dir/yaml/prometheus.yaml