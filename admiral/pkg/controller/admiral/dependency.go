package admiral

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sync"
	"time"

	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/informers/externalversions/admiral/v1"
)

// Handler interface contains the methods that are required
type DepHandler interface {
	Added(obj *v1.Dependency)
	Updated(obj *v1.Dependency)
	Deleted(obj *v1.Dependency)
}

type DependencyController struct {
	K8sClient    kubernetes.Interface
	DepCrdClient clientset.Interface
	DepHandler   DepHandler
	Cache        *depCache
	informer     cache.SharedIndexInformer
}

type depCache struct {
	//map of dependencies key=identity value array of onboarded identitys
	cache map[string]*v1.Dependency
	mutex *sync.Mutex
}

func (d *depCache) Put(dep *v1.Dependency) {
	defer d.mutex.Unlock()
	d.mutex.Lock()
	key := d.getKey(dep)
	d.cache[key] = dep
}

func (d *depCache) getKey(dep *v1.Dependency) string {
	return dep.Name
}

func (d *depCache) Get(identity string) *v1.Dependency {
	return d.cache[identity]
}

func (d *depCache) Delete(dep *v1.Dependency) {
	defer d.mutex.Unlock()
	d.mutex.Lock()
	delete(d.cache, d.getKey(dep))
}

func NewDependencyController(stopCh <-chan struct{}, handler DepHandler, configPath string, namespace string, resyncPeriod time.Duration) (*DependencyController, error) {

	depController := DependencyController{}
	depController.DepHandler = handler

	depCache := depCache{}
	depCache.cache = make(map[string]*v1.Dependency)
	depCache.mutex = &sync.Mutex{}

	depController.Cache = &depCache
	var err error

	depController.K8sClient, err = K8sClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller k8s client: %v", err)
	}

	depController.DepCrdClient, err = AdmiralCrdClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller crd client: %v", err)

	}

	depController.informer = informerV1.NewDependencyInformer(
		depController.DepCrdClient,
		namespace,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController("dependency-ctrl-" + namespace, stopCh, &depController, depController.informer)

	return &depController, nil
}

func (d *DependencyController) Added(ojb interface{}) {
	dep := ojb.(*v1.Dependency)
	d.Cache.Put(dep)
	d.DepHandler.Added(dep)
}

func (d *DependencyController) Updated(obj interface{}, oldObj interface{}) {
	dep := obj.(*v1.Dependency)
	d.Cache.Put(dep)
	d.DepHandler.Updated(dep)
}

func (d *DependencyController) Deleted(ojb interface{}) {
	dep := ojb.(*v1.Dependency)
	d.Cache.Delete(dep)
	d.DepHandler.Deleted(dep)
}
