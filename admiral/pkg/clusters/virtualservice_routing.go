package clusters

import (
	"context"
	"fmt"
	"sort"
	"strings"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	log "github.com/sirupsen/logrus"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getBaseVirtualServiceForIngress generates the base virtual service for the ingress gateway
// This is just the barebones of the ingress virtual service
func getBaseVirtualServiceForIngress() (*v1alpha3.VirtualService, error) {

	gateways := common.GetVSRoutingGateways()
	if len(gateways) == 0 {
		return nil, fmt.Errorf("no gateways configured for ingress virtual service")
	}

	vs := networkingV1Alpha3.VirtualService{
		Gateways: gateways,
		ExportTo: common.GetIngressVSExportToNamespace(),
	}

	// Explicitly labeling the VS for routing
	vsLabels := map[string]string{
		vsRoutingLabel: "enabled",
	}

	return &v1alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: util.IstioSystemNamespace,
			Labels:    vsLabels,
		},
		Spec: vs,
	}, nil
}

// getDefaultSNIHostFromDeployment generates the SNI host from the identity and env
// derived from the deployment
//
// Example: outbound_.80_._.stage.greeting.global
func getDefaultSNIHostFromDeployment(deployment *k8sAppsV1.Deployment) (string, error) {

	if deployment == nil {
		return "", fmt.Errorf("deployment is nil")
	}
	workloadIdentityKey := common.GetWorkloadIdentifier()
	cname := common.GetCname(deployment, workloadIdentityKey, common.GetHostnameSuffix())
	if cname == "" {
		return "", fmt.Errorf("cname is empty")
	}
	sniHost, err := generateSNIHost(cname)
	if err != nil {
		return "", err
	}
	return sniHost, nil
}

// getDefaultSNIHostFromRollout generates the default SNI host from the identity and env
// derived from the rollout
//
// Example: outbound_.80_._.stage.greeting.global
func getDefaultSNIHostFromRollout(rollout *argo.Rollout) (string, error) {

	if rollout == nil {
		return "", fmt.Errorf("rollout is nil")
	}
	workloadIdentityKey := common.GetWorkloadIdentifier()
	// Get default cname for rollout
	cname := common.GetCnameForRollout(rollout, workloadIdentityKey, common.GetHostnameSuffix())
	if cname == "" {
		return "", fmt.Errorf("cname is empty")
	}
	sniHost, err := generateSNIHost(cname)
	if err != nil {
		return "", err
	}
	return sniHost, nil
}

// getCanarySNIHostFromRollout generates the canary SNI host from the identity and env
// derived from the rollout
//
// Example: outbound_.80_._.canary.stage.greeting.canary.global
func getCanarySNIHostFromRollout(rollout *argo.Rollout) (string, error) {

	if rollout == nil {
		return "", fmt.Errorf("rollout is nil")
	}

	cName := common.GetCnameForRollout(rollout, common.GetWorkloadIdentifier(), common.GetHostnameSuffix())
	if cName == "" {
		return "", fmt.Errorf("getCanaryFQDNForRollout, unable to get cname for rollout %s", rollout.Name)
	}
	canaryCname := common.CanaryRolloutCanaryPrefix + common.Sep + cName

	sniHost, err := generateSNIHost(canaryCname)
	if err != nil {
		return "", err
	}

	return sniHost, nil
}

// getPreviewSNIHostFromRollout generates the preview SNI host from the identity and env
// derived from the rollout
//
// Example: outbound_.80_._.preview.stage.greeting.canary.global
func getPreviewSNIHostFromRollout(rollout *argo.Rollout) (string, error) {

	if rollout == nil {
		return "", fmt.Errorf("rollout is nil")
	}

	cName := common.GetCnameForRollout(rollout, common.GetWorkloadIdentifier(), common.GetHostnameSuffix())
	if cName == "" {
		return "", fmt.Errorf("getPreviewFQDNForRollout, unable to get cname for rollout %s", rollout.Name)
	}

	previewCname := common.BlueGreenRolloutPreviewPrefix + common.Sep + cName

	sniHost, err := generateSNIHost(previewCname)
	if err != nil {
		return "", err
	}

	return sniHost, nil
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
func getRouteDestination(host string, port uint32, weight int32) *networkingV1Alpha3.RouteDestination {
	routeDestination := &networkingV1Alpha3.RouteDestination{
		Destination: &networkingV1Alpha3.Destination{
			Host: host,
			Port: &networkingV1Alpha3.PortSelector{
				Number: port,
			},
		},
	}
	if weight > 0 {
		routeDestination.Weight = weight
	}
	return routeDestination
}

// populateVSRouteDestinationForDeployment populates the route destination map
// with key as SNI host and value as the slice of route destination of the svc.cluster.local service
//
// Example:
//
// "outbound_.80_._.stage.greeting.global" ->
//
//	 route:
//	- destination:
//	    host: greeting.sample.svc.cluster.local
//	    port:
//	      number: 80
func populateVSRouteDestinationForDeployment(
	serviceInstance map[string]*k8sV1.Service,
	meshPort uint32,
	deployment *k8sAppsV1.Deployment,
	destinations map[string][]*networkingV1Alpha3.RouteDestination) error {

	if serviceInstance == nil {
		return fmt.Errorf("serviceInstance is nil")
	}
	if serviceInstance[common.Deployment] == nil {
		return fmt.Errorf("service is not associated with a deployment")
	}
	if destinations == nil {
		return fmt.Errorf("destinations map is nil")
	}

	globalFQDN, err := getDefaultSNIHostFromDeployment(deployment)
	if err != nil {
		return err
	}

	host := serviceInstance[common.Deployment].Name + "." +
		serviceInstance[common.Deployment].Namespace + common.DotLocalDomainSuffix
	if destinations[globalFQDN] == nil {
		destinations[globalFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
	}
	destinations[globalFQDN] = append(destinations[globalFQDN], getRouteDestination(host, meshPort, 0))
	return nil
}

// populateVSRouteDestinationForRollout populates the route destination map
// with key as SNI host and value as the slice of route destination of
// bluegreen, canary or default service
// Example BlueGreen:
// "outbound_.80_._.stage.greeting.global" ->
//
// route:
//   - destination:
//     host: rollout-bluegreen-active.sample-rollout-bluegreen.svc.cluster.local
//     port:
//     number: 80
//
// "outbound_.80_._.preview.stage.greeting.global" ->
//
// route:
//   - destination:
//     host: rollout-bluegreen-preview.sample-rollout-bluegreen.svc.cluster.local
//     port:
//     number: 80
//
// Example Canary:
//
// "outbound_.80_._.stage.greeting.global" ->
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
// "outbound_.80_._.canary.stage.greeting.global" ->
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
	if destinations == nil {
		return fmt.Errorf("destinations map is nil")
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
		err := populateDestinationsForCanaryStrategy(
			serviceInstance[common.Rollout], weightedServices, rollout, meshPort, destinations)
		if err != nil {
			return err
		}
		return nil
	}

	// If we reach here, that means the rollout has neither
	// bluegreen or istio canary strategy
	// In this case we pick whatever service we got during the discovery
	// phase and add it as a defaultFQDN
	defaultFQDN, err := getDefaultSNIHostFromRollout(rollout)
	if err != nil {
		return err
	}
	host := serviceInstance[common.Rollout].Name + "." +
		serviceInstance[common.Rollout].Namespace + common.DotLocalDomainSuffix
	if destinations[defaultFQDN] == nil {
		destinations[defaultFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
	}
	destinations[defaultFQDN] = append(destinations[defaultFQDN], getRouteDestination(host, meshPort, 0))

	if len(destinations[defaultFQDN]) > 1 {
		sort.Sort(RouteDestinationSorted(destinations[defaultFQDN]))
	}

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
	if destinations == nil {
		return fmt.Errorf("populateDestinationsForBlueGreenStrategy, destinations is nil for rollout %s",
			rollout.Name)

	}

	previewServiceName := rollout.Spec.Strategy.BlueGreen.PreviewService
	if weightedPreviewService, ok := weightedServices[previewServiceName]; ok {
		previewFQDN, err := getPreviewSNIHostFromRollout(rollout)
		if err != nil {
			return err
		}
		previewServiceInstance := weightedPreviewService.Service
		host := previewServiceInstance.Name + common.Sep +
			previewServiceInstance.Namespace + common.DotLocalDomainSuffix
		if destinations[previewFQDN] == nil {
			destinations[previewFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
		}
		destinations[previewFQDN] = append(destinations[previewFQDN], getRouteDestination(host, meshPort, 0))
	}
	activeServiceName := rollout.Spec.Strategy.BlueGreen.ActiveService
	if activeService, ok := weightedServices[activeServiceName]; ok {
		defaultFQDN, err := getDefaultSNIHostFromRollout(rollout)
		if err != nil {
			return err
		}
		activeServiceInstance := activeService.Service
		host := activeServiceInstance.Name + common.Sep +
			activeServiceInstance.Namespace + common.DotLocalDomainSuffix
		if destinations[defaultFQDN] == nil {
			destinations[defaultFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
		}
		destinations[defaultFQDN] = append(destinations[defaultFQDN], getRouteDestination(host, meshPort, 0))

		if len(destinations[defaultFQDN]) > 1 {
			sort.Sort(RouteDestinationSorted(destinations[defaultFQDN]))
		}
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
	if destinations == nil {
		return fmt.Errorf("populateDestinationsForCanaryStrategy, destinations is nil for rollout %s",
			rollout.Name)

	}

	defaultFQDN, err := getDefaultSNIHostFromRollout(rollout)
	if err != nil {
		return err
	}
	canaryFQDN, err := getCanarySNIHostFromRollout(rollout)
	if err != nil {
		return err
	}

	canaryServiceName := rollout.Spec.Strategy.Canary.CanaryService
	// Loop through the weightedService map and add cluster local service destinations
	// This map should contain the canary or stable service if the VS associated has weights
	// greater than 0.
	// If not, then it would have only one service which has weight set to 100 and that would be
	// considered as the default service.
	weight := int32(0)
	for serviceName, service := range weightedServices {
		host := serviceName + common.Sep + service.Service.Namespace + common.DotLocalDomainSuffix
		if destinations[defaultFQDN] == nil {
			destinations[defaultFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
		}
		if service.Weight > 0 {
			weight = service.Weight
		}
		destinations[defaultFQDN] = append(destinations[defaultFQDN], getRouteDestination(host, meshPort, weight))
	}

	if len(destinations[defaultFQDN]) > 1 {
		sort.Sort(RouteDestinationSorted(destinations[defaultFQDN]))
	}

	// Here we will create a separate canary destination for the canary FQDN
	// This is needed to provide users to validate their canary endpoints
	serviceNamespace := serviceInstance.Namespace
	host := canaryServiceName + common.Sep + serviceNamespace + common.DotLocalDomainSuffix
	if destinations[canaryFQDN] == nil {
		destinations[canaryFQDN] = make([]*networkingV1Alpha3.RouteDestination, 0)
	}
	destinations[canaryFQDN] = append(destinations[canaryFQDN], getRouteDestination(host, meshPort, 0))

	return nil
}

// addUpdateVirtualServicesForSourceIngress adds or updates the cross-cluster routing VirtualServices
// This is where the VirtualServices are created using the services that were discovered during the
// discovery phase.
func addUpdateVirtualServicesForIngress(
	ctx context.Context,
	ctxLogger *log.Entry,
	remoteRegistry *RemoteRegistry,
	sourceClusterToDestinations map[string]map[string][]*networkingV1Alpha3.RouteDestination,
	vsName string) error {

	if remoteRegistry == nil {
		return fmt.Errorf("remoteRegistry is nil")
	}

	if vsName == "" {
		return fmt.Errorf("vsName is empty")
	}

	for sourceCluster, destination := range sourceClusterToDestinations {

		if !common.DoVSRoutingForCluster(sourceCluster) {
			continue
		}

		ctxLogger.Warnf(common.CtxLogFormat, "VSBasedRouting",
			"", "", sourceCluster,
			"Writing phase: addUpdateVirtualServicesForIngress VS based routing enabled for cluster")

		rc := remoteRegistry.GetRemoteController(sourceCluster)

		if rc == nil {
			ctxLogger.Warnf(common.CtxLogFormat, "addUpdateVirtualServicesForIngress",
				"", "", sourceCluster, "remote controller not initialized on this cluster")
			continue
		}

		virtualService, err := getBaseVirtualServiceForIngress()
		if err != nil {
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateVirtualServicesForIngress",
				virtualService.Name, virtualService.Namespace, sourceCluster, err.Error())
			return err
		}

		vsHosts := make([]string, 0)
		tlsRoutes := make([]*networkingV1Alpha3.TLSRoute, 0)

		for globalFQDN, routeDestinations := range destination {
			if routeDestinations == nil || len(routeDestinations) == 0 {
				ctxLogger.Warnf(common.CtxLogFormat, "addUpdateVirtualServicesForIngress",
					"", "", sourceCluster,
					fmt.Sprintf("skipped adding host %s, no valid route destinaton found", globalFQDN))
				continue
			}
			tlsRoute := networkingV1Alpha3.TLSRoute{
				Match: []*networkingV1Alpha3.TLSMatchAttributes{
					{
						Port:     common.DefaultMtlsPort,
						SniHosts: []string{globalFQDN},
					},
				},
				Route: routeDestinations,
			}
			tlsRoutes = append(tlsRoutes, &tlsRoute)
			vsHosts = append(vsHosts, globalFQDN)
		}

		if len(vsHosts) == 0 {
			err := fmt.Errorf(
				"skipped creating virtualservice on cluster %s, no valid hosts found", sourceCluster)
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateVirtualServicesForIngress",
				virtualService.Name, virtualService.Namespace, sourceCluster, err.Error())
			return err
		}
		if len(tlsRoutes) == 0 {
			err := fmt.Errorf(
				"skipped creating virtualservice on cluster %s, no valid tls routes found", sourceCluster)
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateVirtualServicesForIngress",
				virtualService.Name, virtualService.Namespace, sourceCluster, err.Error())
			return err
		}
		sort.Strings(vsHosts)
		virtualService.Spec.Hosts = vsHosts
		sort.Slice(tlsRoutes, func(i, j int) bool {
			return tlsRoutes[i].Match[0].SniHosts[0] < tlsRoutes[j].Match[0].SniHosts[0]
		})
		virtualService.Spec.Tls = tlsRoutes

		virtualService.Name = vsName + "-routing-vs"

		existingVS, err := getExistingVS(ctxLogger, ctx, rc, virtualService.Name, util.IstioSystemNamespace)
		if err != nil {
			ctxLogger.Warn(common.CtxLogFormat, "addUpdateVirtualServicesForIngress",
				virtualService.Name, virtualService.Namespace, sourceCluster, err.Error())
		}

		ctxLogger.Infof(common.CtxLogFormat, "addUpdateVirtualServicesForIngress",
			virtualService.Name, virtualService.Namespace, sourceCluster, "Add/Update ingress virtualservice")
		err = addUpdateVirtualService(
			ctxLogger, ctx, virtualService, existingVS, util.IstioSystemNamespace, rc, remoteRegistry)
		if err != nil {
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateVirtualServicesForIngress",
				virtualService.Name, virtualService.Namespace, sourceCluster, err.Error())
			return err
		}
		ctxLogger.Infof(common.CtxLogFormat, "addUpdateVirtualServicesForIngress",
			virtualService.Name, virtualService.Namespace, sourceCluster, "virtualservice created successfully")
	}

	return nil
}

// getAllVSRouteDestinationsByCluster generates the route destinations for each source cluster
// This is during the discovery phase where the route destinations are created for each source cluster
// For a given identity and env, we are going to build a map of all possible services across deployments
// and rollouts. IN addition, it also adds additional endpoints created using GTP DNS Prefix.
// This map will be used to create the route destinations for the VirtualService
func getAllVSRouteDestinationsByCluster(
	ctxLogger *log.Entry,
	serviceInstance map[string]*k8sV1.Service,
	meshDeployAndRolloutPorts map[string]map[string]uint32,
	weightedServices map[string]*WeightedService,
	rollout *argo.Rollout,
	deployment *k8sAppsV1.Deployment,
	remoteRegistry *RemoteRegistry,
	sourceIdentity string,
	env string) (map[string][]*networkingV1Alpha3.RouteDestination, error) {

	if serviceInstance == nil {
		return nil, fmt.Errorf("serviceInstance is nil")
	}

	destinations := make(map[string][]*networkingV1Alpha3.RouteDestination)

	// Populate the route destinations(svc.cluster.local services) for the deployment
	if serviceInstance[common.Deployment] != nil {
		meshPort, err := getMeshHTTPPortForDeployment(meshDeployAndRolloutPorts)
		if err != nil {
			return nil, err
		}
		err = populateVSRouteDestinationForDeployment(
			serviceInstance, meshPort, deployment, destinations)
		if err != nil {
			return nil, err
		}
	}

	// Populate the route destinations(svc.cluster.local services) for the rollout
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

	// Get the global traffic policy for the env and identity
	// and add the additional endpoints/hosts to the destination map
	globalTrafficPolicy, err := remoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(sourceIdentity, env)
	if err != nil {
		return nil, err
	}
	if globalTrafficPolicy != nil {
		// Add the global traffic policy destinations to the destination map
		gtpDestinations, err := getDestinationsForGTPDNSPrefixes(ctxLogger, globalTrafficPolicy, destinations, env)
		if err != nil {
			return nil, err
		}
		for fqdn, routeDestinations := range gtpDestinations {
			destinations[fqdn] = routeDestinations
		}
	}

	return destinations, nil
}

func getDestinationsForGTPDNSPrefixes(
	ctxLogger *log.Entry,
	globalTrafficPolicy *v1alpha1.GlobalTrafficPolicy,
	destinations map[string][]*networkingV1Alpha3.RouteDestination,
	env string) (map[string][]*networkingV1Alpha3.RouteDestination, error) {

	if globalTrafficPolicy == nil {
		return nil, fmt.Errorf("globaltrafficpolicy is nil")
	}
	if destinations == nil {
		return nil, fmt.Errorf("destinations map is nil")
	}

	gtpDestinations := make(map[string][]*networkingV1Alpha3.RouteDestination)
	for globalFQDN, routeDestinations := range destinations {

		if routeDestinations == nil {
			ctxLogger.Warnf(common.CtxLogFormat, "getDestinationsForGTPDNSPrefixes",
				"", "", globalFQDN, "route destinations is nil")
			continue
		}

		hostWithoutSNIPrefix, err := getFQDNFromSNIHost(globalFQDN)
		if err != nil {
			return nil, err
		}

		if strings.HasPrefix(hostWithoutSNIPrefix, common.BlueGreenRolloutPreviewPrefix) {
			continue
		}

		for _, policy := range globalTrafficPolicy.Spec.Policy {
			if policy.DnsPrefix == common.Default || policy.DnsPrefix == env {
				continue
			}
			newDNSPrefixedSNIHost, err := generateSNIHost(policy.DnsPrefix + common.Sep + hostWithoutSNIPrefix)
			if err != nil {
				return nil, err
			}
			newRD, err := copyRouteDestinations(routeDestinations)
			if err != nil {
				return nil, err
			}
			gtpDestinations[newDNSPrefixedSNIHost] = newRD
		}

	}

	return gtpDestinations, nil
}

func copyRouteDestinations(
	routeDestination []*networkingV1Alpha3.RouteDestination) ([]*networkingV1Alpha3.RouteDestination, error) {
	if routeDestination == nil {
		return nil, fmt.Errorf("routeDestination is nil")
	}
	newRouteDestinations := make([]*networkingV1Alpha3.RouteDestination, 0)
	for _, rd := range routeDestination {
		var newRD = &networkingV1Alpha3.RouteDestination{}
		rd.DeepCopyInto(newRD)
		newRouteDestinations = append(newRouteDestinations, newRD)
	}
	return newRouteDestinations, nil
}

func getMeshHTTPPort(
	resourceType string,
	ports map[string]map[string]uint32) (uint32, error) {
	if ports == nil {
		return 0, fmt.Errorf("ports map is nil")
	}
	if len(ports) == 0 {
		return 0, fmt.Errorf("ports map is empty")
	}
	deploymentPorts, ok := ports[resourceType]
	if !ok {
		return 0, fmt.Errorf("%s ports not found", resourceType)
	}
	for _, port := range deploymentPorts {
		if port > 0 {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no valid port found for %s", resourceType)
}

// getMeshHTTPPortForRollout gets the mesh http port for the rollout
func getMeshHTTPPortForRollout(ports map[string]map[string]uint32) (uint32, error) {
	return getMeshHTTPPort(common.Rollout, ports)
}

// getMeshHTTPPortForDeployment gets the mesh http port for the deployment
func getMeshHTTPPortForDeployment(ports map[string]map[string]uint32) (uint32, error) {
	return getMeshHTTPPort(common.Deployment, ports)
}

// addUpdateDestinationRuleForSourceIngress adds or updates the DestinationRule for the source ingress
// This is where the DestinationRules are created for the cross-cluster VS based routing
// The DestinationRule is created for the .svc.cluster.local hosts that were discovered during the discovery phase
// on each source cluster
func addUpdateDestinationRuleForSourceIngress(
	ctx context.Context,
	ctxLogger *log.Entry,
	remoteRegistry *RemoteRegistry,
	sourceClusterToDRHosts map[string]map[string]string,
	sourceIdentity string) error {

	if remoteRegistry == nil {
		return fmt.Errorf("remoteRegistry is nil")
	}

	for sourceCluster, drHosts := range sourceClusterToDRHosts {

		if !common.DoVSRoutingForCluster(sourceCluster) {
			continue
		}

		ctxLogger.Info(common.CtxLogFormat, "VSBasedRouting",
			"", "", sourceCluster,
			"Writing phase: addUpdateDestinationRuleForSourceIngress VS based routing enabled for cluster")

		if sourceIdentity == "" {
			err := fmt.Errorf("sourceIdentity is empty")
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateDestinationRuleForSourceIngress",
				"", "", sourceCluster, err.Error())
			return err
		}

		san := fmt.Sprintf("%s%s/%s", common.SpiffePrefix, common.GetSANPrefix(), sourceIdentity)

		rc := remoteRegistry.GetRemoteController(sourceCluster)
		if rc == nil {
			ctxLogger.Warnf(common.CtxLogFormat, "addUpdateDestinationRuleForSourceIngress",
				"", "", sourceCluster, "remote controller not initialized on this cluster")
			continue
		}

		for name, drHost := range drHosts {
			drObj := networkingV1Alpha3.DestinationRule{
				Host:     drHost,
				ExportTo: common.GetIngressVSExportToNamespace(),
				TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
					LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
						LbPolicy: &networkingV1Alpha3.LoadBalancerSettings_Simple{
							Simple: getIngressDRLoadBalancerPolicy(),
						},
						LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
							Enabled: &wrappers.BoolValue{Value: false},
						},
					},
					Tls: &networkingV1Alpha3.ClientTLSSettings{
						SubjectAltNames: []string{san},
					},
				},
			}

			if common.IsSlowStartEnabledForCluster(sourceCluster) {
				drObj.TrafficPolicy.LoadBalancer.WarmupDurationSecs =
					&duration.Duration{Seconds: common.GetDefaultWarmupDurationSecs()}
			}

			drName := fmt.Sprintf("%s-routing-dr", name)

			newDR := createDestinationRuleSkeleton(drObj, drName, util.IstioSystemNamespace)

			//Get existing DR
			existingDR, err := rc.
				DestinationRuleController.
				IstioClient.
				NetworkingV1alpha3().
				DestinationRules(util.IstioSystemNamespace).Get(ctx, drName, metaV1.GetOptions{})
			if err != nil {
				ctxLogger.Warnf(common.CtxLogFormat,
					"addUpdateDestinationRuleForSourceIngress",
					drName,
					util.IstioSystemNamespace,
					sourceCluster, fmt.Sprintf("failed getting existing DR, error=%v", err))
				existingDR = nil
			}

			ctxLogger.Infof(common.CtxLogFormat, "addUpdateDestinationRuleForSourceIngress",
				drName, util.IstioSystemNamespace, sourceCluster, "Add/Update ingress destinationrule")

			err = addUpdateDestinationRule(ctxLogger, ctx, newDR, existingDR, util.IstioSystemNamespace, rc, remoteRegistry)
			if err != nil {
				ctxLogger.Errorf(common.CtxLogFormat, "addUpdateDestinationRuleForSourceIngress",
					drName, util.IstioSystemNamespace, sourceCluster, err.Error())
				return err
			}

			ctxLogger.Infof(common.CtxLogFormat, "addUpdateDestinationRuleForSourceIngress",
				drName, util.IstioSystemNamespace, sourceCluster, "destinationrule created successfully")

		}

	}
	return nil
}

// getIngressDRLoadBalancerPolicy return the load balancer policy for the ingress destination rule
// Default is networkingV1Alpha3.LoadBalancerSettings_ROUND_ROBIN
func getIngressDRLoadBalancerPolicy() networkingV1Alpha3.LoadBalancerSettings_SimpleLB {

	switch common.GetIngressLBPolicy() {
	case "round_robin":
		return networkingV1Alpha3.LoadBalancerSettings_ROUND_ROBIN
	case "random":
		return networkingV1Alpha3.LoadBalancerSettings_RANDOM
	case "least_request":
		return networkingV1Alpha3.LoadBalancerSettings_LEAST_REQUEST
	case "passthrough":
		return networkingV1Alpha3.LoadBalancerSettings_PASSTHROUGH
	default:
		return networkingV1Alpha3.LoadBalancerSettings_ROUND_ROBIN
	}

}
