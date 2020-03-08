# Contributing to Admiral

We welcome contributions :)

## Submitting PRs
* Make sure to check existing issues and file an issue before starting to work on a feature/bug. This will help prevent duplication of work. 
Also refer to [Collaboration](./README.md#Collaboration and Communication) for communication channels.

## Setting up for local Development
* Clone the repo and set ADMIRAL_HOME env variable
```bash
git clone https://github.com/istio-ecosystem/admiral.git
export ADMIRAL_HOME=$(pwd)
```
* Run a k8s cluster using [minikube](https://kubernetes.io/docs/setup/learning-environment/minikube/) (you can use any k8s cluster if one exists already)
```bash
minikube start --memory=8192 --cpus=4 --kubernetes-version=v1.14.2
export KUBECONFIG=~/.kube/config
```
* Install [Prerequisites](./docs/Examples.md#Prerequisite)
* Set up necessary permissions and configurations for Admiral
```bash
$ADMIRAL_HOME/install/scripts/dev_setup.sh
```
* Run `admiral` from your IDE (assumes that the kubeconfig is for the cluster is at `~/.kube/config`)
```
/admiral/cmd/admiral/main.go --kube_config "<Path_to_Kubeconfig>"
```

* `Optional`: Adding a second cluster for admiral to monitor (for multi-cluster)
```bash
$ADMIRAL_HOME/install/scripts/cluster-secret.sh <Path_to_Kubeconfig_Admiral_Cluster> <Path_to_Kubeconfig_Remote_Cluster> admiral
```

## Protobuf code generation
* If you've made changes to protobuf model objects and need to re-generate their clientsets, use `sh hack/update-codegen.sh` and checkin the generated files