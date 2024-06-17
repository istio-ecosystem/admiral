package admiral

import (
	"context"
	"fmt"
	"sort"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	"sync"

	k8sV1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// ServiceHandler interface contains the methods that are required
type ServiceHandler interface {
	Added(ctx context.Context, obj *k8sV1.Service)
	Updated(ctx context.Context, obj *k8sV1.Service)
	Deleted(ctx context.Context, obj *k8sV1.Service)
}

type ServiceClusterEntry struct {
	Identity string
	Service  map[string]map[string]*k8sV1.Service //maps namespace to a map of service name:service object
}

type ServiceController struct {
	K8sClient      kubernetes.Interface
	ServiceHandler ServiceHandler
	Cache          *serviceCache
	informer       cache.SharedIndexInformer
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
	if s.shouldIgnoreBasedOnLabels(service) {
		if existing != nil {
			delete(existing.Service[identity], service.Name)
		}
		return //Ignoring services with the ignore label
	}
	if existing == nil {
		existing = &ServiceClusterEntry{
			Service:  make(map[string]map[string]*k8sV1.Service),
			Identity: s.getKey(service),
		}
	}
	namespaceServices := existing.Service[service.Namespace]
	if namespaceServices == nil {
		namespaceServices = make(map[string]*k8sV1.Service)
	}
	namespaceServices[service.Name] = service
	existing.Service[service.Namespace] = namespaceServices
	s.cache[identity] = existing

}

func (s *serviceCache) getKey(service *k8sV1.Service) string {
	return service.Namespace
}

func (s *serviceCache) Get(key string) []*k8sV1.Service {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	serviceClusterEntry := s.cache[key]
	if serviceClusterEntry != nil {
		return getOrderedServices(serviceClusterEntry.Service[key])
	} else {
		return nil
	}
}

func getOrderedServices(serviceMap map[string]*k8sV1.Service) []*k8sV1.Service {
	orderedServices := make([]*k8sV1.Service, 0, len(serviceMap))
	for _, value := range serviceMap {
		orderedServices = append(orderedServices, value)
	}
	if len(orderedServices) > 1 {
		sort.Slice(orderedServices, func(i, j int) bool {
			iTime := orderedServices[i].CreationTimestamp
			jTime := orderedServices[j].CreationTimestamp
			log.Debugf("Service sorting name1=%s creationTime1=%v name2=%s creationTime2=%v", orderedServices[i].Name, iTime, orderedServices[j].Name, jTime)
			return iTime.After(jTime.Time)
		})
	}
	return orderedServices
}

func (s *serviceCache) Delete(service *k8sV1.Service) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	identity := s.getKey(service)
	existing := s.cache[identity]
	if existing != nil {
		delete(existing.Service[identity], service.Name)
		if len(existing.Service[identity]) == 0 {
			delete(s.cache, identity)
		}
	}
}

func (s *serviceCache) GetLoadBalancer(key string, namespace string) (string, int) {
	var (
		lb     = "dummy.admiral.global"
		lbPort = common.DefaultMtlsPort
	)
	services := s.Get(namespace)
	if len(services) == 0 {
		return lb, 0
	}
	for _, service := range services {
		if service.Labels["app"] == key {
			loadBalancerStatus := service.Status.LoadBalancer.Ingress
			if len(loadBalancerStatus) > 0 {
				if len(loadBalancerStatus[0].Hostname) > 0 {
					return loadBalancerStatus[0].Hostname, common.DefaultMtlsPort
				} else {
					return loadBalancerStatus[0].IP, common.DefaultMtlsPort
				}
			} else if len(service.Spec.ExternalIPs) > 0 {
				lb = service.Spec.ExternalIPs[0]
				for _, port := range service.Spec.Ports {
					if port.Port == common.DefaultMtlsPort {
						lbPort = int(port.NodePort)
						return lb, lbPort
					}
				}
			}
		}
	}
	return lb, lbPort
}

func NewServiceController(clusterID string, stopCh <-chan struct{}, handler ServiceHandler, config *rest.Config, resyncPeriod time.Duration) (*ServiceController, error) {

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

	ctx := context.Background()

	serviceController.informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(opts meta_v1.ListOptions) (runtime.Object, error) {
				return serviceController.K8sClient.CoreV1().Services(meta_v1.NamespaceAll).List(ctx, opts)
			},
			WatchFunc: func(opts meta_v1.ListOptions) (watch.Interface, error) {
				return serviceController.K8sClient.CoreV1().Services(meta_v1.NamespaceAll).Watch(ctx, opts)
			},
		},
		&k8sV1.Service{}, resyncPeriod, cache.Indexers{},
	)

	mcd := NewMonitoredDelegator(&serviceController, clusterID, "service")
	NewController("service-ctrl-"+config.Host, stopCh, mcd, serviceController.informer)

	return &serviceController, nil
}

func (s *ServiceController) Added(ctx context.Context, obj interface{}) {
	service := obj.(*k8sV1.Service)
	s.Cache.Put(service)
	s.ServiceHandler.Added(ctx, service)
}

func (s *ServiceController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) {

	service := obj.(*k8sV1.Service)
	s.Cache.Put(service)
	s.ServiceHandler.Updated(ctx, service)
}

func (s *ServiceController) Deleted(ctx context.Context, obj interface{}) {
	service := obj.(*k8sV1.Service)
	s.Cache.Delete(service)
	s.ServiceHandler.Deleted(ctx, service)
}

func (s *serviceCache) shouldIgnoreBasedOnLabels(service *k8sV1.Service) bool {
	return service.Annotations[common.AdmiralIgnoreAnnotation] == "true" || service.Labels[common.AdmiralIgnoreAnnotation] == "true"
}
