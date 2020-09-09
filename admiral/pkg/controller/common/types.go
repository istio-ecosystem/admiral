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
	ArgoRolloutsEnabled        bool
	KubeconfigPath             string
	CacheRefreshDuration       time.Duration
	ClusterRegistriesNamespace string
	DependenciesNamespace      string
	SyncNamespace              string
	EnableSAN                  bool
	SANPrefix                  string
	SecretResolver             string
	LabelSet                   *LabelSet
	LogLevel                   int
	HostnameSuffix             string
	WorkloadSidecarUpdate      string
	WorkloadSidecarName        string
}

func (b AdmiralParams) String() string {
	return fmt.Sprintf("KubeconfigPath=%v ", b.KubeconfigPath) +
		fmt.Sprintf("CacheRefreshDuration=%v ", b.CacheRefreshDuration) +
		fmt.Sprintf("ClusterRegistriesNamespace=%v ", b.ClusterRegistriesNamespace) +
		fmt.Sprintf("DependenciesNamespace=%v ", b.DependenciesNamespace) +
		fmt.Sprintf("EnableSAN=%v ", b.EnableSAN) +
		fmt.Sprintf("SANPrefix=%v ", b.SANPrefix) +
		fmt.Sprintf("LabelSet=%v ", b.LabelSet) +
		fmt.Sprintf("SecretResolver=%v ", b.SecretResolver)
}

type LabelSet struct {
	DeploymentAnnotation                string
	SubsetLabel                         string
	NamespaceSidecarInjectionLabel      string
	NamespaceSidecarInjectionLabelValue string
	AdmiralIgnoreLabel                  string
	WorkloadIdentityKey                 string //Should always be used for both label and annotation (using label as the primary, and falling back to annotation if the label is not found)
	GlobalTrafficDeploymentLabel        string //label used to tie together deployments and globaltrafficpolicy objects. Configured separately from the identity key because this one _must_ be a label
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

func (s *Map) Map() map[string]string {
	if s != nil {
		return s.cache
	} else {
		return nil
	}
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

func (s *MapOfMaps) Get(key string) *Map {
	return s.cache[key]
}

func (s *MapOfMaps) Delete(key string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	delete(s.cache, key)
}

func (s *MapOfMaps) Map() map[string]*Map {
	return s.cache
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
	return s.cache[key]
}

func (s *SidecarEgressMap) Delete(key string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	delete(s.cache, key)
}

func (s *SidecarEgressMap) Map() map[string]map[string]SidecarEgress {
	return s.cache
}
