package registry

import "testing"

// Creates a ClusterIdentity with given name and sourceIdentity values
func TestNewClusterIdentityWithValidInputs(t *testing.T) {
	name := "test-identity"
	sourceIdentity := true

	identity := NewClusterIdentity(name, sourceIdentity)

	if identity.IdentityName != name {
		t.Errorf("expected IdentityName to be %s, got %s", name, identity.IdentityName)
	}

	if identity.SourceIdentity != sourceIdentity {
		t.Errorf("expected SourceIdentity to be %v, got %v", sourceIdentity, identity.SourceIdentity)
	}
}

func TestNewClusterStore(t *testing.T) {
	store := newClusterStore()

	if store.cache == nil {
		t.Error("expected cache to be initialized")
	}

	if store.mutex == nil {
		t.Error("expected mutex to be initialized")
	}
}

func TestNewClusterIdentityStoreHandler(t *testing.T) {
	handler := NewClusterIdentityStoreHandler()

	if handler.store.cache == nil {
		t.Error("expected cache to be initialized")
	}

	if handler.store.mutex == nil {
		t.Error("expected mutex to be initialized")
	}
}

func TestAddUpdateIdentityToCluster(t *testing.T) {
	handler := NewClusterIdentityStoreHandler()
	identity := NewClusterIdentity("test-identity", true)
	clusterName := "test-cluster"

	err := handler.AddUpdateIdentityToCluster(identity, clusterName)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRemoveIdentityToCluster(t *testing.T) {
	handler := NewClusterIdentityStoreHandler()
	identity := NewClusterIdentity("test-identity", true)
	clusterName := "test-cluster"

	err := handler.RemoveIdentityToCluster(identity, clusterName)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestGetAllIdentitiesForCluster(t *testing.T) {
	handler := NewClusterIdentityStoreHandler()
	identity := NewClusterIdentity("test-identity", true)
	clusterName := "test-cluster"

	err := handler.AddUpdateIdentityToCluster(identity, clusterName)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	cache, err := handler.GetAllIdentitiesForCluster(clusterName)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if cache.Store == nil {
		t.Error("expected cache to be initialized")
	}
}
