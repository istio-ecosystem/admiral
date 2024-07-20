package admiral

import (
	"context"
	"fmt"
	"sync"
	"time"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/informers/externalversions/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
)

const (
	dependencyControllerPrefix = "dependency-ctrl"
)

// DepHandler interface contains the methods that are required
type DepHandler interface {
	Added(ctx context.Context, obj *v1.Dependency) error
	Updated(ctx context.Context, obj *v1.Dependency) error
	Deleted(ctx context.Context, obj *v1.Dependency) error
}

type DependencyController struct {
	K8sClient    kubernetes.Interface
	DepCrdClient clientset.Interface
	DepHandler   DepHandler
	Cache        *depCache
	informer     cache.SharedIndexInformer
}

type DependencyItem struct {
	Dependency *v1.Dependency
	Status     string
}

type depCache struct {
	//map of dependencies key=identity value array of onboarded identitys
	cache map[string]*DependencyItem
	mutex *sync.Mutex
}

func (d *depCache) Put(dep *v1.Dependency) {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	key := d.getKey(dep)
	d.cache[key] = &DependencyItem{
		Dependency: dep,
		Status:     common.ProcessingInProgress,
	}
}

func (d *depCache) getKey(dep *v1.Dependency) string {
	return dep.Name
}

func (d *depCache) Get(identity string) *v1.Dependency {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	depItem, ok := d.cache[identity]
	if ok {
		return depItem.Dependency
	}

	return nil
}

func (d *depCache) Delete(dep *v1.Dependency) {
	defer d.mutex.Unlock()
	d.mutex.Lock()
	delete(d.cache, d.getKey(dep))
}

func (d *depCache) GetDependencyProcessStatus(dep *v1.Dependency) string {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	key := d.getKey(dep)

	depItem, ok := d.cache[key]
	if ok {
		return depItem.Status
	}

	return common.NotProcessed
}

func (d *depCache) UpdateDependencyProcessStatus(dep *v1.Dependency, status string) error {
	defer d.mutex.Unlock()
	d.mutex.Lock()

	key := d.getKey(dep)

	depItem, ok := d.cache[key]
	if ok {
		depItem.Status = status
		d.cache[key] = depItem
		return nil
	}

	return fmt.Errorf(LogCacheFormat, "Update", "Dependency",
		dep.Name, dep.Namespace, "", "nothing to update, dependency not found in cache")
}

func NewDependencyController(stopCh <-chan struct{}, handler DepHandler, configPath string, namespace string, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*DependencyController, error) {

	depController := DependencyController{}
	depController.DepHandler = handler

	depCache := depCache{}
	depCache.cache = make(map[string]*DependencyItem)
	depCache.mutex = &sync.Mutex{}

	depController.Cache = &depCache
	var err error

	depController.K8sClient, err = clientLoader.LoadKubeClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller k8s client: %v", err)
	}

	depController.DepCrdClient, err = clientLoader.LoadAdmiralClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller crd client: %v", err)

	}

	depController.informer = informerV1.NewDependencyInformer(
		depController.DepCrdClient,
		namespace,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController(dependencyControllerPrefix+"-"+namespace, "", stopCh, &depController, depController.informer)

	return &depController, nil
}

func (d *DependencyController) Added(ctx context.Context, obj interface{}) error {
	dep, ok := obj.(*v1.Dependency)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Dependency", obj)
	}

	d.Cache.Put(dep)
	return d.DepHandler.Added(ctx, dep)
}

func (d *DependencyController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	dep, ok := obj.(*v1.Dependency)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Dependency", obj)
	}

	d.Cache.Put(dep)
	return d.DepHandler.Updated(ctx, dep)
}

func (d *DependencyController) Deleted(ctx context.Context, obj interface{}) error {
	dep, ok := obj.(*v1.Dependency)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Dependency", obj)
	}
	d.Cache.Delete(dep)
	return d.DepHandler.Deleted(ctx, dep)
}

func (d *DependencyController) GetProcessItemStatus(obj interface{}) (string, error) {
	dependency, ok := obj.(*v1.Dependency)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1.Dependency", obj)
	}
	return d.Cache.GetDependencyProcessStatus(dependency), nil
}

func (d *DependencyController) UpdateProcessItemStatus(obj interface{}, status string) error {
	dependency, ok := obj.(*v1.Dependency)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Dependency", obj)
	}
	return d.Cache.UpdateDependencyProcessStatus(dependency, status)
}

func (d *DependencyController) LogValueOfAdmiralIoIgnore(obj interface{}) {
}

func (d *DependencyController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	dep, ok := obj.(*v1.Dependency)
	if ok && isRetry {
		return d.Cache.Get(dep.Name), nil
	}
	if ok && d.DepCrdClient != nil {
		return d.DepCrdClient.AdmiralV1alpha1().Dependencies(dep.Namespace).Get(ctx, dep.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("depcrd client is not initialized, txId=%s", ctx.Value("txId"))
}
