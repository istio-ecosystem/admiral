package clusters

import (
	"context"
	"fmt"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
)

type DestinationServiceProcessor interface {
	Process(ctx context.Context, dependency *v1.Dependency,
		remoteRegistry *RemoteRegistry, eventType admiral.EventType,
		modifySE ModifySEFunc) error
}
type ProcessDestinationService struct {
}
type DependencyHandler struct {
	RemoteRegistry              *RemoteRegistry
	DepController               *admiral.DependencyController
	DestinationServiceProcessor DestinationServiceProcessor
	RoutingPolicyProcessor      RoutingPolicyProcessor
}

func (dh *DependencyHandler) Added(ctx context.Context, obj *v1.Dependency) error {
	log.Debugf(LogFormat, common.Add, common.DependencyResourceType, obj.Name, "", common.ReceivedStatus)
	return dh.HandleDependencyRecord(ctx, obj, dh.RemoteRegistry, admiral.Add)
}

func (dh *DependencyHandler) Updated(ctx context.Context, obj *v1.Dependency) error {
	log.Debugf(LogFormat, common.Update, common.DependencyResourceType, obj.Name, "", common.ReceivedStatus)
	// need clean up before handle it as added, I need to handle update that delete the dependency, find diff first
	// this is more complex cos want to make sure no other service depend on the same service (which we just removed the dependancy).
	// need to make sure nothing depend on that before cleaning up the SE for that service
	return dh.HandleDependencyRecord(ctx, obj, dh.RemoteRegistry, admiral.Update)
}

func (dh *DependencyHandler) Deleted(ctx context.Context, obj *v1.Dependency) error {
	// special case of update, delete the dependency crd file for one service, need to loop through all ones we plan to update
	// and make sure nobody else is relying on the same SE in same cluster
	log.Debugf(LogFormat, common.Delete, common.DependencyResourceType, obj.Name, "", "Skipping Delete operation")
	return nil
}

func (dh *DependencyHandler) HandleDependencyRecord(ctx context.Context, obj *v1.Dependency,
	remoteRegistry *RemoteRegistry, eventType admiral.EventType) error {
	sourceIdentity := obj.Spec.Source
	if len(sourceIdentity) == 0 {
		log.Infof(LogFormat, string(eventType), common.DependencyResourceType, obj.Name, "", "No identity found namespace="+obj.Namespace)
		return nil
	}

	err := updateIdentityDependencyCache(sourceIdentity, remoteRegistry.AdmiralCache.IdentityDependencyCache, obj)
	if err != nil {
		log.Errorf(LogErrFormat, string(eventType), common.DependencyResourceType, obj.Name, "", "error adding into dependency cache ="+err.Error())
		return err
	}

	log.Debugf(LogFormat, string(eventType), common.DependencyResourceType, obj.Name, "", fmt.Sprintf("added destinations to admiral sourceToDestinations cache. destinationsLength=%d", len(obj.Spec.Destinations)))

	var handleDepRecordErrors error

	// process routing policies.
	err = dh.RoutingPolicyProcessor.ProcessDependency(ctx, eventType, obj)
	if err != nil {
		log.Errorf(LogErrFormat, string(eventType), common.DependencyResourceType, obj.Name, "", fmt.Sprintf("error processing routing policies %v", err))
	}

	// Generate SE/DR/VS for all newly added destination services in the source's cluster
	err = dh.DestinationServiceProcessor.Process(ctx,
		obj,
		remoteRegistry,
		eventType,
		modifyServiceEntryForNewServiceOrPod)
	if err != nil {
		log.Errorf(LogErrFormat, string(eventType),
			common.DependencyResourceType, obj.Name, "", err.Error())
		handleDepRecordErrors = common.AppendError(handleDepRecordErrors, err)
		// This will be re-queued and retried
		return handleDepRecordErrors
	}

	remoteRegistry.AdmiralCache.SourceToDestinations.put(obj)
	return handleDepRecordErrors
}

func isIdentityMeshEnabled(identity string, remoteRegistry *RemoteRegistry) bool {
	if remoteRegistry.AdmiralCache.IdentityClusterCache.Get(identity) != nil {
		return true
	}
	return false
}

func (d *ProcessDestinationService) Process(ctx context.Context, dependency *v1.Dependency,
	remoteRegistry *RemoteRegistry, eventType admiral.EventType, modifySE ModifySEFunc) error {

	if IsCacheWarmupTimeForDependency(remoteRegistry) {
		log.Debugf(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", "processing skipped during cache warm up state")
		return nil
	}

	if !common.IsDependencyProcessingEnabled() {
		log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", "dependency processing is disabled")
		return nil
	}

	destinations, hasNonMeshDestination := getDestinationsToBeProcessed(eventType, dependency, remoteRegistry)
	id := common.GetIdentity(dependency.Labels, nil)
	ctxLogger := common.GetCtxLogger(ctx, id, "-")
	ctxLogger.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", fmt.Sprintf("found %d new destinations: %v", len(destinations), destinations))

	// find source cluster for source identity
	sourceClusters := remoteRegistry.AdmiralCache.IdentityClusterCache.Get(dependency.Spec.Source)
	if sourceClusters == nil {
		// Identity cluster cache does not have entry for identity because
		// the rollout/deployment event hasn't gone through yet.
		// This can be ignored, and not be added back to the dependency controller queue
		// because it will be processed by the rollout/deployment controller
		ctxLogger.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", fmt.Sprintf("identity: %s, does not have any clusters. Skipping calling modifySE", dependency.Spec.Source))
		return nil
	}

	err := processDestinationsForSourceIdentity(ctx, remoteRegistry, eventType, hasNonMeshDestination, sourceClusters, destinations, dependency.Name, modifySE)
	return err
}
