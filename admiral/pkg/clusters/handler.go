package clusters

import (
	"bytes"
	"fmt"
	"net"
	"reflect"
	"strings"
	"time"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/gogo/protobuf/types"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	log "github.com/sirupsen/logrus"
	v1alpha32 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultBaseEjectionTime         int64  = 300
	DefaultConsecutiveGatewayErrors uint32 = 50
	DefaultInterval                 int64  = 60
)

type ServiceEntryHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type DestinationRuleHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type VirtualServiceHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type SidecarHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

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

func getDestinationRule(se *v1alpha32.ServiceEntry, locality string, gtpTrafficPolicy *model.TrafficPolicy) *v1alpha32.DestinationRule {
	var dr = &v1alpha32.DestinationRule{}
	dr.Host = se.Hosts[0]
	dr.TrafficPolicy = &v1alpha32.TrafficPolicy{Tls: &v1alpha32.TLSSettings{Mode: v1alpha32.TLSSettings_ISTIO_MUTUAL}}
	processGtp := true
	if len(locality) == 0 {
		log.Warnf(LogErrFormat, "Process", "GlobalTrafficPolicy", dr.Host, "", "Skipping gtp processing, locality of the cluster nodes cannot be determined. Is this minikube?")
		processGtp = false
	}
	if gtpTrafficPolicy != nil && processGtp {
		var loadBalancerSettings = &v1alpha32.LoadBalancerSettings{
			LbPolicy: &v1alpha32.LoadBalancerSettings_Simple{Simple: v1alpha32.LoadBalancerSettings_ROUND_ROBIN},
		}

		if len(gtpTrafficPolicy.Target) > 0 {
			var localityLbSettings = &v1alpha32.LocalityLoadBalancerSetting{}

			if gtpTrafficPolicy.LbType == model.TrafficPolicy_FAILOVER {
				distribute := make([]*v1alpha32.LocalityLoadBalancerSetting_Distribute, 0)
				targetTrafficMap := make(map[string]uint32)
				for _, tg := range gtpTrafficPolicy.Target {
					//skip 0 values from GTP as that's implicit for locality settings
					if tg.Weight != int32(0) {
						targetTrafficMap[tg.Region] = uint32(tg.Weight)
					}
				}
				distribute = append(distribute, &v1alpha32.LocalityLoadBalancerSetting_Distribute{
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

func getOutlierDetection(se *v1alpha32.ServiceEntry, locality string, gtpTrafficPolicy *model.TrafficPolicy) *v1alpha32.OutlierDetection {

	outlierDetection := &v1alpha32.OutlierDetection{
		BaseEjectionTime:         &types.Duration{Seconds: DefaultBaseEjectionTime},
		ConsecutiveGatewayErrors: &types.UInt32Value{Value: DefaultConsecutiveGatewayErrors},
		Interval:                 &types.Duration{Seconds: DefaultInterval},
	}

	if gtpTrafficPolicy != nil && gtpTrafficPolicy.OutlierDetection != nil {
		if gtpTrafficPolicy.OutlierDetection.BaseEjectionTime > 0 {
			outlierDetection.BaseEjectionTime = &types.Duration{
				Seconds: gtpTrafficPolicy.OutlierDetection.BaseEjectionTime,
			}
		}
		if gtpTrafficPolicy.OutlierDetection.ConsecutiveGatewayErrors > 0 {
			outlierDetection.ConsecutiveGatewayErrors = &types.UInt32Value{
				Value: gtpTrafficPolicy.OutlierDetection.ConsecutiveGatewayErrors,
			}
		}
		if gtpTrafficPolicy.OutlierDetection.Interval > 0 {
			outlierDetection.Interval = &types.Duration{
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

func (dh *DestinationRuleHandler) Added(obj *v1alpha3.DestinationRule) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Add", "DestinationRule", obj.Name, dh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Add", "DestinationRule", obj.Name, dh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	handleDestinationRuleEvent(obj, dh, common.Add, common.DestinationRule)
}

func (dh *DestinationRuleHandler) Updated(obj *v1alpha3.DestinationRule) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Update", "DestinationRule", obj.Name, dh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Update", "DestinationRule", obj.Name, dh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	handleDestinationRuleEvent(obj, dh, common.Update, common.DestinationRule)
}

func (dh *DestinationRuleHandler) Deleted(obj *v1alpha3.DestinationRule) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Delete", "DestinationRule", obj.Name, dh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Delete", "DestinationRule", obj.Name, dh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	handleDestinationRuleEvent(obj, dh, common.Delete, common.DestinationRule)
}

func (vh *VirtualServiceHandler) Added(obj *v1alpha3.VirtualService) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Add", "VirtualService", obj.Name, vh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Add", "VirtualService", obj.Name, vh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	err := handleVirtualServiceEvent(obj, vh, common.Add, common.VirtualService)
	if err != nil {
		log.Error(err)
	}
}

func (vh *VirtualServiceHandler) Updated(obj *v1alpha3.VirtualService) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Update", "VirtualService", obj.Name, vh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Update", "VirtualService", obj.Name, vh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	err := handleVirtualServiceEvent(obj, vh, common.Update, common.VirtualService)
	if err != nil {
		log.Error(err)
	}
}

func (vh *VirtualServiceHandler) Deleted(obj *v1alpha3.VirtualService) {
	if CurrentAdmiralState.ReadOnly {
		log.Infof(LogFormat, "Delete", "VirtualService", obj.Name, vh.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Delete", "VirtualService", obj.Name, vh.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		return
	}
	err := handleVirtualServiceEvent(obj, vh, common.Delete, common.VirtualService)
	if err != nil {
		log.Error(err)
	}
}

func (dh *SidecarHandler) Added(obj *v1alpha3.Sidecar) {}

func (dh *SidecarHandler) Updated(obj *v1alpha3.Sidecar) {}

func (dh *SidecarHandler) Deleted(obj *v1alpha3.Sidecar) {}

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

func handleDestinationRuleEvent(obj *v1alpha3.DestinationRule, dh *DestinationRuleHandler, event common.Event, resourceType common.ResourceType) {
	destinationRule := obj.Spec

	clusterId := dh.ClusterID

	syncNamespace := common.GetSyncNamespace()

	r := dh.RemoteRegistry

	dependentClusters := r.AdmiralCache.CnameDependentClusterCache.Get(destinationRule.Host).Copy()

	if len(dependentClusters) > 0 {

		log.Infof(LogFormat, "Event", "DestinationRule", obj.Name, clusterId, "Processing")

		allDependentClusters := make(map[string]string)

		util.MapCopy(allDependentClusters, dependentClusters)

		allDependentClusters[clusterId] = clusterId

		for _, dependentCluster := range allDependentClusters {

			rc := r.GetRemoteController(dependentCluster)

			if event == common.Delete {

				err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Delete(obj.Name, &v12.DeleteOptions{})
				if err != nil {
					log.Infof(LogFormat, "Delete", "DestinationRule", obj.Name, clusterId, "success")
				} else {
					log.Errorf(LogErrFormat, "Delete", "DestinationRule", obj.Name, clusterId, err)
				}

			} else {

				exist, _ := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(obj.Name, v12.GetOptions{})

				//copy destination rule only to other clusters
				if dependentCluster != clusterId {
					addUpdateDestinationRule(obj, exist, syncNamespace, rc)
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
				err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Delete(obj.Name, &v12.DeleteOptions{})
				if err != nil {
					log.Infof(LogErrFormat, "Delete", "DestinationRule", obj.Name, clusterId, err)
				} else {
					log.Infof(LogFormat, "Delete", "DestinationRule", obj.Name, clusterId, "Success")
				}
			} else {
				exist, _ := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(obj.Name, v12.GetOptions{})
				addUpdateDestinationRule(obj, exist, syncNamespace, rc)
			}
		}
	}
}

func handleVirtualServiceEvent(obj *v1alpha3.VirtualService, vh *VirtualServiceHandler, event common.Event, resourceType common.ResourceType) error {

	log.Infof(LogFormat, "Event", resourceType, obj.Name, vh.ClusterID, "Received event")

	virtualService := obj.Spec

	clusterId := vh.ClusterID

	r := vh.RemoteRegistry

	syncNamespace := common.GetSyncNamespace()

	if len(virtualService.Hosts) > 1 {
		log.Errorf(LogFormat, "Event", resourceType, obj.Name, clusterId, "Skipping as multiple hosts not supported for virtual service namespace="+obj.Namespace)
		return nil
	}

	//check if this virtual service is used by Argo rollouts for canary strategy, if so, update the corresponding SE with appropriate weights
	if common.GetAdmiralParams().ArgoRolloutsEnabled {
		rollouts, err := vh.RemoteRegistry.GetRemoteController(clusterId).RolloutController.RolloutClient.Rollouts(obj.Namespace).List(v12.ListOptions{})

		if err != nil {
			log.Errorf(LogErrFormat, "Get", "Rollout", "Error finding rollouts in namespace="+obj.Namespace, clusterId, err)
		} else {
			if len(rollouts.Items) > 0 {
				for _, rollout := range rollouts.Items {
					if rollout.Spec.Strategy.Canary != nil && rollout.Spec.Strategy.Canary.TrafficRouting != nil && rollout.Spec.Strategy.Canary.TrafficRouting.Istio != nil && rollout.Spec.Strategy.Canary.TrafficRouting.Istio.VirtualService.Name == obj.Name {
						HandleEventForRollout(admiral.Update, &rollout, vh.RemoteRegistry, clusterId)
					}
				}
			}
		}
	}

	dependentClusters := r.AdmiralCache.CnameDependentClusterCache.Get(virtualService.Hosts[0]).Copy()

	if len(dependentClusters) > 0 {

		for _, dependentCluster := range dependentClusters {

			rc := r.GetRemoteController(dependentCluster)

			if clusterId != dependentCluster {

				log.Infof(LogFormat, "Event", "VirtualService", obj.Name, clusterId, "Processing")

				if event == common.Delete {
					log.Infof(LogFormat, "Delete", "VirtualService", obj.Name, clusterId, "Success")
					err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Delete(obj.Name, &v12.DeleteOptions{})
					if err != nil {
						return err
					}

				} else {

					exist, _ := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(obj.Name, v12.GetOptions{})

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

					addUpdateVirtualService(obj, exist, syncNamespace, rc)
				}
			}
		}
		return nil
	} else {
		log.Infof(LogFormat, "Event", "VirtualService", obj.Name, clusterId, "No dependent clusters found")
	}

	//copy the VirtualService `as is` if they are not generated by Admiral (not in CnameDependentClusterCache)
	log.Infof(LogFormat, "Event", "VirtualService", obj.Name, clusterId, "Replicating `as is` to all clusters")
	remoteClusters := r.GetClusterIds()
	for _, ClusterID := range remoteClusters {
		if ClusterID != clusterId {
			rc := r.GetRemoteController(ClusterID)
			if event == common.Delete {
				err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Delete(obj.Name, &v12.DeleteOptions{})
				if err != nil {
					log.Infof(LogErrFormat, "Delete", "VirtualService", obj.Name, clusterId, err)
					return err
				} else {
					log.Infof(LogFormat, "Delete", "VirtualService", obj.Name, clusterId, "Success")
				}
			} else {
				exist, _ := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(obj.Name, v12.GetOptions{})
				addUpdateVirtualService(obj, exist, syncNamespace, rc)
			}
		}
	}
	return nil
}

func addUpdateVirtualService(obj *v1alpha3.VirtualService, exist *v1alpha3.VirtualService, namespace string, rc *RemoteController) {
	var err error
	var op string
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations["app.kubernetes.io/created-by"] = "admiral"
	if exist == nil || len(exist.Spec.Hosts) == 0 {
		obj.Namespace = namespace
		obj.ResourceVersion = ""
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Create(obj)
		op = "Add"
	} else {
		exist.Labels = obj.Labels
		exist.Annotations = obj.Annotations
		exist.Spec = obj.Spec
		op = "Update"
		_, err = rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Update(exist)
	}

	if err != nil {
		log.Errorf(LogErrFormat, op, "VirtualService", obj.Name, rc.ClusterID, err)
	} else {
		log.Infof(LogFormat, op, "VirtualService", obj.Name, rc.ClusterID, "Success")
	}
}

func addUpdateServiceEntry(obj *v1alpha3.ServiceEntry, exist *v1alpha3.ServiceEntry, namespace string, rc *RemoteController) {
	var err error
	var op, diff string
	var skipUpdate bool
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations["app.kubernetes.io/created-by"] = "admiral"
	if exist == nil || exist.Spec.Hosts == nil {
		obj.Namespace = namespace
		obj.ResourceVersion = ""
		_, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Create(obj)
		op = "Add"
		log.Infof(LogFormat+" SE=%s", op, "ServiceEntry", obj.Name, rc.ClusterID, "New SE", obj.Spec.String())
	} else {
		exist.Labels = obj.Labels
		exist.Annotations = obj.Annotations
		op = "Update"
		skipUpdate, diff = skipDestructiveUpdate(rc, obj, exist)
		if diff != "" {
			log.Infof(LogFormat+" diff=%s", op, "ServiceEntry", obj.Name, rc.ClusterID, "Diff in update", diff)
		}
		if skipUpdate {
			log.Infof(LogFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, "Update skipped as it was destructive during Admiral's bootup phase")
			return
		} else {
			exist.Spec = obj.Spec
			_, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Update(exist)
		}

	}

	if err != nil {
		log.Errorf(LogErrFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, err)
	} else {
		log.Infof(LogFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, "Success")
	}
}

func skipDestructiveUpdate(rc *RemoteController, new *v1alpha3.ServiceEntry, old *v1alpha3.ServiceEntry) (skipDestructive bool, diff string) {
	skipDestructive = false
	destructive, diff := getServiceEntryDiff(new, old)
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
	seNew := new.Spec
	seOld := old.Spec

	oldEndpointMap := make(map[string]*v1alpha32.ServiceEntry_Endpoint)
	found := make(map[string]string)
	for _, oEndpoint := range seOld.Endpoints {
		oldEndpointMap[oEndpoint.Address] = oEndpoint
	}
	for _, nEndpoint := range seNew.Endpoints {
		if val, ok := oldEndpointMap[nEndpoint.Address]; ok {
			found[nEndpoint.Address] = "1"
			if !reflect.DeepEqual(val, nEndpoint) {
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

func deleteServiceEntry(exist *v1alpha3.ServiceEntry, namespace string, rc *RemoteController) {
	if exist != nil {
		err := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Delete(exist.Name, &v12.DeleteOptions{})
		if err != nil {
			log.Errorf(LogErrFormat, "Delete", "ServiceEntry", exist.Name, rc.ClusterID, err)
		} else {
			log.Infof(LogFormat, "Delete", "ServiceEntry", exist.Name, rc.ClusterID, "Success")
		}
	}
}

func addUpdateDestinationRule(obj *v1alpha3.DestinationRule, exist *v1alpha3.DestinationRule, namespace string, rc *RemoteController) {
	var err error
	var op string
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations["app.kubernetes.io/created-by"] = "admiral"
	if exist == nil || exist.Name == "" || exist.Spec.Host == "" {
		obj.Namespace = namespace
		obj.ResourceVersion = ""
		_, err = rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Create(obj)
		op = "Add"
	} else {
		exist.Labels = obj.Labels
		exist.Annotations = obj.Annotations
		exist.Spec = obj.Spec
		op = "Update"
		_, err = rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Update(exist)
	}

	if err != nil {
		log.Errorf(LogErrFormat, op, "DestinationRule", obj.Name, rc.ClusterID, err)
	} else {
		log.Infof(LogFormat, op, "DestinationRule", obj.Name, rc.ClusterID, "Success")
	}
}

func deleteDestinationRule(exist *v1alpha3.DestinationRule, namespace string, rc *RemoteController) {
	if exist != nil {
		err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Delete(exist.Name, &v12.DeleteOptions{})
		if err != nil {
			log.Errorf(LogErrFormat, "Delete", "DestinationRule", exist.Name, rc.ClusterID, err)
		} else {
			log.Infof(LogFormat, "Delete", "DestinationRule", exist.Name, rc.ClusterID, "Success")
		}
	}
}
func createServiceEntrySkeletion(se v1alpha32.ServiceEntry, name string, namespace string) *v1alpha3.ServiceEntry {
	return &v1alpha3.ServiceEntry{Spec: se, ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace}}
}

func createSidecarSkeletion(sidecar v1alpha32.Sidecar, name string, namespace string) *v1alpha3.Sidecar {
	return &v1alpha3.Sidecar{Spec: sidecar, ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace}}
}

func createDestinationRuleSkeletion(dr v1alpha32.DestinationRule, name string, namespace string) *v1alpha3.DestinationRule {
	return &v1alpha3.DestinationRule{Spec: dr, ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace}}
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
			ports := GetMeshPorts(rc.ClusterID, service, deployment)
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

func copyEndpoint(e *v1alpha32.ServiceEntry_Endpoint) *v1alpha32.ServiceEntry_Endpoint {
	labels := make(map[string]string)
	util.MapCopy(labels, e.Labels)
	ports := make(map[string]uint32)
	util.MapCopy(ports, e.Ports)
	return &v1alpha32.ServiceEntry_Endpoint{Address: e.Address, Ports: ports, Locality: e.Locality, Labels: labels}
}

// A rollout can use one of 2 stratergies :-
// 1. Canary strategy - which can use a virtual service to manage the weights associated with a stable and canary service. Admiral created endpoints in service entries will use the weights assigned in the Virtual Service
// 2. Blue green strategy- this contains 2 service instances in a namespace, an active service and a preview service. Admiral will use repective service to create active and preview endpoints
func getServiceForRollout(rc *RemoteController, rollout *argo.Rollout) map[string]*WeightedService {

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

	var canaryService, stableService, virtualServiceRouteName string

	var istioCanaryWeights = make(map[string]int32)

	var blueGreenActiveService string
	var blueGreenPreviewService string

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

		//calculate canary weights if canary strategy is using Istio traffic management
		if len(stableService) > 0 && len(canaryService) > 0 && rolloutStrategy.Canary.TrafficRouting != nil && rolloutStrategy.Canary.TrafficRouting.Istio != nil {
			virtualServiceName := rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Name
			virtualService, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(rollout.Namespace).Get(virtualServiceName, v12.GetOptions{})

			if err != nil {
				log.Warnf("Error fetching VirtualService referenced in rollout canary for rollout with name=%s in namespace=%s and cluster=%s err=%v", rollout.Name, rollout.Namespace, rc.ClusterID, err)
			}

			if len(rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Routes) > 0 {
				virtualServiceRouteName = rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Routes[0]
			}

			if virtualService != nil {
				var vs = virtualService.Spec
				if len(vs.Http) > 0 {
					var httpRoute *v1alpha32.HTTPRoute
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
		}
	}

	var matchedServices = make(map[string]*WeightedService)

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
				if val, ok := istioCanaryWeights[service.Name]; ok {
					matchedServices[service.Name] = &WeightedService{Weight: val, Service: service}
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
