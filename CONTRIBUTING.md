# Contributing to Admiral

We welcome contributions :)

## Submitting PRs
* Make sure to check existing issues and file an issue before starting to work on a feature/bug. This will help prevent duplication of work. 

## Setting up for local Development
* Use `Single Cluster` or `Multi-Cluster` sections [HERE](./README.md) to set up the local test cluster, you can use minikube.
<TODO add instructions for Minukube setup>
* Run `admiral` from your IDE.
```
/admiral/cmd/admiral/main.go --kube_config "~./.kube/config"
```

## Protobuf code generation
* If you've made changes to protobuf model objects and need to re-generate their clientsets, use `sh hack/update-codegen.sh` and checkin the generated files