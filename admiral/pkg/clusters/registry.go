package clusters

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"k8s.io/client-go/rest"

	"github.com/sirupsen/logrus"
)

const (
	LogFormat              = "op=%v type=%v name=%v cluster=%s message=%v"
	LogFormatAdv           = "op=%v type=%v name=%v namespace=%s cluster=%s message=%v"
	LogFormatNew           = "op=%v type=%v name=%v namespace=%s identity=%s cluster=%s message=%v"
	LogFormatOperationTime = "op=%v type=%v name=%v namespace=%s cluster=%s message=%v"
	LogErrFormat           = "op=%v type=%v name=%v cluster=%v error=%v"
	AlertLogMsg            = "type assertion failed, %v is not of type string"
	AssertionLogMsg        = "type assertion failed, %v is not of type *RemoteRegistry"
)

func InitAdmiral(ctx context.Context, params common.AdmiralParams) (*RemoteRegistry, error) {
	ctxLogger := logrus.WithFields(logrus.Fields{})
	logrus.Infof("Initializing Admiral with params: %v", params)
	common.InitializeConfig(params)

	//init admiral state
	commonUtil.CurrentAdmiralState = commonUtil.AdmiralState{ReadOnly: ReadOnlyEnabled, IsStateInitialized: StateNotInitialized}
	// start admiral state checker for DR
	drStateChecker := initAdmiralStateChecker(ctx, params.AdmiralStateCheckerName, params.DRStateStoreConfigPath)
	rr := NewRemoteRegistry(ctx, params)
	ctx = context.WithValue(ctx, "remoteRegistry", rr)
	RunAdmiralStateCheck(ctx, params.AdmiralStateCheckerName, drStateChecker)
	pauseForAdmiralToInitializeState()

	var err error
	destinationServiceProcessor := &ProcessDestinationService{}
	wd := DependencyHandler{
		RemoteRegistry:              rr,
		DestinationServiceProcessor: destinationServiceProcessor,
	}

	wd.DepController, err = admiral.NewDependencyController(ctx.Done(), &wd, params.KubeconfigPath, params.DependenciesNamespace, 0, rr.ClientLoader)
	if err != nil {
		return nil, fmt.Errorf("error with dependency controller init: %v", err)
	}

	if !params.ArgoRolloutsEnabled {
		logrus.Info("argo rollouts disabled")
	}

	configMapController, err := admiral.NewConfigMapController(params.ServiceEntryIPPrefix, rr.ClientLoader)
	if err != nil {
		return nil, fmt.Errorf("error with configmap controller init: %v", err)
	}

	rr.AdmiralCache.ConfigMapController = configMapController
	loadServiceEntryCacheData(ctxLogger, ctx, rr.AdmiralCache.ConfigMapController, rr.AdmiralCache)

	err = InitAdmiralWithDefaultPersona(ctx, params, rr)

	if err != nil {
		return nil, err
	}

	go rr.shutdown()

	return rr, err
}

func InitAdmiralHA(ctx context.Context, params common.AdmiralParams) (*RemoteRegistry, error) {
	var (
		err error
		rr  *RemoteRegistry
	)
	logrus.Infof("Initializing Admiral HA with params: %v", params)
	common.InitializeConfig(params)
	if common.GetHAMode() == common.HAController {
		rr = NewRemoteRegistryForHAController(ctx)
	} else {
		return nil, fmt.Errorf("admiral HA only supports %s mode", common.HAController)
	}
	destinationServiceProcessor := &ProcessDestinationService{}
	rr.DependencyController, err = admiral.NewDependencyController(
		ctx.Done(),
		&DependencyHandler{
			RemoteRegistry:              rr,
			DestinationServiceProcessor: destinationServiceProcessor,
		},
		params.KubeconfigPath,
		params.DependenciesNamespace,
		params.CacheReconcileDuration,
		rr.ClientLoader)
	if err != nil {
		return nil, fmt.Errorf("error with DependencyController initialization: %v", err)
	}

	err = InitAdmiralWithDefaultPersona(ctx, params, rr)
	go rr.shutdown()
	return rr, err
}

func InitAdmiralWithDefaultPersona(ctx context.Context, params common.AdmiralParams, w *RemoteRegistry) error {
	logrus.Infof("Initializing Default Persona of Admiral")

	err := createSecretController(ctx, w)
	if err != nil {
		return fmt.Errorf("error with secret control init: %v", err)
	}
	return nil
}

func pauseForAdmiralToInitializeState() {
	// Sleep until Admiral determines state. This is done to make sure events are not skipped during startup while determining READ-WRITE state
	start := time.Now()
	logrus.Info("Pausing thread to let Admiral determine it's READ-WRITE state. This is to let Admiral determine it's state during startup")
	for {
		if commonUtil.CurrentAdmiralState.IsStateInitialized {
			logrus.Infof("Time taken for Admiral to complete state initialization =%v ms", time.Since(start).Milliseconds())
			break
		}
		if time.Since(start).Milliseconds() > 60000 {
			logrus.Error("Admiral not initialized after 60 seconds. Exiting now!!")
			os.Exit(-1)
		}
		logrus.Debug("Admiral is waiting to determine state before proceeding with boot up")
		time.Sleep(100 * time.Millisecond)
	}

}

func createSecretController(ctx context.Context, w *RemoteRegistry) error {
	var (
		err        error
		controller *secret.Controller
	)
	w.secretClient, err = w.ClientLoader.LoadKubeClientFromPath(common.GetKubeconfigPath())
	if err != nil {
		return fmt.Errorf("could not create K8s client: %v", err)
	}
	controller, err = secret.StartSecretController(
		ctx,
		w.secretClient,
		w.createCacheController,
		w.updateCacheController,
		w.deleteCacheController,
		common.GetClusterRegistriesNamespace(),
		common.GetAdmiralProfile(), common.GetAdmiralConfigPath())
	if err != nil {
		return fmt.Errorf("could not start secret controller: %v", err)
	}
	w.SecretController = controller
	return nil
}

func (r *RemoteRegistry) createCacheController(clientConfig *rest.Config, clusterID string, resyncPeriod util.ResyncIntervals) error {
	var (
		err  error
		stop = make(chan struct{})
		rc   = RemoteController{
			stop:      stop,
			ClusterID: clusterID,
			ApiServer: clientConfig.Host,
			StartTime: time.Now(),
		}
	)
	if common.GetHAMode() != common.HAController {
		logrus.Infof("starting ServiceController clusterID: %v", clusterID)
		rc.ServiceController, err = admiral.NewServiceController(stop, &ServiceHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0, r.ClientLoader)
		if err != nil {
			return fmt.Errorf("error with ServiceController initialization, err: %v", err)
		}

		if common.IsClientConnectionConfigProcessingEnabled() {
			logrus.Infof("starting ClientConnectionsConfigController clusterID: %v", clusterID)
			rc.ClientConnectionConfigController, err = admiral.NewClientConnectionConfigController(
				stop, &ClientConnectionConfigHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0, r.ClientLoader)
			if err != nil {
				return fmt.Errorf("error with ClientConnectionsConfigController initialization, err: %v", err)
			}
		} else {
			logrus.Infof("ClientConnectionsConfigController processing is disabled")
		}

		logrus.Infof("starting GlobalTrafficController clusterID: %v", clusterID)
		rc.GlobalTraffic, err = admiral.NewGlobalTrafficController(stop, &GlobalTrafficHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0, r.ClientLoader)
		if err != nil {
			return fmt.Errorf("error with GlobalTrafficController initialization, err: %v", err)
		}

		logrus.Infof("starting OutlierDetectionController clusterID : %v", clusterID)
		rc.OutlierDetectionController, err = admiral.NewOutlierDetectionController(stop, &OutlierDetectionHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0, r.ClientLoader)
		if err != nil {
			return fmt.Errorf("error with OutlierDetectionController initialization, err: %v", err)
		}

		logrus.Infof("starting NodeController clusterID: %v", clusterID)
		rc.NodeController, err = admiral.NewNodeController(stop, &NodeHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, r.ClientLoader)
		if err != nil {
			return fmt.Errorf("error with NodeController controller initialization, err: %v", err)
		}
		logrus.Infof("starting ServiceEntryController for clusterID: %v", clusterID)
		rc.ServiceEntryController, err = istio.NewServiceEntryController(stop, &ServiceEntryHandler{RemoteRegistry: r, ClusterID: clusterID}, clusterID, clientConfig, resyncPeriod.SeAndDrReconcileInterval, r.ClientLoader)
		if err != nil {
			return fmt.Errorf("error with ServiceEntryController initialization, err: %v", err)
		}

		logrus.Infof("starting DestinationRuleController for clusterID: %v", clusterID)
		rc.DestinationRuleController, err = istio.NewDestinationRuleController(stop, &DestinationRuleHandler{RemoteRegistry: r, ClusterID: clusterID}, clusterID, clientConfig, resyncPeriod.SeAndDrReconcileInterval, r.ClientLoader)
		if err != nil {
			return fmt.Errorf("error with DestinationRuleController initialization, err: %v", err)
		}

		logrus.Infof("starting VirtualServiceController for clusterID: %v", clusterID)
		virtualServiceHandler, err := NewVirtualServiceHandler(r, clusterID)
		if err != nil {
			return fmt.Errorf("error initializing VirtualServiceHandler: %v", err)
		}
		rc.VirtualServiceController, err = istio.NewVirtualServiceController(stop, virtualServiceHandler, clientConfig, 0, r.ClientLoader)
		if err != nil {
			return fmt.Errorf("error with VirtualServiceController initialization, err: %v", err)
		}

		logrus.Infof("starting SidecarController for clusterID: %v", clusterID)
		rc.SidecarController, err = istio.NewSidecarController(stop, &SidecarHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0, r.ClientLoader)
		if err != nil {
			return fmt.Errorf("error with SidecarController initialization, err: %v", err)
		}

		logrus.Infof("starting RoutingPoliciesController for clusterID: %v", clusterID)
		rc.RoutingPolicyController, err = admiral.NewRoutingPoliciesController(stop, &RoutingPolicyHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, 0, r.ClientLoader)
		if err != nil {
			return fmt.Errorf("error with RoutingPoliciesController initialization, err: %v", err)
		}
	}
	logrus.Infof("starting DeploymentController for clusterID: %v", clusterID)
	rc.DeploymentController, err = admiral.NewDeploymentController(stop, &DeploymentHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, resyncPeriod.UniversalReconcileInterval, r.ClientLoader)
	if err != nil {
		return fmt.Errorf("error with DeploymentController initialization, err: %v", err)
	}
	logrus.Infof("starting RolloutController clusterID: %v", clusterID)
	if r.AdmiralCache == nil {
		logrus.Warn("admiral cache was nil!")
	} else if r.AdmiralCache.argoRolloutsEnabled {
		rc.RolloutController, err = admiral.NewRolloutsController(stop, &RolloutHandler{RemoteRegistry: r, ClusterID: clusterID}, clientConfig, resyncPeriod.UniversalReconcileInterval, r.ClientLoader)
		if err != nil {
			return fmt.Errorf("error with RolloutController initialization, err: %v", err)
		}
	}
	r.PutRemoteController(clusterID, &rc)
	return nil
}

func (r *RemoteRegistry) updateCacheController(clientConfig *rest.Config, clusterID string, resyncPeriod util.ResyncIntervals) error {
	//We want to refresh the cache controllers. But the current approach is parking the goroutines used in the previous set of controllers, leading to a rather large memory leak.
	//This is a temporary fix to only do the controller refresh if the API Server of the remote cluster has changed
	//The refresh will still park goroutines and still increase memory usage. But it will be a *much* slower leak. Filed https://github.com/istio-ecosystem/admiral/issues/122 for that.
	controller := r.GetRemoteController(clusterID)
	if clientConfig.Host != controller.ApiServer {
		logrus.Infof("Client mismatch, recreating cache controllers for cluster=%v", clusterID)
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
	logrus.Infof(LogFormat, "Delete", "remote-controller", clusterID, clusterID, "success")
	return nil
}
