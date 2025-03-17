package istio

import (
	"context"
	"fmt"
	"sync"
	"time"

	networkingv1alpha3 "istio.io/api/networking/v1alpha3"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/client-go/pkg/clientset/versioned"
	informers "istio.io/client-go/pkg/informers/externalversions/networking/v1alpha3"
	k8sV1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// VirtualServiceHandler interface contains the methods that are required
type VirtualServiceHandler interface {
	Added(ctx context.Context, obj *networking.VirtualService) error
	Updated(ctx context.Context, obj *networking.VirtualService) error
	Deleted(ctx context.Context, obj *networking.VirtualService) error
}

type VirtualServiceController struct {
	IstioClient                 versioned.Interface
	VirtualServiceHandler       VirtualServiceHandler
	VirtualServiceCache         *VirtualServiceCache
	HostToRouteDestinationCache *HostToRouteDestinationCache
	informer                    cache.SharedIndexInformer
}

type HostToRouteDestinationCache struct {
	cache map[string][]*networkingv1alpha3.HTTPRouteDestination
	mutex *sync.RWMutex
}

type VirtualServiceItem struct {
	VirtualService *networking.VirtualService
	Status         string
}

type VirtualServiceCache struct {
	cache map[string]*VirtualServiceItem
	mutex *sync.RWMutex
}

func NewVirtualServiceCache() *VirtualServiceCache {
	return &VirtualServiceCache{
		cache: make(map[string]*VirtualServiceItem),
		mutex: &sync.RWMutex{},
	}
}

func NewHostToRouteDestinationCache() *HostToRouteDestinationCache {
	return &HostToRouteDestinationCache{
		cache: make(map[string][]*networkingv1alpha3.HTTPRouteDestination),
		mutex: &sync.RWMutex{},
	}
}

func (v *VirtualServiceCache) Put(vs *networking.VirtualService) error {

	if vs == nil {
		return fmt.Errorf("vs is nil")
	}

	if !common.IsVSRoutingEnabledVirtualService(vs) {
		return nil
	}

	defer v.mutex.Unlock()
	v.mutex.Lock()

	key := vs.Name

	v.cache[key] = &VirtualServiceItem{
		VirtualService: vs,
		Status:         common.ProcessingInProgress,
	}
	return nil
}

func (v *VirtualServiceCache) Get(vsName string) *networking.VirtualService {
	defer v.mutex.RUnlock()
	v.mutex.RLock()

	vsItem, ok := v.cache[vsName]
	if ok {
		return vsItem.VirtualService
	}

	return nil
}

func (v *VirtualServiceCache) Delete(vs *networking.VirtualService) {
	defer v.mutex.Unlock()
	v.mutex.Lock()
	key := vs.Name
	if v.cache[key] == nil {
		return
	}
	_, ok := v.cache[key]
	if ok {
		delete(v.cache, key)
	}
}

func (v *VirtualServiceController) DoesGenerationMatch(*log.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

func (v *VirtualServiceController) IsOnlyReplicaCountChanged(*log.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

func NewVirtualServiceController(stopCh <-chan struct{}, handler VirtualServiceHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*VirtualServiceController, error) {

	vsController := VirtualServiceController{}
	vsController.VirtualServiceHandler = handler

	vsController.VirtualServiceCache = NewVirtualServiceCache()
	vsController.HostToRouteDestinationCache = NewHostToRouteDestinationCache()

	var err error

	ic, err := clientLoader.LoadIstioClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create virtual service controller k8s client: %v", err)
	}

	vsController.IstioClient = ic
	vsController.informer = informers.NewVirtualServiceInformer(ic, k8sV1.NamespaceAll, resyncPeriod, cache.Indexers{})

	admiral.NewController("virtualservice-ctrl", config.Host, stopCh, &vsController, vsController.informer)

	return &vsController, nil
}

func (sec *VirtualServiceController) Added(ctx context.Context, obj interface{}) error {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	err := sec.VirtualServiceCache.Put(vs)
	if err != nil {
		return err
	}
	err = sec.HostToRouteDestinationCache.Put(vs)
	if err != nil {
		return err
	}
	return sec.VirtualServiceHandler.Added(ctx, vs)
}

func (sec *VirtualServiceController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	sec.VirtualServiceCache.Put(vs)
	sec.HostToRouteDestinationCache.Put(vs)
	return sec.VirtualServiceHandler.Updated(ctx, vs)
}

func (sec *VirtualServiceController) Deleted(ctx context.Context, obj interface{}) error {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	sec.VirtualServiceCache.Delete(vs)
	sec.HostToRouteDestinationCache.Delete(vs)
	return sec.VirtualServiceHandler.Deleted(ctx, vs)
}

func (sec *VirtualServiceController) GetProcessItemStatus(obj interface{}) (string, error) {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	return sec.VirtualServiceCache.GetVSProcessStatus(vs), nil
}

func (sec *VirtualServiceController) UpdateProcessItemStatus(obj interface{}, status string) error {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	return sec.VirtualServiceCache.UpdateVSProcessStatus(vs, status)
}

func (sec *VirtualServiceController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return
	}
	if len(vs.Annotations) > 0 && vs.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", "VirtualService", vs.Name, vs.Namespace, "", "Value=true")
	}
}

func (sec *VirtualServiceController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	/*vs, ok := obj.(*networking.VirtualService)
	if ok && sec.IstioClient != nil {
		return sec.IstioClient.NetworkingV1alpha3().VirtualServices(vs.Namespace).Get(ctx, vs.Name, meta_v1.GetOptions{})
	}*/
	return nil, fmt.Errorf("istio client is not initialized, txId=%s", ctx.Value("txId"))
}

func (v *VirtualServiceCache) GetVSProcessStatus(vs *networking.VirtualService) string {
	defer v.mutex.RUnlock()
	v.mutex.RLock()

	key := vs.Name

	vsItem, ok := v.cache[key]
	if ok {
		return vsItem.Status
	}
	return common.NotProcessed
}

func (v *VirtualServiceCache) UpdateVSProcessStatus(vs *networking.VirtualService, status string) error {
	defer v.mutex.Unlock()
	v.mutex.Lock()

	key := vs.Name

	vsItem, ok := v.cache[key]
	if ok {
		vsItem.Status = status
		v.cache[key] = vsItem
		return nil
	}

	return fmt.Errorf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Update", "VirtualService",
		vs.Name, vs.Namespace, "", "nothing to update, virtualService not found in cache")
}

func (h *HostToRouteDestinationCache) Put(vs *networking.VirtualService) error {

	if vs == nil {
		return fmt.Errorf("failed HostToRouteDestinationCache.Put as virtualService is nil")
	}
	if !common.IsVSRoutingEnabledVirtualService(vs) {
		return nil
	}
	if vs.Spec.Http == nil {
		return nil
	}
	defer h.mutex.Unlock()
	h.mutex.Lock()
	for _, httpRoute := range vs.Spec.Http {
		h.cache[httpRoute.Name] = httpRoute.Route
	}
	return nil
}

func (h *HostToRouteDestinationCache) Get(routeName string) []*networkingv1alpha3.HTTPRouteDestination {
	defer h.mutex.RUnlock()
	h.mutex.RLock()
	if routes, ok := h.cache[routeName]; ok {
		return routes
	}
	return nil
}

func (h *HostToRouteDestinationCache) Delete(vs *networking.VirtualService) error {
	defer h.mutex.Unlock()
	h.mutex.Lock()
	if vs == nil {
		return fmt.Errorf("failed HostToRouteDestinationCache.Put as virtualService is nil")
	}
	if vs.Spec.Http == nil {
		return fmt.Errorf("failed HostToRouteDestinationCache.Put as virtualService.Spec.Http is nil")
	}

	for _, httpRoute := range vs.Spec.Http {
		_, ok := h.cache[httpRoute.Name]
		if ok {
			delete(h.cache, httpRoute.Name)
		}
	}
	return nil
}
