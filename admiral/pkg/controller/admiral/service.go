package admiral

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/prometheus/common/log"
	"github.com/sirupsen/logrus"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
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
	Added(ctx context.Context, obj *k8sV1.Service) error
	Updated(ctx context.Context, obj *k8sV1.Service) error
	Deleted(ctx context.Context, obj *k8sV1.Service) error
}

type ServiceItem struct {
	Service *k8sV1.Service
	Status  string
}

type ServiceClusterEntry struct {
	Identity string
	Service  map[string]map[string]*ServiceItem //maps namespace to a map of service name:service object
}

type ServiceController struct {
	K8sClient      kubernetes.Interface
	ServiceHandler ServiceHandler
	Cache          *serviceCache
	informer       cache.SharedIndexInformer
}

func (s *ServiceController) DoesGenerationMatch(*logrus.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

func (s *ServiceController) IsOnlyReplicaCountChanged(*logrus.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
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
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.ServiceResourceType,
			service.Name, service.Namespace, "", "Value=true")
		if existing != nil {
			delete(existing.Service[identity], service.Name)
		}
		return //Ignoring services with the ignore label
	}
	if existing == nil {
		existing = &ServiceClusterEntry{
			Service:  make(map[string]map[string]*ServiceItem),
			Identity: s.getKey(service),
		}
	}
	namespaceServices := existing.Service[service.Namespace]
	if namespaceServices == nil {
		namespaceServices = make(map[string]*ServiceItem)
	}
	namespaceServices[service.Name] = &ServiceItem{Service: service, Status: common.ProcessingInProgress}
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

func (p *serviceCache) GetSvcProcessStatus(service *k8sV1.Service) string {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	identity := p.getKey(service)

	svcNamespaceMap, ok := p.cache[identity]
	if ok {
		svcNameMap, ok := svcNamespaceMap.Service[service.Namespace]
		if ok {
			svc, ok := svcNameMap[service.Name]
			if ok {
				return svc.Status
			}
		}
	}

	return common.NotProcessed
}

func (p *serviceCache) UpdateSvcProcessStatus(service *k8sV1.Service, status string) error {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	identity := p.getKey(service)

	svcNamespaceMap, ok := p.cache[identity]
	if ok {
		svcNameMap, ok := svcNamespaceMap.Service[service.Namespace]
		if ok {
			svc, ok := svcNameMap[service.Name]
			if ok {
				svc.Status = status
				p.cache[identity] = svcNamespaceMap
				return nil
			}
		}
	}

	return fmt.Errorf(LogCacheFormat, "Update", "Service",
		service.Name, service.Namespace, "", "nothing to update, service not found in cache")
}

func getOrderedServices(serviceMap map[string]*ServiceItem) []*k8sV1.Service {
	orderedServices := make([]*k8sV1.Service, 0, len(serviceMap))
	for _, value := range serviceMap {
		orderedServices = append(orderedServices, value.Service)
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

func (s *serviceCache) GetSingleLoadBalancer(key string, namespace string) (string, int) {
	var (
		lb     = common.DummyAdmiralGlobal
		lbPort = common.DefaultMtlsPort
	)
	services := s.Get(namespace)

	if len(services) == 0 {
		return lb, 0
	}
	for _, service := range services {
		if common.IsIstioIngressGatewayService(service, key) {
			loadBalancerStatus := service.Status.LoadBalancer.Ingress
			if len(loadBalancerStatus) > 0 {
				if len(loadBalancerStatus[0].Hostname) > 0 {
					//Add "." at the end of the address to prevent additional DNS calls via search domains
					if common.IsAbsoluteFQDNEnabled() {
						return loadBalancerStatus[0].Hostname + common.Sep, common.DefaultMtlsPort
					}
					return loadBalancerStatus[0].Hostname, common.DefaultMtlsPort
				} else {
					return loadBalancerStatus[0].IP, common.DefaultMtlsPort
				}
			} else if len(service.Spec.ExternalIPs) > 0 {
				externalIp := service.Spec.ExternalIPs[0]
				for _, port := range service.Spec.Ports {
					if port.Port == common.DefaultMtlsPort {
						lbPort = int(port.NodePort)
						return externalIp, lbPort
					}
				}
			}
		}
	}

	return lb, lbPort
}

func NewServiceController(stopCh <-chan struct{}, handler ServiceHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*ServiceController, error) {

	serviceController := ServiceController{}
	serviceController.ServiceHandler = handler

	podCache := serviceCache{}
	podCache.cache = make(map[string]*ServiceClusterEntry)
	podCache.mutex = &sync.Mutex{}

	serviceController.Cache = &podCache
	var err error

	serviceController.K8sClient, err = clientLoader.LoadKubeClientFromConfig(config)
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

	NewController("service-ctrl", config.Host, stopCh, &serviceController, serviceController.informer)

	return &serviceController, nil
}

func (s *ServiceController) Added(ctx context.Context, obj interface{}) error {
	service, ok := obj.(*k8sV1.Service)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Service", obj)
	}
	s.Cache.Put(service)
	return s.ServiceHandler.Added(ctx, service)
}

func (s *ServiceController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	service, ok := obj.(*k8sV1.Service)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Service", obj)
	}
	s.Cache.Put(service)
	return s.ServiceHandler.Updated(ctx, service)
}

func (s *ServiceController) Deleted(ctx context.Context, obj interface{}) error {
	service, ok := obj.(*k8sV1.Service)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Service", obj)
	}
	err := s.ServiceHandler.Deleted(ctx, service)
	if err == nil {
		s.Cache.Delete(service)
	}
	return err
}

func (d *ServiceController) GetProcessItemStatus(obj interface{}) (string, error) {
	service, ok := obj.(*k8sV1.Service)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1.Service", obj)
	}
	return d.Cache.GetSvcProcessStatus(service), nil
}

func (d *ServiceController) UpdateProcessItemStatus(obj interface{}, status string) error {
	service, ok := obj.(*k8sV1.Service)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Service", obj)
	}
	return d.Cache.UpdateSvcProcessStatus(service, status)
}

func (s *serviceCache) shouldIgnoreBasedOnLabels(service *k8sV1.Service) bool {
	return service.Annotations[common.AdmiralIgnoreAnnotation] == "true" || service.Labels[common.AdmiralIgnoreAnnotation] == "true"
}

func (d *ServiceController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	s, ok := obj.(*k8sV1.Service)
	if !ok {
		return
	}
	if s.Annotations[common.AdmiralIgnoreAnnotation] == "true" || s.Labels[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.DeploymentResourceType,
			s.Name, s.Namespace, "", "Value=true")
	}
}

func (sec *ServiceController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	service, ok := obj.(*k8sV1.Service)
	if ok && isRetry {
		return sec.Cache.Get(service.Namespace), nil
	}
	if ok && sec.K8sClient != nil {
		return sec.K8sClient.CoreV1().Services(service.Namespace).Get(ctx, service.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("kubernetes client is not initialized, txId=%s", ctx.Value("txId"))
}

func (s *serviceCache) GetSingleLoadBalancerTimeout(key string, namespace string) (string, error) {
	services := s.Get(namespace)
	if len(services) == 0 {
		return "", fmt.Errorf("no services found in namespace: %s", namespace)
	}
	for _, service := range services {
		if common.IsIstioIngressGatewayService(service, key) {
			// Will need to change the processing here once AWS lb controller is updated
			// Value will change from "350" to  "tcp.idle_timeout.seconds=350"
			if service.Annotations != nil {
				if service.Annotations[common.NLBIdleTimeoutAnnotation] != "" {
					return service.Annotations[common.NLBIdleTimeoutAnnotation], nil
				}
			}
		}
	}
	return "", fmt.Errorf("no istio-ingressgateway services found in namespace: %s", namespace)
}
