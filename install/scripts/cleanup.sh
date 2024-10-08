#!/bin/bash

while true; do
    clustername=$(kubectl config current-context)

    if [[ $clustername == *"ppd"* || $clustername == *"prd"* || $clustername == *"prod"* ]]
    then
       echo "\$clustername is not a dev cluster"
       exit 1
    fi
    printf "k8s cluster: %s\n" "$clustername"
    printf "Namespaces ['admiral','admiral-sync','sample', 'sample-rollout-canary', 'sample-rollout-bluegreen'] will be deleted.\nDo you wish to proceed?\n"
    options="Please enter yes/Y/y or no/N/n"
    echo $options
    read -p "" yn
    case $yn in
        [Yy]* ) kubectl delete namespace admiral; kubectl delete namespace admiral-sync; kubectl delete namespace sample; kubectl delete namespace sample-rollout-canary; kubectl delete namespace sample-rollout-bluegreen; break;;
        [Nn]* ) exit;;
        * ) echo $options;;
    esac
done
