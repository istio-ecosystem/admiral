package admiral

import (
	"context"
	"strings"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ConfigMapControllerInterface interface {
	GetConfigMap(ctx context.Context) (*v1.ConfigMap, error)
	PutConfigMap(ctx context.Context, newMap *v1.ConfigMap) error
	GetIPPrefixForServiceEntries() (seIPPrefix string)
}

type ConfigMapController struct {
	K8sClient            kubernetes.Interface
	ConfigmapNamespace   string
	ServiceEntryIPPrefix string
}

// todo this is a temp state, eventually changes will have to be made to give each cluster it's own configmap
func NewConfigMapController(seIPPrefix string, clientLoader loader.ClientLoader) (*ConfigMapController, error) {
	kubeconfigPath := common.GetKubeconfigPath()
	namespaceToUse := common.GetSyncNamespace()

	client, err := clientLoader.LoadKubeClientFromPath(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	controller := ConfigMapController{
		K8sClient:            client,
		ConfigmapNamespace:   namespaceToUse,
		ServiceEntryIPPrefix: seIPPrefix,
	}

	return &controller, nil
}

func (c *ConfigMapController) GetConfigMap(ctx context.Context) (*v1.ConfigMap, error) {
	getOpts := metaV1.GetOptions{}
	configMap, err := c.K8sClient.CoreV1().ConfigMaps(c.ConfigmapNamespace).Get(ctx, common.GetSeAddressConfigMap(), getOpts)

	if err == nil {
		return configMap, err
	}

	if strings.Contains(err.Error(), "not found") {
		cm := v1.ConfigMap{}
		cm.Name = common.GetSeAddressConfigMap()
		cm.Namespace = c.ConfigmapNamespace
		configMap, err = c.K8sClient.CoreV1().ConfigMaps(c.ConfigmapNamespace).Create(ctx, &cm, metaV1.CreateOptions{})
	}

	return configMap, err

}

func (c *ConfigMapController) PutConfigMap(ctx context.Context, newMap *v1.ConfigMap) error {
	_, err := c.K8sClient.CoreV1().ConfigMaps(c.ConfigmapNamespace).Update(ctx, newMap, metaV1.UpdateOptions{})
	return err
}

func (c *ConfigMapController) GetIPPrefixForServiceEntries() string {
	return c.ServiceEntryIPPrefix
}
