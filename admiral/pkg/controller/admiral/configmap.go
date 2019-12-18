package admiral

import (
	"errors"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"strings"
)

const configmapName  = "se-address-configmap"

type ConfigMapControllerInterface interface {
	GetConfigMap() (*v1.ConfigMap, error)
	PutConfigMap(newMap *v1.ConfigMap) error
}

type ConfigMapController struct {
	K8sClient kubernetes.Interface
	ConfigmapNamespace string
}

//todo this is a temp state, eventually changes will have to be made to give each cluster it's own configmap

func NewConfigMapController(kubeconfigPath string, namespaceToUse string) (*ConfigMapController, error) {

	if kubeconfigPath == "" {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		client, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}
		controller := ConfigMapController{
			K8sClient: client,
			ConfigmapNamespace: namespaceToUse,
		}
		return &controller, nil
	} else {
		// use the current context in kubeconfig
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}

		// create the clientset
		client, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}
		controller := ConfigMapController{
			K8sClient: client,
			ConfigmapNamespace: namespaceToUse,
		}
		return &controller, nil
	}

}

func (c *ConfigMapController) GetConfigMap() (*v1.ConfigMap, error) {
	getOpts := metaV1.GetOptions{}
	configMap, err := c.K8sClient.CoreV1().ConfigMaps(c.ConfigmapNamespace).Get(configmapName, getOpts)

	if err == nil {
		return configMap, err
	}

	if strings.Contains(err.Error(), "not found") {
		cm := v1.ConfigMap{}
		cm.Name = configmapName
		cm.Namespace = c.ConfigmapNamespace
		configMap, err = c.K8sClient.CoreV1().ConfigMaps(c.ConfigmapNamespace).Create(&cm)
	}

	return configMap, err

}

func (c *ConfigMapController) PutConfigMap(newMap *v1.ConfigMap) error {
	err := validateConfigmapBeforePutting(newMap)
	if err != nil {
		return err
	}
	_, err = c.K8sClient.CoreV1().ConfigMaps(c.ConfigmapNamespace).Update(newMap)
	return err
}

func GetServiceEntryStateFromConfigmap(configmap *v1.ConfigMap) *common.ServiceEntryAddressStore {

	bytes := []byte(configmap.Data["serviceEntryAddressStore"])
	addressStore := common.ServiceEntryAddressStore{}
	err := yaml.Unmarshal(bytes, &addressStore)

	if err != nil {
		logrus.Errorf("Could not unmarshal configmap data. Double check the configmap format. %v", err)
		return nil
	}
	if addressStore.Addresses == nil {
		addressStore.Addresses = []string{}
	}
	if addressStore.EntryAddresses == nil {
		addressStore.EntryAddresses = map[string]string{}
	}


	return &addressStore
}

func validateConfigmapBeforePutting(cm *v1.ConfigMap) error {
	if cm.ResourceVersion == "" {
		return errors.New("resourceversion required") //without it, we can't be sure someone else didn't put something between our read and write
	}
	store := GetServiceEntryStateFromConfigmap(cm)
	if len(store.EntryAddresses) != len(store.Addresses) {
		return errors.New("address cache length mismatch") //should be impossible. We're in a state where the list of addresses doesn't match the map of se:address. Something's been missed and must be fixed
	}
	return nil
}