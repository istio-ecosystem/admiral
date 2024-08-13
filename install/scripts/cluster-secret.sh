#!/bin/bash

if [ "$#" -gt "0" ]; then
  local_cluster=$1
fi

if [ "$#" -gt "1" ]; then
  remote_cluster=$2
fi

if [ "$#" -gt "2" ]; then
  namespace_secrets=$3
fi



#TBD make sure you have context switched
export KUBECONFIG=$remote_cluster
if [ -z "$KUBECONFIG" ]
then
    echo "\$KUBECONFIG is not set"
    exit 1
elif [[ $KUBECONFIG == *"ppd"* || $KUBECONFIG == *"prd"* || $KUBECONFIG == *"prod"* ]]
then
    echo "\$KUBECONFIG is not for a dev cluster"
    exit 1
fi

#prep for creating kubeconfig of remote cluster
export WORK_DIR=$(pwd)
CLUSTER_NAME=$(kubectl config view --minify=true -o "jsonpath={.clusters[].name}" | sed -e 's/[^A-Za-z0-9.-]/-/g')
export KUBECFG_FILE=/tmp/${CLUSTER_NAME}
SERVER=$(kubectl config view --minify=true -o "jsonpath={.clusters[].cluster.server}")
NAMESPACE_SYNC=admiral-sync
SERVICE_ACCOUNT=admiral
SECRET_NAME=admiral-token
CA_DATA=$(kubectl get secret ${SECRET_NAME} -n ${NAMESPACE_SYNC} -o "jsonpath={.data['ca\.crt']}")
RAW_TOKEN=$(kubectl get secret ${SECRET_NAME} -n ${NAMESPACE_SYNC} -o "jsonpath={.data['token']}")
TOKEN=$(kubectl get secret ${SECRET_NAME} -n ${NAMESPACE_SYNC} -o "jsonpath={.data['token']}" | base64 --decode)

#echo 'TOKEN'
#echo $TOKEN

#create kubeconfig for remote cluster
cat <<EOF > ${KUBECFG_FILE}
apiVersion: v1
clusters:
   - cluster:
       certificate-authority-data: ${CA_DATA}
       server: ${SERVER}
     name: ${CLUSTER_NAME}
contexts:
   - context:
       cluster: ${CLUSTER_NAME}
       user: ${CLUSTER_NAME}
     name: ${CLUSTER_NAME}
current-context: ${CLUSTER_NAME}
kind: Config
preferences: {}
users:
   - name: ${CLUSTER_NAME}
     user:
       token: ${TOKEN}
EOF

#export variables for initializing the remote cluster creds on control plane cluster
cat <<EOF > remote_cluster_env_vars
export CLUSTER_NAME=${CLUSTER_NAME}
export KUBECFG_FILE=${KUBECFG_FILE}
EOF

source remote_cluster_env_vars

#TBD make sure you have context switched
#create secret on control plane cluster to connect to remote cluster

#export KUBECONFIG=~/.kube/config
#kubectx minikube
export KUBECONFIG=$local_cluster
if [ -z "$KUBECONFIG" ]
then
    echo "\$KUBECONFIG is not set"
    exit 1
elif [[ $KUBECONFIG == *"ppd"* || $KUBECONFIG == *"prd"* || $KUBECONFIG == *"prod"* ]]
then
    echo "\$KUBECONFIG is not for a dev cluster"
    exit 1
fi

kubectl delete secret ${CLUSTER_NAME} -n $namespace_secrets
kubectl create secret generic ${CLUSTER_NAME} --from-file ${KUBECFG_FILE} -n $namespace_secrets
kubectl label secret ${CLUSTER_NAME} admiral/sync=true -n $namespace_secrets

rm -rf remote_cluster_env_vars
rm -rf $KUBECFG_FILE
