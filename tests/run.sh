#!/bin/bash

[ $# -lt 3 ] && { echo "Usage: $0 <k8s_version> <istio_version> <admiral_install_dir>" ; exit 1; }

k8s_version=$1
istio_version=$2
install_dir=$3

source ./create_cluster.sh $k8s_version "virtualbox"
./install_istio.sh $istio_version "osx"
./dns_setup.sh $install_dir
./install_admiral.sh $install_dir
./install_sample_services.sh $install_dir

#allow for stabilization of DNS and Istio SEs before running tests
sleep 10
./test1.sh "webapp" "sample" "greeting"

#./cleanup.sh $istio_version
