package common

import (
	"sync"
)

type Map struct {
	cache map[string]string
	mutex *sync.Mutex
}


type MapOfMaps struct {
	cache map[string]*Map
	mutex *sync.Mutex
}

type LabelSet struct {
	DeploymentAnnotation                string
	SubsetLabel                         string
	NamespaceSidecarInjectionLabel      string
	NamespaceSidecarInjectionLabelValue string
	AdmiralIgnoreLabel                  string
	WorkloadIdentityKey                 string //Should always be used for both label and annotation (using label as the primary, and falling back to annotation if the label is not found)
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
	return s.cache
}

func (s *MapOfMaps) Put(pkey string, key string, value string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	var mapVal = s.cache[pkey]
	if mapVal == nil {
		mapVal = NewMap()
		mapVal.Put(key, value)
	}
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

