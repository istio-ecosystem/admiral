package test

import (
	"context"

	k8sCoreV1 "k8s.io/api/core/v1"
)

type FakeConfigMapController struct {
	GetError          error
	PutError          error
	ConfigmapToReturn *k8sCoreV1.ConfigMap
}

func (c *FakeConfigMapController) GetConfigMap(ctx context.Context) (*k8sCoreV1.ConfigMap, error) {
	return c.ConfigmapToReturn, c.GetError
}

func (c *FakeConfigMapController) PutConfigMap(ctx context.Context, newMap *k8sCoreV1.ConfigMap) error {
	return c.PutError
}
func (c *FakeConfigMapController) GetIPPrefixForServiceEntries() (seIpPrefix string) {
	return "240.0"
}
