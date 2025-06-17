package clusters

import (
	"context"
	"fmt"
	"k8s.io/utils/strings/slices"
	"net"
	"strconv"
	"strings"
	"time"

	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"google.golang.org/protobuf/types/known/durationpb"

	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	log "github.com/sirupsen/logrus"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DestinationRuleHandler responsible for handling Add/Update/Delete events for
// DestinationRule resources
type DestinationRuleHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

// getDestinationRule constructs a DestinationRule for a given ServiceEntry, applying traffic policies
// such as load balancing, outlier detection, and connection pool settings based on various configurations.
func getDestinationRule(se *networkingV1Alpha3.ServiceEntry,
	locality string,
	gtpTrafficPolicy *model.TrafficPolicy,
	outlierDetection *v1.OutlierDetection,
	clientConnectionSettings *v1.ClientConnectionConfig,
	currentDR *v1alpha3.DestinationRule,
	eventResourceType string,
	ctxLogger *log.Entry,
	event admiral.EventType,
	doDRUpdateForInClusterRouting bool) *networkingV1Alpha3.DestinationRule {

	dr := &networkingV1Alpha3.DestinationRule{
		Host: se.Hosts[0],
		TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
			Tls: &networkingV1Alpha3.ClientTLSSettings{
				Mode: networkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
			},
			LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
				LbPolicy: &networkingV1Alpha3.LoadBalancerSettings_Simple{
					Simple: networkingV1Alpha3.LoadBalancerSettings_LEAST_REQUEST,
				},
				WarmupDurationSecs: &duration.Duration{Seconds: common.GetDefaultWarmupDurationSecs()},
			},
		},
	}

	// In Operator mode, exportTo will be present in the se as well
	if common.EnableExportTo(dr.Host) || common.IsAdmiralOperatorMode() {
		dr.ExportTo = se.ExportTo
	}

	derivedOutlierDetection := getOutlierDetection(se, gtpTrafficPolicy, outlierDetection, common.DisableDefaultAutomaticFailover())
	if derivedOutlierDetection != nil {
		dr.TrafficPolicy.OutlierDetection = derivedOutlierDetection
	}

	clientConnectionSettingsOverride := getClientConnectionPoolOverrides(clientConnectionSettings)
	if clientConnectionSettingsOverride != nil {
		dr.TrafficPolicy.ConnectionPool = clientConnectionSettingsOverride
	}

	if common.EnableActivePassive() &&
		((eventResourceType != common.GTP) || (eventResourceType == common.GTP && event != admiral.Delete)) {

		// This is present to avoid adding the LocalityLbSetting to DRs associated to application which to do
		// not need it
		if distribute := calculateDistribution(se, currentDR); len(distribute) != 0 {
			dr.TrafficPolicy.LoadBalancer.LocalityLbSetting = &networkingV1Alpha3.LocalityLoadBalancerSetting{
				Distribute: distribute,
			}
		}
	}

	if len(locality) == 0 {
		log.Warnf(LogErrFormat, "Process", "GlobalTrafficPolicy", dr.Host, "", "Skipping gtp processing, locality of the cluster nodes cannot be determined. Is this minikube?")
		return dr
	}

	if gtpTrafficPolicy == nil {
		return dr
	}

	// Pin the DR to remote region if in-cluster VS routing is enabled
	// This will only be done if a GTP exists in the NS because by default
	// we are active/passive and the previous state of the DR is lost if we pin the DR.
	if doDRUpdateForInClusterRouting {
		// Perform DR pinning only if it is multi-region
		var err error
		if isSEMultiRegion(se) {
			dr.TrafficPolicy.LoadBalancer.LocalityLbSetting, err = getLocalityLBSettings(locality)
			if err == nil {
				return dr
			} else {
				ctxLogger.Errorf(common.CtxLogFormat,
					"doDRUpdateForInClusterRouting", "", "", "",
					fmt.Sprintf("error getting locality LB settings: %v", err))
			}
		}
	}

	if len(gtpTrafficPolicy.Target) == 0 {
		dr.TrafficPolicy.LoadBalancer = &networkingV1Alpha3.LoadBalancerSettings{
			LbPolicy:           &networkingV1Alpha3.LoadBalancerSettings_Simple{Simple: networkingV1Alpha3.LoadBalancerSettings_LEAST_REQUEST},
			WarmupDurationSecs: &duration.Duration{Seconds: common.GetDefaultWarmupDurationSecs()},
		}
		return dr
	}

	if gtpTrafficPolicy.LbType != model.TrafficPolicy_FAILOVER {
		dr.TrafficPolicy.LoadBalancer.LocalityLbSetting = &networkingV1Alpha3.LocalityLoadBalancerSetting{}
		return dr
	}

	targetTrafficMap := make(map[string]uint32)
	for _, tg := range gtpTrafficPolicy.Target {
		//skip 0 values from GTP as that's implicit for locality settings
		if tg.Weight != int32(0) {
			targetTrafficMap[tg.Region] = uint32(tg.Weight)
		}
	}

	dr.TrafficPolicy.LoadBalancer.LocalityLbSetting = &networkingV1Alpha3.LocalityLoadBalancerSetting{
		Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
			{
				From: locality + "/*",
				To:   targetTrafficMap,
			},
		},
	}

	return dr
}

func calculateDistribution(se *networkingV1Alpha3.ServiceEntry, currentDR *v1alpha3.DestinationRule) []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute {
	distribute := make([]*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute, 0)

	// There are two conditions here:
	// 1. If there is only one endpoint in the SE it means that the application is only available in one region.
	// 	  We will configure the traffic to be routed from all the regions to this region if it is a new application
	//	  and maintain the same configuration if we have already converted it to an A/P before.
	// 2. If there are multiple endpoints in the SE it means that the application is available in multiple regions.
	//    We then check the DR cache to check which is the region that is primary at the moment and retain that information.
	// NOTE: We are ignoring events from the GTP controller as they will be overriden further in the code
	numOfSEendpoints := len(se.Endpoints)
	if numOfSEendpoints == 1 {
		defaultAPDistribution := &networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
			From: "*",
			To:   map[string]uint32{se.Endpoints[0].Locality: 100},
		}

		if currentDR != nil {
			if &currentDR.Spec != (&networkingV1Alpha3.DestinationRule{}) &&
				currentDR.Spec.TrafficPolicy != nil &&
				currentDR.Spec.TrafficPolicy.LoadBalancer != nil {
				if currentDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting == nil {
					// If the application is Active-Active and only in one region convert to Active-Passive
					distribute = append(distribute, defaultAPDistribution)
				}

				if currentDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting != nil &&
					currentDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute != nil &&
					len(currentDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute) == 1 &&
					currentDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute[0].From == "*" {
					// Maintain the same configuration if we have already converted it to an Active-Passive before
					distribute = append(distribute, currentDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute...)
				}
			}
		} else {
			// Configure the traffic to be routed from all the regions to this region if it is a new application
			distribute = append(distribute, defaultAPDistribution)
		}
	} else if numOfSEendpoints != 0 {
		if currentDR != nil {
			if &currentDR.Spec != (&networkingV1Alpha3.DestinationRule{}) &&
				currentDR.Spec.TrafficPolicy != nil &&
				currentDR.Spec.TrafficPolicy.LoadBalancer != nil &&
				currentDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting != nil &&
				currentDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute != nil {
				distribute = append(distribute, currentDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute...)
			}
		}
	}

	return distribute
}

func getClientConnectionPoolOverrides(clientConnectionSettings *v1.ClientConnectionConfig) *networkingV1Alpha3.ConnectionPoolSettings {

	connectionPoolSettings := &networkingV1Alpha3.ConnectionPoolSettings{
		Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
			MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
		},
	}

	if clientConnectionSettings == nil {
		return connectionPoolSettings
	}

	if clientConnectionSettings.Spec.ConnectionPool.Http != nil {

		if clientConnectionSettings.Spec.ConnectionPool.Http.Http2MaxRequests > 0 {
			connectionPoolSettings.Http.Http2MaxRequests =
				clientConnectionSettings.Spec.ConnectionPool.Http.Http2MaxRequests
		}

		if clientConnectionSettings.Spec.ConnectionPool.Http.MaxRequestsPerConnection > 0 {
			connectionPoolSettings.Http.MaxRequestsPerConnection =
				clientConnectionSettings.Spec.ConnectionPool.Http.MaxRequestsPerConnection
		}

		if clientConnectionSettings.Spec.ConnectionPool.Http.IdleTimeout != "" {
			idleTimeout, err := time.ParseDuration(clientConnectionSettings.Spec.ConnectionPool.Http.IdleTimeout)
			if err != nil {
				log.Warnf(
					LogErrFormat, "ClientConnectionConfigOverride", common.ClientConnectionConfig,
					clientConnectionSettings.Name, "", "failed parsing IdleTimeout due to error: "+err.Error())
			} else {
				connectionPoolSettings.Http.IdleTimeout = durationpb.New(idleTimeout)
			}
		}
	}

	if clientConnectionSettings.Spec.ConnectionPool.Tcp != nil {
		if clientConnectionSettings.Spec.ConnectionPool.Tcp.MaxConnectionDuration != "" {
			maxConnectionDuration, err := time.ParseDuration(clientConnectionSettings.Spec.ConnectionPool.Tcp.MaxConnectionDuration)
			if err != nil {
				log.Warnf(
					LogErrFormat, "ClientConnectionConfigOverride", common.ClientConnectionConfig,
					clientConnectionSettings.Name, "", "failed parsing MaxConnectionDuration due to error: "+err.Error())
			} else {
				connectionPoolSettings.Tcp = &networkingV1Alpha3.ConnectionPoolSettings_TCPSettings{
					MaxConnectionDuration: durationpb.New(maxConnectionDuration),
				}
			}
		}
	}

	return connectionPoolSettings
}

func getOutlierDetection(
	se *networkingV1Alpha3.ServiceEntry,
	gtpTrafficPolicy *model.TrafficPolicy,
	outlierDetectionCrd *v1.OutlierDetection,
	disableDefaultAutomaticFailover bool) *networkingV1Alpha3.OutlierDetection {
	if disableDefaultAutomaticFailover {
		if (outlierDetectionCrd == nil || (outlierDetectionCrd.Spec.OutlierConfig == nil)) &&
			(gtpTrafficPolicy == nil || gtpTrafficPolicy.OutlierDetection == nil) {
			return &networkingV1Alpha3.OutlierDetection{
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 0},
				Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
			}
		}
	}

	// When only one endpoint present in the Service Entry:
	// 1. It points to kubernetes service (ends in svc.cluster.local)
	// 2. It is an IPv4 address
	// Then return nil
	if len(se.Endpoints) == 1 &&
		(strings.Contains(se.Endpoints[0].Address, common.DotLocalDomainSuffix) ||
			net.ParseIP(se.Endpoints[0].Address).To4() != nil) {
		log.Infof("service entry endpoint (%v) contains only one endpoint which "+
			"is either kubernetes service or ipv4 address. Not setting outlier", se.Endpoints)
		return nil
	}
	outlierDetection := getOutlierDetectionSkeleton(disableDefaultAutomaticFailover)
	//Give priority to outlier detection crd than GTP. Eventually support for outlier detection via GTP will be stopped.
	if outlierDetectionCrd != nil && outlierDetectionCrd.Spec.OutlierConfig != nil {
		outlierDetection.ConsecutiveGatewayErrors = &wrappers.UInt32Value{Value: outlierDetectionCrd.Spec.OutlierConfig.ConsecutiveGatewayErrors}
		outlierDetection.Interval = &duration.Duration{Seconds: outlierDetectionCrd.Spec.OutlierConfig.Interval}
		outlierDetection.BaseEjectionTime = &duration.Duration{Seconds: outlierDetectionCrd.Spec.OutlierConfig.BaseEjectionTime}
	} else if gtpTrafficPolicy != nil && gtpTrafficPolicy.OutlierDetection != nil {
		setDefaultValuesOfOutlierDetection(outlierDetection)
		if gtpTrafficPolicy.OutlierDetection.BaseEjectionTime > 0 {
			outlierDetection.BaseEjectionTime = &duration.Duration{
				Seconds: gtpTrafficPolicy.OutlierDetection.BaseEjectionTime,
			}
		}
		if gtpTrafficPolicy.OutlierDetection.ConsecutiveGatewayErrors > 0 {
			outlierDetection.ConsecutiveGatewayErrors = &wrappers.UInt32Value{
				Value: gtpTrafficPolicy.OutlierDetection.ConsecutiveGatewayErrors,
			}
		}
		if gtpTrafficPolicy.OutlierDetection.Interval > 0 {
			outlierDetection.Interval = &duration.Duration{
				Seconds: gtpTrafficPolicy.OutlierDetection.Interval,
			}
		}
	}

	if len(se.Endpoints) == 1 {
		//Scenario 1: Only one endpoint present and is remote - outlier detection with 33% ejection (protection against zone specific issues)
		//Making the %33 as 34% will eject 2 endpoints, %33 will eject one
		outlierDetection.MaxEjectionPercent = 33
	} else {
		//Scenario 2: Two endpoints present each with different locality and both remote - outlier detection with 100% ejection
		//Scenario 3: Two endpoints present each with different locality with one local and other remote - outlier detection with 100% ejection
		//for service entries with more than 2 endpoints eject 100% to failover to other endpoint within or outside the same region
		outlierDetection.MaxEjectionPercent = 100
	}
	return outlierDetection
}

func getOutlierDetectionSkeleton(disableDefaultAutomaticFailover bool) *networkingV1Alpha3.OutlierDetection {
	if disableDefaultAutomaticFailover {
		return &networkingV1Alpha3.OutlierDetection{
			// The default Consecutive5XXErrors is set to 5 in envoy, setting to 0 disables 5XX error outlier detection so that ConsecutiveGatewayErrors rule can get evaluated
			Consecutive_5XxErrors: &wrappers.UInt32Value{Value: 0},
		}
	}
	return &networkingV1Alpha3.OutlierDetection{
		BaseEjectionTime:         &duration.Duration{Seconds: DefaultBaseEjectionTime},
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: DefaultConsecutiveGatewayErrors},
		// The default Consecutive5XXErrors is set to 5 in envoy, setting to 0 disables 5XX error outlier detection so that ConsecutiveGatewayErrors rule can get evaluated
		Consecutive_5XxErrors: &wrappers.UInt32Value{Value: DefaultConsecutive5xxErrors},
		Interval:              &duration.Duration{Seconds: DefaultInterval},
	}
}

func setDefaultValuesOfOutlierDetection(outlierDetection *networkingV1Alpha3.OutlierDetection) {
	outlierDetection.BaseEjectionTime = &duration.Duration{Seconds: DefaultBaseEjectionTime}
	outlierDetection.ConsecutiveGatewayErrors = &wrappers.UInt32Value{Value: DefaultConsecutiveGatewayErrors}
	outlierDetection.Interval = &duration.Duration{Seconds: DefaultInterval}
}

func (dh *DestinationRuleHandler) Added(ctx context.Context, obj *v1alpha3.DestinationRule) error {
	if commonUtil.IsAdmiralReadOnly() {
		return nil
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		return nil
	}
	txId := common.FetchTxIdOrGenNew(ctx)
	ctxLogger := log.WithFields(log.Fields{
		"type": "destinationRule",
		"txId": txId,
		"op":   "Add",
	})
	return handleDestinationRuleEvent(ctxLogger, ctx, obj, dh, common.Add, common.DestinationRuleResourceType)
}

func (dh *DestinationRuleHandler) Updated(ctx context.Context, obj *v1alpha3.DestinationRule) error {
	if commonUtil.IsAdmiralReadOnly() {
		return nil
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		return nil
	}
	txId := common.FetchTxIdOrGenNew(ctx)
	ctxLogger := log.WithFields(log.Fields{
		"type": "destinationRule",
		"txId": txId,
		"op":   "Update",
	})
	return handleDestinationRuleEvent(ctxLogger, ctx, obj, dh, common.Update, common.DestinationRuleResourceType)
}

func (dh *DestinationRuleHandler) Deleted(ctx context.Context, obj *v1alpha3.DestinationRule) error {
	if commonUtil.IsAdmiralReadOnly() {
		return nil
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		return nil
	}
	txId := common.FetchTxIdOrGenNew(ctx)
	ctxLogger := log.WithFields(log.Fields{
		"type": "destinationRule",
		"txId": txId,
		"op":   "Delete",
	})
	return handleDestinationRuleEvent(ctxLogger, ctx, obj, dh, common.Delete, common.DestinationRuleResourceType)
}

func handleDestinationRuleEvent(ctxLogger *log.Entry, ctx context.Context, obj *v1alpha3.DestinationRule, dh *DestinationRuleHandler, event common.Event, resourceType common.ResourceType) error {
	var (
		//nolint
		destinationRule      = obj.Spec
		clusterId            = dh.ClusterID
		syncNamespace        = common.GetSyncNamespace()
		r                    = dh.RemoteRegistry
		dependentClusters    = r.AdmiralCache.CnameDependentClusterCache.Get(destinationRule.Host).Copy()
		allDependentClusters = make(map[string]string)
	)

	if len(dependentClusters) > 0 {
		util.MapCopy(allDependentClusters, dependentClusters)
		allDependentClusters[clusterId] = clusterId
		for _, dependentCluster := range allDependentClusters {
			rc := r.GetRemoteController(dependentCluster)
			if rc == nil {
				return fmt.Errorf(LogFormat, "Event", resourceType, obj.Name, dependentCluster, "remote controller not initialized for cluster")
			}
			if rc.DestinationRuleController == nil {
				return fmt.Errorf(LogFormat, "Event", resourceType, obj.Name, dependentCluster, "DestinationRule controller not initialized for cluster")
			}
			if event == common.Delete {
				err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Delete(ctx, obj.Name, metaV1.DeleteOptions{})
				if err != nil {
					if k8sErrors.IsNotFound(err) {
						log.Infof(LogFormat, "Delete", resourceType, obj.Name, clusterId, "Either DestinationRule was already deleted, or it never existed")
					} else {
						log.Errorf(LogErrFormat, "Delete", resourceType, obj.Name, clusterId, err)
					}
				} else {
					log.Infof(LogFormat, "Delete", resourceType, obj.Name, clusterId, "Success")
				}
			} else {
				exist, _ := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(ctx, obj.Name, metaV1.GetOptions{})
				//copy destination rule only to other clusters
				if dependentCluster != clusterId {
					addUpdateDestinationRule(ctxLogger, ctx, obj, exist, syncNamespace, rc, r)
				}
			}
		}
		return nil
	} else {
		log.Infof(LogFormat, "Event", resourceType, obj.Name, clusterId, "No dependent clusters found")
	}

	//copy the DestinationRule `as is` if they are not generated by Admiral
	remoteClusters := r.GetClusterIds()
	for _, ClusterID := range remoteClusters {
		if ClusterID != clusterId {
			rc := r.GetRemoteController(ClusterID)
			if rc == nil {
				return fmt.Errorf(LogFormat, "Event", resourceType, obj.Name, ClusterID, "remote controller not initialized for cluster")
			}
			if rc.DestinationRuleController == nil {
				return fmt.Errorf(LogFormat, "Event", resourceType, obj.Name, ClusterID, "DestinationRule controller not initialized for cluster")
			}
			if event == common.Delete {
				err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Delete(ctx, obj.Name, metaV1.DeleteOptions{})
				if err != nil {
					if k8sErrors.IsNotFound(err) {
						log.Infof(LogFormat, "Delete", "DestinationRule", obj.Name, clusterId, "Either DestinationRule was already deleted, or it never existed")
					} else {
						log.Errorf(LogErrFormat, "Delete", "DestinationRule", obj.Name, clusterId, err)
					}
				} else {
					log.Infof(LogFormat, "Delete", "DestinationRule", obj.Name, clusterId, "Success")
				}

			} else {
				exist, _ := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(ctx, obj.Name, metaV1.GetOptions{})
				addUpdateDestinationRule(ctxLogger, ctx, obj, exist, syncNamespace, rc, r)
			}
		}
	}
	return nil
}

func addUpdateDestinationRule(
	ctxLogger *log.Entry,
	ctx context.Context,
	dr *v1alpha3.DestinationRule,
	exist *v1alpha3.DestinationRule,
	namespace string,
	rc *RemoteController, rr *RemoteRegistry) error {
	var err error
	var op string
	var drAlreadyExists bool
	obj := copyDestinationRule(dr)
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations["app.kubernetes.io/created-by"] = "admiral"

	//Check if DR has the admiral.io/vs-routing label
	// If it does, skip adding ExportTo since it is already set to "istio-system" only
	// The DR created for routing cross cluster traffic should only be exported to istio-system
	skipAddingExportTo := false
	if obj.Labels != nil && obj.Labels[common.VSRoutingLabel] == "enabled" {
		skipAddingExportTo = true
	}

	// At this step we check to make sure the DR does not already have an exportTo value before setting the exportTo value
	// This is because there are two ways to enter this function
	// 1. Through modifyse, in which case obj will already have exportTo filled and we don't want to do a repeat call of getSortedDependentNamespaces
	// 2. Through the flow where we copy customer created DRs to other clusters, in which case it shouldn't have exportTo set and we need to calculate it here.
	if common.EnableExportTo(obj.Spec.Host) && len(obj.Spec.ExportTo) == 0 && !skipAddingExportTo {
		sortedDependentNamespaces := getSortedDependentNamespaces(rr.AdmiralCache, obj.Spec.Host, rc.ClusterID, ctxLogger, false)
		obj.Spec.ExportTo = sortedDependentNamespaces
	}
	drIsNew := exist == nil || exist.Name == "" || exist.Spec.Host == ""
	if drIsNew {
		obj.Namespace = namespace
		obj.ResourceVersion = ""
		_, err = rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Create(ctx, obj, metaV1.CreateOptions{})
		if k8sErrors.IsAlreadyExists(err) {
			// op=%v name=%v namespace=%s cluster=%s message=%v
			ctxLogger.Infof(common.CtxLogFormat, "addUpdateDestinationRule", obj.Name, obj.Namespace, rc.ClusterID, "object already exists. Will update instead")
			drAlreadyExists = true
		} else {
			return err
		}
		op = "Add"
	}
	if !drIsNew || drAlreadyExists {
		if drAlreadyExists {
			exist, err = rc.DestinationRuleController.IstioClient.
				NetworkingV1alpha3().
				DestinationRules(namespace).
				Get(ctx, obj.Name, metav1.GetOptions{})
			if err != nil {
				// when there is an error, assign exist to obj,
				// which will fail in the update operation, but will be retried
				// in the retry logic
				exist = obj
				ctxLogger.Warnf(common.CtxLogFormat, "Update", exist.Name, exist.Namespace, rc.ClusterID, "got error on fetching destinationrule, will retry updating")
			}
		}
		exist.Labels = obj.Labels
		exist.Annotations = obj.Annotations
		//nolint
		exist.Spec = obj.Spec
		op = "Update"
		_, err = rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Update(ctx, exist, metaV1.UpdateOptions{})
		if err != nil {
			err = retryUpdatingDR(ctxLogger, ctx, exist, namespace, rc, err)
		}
	}

	if err != nil {
		ctxLogger.Errorf(LogErrFormat, op, "DestinationRule", obj.Name, rc.ClusterID, err)
		return err
	} else {
		ctxLogger.Infof(LogFormat, op, "DestinationRule", obj.Name, rc.ClusterID, "Success")
	}
	return nil
}

func deleteDestinationRule(ctx context.Context, exist *v1alpha3.DestinationRule, namespace string, rc *RemoteController) error {
	if exist != nil {
		err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Delete(ctx, exist.Name, metaV1.DeleteOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				log.Infof(LogFormat, "Delete", "DestinationRule", exist.Name, rc.ClusterID, "Either DestinationRule was already deleted, or it never existed")
			} else {
				log.Errorf(LogErrFormat, "Delete", "DestinationRule", exist.Name, rc.ClusterID, err)
				return err
			}
		} else {
			log.Infof(LogFormat, "Delete", "DestinationRule", exist.Name, rc.ClusterID, "Success")
		}
	}
	return nil
}

// nolint
func createDestinationRuleSkeleton(dr networkingV1Alpha3.DestinationRule, name string, namespace string) *v1alpha3.DestinationRule {
	return &v1alpha3.DestinationRule{Spec: dr, ObjectMeta: metaV1.ObjectMeta{Name: name, Namespace: namespace}}
}

func retryUpdatingDR(
	ctxLogger *log.Entry, ctx context.Context,
	exist *v1alpha3.DestinationRule, namespace string,
	rc *RemoteController, err error) error {
	numRetries := 5
	if err != nil {
		if k8sErrors.IsConflict(err) {
			for i := 1; i <= numRetries; i++ {
				ctxLogger.Errorf(common.CtxLogFormat, "Update",
					exist.Name, exist.Namespace, rc.ClusterID, fmt.Sprintf("error=%v retrying=%d/%d", err.Error(), i, numRetries))
				updatedDR, err := rc.DestinationRuleController.IstioClient.
					NetworkingV1alpha3().
					DestinationRules(namespace).
					Get(ctx, exist.Name, metav1.GetOptions{})
				if err != nil {
					ctxLogger.Errorf(common.CtxLogFormat, "Update",
						exist.Name, exist.Namespace, rc.ClusterID, fmt.Sprintf("error=%v", err.Error()))
					continue
				}
				ctxLogger.Infof(common.CtxLogFormat, "Update", exist.Name, exist.Namespace, rc.ClusterID,
					fmt.Sprintf("existingResourceVersion=%s resourceVersionUsedForUpdate=%s", updatedDR.ResourceVersion, exist.ResourceVersion))
				//nolint
				updatedDR.Spec = exist.Spec
				updatedDR.Labels = exist.Labels
				updatedDR.Annotations = exist.Annotations
				_, err = rc.DestinationRuleController.IstioClient.
					NetworkingV1alpha3().
					DestinationRules(namespace).
					Update(ctx, updatedDR, metaV1.UpdateOptions{})
				if err == nil {
					return nil
				}
			}
		} else {
			ctxLogger.Errorf(common.CtxLogFormat, "Update", exist.Name, exist.Namespace, rc.ClusterID, "Not retrying error="+err.Error())
		}
	}
	return err
}

func addNLBIdleTimeout(ctx context.Context, ctxLogger *log.Entry, rr *RemoteRegistry, rc *RemoteController, obj *networkingV1Alpha3.DestinationRule, sourceClusterForVSRouting string, sourceIdentity string) *networkingV1Alpha3.DestinationRule {
	// Add the idle timeout to the DR if NLB is enabled for the destination/server cluster before it is copied anywhere

	ctxLogger.Infof(common.CtxLogFormat, "AddNLBIdleTimeout", obj.Host, "", rc.ClusterID, "")
	var sourceClusters []string
	if sourceClusterForVSRouting != "" {
		sourceClusters = []string{sourceClusterForVSRouting}
	} else {
		if rr.AdmiralCache.CnameClusterCache != nil {
			if rr.AdmiralCache.CnameClusterCache.Get(obj.Host) != nil {
				sourceClusters = rr.AdmiralCache.CnameClusterCache.Get(obj.Host).CopyJustValues()
			}
		}
	}

	ctxLogger.Infof(common.CtxLogFormat, "AddNLBIdleTimeout", obj.Host, "", rc.ClusterID, "srcClusters="+strings.Join(sourceClusters, ","))
	/*
		Validate default LB label, source and destination clusters, validate if NLB timeout needed or not
		If default is NLB, and all source cluster in CLB exclude list then return false
		If default is CLB, and any of 1 or more source clusters in NLB include list then return true
		If default is NLB, not all source cluster in CLB exclude list then return true
		If asset level migration enable then return true
		We can't support multiple LB timeout as DR is one. And most case endpoint/LB will be two. So we need to pick one.
		Excluding mailchimp all are fine to pick any one. All mailchimp will be in exclude list, need to put right cluster in DB
	*/
	if !IsNLBTimeoutNeeded(sourceClusters, rr.AdmiralCache.NLBEnabledCluster, rr.AdmiralCache.CLBEnabledCluster, sourceIdentity) {
		ctxLogger.Infof(common.CtxLogFormat, "AddNLBIdleTimeout", obj.Host, "", rc.ClusterID, "Skipped")
		return obj
	}

	// Get rc of the sourceCluster - if there are multiple source clusters with different NLB timeout values this may not be accurate
	sourceClusterRc := rr.GetRemoteController(sourceClusters[0])
	if sourceClusterRc != nil {
		timeout, scErr := sourceClusterRc.ServiceController.Cache.GetSingleLoadBalancerTimeout(common.GetAdmiralParams().NLBIngressLabel, common.NamespaceIstioSystem)
		ctxLogger.Infof(common.CtxLogFormat, "AddNLBIdleTimeoutScErr", obj.Host, "", rc.ClusterID, scErr)
		// Current behavior is to set to 350 if configured timeout is less than 350, otherwise match
		// In the future it should just match whatever is configured
		if scErr != nil {
			// If the istio ingress gateway service wasn't found, it was probably not yet processed and placed in the cache
			// Make a k8s call to get it and put in cache
			igw, kcErr := sourceClusterRc.ServiceController.K8sClient.CoreV1().Services(common.NamespaceIstioSystem).Get(ctx, common.NLBIstioIngressGatewayLabelValue, metaV1.GetOptions{})
			if kcErr != nil {
				ctxLogger.Errorf(common.CtxLogFormat, "AddNLBIdleTimeoutKcErr", obj.Host, "", rc.ClusterID, "no nlb service found for nlb-enabled cluster")
				return obj
			} else {
				timeout = igw.Annotations[common.NLBIdleTimeoutAnnotation]
				sourceClusterRc.ServiceController.Cache.Put(igw)
			}
		}
		obj = buildNLBTimeout(ctxLogger, timeout, obj, rc)
	}
	return obj
}

func buildNLBTimeout(ctxLogger *log.Entry, timeout string, dr *networkingV1Alpha3.DestinationRule, rc *RemoteController) *networkingV1Alpha3.DestinationRule {
	timeoutInt, stErr := strconv.Atoi(timeout)
	if stErr != nil {
		ctxLogger.Errorf(common.CtxLogFormat, "AddNLBIdleTimeoutStErr", dr.Host, "", rc.ClusterID, "failed to convert timeout value to int on istio-ingressgateway-nlb")
		timeoutInt = common.NLBDefaultTimeoutSeconds
	}
	if timeoutInt < common.NLBDefaultTimeoutSeconds {
		timeoutInt = common.NLBDefaultTimeoutSeconds
	}
	if dr.TrafficPolicy == nil {
		dr.TrafficPolicy = &networkingV1Alpha3.TrafficPolicy{}
	}
	if dr.TrafficPolicy.ConnectionPool == nil {
		dr.TrafficPolicy.ConnectionPool = &networkingV1Alpha3.ConnectionPoolSettings{}
	}
	if dr.TrafficPolicy.ConnectionPool.Tcp == nil {
		dr.TrafficPolicy.ConnectionPool.Tcp = &networkingV1Alpha3.ConnectionPoolSettings_TCPSettings{}
	}
	dr.TrafficPolicy.ConnectionPool.Tcp.IdleTimeout = durationpb.New(time.Duration(timeoutInt * int(time.Second)))
	ctxLogger.Infof(common.CtxLogFormat, "AddNLBIdleTimeoutSuccess", dr.Host, "", rc.ClusterID, timeoutInt)
	return dr
}

/*
Validate default LB label, source and destination clusters, validate if NLB timeout needed or not
If default is NLB, and all source cluster in CLB exclude list then return false
If default is CLB, and any of 1 or more source clusters in NLB include list then return true
If default is NLB, not all source cluster in CLB exclude list then return true
We can't support multiple LB timeout as DR is one. And most case endpoint/LB will be two. So we need to pick one.
Excluding mailchimp all are fine to pick any one. All mailchimp will be in exclude list, need to put right cluster in DB
*/
func IsNLBTimeoutNeeded(sourceClusters []string, nlbOverwrite []string, clbOverwrite []string, sourceIdentity string) bool {
	var onlyNLBOverwrite []string

	for _, nlbCluster := range nlbOverwrite {
		nlbSplits := strings.Split(nlbCluster, ":")
		if len(nlbSplits) == 2 {
			//This means entire cluster is not enabled for NLB
			identities := strings.Split(nlbSplits[0], ",")
			if slices.Contains(identities, sourceIdentity) && slices.Contains(sourceClusters, nlbSplits[1]) {
				return true
			}
			onlyNLBOverwrite = append(onlyNLBOverwrite, nlbSplits[1])
		} else {
			//This means entire cluster is enabled for NLB
			onlyNLBOverwrite = append(onlyNLBOverwrite, nlbCluster)
		}
	}

	nlbIntersectClusters := common.SliceIntersection[string](sourceClusters, onlyNLBOverwrite)
	clbIntersectClusters := common.SliceIntersection[string](sourceClusters, clbOverwrite)

	//if sourceAsset is part of asset level migration then return default NLB timeout
	if slices.Contains(common.GetAdmiralParams().NLBEnabledIdentityList, strings.ToLower(sourceIdentity)) {
		return true
	}

	//All source cluster have CLB exception and default is NLB, don't overwrite
	if len(sourceClusters) == len(clbIntersectClusters) && common.GetAdmiralParams().LabelSet.GatewayApp == common.NLBIstioIngressGatewayLabelValue {
		return false
	} else if len(nlbIntersectClusters) > 0 && common.GetAdmiralParams().LabelSet.GatewayApp == common.IstioIngressGatewayLabelValue {
		//Default is CLB, but 1 or more source clusters have NLB enabled, overwrite
		return true
	}
	return common.GetAdmiralParams().LabelSet.GatewayApp == common.NLBIstioIngressGatewayLabelValue
}
