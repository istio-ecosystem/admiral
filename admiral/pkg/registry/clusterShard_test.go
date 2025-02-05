package registry

import "testing"

func TestNewClusterShardStoreHandler(t *testing.T) {
	handler := NewClusterShardStoreHandler()

	if handler == nil {
		t.Error("expected handler to be initialized")
	}
}

func TestAddClusterToShard(t *testing.T) {
	handler := NewClusterShardStoreHandler()
	cluster := "test-cluster"
	shard := "test-shard"

	err := handler.AddClusterToShard(cluster, shard)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRemoveClusterFromShard(t *testing.T) {
	handler := NewClusterShardStoreHandler()
	cluster := "test-cluster"
	shard := "test-shard"

	err := handler.RemoveClusterFromShard(cluster, shard)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestAddAllClustersToShard(t *testing.T) {
	handler := NewClusterShardStoreHandler()
	clusters := []string{"test-cluster1", "test-cluster2"}
	shard := "test-shard"

	err := handler.AddAllClustersToShard(clusters, shard)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
