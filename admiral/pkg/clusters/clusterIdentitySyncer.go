package clusters

import (
	"fmt"

	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	log "github.com/sirupsen/logrus"
)

func updateClusterIdentityCache(
	remoteRegistry *RemoteRegistry,
	sourceClusters []string,
	identity string) error {

	if remoteRegistry == nil {
		return fmt.Errorf("remote registry is not initialized")
	}
	if remoteRegistry.AdmiralCache == nil {
		return fmt.Errorf("admiral cache is not initialized")
	}

	if remoteRegistry.AdmiralCache.SourceToDestinations == nil {
		return fmt.Errorf("source to destination cache is not populated")
	}
	// find assets this identity needs to call
	destinationAssets := remoteRegistry.AdmiralCache.SourceToDestinations.Get(identity)
	for _, cluster := range sourceClusters {
		sourceClusterIdentity := registry.NewClusterIdentity(identity, true)
		err := remoteRegistry.ClusterIdentityStoreHandler.AddUpdateIdentityToCluster(sourceClusterIdentity, cluster)
		if err != nil {
			return err
		}
		for _, destinationAsset := range destinationAssets {
			destinationClusterIdentity := registry.NewClusterIdentity(destinationAsset, false)
			err := remoteRegistry.ClusterIdentityStoreHandler.AddUpdateIdentityToCluster(destinationClusterIdentity, cluster)
			if err != nil {
				return err
			}
		}
	}
	log.Infof("source asset=%s is present in clusters=%v, and has destinations=%v",
		identity, sourceClusters, destinationAssets)
	return nil
}
