package loader

import (
	"fmt"

	argo "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned"
	admiral "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	log "github.com/sirupsen/logrus"
	istio "istio.io/client-go/pkg/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type KubeClientLoader struct{}

// Singleton
var kubeClientLoader = &KubeClientLoader{}

func GetKubeClientLoader() ClientLoader {
	return kubeClientLoader
}

func (loader *KubeClientLoader) LoadAdmiralClientFromPath(kubeConfigPath string) (admiral.Interface, error) {
	config, err := getConfig(kubeConfigPath)
	if err != nil || config == nil {
		return nil, err
	}

	return loader.LoadAdmiralClientFromConfig(config)
}

func (*KubeClientLoader) LoadAdmiralClientFromConfig(config *rest.Config) (admiral.Interface, error) {
	return admiral.NewForConfig(config)
}

func (loader *KubeClientLoader) LoadIstioClientFromPath(kubeConfigPath string) (istio.Interface, error) {
	config, err := getConfig(kubeConfigPath)
	if err != nil || config == nil {
		return nil, err
	}

	return loader.LoadIstioClientFromConfig(config)
}

func (loader *KubeClientLoader) LoadIstioClientFromConfig(config *rest.Config) (istio.Interface, error) {
	return istio.NewForConfig(config)
}

func (loader *KubeClientLoader) LoadArgoClientFromPath(kubeConfigPath string) (argo.Interface, error) {
	config, err := getConfig(kubeConfigPath)
	if err != nil || config == nil {
		return nil, err
	}

	return loader.LoadArgoClientFromConfig(config)
}

func (loader *KubeClientLoader) LoadArgoClientFromConfig(config *rest.Config) (argo.Interface, error) {
	return argo.NewForConfig(config)
}

func (loader *KubeClientLoader) LoadKubeClientFromPath(kubeConfigPath string) (kubernetes.Interface, error) {
	config, err := getConfig(kubeConfigPath)
	if err != nil || config == nil {
		return nil, err
	}

	return loader.LoadKubeClientFromConfig(config)
}

func (loader *KubeClientLoader) LoadKubeClientFromConfig(config *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(config)
}

func getConfig(kubeConfigPath string) (*rest.Config, error) {
	log.Infof("getting kubeconfig from: %#v", kubeConfigPath)
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)

	if err != nil || config == nil {
		return nil, fmt.Errorf("could not retrieve kubeconfig: %v", err)
	}
	return config, err
}
