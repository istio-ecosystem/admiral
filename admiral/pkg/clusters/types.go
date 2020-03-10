package clusters

import (
	"context"
	"errors"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	log "github.com/sirupsen/logrus"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	k8s "k8s.io/client-go/kubernetes"

	"sync"
)

type RemoteController struct {
	ClusterID                 string
	GlobalTraffic             *admiral.GlobalTrafficController
	DeploymentController      *admiral.DeploymentController
	ServiceController         *admiral.ServiceController
	PodController             *admiral.PodController
	NodeController            *admiral.NodeController
	ServiceEntryController    *istio.ServiceEntryController
	DestinationRuleController *istio.DestinationRuleController
	VirtualServiceController  *istio.VirtualServiceController
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
}

type RemoteRegistry struct {
	sync.Mutex
	remoteControllers map[string]*RemoteController
	secretClient      k8s.Interface
	ctx               context.Context
	AdmiralCache      *AdmiralCache
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

type GlobalTrafficHandler struct {
	RemoteRegistry *RemoteRegistry
}

type globalTrafficCache struct {
	//map of global traffic policies key=environment.identity, value: GlobalTrafficPolicy object
	identityCache map[string]*v1.GlobalTrafficPolicy

	//map of dependencies. key=namespace.globaltrafficpolicy name. value Deployment object
	dependencyCache map[string]*k8sAppsV1.Deployment

	mutex *sync.Mutex
}

func (g *globalTrafficCache) GetFromIdentity(identity string, environment string) *v1.GlobalTrafficPolicy {
	return g.identityCache[getCacheKey(environment, identity)]
}

func (g *globalTrafficCache) GetDeployment(gtpName string) *k8sAppsV1.Deployment {
	return g.dependencyCache[gtpName]
}

func (g *globalTrafficCache) Put(gtp *v1.GlobalTrafficPolicy, deployment *k8sAppsV1.Deployment) error {
	if gtp.Name == "" {
		//no GTP, throw error
		return errors.New("cannot add an empty globaltrafficpolicy to the cache")
	}
	defer g.mutex.Unlock()
	g.mutex.Lock()
	log.Infof("Adding Deployment with name %v and gtp with name %v to GTP cache. LabelMatch=%v env=%v", deployment.Name, gtp.Name,gtp.Labels[common.GetGlobalTrafficDeploymentLabel()], gtp.Labels[common.Env])
	if deployment != nil && deployment.Labels != nil {
		//we have a valid deployment
		env := deployment.Spec.Template.Labels[common.Env]
		if env == "" {
			//No environment label, use default value
			env = common.Default
		}
		identity := deployment.Labels[common.GetWorkloadIdentifier()]
		key := getCacheKey(env, identity)
		g.identityCache[key] = gtp
	} else if g.dependencyCache[gtp.Name] != nil {
		//The old GTP matched a deployment, the new one doesn't. So we need to clear that cache.
		oldDeployment := g.dependencyCache[gtp.Name]
		env := oldDeployment.Spec.Template.Labels[common.Env]
		identity := oldDeployment.Labels[common.GetWorkloadIdentifier()]
		key := getCacheKey(env, identity)
		delete(g.identityCache, key)
	}

	g.dependencyCache[gtp.Name] = deployment
	return nil
}

func (g *globalTrafficCache) Delete(gtp *v1.GlobalTrafficPolicy) {
	if gtp.Name == "" {
		//no GTP, nothing to delete
		return
	}
	defer g.mutex.Unlock()
	g.mutex.Lock()
	log.Infof("Deleting gtp with name %v to GTP cache. LabelMatch=%v env=%v", gtp.Name,gtp.Labels[common.GetGlobalTrafficDeploymentLabel()], gtp.Labels[common.Env])

	deployment := g.dependencyCache[gtp.Name]

	if deployment != nil && deployment.Labels != nil {
		
		//we have a valid deployment
		env := deployment.Spec.Template.Labels[common.Env]
		if env == "" {
			//No environment label, use default value
			env = common.Default
		}
		identity := deployment.Labels[common.GetWorkloadIdentifier()]
		key := getCacheKey(env, identity)
		delete(g.identityCache, key)
	}

	delete(g.dependencyCache, gtp.Name)
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

func (dh *DependencyHandler) Added(obj *v1.Dependency) {

	log.Infof(LogFormat, "Event", "dependency-record", obj.Name, "", "Received=true namespace="+obj.Namespace)

	sourceIdentity := obj.Spec.Source

	if len(sourceIdentity) == 0 {
		log.Infof(LogFormat, "Event", "dependency-record", obj.Name, "", "No identity found namespace="+obj.Namespace)
	}

	updateIdentityDependencyCache(sourceIdentity, dh.RemoteRegistry.AdmiralCache.IdentityDependencyCache, obj)

	handleDependencyRecord(sourceIdentity, dh.RemoteRegistry, dh.RemoteRegistry.remoteControllers, obj)

}

func (dh *DependencyHandler) Deleted(obj *v1.Dependency) {
	log.Infof(LogFormat, "Deleted", "dependency", obj.Name, obj.ClusterName, "Skipping, not implemented")
}

func (gtp *GlobalTrafficHandler) Added(obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Added", "trafficpolicy", obj.Name, obj.ClusterName, "received")

	var matchedDeployments []k8sAppsV1.Deployment

	//IMPORTANT: The deployment matched with a GTP will not necessarily be from the same cluster. This is because the same service could be deployed in multiple clusters and we need to guarantee consistent behavior
	for _, remoteCluster := range gtp.RemoteRegistry.remoteControllers {
		matchedDeployments = append(matchedDeployments, remoteCluster.DeploymentController.GetDeploymentByLabel(obj.Labels[common.GetGlobalTrafficDeploymentLabel()], obj.Namespace)...)
	}

	deployments := common.MatchDeploymentsToGTP(obj, matchedDeployments)

	if len(deployments) != 0 {
		for _, deployment := range deployments {
			err := gtp.RemoteRegistry.AdmiralCache.GlobalTrafficCache.Put(obj, &deployment)
			if err != nil {
				log.Errorf("Failed to add nw GTP to cache. Error=%v", err)
				log.Infof(LogFormat, "Added", "trafficpolicy", obj.Name, obj.ClusterName, "Failed")
			}
		}
	} else {
		log.Infof(LogErrFormat, "Added", "trafficpolicy", obj.Name, obj.ClusterName, "Skipping, no matched deployments")
	}

}

func (gtp *GlobalTrafficHandler) Updated(obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Updated", "trafficpolicy", obj.Name, obj.ClusterName, "received")

	var matchedDeployments []k8sAppsV1.Deployment

	//IMPORTANT: The deployment matched with a GTP will not necessarily be from the same cluster. This is because the same service could be deployed in multiple clusters and we need to guarantee consistent behavior
	for _, remoteCluster := range gtp.RemoteRegistry.remoteControllers {
		matchedDeployments = append(matchedDeployments, remoteCluster.DeploymentController.GetDeploymentByLabel(obj.Labels[common.GetGlobalTrafficDeploymentLabel()], obj.Namespace)...)
	}

	deployments := common.MatchDeploymentsToGTP(obj, matchedDeployments)

	if len(deployments) != 0 {
		for _, deployment := range deployments {
			err := gtp.RemoteRegistry.AdmiralCache.GlobalTrafficCache.Put(obj, &deployment)
			if err != nil {
				log.Errorf("Failed to add updated GTP to cache. Error=%v", err)
				log.Infof(LogFormat, "Updated", "trafficpolicy", obj.Name, obj.ClusterName, "Failed")
			}
		}
	} else {
		err := gtp.RemoteRegistry.AdmiralCache.GlobalTrafficCache.Put(obj, nil)
		if err != nil {
			log.Errorf("Failed to add updated GTP to cache. Error=%v", err)
			log.Infof(LogFormat, "Updated", "trafficpolicy", obj.Name, obj.ClusterName, "Failed")
		} else {
			log.Infof(LogErrFormat, "Updated", "trafficpolicy", obj.Name, obj.ClusterName, "Skipping, no matched deployments")
		}
	}
}

func (gtp *GlobalTrafficHandler) Deleted(obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Deleted", "trafficpolicy", obj.Name, obj.ClusterName, "received")

	gtp.RemoteRegistry.AdmiralCache.GlobalTrafficCache.Delete(obj)
}

func (pc *DeploymentHandler) Added(obj *k8sAppsV1.Deployment) {
	log.Infof(LogFormat, "Event", "deployment", obj.Name, "", "Received")

	globalIdentifier := common.GetDeploymentGlobalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		log.Infof(LogFormat, "Event", "deployment", obj.Name, "", "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return
	}

	var matchedGTPs []v1.GlobalTrafficPolicy
	for _, remoteCluster := range pc.RemoteRegistry.remoteControllers {
		matchedGTPs = append(matchedGTPs, remoteCluster.GlobalTraffic.GetGTPByLabel(obj.Labels[common.GetGlobalTrafficDeploymentLabel()], obj.Namespace)...)
	}

	gtp := common.MatchGTPsToDeployment(matchedGTPs, obj)

	if gtp != nil {
		err := pc.RemoteRegistry.AdmiralCache.GlobalTrafficCache.Put(gtp, obj)
		if err != nil {
			log.Errorf("Failed to add Deployment to GTP cache. Error=%v", err)
		} else {
			log.Infof(LogFormat, "Event", "deployment", obj.Name, obj.ClusterName, "Matched to GTP name="+gtp.Name)
		}
	}

	env := common.GetEnv(obj)

	createServiceEntryForNewServiceOrPod(env, globalIdentifier, pc.RemoteRegistry)
}

func (pc *DeploymentHandler) Deleted(obj *k8sAppsV1.Deployment) {
	log.Infof(LogFormat, "Deleted", "deployment", obj.Name, obj.ClusterName, "Skipped, not implemented")
	//todo remove from gtp cache

	//TODO update subset service entries
}

func (pc *PodHandler) Added(obj *k8sV1.Pod) {
	log.Infof(LogFormat, "Event", "deployment", obj.Name, "", "Received")

	globalIdentifier := common.GetPodGlobalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		log.Infof(LogFormat, "Event", "deployment", obj.Name, "", "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return
	}

	//TODO Skip pod events until GTP is implemented
	//createServiceEntryForNewServiceOrPod(obj.Namespace, globalIdentifier, pc.RemoteRegistry)
}

func (pc *PodHandler) Deleted(obj *k8sV1.Pod) {
	//TODO update subset service entries
}

func (nc *NodeHandler) Added(obj *k8sV1.Node) {
	//log.Infof("New Pod %s on cluster: %s in namespace: %s", obj.Name, obj.ClusterName, obj.Namespace)
}

func (pc *NodeHandler) Deleted(obj *k8sV1.Node) {
	//	log.Infof("Pod deleted %s on cluster: %s in namespace: %s", obj.Name, obj.ClusterName, obj.Namespace)
}

func (sc *ServiceHandler) Added(obj *k8sV1.Service) {

	log.Infof(LogFormat, "Event", "service", obj.Name, "", "Received, doing nothing")

	//sourceIdentity := common.GetServiceGlobalIdentifier(obj)
	//
	//if len(sourceIdentity) == 0 {
	//	log.Infof(LogFormat, "Event", "service", obj.Name, "", "Skipped as '" + common.GlobalIdentifier() + " was not found', namespace=" + obj.Namespace)
	//	return
	//}
	//
	//createServiceEntryForNewServiceOrPod(obj.Namespace, sourceIdentity, sc.RemoteRegistry, sc.RemoteRegistry.config.SyncNamespace)

}

func (sc *ServiceHandler) Deleted(obj *k8sV1.Service) {

}

func getCacheKey(environment string, identity string) string {
	return fmt.Sprintf("%s.%s", environment, identity)
}
