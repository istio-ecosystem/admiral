#!/bin/bash

if [ -z "$ADMIRAL_HOME" ]
then
      echo "\$ADMIRAL_HOME is not set"
      exit 1
fi

if [ -z "$KUBECONFIG" ]
then
      echo "\$KUBECONFIG is not set"
      exit 1
fi

cd $ADMIRAL_HOME

make gen-yaml

kubectl apply -f $ADMIRAL_HOME/out/yaml/remotecluster.yaml

kubectl apply -f $ADMIRAL_HOME/out/yaml/demosinglecluster.yaml

kubectl delete deployment admiral -n admiral

$ADMIRAL_HOME/install/scripts/cluster-secret.sh $KUBECONFIG $KUBECONFIG admiral

kubectl apply -f $ADMIRAL_HOME/out/yaml/sample.yaml

kubectl apply -f $ADMIRAL_HOME/out/yaml/sample_dep.yaml