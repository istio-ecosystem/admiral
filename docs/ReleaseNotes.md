# Release notes

## v1.1

This is a minor release that addresses some usability and clean up aspects. 

### Features

 #### [gRPC and http2 protocol support](https://github.com/istio-ecosystem/admiral/issues/76)
 Version 1.1 adds support to use gRPC and http2 protocols in addition to the default http for admiral generated endpoints. Try the [example](https://github.com/istio-ecosystem/admiral/blob/master/docs/Examples.md#grpc-demo) to explore this further.
 
 #### [Deletion support](https://github.com/istio-ecosystem/admiral/issues/9)
 Admiral now cleans up the CNAMEs and the associated configurations when a k8s deployement is deleted.
 
 #### [Guidance on using Admiral in production](https://github.com/istio-ecosystem/admiral/issues/106)
 Added documentation and guidance on how to deploy admiral in a production setting. The documentation is available [here](https://github.com/istio-ecosystem/admiral/blob/master/docs/Examples.md#production-deployment)
 
### Project improvements
Added automated integration tests for admiral to simulates real uses cases

## v1.0

Admiral has graduated to Generally Available! Version 1.0 has a series of bugfixes and includes support for [Argo Rollouts](https://argoproj.github.io/argo-rollouts/)

Once again, many thanks to everyone who has tried out Admiral and provided their valuable feedback throughout the development process. And a special thanks to everyone who has contributed to the project This would not be possible without you.

### Features

 #### [Argo Rollouts](https://github.com/istio-ecosystem/admiral/issues/57)
 Version 1.0 adds support to use Argo Rollouts rather than deployments, allowing you to leverage their advanced deployment capabilities without giving up Admiral's functionality. Try the [example](https://github.com/istio-ecosystem/admiral/blob/master/docs/Examples.md#argo-rollouts) to get started.
 
 #### [Secret Updates](https://github.com/istio-ecosystem/admiral/pull/55)
 Version 1.0 now also properly handles secret updates, allowing for cluster secrets to be ignored or changed without requiring an Admiral restart to take effect.
 
 #### [Ignorable Services](https://github.com/istio-ecosystem/admiral/pull/107)
 Services are added to the set of resources that Admiral can be configured to ignore, using `admiral.io/ignore: "true"`.
 
 #### Assorted quality of life improvements
 Including [an added linter](https://github.com/istio-ecosystem/admiral/pull/117), [dockerfile improvements](https://github.com/istio-ecosystem/admiral/pull/129), [parameter overrides](https://github.com/istio-ecosystem/admiral/pull/115), and a [flag-driven log level](https://github.com/istio-ecosystem/admiral/pull/114)
 
### Bug Fixes

* Fixed a [bug](https://github.com/istio-ecosystem/admiral/issues/102) preventing 100/0 load balancing with Global Traffic Policies.
* Put Argo Rollouts behind a feature flag to prevent [excessive error logging](https://github.com/istio-ecosystem/admiral/issues/109) in clusters without Argo CRDs installed.
* Fixed a [bug](https://github.com/istio-ecosystem/admiral/issues/112) where virtual services in destination namespaces weren't being imported to the [sidecars](https://istio.io/latest/docs/reference/config/networking/sidecar/) in the client namespace. 
* Admiral now correctly updates [service entries](https://istio.io/latest/docs/reference/config/networking/service-entry/) in response to a previously watched being ignored. 
* [Mitigated](https://github.com/istio-ecosystem/admiral/pull/123) a memory leak related to the recreation of cache controllers

## v0.9 
We are excited to announce the release of Admiral version `v0.9` with lots of cool functionality added. This version is ready for production usage and addresses some of the biggest requests from our users.

`We would like to thank all the contributors and everyone who played a role in testing the alpha and beta releases of Admiral.`


### [Global Traffic Policies](./Architecture.md#global-traffic-policy)
Global traffic policies allow defining custom traffic routing behaviour for a Admiral generated CNAME, for example routing all traffic for a service to a specific region or AZ. This feature relies on Istio's [locality load balancing](https://istio.io/docs/ops/configuration/traffic-management/locality-load-balancing/)

Try out this [example](./Examples.md#global-traffic-policy)
### Lots of improvements to usability 
* Only istio resources with `exportTo: *` or `exportTo` field missing are synced across clusters to obey the spec.
* Added a feature to update [Istio Sidecar](https://istio.io/docs/reference/config/networking/sidecar/) resource in the client's namespace. This allows for Admiral based automation to filter what endpoint configuration to be loaded by a istio-proxy and keeping the footprint minimal and still manageable.
* Annotation (`admiral.io/ignore`) to exempt k8s Deployments/Namespaces from Admiral processing. This would be useful for migration k8s Deployments into other clusters.
### Simplified installing the examples and organized them by use case   
* We now have examples for [Single cluster](./Examples.md#single-cluster), [Multi-cluster](./Examples.md#Multicluster) and [Global traffic policies](./Examples.md#global-traffic-policy)
* Installation commands have been turned into bash scripts to reduce the number of steps.
### Bug fixes 
* Handle Admiral crashes in special scenarios like below:
    -   for resource deletions
    -   missing resource permissions
    -   missing k8s Service for a k8s Deployment

### Summary
Complete [list of issues](https://github.com/istio-ecosystem/admiral/milestone/1?closed=1) fixed in `v0.9`

Report issues and/or post your questions via:
* [Admiral slack channel](https://istio.slack.com/archives/CT3F18T08)
* [Github issues](https://github.com/istio-ecosystem/admiral/issues)

Stay tuned for [`v1.0`](https://github.com/istio-ecosystem/admiral/milestone/2) release!!
