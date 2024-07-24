package loader

import (
	argo "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned"
	argofake "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/fake"
	admiral "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	admiralfake "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned/fake"
	istio "istio.io/client-go/pkg/clientset/versioned"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

const FakePrefix = "fake"

// fake clients for the Admiral cluster
var FakeAdmiralClient admiral.Interface = admiralfake.NewSimpleClientset()
var FakeIstioClient istio.Interface = istiofake.NewSimpleClientset()
var FakeKubeClient kubernetes.Interface = kubefake.NewSimpleClientset()
var FakeArgoClient argo.Interface = argofake.NewSimpleClientset()

// fake clients for dependent clusters
var FakeAdmiralClientMap map[string]admiral.Interface = make(map[string]admiral.Interface)
var FakeIstioClientMap map[string]istio.Interface = make(map[string]istio.Interface)
var FakeKubeClientMap map[string]kubernetes.Interface = make(map[string]kubernetes.Interface)
var FakeArgoClientMap map[string]argo.Interface = make(map[string]argo.Interface)

type FakeClientLoader struct{}

// Singleton
var fakeClientLoader = &FakeClientLoader{}

func GetFakeClientLoader() ClientLoader {
	return fakeClientLoader
}

func (loader *FakeClientLoader) LoadAdmiralClientFromPath(path string) (admiral.Interface, error) {
	return FakeAdmiralClient, nil
}

func (*FakeClientLoader) LoadAdmiralClientFromConfig(config *rest.Config) (admiral.Interface, error) {
	admiralClient, ok := FakeAdmiralClientMap[config.Host]
	if !ok {
		admiralClient = admiralfake.NewSimpleClientset()
		FakeAdmiralClientMap[config.Host] = admiralClient
	}
	return admiralClient, nil
}

func (loader *FakeClientLoader) LoadIstioClientFromPath(path string) (istio.Interface, error) {
	return FakeIstioClient, nil
}

func (loader *FakeClientLoader) LoadIstioClientFromConfig(config *rest.Config) (istio.Interface, error) {
	istioClient, ok := FakeIstioClientMap[config.Host]
	if !ok {
		istioClient = istiofake.NewSimpleClientset()
		FakeIstioClientMap[config.Host] = istioClient
	}
	return istioClient, nil
}

func (loader *FakeClientLoader) LoadArgoClientFromPath(path string) (argo.Interface, error) {
	return FakeArgoClient, nil
}

func (loader *FakeClientLoader) LoadArgoClientFromConfig(config *rest.Config) (argo.Interface, error) {
	argoClient, ok := FakeArgoClientMap[config.Host]
	if !ok {
		argoClient = argofake.NewSimpleClientset()
		FakeArgoClientMap[config.Host] = argoClient
	}
	return argoClient, nil
}

func (loader *FakeClientLoader) LoadKubeClientFromPath(path string) (kubernetes.Interface, error) {
	return FakeKubeClient, nil
}

func (loader *FakeClientLoader) LoadKubeClientFromConfig(config *rest.Config) (kubernetes.Interface, error) {
	kubeClient, ok := FakeKubeClientMap[config.Host]
	if !ok {
		kubeClient = kubefake.NewSimpleClientset()
		FakeKubeClientMap[config.Host] = kubeClient
	}
	return kubeClient, nil
}
