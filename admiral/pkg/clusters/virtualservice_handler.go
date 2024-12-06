package clusters

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	constUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	log "github.com/sirupsen/logrus"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	vsRoutingLabel = "admiral.io/vs-routing"
)

// NewVirtualServiceHandler returns a new instance of VirtualServiceHandler after verifying
// the required properties are set correctly
func NewVirtualServiceHandler(remoteRegistry *RemoteRegistry, clusterID string) (*VirtualServiceHandler, error) {
	if remoteRegistry == nil {
		return nil, fmt.Errorf("remote registry is nil, cannot initialize VirtualServiceHandler")
	}
	if clusterID == "" {
		return nil, fmt.Errorf("clusterID is empty, cannot initialize VirtualServiceHandler")
	}
	return &VirtualServiceHandler{
		remoteRegistry:                         remoteRegistry,
		clusterID:                              clusterID,
		updateResource:                         handleVirtualServiceEventForRollout,
		syncVirtualServiceForDependentClusters: syncVirtualServicesToAllDependentClusters,
		syncVirtualServiceForAllClusters:       syncVirtualServicesToAllRemoteClusters,
	}, nil
}

// UpdateResourcesForVirtualService is a type function for processing VirtualService update operations
type UpdateResourcesForVirtualService func(
	ctx context.Context,
	virtualService *v1alpha3.VirtualService,
	remoteRegistry *RemoteRegistry,
	clusterID string,
	handlerFunc HandleEventForRolloutFunc,
) (bool, error)

// SyncVirtualServiceResource is a type function for sync VirtualServices
// for a set of clusters
type SyncVirtualServiceResource func(
	ctx context.Context,
	clusters []string,
	virtualService *v1alpha3.VirtualService,
	event common.Event,
	remoteRegistry *RemoteRegistry,
	sourceCluster string,
	syncNamespace string,
	vsName string,
) error

// VirtualServiceHandler responsible for handling Add/Update/Delete events for
// VirtualService resources
type VirtualServiceHandler struct {
	remoteRegistry                         *RemoteRegistry
	clusterID                              string
	updateResource                         UpdateResourcesForVirtualService
	syncVirtualServiceForDependentClusters SyncVirtualServiceResource
	syncVirtualServiceForAllClusters       SyncVirtualServiceResource
}

func (vh *VirtualServiceHandler) Added(ctx context.Context, obj *v1alpha3.VirtualService) error {
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(LogFormat, common.Add, "VirtualService", obj.Name, vh.clusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return nil
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, common.Add, "VirtualService", obj.Name, vh.clusterID, "Skipping resource from namespace="+obj.Namespace)
		if len(obj.Annotations) > 0 && obj.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
			log.Infof(LogFormat, "admiralIoIgnoreAnnotationCheck", "VirtualService", obj.Name, vh.clusterID, "Value=true namespace="+obj.Namespace)
		}
		return nil
	}
	return vh.handleVirtualServiceEvent(ctx, obj, common.Add)
}

func (vh *VirtualServiceHandler) Updated(ctx context.Context, obj *v1alpha3.VirtualService) error {
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(LogFormat, common.Update, "VirtualService", obj.Name, vh.clusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return nil
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, common.Update, "VirtualService", obj.Name, vh.clusterID, "Skipping resource from namespace="+obj.Namespace)
		if len(obj.Annotations) > 0 && obj.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
			log.Infof(LogFormat, "admiralIoIgnoreAnnotationCheck", "VirtualService", obj.Name, vh.clusterID, "Value=true namespace="+obj.Namespace)
		}
		return nil
	}
	return vh.handleVirtualServiceEvent(ctx, obj, common.Update)
}

func (vh *VirtualServiceHandler) Deleted(ctx context.Context, obj *v1alpha3.VirtualService) error {
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(LogFormat, common.Delete, "VirtualService", obj.Name, vh.clusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return nil
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, common.Delete, "VirtualService", obj.Name, vh.clusterID, "Skipping resource from namespace="+obj.Namespace)
		if len(obj.Annotations) > 0 && obj.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
			log.Debugf(LogFormat, "admiralIoIgnoreAnnotationCheck", "VirtualService", obj.Name, vh.clusterID, "Value=true namespace="+obj.Namespace)
		}
		return nil
	}
	return vh.handleVirtualServiceEvent(ctx, obj, common.Delete)
}

func (vh *VirtualServiceHandler) handleVirtualServiceEvent(ctx context.Context, virtualService *v1alpha3.VirtualService, event common.Event) error {
	var (
		//nolint
		syncNamespace = common.GetSyncNamespace()
	)
	defer logElapsedTimeForVirtualService("handleVirtualServiceEvent="+string(event), vh.clusterID, virtualService)()
	if syncNamespace == "" {
		return fmt.Errorf("expected valid value for sync namespace, got empty")
	}
	if ctx == nil {
		return fmt.Errorf("empty context passed")
	}
	if virtualService == nil {
		return fmt.Errorf("passed %s object is nil", common.VirtualServiceResourceType)
	}
	//nolint
	spec := virtualService.Spec

	log.Infof(LogFormat, event, common.VirtualServiceResourceType, virtualService.Name, vh.clusterID, "Received event")

	if len(spec.Hosts) > 1 {
		log.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, virtualService.Name, vh.clusterID, "Skipping as multiple hosts not supported for virtual service namespace="+virtualService.Namespace)
		return nil
	}

	// check if this virtual service is used by Argo rollouts for canary strategy, if so, update the corresponding SE with appropriate weights
	if common.GetAdmiralParams().ArgoRolloutsEnabled {
		isRolloutCanaryVS, err := vh.updateResource(ctx, virtualService, vh.remoteRegistry, vh.clusterID, HandleEventForRollout)
		if err != nil {
			return err
		}
		if isRolloutCanaryVS {
			log.Infof(LogFormat, "Event", common.VirtualServiceResourceType, virtualService.Name, vh.clusterID,
				"Skipping replicating VirtualService in other clusters as this VirtualService is associated with a Argo Rollout")
			return nil
		}
	}

	if len(spec.Hosts) == 0 {
		log.Infof(LogFormat, "Event", common.VirtualServiceResourceType, virtualService.Name, vh.clusterID, "No hosts found in VirtualService, will not sync to other clusters")
		return nil
	}

	vSName := common.GenerateUniqueNameForVS(virtualService.Namespace, virtualService.Name)

	dependentClusters := vh.remoteRegistry.AdmiralCache.CnameDependentClusterCache.Get(spec.Hosts[0]).CopyJustValues()
	if len(dependentClusters) > 0 {
		// Add source clusters to the list of clusters to copy the virtual service
		sourceClusters := vh.remoteRegistry.AdmiralCache.CnameClusterCache.Get(spec.Hosts[0]).CopyJustValues()
		clusters := append(dependentClusters, sourceClusters...)
		err := vh.syncVirtualServiceForDependentClusters(
			ctx,
			clusters,
			virtualService,
			event,
			vh.remoteRegistry,
			vh.clusterID,
			syncNamespace,
			vSName,
		)
		if err != nil {
			log.Warnf(LogErrFormat, "Sync", common.VirtualServiceResourceType, virtualService.Name, dependentClusters, err.Error()+": sync to dependent clusters will not be retried")
		} else {
			log.Infof(LogFormat, "Sync", common.VirtualServiceResourceType, virtualService.Name, dependentClusters, "synced to all dependent clusters")
		}
		return nil
	}
	log.Infof(LogFormat, "Event", "VirtualService", virtualService.Name, vh.clusterID, "No dependent clusters found")
	// copy the VirtualService `as is` if they are not generated by Admiral (not in CnameDependentClusterCache)
	log.Infof(LogFormat, "Event", "VirtualService", virtualService.Name, vh.clusterID, "Replicating 'as is' to all clusters")
	remoteClusters := vh.remoteRegistry.GetClusterIds()
	err := vh.syncVirtualServiceForAllClusters(
		ctx,
		remoteClusters,
		virtualService,
		event,
		vh.remoteRegistry,
		vh.clusterID,
		syncNamespace,
		vSName,
	)
	if err != nil {
		log.Warnf(LogErrFormat, "Sync", common.VirtualServiceResourceType, virtualService.Name, "*", err.Error()+": sync to remote clusters will not be retried")
		return nil
	}
	log.Infof(LogFormat, "Sync", common.VirtualServiceResourceType, virtualService.Name, "*", "synced to remote clusters")
	return nil
}

// handleVirtualServiceEventForRollout fetches corresponding rollout for the
// virtual service and triggers an update for ServiceEntries and DestinationRules
func handleVirtualServiceEventForRollout(
	ctx context.Context,
	virtualService *v1alpha3.VirtualService,
	remoteRegistry *RemoteRegistry,
	clusterID string,
	handleEventForRollout HandleEventForRolloutFunc) (bool, error) {
	defer logElapsedTimeForVirtualService("handleVirtualServiceEventForRollout", clusterID, virtualService)()
	// This will be set to true, if the VirtualService is configured in any of the
	// argo rollouts present in the namespace
	var isRolloutCanaryVS bool
	if virtualService == nil {
		return isRolloutCanaryVS, fmt.Errorf("VirtualService is nil")
	}
	if remoteRegistry == nil {
		return isRolloutCanaryVS, fmt.Errorf("remoteRegistry is nil")
	}
	rc := remoteRegistry.GetRemoteController(clusterID)
	if rc == nil {
		return isRolloutCanaryVS, fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, virtualService.Name, clusterID, "remote controller not initialized for cluster")
	}
	rolloutController := rc.RolloutController
	if rolloutController == nil {
		return isRolloutCanaryVS, fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, virtualService.Name, clusterID, "argo rollout controller not initialized for cluster")
	}
	rollouts, err := rolloutController.RolloutClient.Rollouts(virtualService.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return isRolloutCanaryVS, fmt.Errorf(LogFormat, "Get", "Rollout", "Error finding rollouts in namespace="+virtualService.Namespace, clusterID, err)
	}
	var allErrors error
	for _, rollout := range rollouts.Items {
		if matchRolloutCanaryStrategy(rollout.Spec.Strategy, virtualService.Name) {
			isRolloutCanaryVS = true
			err = handleEventForRollout(ctx, admiral.Update, &rollout, remoteRegistry, clusterID)
			if err != nil {
				allErrors = common.AppendError(allErrors, fmt.Errorf(LogFormat, "Event", "Rollout", rollout.Name, clusterID, err.Error()))
			}
		}
	}
	return isRolloutCanaryVS, allErrors
}

func syncVirtualServicesToAllDependentClusters(
	ctx context.Context,
	clusters []string,
	virtualService *v1alpha3.VirtualService,
	event common.Event,
	remoteRegistry *RemoteRegistry,
	sourceCluster string,
	syncNamespace string,
	vSName string,
) error {
	defer logElapsedTimeForVirtualService("syncVirtualServicesToAllDependentClusters="+string(event), "", virtualService)()
	if vSName == "" {
		return fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, "", sourceCluster, "VirtualService generated name is empty")
	}
	if virtualService == nil {
		return fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, "", sourceCluster, "VirtualService is nil")
	}
	if remoteRegistry == nil {
		return fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, "", sourceCluster, "remoteRegistry is nil")
	}
	var allClusterErrors error
	var wg sync.WaitGroup
	wg.Add(len(clusters))
	for _, cluster := range clusters {
		go func(ctx context.Context, cluster string, remoteRegistry *RemoteRegistry, virtualServiceCopy *v1alpha3.VirtualService, event common.Event, syncNamespace string) {
			defer wg.Done()
			err := syncVirtualServiceToDependentCluster(
				ctx,
				cluster,
				remoteRegistry,
				virtualServiceCopy,
				event,
				syncNamespace,
				vSName,
			)
			if err != nil {
				allClusterErrors = common.AppendError(allClusterErrors, err)
			}
		}(ctx, cluster, remoteRegistry, virtualService.DeepCopy(), event, syncNamespace)
	}
	wg.Wait()
	return allClusterErrors
}

func syncVirtualServiceToDependentCluster(
	ctx context.Context,
	cluster string,
	remoteRegistry *RemoteRegistry,
	virtualService *v1alpha3.VirtualService,
	event common.Event,
	syncNamespace string,
	vSName string) error {

	ctxLogger := log.WithFields(log.Fields{
		"type":     "syncVirtualServiceToDependentCluster",
		"identity": vSName,
		"txId":     uuid.New().String(),
	})

	defer logElapsedTimeForVirtualService("syncVirtualServiceToDependentCluster="+string(event), cluster, virtualService)()
	rc := remoteRegistry.GetRemoteController(cluster)
	if rc == nil {
		return fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, vSName,
			cluster, "dependent controller not initialized for cluster")
	}
	ctxLogger.Infof(LogFormat, "Event", "VirtualService", vSName, cluster, "Processing")
	if rc.VirtualServiceController == nil {
		return fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, vSName, cluster, "VirtualService controller not initialized for cluster")
	}

	if event == common.Delete {
		// Best effort delete for existing virtual service with old name
		_ = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Delete(ctx, virtualService.Name, metav1.DeleteOptions{})

		err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Delete(ctx, vSName, metav1.DeleteOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				ctxLogger.Infof(LogFormat, "Delete", "VirtualService", vSName, cluster, "Either VirtualService was already deleted, or it never existed")
				return nil
			}
			if isDeadCluster(err) {
				ctxLogger.Warnf(LogErrFormat, "Create/Update", common.VirtualServiceResourceType, vSName, cluster, "dead cluster")
				return nil
			}
			return fmt.Errorf(LogErrFormat, "Delete", "VirtualService", vSName, cluster, err)
		}
		ctxLogger.Infof(LogFormat, "Delete", "VirtualService", vSName, cluster, "Success")
		return nil
	}

	oldVSname := virtualService.Name
	//Update vs name to be unique per namespace
	virtualService.Name = vSName

	exist, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, vSName, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		ctxLogger.Infof(LogFormat, "Get", common.VirtualServiceResourceType, vSName, cluster, "VirtualService does not exist")
		exist = nil
	}
	if isDeadCluster(err) {
		ctxLogger.Warnf(LogErrFormat, "Create/Update", common.VirtualServiceResourceType, vSName, cluster, "dead cluster")
		return nil
	}
	//change destination host for all http routes <service_name>.<ns>. to same as host on the virtual service
	for _, httpRoute := range virtualService.Spec.Http {
		for _, destination := range httpRoute.Route {
			//get at index 0, we do not support wildcards or multiple hosts currently
			if strings.HasSuffix(destination.Destination.Host, common.DotLocalDomainSuffix) {
				destination.Destination.Host = virtualService.Spec.Hosts[0]
			}
		}
	}
	for _, tlsRoute := range virtualService.Spec.Tls {
		for _, destination := range tlsRoute.Route {
			//get at index 0, we do not support wildcards or multiple hosts currently
			if strings.HasSuffix(destination.Destination.Host, common.DotLocalDomainSuffix) {
				destination.Destination.Host = virtualService.Spec.Hosts[0]
			}
		}
	}
	// nolint
	err = addUpdateVirtualService(ctxLogger, ctx, virtualService, exist, syncNamespace, rc, remoteRegistry)

	// Best effort delete for existing virtual service with old name
	_ = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Delete(ctx, oldVSname, metav1.DeleteOptions{})

	return err
}

func syncVirtualServicesToAllRemoteClusters(
	ctx context.Context,
	clusters []string,
	virtualService *v1alpha3.VirtualService,
	event common.Event,
	remoteRegistry *RemoteRegistry,
	sourceCluster string,
	syncNamespace string,
	vSName string) error {
	defer logElapsedTimeForVirtualService("syncVirtualServicesToAllRemoteClusters="+string(event), "*", virtualService)()
	if vSName == "" {
		return fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, "", sourceCluster, "VirtualService generated name is empty")
	}
	if virtualService == nil {
		return fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, "", sourceCluster, "VirtualService is nil")
	}
	if remoteRegistry == nil {
		return fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, "", sourceCluster, "remoteRegistry is nil")
	}
	var allClusterErrors error
	var wg sync.WaitGroup
	wg.Add(len(clusters))
	for _, cluster := range clusters {
		go func(ctx context.Context, cluster string, remoteRegistry *RemoteRegistry, virtualServiceCopy *v1alpha3.VirtualService, event common.Event, syncNamespace string) {
			defer wg.Done()
			err := syncVirtualServiceToRemoteCluster(
				ctx,
				cluster,
				remoteRegistry,
				virtualServiceCopy,
				event,
				syncNamespace,
				vSName,
			)
			if err != nil {
				allClusterErrors = common.AppendError(allClusterErrors, err)
			}
		}(ctx, cluster, remoteRegistry, virtualService.DeepCopy(), event, syncNamespace)
	}
	wg.Wait()
	return allClusterErrors
}

func syncVirtualServiceToRemoteCluster(
	ctx context.Context,
	cluster string,
	remoteRegistry *RemoteRegistry,
	virtualService *v1alpha3.VirtualService,
	event common.Event,
	syncNamespace string,
	vSName string) error {

	ctxLogger := log.WithFields(log.Fields{
		"type":     "syncVirtualServicesToAllRemoteClusters",
		"identity": vSName,
		"txId":     uuid.New().String(),
	})

	defer logElapsedTimeForVirtualService("syncVirtualServiceToRemoteCluster="+string(event), cluster, virtualService)()
	rc := remoteRegistry.GetRemoteController(cluster)
	if rc == nil {
		return fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, vSName, cluster, "remote controller not initialized for cluster")
	}
	if rc.VirtualServiceController == nil {
		return fmt.Errorf(LogFormat, "Event", common.VirtualServiceResourceType, vSName, cluster, "VirtualService controller not initialized for cluster")
	}

	if event == common.Delete {
		// Best effort delete for existing virtual service with old name
		_ = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Delete(ctx, virtualService.Name, metav1.DeleteOptions{})

		err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Delete(ctx, vSName, metav1.DeleteOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				ctxLogger.Infof(LogFormat, "Delete", common.VirtualServiceResourceType, vSName, cluster, "Either VirtualService was already deleted, or it never existed")
				return nil
			}
			if isDeadCluster(err) {
				ctxLogger.Warnf(LogErrFormat, "Delete", common.VirtualServiceResourceType, vSName, cluster, "dead cluster")
				return nil
			}

			return fmt.Errorf(LogErrFormat, "Delete", common.VirtualServiceResourceType, vSName, cluster, err)
		}
		ctxLogger.Infof(LogFormat, "Delete", common.VirtualServiceResourceType, vSName, cluster, "Success")
		return nil
	}
	oldVSname := virtualService.Name
	//Update vs name to be unique per namespace
	virtualService.Name = vSName
	exist, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, vSName, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		ctxLogger.Infof(LogFormat, "Get", common.VirtualServiceResourceType, vSName, cluster, "VirtualService does not exist")
		exist = nil
	}
	if isDeadCluster(err) {
		ctxLogger.Warnf(LogErrFormat, "Create/Update", common.VirtualServiceResourceType, vSName, cluster, "dead cluster")
		return nil
	}
	err = addUpdateVirtualService(ctxLogger, ctx, virtualService, exist, syncNamespace, rc, remoteRegistry)

	// Best effort delete of existing virtual service with old name
	_ = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Delete(ctx, oldVSname, metav1.DeleteOptions{})
	// nolint
	return err
}

func matchRolloutCanaryStrategy(rolloutStrategy argo.RolloutStrategy, virtualServiceName string) bool {
	if rolloutStrategy.Canary == nil ||
		rolloutStrategy.Canary.TrafficRouting == nil ||
		rolloutStrategy.Canary.TrafficRouting.Istio == nil ||
		rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService == nil {
		return false
	}
	return rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Name == virtualServiceName
}

/*
Add/Update Virtual service after checking if the current pod is in ReadOnly mode.
Virtual Service object is not added/updated if the current pod is in ReadOnly mode.
*/
func addUpdateVirtualService(
	ctxLogger *log.Entry,
	ctx context.Context,
	new *v1alpha3.VirtualService,
	exist *v1alpha3.VirtualService,
	namespace string, rc *RemoteController, rr *RemoteRegistry) error {
	var (
		err     error
		op      string
		newCopy = new.DeepCopy()
	)

	format := "virtualservice %s before: %v, after: %v;"

	if newCopy.Annotations == nil {
		newCopy.Annotations = map[string]string{}
	}
	newCopy.Annotations["app.kubernetes.io/created-by"] = "admiral"

	skipAddingExportTo := false
	//Check if VS has the admiral.io/vs-routing label
	// If it does, skip adding ExportTo since it is already set to "istio-system" only
	// The VS created for routing cross cluster traffic should only be exported to istio-system
	if newCopy.Labels != nil && newCopy.Labels[vsRoutingLabel] == "enabled" {
		skipAddingExportTo = true
	}

	if common.EnableExportTo(newCopy.Spec.Hosts[0]) && !skipAddingExportTo {
		sortedDependentNamespaces := getSortedDependentNamespaces(rr.AdmiralCache, newCopy.Spec.Hosts[0], rc.ClusterID, ctxLogger)
		newCopy.Spec.ExportTo = sortedDependentNamespaces
		ctxLogger.Infof(LogFormat, "ExportTo", common.VirtualServiceResourceType, newCopy.Name, rc.ClusterID, fmt.Sprintf("VS usecase-ExportTo updated to %v", newCopy.Spec.ExportTo))
	}
	vsAlreadyExists := false
	if exist == nil {
		op = "Add"
		ctxLogger.Infof(LogFormat, op, common.VirtualServiceResourceType, newCopy.Name, rc.ClusterID,
			fmt.Sprintf("new virtualservice for cluster: %s VirtualService name=%s",
				rc.ClusterID, newCopy.Name))
		newCopy.Namespace = namespace
		newCopy.ResourceVersion = ""
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Create(ctx, newCopy, metav1.CreateOptions{})
		if k8sErrors.IsAlreadyExists(err) {
			ctxLogger.Infof(LogFormat, op, common.VirtualServiceResourceType, newCopy.Name, rc.ClusterID,
				fmt.Sprintf("skipping create virtualservice and it already exists for cluster: %s VirtualService name=%s",
					rc.ClusterID, newCopy.Name))
			vsAlreadyExists = true
		}
	}
	if exist != nil || vsAlreadyExists {
		if vsAlreadyExists {
			exist, err = rc.VirtualServiceController.IstioClient.
				NetworkingV1alpha3().
				VirtualServices(namespace).
				Get(ctx, newCopy.Name, metav1.GetOptions{})
			if err != nil {
				// when there is an error, assign exist to obj,
				// which will fail in the update operation, but will be retried
				// in the retry logic
				exist = newCopy
				ctxLogger.Warnf(common.CtxLogFormat, "Update", exist.Name, exist.Namespace, rc.ClusterID, "got error on fetching se, will retry updating")
			}
		}
		op = "Update"
		ctxLogger.Infof(LogFormat, op, common.VirtualServiceResourceType, newCopy.Name, rc.ClusterID,
			fmt.Sprintf("existing virtualservice for cluster: %s VirtualService name=%s",
				rc.ClusterID, newCopy.Name))
		ctxLogger.Infof(format, op, exist.Spec.String(), newCopy.Spec.String())
		exist.Labels = newCopy.Labels
		exist.Annotations = newCopy.Annotations
		//nolint
		exist.Spec = newCopy.Spec
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Update(ctx, exist, metav1.UpdateOptions{})
		if err != nil {
			err = retryUpdatingVS(ctxLogger, ctx, newCopy, exist, namespace, rc, err, op)
		}
	}

	if err != nil {
		ctxLogger.Errorf(LogErrFormat, op, common.VirtualServiceResourceType, newCopy.Name, rc.ClusterID, err)
		return err
	}
	ctxLogger.Infof(LogFormat, op, common.VirtualServiceResourceType, newCopy.Name, rc.ClusterID, "ExportTo: "+strings.Join(newCopy.Spec.ExportTo, " ")+" Success")
	return nil
}

func retryUpdatingVS(ctxLogger *log.Entry, ctx context.Context, obj *v1alpha3.VirtualService,
	exist *v1alpha3.VirtualService, namespace string, rc *RemoteController, err error, op string) error {
	numRetries := 5
	if err != nil && k8sErrors.IsConflict(err) {
		for i := 0; i < numRetries; i++ {
			vsIdentity := ""
			if obj.Annotations != nil {
				vsIdentity = obj.Labels[common.GetWorkloadIdentifier()]
			}
			ctxLogger.Errorf(LogFormatNew, op, common.VirtualServiceResourceType, obj.Name, obj.Namespace,
				vsIdentity, rc.ClusterID, err.Error()+". will retry the update operation before adding back to the controller queue.")

			updatedVS, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().
				VirtualServices(namespace).Get(ctx, exist.Name, metav1.GetOptions{})
			if err != nil {
				ctxLogger.Infof(LogFormatNew, op, common.VirtualServiceResourceType, exist.Name, exist.Namespace,
					vsIdentity, rc.ClusterID, err.Error()+fmt.Sprintf(". Error getting virtualservice"))
				continue
			}

			ctxLogger.Infof(LogFormatNew, op, common.VirtualServiceResourceType, obj.Name, obj.Namespace,
				vsIdentity, rc.ClusterID, fmt.Sprintf("existingResourceVersion=%s resourceVersionUsedForUpdate=%s",
					updatedVS.ResourceVersion, obj.ResourceVersion))
			updatedVS.Spec = obj.Spec
			updatedVS.Labels = obj.Labels
			updatedVS.Annotations = obj.Annotations
			_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Update(ctx, updatedVS, metav1.UpdateOptions{})
			if err == nil {
				return nil
			}
		}
	}
	return err
}

func isDeadCluster(err error) bool {
	if err == nil {
		return false
	}
	isNoSuchHostErr, _ := regexp.MatchString("dial tcp: lookup(.*): no such host", err.Error())
	return isNoSuchHostErr
}

func logElapsedTimeForVirtualService(operation, clusterID string, virtualService *v1alpha3.VirtualService) func() {
	startTime := time.Now()
	return func() {
		var name string
		var namespace string
		if virtualService != nil {
			name = virtualService.Name
			namespace = virtualService.Namespace
		}
		log.Infof(LogFormatOperationTime,
			operation,
			common.VirtualServiceResourceType,
			name,
			namespace,
			clusterID,
			time.Since(startTime).Milliseconds())
	}
}

// nolint
func createVirtualServiceSkeleton(vs networkingV1Alpha3.VirtualService, name string, namespace string) *v1alpha3.VirtualService {
	return &v1alpha3.VirtualService{Spec: vs, ObjectMeta: metaV1.ObjectMeta{Name: name, Namespace: namespace}}
}

func deleteVirtualService(ctx context.Context, exist *v1alpha3.VirtualService, namespace string, rc *RemoteController) error {
	if exist == nil {
		return fmt.Errorf("the VirtualService passed was nil")
	}
	err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Delete(ctx, exist.Name, metaV1.DeleteOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return fmt.Errorf("either VirtualService was already deleted, or it never existed")
		}
		return err
	}
	return nil
}

// getBaseVirtualServiceForIngress generates the base virtual service for the ingress gateway
// The destinations should be added separately, this func does not have the context of the destinations.
// TODO: Support for multiple hosts and sniHosts. Will be needed when additional endpoints are generated using GTPs
func getBaseVirtualServiceForIngress(host, sniHost string) (*v1alpha3.VirtualService, error) {

	if host == "" {
		return nil, fmt.Errorf("host is empty")
	}

	if sniHost == "" {
		return nil, fmt.Errorf("sniHost is empty")
	}

	gateways := common.GetVSRoutingGateways()
	if len(gateways) == 0 {
		return nil, fmt.Errorf("no gateways configured for ingress virtual service")
	}

	vs := networkingV1Alpha3.VirtualService{
		// We are using the SNI host in hosts field as they need to match
		Hosts:    []string{sniHost},
		Gateways: gateways,
		ExportTo: common.GetIngressVSExportToNamespace(),
		Tls: []*networkingV1Alpha3.TLSRoute{
			{
				Match: []*networkingV1Alpha3.TLSMatchAttributes{
					{
						Port:     common.DefaultMtlsPort,
						SniHosts: []string{sniHost},
					},
				},
			},
		},
	}

	// Explicitly labeling the VS for routing
	vsLabels := map[string]string{
		vsRoutingLabel: "enabled",
	}

	return &v1alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      host + "-routing-vs",
			Namespace: common.GetSyncNamespace(),
			Labels:    vsLabels,
		},
		Spec: vs,
	}, nil
}

// generateIngressVirtualServiceForDeployment generates the base virtual service for a given deployment
// and adds it to the sourceIngressVirtualService map
// sourceIngressVirtualService is a map of globalFQDN (.mesh) endpoint to VirtualService
func generateIngressVirtualServiceForDeployment(
	deployment *k8sAppsV1.Deployment,
	sourceIngressVirtualService map[string]*v1alpha3.VirtualService) error {
	if deployment == nil {
		return fmt.Errorf("deployment is nil")
	}
	workloadIdentityKey := common.GetWorkloadIdentifier()
	cname := common.GetCname(deployment, workloadIdentityKey, common.GetHostnameSuffix())
	if cname == "" {
		return fmt.Errorf("cname is empty")
	}
	sniHost, err := generateSNIHost(cname)
	if err != nil {
		return err
	}
	baseVS, err := getBaseVirtualServiceForIngress(cname, sniHost)
	if err != nil {
		return err
	}
	sourceIngressVirtualService[cname] = baseVS
	return nil
}

// generateIngressVirtualServiceForRollout generates the base virtual service for a given rollout
// and adds it to the sourceIngressVirtualService map
// sourceIngressVirtualService is a map of globalFQDN (.mesh) endpoint to VirtualService
// TODO: Generate VS for canary and blue green strategies
func generateIngressVirtualServiceForRollout(
	rollout *argo.Rollout,
	sourceIngressVirtualService map[string]*v1alpha3.VirtualService) error {
	if rollout == nil {
		return fmt.Errorf("rollout is nil")
	}
	workloadIdentityKey := common.GetWorkloadIdentifier()
	cname := common.GetCnameForRollout(rollout, workloadIdentityKey, common.GetHostnameSuffix())
	if cname == "" {
		return fmt.Errorf("cname is empty")
	}
	sniHost, err := generateSNIHost(cname)
	if err != nil {
		return err
	}
	baseVS, err := getBaseVirtualServiceForIngress(cname, sniHost)
	if err != nil {
		return err
	}
	sourceIngressVirtualService[cname] = baseVS
	return nil
}

// generateSNIHost generates the SNI host for the virtual service in the format outbound_.80_._.<fqdn>
// Example: outbound_.80_._.httpbin.global.mesh
func generateSNIHost(fqdn string) (string, error) {
	if fqdn == "" {
		return "", fmt.Errorf("fqdn is empty")
	}
	return fmt.Sprintf("outbound_.%d_._.%s", common.DefaultServiceEntryPort, fqdn), nil
}

// addUpdateVirtualServicesForSourceIngress adds or updates the cross-cluster routing VirtualServices exported to
// istio-system namespace.
func addUpdateVirtualServicesForSourceIngress(
	ctx context.Context,
	ctxLogger *log.Entry,
	remoteRegistry *RemoteRegistry,
	sourceServices map[string]map[string]*k8sV1.Service,
	sourceIngressVirtualService map[string]*v1alpha3.VirtualService,
	sourceDeployment map[string]*k8sAppsV1.Deployment,
	sourceRollout map[string]*argo.Rollout) error {

	if remoteRegistry == nil {
		return fmt.Errorf("remoteRegistry is nil")
	}

	for sourceCluster, serviceInstance := range sourceServices {

		destinationHostPort := make(map[string]uint32)
		if serviceInstance[common.Deployment] != nil {
			host := serviceInstance[common.Deployment].Name + "." + serviceInstance[common.Deployment].Namespace + ".svc.cluster.local"
			protocolPortMap := common.GetMeshPortsForDeployments(sourceCluster, serviceInstance[common.Deployment], sourceDeployment[sourceCluster])
			if port, ok := protocolPortMap[constUtil.Http]; ok {
				destinationHostPort[host] = port
			}
		}
		if serviceInstance[common.Rollout] != nil {
			host := serviceInstance[common.Rollout].Name + "." + serviceInstance[common.Rollout].Namespace + ".svc.cluster.local"
			protocolPortMap := GetMeshPortsForRollout(sourceCluster, serviceInstance[common.Deployment], sourceRollout[sourceCluster])
			if port, ok := protocolPortMap[constUtil.Http]; ok {
				destinationHostPort[host] = port
			}
		}

		if len(destinationHostPort) == 0 {
			return fmt.Errorf("no destination found for the ingress virtualservice")
		}

		rc := remoteRegistry.GetRemoteController(sourceCluster)

		if rc == nil {
			ctxLogger.Warnf(common.CtxLogFormat, "addUpdateVirtualServicesForSourceIngress",
				"", "", sourceCluster, "remote controller not initialized on this cluster")
			continue
		}

		for fqdn, virtualService := range sourceIngressVirtualService {
			if len(virtualService.Spec.Tls) == 0 {
				return fmt.Errorf("no TLSRoute found in the ingress virtualservice with host %s", fqdn)
			}
			if len(virtualService.Spec.Tls) > 1 {
				return fmt.Errorf("more than one TLSRoute found in the ingress virtualservice with host %s", fqdn)
			}
			routeDestinations := make([]*networkingV1Alpha3.RouteDestination, 0)
			for host, port := range destinationHostPort {
				routeDestination := &networkingV1Alpha3.RouteDestination{
					Destination: &networkingV1Alpha3.Destination{
						Host: host,
						Port: &networkingV1Alpha3.PortSelector{
							Number: port,
						},
					},
				}
				routeDestinations = append(routeDestinations, routeDestination)
			}
			virtualService.Spec.Tls[0].Route = routeDestinations

			existingVS, err := getExistingVS(ctxLogger, ctx, rc, virtualService.Name)
			if err != nil {
				ctxLogger.Warn(err.Error())
			}

			ctxLogger.Infof(common.CtxLogFormat, "addUpdateVirtualServicesForSourceIngress",
				virtualService.Name, virtualService.Namespace, sourceCluster, "Add/Update ingress virtualservice")
			err = addUpdateVirtualService(
				ctxLogger, ctx, virtualService, existingVS, common.GetSyncNamespace(), rc, remoteRegistry)

		}
	}
	return nil
}
