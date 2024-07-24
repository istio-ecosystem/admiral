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
	versioned "istio.io/client-go/pkg/clientset/versioned"
	informers "istio.io/client-go/pkg/informers/externalversions/networking/v1alpha3"
	k8sV1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// Handler interface contains the methods that are required
type DestinationRuleHandler interface {
	Added(ctx context.Context, obj *networking.DestinationRule) error
	Updated(ctx context.Context, obj *networking.DestinationRule) error
	Deleted(ctx context.Context, obj *networking.DestinationRule) error
}

type DestinationRuleEntry struct {
	Identity        string
	DestinationRule *networking.DestinationRule
}

type DestinationRuleController struct {
	IstioClient            versioned.Interface
	DestinationRuleHandler DestinationRuleHandler
	informer               cache.SharedIndexInformer
	Cache                  *DestinationRuleCache
	Cluster                string
}

type DestinationRuleItem struct {
	DestinationRule *networking.DestinationRule
	Status          string
}

type DestinationRuleCache struct {
	cache map[string]*DestinationRuleItem
	mutex *sync.RWMutex
}

func NewDestinationRuleCache() *DestinationRuleCache {
	return &DestinationRuleCache{
		cache: map[string]*DestinationRuleItem{},
		mutex: &sync.RWMutex{},
	}
}

func (d *DestinationRuleCache) getKey(dr *networking.DestinationRule) string {
	return makeKey(dr.Name, dr.Namespace)
}

func makeKey(str1, str2 string) string {
	return str1 + "/" + str2
}

func (d *DestinationRuleCache) Put(dr *networking.DestinationRule) {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	key := d.getKey(dr)

	d.cache[key] = &DestinationRuleItem{
		DestinationRule: dr,
		Status:          common.ProcessingInProgress,
	}
}

func (d *DestinationRuleCache) Get(identity string, namespace string) *networking.DestinationRule {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	drItem, ok := d.cache[makeKey(identity, namespace)]
	if ok {
		return drItem.DestinationRule
	}

	log.Infof("no destinationrule found in cache for identity=%s", identity)
	return nil
}

func (d *DestinationRuleCache) Delete(dr *networking.DestinationRule) {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	key := d.getKey(dr)

	_, ok := d.cache[key]
	if ok {
		delete(d.cache, key)
	}
}

func (d *DestinationRuleCache) GetDRProcessStatus(dr *networking.DestinationRule) string {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	key := d.getKey(dr)

	dc, ok := d.cache[key]
	if ok {
		return dc.Status
	}
	return common.NotProcessed
}

func (d *DestinationRuleCache) UpdateDRProcessStatus(dr *networking.DestinationRule, status string) error {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	key := d.getKey(dr)

	dc, ok := d.cache[key]
	if ok {

		dc.Status = status
		d.cache[key] = dc
		return nil
	}

	return fmt.Errorf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Update", "DestinationRule",
		dr.Name, dr.Namespace, "", "nothing to update, destinationrule not found in cache")
}

func NewDestinationRuleController(stopCh <-chan struct{}, handler DestinationRuleHandler, clusterID string, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*DestinationRuleController, error) {

	drController := DestinationRuleController{}
	drController.DestinationRuleHandler = handler

	drCache := DestinationRuleCache{}
	drCache.cache = make(map[string]*DestinationRuleItem)
	drCache.mutex = &sync.RWMutex{}
	drController.Cache = &drCache

	drController.Cluster = clusterID

	var err error

	ic, err := clientLoader.LoadIstioClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination rule controller k8s client: %v", err)
	}

	drController.IstioClient = ic

	drController.informer = informers.NewDestinationRuleInformer(ic, k8sV1.NamespaceAll, resyncPeriod, cache.Indexers{})

	admiral.NewController("destinationrule-ctrl", config.Host, stopCh, &drController, drController.informer)

	return &drController, nil
}

func (drc *DestinationRuleController) Added(ctx context.Context, obj interface{}) error {
	dr, ok := obj.(*networking.DestinationRule)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.DestinationRule", obj)
	}
	drc.Cache.Put(dr)
	return drc.DestinationRuleHandler.Added(ctx, dr)
}

func (drc *DestinationRuleController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	dr, ok := obj.(*networking.DestinationRule)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.DestinationRule", obj)
	}
	drc.Cache.Put(dr)
	return drc.DestinationRuleHandler.Updated(ctx, dr)
}

func (drc *DestinationRuleController) Deleted(ctx context.Context, obj interface{}) error {
	dr, ok := obj.(*networking.DestinationRule)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.DestinationRule", obj)
	}
	drc.Cache.Delete(dr)
	return drc.DestinationRuleHandler.Deleted(ctx, dr)
}

func (drc *DestinationRuleController) GetProcessItemStatus(obj interface{}) (string, error) {
	dr, ok := obj.(*networking.DestinationRule)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.DestinationRule", obj)
	}
	return drc.Cache.GetDRProcessStatus(dr), nil
}

func (drc *DestinationRuleController) UpdateProcessItemStatus(obj interface{}, status string) error {
	dr, ok := obj.(*networking.DestinationRule)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.DestinationRule", obj)
	}
	return drc.Cache.UpdateDRProcessStatus(dr, status)
}

func (drc *DestinationRuleController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	dr, ok := obj.(*networking.DestinationRule)
	if !ok {
		return
	}
	if len(dr.Annotations) > 0 && dr.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", "DestinationRule", dr.Name, dr.Namespace, "", "Value=true")
	}
}

func (drc *DestinationRuleController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	/*dr, ok := obj.(*networking.DestinationRule)
	if ok && d.IstioClient != nil {
		return d.IstioClient.NetworkingV1alpha3().DestinationRules(dr.Namespace).Get(ctx, dr.Name, meta_v1.GetOptions{})
	}*/
	return nil, fmt.Errorf("istio client is not initialized, txId=%s", ctx.Value("txId"))
}
