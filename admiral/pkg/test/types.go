package test

import (
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