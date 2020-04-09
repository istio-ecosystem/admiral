
## Install

### Prerequisite

One or more k8s clusters will need the following steps executed

#### Install the below utilities

`Note`: If running in windows, a bash shell is required (cygwin)

* Install [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
* Install [minikube](https://istio.io/docs/setup/platform-setup/minikube/) to bring up a k8s cluster locally (Make sure your `$KUBECONFIG` points to `minikube` before proceeding)
* Install [helm](https://github.com/helm/helm/blob/master/docs/install.md)
* Install [wget](https://www.gnu.org/software/wget/)

#### Install Argo-rollouts
Refer [this](https://argoproj.github.io/argo-rollouts/) for details on Argo-Rollouts

```
kubectl create namespace argo-rollouts

kubectl apply -n argo-rollouts -f https://raw.githubusercontent.com/argoproj/argo-rollouts/stable/manifests/install.yaml
```

#### Install Istio

```
#Download

wget https://github.com/istio/istio/releases/download/1.4.3/istio-1.4.3-osx.tar.gz
OR
wget https://github.com/istio/istio/releases/download/1.4.3/istio-1.4.3-linux.tar.gz
OR
wget https://github.com/istio/istio/releases/download/1.4.3/istio-1.4.3-win.tar.gz

#Extract

tar -xf istio-1.4.3-osx.tar.gz
OR
tar -xf istio-1.4.3-linux.tar.gz
OR
tar -xf istio-1.4.3-win.tar.gz
```

```
#Create istio-system namespace

kubectl create ns istio-system
```
```
#Create k8s secret to be used by Citadel for mTLS cert generation

kubectl create secret generic cacerts -n istio-system \
    --from-file=istio-1.4.3/samples/certs/ca-cert.pem \
    --from-file=istio-1.4.3/samples/certs/ca-key.pem \
    --from-file=istio-1.4.3/samples/certs/root-cert.pem \
    --from-file=istio-1.4.3/samples/certs/cert-chain.pem
```
```
#Generate, install and verify Istio CRDs

helm template istio-1.4.3/install/kubernetes/helm/istio-init --namespace istio-system | kubectl apply -f -

#Make sure Istio crds are installed

kubectl get crds | grep 'istio.io' | wc -l
```
```
#Generate & Install Istio

helm template istio-1.4.3/install/kubernetes/helm/istio --namespace istio-system \
    -f istio-1.4.3/install/kubernetes/helm/istio/example-values/values-istio-multicluster-gateways.yaml | kubectl apply -f -

#Verify that istio pods are up

kubectl get pods -n istio-system
```

#### DNS setup
In a k8s cluster, you will have a DNS component that would resolve names. Admiral generates names ending in global (Ex: `stage.greeting.global`) which can be resolved by istiocoredns (as its watching Istio ServiceEntries created by Admiral with those names) installed as part of Istio.
So you have to point DNS resolution for names ending in `global` to point to `ClusterIp` of istiocoredns service. The below step is to point coredns in a k8s cluster to istiocoredns. If you are using kube-dns, you can tweak this script.

```Note: The below script wipes out existing codedns config map, please manually edit it if you want to try this in a cluster with real services/traffic```

```
#Run the below script for having coredns point to istiocoredns for dns lookups of names ending in global

./admiral-install-v0.1-beta/scripts/redirect-dns.sh
```

#### Remove envoy cluster rewrite filter
Delete Istio's envoy filter for translating `global` to `svc.cluster.local` at istio-ingressgateway because we don't need that as Admiral generates Service Entries for cross cluster communication to just work!
```
# Delete envoy filter for translating `global` to `svc.cluster.local`
kubectl delete envoyfilter istio-multicluster-ingressgateway -n istio-system
```

`Reference:` [K8s cluster installed with Istio_replicated control planes](https://istio.io/docs/setup/install/multicluster/gateways/#deploy-the-istio-control-plane-in-each-cluster)


## Example Installations & Demos

### Single cluster

#### Install/Run Admiral

```
#Download and extract admiral

wget https://github.com/istio-ecosystem/admiral/releases/download/v0.1-beta/admiral-install-v0.1-beta.tar.gz
tar xvf admiral-install-v0.1-beta.tar.gz
```

```
#Install admiral

kubectl apply -f ./admiral-install-v0.1-beta/yaml/remotecluster.yaml
kubectl apply -f ./admiral-install-v0.1-beta/yaml/demosinglecluster.yaml

#Verify admiral is running

kubectl get pods -n admiral
```

```
#Create the secret for admiral to monitor.

#Since this is for a single cluster demo the remote and local context are the same
./admiral-install-v0.1-beta/scripts/cluster-secret.sh $KUBECONFIG  $KUBECONFIG admiral
```
```
#Verify the secret
kubectl get secrets -n admiral
```

#### Deploy Sample Argo-Rollouts Services with Blue-Green stratergy
Refer [this](https://argoproj.github.io/argo-rollouts/features/bluegreen/) for details on Blue-Green stratergy with Argo-Rollouts

```
#Install test services

kubectl apply -f ./admiral-install-v0.1-beta/yaml/sample-greeting-rollout-bluegreen.yaml
```
```
#Install the dependency CR (this is optional)

kubectl apply -f ./admiral-install-v0.1-beta/yaml/sample_dep.yaml

#Verify that admiral created service names for 'greeting' service

kubectl get serviceentry -n admiral-sync

```

#### Demo

Now, run the command below that uses the CNAME generated by Admiral
```
kubectl exec --namespace=sample-rollout-bluegreen -it $(kubectl get pod -l "app=webapp" --namespace=sample-rollout-bluegreen -o jsonpath='{.items[0].metadata.name}') -c webapp -- curl -v http://bluegreen.greeting.global

```

#### Generated configuration

Admiral generated Istio configuration.  

##### ServiceEntry

Two service entries were created in the `admiral-sync` namespace.

```kubectl get ServiceEntry -n admiral-sync```

```
NAME                      HOSTS                    LOCATION        RESOLUTION   AGE
bluegreen.greeting.global-se   [bluegreen.greeting.global]   MESH_INTERNAL   DNS          76m
default.webapp.global-se   [default.webapp.global]   MESH_INTERNAL   DNS          76m
```

```kubectl get ServiceEntry bluegreen.greeting.global-se  -n admiral-sync -o yaml```

Looking in more detail the hostname bluegreen.greeting.global is pointing back the default k8s FQDNs

```
apiVersion: networking.istio.io/v1alpha3
kind: ServiceEntry
metadata:
  creationTimestamp: "2020-04-09T09:34:00Z"
  generation: 1
  labels:
    identity: greeting
  name: bluegreen.greeting.global-se
  namespace: admiral-sync
  resourceVersion: "4692"
  selfLink: /apis/networking.istio.io/v1alpha3/namespaces/admiral-sync/serviceentries/bluegreen.greeting.global-se
  uid: 3e5c08e1-7a45-11ea-bea9-0800275e624b
spec:
  addresses:
  - 240.0.10.1
  endpoints:
  - address: rollout-bluegreen-active.sample-rollout-bluegreen.svc.cluster.local
    ports:
      http: 80
  hosts:
  - bluegreen.greeting.global
  location: MESH_INTERNAL
  ports:
  - name: http
    number: 80
    protocol: http
  resolution: DNS
```

#### Deploy Sample Argo-Rollouts Services with Canary stratergy
Refer [this](https://argoproj.github.io/argo-rollouts/features/canary/) for details on Canary stratergy with Argo-Rollouts

```
#Install test services

kubectl apply -f ./admiral-install-v0.1-beta/yaml/sample-greeting-rollout-canary.yaml
```
```
#Install the dependency CR (this is optional)

kubectl apply -f ./admiral-install-v0.1-beta/yaml/sample_dep.yaml

#Verify that admiral created service names for 'greeting' service

kubectl get serviceentry -n admiral-sync

```

#### Demo

Now, run the command below that uses the CNAME generated by Admiral
```
kubectl exec --namespace=sample-rollout-canary -it $(kubectl get pod -l "app=webapp" --namespace=sample-rollout-canary -o jsonpath='{.items[0].metadata.name}') -c webapp -- curl -v http://canary.greeting.global

```

#### Generated configuration

Admiral generated Istio configuration.  

##### ServiceEntry

Two service entries were created in the `admiral-sync` namespace.

```kubectl get ServiceEntry -n admiral-sync```

```
NAME                      HOSTS                    LOCATION        RESOLUTION   AGE
canary.greeting.global-se   [canary.greeting.global]   MESH_INTERNAL   DNS          76m
default.webapp.global-se   [default.webapp.global]   MESH_INTERNAL   DNS          76m
```

```kubectl get ServiceEntry canary.greeting.global-se  -n admiral-sync -o yaml```

Looking in more detail the hostname canary.greeting.global is pointing back the default k8s FQDNs

```
apiVersion: networking.istio.io/v1alpha3
kind: ServiceEntry
metadata:
  creationTimestamp: "2020-04-09T09:34:02Z"
  generation: 1
  labels:
    identity: greeting
  name: canary.greeting.global-se
  namespace: admiral-sync
  resourceVersion: "4697"
  selfLink: /apis/networking.istio.io/v1alpha3/namespaces/admiral-sync/serviceentries/canary.greeting.global-se
  uid: 3f51ff47-7a45-11ea-bea9-0800275e624b
spec:
  addresses:
  - 240.0.10.2
  endpoints:
  - address: rollout-canary.sample-rollout-canary.svc.cluster.local
    ports:
      http: 80
  hosts:
  - canary.greeting.global
  location: MESH_INTERNAL
  ports:
  - name: http
    number: 80
    protocol: http
  resolution: DNS
```

