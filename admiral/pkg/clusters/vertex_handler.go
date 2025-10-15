package clusters

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"github.com/numaproj/numaflow/pkg/apis/numaflow/v1alpha1"
)

type VertexHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

func (pc *VertexHandler) Added(ctx context.Context, obj *v1alpha1.Vertex) error {
	err := HandleEventForVertex(ctx, admiral.Add, obj, pc.RemoteRegistry, pc.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Add, common.Vertex, obj.Name, pc.ClusterID, err)
	}
	return nil
}

func (pc *VertexHandler) Deleted(ctx context.Context, obj *v1alpha1.Vertex) error {
	err := HandleEventForVertex(ctx, admiral.Delete, obj, pc.RemoteRegistry, pc.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Delete, common.Vertex, obj.Name, pc.ClusterID, err)
	}
	return nil
}

// HandleEventForVertexFunc is a handler function for vertex events
type HandleEventForVertexFunc func(
	ctx context.Context, event admiral.EventType, obj *v1alpha1.Vertex,
	remoteRegistry *RemoteRegistry, clusterName string) error

// helper function to handle add and delete for VertexHandler
func HandleEventForVertex(ctx context.Context, event admiral.EventType, obj *v1alpha1.Vertex,
	remoteRegistry *RemoteRegistry, clusterName string) error {

	globalIdentifier := common.GetVertexGlobalIdentifier(obj)
	originalIdentifier := common.GetVertexOriginalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		return nil
	}

	env := common.GetEnvForVertex(obj)

	ctx = context.WithValue(ctx, common.ClusterName, clusterName)
	ctx = context.WithValue(ctx, common.EventResourceType, common.Vertex)

	_ = callRegistryForVertex(ctx, event, remoteRegistry, globalIdentifier, clusterName, obj)

	if remoteRegistry.AdmiralCache != nil {
		if remoteRegistry.AdmiralCache.IdentityClusterCache != nil {
			remoteRegistry.AdmiralCache.IdentityClusterCache.Put(globalIdentifier, clusterName, clusterName)
		}
		if common.EnableSWAwareNSCaches() {
			if remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache != nil {
				remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Put(globalIdentifier, clusterName, obj.Namespace, obj.Namespace)
			}
			if remoteRegistry.AdmiralCache.PartitionIdentityCache != nil && len(common.GetVertexIdentityPartition(obj)) > 0 {
				remoteRegistry.AdmiralCache.PartitionIdentityCache.Put(globalIdentifier, originalIdentifier)
			}
		}
	}

	// Use the same function as added vertex function to update and put new service entry in place to replace old one
	_, err := modifyServiceEntryForNewServiceOrPod(ctx, event, env, globalIdentifier, remoteRegistry)
	if common.ClientInitiatedProcessingEnabledForControllers() {
		var c ClientDependencyRecordProcessor
		log.Infof(LogFormat, event, common.Vertex, obj.Name, clusterName, "Client initiated processing started for "+globalIdentifier)
		depProcessErr := c.processClientDependencyRecord(ctx, remoteRegistry, globalIdentifier, clusterName, obj.Namespace, false)
		if depProcessErr != nil {
			return common.AppendError(err, depProcessErr)
		}
	}
	return err
}

func callRegistryForVertex(ctx context.Context, event admiral.EventType, remoteRegistry *RemoteRegistry, globalIdentifier string, clusterName string, obj *v1alpha1.Vertex) error {
	var err error
	if common.IsAdmiralStateSyncerMode() && common.IsStateSyncerCluster(clusterName) && remoteRegistry.RegistryClient != nil {
		switch event {
		case admiral.Add:
			err = remoteRegistry.RegistryClient.PutHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, common.Vertex, ctx.Value("txId").(string), obj)
		case admiral.Update:
			err = remoteRegistry.RegistryClient.PutHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, common.Vertex, ctx.Value("txId").(string), obj)
		case admiral.Delete:
			err = remoteRegistry.RegistryClient.DeleteHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, common.Vertex, ctx.Value("txId").(string))
		}
		if err != nil {
			err = fmt.Errorf(LogFormat, event, common.Vertex, obj.Name, clusterName, "failed to "+string(event)+" "+common.Vertex+" with err: "+err.Error())
			log.Error(err)
		}
	}
	return err
}
