
 # admiral

Operating istio across multi clusters can be confusing.  Admiral takes an opinionated view on this configuration and provides automatic provisioning and syncing across clusters to facilitate communication.  This removes the complexity from developers and mesh operators and pushes this complexity into automation.

## Install

## Sample

```kubeclt apply ```

### Generated config



#### ServiceEntry



#### Virtual Service



## Concepts
### Global Identifier

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
apiVersion: admiral.com/v1
kind: GlobalTrafficPolicy
metadata:
  name: policy-asset1
spec:
  selector:
    asset: asset1
  policy:
  - dns: service1.mesh.intuit.com
    lbtype: TOPOLOGY
  - dns: service1-west2.mesh.intuit.com
    lbtype: FAILOVER
    target:
    - region: uswest-2
      weight: 100
    - region: useast-2
      weight: 0
  - dns: service1-east.mesh.intuit.com
    lbtype: FAILOVER
    target:
    - region: uswest-2
      weight: 0
    - region: useast-2
      weight: 100
---
```
