package common

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
)

type Map struct {
	cache map[string]string
	mutex *sync.Mutex
}

type MapOfMaps struct {
	cache map[string]*Map
	mutex *sync.RWMutex
}

type MapOfMapOfMaps struct {
	cache map[string]*MapOfMaps
	mutex *sync.RWMutex
}

type MapOfMapOfMapsOfMaps struct {
	cache map[string]*MapOfMapOfMaps
	mutex *sync.RWMutex
}

type SidecarEgress struct {
	Namespace string
	FQDN      string
	CNAMEs    *Map
}

// maintains a map from workload identity -> map[namespace]SidecarEgress
type SidecarEgressMap struct {
	cache map[string]map[string]SidecarEgress
	mutex *sync.Mutex
}

type AdmiralParams struct {
	ArgoRolloutsEnabled                              bool
	KubeconfigPath                                   string
	SecretFilterTags                                 string
	CacheReconcileDuration                           time.Duration
	SeAndDrCacheReconcileDuration                    time.Duration
	ClusterRegistriesNamespace                       string
	DependenciesNamespace                            string
	DnsConfigFile                                    string
	DNSTimeoutMs                                     int
	DNSRetries                                       int
	TrafficConfigNamespace                           string
	SyncNamespace                                    string
	EnableSAN                                        bool
	SANPrefix                                        string
	AdmiralConfig                                    string
	Profile                                          string
	LabelSet                                         *LabelSet
	LogLevel                                         int
	HostnameSuffix                                   string
	PreviewHostnamePrefix                            string
	MetricsEnabled                                   bool
	ChannelCapacity                                  int
	WorkloadSidecarUpdate                            string
	WorkloadSidecarName                              string
	AdmiralStateCheckerName                          string
	DRStateStoreConfigPath                           string
	ServiceEntryIPPrefix                             string
	EnvoyFilterVersion                               string
	DeprecatedEnvoyFilterVersion                     string
	EnvoyFilterAdditionalConfig                      string
	EnableRoutingPolicy                              bool
	RoutingPolicyClusters                            []string
	ExcludedIdentityList                             []string
	AdditionalEndpointSuffixes                       []string
	AdditionalEndpointLabelFilters                   []string
	HAMode                                           string
	EnableWorkloadDataStorage                        bool
	EnableDiffCheck                                  bool
	EnableProxyEnvoyFilter                           bool
	EnableDependencyProcessing                       bool
	DeploymentOrRolloutWorkerConcurrency             int
	DependentClusterWorkerConcurrency                int
	SeAddressConfigmap                               string
	DependencyWarmupMultiplier                       int
	EnableOutlierDetection                           bool
	EnableClientConnectionConfigProcessing           bool
	MaxRequestsPerConnection                         int32
	EnableAbsoluteFQDN                               bool
	EnableAbsoluteFQDNForLocalEndpoints              bool
	DisableDefaultAutomaticFailover                  bool
	EnableServiceEntryCache                          bool
	AlphaIdentityList                                []string
	EnableDestinationRuleCache                       bool
	DisableIPGeneration                              bool
	EnableActivePassive                              bool
	EnableSWAwareNSCaches                            bool
	ClientInitiatedProcessingEnabledForControllers   bool
	ClientInitiatedProcessingEnabledForDynamicConfig bool
	InitiateClientInitiatedProcessingFor             []string
	ExportToIdentityList                             []string
	ExportToMaxNamespaces                            int
	EnableSyncIstioResourcesToSourceClusters         bool
	DefaultWarmupDurationSecs                        int64
	EnableGenerationCheck                            bool
	EnableIsOnlyReplicaCountChangedCheck             bool
	PreventSplitBrain                                bool
	IgnoreLabelsAnnotationsVSCopyList                []string

	// Cartographer specific params
	TrafficConfigPersona      bool
	TrafficConfigIgnoreAssets []string // used to ignore applying of client side envoy filters
	CartographerFeatures      map[string]string
	TrafficConfigScope        string
	LogToFile                 bool
	LogFilePath               string
	LogFileSizeInMBs          int

	// Air specific
	GatewayAssetAliases []string

	//Admiral 2.0 params
	AdmiralOperatorMode        bool
	OperatorSyncNamespace      string
	AdmiralStateSyncerMode     bool
	OperatorIdentityValue      string
	ShardIdentityValue         string
	OperatorSecretFilterTags   string
	RegistryClientHost         string
	RegistryClientAppId        string
	RegistryClientAppSecret    string
	RegistryClientBaseURI      string
	AdmiralAppEnv              string
	AdmiralStateSyncerClusters []string

	// VS Based Routing
	EnableVSRouting                   bool
	VSRoutingGateways                 []string
	IngressVSExportToNamespaces       []string
	IngressLBPolicy                   string
	VSRoutingDisabledClusters         []string
	VSRoutingSlowStartEnabledClusters []string

	// VS Based Routing In-Cluster
	EnableVSRoutingInCluster            bool
	VSRoutingInClusterEnabledResources  map[string]string
	VSRoutingInClusterDisabledResources map[string]string
	EnableCustomVSMerge                 bool
	ProcessVSCreatedBy                  string

	//Client discovery (types requiring mesh egress only)
	EnableClientDiscovery          bool
	ClientDiscoveryClustersForJobs []string
	DiscoveryClustersForNumaflow   []string

	//DynamicConfig setting
	EnableDynamicConfig            bool
	DynamicConfigDynamoDBTableName string
	DynamicSyncPeriod              int

	//NLB Specific
	NLBEnabledClusters     []string
	NLBEnabledIdentityList []string
	CLBEnabledClusters     []string
	NLBIngressLabel        string
	CLBIngressLabel        string

	// Slow Start
	EnableTrafficConfigProcessingForSlowStart bool
}

func (b AdmiralParams) String() string {
	return fmt.Sprintf("KubeconfigPath=%v ", b.KubeconfigPath) +
		fmt.Sprintf("CacheRefreshDuration=%v ", b.CacheReconcileDuration) +
		fmt.Sprintf("SEAndDRCacheRefreshDuration=%v ", b.SeAndDrCacheReconcileDuration) +
		fmt.Sprintf("ClusterRegistriesNamespace=%v ", b.ClusterRegistriesNamespace) +
		fmt.Sprintf("DependenciesNamespace=%v ", b.DependenciesNamespace) +
		fmt.Sprintf("EnableSAN=%v ", b.EnableSAN) +
		fmt.Sprintf("SANPrefix=%v ", b.SANPrefix) +
		fmt.Sprintf("LabelSet=%v ", b.LabelSet) +
		fmt.Sprintf("SecretResolver=%v ", b.Profile) +
		fmt.Sprintf("Profile=%v ", b.Profile) +
		fmt.Sprintf("AdmiralStateCheckerName=%v ", b.AdmiralStateCheckerName) +
		fmt.Sprintf("DRStateStoreConfigPath=%v ", b.DRStateStoreConfigPath) +
		fmt.Sprintf("ServiceEntryIPPrefix=%v ", b.ServiceEntryIPPrefix) +
		fmt.Sprintf("EnvoyFilterVersion=%v ", b.EnvoyFilterVersion) +
		fmt.Sprintf("DeprecatedEnvoyFilterVersion=%v ", b.DeprecatedEnvoyFilterVersion) +
		fmt.Sprintf("EnableRoutingPolicy=%v ", b.EnableRoutingPolicy) +
		fmt.Sprintf("TrafficConfigNamespace=%v ", b.TrafficConfigNamespace) +
		fmt.Sprintf("TrafficConfigPersona=%v ", b.TrafficConfigPersona) +
		fmt.Sprintf("CartographerFeatures=%v ", b.CartographerFeatures) +
		fmt.Sprintf("DefaultWarmupDuration=%v ", b.DefaultWarmupDurationSecs)
}

type LabelSet struct {
	DeploymentAnnotation                string
	SubsetLabel                         string
	NamespaceSidecarInjectionLabel      string
	NamespaceSidecarInjectionLabelValue string
	AdmiralIgnoreLabel                  string
	PriorityKey                         string
	WorkloadIdentityKey                 string //Should always be used for both label and annotation (using label as the primary, and falling back to annotation if the label is not found)
	TrafficConfigIdentityKey            string //Should always be used for both label and annotation (using label as the primary, and falling back to annotation if the label is not found)
	EnvKey                              string //key used to group deployments by env. The order would be to use annotation `EnvKey` and then label `EnvKey` and then fallback to label `env` label
	GatewayApp                          string //the value for `app` key that will be used to fetch the loadblancer for cross cluster calls, also referred to as east west gateway
	AdmiralCRDIdentityLabel             string //Label Used to identify identity label for crd
	IdentityPartitionKey                string //Label used for partitioning assets with same identity into groups
	ShardIdentityLabelKey               string
}

type TrafficObject struct {
	TrafficConfig *v1.TrafficConfig
	ClusterID     string
	Ctx           *Context
	Event         string
}

type Context struct {
	Ctx      context.Context
	Log      *log.Entry
	Property map[string]string
}

func NewSidecarEgressMap() *SidecarEgressMap {
	n := new(SidecarEgressMap)
	n.cache = make(map[string]map[string]SidecarEgress)
	n.mutex = &sync.Mutex{}
	return n
}

func NewMap() *Map {
	n := new(Map)
	n.cache = make(map[string]string)
	n.mutex = &sync.Mutex{}
	return n
}

func NewMapOfMaps() *MapOfMaps {
	n := new(MapOfMaps)
	n.cache = make(map[string]*Map)
	n.mutex = &sync.RWMutex{}
	return n
}

func NewMapOfMapOfMaps() *MapOfMapOfMaps {
	n := new(MapOfMapOfMaps)
	n.cache = make(map[string]*MapOfMaps)
	n.mutex = &sync.RWMutex{}
	return n
}

func NewMapOfMapOfMapOfMaps() *MapOfMapOfMapsOfMaps {
	n := new(MapOfMapOfMapsOfMaps)
	n.cache = make(map[string]*MapOfMapOfMaps)
	n.mutex = &sync.RWMutex{}
	return n
}

func (s *Map) Put(key string, value string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	s.cache[key] = value
}

func (s *Map) Get(key string) string {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	return s.cache[key]
}

func (s *Map) CheckIfPresent(key string) bool {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	if _, ok := s.cache[key]; ok {
		return true
	}
	return false
}

func (s *Map) Len() int {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	return len(s.cache)
}

func (s *Map) Delete(key string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	delete(s.cache, key)
}

func (s *Map) Copy() map[string]string {
	if s != nil {
		defer s.mutex.Unlock()
		s.mutex.Lock()
		var copy = make(map[string]string)
		for k, v := range s.cache {
			copy[k] = v
		}
		return copy
	} else {
		return nil
	}
}

func (s *Map) CopyJustValues() []string {
	var copy []string
	if s != nil {
		defer s.mutex.Unlock()
		s.mutex.Lock()
		for _, v := range s.cache {
			copy = append(copy, v)
		}
	}
	return copy
}

func (s *Map) Range(fn func(k string, v string)) {
	s.mutex.Lock()
	for k, v := range s.cache {
		fn(k, v)
	}
	s.mutex.Unlock()
}

func (s *MapOfMaps) Put(pkey string, key string, value string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	var mapVal = s.cache[pkey]
	if mapVal == nil {
		mapVal = NewMap()
	}
	mapVal.Put(key, value)
	s.cache[pkey] = mapVal
}

func (s *MapOfMaps) DeleteMap(pkey string, key string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	var mapVal = s.cache[pkey]
	if mapVal == nil {
		return
	}
	mapVal.Delete(key)
	s.cache[pkey] = mapVal
}

func (s *MapOfMaps) PutMap(pkey string, inputMap *Map) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	s.cache[pkey] = inputMap
}

func (s *MapOfMaps) Get(key string) *Map {
	s.mutex.Lock()
	val := s.cache[key]
	s.mutex.Unlock()
	return val
}

func (s *MapOfMaps) Delete(key string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	delete(s.cache, key)
}

func (s *MapOfMaps) Range(fn func(k string, v *Map)) {
	s.mutex.Lock()
	for k, v := range s.cache {
		fn(k, v)
	}
	s.mutex.Unlock()
}

func (s *MapOfMaps) Len() int {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	return len(s.cache)
}

func (s *MapOfMaps) GetKeys() []string {
	defer s.mutex.RUnlock()
	s.mutex.RLock()
	keys := []string{}
	for k := range s.cache {
		keys = append(keys, k)
	}
	return keys
}

func (s *MapOfMapOfMaps) Put(pkey string, skey string, key, value string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	var mapOfMapsVal = s.cache[pkey]
	if mapOfMapsVal == nil {
		mapOfMapsVal = NewMapOfMaps()
	}
	mapOfMapsVal.Put(skey, key, value)
	s.cache[pkey] = mapOfMapsVal
}

func (s *MapOfMapOfMaps) PutMapofMaps(key string, value *MapOfMaps) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	s.cache[key] = value
}

func (s *MapOfMapOfMaps) Get(key string) *MapOfMaps {
	s.mutex.RLock()
	val := s.cache[key]
	s.mutex.RUnlock()
	return val
}

func (s *MapOfMapOfMaps) Len() int {
	defer s.mutex.RUnlock()
	s.mutex.RLock()
	return len(s.cache)
}

func (s *Map) GetValues() []string {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	keys := make([]string, 0)
	for _, val := range s.cache {
		keys = append(keys, val)
	}
	return keys
}

func (s *Map) GetKeys() []string {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	keys := make([]string, 0)
	for key, _ := range s.cache {
		keys = append(keys, key)
	}
	return keys
}

func (s *SidecarEgressMap) Put(identity string, namespace string, fqdn string, cnames map[string]string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	var mapVal = s.cache[identity]
	if mapVal == nil {
		mapVal = make(map[string]SidecarEgress)
	}
	cnameMap := NewMap()
	for k, v := range cnames {
		cnameMap.Put(k, v)
	}
	mapVal[namespace] = SidecarEgress{Namespace: namespace, FQDN: fqdn, CNAMEs: cnameMap}
	s.cache[identity] = mapVal
}

func (s *SidecarEgressMap) Get(key string) map[string]SidecarEgress {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	return s.cache[key]
}

func (s *SidecarEgressMap) Delete(key string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	delete(s.cache, key)
}

// Range is a thread safe iterator to iterate through the SidecarEgress map
func (s *SidecarEgressMap) Range(fn func(k string, v map[string]SidecarEgress)) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	for k, v := range s.cache {
		fn(k, v)
	}
}

type ProxyFilterRequestObject struct {
	Identity           string
	ProxiedServiceInfo *ProxiedServiceInfo
	DnsConfigFile      string
	DnsRetries         int
	DnsTimeoutMs       int
	ClusterID          string
	Ctx                *Context
	Event              string
}

type ProxyFilterConfig struct {
	ConfigFile        string                `json:"configFile"`
	DNSTimeoutMs      int                   `json:"dnsTimeoutMs"`
	DNSRetries        int                   `json:"dnsRetries"`
	GatewayAssetAlias string                `json:"gatewayAssetAlias"`
	Services          []*ProxiedServiceInfo `json:"services"`
}

type ProxiedServiceInfo struct {
	Identity     string                       `json:"assetAlias"`
	ProxyAlias   string                       `json:"-"`
	Environments []*ProxiedServiceEnvironment `json:"environments"`
}

type ProxiedServiceEnvironment struct {
	Environment string   `json:"environment"`
	DnsName     string   `json:"dnsName"`
	CNames      []string `json:"cNames"`
}

func (c *ProxyFilterConfig) String() string {
	return fmt.Sprintf("{ConfigFile: %s, DNSTimeoutMs:%d, DNSRetries: %d, GatewayAssetAlias: %s, Services: %s}", c.ConfigFile, c.DNSTimeoutMs, c.DNSRetries, c.GatewayAssetAlias, c.Services)
}

func (s *ProxiedServiceInfo) String() string {
	return fmt.Sprintf("{Identity:%s, Enviroments: %v}", s.Identity, s.Environments)
}

func (s *ProxiedServiceEnvironment) String() string {
	return fmt.Sprintf("{Environment:%s, DnsName: %s, CNames: %s}", s.Environment, s.DnsName, s.CNames)
}

type K8sObject struct {
	Name        string
	Namespace   string
	Type        string
	Annotations map[string]string
	Labels      map[string]string
	Status      string
}

func (s *MapOfMapOfMapsOfMaps) Put(pkey string, skey string, tkey string, key, value string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	var mapOfMapOfMapsVal = s.cache[pkey]
	if mapOfMapOfMapsVal == nil {
		mapOfMapOfMapsVal = NewMapOfMapOfMaps()
	}
	mapOfMapOfMapsVal.Put(skey, tkey, key, value)
	s.cache[pkey] = mapOfMapOfMapsVal
}

func (s *MapOfMapOfMapsOfMaps) PutMapofMapsofMaps(key string, value *MapOfMapOfMaps) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	s.cache[key] = value
}

func (s *MapOfMapOfMapsOfMaps) Get(key string) *MapOfMapOfMaps {
	s.mutex.RLock()
	val := s.cache[key]
	s.mutex.RUnlock()
	return val
}

func (s *MapOfMapOfMapsOfMaps) Len() int {
	defer s.mutex.RUnlock()
	s.mutex.RLock()
	return len(s.cache)
}
