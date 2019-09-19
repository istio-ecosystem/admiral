package admiral

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"time"

	k8sV1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sync"
)

const IstioIngressServiceName = "istio-ingressgateway"
const IstioSystemNameSpace = "istio-system"

// Handler interface contains the methods that are required
type ServiceHandler interface {
	Added(obj *k8sV1.Service)
	Deleted(obj *k8sV1.Service)
}

type ServiceClusterEntry struct {
	Identity string
	Service  map[string][]*k8sV1.Service
}

type ServiceController struct {
	K8sClient      kubernetes.Interface
	ServiceHandler ServiceHandler
	Cache          *serviceCache
	informer       cache.SharedIndexInformer
	ctl            *Controller
}

type serviceCache struct {
	//map of dependencies key=identity value array of onboarded identities
	cache map[string]*ServiceClusterEntry
	mutex *sync.Mutex
}

func (s *serviceCache) Put(service *k8sV1.Service) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	identity := s.getKey(service)
	existing := s.cache[identity]
	if existing == nil {
		existing = &ServiceClusterEntry{
			Service:  make(map[string][]*k8sV1.Service),
			Identity: s.getKey(service),
		}
		s.cache[identity] = existing
	}
	namespaceServices := existing.Service[service.Namespace]
	if namespaceServices == nil {
		namespaceServices = make ([]*k8sV1.Service, 0)
	}
	namespaceServices = append(namespaceServices, service)
	existing.Service[service.Namespace] = namespaceServices

}

func (s *serviceCache) getKey(service *k8sV1.Service) string {
	return service.Namespace
}

func (s *serviceCache) Get(key string) *ServiceClusterEntry {
	return s.cache[key]
}

func (s *serviceCache) Delete(pod *ServiceClusterEntry) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	delete(s.cache, pod.Identity)
}

func (s *serviceCache) GetLoadBalancer(key string, namespace string) string {

	service := s.Get(namespace)
	if service == nil || service.Service[namespace] == nil {
		return ""
	}
	for _, service := range service.Service[namespace] {
		if service.Name == key {
			return service.Status.LoadBalancer.Ingress[0].Hostname
		}
	}
	return ""

}

func NewServiceController(stopCh <-chan struct{}, handler ServiceHandler, config *rest.Config, resyncPeriod time.Duration) (*ServiceController, error) {

	serviceController := ServiceController{}
	serviceController.ServiceHandler = handler

	podCache := serviceCache{}
	podCache.cache = make(map[string]*ServiceClusterEntry)
	podCache.mutex = &sync.Mutex{}

	serviceController.Cache = &podCache
	var err error

	serviceController.K8sClient, err = K8sClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create ingress service controller k8s client: %v", err)
	}

	serviceController.informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(opts meta_v1.ListOptions) (runtime.Object, error) {
				return serviceController.K8sClient.CoreV1().Services(meta_v1.NamespaceAll).List(opts)
			},
			WatchFunc: func(opts meta_v1.ListOptions) (watch.Interface, error) {
				return serviceController.K8sClient.CoreV1().Services(meta_v1.NamespaceAll).Watch(opts)
			},
		},
		&k8sV1.Service{}, resyncPeriod, cache.Indexers{},
	)

	NewController(stopCh, &serviceController, serviceController.informer)

	return &serviceController, nil
}

func (s *ServiceController) Added(ojb interface{}) {
	service := ojb.(*k8sV1.Service)
	s.Cache.Put(service)
	s.ServiceHandler.Added(service)
}

func (s *ServiceController) Deleted(name string) {
	//pod := ojb.(*k8sV1.Pod)

	//TODO figure this out
	//d.Cache.Delete(dep)
	s.ServiceHandler.Deleted(nil)

}
