package admiral

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/informers/externalversions/admiral/v1"
)

// DependencyProxyHandler interface contains the methods that are required
type DependencyProxyHandler interface {
	Added(ctx context.Context, obj *v1.DependencyProxy)
	Updated(ctx context.Context, obj *v1.DependencyProxy)
	Deleted(ctx context.Context, obj *v1.DependencyProxy)
}

type DependencyProxyController struct {
	K8sClient              kubernetes.Interface
	admiralCRDClient       clientset.Interface
	DependencyProxyHandler DependencyProxyHandler
	Cache                  *dependencyProxyCache
	informer               cache.SharedIndexInformer
}

type dependencyProxyCache struct {
	//map of dependencies key=identity value array of onboarded identitys
	cache map[string]*v1.DependencyProxy
	mutex *sync.Mutex
}

func (d *dependencyProxyCache) Put(dep *v1.DependencyProxy) {
	defer d.mutex.Unlock()
	d.mutex.Lock()
	key := d.getKey(dep)
	d.cache[key] = dep
}

func (d *dependencyProxyCache) getKey(dep *v1.DependencyProxy) string {
	return dep.Name
}

func (d *dependencyProxyCache) Get(identity string) *v1.DependencyProxy {
	defer d.mutex.Unlock()
	d.mutex.Lock()
	return d.cache[identity]
}

func (d *dependencyProxyCache) Delete(dep *v1.DependencyProxy) {
	defer d.mutex.Unlock()
	d.mutex.Lock()
	delete(d.cache, d.getKey(dep))
}

func NewDependencyProxyController(stopCh <-chan struct{}, handler DependencyProxyHandler, configPath string, namespace string, resyncPeriod time.Duration) (*DependencyProxyController, error) {

	controller := DependencyProxyController{}
	controller.DependencyProxyHandler = handler

	depProxyCache := dependencyProxyCache{}
	depProxyCache.cache = make(map[string]*v1.DependencyProxy)
	depProxyCache.mutex = &sync.Mutex{}

	controller.Cache = &depProxyCache
	var err error

	controller.K8sClient, err = K8sClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller k8s client: %v", err)
	}

	controller.admiralCRDClient, err = AdmiralCrdClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller crd client: %v", err)

	}

	controller.informer = informerV1.NewDependencyProxyInformer(
		controller.admiralCRDClient,
		namespace,
		resyncPeriod,
		cache.Indexers{},
	)

	mcd := NewMonitoredDelegator(&controller, "primary", "dependencyproxy")
	NewController("dependencyproxy-ctrl-"+namespace, stopCh, mcd, controller.informer)

	return &controller, nil
}

func (d *DependencyProxyController) Added(ctx context.Context, ojb interface{}) {
	dep := ojb.(*v1.DependencyProxy)
	d.Cache.Put(dep)
	d.DependencyProxyHandler.Added(ctx, dep)
}

func (d *DependencyProxyController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) {
	dep := obj.(*v1.DependencyProxy)
	d.Cache.Put(dep)
	d.DependencyProxyHandler.Updated(ctx, dep)
}

func (d *DependencyProxyController) Deleted(ctx context.Context, ojb interface{}) {
	dep := ojb.(*v1.DependencyProxy)
	d.Cache.Delete(dep)
	d.DependencyProxyHandler.Deleted(ctx, dep)
}
