#!/bin/bash

[ $# -lt 1 ] && { echo "Usage: $0 <istio_version>" ; exit 1; }

rm -rf "istio-$istio_version"*
minikube delete