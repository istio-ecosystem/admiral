package registry

import (
	"fmt"
	"sync"
)

// ClusterIdentityStore stores mapping of identity and
// the cluster in which resources for them need to be
// created
type ClusterIdentityStore interface {
	AddUpdateIdentityToCluster(identity ClusterIdentity, clusterName string) error
	RemoveIdentityToCluster(identity ClusterIdentity, clusterName string) error
	GetAllIdentitiesForCluster(clusterName string) (IdentityStore, error)
	AddIdentityConfiguration() error
}

type clusterIdentityStoreHandler struct {
	store clusterStore
}
type ClusterIdentity struct {
	IdentityName   string
	SourceIdentity bool
}

func NewClusterIdentity(name string, sourceIdentity bool) ClusterIdentity {
	return ClusterIdentity{
		IdentityName:   name,
		SourceIdentity: sourceIdentity,
	}
}

type IdentityStore struct {
	Store map[string]ClusterIdentity
}

type clusterStore struct {
	cache map[string]IdentityStore
	mutex *sync.RWMutex
}

func newClusterStore() clusterStore {
	return clusterStore{
		cache: make(map[string]IdentityStore),
		mutex: &sync.RWMutex{},
	}
}

func NewClusterIdentityStoreHandler() *clusterIdentityStoreHandler {
	return &clusterIdentityStoreHandler{
		store: newClusterStore(),
	}
}

func (s *clusterIdentityStoreHandler) AddUpdateIdentityToCluster(identity ClusterIdentity, clusterName string) error {
	err := s.addUpdateCache(identity, clusterName)
	return err
}

func (s *clusterIdentityStoreHandler) RemoveIdentityToCluster(identity ClusterIdentity, clusterName string) error {
	err := s.deleteCache(identity, clusterName)
	return err
}

func (s *clusterIdentityStoreHandler) GetAllIdentitiesForCluster(clusterName string) (IdentityStore, error) {
	if clusterName == "" {
		return IdentityStore{}, fmt.Errorf("empty cluster name=''")
	}
	cache, ok := s.store.cache[clusterName]
	if !ok {
		return IdentityStore{}, fmt.Errorf("no record for cluster=%s", clusterName)
	}
	return cache, nil
}

func (s *clusterIdentityStoreHandler) AddIdentityConfiguration() error {
	return nil
}

func (s *clusterIdentityStoreHandler) addUpdateCache(identity ClusterIdentity, clusterName string) error {
	defer s.store.mutex.Unlock()
	s.store.mutex.Lock()
	cache, ok := s.store.cache[clusterName]
	if !ok {
		s.store.cache[clusterName] = IdentityStore{
			Store: map[string]ClusterIdentity{
				identity.IdentityName: identity,
			},
		}
		return nil
	}
	cache.Store[identity.IdentityName] = identity
	return nil
}

func (s *clusterIdentityStoreHandler) deleteCache(identity ClusterIdentity, clusterName string) error {
	defer s.store.mutex.Unlock()
	s.store.mutex.Lock()
	cache, ok := s.store.cache[clusterName]
	if !ok {
		return nil
	}
	delete(cache.Store, identity.IdentityName)
	s.store.cache[clusterName] = cache
	return nil
}
