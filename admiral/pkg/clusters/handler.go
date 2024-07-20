package clusters

import (
	"context"
	"fmt"
	"sort"
	"strings"

	rolloutsV1Alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	admiralV1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	log "github.com/sirupsen/logrus"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultBaseEjectionTime         int64  = 300
	DefaultConsecutiveGatewayErrors uint32 = 50
	DefaultConsecutive5xxErrors     uint32 = 0
	DefaultInterval                 int64  = 60
	DefaultMaxRequestsPerConnection int32  = 100
)

// WeightedService utility to store weighted services for argo rollouts
type WeightedService struct {
	Weight  int32
	Service *coreV1.Service
}

func updateIdentityDependencyCache(sourceIdentity string, identityDependencyCache *common.MapOfMaps, dr *admiralV1.Dependency) error {
	for _, dIdentity := range dr.Spec.Destinations {
		identityDependencyCache.Put(dIdentity, sourceIdentity, sourceIdentity)
	}
	log.Debugf(LogFormat, "Update", "dependency-cache", dr.Name, "", "Updated=true namespace="+dr.Namespace)
	return nil
}

func getIstioResourceName(host string, suffix string) string {
	return strings.ToLower(host) + suffix
}

func IgnoreIstioResource(exportTo []string, annotations map[string]string, namespace string) bool {
	if len(annotations) > 0 && annotations[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof(LogFormat, "admiralIoIgnoreAnnotationCheck", "", "", "", "Value=true namespace="+namespace)
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

	if common.IsDefaultPersona() && len(annotations) > 0 && annotations[common.CreatedBy] == common.Cartographer {
		return true
	}

	return true
}

func getServiceForDeployment(rc *RemoteController, deployment *appsV1.Deployment) (*coreV1.Service, error) {
	if deployment == nil {
		return nil, fmt.Errorf(LogFormatAdv, "Get", "Service", "", "", rc.ClusterID, "error getting service, deployment is nil.")
	}

	if deployment.Spec.Selector == nil || deployment.Spec.Selector.MatchLabels == nil {
		return nil, fmt.Errorf(LogFormatAdv, "Get", "Service", deployment.Name, deployment.Namespace, rc.ClusterID, "no selectors found")
	}

	cachedServices := rc.ServiceController.Cache.Get(deployment.Namespace)
	if cachedServices == nil {
		return nil, fmt.Errorf(LogFormatAdv, "Get", "Service", deployment.Name, deployment.Namespace, rc.ClusterID, "no cached services found for deployment.")
	}

	// Sort the cachedServices such that the service are sorted based on creation time
	sort.Slice(cachedServices, func(i, j int) bool {
		return cachedServices[i].CreationTimestamp.Before(&cachedServices[j].CreationTimestamp)
	})

	var matchedService *coreV1.Service
	for _, service := range cachedServices {
		var match = common.IsServiceMatch(service.Spec.Selector, deployment.Spec.Selector)
		//make sure the service matches the deployment Selector and also has a mesh port in the port spec
		if match {
			ports := GetMeshPortsForDeployments(rc.ClusterID, service, deployment)
			if len(ports) > 0 {
				matchedService = service
				break
			}
		}
	}

	if matchedService == nil {
		return nil, fmt.Errorf(LogFormatAdv, "Get", "Service", deployment.Name, deployment.Namespace, rc.ClusterID, "no matching service instances found")
	}

	return matchedService, nil
}

func copyEndpoint(e *networkingV1Alpha3.WorkloadEntry) *networkingV1Alpha3.WorkloadEntry {
	var (
		labels = make(map[string]string)
		ports  = make(map[string]uint32)
	)
	util.MapCopy(labels, e.Labels)
	util.MapCopy(ports, e.Ports)
	return &networkingV1Alpha3.WorkloadEntry{Address: e.Address, Ports: ports, Locality: e.Locality, Labels: labels}
}

// A rollout can use one of 2 stratergies :-
// 1. Canary strategy - which can use a virtual service to manage the weights associated with a stable and canary service. Admiral created endpoints in service entries will use the weights assigned in the Virtual Service
// 2. Blue green strategy- this contains 2 service instances in a namespace, an active service and a preview service. Admiral will use repective service to create active and preview endpoints
func getServiceForRollout(ctx context.Context, rc *RemoteController, rollout *rolloutsV1Alpha1.Rollout) map[string]*WeightedService {
	if rollout == nil {
		return nil
	}

	if rollout.Spec.Selector == nil || rollout.Spec.Selector.MatchLabels == nil {
		log.Infof("No selector for rollout=%s in namespace=%s and cluster=%s", rollout.Name, rollout.Namespace, rc.ClusterID)
		return nil
	}

	cachedServices := rc.ServiceController.Cache.Get(rollout.Namespace)
	if cachedServices == nil {
		return nil
	}

	if rollout.Spec.Strategy == (rolloutsV1Alpha1.RolloutStrategy{}) {
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
		//If istio canary perform below operations
		if rolloutStrategy.Canary.TrafficRouting != nil && rolloutStrategy.Canary.TrafficRouting.Istio != nil {
			canaryService = rolloutStrategy.Canary.CanaryService
			stableService = rolloutStrategy.Canary.StableService

			//calculate canary weights if canary strategy is using Istio traffic management
			if len(stableService) > 0 && len(canaryService) > 0 {
				//pick stable service if specified
				istioCanaryWeights[stableService] = 1

				if rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService != nil && rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Name != "" {
					virtualServiceName := rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Name
					virtualService, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(rollout.Namespace).Get(ctx, virtualServiceName, metaV1.GetOptions{})

					if err != nil {
						log.Warnf("Error fetching VirtualService referenced in rollout canary for rollout with name=%s in namespace=%s and cluster=%s err=%v", rollout.Name, rollout.Namespace, rc.ClusterID, err)
					}

					if rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Routes != nil && len(rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Routes) > 0 {
						virtualServiceRouteName = rolloutStrategy.Canary.TrafficRouting.Istio.VirtualService.Routes[0]
					}

					if virtualService != nil {
						//nolint
						var vs = virtualService.Spec
						if len(vs.Http) > 0 {
							var httpRoute *networkingV1Alpha3.HTTPRoute
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
		} else {
			/*
				This change is for MESH-2786, where if not istio canary then all traffic will need to go to root service
				since istio does not know the split info as there is no virtual service
			*/

			sName := GetServiceWithSuffixMatch(common.RolloutRootServiceSuffix, cachedServices)
			if len(sName) <= 0 {
				//Fallback if root service not found
				log.Infof("root service not found, falling back to stable for rollout=%s in namespace=%s and cluster=%s", rollout.Name, rollout.Namespace, rc.ClusterID)
				sName = GetServiceWithSuffixMatch(common.RolloutStableServiceSuffix, cachedServices)
			}

			// If root and stable not found, exit canary logic and use generic logic to choose random service
			if len(sName) != 0 {
				for _, service := range cachedServices {
					if sName != service.Name {
						continue
					}
					match := common.IsServiceMatch(service.Spec.Selector, rollout.Spec.Selector)
					//make sure the service matches the rollout Selector and also has a mesh port in the port spec
					if match {
						ports := GetMeshPortsForRollout(rc.ClusterID, service, rollout)
						if len(ports) > 0 {
							//Adding 100% traffic to this service
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

func GetServiceWithSuffixMatch(suffix string, services []*coreV1.Service) string {
	for _, service := range services {
		if strings.HasSuffix(service.Name, suffix) {
			return service.Name
		}
	}
	return ""
}
