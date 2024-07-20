package admiral

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/informers/externalversions/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
)

// DependencyProxyHandler interface contains the methods that are required
type DependencyProxyHandler interface {
	Added(ctx context.Context, obj *v1.DependencyProxy) error
	Updated(ctx context.Context, obj *v1.DependencyProxy) error
	Deleted(ctx context.Context, obj *v1.DependencyProxy) error
}

type DependencyProxyController struct {
	K8sClient              kubernetes.Interface
	admiralCRDClient       clientset.Interface
	DependencyProxyHandler DependencyProxyHandler
	Cache                  *dependencyProxyCache
	informer               cache.SharedIndexInformer
}

type DependencyProxyItem struct {
	DependencyProxy *v1.DependencyProxy
	Status          string
}

type dependencyProxyCache struct {
	//map of dependencies key=identity value array of onboarded identitys
	cache map[string]*DependencyProxyItem
	mutex *sync.Mutex
}

func (d *dependencyProxyCache) Put(dep *v1.DependencyProxy) {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	key := d.getKey(dep)
	d.cache[key] = &DependencyProxyItem{
		DependencyProxy: dep,
		Status:          common.ProcessingInProgress,
	}
}

func (d *dependencyProxyCache) getKey(dep *v1.DependencyProxy) string {
	return dep.Name
}

func (d *dependencyProxyCache) Get(identity string) *v1.DependencyProxy {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	depItem, ok := d.cache[identity]
	if ok {
		return depItem.DependencyProxy
	}

	return nil
}

func (d *dependencyProxyCache) Delete(dep *v1.DependencyProxy) {
	defer d.mutex.Unlock()
	d.mutex.Lock()
	delete(d.cache, d.getKey(dep))
}

func (d *dependencyProxyCache) GetDependencyProxyProcessStatus(dep *v1.DependencyProxy) string {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	key := d.getKey(dep)

	depItem, ok := d.cache[key]
	if ok {
		return depItem.Status
	}

	return common.NotProcessed
}

func (d *dependencyProxyCache) UpdateDependencyProxyProcessStatus(dep *v1.DependencyProxy, status string) error {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	key := d.getKey(dep)

	depItem, ok := d.cache[key]
	if ok {
		depItem.Status = status
		d.cache[key] = depItem
		return nil
	}

	return fmt.Errorf(LogCacheFormat, "Update", "DependencyProxy",
		dep.Name, dep.Namespace, "", "nothing to update, dependency proxy not found in cache")
}

func NewDependencyProxyController(stopCh <-chan struct{}, handler DependencyProxyHandler, configPath string, namespace string, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*DependencyProxyController, error) {

	controller := DependencyProxyController{}
	controller.DependencyProxyHandler = handler

	depProxyCache := dependencyProxyCache{}
	depProxyCache.cache = make(map[string]*DependencyProxyItem)
	depProxyCache.mutex = &sync.Mutex{}

	controller.Cache = &depProxyCache
	var err error

	controller.K8sClient, err = clientLoader.LoadKubeClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller k8s client: %v", err)
	}

	controller.admiralCRDClient, err = clientLoader.LoadAdmiralClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller crd client: %v", err)

	}

	controller.informer = informerV1.NewDependencyProxyInformer(
		controller.admiralCRDClient,
		namespace,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController("dependencyproxy-ctrl", "", stopCh, &controller, controller.informer)

	return &controller, nil
}

func (d *DependencyProxyController) Added(ctx context.Context, obj interface{}) error {
	dep, ok := obj.(*v1.DependencyProxy)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.DependencyProxy", obj)
	}
	d.Cache.Put(dep)
	return d.DependencyProxyHandler.Added(ctx, dep)
}

func (d *DependencyProxyController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	dep, ok := obj.(*v1.DependencyProxy)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.DependencyProxy", obj)
	}
	d.Cache.Put(dep)
	return d.DependencyProxyHandler.Updated(ctx, dep)
}

func (d *DependencyProxyController) Deleted(ctx context.Context, obj interface{}) error {
	dep, ok := obj.(*v1.DependencyProxy)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.DependencyProxy", obj)
	}
	d.Cache.Delete(dep)
	return d.DependencyProxyHandler.Deleted(ctx, dep)
}

func (d *DependencyProxyController) GetProcessItemStatus(obj interface{}) (string, error) {
	dependencyProxy, ok := obj.(*v1.DependencyProxy)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1.DependencyProxy", obj)
	}
	return d.Cache.GetDependencyProxyProcessStatus(dependencyProxy), nil
}

func (d *DependencyProxyController) UpdateProcessItemStatus(obj interface{}, status string) error {
	dependencyProxy, ok := obj.(*v1.DependencyProxy)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.DependencyProxy", obj)
	}
	return d.Cache.UpdateDependencyProxyProcessStatus(dependencyProxy, status)
}

func (d *DependencyProxyController) LogValueOfAdmiralIoIgnore(obj interface{}) {
}

func (d *DependencyProxyController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	dependencyProxy, ok := obj.(*v1.DependencyProxy)
	if ok && isRetry {
		return d.Cache.Get(dependencyProxy.Name), nil
	}
	if ok && d.admiralCRDClient != nil {
		return d.admiralCRDClient.AdmiralV1alpha1().DependencyProxies(dependencyProxy.Namespace).Get(ctx, dependencyProxy.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("admiralcrd client is not initialized, txId=%s", ctx.Value("txId"))
}
