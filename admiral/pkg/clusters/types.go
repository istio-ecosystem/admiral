package clusters

import (
	"context"
	"errors"
	"fmt"
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	log "github.com/sirupsen/logrus"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	k8s "k8s.io/client-go/kubernetes"
	"sync"
	"time"
)

type RemoteController struct {
	ClusterID                 string
	ApiServer                 string
	StartTime				  time.Time
	GlobalTraffic             *admiral.GlobalTrafficController
	DeploymentController      *admiral.DeploymentController
	ServiceController         *admiral.ServiceController
	NodeController            *admiral.NodeController
	ServiceEntryController    *istio.ServiceEntryController
	DestinationRuleController *istio.DestinationRuleController
	VirtualServiceController  *istio.VirtualServiceController
	SidecarController         *istio.SidecarController
	RolloutController         *admiral.RolloutController
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

type RolloutHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type globalTrafficCache struct {
	//map of global traffic policies key=environment.identity, value: GlobalTrafficPolicy object
	identityCache map[string]*v1.GlobalTrafficPolicy

	//map of dependencies. key=namespace.globaltrafficpolicy name. value Deployment object
	dependencyCache map[string]*k8sAppsV1.Deployment

	//map of dependencies. key=namespace.globaltrafficpolicy name. value Rollout object
	dependencyRolloutCache map[string]*argo.Rollout

	mutex *sync.Mutex
}

func (g *globalTrafficCache) GetFromIdentity(identity string, environment string) *v1.GlobalTrafficPolicy {
	return g.identityCache[getCacheKey(environment, identity)]
}

func (g *globalTrafficCache) GetDeployment(gtpName string) *k8sAppsV1.Deployment {
	return g.dependencyCache[gtpName]
}

func (g *globalTrafficCache) GetRollout(gtpName string) *argo.Rollout {
	return g.dependencyRolloutCache[gtpName]
}

func (g *globalTrafficCache) Put(gtp *v1.GlobalTrafficPolicy, deployment *k8sAppsV1.Deployment) error {
	if gtp.Name == "" {
		//no GTP, throw error
		return errors.New("cannot add an empty globaltrafficpolicy to the cache")
	}
	defer g.mutex.Unlock()
	g.mutex.Lock()
	var gtpEnv = common.GetGtpEnv(gtp)
	if deployment != nil && deployment.Labels != nil {
		log.Infof("Adding Deployment with name %v and gtp with name %v to GTP cache. LabelMatch=%v env=%v", deployment.Name, gtp.Name, gtp.Labels[common.GetGlobalTrafficDeploymentLabel()], gtpEnv)
		//we have a valid deployment
		env := common.GetEnv(deployment)
		identity := deployment.Labels[common.GetWorkloadIdentifier()]
		key := getCacheKey(env, identity)
		g.identityCache[key] = gtp
	} else if g.dependencyCache[gtp.Name] != nil {
		log.Infof("Adding gtp with name %v to GTP cache. LabelMatch=%v env=%v", gtp.Name, gtp.Labels[common.GetGlobalTrafficDeploymentLabel()], gtpEnv)
		//The old GTP matched a deployment, the new one doesn't. So we need to clear that cache.
		oldDeployment := g.dependencyCache[gtp.Name]
		env := common.GetEnv(oldDeployment)
		identity := oldDeployment.Labels[common.GetWorkloadIdentifier()]
		key := getCacheKey(env, identity)
		delete(g.identityCache, key)
	}

	g.dependencyCache[gtp.Name] = deployment

	return nil
}

func (g *globalTrafficCache) PutRollout(gtp *v1.GlobalTrafficPolicy, rollout *argo.Rollout) error {
	if gtp.Name == "" {
		//no GTP, throw error
		return errors.New("cannot add an empty globaltrafficpolicy to the cache")
	}
	defer g.mutex.Unlock()
	g.mutex.Lock()
	var gtpEnv = common.GetGtpEnv(gtp)
	if rollout != nil && rollout.Labels != nil {
		log.Infof("Adding Rollout with name %v and gtp with name %v to GTP cache. LabelMatch=%v env=%v", rollout.Name, gtp.Name, gtp.Labels[common.GetGlobalTrafficDeploymentLabel()], gtpEnv)
		//we have a valid rollout
		env := common.GetEnvForRollout(rollout)
		identity := rollout.Labels[common.GetWorkloadIdentifier()]
		key := getCacheKey(env, identity)
		g.identityCache[key] = gtp
	} else if g.dependencyRolloutCache[gtp.Name] != nil {
		log.Infof("Adding gtp with name %v to GTP cache. LabelMatch=%v env=%v", gtp.Name, gtp.Labels[common.GetGlobalTrafficDeploymentLabel()], gtpEnv)
		//The old GTP matched a rollout, the new one doesn't. So we need to clear that cache.
		oldRollout := g.dependencyRolloutCache[gtp.Name]
		env := common.GetEnvForRollout(oldRollout)
		identity := oldRollout.Labels[common.GetWorkloadIdentifier()]
		key := getCacheKey(env, identity)
		delete(g.identityCache, key)
	}

	g.dependencyRolloutCache[gtp.Name] = rollout

	return nil
}

func (g *globalTrafficCache) Delete(gtp *v1.GlobalTrafficPolicy) {
	if gtp.Name == "" {
		//no GTP, nothing to delete
		return
	}
	defer g.mutex.Unlock()
	g.mutex.Lock()
	log.Infof("Deleting gtp with name %v to GTP cache. LabelMatch=%v env=%v", gtp.Name, gtp.Labels[common.GetGlobalTrafficDeploymentLabel()], common.GetGtpEnv(gtp))

	deployment := g.dependencyCache[gtp.Name]

	if deployment != nil && deployment.Labels != nil {

		//we have a valid deployment
		env := common.GetEnv(deployment)
		identity := deployment.Labels[common.GetWorkloadIdentifier()]
		key := getCacheKey(env, identity)
		delete(g.identityCache, key)
	}
	rollout := g.dependencyRolloutCache[gtp.Name]

	if rollout != nil && rollout.Labels != nil {

		//we have a valid rollout
		env := common.GetEnvForRollout(rollout)
		identity := rollout.Labels[common.GetWorkloadIdentifier()]
		key := getCacheKey(env, identity)
		delete(g.identityCache, key)
	}

	delete(g.dependencyCache, gtp.Name)
	delete(g.dependencyRolloutCache, gtp.Name)
}

type DeploymentHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type PodHandler struct {
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
	log.Infof(LogFormat, "Added", "trafficpolicy", obj.Name, gtp.ClusterID, "received")

	var matchedDeployments []k8sAppsV1.Deployment
	var matchedRollouts []argo.Rollout

	//IMPORTANT: The deployment/Rollout matched with a GTP will not necessarily be from the same cluster. This is because the same service could be deployed in multiple clusters and we need to guarantee consistent behavior
	for _, remoteCluster := range gtp.RemoteRegistry.RemoteControllers {
		matchedDeployments = append(matchedDeployments, remoteCluster.DeploymentController.GetDeploymentByLabel(obj.Labels[common.GetGlobalTrafficDeploymentLabel()], obj.Namespace)...)
		if gtp.RemoteRegistry.AdmiralCache.argoRolloutsEnabled {
			matchedRollouts = append(matchedRollouts, remoteCluster.RolloutController.GetRolloutByLabel(obj.Labels[common.GetGlobalTrafficDeploymentLabel()], obj.Namespace)...)
		}
	}

	deployments := common.MatchDeploymentsToGTP(obj, matchedDeployments)
	rollouts := common.MatchRolloutsToGTP(obj, matchedRollouts)

	if len(deployments) != 0 {
		for _, deployment := range deployments {
			err := gtp.RemoteRegistry.AdmiralCache.GlobalTrafficCache.Put(obj, &deployment)
			if err != nil {
				log.Errorf("Failed to add nw GTP to cache. Error=%v", err)
				log.Infof(LogFormat, "Added", "trafficpolicy", obj.Name, gtp.ClusterID, "Failed")
			}
		}
	}

	if len(rollouts) != 0 {
		for _, rollout := range rollouts {
			err := gtp.RemoteRegistry.AdmiralCache.GlobalTrafficCache.PutRollout(obj, &rollout)
			if err != nil {
				log.Errorf("Failed to add new GTP to cache. Error=%v", err)
				log.Errorf(LogErrFormat, "Added", "trafficpolicy", obj.Name, gtp.ClusterID, "Failed")
			}
		}
	}

	if len(deployments) == 0 && len(rollouts) == 0 {
		log.Infof(LogErrFormat, "Added", "trafficpolicy", obj.Name, gtp.ClusterID, "Skipping, no matched deployments/rollouts")
	}

}

func (gtp *GlobalTrafficHandler) Updated(obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Updated", "trafficpolicy", obj.Name, gtp.ClusterID, "received")

	var matchedDeployments []k8sAppsV1.Deployment
	var matchedRollouts []argo.Rollout

	//IMPORTANT: The deployment/Rollout matched with a GTP will not necessarily be from the same cluster. This is because the same service could be deployed in multiple clusters and we need to guarantee consistent behavior
	for _, remoteCluster := range gtp.RemoteRegistry.RemoteControllers {
		matchedDeployments = append(matchedDeployments, remoteCluster.DeploymentController.GetDeploymentByLabel(obj.Labels[common.GetGlobalTrafficDeploymentLabel()], obj.Namespace)...)
		if gtp.RemoteRegistry.AdmiralCache.argoRolloutsEnabled {
			matchedRollouts = append(matchedRollouts, remoteCluster.RolloutController.GetRolloutByLabel(obj.Labels[common.GetGlobalTrafficDeploymentLabel()], obj.Namespace)...)
		}
	}

	deployments := common.MatchDeploymentsToGTP(obj, matchedDeployments)
	rollouts := common.MatchRolloutsToGTP(obj, matchedRollouts)

	if len(deployments) != 0 {
		for _, deployment := range deployments {
			err := gtp.RemoteRegistry.AdmiralCache.GlobalTrafficCache.Put(obj, &deployment)
			if err != nil {
				log.Errorf("Failed to add updated GTP to cache. Error=%v", err)
				log.Infof(LogFormat, "Updated", "trafficpolicy", obj.Name, gtp.ClusterID, "Failed")
			}
		}
	} else {
		err := gtp.RemoteRegistry.AdmiralCache.GlobalTrafficCache.Put(obj, nil)
		if err != nil {
			log.Errorf("Failed to add updated GTP to cache. Error=%v", err)
			log.Infof(LogFormat, "Updated", "trafficpolicy", obj.Name, gtp.ClusterID, "Failed")
		} else {
			log.Infof(LogErrFormat, "Updated", "trafficpolicy", obj.Name, gtp.ClusterID, "Skipping, no matched deployments")
		}
	}

	if len(rollouts) != 0 {
		for _, rollout := range rollouts {
			err := gtp.RemoteRegistry.AdmiralCache.GlobalTrafficCache.PutRollout(obj, &rollout)
			if err != nil {
				log.Errorf("Failed to add updated GTP to cache. Error=%v", err)
				log.Infof(LogFormat, "Updated", "trafficpolicy", obj.Name, gtp.ClusterID, "Failed")
			}
		}
	} else {
		err := gtp.RemoteRegistry.AdmiralCache.GlobalTrafficCache.PutRollout(obj, nil)
		if err != nil {
			log.Errorf("Failed to add updated GTP to cache. Error=%v", err)
			log.Infof(LogFormat, "Updated", "trafficpolicy", obj.Name, gtp.ClusterID, "Failed")
		} else {
			log.Infof(LogErrFormat, "Updated", "trafficpolicy", obj.Name, gtp.ClusterID, "Skipping, no matched rollouts")
		}
	}
}

func (gtp *GlobalTrafficHandler) Deleted(obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Deleted", "trafficpolicy", obj.Name, gtp.ClusterID, "received")
	gtp.RemoteRegistry.AdmiralCache.GlobalTrafficCache.Delete(obj)
}

func (pc *DeploymentHandler) Added(obj *k8sAppsV1.Deployment) {
	HandleEventForDeployment(admiral.Add, obj, pc.RemoteRegistry, pc.ClusterID)
}

func (pc *DeploymentHandler) Deleted(obj *k8sAppsV1.Deployment) {
	HandleEventForDeployment(admiral.Delete, obj, pc.RemoteRegistry, pc.ClusterID)
}

func (pc *PodHandler) Added(obj *k8sV1.Pod) {
	log.Infof(LogFormat, "Event", "deployment", obj.Name, pc.ClusterID, "Received")

	globalIdentifier := common.GetPodGlobalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		log.Infof(LogFormat, "Event", "deployment", obj.Name, "", "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return
	}

	//TODO Skip pod events until GTP is implemented
	//modifyServiceEntryForNewServiceOrPod(obj.Namespace, globalIdentifier, pc.RemoteRegistry)
}

func (pc *PodHandler) Deleted(obj *k8sV1.Pod) {
	//TODO update subset service entries
}

func getCacheKey(environment string, identity string) string {
	return fmt.Sprintf("%s.%s", environment, identity)
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

	log.Infof(LogFormat, event, "rollout", obj.Name, clusterName,  "Received")
	globalIdentifier := common.GetRolloutGlobalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		log.Infof(LogFormat, "Event", "rollout", obj.Name, clusterName, "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return
	}

	var matchedGTPs []v1.GlobalTrafficPolicy
	for _, remoteCluster := range remoteRegistry.RemoteControllers {
		matchedGTPs = append(matchedGTPs, remoteCluster.GlobalTraffic.GetGTPByLabel(obj.Labels[common.GetGlobalTrafficDeploymentLabel()], obj.Namespace)...)
	}

	gtp := common.MatchGTPsToRollout(matchedGTPs, obj)

	if gtp != nil {
		if event == admiral.Add {
			err := remoteRegistry.AdmiralCache.GlobalTrafficCache.PutRollout(gtp, obj)
			if err != nil {
				log.Errorf("Failed to add Rollout to GTP cache. Error=%v", err)
			} else {
				log.Infof(LogFormat, "Event", "rollout", obj.Name, clusterName, "Matched to GTP name="+gtp.Name)
			}
		} else if event == admiral.Delete {
			remoteRegistry.AdmiralCache.GlobalTrafficCache.Delete(gtp)
			log.Infof(LogFormat, event, "rollout", obj.Name, clusterName, "Matched to GTP name="+gtp.Name)
		}
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

	var matchedGTPs []v1.GlobalTrafficPolicy
	for _, remoteCluster := range remoteRegistry.RemoteControllers {
		matchedGTPs = append(matchedGTPs, remoteCluster.GlobalTraffic.GetGTPByLabel(obj.Labels[common.GetGlobalTrafficDeploymentLabel()], obj.Namespace)...)
	}

	gtp := common.MatchGTPsToDeployment(matchedGTPs, obj)

	if gtp != nil {
		if event == admiral.Add {
			err := remoteRegistry.AdmiralCache.GlobalTrafficCache.Put(gtp, obj)
			if err != nil {
				log.Errorf("Failed to add Deployment to GTP cache. Error=%v", err)
			} else {
				log.Infof(LogFormat, "Event", "deployment", obj.Name, clusterName, "Matched to GTP name="+gtp.Name)
			}
		} else if event == admiral.Delete {
			remoteRegistry.AdmiralCache.GlobalTrafficCache.Delete(gtp)
			log.Infof(LogFormat, event, "deployment", obj.Name, clusterName, "Matched to GTP name="+gtp.Name)
		}
	}

	env := common.GetEnv(obj)

	// Use the same function as added deployment function to update and put new service entry in place to replace old one
	modifyServiceEntryForNewServiceOrPod(event, env, globalIdentifier, remoteRegistry)
}
