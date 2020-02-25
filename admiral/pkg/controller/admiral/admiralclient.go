package admiral

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
)

// retrieve the Kubernetes cluster client from outside of the cluster
func AdmiralCrdClientFromPath(kubeConfigPath string) (clientset.Interface, error) {
	config, err := getConfig(kubeConfigPath)
	if err != nil || config == nil {
		return nil, err
	}
	return AdmiralCrdClientFromConfig(config)
}

func AdmiralCrdClientFromConfig(config *rest.Config) (clientset.Interface, error) {
	return clientset.NewForConfig(config)
}

func K8sClientFromConfig(config *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(config)
}

func K8sClientFromPath(kubeConfigPath string) (kubernetes.Interface, error) {

	config, err := getConfig(kubeConfigPath)
	if err != nil || config == nil {
		return nil, err
	}
	return K8sClientFromConfig(config)
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
