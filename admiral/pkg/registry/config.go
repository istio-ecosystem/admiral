package registry

import (
	admiralV1Alpha1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	networking "istio.io/api/networking/v1alpha3"
)

type IdentityConfig struct {
	IdentityName string                            `json:"identityName"`
	Clusters     map[string]*IdentityConfigCluster `json:"clusters"`
	ClientAssets map[string]string                 `json:"clientAssets"`
}

func (config *IdentityConfig) PutClusterConfig(name string, clusterConfig IdentityConfigCluster) error {
	return nil
}

type IdentityConfigCluster struct {
	Name            string                                `json:"name"`
	Locality        string                                `json:"locality"`
	IngressEndpoint string                                `json:"ingressEndpoint"`
	IngressPort     string                                `json:"ingressPort"`
	IngressPortName string                                `json:"ingressPortName"`
	Environment     map[string]*IdentityConfigEnvironment `json:"environment"`
}

func (config *IdentityConfigCluster) PutEnvironment(name string, environmentConfig IdentityConfigEnvironment) error {
	return nil
}

func (config *IdentityConfigCluster) PutClientAssets(clientAssets []string) error {
	return nil
}

type RegistryServiceConfig struct {
	Name   string            `json:"name"`
	Weight int               `json:"weight"`
	Ports  map[string]uint32 `json:"ports"`
}

type TrafficPolicy struct {
	GlobalTrafficPolicy    admiralV1Alpha1.GlobalTrafficPolicy    `json:"globaltrafficpolicy"`
	OutlierDetection       admiralV1Alpha1.OutlierDetection       `json:"outlierdetection"`
	ClientConnectionConfig admiralV1Alpha1.ClientConnectionConfig `json:"clientconnectionconfig"`
}

type IdentityConfigEnvironment struct {
	Name          string                            `json:"name"`
	Namespace     string                            `json:"namespace"`
	Services      map[string]*RegistryServiceConfig `json:"services"`
	ServiceName   string                            `json:"serviceName"`
	Type          string                            `json:"type"`
	Selectors     map[string]string                 `json:"selectors"`
	Ports         []*networking.ServicePort         `json:"ports"`
	TrafficPolicy TrafficPolicy                     `json:"trafficPolicy"`
	Event         admiral.EventType                 `json:"event"`
}

type RegistryServiceConfigSorted []*RegistryServiceConfig

func (r RegistryServiceConfigSorted) Len() int {
	return len(r)
}

func (r RegistryServiceConfigSorted) Less(i, j int) bool {
	return r[i].Name < r[j].Name
}

func (r RegistryServiceConfigSorted) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}
