package clusters

import (
	"context"
	"fmt"

	rolloutsV1Alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ServiceHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

func (sh *ServiceHandler) Added(ctx context.Context, obj *coreV1.Service) error {
	ctx = context.WithValue(ctx, common.EventType, admiral.Add)
	err := handleEventForService(ctx, obj, sh.RemoteRegistry, sh.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Add, common.ServiceResourceType, obj.Name, sh.ClusterID, err)
	}
	return nil
}

func (sh *ServiceHandler) Updated(ctx context.Context, obj *coreV1.Service) error {
	ctx = context.WithValue(ctx, common.EventType, admiral.Update)
	err := handleEventForService(ctx, obj, sh.RemoteRegistry, sh.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Update, common.ServiceResourceType, obj.Name, sh.ClusterID, err)
	}
	return nil
}

func (sh *ServiceHandler) Deleted(ctx context.Context, obj *coreV1.Service) error {
	ctx = context.WithValue(ctx, common.EventType, admiral.Delete)
	err := handleEventForService(ctx, obj, sh.RemoteRegistry, sh.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Delete, common.ServiceResourceType, obj.Name, sh.ClusterID, err)
	}
	return nil
}

func handleEventForService(
	ctx context.Context,
	svc *coreV1.Service,
	remoteRegistry *RemoteRegistry,
	clusterName string) error {
	if svc.Spec.Selector == nil {
		return fmt.Errorf("selector missing on service=%s in namespace=%s cluster=%s", svc.Name, svc.Namespace, clusterName)
	}

	rc := remoteRegistry.GetRemoteController(clusterName)
	if rc == nil {
		return fmt.Errorf("could not find the remote controller for cluster=%s", clusterName)
	}

	var handleSvcEventError error
	deploymentController := rc.DeploymentController
	rolloutController := rc.RolloutController
	serviceController := rc.ServiceController

	if deploymentController != nil && serviceController != nil {
		err := handleServiceEventForDeployment(ctx, svc, remoteRegistry, clusterName, deploymentController, serviceController, HandleEventForDeployment)
		if err != nil {
			handleSvcEventError = common.AppendError(handleSvcEventError, err)
		}
	}

	if common.GetAdmiralParams().ArgoRolloutsEnabled && rolloutController != nil && serviceController != nil {
		err := handleServiceEventForRollout(ctx, svc, remoteRegistry, clusterName, rolloutController, serviceController, HandleEventForRollout)
		if err != nil {
			handleSvcEventError = common.AppendError(handleSvcEventError, err)
		}
	}

	return handleSvcEventError
}

func handleServiceEventForDeployment(
	ctx context.Context,
	svc *coreV1.Service,
	remoteRegistry *RemoteRegistry,
	clusterName string,
	deployController *admiral.DeploymentController,
	serviceController *admiral.ServiceController,
	deploymentHandler HandleEventForDeploymentFunc) error {
	var (
		allErrors   error
		deployments []appsV1.Deployment
	)

	eventType, ok := ctx.Value(common.EventType).(admiral.EventType)
	if !ok {
		return fmt.Errorf(AlertLogMsg, ctx.Value(common.EventType))
	}

	if common.IsIstioIngressGatewayService(svc, common.GetAdmiralParams().LabelSet.GatewayApp) || common.IsIstioIngressGatewayService(svc, common.GetAdmiralParams().NLBIngressLabel) {
		// The eventType is overridden to admiral.Update. This is mainly
		// for admiral.Delete events sent for the ingress in the cluster
		// else it would delete all the SEs in the source and dependent clusters
		eventType = admiral.Update
		deployments = deployController.Cache.List()
		if common.IsAdmiralStateSyncerMode() && common.IsStateSyncerCluster(clusterName) {
			regErr := remoteRegistry.RegistryClient.PutClusterGateway(clusterName, svc.Name, svc.Status.LoadBalancer.Ingress[0].Hostname, "", "istio-ingressgateway", ctx.Value("txId").(string), nil)
			if regErr != nil {
				log.Errorf(LogFormat, "Event", "Deployment", "", clusterName,
					fmt.Sprintf("failed to push cluster gateway in namespace %s for service %s", svc.Namespace, svc.Name))
			}
		}
	} else {
		deployments = deployController.GetDeploymentBySelectorInNamespace(ctx, svc.Spec.Selector, svc.Namespace)
	}

	for _, deployment := range deployments {
		// If the eventType is a admiral.Delete we want to compute if there are any other services associated to the deployment
		// If Yes - We change the eventType to admiral.Update and delete the svc from the cache for which we got an event for. This is
		// done to update the SE with the new endpoints.
		// If No - We are safe to assume that there was only one associate service and the related SE is deleted
		// NOTE: if there is an err returned from checkIfThereAreMultipleMatchingServices we continue to prevent any
		// destructive updates
		_ = callRegistryForService(ctx, eventType, remoteRegistry, common.GetDeploymentGlobalIdentifier(&deployment), clusterName, svc)
		if eventType == admiral.Delete {
			multipleSvcExist, err := checkIfThereAreMultipleMatchingServices(svc, serviceController, deployment, clusterName)
			if err != nil {
				allErrors = common.AppendError(allErrors, err)
				continue
			}
			if multipleSvcExist {
				eventType = admiral.Update
				ctx = context.WithValue(ctx, common.EventType, admiral.Update)
				serviceController.Cache.Delete(svc)
			}
		}

		err := deploymentHandler(ctx, eventType, &deployment, remoteRegistry, clusterName)
		if err != nil {
			allErrors = common.AppendError(allErrors, err)
		}
	}

	return allErrors
}

func handleServiceEventForRollout(
	ctx context.Context,
	svc *coreV1.Service,
	remoteRegistry *RemoteRegistry,
	clusterName string,
	rolloutController *admiral.RolloutController,
	serviceController *admiral.ServiceController,
	rolloutHandler HandleEventForRolloutFunc) error {
	var (
		allErrors error
		rollouts  []rolloutsV1Alpha1.Rollout
	)

	eventType, ok := ctx.Value(common.EventType).(admiral.EventType)
	if !ok {
		return fmt.Errorf(AlertLogMsg, ctx.Value(common.EventType))
	}

	if common.IsIstioIngressGatewayService(svc, common.GetAdmiralParams().LabelSet.GatewayApp) || common.IsIstioIngressGatewayService(svc, common.GetAdmiralParams().NLBIngressLabel) {
		// The eventType is overridden to admiral.Update. This is mainly
		// for admiral.Delete events sent for the ingress in the cluster
		// else it would delete all the SEs in the source and dependent clusters
		eventType = admiral.Update
		rollouts = rolloutController.Cache.List()
		if common.IsAdmiralStateSyncerMode() && common.IsStateSyncerCluster(clusterName) {
			regErr := remoteRegistry.RegistryClient.PutClusterGateway(clusterName, svc.Name, svc.Status.LoadBalancer.Ingress[0].Hostname, "", "istio-ingressgateway", ctx.Value("txId").(string), nil)
			if regErr != nil {
				log.Errorf(LogFormat, "Event", "Rollout", "", clusterName,
					fmt.Sprintf("failed to push cluster gateway in namespace %s for service %s", svc.Namespace, svc.Name))
			}
		}
	} else {
		rollouts = rolloutController.GetRolloutBySelectorInNamespace(ctx, svc.Spec.Selector, svc.Namespace)
	}

	for _, rollout := range rollouts {
		// If the eventType is a admiral.Delete we want to compute if there are any other services associated to the rollout
		// If Yes - We change the eventType to admiral.Update and delete the svc from the cache for which we got an event for. This is
		// done to update the SE with the new endpoints.
		// If No - We are safe to assume that there was only one associate service and the related SE is deleted
		// NOTE: if there is an err returned from checkIfThereAreMultipleMatchingServices we continue to prevent any
		// destructive updates
		_ = callRegistryForService(ctx, eventType, remoteRegistry, common.GetRolloutGlobalIdentifier(&rollout), clusterName, svc)
		if eventType == admiral.Delete {
			multipleSvcExist, err := checkIfThereAreMultipleMatchingServices(svc, serviceController, rollout, clusterName)
			if err != nil {
				allErrors = common.AppendError(allErrors, err)
				continue
			}
			if multipleSvcExist {
				eventType = admiral.Update
				ctx = context.WithValue(ctx, common.EventType, admiral.Update)
				serviceController.Cache.Delete(svc)
			}
		}

		err := rolloutHandler(ctx, eventType, &rollout, remoteRegistry, clusterName)
		if err != nil {
			allErrors = common.AppendError(allErrors, err)
		}
	}

	return allErrors
}

// checkIfThereAreMultipleMatchingServices checks if there are multiple matching services in the namespace associated to the deployment/rollout
func checkIfThereAreMultipleMatchingServices(svc *coreV1.Service, serviceController *admiral.ServiceController, obj interface{}, clusterName string) (bool, error) {
	var (
		selector *metav1.LabelSelector
		appType  string
		ports    map[string]uint32
	)

	matchedServices := make(map[string]bool)
	cachedServices := serviceController.Cache.Get(svc.Namespace)
	if cachedServices == nil {
		return false, fmt.Errorf("service to be deleted does not exist in the cache")
	}

	switch v := obj.(type) {
	case rolloutsV1Alpha1.Rollout:
		selector = v.Spec.Selector
		appType = common.Rollout
	case appsV1.Deployment:
		selector = v.Spec.Selector
		appType = common.Deployment
	default:
		return false, fmt.Errorf("type assertion failed, %v is not of type *v1.Deployment or *argo.Rollout", obj)
	}

	for _, service := range cachedServices {
		match := common.IsServiceMatch(service.Spec.Selector, selector)
		if match {
			if appType == common.Deployment {
				deployment, ok := obj.(appsV1.Deployment)
				if !ok {
					return false, fmt.Errorf("type assertion failed, %v is not of type *v1.Deployment", obj)
				}
				ports = common.GetMeshPortsForDeployments(clusterName, service, &deployment)
			} else {
				rollout, ok := obj.(rolloutsV1Alpha1.Rollout)
				if !ok {
					return false, fmt.Errorf("type assertion failed, %v is not of type *argo.Rollout", obj)
				}
				ports = GetMeshPortsForRollout(clusterName, service, &rollout)
			}

			if len(ports) > 0 {
				matchedServices[service.Name] = true
			}
		}
	}

	// If length of the matched services for a deployment/rollout is greater than 1
	// or the delete event is received for a service that does not match the deployment/rollout
	// then return true so that there is an admiral.Update sent rather than admiral.Delete
	// later in the code
	if len(matchedServices) > 1 || !matchedServices[svc.Name] {
		return true, nil
	}

	return false, nil
}

func callRegistryForService(ctx context.Context, event admiral.EventType, registry *RemoteRegistry, globalIdentifier string, clusterName string, obj *coreV1.Service) error {
	var err error
	if common.IsAdmiralStateSyncerMode() && common.IsStateSyncerCluster(clusterName) && registry.RegistryClient != nil {
		switch event {
		case admiral.Add:
			err = registry.RegistryClient.PutHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, "Service", ctx.Value("txId").(string), obj)
		case admiral.Update:
			err = registry.RegistryClient.PutHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, "Service", ctx.Value("txId").(string), obj)
		case admiral.Delete:
			err = registry.RegistryClient.DeleteHostingData(clusterName, obj.Namespace, obj.Name, globalIdentifier, "Service", ctx.Value("txId").(string))
		}
		if err != nil {
			err = fmt.Errorf(LogFormat, event, "Service", obj.Name, clusterName, "failed to "+string(event)+" Service with err: "+err.Error())
			log.Error(err)
		}
	}
	return err
}
