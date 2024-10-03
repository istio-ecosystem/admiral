package clusters

import (
	"context"
	"fmt"
	"strings"

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

func getDestinationsToBeProcessed(eventType admiral.EventType,
	updatedDependency *v1.Dependency, remoteRegistry *RemoteRegistry) ([]string, bool) {
	updatedDestinations := make([]string, 0)
	existingDestination := remoteRegistry.AdmiralCache.SourceToDestinations.Get(updatedDependency.Spec.Source)
	var nonMeshEnabledExists bool
	lookup := make(map[string]bool)
	//if this is an update, build a look up table to process only the diff
	if eventType == admiral.Update {
		for _, dest := range existingDestination {
			lookup[dest] = true
		}
	}

	for _, destination := range updatedDependency.Spec.Destinations {
		if !isIdentityMeshEnabled(destination, remoteRegistry) {
			nonMeshEnabledExists = true
		}
		if ok := lookup[destination]; !ok {
			updatedDestinations = append(updatedDestinations, destination)
		}
	}
	return updatedDestinations, nonMeshEnabledExists
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
	log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", fmt.Sprintf("found %d new destinations: %v", len(destinations), destinations))

	var processingErrors error
	var message string
	counter := 1
	totalDestinations := len(destinations)
	// find source cluster for source identity
	sourceClusters := remoteRegistry.AdmiralCache.IdentityClusterCache.Get(dependency.Spec.Source)
	if sourceClusters == nil {
		// Identity cluster cache does not have entry for identity because
		// the rollout/deployment event hasn't gone through yet.
		// This can be ignored, and not be added back to the dependency controller queue
		// because it will be processed by the rollout/deployment controller
		log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", fmt.Sprintf("identity: %s, does not have any clusters. Skipping calling modifySE", dependency.Spec.Source))
		return nil
	}

	for _, destinationIdentity := range destinations {
		if strings.Contains(strings.ToLower(destinationIdentity), strings.ToLower(common.ServicesGatewayIdentity)) &&
			!hasNonMeshDestination {
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "",
				fmt.Sprintf("All destinations are MESH enabled. Skipping processing: %v. Destinations: %v", destinationIdentity, dependency.Spec.Destinations))
			continue
		}

		// In case of self on-boarding skip the update for the destination as it is the same as the source
		if strings.EqualFold(dependency.Spec.Source, destinationIdentity) {
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "",
				fmt.Sprintf("Destination identity is same as source identity. Skipping processing: %v.", destinationIdentity))
			continue
		}

		destinationClusters := remoteRegistry.AdmiralCache.IdentityClusterCache.Get(destinationIdentity)
		log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", fmt.Sprintf("processing destination %d/%d destinationIdentity=%s", counter, totalDestinations, destinationIdentity))
		clusters := remoteRegistry.AdmiralCache.IdentityClusterCache.Get(destinationIdentity)
		if destinationClusters == nil || destinationClusters.Len() == 0 {
			listOfSourceClusters := strings.Join(sourceClusters.GetKeys(), ",")
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, listOfSourceClusters,
				fmt.Sprintf("destinationClusters does not have any clusters. Skipping processing: %v.", destinationIdentity))
			continue
		}
		if clusters == nil {
			// When destination identity's cluster is not found, then
			// skip calling modify SE because:
			// 1. The destination identity might be NON MESH. Which means this error will always happen
			//    and there is no point calling modifySE.
			// 2. It could be that the IdentityClusterCache is not updated.
			//    It is the deployment/rollout controllers responsibility to update the cache
			//    without which the cache will always be empty. Now when deployment/rollout event occurs
			//    that will result in calling modify SE and perform the same operations which this function is trying to do
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "",
				fmt.Sprintf("no cluster found for destinationIdentity: %s. Skipping calling modifySE", destinationIdentity))
			continue
		}

		for _, destinationClusterID := range clusters.GetKeys() {
			message = fmt.Sprintf("processing cluster=%s for destinationIdentity=%s", destinationClusterID, destinationIdentity)
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", message)
			rc := remoteRegistry.GetRemoteController(destinationClusterID)
			if rc == nil {
				processingErrors = common.AppendError(processingErrors,
					fmt.Errorf("no remote controller found in cache for cluster %s", destinationClusterID))
				continue
			}
			ctx = context.WithValue(ctx, "clusterName", destinationClusterID)

			if rc.DeploymentController != nil {
				deploymentEnvMap := rc.DeploymentController.Cache.GetByIdentity(destinationIdentity)
				if len(deploymentEnvMap) != 0 {
					ctx = context.WithValue(ctx, "eventResourceType", common.Deployment)
					ctx = context.WithValue(ctx, common.DependentClusterOverride, sourceClusters)
					for env := range deploymentEnvMap {
						message = fmt.Sprintf("calling modifySE for env=%s destinationIdentity=%s", env, destinationIdentity)
						log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", message)
						_, err := modifySE(ctx, eventType, env, destinationIdentity, remoteRegistry)
						if err != nil {
							message = fmt.Sprintf("error occurred in modifySE func for env=%s destinationIdentity=%s", env, destinationIdentity)
							log.Errorf(LogErrFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", err.Error()+". "+message)
							processingErrors = common.AppendError(processingErrors, err)
						}
					}
					continue
				}
			}
			if rc.RolloutController != nil {
				rolloutEnvMap := rc.RolloutController.Cache.GetByIdentity(destinationIdentity)
				if len(rolloutEnvMap) != 0 {
					ctx = context.WithValue(ctx, "eventResourceType", common.Rollout)
					ctx = context.WithValue(ctx, common.DependentClusterOverride, sourceClusters)
					for env := range rolloutEnvMap {
						message = fmt.Sprintf("calling modifySE for env=%s destinationIdentity=%s", env, destinationIdentity)
						log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", message)
						_, err := modifySE(ctx, eventType, env, destinationIdentity, remoteRegistry)
						if err != nil {
							message = fmt.Sprintf("error occurred in modifySE func for env=%s destinationIdentity=%s", env, destinationIdentity)
							log.Errorf(LogErrFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", err.Error()+". "+message)
							processingErrors = common.AppendError(processingErrors, err)
						}
					}
					continue
				}
			}
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "", fmt.Sprintf("done processing destinationIdentity=%s", destinationIdentity))
			log.Warnf(LogFormat, string(eventType), common.DependencyResourceType, dependency.Name, "",
				fmt.Sprintf("neither deployment or rollout controller initialized in cluster %s and destination identity %s", destinationClusterID, destinationIdentity))
			counter++
		}
	}
	return processingErrors
}
