
 # Admiral

**Multi-cluster mesh scalable and automated**

Istio has a very robust set of multi-cluster capabilities.   Managing this configuration in multiple cluster at scale is challenging.  Admiral takes an opinionated view on this configuration and provides automatic provisioning and syncing across clusters.  This removes the complexity from developers and mesh operators pushing this complexity into automation.

## Install

### Prerequisite
[Install Istio with replicated control planes](https://istio.io/docs/setup/install/multicluster/gateways/)

## Examples

### Single cluster
```kubeclt apply something```

#### Generated config

Admiral watching the cluster generated some Istio configuration.  

##### ServiceEntry

```kubeclt get ServiceEntry```

##### Virtual Service

```kubeclt get ServiceEntry```

### Multi cluster
```kubeclt apply something```

#### Generated config

Admiral watching the cluster generated some Istio configuration.  

##### ServiceEntry

```kubeclt get ServiceEntry```

##### Virtual Service

```kubeclt get ServiceEntry```

## Multi-Cluster Deployment Topology

![alt text](https://user-images.githubusercontent.com/35096265/65183155-b8244b00-da17-11e9-9f2d-cce5a96fe2e8.png "Admiral Architecture")

Admiral acts as a control watching k8s cluster that have a credential stored as a secret object.  Admiral delivers Istio configuration to each cluster to enable the service to communicate.  This configuration is contextual to each cluster, removing the need for service owners from needing to know the mesh cluster topology.

### Contextual configuration
With multiple Istio control planes, Admiral instruments contextual Istio configuration for a given k8s service for a cluster from which this service is being accessed from. For example, k8s service running in k8s cluster 1 and k8s cluster 2 when accessed from k8s cluster 3 should be distributed across Istio ingress load balancer in cluster 1 and cluster 2. When the same k8s service is accessed from cluster 2, it should be distributed across local FQDN and the Istio ingress load balancer in cluster 1.


### Why is the Configuration opinionated?

There are three main aspects that lead us to our deployment opinion on how to deploy service mesh globally.

- non homologous k8s clusters for HA/DR deployments.  The same service binary would not be deployed in a namespace with the same name across two different regions.  The namespace naming schema that is utilized, creates a globally unique name.  This prevents us from using the default namespace FQDN.  Also, the namespace is an administrative context boundary.  This means if ownership of the service binary moved from one team to another the service would be deployed in a namespace owned by the new team. If the FQDN used to address the service is bound to the namespace to would make the transition cumbersome and cause client to need to change how the service is called.  For these reasons we chose to use a naming abstraction that does not include the namespace in the service’s FQDN.

- Authoring context, if a single mesh control plane is used with many remote k8s clusters attached to it the authoring of k8s configuration and Istio configuration would be disparate.  We expect development teams to manage and utilize Istio features so have these separate would be cumbersome.  For example, Istio routing configurations deployed with CD pipeline would have to be applied to a k8s api service in a cluster that is different then the main hosting k8s cluster but is driving routing behavior in the main hosting k8s cluster.   Also the resilience and scalability of a single control plane was concerning but we didn’t have data to prove it would be an issue.  

- The need to have several FQDN for the same service that have different resolution and load balancing properties.  For example, service1.mesh.com could resolve local topology first and if then to a remote location.  Also service1 would need a names like service1-west.mesh.com and service1-east.mesh.com each of these names would resolve the region in the name respectively and the opposite region in a failure scenario.  These names are often used for testing in one region during deployments and for troubleshooting.


## Connecting clusters

Each cluster is an independent cluster with an Istio control plane.  Admiral needs a k8s context stored as a secret


## Global Identifier

Admiral utilizes the concept of a global service identifier.  This identifier is attached to k8s service definitions as a label.  This label can be anything and will defined in the following types selectors.  

This global identifier is needed to identify the same service running on multiple regions for Active/Active or DR deployments.  This prevents the need to have namespace names the same across multi-region deployments.

```
---
apiVersion: v1
kind: Service
metadata:
  name: my-service
  labels:
    app: service1
    env: dev
spec:
  ports:
  - port: 80
    protocol: TCP
  selector:
    app: service1
---    
```
Add a label to the service this label will be used to
## DNS names

By default dns names will be constructed using the values of two labels from the service.  
- env
- global-identifier as defined by the Dependency type  

{env}.{global-identifier}.mesh

For the service above

**dev.service1.mesh**

If the env label is not present it will be dropped from the dns name.

**service1.mesh**

## Types

Admiral introduces two new CRDs to control the cross cluster automation.

### Dependency

In this example we define a dependency type.  This dependency has an identityLabel with value app.  This will define the value of the app label on and k8s service will be used as the global identifier.

```
---
apiVersion: admiral.io/v1alpha1
kind: Dependency
metadata:
  name: dependency
  namespace: admiral
spec:
  source: *
  identityLabel: app
  destinations:
    - *    
---    
```  
If a dependency with `*` is defined for the source and destinations it will cause all config to be sync to all clusters.  This is great for getting started but defining dependencies should be used for production deployments.

The dependency type can be used for more then just picking the
global identifier.  The dependency information is used to prune
the configuration syncing so only configuration needed in each
cluster is provisioned by admiral.

```
---
apiVersion: admiral.io/v1alpha1
kind: Dependency
metadata:
  name: dependency
  namespace: admiral
spec:
  source: service1
  identityLabel: app
  destinations:
    - service2
    - service3
---    
```
This config tells admiral to only sync configuration for service2 and service3 to any cluster where service1 is running.


Once granular dependency types are defined the identityLabel can be different for separate entries.

```
---
apiVersion: admiral.io/v1alpha1
kind: Dependency
metadata:
  name: dependency
  namespace: admiral
spec:
  source: service2
  identityLabel: name
  destinations:
    - service3    
---    
```

The default dns name that is created for service2 would use the value of the name label.


### Global Traffic Routing



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
