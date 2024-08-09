package registry

// ClusterShardStore stores mapping of clusters
// and the shard they belong to
type ClusterShardStore interface {
	AddClusterToShard(cluster, shard string) error
	RemoveClusterFromShard(cluster, shard string) error
	AddAllClustersToShard(clusters []string, shard string) error
}

type clusterShardStoreHandler struct {
}

func NewClusterShardStoreHandler() *clusterShardStoreHandler {
	return &clusterShardStoreHandler{}
}

func (c *clusterShardStoreHandler) AddClusterToShard(cluster, shard string) error {
	return nil
}

func (c *clusterShardStoreHandler) RemoveClusterFromShard(cluster, shard string) error {
	return nil
}

func (c *clusterShardStoreHandler) AddAllClustersToShard(clusters []string, shard string) error {
	return nil
}
