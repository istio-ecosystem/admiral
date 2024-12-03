package loader

import (
	argo "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned"
	admiralapi "github.com/istio-ecosystem/admiral-api/pkg/client/clientset/versioned"
	admiral "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	numaflow "github.com/numaproj/numaflow/pkg/client/clientset/versioned"
	istio "istio.io/client-go/pkg/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const FakeKubeconfigPath = "fake.config"

type ClientLoader interface {
	LoadAdmiralClientFromPath(path string) (admiral.Interface, error)
	LoadAdmiralClientFromConfig(config *rest.Config) (admiral.Interface, error)

	LoadAdmiralApiClientFromPath(path string) (admiralapi.Interface, error)
	LoadAdmiralApiClientFromConfig(config *rest.Config) (admiralapi.Interface, error)

	LoadIstioClientFromPath(path string) (istio.Interface, error)
	LoadIstioClientFromConfig(config *rest.Config) (istio.Interface, error)

	LoadArgoClientFromPath(path string) (argo.Interface, error)
	LoadArgoClientFromConfig(config *rest.Config) (argo.Interface, error)

	LoadKubeClientFromPath(path string) (kubernetes.Interface, error)
	LoadKubeClientFromConfig(config *rest.Config) (kubernetes.Interface, error)

	LoadNumaflowClientFromPath(path string) (numaflow.Interface, error)
	LoadNumaflowClientFromConfig(config *rest.Config) (numaflow.Interface, error)
}
