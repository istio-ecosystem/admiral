# Contributing to Admiral

We welcome contributions :)

## Submitting PRs
* Make sure to check existing issues and file an issue before starting to work on a feature/bug. This will help prevent duplication of work. 
Also refer to [Collaboration](./README.md) for communication channels.

## Setting up for local Development
* Clone the repo and set ADMIRAL_HOME env variable
```bash
git clone https://github.com/istio-ecosystem/admiral.git
cd admiral
export ADMIRAL_HOME=$(pwd)
```
* Run a [minikube](https://kubernetes.io/docs/setup/learning-environment/minikube/) k8s cluster using existing script (you can use any k8s cluster if one exists already)
* Note: Recommend using k8s version 1.16.8 or above to work with recent istio version
```bash
$ADMIRAL_HOME/tests/create_cluster.sh 1.16.8
export KUBECONFIG=~/.kube/config
```
* Install [Prerequisites](./docs/Examples.md#Prerequisite) and make sure to install istio control plane in cluster. Alternatively, you can use the script to install istio control plane on the cluster created in previous step:
                                                                                                   
Mac: `$ADMIRAL_HOME/tests/install_istio.sh 1.7.4 osx` 

Mac (Apple Silicon): `$ADMIRAL_HOME/tests/install_istio.sh 1.7.4 osx-arm64`

Linux: `$ADMIRAL_HOME/tests/install_istio.sh 1.7.4 linux`                                                                                            

* Set up necessary permissions and configurations for Admiral

```bash
$ADMIRAL_HOME/install/scripts/dev_setup.sh
```
* Run `admiral` from your IDE (assumes that the kubeconfig is for the cluster is at `~/.kube/config`)
```
$ADMIRAL_HOME/admiral/cmd/admiral/main.go --kube_config "<Path_to_Kubeconfig>"
```

* `Optional`: Adding a second cluster for admiral to monitor (for multi-cluster)
```bash
$ADMIRAL_HOME/install/scripts/cluster-secret.sh <Path_to_Kubeconfig_Admiral_Cluster> <Path_to_Kubeconfig_Remote_Cluster> admiral
```

* `Optional`: Run `admiral` inside the minikube cluster
```bash
# point to the admiral primary cluster
export KUBECONFIG=<Path_to_Kubeconfig_Admiral_Cluster>

# build the yaml files
make gen-yaml

# run the install scripts
$ADMIRAL_HOME/install/scripts/install_admiral.sh $ADMIRAL_HOME/out/

# get the http url for admiral api
minikube service admiral-http -n admiral --url

# get the http url for admiral metrics api
minikube service admiral-metrics -n admiral --url
```

* `Optional`: Install prometheus for metrics.
```bash
# build the yaml files
make gen-yaml

# run the install scripts
$ADMIRAL_HOME/install/scripts/install_prometheus.sh $ADMIRAL_HOME/out

# get the http url for prometheus dashboard
minikube service prometheus -n istio-system --url
```

## Protobuf code generation
* If you've made changes to protobuf model objects and need to re-generate their clientsets, use `sh hack/update-codegen.sh` and checkin the generated files

## Integration tests
### Single cluster
Single cluster integration tests can be run locally using minikube. By default if env var IS_LOCAL is not set, minikube use virtual box to spin up minikube, when running in cluster, need to set IS_LOCAL var to false.
```
make gen-yaml
cd $ADMIRAL_HOME/tests
./run.sh "1.16.8" "1.7.4" "../out"
```
* Multi-cluster 
```
TODO
```
  