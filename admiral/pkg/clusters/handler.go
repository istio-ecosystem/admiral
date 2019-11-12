package clusters

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/sirupsen/logrus"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"

	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"

	networking "istio.io/api/networking/v1alpha3"
)


type DependencyHandler struct {
	RemoteRegistry *RemoteRegistry
	DepController  *admiral.DependencyController
}

type GlobalTrafficHandler struct {
	RemoteRegistry *RemoteRegistry
	GlobalTrafficController  *admiral.GlobalTrafficController
}

type DeploymentHandler struct {
	RemoteRegistry *RemoteRegistry
}

type PodHandler struct {
	RemoteRegistry *RemoteRegistry
}

type NodeHandler struct {
	RemoteRegistry *RemoteRegistry
}

type ServiceHandler struct {
	RemoteRegistry *RemoteRegistry
}

type ServiceEntryHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID string
}

type DestinationRuleHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID string
}

type VirtualServiceHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID string
}

func (dh *DependencyHandler) Added(obj *v1.Dependency) {

	logrus.Infof(LogFormat, "Event", "dependency-record", obj.Name, "", "Received=true namespace="+obj.Namespace)

	sourceIdentity := obj.Spec.Source

	if len(sourceIdentity) == 0 {
		logrus.Infof(LogFormat, "Event", "dependency-record", obj.Name, "", "No identity found namespace="+obj.Namespace)
	}

	updateIdentityDependencyCache(sourceIdentity, dh.RemoteRegistry.AdmiralCache.IdentityDependencyCache, obj)

	handleDependencyRecord(obj.Spec.IdentityLabel, sourceIdentity, dh.RemoteRegistry, dh.RemoteRegistry.remoteControllers, dh.RemoteRegistry.config, obj)

}

func updateIdentityDependencyCache(sourceIdentity string, identityDependencyCache *common.MapOfMaps, dr *v1.Dependency) {
	for _, dIdentity := range dr.Spec.Destinations {
		identityDependencyCache.Put(dIdentity, sourceIdentity, sourceIdentity)
	}
	logrus.Infof(LogFormat, "Update", "dependency-cache", dr.Name, "", "Updated=true namespace="+dr.Namespace)
}

func handleDependencyRecord(identifier string, sourceIdentity string, r *RemoteRegistry, rcs map[string]*RemoteController, config AdmiralParams, obj *v1.Dependency) {

	destinationIdentitys := obj.Spec.Destinations

	destinationClusters := make(map[string]string)

	sourceClusters := make(map[string]string)

	var serviceEntries = make(map[string]*networking.ServiceEntry)

	for _, rc := range rcs {

		//for every cluster the source identity is running, add their istio ingress as service entry address
		tempDeployment := rc.DeploymentController.Cache.Get(sourceIdentity)
		if tempDeployment != nil {
			sourceClusters[rc.ClusterID] = rc.ClusterID
			r.AdmiralCache.IdentityClusterCache.Put(sourceIdentity, rc.ClusterID, rc.ClusterID)
		}

		//create and store destination service entries
		for _, destinationCluster := range destinationIdentitys {
			destDeployment := rc.DeploymentController.Cache.Get(destinationCluster)
			if destDeployment == nil {
				continue
			}

			//deployment can be in multiple clusters, create SEs for all clusters

			for _, deployment := range destDeployment.Deployments {

				r.AdmiralCache.IdentityClusterCache.Put(destinationCluster, rc.ClusterID, rc.ClusterID)

				deployments := rc.DeploymentController.Cache.Get(destinationCluster)

				if deployments == nil || len(deployments.Deployments) == 0 {
					continue
				}
				//TODO pass deployment
				tmpSe := createServiceEntry(identifier, rc, config, r.AdmiralCache, deployment[0], serviceEntries)

				if tmpSe == nil {
					continue
				}

				destinationClusters[rc.ClusterID] = tmpSe.Hosts[0] //Only single host supported

				r.AdmiralCache.CnameIdentityCache.Store(tmpSe.Hosts[0], destinationCluster)

				serviceEntries[tmpSe.Hosts[0]] = tmpSe
			}

		}
	}

	if len(sourceClusters) == 0 || len(serviceEntries) == 0 {
		logrus.Infof(LogFormat, "Event", "dependency-record", sourceIdentity, "", "skipped")
		return
	}

	for dCluster, globalFqdn := range destinationClusters {
		for _, sCluster := range sourceClusters {
			r.AdmiralCache.CnameClusterCache.Put(globalFqdn, dCluster, dCluster)
			r.AdmiralCache.CnameDependentClusterCache.Put(globalFqdn, sCluster, sCluster)
			//filter out the source clusters same as destinationClusters
			delete(sourceClusters, dCluster)
		}
	}

	//add service entries for all dependencies in source cluster
	addServiceEntriesWithDr(r, sourceClusters, rcs, serviceEntries)
}

func getIstioResourceName(host string, suffix string) string {
	return strings.ToLower(host) + suffix
}

func createServiceEntry(identifier string, rc *RemoteController, config AdmiralParams, admiralCache *AdmiralCache,
	destDeployment *k8sAppsV1.Deployment, serviceEntries map[string]*networking.ServiceEntry) *networking.ServiceEntry {

	globalFqdn := common.GetCname(destDeployment, identifier)

	if len(globalFqdn) == 0 {
		return nil
	}

	var san []string
	if config.EnableSAN {
		tmpSan := common.GetSAN(config.SANPrefix, destDeployment, identifier)
		if len(tmpSan) > 0 {
			san = []string{common.GetSAN(config.SANPrefix, destDeployment, identifier)}
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
			Addresses:       []string{common.GetLocalAddressForSe(getIstioResourceName(globalFqdn, "-se"), admiralCache.ServiceEntryAddressCache)},
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

func addServiceEntriesWithDr(r *RemoteRegistry, sourceClusters map[string]string, rcs map[string]*RemoteController, serviceEntries map[string]*networking.ServiceEntry) {
	var syncNamespace = r.config.SyncNamespace
	for _, se := range serviceEntries {

		//add service entry
		serviceEntryName := getIstioResourceName(se.Hosts[0], "-se")

		destinationRuleName := getIstioResourceName(se.Hosts[0], "-default-dr")

		for _, sourceCluster := range sourceClusters {

			rc := rcs[sourceCluster]

			if rc == nil {
				logrus.Warnf(LogFormat, "Find", "remote-controller", sourceCluster, sourceCluster, "doesn't exist")
				continue
			}

			var oldServiceEntry *v1alpha3.ServiceEntry
			oldServiceEntry, err := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(syncNamespace).Get(serviceEntryName, v12.GetOptions{})
			if err != nil {
				oldServiceEntry = nil
			}

			newServiceEntry := createServiceEntrySkeletion(*se, serviceEntryName, syncNamespace)

			//Add a label
			identityId, ok := r.AdmiralCache.CnameIdentityCache.Load(se.Hosts[0])
			if ok {
				newServiceEntry.Labels = map[string]string{common.DefaultGlobalIdentifier(): fmt.Sprintf("%v", identityId)}
			}

			addUpdateServiceEntry(newServiceEntry, oldServiceEntry, syncNamespace, rc)

			oldDestinationRule, err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(destinationRuleName, v12.GetOptions{})

			if err != nil {
				oldDestinationRule = nil
			}

			globalTrafficPolicy := getMatchingGlobalTrafficPolicy(rc, identityId.(string))

			destinationRule := getDestinationRule(se.Hosts[0], rc.NodeController.Locality.Region, globalTrafficPolicy)

			newDestinationRule  := createDestinationRulSkeletion(*destinationRule, destinationRuleName, syncNamespace)

			addUpdateDestinationRule(newDestinationRule, oldDestinationRule, syncNamespace, rc)
		}

	}
}

//TODO use selector on pod, instead of hardcoded identityId
func getMatchingGlobalTrafficPolicy(rc *RemoteController, identityId string) *v1.GlobalTrafficPolicy {
	return rc.GlobalTraffic.Cache.Get(identityId)
}

func makeVirtualService(host string, destination string, port uint32) *networking.VirtualService {
	return &networking.VirtualService{Hosts: []string{host},
		Gateways: []string{common.Mesh, common.MulticlusterIngressGateway},
		ExportTo: []string{"*"},
		Http:     []*networking.HTTPRoute{{Route: []*networking.HTTPRouteDestination{{Destination: &networking.Destination{Host: destination, Port: &networking.PortSelector{Number: port}}}}}}}
}

func makeRemoteEndpointForServiceEntry(address string, locality string, portName string) *networking.ServiceEntry_Endpoint {
	return &networking.ServiceEntry_Endpoint{Address: address,
		Locality: locality,
		Ports:    map[string]uint32{portName: common.DefaultMtlsPort}} //
}

func getDestinationRule(host string, locality string, gtpWrapper *v1.GlobalTrafficPolicy) *networking.DestinationRule {
	var dr = &networking.DestinationRule{}
	dr.Host = host
	dr.TrafficPolicy = &networking.TrafficPolicy{Tls: &networking.TLSSettings{Mode: networking.TLSSettings_ISTIO_MUTUAL}}
	if gtpWrapper != nil {
		var loadBalancerSettings = &networking.LoadBalancerSettings{}
		gtp := gtpWrapper.Spec
		gtpTrafficPolicy := gtp.Policy[0]
		if len(gtpTrafficPolicy.Target) > 0 {
			var localityLbSettings = &networking.LocalityLoadBalancerSetting{}
			if gtpTrafficPolicy.LbType == model.TrafficPolicy_FAILOVER {
				distribute := make([]*networking.LocalityLoadBalancerSetting_Distribute, 0)
				targetTrafficMap := make(map[string]uint32)
				for _, tg := range gtpTrafficPolicy.Target {
					targetTrafficMap[tg.Region] = uint32(tg.Weight)
				}
				distribute = append(distribute, &networking.LocalityLoadBalancerSetting_Distribute{
					From: locality,
					To:   targetTrafficMap,
				})
				localityLbSettings.Distribute = distribute
			} else {
				//this will have default behavior
			}
			dr.TrafficPolicy.LoadBalancer.LocalityLbSetting = localityLbSettings
			dr.TrafficPolicy.LoadBalancer = loadBalancerSettings
		}
	}
	return dr
}

func (dh *DependencyHandler) Deleted(obj *v1.Dependency) {
	logrus.Infof(LogFormat, "Deleted", "dependency", obj.Name, obj.ClusterName, "Skipping, not implemented")
}

func (gtp *GlobalTrafficHandler) Added(obj *v1.GlobalTrafficPolicy) {
	logrus.Infof(LogFormat, "Added", "trafficpolicy", obj.Name, obj.ClusterName, "Skipping, not implemented")
}

func (gtp *GlobalTrafficHandler) Updated(obj *v1.GlobalTrafficPolicy) {
	logrus.Infof(LogFormat, "Updated", "trafficpolicy", obj.Name, obj.ClusterName, "Skipping, not implemented")
}

func (gtp *GlobalTrafficHandler) Deleted(obj *v1.GlobalTrafficPolicy) {
	logrus.Infof(LogFormat, "Deleted", "trafficpolicy", obj.Name, obj.ClusterName, "Skipping, not implemented")
}

func (pc *DeploymentHandler) Added(obj *k8sAppsV1.Deployment) {
	logrus.Infof(LogFormat, "Event", "deployment", obj.Name, "", "Received")

	globalIdentifier := common.GetDeploymentGlobalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		logrus.Infof(LogFormat, "Event", "deployment", obj.Name, "", "Skipped as '"+common.DefaultGlobalIdentifier()+" was not found', namespace="+obj.Namespace)
		return
	}

	createServiceEntryForNewServiceOrPod(obj.Namespace, globalIdentifier, pc.RemoteRegistry)
}

func (pc *DeploymentHandler) Deleted(obj *k8sAppsV1.Deployment) {
	//TODO update subset service entries
}

func (pc *PodHandler) Added(obj *k8sV1.Pod) {
	logrus.Infof(LogFormat, "Event", "deployment", obj.Name, "", "Received")

	globalIdentifier := common.GetPodGlobalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		logrus.Infof(LogFormat, "Event", "deployment", obj.Name, "", "Skipped as '"+common.DefaultGlobalIdentifier()+" was not found', namespace="+obj.Namespace)
		return
	}

	createServiceEntryForNewServiceOrPod(obj.Namespace, globalIdentifier, pc.RemoteRegistry)
}

func (pc *PodHandler) Deleted(obj *k8sV1.Pod) {
	//TODO update subset service entries
}

func (nc *NodeHandler) Added(obj *k8sV1.Node) {
	//logrus.Infof("New Pod %s on cluster: %s in namespace: %s", obj.Name, obj.ClusterName, obj.Namespace)
}

func (pc *NodeHandler) Deleted(obj *k8sV1.Node) {
	//	logrus.Infof("Pod deleted %s on cluster: %s in namespace: %s", obj.Name, obj.ClusterName, obj.Namespace)
}

func (ic *ServiceEntryHandler) Added(obj *v1alpha3.ServiceEntry) {
	//logrus.Infof("New Pod %s on cluster: %s in namespace: %s", obj.Name, obj.ClusterName, obj.Namespace)
}

func (ic *ServiceEntryHandler) Updated(obj *v1alpha3.ServiceEntry) {
	//	logrus.Infof("Pod deleted %s on cluster: %s in namespace: %s", obj.Name, obj.ClusterName, obj.Namespace)
}

func (ic *ServiceEntryHandler) Deleted(obj *v1alpha3.ServiceEntry) {
	//	logrus.Infof("Pod deleted %s on cluster: %s in namespace: %s", obj.Name, obj.ClusterName, obj.Namespace)
}

func (dh *DestinationRuleHandler) Added(obj *v1alpha3.DestinationRule) {
	handleDestinationRuleEvent(obj, dh, common.Add, common.DestinationRule)
}

func (dh *DestinationRuleHandler) Updated(obj *v1alpha3.DestinationRule) {
	handleDestinationRuleEvent(obj, dh, common.Update, common.DestinationRule)
}

func (dh *DestinationRuleHandler) Deleted(obj *v1alpha3.DestinationRule) {
	handleDestinationRuleEvent(obj, dh, common.Delete, common.DestinationRule)
}

func (vh *VirtualServiceHandler) Added(obj *v1alpha3.VirtualService) {
	handleVirtualServiceEvent(obj, vh, common.Add, common.VirtualService)
}

func (vh *VirtualServiceHandler) Updated(obj *v1alpha3.VirtualService) {
	handleVirtualServiceEvent(obj, vh, common.Update, common.VirtualService)
}

func (vh *VirtualServiceHandler) Deleted(obj *v1alpha3.VirtualService) {
	handleVirtualServiceEvent(obj, vh, common.Delete, common.VirtualService)
}


func (sc *ServiceHandler) Added(obj *k8sV1.Service) {

	logrus.Infof(LogFormat, "Event", "service", obj.Name, "", "Received, doing nothing")

	//sourceIdentity := common.GetServiceGlobalIdentifier(obj)
	//
	//if len(sourceIdentity) == 0 {
	//	logrus.Infof(LogFormat, "Event", "service", obj.Name, "", "Skipped as '" + common.GlobalIdentifier() + " was not found', namespace=" + obj.Namespace)
	//	return
	//}
	//
	//createServiceEntryForNewServiceOrPod(obj.Namespace, sourceIdentity, sc.RemoteRegistry)

}

func handleDestinationRuleEvent(obj *v1alpha3.DestinationRule, dh *DestinationRuleHandler, event common.Event, resourceType common.ResourceType) {
	destinationRule := obj.Spec

	clusterId := dh.ClusterID

	localDrName := obj.Name + "-local"

	var localIdentityId string

	syncNamespace := dh.RemoteRegistry.config.SyncNamespace

	r := dh.RemoteRegistry

	if obj.Namespace == syncNamespace || obj.Namespace == common.NamespaceKubeSystem {
		logrus.Infof(LogFormat, "Event", "DestinationRule", obj.Name, clusterId, "Skipping the namespace: "+obj.Namespace)
		return
	}

	dependentClusters := r.AdmiralCache.CnameDependentClusterCache.Get(destinationRule.Host)

	if dependentClusters == nil {
		logrus.Infof("Skipping event: %s from cluster %s for %v", "DestinationRule", clusterId, destinationRule)
		logrus.Infof(LogFormat, "Event", "DestinationRule", obj.Name, clusterId, "No dependent clusters found")
		return
	}

	logrus.Infof(LogFormat, "Event", "DestinationRule", obj.Name, clusterId, "Processing")

	//Create label based service entry in source and dependent clusters for subset routing to work
	host := destinationRule.Host

	basicSEName := getIstioResourceName(host, "-se")

	seName := getIstioResourceName(obj.Name, "-se")

	allDependentClusters := make(map[string]string)

	util.MapCopy(allDependentClusters, dependentClusters.Map())

	allDependentClusters[clusterId] = clusterId

	for _, dependentCluster := range allDependentClusters {

		rc := r.remoteControllers[dependentCluster]

		var newServiceEntry *v1alpha3.ServiceEntry

		var existsServiceEntry *v1alpha3.ServiceEntry

		var drServiceEntries = make(map[string]*networking.ServiceEntry)

		exist, err := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(r.config.SyncNamespace).Get(basicSEName, v12.GetOptions{})

		var identityId = ""

		if exist == nil || err != nil {

			logrus.Warnf(LogFormat, "Find", "ServiceEntry", basicSEName, dependentCluster, "Failed")

		} else {

			serviceEntry := exist.Spec

			identityRaw, ok := r.AdmiralCache.CnameIdentityCache.Load(serviceEntry.Hosts[0])

			if ok {
				identityId = fmt.Sprintf("%v", identityRaw)
				if dependentCluster == clusterId {
					localIdentityId = identityId
				}
				drServiceEntries = createSeWithDrLabels(rc, dependentCluster == clusterId, identityId, seName, &serviceEntry, &destinationRule, r.AdmiralCache.ServiceEntryAddressCache)
			}

		}

		if event == common.Delete {

			rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(r.config.SyncNamespace).Delete(obj.Name, &v12.DeleteOptions{})
			logrus.Infof(LogFormat, "Delete", "DestinationRule", obj.Name, clusterId, "success")
			rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(r.config.SyncNamespace).Delete(seName, &v12.DeleteOptions{})
			logrus.Infof(LogFormat, "Delete", "ServiceEntry", seName, clusterId, "success")
			for _, subset := range destinationRule.Subsets {
				sseName := seName + common.Dash + subset.Name
				rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(r.config.SyncNamespace).Delete(sseName, &v12.DeleteOptions{})
				logrus.Infof(LogFormat, "Delete", "ServiceEntry", sseName, clusterId, "success")
			}
			rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(r.config.SyncNamespace).Delete(localDrName, &v12.DeleteOptions{})
			logrus.Infof(LogFormat, "Delete", "DestinationRule", localDrName, clusterId, "success")

		} else {

			exist, _ := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(r.config.SyncNamespace).Get(obj.Name, v12.GetOptions{})

			//copy destination rule only to other clusters
			if dependentCluster != clusterId {
				addUpdateDestinationRule(obj, exist, r.config.SyncNamespace, rc)
			}

			if drServiceEntries != nil {
				for _seName, se := range drServiceEntries {
					existsServiceEntry, _ = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(r.config.SyncNamespace).Get(_seName, v12.GetOptions{})
					newServiceEntry = createServiceEntrySkeletion(*se, _seName, r.config.SyncNamespace)
					if err != nil {
						logrus.Warnf(LogErrFormat, "Create", "ServiceEntry", seName, clusterId, err)
					}
					if newServiceEntry != nil {
						addUpdateServiceEntry(newServiceEntry, existsServiceEntry, r.config.SyncNamespace, rc)
					}
					//cache the subset service entries for updating them later for pod events
					if dependentCluster == clusterId && se.Resolution == networking.ServiceEntry_STATIC {
						r.AdmiralCache.SubsetServiceEntryIdentityCache.Store(identityId, map[string]string{_seName: clusterId})
					}
				}
			}

			if dependentCluster == clusterId {
				//we need a destination rule with local fqdn for destination rules created with cnames to work in local cluster
				createDestinationRuleForLocal(rc, localDrName, localIdentityId, clusterId, &destinationRule, r.config.SyncNamespace)
			}

		}
	}
}

func handleVirtualServiceEvent(obj *v1alpha3.VirtualService, vh *VirtualServiceHandler, event common.Event, resourceType common.ResourceType) {

	logrus.Infof(LogFormat, "Event", resourceType, obj.Name, vh.ClusterID, "Received event")

	virtualService := obj.Spec

	clusterId := vh.ClusterID

	r := vh.RemoteRegistry

	if obj.Namespace == r.config.SyncNamespace {
		logrus.Infof(LogFormat, "Event", resourceType, obj.Name, clusterId, "Skipping the namespace: "+obj.Namespace)
		return
	}

	if len(virtualService.Hosts) > 1 {
		logrus.Errorf(LogFormat, "Event", resourceType, obj.Name, clusterId, "Skipping as multiple hosts not supported for virtual service namespace="+obj.Namespace)
		return
	}

	dependentClusters := r.AdmiralCache.CnameDependentClusterCache.Get(virtualService.Hosts[0])

	if dependentClusters == nil {
		logrus.Infof(LogFormat, "Event", resourceType, obj.Name, clusterId, "No dependent clusters found")
		return
	}

	logrus.Infof(LogFormat, "Event", "VirtualService", obj.Name, clusterId, "Processing")

	for _, dependentCluster := range dependentClusters.Map() {

		rc := r.remoteControllers[dependentCluster]

		if clusterId != dependentCluster {

			if event == common.Delete {
				logrus.Infof(LogFormat, "Delete", "VirtualService", obj.Name, clusterId, "Success")
				rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(r.config.SyncNamespace).Delete(obj.Name, &v12.DeleteOptions{})

			} else {

				exist, _ := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(r.config.SyncNamespace).Get(obj.Name, v12.GetOptions{})

				//change destination host for all http routes <service_name>.<ns>. to same as host on the virtual service
				for _, httpRoute := range virtualService.Http {
					for _, destination := range httpRoute.Route {
						//get at index 0, we do not support wildcards or multiple hosts currently
						destination.Destination.Host = virtualService.Hosts[0]
					}
				}

				for _, tlsRoute := range virtualService.Tls {
					for _, destination := range tlsRoute.Route {
						//get at index 0, we do not support wildcards or multiple hosts currently
						destination.Destination.Host = virtualService.Hosts[0]
					}
				}

				addUpdateVirtualService(obj, exist, vh.RemoteRegistry.config.SyncNamespace, rc)
			}
		}

	}
}

func addUpdateVirtualService(obj *v1alpha3.VirtualService, exist *v1alpha3.VirtualService, namespace string, rc *RemoteController) {
	var err error
	var op string
	if exist == nil {
		obj.Namespace = namespace
		obj.ResourceVersion = ""
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Create(obj)
		op = "Add"
	} else {
		exist.Labels = obj.Labels
		exist.Annotations = obj.Annotations
		exist.Spec = obj.Spec
		op = "Update"
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Update(obj)
	}

	if err != nil {
		logrus.Infof(LogErrFormat, op, "VirtualService", obj.Name, rc.ClusterID, err)
	} else {
		logrus.Infof(LogErrFormat, op, "VirtualService", obj.Name, rc.ClusterID, "Success")
	}
}

func addUpdateServiceEntry(obj *v1alpha3.ServiceEntry, exist *v1alpha3.ServiceEntry, namespace string, rc *RemoteController) {
	var err error
	var op string
	if exist == nil {
		obj.Namespace = namespace
		obj.ResourceVersion = ""
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Create(obj)
		op = "Add"
	} else {
		exist.Labels = obj.Labels
		exist.Annotations = obj.Annotations
		exist.Spec = obj.Spec
		op = "Update"
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Update(obj)
	}

	if err != nil {
		logrus.Infof(LogErrFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, err)
	} else {
		logrus.Infof(LogErrFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, "Success")
	}
}

func addUpdateDestinationRule(obj *v1alpha3.DestinationRule, exist *v1alpha3.DestinationRule, namespace string, rc *RemoteController) {
	var err error
	var op string
	if exist == nil {
		obj.Namespace = namespace
		obj.ResourceVersion = ""
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Create(obj)
		op = "Add"
	} else {
		exist.Labels = obj.Labels
		exist.Annotations = obj.Annotations
		exist.Spec = obj.Spec
		op = "Update"
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Update(obj)
	}

	if err != nil {
		logrus.Infof(LogErrFormat, op, "DestinationRule", obj.Name, rc.ClusterID, err)
	} else {
		logrus.Infof(LogErrFormat, op, "DestinationRule", obj.Name, rc.ClusterID, "Success")
	}
}

func createVirtualServiceSkeletion(vs networking.VirtualService, name string, namespace string) *v1alpha3.VirtualService {
	return &v1alpha3.VirtualService{Spec:vs, ObjectMeta: v12.ObjectMeta{Name:name, Namespace: namespace}}
}

func createServiceEntrySkeletion(se networking.ServiceEntry, name string, namespace string) *v1alpha3.ServiceEntry {
	return &v1alpha3.ServiceEntry{Spec:se, ObjectMeta: v12.ObjectMeta{Name:name, Namespace: namespace}}
}

func createDestinationRulSkeletion(dr networking.DestinationRule, name string, namespace string) *v1alpha3.DestinationRule {
	return &v1alpha3.DestinationRule{Spec:dr, ObjectMeta: v12.ObjectMeta{Name:name, Namespace: namespace}}
}

func createServiceEntryForNewServiceOrPod(namespace string, sourceIdentity string, remoteRegistry *RemoteRegistry) {
	//create a service entry, destination rule and virtual service in the local cluster
	sourceServices := make(map[string]*k8sV1.Service)

	sourceDeployments := make(map[string]*k8sAppsV1.Deployment)

	var serviceEntries = make(map[string]*networking.ServiceEntry)

	var cname string

	for _, rc := range remoteRegistry.remoteControllers {

		deployment := rc.DeploymentController.Cache.Get(sourceIdentity)

		if deployment == nil || deployment.Deployments[namespace] == nil {
			continue
		}

		deploymentInstance := deployment.Deployments[namespace]

		serviceInstance := getServiceForDeployment(rc, deploymentInstance[0], namespace)

		if serviceInstance == nil {
			continue
		}

		cname = common.GetCname(deploymentInstance[0], "identity")

		remoteRegistry.AdmiralCache.IdentityClusterCache.Put(sourceIdentity, rc.ClusterID, rc.ClusterID)
		remoteRegistry.AdmiralCache.CnameClusterCache.Put(cname, rc.ClusterID, rc.ClusterID)
		remoteRegistry.AdmiralCache.CnameIdentityCache.Store(cname, sourceIdentity)
		sourceServices[rc.ClusterID] = serviceInstance

		sourceDeployments[rc.ClusterID] = deploymentInstance[0]

		createServiceEntry("identity", rc, remoteRegistry.config, remoteRegistry.AdmiralCache, deploymentInstance[0], serviceEntries)

	}

	dependents := remoteRegistry.AdmiralCache.IdentityDependencyCache.Get(sourceIdentity)

	dependentClusters := getDependentClusters(dependents, remoteRegistry.AdmiralCache.IdentityClusterCache, sourceServices)

	//update cname dependent cluster cache
	for clusterId, _ := range dependentClusters {
		remoteRegistry.AdmiralCache.CnameDependentClusterCache.Put(cname, clusterId, clusterId)
	}

	addServiceEntriesWithDr(remoteRegistry, dependentClusters, remoteRegistry.remoteControllers, serviceEntries)

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
					addServiceEntriesWithDr(remoteRegistry, map[string]string{sourceCluster: sourceCluster}, remoteRegistry.remoteControllers,
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
			//	logrus.Errorf(LogErrFormat, "Create", istioModel.VirtualService.Type, virtualServiceName, rc.ClusterID, err)
			//}
		}
	}
}

func getServiceForDeployment(rc *RemoteController, deployment *k8sAppsV1.Deployment, namespace string) *k8sV1.Service {

	cachedService := rc.ServiceController.Cache.Get(namespace)

	if cachedService == nil {
		return nil
	}
	var matchedService *k8sV1.Service
	for _, service := range cachedService.Service[namespace] {
		var match = true
		for lkey, lvalue := range service.Spec.Selector {
			value, ok := deployment.Spec.Selector.MatchLabels[lkey]
			if !ok || value != lvalue {
				match = false
				break
			}
		}
		//make sure the service matches the deployment Selector and also has a mesh port in the port spec
		if match {
			ports := GetMeshPorts(rc.ClusterID, service, deployment)
			if len(ports) > 0 {
				matchedService = service
				break
			}
		}
	}
	return matchedService
}

func getDependentClusters(dependents *common.Map, identityClusterCache *common.MapOfMaps, sourceServices map[string]*k8sV1.Service) map[string]string {
	var dependentClusters = make(map[string]string)
	//TODO optimize this map construction
	if dependents != nil {
		for identity, clusters := range identityClusterCache.Map() {
			for depIdentity, _ := range dependents.Map() {
				if identity == depIdentity {
					for _, clusterId := range clusters.Map() {
						_, ok := sourceServices[clusterId]
						if !ok {
							dependentClusters[clusterId] = clusterId
						}
					}
				}
			}
		}
	}
	return dependentClusters
}

func (sc *ServiceHandler) Deleted(obj *k8sV1.Service) {

}

func createDestinationRuleForLocal(remoteController *RemoteController, localDrName string, identityId string, clusterId string,
	destinationRule *networking.DestinationRule, syncNamespace string) {

	deployment := remoteController.DeploymentController.Cache.Get(identityId)

	if deployment == nil || len(deployment.Deployments) == 0 {
		logrus.Errorf(LogFormat, "Find", "deployment", identityId, remoteController.ClusterID, "Couldn't find deployment with identity")
		return
	}

	//TODO this will pull a random deployment from some cluster which might not be the right deployment
	var deploymentInstance *k8sAppsV1.Deployment
	for _, value := range deployment.Deployments {
		deploymentInstance = value[0]
		break
	}

	serviceInstance := getServiceForDeployment(remoteController, deploymentInstance, deploymentInstance.Namespace)

	cname := common.GetCname(deploymentInstance, "identity")
	if cname == destinationRule.Host {
		destinationRule.Host = serviceInstance.Name + common.Sep + serviceInstance.Namespace + common.DotLocalDomainSuffix
		existsDestinationRule, err := remoteController.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(localDrName, v12.GetOptions{})
		if err != nil {
			logrus.Warnf(LogErrFormat, "Find", "DestinationRule", localDrName, clusterId, err)
		}
		newDestinationRule := createDestinationRulSkeletion(*destinationRule, localDrName, syncNamespace)


		if newDestinationRule != nil {
			addUpdateDestinationRule(newDestinationRule, existsDestinationRule, syncNamespace, remoteController)
		}
	}
}

func createSeWithDrLabels(remoteController *RemoteController, localCluster bool, identityId string, seName string, se *networking.ServiceEntry,
	dr *networking.DestinationRule, seAddressMap *common.Map) map[string]*networking.ServiceEntry {
	var allSes = make(map[string]*networking.ServiceEntry)
	var newSe = copyServiceEntry(se)

	newSe.Addresses = []string{common.GetLocalAddressForSe(seName, seAddressMap)}

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

func copyEndpoint(e *networking.ServiceEntry_Endpoint) *networking.ServiceEntry_Endpoint {
	labels := make(map[string]string)
	util.MapCopy(labels, e.Labels)
	ports := make(map[string]uint32)
	util.MapCopy(ports, e.Ports)
	return &networking.ServiceEntry_Endpoint{Address: e.Address, Ports: ports, Locality: e.Locality, Labels: labels}
}

func copyServiceEntry(se *networking.ServiceEntry) *networking.ServiceEntry {
	return &networking.ServiceEntry{Ports: se.Ports, Resolution: se.Resolution, Hosts: se.Hosts, Location: se.Location,
		SubjectAltNames: se.SubjectAltNames, ExportTo: se.ExportTo, Endpoints: se.Endpoints, Addresses: se.Addresses}
}