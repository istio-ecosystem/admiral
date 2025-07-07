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
	Len() int
}

type VirtualServiceController struct {
	IstioClient                 versioned.Interface
	VirtualServiceHandler       VirtualServiceHandler
	VirtualServiceCache         IVirtualServiceCache
	IdentityVirtualServiceCache IIdentityVirtualServiceCache
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

type IIdentityVirtualServiceCache interface {
	Get(identity string) map[string]*networking.VirtualService
	Put(vs *networking.VirtualService) error
	Delete(vs *networking.VirtualService) error
	Len() int
}

// IdentityVirtualServiceCache holds VS that are in identity's NS
// excluding argo VS
// identity:[vsName:vs]
type IdentityVirtualServiceCache struct {
	cache map[string]map[string]*networking.VirtualService
	mutex *sync.RWMutex
}

func NewIdentityVirtualServiceCache() *IdentityVirtualServiceCache {
	return &IdentityVirtualServiceCache{
		cache: make(map[string]map[string]*networking.VirtualService),
		mutex: &sync.RWMutex{},
	}
}

func (i *IdentityVirtualServiceCache) Get(identity string) map[string]*networking.VirtualService {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	return i.cache[strings.ToLower(identity)]
}

func (i *IdentityVirtualServiceCache) Put(vs *networking.VirtualService) error {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	namespace := vs.Namespace
	if namespace == "" {
		return fmt.Errorf(
			"failed to put virtualService in IdentityVirtualServiceCache as vs namespace is empty")
	}
	if namespace == common.GetSyncNamespace() || namespace == common.NamespaceIstioSystem {
		return nil
	}
	name := vs.Name
	if name == "" {
		return fmt.Errorf(
			"failed to put virtualService in IdentityVirtualServiceCache as vs name is empty")
	}
	hosts := vs.Spec.Hosts
	if hosts == nil {
		return nil
	}
	if len(hosts) == 0 {
		return nil
	}
	// This is to skip Argo VS
	if isArgoVS(hosts, vs.Labels) {
		return nil
	}
	identities := getIdentitiesFromVSHostName(hosts[0])
	for _, identity := range identities {
		identity = strings.ToLower(identity)
		if i.cache[identity] == nil {
			i.cache[identity] = map[string]*networking.VirtualService{name: vs}
			continue
		}
		i.cache[identity][name] = vs
	}
	return nil
}

// isArgoVS checks if the virtual service is an Argo VS by checking if any of the hosts start with "dummy"
func isArgoVS(hosts []string, labels map[string]string) bool {
	for _, host := range hosts {
		if strings.HasPrefix(host, "dummy") {
			return true
		}
	}
	return false
}

func (i *IdentityVirtualServiceCache) Delete(vs *networking.VirtualService) error {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	namespace := vs.Namespace
	if namespace == "" {
		return fmt.Errorf(
			"failed to delete virtualService in IdentityVirtualServiceCache as vs namespace is empty")
	}
	if namespace == common.GetSyncNamespace() || namespace == common.NamespaceIstioSystem {
		return nil
	}
	name := vs.Name
	if name == "" {
		return fmt.Errorf(
			"failed to delete virtualService in IdentityVirtualServiceCache as vs name is empty")
	}
	hosts := vs.Spec.Hosts
	if hosts == nil {
		return nil
	}
	if len(hosts) == 0 {
		return nil
	}
	identities := getIdentitiesFromVSHostName(hosts[0])
	for _, identity := range identities {
		identity = strings.ToLower(identity)
		vsMap, ok := i.cache[identity]
		if !ok {
			continue
		}
		delete(vsMap, name)
	}
	return nil
}

func (i *IdentityVirtualServiceCache) Len() int {
	defer i.mutex.RUnlock()
	i.mutex.RLock()
	return len(i.cache)
}

func getIdentitiesFromVSHostName(hostName string) []string {

	if hostName == "" {
		return nil
	}
	hostNameSplit := strings.Split(hostName, ".")
	hostNameWithoutSuffix := hostNameSplit[:len(hostNameSplit)-1]
	if len(hostNameWithoutSuffix) < 2 {
		return nil
	}
	identities := make([]string, 0)
	identities = append(identities, strings.Join(hostNameWithoutSuffix[1:], "."))
	if len(hostNameWithoutSuffix) < 3 {
		return identities
	}
	identities = append(identities, strings.Join(hostNameWithoutSuffix[2:], "."))
	return identities
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
	vsController.IdentityVirtualServiceCache = NewIdentityVirtualServiceCache()

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
	log.Infof("VirtualServiceCache length: %d", v.VirtualServiceCache.Len())
	if err != nil {
		return err
	}
	err = v.HostToRouteDestinationCache.Put(vs)
	log.Infof("HostToRouteDestinationCache length: %d", v.HostToRouteDestinationCache.Len())
	if err != nil {
		return err
	}
	v.IdentityVirtualServiceCache.Put(vs)
	log.Infof("IdentityVirtualServiceCache length: %d", v.IdentityVirtualServiceCache.Len())
	return v.VirtualServiceHandler.Added(ctx, vs)
}

func (v *VirtualServiceController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	v.VirtualServiceCache.Put(vs)
	log.Infof("VirtualServiceCache length: %d", v.VirtualServiceCache.Len())
	v.HostToRouteDestinationCache.Put(vs)
	log.Infof("HostToRouteDestinationCache length: %d", v.HostToRouteDestinationCache.Len())
	v.IdentityVirtualServiceCache.Put(vs)
	log.Infof("IdentityVirtualServiceCache length: %d", v.IdentityVirtualServiceCache.Len())
	return v.VirtualServiceHandler.Updated(ctx, vs)
}

func (v *VirtualServiceController) Deleted(ctx context.Context, obj interface{}) error {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	v.VirtualServiceCache.Delete(vs)
	v.HostToRouteDestinationCache.Delete(vs)
	v.IdentityVirtualServiceCache.Delete(vs)
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

func (vs *VirtualServiceCache) Len() int {
	defer vs.mutex.RUnlock()
	vs.mutex.RLock()
	return len(vs.cache)
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

func (h *HostToRouteDestinationCache) Len() int {
	defer h.mutex.RUnlock()
	h.mutex.RLock()
	return len(h.cache)
}
