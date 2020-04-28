#!/usr/bin/env bash

function ver { printf "%03d%03d%03d%03d" $(echo "$1" | tr '.' ' '); }
target_coredns_version=1.6.7
target_coredns_image="docker.io/coredns/coredns:$target_coredns_version"
coredns_upgrade_skip_msg="Skipping coredns upgrade and redirection for admiral as this needs coredns version > 1.5.0"

CORE_DNS_IP=$(kubectl get svc istiocoredns -n istio-system -o jsonpath='{.spec.clusterIP}')

coredns_image=$(kubectl get deployment coredns -n kube-system -o=jsonpath='{$.spec.template.spec.containers[:1].image}')
coredns_ver=$(echo $coredns_image | awk -F: '{print $2}')
#if coredns version < 1.5.0, then upgrade to 1.6.7
if [ $(ver $coredns_ver) -lt $(ver 1.5.0) ]
then
    printf "Current codedns version: %s\n" "$coredns_ver"
    printf "Coredns will be upgraded to $target_coredns_image.\nDo you wish to proceed?\n"
    options="Please enter yes/Y/y or no/N/n"
    echo $options
    read -p "" yn
    case $yn in
        [Yy]* ) kubectl set image deployment/coredns -n kube-system coredns=$target_coredns_image; kubectl rollout status deployment/coredns -n kube-system;;
        * ) echo $coredns_upgrade_skip_msg; exit ;;
    esac
else
    echo "Running compatible coredns version: $coredns_ver"
fi

echo "Istio coredns is hosted at: $CORE_DNS_IP"

export WORK_DIR=$(pwd)
export CORE_DNS_CM=/tmp/coredns-cm.yaml

cat <<EOF > ${CORE_DNS_CM}
data:
  Corefile: |
    .:53 {
        errors
        health {
            lameduck 5s
        }
        ready
        kubernetes cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        prometheus :9153
        forward . /etc/resolv.conf
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
EOF

kubectl patch configmap/coredns -n kube-system --patch "$(cat $CORE_DNS_CM)"

rm -rf $CORE_DNS_CM

kubectl get pods -n kube-system -oname |grep coredns |xargs kubectl delete -n kube-system



