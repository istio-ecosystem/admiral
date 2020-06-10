#!/bin/bash

while true; do
    clustername=$(kubectl config current-context)
    printf "k8s cluster: %s\n" "$clustername"
    printf "Namespaces ['admiral','admiral-sync','sample'] will be deleted.\nDo you wish to proceed?\n"
    options="Please enter yes/Y/y or no/N/n"
    echo $options
    read -p "" yn
    case $yn in
        [Yy]* ) kubectl delete namespace admiral; kubectl delete namespace admiral-sync; kubectl delete namespace sample; break;;
        [Nn]* ) exit;;
        * ) echo $options;;
    esac
done
