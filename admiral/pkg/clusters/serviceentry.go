package clusters

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"gopkg.in/yaml.v2"
	k8errors "k8s.io/apimachinery/pkg/api/errors"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	log "github.com/sirupsen/logrus"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
)

type SeDrTuple struct {
	SeName                      string
	DrName                      string
	ServiceEntry                *networking.ServiceEntry
	DestinationRule             *networking.DestinationRule
	SeDnsPrefix                 string
	SeDrGlobalTrafficPolicyName string
}

const (
	resourceCreatedByAnnotationLabel = "app.kubernetes.io/created-by"
	resourceCreatedByAnnotationValue = "admiral"
)

func createServiceEntryForDeployment(ctx context.Context, event admiral.EventType, rc *RemoteController, admiralCache *AdmiralCache,
	meshPorts map[string]uint32, destDeployment *k8sAppsV1.Deployment, serviceEntries map[string]*networking.ServiceEntry) *networking.ServiceEntry {

	workloadIdentityKey := common.GetWorkloadIdentifier()
	globalFqdn := common.GetCname(destDeployment, workloadIdentityKey, common.GetHostnameSuffix())

	//Handling retries for getting/putting service entries from/in cache

	address := getUniqueAddress(ctx, admiralCache, globalFqdn)

	if len(globalFqdn) == 0 || len(address) == 0 {
		return nil
	}

	san := getSanForDeployment(destDeployment, workloadIdentityKey)
	return generateServiceEntry(event, admiralCache, meshPorts, globalFqdn, rc, serviceEntries, address, san)
}

func modifyServiceEntryForNewServiceOrPod(
	ctx context.Context, event admiral.EventType, env string,
	sourceIdentity string, remoteRegistry *RemoteRegistry) map[string]*networking.ServiceEntry {
	defer util.LogElapsedTime("modifyServiceEntryForNewServiceOrPod", sourceIdentity, env, "")()

	if remoteRegistry.ServiceEntryUpdateSuspender.SuspendUpdate(sourceIdentity, env) {
		log.Infof(LogFormat, event, env, sourceIdentity, "",
			"skipping update because endpoint generation is suspended for identity '"+sourceIdentity+"' in environment '"+env+"'")
		return nil
	}

	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, event, env, sourceIdentity, "", "Processing skipped as Admiral is in Read-only mode")
		return nil
	}

	if IsCacheWarmupTime(remoteRegistry) {
		log.Infof(LogFormat, event, env, sourceIdentity, "", "Processing skipped during cache warm up state")
		return nil
	}

	var (
		cname                  string
		namespace              string
		serviceInstance        *k8sV1.Service
		rollout                *argo.Rollout
		deployment             *k8sAppsV1.Deployment
		start                  = time.Now()
		gtpKey                 = common.ConstructGtpKey(env, sourceIdentity)
		clusters               = remoteRegistry.GetClusterIds()
		gtps                   = make(map[string][]*v1.GlobalTrafficPolicy)
		weightedServices       = make(map[string]*WeightedService)
		cnames                 = make(map[string]string)
		sourceServices         = make(map[string]*k8sV1.Service)
		sourceWeightedServices = make(map[string]map[string]*WeightedService)
		sourceDeployments      = make(map[string]*k8sAppsV1.Deployment)
		sourceRollouts         = make(map[string]*argo.Rollout)
		serviceEntries         = make(map[string]*networking.ServiceEntry)
	)

	for _, clusterId := range clusters {
		rc := remoteRegistry.GetRemoteController(clusterId)
		if rc == nil {
			log.Warnf(LogFormat, "Find", "remote-controller", clusterId, clusterId, "remote controller not available/initialized for the cluster")
			continue
		}
		if rc.DeploymentController != nil {
			deployment = rc.DeploymentController.Cache.Get(sourceIdentity, env)
		}
		if rc.RolloutController != nil {
			rollout = rc.RolloutController.Cache.Get(sourceIdentity, env)
		}
		if deployment == nil && rollout == nil {
			log.Infof("Neither deployment nor rollouts found for identity=%s in env=%s namespace=%s", sourceIdentity, env, namespace)
			continue
		}
		if deployment != nil {
			remoteRegistry.AdmiralCache.IdentityClusterCache.Put(sourceIdentity, rc.ClusterID, rc.ClusterID)
			serviceInstance = getServiceForDeployment(rc, deployment)
			if serviceInstance == nil {
				continue
			}
			namespace = deployment.Namespace
			localMeshPorts := GetMeshPorts(rc.ClusterID, serviceInstance, deployment)

			cname = common.GetCname(deployment, common.GetWorkloadIdentifier(), common.GetHostnameSuffix())
			sourceDeployments[rc.ClusterID] = deployment
			createServiceEntryForDeployment(ctx, event, rc, remoteRegistry.AdmiralCache, localMeshPorts, deployment, serviceEntries)
		} else if rollout != nil {
			remoteRegistry.AdmiralCache.IdentityClusterCache.Put(sourceIdentity, rc.ClusterID, rc.ClusterID)
			weightedServices = getServiceForRollout(ctx, rc, rollout)
			if len(weightedServices) == 0 {
				continue
			}

			//use any service within the weightedServices for determining ports etc.
			for _, sInstance := range weightedServices {
				serviceInstance = sInstance.Service
				break
			}
			namespace = rollout.Namespace
			localMeshPorts := GetMeshPortsForRollout(rc.ClusterID, serviceInstance, rollout)

			cname = common.GetCnameForRollout(rollout, common.GetWorkloadIdentifier(), common.GetHostnameSuffix())
			cnames[cname] = "1"
			sourceRollouts[rc.ClusterID] = rollout
			createServiceEntryForRollout(ctx, event, rc, remoteRegistry.AdmiralCache, localMeshPorts, rollout, serviceEntries)
		} else {
			continue
		}

		gtpsInNamespace := rc.GlobalTraffic.Cache.Get(gtpKey, namespace)
		if len(gtpsInNamespace) > 0 {
			if log.IsLevelEnabled(log.DebugLevel) {
				log.Debugf("GTPs found for identity=%s in env=%s namespace=%s gtp=%v", sourceIdentity, env, namespace, gtpsInNamespace)
			}
			gtps[rc.ClusterID] = gtpsInNamespace
		} else {
			log.Debugf("No GTPs found for identity=%s in env=%s namespace=%s with key=%s", sourceIdentity, env, namespace, gtpKey)
		}

		remoteRegistry.AdmiralCache.IdentityClusterCache.Put(sourceIdentity, rc.ClusterID, rc.ClusterID)
		// workload selector cache is needed for routingPolicy's envoyFilter to match the dependency and apply to the right POD
		// using service labels
		workloadSelectors := GetServiceSelector(rc.ClusterID, serviceInstance)
		if workloadSelectors != nil {
			remoteRegistry.AdmiralCache.WorkloadSelectorCache.PutMap(sourceIdentity+rc.ClusterID, workloadSelectors)
		}
		remoteRegistry.AdmiralCache.CnameClusterCache.Put(cname, rc.ClusterID, rc.ClusterID)
		remoteRegistry.AdmiralCache.CnameIdentityCache.Store(cname, sourceIdentity)
		sourceServices[rc.ClusterID] = serviceInstance
		sourceWeightedServices[rc.ClusterID] = weightedServices
	}

	util.LogElapsedTimeSince("BuildServiceEntry", sourceIdentity, env, "", start)

	//cache the latest GTP in global cache to be reused during DR creation
	updateGlobalGtpCache(remoteRegistry.AdmiralCache, sourceIdentity, env, gtps)

	dependents := remoteRegistry.AdmiralCache.IdentityDependencyCache.Get(sourceIdentity).Copy()

	//handle local updates (source clusters first)
	//update the address to local fqdn for service entry in a cluster local to the service instance

	start = time.Now()

	for sourceCluster, serviceInstance := range sourceServices {
		localFqdn := serviceInstance.Name + common.Sep + serviceInstance.Namespace + common.DotLocalDomainSuffix
		rc := remoteRegistry.GetRemoteController(sourceCluster)
		var meshPorts map[string]uint32
		blueGreenStrategy := isBlueGreenStrategy(sourceRollouts[sourceCluster])

		if len(sourceDeployments) > 0 {
			meshPorts = GetMeshPorts(sourceCluster, serviceInstance, sourceDeployments[sourceCluster])
		} else {
			meshPorts = GetMeshPortsForRollout(sourceCluster, serviceInstance, sourceRollouts[sourceCluster])
		}

		for key, serviceEntry := range serviceEntries {
			if len(serviceEntry.Endpoints) == 0 {
				AddServiceEntriesWithDr(
					ctx, remoteRegistry, map[string]string{sourceCluster: sourceCluster},
					map[string]*networking.ServiceEntry{key: serviceEntry})
			}
			clusterIngress, _ := rc.ServiceController.Cache.GetLoadBalancer(common.GetAdmiralParams().LabelSet.GatewayApp, common.NamespaceIstioSystem)
			for _, ep := range serviceEntry.Endpoints {
				//replace istio ingress-gateway address with local fqdn, note that ingress-gateway can be empty (not provisoned, or is not up)
				if ep.Address == clusterIngress || ep.Address == "" {
					// Update endpoints with locafqdn for active and preview se of bluegreen rollout
					if blueGreenStrategy {
						oldPorts := ep.Ports
						updateEndpointsForBlueGreen(sourceRollouts[sourceCluster], sourceWeightedServices[sourceCluster], cnames, ep, sourceCluster, key)
						AddServiceEntriesWithDr(
							ctx, remoteRegistry, map[string]string{sourceCluster: sourceCluster},
							map[string]*networking.ServiceEntry{key: serviceEntry})
						//swap it back to use for next iteration
						ep.Address = clusterIngress
						ep.Ports = oldPorts
						// see if we have weighted services (rollouts with canary strategy)
					} else if len(sourceWeightedServices[sourceCluster]) > 1 {
						//add one endpoint per each service, may be modify
						var se = copyServiceEntry(serviceEntry)
						updateEndpointsForWeightedServices(se, sourceWeightedServices[sourceCluster], clusterIngress, meshPorts)
						AddServiceEntriesWithDr(
							ctx, remoteRegistry, map[string]string{sourceCluster: sourceCluster},
							map[string]*networking.ServiceEntry{key: se})
					} else {
						ep.Address = localFqdn
						oldPorts := ep.Ports
						ep.Ports = meshPorts
						AddServiceEntriesWithDr(
							ctx, remoteRegistry, map[string]string{sourceCluster: sourceCluster},
							map[string]*networking.ServiceEntry{key: serviceEntry})
						// swap it back to use for next iteration
						ep.Address = clusterIngress
						ep.Ports = oldPorts
					}
				}
			}

		}

		err := generateProxyVirtualServiceForDependencies(ctx, remoteRegistry, sourceIdentity, rc)
		if err != nil {
			log.Error(err)
		}

		if common.GetWorkloadSidecarUpdate() == "enabled" {
			modifySidecarForLocalClusterCommunication(
				ctx, serviceInstance.Namespace, sourceIdentity,
				remoteRegistry.AdmiralCache.DependencyNamespaceCache, rc)
		}

		for _, val := range dependents {
			remoteRegistry.AdmiralCache.DependencyNamespaceCache.Put(val, serviceInstance.Namespace, localFqdn, cnames)
		}
	}

	util.LogElapsedTimeSince("WriteServiceEntryToSourceClusters", sourceIdentity, env, "", start)

	//Write to dependent clusters
	start = time.Now()
	dependentClusters := getDependentClusters(dependents, remoteRegistry.AdmiralCache.IdentityClusterCache, sourceServices)

	//update cname dependent cluster cache
	for clusterId := range dependentClusters {
		remoteRegistry.AdmiralCache.CnameDependentClusterCache.Put(cname, clusterId, clusterId)
	}

	AddServiceEntriesWithDr(ctx, remoteRegistry, dependentClusters, serviceEntries)

	util.LogElapsedTimeSince("WriteServiceEntryToDependentClusters", sourceIdentity, env, "", start)

	return serviceEntries
}

func generateProxyVirtualServiceForDependencies(ctx context.Context, remoteRegistry *RemoteRegistry, sourceIdentity string, rc *RemoteController) error {
	if remoteRegistry.AdmiralCache.SourceToDestinations == nil {
		return fmt.Errorf("failed to generate proxy virtual service for sourceIdentity %s as remoteRegistry.AdmiralCache.DependencyLookupCache is nil", sourceIdentity)
	}
	if remoteRegistry.AdmiralCache.DependencyProxyVirtualServiceCache == nil {
		return fmt.Errorf("failed to generate proxy virtual service for sourceIdentity %s as remoteRegistry.AdmiralCache.DependencyProxyVirtualServiceCache is nil", sourceIdentity)
	}
	dependencies := remoteRegistry.AdmiralCache.SourceToDestinations.Get(sourceIdentity)
	if dependencies == nil {
		log.Infof("skipped generating proxy virtual service as there are no dependencies found for sourceIdentity %s", sourceIdentity)
		return nil
	}
	for _, dependency := range dependencies {
		vs := remoteRegistry.AdmiralCache.DependencyProxyVirtualServiceCache.get(dependency)
		if vs == nil || len(vs) == 0 {
			continue
		}
		log.Infof("found dependency proxy virtual service for destination: %s, source: %s", dependency, sourceIdentity)
		for _, v := range vs {
			existingVS, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(common.GetSyncNamespace()).Get(ctx, v.Name, v12.GetOptions{})
			if err != nil && k8errors.IsNotFound(err) {
				log.Infof("proxy VirtualService %s not found", v.Name)
			}
			err = addUpdateVirtualService(ctx, v, existingVS, common.GetSyncNamespace(), rc)
			if err != nil {
				return fmt.Errorf("failed generating proxy VirtualService %s due to error: %w", v.Name, err)
			}
			log.Infof("successfully generated proxy VirtualService %s", v.Name)
		}
	}
	return nil
}

//Does two things;
//i)  Picks the GTP that was created most recently from the passed in GTP list based on GTP priority label (GTPs from all clusters)
//ii) Updates the global GTP cache with the selected GTP in i)
func updateGlobalGtpCache(cache *AdmiralCache, identity, env string, gtps map[string][]*v1.GlobalTrafficPolicy) {
	defer util.LogElapsedTime("updateGlobalGtpCache", identity, env, "")()
	gtpsOrdered := make([]*v1.GlobalTrafficPolicy, 0)
	for _, gtpsInCluster := range gtps {
		gtpsOrdered = append(gtpsOrdered, gtpsInCluster...)
	}
	if len(gtpsOrdered) == 0 {
		log.Debugf("No GTPs found for identity=%s in env=%s. Deleting global cache entries if any", identity, env)
		cache.GlobalTrafficCache.Delete(identity, env)
		return
	} else if len(gtpsOrdered) > 1 {
		log.Debugf("More than one GTP found for identity=%s in env=%s.", identity, env)
		//sort by creation time and priority, gtp with highest priority and most recent at the beginning
		sortGtpsByPriorityAndCreationTime(gtpsOrdered, identity, env)
	}

	mostRecentGtp := gtpsOrdered[0]

	err := cache.GlobalTrafficCache.Put(mostRecentGtp)

	if err != nil {
		log.Errorf("Error in updating GTP with name=%s in namespace=%s as actively used for identity=%s with err=%v", mostRecentGtp.Name, mostRecentGtp.Namespace, common.GetGtpKey(mostRecentGtp), err)
	} else {
		log.Infof("GTP with name=%s in namespace=%s is actively used for identity=%s", mostRecentGtp.Name, mostRecentGtp.Namespace, common.GetGtpKey(mostRecentGtp))
	}
}

func sortGtpsByPriorityAndCreationTime(gtpsToOrder []*v1.GlobalTrafficPolicy, identity string, env string) {
	sort.Slice(gtpsToOrder, func(i, j int) bool {
		iPriority := getGtpPriority(gtpsToOrder[i])
		jPriority := getGtpPriority(gtpsToOrder[j])

		iTime := gtpsToOrder[i].CreationTimestamp
		jTime := gtpsToOrder[j].CreationTimestamp

		if iPriority != jPriority {
			log.Debugf("GTP sorting identity=%s env=%s name1=%s creationTime1=%v priority1=%d name2=%s creationTime2=%v priority2=%d", identity, env, gtpsToOrder[i].Name, iTime, iPriority, gtpsToOrder[j].Name, jTime, jPriority)
			return iPriority > jPriority
		}
		log.Debugf("GTP sorting identity=%s env=%s name1=%s creationTime1=%v priority1=%d name2=%s creationTime2=%v priority2=%d", identity, env, gtpsToOrder[i].Name, iTime, iPriority, gtpsToOrder[j].Name, jTime, jPriority)
		return iTime.After(jTime.Time)
	})
}
func getGtpPriority(gtp *v1.GlobalTrafficPolicy) int {
	if val, ok := gtp.ObjectMeta.Labels[common.GetAdmiralParams().LabelSet.PriorityKey]; ok {
		if convertedValue, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return convertedValue
		}
	}
	return 0
}
func updateEndpointsForBlueGreen(rollout *argo.Rollout, weightedServices map[string]*WeightedService, cnames map[string]string,
	ep *networking.WorkloadEntry, sourceCluster string, meshHost string) {
	activeServiceName := rollout.Spec.Strategy.BlueGreen.ActiveService
	previewServiceName := rollout.Spec.Strategy.BlueGreen.PreviewService

	if previewService, ok := weightedServices[previewServiceName]; strings.HasPrefix(meshHost, common.BlueGreenRolloutPreviewPrefix+common.Sep) && ok {
		previewServiceInstance := previewService.Service
		localFqdn := previewServiceInstance.Name + common.Sep + previewServiceInstance.Namespace + common.DotLocalDomainSuffix
		cnames[localFqdn] = "1"
		ep.Address = localFqdn
		ep.Ports = GetMeshPortsForRollout(sourceCluster, previewServiceInstance, rollout)
	} else if activeService, ok := weightedServices[activeServiceName]; ok {
		activeServiceInstance := activeService.Service
		localFqdn := activeServiceInstance.Name + common.Sep + activeServiceInstance.Namespace + common.DotLocalDomainSuffix
		cnames[localFqdn] = "1"
		ep.Address = localFqdn
		ep.Ports = GetMeshPortsForRollout(sourceCluster, activeServiceInstance, rollout)
	}
}

//update endpoints for Argo rollouts specific Service Entries to account for traffic splitting (Canary strategy)
func updateEndpointsForWeightedServices(serviceEntry *networking.ServiceEntry, weightedServices map[string]*WeightedService, clusterIngress string, meshPorts map[string]uint32) {
	var endpoints = make([]*networking.WorkloadEntry, 0)
	var endpointToReplace *networking.WorkloadEntry

	//collect all endpoints except the one to replace
	for _, ep := range serviceEntry.Endpoints {
		if ep.Address == clusterIngress || ep.Address == "" {
			endpointToReplace = ep
		} else {
			endpoints = append(endpoints, ep)
		}
	}

	if endpointToReplace == nil {
		return
	}

	//create endpoints based on weightedServices
	for _, serviceInstance := range weightedServices {
		//skip service instances with 0 weight
		if serviceInstance.Weight <= 0 {
			continue
		}
		var ep = copyEndpoint(endpointToReplace)
		ep.Ports = meshPorts
		ep.Address = serviceInstance.Service.Name + common.Sep + serviceInstance.Service.Namespace + common.DotLocalDomainSuffix
		ep.Weight = uint32(serviceInstance.Weight)
		endpoints = append(endpoints, ep)
	}
	serviceEntry.Endpoints = endpoints
}

func modifySidecarForLocalClusterCommunication(
	ctx context.Context, sidecarNamespace, sourceIdentity string,
	sidecarEgressMap *common.SidecarEgressMap, rc *RemoteController) {

	//get existing sidecar from the cluster
	sidecarConfig := rc.SidecarController

	sidecarEgressMap.Range(func(k string, v map[string]common.SidecarEgress) {
		if k == sourceIdentity {
			sidecarEgress := v
			if sidecarConfig == nil || sidecarEgress == nil {
				return
			}

			sidecar, err := sidecarConfig.IstioClient.NetworkingV1alpha3().Sidecars(sidecarNamespace).Get(ctx, common.GetWorkloadSidecarName(), v12.GetOptions{})
			if err != nil {
				return
			}
			if sidecar == nil || (sidecar.Spec.Egress == nil) {
				return
			}

			//copy and add our new local FQDN
			newSidecar := copySidecar(sidecar)

			egressHosts := make(map[string]string)

			for _, sidecarEgress := range sidecarEgress {
				egressHost := sidecarEgress.Namespace + "/" + sidecarEgress.FQDN
				egressHosts[egressHost] = egressHost
				for cname := range sidecarEgress.CNAMEs {
					scopedCname := sidecarEgress.Namespace + "/" + cname
					egressHosts[scopedCname] = scopedCname
				}
			}

			for egressHost := range egressHosts {
				if !util.Contains(newSidecar.Spec.Egress[0].Hosts, egressHost) {
					newSidecar.Spec.Egress[0].Hosts = append(newSidecar.Spec.Egress[0].Hosts, egressHost)
				}
			}

			//nolint
			newSidecarConfig := createSidecarSkeleton(newSidecar.Spec, common.GetWorkloadSidecarName(), sidecarNamespace)

			//insert into cluster
			if newSidecarConfig != nil {
				addUpdateSidecar(ctx, newSidecarConfig, sidecar, sidecarNamespace, rc)
			}
		}
	})
}

func addUpdateSidecar(ctx context.Context, obj *v1alpha3.Sidecar, exist *v1alpha3.Sidecar, namespace string, rc *RemoteController) {
	var err error
	_, err = rc.SidecarController.IstioClient.NetworkingV1alpha3().Sidecars(namespace).Update(ctx, obj, v12.UpdateOptions{})
	if err != nil {
		log.Infof(LogErrFormat, "Update", "Sidecar", obj.Name, rc.ClusterID, err)
	} else {
		log.Infof(LogErrFormat, "Update", "Sidecar", obj.Name, rc.ClusterID, "Success")
	}
}

func copySidecar(sidecar *v1alpha3.Sidecar) *v1alpha3.Sidecar {
	newSidecarObj := &v1alpha3.Sidecar{}
	newSidecarObj.Spec.WorkloadSelector = sidecar.Spec.WorkloadSelector
	newSidecarObj.Spec.Ingress = sidecar.Spec.Ingress
	newSidecarObj.Spec.Egress = sidecar.Spec.Egress
	return newSidecarObj
}

//AddServiceEntriesWithDr will create the default service entries and also additional ones specified in GTP
func AddServiceEntriesWithDr(ctx context.Context, rr *RemoteRegistry, sourceClusters map[string]string, serviceEntries map[string]*networking.ServiceEntry) {
	cache := rr.AdmiralCache
	syncNamespace := common.GetSyncNamespace()
	for _, se := range serviceEntries {

		var identityId string
		if identityValue, ok := cache.CnameIdentityCache.Load(se.Hosts[0]); ok {
			identityId = fmt.Sprint(identityValue)
		}

		splitByEnv := strings.Split(se.Hosts[0], common.Sep)
		var env = splitByEnv[0]

		globalTrafficPolicy := cache.GlobalTrafficCache.GetFromIdentity(identityId, env)

		for _, sourceCluster := range sourceClusters {

			rc := rr.GetRemoteController(sourceCluster)

			if rc == nil || rc.NodeController == nil || rc.NodeController.Locality == nil {
				log.Warnf(LogFormat, "Find", "remote-controller", sourceCluster, sourceCluster, "locality not available for the cluster")
				continue
			}

			//check if there is a gtp and add additional hosts/destination rules
			var seDrSet = createSeAndDrSetFromGtp(ctx, env, rc.NodeController.Locality.Region, se, globalTrafficPolicy, cache)

			for _, seDr := range seDrSet {
				oldServiceEntry, err := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(syncNamespace).Get(ctx, seDr.SeName, v12.GetOptions{})
				// if old service entry not find, just create a new service entry instead
				if err != nil {
					log.Infof(LogFormat, "Get (error)", "old ServiceEntry", seDr.SeName, sourceCluster, err)
					oldServiceEntry = nil
				}

				// check if the existing service entry was created outside of admiral
				// if it was, then admiral will not take any action on this SE
				skipSEUpdate := false
				if oldServiceEntry != nil && !isGeneratedByAdmiral(oldServiceEntry.Annotations) {
					log.Infof(LogFormat, "update", "ServiceEntry", oldServiceEntry.Name, sourceCluster, "skipped updating the SE as there exists a custom SE with the same name in "+syncNamespace+" namespace")
					skipSEUpdate = true
				}

				oldDestinationRule, err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(ctx, seDr.DrName, v12.GetOptions{})

				if err != nil {
					log.Infof(LogFormat, "Get (error)", "old DestinationRule", seDr.DrName, sourceCluster, err)
					oldDestinationRule = nil
				}

				// check if the existing destination rule was created outside of admiral
				// if it was, then admiral will not take any action on this DR
				skipDRUpdate := false
				if oldDestinationRule != nil && !isGeneratedByAdmiral(oldDestinationRule.Annotations) {
					log.Infof(LogFormat, "update", "DestinationRule", oldDestinationRule.Name, sourceCluster, "skipped updating the DR as there exists a custom DR with the same name in "+syncNamespace+" namespace")
					skipDRUpdate = true
				}

				if skipSEUpdate && skipDRUpdate {
					return
				}

				var deleteOldServiceEntry = false
				if oldServiceEntry != nil && !skipSEUpdate {
					areEndpointsValid := validateAndProcessServiceEntryEndpoints(oldServiceEntry)
					if !areEndpointsValid && len(oldServiceEntry.Spec.Endpoints) == 0 {
						deleteOldServiceEntry = true
					}
				}

				//clean service entry in case no endpoints are configured or if all the endpoints are invalid
				if (len(seDr.ServiceEntry.Endpoints) == 0) || deleteOldServiceEntry {
					if !skipSEUpdate {
						deleteServiceEntry(ctx, oldServiceEntry, syncNamespace, rc)
						cache.SeClusterCache.Delete(seDr.ServiceEntry.Hosts[0])
					}
					if !skipDRUpdate {
						// after deleting the service entry, destination rule also need to be deleted if the service entry host no longer exists
						deleteDestinationRule(ctx, oldDestinationRule, syncNamespace, rc)
					}
				} else {
					if !skipSEUpdate {
						//nolint
						newServiceEntry := createServiceEntrySkeletion(*seDr.ServiceEntry, seDr.SeName, syncNamespace)
						if newServiceEntry != nil {
							newServiceEntry.Labels = map[string]string{
								common.GetWorkloadIdentifier(): fmt.Sprintf("%v", identityId),
								common.GetEnvKey():             fmt.Sprintf("%v", env),
							}
							if newServiceEntry.Annotations == nil {
								newServiceEntry.Annotations = map[string]string{}
							}
							if seDr.SeDnsPrefix != "" && seDr.SeDnsPrefix != common.Default {
								newServiceEntry.Annotations["dns-prefix"] = seDr.SeDnsPrefix
							}
							if seDr.SeDrGlobalTrafficPolicyName != "" {
								newServiceEntry.Annotations["associated-gtp"] = seDr.SeDrGlobalTrafficPolicyName
							}
							addUpdateServiceEntry(ctx, newServiceEntry, oldServiceEntry, syncNamespace, rc)
							cache.SeClusterCache.Put(newServiceEntry.Spec.Hosts[0], rc.ClusterID, rc.ClusterID)
						}
					}

					if !skipDRUpdate {
						//nolint
						newDestinationRule := createDestinationRuleSkeletion(*seDr.DestinationRule, seDr.DrName, syncNamespace)
						// if event was deletion when this function was called, then GlobalTrafficCache should already deleted the cache globalTrafficPolicy is an empty shell object
						addUpdateDestinationRule(ctx, newDestinationRule, oldDestinationRule, syncNamespace, rc)
					}
				}
			}
		}
	}
}

func isGeneratedByAdmiral(annotations map[string]string) bool {
	seAnnotationVal, ok := annotations[resourceCreatedByAnnotationLabel]
	if !ok || seAnnotationVal != resourceCreatedByAnnotationValue {
		return false
	}
	return true
}

func createSeAndDrSetFromGtp(ctx context.Context, env, region string, se *networking.ServiceEntry, globalTrafficPolicy *v1.GlobalTrafficPolicy,
	cache *AdmiralCache) map[string]*SeDrTuple {
	var defaultDrName = getIstioResourceName(se.Hosts[0], "-default-dr")
	var defaultSeName = getIstioResourceName(se.Hosts[0], "-se")
	var seDrSet = make(map[string]*SeDrTuple)
	if globalTrafficPolicy != nil {
		gtp := globalTrafficPolicy.Spec
		for _, gtpTrafficPolicy := range gtp.Policy {
			var modifiedSe = se
			var host = se.Hosts[0]
			var drName, seName = defaultDrName, defaultSeName
			if gtpTrafficPolicy.Dns != "" {
				log.Warnf("Using the deprecated field `dns` in gtp: %v in namespace: %v", globalTrafficPolicy.Name, globalTrafficPolicy.Namespace)
			}
			if gtpTrafficPolicy.DnsPrefix != env && gtpTrafficPolicy.DnsPrefix != common.Default &&
				gtpTrafficPolicy.Dns != host {
				host = common.GetCnameVal([]string{gtpTrafficPolicy.DnsPrefix, se.Hosts[0]})
				drName, seName = getIstioResourceName(host, "-dr"), getIstioResourceName(host, "-se")
				modifiedSe = copyServiceEntry(se)
				modifiedSe.Hosts[0] = host
				modifiedSe.Addresses[0] = getUniqueAddress(ctx, cache, host)
			}
			var seDr = &SeDrTuple{
				DrName:                      drName,
				SeName:                      seName,
				DestinationRule:             getDestinationRule(modifiedSe, region, gtpTrafficPolicy),
				ServiceEntry:                modifiedSe,
				SeDnsPrefix:                 gtpTrafficPolicy.DnsPrefix,
				SeDrGlobalTrafficPolicyName: globalTrafficPolicy.Name,
			}
			seDrSet[host] = seDr
		}
	}
	//create a destination rule for default hostname if that wasn't overriden in gtp
	if _, ok := seDrSet[se.Hosts[0]]; !ok {
		var seDr = &SeDrTuple{
			DrName:          defaultDrName,
			SeName:          defaultSeName,
			DestinationRule: getDestinationRule(se, region, nil),
			ServiceEntry:    se,
		}
		seDrSet[se.Hosts[0]] = seDr
	}
	return seDrSet
}

func makeRemoteEndpointForServiceEntry(address string, locality string, portName string, portNumber int) *networking.WorkloadEntry {
	return &networking.WorkloadEntry{Address: address,
		Locality: locality,
		Ports:    map[string]uint32{portName: uint32(portNumber)}} //
}

func copyServiceEntry(se *networking.ServiceEntry) *networking.ServiceEntry {
	var newSe = &networking.ServiceEntry{}
	se.DeepCopyInto(newSe)
	return newSe
}

func loadServiceEntryCacheData(ctx context.Context, c admiral.ConfigMapControllerInterface, admiralCache *AdmiralCache) {
	configmap, err := c.GetConfigMap(ctx)
	if err != nil {
		log.Warnf("Failed to refresh configmap state Error: %v", err)
		return //No need to invalidate the cache
	}

	entryCache := GetServiceEntryStateFromConfigmap(configmap)

	if entryCache != nil {
		*admiralCache.ServiceEntryAddressStore = *entryCache
		log.Infof("Successfully updated service entry cache state")
	}

}

//GetLocalAddressForSe gets a guarenteed unique local address for a serviceentry. Returns the address, True iff the configmap was updated false otherwise, and an error if any
//Any error coupled with an empty string address means the method should be retried
func GetLocalAddressForSe(ctx context.Context, seName string, seAddressCache *ServiceEntryAddressStore, configMapController admiral.ConfigMapControllerInterface) (string, bool, error) {
	var address = seAddressCache.EntryAddresses[seName]
	if len(address) == 0 {
		address, err := GenerateNewAddressAndAddToConfigMap(ctx, seName, configMapController)
		return address, true, err
	}
	return address, false, nil
}

func GetServiceEntriesByCluster(ctx context.Context, clusterID string, remoteRegistry *RemoteRegistry) ([]*v1alpha3.ServiceEntry, error) {
	remoteController := remoteRegistry.GetRemoteController(clusterID)

	if remoteController != nil {
		serviceEnteries, err := remoteController.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(common.GetSyncNamespace()).List(ctx, v12.ListOptions{})

		if err != nil {
			log.Errorf(LogFormat, "Get", "ServiceEntries", "", clusterID, err)
			return nil, err
		}

		return serviceEnteries.Items, nil
	} else {
		err := fmt.Errorf("Admiral is not monitoring cluster %s", clusterID)
		return nil, err
	}
}

//GenerateNewAddressAndAddToConfigMap an atomic fetch and update operation against the configmap (using K8s built in optimistic consistency mechanism via resource version)
func GenerateNewAddressAndAddToConfigMap(ctx context.Context, seName string, configMapController admiral.ConfigMapControllerInterface) (string, error) {
	//1. get cm, see if there. 2. gen new uq address. 3. put configmap. RETURN SUCCESSFULLY IFF CONFIGMAP PUT SUCCEEDS
	cm, err := configMapController.GetConfigMap(ctx)
	if err != nil {
		return "", err
	}

	newAddressState := GetServiceEntryStateFromConfigmap(cm)

	if newAddressState == nil {
		return "", errors.New("could not unmarshall configmap yaml")
	}

	if val, ok := newAddressState.EntryAddresses[seName]; ok { //Someone else updated the address state, so we'll use that
		return val, nil
	}

	secondIndex := (len(newAddressState.Addresses) / 255) + 10
	firstIndex := (len(newAddressState.Addresses) % 255) + 1
	address := configMapController.GetIPPrefixForServiceEntries() + common.Sep + strconv.Itoa(secondIndex) + common.Sep + strconv.Itoa(firstIndex)

	for util.Contains(newAddressState.Addresses, address) {
		if firstIndex < 255 {
			firstIndex++
		} else {
			secondIndex++
			firstIndex = 0
		}
		address = configMapController.GetIPPrefixForServiceEntries() + common.Sep + strconv.Itoa(secondIndex) + common.Sep + strconv.Itoa(firstIndex)
	}
	newAddressState.Addresses = append(newAddressState.Addresses, address)
	newAddressState.EntryAddresses[seName] = address

	err = putServiceEntryStateFromConfigmap(ctx, configMapController, cm, newAddressState)

	if err != nil {
		return "", err
	}
	return address, nil
}

//puts new data into an existing configmap. Providing the original is necessary to prevent fetch and update race conditions
func putServiceEntryStateFromConfigmap(ctx context.Context, c admiral.ConfigMapControllerInterface, originalConfigmap *k8sV1.ConfigMap, data *ServiceEntryAddressStore) error {
	if originalConfigmap == nil {
		return errors.New("configmap must not be nil")
	}

	bytes, err := yaml.Marshal(data)

	if err != nil {
		log.Errorf("Failed to put service entry state into the configmap. %v", err)
		return err
	}

	if originalConfigmap.Data == nil {
		originalConfigmap.Data = map[string]string{}
	}

	originalConfigmap.Data["serviceEntryAddressStore"] = string(bytes)

	err = ValidateConfigmapBeforePutting(originalConfigmap)
	if err != nil {
		log.Errorf("Configmap failed validation. Something is wrong. Error: %v", err)
		return err
	}

	return c.PutConfigMap(ctx, originalConfigmap)
}

func createServiceEntryForRollout(ctx context.Context, event admiral.EventType, rc *RemoteController, admiralCache *AdmiralCache,
	meshPorts map[string]uint32, destRollout *argo.Rollout, serviceEntries map[string]*networking.ServiceEntry) *networking.ServiceEntry {

	workloadIdentityKey := common.GetWorkloadIdentifier()
	globalFqdn := common.GetCnameForRollout(destRollout, workloadIdentityKey, common.GetHostnameSuffix())

	//Handling retries for getting/putting service entries from/in cache

	address := getUniqueAddress(ctx, admiralCache, globalFqdn)

	if len(globalFqdn) == 0 || len(address) == 0 {
		return nil
	}

	san := getSanForRollout(destRollout, workloadIdentityKey)

	if destRollout.Spec.Strategy.BlueGreen != nil && destRollout.Spec.Strategy.BlueGreen.PreviewService != "" {
		rolloutServices := getServiceForRollout(ctx, rc, destRollout)
		if _, ok := rolloutServices[destRollout.Spec.Strategy.BlueGreen.PreviewService]; ok {
			previewGlobalFqdn := common.BlueGreenRolloutPreviewPrefix + common.Sep + common.GetCnameForRollout(destRollout, workloadIdentityKey, common.GetHostnameSuffix())
			previewAddress := getUniqueAddress(ctx, admiralCache, previewGlobalFqdn)
			if len(previewGlobalFqdn) != 0 && len(previewAddress) != 0 {
				generateServiceEntry(event, admiralCache, meshPorts, previewGlobalFqdn, rc, serviceEntries, previewAddress, san)
			}
		}
	}

	tmpSe := generateServiceEntry(event, admiralCache, meshPorts, globalFqdn, rc, serviceEntries, address, san)
	return tmpSe
}

func getSanForDeployment(destDeployment *k8sAppsV1.Deployment, workloadIdentityKey string) (san []string) {
	if common.GetEnableSAN() {
		tmpSan := common.GetSAN(common.GetSANPrefix(), destDeployment, workloadIdentityKey)
		if len(tmpSan) > 0 {
			return []string{common.GetSAN(common.GetSANPrefix(), destDeployment, workloadIdentityKey)}
		}
	}
	return nil

}

func getSanForRollout(destRollout *argo.Rollout, workloadIdentityKey string) (san []string) {
	if common.GetEnableSAN() {
		tmpSan := common.GetSANForRollout(common.GetSANPrefix(), destRollout, workloadIdentityKey)
		if len(tmpSan) > 0 {
			return []string{common.GetSANForRollout(common.GetSANPrefix(), destRollout, workloadIdentityKey)}
		}
	}
	return nil

}

func getUniqueAddress(ctx context.Context, admiralCache *AdmiralCache, globalFqdn string) (address string) {

	//initializations
	var err error = nil
	maxRetries := 3
	counter := 0
	address = ""
	needsCacheUpdate := false

	for err == nil && counter < maxRetries {
		address, needsCacheUpdate, err = GetLocalAddressForSe(ctx, getIstioResourceName(globalFqdn, "-se"), admiralCache.ServiceEntryAddressStore, admiralCache.ConfigMapController)

		if err != nil {
			log.Errorf("Error getting local address for Service Entry. Err: %v", err)
			break
		}

		//random expo backoff
		timeToBackoff := rand.Intn(int(math.Pow(100.0, float64(counter)))) //get a random number between 0 and 100^counter. Will always be 0 the first time, will be 0-100 the second, and 0-1000 the third
		time.Sleep(time.Duration(timeToBackoff) * time.Millisecond)

		counter++
	}

	if err != nil {
		log.Errorf("Could not get unique address after %v retries. Failing to create serviceentry name=%v", maxRetries, globalFqdn)
		return address
	}

	if needsCacheUpdate {
		loadServiceEntryCacheData(ctx, admiralCache.ConfigMapController, admiralCache)
	}

	return address
}

func generateServiceEntry(event admiral.EventType, admiralCache *AdmiralCache, meshPorts map[string]uint32, globalFqdn string, rc *RemoteController, serviceEntries map[string]*networking.ServiceEntry, address string, san []string) *networking.ServiceEntry {
	admiralCache.CnameClusterCache.Put(globalFqdn, rc.ClusterID, rc.ClusterID)

	tmpSe := serviceEntries[globalFqdn]

	var finalProtocol = common.Http

	var sePorts = []*networking.Port{{Number: uint32(common.DefaultServiceEntryPort),
		Name: finalProtocol, Protocol: finalProtocol}}

	for protocol := range meshPorts {
		sePorts = []*networking.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: protocol, Protocol: protocol}}
		finalProtocol = protocol
	}

	if tmpSe == nil {
		tmpSe = &networking.ServiceEntry{
			Hosts:           []string{globalFqdn},
			Ports:           sePorts,
			Location:        networking.ServiceEntry_MESH_INTERNAL,
			Resolution:      networking.ServiceEntry_DNS,
			Addresses:       []string{address}, //It is possible that the address is an empty string. That is fine as the se creation will fail and log an error
			SubjectAltNames: san,
		}
		tmpSe.Endpoints = []*networking.WorkloadEntry{}
	}

	endpointAddress, port := rc.ServiceController.Cache.GetLoadBalancer(common.GetAdmiralParams().LabelSet.GatewayApp, common.NamespaceIstioSystem)

	var locality string
	if rc.NodeController.Locality != nil {
		locality = rc.NodeController.Locality.Region
	}
	seEndpoint := makeRemoteEndpointForServiceEntry(endpointAddress,
		locality, finalProtocol, port)

	// if the action is deleting an endpoint from service entry, loop through the list and delete matching ones
	if event == admiral.Add || event == admiral.Update {
		tmpSe.Endpoints = append(tmpSe.Endpoints, seEndpoint)
	} else if event == admiral.Delete {
		// create a tmp endpoint list to store all the endpoints that we intend to keep
		remainEndpoints := []*networking.WorkloadEntry{}
		// if the endpoint is not equal to the endpoint we intend to delete, append it to remainEndpoint list
		for _, existingEndpoint := range tmpSe.Endpoints {
			if !reflect.DeepEqual(existingEndpoint, seEndpoint) {
				remainEndpoints = append(remainEndpoints, existingEndpoint)
			}
		}
		// If no endpoints left for particular SE, we can delete the service entry object itself later inside function
		// AddServiceEntriesWithDr when updating SE, leave an empty shell skeleton here
		tmpSe.Endpoints = remainEndpoints
	}

	serviceEntries[globalFqdn] = tmpSe

	return tmpSe
}

func isBlueGreenStrategy(rollout *argo.Rollout) bool {
	if rollout != nil && &rollout.Spec != (&argo.RolloutSpec{}) && rollout.Spec.Strategy != (argo.RolloutStrategy{}) {
		if rollout.Spec.Strategy.BlueGreen != nil {
			return true
		}
	}
	return false
}
