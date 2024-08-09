package registry

import (
	"context"
	"encoding/json"
	"os"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	coreV1 "k8s.io/api/core/v1"
)

// IdentityConfiguration is an interface to fetch configuration from a registry
// backend. The backend can provide an API to give configurations per identity,
// or if given a cluster name, it will provide the configurations for all
// the identities present in that cluster.
type IdentityConfiguration interface {
	GetByIdentityName(identityAlias string, ctx context.Context) (IdentityConfig, error)
	GetByClusterName(clusterName string, ctx context.Context) ([]IdentityConfig, error)
}

type registryClient struct {
	registryEndpoint string
	operatorCluster  string
}

func NewRegistryClient(options ...func(client *registryClient)) *registryClient {
	registryClient := &registryClient{}
	for _, o := range options {
		o(registryClient)
	}
	return registryClient
}

func WithRegistryEndpoint(registryEndpoint string) func(*registryClient) {
	return func(c *registryClient) {
		c.registryEndpoint = registryEndpoint
	}
}

func WithOperatorCluster(operatorCluster string) func(*registryClient) {
	return func(c *registryClient) {
		c.operatorCluster = operatorCluster
	}
}

type IdentityConfig struct {
	Assetname string                  `json:"assetname"`
	Clusters  []IdentityConfigCluster `json:"clusters"`
}

type IdentityConfigCluster struct {
	Name            string                      `json:"name"`
	Locality        string                      `json:"locality"`
	IngressEndpoint string                      `json:"ingressEndpoint"`
	IngressPort     string                      `json:"ingressPort"`
	IngressPortName string                      `json:"ingressPortName"`
	Environment     []IdentityConfigEnvironment `json:"environment"`
	ClientAssets    []map[string]string         `json:"clientAssets"`
	// Why is clientAssets under cluster? shouldn't it be regardless of cluster??/???
}

type IdentityConfigEnvironment struct {
	Name          string                           `json:"name"`
	Namespace     string                           `json:"namespace"`
	ServiceName   string                           `json:"serviceName"`
	Type          string                           `json:"type"`
	Selectors     map[string]string                `json:"selectors"`
	Ports         []coreV1.ServicePort             `json:"ports"`
	TrafficPolicy networkingV1Alpha3.TrafficPolicy `json:"trafficPolicy"`
}

// GetByIdentityName calls the registry API to fetch the IdentityConfig for
// the given identityAlias
func (c *registryClient) GetByIdentityName(identityAlias string, ctx context.Context) (IdentityConfig, error) {
	//jsonResult = os.request(/asset/identityAlias/configurations)
	ctxLogger := common.GetCtxLogger(ctx, identityAlias, "")
	ctxLogger.Infof(common.CtxLogFormat, "GetByIdentityName", identityAlias, "", c.operatorCluster, "")
	byteValue, err := os.ReadFile("testdata/" + identityAlias + "IdentityConfiguration.json")
	if err != nil {
		ctxLogger.Infof(common.CtxLogFormat, "GetByIdentityName", identityAlias, "", c.operatorCluster, err)
	}
	var identityConfigUnmarshalResult IdentityConfig
	err = json.Unmarshal(byteValue, &identityConfigUnmarshalResult)
	if err != nil {
		ctxLogger.Infof(common.CtxLogFormat, "GetByIdentityName", identityAlias, "", c.operatorCluster, err)
	}
	return identityConfigUnmarshalResult, err
}

// GetByClusterName calls the registry API to fetch the IdentityConfigs for
// every identity on the cluster.
func (c *registryClient) GetByClusterName(clusterName string, ctx context.Context) ([]IdentityConfig, error) {
	//jsonResult = os.request(/cluster/{cluster_id}/configurations
	ctxLogger := common.GetCtxLogger(ctx, "", "")
	ctxLogger.Infof(common.CtxLogFormat, "GetByClusterName", "", "", clusterName, "")
	//identities := getIdentitiesForCluster(clusterName) - either queries shard CRD or shard CRD controller calls this func with those as parameters
	identities := []string{clusterName}
	identityConfigs := []IdentityConfig{}
	var err error
	for _, identity := range identities {
		identityConfig, identityErr := c.GetByIdentityName(identity, ctx)
		if identityErr != nil {
			err = identityErr
			ctxLogger.Infof(common.CtxLogFormat, "GetByClusterName", "", "", clusterName, identityErr)
		}
		identityConfigs = append(identityConfigs, identityConfig)
	}
	return identityConfigs, err
}
