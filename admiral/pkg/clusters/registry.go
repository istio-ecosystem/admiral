package clusters

import (
	"context"
	"github.com/gogo/protobuf/proto"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"k8s.io/client-go/rest"
	"strings"
	"time"

	"fmt"

	"istio.io/istio/pkg/log"
	"sync"

	istioModel "istio.io/istio/pilot/pkg/model"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"

	networking "istio.io/api/networking/v1alpha3"
	istioKube "istio.io/istio/pilot/pkg/serviceregistry/kube"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LogFormat    = "op=%s type=%v name=%v cluster=%s message=%s"
	LogErrFormat = "op=%s type=%v name=%v cluster=%s, e=%v"
)

func updateIdentityDependencyCache(sourceIdentity string, identityDependencyCache *common.MapOfMaps, dr *v1.Dependency) {
	for _, dIdentity := range dr.Spec.Destinations {
		identityDependencyCache.Put(dIdentity, sourceIdentity, sourceIdentity)
	}
	log.Infof(LogFormat, "Update", "dependency-cache", dr.Name, "", "Updated=true namespace="+dr.Namespace)
}

func handleDependencyRecord(identifier string, sourceIdentity string, admiralCache *AdmiralCache, rcs map[string]*RemoteController, config AdmiralParams, obj *v1.Dependency) {

	destinationIdentitys := obj.Spec.Destinations

	destinationClusters := make(map[string]string)

	sourceClusters := make(map[string]string)

	var serviceEntries = make(map[string]*networking.ServiceEntry)

	for _, rc := range rcs {

		//for every cluster the source identity is running, add their istio ingress as service entry address
		tempDeployment := rc.DeploymentController.Cache.Get(sourceIdentity)
		if tempDeployment != nil {
			sourceClusters[rc.ClusterID] = rc.ClusterID
			admiralCache.IdentityClusterCache.Put(sourceIdentity, rc.ClusterID, rc.ClusterID)
		}

		//create and store destination service entries
		for _, destinationCluster := range destinationIdentitys {
			destDeployment := rc.DeploymentController.Cache.Get(destinationCluster)
			if destDeployment == nil {
				continue
			}

			//deployment can be in multiple clusters, create SEs for all clusters

			for _, deployment := range destDeployment.Deployments {

				admiralCache.IdentityClusterCache.Put(destinationCluster, rc.ClusterID, rc.ClusterID)

				deployments := rc.DeploymentController.Cache.Get(destinationCluster)

				if deployments == nil || len(deployments.Deployments) == 0 {
					continue
				}
				//TODO pass deployment

				tmpSe := createServiceEntry(rc, config, admiralCache, deployment[0], serviceEntries)

				if tmpSe == nil {
					continue
				}

				destinationClusters[rc.ClusterID] = tmpSe.Hosts[0] //Only single host supported

				admiralCache.CnameIdentityCache.Store(tmpSe.Hosts[0], destinationCluster)

				serviceEntries[tmpSe.Hosts[0]] = tmpSe
			}

		}
	}

	if len(sourceClusters) == 0 || len(serviceEntries) == 0 {
		log.Infof(LogFormat, "Event", "dependency-record", sourceIdentity, "", "skipped")
		return
	}

	for dCluster, globalFqdn := range destinationClusters {
		for _, sCluster := range sourceClusters {
			admiralCache.CnameClusterCache.Put(globalFqdn, dCluster, dCluster)
			admiralCache.CnameDependentClusterCache.Put(globalFqdn, sCluster, sCluster)
			//filter out the source clusters same as destinationClusters
			delete(sourceClusters, dCluster)
		}
	}

	//add service entries for all dependencies in source cluster
	AddServiceEntriesWithDr(admiralCache, sourceClusters, rcs, serviceEntries, config.SyncNamespace)
}

func getIstioResourceName(host string, suffix string) string {
	return strings.ToLower(host) + suffix
}


func createIstioConfig(schema istio.ProtoSchema, object proto.Message, name string, namespace string) (*istio.Config, error) {
	return istio.ConvertIstioType(schema, object, name, namespace)
}

func makeVirtualService(host string, destination string, port uint32) *networking.VirtualService {
	return &networking.VirtualService{Hosts: []string{host},
		Gateways: []string{common.Mesh, common.MulticlusterIngressGateway},
		ExportTo: []string{"*"},
		Http:     []*networking.HTTPRoute{{Route: []*networking.HTTPRouteDestination{{Destination: &networking.Destination{Host: destination, Port: &networking.PortSelector{Port: &networking.PortSelector_Number{Number: port}}}}}}}}
}


func getDestinationRule(host string) *networking.DestinationRule {
	return &networking.DestinationRule{Host: host,
		TrafficPolicy: &networking.TrafficPolicy{Tls: &networking.TLSSettings{Mode: networking.TLSSettings_ISTIO_MUTUAL}}}
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

func InitAdmiral(ctx context.Context, params AdmiralParams) (*RemoteRegistry, error) {

	log.Infof("Initializing Admiral with params: %v", params)

	if params.LabelSet != nil {
		common.OverrideDefaultWorkloadIdentifier(params.LabelSet.WorkloadIdentityKey)
	}

	w := RemoteRegistry{
		ctx: ctx,
	}

	wd := DependencyHandler{
		RemoteRegistry: &w,
	}

	var err error
	wd.DepController, err = admiral.NewDependencyController(ctx.Done(), &wd, params.KubeconfigPath, params.DependenciesNamespace, params.CacheRefreshDuration)
	if err != nil {
		return nil, fmt.Errorf(" Error with dependency controller init: %v", err)
	}

	w.config = params
	w.remoteControllers = make(map[string]*RemoteController)

	w.AdmiralCache = &AdmiralCache{
		IdentityClusterCache:            common.NewMapOfMaps(),
		CnameClusterCache:               common.NewMapOfMaps(),
		CnameDependentClusterCache:      common.NewMapOfMaps(),
		ClusterLocalityCache:            common.NewMapOfMaps(),
		IdentityDependencyCache:         common.NewMapOfMaps(),
		CnameIdentityCache:              &sync.Map{},
		SubsetServiceEntryIdentityCache: &sync.Map{},
		ServiceEntryAddressStore:	     &ServiceEntryAddressStore{EntryAddresses: map[string]string{}, Addresses: []string{},}}

	configMapController, err := admiral.NewConfigMapController(w.config.KubeconfigPath, w.config.SyncNamespace)
	if err != nil {
		return nil, fmt.Errorf(" Error with configmap controller init: %v", err)
	}
	w.AdmiralCache.ConfigMapController = configMapController
	loadServiceEntryCacheData(w.AdmiralCache.ConfigMapController, w.AdmiralCache)

	err = createSecretController(ctx, &w, params)
	if err != nil {
		return nil, fmt.Errorf(" Error with secret control init: %v", err)
	}

	go w.shutdown()

	return &w, nil
}

func createSecretController(ctx context.Context, w *RemoteRegistry, params AdmiralParams) error {
	var err error

	w.secretClient, err = admiral.K8sClientFromPath(params.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("could not create K8s client: %v", err)
	}

	err = secret.StartSecretController(w.secretClient,
		w.createCacheController,
		w.deleteCacheController,
		w.config.ClusterRegistriesNamespace,
		ctx, params.SecretResolver)

	if err != nil {
		return fmt.Errorf("could not start secret controller: %v", err)
	}

	return nil
}

func (r *RemoteRegistry) createCacheController(clientConfig *rest.Config, clusterID string, resyncPeriod time.Duration) error {

	opts := istioKube.ControllerOptions{
		WatchedNamespace: meta_v1.NamespaceAll,
		ResyncPeriod:     resyncPeriod,
		DomainSuffix:     "cluster.local",
	}

	stop := make(chan struct{})

	rc := RemoteController{
		stop:      stop,
		ClusterID: clusterID,
	}

	err := r.createIstioController(clientConfig, opts, &rc, stop, clusterID)

	if err != nil {
		return fmt.Errorf(" Error with Istio controller init: %v", err)
	}

	log.Infof("starting global traffic policy controller clusterID: %v", clusterID)
	rc.GlobalTraffic, err = admiral.NewGlobalTrafficController(stop, &GlobalTrafficHandler{RemoteRegistry: r}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with GlobalTrafficController controller init: %v", err)
	}

	log.Infof("starting deployment controller clusterID: %v", clusterID)
	rc.DeploymentController, err = admiral.NewDeploymentController(stop, &DeploymentHandler{RemoteRegistry: r}, clientConfig, resyncPeriod, r.config.LabelSet)

	if err != nil {
		return fmt.Errorf(" Error with DeploymentController controller init: %v", err)
	}

	log.Infof("starting pod controller clusterID: %v", clusterID)
	rc.PodController, err = admiral.NewPodController(stop, &PodHandler{RemoteRegistry: r}, clientConfig, resyncPeriod, r.config.LabelSet)

	if err != nil {
		return fmt.Errorf(" Error with PodController controller init: %v", err)
	}

	log.Infof("starting node controller clusterID: %v", clusterID)
	rc.NodeController, err = admiral.NewNodeController(stop, &NodeHandler{RemoteRegistry: r}, clientConfig)

	if err != nil {
		return fmt.Errorf(" Error with NodeController controller init: %v", err)
	}

	log.Infof("starting service controller clusterID: %v", clusterID)
	rc.ServiceController, err = admiral.NewServiceController(stop, &ServiceHandler{RemoteRegistry: r}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with ServiceController controller init: %v", err)
	}

	r.Lock()
	defer r.Unlock()
	r.remoteControllers[clusterID] = &rc

	log.Infof("Create Controller %s", clusterID)

	return nil
}

func (r *RemoteRegistry) createIstioController(clientConfig *rest.Config, opts istioKube.ControllerOptions, remoteController *RemoteController, stop <-chan struct{}, clusterID string) error {
	configClient, err := istio.NewClient(clientConfig, "", istio.IstioConfigTypes, opts.DomainSuffix)
	if err != nil {
		return fmt.Errorf("error creating istio client to the remote clusterId: %s error:%v", clusterID, err)
	}
	configStore := istio.NewController(configClient, opts)

	configStore.RegisterEventHandler(istioModel.VirtualService.Type, func(m istio.Config, e istio.Event) {
		virtualService := m.Spec.(*networking.VirtualService)

		clusterId := remoteController.ClusterID

		if m.Namespace == r.config.SyncNamespace {
			log.Infof(LogFormat, "Event", e.String(), m.Name, clusterId, "Skipping the namespace: "+m.Namespace)
			return
		}

		if len(virtualService.Hosts) > 1 {
			log.Errorf(LogFormat, "Event", e.String(), m.Name, clusterId, "Skipping as multiple hosts not supported for virtual service namespace="+m.Namespace)
			return
		}

		dependentClusters := r.AdmiralCache.CnameDependentClusterCache.Get(virtualService.Hosts[0])

		if dependentClusters == nil {
			log.Infof(LogFormat, "Event", e.String(), m.Name, clusterId, "No dependent clusters found")
			return
		}

		log.Infof(LogFormat, "Event", e.String(), m.Name, clusterId, "Processing")

		for _, dependentCluster := range dependentClusters.Map() {

			rc := r.remoteControllers[dependentCluster]

			if clusterId != dependentCluster {

				if istio.EventDelete == e {

					log.Infof(LogFormat, "Delete", istioModel.DestinationRule.Type, m.Name, clusterId, "Success")
					rc.IstioConfigStore.Delete(istioModel.DestinationRule.Type, m.Name, r.config.SyncNamespace)

				} else {

					exist := rc.IstioConfigStore.Get(istioModel.VirtualService.Type, m.Name, r.config.SyncNamespace)

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

					addUpdateIstioResource(rc, m, exist, istioModel.VirtualService.Type, r.config.SyncNamespace)
				}
			}

		}

	})

	configStore.RegisterEventHandler(istioModel.DestinationRule.Type, func(m istio.Config, e istio.Event) {
		destinationRule := m.Spec.(*networking.DestinationRule)

		clusterId := remoteController.ClusterID

		localDrName := m.Name + "-local"

		var localIdentityId string

		if m.Namespace == r.config.SyncNamespace || m.Namespace == common.NamespaceKubeSystem {
			log.Infof(LogFormat, "Event", e.String(), m.Name, clusterId, "Skipping the namespace: "+m.Namespace)
			return
		}

		dependentClusters := r.AdmiralCache.CnameDependentClusterCache.Get(destinationRule.Host)

		if dependentClusters == nil {
			log.Infof("Skipping event: %s from cluster %s for %v", e.String(), clusterId, destinationRule)
			log.Infof(LogFormat, "Event", e.String(), m.Name, clusterId, "No dependent clusters found")
			return
		}

		log.Infof(LogFormat, "Event", e.String(), m.Name, clusterId, "Processing")

		//Create label based service entry in source and dependent clusters for subset routing to work
		host := destinationRule.Host

		basicSEName := getIstioResourceName(host, "-se")

		seName := getIstioResourceName(m.Name, "-se")

		allDependentClusters := make(map[string]string)

		util.MapCopy(allDependentClusters, dependentClusters.Map())

		allDependentClusters[clusterId] = clusterId

		for _, dependentCluster := range allDependentClusters {

			rc := r.remoteControllers[dependentCluster]

			var newServiceEntry *istio.Config

			var existsServiceEntry *istio.Config

			var drServiceEntries = make(map[string]*networking.ServiceEntry)

			exist := rc.IstioConfigStore.Get(istioModel.ServiceEntry.Type, basicSEName, r.config.SyncNamespace)

			var identityId = ""

			if exist == nil {

				log.Warnf(LogFormat, "Find", istioModel.ServiceEntry.Type, basicSEName, clusterID, "Failed")

			} else {

				serviceEntry := exist.Spec.(*networking.ServiceEntry)

				identityRaw, ok := r.AdmiralCache.CnameIdentityCache.Load(serviceEntry.Hosts[0])

				if ok {
					identityId = fmt.Sprintf("%v", identityRaw)
					if dependentCluster == clusterId {
						localIdentityId = identityId
					}
					drServiceEntries = createSeWithDrLabels(remoteController, dependentCluster == clusterId, identityId, seName, serviceEntry, destinationRule, r.AdmiralCache.ServiceEntryAddressStore, r.AdmiralCache.ConfigMapController)
				}

			}

			if istio.EventDelete == e {

				rc.IstioConfigStore.Delete(istioModel.DestinationRule.Type, m.Name, r.config.SyncNamespace)
				log.Infof(LogFormat, "Delete", istioModel.ServiceEntry.Type, m.Name, clusterId, "success")
				rc.IstioConfigStore.Delete(istioModel.ServiceEntry.Type, seName, r.config.SyncNamespace)
				log.Infof(LogFormat, "Delete", istioModel.ServiceEntry.Type, seName, clusterId, "success")
				for _, subset := range destinationRule.Subsets {
					sseName := seName + common.Dash + subset.Name
					rc.IstioConfigStore.Delete(istioModel.ServiceEntry.Type, sseName, r.config.SyncNamespace)
					log.Infof(LogFormat, "Delete", istioModel.ServiceEntry.Type, sseName, clusterId, "success")
				}
				rc.IstioConfigStore.Delete(istioModel.DestinationRule.Type, localDrName, r.config.SyncNamespace)
				log.Infof(LogFormat, "Delete", istioModel.DestinationRule.Type, localDrName, clusterId, "success")

			} else {

				exist := rc.IstioConfigStore.Get(istioModel.DestinationRule.Type, m.Name, r.config.SyncNamespace)

				//copy destination rule only to other clusters
				if dependentCluster != clusterId {
					addUpdateIstioResource(rc, m, exist, istioModel.DestinationRule.Type, r.config.SyncNamespace)
				}

				if drServiceEntries != nil {
					for _seName, se := range drServiceEntries {
						existsServiceEntry = rc.IstioConfigStore.Get(istioModel.ServiceEntry.Type, _seName, r.config.SyncNamespace)
						newServiceEntry, err = createIstioConfig(istio.ServiceEntryProto, se, _seName, r.config.SyncNamespace)
						if err != nil {
							log.Warnf(LogErrFormat, "Create", istioModel.ServiceEntry.Type, seName, clusterId, err)
						}
						if newServiceEntry != nil {
							addUpdateIstioResource(rc, *newServiceEntry, existsServiceEntry, istioModel.ServiceEntry.Type, r.config.SyncNamespace)
						}
						//cache the subset service entries for updating them later for pod events
						if dependentCluster == clusterId && se.Resolution == networking.ServiceEntry_STATIC {
							r.AdmiralCache.SubsetServiceEntryIdentityCache.Store(identityId, map[string]string{_seName: clusterId})
						}
					}
				}

				if dependentCluster == clusterId {
					//we need a destination rule with local fqdn for destination rules created with cnames to work in local cluster
					createDestinationRuleForLocal(remoteController, localDrName, localIdentityId, clusterId, destinationRule, r.config.SyncNamespace, r.config.HostnameSuffix, r.config.LabelSet.WorkloadIdentityKey)
				}

			}
		}

	})

	remoteController.IstioConfigStore = configStore

	go configStore.Run(stop)
	return nil
}

func createDestinationRuleForLocal(remoteController *RemoteController, localDrName string, identityId string, clusterId string,
	destinationRule *networking.DestinationRule, syncNamespace string, nameSuffix string, identifier string) {

	deployment := remoteController.DeploymentController.Cache.Get(identityId)

	if deployment == nil || len(deployment.Deployments) == 0 {
		log.Errorf(LogFormat, "Find", "deployment", identityId, remoteController.ClusterID, "Couldn't find deployment with identity")
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
		existsDestinationRule := remoteController.IstioConfigStore.Get(istioModel.DestinationRule.Type, localDrName, syncNamespace)
		newDestinationRule, err := createIstioConfig(istio.DestinationRuleProto, destinationRule, localDrName, syncNamespace)

		if err != nil {
			log.Warnf(LogErrFormat, "Create", istioModel.DestinationRule.Type, localDrName, clusterId, err)
		}
		if newDestinationRule != nil {
			addUpdateIstioResource(remoteController, *newDestinationRule, existsDestinationRule, istioModel.DestinationRule.Type, syncNamespace)
		}
	}
}

func copyEndpoint(e *networking.ServiceEntry_Endpoint) *networking.ServiceEntry_Endpoint {
	labels := make(map[string]string)
	util.MapCopy(labels, e.Labels)
	ports := make(map[string]uint32)
	util.MapCopy(ports, e.Ports)
	return &networking.ServiceEntry_Endpoint{Address: e.Address, Ports: ports, Locality: e.Locality, Labels: labels}
}

func addUpdateIstioResource(rc *RemoteController, m istio.Config, exist *istio.Config, t string, syncNamespace string) {
	var e error
	var verb string
	if exist == nil {
		m.Namespace = syncNamespace
		m.ResourceVersion = ""
		verb = "Add"
		_, e = rc.IstioConfigStore.Create(m)

	} else {
		if len(exist.Labels) > 0 {
			if _, ok := exist.Labels["disable-update"]; ok {
				verb = "Skipped"
			}
		}

		if verb != "Skipped" {
			exist.Labels = m.Labels
			exist.Spec = m.Spec
			exist.Annotations = m.Annotations
			verb = "Update"
			_, e = rc.IstioConfigStore.Update(*exist)
		}
	}

	//TODO this should return an error
	if e != nil {
		log.Errorf(LogErrFormat, verb, t, m.Spec, rc.ClusterID, e)
	} else if verb != "Skipped" {
		log.Infof(LogFormat, verb, t, m.Name, rc.ClusterID, "success")
	} else {
		log.Infof(LogFormat, verb, t, m.Name, rc.ClusterID, "Found Skip flag")
	}
}

func (r *RemoteRegistry) deleteCacheController(clusterID string) error {

	controller, ok := r.remoteControllers[clusterID]

	if ok {
		close(controller.stop)
	}

	r.Lock()
	defer r.Unlock()
	delete(r.remoteControllers, clusterID)

	log.Infof(LogFormat, "Delete", "remote-controller", clusterID, clusterID, "success")
	return nil
}

