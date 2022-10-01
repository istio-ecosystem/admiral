package common

import (
	"fmt"
	"sync"
	"time"
)

type Map struct {
	cache map[string]string
	mutex *sync.Mutex
}

type MapOfMaps struct {
	cache map[string]*Map
	mutex *sync.Mutex
}

type SidecarEgress struct {
	Namespace string
	FQDN      string
	CNAMEs    map[string]string
}

//maintains a map from workload identity -> map[namespace]SidecarEgress
type SidecarEgressMap struct {
	cache map[string]map[string]SidecarEgress
	mutex *sync.Mutex
}

type AdmiralParams struct {
	ArgoRolloutsEnabled         bool
	KubeconfigPath              string
	CacheRefreshDuration        time.Duration
	ClusterRegistriesNamespace  string
	DependenciesNamespace       string
	SyncNamespace               string
	EnableSAN                   bool
	SANPrefix                   string
	SecretResolver              string
	LabelSet                    *LabelSet
	LogLevel                    int
	HostnameSuffix              string
	PreviewHostnamePrefix       string
	MetricsEnabled              bool
	WorkloadSidecarUpdate       string
	WorkloadSidecarName         string
	AdmiralStateCheckerName     string
	DRStateStoreConfigPath      string
	ServiceEntryIPPrefix        string
	EnvoyFilterVersion          string
	EnvoyFilterAdditionalConfig string
	EnableRoutingPolicy         bool
	ExcludedIdentityList        []string
}

func (b AdmiralParams) String() string {
	return fmt.Sprintf("KubeconfigPath=%v ", b.KubeconfigPath) +
		fmt.Sprintf("CacheRefreshDuration=%v ", b.CacheRefreshDuration) +
		fmt.Sprintf("ClusterRegistriesNamespace=%v ", b.ClusterRegistriesNamespace) +
		fmt.Sprintf("DependenciesNamespace=%v ", b.DependenciesNamespace) +
		fmt.Sprintf("EnableSAN=%v ", b.EnableSAN) +
		fmt.Sprintf("SANPrefix=%v ", b.SANPrefix) +
		fmt.Sprintf("LabelSet=%v ", b.LabelSet) +
		fmt.Sprintf("SecretResolver=%v ", b.SecretResolver) +
		fmt.Sprintf("AdmiralStateCheckername=%v ", b.AdmiralStateCheckerName) +
		fmt.Sprintf("DRStateStoreConfigPath=%v ", b.DRStateStoreConfigPath) +
		fmt.Sprintf("ServiceEntryIPPrefix=%v ", b.ServiceEntryIPPrefix) +
		fmt.Sprintf("EnvoyFilterVersion=%v ", b.EnvoyFilterVersion) +
		fmt.Sprintf("EnableRoutingPolicy=%v ", b.EnableRoutingPolicy)
}

type LabelSet struct {
	DeploymentAnnotation                string
	SubsetLabel                         string
	NamespaceSidecarInjectionLabel      string
	NamespaceSidecarInjectionLabelValue string
	AdmiralIgnoreLabel                  string
	PriorityKey                         string
	WorkloadIdentityKey                 string //Should always be used for both label and annotation (using label as the primary, and falling back to annotation if the label is not found)
	GlobalTrafficDeploymentLabel        string //label used to tie together deployments and globaltrafficpolicy objects. Configured separately from the identity key because this one _must_ be a label
	EnvKey                              string //key used to group deployments by env. The order would be to use annotation `EnvKey` and then label `EnvKey` and then fallback to label `env` label
	GatewayApp                          string //the value for `app` key that will be used to fetch the loadblancer for cross cluster calls, also referred to as east west gateway
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
	n.mutex = &sync.Mutex{}
	return n
}

func (s *Map) Put(key string, value string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	s.cache[key] = value
}

func (s *Map) Get(key string) string {
	return s.cache[key]
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

func (s *MapOfMaps) Map() map[string]*Map {
	return s.cache
}

func (s *MapOfMaps) Range(fn func(k string, v *Map)) {
	s.mutex.Lock()
	for k, v := range s.cache {
		fn(k, v)
	}
	s.mutex.Unlock()
}

func (s *SidecarEgressMap) Put(identity string, namespace string, fqdn string, cnames map[string]string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	var mapVal = s.cache[identity]
	if mapVal == nil {
		mapVal = make(map[string]SidecarEgress)
	}
	mapVal[namespace] = SidecarEgress{Namespace: namespace, FQDN: fqdn, CNAMEs: cnames}
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

// Map func returns a map of identity to namespace:SidecarEgress map
// Iterating through the returned map is not implicitly thread safe,
// use (s *SidecarEgressMap) Range() func instead.
func (s *SidecarEgressMap) Map() map[string]map[string]SidecarEgress {
	return s.cache
}

// Range is a thread safe iterator to iterate through the SidecarEgress map
func (s *SidecarEgressMap) Range(fn func(k string, v map[string]SidecarEgress)) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	for k, v := range s.cache {
		fn(k, v)
	}
}
