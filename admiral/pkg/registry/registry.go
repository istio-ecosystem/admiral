package registry

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	log "github.com/sirupsen/logrus"
)

const (
	IntuitTid = "intuit_tid"
)

// IdentityConfiguration is an interface to fetch configuration from a registry
// backend. The backend can provide an API to give configurations per identity,
// or if given a cluster name, it will provide the configurations for all
// the identities present in that cluster.
type IdentityConfiguration interface {
	GetIdentityConfigByIdentityName(identityAlias string, ctxLogger *log.Entry) (IdentityConfig, error)
	GetIdentityConfigByClusterName(clusterName string, ctxLogger *log.Entry) ([]IdentityConfig, error)
}

type registryClient struct {
	client BaseClient
}

func NewRegistryClient(options ...func(client *registryClient)) *registryClient {
	registryClient := &registryClient{}
	for _, o := range options {
		o(registryClient)
	}
	return registryClient
}

func WithBaseClientConfig(clientConfig *Config) func(*registryClient) {
	return func(c *registryClient) {
		c.client = NewClient(clientConfig)
	}
}

func (c *registryClient) PutClusterGateway(cluster, name, url, notes, tid string, labels []string) error {
	r, err := http.NewRequest("PUT", url, nil)
	authHeader := r.Header.Get("Authorization")
	authData, err := ParsePrivateAuthHeader(authHeader)
	response, err := c.client.MakePrivateAuthCall(url, authData, tid, r.Method, nil)
	return nil
	//sort labels?
	//preferably put this in service handler where we check if common.IsIstioIngressGatewayService(svc)
}

func (c *registryClient) DeleteClusterGateway(cluster, name string) error {
	return nil
}

func (c *registryClient) PutCustomData(cluster, namespace, key string, value interface{}) error {
	return nil
	// switch-case based on key (CCC/GTP/VS/OD/etc)
	// traffic config?
}

func (c *registryClient) DeleteCustomData(cluster, namespace, key string) error {
	return nil
}

func (c *registryClient) PutHostingData(cluster, namespace, key string, value interface{}) error {
	return nil
	// switch-case based on key (service/rollout/deployment)
	// Where does sidecar and envoy filter go?
}

func (c *registryClient) DeleteHostingData(cluster, namespace, key string) error {
	return nil
}

// GetIdentityConfigByIdentityName calls the registry API to fetch the IdentityConfig for
// the given identityAlias
func (c *registryClient) GetIdentityConfigByIdentityName(identityAlias string, ctxLogger *log.Entry) (IdentityConfig, error) {
	//TODO: Use real result from registry and remove string splitting to match test file names
	defer util.LogElapsedTime("GetIdentityConfigByIdentityName", identityAlias, "", "")
	byteValue, err := readIdentityConfigFromFile(strings.Split(identityAlias, "."))
	if err != nil {
		ctxLogger.Infof(common.CtxLogFormat, "GetByIdentityName", identityAlias, "", "", err)
	}
	var identityConfigUnmarshalResult IdentityConfig
	err = json.Unmarshal(byteValue, &identityConfigUnmarshalResult)
	if err != nil {
		ctxLogger.Infof(common.CtxLogFormat, "GetByIdentityName", identityAlias, "", "", err)
	}
	return identityConfigUnmarshalResult, err
}

func readIdentityConfigFromFile(shortAlias []string) ([]byte, error) {
	pathName := "testdata/" + shortAlias[len(shortAlias)-1] + "IdentityConfiguration.json"
	if common.GetSecretFilterTags() == common.GetOperatorSecretFilterTags() && common.GetOperatorSyncNamespace() != "" {
		pathName = "/etc/serviceregistry/config/" + shortAlias[len(shortAlias)-1] + "IdentityConfiguration.json"
	}
	return os.ReadFile(pathName)
}

// GetIdentityConfigByClusterName calls the registry API to fetch the IdentityConfigs for
// every identity on the cluster.
func (c *registryClient) GetIdentityConfigByClusterName(clusterName string, ctxLogger *log.Entry) ([]IdentityConfig, error) {
	defer util.LogElapsedTime("GetIdentityConfigByClusterName", "", "", clusterName)
	//TODO: need to call this function once during startup time to warm the cache
	//jsonResult = os.request(/cluster/{cluster_id}/configurations
	ctxLogger.Infof(common.CtxLogFormat, "GetByClusterName", "", "", clusterName, "")
	//identities := getIdentitiesForCluster(clusterName) - either queries shard CRD or shard CRD controller calls this func with those as parameters
	identities := []string{clusterName}
	identityConfigs := []IdentityConfig{}
	var err error
	for _, identity := range identities {
		identityConfig, identityErr := c.GetIdentityConfigByIdentityName(identity, ctxLogger)
		if identityErr != nil {
			err = identityErr
			ctxLogger.Infof(common.CtxLogFormat, "GetByClusterName", "", "", clusterName, identityErr)
		}
		identityConfigs = append(identityConfigs, identityConfig)
	}
	return identityConfigs, err
}
