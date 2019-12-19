package test

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"gopkg.in/yaml.v2"
	k8sCoreV1 "k8s.io/api/core/v1"

)

type FakeConfigMapController struct {
	GetError error
	PutError error
	ConfigmapToReturn *k8sCoreV1.ConfigMap
}

func (c *FakeConfigMapController) GetConfigMap() (*k8sCoreV1.ConfigMap, error) {
	return c.ConfigmapToReturn, c.GetError
}

func (c *FakeConfigMapController) PutConfigMap(newMap *k8sCoreV1.ConfigMap) error {
	return c.PutError
}

func BuildFakeConfigMapFromAddressStore(addressStore *common.ServiceEntryAddressStore) *k8sCoreV1.ConfigMap{
	bytes,_ := yaml.Marshal(addressStore)

	cm := k8sCoreV1.ConfigMap{
		Data: map[string]string{"serviceEntryAddressStore": string(bytes)},
	}
	cm.Name="se-address-configmap"
	cm.Namespace="admiral-remote-ctx"
	return &cm
}