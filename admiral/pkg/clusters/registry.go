package clusters

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"sync"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"

)

const (
	LogFormat    = "op=%s type=%v name=%v cluster=%s message=%s"
	LogErrFormat = "op=%s type=%v name=%v cluster=%s, e=%v"
)

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
		ServiceEntryAddressStore:        &ServiceEntryAddressStore{EntryAddresses: map[string]string{}, Addresses: []string{}}}

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

	stop := make(chan struct{})

	rc := RemoteController{
		stop:      stop,
		ClusterID: clusterID,
	}

	var err error

	log.Infof("starting global traffic policy controller custerID: %v", clusterID)
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

	log.Infof("starting service entry controller for custerID: %v", clusterID)
	rc.ServiceEntryController, err = istio.NewServiceEntryController(stop, &ServiceEntryHandler{RemoteRegistry: r, ClusterID:clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with ServiceEntryController init: %v", err)
	}

	log.Infof("starting destination rule controller for custerID: %v", clusterID)
	rc.DestinationRuleController, err = istio.NewDestinationRuleController(stop, &DestinationRuleHandler{RemoteRegistry: r, ClusterID:clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with DestinationRuleController init: %v", err)
	}

	log.Infof("starting virtual service controller for custerID: %v", clusterID)
	rc.VirtualServiceController, err = istio.NewVirtualServiceController(stop, &VirtualServiceHandler{RemoteRegistry: r, ClusterID:clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with VirtualServiceController init: %v", err)
	}

	r.Lock()
	defer r.Unlock()
	r.remoteControllers[clusterID] = &rc

	log.Infof("Create Controller %s", clusterID)

	return nil
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

