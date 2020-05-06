#!/bin/bash

[ $# -lt 3 ] && { echo "Usage: $0 <k8s_version> <istio_version> <admiral_install_dir>" ; exit 1; }

k8s_version=$1
istio_version=$2
install_dir=$3

source ./create_cluster.sh $k8s_version "virtualbox"
./install_istio.sh $istio_version "osx"
export KUBECONFIG=/Users/sa/.kube/config
./dns_setup.sh $install_dir
$install_dir/scripts/install_admiral.sh $install_dir
$install_dir/scripts/install_rollouts.sh
$install_dir/scripts/cluster-secret.sh $KUBECONFIG  $KUBECONFIG admiral
$install_dir/scripts/install_sample_services.sh $install_dir

#allow for stabilization of DNS and Istio SEs before running tests
sleep 10
./test1.sh "webapp" "sample" "greeting"
./test2.sh "webapp" "sample-rollout-bluegreen" "greeting"

./cleanup.sh $istio_version