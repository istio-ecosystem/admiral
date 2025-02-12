package clusters

import (
	"context"
	"fmt"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
)

type RolloutHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

func (rh *RolloutHandler) Added(ctx context.Context, obj *argo.Rollout) error {
	err := HandleEventForRollout(ctx, admiral.Add, obj, rh.RemoteRegistry, rh.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Add, common.RolloutResourceType, obj.Name, rh.ClusterID, err)
	}
	return err
}

func (rh *RolloutHandler) Updated(ctx context.Context, obj *argo.Rollout) error {
	return nil
}

func (rh *RolloutHandler) Deleted(ctx context.Context, obj *argo.Rollout) error {
	err := HandleEventForRollout(ctx, admiral.Delete, obj, rh.RemoteRegistry, rh.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Delete, common.RolloutResourceType, obj.Name, rh.ClusterID, err)
	}
	return err
}

type HandleEventForRolloutFunc func(ctx context.Context, event admiral.EventType, obj *argo.Rollout,
	remoteRegistry *RemoteRegistry, clusterName string) error

// HandleEventForRollout helper function to handle add and delete for RolloutHandler
func HandleEventForRollout(ctx context.Context, event admiral.EventType, obj *argo.Rollout,
	remoteRegistry *RemoteRegistry, clusterName string) error {
	globalIdentifier := common.GetRolloutGlobalIdentifier(obj)
	originalIdentifier := common.GetRolloutOriginalIdentifier(obj)
	if len(globalIdentifier) == 0 {
		return nil
	}
	env := common.GetEnvForRollout(obj)

	ctx = context.WithValue(ctx, "clusterName", clusterName)
	ctx = context.WithValue(ctx, "eventResourceType", common.Rollout)

	if common.IsAdmiralStateSyncerMode() && common.IsStateSyncerCluster(clusterName) {
		if event != admiral.Delete {
			err := remoteRegistry.RegistryClient.PutHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, common.Rollout, ctx.Value("txId").(string), obj)
			if err != nil {
				log.Errorf(LogFormat, event, common.RolloutResourceType, obj.Name, clusterName, "failed to put "+common.Rollout+" hosting data for identity="+globalIdentifier)
			}
		} else {
			err := remoteRegistry.RegistryClient.DeleteHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, common.Rollout, ctx.Value("txId").(string))
			if err != nil {
				log.Errorf(LogFormat, event, common.RolloutResourceType, obj.Name, clusterName, "failed to delete "+common.Rollout+" hosting data for identity="+globalIdentifier)
			}
		}
	}

	if remoteRegistry.AdmiralCache != nil {
		if remoteRegistry.AdmiralCache.IdentityClusterCache != nil {
			remoteRegistry.AdmiralCache.IdentityClusterCache.Put(globalIdentifier, clusterName, clusterName)
		}
		if common.EnableSWAwareNSCaches() {
			if remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache != nil {
				remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Put(globalIdentifier, clusterName, obj.Namespace, obj.Namespace)
			}
			if remoteRegistry.AdmiralCache.PartitionIdentityCache != nil && len(common.GetRolloutIdentityPartition(obj)) > 0 {
				remoteRegistry.AdmiralCache.PartitionIdentityCache.Put(globalIdentifier, originalIdentifier)
			}
		}
	}

	// Use the same function as added deployment function to update and put new service entry in place to replace old one
	_, err := modifyServiceEntryForNewServiceOrPod(ctx, event, env, globalIdentifier, remoteRegistry)

	if common.ClientInitiatedProcessingEnabled() {
		log.Infof(LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "Client initiated processing started for "+globalIdentifier)
		rolloutProcessErr := processClientDependencyRecord(ctx, remoteRegistry, globalIdentifier, clusterName, obj.Namespace)
		if rolloutProcessErr != nil {
			return common.AppendError(err, rolloutProcessErr)
		}
	}
	return err
}
