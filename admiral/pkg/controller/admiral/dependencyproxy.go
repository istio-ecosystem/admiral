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

// DepProxyHandler interface contains the methods that are required
type DepProxyHandler interface {
	Added(ctx context.Context, obj *v1.DependencyProxy)
	Updated(ctx context.Context, obj *v1.DependencyProxy)
	Deleted(ctx context.Context, obj *v1.DependencyProxy)
}

type DependencyProxyController struct {
	K8sClient       kubernetes.Interface
	DepCrdClient    clientset.Interface
	DepProxyHandler DepProxyHandler
	Cache           *depProxyCache
	informer        cache.SharedIndexInformer
}

type depProxyCache struct {
	//map of dependencies key=identity value array of onboarded identitys
	cache map[string]*v1.DependencyProxy
	mutex *sync.Mutex
}

func (d *depProxyCache) Put(dep *v1.DependencyProxy) {
	defer d.mutex.Unlock()
	d.mutex.Lock()
	key := d.getKey(dep)
	d.cache[key] = dep
}

func (d *depProxyCache) getKey(dep *v1.DependencyProxy) string {
	return dep.Name
}

func (d *depProxyCache) Get(identity string) *v1.DependencyProxy {
	defer d.mutex.Unlock()
	d.mutex.Lock()
	return d.cache[identity]
}

func (d *depProxyCache) Delete(dep *v1.DependencyProxy) {
	defer d.mutex.Unlock()
	d.mutex.Lock()
	delete(d.cache, d.getKey(dep))
}

func NewDependencyProxyController(stopCh <-chan struct{}, handler DepProxyHandler, configPath string, namespace string, resyncPeriod time.Duration) (*DependencyProxyController, error) {

	depProxyController := DependencyProxyController{}
	depProxyController.DepProxyHandler = handler

	depProxyCache := depProxyCache{}
	depProxyCache.cache = make(map[string]*v1.DependencyProxy)
	depProxyCache.mutex = &sync.Mutex{}

	depProxyController.Cache = &depProxyCache
	var err error

	depProxyController.K8sClient, err = K8sClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller k8s client: %v", err)
	}

	depProxyController.DepCrdClient, err = AdmiralCrdClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller crd client: %v", err)

	}

	depProxyController.informer = informerV1.NewDependencyProxyInformer(
		depProxyController.DepCrdClient,
		namespace,
		resyncPeriod,
		cache.Indexers{},
	)

	mcd := NewMonitoredDelegator(&depProxyController, "primary", "dependencyproxy")
	NewController("dependencyproxy-ctrl-"+namespace, stopCh, mcd, depProxyController.informer)

	return &depProxyController, nil
}

func (d *DependencyProxyController) Added(ctx context.Context, ojb interface{}) {
	dep := ojb.(*v1.DependencyProxy)
	d.Cache.Put(dep)
	d.DepProxyHandler.Added(ctx, dep)
}

func (d *DependencyProxyController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) {
	dep := obj.(*v1.DependencyProxy)
	d.Cache.Put(dep)
	d.DepProxyHandler.Updated(ctx, dep)
}

func (d *DependencyProxyController) Deleted(ctx context.Context, ojb interface{}) {
	dep := ojb.(*v1.DependencyProxy)
	d.Cache.Delete(dep)
	d.DepProxyHandler.Deleted(ctx, dep)
}
