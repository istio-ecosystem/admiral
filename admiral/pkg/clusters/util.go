package clusters

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	networking "istio.io/api/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
)

type WorkloadEntrySorted []*networking.WorkloadEntry

func GetMeshPortsForDeployments(clusterName string, destService *k8sV1.Service,
	destDeployment *k8sAppsV1.Deployment) map[string]uint32 {

	if destService == nil || destDeployment == nil {
		logrus.Warnf("Deployment or Service is nil cluster=%s", clusterName)
		return nil
	}

	var meshPorts string
	if destDeployment.Spec.Template.Annotations == nil {
		meshPorts = ""
	} else {
		meshPorts = destDeployment.Spec.Template.Annotations[common.SidecarEnabledPorts]
	}
	ports := getMeshPortsHelper(meshPorts, destService, clusterName)
	return ports
}

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
	ports := getMeshPortsHelper(meshPorts, destService, clusterName)
	return ports
}

// Get the service selector to add as workload selector for envoyFilter
func GetServiceSelector(clusterName string, destService *k8sV1.Service) *common.Map {
	var selectors = destService.Spec.Selector
	if len(selectors) == 0 {
		logrus.Infof(LogFormat, "GetServiceLabels", "no selectors present", destService.Name, clusterName, selectors)
		return nil
	}
	var tempMap = common.NewMap()
	for key, value := range selectors {
		tempMap.Put(key, value)
	}
	logrus.Infof(LogFormat, "GetServiceLabels", "selectors present", destService.Name, clusterName, selectors)
	return tempMap
}

func getMeshPortsHelper(meshPorts string, destService *k8sV1.Service, clusterName string) map[string]uint32 {
	var ports = make(map[string]uint32)

	if destService == nil {
		return ports
	}
	if len(meshPorts) == 0 {
		logrus.Infof(LogFormatAdv, "GetMeshPorts", "service", destService.Name, destService.Namespace,
			clusterName, "No mesh ports present, defaulting to first port")
		if destService.Spec.Ports != nil && len(destService.Spec.Ports) > 0 {
			var protocol = util.GetPortProtocol(destService.Spec.Ports[0].Name)
			ports[protocol] = uint32(destService.Spec.Ports[0].Port)
		}
		return ports
	}

	meshPortsSplit := strings.Split(meshPorts, ",")

	if len(meshPortsSplit) > 1 {
		logrus.Warnf(LogErrFormat, "Get", "MeshPorts", "", clusterName,
			"Multiple inbound mesh ports detected, admiral generates service entry with first matched port and protocol")
	}

	//fetch the first valid port if there is more than one mesh port
	var meshPortMap = make(map[uint32]uint32)
	for _, meshPort := range meshPortsSplit {
		port, err := strconv.ParseUint(meshPort, 10, 32)
		if err == nil {
			meshPortMap[uint32(port)] = uint32(port)
			break
		}
	}
	for _, servicePort := range destService.Spec.Ports {
		//handling relevant protocols from here:
		// https://istio.io/latest/docs/ops/configuration/traffic-management/protocol-selection/#manual-protocol-selection
		//use target port if present to match the annotated mesh port
		targetPort := uint32(servicePort.Port)
		if servicePort.TargetPort.StrVal != "" {
			port, err := strconv.Atoi(servicePort.TargetPort.StrVal)
			if err != nil {
				logrus.Warnf(LogErrFormat, "GetMeshPorts", "Failed to parse TargetPort", destService.Name, clusterName, err)
			}
			if port > 0 {
				targetPort = uint32(port)
			}

		}
		if servicePort.TargetPort.IntVal != 0 {
			targetPort = uint32(servicePort.TargetPort.IntVal)
		}
		if _, ok := meshPortMap[targetPort]; ok {
			var protocol = util.GetPortProtocol(servicePort.Name)
			logrus.Infof(LogFormatAdv, "MeshPort", servicePort.Port, destService.Name, destService.Namespace,
				clusterName, "Protocol: "+protocol)
			ports[protocol] = uint32(servicePort.Port)
			break
		}
	}
	return ports
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
func GenerateServiceEntryForCanary(ctxLogger *logrus.Entry, ctx context.Context, event admiral.EventType, rc *RemoteController, admiralCache *AdmiralCache,
	meshPorts map[string]uint32, destRollout *argo.Rollout, serviceEntries map[string]*networking.ServiceEntry, workloadIdentityKey string, san []string) error {

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
			err := generateSECanary(ctxLogger, ctx, event, rc, admiralCache, meshPorts, serviceEntries, san, canaryGlobalFqdn)
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
		logrus.Infof("no selector for rollout=%s in namespace=%s and cluster=%s", rollout.Name, rollout.Namespace, rc.ClusterID)
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
func generateSECanary(ctxLogger *logrus.Entry, ctx context.Context, event admiral.EventType, rc *RemoteController, admiralCache *AdmiralCache, meshPorts map[string]uint32, serviceEntries map[string]*networking.ServiceEntry, san []string, fqdn string) error {

	address, err := getUniqueAddress(ctxLogger, ctx, admiralCache, fqdn)
	if err != nil {
		logrus.Errorf("failed to generate unique address for canary fqdn - %v error - %v", fqdn, err)
		return err
	}
	// This check preserves original behavior of checking for non-empty fqdn and address before
	// generating SE when disable_ip_generation=false. When disable_ip_generation=true, it still
	// checks for non-empty fqdn but allows for empty address.
	if len(fqdn) != 0 && (common.DisableIPGeneration() || len(address) != 0) {
		logrus.Infof("se generated for canary fqdn - %v", fqdn)
		generateServiceEntry(ctxLogger, event, admiralCache, meshPorts, fqdn, rc, serviceEntries, address, san, common.Rollout)
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
		} else {
			logrus.Infof("Filtering out %v from sourceClusters list as it is present in destinationClusters", k)
		}
	})
	return filteredSourceClusters
}

// getSortedDependentNamespaces takes a cname and reduces it to its base form (without canary/bluegreen prefix) and fetches the partitionedIdentity based on that
// Then, it checks if the clusterId matches any of the source clusters, and if so, adds istio-system to the list of dependent namespaces
// Then, it fetches the dependent namespaces based on the cname or cnameWithoutPrefix and adds them to the list of dependent namespaces
// If the list is above the maximum number of allowed exportTo values, it replaces the entries with "*"
// Otherwise, it sorts and dedups the list of dependent namespaces and returns them.
func getSortedDependentNamespaces(admiralCache *AdmiralCache, cname string, clusterId string, ctxLogger *logrus.Entry) []string {
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
						namespaceSlice = append(namespaceSlice, sourceNamespacesInCluster.GetKeys()...)
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
			namespaceSlice = append(namespaceSlice, namespaces.GetKeys()...)
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
	ctxLogger.Infof("getSortedDependentNamespaces for cname %v and cluster %v got namespaces: %v", cname, clusterId, dedupNamespaceSlice)
	return dedupNamespaceSlice
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
