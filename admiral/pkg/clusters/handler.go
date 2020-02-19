package clusters

import (
	"fmt"
	"github.com/gogo/protobuf/types"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/sirupsen/logrus"
	v1alpha32 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"

	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"

)

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

func updateIdentityDependencyCache(sourceIdentity string, identityDependencyCache *common.MapOfMaps, dr *v1.Dependency) {
	for _, dIdentity := range dr.Spec.Destinations {
		identityDependencyCache.Put(dIdentity, sourceIdentity, sourceIdentity)
	}
	logrus.Infof(LogFormat, "Update", "dependency-cache", dr.Name, "", "Updated=true namespace="+dr.Namespace)
}

func handleDependencyRecord(sourceIdentity string, r *RemoteRegistry, rcs map[string]*RemoteController, config AdmiralParams, obj *v1.Dependency) {

	destinationIdentitys := obj.Spec.Destinations

	destinationClusters := make(map[string]string)

	sourceClusters := make(map[string]string)

	var serviceEntries = make(map[string]*v1alpha32.ServiceEntry)

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
				tmpSe := createServiceEntry(rc, config, r.AdmiralCache, deployment[0], serviceEntries)

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
	AddServiceEntriesWithDr(r, sourceClusters, rcs, serviceEntries, config.SyncNamespace)
}

func getIstioResourceName(host string, suffix string) string {
	return strings.ToLower(host) + suffix
}

//TODO use selector on pod, instead of hardcoded identityId
func getMatchingGlobalTrafficPolicy(rc *RemoteController, identityId string) *v1.GlobalTrafficPolicy {
	return rc.GlobalTraffic.Cache.Get(identityId)
}

func makeVirtualService(host string, destination string, port uint32) *v1alpha32.VirtualService {
	return &v1alpha32.VirtualService{Hosts: []string{host},
		Gateways: []string{common.MulticlusterIngressGateway},
		ExportTo: []string{"*"},
		Http:     []*v1alpha32.HTTPRoute{{Route: []*v1alpha32.HTTPRouteDestination{{Destination: &v1alpha32.Destination{Host: destination, Port: &v1alpha32.PortSelector{Number: port}}}}}}}
}

func getDestinationRule(host string, locality string, gtpWrapper *v1.GlobalTrafficPolicy) *v1alpha32.DestinationRule {
	var dr = &v1alpha32.DestinationRule{}
	dr.Host = host
	dr.TrafficPolicy = &v1alpha32.TrafficPolicy{Tls: &v1alpha32.TLSSettings{Mode: v1alpha32.TLSSettings_ISTIO_MUTUAL}}
	if gtpWrapper != nil {
		var loadBalancerSettings = &v1alpha32.LoadBalancerSettings{
			LbPolicy: &v1alpha32.LoadBalancerSettings_Simple{Simple: v1alpha32.LoadBalancerSettings_ROUND_ROBIN},
		}
		gtp := gtpWrapper.Spec
		gtpTrafficPolicy := gtp.Policy[0]
		if len(gtpTrafficPolicy.Target) > 0 {
			var localityLbSettings = &v1alpha32.LocalityLoadBalancerSetting{}
			if gtpTrafficPolicy.LbType == model.TrafficPolicy_FAILOVER {
				distribute := make([]*v1alpha32.LocalityLoadBalancerSetting_Distribute, 0)
				targetTrafficMap := make(map[string]uint32)
				for _, tg := range gtpTrafficPolicy.Target {
					targetTrafficMap[tg.Region] = uint32(tg.Weight)
				}
				distribute = append(distribute, &v1alpha32.LocalityLoadBalancerSetting_Distribute{
					From: locality + "/*",
					To:   targetTrafficMap,
				})
				localityLbSettings.Distribute = distribute
			} else {
				//this will have default behavior
			}
			loadBalancerSettings.LocalityLbSetting = localityLbSettings
			dr.TrafficPolicy.LoadBalancer = loadBalancerSettings
			dr.TrafficPolicy.OutlierDetection = &v1alpha32.OutlierDetection{
				BaseEjectionTime: &types.Duration{Seconds: 120},
				ConsecutiveErrors: 10,
				Interval: &types.Duration{Seconds: 60},
			}
		}
	}
	return dr
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

		var drServiceEntries = make(map[string]*v1alpha32.ServiceEntry)

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
				drServiceEntries = createSeWithDrLabels(rc, dependentCluster == clusterId, identityId, seName, &serviceEntry, &destinationRule, r.AdmiralCache.ServiceEntryAddressStore, r.AdmiralCache.ConfigMapController)
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
					if dependentCluster == clusterId && se.Resolution == v1alpha32.ServiceEntry_STATIC {
						r.AdmiralCache.SubsetServiceEntryIdentityCache.Store(identityId, map[string]string{_seName: clusterId})
					}
				}
			}

			if dependentCluster == clusterId {
				//we need a destination rule with local fqdn for destination rules created with cnames to work in local cluster
				createDestinationRuleForLocal(rc, localDrName, localIdentityId, clusterId, &destinationRule, r.config.SyncNamespace, r.config.HostnameSuffix, r.config.LabelSet.WorkloadIdentityLabel)
			}

		}
	}
}

func createDestinationRuleForLocal(remoteController *RemoteController, localDrName string, identityId string, clusterId string,
	destinationRule *v1alpha32.DestinationRule, syncNamespace string, nameSuffix string, identifier string) {

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

	serviceInstance := getServiceForDeployment(remoteController, deploymentInstance)

	cname := common.GetCname(deploymentInstance, identifier, nameSuffix)
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
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Update(exist)
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
		_, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Create(obj)
		op = "Add"
	} else {
		exist.Labels = obj.Labels
		exist.Annotations = obj.Annotations
		exist.Spec = obj.Spec
		op = "Update"
		_, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Update(exist)
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
		_, err = rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Create(obj)
		op = "Add"
	} else {
		exist.Labels = obj.Labels
		exist.Annotations = obj.Annotations
		exist.Spec = obj.Spec
		op = "Update"
		_, err = rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Update(exist)
	}

	if err != nil {
		logrus.Infof(LogErrFormat, op, "DestinationRule", obj.Name, rc.ClusterID, err)
	} else {
		logrus.Infof(LogErrFormat, op, "DestinationRule", obj.Name, rc.ClusterID, "Success")
	}
}

func createVirtualServiceSkeletion(vs v1alpha32.VirtualService, name string, namespace string) *v1alpha3.VirtualService {
	return &v1alpha3.VirtualService{Spec:vs, ObjectMeta: v12.ObjectMeta{Name:name, Namespace: namespace}}
}

func createServiceEntrySkeletion(se v1alpha32.ServiceEntry, name string, namespace string) *v1alpha3.ServiceEntry {
	return &v1alpha3.ServiceEntry{Spec:se, ObjectMeta: v12.ObjectMeta{Name:name, Namespace: namespace}}
}

func createDestinationRulSkeletion(dr v1alpha32.DestinationRule, name string, namespace string) *v1alpha3.DestinationRule {
	return &v1alpha3.DestinationRule{Spec:dr, ObjectMeta: v12.ObjectMeta{Name:name, Namespace: namespace}}
}

func getServiceForDeployment(rc *RemoteController, deployment *k8sAppsV1.Deployment) *k8sV1.Service {

	cachedService := rc.ServiceController.Cache.Get(deployment.Namespace)

	if cachedService == nil {
		return nil
	}
	var matchedService *k8sV1.Service
	for _, service := range cachedService.Service[deployment.Namespace] {
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

func copyEndpoint(e *v1alpha32.ServiceEntry_Endpoint) *v1alpha32.ServiceEntry_Endpoint {
	labels := make(map[string]string)
	util.MapCopy(labels, e.Labels)
	ports := make(map[string]uint32)
	util.MapCopy(ports, e.Ports)
	return &v1alpha32.ServiceEntry_Endpoint{Address: e.Address, Ports: ports, Locality: e.Locality, Labels: labels}
}