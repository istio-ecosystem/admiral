package clusters

import (
	"context"
	"errors"
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	log "github.com/sirupsen/logrus"
	k8sAppsV1 "k8s.io/api/apps/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"sync"
	"time"
)

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
	CnameClusterCache               *common.MapOfMaps
	CnameDependentClusterCache      *common.MapOfMaps
	CnameIdentityCache              *sync.Map
	IdentityClusterCache            *common.MapOfMaps
	ClusterLocalityCache            *common.MapOfMaps
	IdentityDependencyCache         *common.MapOfMaps
	SubsetServiceEntryIdentityCache *sync.Map
	ServiceEntryAddressStore        *ServiceEntryAddressStore
	ConfigMapController             admiral.ConfigMapControllerInterface //todo this should be in the remotecontrollers map once we expand it to have one configmap per cluster
	GlobalTrafficCache              *globalTrafficCache                  //The cache needs to live in the handler because it needs access to deployments
	DependencyNamespaceCache        *common.SidecarEgressMap
	SeClusterCache                  *common.MapOfMaps
	RoutingPolicyCache				*routingPolicyCache
	RoutingPolicyFilterCache		*routingPolicyFilterCache
	argoRolloutsEnabled bool
}

type RemoteRegistry struct {
	sync.Mutex
	RemoteControllers map[string]*RemoteController
	SecretController  *secret.Controller
	secretClient      k8s.Interface
	ctx               context.Context
	AdmiralCache      *AdmiralCache
}

func (r *RemoteRegistry) shutdown() {

	done := r.ctx.Done()
	//wait for the context to close
	<-done

	//close the remote controllers stop channel
	for _, v := range r.RemoteControllers {
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

type GlobalTrafficHandler struct {
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

	log.Infof("Adding GTP with name %v to GTP cache. LabelMatch=%v env=%v", gtp.Name, gtpIdentity, gtpEnv)
	identity := gtp.Labels[common.GetGlobalTrafficDeploymentLabel()]
	key := common.ConstructGtpKey(gtpEnv, identity)
	g.identityCache[key] = gtp

	return nil
}

func (g *globalTrafficCache) Delete(identity string, environment string) {
	key := common.ConstructGtpKey(environment, identity)
	if _, ok := g.identityCache[key]; ok {
		log.Infof("Deleting gtp with key=%s from global GTP cache", key)
		delete(g.identityCache, key)
	}
}

type RolloutHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type RoutingPolicyHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID	   string
}

type routingPolicyCache struct {
	//map of routing policies key=environment.identity, value: RoutingPolicy object
	// only one routing policy per identity + env is allowed
	identityCache map[string]*v1.RoutingPolicy

	//// map of dependent identity + env -> [] routingPolicy. There can be potentially multiple routingPolicies that can apply to a client.
	//dependentRpCache map[string][]*v1.RoutingPolicy

	mutex *sync.Mutex
}


func (r *routingPolicyCache ) GetFromIdentity(identity string, environment string) *v1.RoutingPolicy {
	return r.identityCache[common.ConstructRoutingPolicyKey(environment, identity)]
}

func (r *routingPolicyCache) Put(rp *v1.RoutingPolicy) error {
	if rp.Name == "" {
		//no RoutingPolicy, throw error
		return errors.New("cannot add an empty RoutingPolicy to the cache")
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

func (r *routingPolicyCache) Delete(identity string, environment string) {
	key := common.ConstructRoutingPolicyKey(environment, identity)
	if _, ok := r.identityCache[key]; ok {
		log.Infof("Deleting RoutingPolicy with key=%s from global RoutingPolicy cache", key)
		delete(r.identityCache, key)
	}
}

type routingPolicyFilterCache struct {
	// map of envoyFilters key=environment+identity of the routingPolicy, value is a map [clusterId -> map [filterName -> filterName]]
	filterCache map[string]map[string]map[string]string
	mutex *sync.Mutex
}

func (r *routingPolicyFilterCache) Get(identityEnvKey string) (filters map[string]map[string]string) {
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
	defer r.mutex.Unlock()
	r.mutex.Lock()
	// delete all envoyFilters for a given identity+env key
	delete(r.filterCache, identityEnvKey)
}
func (r RoutingPolicyHandler) Added(obj *v1.RoutingPolicy) {
	if common.ShouldIgnoreResource(obj.ObjectMeta) {
		log.Infof(LogErrFormat, "success", "routingpolicy", obj.Name, obj.ClusterName, "Ignored the RoutingPolicy because of the annotation")
		return
	}
	dependents, done := getDependents(obj, r)
	if done {
		return
	}
	r.processroutingPolicy(dependents, obj, admiral.Add)
	log.Info("Finished processing routing policy")
}

func (r RoutingPolicyHandler) processroutingPolicy(dependents map[string]string, routingPolicy *v1.RoutingPolicy, eventType admiral.EventType ) {
	for _, remoteController := range r.RemoteRegistry.RemoteControllers {

		for _, dependent := range dependents {

			// Check if the dependent exists in this remoteCluster. If so, we create an envoyFilter with dependent identity as workload selector
			if _, ok := r.RemoteRegistry.AdmiralCache.IdentityClusterCache.Get(dependent).Copy()[remoteController.ClusterID]; ok {

				filter, err := createOrUpdateEnvoyFilter(remoteController, routingPolicy, eventType, dependent, r.RemoteRegistry.AdmiralCache)
				if err != nil {
					log.Errorf(LogErrFormat, admiral.Add, "routingpolicy", routingPolicy.Name, remoteController.ClusterID, err)
				}
				log.Infof("msg=%s name=%s cluster=%s", "created envoyfilter", filter.Name, remoteController.ClusterID)
			}
		}

	}
}

func (r RoutingPolicyHandler) Updated(obj *v1.RoutingPolicy) {
	if common.ShouldIgnoreResource(obj.ObjectMeta) {
		log.Infof(LogFormat, admiral.Update, "routingpolicy", obj.Name, obj.ClusterName, "Ignored the RoutingPolicy because of the annotation")
		// We need to process this as a delete event.
		r.Deleted(obj)
		return
	}
	dependents, missingIdentityLabel := getDependents(obj, r)
	if missingIdentityLabel {
		return
	}
	r.processroutingPolicy(dependents, obj, admiral.Update)
	log.Info("Updated routing policy")
}

func getDependents(obj *v1.RoutingPolicy, r RoutingPolicyHandler) (map[string]string, bool) {
	sourceIdentity := common.GetRoutingPolicyIdentity(obj)
	if len(sourceIdentity) == 0 {
		err := errors.New("identity label is missing")
		log.Warnf(LogErrFormat, "add", "RoutingPolicy", obj.Name, r.ClusterID, err)
		return nil, true
	}

	dependents := r.RemoteRegistry.AdmiralCache.IdentityDependencyCache.Get(sourceIdentity).Copy()
	return dependents, false
}

func (r RoutingPolicyHandler) Deleted(obj *v1.RoutingPolicy) {
	log.Info("Deleted routing policy")
	dependents, missingIdentityLabel := getDependents(obj, r)
	if !missingIdentityLabel {
		 r.deleteEnvoyFilters(dependents, obj, admiral.Delete)
	}
}

func (r RoutingPolicyHandler) deleteEnvoyFilters(dependents map[string]string, obj *v1.RoutingPolicy, eventType admiral.EventType) {
	for _, dependent := range dependents {
		key := dependent + common.GetRoutingPolicyEnv(obj)
		clusterIdFilterMap := r.RemoteRegistry.AdmiralCache.RoutingPolicyFilterCache.Get(key)
		r.RemoteRegistry.AdmiralCache.RoutingPolicyFilterCache.Delete(key)
		for _, rc := range r.RemoteRegistry.RemoteControllers {
			if filterMap, ok := clusterIdFilterMap[rc.ClusterID]; ok {
				for _, filter := range filterMap {
					log.Infof(LogFormat, eventType, "envoyfilter", filter, rc.ClusterID, "deleting")
					err := rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").Delete(filter, &metaV1.DeleteOptions{})
					if err != nil {
						log.Errorf(LogErrFormat, eventType, "envoyfilter", filter, rc.ClusterID, err)
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

func (dh *DependencyHandler) Added(obj *v1.Dependency) {

	log.Infof(LogFormat, "Add", "dependency-record", obj.Name, "", "Received=true namespace="+obj.Namespace)

	HandleDependencyRecord(obj, dh.RemoteRegistry)

}

func (dh *DependencyHandler) Updated(obj *v1.Dependency) {

	log.Infof(LogFormat, "Update", "dependency-record", obj.Name, "", "Received=true namespace="+obj.Namespace)

	// need clean up before handle it as added, I need to handle update that delete the dependency, find diff first
	// this is more complex cos want to make sure no other service depend on the same service (which we just removed the dependancy).
	// need to make sure nothing depend on that before cleaning up the SE for that service
	HandleDependencyRecord(obj, dh.RemoteRegistry)

}

func HandleDependencyRecord(obj *v1.Dependency, remoteRegitry *RemoteRegistry) {
	sourceIdentity := obj.Spec.Source

	if len(sourceIdentity) == 0 {
		log.Infof(LogFormat, "Event", "dependency-record", obj.Name, "", "No identity found namespace="+obj.Namespace)
	}

	updateIdentityDependencyCache(sourceIdentity, remoteRegitry.AdmiralCache.IdentityDependencyCache, obj)
}

func (dh *DependencyHandler) Deleted(obj *v1.Dependency) {
	// special case of update, delete the dependency crd file for one service, need to loop through all ones we plan to update
	// and make sure nobody else is relying on the same SE in same cluster
	log.Infof(LogFormat, "Deleted", "dependency", obj.Name, "", "Skipping, not implemented")
}

func (gtp *GlobalTrafficHandler) Added(obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Added", "globaltrafficpolicy", obj.Name, gtp.ClusterID, "received")
}

func (gtp *GlobalTrafficHandler) Updated(obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Updated", "globaltrafficpolicy", obj.Name, gtp.ClusterID, "received")
}

func (gtp *GlobalTrafficHandler) Deleted(obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Deleted", "globaltrafficpolicy", obj.Name, gtp.ClusterID, "received")
}

func (pc *DeploymentHandler) Added(obj *k8sAppsV1.Deployment) {
	HandleEventForDeployment(admiral.Add, obj, pc.RemoteRegistry, pc.ClusterID)
}

func (pc *DeploymentHandler) Deleted(obj *k8sAppsV1.Deployment) {
	HandleEventForDeployment(admiral.Delete, obj, pc.RemoteRegistry, pc.ClusterID)
}

func (rh *RolloutHandler) Added(obj *argo.Rollout) {
	HandleEventForRollout(admiral.Add, obj, rh.RemoteRegistry, rh.ClusterID)
}

func (rh *RolloutHandler) Updated(obj *argo.Rollout) {
	log.Infof(LogFormat, "Updated", "rollout", obj.Name, rh.ClusterID, "received")
}

func (rh *RolloutHandler) Deleted(obj *argo.Rollout) {
	HandleEventForRollout(admiral.Delete, obj, rh.RemoteRegistry, rh.ClusterID)
}

// helper function to handle add and delete for RolloutHandler
func HandleEventForRollout(event admiral.EventType, obj *argo.Rollout, remoteRegistry *RemoteRegistry, clusterName string) {

	log.Infof(LogFormat, event, "rollout", obj.Name, clusterName, "Received")
	globalIdentifier := common.GetRolloutGlobalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		log.Infof(LogFormat, "Event", "rollout", obj.Name, clusterName, "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return
	}

	env := common.GetEnvForRollout(obj)

	// Use the same function as added deployment function to update and put new service entry in place to replace old one
	modifyServiceEntryForNewServiceOrPod(event, env, globalIdentifier, remoteRegistry)
}

// helper function to handle add and delete for DeploymentHandler
func HandleEventForDeployment(event admiral.EventType, obj *k8sAppsV1.Deployment, remoteRegistry *RemoteRegistry, clusterName string) {
	log.Infof(LogFormat, event, "deployment", obj.Name, clusterName, "Received")

	globalIdentifier := common.GetDeploymentGlobalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		log.Infof(LogFormat, "Event", "deployment", obj.Name, clusterName, "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return
	}

	env := common.GetEnv(obj)

	// Use the same function as added deployment function to update and put new service entry in place to replace old one
	modifyServiceEntryForNewServiceOrPod(event, env, globalIdentifier, remoteRegistry)
}
