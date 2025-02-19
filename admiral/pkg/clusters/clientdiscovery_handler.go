package clusters

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	log "github.com/sirupsen/logrus"
)

type ClientDiscoveryHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

func (cdh *ClientDiscoveryHandler) Added(ctx context.Context, obj *common.K8sObject) error {
	err := HandleEventForClientDiscovery(ctx, admiral.Add, obj, cdh.RemoteRegistry, cdh.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Add, common.JobResourceType, obj.Name, cdh.ClusterID, err)
	}
	return err
}

func HandleEventForClientDiscovery(ctx context.Context, event admiral.EventType, obj *common.K8sObject,
	remoteRegistry *RemoteRegistry, clusterName string) error {
	globalIdentifier := common.GetGlobalIdentifier(obj.Annotations, obj.Labels)
	originalIdentifier := common.GetOriginalIdentifier(obj.Annotations, obj.Labels)
	if len(globalIdentifier) == 0 {
		return nil
	}

	ctx = context.WithValue(ctx, "clusterName", clusterName)
	ctx = context.WithValue(ctx, "eventResourceType", obj.Type)

	_ = callRegistryForClientDiscovery(ctx, event, remoteRegistry, globalIdentifier, clusterName, obj)

	if remoteRegistry.AdmiralCache != nil {

		UpdateIdentityClusterCache(remoteRegistry, globalIdentifier, clusterName)

		if common.EnableSWAwareNSCaches() {
			if remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache != nil {
				remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Put(globalIdentifier, clusterName, obj.Namespace, obj.Namespace)
			}
			if remoteRegistry.AdmiralCache.PartitionIdentityCache != nil && len(common.GetIdentityPartition(obj.Annotations, obj.Labels)) > 0 {
				remoteRegistry.AdmiralCache.PartitionIdentityCache.Put(globalIdentifier, originalIdentifier)
			}
		}
	} else {
		log.Warnf(LogFormatAdv, "Process", obj.Type, obj.Name, obj.Namespace, clusterName, "Skipping client discovery as Admiral cache is not initialized for identity="+globalIdentifier)
		return fmt.Errorf(common.CtxLogFormat, event, obj.Name, obj.Namespace, clusterName, "processing skipped as Admiral cache is not initialized for identity="+globalIdentifier)
	}

	if commonUtil.IsAdmiralReadOnly() {
		return nil
	}

	if IsCacheWarmupTime(remoteRegistry) {
		return fmt.Errorf(common.CtxLogFormat, event, obj.Name, obj.Namespace, clusterName, "processing skipped during cache warm up state for env="+" identity="+globalIdentifier)
	}

	//if we have a deployment/rollout in this namespace skip processing to save some cycles
	if DeploymentOrRolloutExistsInNamespace(remoteRegistry, globalIdentifier, clusterName, obj.Namespace) {
		return nil
	}

	//write SEs required for this client
	depRecord := remoteRegistry.DependencyController.Cache.Get(globalIdentifier)

	if depRecord == nil {
		return nil
	}
	err := remoteRegistry.DependencyController.DepHandler.Added(ctx, depRecord)

	if err != nil {
		return fmt.Errorf(LogFormatAdv, "Process", obj.Type, obj.Name, obj.Namespace, clusterName, "Error processing client discovery")
	}

	return nil
}

func callRegistryForClientDiscovery(ctx context.Context, event admiral.EventType, registry *RemoteRegistry, globalIdentifier string, clusterName string, obj *common.K8sObject) error {
	var err error
	if common.IsAdmiralStateSyncerMode() && common.IsStateSyncerCluster(clusterName) && registry.RegistryClient != nil {
		switch event {
		case admiral.Add:
			err = registry.RegistryClient.PutHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, obj.Type, ctx.Value("txId").(string), obj)
		case admiral.Update:
			err = registry.RegistryClient.PutHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, obj.Type, ctx.Value("txId").(string), obj)
		case admiral.Delete:
			err = registry.RegistryClient.DeleteHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, obj.Type, ctx.Value("txId").(string))
		}
		if err != nil {
			err = fmt.Errorf(LogFormat, event, obj.Type, obj.Name, clusterName, "failed to "+string(event)+" "+obj.Type+" with err: "+err.Error())
			log.Error(err)
		}
	}
	return err
}
