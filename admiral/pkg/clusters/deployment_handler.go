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

	log.Infof(LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, common.ReceivedStatus)
	globalIdentifier := common.GetDeploymentGlobalIdentifier(obj)
	log.Infof(LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "globalIdentifier is "+globalIdentifier)
	originalIdentifier := common.GetDeploymentOriginalIdentifier(obj)
	log.Infof(LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "originalIdentifier is "+originalIdentifier)

	if len(globalIdentifier) == 0 {
		log.Infof(LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return nil
	}

	env := common.GetEnv(obj)

	ctx = context.WithValue(ctx, common.ClusterName, clusterName)
	ctx = context.WithValue(ctx, common.EventResourceType, common.Deployment)

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
				log.Infof(LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "PartitionIdentityCachePut "+globalIdentifier+" for "+originalIdentifier)
			}
		}
	}

	// Use the same function as added deployment function to update and put new service entry in place to replace old one
	_, err := modifyServiceEntryForNewServiceOrPod(ctx, event, env, globalIdentifier, remoteRegistry)
	if common.ClientInitiatedProcessingEnabled() {
		log.Infof(LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "Client initiated processing started for "+globalIdentifier)
		depProcessErr := processClientDependencyRecord(ctx, remoteRegistry, globalIdentifier, clusterName, obj.Namespace)
		if depProcessErr != nil {
			return common.AppendError(err, depProcessErr)
		}
	}
	return err
}
