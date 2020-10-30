#!/bin/bash

function ver { printf "%03d%03d%03d%03d" $(echo "$1" | tr '.' ' '); }

[ $# -lt 2 ] && { echo "Usage: $0 <istio_version> [osx|linux]" ; exit 1; }

istio_version=$1
os=$2

#Download & extract Istio

#Downloading istiofunction ver { printf "%03d%03d%03d%03d" $(echo "$1" | tr '.' ' '); }

wget "https://github.com/istio/istio/releases/download/$istio_version/istio-$istio_version-$os.tar.gz"

#Extracting istio
tar -xf "istio-$istio_version-$os.tar.gz"

#Create istio-system namespace

kubectl create ns istio-system

#Create k8s secret to be used by Citadel for mTLS cert generation
kubectl create secret generic cacerts -n istio-system --from-file="istio-$istio_version/samples/certs/ca-cert.pem" --from-file="istio-$istio_version/samples/certs/ca-key.pem" --from-file="istio-$istio_version/samples/certs/root-cert.pem" --from-file="istio-$istio_version/samples/certs/cert-chain.pem"

#Generate, install and verify Istio CRDs

if [ $(ver $istio_version) -lt $(ver 1.5.0) ]
then
    helm template "istio-$istio_version/install/kubernetes/helm/istio-init" --name istio-init --namespace istio-system | kubectl apply -f -
    #Make sure Istio crds are installed
    crds_count=0
    while [ $crds_count -lt 1 ]
    do
      crds_count=$(kubectl get crds | grep 'istio.io' | wc -l)
    done

    helm template "istio-$istio_version/install/kubernetes/helm/istio" --name istio --namespace istio-system -f "istio-$istio_version/install/kubernetes/helm/istio/example-values/values-istio-multicluster-gateways.yaml" | kubectl apply -f -

    #Verify that pilot is up and running
    kubectl rollout status deployment istio-pilot -n istio-system

    #Verify that sidecar injector is running
    kubectl rollout status deployment istio-sidecar-injector -n istio-system

elif [ $(ver $istio_version) -lt $(ver 1.6.0) ]
then
    "./istio-$istio_version/bin/istioctl" manifest apply -f "istio-$istio_version/install/kubernetes/operator/examples/multicluster/values-istio-multicluster-gateways.yaml" --set components.egressGateways[0].enabled=false --set addonComponents.prometheus.enabled=false
    #Verify that istiod is up and running
    kubectl rollout status deployment istiod -n istio-system
else
    "./istio-$istio_version/bin/istioctl" install -f "istio-$istio_version/manifests/examples/multicluster/values-istio-multicluster-gateways.yaml" --set components.egressGateways[0].enabled=false --set addonComponents.prometheus.enabled=false --set .values.global.proxy.resources.requests.memory=64Mi --set .values.global.proxy.resources.requests.cpu=50m
    #Verify that istiod is up and running
    kubectl rollout status deployment istiod -n istio-system
fi

#Deleting ingressgateway to make space in integration tests
#if [[ $IS_LOCAL == "false" ]]; then
#kubectl delete deployment istio-ingressgateway -n istio-system
#fi

# Delete envoy filter for translating `global` to `svc.cluster.local`
kubectl delete envoyfilter istio-multicluster-ingressgateway -n istio-system

