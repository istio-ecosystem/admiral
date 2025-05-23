package clusters

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	k8sAppsV1 "k8s.io/api/apps/v1"
)

type DeploymentHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

func (pc *DeploymentHandler) Added(ctx context.Context, obj *k8sAppsV1.Deployment) error {
	err := HandleEventForDeployment(ctx, admiral.Add, obj, pc.RemoteRegistry, pc.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Add, common.DeploymentResourceType, obj.Name, pc.ClusterID, err)
	}
	return nil
}

func (pc *DeploymentHandler) Deleted(ctx context.Context, obj *k8sAppsV1.Deployment) error {
	err := HandleEventForDeployment(ctx, admiral.Delete, obj, pc.RemoteRegistry, pc.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Delete, common.DeploymentResourceType, obj.Name, pc.ClusterID, err)
	}
	return nil
}

// HandleEventForDeploymentFunc is a handler function for deployment events
type HandleEventForDeploymentFunc func(
	ctx context.Context, event admiral.EventType, obj *k8sAppsV1.Deployment,
	remoteRegistry *RemoteRegistry, clusterName string) error

// helper function to handle add and delete for DeploymentHandler
func HandleEventForDeployment(ctx context.Context, event admiral.EventType, obj *k8sAppsV1.Deployment,
	remoteRegistry *RemoteRegistry, clusterName string) error {

	globalIdentifier := common.GetDeploymentGlobalIdentifier(obj)
	originalIdentifier := common.GetDeploymentOriginalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		return nil
	}

	env := common.GetEnv(obj)

	ctx = context.WithValue(ctx, common.ClusterName, clusterName)
	ctx = context.WithValue(ctx, common.EventResourceType, common.Deployment)

	_ = callRegistryForDeployment(ctx, event, remoteRegistry, globalIdentifier, clusterName, obj)

	if remoteRegistry.AdmiralCache != nil {
		if remoteRegistry.AdmiralCache.IdentityClusterCache != nil {
			remoteRegistry.AdmiralCache.IdentityClusterCache.Put(globalIdentifier, clusterName, clusterName)
		}
		if common.EnableSWAwareNSCaches() {
			if remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache != nil {
				remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Put(globalIdentifier, clusterName, obj.Namespace, obj.Namespace)
			}
			if remoteRegistry.AdmiralCache.PartitionIdentityCache != nil && len(common.GetDeploymentIdentityPartition(obj)) > 0 {
				remoteRegistry.AdmiralCache.PartitionIdentityCache.Put(globalIdentifier, originalIdentifier)
			}
		}
	}

	// Use the same function as added deployment function to update and put new service entry in place to replace old one
	_, err := modifyServiceEntryForNewServiceOrPod(ctx, event, env, globalIdentifier, remoteRegistry)
	if common.ClientInitiatedProcessingEnabledForControllers() {
		var c ClientDependencyRecordProcessor
		log.Infof(LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "Client initiated processing started for "+globalIdentifier)
		depProcessErr := c.processClientDependencyRecord(ctx, remoteRegistry, globalIdentifier, clusterName, obj.Namespace, false)
		if depProcessErr != nil {
			return common.AppendError(err, depProcessErr)
		}
	}
	return err
}

func callRegistryForDeployment(ctx context.Context, event admiral.EventType, registry *RemoteRegistry, globalIdentifier string, clusterName string, obj *k8sAppsV1.Deployment) error {
	var err error
	if common.IsAdmiralStateSyncerMode() && common.IsStateSyncerCluster(clusterName) && registry.RegistryClient != nil {
		switch event {
		case admiral.Add:
			err = registry.RegistryClient.PutHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, common.Deployment, ctx.Value("txId").(string), obj)
		case admiral.Update:
			err = registry.RegistryClient.PutHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, common.Deployment, ctx.Value("txId").(string), obj)
		case admiral.Delete:
			err = registry.RegistryClient.DeleteHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, common.Deployment, ctx.Value("txId").(string))
		}
		if err != nil {
			err = fmt.Errorf(LogFormat, event, common.Deployment, obj.Name, clusterName, "failed to "+string(event)+" "+common.Deployment+" with err: "+err.Error())
			log.Error(err)
		}
	}
	return err
}
