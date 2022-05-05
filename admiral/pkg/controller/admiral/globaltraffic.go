package admiral

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/sirupsen/logrus"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"sync"
	"time"

	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/informers/externalversions/admiral/v1"
)

// Handler interface contains the methods that are required
type GlobalTrafficHandler interface {
	Added(obj *v1.GlobalTrafficPolicy)
	Updated(obj *v1.GlobalTrafficPolicy)
	Deleted(obj *v1.GlobalTrafficPolicy)
}

type GlobalTrafficController struct {
	CrdClient            clientset.Interface
	GlobalTrafficHandler GlobalTrafficHandler
	Cache             	*gtpCache
	informer             cache.SharedIndexInformer
}

type gtpCache struct {
	//map of gtps key=identity+env value is a map of gtps namespace -> map name -> gtp
	cache map[string]map[string]map[string]*v1.GlobalTrafficPolicy
	mutex *sync.Mutex
}

func (p *gtpCache) Put(obj *v1.GlobalTrafficPolicy) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	key := common.GetGtpKey(obj)
	namespacesWithGtps := p.cache[key]
	if namespacesWithGtps == nil {
		namespacesWithGtps = make(map[string]map[string]*v1.GlobalTrafficPolicy)
	}
	namespaceGtps := namespacesWithGtps[obj.Namespace]
	if namespaceGtps == nil {
		namespaceGtps = make(map[string]*v1.GlobalTrafficPolicy)
	}
	if common.ShouldIgnoreResource(obj.ObjectMeta){
		delete(namespaceGtps, obj.Name)
	} else {
		namespaceGtps[obj.Name] = obj
	}

	namespacesWithGtps[obj.Namespace] = namespaceGtps
	p.cache[key] = namespacesWithGtps
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		logrus.Debugf("GTP cache for key=%s gtp=%v", key, namespacesWithGtps)
	}
}

func (p *gtpCache) Delete(obj *v1.GlobalTrafficPolicy) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	key := common.GetGtpKey(obj)
	namespacesWithGtps := p.cache[key]
	if namespacesWithGtps == nil {
		return
	}
	namespaceGtps := namespacesWithGtps[obj.Namespace]
	if namespaceGtps == nil {
		return
	}
	delete(namespaceGtps, obj.Name)
	namespacesWithGtps[obj.Namespace] = namespaceGtps
	p.cache[key] = namespacesWithGtps
}

//fetch gtps for a key from namespace
func (p *gtpCache) Get(key, namespace string) []*v1.GlobalTrafficPolicy {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	namespacesWithGtp := p.cache[key]
	matchedGtps := make([]*v1.GlobalTrafficPolicy, 0)
	for ns, gtps := range namespacesWithGtp {
		 if namespace == ns {
			 for _, gtp := range gtps {
			 	logrus.Debugf("GTP match for identity=%s, from namespace=%v", key, ns)
			 	//make a copy for safer iterations elsewhere
			 	matchedGtps = append(matchedGtps, gtp.DeepCopy())
			 }
		 }
	}
	return matchedGtps
}

func NewGlobalTrafficController(stopCh <-chan struct{}, handler GlobalTrafficHandler, configPath *rest.Config, resyncPeriod time.Duration) (*GlobalTrafficController, error) {

	globalTrafficController := GlobalTrafficController{}

	globalTrafficController.GlobalTrafficHandler = handler

	gtpCache := gtpCache{}
	gtpCache.cache = make(map[string]map[string]map[string]*v1.GlobalTrafficPolicy)
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

	NewController("gtp-ctrl-" + configPath.Host, stopCh, &globalTrafficController, globalTrafficController.informer)

	return &globalTrafficController, nil
}

func (d *GlobalTrafficController) Added(ojb interface{}) {
	gtp := ojb.(*v1.GlobalTrafficPolicy)
	d.Cache.Put(gtp)
	d.GlobalTrafficHandler.Added(gtp)
}

func (d *GlobalTrafficController) Updated(ojb interface{}, oldObj interface{}) {
	gtp := ojb.(*v1.GlobalTrafficPolicy)
	d.Cache.Put(gtp)
	d.GlobalTrafficHandler.Updated(gtp)
}

func (d *GlobalTrafficController) Deleted(ojb interface{}) {
	gtp := ojb.(*v1.GlobalTrafficPolicy)
	d.Cache.Delete(gtp)
	d.GlobalTrafficHandler.Deleted(gtp)
}