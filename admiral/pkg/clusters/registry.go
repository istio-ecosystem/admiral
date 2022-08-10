package clusters

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"k8s.io/client-go/rest"
	"os"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	log "github.com/sirupsen/logrus"
)

const (
	LogFormat    = "op=%s type=%v name=%v cluster=%s message=%s"
	LogErrFormat = "op=%s type=%v name=%v cluster=%s, e=%v"
)

func InitAdmiral(ctx context.Context, params common.AdmiralParams) (*RemoteRegistry, error) {

	log.Infof("Initializing Admiral with params: %v", params)

	common.InitializeConfig(params)

	CurrentAdmiralState = AdmiralState{ReadOnly: ReadOnlyEnabled, IsStateInitialized: StateNotInitialized}
	startAdmiralStateChecker(ctx, params)
	pauseForAdmiralToInitializeState()

	w := NewRemoteRegistry(ctx, params)

	wd := DependencyHandler{
		RemoteRegistry: w,
	}

	var err error
	wd.DepController, err = admiral.NewDependencyController(ctx.Done(), &wd, params.KubeconfigPath, params.DependenciesNamespace, params.CacheRefreshDuration)
	if err != nil {
		return nil, fmt.Errorf("error with dependency controller init: %v", err)
	}

	if !params.ArgoRolloutsEnabled {
		log.Info("argo rollouts disabled")
	}

	configMapController, err := admiral.NewConfigMapController(params.ServiceEntryIPPrefix)
	if err != nil {
		return nil, fmt.Errorf("error with configmap controller init: %v", err)
	}
	w.AdmiralCache.ConfigMapController = configMapController
	loadServiceEntryCacheData(w.AdmiralCache.ConfigMapController, w.AdmiralCache)

	err = createSecretController(ctx, w)
	if err != nil {
		return nil, fmt.Errorf("error with secret control init: %v", err)
	}

	go w.shutdown()

	return w, nil
}

func pauseForAdmiralToInitializeState() {
	// Sleep until Admiral determines state. This is done to make sure events are not skipped during startup while determining READ-WRITE state
	start := time.Now()
	log.Info("Pausing thread to let Admiral determine it's READ-WRITE state. This is to let Admiral determine it's state during startup")
	for {
		if CurrentAdmiralState.IsStateInitialized {
			log.Infof("Time taken for Admiral to complete state initialization =%v ms", time.Since(start).Milliseconds())
			break
		}
		if time.Since(start).Milliseconds() > 60000 {
			log.Error("Admiral not initialized after 60 seconds. Exiting now!!")
			os.Exit(-1)
		}
		log.Debug("Admiral is waiting to determine state before proceeding with boot up")
		time.Sleep(100 * time.Millisecond)
	}

}

func createSecretController(ctx context.Context, w *RemoteRegistry) error {
	var err error
	var controller *secret.Controller

	w.secretClient, err = admiral.K8sClientFromPath(common.GetKubeconfigPath())
	if err != nil {
		return fmt.Errorf("could not create K8s client: %v", err)
	}

	controller, err = secret.StartSecretController(w.secretClient,
		w.createCacheController,
		w.updateCacheController,
		w.deleteCacheController,
		common.GetClusterRegistriesNamespace(),
		ctx, common.GetSecretResolver())

	if err != nil {
		return fmt.Errorf("could not start secret controller: %v", err)
	}

	w.SecretController = controller

	return nil
}

func (r *RemoteRegistry) createCacheController(clientConfig *rest.Config, clusterID string, resyncPeriod time.Duration) error {

	stop := make(chan struct{})

	rc := RemoteController{
		stop:      stop,
		ClusterID: clusterID,
		ApiServer: clientConfig.Host,
		StartTime: time.Now(),
	}

	var err error

	log.Infof("starting service controller clusterID: %v", clusterID)
	rc.ServiceController, err = admiral.NewServiceController(clusterID, stop, &ServiceHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0)

	if err != nil {
		return fmt.Errorf("error with ServiceController controller init: %v", err)
	}

	log.Infof("starting global traffic policy controller custerID: %v", clusterID)

	rc.GlobalTraffic, err = admiral.NewGlobalTrafficController(clusterID, stop, &GlobalTrafficHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0)

	if err != nil {
		return fmt.Errorf("error with GlobalTrafficController controller init: %v", err)
	}

	log.Infof("starting node controller clusterID: %v", clusterID)
	rc.NodeController, err = admiral.NewNodeController(clusterID, stop, &NodeHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig)

	if err != nil {
		return fmt.Errorf("error with NodeController controller init: %v", err)
	}

	log.Infof("starting service entry controller for custerID: %v", clusterID)
	rc.ServiceEntryController, err = istio.NewServiceEntryController(clusterID, stop, &ServiceEntryHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0)

	if err != nil {
		return fmt.Errorf("error with ServiceEntryController init: %v", err)
	}

	log.Infof("starting destination rule controller for custerID: %v", clusterID)
	rc.DestinationRuleController, err = istio.NewDestinationRuleController(clusterID, stop, &DestinationRuleHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0)

	if err != nil {
		return fmt.Errorf("error with DestinationRuleController init: %v", err)
	}

	log.Infof("starting virtual service controller for custerID: %v", clusterID)
	rc.VirtualServiceController, err = istio.NewVirtualServiceController(clusterID, stop, &VirtualServiceHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0)

	if err != nil {
		return fmt.Errorf("error with VirtualServiceController init: %v", err)
	}

	rc.SidecarController, err = istio.NewSidecarController(clusterID, stop, &SidecarHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0)

	if err != nil {
		return fmt.Errorf("error with DestinationRuleController init: %v", err)
	}

	log.Infof("starting deployment controller clusterID: %v", clusterID)
	rc.DeploymentController, err = admiral.NewDeploymentController(clusterID, stop, &DeploymentHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf("error with DeploymentController controller init: %v", err)
	}

	if r.AdmiralCache == nil {
		log.Warn("admiral cache was nil!")
	} else if r.AdmiralCache.argoRolloutsEnabled {
		log.Infof("starting rollout controller clusterID: %v", clusterID)
		rc.RolloutController, err = admiral.NewRolloutsController(clusterID, stop, &RolloutHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, resyncPeriod)

		if err != nil {
			return fmt.Errorf("error with Rollout controller init: %v", err)
		}
	}
	
	log.Infof("starting Routing Policies controller for custerID: %v", clusterID)
	rc.RoutingPolicyController, err = admiral.NewRoutingPoliciesController(stop, &RoutingPolicyHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 1 * time.Minute)

	if err != nil {
		return fmt.Errorf("error with virtualServiceController init: %v", err)
	}

	log.Infof("starting node controller clusterID: %v", clusterID)
	rc.NodeController, err = admiral.NewNodeController(clusterID, stop, &NodeHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig)

	if err != nil {
		return fmt.Errorf("error with NodeController controller init: %v", err)
	}

	log.Infof("starting service controller clusterID: %v", clusterID)
	rc.ServiceController, err = admiral.NewServiceController(clusterID, stop, &ServiceHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf("error with ServiceController controller init: %v", err)
	}

	log.Infof("starting service entry controller for custerID: %v", clusterID)
	rc.ServiceEntryController, err = istio.NewServiceEntryController(clusterID, stop, &ServiceEntryHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf("error with ServiceEntryController init: %v", err)
	}

	log.Infof("starting destination rule controller for custerID: %v", clusterID)
	rc.DestinationRuleController, err = istio.NewDestinationRuleController(clusterID, stop, &DestinationRuleHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf("error with DestinationRuleController init: %v", err)
	}

	log.Infof("starting virtual service controller for custerID: %v", clusterID)
	rc.VirtualServiceController, err = istio.NewVirtualServiceController(clusterID, stop, &VirtualServiceHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf("error with VirtualServiceController init: %v", err)
	}

	rc.SidecarController, err = istio.NewSidecarController(clusterID, stop, &SidecarHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf("error with DestinationRuleController init: %v", err)
	}

	r.PutRemoteController(clusterID, &rc)

	log.Infof("Create Controller %s", clusterID)

	return nil
}

func (r *RemoteRegistry) updateCacheController(clientConfig *rest.Config, clusterID string, resyncPeriod time.Duration) error {
	//We want to refresh the cache controllers. But the current approach is parking the goroutines used in the previous set of controllers, leading to a rather large memory leak.
	//This is a temporary fix to only do the controller refresh if the API Server of the remote cluster has changed
	//The refresh will still park goroutines and still increase memory usage. But it will be a *much* slower leak. Filed https://github.com/istio-ecosystem/admiral/issues/122 for that.
	controller := r.GetRemoteController(clusterID)

	if clientConfig.Host != controller.ApiServer {
		log.Infof("Client mismatch, recreating cache controllers for cluster=%v", clusterID)

		if err := r.deleteCacheController(clusterID); err != nil {
			return err
		}
		return r.createCacheController(clientConfig, clusterID, resyncPeriod)

	}
	return nil
}

func (r *RemoteRegistry) deleteCacheController(clusterID string) error {

	controller := r.GetRemoteController(clusterID)

	if controller != nil {
		close(controller.stop)
	}

	r.DeleteRemoteController(clusterID)

	log.Infof(LogFormat, "Delete", "remote-controller", clusterID, clusterID, "success")
	return nil
}
