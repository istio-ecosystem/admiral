package istio

import (
	"context"
	"fmt"
	"sync"
	"time"

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

// SidecarHandler interface contains the methods that are required
type SidecarHandler interface {
	Added(ctx context.Context, obj *networking.Sidecar) error
	Updated(ctx context.Context, obj *networking.Sidecar) error
	Deleted(ctx context.Context, obj *networking.Sidecar) error
}

type SidecarEntry struct {
	Identity string
	Sidecar  *networking.Sidecar
}

type SidecarController struct {
	IstioClient    versioned.Interface
	SidecarHandler SidecarHandler
	informer       cache.SharedIndexInformer
	SidecarCache   ISidecarCache
}

type ISidecarCache interface {
	Put(sidecar *networking.Sidecar) error
	Get(sidecarName, sidecarNamespace string) *networking.Sidecar
	Delete(sidecar *networking.Sidecar)
	GetSidecarProcessStatus(sidecar *networking.Sidecar) string
	UpdateSidecarProcessStatus(sidecar *networking.Sidecar, status string) error
}

type SidecarItem struct {
	Sidecar *networking.Sidecar
	Status  string
}

type SidecarCache struct {
	cache map[string]*SidecarItem
	mutex *sync.RWMutex
}

// TODO: Add unit tests
func (s *SidecarCache) Put(sidecar *networking.Sidecar) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !common.IsSidecarCachingEnabled() {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Put", "Sidecar",
			sidecar.Name, sidecar.Namespace, "", "sidecar caching is disabled, skipping")
		return nil
	}

	if sidecar == nil {
		return fmt.Errorf("sidecar is nil")
	}

	if sidecar.Name != common.GetWorkloadSidecarName() {
		log.Debugf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Put", "Sidecar",
			sidecar.Name, sidecar.Namespace, "", "skipping sidecar as it is not the workload sidecar")
		return nil
	}
	if sidecar.Spec.Egress == nil || len(sidecar.Spec.Egress) == 0 {
		return fmt.Errorf("sidecar has no egress")
	}
	if sidecar.Spec.Egress[0].Hosts == nil {
		return fmt.Errorf("sidecar has no hosts")
	}
	if len(sidecar.Spec.Egress[0].Hosts) > common.GetMaxSidecarEgressHostsLimitToCache() {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Put", "Sidecar",
			sidecar.Name, sidecar.Namespace, "", "skipping sidecar caching as its egress hosts exceed the limit")
		return nil
	}

	key := fmt.Sprintf("%s/%s", sidecar.Namespace, sidecar.Name)
	item := &SidecarItem{
		Sidecar: sidecar,
		Status:  common.ProcessingInProgress,
	}

	s.cache[key] = item
	return nil
}

func (s *SidecarCache) Get(sidecarName, sidecarNamespace string) *networking.Sidecar {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if sidecarName == "" || sidecarNamespace == "" {
		return nil
	}

	key := fmt.Sprintf("%s/%s", sidecarNamespace, sidecarName)

	if item, exists := s.cache[key]; exists {
		return item.Sidecar
	}
	return nil
}

func (s *SidecarCache) Delete(sidecar *networking.Sidecar) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if sidecar == nil {
		return
	}

	key := fmt.Sprintf("%s/%s", sidecar.Namespace, sidecar.Name)

	if _, exists := s.cache[key]; exists {
		delete(s.cache, key)
	}
}

func (s *SidecarCache) GetSidecarProcessStatus(sidecar *networking.Sidecar) string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if sidecar == nil {
		return common.NotProcessed
	}

	key := fmt.Sprintf("%s/%s", sidecar.Namespace, sidecar.Name)

	if item, exists := s.cache[key]; exists {
		return item.Status
	}
	return common.NotProcessed
}

func (s *SidecarCache) UpdateSidecarProcessStatus(sidecar *networking.Sidecar, status string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if sidecar == nil {
		return fmt.Errorf("sidecar is nil")
	}

	key := fmt.Sprintf("%s/%s", sidecar.Namespace, sidecar.Name)

	if item, exists := s.cache[key]; exists {
		item.Status = status
		s.cache[key] = item
		return nil
	}
	log.Debugf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Update", "Sidecar",
		sidecar.Name, sidecar.Namespace, "", "nothing to update, sidecar not found in cache")
	return nil
}

func NewSidecarCache() *SidecarCache {
	return &SidecarCache{
		cache: make(map[string]*SidecarItem),
		mutex: &sync.RWMutex{},
	}
}

func (s *SidecarController) DoesGenerationMatch(*log.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

func (s *SidecarController) IsOnlyReplicaCountChanged(*log.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

func NewSidecarController(stopCh <-chan struct{}, handler SidecarHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*SidecarController, error) {

	sidecarController := SidecarController{}
	sidecarController.SidecarHandler = handler
	sidecarController.SidecarCache = NewSidecarCache()

	var err error

	ic, err := clientLoader.LoadIstioClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create sidecar controller k8s client: %v", err)
	}

	sidecarController.IstioClient = ic

	sidecarController.informer = informers.NewSidecarInformer(ic, k8sV1.NamespaceAll, resyncPeriod, cache.Indexers{})

	admiral.NewController("sidecar-ctrl", config.Host, stopCh, &sidecarController, sidecarController.informer)

	return &sidecarController, nil
}

func (sec *SidecarController) Added(ctx context.Context, obj interface{}) error {
	sidecar, ok := obj.(*networking.Sidecar)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.Sidecar", obj)
	}
	return sec.SidecarHandler.Added(ctx, sidecar)
}

func (sec *SidecarController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	sidecar, ok := obj.(*networking.Sidecar)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.Sidecar", obj)
	}
	return sec.SidecarHandler.Updated(ctx, sidecar)
}

func (sec *SidecarController) Deleted(ctx context.Context, obj interface{}) error {
	sidecar, ok := obj.(*networking.Sidecar)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.Sidecar", obj)
	}
	return sec.SidecarHandler.Deleted(ctx, sidecar)
}

func (sec *SidecarController) GetProcessItemStatus(obj interface{}) (string, error) {
	return common.NotProcessed, nil
}

func (sec *SidecarController) UpdateProcessItemStatus(obj interface{}, status string) error {
	return nil
}

func (sec *SidecarController) LogValueOfAdmiralIoIgnore(obj interface{}) {
}

func (sec *SidecarController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	/*sidecar, ok := obj.(*networking.Sidecar)
	if ok && sec.IstioClient != nil {
		return sec.IstioClient.NetworkingV1alpha3().Sidecars(sidecar.Namespace).Get(ctx, sidecar.Name, meta_v1.GetOptions{})
	}*/
	return nil, fmt.Errorf("istio client is not initialized, txId=%s", ctx.Value("txId"))
}
