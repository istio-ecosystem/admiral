#!/usr/bin/env bash


CORE_DNS_IP=$(kubectl get svc istiocoredns -n istio-system -o jsonpath='{.spec.clusterIP}')

echo ${CORE_DNS_IP}

