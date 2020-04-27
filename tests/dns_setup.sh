#!/bin/bash

[ $# -lt 1 ] && { echo "Usage: $0 <admiral_install_dir>" ; exit 1; }

install_dir=$1

#verify KUBECONFIG is set
if [ -z "$KUBECONFIG" ]
then
      echo "\$KUBECONFIG is not set"
      exit 1;
fi

#Point hosts ending in global to be resolved by istio coredns
$install_dir/scripts/redirect-dns.sh