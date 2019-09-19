package admiral

import (
	"fmt"
	"github.com/admiral/admiral/pkg/apis/admiral/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"sync"
	"time"

	clientset "github.com/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/admiral/admiral/pkg/client/informers/externalversions/admiral/v1"
)

// Handler interface contains the methods that are required
type GlobalTrafficHandler interface {
	Added(obj *v1.GlobalTrafficPolicy)
	Deleted(obj *v1.GlobalTrafficPolicy)
}

type GlobalTrafficController struct {
	CrdClient  clientset.Interface
	DepHandler GlobalTrafficHandler
	Cache      *globalTarfficCache
	informer   cache.SharedIndexInformer
	ctl        *Controller
}

type globalTarfficCache struct {
	//map of dependencies key=identity value array of onboarded identitys
	cache map[string]*v1.GlobalTrafficPolicy
	mutex *sync.Mutex
}

func (g *globalTarfficCache) Put(dep *v1.GlobalTrafficPolicy) {
	defer g.mutex.Unlock()
	g.mutex.Lock()
	key := g.getKey(dep)
	g.cache[key] = dep
}

func (d *globalTarfficCache) getKey(dep *v1.GlobalTrafficPolicy) string {
	return dep.Spec.Selector["identity"]
}

func (g *globalTarfficCache) Get(identity string) *v1.GlobalTrafficPolicy {
	return g.cache[identity]
}

func (g *globalTarfficCache) Delete(dep *v1.GlobalTrafficPolicy) {
	defer g.mutex.Unlock()
	g.mutex.Lock()
	delete(g.cache, g.getKey(dep))
}

func (g *GlobalTrafficController) GetGlobalTrafficPolicies() ([]v1.GlobalTrafficPolicy, error) {

	gtp := g.CrdClient.AdmiralV1().GlobalTrafficPolicies(meta_v1.NamespaceAll)
	dl, err := gtp.List(meta_v1.ListOptions{})

	if err != nil {
		return nil, err
	}
	return dl.Items, err
}

func NewGlobalTrafficController(stopCh <-chan struct{}, handler GlobalTrafficHandler, configPath *rest.Config, resyncPeriod time.Duration) (*GlobalTrafficController, error) {

	globalTrafficController := GlobalTrafficController{}
	globalTrafficController.DepHandler = handler

	gtpCache := globalTarfficCache{}
	gtpCache.cache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.mutex = &sync.Mutex{}

	globalTrafficController.Cache = &gtpCache

	var err error

	globalTrafficController.CrdClient, err = AdmiralCrdClientFromConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create global traffic controller crd client: %v", err)
	}

	globalTrafficController.informer = informerV1.NewGlobalTrafficPolicyInformer(
		globalTrafficController.CrdClient,
		meta_v1.NamespaceAll,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController(stopCh, &globalTrafficController, globalTrafficController.informer)

	return &globalTrafficController, nil
}

func (d *GlobalTrafficController) Added(ojb interface{}) {
	dep := ojb.(*v1.GlobalTrafficPolicy)
	d.Cache.Put(dep)
	d.DepHandler.Added(dep)
}

func (d *GlobalTrafficController) Deleted(name string) {
	//TODO this delete needs to be sorted out
	d.DepHandler.Deleted(nil)
}
