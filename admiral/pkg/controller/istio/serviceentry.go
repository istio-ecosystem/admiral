package istio

import (
	"context"
	"fmt"
	"time"

	"sync"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	versioned "istio.io/client-go/pkg/clientset/versioned"
	informers "istio.io/client-go/pkg/informers/externalversions/networking/v1alpha3"
	k8sV1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// Handler interface contains the methods that are required
type ServiceEntryHandler interface {
	Added(obj *networking.ServiceEntry) error
	Updated(obj *networking.ServiceEntry) error
	Deleted(obj *networking.ServiceEntry) error
}

type ServiceEntryController struct {
	IstioClient         versioned.Interface
	ServiceEntryHandler ServiceEntryHandler
	informer            cache.SharedIndexInformer
	Cache               *ServiceEntryCache
	Cluster             string
}

func (s *ServiceEntryController) DoesGenerationMatch(*log.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

type ServiceEntryItem struct {
	ServiceEntry *networking.ServiceEntry
	Status       string
}

type ServiceEntryCache struct {
	cache map[string]map[string]*ServiceEntryItem
	mutex *sync.RWMutex
}

func NewServiceEntryCache() *ServiceEntryCache {
	return &ServiceEntryCache{
		cache: map[string]map[string]*ServiceEntryItem{},
		mutex: &sync.RWMutex{},
	}
}

func (d *ServiceEntryCache) getKey(se *networking.ServiceEntry) string {
	return se.Name
}

func (d *ServiceEntryCache) Put(se *networking.ServiceEntry, cluster string) {
	defer d.mutex.Unlock()
	d.mutex.Lock()
	key := d.getKey(se)

	var (
		seInCluster map[string]*ServiceEntryItem
	)

	if value, ok := d.cache[cluster]; !ok {
		seInCluster = make(map[string]*ServiceEntryItem)
	} else {
		seInCluster = value
	}

	seInCluster[key] = &ServiceEntryItem{
		ServiceEntry: se,
		Status:       common.ProcessingInProgress,
	}

	d.cache[cluster] = seInCluster
}

func (d *ServiceEntryCache) Get(identity string, cluster string) *networking.ServiceEntry {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	seInCluster, ok := d.cache[cluster]
	if ok {
		se, ok := seInCluster[identity]
		if ok {
			return se.ServiceEntry
		}
	}
	log.Infof("no service entry found in cache for identity=%s cluster=%s", identity, cluster)
	return nil
}

func (d *ServiceEntryCache) Delete(se *networking.ServiceEntry, cluster string) {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	seInCluster, ok := d.cache[cluster]
	if ok {
		delete(seInCluster, d.getKey(se))
	}
}

func (d *ServiceEntryCache) GetSEProcessStatus(se *networking.ServiceEntry, cluster string) string {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	seInCluster, ok := d.cache[cluster]
	if ok {
		key := d.getKey(se)
		sec, ok := seInCluster[key]
		if ok {
			return sec.Status
		}
	}

	return common.NotProcessed
}

func (d *ServiceEntryCache) UpdateSEProcessStatus(se *networking.ServiceEntry, cluster string, status string) error {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	seInCluster, ok := d.cache[cluster]
	if ok {
		key := d.getKey(se)
		sec, ok := seInCluster[key]
		if ok {
			sec.Status = status
			seInCluster[key] = sec
			return nil
		}
	}

	return fmt.Errorf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Update", "ServiceEntry",
		se.Name, se.Namespace, "", "nothing to update, serviceentry not found in cache")
}

func NewServiceEntryController(stopCh <-chan struct{}, handler ServiceEntryHandler, clusterID string, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*ServiceEntryController, error) {
	seController := ServiceEntryController{}
	seController.ServiceEntryHandler = handler

	seCache := ServiceEntryCache{}
	seCache.cache = make(map[string]map[string]*ServiceEntryItem)
	seCache.mutex = &sync.RWMutex{}
	seController.Cache = &seCache

	seController.Cluster = clusterID

	var err error

	ic, err := clientLoader.LoadIstioClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create service entry k8s client: %v", err)
	}

	seController.IstioClient = ic

	seController.informer = informers.NewServiceEntryInformer(ic, k8sV1.NamespaceAll, resyncPeriod, cache.Indexers{})

	admiral.NewController("serviceentry-ctrl", config.Host, stopCh, &seController, seController.informer)

	return &seController, nil
}

func (sec *ServiceEntryController) Added(ctx context.Context, obj interface{}) error {
	se, ok := obj.(*networking.ServiceEntry)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.ServiceEntry", obj)
	}
	sec.Cache.Put(se, sec.Cluster)
	return sec.ServiceEntryHandler.Added(se)
}

func (sec *ServiceEntryController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	se, ok := obj.(*networking.ServiceEntry)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.ServiceEntry", obj)
	}
	sec.Cache.Put(se, sec.Cluster)
	return sec.ServiceEntryHandler.Updated(se)
}

func (sec *ServiceEntryController) Deleted(ctx context.Context, obj interface{}) error {
	se, ok := obj.(*networking.ServiceEntry)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.ServiceEntry", obj)
	}

	sec.Cache.Delete(se, sec.Cluster)
	return sec.ServiceEntryHandler.Deleted(se)
}

func (sec *ServiceEntryController) GetProcessItemStatus(obj interface{}) (string, error) {
	se, ok := obj.(*networking.ServiceEntry)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.ServiceEntry", obj)
	}
	return sec.Cache.GetSEProcessStatus(se, sec.Cluster), nil
}

func (sec *ServiceEntryController) UpdateProcessItemStatus(obj interface{}, status string) error {
	se, ok := obj.(*networking.ServiceEntry)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.ServiceEntry", obj)
	}
	return sec.Cache.UpdateSEProcessStatus(se, sec.Cluster, status)
}

func (sec *ServiceEntryController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	se, ok := obj.(*networking.ServiceEntry)
	if !ok {
		return
	}
	if len(se.Annotations) > 0 && se.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", "ServiceEntry", se.Name, se.Namespace, "", "Value=true")
	}
}

func (sec *ServiceEntryController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	/*se, ok := obj.(*networking.ServiceEntry)
	if ok && sec.IstioClient != nil {
		return sec.IstioClient.NetworkingV1alpha3().ServiceEntries(se.Namespace).Get(ctx, se.Name, meta_v1.GetOptions{})
	}*/
	return nil, fmt.Errorf("istio client is not initialized, txId=%s", ctx.Value("txId"))
}
