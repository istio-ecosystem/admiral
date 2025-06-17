package clusters

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/common/log"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	networking "istio.io/api/networking/v1alpha3"
	k8sV1 "k8s.io/api/core/v1"
)

type WorkloadEntrySorted []*networking.WorkloadEntry
type TLSRoutesSorted []*networking.TLSRoute

func GetMeshPortsForRollout(clusterName string, destService *k8sV1.Service,
	destRollout *argo.Rollout) map[string]uint32 {
	if destService == nil || destRollout == nil {
		logrus.Warnf("Rollout or Service is nil cluster=%s", clusterName)
		return nil
	}

	var meshPorts string
	if destRollout.Spec.Template.Annotations == nil {
		meshPorts = ""
	} else {
		meshPorts = destRollout.Spec.Template.Annotations[common.SidecarEnabledPorts]
	}
	ports := common.GetMeshPortsHelper(meshPorts, destService, clusterName)
	return ports
}

// Get the service selector to add as workload selector for envoyFilter
func GetServiceSelector(clusterName string, destService *k8sV1.Service) *common.Map {
	var selectors = destService.Spec.Selector
	if len(selectors) == 0 {
		return nil
	}
	var tempMap = common.NewMap()
	for key, value := range selectors {
		tempMap.Put(key, value)
	}
	return tempMap
}

func GetServiceEntryStateFromConfigmap(configmap *k8sV1.ConfigMap) *ServiceEntryAddressStore {

	bytes := []byte(configmap.Data["serviceEntryAddressStore"])
	addressStore := ServiceEntryAddressStore{}
	err := yaml.Unmarshal(bytes, &addressStore)

	if err != nil {
		logrus.Errorf("Could not unmarshal configmap data. Double check the configmap format. %v", err)
		return nil
	}
	if addressStore.Addresses == nil {
		addressStore.Addresses = []string{}
	}
	if addressStore.EntryAddresses == nil {
		addressStore.EntryAddresses = map[string]string{}
	}

	return &addressStore
}

func ValidateConfigmapBeforePutting(cm *k8sV1.ConfigMap) error {
	if cm.ResourceVersion == "" {
		return errors.New("resourceversion required") //without it, we can't be sure someone else didn't put something between our read and write
	}
	store := GetServiceEntryStateFromConfigmap(cm)
	if len(store.EntryAddresses) != len(store.Addresses) {
		return errors.New("address cache length mismatch") //should be impossible. We're in a state where the list of addresses doesn't match the map of se:address. Something's been missed and must be fixed
	}
	return nil
}

func IsCacheWarmupTime(remoteRegistry *RemoteRegistry) bool {
	return time.Since(remoteRegistry.StartTime) < common.GetAdmiralParams().CacheReconcileDuration
}

func IsCacheWarmupTimeForDependency(remoteRegistry *RemoteRegistry) bool {
	return time.Since(remoteRegistry.StartTime) < (common.GetAdmiralParams().CacheReconcileDuration * time.Duration(common.DependencyWarmupMultiplier()))
}

// removeSeEndpoints is used determine if we want to add, update or delete the endpoints for the current cluster being processed.
// Based on this information we will decide if we should add, update or delete the SE in the source as well as dependent clusters.
func removeSeEndpoints(eventCluster string, event admiral.EventType, clusterId string, deployToRolloutMigration bool, appType string, clusterAppDeleteMap map[string]string) (admiral.EventType, bool) {
	eventType := event
	deleteCluster := false

	if event == admiral.Delete {
		if eventCluster == clusterId {
			deleteCluster = true
			// If both the deployment and rollout are present and the cluster for which
			// the function was called is not the cluster for which the delete event was sent
			// we update the event to admiral.Update
			if deployToRolloutMigration && appType != clusterAppDeleteMap[eventCluster] {
				eventType = admiral.Update
			}
		} else {
			eventType = admiral.Update
		}
	}

	return eventType, deleteCluster
}

// GenerateServiceEntryForCanary - generates a service entry only for canary endpoint
// This is required for rollouts to test only canary version of the services
func GenerateServiceEntryForCanary(ctxLogger *logrus.Entry, ctx context.Context, event admiral.EventType, rc *RemoteController, admiralCache *AdmiralCache, meshPorts map[string]uint32, destRollout *argo.Rollout, serviceEntries map[string]*networking.ServiceEntry, workloadIdentityKey string, san []string, sourceIdentity string) error {

	if destRollout.Spec.Strategy.Canary != nil && destRollout.Spec.Strategy.Canary.CanaryService != "" &&
		destRollout.Spec.Strategy.Canary.TrafficRouting != nil && destRollout.Spec.Strategy.Canary.TrafficRouting.Istio != nil {
		rolloutServices := GetAllServicesForRollout(rc, destRollout)
		logrus.Debugf("number of services %d matched for rollout %s in namespace=%s and cluster=%s", len(rolloutServices), destRollout.Name, destRollout.Namespace, rc.ClusterID)
		if rolloutServices == nil {
			return nil
		}
		if _, ok := rolloutServices[destRollout.Spec.Strategy.Canary.CanaryService]; ok {
			canaryGlobalFqdn := common.CanaryRolloutCanaryPrefix + common.Sep + common.GetCnameForRollout(destRollout, workloadIdentityKey, common.GetHostnameSuffix())
			admiralCache.CnameIdentityCache.Store(canaryGlobalFqdn, common.GetRolloutGlobalIdentifier(destRollout))
			err := generateSECanary(ctxLogger, ctx, event, rc, admiralCache, meshPorts, serviceEntries, san, canaryGlobalFqdn, sourceIdentity)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Returns all services that match the rollot selector, in case of canary strategy this should return a map with root, stable and canary services
func GetAllServicesForRollout(rc *RemoteController, rollout *argo.Rollout) map[string]*WeightedService {

	if rollout == nil {
		return nil
	}

	if rollout.Spec.Selector == nil || rollout.Spec.Selector.MatchLabels == nil {
		return nil
	}

	cachedServices := rc.ServiceController.Cache.Get(rollout.Namespace)

	if cachedServices == nil {
		return nil
	}
	var matchedServices = make(map[string]*WeightedService)

	for _, service := range cachedServices {
		match := common.IsServiceMatch(service.Spec.Selector, rollout.Spec.Selector)
		//make sure the service matches the rollout Selector and also has a mesh port in the port spec
		if match {
			ports := GetMeshPortsForRollout(rc.ClusterID, service, rollout)
			if len(ports) > 0 {
				//Weights are not important, this just returns list of all services matching rollout
				matchedServices[service.Name] = &WeightedService{Weight: 1, Service: service}
				logrus.Debugf("service matched=%s rollout=%s in namespace=%s and cluster=%s", service.Name, rollout.Name, rollout.Namespace, rc.ClusterID)
			}
		}
	}
	return matchedServices
}

// generateSECanary generates uniqui IP address for the SE, it also calls generateServiceEntry to create the skeleton Service entry
func generateSECanary(ctxLogger *logrus.Entry, ctx context.Context, event admiral.EventType, rc *RemoteController, admiralCache *AdmiralCache, meshPorts map[string]uint32, serviceEntries map[string]*networking.ServiceEntry, san []string, fqdn string, sourceIdentity string) error {

	address, err := getUniqueAddress(ctxLogger, ctx, admiralCache, fqdn)
	if err != nil {
		logrus.Errorf("failed to generate unique address for canary fqdn - %v error - %v", fqdn, err)
		return err
	}
	// This check preserves original behavior of checking for non-empty fqdn and address before
	// generating SE when disable_ip_generation=false. When disable_ip_generation=true, it still
	// checks for non-empty fqdn but allows for empty address.
	if len(fqdn) != 0 && (common.DisableIPGeneration() || len(address) != 0) {
		generateServiceEntry(ctxLogger, event, admiralCache, meshPorts, fqdn, rc, serviceEntries, address, san, common.Rollout, sourceIdentity)
	}
	return nil
}

// Checks if istio strategy is used by rollout, also if there is a canary service defined in the spec
func IsCanaryIstioStrategy(rollout *argo.Rollout) bool {
	if rollout != nil && &rollout.Spec != (&argo.RolloutSpec{}) && rollout.Spec.Strategy != (argo.RolloutStrategy{}) {
		if rollout.Spec.Strategy.Canary != nil && rollout.Spec.Strategy.Canary.TrafficRouting != nil && rollout.Spec.Strategy.Canary.TrafficRouting.Istio != nil &&
			len(rollout.Spec.Strategy.Canary.CanaryService) > 0 {
			return true
		}
	}
	return false
}

// filterClusters removes the clusters from the sourceClusters which are co-located in
// the same cluster as the destination service
func filterClusters(sourceClusters, destinationClusters *common.Map) *common.Map {
	filteredSourceClusters := common.NewMap()
	sourceClusters.Range(func(k string, v string) {
		if destinationClusters != nil && !destinationClusters.CheckIfPresent(k) {
			filteredSourceClusters.Put(k, v)
		}
	})
	return filteredSourceClusters
}

// getSortedDependentNamespaces takes a cname and reduces it to its base form (without canary/bluegreen prefix) and fetches the partitionedIdentity based on that
// Then, it checks if the clusterId matches any of the source clusters, and if so, adds istio-system to the list of dependent namespaces
// Then, it fetches the dependent namespaces based on the cname or cnameWithoutPrefix and adds them to the list of dependent namespaces
// If the list is above the maximum number of allowed exportTo values, it replaces the entries with "*"
// Otherwise, it sorts and dedups the list of dependent namespaces and returns them.
func getSortedDependentNamespaces(
	admiralCache *AdmiralCache,
	cname string,
	clusterId string,
	ctxLogger *logrus.Entry,
	skipIstioNSFromExportTo bool) []string {

	var clusterNamespaces *common.MapOfMaps
	var namespaceSlice []string
	var cnameWithoutPrefix string
	cname = strings.ToLower(cname)
	if strings.HasPrefix(cname, common.CanaryRolloutCanaryPrefix+common.Sep) {
		cnameWithoutPrefix = strings.TrimPrefix(cname, common.CanaryRolloutCanaryPrefix+common.Sep)
	} else if strings.HasPrefix(cname, common.BlueGreenRolloutPreviewPrefix+common.Sep) {
		cnameWithoutPrefix = strings.TrimPrefix(cname, common.BlueGreenRolloutPreviewPrefix+common.Sep)
	}
	if admiralCache == nil || admiralCache.CnameDependentClusterNamespaceCache == nil {
		return namespaceSlice
	}
	//This section gets the identity and uses it to fetch the identity's source clusters
	//If the cluster we are fetching dependent namespaces for is also a source cluster
	//Then we add istio-system to the list of namespaces for ExportTo
	if admiralCache.CnameIdentityCache != nil {
		partitionedIdentity, ok := admiralCache.CnameIdentityCache.Load(cname)
		if ok && admiralCache.IdentityClusterCache != nil {
			sourceClusters := admiralCache.IdentityClusterCache.Get(partitionedIdentity.(string))
			if sourceClusters != nil && sourceClusters.Get(clusterId) != "" {
				namespaceSlice = append(namespaceSlice, common.NamespaceIstioSystem)

				// Add source namespaces s.t. throttle filter can query envoy clusters
				if admiralCache.IdentityClusterNamespaceCache != nil && admiralCache.IdentityClusterNamespaceCache.Get(partitionedIdentity.(string)) != nil {
					sourceNamespacesInCluster := admiralCache.IdentityClusterNamespaceCache.Get(partitionedIdentity.(string)).Get(clusterId)
					if sourceNamespacesInCluster != nil && sourceNamespacesInCluster.Len() > 0 {
						namespaceSlice = append(namespaceSlice, sourceNamespacesInCluster.GetValues()...)
					}
				}
			}
		}
	}
	cnameWithoutPrefix = strings.TrimSpace(cnameWithoutPrefix)
	clusterNamespaces = admiralCache.CnameDependentClusterNamespaceCache.Get(cname)
	if clusterNamespaces == nil && cnameWithoutPrefix != "" {
		clusterNamespaces = admiralCache.CnameDependentClusterNamespaceCache.Get(cnameWithoutPrefix)
		if clusterNamespaces != nil {
			admiralCache.CnameDependentClusterNamespaceCache.PutMapofMaps(cname, clusterNamespaces)
			ctxLogger.Infof("clusterNamespaces for prefixed cname %v  was empty, replacing with clusterNamespaces for %v", cname, cnameWithoutPrefix)
		}
	}
	if clusterNamespaces != nil && clusterNamespaces.Len() > 0 {
		namespaces := clusterNamespaces.Get(clusterId)
		if namespaces != nil && namespaces.Len() > 0 {
			namespaceSlice = append(namespaceSlice, namespaces.GetValues()...)
			if len(namespaceSlice) > common.GetExportToMaxNamespaces() {
				namespaceSlice = []string{"*"}
				ctxLogger.Infof("exceeded max namespaces for cname=%s in cluster=%s", cname, clusterId)
			}
			sort.Strings(namespaceSlice)
		}
	}
	// this is to avoid duplication in namespaceSlice e.g. dynamicrouting deployment present in istio-system can be a dependent of blackhole on blackhole's source cluster
	var dedupNamespaceSlice []string
	for i := 0; i < len(namespaceSlice); i++ {
		if i == 0 || namespaceSlice[i] != namespaceSlice[i-1] {
			dedupNamespaceSlice = append(dedupNamespaceSlice, namespaceSlice[i])
		}
	}
	var finalDeDupedNamespaces []string
	for _, s := range dedupNamespaceSlice {
		if skipIstioNSFromExportTo && s == common.NamespaceIstioSystem {
			continue
		}
		finalDeDupedNamespaces = append(finalDeDupedNamespaces, s)
	}
	ctxLogger.Infof("getSortedDependentNamespaces for cname %v and cluster %v got namespaces: %v", cname, clusterId, finalDeDupedNamespaces)
	return finalDeDupedNamespaces
}

func (w WorkloadEntrySorted) Len() int {
	return len(w)
}

func (w WorkloadEntrySorted) Less(i, j int) bool {
	return w[i].Address < w[j].Address
}

func (w WorkloadEntrySorted) Swap(i, j int) {
	w[i], w[j] = w[j], w[i]
}

// TODO: it should return an error when locality is not found
func getLocality(rc *RemoteController) string {
	if rc.NodeController.Locality != nil {
		return rc.NodeController.Locality.Region
	}
	return ""
}

func getIngressPortName(meshPorts map[string]uint32) string {
	var finalProtocol = commonUtil.Http
	for protocol := range meshPorts {
		finalProtocol = protocol
	}
	return finalProtocol
}

func parseWeightedService(weightedServices map[string]*WeightedService, meshPorts map[string]uint32) []*registry.RegistryServiceConfig {
	services := make([]*registry.RegistryServiceConfig, 0, len(weightedServices))

	for _, serviceInstance := range weightedServices {
		if serviceInstance.Weight <= 0 {
			continue
		}
		services = append(services, &registry.RegistryServiceConfig{
			Name:      serviceInstance.Service.Name,
			Weight:    int(serviceInstance.Weight),
			Ports:     meshPorts,
			Selectors: serviceInstance.Service.Spec.Selector,
		})
	}
	return services
}

func parseMigrationService(migrationServices map[string]*k8sV1.Service, meshPorts map[string]map[string]uint32) []*registry.RegistryServiceConfig {
	services := make([]*registry.RegistryServiceConfig, 0, len(migrationServices))
	services = append(services, &registry.RegistryServiceConfig{
		Name:      migrationServices[common.Deployment].Name,
		Ports:     meshPorts[common.Deployment],
		Selectors: migrationServices[common.Deployment].Spec.Selector,
	})
	services = append(services, &registry.RegistryServiceConfig{
		Name:      migrationServices[common.Rollout].Name,
		Ports:     meshPorts[common.Rollout],
		Selectors: migrationServices[common.Rollout].Spec.Selector,
	})
	return services
}

type ProcessClientDependencyRecord interface {
	processClientDependencyRecord(ctx context.Context, remoteRegistry *RemoteRegistry, globalIdentifier string, clusterName string, clientNs string, bypass bool) error
}
type ClientDependencyRecordProcessor struct{}

func (c ClientDependencyRecordProcessor) processClientDependencyRecord(ctx context.Context, remoteRegistry *RemoteRegistry, globalIdentifier string, clusterName string, clientNs string, bypass bool) error {
	var destinationsToBeProcessed []string
	if IsCacheWarmupTimeForDependency(remoteRegistry) {
		log.Debugf(LogFormat, "Update", common.DependencyResourceType, globalIdentifier, clusterName, "processing skipped during cache warm up state for dependency")
		return nil
	}

	destinationsToBeProcessed = getDestinationsToBeProcessedForClientInitiatedProcessing(remoteRegistry, globalIdentifier, clusterName, clientNs, destinationsToBeProcessed, bypass)
	log.Infof(LogFormat, "Update", common.DependencyResourceType, globalIdentifier, clusterName, fmt.Sprintf("destinationsToBeProcessed=%v", destinationsToBeProcessed))
	if len(destinationsToBeProcessed) == 0 {
		log.Infof(LogFormat, "Update", common.DependencyResourceType, globalIdentifier, clusterName, "no destinations to be processed")
		return nil
	}
	err := processDestinationsForSourceIdentity(ctx, remoteRegistry, "Update", true, common.NewMap(), destinationsToBeProcessed, globalIdentifier, modifyServiceEntryForNewServiceOrPod)

	if err != nil {
		return errors.New("failed to perform client initiated processing for " + globalIdentifier + ", got error: " + err.Error())
	}
	return nil
}

func getDestinationsToBeProcessedForClientInitiatedProcessing(remoteRegistry *RemoteRegistry, globalIdentifier string, clusterName string, clientNs string, destinationsToBeProcessed []string, bypass bool) []string {
	actualServerIdentities := remoteRegistry.AdmiralCache.SourceToDestinations.Get(globalIdentifier)
	processedClientClusters := remoteRegistry.AdmiralCache.ClientClusterNamespaceServerCache.Get(clusterName)

	if actualServerIdentities == nil {
		return nil
	}
	var meshServerIdentities []string

	for _, actualServerIdentity := range actualServerIdentities {
		if isIdentityMeshEnabled(actualServerIdentity, remoteRegistry) {
			meshServerIdentities = append(meshServerIdentities, actualServerIdentity)
		}
	}

	if bypass || processedClientClusters == nil || processedClientClusters.Get(clientNs) == nil {
		destinationsToBeProcessed = meshServerIdentities
	} else {
		processedClientNamespaces := processedClientClusters.Get(clientNs)
		for _, actualServerIdentity := range meshServerIdentities {
			if processedClientNamespaces.Get(actualServerIdentity) == "" {
				destinationsToBeProcessed = append(destinationsToBeProcessed, actualServerIdentity)
			}
		}
	}
	sort.Strings(destinationsToBeProcessed)
	// Remove duplicates
	var dedupDestinationSlice []string
	for i := 0; i < len(destinationsToBeProcessed); i++ {
		if i == 0 || destinationsToBeProcessed[i] != destinationsToBeProcessed[i-1] {
			dedupDestinationSlice = append(dedupDestinationSlice, destinationsToBeProcessed[i])
		}
	}
	return dedupDestinationSlice
}

func processDestinationsForSourceIdentity(ctx context.Context, remoteRegistry *RemoteRegistry, eventType admiral.EventType, hasNonMeshDestination bool, sourceClusters *common.Map, destinations []string, sourceIdentity string, modifySE ModifySEFunc) error {
	var message string
	var processingErrors error
	counter := 1
	totalDestinations := len(destinations)

	for _, destinationIdentity := range destinations {
		if strings.Contains(strings.ToLower(destinationIdentity), strings.ToLower(common.ServicesGatewayIdentity)) &&
			!hasNonMeshDestination {
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, sourceIdentity, "",
				fmt.Sprintf("All destinations are MESH enabled. Skipping processing: %v", destinationIdentity))
			continue
		}

		// In case of self on-boarding skip the update for the destination as it is the same as the source
		if strings.EqualFold(sourceIdentity, destinationIdentity) {
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, sourceIdentity, "",
				fmt.Sprintf("Destination identity is same as source identity. Skipping processing: %v.", destinationIdentity))
			continue
		}

		destinationClusters := remoteRegistry.AdmiralCache.IdentityClusterCache.Get(destinationIdentity)
		log.Infof(LogFormat, string(eventType), common.DependencyResourceType, sourceIdentity, "", fmt.Sprintf("processing destination %d/%d destinationIdentity=%s", counter, totalDestinations, destinationIdentity))
		clusters := remoteRegistry.AdmiralCache.IdentityClusterCache.Get(destinationIdentity)
		if destinationClusters == nil || destinationClusters.Len() == 0 {
			listOfSourceClusters := strings.Join(sourceClusters.GetValues(), ",")
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, sourceIdentity, listOfSourceClusters,
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
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, sourceIdentity, "",
				fmt.Sprintf("no cluster found for destinationIdentity: %s. Skipping calling modifySE", destinationIdentity))
			continue
		}

		for _, destinationClusterID := range clusters.GetValues() {
			message = fmt.Sprintf("processing cluster=%s for destinationIdentity=%s", destinationClusterID, destinationIdentity)
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, sourceIdentity, "", message)
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
						log.Infof(LogFormat, string(eventType), common.DependencyResourceType, sourceIdentity, "", message)
						_, err := modifySE(ctx, eventType, env, destinationIdentity, remoteRegistry)
						if err != nil {
							message = fmt.Sprintf("error occurred in modifySE func for env=%s destinationIdentity=%s", env, destinationIdentity)
							log.Errorf(LogErrFormat, string(eventType), common.DependencyResourceType, sourceIdentity, "", err.Error()+". "+message)
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
						log.Infof(LogFormat, string(eventType), common.DependencyResourceType, sourceIdentity, "", message)
						_, err := modifySE(ctx, eventType, env, destinationIdentity, remoteRegistry)
						if err != nil {
							message = fmt.Sprintf("error occurred in modifySE func for env=%s destinationIdentity=%s", env, destinationIdentity)
							log.Errorf(LogErrFormat, string(eventType), common.DependencyResourceType, sourceIdentity, "", err.Error()+". "+message)
							processingErrors = common.AppendError(processingErrors, err)
						}
					}
					continue
				}
			}
			log.Infof(LogFormat, string(eventType), common.DependencyResourceType, sourceIdentity, "", fmt.Sprintf("done processing destinationIdentity=%s", destinationIdentity))
			log.Warnf(LogFormat, string(eventType), common.DependencyResourceType, sourceIdentity, "",
				fmt.Sprintf("neither deployment or rollout controller initialized in cluster %s and destination identity %s", destinationClusterID, destinationIdentity))
			counter++
		}
	}
	return processingErrors
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

func isServiceControllerInitialized(rc *RemoteController) error {
	if rc == nil {
		return fmt.Errorf("remote controller not initialized")
	}
	if rc.ServiceController == nil {
		return fmt.Errorf("service controller not initialized")
	}
	if rc.ServiceController.Cache == nil {
		return fmt.Errorf("service controller cache not initialized")
	}
	return nil
}
