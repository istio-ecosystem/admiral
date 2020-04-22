# Release notes

## v0.9 
We are excited to announce the release of Admiral version `v0.9` with lots of cool functionality added. This version is ready for production usage and addresses some of the biggest requests from our users.

`We would like to thank all the contributors and everyone who played a role in testing the alpha and beta releases of Admiral.`


### [Global Traffic Policies](./Architecture.md#Global Traffic Policy)
Global traffic policies allow defining custom traffic routing behaviour for a Admiral generated CNAME, for example routing all traffic for a service to a specific region or AZ. This feature relies on Istio's [locality load balancing](https://istio.io/docs/ops/configuration/traffic-management/locality-load-balancing/)

Try out this [example](./Examples.md#Global traffic policy)
### Lots of improvements to usability 
* Only istio resources with `exportTo: *` or `exportTo` field missing are synced across clusters.
* Added a feature to update Istio Sidecar resource in the client's namespace. This allows for Admiral based automation to filter what endpoint configuration to be loaded by a istio-proxy and keeping the footprint minimal and still manageable.
* Annotation (`admiral.io/ignore`) to exempt k8s Deployments/Namespaces from Admiral processing. This would be useful for migration k8s Deployments into other clusters.
### Simplified installing the examples and organized them by use case   
* We now have examples for [Single cluster](./Examples.md#Single cluster), [Multi-cluster](./Examples.md#Multicluster) and [Global traffic policies](./Examples.md#Global traffic policy)
* Installation commands have been turned into bash scripts to reduce the number of steps.
### Bug fixes 
* Handle Admiral crashes in special scenarios like below:
    -   for resource deletions
    -   missing resource permissions
    -   missing k8s Service for a k8s Deployment

### Summary
Complete list of [issues fixed in v0.9](https://github.com/istio-ecosystem/admiral/milestone/1?closed=1)

Report issues and/or post your questions via:
* [Admiral slack channel](https://istio.slack.com/archives/CT3F18T08)
* [Github issues](https://github.com/istio-ecosystem/admiral/issues)

Stay tuned for [`v1.0`](https://github.com/istio-ecosystem/admiral/milestone/2) release!!