package clusters

import (
	"context"
	"fmt"
	"sync"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
)

type dependencyProxyVirtualServiceCache struct {
	identityVSCache map[string]map[string]*v1alpha3.VirtualService
	mutex           *sync.Mutex
}

func (d *dependencyProxyVirtualServiceCache) put(outerKey string, innerKey string, value *v1alpha3.VirtualService) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if _, ok := d.identityVSCache[outerKey]; !ok {
		d.identityVSCache[outerKey] = make(map[string]*v1alpha3.VirtualService)
	}
	d.identityVSCache[outerKey][innerKey] = value
}

func (d *dependencyProxyVirtualServiceCache) get(key string) map[string]*v1alpha3.VirtualService {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.identityVSCache[key]
}

// updateIdentityDependencyProxyCache adds/updates the map-of-map cache with destination identity as the outer key
// and the admiral.io/env+destinationIdentity as the inner key with the value of generate virtualservice.
// Example: <greeting>:{<stage-greeting>:*v1alpha1.VirtualService}
func updateIdentityDependencyProxyCache(ctx context.Context, cache *dependencyProxyVirtualServiceCache,
	dependencyProxyObj *v1.DependencyProxy, dependencyProxyConverter DependencyProxyConverter) error {

	if cache == nil {
		return fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyVirtualServiceCache is nil")
	}
	if cache.identityVSCache == nil {
		return fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyVirtualServiceCache.identityVSCache is nil")
	}

	if dependencyProxyObj == nil {
		return fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyObj is nil")
	}
	if dependencyProxyObj.Annotations == nil {
		return fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyObj.Annotations is nil")
	}

	env := dependencyProxyObj.Annotations[common.GetEnvKey()]
	if env == "" {
		return fmt.Errorf("%s is empty", common.GetEnvKey())
	}

	if dependencyProxyObj.Spec.Destination == nil {
		return fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyObj.Spec.Destination is nil")
	}
	if dependencyProxyObj.Spec.Destination.Identity == "" {
		return fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyObj.Spec.Destination.Identity is empty")
	}

	vs, err := generateVSFromDependencyProxy(ctx, dependencyProxyObj, dependencyProxyConverter)
	if err != nil {
		return err
	}

	cache.put(dependencyProxyObj.Spec.Destination.Identity, fmt.Sprintf("%s-%s", env, dependencyProxyObj.Spec.Destination.Identity), vs)
	return nil
}

// generateVSFromDependencyProxy will generate VirtualServices from the configurations provided in the
// *v1.DependencyProxy object
func generateVSFromDependencyProxy(ctx context.Context, dependencyProxyObj *v1.DependencyProxy,
	dependencyProxyConverter DependencyProxyConverter) (*v1alpha3.VirtualService, error) {

	if dependencyProxyConverter == nil {
		return nil, fmt.Errorf("failed to generate proxy VirtualService due to error: dependencyProxyConverter is nil")
	}

	proxyCNAME, err := dependencyProxyConverter.GenerateProxyDestinationHostName(dependencyProxyObj)
	if err != nil {
		return nil, fmt.Errorf("failed to generate proxy VirtualService due to error: %w", err)
	}
	virtualServiceHostnames, err := dependencyProxyConverter.GenerateVirtualServiceHostNames(dependencyProxyObj)
	if err != nil {
		return nil, fmt.Errorf("failed to generate proxy VirtualService due to error: %w", err)
	}

	defaultVSName := getIstioResourceName(virtualServiceHostnames[0], "-vs")

	vsRoutes := []*networkingv1alpha3.HTTPRouteDestination{
		{
			Destination: &networkingv1alpha3.Destination{
				Host: proxyCNAME,
				Port: &networkingv1alpha3.PortSelector{
					Number: common.DefaultServiceEntryPort,
				},
			},
		},
	}
	vs := networkingv1alpha3.VirtualService{
		Hosts: virtualServiceHostnames,
		Http: []*networkingv1alpha3.HTTPRoute{
			{
				Route: vsRoutes,
			},
		},
	}

	syncNamespace := common.GetSyncNamespace()
	if syncNamespace == "" {
		return nil, fmt.Errorf("failed to generate proxy VirtualService due to error: syncnamespace is empty")
	}

	// nolint
	return createVirtualServiceSkeleton(vs, defaultVSName, syncNamespace), nil
}
