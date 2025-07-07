package clusters

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"k8s.io/apimachinery/pkg/labels"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/core/vsrouting"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	log "github.com/sirupsen/logrus"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type addUpdateSidecarFunc func(
	ctxLogger *log.Entry,
	ctx context.Context,
	newSidecarConfig *v1alpha3.Sidecar,
	cachedSidecar *v1alpha3.Sidecar,
	clientNamespace string,
	rc *RemoteController)

type envCustomVSTuple struct {
	env      string
	customVS *v1alpha3.VirtualService
}

type GetCustomVirtualService func(context.Context, *log.Entry, *RemoteController, string, string) ([]envCustomVSTuple, error)

type VSRouteComparator func(*networkingV1Alpha3.VirtualService, *networkingV1Alpha3.VirtualService) (bool, error)
type HTTPRouteSorted []*networkingV1Alpha3.HTTPRoute

func (r HTTPRouteSorted) Len() int {
	return len(r)
}

func (r HTTPRouteSorted) Less(i, j int) bool {
	return r[i].Name < r[j].Name
}

func (r HTTPRouteSorted) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

// getBaseInClusterVirtualService generates the base in-cluster virtual service
func getBaseInClusterVirtualService() (*v1alpha3.VirtualService, error) {
	return &v1alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: util.IstioSystemNamespace,
			Labels: map[string]string{
				common.VSRoutingLabel: "enabled",
				common.VSRoutingType:  common.VSRoutingTypeInCluster,
			},
		},
		Spec: networkingV1Alpha3.VirtualService{},
	}, nil
}

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
		common.VSRoutingLabel: "enabled",
	}

	return &v1alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: util.IstioSystemNamespace,
			Labels:    vsLabels,
		},
		Spec: vs,
	}, nil
}

// getDefaultFQDNFromDeployment generates default FQDN from the identity and env
// derived from the deployment
//
// Example: stage.greeting.global
func getDefaultFQDNFromDeployment(deployment *k8sAppsV1.Deployment) (string, error) {

	if deployment == nil {
		return "", fmt.Errorf("deployment is nil")
	}
	workloadIdentityKey := common.GetWorkloadIdentifier()
	cname := common.GetCname(deployment, workloadIdentityKey, common.GetHostnameSuffix())
	if cname == "" {
		return "", fmt.Errorf("cname is empty")
	}
	return cname, nil
}

// getDefaultFQDNFromRollout generates the default FQDN from the identity and env
// derived from the rollout
//
// Example: stage.greeting.global
func getDefaultFQDNFromRollout(rollout *argo.Rollout) (string, error) {

	if rollout == nil {
		return "", fmt.Errorf("rollout is nil")
	}
	workloadIdentityKey := common.GetWorkloadIdentifier()
	// Get default cname for rollout
	cname := common.GetCnameForRollout(rollout, workloadIdentityKey, common.GetHostnameSuffix())
	if cname == "" {
		return "", fmt.Errorf("cname is empty")
	}
	return cname, nil
}

// getCanaryFQDNFromRollout generates the canary FQDN from the identity and env
// derived from the rollout
//
// Example: canary.stage.greeting.canary.global
func getCanaryFQDNFromRollout(rollout *argo.Rollout) (string, error) {

	if rollout == nil {
		return "", fmt.Errorf("rollout is nil")
	}

	cName := common.GetCnameForRollout(rollout, common.GetWorkloadIdentifier(), common.GetHostnameSuffix())
	if cName == "" {
		return "", fmt.Errorf("getCanaryFQDNForRollout, unable to get cname for rollout %s", rollout.Name)
	}
	canaryCname := common.CanaryRolloutCanaryPrefix + common.Sep + cName

	return canaryCname, nil
}

// getPreviewFQDNFromRollout generates the preview FQDN from the identity and env
// derived from the rollout
//
// Example: preview.stage.greeting.canary.global
func getPreviewFQDNFromRollout(rollout *argo.Rollout) (string, error) {

	if rollout == nil {
		return "", fmt.Errorf("rollout is nil")
	}

	cName := common.GetCnameForRollout(rollout, common.GetWorkloadIdentifier(), common.GetHostnameSuffix())
	if cName == "" {
		return "", fmt.Errorf("getPreviewFQDNForRollout, unable to get cname for rollout %s", rollout.Name)
	}

	previewCname := common.BlueGreenRolloutPreviewPrefix + common.Sep + cName

	return previewCname, nil
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
func getRouteDestination(host string, port uint32, weight int32) *vsrouting.RouteDestination {
	routeDestination := &vsrouting.RouteDestination{
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
// "stage.greeting.global" ->
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
	destinations map[string][]*vsrouting.RouteDestination) error {

	if serviceInstance == nil {
		return fmt.Errorf("serviceInstance is nil")
	}
	if serviceInstance[common.Deployment] == nil {
		return fmt.Errorf("service is not associated with a deployment")
	}
	if destinations == nil {
		return fmt.Errorf("destinations map is nil")
	}

	globalFQDN, err := getDefaultFQDNFromDeployment(deployment)
	if err != nil {
		return err
	}

	host := serviceInstance[common.Deployment].Name + "." +
		serviceInstance[common.Deployment].Namespace + common.DotLocalDomainSuffix
	if destinations[globalFQDN] == nil {
		destinations[globalFQDN] = make([]*vsrouting.RouteDestination, 0)
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
// "preview.stage.greeting.global" ->
//
// route:
//   - destination:
//     host: rollout-bluegreen-preview.sample-rollout-bluegreen.svc.cluster.local
//     port:
//     number: 80
//
// Example Canary:
//
// "stage.greeting.global" ->
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
// "canary.stage.greeting.global" ->
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
	destinations map[string][]*vsrouting.RouteDestination) error {

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
	defaultFQDN, err := getDefaultFQDNFromRollout(rollout)
	if err != nil {
		return err
	}
	host := serviceInstance[common.Rollout].Name + "." +
		serviceInstance[common.Rollout].Namespace + common.DotLocalDomainSuffix
	if destinations[defaultFQDN] == nil {
		destinations[defaultFQDN] = make([]*vsrouting.RouteDestination, 0)
	}
	destinations[defaultFQDN] = append(destinations[defaultFQDN], getRouteDestination(host, meshPort, 0))

	if len(destinations[defaultFQDN]) > 1 {
		sort.Sort(vsrouting.RouteDestinationSorted(destinations[defaultFQDN]))
	}

	return nil
}

// populateDestinationsForBlueGreenStrategy populates the route destination map with
// preview and active service destinations for the bluegreen rollout
func populateDestinationsForBlueGreenStrategy(
	weightedServices map[string]*WeightedService,
	rollout *argo.Rollout,
	meshPort uint32,
	destinations map[string][]*vsrouting.RouteDestination) error {

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
		previewFQDN, err := getPreviewFQDNFromRollout(rollout)
		if err != nil {
			return err
		}
		previewServiceInstance := weightedPreviewService.Service
		host := previewServiceInstance.Name + common.Sep +
			previewServiceInstance.Namespace + common.DotLocalDomainSuffix
		if destinations[previewFQDN] == nil {
			destinations[previewFQDN] = make([]*vsrouting.RouteDestination, 0)
		}
		destinations[previewFQDN] = append(destinations[previewFQDN], getRouteDestination(host, meshPort, 0))
	}
	activeServiceName := rollout.Spec.Strategy.BlueGreen.ActiveService
	if activeService, ok := weightedServices[activeServiceName]; ok {
		defaultFQDN, err := getDefaultFQDNFromRollout(rollout)
		if err != nil {
			return err
		}
		activeServiceInstance := activeService.Service
		host := activeServiceInstance.Name + common.Sep +
			activeServiceInstance.Namespace + common.DotLocalDomainSuffix
		if destinations[defaultFQDN] == nil {
			destinations[defaultFQDN] = make([]*vsrouting.RouteDestination, 0)
		}
		destinations[defaultFQDN] = append(destinations[defaultFQDN], getRouteDestination(host, meshPort, 0))

		if len(destinations[defaultFQDN]) > 1 {
			sort.Sort(vsrouting.RouteDestinationSorted(destinations[defaultFQDN]))
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
	destinations map[string][]*vsrouting.RouteDestination) error {

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

	defaultFQDN, err := getDefaultFQDNFromRollout(rollout)
	if err != nil {
		return err
	}
	canaryFQDN, err := getCanaryFQDNFromRollout(rollout)
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
			destinations[defaultFQDN] = make([]*vsrouting.RouteDestination, 0)
		}
		if service.Weight > 0 && len(weightedServices) > 1 {
			weight = service.Weight
		}
		destinations[defaultFQDN] = append(destinations[defaultFQDN], getRouteDestination(host, meshPort, weight))
	}

	if len(destinations[defaultFQDN]) > 1 {
		sort.Sort(vsrouting.RouteDestinationSorted(destinations[defaultFQDN]))
	}

	// Here we will create a separate canary destination for the canary FQDN
	// This is needed to provide users to validate their canary endpoints
	serviceNamespace := serviceInstance.Namespace
	host := canaryServiceName + common.Sep + serviceNamespace + common.DotLocalDomainSuffix
	if destinations[canaryFQDN] == nil {
		destinations[canaryFQDN] = make([]*vsrouting.RouteDestination, 0)
	}
	destinations[canaryFQDN] = append(destinations[canaryFQDN], getRouteDestination(host, meshPort, 0))

	return nil
}

func generateAuthorityMatches(globalFQDN, sourceIdentity string) []*networkingV1Alpha3.HTTPMatchRequest {
	idx := strings.Index(strings.ToLower(globalFQDN), strings.ToLower(sourceIdentity))
	if idx == -1 {
		return []*networkingV1Alpha3.HTTPMatchRequest{
			{
				Authority: &networkingV1Alpha3.StringMatch{
					MatchType: &networkingV1Alpha3.StringMatch_Prefix{
						Prefix: globalFQDN,
					},
				},
			},
		}
	}

	upperCase := strings.ToUpper(sourceIdentity[:1]) + sourceIdentity[1:]
	globalFQDNWithUpperCaseIdentity := globalFQDN[:idx] + upperCase + globalFQDN[idx+len(sourceIdentity):]

	lowerCase := strings.ToLower(sourceIdentity[:1]) + sourceIdentity[1:]
	globalFQDNWithLowerCaseIdentity := globalFQDN[:idx] + lowerCase + globalFQDN[idx+len(sourceIdentity):]

	return []*networkingV1Alpha3.HTTPMatchRequest{
		{
			Authority: &networkingV1Alpha3.StringMatch{
				MatchType: &networkingV1Alpha3.StringMatch_Prefix{
					Prefix: globalFQDNWithLowerCaseIdentity,
				},
			},
		},
		{
			Authority: &networkingV1Alpha3.StringMatch{
				MatchType: &networkingV1Alpha3.StringMatch_Prefix{
					Prefix: globalFQDNWithUpperCaseIdentity,
				},
			},
		},
	}

}

// generateVirtualServiceForIncluster generates the VirtualService for the in-cluster routing
func generateVirtualServiceForIncluster(
	ctx context.Context,
	ctxLogger *log.Entry,
	destination map[string][]*vsrouting.RouteDestination,
	vsName string,
	remoteRegistry *RemoteRegistry,
	sourceCluster string,
	sourceIdentity string,
	env string) (*v1alpha3.VirtualService, error) {

	virtualService, err := getBaseInClusterVirtualService()
	if err != nil {
		return nil, err
	}

	vsHosts := make([]string, 0)
	httpRoutes := make([]*networkingV1Alpha3.HTTPRoute, 0)

	for globalFQDN, routeDestinations := range destination {
		if routeDestinations == nil || len(routeDestinations) == 0 {
			continue
		}
		httpRoute := networkingV1Alpha3.HTTPRoute{
			Match: generateAuthorityMatches(globalFQDN, sourceIdentity),
			Name:  globalFQDN,
		}
		httpRouteDestinations := make([]*networkingV1Alpha3.HTTPRouteDestination, 0)
		for _, routeDestination := range routeDestinations {
			httpRouteDestinations = append(httpRouteDestinations, routeDestination.ToHTTPRouteDestination())
		}
		httpRoute.Route = httpRouteDestinations
		httpRoutes = append(httpRoutes, &httpRoute)
		vsHosts = append(vsHosts, globalFQDN)
	}

	if len(vsHosts) == 0 {
		return nil, fmt.Errorf(
			"skipped creating virtualservice as there are no valid hosts found")
	}
	if len(httpRoutes) == 0 {
		return nil, fmt.Errorf(
			"skipped creating virtualservice on cluster as there are no valid http routes found")
	}
	sort.Strings(vsHosts)
	virtualService.Spec.Hosts = vsHosts
	sort.Slice(httpRoutes, func(i, j int) bool {
		return httpRoutes[i].Match[0].Authority.String() < httpRoutes[j].Match[0].Authority.String()
	})
	virtualService.Spec.Http = httpRoutes

	virtualService.Name = fmt.Sprintf("%s-%s", vsName, common.InclusterVSNameSuffix)

	// Add the exportTo namespaces to the virtual service
	virtualService.Spec.ExportTo = []string{common.GetSyncNamespace()}
	vsRoutingInclusterEnabledForClusterAndIdentity := false
	if common.EnableExportTo(vsName) &&
		DoVSRoutingInClusterForClusterAndIdentity(
			ctx, ctxLogger, env, sourceCluster, sourceIdentity, remoteRegistry, false) {
		vsRoutingInclusterEnabledForClusterAndIdentity = true
		virtualService.Spec.ExportTo = getSortedDependentNamespaces(
			remoteRegistry.AdmiralCache, vsName, sourceCluster, ctxLogger, true)
	}
	ctxLogger.Infof(common.CtxLogFormat, "VSBasedRoutingInCluster",
		virtualService.Name, virtualService.Namespace, sourceCluster,
		fmt.Sprintf("Writing phase: generateVirtualServiceForIncluster: VSRoutingInClusterEnabled: %v", vsRoutingInclusterEnabledForClusterAndIdentity))

	return virtualService, nil
}

// generateVirtualServiceForIngress generates the VirtualService for the cross-cluster routing
func generateVirtualServiceForIngress(
	destination map[string][]*vsrouting.RouteDestination,
	vsName string) (*v1alpha3.VirtualService, error) {

	virtualService, err := getBaseVirtualServiceForIngress()
	if err != nil {
		return nil, err
	}

	vsHosts := make([]string, 0)
	tlsRoutes := make([]*networkingV1Alpha3.TLSRoute, 0)

	for globalFQDN, routeDestinations := range destination {
		if routeDestinations == nil || len(routeDestinations) == 0 {
			continue
		}
		hostWithSNIPrefix, err := generateSNIHost(globalFQDN)
		if err != nil {
			continue
		}
		tlsRoute := networkingV1Alpha3.TLSRoute{
			Match: []*networkingV1Alpha3.TLSMatchAttributes{
				{
					Port:     common.DefaultMtlsPort,
					SniHosts: []string{hostWithSNIPrefix},
				},
			},
		}
		tlsRouteDestinations := make([]*networkingV1Alpha3.RouteDestination, 0)
		for _, routeDestination := range routeDestinations {
			tlsRouteDestinations = append(tlsRouteDestinations, routeDestination.ToTLSRouteDestination())
		}
		tlsRoute.Route = tlsRouteDestinations
		tlsRoutes = append(tlsRoutes, &tlsRoute)
		vsHosts = append(vsHosts, hostWithSNIPrefix)
	}

	if len(vsHosts) == 0 {
		return nil, fmt.Errorf(
			"skipped creating virtualservice as there are no valid hosts found")
	}
	if len(tlsRoutes) == 0 {
		return nil, fmt.Errorf(
			"skipped creating virtualservice on cluster as there are no valid tls routes found")
	}
	sort.Strings(vsHosts)
	virtualService.Spec.Hosts = vsHosts
	sort.Slice(tlsRoutes, func(i, j int) bool {
		return tlsRoutes[i].Match[0].SniHosts[0] < tlsRoutes[j].Match[0].SniHosts[0]
	})
	virtualService.Spec.Tls = tlsRoutes

	virtualService.Name = vsName + "-routing-vs"

	return virtualService, nil
}

// doReconcileVirtualService checks if desired virtualservice state has changed from the one that is cached
// returns true if it has, else returns false
func doReconcileVirtualService(
	rc *RemoteController,
	desiredVirtualService *v1alpha3.VirtualService,
	doRoutesMatch VSRouteComparator,
) (bool, error) {
	if rc == nil {
		return true, fmt.Errorf("remoteController is nil")
	}
	if desiredVirtualService == nil {
		return true, fmt.Errorf("virtualService is nil")
	}
	if rc.VirtualServiceController == nil {
		return true, fmt.Errorf("virtualService controller is nil")
	}
	if rc.VirtualServiceController.VirtualServiceCache == nil {
		return true, fmt.Errorf("virtualServiceCache is nil")
	}
	vsName := desiredVirtualService.Name
	cachedVS := rc.VirtualServiceController.VirtualServiceCache.Get(vsName)
	if cachedVS == nil {
		return true, nil
	}
	cachedVSSpec := cachedVS.Spec.DeepCopy()
	desiredVirtualServiceSpec := desiredVirtualService.Spec.DeepCopy()
	// Check if exportTo has a diff
	slices.Sort(cachedVSSpec.ExportTo)
	slices.Sort(desiredVirtualServiceSpec.ExportTo)
	if !reflect.DeepEqual(cachedVSSpec.ExportTo, desiredVirtualServiceSpec.ExportTo) {
		return true, nil
	}

	// Check if hosts have a diff
	slices.Sort(cachedVSSpec.Hosts)
	slices.Sort(desiredVirtualServiceSpec.Hosts)
	if !reflect.DeepEqual(cachedVSSpec.Hosts, desiredVirtualServiceSpec.Hosts) {
		return true, nil
	}

	// Check if routes have a diff
	routeMatched, err := doRoutesMatch(cachedVSSpec, desiredVirtualServiceSpec)
	if err != nil {
		return true, err
	}
	if !routeMatched {
		return true, err
	}

	if desiredVirtualServiceSpec.Gateways != nil {
		// Check is gateways have a diff
		slices.Sort(cachedVSSpec.Gateways)
		slices.Sort(desiredVirtualServiceSpec.Gateways)
		if !reflect.DeepEqual(cachedVSSpec.Gateways, desiredVirtualServiceSpec.Gateways) {
			return true, nil
		}
	}

	return false, nil
}

// addUpdateInClusterVirtualServices adds or updates the in-cluster routing VirtualServices
// This is where the VirtualServices are created using the services that were discovered during the
// discovery phase.
func addUpdateInClusterVirtualServices(
	ctx context.Context,
	ctxLogger *log.Entry,
	remoteRegistry *RemoteRegistry,
	sourceClusterToDestinations map[string]map[string][]*vsrouting.RouteDestination,
	cname string,
	sourceIdentity string,
	env string,
	sourceClusterToEventNsCache map[string]string) error {

	if sourceIdentity == "" {
		return fmt.Errorf("identity is empty")
	}

	if remoteRegistry == nil {
		return fmt.Errorf("remoteRegistry is nil")
	}

	if cname == "" {
		return fmt.Errorf("cname is empty")
	}

	for sourceCluster, destination := range sourceClusterToDestinations {

		ctxLogger.Debugf(common.CtxLogFormat, "VSBasedRoutingInCluster",
			"", "", sourceCluster,
			"Writing phase: addUpdateInClusterVirtualServices VS based routing enabled for cluster")

		rc := remoteRegistry.GetRemoteController(sourceCluster)

		if rc == nil {
			ctxLogger.Warnf(common.CtxLogFormat, "addUpdateInClusterVirtualServices",
				"", "", sourceCluster, "remote controller not initialized on this cluster")
			continue
		}

		virtualService, err := generateVirtualServiceForIncluster(
			ctx, ctxLogger, destination, cname, remoteRegistry, sourceCluster, sourceIdentity, env)
		if err != nil {
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateInClusterVirtualServices",
				"", "", sourceCluster, err.Error())
			return err
		}

		// Update the client's default sidecar with cluster local services
		// This is to make sure that the client proxies have the necessary .local clusters
		// available in its config
		err = updateClientSidecarWithClusterLocalServices(ctx,
			ctxLogger, rc, virtualService, sourceCluster, sourceClusterToEventNsCache, addUpdateSidecar)
		if err != nil {
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateInClusterVirtualServices",
				virtualService.Name, virtualService.Namespace, sourceCluster,
				fmt.Sprintf("updateClientSidecarWithClusterLocalServices failed due to %v", err.Error()))
		}

		virtualServicesToBeProcessed := []*v1alpha3.VirtualService{virtualService}

		// Merge the incluster vs with custom virtualservice, if enabled
		if common.IsCustomVSMergeEnabled() {
			mergedVirtualServices, err :=
				mergeCustomVirtualServices(
					ctx, ctxLogger, rc, virtualService, env, sourceIdentity, sourceCluster, getCustomVirtualService)
			if err != nil {
				return err
			}
			if mergedVirtualServices != nil && len(mergedVirtualServices) > 0 {
				virtualServicesToBeProcessed = mergedVirtualServices
			} else {
				ctxLogger.Infof(common.CtxLogFormat, "addUpdateInClusterVirtualServices",
					virtualService.Name, virtualService.Namespace, sourceCluster,
					fmt.Sprintf("merge skipped as no custom virtual services found for env %s", env))
			}
		}

		for _, vs := range virtualServicesToBeProcessed {
			// Reconciliation check - start
			ctxLogger.Infof(
				common.CtxLogFormat, "ReconcileVirtualService", vs.Name, "", sourceCluster,
				"checking if incluster routing virtualService requires reconciliation")
			reconcileRequired, err :=
				doReconcileVirtualService(rc, vs, httpRoutesComparator)
			if err != nil {
				ctxLogger.Errorf(common.CtxLogFormat, "addUpdateInClusterVirtualServices",
					vs.Name, vs.Namespace, sourceCluster,
					fmt.Sprintf("doReconcileVirtualService failed due to %v", err.Error()))
			}
			if !reconcileRequired {
				// Perform DR pinning to remote region if required
				// Even if there is no change to the VS, DR pinning might be required.
				// This is handle cases where a GTP is added after the incluster VS is created.
				// The GTP did not change the incluster VS, but we still need to pin the DR
				if shouldPerformDRPinning(ctx, ctxLogger, rc, cname, sourceCluster, sourceIdentity, env, remoteRegistry) {
					err = performDRPinning(ctx, ctxLogger, remoteRegistry, rc, vs, env, sourceCluster)
					if err != nil {
						ctxLogger.Errorf(common.CtxLogFormat, "addUpdateInClusterVirtualServices",
							vs.Name, vs.Namespace, sourceCluster,
							"performDRPinning failed due to %v", err.Error())
					}
				}
				ctxLogger.Infof(
					common.CtxLogFormat, "ReconcileVirtualService", vs.Name, "", sourceCluster,
					"reconcile=false")
				continue
			}
			ctxLogger.Infof(
				common.CtxLogFormat, "ReconcileVirtualService", vs.Name, "", sourceCluster,
				"reconcile=true")
			// Reconciliation check - End

			existingVS, err := getExistingVS(ctxLogger, ctx, rc, vs.Name, util.IstioSystemNamespace)
			if err != nil {
				ctxLogger.Warn(common.CtxLogFormat, "addUpdateInClusterVirtualServices",
					vs.Name, vs.Namespace, sourceCluster, err.Error())
			}

			ctxLogger.Infof(common.CtxLogFormat, "addUpdateInClusterVirtualServices",
				vs.Name, vs.Namespace, sourceCluster, "Add/Update ingress virtualservice")

			err = addUpdateVirtualService(
				ctxLogger, ctx, vs, existingVS, util.IstioSystemNamespace, rc, remoteRegistry)
			if err != nil {
				ctxLogger.Errorf(common.CtxLogFormat, "addUpdateInClusterVirtualServices",
					vs.Name, vs.Namespace, sourceCluster, err.Error())
				return err
			}
			ctxLogger.Infof(common.CtxLogFormat, "addUpdateInClusterVirtualServices",
				vs.Name, vs.Namespace, sourceCluster, "virtualservice created/updated successfully")

			// Perform DR pinning to remote region if required
			if shouldPerformDRPinning(ctx, ctxLogger, rc, cname, sourceCluster, sourceIdentity, env, remoteRegistry) {
				err = performDRPinning(ctx, ctxLogger, remoteRegistry, rc, vs, env, sourceCluster)
				if err != nil {
					ctxLogger.Errorf(common.CtxLogFormat, "addUpdateInClusterVirtualServices",
						vs.Name, vs.Namespace, sourceCluster,
						"performDRPinning failed due to %v", err.Error())
				}
			}

		}

	}

	return nil
}

func updateClientSidecarWithClusterLocalServices(
	ctx context.Context,
	ctxLogger *log.Entry,
	rc *RemoteController,
	vs *v1alpha3.VirtualService,
	sourceCluster string,
	sourceClusterToEventNsCache map[string]string,
	addUpdateSidecar addUpdateSidecarFunc) error {

	if !common.IsSidecarCachingEnabled() {
		ctxLogger.Infof(common.CtxLogFormat, "updateClientSidecarWithClusterLocalServices",
			vs.Name, vs.Namespace, sourceCluster,
			"sidecar caching is disabled, skipping")
		return nil
	}

	if rc == nil {
		return fmt.Errorf("remoteController is nil")
	}
	if rc.SidecarController == nil {
		return fmt.Errorf("sidecarController is nil")
	}
	if rc.SidecarController.SidecarCache == nil {
		return fmt.Errorf("sidecarCache is nil")
	}
	if vs == nil {
		return fmt.Errorf("virtualService is nil")
	}
	if sourceClusterToEventNsCache == nil {
		return fmt.Errorf("sourceClusterToEventNsCache is nil")
	}
	identityNamespace := sourceClusterToEventNsCache[sourceCluster]
	if identityNamespace == "" {
		return fmt.Errorf("identityNamespace is empty for sourceCluster %s", sourceCluster)
	}
	newHostToAddToSidecarEgress := fmt.Sprintf("%s/*.svc.cluster.local", identityNamespace)

	exportToNamespaces := vs.Spec.ExportTo
	if exportToNamespaces == nil || len(exportToNamespaces) == 0 {
		return fmt.Errorf("exportToNamespaces is nil or empty for virtualService %s", vs.Name)
	}
	clientNamepaces := make([]string, 0)
	for _, namespace := range exportToNamespaces {
		if namespace == common.GetSyncNamespace() {
			ctxLogger.Infof(common.CtxLogFormat, "updateClientSidecarWithClusterLocalServices",
				vs.Name, vs.Namespace, sourceCluster, "virtualservice contains sync namespace, skipping update")
			return nil
		}
		// We skip the self namespace as the sidecar in the identity's namespace
		// already has ./* in its egress host
		if namespace == identityNamespace {
			continue
		}
		clientNamepaces = append(clientNamepaces, namespace)
	}
	if len(clientNamepaces) == 0 {
		ctxLogger.Infof(common.CtxLogFormat, "updateClientSidecarWithClusterLocalServices",
			vs.Name, vs.Namespace, sourceCluster,
			"no client namespaces found to update sidecar with cluster local services")
		return nil
	}

	// For each client namespace, we will update the sidecar with the cluster local services
	for _, clientNamespace := range clientNamepaces {
		cachedSidecar := rc.SidecarController.SidecarCache.Get(common.GetWorkloadSidecarName(), clientNamespace)
		if cachedSidecar == nil {
			ctxLogger.Infof(common.CtxLogFormat, "updateClientSidecarWithClusterLocalServices",
				vs.Name, vs.Namespace, sourceCluster,
				fmt.Sprintf("skipped updating sidecar in namespace %s as it is missing in the cache", clientNamespace))
			continue
		}
		if cachedSidecar.Spec.Egress == nil || len(cachedSidecar.Spec.Egress) == 0 {
			ctxLogger.Infof(common.CtxLogFormat, "updateClientSidecarWithClusterLocalServices",
				vs.Name, vs.Namespace, sourceCluster,
				fmt.Sprintf("skipped updating sidecar in namespace %s as no egress found", clientNamespace))
			continue
		}
		cachedSidecarEgressHosts := cachedSidecar.Spec.Egress[0].Hosts
		if cachedSidecarEgressHosts == nil || len(cachedSidecarEgressHosts) == 0 {
			ctxLogger.Infof(common.CtxLogFormat, "updateClientSidecarWithClusterLocalServices",
				vs.Name, vs.Namespace, sourceCluster,
				fmt.Sprintf("skipped updating sidecar in namespace %s as no egress hosts found", clientNamespace))
			continue
		}
		lookup := make(map[string]bool)
		for _, egressHost := range cachedSidecarEgressHosts {
			lookup[egressHost] = true
		}
		if lookup[newHostToAddToSidecarEgress] {
			continue
		}
		newSidecar := copySidecar(cachedSidecar)
		newSidecar.Spec.Egress[0].Hosts = append(newSidecar.Spec.Egress[0].Hosts, newHostToAddToSidecarEgress)
		newSidecarConfig := createSidecarSkeleton(newSidecar.Spec, common.GetWorkloadSidecarName(), clientNamespace)

		addUpdateSidecar(ctxLogger, ctx, newSidecarConfig, cachedSidecar, clientNamespace, rc)
	}

	return nil

}

// shouldPerformDRPinning checks if the DR pinning to remote region is required
// It checks if the identity is multi-region and if there is a GTP for the identity
// If the identity is multi-region and has GTP in the NS, it returns true, else false
// TODO: Add unit tests
func shouldPerformDRPinning(
	ctx context.Context,
	ctxLogger *log.Entry,
	rc *RemoteController,
	cname string,
	sourceCluster string,
	sourceIdentity string,
	env string,
	remoteRegistry *RemoteRegistry) bool {

	vsName := fmt.Sprintf("%s-%s", cname, common.InclusterVSNameSuffix)

	if rc == nil {
		ctxLogger.Warnf(common.CtxLogFormat, "shouldPerformDRPinning",
			vsName, common.NamespaceIstioSystem, sourceCluster,
			"skipped pinning DR to remote region as remoteController is nil")
		return false
	}
	if remoteRegistry == nil {
		ctxLogger.Warnf(common.CtxLogFormat, "shouldPerformDRPinning",
			vsName, common.NamespaceIstioSystem, sourceCluster,
			"skipped pinning DR to remote region as remoteRegistry is nil")
		return false
	}
	if remoteRegistry.AdmiralCache == nil {
		ctxLogger.Warnf(common.CtxLogFormat, "shouldPerformDRPinning",
			vsName, common.NamespaceIstioSystem, sourceCluster,
			"skipped pinning DR to remote region as AdmiralCache is nil")
		return false
	}
	if remoteRegistry.AdmiralCache.GlobalTrafficCache == nil {
		ctxLogger.Warnf(common.CtxLogFormat, "shouldPerformDRPinning",
			vsName, common.NamespaceIstioSystem, sourceCluster,
			"skipped pinning DR to remote region as GlobalTrafficCache is nil")
		return false
	}

	// performCartographerVSCheck param is set to true since it is needed to make a decision
	// if DR pinning should be performed
	if !DoVSRoutingInClusterForClusterAndIdentity(
		ctx, ctxLogger, env, sourceCluster, sourceIdentity, remoteRegistry, true) {
		ctxLogger.Infof(common.CtxLogFormat, "shouldPerformDRPinning",
			vsName, common.NamespaceIstioSystem, sourceCluster,
			fmt.Sprintf("DoVSRoutingInClusterForClusterAndIdentity=false for cluster %s and identity %s", sourceCluster, sourceIdentity))
		return false
	}

	// Pin the DR only if there is a GTP for the identity.
	// We can't pin the DR without a GTP as we would loose the previous state of the DR
	// if we had to rollback.
	globalTrafficPolicy, err :=
		remoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(sourceIdentity, env)
	if err != nil {
		ctxLogger.Infof(common.CtxLogFormat, "shouldPerformDRPinning",
			vsName, common.NamespaceIstioSystem, sourceCluster,
			fmt.Sprintf(
				"skipped pinning DR to remote region for identity %s due to err %v", sourceIdentity, err.Error()))
		return false
	}
	if globalTrafficPolicy == nil {
		ctxLogger.Infof(common.CtxLogFormat, "shouldPerformDRPinning",
			vsName, common.NamespaceIstioSystem, sourceCluster,
			fmt.Sprintf(
				"skipped pinning DR to remote region for identity %s as there is no GTP in the namespace", sourceIdentity))
		return false
	}

	// Get SE from cache. We need this to see if this identity is multi-region
	// This is required later to pin DR to the remote region
	if rc.ServiceEntryController == nil || rc.ServiceEntryController.Cache == nil {
		ctxLogger.Warnf(common.CtxLogFormat, "shouldPerformDRPinning",
			vsName, common.NamespaceIstioSystem, sourceCluster,
			"skipped pinning DR to remote region as ServiceEntryController or Cache is nil")
		return false
	}
	SEName := fmt.Sprintf("%s-se", cname)
	cachedSE := rc.ServiceEntryController.Cache.Get(SEName, sourceCluster)
	if cachedSE != nil {
		// Pinning DR to remote region is only needed if the identity is multi-region
		if !isSEMultiRegion(&cachedSE.Spec) {
			ctxLogger.Infof(common.CtxLogFormat, "shouldPerformDRPinning",
				vsName, common.NamespaceIstioSystem, sourceCluster,
				"skipped pinning DR to remote region as the identity is not multi-region")
			return false
		}
	} else {
		ctxLogger.Infof(common.CtxLogFormat, "shouldPerformDRPinning",
			vsName, common.NamespaceIstioSystem, sourceCluster,
			fmt.Sprintf(
				"skipped pinning DR to remote region as no SE found in cache with name %s in cluster %s", SEName, sourceCluster))
		return false
	}

	return true
}

func isSEMultiRegion(se *networkingV1Alpha3.ServiceEntry) bool {
	if se == nil {
		return false
	}
	regions := make(map[string]bool)
	// Can't just do length of endpoints because there could be
	// a rollout and a deployment in the same namespace
	for _, ep := range se.Endpoints {
		regions[ep.Locality] = true
	}
	if len(regions) > 1 {
		return true
	}
	return false
}

// performDRPinning updates the destination rules to pin the .mesh/.global DR to the remote region
func performDRPinning(ctx context.Context,
	ctxLogger *log.Entry,
	remoteRegistry *RemoteRegistry,
	rc *RemoteController,
	vs *v1alpha3.VirtualService,
	env string,
	sourceCluster string) error {

	if remoteRegistry == nil {
		return fmt.Errorf("remoteRegistry is nil")
	}
	if rc == nil {
		return fmt.Errorf("remoteController is nil")
	}
	if vs == nil {
		return fmt.Errorf("virtualService is nil")
	}

	currentLocality, err := getClusterRegion(remoteRegistry, sourceCluster, rc)
	if err != nil {
		return fmt.Errorf("getClusterRegion failed due to %w", err)
	}

	var errs []error
	// Update the .global/.mesh DR to point to the remote region
	for _, host := range vs.Spec.Hosts {
		if !strings.HasSuffix(host, common.GetHostnameSuffix()) {
			continue
		}
		drName := fmt.Sprintf("%s-default-dr", host)
		// Check if it is an additional endpoint
		if !strings.HasPrefix(host, env) {
			drName = fmt.Sprintf("%s-dr", host)
		}
		// Get DR from cache
		cachedDR := rc.DestinationRuleController.Cache.Get(drName, common.GetSyncNamespace())
		if cachedDR == nil {
			errs = append(errs, fmt.Errorf(
				"skipped pinning DR to remote region as no cached DR found with drName %s in cluster %s",
				drName, sourceCluster))
			continue
		}
		newDR := cachedDR.DeepCopy()

		if newDR.Spec.TrafficPolicy == nil ||
			newDR.Spec.TrafficPolicy.LoadBalancer == nil ||
			newDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting == nil {
			errs = append(errs, fmt.Errorf(
				"skipped pinning DR to remote region as TrafficPolicy or LoadBalancer or LocalityLbSetting is nil for DR %s in cluster %s",
				drName, sourceCluster))
			continue
		}

		newDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting, err = getLocalityLBSettings(currentLocality)
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"performDRPinning failed for DR %s in cluster %s: %w", drName, sourceCluster, err))
			continue
		}
		doReconcileDR := reconcileDestinationRule(
			ctxLogger, true, rc, &newDR.Spec, drName, sourceCluster, common.GetSyncNamespace())
		if !doReconcileDR {
			continue
		}
		err = addUpdateDestinationRule(ctxLogger, ctx, newDR, cachedDR, common.GetSyncNamespace(), rc, remoteRegistry)
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"performDRPinning failed for DR %s in cluster %s: %w", drName, sourceCluster, err))
			continue
		}
		ctxLogger.Infof(common.CtxLogFormat, "performDRPinning",
			drName, common.GetSyncNamespace(), sourceCluster, "DR pinning completed successfully")
	}

	return errors.Join(errs...)

}

func getLocalityLBSettings(currentLocality string) (*networkingV1Alpha3.LocalityLoadBalancerSetting, error) {

	if currentLocality == "" {
		return nil, fmt.Errorf("currentLocality is empty")
	}

	remoteRegion := common.WestLocality
	if currentLocality == common.WestLocality {
		remoteRegion = common.EastLocality
	}

	return &networkingV1Alpha3.LocalityLoadBalancerSetting{
		Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
			{
				From: "*",
				To:   map[string]uint32{remoteRegion: 100},
			},
		},
	}, nil
}

// mergeCustomVirtualServices gets the custom virtualservices based on the identity passed
// The getCustomVirtualService func will return a slice of env to custom VS key-value pair.
// For each of the key value pair returned, we check if the env in the map matches the env passed
// to the func.
// If it does, then we merge the corresponding customVS to the virtualservice that was
// passed to the func.
// Else, we fetch the VS from the controller cache and use that to merge with the customVS.
// The func in the end returns a slice of merged virtualservices.
// TODO: Missing unit tests
func mergeCustomVirtualServices(
	ctx context.Context,
	ctxLogger *log.Entry,
	rc *RemoteController,
	virtualService *v1alpha3.VirtualService,
	env string,
	sourceIdentity string,
	sourceCluster string,
	getCustomVirtualService GetCustomVirtualService,
) ([]*v1alpha3.VirtualService, error) {

	if rc == nil {
		return nil, fmt.Errorf("remote controller not initialized")
	}
	if virtualService == nil {
		return nil, fmt.Errorf("nil virtualService")
	}
	if rc.VirtualServiceController == nil {
		return nil, fmt.Errorf("virtualServiceController is nil")
	}
	if rc.VirtualServiceController.VirtualServiceCache == nil {
		return nil, fmt.Errorf("virtualServiceController.VirtualServiceCache is nil")
	}

	mergedVirtualServices := make([]*v1alpha3.VirtualService, 0)

	customVirtualServices, err := getCustomVirtualService(ctx, ctxLogger, rc, env, sourceIdentity)
	if err != nil {
		return nil, fmt.Errorf("getCustomVirtualService failed due to %w", err)
	}
	if customVirtualServices == nil {
		return mergedVirtualServices, nil
	}
	for _, tuple := range customVirtualServices {
		// env matches for which the event was received
		// then use the virtualService passed to this func
		if tuple.env == env {
			mergedVirtualService, err := mergeVS(tuple.customVS, virtualService, rc)
			if err != nil {
				return nil, err
			}
			err = rc.VirtualServiceController.HostToRouteDestinationCache.Put(mergedVirtualService)
			if err != nil {
				return nil, err
			}
			mergedVirtualServices = append(
				mergedVirtualServices, mergedVirtualService)
			continue
		}
		// if env is not for the event's env
		// then get the VS from the cache and merge it
		vsName := strings.ToLower(
			common.GetCnameVal([]string{tuple.env, sourceIdentity, common.GetHostnameSuffix()}))
		vsName = fmt.Sprintf("%s-%s", vsName, common.InclusterVSNameSuffix)
		vsFromCache := rc.VirtualServiceController.VirtualServiceCache.Get(vsName)
		if vsFromCache == nil {
			ctxLogger.Infof(common.CtxLogFormat, "mergeCustomVirtualServices",
				vsName, "", sourceCluster,
				fmt.Sprintf("no custom virtualservice found for env %s", tuple.env))
			continue
		}
		mergedVirtualService, err := mergeVS(tuple.customVS, vsFromCache, rc)
		if err != nil {
			return nil, err
		}
		mergedVirtualServices = append(
			mergedVirtualServices, mergedVirtualService)
	}

	return mergedVirtualServices, nil
}

// getCustomVirtualService returns a slice of key-value pair of env to virtualService
// The custom VS could have a common VS for multiple envs. These envs are separated with an
// underscore. This func returns a sorted slice of env to virtualService map.
// The slice is sorted by keeping the env that is passed as a param to this func at the 0th
// index.
// Example: if the env passed is stage and the createdForEnv in the VS has "stage1_stage2_stage"
// []{{"stage":virtualService}, {"stage1": virtualService}, {"stage2": virtualService}}
func getCustomVirtualService(
	ctx context.Context,
	ctxLogger *log.Entry,
	rc *RemoteController,
	env string,
	identity string) ([]envCustomVSTuple, error) {

	if rc == nil {
		return nil, fmt.Errorf("remoteController is nil")
	}
	if env == "" {
		return nil, fmt.Errorf("env is empty")
	}
	if identity == "" {
		return nil, fmt.Errorf("identity is empty")
	}

	labelSelector := metaV1.LabelSelector{MatchLabels: map[string]string{
		common.CreatedFor: strings.ToLower(identity),
		common.CreatedBy:  common.GetProcessVSCreatedBy(),
	}}

	virtualServiceList, err := getAllVirtualServices(ctxLogger, ctx, rc, common.GetSyncNamespace(),
		metaV1.ListOptions{
			LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
		})

	if err != nil {
		return nil, fmt.Errorf(
			"failed to fetch custom vs with env %s and identity %s due to error %w", env, identity, err)
	}
	if len(virtualServiceList.Items) == 0 {
		return nil, nil
	}
	var matchedVS envCustomVSTuple

	finalVirtualServices := make([]envCustomVSTuple, 0)
	foundMatchingVirtualServices := false

	for _, vs := range virtualServiceList.Items {
		vsForOtherEnvs := make([]envCustomVSTuple, 0)
		annotations := vs.Annotations
		if annotations == nil {
			continue
		}
		createdForEnv, ok := annotations[common.CreatedForEnv]
		if !ok {
			continue
		}
		if createdForEnv == "" {
			continue
		}
		//This is to handle multi env usecase
		splitEnvs := strings.Split(createdForEnv, "_")
		for _, splitEnv := range splitEnvs {
			if splitEnv == env {
				matchedVS = envCustomVSTuple{env: env, customVS: vs}
				foundMatchingVirtualServices = true
				continue
			}
			vsForOtherEnvs = append(vsForOtherEnvs, envCustomVSTuple{env: splitEnv, customVS: vs})
		}
		if foundMatchingVirtualServices {
			// The matched env should be added at the 0th index
			finalVirtualServices = append(finalVirtualServices, matchedVS)
			finalVirtualServices = append(finalVirtualServices, vsForOtherEnvs...)
			break
		}
	}

	return finalVirtualServices, nil
}

// httpRoutesComparator comparator that matches the routes between two virtualservice spec
// This will be used to check if reconciliation is required
func httpRoutesComparator(
	vs1Spec *networkingV1Alpha3.VirtualService,
	vs2Spec *networkingV1Alpha3.VirtualService) (bool, error) {
	if vs1Spec == nil {
		return false, fmt.Errorf("vs1Spec is nil")
	}
	if vs2Spec == nil {
		return false, fmt.Errorf("vs2Spec is nil")
	}
	if vs1Spec.Http == nil {
		return false, fmt.Errorf("vs1.Spec.Http is nil")
	}
	if vs2Spec.Http == nil {
		return false, fmt.Errorf("vs2.Spec.Http is nil")
	}
	sort.Sort(HTTPRouteSorted(vs1Spec.Http))
	sort.Sort(HTTPRouteSorted(vs2Spec.Http))
	for _, route := range vs1Spec.Http {
		sort.Slice(route.Route, func(i, j int) bool {
			return route.Route[i].Destination.Host < route.Route[j].Destination.Host
		})
	}
	for _, route := range vs2Spec.Http {
		sort.Slice(route.Route, func(i, j int) bool {
			return route.Route[i].Destination.Host < route.Route[j].Destination.Host
		})
	}
	if reflect.DeepEqual(vs1Spec.Http, vs2Spec.Http) {
		return true, nil
	}
	return false, nil
}

// tlsRoutesComparator comparator that matches the routes between two virtualservice spec
// This will be used to check if reconciliation is required
func tlsRoutesComparator(
	vs1Spec *networkingV1Alpha3.VirtualService,
	vs2Spec *networkingV1Alpha3.VirtualService) (bool, error) {
	if vs1Spec == nil {
		return false, fmt.Errorf("vs1Spec is nil")
	}
	if vs2Spec == nil {
		return false, fmt.Errorf("vs2Spec is nil")
	}
	if vs1Spec.Tls == nil {
		return false, fmt.Errorf("vs1.Spec.Tls is nil")
	}
	if vs2Spec.Tls == nil {
		return false, fmt.Errorf("vs2.Spec.Tls is nil")
	}
	for _, route := range vs1Spec.Tls {
		sort.Slice(route.Route, func(i, j int) bool {
			return route.Route[i].Destination.Host < route.Route[j].Destination.Host
		})
	}
	for _, route := range vs2Spec.Tls {
		sort.Slice(route.Route, func(i, j int) bool {
			return route.Route[i].Destination.Host < route.Route[j].Destination.Host
		})
	}
	if reflect.DeepEqual(vs1Spec.Tls, vs2Spec.Tls) {
		return true, nil
	}
	return false, nil
}

// addUpdateVirtualServicesForSourceIngress adds or updates the cross-cluster routing VirtualServices
// This is where the VirtualServices are created using the services that were discovered during the
// discovery phase.
func addUpdateVirtualServicesForIngress(
	ctx context.Context,
	ctxLogger *log.Entry,
	remoteRegistry *RemoteRegistry,
	sourceClusterToDestinations map[string]map[string][]*vsrouting.RouteDestination,
	vsName string) error {

	if remoteRegistry == nil {
		return fmt.Errorf("remoteRegistry is nil")
	}

	if vsName == "" {
		return fmt.Errorf("vsName is empty")
	}

	for sourceCluster, destination := range sourceClusterToDestinations {

		if !common.DoVSRoutingForCluster(sourceCluster) {
			ctxLogger.Infof(common.CtxLogFormat, "VSBasedRouting",
				"", "", sourceCluster,
				"Writing phase: addUpdateVirtualServicesForIngress VS based routing disabled for cluster")
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

		virtualService, err := generateVirtualServiceForIngress(destination, vsName)
		if err != nil {
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateVirtualServicesForIngress",
				"", "", sourceCluster, err.Error())
			return err
		}

		ctxLogger.Infof(
			common.CtxLogFormat, "ReconcileVirtualService", virtualService.Name, "", sourceCluster,
			"checking if ingress routing virtualService requires reconciliation")
		reconcileRequired, err :=
			doReconcileVirtualService(rc, virtualService, tlsRoutesComparator)
		if err != nil {
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateVirtualServicesForIngress",
				virtualService.Name, virtualService.Namespace, sourceCluster,
				fmt.Sprintf("doReconcileVirtualService failed due to %v", err.Error()))
		}
		if !reconcileRequired {
			ctxLogger.Infof(
				common.CtxLogFormat, "ReconcileVirtualService", virtualService.Name, "", sourceCluster,
				"reconcile=false")
			continue
		}
		ctxLogger.Infof(
			common.CtxLogFormat, "ReconcileVirtualService", virtualService.Name, "", sourceCluster,
			"reconcile=true")

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
// and rollouts.
// This map will be used to create the route destinations for the VirtualService
func getAllVSRouteDestinationsByCluster(
	serviceInstance map[string]*k8sV1.Service,
	meshDeployAndRolloutPorts map[string]map[string]uint32,
	weightedServices map[string]*WeightedService,
	rollout *argo.Rollout,
	deployment *k8sAppsV1.Deployment,
	resourceTypeBeingDeleted string) (map[string][]*vsrouting.RouteDestination, error) {

	if serviceInstance == nil {
		return nil, fmt.Errorf("serviceInstance is nil")
	}

	ingressDestinations := make(map[string][]*vsrouting.RouteDestination)

	// Populate the route destinations(svc.cluster.local services) for the deployment
	if serviceInstance[common.Deployment] != nil && resourceTypeBeingDeleted != common.Deployment {
		meshPort, err := getMeshHTTPPortForDeployment(meshDeployAndRolloutPorts)
		if err != nil {
			return nil, err
		}
		err = populateVSRouteDestinationForDeployment(
			serviceInstance, meshPort, deployment, ingressDestinations)
		if err != nil {
			return nil, err
		}
	}

	// Populate the route destinations(svc.cluster.local services) for the rollout
	if serviceInstance[common.Rollout] != nil && resourceTypeBeingDeleted != common.Rollout {
		meshPort, err := getMeshHTTPPortForRollout(meshDeployAndRolloutPorts)
		if err != nil {
			return nil, err
		}
		err = populateVSRouteDestinationForRollout(
			serviceInstance, weightedServices, rollout, meshPort, ingressDestinations)
		if err != nil {
			return nil, err
		}
	}

	return ingressDestinations, nil
}

// processGTPAndAddWeightsByCluster updates the route destinations map with global traffic policy (GTP) destinations
// and adjusts the weights of the route destinations based on the GTP configuration.
// This method updates the provided destinations map with GTP based entries.
// Also calls addWeightsToRouteDestinations to ensure that the weights of the route destinations are correctly adjusted.
func processGTPAndAddWeightsByCluster(ctxLogger *log.Entry,
	remoteRegistry *RemoteRegistry,
	sourceIdentity string,
	env string,
	sourceClusterLocality string,
	destinations map[string][]*vsrouting.RouteDestination,
	updateWeights bool, cname, sourceCluster string) error {
	//update ingress gtp destination
	// Get the global traffic policy for the env and identity
	// and add the additional endpoints/hosts to the destination map
	globalTrafficPolicy, err := remoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(sourceIdentity, env)
	if err != nil {
		return err
	}
	// If there is no GTP and active/passive is default and the func is called for
	// in-cluster vs. (updateWeights signifies in-cluster vs and not ingress vs)
	if globalTrafficPolicy == nil && updateWeights && common.EnableActivePassive() {
		// doActivePassiveInClusterVS func returns a dummy GlobalTrafficPolicy
		// that is used to perform active/passive routing for in-cluster VS.
		globalTrafficPolicy, err = doActivePassiveInClusterVS(
			remoteRegistry, cname, sourceCluster, sourceClusterLocality)
		if err != nil {
			ctxLogger.Warnf(common.CtxLogFormat, "doActivePassiveInClusterVS",
				cname, "", sourceCluster, err.Error())
		}
	}
	if globalTrafficPolicy != nil {
		// Add the global traffic policy destinations to the destination map for ingress vs
		gtpDestinations, err := getDestinationsForGTPDNSPrefixes(ctxLogger, globalTrafficPolicy, destinations, env, sourceClusterLocality, updateWeights)
		if err != nil {
			return err
		}
		for fqdn, routeDestinations := range gtpDestinations {
			destinations[fqdn] = routeDestinations
		}
	}

	err = addWeightsToRouteDestinations(destinations)
	if err != nil {
		return err
	}

	return nil
}

// doActivePassiveInClusterVS is a helper function that creates a dummy GlobalTrafficPolicy
// to perform an active/passive routing for in-cluster VS.
// First we fetch the ServiceEntry from cache to find out if the identity is multi-region.
// If it is multi-region, we fetch the DestinationRule from cache to find out the locality that is passive.
// Once we identify that the current cluster is the passive region, then we create a dummy failover GTP to fail
// traffic to the active region mentioned in the DR.
func doActivePassiveInClusterVS(remoteRegistry *RemoteRegistry,
	cname string,
	sourceCluster string,
	sourceClusterLocality string) (*v1alpha1.GlobalTrafficPolicy, error) {

	if remoteRegistry == nil {
		return nil, fmt.Errorf("remoteRegistry is nil")
	}
	rc := remoteRegistry.GetRemoteController(sourceCluster)
	if rc == nil {
		return nil, fmt.Errorf("remotecontroller is nil for cluster %s", sourceCluster)
	}
	if rc.DestinationRuleController == nil {
		return nil, fmt.Errorf("destinationRuleController is nil for cluster %s", sourceCluster)
	}
	if rc.DestinationRuleController.Cache == nil {
		return nil, fmt.Errorf("destinationRuleController.Cache is nil for cluster %s", sourceCluster)
	}
	if rc.ServiceEntryController == nil {
		return nil, fmt.Errorf("serviceEntryController is nil for cluster %s", sourceCluster)
	}
	if rc.ServiceEntryController.Cache == nil {
		return nil, fmt.Errorf("serviceEntryController.Cache is nil for cluster %s", sourceCluster)
	}
	seName := fmt.Sprintf("%s-se", cname)
	seFromCache := rc.ServiceEntryController.Cache.Get(seName, sourceCluster)
	if seFromCache == nil {
		return nil, fmt.Errorf("no se found in cache for seName %s", seName)
	}
	if !isSEMultiRegion(&seFromCache.Spec) {
		return nil, fmt.Errorf("skipped active passive for incluster as the SE is not multi-region %s", seName)
	}
	drName := fmt.Sprintf("%s-default-dr", cname)
	drFromCache := rc.DestinationRuleController.Cache.Get(drName, common.GetSyncNamespace())
	if drFromCache == nil {
		return nil, fmt.Errorf("no dr found in cache for drName %s", drName)
	}
	if drFromCache.Spec.TrafficPolicy == nil ||
		drFromCache.Spec.TrafficPolicy.LoadBalancer == nil ||
		drFromCache.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting == nil ||
		drFromCache.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute == nil {
		return nil, fmt.Errorf(
			"skipped active passive for incluster as the DR has no localityLBSetting %s", drName)
	}
	distribution := drFromCache.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute
	if len(distribution) != 1 {
		return nil, fmt.Errorf("distribution on the DR %s has a traffic split", drName)
	}
	if _, ok := distribution[0].To[sourceClusterLocality]; ok {
		return nil, fmt.Errorf(
			"the DR %s is pointing to the active cluster %s already", drName, sourceClusterLocality)
	}
	activeLocality := ""
	for currentLocalityOnDR := range distribution[0].To {
		activeLocality = currentLocalityOnDR
	}
	if activeLocality == "" {
		return nil, fmt.Errorf("current locality is empty for dr %s", drName)
	}
	globalTrafficPolicy := &v1alpha1.GlobalTrafficPolicy{
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					DnsPrefix: common.Default,
					LbType:    model.TrafficPolicy_FAILOVER,
					Target: []*model.TrafficGroup{
						{
							Region: sourceClusterLocality,
							Weight: int32(0),
						},
						{
							Region: activeLocality,
							Weight: int32(100),
						},
					},
				},
			},
		},
	}
	return globalTrafficPolicy, nil
}

// addWeightsToRouteDestinations ensures that the weights of route destinations in the provided map
// are correctly distributed to sum to 100 or 0.
func addWeightsToRouteDestinations(destinations map[string][]*vsrouting.RouteDestination) error {
	if destinations == nil {
		return fmt.Errorf("route destinations map is nil")
	}
	for _, routeDestinations := range destinations {
		if len(routeDestinations) > 1 {
			// Check if their weights total to 100
			totalWeight := int32(0)
			for _, destination := range routeDestinations {
				totalWeight += destination.Weight
			}
			if totalWeight == 100 {
				continue
			}
			if totalWeight > 0 {
				log.Warnf("total weight is %d, expected 100 or 0", totalWeight)
				continue
			}
			weightSplits := getWeightSplits(len(routeDestinations))
			for i, destination := range routeDestinations {
				destination.Weight = weightSplits[i]
			}
		}
	}
	return nil
}

func getWeightSplits(numberOfSplits int) []int32 {
	if numberOfSplits == 0 {
		return []int32{}
	}
	base := 100 / numberOfSplits
	r := 100 % numberOfSplits
	weights := make([]int32, numberOfSplits)

	for i := 0; i < numberOfSplits; i++ {
		if r > 0 {
			weights[i] = int32(base + 1)
			r--
			continue
		}
		weights[i] = int32(base)
	}
	return weights
}

// getDestinationsForGTPDNSPrefixes processes the provided GlobalTrafficPolicy and updates the route destinations
// map with DNS-prefixed hosts and adjusted weights based on the policy configuration.
func getDestinationsForGTPDNSPrefixes(
	ctxLogger *log.Entry,
	globalTrafficPolicy *v1alpha1.GlobalTrafficPolicy,
	destinations map[string][]*vsrouting.RouteDestination,
	env string,
	sourceClusterLocality string,
	updateWeights bool) (map[string][]*vsrouting.RouteDestination, error) {

	if globalTrafficPolicy == nil {
		return nil, fmt.Errorf("globaltrafficpolicy is nil")
	}
	if destinations == nil {
		return nil, fmt.Errorf("destinations map is nil")
	}

	gtpDestinations := make(map[string][]*vsrouting.RouteDestination)
	for globalFQDN, routeDestinations := range destinations {

		if routeDestinations == nil {
			ctxLogger.Warnf(common.CtxLogFormat, "getDestinationsForGTPDNSPrefixes",
				"", "", globalFQDN, "route destinations is nil")
			continue
		}

		if strings.HasPrefix(globalFQDN, common.BlueGreenRolloutPreviewPrefix) {
			continue
		}

		var routeHost string

		for _, policy := range globalTrafficPolicy.Spec.Policy {
			weights := make(map[string]int32)
			var remoteRD *vsrouting.RouteDestination
			if policy.Target != nil {
				for _, target := range policy.Target {
					weights[target.Region] = target.Weight
				}
			}

			if !updateWeights && len(weights) == 0 && (policy.DnsPrefix == common.Default || policy.DnsPrefix == env) {
				continue
			} else if policy.DnsPrefix == common.Default || policy.DnsPrefix == env {
				routeHost = globalFQDN
			} else {
				routeHost = policy.DnsPrefix + common.Sep + globalFQDN
			}

			newRD, err := copyRouteDestinations(routeDestinations)
			if err != nil {
				return nil, err
			}

			if policy.LbType == model.TrafficPolicy_TOPOLOGY || !updateWeights || len(weights) == 0 {
				gtpDestinations[routeHost] = newRD
				continue
			}

			for _, rd := range newRD {
				if !strings.HasSuffix(rd.Destination.Host, common.DotLocalDomainSuffix) {
					continue
				}
				weightForLocality := weights[sourceClusterLocality]
				if weightForLocality == 100 {
					continue
				}

				if weightForLocality == 0 {
					remoteRD = getRouteDestination(routeHost, 80, 100-weightForLocality)
					rd.Weight = weightForLocality
					continue
				}

				if rd.Weight != 0 {
					rd.Weight = int32((float32(rd.Weight) / 100) * float32(weightForLocality))
				} else {
					rd.Weight = weightForLocality
				}
				remoteRD = getRouteDestination(routeHost, 80, 100-weightForLocality)
			}
			if remoteRD != nil {
				newRD = append(newRD, remoteRD)
			}
			gtpDestinations[routeHost] = newRD
		}
	}

	return gtpDestinations, nil
}

func copyRouteDestinations(
	routeDestination []*vsrouting.RouteDestination) ([]*vsrouting.RouteDestination, error) {
	if routeDestination == nil {
		return nil, fmt.Errorf("routeDestination is nil")
	}
	newRouteDestinations := make([]*vsrouting.RouteDestination, 0)
	for _, rd := range routeDestination {
		var newRD = &vsrouting.RouteDestination{}
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

// addUpdateInClusterDestinationRule adds or updates the DestinationRule for the source cluster client proxies
// This is where the DestinationRules are created for the in-cluster VS based routing
// The DestinationRule is created for the .svc.cluster.local hosts that were discovered during the discovery phase
// on each source cluster
func addUpdateInClusterDestinationRule(
	ctx context.Context,
	ctxLogger *log.Entry,
	remoteRegistry *RemoteRegistry,
	sourceClusterToDRHosts map[string]map[string]string,
	sourceIdentity string,
	cname string,
	env string) error {

	if remoteRegistry == nil {
		return fmt.Errorf("remoteRegistry is nil")
	}
	var clientConnectionSettings *v1alpha1.ClientConnectionConfig
	var err error
	if remoteRegistry.AdmiralCache != nil && remoteRegistry.AdmiralCache.ClientConnectionConfigCache != nil {
		clientConnectionSettings, err =
			remoteRegistry.AdmiralCache.ClientConnectionConfigCache.GetFromIdentity(sourceIdentity, env)
		if err != nil {
			ctxLogger.Warnf(common.CtxLogFormat, "addUpdateInClusterDestinationRule",
				sourceIdentity, "", "",
				fmt.Sprintf("no clientConnectionConfig found for identity %s env %s", sourceIdentity, env))
		}
	}

	if sourceIdentity == "" {
		return fmt.Errorf("sourceIdentity is empty")
	}

	if cname == "" {
		return fmt.Errorf("cname is empty")
	}

	for sourceCluster, drHosts := range sourceClusterToDRHosts {
		san := fmt.Sprintf("%s%s/%s", common.SpiffePrefix, common.GetSANPrefix(), sourceIdentity)

		clientTLSSettings := &networkingV1Alpha3.ClientTLSSettings{
			Mode:            networkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
			SubjectAltNames: []string{san},
		}

		exportToNamespaces := getSortedDependentNamespaces(
			remoteRegistry.AdmiralCache, cname, sourceCluster, ctxLogger, true)

		err := addUpdateRoutingDestinationRule(
			ctx, ctxLogger, remoteRegistry, drHosts, sourceCluster,
			common.InclusterDRSuffix, exportToNamespaces, clientTLSSettings, clientConnectionSettings)

		if err != nil {
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateDestinationRuleForSourceIngress",
				"", "", sourceCluster, err.Error())
			continue
		}
	}
	return nil
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

	for sourceCluster, drHosts := range sourceClusterToDRHosts {

		if !common.DoVSRoutingForCluster(sourceCluster) {
			ctxLogger.Infof(common.CtxLogFormat, "VSBasedRouting",
				"", "", sourceCluster,
				"Writing phase: addUpdateDestinationRuleForSourceIngress VS based routing disabled for cluster")
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

		clientTLSSettings := &networkingV1Alpha3.ClientTLSSettings{
			SubjectAltNames: []string{san},
		}

		err := addUpdateRoutingDestinationRule(
			ctx, ctxLogger, remoteRegistry, drHosts, sourceCluster,
			common.RoutingDRSuffix, common.GetIngressVSExportToNamespace(), clientTLSSettings, nil)

		if err != nil {
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateDestinationRuleForSourceIngress",
				"", "", sourceCluster, err.Error())
			continue
		}
	}
	return nil
}

// addUpdateRoutingDestinationRule creates the DR for VS Based Routing
func addUpdateRoutingDestinationRule(
	ctx context.Context,
	ctxLogger *log.Entry,
	remoteRegistry *RemoteRegistry,
	drHosts map[string]string,
	sourceCluster string,
	drNameSuffix string,
	exportToNamespaces []string,
	clientTLSSettings *networkingV1Alpha3.ClientTLSSettings,
	clientConnectionSettings *v1alpha1.ClientConnectionConfig) error {

	if remoteRegistry == nil {
		return fmt.Errorf("remoteRegistry is nil")
	}

	rc := remoteRegistry.GetRemoteController(sourceCluster)
	if rc == nil {
		return fmt.Errorf("remote controller not initialized on this cluster")
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
				Tls: clientTLSSettings,
			},
		}

		if common.IsSlowStartEnabledForCluster(sourceCluster) {
			drObj.TrafficPolicy.LoadBalancer.WarmupDurationSecs =
				&duration.Duration{Seconds: common.GetDefaultWarmupDurationSecs()}
		}

		drName := fmt.Sprintf("%s-%s", name, drNameSuffix)

		if common.IsTrafficConfigProcessingEnabledForSlowStart() {
			assetKey, hasAssetKey := ctx.Value(common.TrafficConfigIdentity).(string)
			if hasAssetKey {
				workloadEnvKey, hasWorkloadEnvKey := ctx.Value(common.TrafficConfigContextWorkloadEnvKey).(string)
				if hasWorkloadEnvKey && workloadEnvKey != "" {
					err := processSlowStartConfig(remoteRegistry, ctxLogger, assetKey, workloadEnvKey, &drObj, drName, sourceCluster)
					if err != nil {
						return err
					}
				}
			}
		}

		clientConnectionSettingsOverride := getClientConnectionPoolOverrides(clientConnectionSettings)
		if clientConnectionSettingsOverride != nil {
			drObj.TrafficPolicy.ConnectionPool = clientConnectionSettingsOverride
		}
		if common.DisableDefaultAutomaticFailover() {
			// If automatic failover is disabled, we set the outlier detection settings to zero
			// TODO: need add OOD processing similar to SE based routing
			drObj.TrafficPolicy.OutlierDetection = &networkingV1Alpha3.OutlierDetection{
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 0},
				Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
			}
		}

		newDR := createDestinationRuleSkeleton(drObj, drName, util.IstioSystemNamespace)

		newDR.Labels = map[string]string{
			common.VSRoutingLabel: "enabled",
		}

		newDR.Spec.ExportTo = exportToNamespaces

		doReconcileDR := reconcileDestinationRule(
			ctxLogger, true, rc, &newDR.Spec, drName, sourceCluster, util.IstioSystemNamespace)
		if !doReconcileDR {
			continue
		}

		//Get existing DR
		existingDR, err := rc.
			DestinationRuleController.
			IstioClient.
			NetworkingV1alpha3().
			DestinationRules(util.IstioSystemNamespace).Get(ctx, drName, metaV1.GetOptions{})
		if err != nil {
			ctxLogger.Warnf(common.CtxLogFormat,
				"addUpdateRoutingDestinationRule",
				drName,
				util.IstioSystemNamespace,
				sourceCluster, fmt.Sprintf("failed getting existing DR, error=%v", err))
			existingDR = nil
		}

		err = addUpdateDestinationRule(ctxLogger, ctx, newDR, existingDR, util.IstioSystemNamespace, rc, remoteRegistry)
		if err != nil {
			ctxLogger.Errorf(common.CtxLogFormat, "addUpdateRoutingDestinationRule",
				drName, util.IstioSystemNamespace, sourceCluster, err.Error())
			continue
		}

		ctxLogger.Infof(common.CtxLogFormat, "addUpdateRoutingDestinationRule",
			drName, util.IstioSystemNamespace, sourceCluster, "destinationrule created successfully")

		rc.DestinationRuleController.Cache.Put(newDR)

	}

	return nil
}

// processSlowStartConfig handles the processing of slow start traffic configuration
// and applies warmup duration to destination rule if found
func processSlowStartConfig(remoteRegistry *RemoteRegistry, ctxLogger *log.Entry, assetKey string, workloadEnvKey string,
	drObj *networkingV1Alpha3.DestinationRule, drName string, sourceCluster string) error {
	assetConfigMap := remoteRegistry.AdmiralCache.SlowStartConfigCache.Get(assetKey)
	if assetConfigMap == nil {
		return nil
	}

	// Use a function to allow breaking from the Range method
	var warmupDurationValue int64
	var err error
	assetConfigMap.Range(func(envKey string, envConfig *common.Map) {
		// Only process if not already found
		if envConfig.CheckIfPresent(workloadEnvKey) {
			// Found a match, get the value
			durationStr := envConfig.Get(workloadEnvKey)
			if durationStr == "" {
				warmupDurationValue = common.GetDefaultWarmupDurationSecs()
			} else {
				warmupDurationValue, err = strconv.ParseInt(durationStr, 10, 64)
				if err != nil {
					ctxLogger.Warnf(common.CtxLogFormat,
						"addUpdateRoutingDestinationRule",
						drName, util.IstioSystemNamespace, sourceCluster,
						fmt.Sprintf("Failed to parse warmup duration for workload env %s: %v",
							workloadEnvKey, err))
					warmupDurationValue = common.GetDefaultWarmupDurationSecs()
				}
			}
		}
	})

	// Apply the found duration to the destination rule
	drObj.TrafficPolicy.LoadBalancer.WarmupDurationSecs = &duration.Duration{Seconds: warmupDurationValue}
	ctxLogger.Infof(common.CtxLogFormat,
		"addUpdateRoutingDestinationRule",
		drName, util.IstioSystemNamespace, sourceCluster,
		fmt.Sprintf("Applied warmup duration of %d seconds", warmupDurationValue))

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

// performInVSRoutingRollback This function will rollback either all the in-cluster virtualservices
// or rollback a specific vs of a specific identity.
// The rollback is performed by setting the exportTo to the sync namespace.
func performInVSRoutingRollback(
	ctx context.Context,
	ctxLogger *log.Entry,
	remoteRegistry *RemoteRegistry,
	sourceIdentity string,
	sourceClusterToEventNsCache map[string]string,
	vsname string) error {

	if remoteRegistry == nil {
		return fmt.Errorf("remoteRegistry is nil")
	}
	if sourceIdentity == "" {
		return fmt.Errorf("source identity is empty")
	}
	if sourceClusterToEventNsCache == nil {
		return fmt.Errorf("sourceClusterToEventNsCache is nil")
	}
	if vsname == "" {
		return fmt.Errorf("vsname is empty")
	}

	labelSelector := metaV1.LabelSelector{MatchLabels: map[string]string{
		common.VSRoutingType: common.VSRoutingTypeInCluster,
	}}

	errs := make([]error, 0)
	for clusterID := range sourceClusterToEventNsCache {
		rc := remoteRegistry.GetRemoteController(clusterID)
		if rc == nil {
			return fmt.Errorf("remote controller not initialized on cluster %v", clusterID)
		}
		if IsVSRoutingInClusterDisabledForCluster(clusterID) {
			// Disable all in-cluster VS
			virtualServiceList, err := getAllVirtualServices(ctxLogger, ctx, rc, util.IstioSystemNamespace,
				metaV1.ListOptions{
					LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
				})
			if err != nil {
				e := fmt.Errorf(
					"error while getting all virtualservices, rollback failed on cluster %v due to %w",
					clusterID, err)
				ctxLogger.Errorf(common.LogErrFormat, "Delete",
					"performInVSRoutingRollback", "", clusterID, e)
				errs = append(errs, err)
				continue
			}
			for _, virtualService := range virtualServiceList.Items {
				virtualService.Spec.ExportTo = []string{common.GetSyncNamespace()}
				err := updateVirtualService(ctx, virtualService, util.IstioSystemNamespace, rc)
				if err != nil {
					e := fmt.Errorf(
						"failed to rollback in-cluster virtual service on cluster %v due to %w", clusterID, err)
					ctxLogger.Errorf(common.LogErrFormat, "Delete",
						"performInVSRoutingRollback", virtualService.Name, clusterID, e)
					errs = append(errs, e)
				}
			}
			if len(errs) == 0 {
				ctxLogger.Infof(common.CtxLogFormat, "performInVSRoutingRollback",
					"", util.IstioSystemNamespace, clusterID, "successfully rolled back to in-cluster virtualservices")
			}
			continue
		}
		if IsVSRoutingInClusterDisabledForIdentity(clusterID, sourceIdentity) {
			// If we enter this block that means the entire cluster is not disabled
			// just a single identity's VS need to be rolled back.
			virtualServiceName := fmt.Sprintf("%s-%s", vsname, common.InclusterVSNameSuffix)
			existingVS, err := getExistingVS(
				ctxLogger, ctx, rc, virtualServiceName, util.IstioSystemNamespace)
			if err != nil {
				e := fmt.Errorf(
					"error while getting virtualservice %v, rollback failed on cluster %v due to %w",
					virtualServiceName, clusterID, err)
				ctxLogger.Errorf(common.LogErrFormat, "Delete",
					"performInVSRoutingRollback", virtualServiceName, clusterID, e)
				errs = append(errs, e)
				continue
			}
			if existingVS == nil {
				ctxLogger.Infof(common.CtxLogFormat, "performInVSRoutingRollback",
					virtualServiceName, util.IstioSystemNamespace, clusterID, "virtualservice does not exist")
				continue
			}
			existingVS.Spec.ExportTo = []string{common.GetSyncNamespace()}
			err = updateVirtualService(ctx, existingVS, util.IstioSystemNamespace, rc)
			if err != nil {
				e := fmt.Errorf(
					"failed rolling back virtualservice %v due to %w",
					virtualServiceName, err)
				ctxLogger.Errorf(common.LogErrFormat, "Delete",
					"performInVSRoutingRollback", virtualServiceName, clusterID, e)
				errs = append(errs, e)
				continue
			}
			if len(errs) == 0 {
				ctxLogger.Infof(common.CtxLogFormat, "performInVSRoutingRollback",
					virtualServiceName, util.IstioSystemNamespace, clusterID, "successfully rolled back in-cluster virtualservice")
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}
	return nil
}

func mergeVS(
	customVS *v1alpha3.VirtualService,
	inclusterVS *v1alpha3.VirtualService,
	rc *RemoteController) (*v1alpha3.VirtualService, error) {

	if customVS == nil {
		return nil, fmt.Errorf("custom VS is nil")
	}
	if inclusterVS == nil {
		return nil, fmt.Errorf("incluster VS is nil")
	}
	if rc == nil {
		return nil, fmt.Errorf("remote controller is nil")
	}

	newVS := inclusterVS.DeepCopy()

	// Merge the hosts of both VS and then de-dup it
	mergedVSHosts := mergeHosts(customVS.Spec.Hosts, inclusterVS.Spec.Hosts)

	// This is where all the .mesh destination host in the custom VS will be
	// replaced by .local
	modifiedCustomVSRouteDestinations, err := modifyCustomVSHTTPRoutes(customVS.Spec.Http, inclusterVS.Spec.Http, rc)
	if err != nil {
		return nil, err
	}

	// Get Hosts Diff
	// This is needed to get hosts that are not in custom VS
	// The routes for such hosts have to be added towards the top
	// for them to match on authority and avoid any special routing rules being
	// applied by the custom VS
	hostDiff := getHostsDiff(inclusterVS.Spec.Hosts, customVS.Spec.Hosts)

	// Time to sort the routes
	// The non-default fqdn from the in-cluster VS will be added first
	// Then the fdqn that were in the custom VS will be added next
	// then the remaining
	sortedRoutes := sortVSRoutes(
		modifiedCustomVSRouteDestinations,
		inclusterVS.Spec.Http,
		hostDiff)

	newVS.Spec.Http = sortedRoutes
	newVS.Spec.Hosts = mergedVSHosts

	return newVS, nil

}

// getHostsDiff returns a map of hosts that are in in-cluster VS host list but
// not in the custom VS
func getHostsDiff(inclusterHosts []string, customVSHosts []string) map[string]bool {
	result := make(map[string]bool)
	lookup := make(map[string]bool)
	for _, host := range customVSHosts {
		lookup[host] = true
	}

	for _, host := range inclusterHosts {
		if lookup[host] {
			continue
		}
		result[host] = true
	}
	return result
}

func sortVSRoutes(
	customVSRoutes []*networkingV1Alpha3.HTTPRoute,
	inclusterVSRoutes []*networkingV1Alpha3.HTTPRoute,
	hostsNotInCustomVS map[string]bool) []*networkingV1Alpha3.HTTPRoute {

	nonDefaultRoutes := make([]*networkingV1Alpha3.HTTPRoute, 0)
	defaultRoutes := make([]*networkingV1Alpha3.HTTPRoute, 0)
	finalMergedRoutes := make([]*networkingV1Alpha3.HTTPRoute, 0)

	// This is to make sure the hosts that are not in the customVS
	// their routes are ordered first.
	for _, route := range inclusterVSRoutes {
		if hostsNotInCustomVS[route.Name] {
			nonDefaultRoutes = append(nonDefaultRoutes, route)
			continue
		}
		defaultRoutes = append(defaultRoutes, route)
	}

	// This is to make sure to remove the customVS routes
	// that may already exists in the incluster vs coming from cache
	deduplicatedDefaultRoutes := make([]*networkingV1Alpha3.HTTPRoute, 0)
	customVSRouteNames := make(map[string]bool)
	for _, route := range customVSRoutes {
		customVSRouteNames[route.Name] = true
	}
	for _, route := range defaultRoutes {
		if !customVSRouteNames[route.Name] {
			deduplicatedDefaultRoutes = append(deduplicatedDefaultRoutes, route)
		}
	}

	finalMergedRoutes = append(finalMergedRoutes, nonDefaultRoutes...)
	finalMergedRoutes = append(finalMergedRoutes, customVSRoutes...)
	finalMergedRoutes = append(finalMergedRoutes, deduplicatedDefaultRoutes...)

	return finalMergedRoutes
}

// modifyCustomVSHTTPRoutes modifies the HTTP Route destination by switching the .global/.mesh FQDN with
// .svc.cluster.local destinations. The .global/.mesh destination will be looked up
// in the passed in-cluster VS routes first and if not found, lookup will be performed
// on the HostRouteDestination cache for a fqdn that might be on another NS or with a separate admiral.io/env.
// If a FQDN in custom VS is not found in any, then we'll keep it as-is.
// This is needed if a custom VS thats exists in the sync namespace
// for an identity.
func modifyCustomVSHTTPRoutes(
	customVSRoutes []*networkingV1Alpha3.HTTPRoute,
	inclusterVSRoutes []*networkingV1Alpha3.HTTPRoute,
	remoteController *RemoteController) ([]*networkingV1Alpha3.HTTPRoute, error) {
	if customVSRoutes == nil {
		return nil, fmt.Errorf("custom VS HTTPRoutes is nil")
	}
	if inclusterVSRoutes == nil {
		return nil, fmt.Errorf("incluster VS HTTPRoutes is nil")
	}
	if remoteController == nil {
		return nil, fmt.Errorf("remote controller is nil")
	}

	// Create a lookup map FQDN -> []*RouteDestinations
	inClusterRouteLookup := make(map[string][]*networkingV1Alpha3.HTTPRouteDestination)
	for _, route := range inclusterVSRoutes {
		inClusterRouteLookup[route.Name] = route.Route
	}

	newCustomVSHTTPRoutes := make([]*networkingV1Alpha3.HTTPRoute, 0)
	for _, httpRoute := range customVSRoutes {
		copyHTTPRoute := httpRoute.DeepCopy()
		newRouteDestinations := make([]*networkingV1Alpha3.HTTPRouteDestination, 0)
		for _, routeDestination := range httpRoute.Route {
			host := routeDestination.Destination.Host
			// Check for this fqdn in the current incluster VS routes first
			rd, ok := inClusterRouteLookup[host]
			if !ok {
				// If it is not in the current in-cluster VS then check in the cluster's
				// HostToRouteDestinationCache cache
				rd = remoteController.VirtualServiceController.HostToRouteDestinationCache.Get(host)
			}
			if rd == nil {
				// If we are unable to find it in both maps then we add the routedestination as is
				newRouteDestinations = append(newRouteDestinations, routeDestination)
				continue
			}
			// Adjust the weights only if the customVS's routedestination
			// has a weight associated with it.
			if routeDestination.Weight == 0 {
				newRouteDestinations = append(newRouteDestinations, rd...)
				continue
			}
			adjustedRD, err := adjustWeights(rd, routeDestination.Weight)
			if err != nil {
				return nil, err
			}
			newRouteDestinations = append(newRouteDestinations, adjustedRD...)
		}
		copyHTTPRoute.Route = newRouteDestinations
		newCustomVSHTTPRoutes = append(newCustomVSHTTPRoutes, copyHTTPRoute)
	}

	return newCustomVSHTTPRoutes, nil
}

// adjustWeights splits the percentage on each routeDestination based on the weight
// passed
func adjustWeights(
	routeDestinations []*networkingV1Alpha3.HTTPRouteDestination,
	weight int32) ([]*networkingV1Alpha3.HTTPRouteDestination, error) {

	if routeDestinations == nil {
		return nil, fmt.Errorf("slice of HTTPRouteDestination is nil")
	}
	adjustedRDs := make([]*networkingV1Alpha3.HTTPRouteDestination, 0)
	for _, rd := range routeDestinations {
		newRD := rd.DeepCopy()
		if rd.Weight != 0 {
			newRD.Weight = int32((float32(rd.Weight) / 100) * float32(weight))
			adjustedRDs = append(adjustedRDs, newRD)
			continue
		}
		newRD.Weight = weight
		adjustedRDs = append(adjustedRDs, newRD)
	}
	return adjustedRDs, nil
}

// mergeHosts merges the hosts and dedups it
func mergeHosts(hosts1 []string, hosts2 []string) []string {
	lookup := make(map[string]bool)
	mergedHosts := make([]string, 0)
	for _, host := range hosts1 {
		lookup[host] = true
		mergedHosts = append(mergedHosts, host)
	}

	for _, host := range hosts2 {
		if !lookup[host] {
			lookup[host] = true
			mergedHosts = append(mergedHosts, host)
		}
	}

	return mergedHosts
}

// DoVSRoutingInClusterForClusterAndIdentity determines whether in-cluster VS routing is enabled globally
// or for a specific cluster and identity.
// It also checks if there is a custom Virtual Service in the identity's namespace
// and if the Cartographer Virtual Service is disabled for the given cluster and identity.
// performCartographerVSCheck is a flag to indicate whether to perform the Cartographer VS check as this check
// is only needed to verify if .mesh DR pinning should be performed.
func DoVSRoutingInClusterForClusterAndIdentity(
	ctx context.Context,
	ctxLogger *log.Entry,
	env,
	cluster,
	identity string,
	remoteRegistry *RemoteRegistry,
	performCartographerVSCheck bool) bool {

	// Check if the feature is enabled globally
	if !common.GetEnableVSRoutingInCluster() {
		return false
	}

	if IsVSRoutingInClusterDisabledForCluster(cluster) || IsVSRoutingInClusterDisabledForIdentity(cluster, identity) {
		ctxLogger.Infof(common.CtxLogFormat, "DoVSRoutingInClusterForClusterAndIdentity",
			identity, "", cluster, "VS routing in-cluster is disabled for cluster/identity")
		return false
	}

	enabledResources := common.GetVSRoutingInClusterEnabledResources()

	isInClusterVSEnabledForClusterOrIdentity :=
		enabledResources["*"] == "*" ||
			enabledResources[cluster] == "*" ||
			checkClusterIdentity(enabledResources["*"], identity) ||
			checkClusterIdentity(enabledResources[cluster], identity)

	// If the feature is enabled for the cluster or identity, we need to perform
	// additional checks.
	if isInClusterVSEnabledForClusterOrIdentity {
		if remoteRegistry == nil {
			ctxLogger.Warnf(common.CtxLogFormat, "DoVSRoutingInClusterForClusterAndIdentity",
				identity, "", cluster, "remoteRegistry is nil")
			return false
		}
		// Check if there is any custom VS in the identity's namespace
		// We will disable this feature if there is a custom VS in the identity's namespace
		hasVSInNS, err := DoesIdentityHaveVS(remoteRegistry, identity)
		if err != nil {
			ctxLogger.Warnf(common.CtxLogFormat, "DoVSRoutingInClusterForClusterAndIdentity",
				identity, "", cluster, fmt.Sprintf("error checking if identity has VS %v", err))
			return false
		}
		if hasVSInNS {
			ctxLogger.Infof(common.CtxLogFormat, "DoVSRoutingInClusterForClusterAndIdentity",
				identity, "", cluster, "identity has a custom VS in its namespace")
			return false
		}

		// This should be set to true if we need to check if .mesh DR should be pinned to remote
		// region or not. For the DR to be pinned, the cartographer VS should have exportTo set to
		// dot
		if performCartographerVSCheck {
			// Check if the Cartographer Virtual Service is disabled
			// We will disable this feature if the Cartographer VS does not have dot in exportTo
			rc := remoteRegistry.GetRemoteController(cluster)
			if rc == nil {
				ctxLogger.Warnf(common.CtxLogFormat, "DoVSRoutingInClusterForClusterAndIdentity",
					identity, "", cluster, "remote controller is nil")
				return false
			}
			isCartographerVSDisabled, err := IsCartographerVSDisabled(ctx, ctxLogger, rc, env, identity, getCustomVirtualService)
			if err != nil {
				ctxLogger.Warnf(common.CtxLogFormat, "DoVSRoutingInClusterForClusterAndIdentity",
					identity, "", cluster, fmt.Sprintf("failed IsCartographerVSDisabled check due to error %v", err))
				return false
			}
			if !isCartographerVSDisabled {
				ctxLogger.Infof(common.CtxLogFormat, "DoVSRoutingInClusterForClusterAndIdentity",
					identity, "", cluster, fmt.Sprintf("isCartographerVSDisabled=%v", isCartographerVSDisabled))
				return false
			}
		}

		return true
	}

	return false
}

// Verify the specific identity is part of the configured identities
func checkClusterIdentity(identities string, identity string) bool {
	if strings.TrimSpace(identities) == "*" {
		return true
	}

	for _, id := range strings.Split(identities, ",") {
		if strings.TrimSpace(id) == strings.TrimSpace(identity) {
			return true
		}
	}
	return false
}

// DoDRUpdateForInClusterVSRouting determines whether mesh DR pinning should be skipped
// for in-cluster virtual service routing for the given cluster and identity.
// This is needed for in-cluster VS routing as the DR create/update is done.
func DoDRUpdateForInClusterVSRouting(
	ctx context.Context,
	ctxLogger *log.Entry,
	env string,
	cluster string,
	identity string,
	isSourceCluster bool,
	remoteRegistry *RemoteRegistry,
	se *networkingV1Alpha3.ServiceEntry,
	performCartographerVSCheck bool) bool {

	if remoteRegistry == nil {
		ctxLogger.Warnf(common.CtxLogFormat, "DoDRUpdateForInClusterVSRouting",
			"", "", cluster, "remoteRegistry is nil")
		return false
	}
	// Check if the incluster VS has valid exportTo namespaces (not sync namespace)
	hasValidInClusterVS, err := hasInClusterVSWithValidExportToNS(se, remoteRegistry.GetRemoteController(cluster))
	if err != nil {
		ctxLogger.Warnf(common.CtxLogFormat, "DoDRUpdateForInClusterVSRouting",
			identity, "", cluster, fmt.Sprintf("error checking for valid in-cluster VS %v", err))
		return false
	}
	if !hasValidInClusterVS {
		ctxLogger.Infof(common.CtxLogFormat, "DoDRUpdateForInClusterVSRouting",
			identity, "", cluster, "skipping DR update as incluter VS does not have valid exportTo namespaces")
		return false
	}
	if isSourceCluster &&
		DoVSRoutingInClusterForClusterAndIdentity(ctx, ctxLogger, env, cluster, identity, remoteRegistry, performCartographerVSCheck) {
		return true
	}
	return false
}

// IsVSRoutingInClusterDisabledForIdentity checks whether in-cluster vs routing is disabled
// for a specific identity, either globally across all clusters or for a specific cluster.
func IsVSRoutingInClusterDisabledForIdentity(cluster, identity string) bool {

	vsRoutingInClusterDisabledResources := common.GetVSRoutingInClusterDisabledResources()

	if checkClusterIdentity(vsRoutingInClusterDisabledResources["*"], identity) || checkClusterIdentity(vsRoutingInClusterDisabledResources[cluster], identity) {
		return true
	}
	return false
}

// IsVSRoutingInClusterDisabledForCluster checks whether in-cluster vs routing is disabled globally or for specific cluster resources
func IsVSRoutingInClusterDisabledForCluster(cluster string) bool {

	vsRoutingInClusterDisabledResources := common.GetVSRoutingInClusterDisabledResources()

	return vsRoutingInClusterDisabledResources["*"] == "*" || vsRoutingInClusterDisabledResources[cluster] == "*"
}
