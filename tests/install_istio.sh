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

if [ $(ver $istio_version) -lt $(ver 1.8.6) ]
then
    echo "Istio version $istio_version is no longer officially supported by this version of Admiral"
    exit 1
else
    #install istio core with DNS proxying enabled and multicluster enabled
cat <<EOF > cluster1.yaml
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
spec:
  meshConfig:
    defaultConfig:
      proxyMetadata:
        # Enable Istio agent to handle DNS requests for known hosts
        # Unknown hosts will automatically be resolved using upstream dns servers in resolv.conf
        ISTIO_META_DNS_CAPTURE: "true"
  values:
    global:
      meshID: admiral1
      multiCluster:
        clusterName: admiral1
      network: admiral1
EOF

    "./istio-$istio_version/bin/istioctl" install -f cluster1.yaml -y
fi
rm -rf cluster1.yaml
kubectl rollout status deployment istiod -n istio-system

