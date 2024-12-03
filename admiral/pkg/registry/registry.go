package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	log "github.com/sirupsen/logrus"
)

// ClientAPI is an interface to fetch configuration from a registry
// backend. The backend can provide an API to give configurations per identity,
// or if given a cluster name, it will provide the configurations for all
// the identities present in that cluster.
type ClientAPI interface {
	GetIdentityConfigByIdentityName(identityAlias string, ctxLogger *log.Entry) (IdentityConfig, error)
	GetIdentityConfigByClusterName(clusterName string, ctxLogger *log.Entry) ([]IdentityConfig, error)
	PutClusterGateway(cluster, name, ingressURL, notes, resourceType, tid string, labels []string) error
	DeleteClusterGateway(cluster, name, resourceType, tid string) error
	PutCustomData(cluster, namespace, name, resourceType, tid string, value interface{}) error
	DeleteCustomData(cluster, namespace, name, resourceType, tid string) error
	PutHostingData(cluster, namespace, name, assetAlias, resourceType, tid string, metadata map[string]interface{}) error
	DeleteHostingData(cluster, namespace, name, resourceType, tid string) error
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

func marshalDataForRegistry(data map[string]interface{}, url string) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("json body for request to %s was nil", url)
	}
	body, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json body for http request to %s", url)
	}
	return body, nil
}

func makeCallToRegistry(url, tid, method string, data map[string]interface{}, client BaseClient) error {
	var body []byte
	var err error
	var response *http.Response
	if data != nil {
		body, err = marshalDataForRegistry(data, url)
		if err != nil {
			return err
		}
	}
	response, err = client.MakePrivateAuthCall(url, tid, method, body)
	if err != nil {
		return err
	}
	if response == nil {
		return fmt.Errorf("response for request to %s was nil", url)
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("response code for request to %s was %v", url, response.StatusCode)
	}
	return nil
}

func (c *registryClient) PutClusterGateway(cluster, name, ingressURL, notes, resourceType, tid string, labels []string) error {
	url := fmt.Sprintf("%s/%s/k8s/clusters/%s/gateways/%s?type=%s", c.client.GetConfig().Host, c.client.GetConfig().BaseURI, cluster, name, resourceType)
	data := map[string]interface{}{
		"name":  name, // name of the actual gateway object in the cluster, not assetAlias
		"url":   ingressURL,
		"label": strings.Join(labels, ", "), // sort labels?
		"notes": notes,                      // not sure what the point of this is
		"type":  resourceType,
	}
	return makeCallToRegistry(url, tid, http.MethodPut, data, c.client)
	//preferably put this in service handler where we check if common.IsIstioIngressGatewayService(svc)
}

func (c *registryClient) DeleteClusterGateway(cluster, name, resourceType, tid string) error {
	url := fmt.Sprintf("%s/%s/k8s/clusters/%s/gateways/%s?type=%s", c.client.GetConfig().Host, c.client.GetConfig().BaseURI, cluster, name, resourceType)
	return makeCallToRegistry(url, tid, http.MethodDelete, nil, c.client)
}

func (c *registryClient) PutCustomData(cluster, namespace, name, resourceType, tid string, value interface{}) error {
	url := fmt.Sprintf("%s/%s/k8s/clusters/%s/namespaces/%s/customdata/%s?type=%s", c.client.GetConfig().Host, c.client.GetConfig().BaseURI, cluster, namespace, name, resourceType)
	byteVal, _ := json.Marshal(value)
	strVal := string(byteVal)
	data := map[string]interface{}{
		"name":  name,         // name of the object in the cluster, not assetAlias
		"type":  resourceType, // GlobalTrafficPolicy/CCC/OD/VS/etc
		"value": strVal,       // could be the entire yaml or whatever we want here
	}
	return makeCallToRegistry(url, tid, http.MethodPut, data, c.client)
	// traffic config?
}

func (c *registryClient) DeleteCustomData(cluster, namespace, name, resourceType, tid string) error {
	url := fmt.Sprintf("%s/%s/k8s/clusters/%s/namespaces/%s/customdata/%s?type=%s", c.client.GetConfig().Host, c.client.GetConfig().BaseURI, cluster, namespace, name, resourceType)
	return makeCallToRegistry(url, tid, http.MethodDelete, nil, c.client)
}

func (c *registryClient) PutHostingData(cluster, namespace, name, assetAlias, resourceType, tid string, metadata map[string]interface{}) error {
	url := fmt.Sprintf("%s/%s/k8s/clusters/%s/namespaces/%s/hostings/%s", c.client.GetConfig().Host, c.client.GetConfig().BaseURI, cluster, namespace, name)
	data := map[string]interface{}{
		common.AssetAlias: assetAlias,
		"name":            name,         // name of the object in the cluster ie env-assetAlias-rollout
		"type":            resourceType, // service/deployment/rollout/etc
		"metadata":        metadata,     // this will be filled in differently for service/deployment/rollout/etc
	}
	return makeCallToRegistry(url, tid, http.MethodPut, data, c.client)
	// Where does sidecar and envoy filter go?
}

func (c *registryClient) DeleteHostingData(cluster, namespace, name, resourceType, tid string) error {
	url := fmt.Sprintf("%s/%s/k8s/clusters/%s/namespaces/%s/hostings/%s?type=%s", c.client.GetConfig().Host, c.client.GetConfig().BaseURI, cluster, namespace, name, resourceType)
	return makeCallToRegistry(url, tid, http.MethodDelete, nil, c.client)
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
