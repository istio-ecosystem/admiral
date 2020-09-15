#!/bin/bash

[ $# -lt 3 ] && { echo "Usage: $0 <k8s_version> <istio_version> <admiral_install_dir>" ; exit 1; }

k8s_version=$1
istio_version=$2
install_dir=$3

# Uncomment below to run integration test on local computer
#source ./create_cluster.sh $k8s_version "virtualbox"
# change linux to "osx" when running on local computer
echo "$OSTYPE"
if [[ "$OSTYPE" == "darwin"* ]]; then
  export os="osx"
else
  export os="linux"
./install_istio.sh $istio_version $os
# Uncomment below line if setup fails due to KUBECONFIG not set
export KUBECONFIG=~/.kube/config
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