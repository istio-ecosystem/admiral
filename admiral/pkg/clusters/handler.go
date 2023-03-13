package clusters

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/testing/protocmp"
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultBaseEjectionTime         int64  = 300
	DefaultConsecutiveGatewayErrors uint32 = 50
	DefaultInterval                 int64  = 60
	DefaultHTTP2MaxRequests         int32  = 1000
	DefaultMaxRequestsPerConnection int32  = 100
)

// ServiceEntryHandler responsible for handling Add/Update/Delete events for
// ServiceEntry resources
type ServiceEntryHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

// DestinationRuleHandler responsible for handling Add/Update/Delete events for
// DestinationRule resources
type DestinationRuleHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

// VirtualServiceHandler responsible for handling Add/Update/Delete events for
// VirtualService resources
type VirtualServiceHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

// SidecarHandler responsible for handling Add/Update/Delete events for
// Sidecar resources
type SidecarHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

// WeightedService utility to store weighted services for argo rollouts
type WeightedService struct {
	Weight  int32
	Service *k8sV1.Service
}

func updateIdentityDependencyCache(sourceIdentity string, identityDependencyCache *common.MapOfMaps, dr *v1.Dependency) {
	for _, dIdentity := range dr.Spec.Destinations {
		identityDependencyCache.Put(dIdentity, sourceIdentity, sourceIdentity)
	}
	log.Infof(LogFormat, "Update", "dependency-cache", dr.Name, "", "Updated=true namespace="+dr.Namespace)
}

func getIstioResourceName(host string, suffix string) string {
	return strings.ToLower(host) + suffix
}

func getDestinationRule(se *networkingv1alpha3.ServiceEntry, locality string, gtpTrafficPolicy *model.TrafficPolicy) *networkingv1alpha3.DestinationRule {
	var (
		processGtp = true
		dr         = &networkingv1alpha3.DestinationRule{}
	)
	dr.Host = se.Hosts[0]
	dr.TrafficPolicy = &networkingv1alpha3.TrafficPolicy{
		Tls: &networkingv1alpha3.ClientTLSSettings{
			Mode: networkingv1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &networkingv1alpha3.ConnectionPoolSettings{
			Http: &networkingv1alpha3.ConnectionPoolSettings_HTTPSettings{
				Http2MaxRequests:         DefaultHTTP2MaxRequests,
				MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
			},
		},
		LoadBalancer: &networkingv1alpha3.LoadBalancerSettings{
			LbPolicy: &networkingv1alpha3.LoadBalancerSettings_Simple{
				Simple: networkingv1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
		},
	}

	if len(locality) == 0 {
		log.Warnf(LogErrFormat, "Process", "GlobalTrafficPolicy", dr.Host, "", "Skipping gtp processing, locality of the cluster nodes cannot be determined. Is this minikube?")
		processGtp = false
	}
	if gtpTrafficPolicy != nil && processGtp {
		var loadBalancerSettings = &networkingv1alpha3.LoadBalancerSettings{
			LbPolicy: &networkingv1alpha3.LoadBalancerSettings_Simple{Simple: networkingv1alpha3.LoadBalancerSettings_LEAST_REQUEST},
		}

		if len(gtpTrafficPolicy.Target) > 0 {
			var localityLbSettings = &networkingv1alpha3.LocalityLoadBalancerSetting{}
			if gtpTrafficPolicy.LbType == model.TrafficPolicy_FAILOVER {
				distribute := make([]*networkingv1alpha3.LocalityLoadBalancerSetting_Distribute, 0)
				targetTrafficMap := make(map[string]uint32)
				for _, tg := range gtpTrafficPolicy.Target {
					//skip 0 values from GTP as that's implicit for locality settings
					if tg.Weight != int32(0) {
						targetTrafficMap[tg.Region] = uint32(tg.Weight)
					}
				}
				distribute = append(distribute, &networkingv1alpha3.LocalityLoadBalancerSetting_Distribute{
					From: locality + "/*",
					To:   targetTrafficMap,
				})
				localityLbSettings.Distribute = distribute
			}
			// else default behavior
			loadBalancerSettings.LocalityLbSetting = localityLbSettings
			dr.TrafficPolicy.LoadBalancer = loadBalancerSettings
		}
	}
	dr.TrafficPolicy.OutlierDetection = getOutlierDetection(se, locality, gtpTrafficPolicy)
	return dr
}

func getOutlierDetection(se *networkingv1alpha3.ServiceEntry, locality string, gtpTrafficPolicy *model.TrafficPolicy) *networkingv1alpha3.OutlierDetection {
	outlierDetection := &networkingv1alpha3.OutlierDetection{
		BaseEjectionTime:         &duration.Duration{Seconds: DefaultBaseEjectionTime},
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: DefaultConsecutiveGatewayErrors},
		Interval:                 &duration.Duration{Seconds: DefaultInterval},
	}

	if gtpTrafficPolicy != nil && gtpTrafficPolicy.OutlierDetection != nil {
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

	//Scenario 1: Only one endpoint present and is local service (ends in svc.cluster.local) - no outlier detection (optimize this for headless services in future?)
	if len(se.Endpoints) == 1 && (strings.Contains(se.Endpoints[0].Address, common.DotLocalDomainSuffix) || net.ParseIP(se.Endpoints[0].Address).To4() != nil) {
		return nil
	} else if len(se.Endpoints) == 1 {
		//Scenario 2: Only one endpoint present and is remote - outlier detection with 34% ejection (protection against zone specific issues)
		outlierDetection.MaxEjectionPercent = 34
	} else {
		//Scenario 3: Two endpoints present each with different locality and both remote - outlier detection with 100% ejection
		//Scenario 4: Two endpoints present each with different locality with one local and other remote - outlier detection with 100% ejection
		//for service entries with more than 2 endpoints eject 100% to failover to other endpoint within or outside the same region
		outlierDetection.MaxEjectionPercent = 100
	}
	return outlierDetection
}

func (se *ServiceEntryHandler) Added(obj *v1alpha3.ServiceEntry) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Add", "ServiceEntry", obj.Name, se.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Add", "ServiceEntry", obj.Name, se.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
}

func (se *ServiceEntryHandler) Updated(obj *v1alpha3.ServiceEntry) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Update", "ServiceEntry", obj.Name, se.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Update", "ServiceEntry", obj.Name, se.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
}

func (se *ServiceEntryHandler) Deleted(obj *v1alpha3.ServiceEntry) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Delete", "ServiceEntry", obj.Name, se.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Delete", "ServiceEntry", obj.Name, se.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
}

func (dh *DestinationRuleHandler) Added(ctx context.Context, obj *v1alpha3.DestinationRule) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Add", "DestinationRule", obj.Name, dh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Add", "DestinationRule", obj.Name, dh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	handleDestinationRuleEvent(ctx, obj, dh, common.Add, common.DestinationRule)
}

func (dh *DestinationRuleHandler) Updated(ctx context.Context, obj *v1alpha3.DestinationRule) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Update", "DestinationRule", obj.Name, dh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Update", "DestinationRule", obj.Name, dh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	handleDestinationRuleEvent(ctx, obj, dh, common.Update, common.DestinationRule)
}

func (dh *DestinationRuleHandler) Deleted(ctx context.Context, obj *v1alpha3.DestinationRule) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Delete", "DestinationRule", obj.Name, dh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Delete", "DestinationRule", obj.Name, dh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	handleDestinationRuleEvent(ctx, obj, dh, common.Delete, common.DestinationRule)
}

func (vh *VirtualServiceHandler) Added(ctx context.Context, obj *v1alpha3.VirtualService) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Add", "VirtualService", obj.Name, vh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Add", "VirtualService", obj.Name, vh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	err := handleVirtualServiceEvent(ctx, obj, vh, common.Add, common.VirtualService)
	if err != nil {
		log.Error(err)
	}
}

func (vh *VirtualServiceHandler) Updated(ctx context.Context, obj *v1alpha3.VirtualService) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Update", "VirtualService", obj.Name, vh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Update", "VirtualService", obj.Name, vh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	err := handleVirtualServiceEvent(ctx, obj, vh, common.Update, common.VirtualService)
	if err != nil {
		log.Error(err)
	}
}

func (vh *VirtualServiceHandler) Deleted(ctx context.Context, obj *v1alpha3.VirtualService) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Delete", "VirtualService", obj.Name, vh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Delete", "VirtualService", obj.Name, vh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	err := handleVirtualServiceEvent(ctx, obj, vh, common.Delete, common.VirtualService)
	if err != nil {
		log.Error(err)
	}
}

func (dh *SidecarHandler) Added(ctx context.Context, obj *v1alpha3.Sidecar) {}

func (dh *SidecarHandler) Updated(ctx context.Context, obj *v1alpha3.Sidecar) {}

func (dh *SidecarHandler) Deleted(ctx context.Context, obj *v1alpha3.Sidecar) {}

func IgnoreIstioResource(exportTo []string, annotations map[string]string, namespace string) bool {
	if len(annotations) > 0 && annotations[common.AdmiralIgnoreAnnotation] == "true" {
		return true
	}

	if namespace == common.NamespaceIstioSystem || namespace == common.NamespaceKubeSystem || namespace == common.GetSyncNamespace() {
		return true
	}

	if len(exportTo) == 0 {
		return false
	} else {
		for _, namespace := range exportTo {
			if namespace == "*" {
				return false
			}
		}
	}
	return true
}

func handleDestinationRuleEvent(ctx context.Context, obj *v1alpha3.DestinationRule, dh *DestinationRuleHandler, event common.Event, resourceType common.ResourceType) {
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
		log.Infof(LogFormat, "Event", "DestinationRule", obj.Name, clusterId, "Processing")
		util.MapCopy(allDependentClusters, dependentClusters)
		allDependentClusters[clusterId] = clusterId
		for _, dependentCluster := range allDependentClusters {
			rc := r.GetRemoteController(dependentCluster)
			if event == common.Delete {
				err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Delete(ctx, obj.Name, metav1.DeleteOptions{})
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
				exist, _ := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(ctx, obj.Name, metav1.GetOptions{})
				//copy destination rule only to other clusters
				if dependentCluster != clusterId {
					addUpdateDestinationRule(ctx, obj, exist, syncNamespace, rc)
				}
			}
		}
		return
	} else {
		log.Infof(LogFormat, "Event", "DestinationRule", obj.Name, clusterId, "No dependent clusters found")
	}

	//copy the DestinationRule `as is` if they are not generated by Admiral
	remoteClusters := r.GetClusterIds()
	for _, ClusterID := range remoteClusters {
		if ClusterID != clusterId {
			rc := r.GetRemoteController(ClusterID)
			if event == common.Delete {
				err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Delete(ctx, obj.Name, metav1.DeleteOptions{})
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
				exist, _ := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(ctx, obj.Name, metav1.GetOptions{})
				addUpdateDestinationRule(ctx, obj, exist, syncNamespace, rc)
			}
		}
	}
}

func handleVirtualServiceEvent(
	ctx context.Context, obj *v1alpha3.VirtualService, vh *VirtualServiceHandler,
	event common.Event, resourceType common.ResourceType) error {
	var (
		//nolint
		virtualService = obj.Spec
		clusterId      = vh.ClusterID
		r              = vh.RemoteRegistry
		syncNamespace  = common.GetSyncNamespace()
	)
	log.Infof(LogFormat, "Event", resourceType, obj.Name, vh.ClusterID, "Received event")

	if len(virtualService.Hosts) > 1 {
		log.Errorf(LogFormat, "Event", resourceType, obj.Name, clusterId, "Skipping as multiple hosts not supported for virtual service namespace="+obj.Namespace)
		return nil
	}

	// check if this virtual service is used by Argo rollouts for canary strategy, if so, update the corresponding SE with appropriate weights
	if common.GetAdmiralParams().ArgoRolloutsEnabled {
		rollouts, err := vh.RemoteRegistry.GetRemoteController(clusterId).RolloutController.RolloutClient.Rollouts(obj.Namespace).List(ctx, metav1.ListOptions{})

		if err != nil {
			log.Errorf(LogErrFormat, "Get", "Rollout", "Error finding rollouts in namespace="+obj.Namespace, clusterId, err)
		} else {
			if len(rollouts.Items) > 0 {
				for _, rollout := range rollouts.Items {
					if rollout.Spec.Strategy.Canary != nil && rollout.Spec.Strategy.Canary.TrafficRouting != nil && rollout.Spec.Strategy.Canary.TrafficRouting.Istio != nil && rollout.Spec.Strategy.Canary.TrafficRouting.Istio.VirtualService.Name == obj.Name {
						HandleEventForRollout(ctx, admiral.Update, &rollout, vh.RemoteRegistry, clusterId)
					}
				}
			}
		}
	}

	if len(virtualService.Hosts) != 0 {
		dependentClusters := r.AdmiralCache.CnameDependentClusterCache.Get(virtualService.Hosts[0]).Copy()
		if len(dependentClusters) > 0 {
			for _, dependentCluster := range dependentClusters {
				rc := r.GetRemoteController(dependentCluster)
				if clusterId != dependentCluster {
					log.Infof(LogFormat, "Event", "VirtualService", obj.Name, clusterId, "Processing")
					if event == common.Delete {
						err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Delete(ctx, obj.Name, metav1.DeleteOptions{})
						if err != nil {
							if k8sErrors.IsNotFound(err) {
								log.Infof(LogFormat, "Delete", "VirtualService", obj.Name, clusterId, "Either VirtualService was already deleted, or it never existed")
							} else {
								log.Errorf(LogErrFormat, "Delete", "VirtualService", obj.Name, clusterId, err)
							}
						} else {
							log.Infof(LogFormat, "Delete", "VirtualService", obj.Name, clusterId, "Success")
						}
					} else {
						exist, _ := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, obj.Name, metav1.GetOptions{})
						//change destination host for all http routes <service_name>.<ns>. to same as host on the virtual service
						for _, httpRoute := range virtualService.Http {
							for _, destination := range httpRoute.Route {
								//get at index 0, we do not support wildcards or multiple hosts currently
								if strings.HasSuffix(destination.Destination.Host, common.DotLocalDomainSuffix) {
									destination.Destination.Host = virtualService.Hosts[0]
								}
							}
						}
						for _, tlsRoute := range virtualService.Tls {
							for _, destination := range tlsRoute.Route {
								//get at index 0, we do not support wildcards or multiple hosts currently
								if strings.HasSuffix(destination.Destination.Host, common.DotLocalDomainSuffix) {
									destination.Destination.Host = virtualService.Hosts[0]
								}
							}
						}
						// nolint
						addUpdateVirtualService(ctx, obj, exist, syncNamespace, rc)
					}
				}
			}
			return nil
		} else {
			log.Infof(LogFormat, "Event", "VirtualService", obj.Name, clusterId, "No dependent clusters found")
		}
	}

	// copy the VirtualService `as is` if they are not generated by Admiral (not in CnameDependentClusterCache)
	log.Infof(LogFormat, "Event", "VirtualService", obj.Name, clusterId, "Replicating 'as is' to all clusters")
	remoteClusters := r.GetClusterIds()
	for _, ClusterID := range remoteClusters {
		if ClusterID != clusterId {
			rc := r.GetRemoteController(ClusterID)
			if event == common.Delete {
				err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Delete(ctx, obj.Name, metav1.DeleteOptions{})
				if err != nil {
					if k8sErrors.IsNotFound(err) {
						log.Infof(LogFormat, "Delete", "VirtualService", obj.Name, clusterId, "Either VirtualService was already deleted, or it never existed")
					} else {
						log.Errorf(LogErrFormat, "Delete", "VirtualService", obj.Name, clusterId, err)
					}
				} else {
					log.Infof(LogFormat, "Delete", "VirtualService", obj.Name, clusterId, "Success")
				}
			} else {
				exist, _ := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, obj.Name, metav1.GetOptions{})
				// nolint
				addUpdateVirtualService(ctx, obj, exist, syncNamespace, rc)
			}
		}
	}
	return nil
}

func addUpdateVirtualService(ctx context.Context, obj *v1alpha3.VirtualService, exist *v1alpha3.VirtualService, namespace string, rc *RemoteController) error {
	var (
		err error
		op  string
	)

	format := "virtualservice %s before: %v, after: %v;"

	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations["app.kubernetes.io/created-by"] = "admiral"
	if exist == nil || len(exist.Spec.Hosts) == 0 {
		obj.Namespace = namespace
		obj.ResourceVersion = ""
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Create(ctx, obj, metav1.CreateOptions{})
		op = "Add"
	} else {
		op = "Update"
		log.Infof(format, op, exist.Spec.String(), obj.Spec.String())
		exist.Labels = obj.Labels
		exist.Annotations = obj.Annotations
		//nolint
		exist.Spec = obj.Spec
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Update(ctx, exist, metav1.UpdateOptions{})
	}

	if err != nil {
		log.Errorf(LogErrFormat, op, "VirtualService", obj.Name, rc.ClusterID, err)
		return err
	}
	log.Infof(LogFormat, op, "VirtualService", obj.Name, rc.ClusterID, "Success")
	return nil
}

func validateAndProcessServiceEntryEndpoints(obj *v1alpha3.ServiceEntry) bool {
	var areEndpointsValid = true

	temp := make([]*networkingv1alpha3.WorkloadEntry, 0)
	for _, endpoint := range obj.Spec.Endpoints {
		if endpoint.Address == "dummy.admiral.global" {
			areEndpointsValid = false
		} else {
			temp = append(temp, endpoint)
		}
	}
	obj.Spec.Endpoints = temp
	log.Infof("type=ServiceEntry, name=%s, endpointsValid=%v, numberOfValidEndpoints=%d", obj.Name, areEndpointsValid, len(obj.Spec.Endpoints))

	return areEndpointsValid
}

func addUpdateServiceEntry(ctx context.Context, obj *v1alpha3.ServiceEntry, exist *v1alpha3.ServiceEntry, namespace string, rc *RemoteController) {
	var (
		err        error
		op, diff   string
		skipUpdate bool
	)

	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations["app.kubernetes.io/created-by"] = "admiral"

	areEndpointsValid := validateAndProcessServiceEntryEndpoints(obj)

	if exist == nil || exist.Spec.Hosts == nil {
		op = "Add"
		//se will be created if endpoints are valid, in case they are not valid se will be created with just valid endpoints
		if len(obj.Spec.Endpoints) > 0 {
			obj.Namespace = namespace
			obj.ResourceVersion = ""
			_, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Create(ctx, obj, metav1.CreateOptions{})
			log.Infof(LogFormat+" SE=%s", op, "ServiceEntry", obj.Name, rc.ClusterID, "New SE", obj.Spec.String())
		} else {
			log.Errorf(LogFormat+" SE=%s", op, "ServiceEntry", obj.Name, rc.ClusterID, "Creation of SE skipped as endpoints are not valid", obj.Spec.String())
		}
	} else {
		op = "Update"
		if areEndpointsValid { //update will happen only when all the endpoints are valid
			exist.Labels = obj.Labels
			exist.Annotations = obj.Annotations
			skipUpdate, diff = skipDestructiveUpdate(rc, obj, exist)
			if diff != "" {
				log.Infof(LogFormat+" diff=%s", op, "ServiceEntry", obj.Name, rc.ClusterID, "Diff in update", diff)
			}
			if skipUpdate {
				log.Infof(LogFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, "Update skipped as it was destructive during Admiral's bootup phase")
				return
			} else {
				//nolint
				exist.Spec = obj.Spec
				_, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Update(ctx, exist, metav1.UpdateOptions{})
			}
		} else {
			log.Infof(LogFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, "SE could not be updated as all the recived endpoints are not valid.")
		}
	}

	if err != nil {
		log.Errorf(LogErrFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, err)
	} else {
		log.Infof(LogFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, "Success")
	}
}

func skipDestructiveUpdate(rc *RemoteController, new *v1alpha3.ServiceEntry, old *v1alpha3.ServiceEntry) (bool, string) {
	var (
		skipDestructive   = false
		destructive, diff = getServiceEntryDiff(new, old)
	)
	//do not update SEs during bootup phase if they are destructive
	if time.Since(rc.StartTime) < (2*common.GetAdmiralParams().CacheRefreshDuration) && destructive {
		skipDestructive = true
	}
	return skipDestructive, diff
}

//Diffs only endpoints
func getServiceEntryDiff(new *v1alpha3.ServiceEntry, old *v1alpha3.ServiceEntry) (destructive bool, diff string) {
	//we diff only if both objects exist
	if old == nil || new == nil {
		return false, ""
	}
	destructive = false
	format := "%s %s before: %v, after: %v;"
	var buffer bytes.Buffer
	//nolint
	seNew := new.Spec
	//nolint
	seOld := old.Spec

	oldEndpointMap := make(map[string]*networkingv1alpha3.WorkloadEntry)
	found := make(map[string]string)
	for _, oEndpoint := range seOld.Endpoints {
		oldEndpointMap[oEndpoint.Address] = oEndpoint
	}
	for _, nEndpoint := range seNew.Endpoints {
		if val, ok := oldEndpointMap[nEndpoint.Address]; ok {
			found[nEndpoint.Address] = "1"
			if !cmp.Equal(val, nEndpoint, protocmp.Transform()) {
				destructive = true
				buffer.WriteString(fmt.Sprintf(format, "endpoint", "Update", val.String(), nEndpoint.String()))
			}
		} else {
			buffer.WriteString(fmt.Sprintf(format, "endpoint", "Add", "", nEndpoint.String()))
		}
	}

	for key := range oldEndpointMap {
		if _, ok := found[key]; !ok {
			destructive = true
			buffer.WriteString(fmt.Sprintf(format, "endpoint", "Delete", oldEndpointMap[key].String(), ""))
		}
	}

	diff = buffer.String()
	return destructive, diff
}

func deleteVirtualService(ctx context.Context, exist *v1alpha3.VirtualService, namespace string, rc *RemoteController) error {
	if exist == nil {
		return fmt.Errorf("the VirtualService passed was nil")
	}
	err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Delete(ctx, exist.Name, metav1.DeleteOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return fmt.Errorf("either VirtualService was already deleted, or it never existed")
		}
		return err
	}
	return nil
}

func deleteServiceEntry(ctx context.Context, exist *v1alpha3.ServiceEntry, namespace string, rc *RemoteController) {
	if exist != nil {
		err := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Delete(ctx, exist.Name, metav1.DeleteOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				log.Infof(LogFormat, "Delete", "ServiceEntry", exist.Name, rc.ClusterID, "Either ServiceEntry was already deleted, or it never existed")
			} else {
				log.Errorf(LogErrFormat, "Delete", "ServiceEntry", exist.Name, rc.ClusterID, err)
			}
		} else {
			log.Infof(LogFormat, "Delete", "ServiceEntry", exist.Name, rc.ClusterID, "Success")
		}
	}
}

func addUpdateDestinationRule(ctx context.Context, obj *v1alpha3.DestinationRule, exist *v1alpha3.DestinationRule, namespace string, rc *RemoteController) {
	var err error
	var op string
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations["app.kubernetes.io/created-by"] = "admiral"
	if exist == nil || exist.Name == "" || exist.Spec.Host == "" {
		obj.Namespace = namespace
		obj.ResourceVersion = ""
		_, err = rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Create(ctx, obj, metav1.CreateOptions{})
		op = "Add"
	} else {
		exist.Labels = obj.Labels
		exist.Annotations = obj.Annotations
		//nolint
		exist.Spec = obj.Spec
		op = "Update"
		_, err = rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Update(ctx, exist, metav1.UpdateOptions{})
	}

	if err != nil {
		log.Errorf(LogErrFormat, op, "DestinationRule", obj.Name, rc.ClusterID, err)
	} else {
		log.Infof(LogFormat, op, "DestinationRule", obj.Name, rc.ClusterID, "Success")
	}
}

func deleteDestinationRule(ctx context.Context, exist *v1alpha3.DestinationRule, namespace string, rc *RemoteController) {
	if exist != nil {
		err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Delete(ctx, exist.Name, metav1.DeleteOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				log.Infof(LogFormat, "Delete", "DestinationRule", exist.Name, rc.ClusterID, "Either DestinationRule was already deleted, or it never existed")
			} else {
				log.Errorf(LogErrFormat, "Delete", "DestinationRule", exist.Name, rc.ClusterID, err)
			}
		} else {
			log.Infof(LogFormat, "Delete", "DestinationRule", exist.Name, rc.ClusterID, "Success")
		}
	}
}

//nolint
func createServiceEntrySkeletion(se networkingv1alpha3.ServiceEntry, name string, namespace string) *v1alpha3.ServiceEntry {
	return &v1alpha3.ServiceEntry{Spec: se, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

//nolint
func createSidecarSkeleton(sidecar networkingv1alpha3.Sidecar, name string, namespace string) *v1alpha3.Sidecar {
	return &v1alpha3.Sidecar{Spec: sidecar, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

//nolint
func createDestinationRuleSkeletion(dr networkingv1alpha3.DestinationRule, name string, namespace string) *v1alpha3.DestinationRule {
	return &v1alpha3.DestinationRule{Spec: dr, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

//nolint
func createVirtualServiceSkeleton(vs networkingv1alpha3.VirtualService, name string, namespace string) *v1alpha3.VirtualService {
	return &v1alpha3.VirtualService{Spec: vs, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

func getServiceForDeployment(rc *RemoteController, deployment *k8sAppsV1.Deployment) *k8sV1.Service {
	if deployment == nil {
		return nil
	}
	cachedServices := rc.ServiceController.Cache.Get(deployment.Namespace)
	if cachedServices == nil {
		return nil
	}
	var matchedService *k8sV1.Service
	for _, service := range cachedServices {
		var match = common.IsServiceMatch(service.Spec.Selector, deployment.Spec.Selector)
		//make sure the service matches the deployment Selector and also has a mesh port in the port spec
		if match {
			ports := GetMeshPortsForDeployment(rc.ClusterID, service, deployment)
			if len(ports) > 0 {
				matchedService = service
				break
			}
		}
	}
	return matchedService
}

func getDependentClusters(dependents map[string]string, identityClusterCache *common.MapOfMaps, sourceServices map[string]*k8sV1.Service) map[string]string {
	var dependentClusters = make(map[string]string)
	if dependents == nil {
		return dependentClusters
	}
	for depIdentity := range dependents {
		clusters := identityClusterCache.Get(depIdentity)
		if clusters == nil {
			continue
		}
		clusters.Range(func(k string, clusterID string) {
			_, ok := sourceServices[clusterID]
			if !ok {
				dependentClusters[clusterID] = clusterID
			}
		})
	}
	return dependentClusters
}

func copyEndpoint(e *networkingv1alpha3.WorkloadEntry) *networkingv1alpha3.WorkloadEntry {
	var (
		labels = make(map[string]string)
		ports  = make(map[string]uint32)
	)
	util.MapCopy(labels, e.Labels)
	util.MapCopy(ports, e.Ports)
	return &networkingv1alpha3.WorkloadEntry{Address: e.Address, Ports: ports, Locality: e.Locality, Labels: labels}
}

// A rollout can use one of 2 stratergies :-
// 1. Canary strategy - which can use a virtual service to manage the weights associated with a stable and canary service. Admiral created endpoints in service entries will use the weights assigned in the Virtual Service
// 2. Blue green strategy- this contains 2 service instances in a namespace, an active service and a preview service. Admiral will use repective service to create active and preview endpoints
func getServiceForRollout(ctx context.Context, rc *RemoteController, rollout *argo.Rollout) map[string]*WeightedService {
	if rollout == nil {
		return nil
	}
	cachedServices := rc.ServiceController.Cache.Get(rollout.Namespace)
	if cachedServices == nil {
		return nil
	}
	rolloutStrategy := rollout.Spec.Strategy
	if rolloutStrategy.BlueGreen == nil && rolloutStrategy.Canary == nil {
		return nil
	}
	var (
		canaryService           string
		stableService           string
		virtualServiceRouteName string
		istioCanaryWeights      = make(map[string]int32)
		blueGreenActiveService  string
		blueGreenPreviewService string
		matchedServices         = make(map[string]*WeightedService)
	)

	if rolloutStrategy.BlueGreen != nil {
		// If rollout uses blue green strategy
		blueGreenActiveService = rolloutStrategy.BlueGreen.ActiveService
		blueGreenPreviewService = rolloutStrategy.BlueGreen.PreviewService
		if len(blueGreenActiveService) == 0 {
			//pick a service that ends in RolloutActiveServiceSuffix if one is available
			blueGreenActiveService = GetServiceWithSuffixMatch(common.RolloutActiveServiceSuffix, cachedServices)
		}
	} else if rolloutStrategy.Canary != nil {
		canaryService = rolloutStrategy.Canary.CanaryService
		stableService = rolloutStrategy.Canary.StableService

		//calculate canary weights if canary strategy is using Istio traffic management
		if len(stableService) > 0 && len(canaryService) > 0 && rolloutStrategy.Canary.TrafficRouting != nil && rolloutStrategy.Canary.TrafficRouting.Istio != nil {
			//pick stable service if specified
			if len(stableService) > 0 {
				istioCanaryWeights[stableService] = 1
			} else {
				//pick a service that ends in RolloutStableServiceSuffix if one is available
				sName := GetServiceWithSuffixMatch(common.RolloutStableServiceSuffix, cachedServices)
				if len(sName) > 0 {
					istioCanaryWeights[sName] = 1
				}
			}

			virtualServiceName := rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Name
			virtualService, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(rollout.Namespace).Get(ctx, virtualServiceName, metav1.GetOptions{})

			if err != nil {
				log.Warnf("Error fetching VirtualService referenced in rollout canary for rollout with name=%s in namespace=%s and cluster=%s err=%v", rollout.Name, rollout.Namespace, rc.ClusterID, err)
			}

			if len(rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Routes) > 0 {
				virtualServiceRouteName = rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Routes[0]
			}

			if virtualService != nil {
				//nolint
				var vs = virtualService.Spec
				if len(vs.Http) > 0 {
					var httpRoute *networkingv1alpha3.HTTPRoute
					if len(virtualServiceRouteName) > 0 {
						for _, route := range vs.Http {
							if route.Name == virtualServiceRouteName {
								httpRoute = route
								log.Infof("VirtualService route referenced in rollout found, for rollout with name=%s route=%s in namespace=%s and cluster=%s", rollout.Name, virtualServiceRouteName, rollout.Namespace, rc.ClusterID)
								break
							} else {
								log.Debugf("Argo rollout VirtualService route name didn't match with a route, for rollout with name=%s route=%s in namespace=%s and cluster=%s", rollout.Name, route.Name, rollout.Namespace, rc.ClusterID)
							}
						}
					} else {
						if len(vs.Http) == 1 {
							httpRoute = vs.Http[0]
							log.Debugf("Using the default and the only route in Virtual Service, for rollout with name=%s route=%s in namespace=%s and cluster=%s", rollout.Name, "", rollout.Namespace, rc.ClusterID)
						} else {
							log.Errorf("Skipping VirtualService referenced in rollout as it has MORE THAN ONE route but no name route selector in rollout, for rollout with name=%s in namespace=%s and cluster=%s", rollout.Name, rollout.Namespace, rc.ClusterID)
						}
					}
					if httpRoute != nil {
						//find the weight associated with the destination (k8s service)
						for _, destination := range httpRoute.Route {
							if (destination.Destination.Host == canaryService || destination.Destination.Host == stableService) && destination.Weight > 0 {
								istioCanaryWeights[destination.Destination.Host] = destination.Weight
							}
						}
					}
				} else {
					log.Warnf("No VirtualService was specified in rollout or the specified VirtualService has NO routes, for rollout with name=%s in namespace=%s and cluster=%s", rollout.Name, rollout.Namespace, rc.ClusterID)
				}
			}
			for _, service := range cachedServices {
				match := common.IsServiceMatch(service.Spec.Selector, rollout.Spec.Selector)
				//make sure the service matches the rollout Selector and also has a mesh port in the port spec
				if match {
					ports := GetMeshPortsForRollout(rc.ClusterID, service, rollout)
					if len(ports) > 0 {
						if val, ok := istioCanaryWeights[service.Name]; ok {
							matchedServices[service.Name] = &WeightedService{Weight: val, Service: service}
						}
					}
				}
			}
			return matchedServices
		} else if len(stableService) > 0 {
			for _, service := range cachedServices {
				//skip services that are not referenced in the rollout
				if service.ObjectMeta.Name != stableService {
					log.Infof("Skipping service=%s for rollout=%s in namespace=%s and cluster=%s", service.Name, rollout.Name, rollout.Namespace, rc.ClusterID)
					continue
				}
				match := common.IsServiceMatch(service.Spec.Selector, rollout.Spec.Selector)
				//make sure the service matches the rollout Selector and also has a mesh port in the port spec
				if match {
					ports := GetMeshPortsForRollout(rc.ClusterID, service, rollout)
					if len(ports) > 0 {
						if len(istioCanaryWeights) == 0 {
							matchedServices[service.Name] = &WeightedService{Weight: 1, Service: service}
							return matchedServices
						}
					}
				}
			}
		}
	}

	for _, service := range cachedServices {
		//skip services that are not referenced in the rollout
		if len(blueGreenActiveService) > 0 && service.ObjectMeta.Name != blueGreenActiveService && service.ObjectMeta.Name != blueGreenPreviewService {
			log.Infof("Skipping service=%s for rollout=%s in namespace=%s and cluster=%s", service.Name, rollout.Name, rollout.Namespace, rc.ClusterID)
			continue
		}
		match := common.IsServiceMatch(service.Spec.Selector, rollout.Spec.Selector)
		//make sure the service matches the rollout Selector and also has a mesh port in the port spec
		if match {
			ports := GetMeshPortsForRollout(rc.ClusterID, service, rollout)
			if len(ports) > 0 {
				// if the strategy is bluegreen return matched services
				// else if using canary with NO istio traffic management, pick the first service that matches
				if rolloutStrategy.BlueGreen != nil {
					matchedServices[service.Name] = &WeightedService{Weight: 1, Service: service}
				} else if len(istioCanaryWeights) == 0 {
					matchedServices[service.Name] = &WeightedService{Weight: 1, Service: service}
					break
				}
			}
		}
	}
	return matchedServices
}

func GetServiceWithSuffixMatch(suffix string, services []*k8sV1.Service) string {
	for _, service := range services {
		if strings.HasSuffix(service.Name, suffix) {
			return service.Name
		}
	}
	return ""
}
