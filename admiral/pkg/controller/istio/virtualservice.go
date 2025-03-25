package istio

import (
	"context"
	"fmt"
	"strings"
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

type IVirtualServiceCache interface {
	Put(vs *networking.VirtualService) error
	Get(vsName string) *networking.VirtualService
	Delete(vs *networking.VirtualService)
	GetVSProcessStatus(vs *networking.VirtualService) string
	UpdateVSProcessStatus(vs *networking.VirtualService, status string) error
}

type VirtualServiceController struct {
	IstioClient                 versioned.Interface
	VirtualServiceHandler       VirtualServiceHandler
	VirtualServiceCache         IVirtualServiceCache
	HostToRouteDestinationCache *HostToRouteDestinationCache
	informer                    cache.SharedIndexInformer
}

// HostToRouteDestinationCache holds only in-cluster VS's FQDN -> []HttpRouteDestinations cache
type HostToRouteDestinationCache struct {
	cache map[string][]*networkingv1alpha3.HTTPRouteDestination
	mutex *sync.RWMutex
}

type VirtualServiceItem struct {
	VirtualService *networking.VirtualService
	Status         string
}

// VirtualServiceCache holds only VSRouting enabled virtualServices
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
	if vs == nil {
		return
	}

	if !common.IsVSRoutingEnabledVirtualService(vs) {
		return
	}
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

func (v *VirtualServiceController) Added(ctx context.Context, obj interface{}) error {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	err := v.VirtualServiceCache.Put(vs)
	if err != nil {
		return err
	}
	err = v.HostToRouteDestinationCache.Put(vs)
	if err != nil {
		return err
	}
	return v.VirtualServiceHandler.Added(ctx, vs)
}

func (v *VirtualServiceController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	v.VirtualServiceCache.Put(vs)
	v.HostToRouteDestinationCache.Put(vs)
	return v.VirtualServiceHandler.Updated(ctx, vs)
}

func (v *VirtualServiceController) Deleted(ctx context.Context, obj interface{}) error {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	v.VirtualServiceCache.Delete(vs)
	v.HostToRouteDestinationCache.Delete(vs)
	return v.VirtualServiceHandler.Deleted(ctx, vs)
}

func (v *VirtualServiceController) GetProcessItemStatus(obj interface{}) (string, error) {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	return v.VirtualServiceCache.GetVSProcessStatus(vs), nil
}

func (v *VirtualServiceController) UpdateProcessItemStatus(obj interface{}, status string) error {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	return v.VirtualServiceCache.UpdateVSProcessStatus(vs, status)
}

func (v *VirtualServiceController) LogValueOfAdmiralIoIgnore(obj interface{}) {
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
	if vs == nil {
		return common.NotProcessed
	}
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
	if vs == nil {
		return fmt.Errorf("vs is nil")
	}
	defer v.mutex.Unlock()
	v.mutex.Lock()

	key := vs.Name

	vsItem, ok := v.cache[key]
	if ok {
		vsItem.Status = status
		v.cache[key] = vsItem
		return nil
	}

	log.Debugf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Update", "VirtualService",
		vs.Name, vs.Namespace, "", "nothing to update, virtualService not found in cache")
	return nil
}

func (h *HostToRouteDestinationCache) Put(vs *networking.VirtualService) error {

	if vs == nil {
		return fmt.Errorf("failed HostToRouteDestinationCache.Put as virtualService is nil")
	}
	if !common.IsVSRoutingInClusterVirtualService(vs) {
		return nil
	}
	if vs.Spec.Http == nil {
		return nil
	}
	defer h.mutex.Unlock()
	h.mutex.Lock()
	for _, httpRoute := range vs.Spec.Http {
		if strings.HasSuffix(httpRoute.Name, common.GetHostnameSuffix()) {
			h.cache[httpRoute.Name] = httpRoute.Route
		}
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
	if vs == nil {
		return fmt.Errorf("failed HostToRouteDestinationCache.Delete as virtualService is nil")
	}
	if !common.IsVSRoutingInClusterVirtualService(vs) {
		return nil
	}
	defer h.mutex.Unlock()
	h.mutex.Lock()
	if vs.Spec.Http == nil {
		return fmt.Errorf("failed HostToRouteDestinationCache.Delete as virtualService.Spec.Http is nil")
	}
	for _, httpRoute := range vs.Spec.Http {
		_, ok := h.cache[httpRoute.Name]
		if ok {
			delete(h.cache, httpRoute.Name)
		}
	}
	return nil
}
