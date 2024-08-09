package clusters

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/pkg/errors"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	"github.com/sirupsen/logrus"
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
	logrus.Infof("source asset=%s is present in clusters=%v, and has destinations=%v",
		identity, sourceClusters, destinationAssets)
	return nil
}

func updateRegistryConfigForClusterPerEnvironment(ctxLogger *logrus.Entry, remoteRegistry *RemoteRegistry, registryConfig registry.IdentityConfig) error {
	task := "updateRegistryConfigForClusterPerEnvironment"
	defer util.LogElapsedTimeForTask(ctxLogger, task, registryConfig.IdentityName, "", "", "processingTime")()
	k8sClient, err := remoteRegistry.ClientLoader.LoadKubeClientFromPath(common.GetKubeconfigPath())
	if err != nil && common.GetSecretFilterTags() == "admiral/syncrtay" {
		ctxLogger.Infof(common.CtxLogFormat, task, registryConfig.IdentityName, "", "", "unable to get kube client")
		return errors.Wrap(err, "unable to get kube client")
	}
	for _, clusterConfig := range registryConfig.Clusters {
		clusterName := clusterConfig.Name
		ctxLogger.Infof(common.CtxLogFormat, task, registryConfig.IdentityName, "", clusterName, "processing cluster")
		for _, environmentConfig := range clusterConfig.Environment {
			environmentName := environmentConfig.Name
			ctxLogger.Infof(common.CtxLogFormat, task, registryConfig.IdentityName, "", clusterName, "processing environment="+environmentName)
			err := remoteRegistry.ConfigSyncer.UpdateEnvironmentConfigByCluster(
				ctxLogger,
				environmentName,
				clusterName,
				registryConfig,
				registry.NewConfigMapWriter(k8sClient, ctxLogger),
			)
			if err != nil {
				ctxLogger.Errorf(common.CtxLogFormat, task, registryConfig.IdentityName, "", clusterName, "processing environment="+environmentName+" error="+err.Error())
				return err
			}
		}
	}
	return nil
}
