#!/bin/bash

[ $# -lt 3 ] && { echo "Usage: $0 <k8s_version> <istio_version> <admiral_install_dir>" ; exit 1; }

k8s_version=$1
istio_version=$2
install_dir=$3
os=""
source ./create_cluster.sh $k8s_version
# Uncomment below line if setup fails due to KUBECONFIG not set
export KUBECONFIG=~/.kube/config
# change $os from "linux" to "osx" when running on local computer
if [[ "$OSTYPE" == "darwin"* ]]; then
  os="osx"
else
  if [[ $istio_version == "1.5"* ]]; then
    os="linux"
  else
    os="linux-amd64"
  fi
fi
#./install_istio.sh $istio_version $os
./dns_setup.sh $install_dir
$install_dir/scripts/install_admiral.sh $install_dir
$install_dir/scripts/install_rollouts.sh
$install_dir/scripts/cluster-secret.sh $KUBECONFIG  $KUBECONFIG admiral
$install_dir/scripts/install_sample_services.sh $install_dir

#allow for stabilization of DNS and Istio SEs before running tests
sleep 10
./test1.sh "webapp" "sample" "greeting" | tee tmp.txt
result=$(grep -o -i PASS tmp.txt | wc -l)
cat tmp.txt
echo "mengying"
echo $result
#if [[ $result != 1 ]]; then
#  exit 1
#else
#  continue
#fi
echo $?
if [ $? -eq 1 ]; then
  exit 1
fi
#./test2.sh "webapp" "sample-rollout-bluegreen" "greeting" | tee tmp.txt
#result=$(grep -o -i PASS tmp.txt | wc -l)
#if [[ $result != 1 ]]; then
#  exit 1
#fi
#rm tmp.txt
#./cleanup.sh $istio_version