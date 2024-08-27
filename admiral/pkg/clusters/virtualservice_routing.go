package clusters

import (
	"context"
	"fmt"
	"strings"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultFQDN = "default"

	previewFQDN = "preview"
	canaryFQDN  = "canary"
)

// getBaseVirtualServiceForIngress generates the base virtual service for the ingress gateway
// The destinations should be added separately, this func does not have the context of the destinations.
func getBaseVirtualServiceForIngress(hosts, sniHosts []string) (*v1alpha3.VirtualService, error) {

	if len(hosts) == 0 {
		return nil, fmt.Errorf("host is empty")
	}

	if len(sniHosts) == 0 {
		return nil, fmt.Errorf("sniHost is empty")
	}

	gateways := common.GetVSRoutingGateways()
	if len(gateways) == 0 {
		return nil, fmt.Errorf("no gateways configured for ingress virtual service")
	}

	tlsRoutes := make([]*networkingV1Alpha3.TLSRoute, 0)

	for _, sniHost := range sniHosts {
		tlsRoute := networkingV1Alpha3.TLSRoute{
			Match: []*networkingV1Alpha3.TLSMatchAttributes{
				{
					Port:     common.DefaultMtlsPort,
					SniHosts: []string{sniHost},
				},
			},
		}
		tlsRoutes = append(tlsRoutes, &tlsRoute)
	}

	vs := networkingV1Alpha3.VirtualService{
		Hosts:    sniHosts,
		Gateways: gateways,
		ExportTo: []string{"istio-system"},
		Tls:      tlsRoutes,
	}

	// Explicitly labeling the VS for routing
	vsLabels := map[string]string{
		vsRoutingLabel: "enabled",
	}

	return &v1alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      hosts[0] + "-routing-vs",
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
	baseVS, err := getBaseVirtualServiceForIngress([]string{cname}, []string{sniHost})
	if err != nil {
		return err
	}
	sourceIngressVirtualService[cname] = baseVS
	return nil
}

// generateIngressVirtualServiceForRollout generates the base virtual service for a given rollout
// and adds it to the sourceIngressVirtualService map
// sourceIngressVirtualService is a map of globalFQDN (.mesh) endpoint to VirtualService
func generateIngressVirtualServiceForRollout(
	ctx context.Context,
	ctxLogger *log.Entry,
	rollout *argo.Rollout,
	sourceIngressVirtualService map[string]*v1alpha3.VirtualService,
	rc *RemoteController) error {
	if rollout == nil {
		return fmt.Errorf("rollout is nil")
	}
	workloadIdentityKey := common.GetWorkloadIdentifier()

	hosts := make([]string, 0)
	sniHosts := make([]string, 0)

	// Get default cname for rollout
	cname := common.GetCnameForRollout(rollout, workloadIdentityKey, common.GetHostnameSuffix())
	if cname == "" {
		return fmt.Errorf("cname is empty")
	}
	sniHost, err := generateSNIHost(cname)
	if err != nil {
		return err
	}
	hosts = append(hosts, cname)
	sniHosts = append(sniHosts, sniHost)

	// Get preview fqdn for the rollout if any
	previewCname := getPreviewFQDNForRollout(ctx, rollout, rc)
	if previewCname != "" {
		hosts = append(hosts, previewCname)
		previewSNIHost, err := generateSNIHost(previewCname)
		if err != nil {
			return err
		}
		sniHosts = append(sniHosts, previewSNIHost)
	}

	// Get canary FQDN for the rollout if any
	canaryCname := getCanaryFQDNForRollout(ctxLogger, rollout, rc)
	if canaryCname != "" {
		hosts = append(hosts, canaryCname)
		canarySNIHost, err := generateSNIHost(canaryCname)
		if err != nil {
			return err
		}
		sniHosts = append(sniHosts, canarySNIHost)
	}

	baseVS, err := getBaseVirtualServiceForIngress(hosts, sniHosts)
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

// getFQDNFromSNIHost extracts the FQDN from the SNI host
// Example: outbound_.80_._.httpbin.global.mesh -> httpbin.global.mesh
func getFQDNFromSNIHost(sniHost string) (string, error) {
	if sniHost == "" {
		return "", fmt.Errorf("sniHost is empty")
	}
	if !strings.HasPrefix(sniHost, "outbound_.") {
		return "", fmt.Errorf("invalid SNI host format, should start with outbound_")
	}
	prefix := fmt.Sprintf("outbound_.%d_._.", common.DefaultServiceEntryPort)
	return strings.Replace(sniHost, prefix, "", -1), nil
}

// getRouteDestination generates a RouteDestination object for the given host and port
func getRouteDestination(host string, port uint32) *networkingV1Alpha3.RouteDestination {
	return &networkingV1Alpha3.RouteDestination{
		Destination: &networkingV1Alpha3.Destination{
			Host: host,
			Port: &networkingV1Alpha3.PortSelector{
				Number: port,
			},
		},
	}
}

// populateVSRouteDestinationForDeployment populates the route destination map
// with key as defaultFQDN and value as the slice of route destination of the svc.cluster.local service
// Example: "default" ->
//
//	 route:
//	- destination:
//	    host: greeting.sample.svc.cluster.local
//	    port:
//	      number: 80
func populateVSRouteDestinationForDeployment(
	serviceInstance map[string]*k8sV1.Service,
	meshPort uint32,
	destinations map[string][]*networkingV1Alpha3.RouteDestination) error {

	if serviceInstance == nil {
		return fmt.Errorf("serviceInstance is nil")
	}
	if serviceInstance[common.Deployment] == nil {
		return fmt.Errorf("service is not associated with a deployment")
	}

	host := serviceInstance[common.Deployment].Name + "." +
		serviceInstance[common.Deployment].Namespace + ".svc.cluster.local"
	if destinations[defaultFQDN] == nil {
		destinations[defaultFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
	}
	destinations[defaultFQDN] = append(destinations[defaultFQDN], getRouteDestination(host, meshPort))
	return nil
}

// populateVSRouteDestinationForRollout populates the route destination map
// with key as default/preview/canary and value as the slice of route destination of
// bluegreen, canary or default service
// Example BlueGreen:
// "default" ->
//
// route:
//   - destination:
//     host: rollout-bluegreen-active.sample-rollout-bluegreen.svc.cluster.local
//     port:
//     number: 80
//
// "preview" ->
//
// route:
//   - destination:
//     host: rollout-bluegreen-preview.sample-rollout-bluegreen.svc.cluster.local
//     port:
//     number: 80
//
// Example Canary:
//
// "default" ->
//
//		route:
//		- destination:
//		    host: rollout-canary-svc.sample-rollout-canary.svc.cluster.local
//		    port:
//		      number: 80
//	     weight: 10
//		- destination:
//		    host: rollout-stable-svc.sample-rollout-canary.svc.cluster.local
//		    port:
//		      number: 80
//	     weight: 90
//
// "canary" ->
//
//	route:
//	- destination:
//	    host: rollout-canary-svc.sample-rollout-canary.svc.cluster.local
//	    port:
//	      number: 80
func populateVSRouteDestinationForRollout(
	serviceInstance map[string]*k8sV1.Service,
	weightedServices map[string]*WeightedService,
	rollout *argo.Rollout,
	meshPort uint32,
	destinations map[string][]*networkingV1Alpha3.RouteDestination) error {

	if serviceInstance == nil {
		return fmt.Errorf("serviceInstance is nil")
	}
	if serviceInstance[common.Rollout] == nil {
		return fmt.Errorf("service is not associated with a rollout")
	}
	if rollout == nil {
		return fmt.Errorf("rollout is nil")
	}

	// Check if rollout has a bluegreen strategy
	// If so, get the preview service svc.cluster.local destination
	// and add active service svc.cluster.local destination to defaultFQDN
	if isBlueGreenStrategy(rollout) {
		err := populateDestinationsForBlueGreenStrategy(weightedServices, rollout, meshPort, destinations)
		if err != nil {
			return err
		}
		return nil
	}

	// Check if rollout has a canary strategy
	// If so, get the canary service destination
	// and add the stable service destination to the defaultFQDN
	if IsCanaryIstioStrategy(rollout) {
		err := populateDestinationsForCanaryStrategy(serviceInstance[common.Rollout], weightedServices,
			rollout, meshPort, destinations)
		if err != nil {
			return err
		}
		return nil
	}

	// If we reach here, that means the rollout has neither
	// bluegreen or istio canary strategy
	// In this case we pick whatever service we got during the discovery
	// phase and add it as a defaultFQDN
	host := serviceInstance[common.Rollout].Name + "." + rollout.Namespace + ".svc.cluster.local"
	if destinations[defaultFQDN] == nil {
		destinations[defaultFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
	}
	destinations[defaultFQDN] = append(destinations[defaultFQDN], getRouteDestination(host, meshPort))
	return nil
}

// populateDestinationsForBlueGreenStrategy populates the route destination map with
// preview and active service destinations for the bluegreen rollout
func populateDestinationsForBlueGreenStrategy(
	weightedServices map[string]*WeightedService,
	rollout *argo.Rollout,
	meshPort uint32,
	destinations map[string][]*networkingV1Alpha3.RouteDestination) error {

	if rollout == nil {
		return fmt.Errorf("populateDestinationsForBlueGreenStrategy, rollout is nil")
	}
	if weightedServices == nil {
		return fmt.Errorf("populateDestinationsForBlueGreenStrategy, weightedServices is nil for rollout %s",
			rollout.Name)
	}

	previewServiceName := rollout.Spec.Strategy.BlueGreen.PreviewService
	if weightedPreviewService, ok := weightedServices[previewServiceName]; ok {
		previewServiceInstance := weightedPreviewService.Service
		host := previewServiceInstance.Name + common.Sep +
			previewServiceInstance.Namespace + common.GetLocalDomainSuffix()
		if destinations[previewFQDN] == nil {
			destinations[previewFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
		}
		destinations[previewFQDN] = append(destinations[previewFQDN], getRouteDestination(host, meshPort))
	}
	activeServiceName := rollout.Spec.Strategy.BlueGreen.ActiveService
	if activeService, ok := weightedServices[activeServiceName]; ok {
		activeServiceInstance := activeService.Service
		host := activeServiceInstance.Name + common.Sep +
			activeServiceInstance.Namespace + common.GetLocalDomainSuffix()
		if destinations[defaultFQDN] == nil {
			destinations[defaultFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
		}
		destinations[defaultFQDN] = append(destinations[defaultFQDN], getRouteDestination(host, meshPort))
	}
	return nil
}

// populateDestinationsForCanaryStrategy populates the route destination map with
// canary and stable service destinations for the canary rollout
func populateDestinationsForCanaryStrategy(
	serviceInstance *k8sV1.Service,
	weightedServices map[string]*WeightedService,
	rollout *argo.Rollout,
	meshPort uint32,
	destinations map[string][]*networkingV1Alpha3.RouteDestination) error {

	if serviceInstance == nil {
		return fmt.Errorf("populateDestinationsForCanaryStrategy, serviceInstance is nil")
	}
	if rollout == nil {
		return fmt.Errorf("populateDestinationsForCanaryStrategy, rollout is nil")
	}
	if weightedServices == nil {
		return fmt.Errorf("populateDestinationsForCanaryStrategy, weightedServices is nil for rollout %s",
			rollout.Name)
	}

	// Loop through the weightedService map and add cluster local service destinations
	// This map should contain the canary or stable service if the VS associated has weights
	// greater than 0.
	// If not, then it would have only one service which has weight set to 100 and that would be
	// considered as the default service.
	for serviceName, service := range weightedServices {
		host := serviceName + common.Sep + service.Service.Namespace + common.GetLocalDomainSuffix()
		if destinations[defaultFQDN] == nil {
			destinations[defaultFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
		}
		destinations[defaultFQDN] = append(destinations[defaultFQDN], getRouteDestination(host, meshPort))
	}

	// Here we will create a separate canary destination for the canary FQDN
	// This is needed to provide users to validate their canary endpoints
	serviceNamespace := serviceInstance.Namespace
	canaryServiceName := rollout.Spec.Strategy.Canary.CanaryService
	host := canaryServiceName + common.Sep + serviceNamespace + common.GetLocalDomainSuffix()
	if destinations[canaryFQDN] == nil {
		destinations[canaryFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
	}
	destinations[canaryFQDN] = append(destinations[canaryFQDN], getRouteDestination(host, meshPort))

	return nil
}

// addUpdateVirtualServicesForSourceIngress adds or updates the cross-cluster routing VirtualServices
// This is where the base Ingress VirtualServices are updated with the route destinations
// that were created during the discovery phase
func addUpdateVirtualServicesForSourceIngress(
	ctx context.Context,
	ctxLogger *log.Entry,
	remoteRegistry *RemoteRegistry,
	sourceIngressVirtualService map[string]*v1alpha3.VirtualService,
	sourceClusterToDestinations map[string]map[string][]*networkingV1Alpha3.RouteDestination) error {

	if remoteRegistry == nil {
		return fmt.Errorf("remoteRegistry is nil")
	}

	if len(sourceClusterToDestinations) == 0 {
		return fmt.Errorf("no destination found for the ingress virtualservice")
	}

	for sourceCluster, destination := range sourceClusterToDestinations {
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
			matchesWithNoDestinations := make([]int, 0)
			for i, tlsRoute := range virtualService.Spec.Tls {
				tlsMatches := tlsRoute.Match
				for _, tlsMatch := range tlsMatches {
					if len(tlsMatch.SniHosts) == 0 {
						return fmt.Errorf("no SNIHosts found in the ingress virtualservice %s",
							virtualService.Name)
					}
					if len(tlsMatch.SniHosts) > 1 {
						return fmt.Errorf("more than one SNIHosts found in the ingress virtualservice %s",
							virtualService.Name)
					}
					routeDestinations := destination[defaultFQDN]
					sniHost := tlsMatch.SniHosts[0]
					hostWithoutSNIPrefix, err := getFQDNFromSNIHost(sniHost)
					if err != nil {
						ctxLogger.Infof(common.CtxLogFormat, "addUpdateVirtualServicesForSourceIngress",
							virtualService.Name, virtualService.Namespace, sourceCluster, err.Error())
						continue
					}
					if strings.HasPrefix(hostWithoutSNIPrefix, common.BlueGreenRolloutPreviewPrefix) {
						if destination[previewFQDN] == nil {
							ctxLogger.Warnf(common.CtxLogFormat, "addUpdateVirtualServicesForSourceIngress",
								"", "", sourceCluster,
								"skipped adding preview service, no valid route destinaton found")
							continue
						}
						routeDestinations = destination[previewFQDN]
					}
					if strings.HasPrefix(hostWithoutSNIPrefix, common.CanaryRolloutCanaryPrefix) {
						if destination[canaryFQDN] == nil {
							ctxLogger.Warnf(common.CtxLogFormat, "addUpdateVirtualServicesForSourceIngress",
								"", "", sourceCluster,
								"skipped adding canary service, no valid route destinaton found")
							continue
						}
						routeDestinations = destination[canaryFQDN]
					}
					if len(routeDestinations) == 0 {
						matchesWithNoDestinations = append(matchesWithNoDestinations, i)
						continue
					}
					tlsRoute.Route = routeDestinations
				}

			}

			// Remove the matches with no destinations
			for _, indexToBeDeleted := range matchesWithNoDestinations {
				virtualService.Spec.Tls[indexToBeDeleted] = virtualService.Spec.Tls[len(virtualService.Spec.Tls)-1]
				virtualService.Spec.Tls = virtualService.Spec.Tls[:len(virtualService.Spec.Tls)-1]
			}

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

// getAllVSRouteDestinationsByCluster generates the route destinations for each source cluster
// This is during the discovery phase where the route destinations are created for each source cluster
// For a given identity and env, we are going to build a map of all possible services across deployments
// and rollouts. This map will be used to create the route destinations for the VirtualService
func getAllVSRouteDestinationsByCluster(
	serviceInstance map[string]*k8sV1.Service,
	meshDeployAndRolloutPorts map[string]map[string]uint32,
	weightedServices map[string]*WeightedService,
	rollout *argo.Rollout) (map[string][]*networkingV1Alpha3.RouteDestination, error) {

	destinations := make(map[string][]*networkingV1Alpha3.RouteDestination)
	if serviceInstance[common.Deployment] != nil {
		meshPort, err := getMeshHTTPPortForDeployment(meshDeployAndRolloutPorts)
		if err != nil {
			return nil, err
		}
		err = populateVSRouteDestinationForDeployment(
			serviceInstance, meshPort, destinations)
		if err != nil {
			return nil, err
		}
	}
	if serviceInstance[common.Rollout] != nil {
		meshPort, err := getMeshHTTPPortForRollout(meshDeployAndRolloutPorts)
		if err != nil {
			return nil, err
		}
		err = populateVSRouteDestinationForRollout(
			serviceInstance, weightedServices, rollout, meshPort, destinations)
		if err != nil {
			return nil, err
		}
	}
	return destinations, nil
}

// getMeshHTTPPortForRollout gets the mesh http port for the rollout
func getMeshHTTPPortForRollout(ports map[string]map[string]uint32) (uint32, error) {
	if ports == nil {
		return 0, fmt.Errorf("ports map is nil")
	}
	rolloutPorts, ok := ports[common.Rollout]
	if !ok {
		return 0, fmt.Errorf("rollout ports not found")
	}
	for _, port := range rolloutPorts {
		if port > 0 {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no valid port found for rollout")
}

// getMeshHTTPPortForDeployment gets the mesh http port for the deployment
func getMeshHTTPPortForDeployment(ports map[string]map[string]uint32) (uint32, error) {
	if ports == nil {
		return 0, fmt.Errorf("ports map is nil")
	}
	deploymentPorts, ok := ports[common.Deployment]
	if !ok {
		return 0, fmt.Errorf("deployment ports not found")
	}
	for _, port := range deploymentPorts {
		if port > 0 {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no valid port found for deployment")
}
