package clusters

import (
	"fmt"
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/gogo/protobuf/types"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	log "github.com/sirupsen/logrus"
	networking "istio.io/api/networking/v1alpha3"
	v1alpha32 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

const ROLLOUT_POD_HASH_LABEL string = "rollouts-pod-template-hash"

type ServiceEntryHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type DestinationRuleHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type VirtualServiceHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type SidecarHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

func updateIdentityDependencyCache(sourceIdentity string, identityDependencyCache *common.MapOfMaps, dr *v1.Dependency) {
	for _, dIdentity := range dr.Spec.Destinations {
		identityDependencyCache.Put(dIdentity, sourceIdentity, sourceIdentity)
	}
	log.Infof(LogFormat, "Update", "dependency-cache", dr.Name, "", "Updated=true namespace="+dr.Namespace)
}

func handleDependencyRecord(sourceIdentity string, r *RemoteRegistry, rcs map[string]*RemoteController, obj *v1.Dependency) {

	destinationIdentitys := obj.Spec.Destinations

	destinationClusters := make(map[string]string)

	sourceClusters := make(map[string]string)

	var serviceEntries = make(map[string]*v1alpha32.ServiceEntry)

	for _, rc := range rcs {

		//for every cluster the source identity is running, add their istio ingress as service entry address
		tempDeployment := rc.DeploymentController.Cache.Get(sourceIdentity)
		//If there is  no deployment, check if a rollout is available.
		tempRollout := rc.RolloutController.Cache.Get(sourceIdentity)
		if tempDeployment != nil || tempRollout != nil {
			sourceClusters[rc.ClusterID] = rc.ClusterID
			r.AdmiralCache.IdentityClusterCache.Put(sourceIdentity, rc.ClusterID, rc.ClusterID)
		}

		//create and store destination service entries
		for _, destinationCluster := range destinationIdentitys {
			destDeployment := rc.DeploymentController.Cache.Get(destinationCluster)
			destRollout := rc.RolloutController.Cache.Get(destinationCluster)
			var tmpSe *networking.ServiceEntry

			// Assumption :- We will have either a deployment or  a rollout mapped to an identity but never both in a cluster
			if destDeployment != nil {
				//deployment can be in multiple clusters, create SEs for all clusters

				for _, deployment := range destDeployment.Deployments {

					r.AdmiralCache.IdentityClusterCache.Put(destinationCluster, rc.ClusterID, rc.ClusterID)

					deployments := rc.DeploymentController.Cache.Get(destinationCluster)

					if deployments == nil || len(deployments.Deployments) == 0 {
						continue
					}

					serviceInstance := getServiceForDeployment(rc, deployment)

					if serviceInstance == nil {
						continue
					}

					meshPorts := GetMeshPorts(rc.ClusterID, serviceInstance, deployment)

					tmpSe = createServiceEntry(admiral.Add, rc, r.AdmiralCache, meshPorts, deployment, serviceEntries)

					if tmpSe == nil {
						continue
					}
				}
			} else if destRollout != nil {
				//rollouts can be in multiple clusters, create SEs for all clusters

				for _, rollout := range destRollout.Rollouts {

					r.AdmiralCache.IdentityClusterCache.Put(destinationCluster, rc.ClusterID, rc.ClusterID)

					rollouts := rc.RolloutController.Cache.Get(destinationCluster)

					if rollouts == nil || len(rollouts.Rollouts) == 0 {
						continue
					}

					serviceInstance := getServiceForRollout(rc, rollout)

					if serviceInstance == nil {
						continue
					}

					meshPorts := GetMeshPortsForRollout(rc.ClusterID, serviceInstance, rollout)

					tmpSe = createServiceEntryForRollout(admiral.Add, rc, r.AdmiralCache, meshPorts, rollout, serviceEntries)

					if tmpSe == nil {
						continue
					}
				}
			}
			if tmpSe == nil {
				continue
			}
			destinationClusters[rc.ClusterID] = tmpSe.Hosts[0] //Only single host supported

			r.AdmiralCache.CnameIdentityCache.Store(tmpSe.Hosts[0], destinationCluster)

			serviceEntries[tmpSe.Hosts[0]] = tmpSe
		}
	}

	if len(sourceClusters) == 0 || len(serviceEntries) == 0 {
		log.Infof(LogFormat, "Event", "dependency-record", sourceIdentity, "", "skipped")
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
	AddServiceEntriesWithDr(r.AdmiralCache, sourceClusters, rcs, serviceEntries)
}

func getIstioResourceName(host string, suffix string) string {
	return strings.ToLower(host) + suffix
}

func makeIngressOnlyVirtualService(host string, destination string, port uint32) *v1alpha32.VirtualService {
	return makeVirtualService(host, []string{common.MulticlusterIngressGateway}, destination, port)
}

func makeVirtualService(host string, gateways []string, destination string, port uint32) *v1alpha32.VirtualService {
	return &v1alpha32.VirtualService{Hosts: []string{host},
		Gateways: gateways,
		ExportTo: []string{"*"},
		Http:     []*v1alpha32.HTTPRoute{{Route: []*v1alpha32.HTTPRouteDestination{{Destination: &v1alpha32.Destination{Host: destination, Port: &v1alpha32.PortSelector{Number: port}}}}}}}
}

func getDestinationRule(host string, locality string, gtpTrafficPolicy *model.TrafficPolicy) *v1alpha32.DestinationRule {
	var dr = &v1alpha32.DestinationRule{}
	dr.Host = host
	dr.TrafficPolicy = &v1alpha32.TrafficPolicy{Tls: &v1alpha32.TLSSettings{Mode: v1alpha32.TLSSettings_ISTIO_MUTUAL}}
	processGtp := true
	if len(locality) == 0 {
		log.Warnf(LogErrFormat, "Process", "GlobalTrafficPolicy", host, "", "Skipping gtp processing, locality of the cluster nodes cannot be determined. Is this minikube?")
		processGtp = false
	}
	outlierDetection := &v1alpha32.OutlierDetection{
		BaseEjectionTime:  &types.Duration{Seconds: 120},
		ConsecutiveErrors: int32(10),
		Interval:          &types.Duration{Seconds: 5},
	}
	if gtpTrafficPolicy != nil && processGtp {
		var loadBalancerSettings = &v1alpha32.LoadBalancerSettings{
			LbPolicy: &v1alpha32.LoadBalancerSettings_Simple{Simple: v1alpha32.LoadBalancerSettings_ROUND_ROBIN},
		}

		if len(gtpTrafficPolicy.Target) > 0 {
			var localityLbSettings = &v1alpha32.LocalityLoadBalancerSetting{}

			if gtpTrafficPolicy.LbType == model.TrafficPolicy_FAILOVER {
				distribute := make([]*v1alpha32.LocalityLoadBalancerSetting_Distribute, 0)
				targetTrafficMap := make(map[string]uint32)
				for _, tg := range gtpTrafficPolicy.Target {
					//skip 0 values from GTP as that's implicit for locality settings
					if tg.Weight != int32(0) {
						targetTrafficMap[tg.Region] = uint32(tg.Weight)
					}
				}
				distribute = append(distribute, &v1alpha32.LocalityLoadBalancerSetting_Distribute{
					From: locality + "/*",
					To:   targetTrafficMap,
				})
				localityLbSettings.Distribute = distribute
			}
			// else default behavior
			loadBalancerSettings.LocalityLbSetting = localityLbSettings
			dr.TrafficPolicy.LoadBalancer = loadBalancerSettings
		}
	}
	dr.TrafficPolicy.OutlierDetection = outlierDetection
	return dr
}

func (se *ServiceEntryHandler) Added(obj *v1alpha3.ServiceEntry) {
	if IgnoreIstioResource(obj.Spec.ExportTo) {
		return
	}
}

func (se *ServiceEntryHandler) Updated(obj *v1alpha3.ServiceEntry) {
	if IgnoreIstioResource(obj.Spec.ExportTo) {
		return
	}
}

func (se *ServiceEntryHandler) Deleted(obj *v1alpha3.ServiceEntry) {
	if IgnoreIstioResource(obj.Spec.ExportTo) {
		return
	}
}

func (dh *DestinationRuleHandler) Added(obj *v1alpha3.DestinationRule) {
	if IgnoreIstioResource(obj.Spec.ExportTo) {
		return
	}
	handleDestinationRuleEvent(obj, dh, common.Add, common.DestinationRule)
}

func (dh *DestinationRuleHandler) Updated(obj *v1alpha3.DestinationRule) {
	if IgnoreIstioResource(obj.Spec.ExportTo) {
		return
	}
	handleDestinationRuleEvent(obj, dh, common.Update, common.DestinationRule)
}

func (dh *DestinationRuleHandler) Deleted(obj *v1alpha3.DestinationRule) {
	if IgnoreIstioResource(obj.Spec.ExportTo) {
		return
	}
	handleDestinationRuleEvent(obj, dh, common.Delete, common.DestinationRule)
}

func (vh *VirtualServiceHandler) Added(obj *v1alpha3.VirtualService) {
	if IgnoreIstioResource(obj.Spec.ExportTo) {
		return
	}
	err := handleVirtualServiceEvent(obj, vh, common.Add, common.VirtualService)
	if err != nil {
		log.Error(err)
	}
}

func (vh *VirtualServiceHandler) Updated(obj *v1alpha3.VirtualService) {
	if IgnoreIstioResource(obj.Spec.ExportTo) {
		return
	}
	err := handleVirtualServiceEvent(obj, vh, common.Update, common.VirtualService)
	if err != nil {
		log.Error(err)
	}
}

func (vh *VirtualServiceHandler) Deleted(obj *v1alpha3.VirtualService) {
	if IgnoreIstioResource(obj.Spec.ExportTo) {
		return
	}
	err := handleVirtualServiceEvent(obj, vh, common.Delete, common.VirtualService)
	if err != nil {
		log.Error(err)
	}
}

func (dh *SidecarHandler) Added(obj *v1alpha3.Sidecar) {}

func (dh *SidecarHandler) Updated(obj *v1alpha3.Sidecar) {}

func (dh *SidecarHandler) Deleted(obj *v1alpha3.Sidecar) {}

func IgnoreIstioResource(exportTo []string) bool {
	if len(exportTo) == 0 {
		return false
	} else {
		for _, namespace := range exportTo {
			if namespace == "*" {
				return false
			}
		}
	}
	return true
}

func handleDestinationRuleEvent(obj *v1alpha3.DestinationRule, dh *DestinationRuleHandler, event common.Event, resourceType common.ResourceType) {
	destinationRule := obj.Spec

	clusterId := dh.ClusterID

	localDrName := obj.Name + "-local"

	var localIdentityId string

	syncNamespace := common.GetSyncNamespace()

	r := dh.RemoteRegistry

	if obj.Namespace == syncNamespace || obj.Namespace == common.NamespaceKubeSystem {
		log.Infof(LogFormat, "Event", "DestinationRule", obj.Name, clusterId, "Skipping the namespace: "+obj.Namespace)
		return
	}

	dependentClusters := r.AdmiralCache.CnameDependentClusterCache.Get(destinationRule.Host)

	if dependentClusters == nil {
		log.Infof("Skipping event: %s from cluster %s for %v", "DestinationRule", clusterId, destinationRule)
		log.Infof(LogFormat, "Event", "DestinationRule", obj.Name, clusterId, "No dependent clusters found")
		return
	}

	log.Infof(LogFormat, "Event", "DestinationRule", obj.Name, clusterId, "Processing")

	//Create label based service entry in source and dependent clusters for subset routing to work
	host := destinationRule.Host

	basicSEName := getIstioResourceName(host, "-se")

	seName := getIstioResourceName(obj.Name, "-se")

	allDependentClusters := make(map[string]string)

	util.MapCopy(allDependentClusters, dependentClusters.Map())

	allDependentClusters[clusterId] = clusterId

	for _, dependentCluster := range allDependentClusters {

		rc := r.RemoteControllers[dependentCluster]

		var newServiceEntry *v1alpha3.ServiceEntry

		var existsServiceEntry *v1alpha3.ServiceEntry

		var drServiceEntries = make(map[string]*v1alpha32.ServiceEntry)

		exist, err := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(syncNamespace).Get(basicSEName, v12.GetOptions{})

		var identityId = ""

		if exist == nil || err != nil {

			log.Warnf(LogFormat, "Find", "ServiceEntry", basicSEName, dependentCluster, "Failed")

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

			err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Delete(obj.Name, &v12.DeleteOptions{})
			if err != nil {
				log.Infof(LogFormat, "Delete", "DestinationRule", obj.Name, clusterId, "success")
			} else {
				log.Error(LogFormat, err)
			}
			err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(syncNamespace).Delete(seName, &v12.DeleteOptions{})
			if err != nil {
				log.Infof(LogFormat, "Delete", "ServiceEntry", seName, clusterId, "success")
			} else {
				log.Error(LogFormat, err)
			}
			for _, subset := range destinationRule.Subsets {
				sseName := seName + common.Dash + subset.Name
				err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(syncNamespace).Delete(sseName, &v12.DeleteOptions{})
				if err != nil {
					log.Infof(LogFormat, "Delete", "ServiceEntry", sseName, clusterId, "success")
				} else {
					log.Error(LogFormat, err)
				}
			}
			err = rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Delete(localDrName, &v12.DeleteOptions{})
			if err != nil {
				log.Infof(LogFormat, "Delete", "DestinationRule", localDrName, clusterId, "success")
			} else {
				log.Error(LogFormat, err)
			}

		} else {

			exist, _ := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(obj.Name, v12.GetOptions{})

			//copy destination rule only to other clusters
			if dependentCluster != clusterId {
				addUpdateDestinationRule(obj, exist, syncNamespace, rc)
			}

			for _seName, se := range drServiceEntries {
				existsServiceEntry, _ = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(syncNamespace).Get(_seName, v12.GetOptions{})
				newServiceEntry = createServiceEntrySkeletion(*se, _seName, syncNamespace)
				if err != nil {
					log.Warnf(LogErrFormat, "Create", "ServiceEntry", seName, clusterId, err)
				}
				if newServiceEntry != nil {
					addUpdateServiceEntry(newServiceEntry, existsServiceEntry, syncNamespace, rc)
					r.AdmiralCache.SeClusterCache.Put(newServiceEntry.Spec.Hosts[0], rc.ClusterID, rc.ClusterID)
				}
				//cache the subset service entries for updating them later for pod events
				if dependentCluster == clusterId && se.Resolution == v1alpha32.ServiceEntry_STATIC {
					r.AdmiralCache.SubsetServiceEntryIdentityCache.Store(identityId, map[string]string{_seName: clusterId})
				}
			}

			if dependentCluster == clusterId {
				//we need a destination rule with local fqdn for destination rules created with cnames to work in local cluster
				createDestinationRuleForLocal(rc, localDrName, localIdentityId, clusterId, &destinationRule)
			}

		}
	}
}

func createDestinationRuleForLocal(remoteController *RemoteController, localDrName string, identityId string, clusterId string,
	destinationRule *v1alpha32.DestinationRule) {

	deployment := remoteController.DeploymentController.Cache.Get(identityId)

	if deployment == nil || len(deployment.Deployments) == 0 {
		log.Errorf(LogFormat, "Find", "deployment", identityId, remoteController.ClusterID, "Couldn't find deployment with identity")
		return
	}

	//TODO this will pull a random deployment from some cluster which might not be the right deployment
	var deploymentInstance *k8sAppsV1.Deployment
	for _, value := range deployment.Deployments {
		deploymentInstance = value
		break
	}

	syncNamespace := common.GetSyncNamespace()
	serviceInstance := getServiceForDeployment(remoteController, deploymentInstance)

	cname := common.GetCname(deploymentInstance, common.GetHostnameSuffix(), common.GetWorkloadIdentifier())
	if cname == destinationRule.Host {
		destinationRule.Host = serviceInstance.Name + common.Sep + serviceInstance.Namespace + common.DotLocalDomainSuffix
		existsDestinationRule, err := remoteController.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(localDrName, v12.GetOptions{})
		if err != nil {
			log.Warnf(LogErrFormat, "Find", "DestinationRule", localDrName, clusterId, err)
		}
		newDestinationRule := createDestinationRuleSkeletion(*destinationRule, localDrName, syncNamespace)

		if newDestinationRule != nil {
			addUpdateDestinationRule(newDestinationRule, existsDestinationRule, syncNamespace, remoteController)
		}
	}
}

func handleVirtualServiceEvent(obj *v1alpha3.VirtualService, vh *VirtualServiceHandler, event common.Event, resourceType common.ResourceType) error {

	log.Infof(LogFormat, "Event", resourceType, obj.Name, vh.ClusterID, "Received event")

	virtualService := obj.Spec

	clusterId := vh.ClusterID

	r := vh.RemoteRegistry

	syncNamespace := common.GetSyncNamespace()

	if obj.Namespace == syncNamespace {
		log.Infof(LogFormat, "Event", resourceType, obj.Name, clusterId, "Skipping the namespace: "+obj.Namespace)
		return nil
	}

	if len(virtualService.Hosts) > 1 {
		log.Errorf(LogFormat, "Event", resourceType, obj.Name, clusterId, "Skipping as multiple hosts not supported for virtual service namespace="+obj.Namespace)
		return nil
	}

	dependentClusters := r.AdmiralCache.CnameDependentClusterCache.Get(virtualService.Hosts[0])

	if dependentClusters == nil {
		log.Infof(LogFormat, "Event", resourceType, obj.Name, clusterId, "No dependent clusters found")
		return nil
	}

	log.Infof(LogFormat, "Event", "VirtualService", obj.Name, clusterId, "Processing")

	for _, dependentCluster := range dependentClusters.Map() {

		rc := r.RemoteControllers[dependentCluster]

		if clusterId != dependentCluster {

			if event == common.Delete {
				log.Infof(LogFormat, "Delete", "VirtualService", obj.Name, clusterId, "Success")
				err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Delete(obj.Name, &v12.DeleteOptions{})
				if err != nil {
					return err
				}

			} else {

				exist, _ := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(obj.Name, v12.GetOptions{})

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

				addUpdateVirtualService(obj, exist, syncNamespace, rc)
			}
		}

	}
	return nil
}

func addUpdateVirtualService(obj *v1alpha3.VirtualService, exist *v1alpha3.VirtualService, namespace string, rc *RemoteController) {
	var err error
	var op string
	if exist == nil || len(exist.Spec.Hosts) == 0 {
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
		log.Errorf(LogErrFormat, op, "VirtualService", obj.Name, rc.ClusterID, err)
	} else {
		log.Infof(LogFormat, op, "VirtualService", obj.Name, rc.ClusterID, "Success")
	}
}

func deleteVirtualService(exist *v1alpha3.VirtualService, namespace string, rc *RemoteController) {
	if exist != nil {
		err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Delete(exist.Name, &v12.DeleteOptions{})
		if err != nil {
			log.Errorf(LogErrFormat, "Delete", "VirtualService", exist.Name, rc.ClusterID, err)
		} else {
			log.Infof(LogFormat, "Delete", "VirtualService", exist.Name, rc.ClusterID, "Success")
		}
	}
}
func addUpdateServiceEntry(obj *v1alpha3.ServiceEntry, exist *v1alpha3.ServiceEntry, namespace string, rc *RemoteController) {
	var err error
	var op string
	if exist == nil || exist.Spec.Hosts == nil {
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
		log.Errorf(LogErrFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, err)
	} else {
		log.Infof(LogFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, "Success")
	}
}

func deleteServiceEntry(exist *v1alpha3.ServiceEntry, namespace string, rc *RemoteController) {
	if exist != nil {
		err := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Delete(exist.Name, &v12.DeleteOptions{})
		if err != nil {
			log.Errorf(LogErrFormat, "Delete", "ServiceEntry", exist.Name, rc.ClusterID, err)
		} else {
			log.Infof(LogFormat, "Delete", "ServiceEntry", exist.Name, rc.ClusterID, "Success")
		}
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
		log.Errorf(LogErrFormat, op, "DestinationRule", obj.Name, rc.ClusterID, err)
	} else {
		log.Infof(LogFormat, op, "DestinationRule", obj.Name, rc.ClusterID, "Success")
	}
}

func deleteDestinationRule(exist *v1alpha3.DestinationRule, namespace string, rc *RemoteController) {
	if exist != nil {
		err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Delete(exist.Name, &v12.DeleteOptions{})
		if err != nil {
			log.Errorf(LogErrFormat, "Delete", "DestinationRule", exist.Name, rc.ClusterID, err)
		} else {
			log.Infof(LogFormat, "Delete", "DestinationRule", exist.Name, rc.ClusterID, "Success")
		}
	}
}
func createServiceEntrySkeletion(se v1alpha32.ServiceEntry, name string, namespace string) *v1alpha3.ServiceEntry {
	return &v1alpha3.ServiceEntry{Spec: se, ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace}}
}

func createSidecarSkeletion(sidecar v1alpha32.Sidecar, name string, namespace string) *v1alpha3.Sidecar {
	return &v1alpha3.Sidecar{Spec: sidecar, ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace}}
}

func createDestinationRuleSkeletion(dr v1alpha32.DestinationRule, name string, namespace string) *v1alpha3.DestinationRule {
	return &v1alpha3.DestinationRule{Spec: dr, ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace}}
}

func createVirtualServiceSkeletion(se v1alpha32.VirtualService, name string, namespace string) *v1alpha3.VirtualService {
	return &v1alpha3.VirtualService{Spec: se, ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace}}
}

func getServiceForDeployment(rc *RemoteController, deployment *k8sAppsV1.Deployment) *k8sV1.Service {

	if deployment == nil {
		return nil
	}

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
			for depIdentity := range dependents.Map() {
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

// A rollout can use one of 2 stratergies :-
// 1. Canary stratergy- this contains only one service instance
// 2. Blue green stratergy- this contains 2 service instances in a namespace, an active service and a preview service. Admiral should always use the active service
func getServiceForRollout(rc *RemoteController, rollout *argo.Rollout) *k8sV1.Service {

	if rollout == nil {
		return nil
	}
	cachedService := rc.ServiceController.Cache.Get(rollout.Namespace)

	if cachedService == nil {
		return nil
	}
	rolloutStrategy := rollout.Spec.Strategy

	if rolloutStrategy.BlueGreen == nil && rolloutStrategy.Canary == nil {
		return nil
	}

	var blueGreenActiveService string
	if rolloutStrategy.BlueGreen != nil {
		// If rollout uses blue green strategy, use the active service
		blueGreenActiveService = rolloutStrategy.BlueGreen.ActiveService
	}
	var matchedService *k8sV1.Service
	for _, service := range cachedService.Service[rollout.Namespace] {
		var match = true
		// Both active and passive service have similar label selector. Use active service name to filter in case of blue green stratergy
		if len(blueGreenActiveService) > 0 && service.ObjectMeta.Name != blueGreenActiveService {
			continue
		}
		for lkey, lvalue := range service.Spec.Selector {
			// Rollouts controller adds a dynamic label with name rollouts-pod-template-hash to both active and passive replicasets.
			// This dynamic label is not available on the rollout template. Hence ignoring the label with name rollouts-pod-template-hash
			if lkey == ROLLOUT_POD_HASH_LABEL {
				continue
			}
			value, ok := rollout.Spec.Selector.MatchLabels[lkey]
			if !ok || value != lvalue {
				match = false
				break
			}
		}
		//make sure the service matches the rollout Selector and also has a mesh port in the port spec
		if match {
			ports := GetMeshPortsForRollout(rc.ClusterID, service, rollout)
			if len(ports) > 0 {
				matchedService = service
				break
			}
		}
	}
	return matchedService
}
