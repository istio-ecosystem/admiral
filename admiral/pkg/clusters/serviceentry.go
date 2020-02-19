package clusters

import (
	"errors"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	networking "istio.io/api/networking/v1alpha3"
	istioModel "istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/log"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	"math"
	"math/rand"
	"strconv"
	"time"
)

func createServiceEntry(rc *RemoteController, admiralCache *AdmiralCache,
	destDeployment *k8sAppsV1.Deployment, serviceEntries map[string]*networking.ServiceEntry) *networking.ServiceEntry {

	workloadIdentityKey := common.GetWorkloadIdentifier()
	globalFqdn := common.GetCname(destDeployment, workloadIdentityKey, common.GetHostnameSuffix())

	//Handling retries for getting/putting service entries from/in cache

	//initializations
	var err error =  nil
	maxRetries := 3
	counter := 0
	address := ""
	needsCacheUpdate := false

	for err==nil && counter<maxRetries {
		address, needsCacheUpdate, err = GetLocalAddressForSe(getIstioResourceName(globalFqdn, "-se"), admiralCache.ServiceEntryAddressStore, admiralCache.ConfigMapController)

		if err != nil {
			log.Errorf("Error getting local address for Service Entry. Err: %v", err)
			break
		}

		//random expo backoff
		timeToBackoff := rand.Intn(int(math.Pow(100.0, float64(counter)))) //get a random number between 0 and 100^counter. Will always be 0 the first time, will be 0-100 the second, and 0-1000 the third
		time.Sleep(time.Duration(timeToBackoff)*time.Millisecond)

		counter++
	}

	if err != nil {
		log.Errorf("Could not get unique address after %v retries. Failing to create serviceentry name=%v", maxRetries, globalFqdn)
		return nil
	}

	if needsCacheUpdate {
		loadServiceEntryCacheData(admiralCache.ConfigMapController, admiralCache)
	}

	if len(globalFqdn) == 0 {
		return nil
	}

	var san []string
	if common.GetEnableSAN() {
		tmpSan := common.GetSAN(common.GetSANPrefix(), destDeployment, workloadIdentityKey)
		if len(tmpSan) > 0 {
			san = []string{common.GetSAN(common.GetSANPrefix(), destDeployment, workloadIdentityKey)}
		}
	} else {
		san = nil
	}

	admiralCache.CnameClusterCache.Put(globalFqdn, rc.ClusterID, rc.ClusterID)

	tmpSe := serviceEntries[globalFqdn]

	if tmpSe == nil {

		tmpSe = &networking.ServiceEntry{
			Hosts: []string{globalFqdn},
			//ExportTo: []string{"*"}, --> //TODO this is causing a coredns plugin to fail serving the DNS entry
			Ports: []*networking.Port{{Number: uint32(common.DefaultHttpPort),
				Name: common.Http, Protocol: common.Http}},
			Location:        networking.ServiceEntry_MESH_INTERNAL,
			Resolution:      networking.ServiceEntry_DNS,
			Addresses:       []string{address}, //It is possible that the address is an empty string. That is fine as the se creation will fail and log an error
			SubjectAltNames: san,
		}
		tmpSe.Endpoints = []*networking.ServiceEntry_Endpoint{}
	}

	endpointAddress := rc.ServiceController.Cache.GetLoadBalancer(admiral.IstioIngressServiceName, common.NamespaceIstioSystem)
	var locality string
	if rc.NodeController.Locality != nil {
		locality = rc.NodeController.Locality.Region
	}
	seEndpoint := makeRemoteEndpointForServiceEntry(endpointAddress,
		locality, common.Http)
	tmpSe.Endpoints = append(tmpSe.Endpoints, seEndpoint)

	serviceEntries[globalFqdn] = tmpSe

	return tmpSe
}

func createServiceEntryForNewServiceOrPod(env string, sourceIdentity string, remoteRegistry *RemoteRegistry) map[string]*networking.ServiceEntry {
	//create a service entry, destination rule and virtual service in the local cluster
	sourceServices := make(map[string]*k8sV1.Service)

	sourceDeployments := make(map[string]*k8sAppsV1.Deployment)

	var serviceEntries = make(map[string]*networking.ServiceEntry)

	var cname string

	for _, rc := range remoteRegistry.remoteControllers {

		deployment := rc.DeploymentController.Cache.Get(sourceIdentity)

		if deployment == nil || deployment.Deployments[env] == nil {
			continue
		}

		deploymentInstance := deployment.Deployments[env]

		serviceInstance := getServiceForDeployment(rc, deploymentInstance[0])

		cname = common.GetCname(deploymentInstance[0], common.GetWorkloadIdentifier(), cname)

		remoteRegistry.AdmiralCache.IdentityClusterCache.Put(sourceIdentity, rc.ClusterID, rc.ClusterID)
		remoteRegistry.AdmiralCache.CnameClusterCache.Put(cname, rc.ClusterID, rc.ClusterID)
		remoteRegistry.AdmiralCache.CnameIdentityCache.Store(cname, sourceIdentity)
		sourceServices[rc.ClusterID] = serviceInstance

		sourceDeployments[rc.ClusterID] = deploymentInstance[0]

		createServiceEntry(rc, remoteRegistry.AdmiralCache, deploymentInstance[0], serviceEntries)

	}

	dependents := remoteRegistry.AdmiralCache.IdentityDependencyCache.Get(sourceIdentity)

	dependentClusters := getDependentClusters(dependents, remoteRegistry.AdmiralCache.IdentityClusterCache, sourceServices)

	//update cname dependent cluster cache
	for clusterId, _ := range dependentClusters {
		remoteRegistry.AdmiralCache.CnameDependentClusterCache.Put(cname, clusterId, clusterId)
	}

	AddServiceEntriesWithDr(remoteRegistry.AdmiralCache, dependentClusters, remoteRegistry.remoteControllers, serviceEntries)

	//update the address to local fqdn for service entry in a cluster local to the service instance
	for sourceCluster, serviceInstance := range sourceServices {
		localFqdn := serviceInstance.Name + common.Sep + serviceInstance.Namespace + common.DotLocalDomainSuffix
		rc := remoteRegistry.remoteControllers[sourceCluster]
		var meshPorts = GetMeshPorts(sourceCluster, serviceInstance, sourceDeployments[sourceCluster])
		for key, serviceEntry := range serviceEntries {
			for _, ep := range serviceEntry.Endpoints {
				clusterIngress := rc.ServiceController.Cache.GetLoadBalancer(admiral.IstioIngressServiceName, common.NamespaceIstioSystem)
				//replace istio ingress-gateway address with local fqdn, note that ingress-gateway can be empty (not provisoned, or is not up)
				if ep.Address == clusterIngress || ep.Address == "" {
					ep.Address = localFqdn
					oldPorts := ep.Ports
					ep.Ports = meshPorts
					AddServiceEntriesWithDr(remoteRegistry.AdmiralCache, map[string]string{sourceCluster: sourceCluster}, remoteRegistry.remoteControllers,
						map[string]*networking.ServiceEntry{key: serviceEntry})
					//swap it back to use for next iteration
					ep.Address = clusterIngress
					ep.Ports = oldPorts
				}
			}

			//add virtual service for routing locally in within the cluster
			//virtualServiceName := getIstioResourceName(cname, "-default-vs")
			//
			//oldVirtualService := rc.IstioConfigStore.Get(istioModel.VirtualService.Type, virtualServiceName, remoteRegistry.config.SyncNamespace)
			//
			//virtualService := makeVirtualService(serviceEntry.Hosts[0], localFqdn, meshPorts[common.Http])
			//
			//newVirtualService, err := createIstioConfig(istio.VirtualServiceProto, virtualService, virtualServiceName, remoteRegistry.config.SyncNamespace)
			//
			//if err == nil {
			//	addUpdateIstioResource(rc, *newVirtualService, oldVirtualService, virtualServiceName, syncNamespace)
			//} else {
			//	log.Errorf(LogErrFormat, "Create", istioModel.VirtualService.Type, virtualServiceName, rc.ClusterID, err)
			//}
		}
	}
	return serviceEntries
}

func createSeWithDrLabels(remoteController *RemoteController, localCluster bool, identityId string, seName string, se *networking.ServiceEntry,
	dr *networking.DestinationRule, seAddressCache *ServiceEntryAddressStore, configmapController admiral.ConfigMapControllerInterface) map[string]*networking.ServiceEntry {
	var allSes = make(map[string]*networking.ServiceEntry)
	var newSe = copyServiceEntry(se)

	address, _, err := GetLocalAddressForSe(seName, seAddressCache, configmapController)
	if err != nil {
		log.Warnf("Failed to get address for dr service entry. Not creating it. err:%v",err)
		return nil
	}
	newSe.Addresses = []string{address}

	var endpoints = make([]*networking.ServiceEntry_Endpoint, 0)

	for _, endpoint := range se.Endpoints {
		for _, subset := range dr.Subsets {
			newEndpoint := copyEndpoint(endpoint)
			newEndpoint.Labels = subset.Labels

			////create a service entry with name subsetSeName
			//if localCluster {
			//	subsetSeName := seName + common.Dash + subset.Name
			//	subsetSeAddress := strings.Split(se.Hosts[0], common.DotMesh)[0] + common.Sep + subset.Name + common.DotMesh BROKEN MUST FIX //todo fix the cname format here
			//
			//	//TODO uncomment the line below when subset routing across clusters is fixed
			//	//newEndpoint.Address = subsetSeAddress
			//
			//	subSetSe := createSeWithPodIps(remoteController, identityId, subsetSeName, subsetSeAddress, newSe, newEndpoint, subset, seAddressMap)
			//	if subSetSe != nil {
			//		allSes[subsetSeName] = subSetSe
			//		//TODO create default DestinationRules for these subset SEs
			//	}
			//}

			endpoints = append(endpoints, newEndpoint)

		}
	}
	newSe.Endpoints = endpoints
	allSes[seName] = newSe
	return allSes
}

func AddServiceEntriesWithDr(cache *AdmiralCache, sourceClusters map[string]string, rcs map[string]*RemoteController, serviceEntries map[string]*networking.ServiceEntry) {
	syncNamespace := common.GetSyncNamespace()
	for _, se := range serviceEntries {

		//add service entry
		serviceEntryName := getIstioResourceName(se.Hosts[0], "-se")

		destinationRuleName := getIstioResourceName(se.Hosts[0], "-default-dr")

		for _, sourceCluster := range sourceClusters {

			rc := rcs[sourceCluster]

			if rc == nil {
				log.Warnf(LogFormat, "Find", "remote-controller", sourceCluster, sourceCluster, "doesn't exist")
				continue
			}

			oldServiceEntry := rc.IstioConfigStore.Get(istioModel.ServiceEntry.Type, serviceEntryName, syncNamespace)

			newServiceEntry, err := createIstioConfig(istio.ServiceEntryProto, se, serviceEntryName, syncNamespace)

			//Add a label
			if identityId, ok := cache.CnameIdentityCache.Load(se.Hosts[0]); ok {
				newServiceEntry.Labels = map[string]string{common.GetWorkloadIdentifier(): fmt.Sprintf("%v", identityId)}
			}

			if err == nil {
				addUpdateIstioResource(rc, *newServiceEntry, oldServiceEntry, istioModel.ServiceEntry.Type, syncNamespace)
			} else {
				log.Infof(LogFormat, "CreateConfig", istioModel.ServiceEntry.Type, serviceEntryName, sourceCluster, err)
			}

			//add destination rule
			oldDestinationRule := rc.IstioConfigStore.Get(istioModel.DestinationRule.Type, destinationRuleName, syncNamespace)

			destinationRule := getDestinationRule(se.Hosts[0])

			newDestinationRule, err := createIstioConfig(istio.DestinationRuleProto, destinationRule, destinationRuleName, syncNamespace)

			if err == nil {
				addUpdateIstioResource(rc, *newDestinationRule, oldDestinationRule, istioModel.DestinationRule.Type, syncNamespace)
			} else {
				log.Infof(LogFormat, "CreateConfig", istioModel.DestinationRule.Type, destinationRuleName, sourceCluster, err)
			}
		}
	}
}

func makeRemoteEndpointForServiceEntry(address string, locality string, portName string) *networking.ServiceEntry_Endpoint {
	return &networking.ServiceEntry_Endpoint{Address: address,
		Locality: locality,
		Ports:    map[string]uint32{portName: common.DefaultMtlsPort}} //
}

func copyServiceEntry(se *networking.ServiceEntry) *networking.ServiceEntry {
	return &networking.ServiceEntry{Ports: se.Ports, Resolution: se.Resolution, Hosts: se.Hosts, Location: se.Location,
		SubjectAltNames: se.SubjectAltNames, ExportTo: se.ExportTo, Endpoints: se.Endpoints, Addresses: se.Addresses}
}

func loadServiceEntryCacheData(c admiral.ConfigMapControllerInterface, admiralCache *AdmiralCache) {
	configmap, err := c.GetConfigMap()
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

//Gets a guarenteed unique local address for a serviceentry. Returns the address, True iff the configmap was updated false otherwise, and an error if any
//Any error coupled with an empty string address means the method should be retried
func GetLocalAddressForSe(seName string, seAddressCache *ServiceEntryAddressStore, configMapController admiral.ConfigMapControllerInterface) (string, bool, error) {
	var address = seAddressCache.EntryAddresses[seName]
	if len(address) == 0 {
		address, err := GenerateNewAddressAndAddToConfigMap(seName, configMapController)
		return address, true, err
	}
	return address, false, nil
}

//an atomic fetch and update operation against the configmap (using K8s built in optimistic consistency mechanism via resource version)
func GenerateNewAddressAndAddToConfigMap(seName string, configMapController admiral.ConfigMapControllerInterface)  (string, error){
	//1. get cm, see if there. 2. gen new uq address. 3. put configmap. RETURN SUCCESSFULLY IFF CONFIGMAP PUT SUCCEEDS
	cm, err := configMapController.GetConfigMap()
	if err != nil {
		return "", err
	}

	newAddressState := GetServiceEntryStateFromConfigmap(cm)

	if newAddressState == nil {
		return "",  errors.New("could not unmarshall configmap yaml")
	}

	if val, ok := newAddressState.EntryAddresses[seName]; ok { //Someone else updated the address state, so we'll use that
		return val, nil
	}

	secondIndex := (len(newAddressState.Addresses) / 255) + 10
	firstIndex := (len(newAddressState.Addresses) % 255) + 1
	address := common.LocalAddressPrefix + common.Sep + strconv.Itoa(secondIndex) + common.Sep + strconv.Itoa(firstIndex)

	for util.Contains(newAddressState.Addresses, address) {
		if firstIndex<255 {
			firstIndex++
		} else {
			secondIndex++
			firstIndex=0
		}
		address = common.LocalAddressPrefix + common.Sep + strconv.Itoa(secondIndex) + common.Sep + strconv.Itoa(firstIndex)
	}
	newAddressState.Addresses = append(newAddressState.Addresses, address)
	newAddressState.EntryAddresses[seName] = address

	err = putServiceEntryStateFromConfigmap(configMapController, cm, newAddressState)

	if err != nil {
		return "", err
	}
	return address, nil
}

//puts new data into an existing configmap. Providing the original is necessary to prevent fetch and update race conditions
func putServiceEntryStateFromConfigmap(c admiral.ConfigMapControllerInterface, originalConfigmap *k8sV1.ConfigMap, data *ServiceEntryAddressStore) error {
	if originalConfigmap == nil {
		return errors.New("configmap must not be nil")
	}

	bytes, err := yaml.Marshal(data)

	if err != nil {
		logrus.Errorf("Failed to put service entry state into the configmap. %v", err)
		return err
	}

	if originalConfigmap.Data == nil {
		originalConfigmap.Data = map[string]string{}
	}

	originalConfigmap.Data["serviceEntryAddressStore"] = string(bytes)

	err = ValidateConfigmapBeforePutting(originalConfigmap)
	if err != nil {
		logrus.Errorf("Configmap failed validation. Something is wrong. Error: %v", err)
		return err
	}

	return c.PutConfigMap(originalConfigmap)
}