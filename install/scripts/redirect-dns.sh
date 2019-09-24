#!/usr/bin/env bash


CORE_DNS_IP=$(kubectl get svc istiocoredns -n istio-system -o jsonpath='{.spec.clusterIP}')

echo "Istio coredns is hosted at: $CORE_DNS_IP"

export WORK_DIR=$(pwd)
export CORE_DNS_CM=/tmp/coredns-cm.yaml

cat <<EOF > ${CORE_DNS_CM}
apiVersion: v1
data:
  Corefile: |
    .:53 {
        errors
        health
        kubernetes cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           upstream
           fallthrough in-addr.arpa ip6.arpa
        }
        prometheus :9153
        proxy . /etc/resolv.conf
        cache 30
        loop
        reload
        loadbalance
    }
    global:53 {
        errors
        cache 30
        forward . ${CORE_DNS_IP}
    }
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
EOF

kubectl apply -f $CORE_DNS_CM

rm -rf $CORE_DNS_CM

kubectl get pods -n kube-system -oname |grep coredns |xargs kubectl delete -n kube-system



