package clusters

import (
	"context"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"istio.io/istio/pkg/log"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	k8s "k8s.io/client-go/kubernetes"

	"sync"
)

type RemoteController struct {
	ClusterID            string
	IstioConfigStore     istio.ConfigStoreCache
	GlobalTraffic        *admiral.GlobalTrafficController
	DeploymentController *admiral.DeploymentController
	ServiceController    *admiral.ServiceController
	PodController        *admiral.PodController
	NodeController       *admiral.NodeController
	stop                 chan struct{}
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
	ServiceEntryAddressStore	 	*ServiceEntryAddressStore
	ConfigMapController		     	admiral.ConfigMapControllerInterface //todo this should be in the remotecontrollers map once we expand it to have one configmap per cluster
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
	EntryAddresses	map[string]string `yaml:"entry-addresses,omitempty"`
	Addresses		[]string   `yaml:"addresses,omitempty"` //trading space for efficiency - this will give a quick way to validate that the address is unique
}

type DependencyHandler struct {
	RemoteRegistry *RemoteRegistry
	DepController  *admiral.DependencyController
}

type GlobalTrafficHandler struct {
	RemoteRegistry *RemoteRegistry
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

	handleDependencyRecord(obj.Spec.IdentityLabel, sourceIdentity, dh.RemoteRegistry.AdmiralCache, dh.RemoteRegistry.remoteControllers, obj)

}


func (dh *DependencyHandler) Deleted(obj *v1.Dependency) {
	log.Infof(LogFormat, "Deleted", "dependency", obj.Name, obj.ClusterName, "Skipping, not implemented")
}

func (gtp *GlobalTrafficHandler) Added(obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Added", "trafficpolicy", obj.Name, obj.ClusterName, "Skipping, not implemented")
}

func (gtp *GlobalTrafficHandler) Deleted(obj *v1.GlobalTrafficPolicy) {
	log.Infof(LogFormat, "Deleted", "trafficpolicy", obj.Name, obj.ClusterName, "Skipping, not implemented")
}

func (pc *DeploymentHandler) Added(obj *k8sAppsV1.Deployment) {
	log.Infof(LogFormat, "Event", "deployment", obj.Name, "", "Received")

	globalIdentifier := common.GetDeploymentGlobalIdentifier(obj)

	if len(globalIdentifier) == 0 {
		log.Infof(LogFormat, "Event", "deployment", obj.Name, "", "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return
	}

	env := common.GetEnv(obj)

	createServiceEntryForNewServiceOrPod(env, globalIdentifier, pc.RemoteRegistry)
}

func (pc *DeploymentHandler) Deleted(obj *k8sAppsV1.Deployment) {
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
