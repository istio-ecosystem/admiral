# Admiral Architecture

![alt text](https://user-images.githubusercontent.com/35096265/65183155-b8244b00-da17-11e9-9f2d-cce5a96fe2e8.png "Admiral Architecture")

Admiral acts as a controller watching k8s clusters that have a credential stored as a secret object which the namespace Admiral is running in.  Admiral delivers Istio configuration to each cluster to enable services to communicate.  This configuration is contextual to each cluster, removing the need for service owners to know the mesh cluster topology.

## Contextual Configuration
With multiple Istio control planes, Admiral instruments contextual Istio configuration for a given k8s service for a cluster from which this service is being accessed from. For example, k8s service running in k8s cluster 1 and k8s cluster 2 when accessed from k8s cluster 3 should be distributed across Istio ingress load balancer in cluster 1 and cluster 2. When the same k8s service is accessed from cluster 2, it should be distributed across local FQDN and the Istio ingress load balancer in cluster 1.


## Why is the Configuration Opinionated?

There are three main aspects that led to deploying service mesh globally with the opinionated configuration automation.

- Non homologous k8s clusters for HA/DR deployments.  The same service binary would not be deployed in a namespace with the same name across two different regions.  The namespace naming schema that is utilized, creates a globally unique name.  This prevents us from using the default namespace FQDN.  Also, the namespace is an administrative context boundary.  This means if ownership of the service binary moved from one team to another the service would be deployed in a namespace owned by the new team. If the FQDN used to address the service is bound to the namespace transitioning ownership becomes cumbersome and causes clients to change how the service is called.  For these reasons we chose to use a naming abstraction that does not include the namespace in the service’s FQDN.

- Authoring context, if a single mesh control plane is used with many remote k8s clusters attached to it the authoring of k8s configuration and Istio configuration would be disparate.  We expect development teams to manage and utilize Istio features so having these separate would be cumbersome.  For example, Istio routing configurations (VirtualService) deployed with CD pipeline would have to be applied to a k8s api-server in a cluster that is different then the main hosting k8s cluster.  Also, the resiliency and scalability of a single control plane was concerning but we didn’t have data to prove it would be an issue.  

- The need to have several FQDN for the same service that have different resolution and load balancing properties.  For example, service1.global could resolve to local topology first.  Service1 being deployed in multiple regions would need a names like service1-west.global and service1-east.global. Each of these names would resolve the region in the name respectively and the opposite region in a failure scenario.  These names are often used for testing in one region during deployments and for troubleshooting.

# Connecting Clusters

Each cluster is an independent cluster with an Istio control plane.  Admiral needs a k8s context to watch each cluster stored as a secret.  This is used for Admiral to watch and generate configuration.

# Global Identifier

Admiral utilizes the concept of a global service identifier.  This identifier is attached to k8s service definitions as a label.  This label can be anything and will be defined in the following Dependency types identityLabel field.  

This global identifier is needed to identify the same workload running on multiple regions for Active/Active or DR deployments.  This prevents the need to have namespace names consistent across multi-region deployments.

    ---

    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: greeting
      namespace: sample
    spec:
      replicas: 1
      selector:
        matchLabels:
          app: greeting
      template:
        metadata:
          annotations:
            admiral.io/env: stage
            sidecar.istio.io/inject: "true"
          labels:
            app: greeting
            identity: greeting
        spec:
          containers:
            - image: nginx
              name: greeting
              ports:
                - containerPort: 80
              volumeMounts:
                - mountPath: /etc/nginx
                  name: nginx-conf
                  readOnly: true
                - mountPath: /var/log/nginx
                  name: log
              resources:
                requests:
                  cpu: "10m"
                  memory: "50Mi"
                limits:
                  cpu: "20m"
                  memory: "75Mi"
          volumes:
            - configMap:
                items:
                  - key: nginx.conf
                    path: nginx.conf
                name: nginx-conf
              name: nginx-conf
            - emptyDir: {}
              name: log
    ---    
Deployment admiral.io/env annotation/label will be used to create the default dns name.

# DNS Names

By default dns names will be constructed using the following values from the k8s deployment.  
- `Annotation/Label`: `admiral.io/env` with a fallback to `env`
- `Label`: global-identifier as defined by the Dependency type  

**{admiral.io/env}.{global-identifier}.global**

For the k8s deployment above

**stage.greeting.global**

If the `admiral.io/env` annotation or `env` label is not present the word `default` will be used.

**default.greeting.global**

*No "real" dns name are created but the coredns plug-in is used with back ServiceEntries*

# Types

Admiral introduces two new CRDs to control the cross cluster automation.

## Dependency

The Dependency type can be used for more than just picking the global identifier.  The dependency information is used to prune the configuration syncing so only configuration needed in each
cluster is provisioned by Admiral.  

*If the wild card and specific dependency object are both present any service not matching a specific rule will be sync to all clusters*

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
   
This config tells Admiral to only sync configuration for service2 and service3 to any cluster where service1 is running.
Once granular dependency types are defined the identityLabel can be different for separate entries.

## Global Traffic Policy

Using the Global Traffic policy type will allow for the creation of multiple dns names with different routing locality configuration for the service.

    ---
    apiVersion: admiral.io/v1alpha1
    kind: GlobalTrafficPolicy
    metadata:
      name: gtp-service1
      labels:
        identity: service1
    spec:
      policy:
      - dns: default
        lbtype: TOPOLOGY
      - dns: service1-west
        lbtype: FAILOVER
        target:
        - region: uswest-2
          weight: 100
        - region: useast-2
          weight: 0
      - dns: service1-east
        lbtype: FAILOVER
        target:
        - region: uswest-2
          weight: 0
        - region: useast-2
          weight: 100
    ---

In this example the service object with the identity=service1 label will have 3 dns names created that map to it.
- stage.service1.global - pins traffic to the local region the traffic originated
- service1-west.stage.service1.global - sends traffic to the west region and only to east if west in unavailable
- service1-east.stage.service1.global - sends traffic to the east region and only to west if east in unavailable



### Global Traffic Policy Linking

A global traffic policy is matched to a deployment if and only if their values for the `globaltraffic_deployment_label` labels match. The default label key for `globaltraffic_deployment_label` is `identity`, meaning that in the default case you can just apply the same identity label that you're already using for your deployments to your global traffic policies.

##### Label matching

GTPs will only match with deployments in the same namespace. A lack of identity label is taken to mean "identity: default", so if both resources in the same namespace lack the label, they will match.

| GTP Label            | Deployment Label     | Matching |
|----------------------|----------------------|----------|
| <none>               | <none>               | Yes      |
| identity: "service1" | identity: "service1" | Yes      |
| identity: "service1" | identity: "service2" | No       |
| identity: "service1" | <none>               | No       |
| <none>               | identity: "service1" | No       |