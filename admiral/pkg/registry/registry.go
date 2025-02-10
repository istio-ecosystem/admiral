package registry

import (
	"encoding/json"
	"fmt"
	util2 "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"io"
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
	PutHostingData(cluster, namespace, name, assetAlias, resourceType, tid string, value interface{}) error
	DeleteHostingData(cluster, namespace, name, assetAlias, resourceType, tid string) error
}

type RegistryClient struct {
	Client util2.BaseClient
}

func NewRegistryClient(options ...func(client *RegistryClient)) *RegistryClient {
	registryClient := &RegistryClient{}
	for _, o := range options {
		o(registryClient)
	}
	return registryClient
}

func WithBaseClientConfig(clientConfig *util2.Config) func(*RegistryClient) {
	return func(c *RegistryClient) {
		c.Client = util2.NewClient(clientConfig)
	}
}

func NewDefaultRegistryClient() *RegistryClient {
	registryClientParams := common.GetRegistryClientConfig()
	defaultRegistryClientConfig := &util2.Config{
		Host:      registryClientParams["Host"],
		AppId:     registryClientParams["AppId"],
		AppSecret: registryClientParams["AppSecret"],
		BaseURI:   registryClientParams["BaseURI"],
	}
	return NewRegistryClient(WithBaseClientConfig(defaultRegistryClientConfig))
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

func makeCallToRegistry(url, tid, method string, data map[string]interface{}, client util2.BaseClient) error {
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
		respBody := "nil response body"
		if response.Body != nil {
			defer response.Body.Close()
			bytes, err := io.ReadAll(response.Body)
			if err != nil {
				log.Errorf("op=%v type=%v url=%v data=%v, err=%s txId=%v", "readResponseBody", "responseErr", url, data, err.Error(), tid)
			}
			respBody = string(bytes)
		}
		return fmt.Errorf("response code for request to %s was %v with body: %v", url, response.StatusCode, respBody)
	}
	return nil
}

func (c *RegistryClient) PutClusterGateway(cluster, name, ingressURL, notes, resourceType, tid string, labels []string) error {
	url := fmt.Sprintf("%s/%s/k8s/clusters/%s/gateways/%s?type=%s", c.Client.GetConfig().Host, c.Client.GetConfig().BaseURI, cluster, name, resourceType)
	data := map[string]interface{}{
		"name":  name, // name of the actual gateway object in the cluster, not assetAlias
		"url":   ingressURL,
		"label": strings.Join(labels, ", "), // sort labels?
		"notes": notes,                      // not sure what the point of this is
		"type":  resourceType,
	}
	return makeCallToRegistry(url, tid, http.MethodPut, data, c.Client)
	//preferably put this in service handler where we check if common.IsIstioIngressGatewayService(svc)
}

func (c *RegistryClient) DeleteClusterGateway(cluster, name, resourceType, tid string) error {
	url := fmt.Sprintf("%s/%s/k8s/clusters/%s/gateways/%s?type=%s", c.Client.GetConfig().Host, c.Client.GetConfig().BaseURI, cluster, name, resourceType)
	return makeCallToRegistry(url, tid, http.MethodDelete, nil, c.Client)
}

func (c *RegistryClient) PutCustomData(cluster, namespace, name, resourceType, tid string, value interface{}) error {
	url := fmt.Sprintf("%s/%s/k8s/clusters/%s/namespaces/%s/customdata/%s?type=%s", c.Client.GetConfig().Host, c.Client.GetConfig().BaseURI, cluster, namespace, name, resourceType)
	byteVal, _ := json.Marshal(value)
	strVal := string(byteVal)
	data := map[string]interface{}{
		"name":  name,         // name of the object in the cluster, not assetAlias
		"type":  resourceType, // GlobalTrafficPolicy/CCC/OD/VS/etc
		"value": strVal,       // could be the entire yaml or whatever we want here
	}
	return makeCallToRegistry(url, tid, http.MethodPut, data, c.Client)
	// traffic config?
}

func (c *RegistryClient) DeleteCustomData(cluster, namespace, name, resourceType, tid string) error {
	url := fmt.Sprintf("%s/%s/k8s/clusters/%s/namespaces/%s/customdata/%s?type=%s", c.Client.GetConfig().Host, c.Client.GetConfig().BaseURI, cluster, namespace, name, resourceType)
	return makeCallToRegistry(url, tid, http.MethodDelete, nil, c.Client)
}

func (c *RegistryClient) PutHostingData(cluster, namespace, name, assetAlias, resourceType, tid string, value interface{}) error {
	hostingMetadataUrl := fmt.Sprintf("%s/%s/k8s/clusters/%s/namespaces/%s/hostings/%s/metadata/%s?type=%s&env=%s&assetAlias=%s", c.Client.GetConfig().Host, c.Client.GetConfig().BaseURI, cluster, namespace, name, "obj", resourceType, common.GetAdmiralAppEnv(), assetAlias)
	byteVal, err := json.Marshal(value)
	strVal := string(byteVal)
	hostingMetadataPayload := map[string]interface{}{
		"json": strVal,
		"key":  "obj", //needs to be the same the value after metadata/
		"type": resourceType,
		//"value": "string",
	}
	if err != nil {
		log.Errorf("op=%v type=%v name=%v cluster=%s message=%v", "registryEvent", "hosting", name, cluster, "jsonMarshalErr: "+err.Error()+", strVal: "+strVal+", hostingMetadataUrl: "+hostingMetadataUrl+", txId: "+tid)
	} else {
		log.Errorf("op=%v type=%v name=%v cluster=%s message=%v", "registryEvent", "hosting", name, cluster, "strVal: "+strVal+", hostingMetadataUrl: "+hostingMetadataUrl+", txId: "+tid)
	}
	metadataErr := makeCallToRegistry(hostingMetadataUrl, tid, http.MethodPut, hostingMetadataPayload, c.Client)
	if metadataErr != nil {
		// Check if call failed because this hosting did not exist in registry - if so, then create the namespace and hosting and populate with the metadata separately
		// The metadata is added in a separate call so that we avoid a race condition between two hosting metadata calls for a new hosting overwriting one another
		log.Errorf("op=%v type=%v name=%v cluster=%s message=%v", "registryEvent", "hostingMetadataErr", name, cluster, "metadataerr: "+metadataErr.Error()+", hostingMetadataUrl: "+hostingMetadataUrl+", txId: "+tid)
		if strings.Contains(metadataErr.Error(), "404") {
			namespaceUrl := fmt.Sprintf("%s/%s/k8s/clusters/%s/namespaces/%s", c.Client.GetConfig().Host, c.Client.GetConfig().BaseURI, cluster, namespace)
			namespacePayload := map[string]interface{}{
				"name": namespace,
			}
			namespaceErr := makeCallToRegistry(namespaceUrl, tid, http.MethodPut, namespacePayload, c.Client)
			if namespaceErr != nil {
				log.Errorf("op=%v type=%v name=%v cluster=%s message=%v", "registryEvent", "namespaceErr", name, cluster, "namespaceerr: "+namespaceErr.Error()+", namespaceUrl: "+namespaceUrl+", txId: "+tid)
			}
			hostingUrl := fmt.Sprintf("%s/%s/k8s/clusters/%s/namespaces/%s/hostings/%s?type=%s&env=%s&assetAlias=%s", c.Client.GetConfig().Host, c.Client.GetConfig().BaseURI, cluster, namespace, name, resourceType, common.GetAdmiralAppEnv(), assetAlias)
			hostingPayload := map[string]interface{}{
				"assetAlias": assetAlias,
				"env":        common.GetAdmiralAppEnv(),
				"name":       name, //k8s hosting object name
				"type":       resourceType,
			}
			hostingErr := makeCallToRegistry(hostingUrl, tid, http.MethodPut, hostingPayload, c.Client)
			if hostingErr != nil {
				log.Errorf("op=%v type=%v name=%v cluster=%s message=%v", "registryEvent", "hostingErr", name, cluster, "hostingerr: "+hostingErr.Error()+", hostingUrl: "+hostingUrl+", txId: "+tid)
				return hostingErr
			} else {
				//TODO: possibly this needs to run regardless of error or not - if two hostings picked up, could have race condition here
				//Also, the hostingErr could fail due to duplicate key error
				return makeCallToRegistry(hostingMetadataUrl, tid, http.MethodPut, hostingMetadataPayload, c.Client)
			}
		}
	}
	return metadataErr
	// Where does sidecar and envoy filter go?
}

func (c *RegistryClient) DeleteHostingData(cluster, namespace, name, assetAlias, resourceType, tid string) error {
	url := fmt.Sprintf("%s/%s/k8s/clusters/%s/namespaces/%s/hostings/%s/metadata/%s?type=%s&env=%s&assetAlias=%s", c.Client.GetConfig().Host, c.Client.GetConfig().BaseURI, cluster, namespace, assetAlias, name, resourceType, common.GetAdmiralAppEnv(), assetAlias)
	// When do we delete the actual hosting instead of the metadata? IMO this should be handled by registry when the actual asset is deleted.
	return makeCallToRegistry(url, tid, http.MethodDelete, nil, c.Client)
}

// GetIdentityConfigByIdentityName calls the registry API to fetch the IdentityConfig for
// the given identityAlias
func (c *RegistryClient) GetIdentityConfigByIdentityName(identityAlias string, ctxLogger *log.Entry) (IdentityConfig, error) {
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
func (c *RegistryClient) GetIdentityConfigByClusterName(clusterName string, ctxLogger *log.Entry) ([]IdentityConfig, error) {
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
