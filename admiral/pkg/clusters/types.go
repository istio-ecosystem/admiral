package clusters

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	log "github.com/sirupsen/logrus"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	k8s "k8s.io/client-go/kubernetes"
)

type ServiceEntrySuspender interface {
	SuspendUpdate(identity string, environment string) bool
}

type IgnoredIdentityCache struct {
	RWLock                 *sync.RWMutex
	Enabled                bool                `json:"enabled"`
	All                    bool                `json:"all"`
	ClusterEnvironment     string              `json:"clusterEnvironment"`
	EnvironmentsByIdentity map[string][]string `json:"environmentsByIdentities"`
}

type RemoteController struct {
	ClusterID                 string
	ApiServer                 string
	StartTime                 time.Time
	GlobalTraffic             *admiral.GlobalTrafficController
	DeploymentController      *admiral.DeploymentController
	ServiceController         *admiral.ServiceController
	NodeController            *admiral.NodeController
	ServiceEntryController    *istio.ServiceEntryController
	DestinationRuleController *istio.DestinationRuleController
	VirtualServiceController  *istio.VirtualServiceController
	SidecarController         *istio.SidecarController
	RolloutController         *admiral.RolloutController
	RoutingPolicyController   *admiral.RoutingPolicyController
	stop                      chan struct{}
	//listener for normal types
}

type AdmiralCache struct {
	CnameClusterCache                  *common.MapOfMaps
	CnameDependentClusterCache         *common.MapOfMaps
	CnameIdentityCache                 *sync.Map
	IdentityClusterCache               *common.MapOfMaps
	WorkloadSelectorCache              *common.MapOfMaps
	ClusterLocalityCache               *common.MapOfMaps
	IdentityDependencyCache            *common.MapOfMaps
	SubsetServiceEntryIdentityCache    *sync.Map
	ServiceEntryAddressStore           *ServiceEntryAddressStore
	ConfigMapController                admiral.ConfigMapControllerInterface //todo this should be in the remotecontrollers map once we expand it to have one configmap per cluster
	GlobalTrafficCache                 *globalTrafficCache                  //The cache needs to live in the handler because it needs access to deployments
	DependencyNamespaceCache           *common.SidecarEgressMap
	SeClusterCache                     *common.MapOfMaps
	RoutingPolicyFilterCache           *routingPolicyFilterCache
	RoutingPolicyCache                 *routingPolicyCache
	DependencyProxyVirtualServiceCache *dependencyProxyVirtualServiceCache
	SourceToDestinations               *sourceToDestinations //This cache is to fetch list of all dependencies for a given source identity
	argoRolloutsEnabled                bool
}

type RemoteRegistry struct {
	sync.Mutex
	remoteControllers           map[string]*RemoteController
	SecretController            *secret.Controller
	secretClient                k8s.Interface
	ctx                         context.Context
	AdmiralCache                *AdmiralCache
	StartTime                   time.Time
	ServiceEntryUpdateSuspender ServiceEntrySuspender
	ExcludedIdentityMap         map[string]bool
}

func NewRemoteRegistry(ctx context.Context, params common.AdmiralParams) *RemoteRegistry {
	var serviceEntryUpdateSuspender ServiceEntrySuspender
	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.mutex = &sync.Mutex{}
	rpFilterCache := &routingPolicyFilterCache{}
	rpFilterCache.filterCache = make(map[string]map[string]map[string]string)
	rpFilterCache.mutex = &sync.Mutex{}
	rpCache := &routingPolicyCache{}
	rpCache.identityCache = make(map[string]*v1.RoutingPolicy)
	rpCache.mutex = &sync.Mutex{}
	admiralCache := &AdmiralCache{
		IdentityClusterCache:            common.NewMapOfMaps(),
		CnameClusterCache:               common.NewMapOfMaps(),
		CnameDependentClusterCache:      common.NewMapOfMaps(),
		ClusterLocalityCache:            common.NewMapOfMaps(),
		IdentityDependencyCache:         common.NewMapOfMaps(),
		WorkloadSelectorCache:           common.NewMapOfMaps(),
		RoutingPolicyFilterCache:        rpFilterCache,
		RoutingPolicyCache:              rpCache,
		DependencyNamespaceCache:        common.NewSidecarEgressMap(),
		CnameIdentityCache:              &sync.Map{},
		SubsetServiceEntryIdentityCache: &sync.Map{},
		ServiceEntryAddressStore:        &ServiceEntryAddressStore{EntryAddresses: map[string]string{}, Addresses: []string{}},
		GlobalTrafficCache:              gtpCache,
		SeClusterCache:                  common.NewMapOfMaps(),
		argoRolloutsEnabled:             params.ArgoRolloutsEnabled,
		DependencyProxyVirtualServiceCache: &dependencyProxyVirtualServiceCache{
			identityVSCache: make(map[string]map[string]*v1alpha3.VirtualService),
			mutex:           &sync.Mutex{},
		},
		SourceToDestinations: &sourceToDestinations{
			sourceDestinations: make(map[string][]string),
			mutex:              &sync.Mutex{},
		},
	}
	if common.GetSecretResolver() == "" {
		serviceEntryUpdateSuspender = NewDefaultServiceEntrySuspender(params.ExcludedIdentityList)
	} else {
		serviceEntryUpdateSuspender = NewDummyServiceEntrySuspender()
	}
	return &RemoteRegistry{
		ctx:                         ctx,
		StartTime:                   time.Now(),
		remoteControllers:           make(map[string]*RemoteController),
		AdmiralCache:                admiralCache,
		ServiceEntryUpdateSuspender: serviceEntryUpdateSuspender,
	}
}

type sourceToDestinations struct {
	sourceDestinations map[string][]string
	mutex              *sync.Mutex
}

func (d *sourceToDestinations) put(dependencyObj *v1.Dependency) {
	if dependencyObj.Spec.Source == "" {
		return
	}
	if dependencyObj.Spec.Destinations == nil {
		return
	}
	if len(dependencyObj.Spec.Destinations) <= 0 {
		return
	}
	d.mutex.Lock()
	d.sourceDestinations[dependencyObj.Spec.Source] = dependencyObj.Spec.Destinations
	defer d.mutex.Unlock()
}

func (d *sourceToDestinations) Get(key string) []string {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.sourceDestinations[key]
}

func (r *RemoteRegistry) GetRemoteController(clusterId string) *RemoteController {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	return r.remoteControllers[clusterId]
}

func (r *RemoteRegistry) PutRemoteController(clusterId string, rc *RemoteController) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	r.remoteControllers[clusterId] = rc
}

func (r *RemoteRegistry) DeleteRemoteController(clusterId string) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	delete(r.remoteControllers, clusterId)
}

func (r *RemoteRegistry) RangeRemoteControllers(fn func(k string, v *RemoteController)) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	for k, v := range r.remoteControllers {
		fn(k, v)
	}
}

func (r *RemoteRegistry) GetClusterIds() []string {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	var clusters = make([]string, 0, len(r.remoteControllers))
	for k := range r.remoteControllers {
		clusters = append(clusters, k)
	}
	return clusters
}

func (r *RemoteRegistry) shutdown() {

	done := r.ctx.Done()
	//wait for the context to close
	<-done

	//close the remote controllers stop channel
	for _, v := range r.remoteControllers {
		close(v.stop)
	}
}

type ServiceEntryAddressStore struct {
	EntryAddresses map[string]string `yaml:"entry-addresses,omitempty"`
	Addresses      []string          `yaml:"addresses,omitempty"` //trading space for efficiency - this will give a quick way to validate that the address is unique
}

type DependencyHandler struct {
	RemoteRegistry *RemoteRegistry
	DepController  *admiral.DependencyController
}

type DependencyProxyHandler struct {
	RemoteRegistry                          *RemoteRegistry
	DepController                           *admiral.DependencyProxyController
	dependencyProxyDefaultHostNameGenerator DependencyProxyDefaultHostNameGenerator
}

type GlobalTrafficHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type RolloutHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type globalTrafficCache struct {
	//map of global traffic policies key=environment.identity, value: GlobalTrafficPolicy object
	identityCache map[string]*v1.GlobalTrafficPolicy

	mutex *sync.Mutex
}

func (g *globalTrafficCache) GetFromIdentity(identity string, environment string) *v1.GlobalTrafficPolicy {
	return g.identityCache[common.ConstructGtpKey(environment, identity)]
}

func (g *globalTrafficCache) Put(gtp *v1.GlobalTrafficPolicy) error {
	if gtp.Name == "" {
		//no GTP, throw error
		return errors.New("cannot add an empty globaltrafficpolicy to the cache")
	}
	defer g.mutex.Unlock()
	g.mutex.Lock()
	var gtpIdentity = gtp.Labels[common.GetGlobalTrafficDeploymentLabel()]
	var gtpEnv = common.GetGtpEnv(gtp)

	log.Infof("adding GTP with name %v to GTP cache. LabelMatch=%v env=%v", gtp.Name, gtpIdentity, gtpEnv)
	identity := gtp.Labels[common.GetGlobalTrafficDeploymentLabel()]
	key := common.ConstructGtpKey(gtpEnv, identity)
	g.identityCache[key] = gtp
	return nil
}

func (g *globalTrafficCache) Delete(identity string, environment string) {
	key := common.ConstructGtpKey(environment, identity)
	if _, ok := g.identityCache[key]; ok {
		log.Infof("deleting gtp with key=%s from global GTP cache", key)
		delete(g.identityCache, key)
	}
}

type RoutingPolicyHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type routingPolicyCache struct {
	// map of routing policies key=environment.identity, value: RoutingPolicy object
	// only one routing policy per identity + env is allowed
	identityCache map[string]*v1.RoutingPolicy
	mutex         *sync.Mutex
}

func (r *routingPolicyCache) Delete(identity string, environment string) {
	defer r.mutex.Unlock()
	r.mutex.Lock()
	key := common.ConstructRoutingPolicyKey(environment, identity)
	if _, ok := r.identityCache[key]; ok {
		log.Infof("deleting RoutingPolicy with key=%s from global RoutingPolicy cache", key)
		delete(r.identityCache, key)
	}
}

func (r *routingPolicyCache) GetFromIdentity(identity string, environment string) *v1.RoutingPolicy {
	defer r.mutex.Unlock()
	r.mutex.Lock()
	return r.identityCache[common.ConstructRoutingPolicyKey(environment, identity)]
}

func (r *routingPolicyCache) Put(rp *v1.RoutingPolicy) error {
	if rp == nil || rp.Name == "" {
		// no RoutingPolicy, throw error
		return errors.New("cannot add an empty RoutingPolicy to the cache")
	}
	if rp.Labels == nil {
		return errors.New("labels empty in RoutingPolicy")
	}
	defer r.mutex.Unlock()
	r.mutex.Lock()
	var rpIdentity = rp.Labels[common.GetRoutingPolicyLabel()]
	var rpEnv = common.GetRoutingPolicyEnv(rp)

	log.Infof("Adding RoutingPolicy with name %v to RoutingPolicy cache. LabelMatch=%v env=%v", rp.Name, rpIdentity, rpEnv)
	key := common.ConstructRoutingPolicyKey(rpEnv, rpIdentity)
	r.identityCache[key] = rp

	return nil
}

type routingPolicyFilterCache struct {
	// map of envoyFilters key=environment+identity of the routingPolicy, value is a map [clusterId -> map [filterName -> filterName]]
	filterCache map[string]map[string]map[string]string
	mutex       *sync.Mutex
}

func (r *routingPolicyFilterCache) Get(identityEnvKey string) (filters map[string]map[string]string) {
	defer r.mutex.Unlock()
	r.mutex.Lock()
	return r.filterCache[identityEnvKey]
}

func (r *routingPolicyFilterCache) Put(identityEnvKey string, clusterId string, filterName string) {
	defer r.mutex.Unlock()
	r.mutex.Lock()
	if r.filterCache[identityEnvKey] == nil {
		r.filterCache[identityEnvKey] = make(map[string]map[string]string)
	}

	if r.filterCache[identityEnvKey][clusterId] == nil {
		r.filterCache[identityEnvKey][clusterId] = make(map[string]string)
	}
	r.filterCache[identityEnvKey][clusterId][filterName] = filterName
}

func (r *routingPolicyFilterCache) Delete(identityEnvKey string) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, admiral.Delete, "routingpolicy", identityEnvKey, "", "skipping read-only mode")
		return
	}
	if common.GetEnableRoutingPolicy() {
		defer r.mutex.Unlock()
		r.mutex.Lock()
		// delete all envoyFilters for a given identity+env key
		delete(r.filterCache, identityEnvKey)
	} else {
		log.Infof(LogFormat, admiral.Delete, "routingpolicy", identityEnvKey, "", "routingpolicy disabled")
	}
}
func (r RoutingPolicyHandler) Added(ctx context.Context, obj *v1.RoutingPolicy) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, admiral.Add, "routingpolicy", "", "", "skipping read-only mode")
		return
	}
	if common.GetEnableRoutingPolicy() {
		if common.ShouldIgnoreResource(obj.ObjectMeta) {
			log.Infof(LogFormat, "success", "routingpolicy", obj.Name, "", "Ignored the RoutingPolicy because of the annotation")
			return
		}
		dependents := getDependents(obj, r)
		if len(dependents) == 0 {
			log.Info("No dependents found for Routing Policy - ", obj.Name)
			return
		}
		r.processroutingPolicy(ctx, dependents, obj, admiral.Add)

		log.Infof(LogFormat, admiral.Add, "routingpolicy", obj.Name, "", "finished processing routing policy")
	} else {
		log.Infof(LogFormat, admiral.Add, "routingpolicy", obj.Name, "", "routingpolicy disabled")
	}
}

func (r RoutingPolicyHandler) processroutingPolicy(ctx context.Context, dependents map[string]string, routingPolicy *v1.RoutingPolicy, eventType admiral.EventType) {
	for _, remoteController := range r.RemoteRegistry.remoteControllers {
		for _, dependent := range dependents {

			// Check if the dependent exists in this remoteCluster. If so, we create an envoyFilter with dependent identity as workload selector
			if _, ok := r.RemoteRegistry.AdmiralCache.IdentityClusterCache.Get(dependent).Copy()[remoteController.ClusterID]; ok {
				selectors := r.RemoteRegistry.AdmiralCache.WorkloadSelectorCache.Get(dependent + remoteController.ClusterID).Copy()
				if len(selectors) != 0 {

					filter, err := createOrUpdateEnvoyFilter(ctx, remoteController, routingPolicy, eventType, dependent, r.RemoteRegistry.AdmiralCache, selectors)
					if err != nil {
						// Best effort create
						log.Errorf(LogErrFormat, eventType, "routingpolicy", routingPolicy.Name, remoteController.ClusterID, err)
					} else {
						log.Infof("msg=%s name=%s cluster=%s", "created envoyfilter", filter.Name, remoteController.ClusterID)
					}
				}
			}
		}

	}
}

func (r RoutingPolicyHandler) Updated(ctx context.Context, obj *v1.RoutingPolicy) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, admiral.Update, "routingpolicy", "", "", "skipping read-only mode")
		return
	}
	if common.GetEnableRoutingPolicy() {
		if common.ShouldIgnoreResource(obj.ObjectMeta) {
			log.Infof(LogFormat, admiral.Update, "routingpolicy", obj.Name, "", "Ignored the RoutingPolicy because of the annotation")
			// We need to process this as a delete event.
			r.Deleted(ctx, obj)
			return
		}
		dependents := getDependents(obj, r)
		if len(dependents) == 0 {
			return
		}
		r.processroutingPolicy(ctx, dependents, obj, admiral.Update)

		log.Infof(LogFormat, admiral.Update, "routingpolicy", obj.Name, "", "updated routing policy")
	} else {
		log.Infof(LogFormat, admiral.Update, "routingpolicy", obj.Name, "", "routingpolicy disabled")
	}
}

// getDependents - Returns the client dependents for the destination service with routing policy
// Returns a list of asset ID's of the client services or nil if no dependents are found
func getDependents(obj *v1.RoutingPolicy, r RoutingPolicyHandler) map[string]string {
	sourceIdentity := common.GetRoutingPolicyIdentity(obj)
	if len(sourceIdentity) == 0 {
		err := errors.New("identity label is missing")
		log.Warnf(LogErrFormat, "add", "RoutingPolicy", obj.Name, r.ClusterID, err)
		return nil
	}

	dependents := r.RemoteRegistry.AdmiralCache.IdentityDependencyCache.Get(sourceIdentity).Copy()
	return dependents
}

func (r RoutingPolicyHandler) Deleted(ctx context.Context, obj *v1.RoutingPolicy) {
	dependents := getDependents(obj, r)
	if len(dependents) != 0 {
		r.deleteEnvoyFilters(ctx, dependents, obj, admiral.Delete)
		log.Infof(LogFormat, admiral.Delete, "routingpolicy", obj.Name, "", "deleted envoy filter for routing policy")
	}
}

func (r RoutingPolicyHandler) deleteEnvoyFilters(ctx context.Context, dependents map[string]string, obj *v1.RoutingPolicy, eventType admiral.EventType) {
	for _, dependent := range dependents {
		key := dependent + common.GetRoutingPolicyEnv(obj)
		clusterIdFilterMap := r.RemoteRegistry.AdmiralCache.RoutingPolicyFilterCache.Get(key)
		for _, rc := range r.RemoteRegistry.remoteControllers {
			if filterMap, ok := clusterIdFilterMap[rc.ClusterID]; ok {
				for _, filter := range filterMap {
					log.Infof(LogFormat, eventType, "envoyfilter", filter, rc.ClusterID, "deleting")
					err := rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").Delete(ctx, filter, metaV1.DeleteOptions{})
					if err != nil {
						// Best effort delete
						log.Errorf(LogErrFormat, eventType, "envoyfilter", filter, rc.ClusterID, err)
					} else {
						log.Infof(LogFormat, eventType, "envoyfilter", filter, rc.ClusterID, "deleting from cache")
						r.RemoteRegistry.AdmiralCache.RoutingPolicyFilterCache.Delete(key)
					}
				}
			}
		}
	}
}

type DeploymentHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type NodeHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type ServiceHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

func (sh *ServiceHandler) Added(ctx context.Context, obj *k8sV1.Service) {
	log.Infof(LogFormat, "Added", "service", obj.Name, sh.ClusterID, "received")
	err := HandleEventForService(ctx, obj, sh.RemoteRegistry, sh.ClusterID)
	if err != nil {
		log.Errorf(LogErrFormat, "Error", "service", obj.Name, sh.ClusterID, err)
	}
}

func (sh *ServiceHandler) Updated(ctx context.Context, obj *k8sV1.Service) {
	log.Infof(LogFormat, "Updated", "service", obj.Name, sh.ClusterID, "received")
	err := HandleEventForService(ctx, obj, sh.RemoteRegistry, sh.ClusterID)
	if err != nil {
		log.Errorf(LogErrFormat, "Error", "service", obj.Name, sh.ClusterID, err)
	}
}

func (sh *ServiceHandler) Deleted(ctx context.Context, obj *k8sV1.Service) {
	log.Infof(LogFormat, "Deleted", "service", obj.Name, sh.ClusterID, "received")
	err := HandleEventForService(ctx, obj, sh.RemoteRegistry, sh.ClusterID)
	if err != nil {
		log.Errorf(LogErrFormat, "Error", "service", obj.Name, sh.ClusterID, err)
	}
}

func HandleEventForService(ctx context.Context, svc *k8sV1.Service, remoteRegistry *RemoteRegistry, clusterName string) error {
	if svc.Spec.Selector == nil {
		return fmt.Errorf("selector missing on service=%s in namespace=%s cluster=%s", svc.Name, svc.Namespace, clusterName)
	}
	rc := remoteRegistry.GetRemoteController(clusterName)
	if rc == nil {
		return fmt.Errorf("could not find the remote controller for cluster=%s", clusterName)
	}
	deploymentController := rc.DeploymentController
	rolloutController := rc.RolloutController
	if deploymentController != nil {
		matchingDeployments := deploymentController.GetDeploymentBySelectorInNamespace(ctx, svc.Spec.Selector, svc.Namespace)
		if len(matchingDeployments) > 0 {
			for _, deployment := range matchingDeployments {
				HandleEventForDeployment(ctx, admiral.Update, &deployment, remoteRegistry, clusterName)
			}
		}
	}
	if common.GetAdmiralParams().ArgoRolloutsEnabled && rolloutController != nil {
		matchingRollouts := rolloutController.GetRolloutBySelectorInNamespace(ctx, svc.Spec.Selector, svc.Namespace)

		if len(matchingRollouts) > 0 {
			for _, rollout := range matchingRollouts {
				HandleEventForRollout(ctx, admiral.Update, &rollout, remoteRegistry, clusterName)
			}
		}
	}
	return nil
}

func (dh *DependencyHandler) Added(ctx context.Context, obj *v1.Dependency) {

	log.Infof(LogFormat, "Add", "dependency-record", obj.Name, "", "Received=true namespace="+obj.Namespace)

	HandleDependencyRecord(ctx, obj, dh.RemoteRegistry)

}

func (dh *DependencyHandler) Updated(ctx context.Context, obj *v1.Dependency) {

	log.Infof(LogFormat, "Update", "dependency-record", obj.Name, "", "Received=true namespace="+obj.Namespace)

	// need clean up before handle it as added, I need to handle update that delete the dependency, find diff first
	// this is more complex cos want to make sure no other service depend on the same service (which we just removed the dependancy).
	// need to make sure nothing depend on that before cleaning up the SE for that service
	HandleDependencyRecord(ctx, obj, dh.RemoteRegistry)

}

func HandleDependencyRecord(ctx context.Context, obj *v1.Dependency, remoteRegitry *RemoteRegistry) {
	sourceIdentity := obj.Spec.Source

	if len(sourceIdentity) == 0 {
		log.Infof(LogFormat, "Event", "dependency-record", obj.Name, "", "No identity found namespace="+obj.Namespace)
	}

	updateIdentityDependencyCache(sourceIdentity, remoteRegitry.AdmiralCache.IdentityDependencyCache, obj)

	remoteRegitry.AdmiralCache.SourceToDestinations.put(obj)

}

func (dh *DependencyHandler) Deleted(ctx context.Context, obj *v1.Dependency) {
	// special case of update, delete the dependency crd file for one service, need to loop through all ones we plan to update
	// and make sure nobody else is relying on the same SE in same cluster
	log.Infof(LogFormat, "Deleted", "dependency", obj.Name, "", "Skipping, not implemented")
}

func (gtp *GlobalTrafficHandler) Added(ctx context.Context, obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Added", "globaltrafficpolicy", obj.Name, gtp.ClusterID, "received")
	err := HandleEventForGlobalTrafficPolicy(ctx, admiral.Add, obj, gtp.RemoteRegistry, gtp.ClusterID)
	if err != nil {
		log.Infof(err.Error())
	}
}

func (gtp *GlobalTrafficHandler) Updated(ctx context.Context, obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Updated", "globaltrafficpolicy", obj.Name, gtp.ClusterID, "received")
	err := HandleEventForGlobalTrafficPolicy(ctx, admiral.Update, obj, gtp.RemoteRegistry, gtp.ClusterID)
	if err != nil {
		log.Infof(err.Error())
	}
}

func (gtp *GlobalTrafficHandler) Deleted(ctx context.Context, obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Deleted", "globaltrafficpolicy", obj.Name, gtp.ClusterID, "received")
	err := HandleEventForGlobalTrafficPolicy(ctx, admiral.Delete, obj, gtp.RemoteRegistry, gtp.ClusterID)
	if err != nil {
		log.Infof(err.Error())
	}
}

func (pc *DeploymentHandler) Added(ctx context.Context, obj *k8sAppsV1.Deployment) {
	HandleEventForDeployment(ctx, admiral.Add, obj, pc.RemoteRegistry, pc.ClusterID)
}

func (pc *DeploymentHandler) Deleted(ctx context.Context, obj *k8sAppsV1.Deployment) {
	HandleEventForDeployment(ctx, admiral.Delete, obj, pc.RemoteRegistry, pc.ClusterID)
}

func (rh *RolloutHandler) Added(ctx context.Context, obj *argo.Rollout) {
	HandleEventForRollout(ctx, admiral.Add, obj, rh.RemoteRegistry, rh.ClusterID)
}

func (rh *RolloutHandler) Updated(ctx context.Context, obj *argo.Rollout) {
	log.Infof(LogFormat, "Updated", "rollout", obj.Name, rh.ClusterID, "received")
}

func (rh *RolloutHandler) Deleted(ctx context.Context, obj *argo.Rollout) {
	HandleEventForRollout(ctx, admiral.Delete, obj, rh.RemoteRegistry, rh.ClusterID)
}

// HandleEventForRollout helper function to handle add and delete for RolloutHandler
func HandleEventForRollout(ctx context.Context, event admiral.EventType, obj *argo.Rollout, remoteRegistry *RemoteRegistry, clusterName string) {

	log.Infof(LogFormat, event, "rollout", obj.Name, clusterName, "Received")
	globalIdentifier := common.GetRolloutGlobalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		log.Infof(LogFormat, "Event", "rollout", obj.Name, clusterName, "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return
	}

	env := common.GetEnvForRollout(obj)

	// Use the same function as added deployment function to update and put new service entry in place to replace old one
	modifyServiceEntryForNewServiceOrPod(ctx, event, env, globalIdentifier, remoteRegistry)
}

// helper function to handle add and delete for DeploymentHandler
func HandleEventForDeployment(ctx context.Context, event admiral.EventType, obj *k8sAppsV1.Deployment, remoteRegistry *RemoteRegistry, clusterName string) {

	log.Infof(LogFormat, event, "deployment", obj.Name, clusterName, "Received")
	globalIdentifier := common.GetDeploymentGlobalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		log.Infof(LogFormat, "Event", "deployment", obj.Name, clusterName, "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return
	}

	env := common.GetEnv(obj)

	// Use the same function as added deployment function to update and put new service entry in place to replace old one
	modifyServiceEntryForNewServiceOrPod(ctx, event, env, globalIdentifier, remoteRegistry)
}

// HandleEventForGlobalTrafficPolicy processes all the events related to GTPs
func HandleEventForGlobalTrafficPolicy(ctx context.Context, event admiral.EventType, gtp *v1.GlobalTrafficPolicy,
	remoteRegistry *RemoteRegistry, clusterName string) error {

	globalIdentifier := common.GetGtpIdentity(gtp)

	if len(globalIdentifier) == 0 {
		return fmt.Errorf(LogFormat, "Event", "globaltrafficpolicy", gtp.Name, clusterName, "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+gtp.Namespace)
	}

	env := common.GetGtpEnv(gtp)

	// For now we're going to force all the events to update only in order to prevent
	// the endpoints from being deleted.
	// TODO: Need to come up with a way to prevent deleting default endpoints so that this hack can be removed.
	// Use the same function as added deployment function to update and put new service entry in place to replace old one
	modifyServiceEntryForNewServiceOrPod(ctx, admiral.Update, env, globalIdentifier, remoteRegistry)
	return nil
}

func (dh *DependencyProxyHandler) Added(ctx context.Context, obj *v1.DependencyProxy) {
	log.Infof(LogFormat, "Add", "dependencyproxy", obj.Name, "", "Received=true namespace="+obj.Namespace)
	err := updateIdentityDependencyProxyCache(ctx, dh.RemoteRegistry.AdmiralCache.DependencyProxyVirtualServiceCache, obj, dh.dependencyProxyDefaultHostNameGenerator)
	if err != nil {
		log.Errorf(LogErrFormat, "Add", "dependencyproxy", obj.Name, "", err)
	}
}

func (dh *DependencyProxyHandler) Updated(ctx context.Context, obj *v1.DependencyProxy) {
	log.Infof(LogFormat, "Update", "dependencyproxy", obj.Name, "", "Received=true namespace="+obj.Namespace)
	err := updateIdentityDependencyProxyCache(ctx, dh.RemoteRegistry.AdmiralCache.DependencyProxyVirtualServiceCache, obj, dh.dependencyProxyDefaultHostNameGenerator)
	if err != nil {
		log.Errorf(LogErrFormat, "Add", "dependencyproxy", obj.Name, "", err)
	}
}

func (dh *DependencyProxyHandler) Deleted(ctx context.Context, obj *v1.DependencyProxy) {
	log.Infof(LogFormat, "Deleted", "dependencyproxy", obj.Name, "", "Skipping, not implemented")
}
