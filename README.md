 # Admiral

[![CircleCI](https://circleci.com/gh/istio-ecosystem/admiral/tree/master.svg?style=svg)](https://circleci.com/gh/istio-ecosystem/admiral/tree/master)

**Admiral provides automatic configuration for multiple istio deployments to work as a single mesh**

Istio has a very robust set of multi-cluster capabilities.  Managing this configuration in multiple cluster at scale is challenging.  Admiral takes an opinionated view on this configuration and provides automatic provisioning and syncing across clusters.  This removes the complexity from developers and mesh operators pushing this complexity into automation.

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

### Multi-Cluster
```kubeclt apply something```

#### Generated config

Admiral watching the cluster generated some Istio configuration.  

##### ServiceEntry

```kubeclt get ServiceEntry```

##### Virtual Service

```kubeclt get ServiceEntry```

## Multi-Cluster Deployment Topology

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

If the env label is not present it will be dropped from the dns name.

**service1.mesh**

*No "real" dns name are created but the core dns plug is used with back ServiceEntries*

## Types

Admiral introduces two new CRDs to control the cross cluster automation.

### Dependency

In this example we define a Dependency type.  This dependency has an identityLabel with value app.  This will cause the value of the app label on service type to be used as the global identifier.

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
  identityLabel: app
  destinations:
    - service2
    - service3
---    
```
This config tells Admiral to only sync configuration for service2 and service3 to any cluster where service1 is running.

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

## Contributing
Refer to [Contributing doc](./CONTRIBUTING.md)

