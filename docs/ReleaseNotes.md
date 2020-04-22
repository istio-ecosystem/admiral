# Release notes

## v0.9 
Excited to announce the release of admiral version `v0.9` with lots of cool functionality added and it is ready for production usage. Following are the highlights of this release:

`We would like to thank all the contributors and everyone who played a role in testing the aplha and beta releases of admiral.`

### [Global Traffic Policies](./Architecture.md#Global Traffic Policy)
Global traffic policies allow defining custom traffic routing behaviour for a admiral generated CNAME, for example routing all traffic for a service to uswest region. This feature relies on Istio's [locality load balancing](https://istio.io/docs/ops/configuration/traffic-management/locality-load-balancing/)

Try out this [example](./Examples.md#Global traffic policy)
### Lots of improvements to usability 
* Only istio resources with `exportTo: *` or `exportTo` field missing are synced across clusters.
* Added a feature to update Istio Sidecar resource in the client's namespace. This allows for admiral based automation to filter what services to be loaded by a istio proxy and keeping the proxy foot print minimal and still manageable.
* Annotation (`admiral.io/ignore`) to exempt deployments/namespaces from Admiral processing (would be useful for migration into other clusters)
### Simplified installing the examples and organized them by use case
* We now have examples for [Single cluster](./Examples.md#Single cluster), [Multi-cluster](./Examples.md#Multicluster) and [Global traffic policies](./Examples.md#Global traffic policy)
* Installation commands have been turned into bash scripts to reduce the number of steps.
### Bug fixes 
* Handle admiral crashes in special scenarios like below:
    -   for resource deletions
    -   missing resource permissions
    -   missing service for a deployment

### Summary
Complete list of [issues fixed in v0.9](https://github.com/istio-ecosystem/admiral/milestone/1?closed=1)

Report issues and post your questions on [Admiral slack](https://istio.slack.com/archives/CT3F18T08) or via [github issues](https://github.com/istio-ecosystem/admiral/issues)

Stay tuned for [`v1.0`](https://github.com/istio-ecosystem/admiral/milestone/2) release!!   