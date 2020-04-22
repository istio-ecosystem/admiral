#!/bin/bash

(cd ../; make gen-yaml)

k8s_versions=(
#                "1.18.0"
               "1.16.8"
#               "1.14.2"
)
istio_versions=(
#                "1.4.6"
                "1.5.1"
)


for k8s_version in "${k8s_versions[@]}"
do
	for istio_version in "${istio_versions[@]}"
  do
    echo "Testing with k8s verision: $k8s_version and istio version: $istio_version"
    ./run.sh $k8s_version $istio_version "../out"
#    pass=$(./run.sh $k8s_version $istio_version "../out" | grep -c "PASS")
#    if [ $pass -lt 1 ]
#    then
#      echo "FAIL: k8s verision: $k8s_version and istio version: $istio_version"
#    else
#      echo "PASS: k8s verision: $k8s_version and istio version: $istio_version"
#    fi
  done
done
