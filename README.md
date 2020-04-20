
<p align="center">
  <img src="https://user-images.githubusercontent.com/35096265/65359707-33216900-dbb2-11e9-8622-dc76c3882c02.png" width="500">  
</p>


[![CircleCI](https://circleci.com/gh/istio-ecosystem/admiral/tree/master.svg?style=svg)](https://circleci.com/gh/istio-ecosystem/admiral/tree/master) [![codecov](https://codecov.io/gh/istio-ecosystem/admiral/branch/master/graph/badge.svg)](https://codecov.io/gh/istio-ecosystem/admiral)

**Admiral provides automatic configuration and service discovery for multicluster Istio service mesh**

Istio has a very robust set of multi-cluster capabilities.  Managing this configuration across multiple clusters at scale is challenging.  Admiral takes an opinionated view on this configuration and provides automatic provisioning and syncing across clusters.  This removes the complexity from developers and mesh operators pushing this complexity into automation.

## Install

### Prerequisite

One or more k8s clusters.

**Example setup for a K8s cluster**

`Note`: If running in windows, a bash shell is required (cygwin)

* Install [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
* Install [minikube](https://istio.io/docs/setup/platform-setup/minikube/) to bring up a k8s cluster locally (Make sure your `$KUBECONFIG` points to `minikube` before proceeding)
* Install [helm](https://github.com/helm/helm/blob/master/docs/install.md)
* Install [wget](https://www.gnu.org/software/wget/)

```
#Download & extract Istio

#Download

wget https://github.com/istio/istio/releases/download/1.3.3/istio-1.3.3-osx.tar.gz
OR
wget https://github.com/istio/istio/releases/download/1.3.3/istio-1.3.3-linux.tar.gz
OR
wget https://github.com/istio/istio/releases/download/1.3.3/istio-1.3.3-win.tar.gz

#Extract

tar -xf istio-1.3.3-osx.tar.gz
OR
tar -xf istio-1.3.3-linux.tar.gz
OR
tar -xf istio-1.3.3-win.tar.gz
```

```
#Create istio-system namespace

kubectl create ns istio-system
```
```
#Create k8s secret to be used by Citadel for mTLS cert generation

kubectl create secret generic cacerts -n istio-system \
    --from-file=istio-1.3.3/samples/certs/ca-cert.pem \
    --from-file=istio-1.3.3/samples/certs/ca-key.pem \
    --from-file=istio-1.3.3/samples/certs/root-cert.pem \
    --from-file=istio-1.3.3/samples/certs/cert-chain.pem
```
```
#Generate, install and verify Istio CRDs

helm template istio-1.3.3/install/kubernetes/helm/istio-init --name istio-init --namespace istio-system | kubectl apply -f -

#Make sure Istio crds are installed

kubectl get crds | grep 'istio.io' | wc -l
```
```
#Generate & Install Istio

helm template istio-1.3.3/install/kubernetes/helm/istio --name istio --namespace istio-system \
    -f istio-1.3.3/install/kubernetes/helm/istio/example-values/values-istio-multicluster-gateways.yaml | kubectl apply -f -

#Verify that istio pods are up

kubectl get pods -n istio-system
```

`Reference:` [K8s cluster installed with Istio_replicated control planes](https://istio.io/docs/setup/install/multicluster/gateways/#deploy-the-istio-control-plane-in-each-cluster)


## Examples

### Single cluster

#### Setup Admiral

```
#Download and extract admiral

wget https://github.com/istio-ecosystem/admiral/releases/download/v0.1-alpha/admiral-install-v0.1-alpha.tar.gz
tar xvf admiral-install-v0.1-alpha.tar.gz
```

```
#Install admiral

kubectl apply -f ./admiral-install-v0.1-alpha/yaml/remotecluster.yaml
kubectl apply -f ./admiral-install-v0.1-alpha/yaml/demosinglecluster.yaml

#Verify admiral is running

kubectl get pods -n admiral
```

```
#Create the secret for admiral to monitor.

#Since this is for a single cluster demo the remote and local context are the same
./admiral-install-v0.1-alpha/scripts/cluster-secret.sh $KUBECONFIG  $KUBECONFIG admiral

#Verify the secret

kubectl get secrets -n admiral
```
```
#Point hosts ending in global to be resolved by istio coredns

./admiral-install-v0.1-alpha/scripts/redirect-dns.sh
```
#### Setup Sample Apps

```
#Install test services

kubectl apply -f ./admiral-install-v0.1-alpha/yaml/sample.yaml
```
```
#Install the dependency CR

kubectl apply -f ./admiral-install-v0.1-alpha/yaml/sample_dep.yaml

#Verify that admiral created service names for 'greeting' service

kubectl get serviceentry -n admiral-sync

```

#### Test

```
kubectl exec --namespace=sample -it $(kubectl get pod -l "app=webapp" --namespace=sample -o jsonpath='{.items[0].metadata.name}') -c webapp -- curl -v http://default.greeting.global
```


#### Generated configuration

Admiral generated Istio configuration.  

##### ServiceEntry

Two service entries were created in the `admiral-sync` namespace.

```kubectl get ServiceEntry -n admiral-sync```

```
NAME                      HOSTS                    LOCATION        RESOLUTION   AGE
default.greeting.global-se   [default.greeting.global]   MESH_INTERNAL   DNS          76m
default.webapp.global-se   [default.webapp.global]   MESH_INTERNAL   DNS          76m
```

```kubectl get ServiceEntry default.greeting.global-se  -n admiral-sync -o yaml```

Looking in more detail the hostname default.greeting.global is pointing back the default k8s FQDNs

```
apiVersion: networking.istio.io/v1alpha3
kind: ServiceEntry
metadata:
  creationTimestamp: "2019-09-20T22:04:59Z"
  generation: 1
  labels:
    identity: greeting
  name: default.greeting.global-se
  namespace: admiral-sync
  resourceVersion: "452814"
  selfLink: /apis/networking.istio.io/v1alpha3/namespaces/admiral-sync/serviceentries/default.greeting.global-se
  uid: b02cdbee-dbf2-11e9-9461-0aa9b467cf9c
spec:
  addresses:
  - 127.0.10.2
  endpoints:
  - address: greeting.sample.svc.cluster.local
    locality: us-west-2
    ports:
      http: 80
  hosts:
  - default.greeting.global
  location: MESH_INTERNAL
  ports:
  - name: http
    number: 80
    protocol: http
  resolution: DNS

```

## Admiral Architecture

![alt text](https://user-images.githubusercontent.com/35096265/65183155-b8244b00-da17-11e9-9f2d-cce5a96fe2e8.png "Admiral Architecture")

Admiral acts as a controller watching k8s clusters that have a credential stored as a secret object which the namespace Admiral is running in.  Admiral delivers Istio configuration to each cluster to enable services to communicate.  This configuration is contextual to each cluster, removing the need for service owners to know the mesh cluster topology.

### Contextual Configuration
With multiple Istio control planes, Admiral instruments contextual Istio configuration for a given k8s service for a cluster from which this service is being accessed from. For example, k8s service running in k8s cluster 1 and k8s cluster 2 when accessed from k8s cluster 3 should be distributed across Istio ingress load balancer in cluster 1 and cluster 2. When the same k8s service is accessed from cluster 2, it should be distributed across local FQDN and the Istio ingress load balancer in cluster 1.


### Why is the Configuration Opinionated?

There are three main aspects that led to deploying service mesh globally with the opinionated configuration automation.

- Non homologous k8s clusters for HA/DR deployments.  The same service binary would not be deployed in a namespace with the same name across two different regions.  The namespace naming schema that is utilized, creates a globally unique name.  This prevents us from using the default namespace FQDN.  Also, the namespace is an administrative context boundary.  This means if ownership of the service binary moved from one team to another the service would be deployed in a namespace owned by the new team. If the FQDN used to address the service is bound to the namespace transitioning ownership becomes cumbersome and causes clients to change how the service is called.  For these reasons we chose to use a naming abstraction that does not include the namespace in the service’s FQDN.

- Authoring context, if a single mesh control plane is used with many remote k8s clusters attached to it the authoring of k8s configuration and Istio configuration would be disparate.  We expect development teams to manage and utilize Istio features so having these separate would be cumbersome.  For example, Istio routing configurations (VirtualService) deployed with CD pipeline would have to be applied to a k8s api-server in a cluster that is different then the main hosting k8s cluster.  Also, the resiliency and scalability of a single control plane was concerning but we didn’t have data to prove it would be an issue.  

- The need to have several FQDN for the same service that have different resolution and load balancing properties.  For example, service1.global could resolve to local topology first.  Service1 being deployed in multiple regions would need a names like service1-west.global and service1-east.global. Each of these names would resolve the region in the name respectively and the opposite region in a failure scenario.  These names are often used for testing in one region during deployments and for troubleshooting.

## Connecting Clusters

Each cluster is an independent cluster with an Istio control plane.  Admiral needs a k8s context to watch each cluster stored as a secret.  This is used for Admiral to watch and generate configuration.

## Global Identifier

Admiral utilizes the concept of a global service identifier.  This identifier is attached to k8s service definitions as a label.  This label can be anything and will be defined in the following Dependency types identityLabel field.  

This global identifier is needed to identify the same service running on multiple regions for Active/Active or DR deployments.  This prevents the need to have namespace names consistent across multi-region deployments.

```
---
apiVersion: v1
kind: Service
metadata:
  name: my-service
  labels:
    app: service1
    identity: service1
    env: dev
spec:
  ports:
  - port: 80
    protocol: TCP
  selector:
    app: service1
---    
```
Service labels will be used to create the default dns name.

## DNS Names

By default dns names will be constructed using the values of two labels from the service.  
- env
- global-identifier as defined by the Dependency type  

**{env}.{global-identifier}.global**

For the service above

**dev.service1.global**

If the env label is not present the word default will be used.

**default.service1.mesh**

*No "real" dns name are created but the core dns plug is used with back ServiceEntries*

## Types

Admiral introduces two new CRDs to control the cross cluster automation.

### Dependency

The Dependency type can be used for more than just picking the global identifier.  The dependency information is used to prune the configuration syncing so only configuration needed in each
cluster is provisioned by Admiral.  

*If the wild card and specific dependency object are both present any service not matching a specific rule will be sync to all clusters*

```
---
apiVersion: admiral.io/v1alpha1
kind: Dependency
metadata:
  name: dependency
  namespace: admiral
spec:
  source: service1
  identityLabel: identity
  destinations:
    - service2
    - service3
---    
```
This config tells Admiral to only sync configuration for service2 and service3 to any cluster where service1 is running.
Once granular dependency types are defined the identityLabel can be different for separate entries.

### Global Traffic Policy

Using the Global Traffic policy type will allow for the creation of multiple dns names with different routing locality configuration for the service.

```
---
apiVersion: admiral.io/v1alpha1
kind: GlobalTrafficPolicy
metadata:
  name: gtp-service1
spec:
  selector:
    app: service1
  policy:
  - dns: service1.global
    lbtype: TOPOLOGY
  - dns: service1-west.global
    lbtype: FAILOVER
    target:
    - region: uswest-2
      weight: 100
    - region: useast-2
      weight: 0
  - dns: service1-east.global
    lbtype: FAILOVER
    target:
    - region: uswest-2
      weight: 0
    - region: useast-2
      weight: 100
---
```

In this example the service object with the app=service1 label will have 3 dns names created that map to it.
- service1.global - pins traffic the local region the traffic originated
- service1-west.global - sends traffic to the west region and only to east if west in unavailable
- service1-east.global - sends traffic to the east region and only to west if east in unavailable

## Who uses Admiral?

Organizations below are **officially** using Admiral. Please send a PR with your organization name if you are using Admiral.

1. [Intuit](https://www.intuit.com/)

## Community Blogs and Presentations

1. [Stitching a Service Mesh Across Hundreds of Discrete Networks](https://www.youtube.com/watch?v=EWyNbBn1vns)

2. [Multicluster Istio configuration and service discovery using Admiral](https://istio.io/blog/2020/multi-cluster-mesh-automation/)

## Collaboration and Communication

[Admiral Slack Channel](https://istio.slack.com/archives/GP0QM9ZJ8) - `Note:` This channel is under Istio slack org, please fill out this [form](https://docs.google.com/forms/d/e/1FAIpQLSfdsupDfOWBtNVvVvXED6ULxtR4UIsYGCH_cQcRr0VcG1ZqQQ/viewform) to get access to Istio slack.

## Contributing
Refer to [Contributing doc](./CONTRIBUTING.md)
