#!/bin/bash

if [ -z "$KUBECONFIG" ]
then
    echo "\$KUBECONFIG is not set"
    exit 1
elif [[ $KUBECONFIG == *"ppd"* || $KUBECONFIG == *"prd"* || $KUBECONFIG == *"prod"* ]]
then
    echo "\$KUBECONFIG is not for the dev cluster"
    exit 1
fi

echo "**********Installing numaflow *********"
kubectl create ns numaflow-system
kubectl apply -n numaflow-system -f https://raw.githubusercontent.com/numaproj/numaflow/stable/config/install.yaml
kubectl apply -f https://raw.githubusercontent.com/numaproj/numaflow/stable/examples/0-isbsvc-jetstream.yaml
echo "****Numaflow installed*******"
