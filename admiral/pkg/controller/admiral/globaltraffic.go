package admiral

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/common/log"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/sirupsen/logrus"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/informers/externalversions/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
)

// GlobalTrafficHandler interface contains the methods that are required
type GlobalTrafficHandler interface {
	Added(ctx context.Context, obj *v1.GlobalTrafficPolicy) error
	Updated(ctx context.Context, obj *v1.GlobalTrafficPolicy) error
	Deleted(ctx context.Context, obj *v1.GlobalTrafficPolicy) error
}

type GlobalTrafficController struct {
	CrdClient            clientset.Interface
	GlobalTrafficHandler GlobalTrafficHandler
	Cache                *gtpCache
	informer             cache.SharedIndexInformer
}

type gtpItem struct {
	GlobalTrafficPolicy *v1.GlobalTrafficPolicy
	Status              string
}

type gtpCache struct {
	//map of gtps key=identity+env value is a map of gtps namespace -> map name -> gtp
	cache map[string]map[string]map[string]*gtpItem
	mutex *sync.Mutex
}

func (p *gtpCache) Put(obj *v1.GlobalTrafficPolicy) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	key := common.GetGtpKey(obj)
	namespacesWithGtps := p.cache[key]
	if namespacesWithGtps == nil {
		namespacesWithGtps = make(map[string]map[string]*gtpItem)
	}
	namespaceGtps := namespacesWithGtps[obj.Namespace]
	if namespaceGtps == nil {
		namespaceGtps = make(map[string]*gtpItem)
	}
	if common.ShouldIgnoreResource(obj.ObjectMeta) {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.GlobalTrafficPolicyResourceType,
			obj.Name, obj.Namespace, "", "Value=true")
		delete(namespaceGtps, obj.Name)
	} else {
		namespaceGtps[obj.Name] = &gtpItem{
			GlobalTrafficPolicy: obj,
			Status:              common.ProcessingInProgress,
		}
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

// fetch gtps for a key from namespace
func (p *gtpCache) Get(key, namespace string) []*v1.GlobalTrafficPolicy {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	namespacesWithGtp := p.cache[key]
	matchedGtps := make([]*v1.GlobalTrafficPolicy, 0)
	for ns, gtpItems := range namespacesWithGtp {
		if namespace == ns {
			for _, item := range gtpItems {
				logrus.Debugf("GTP match for identity=%s, from namespace=%v", key, ns)
				//make a copy for safer iterations elsewhere
				matchedGtps = append(matchedGtps, item.GlobalTrafficPolicy.DeepCopy())
			}
		}
	}
	return matchedGtps
}

func (p *gtpCache) GetGTPProcessStatus(gtp *v1.GlobalTrafficPolicy) string {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	key := common.GetGtpKey(gtp)

	namespacesWithGtps, ok := p.cache[key]
	if ok {
		namespaceGtps, ok := namespacesWithGtps[gtp.Namespace]
		if ok {
			nameGtp, ok := namespaceGtps[gtp.Name]
			if ok {
				return nameGtp.Status
			}
		}
	}

	return common.NotProcessed
}

func (p *gtpCache) UpdateGTPProcessStatus(gtp *v1.GlobalTrafficPolicy, status string) error {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	key := common.GetGtpKey(gtp)

	namespacesWithGtps, ok := p.cache[key]
	if ok {
		namespaceGtps, ok := namespacesWithGtps[gtp.Namespace]
		if ok {
			nameGtp, ok := namespaceGtps[gtp.Name]
			if ok {
				nameGtp.Status = status
				p.cache[key] = namespacesWithGtps
				return nil
			}
		}
	}

	return fmt.Errorf(LogCacheFormat, "Update", "GTP",
		gtp.Name, gtp.Namespace, "", "nothing to update, gtp not found in cache")
}

func NewGlobalTrafficController(stopCh <-chan struct{}, handler GlobalTrafficHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*GlobalTrafficController, error) {

	globalTrafficController := GlobalTrafficController{}

	globalTrafficController.GlobalTrafficHandler = handler

	gtpCache := gtpCache{}
	gtpCache.cache = make(map[string]map[string]map[string]*gtpItem)
	gtpCache.mutex = &sync.Mutex{}

	globalTrafficController.Cache = &gtpCache

	var err error

	globalTrafficController.CrdClient, err = clientLoader.LoadAdmiralClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create global traffic controller crd client: %v", err)
	}

	globalTrafficController.informer = informerV1.NewGlobalTrafficPolicyInformer(
		globalTrafficController.CrdClient,
		meta_v1.NamespaceAll,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController(common.GTPCtrl, config.Host, stopCh, &globalTrafficController, globalTrafficController.informer)

	return &globalTrafficController, nil
}

func (d *GlobalTrafficController) Added(ctx context.Context, obj interface{}) error {
	gtp, ok := obj.(*v1.GlobalTrafficPolicy)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.GlobalTrafficPolicy", obj)
	}
	d.Cache.Put(gtp)
	return d.GlobalTrafficHandler.Added(ctx, gtp)
}

func (d *GlobalTrafficController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	gtp, ok := obj.(*v1.GlobalTrafficPolicy)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.GlobalTrafficPolicy", obj)
	}
	d.Cache.Put(gtp)
	return d.GlobalTrafficHandler.Updated(ctx, gtp)
}

func (d *GlobalTrafficController) Deleted(ctx context.Context, obj interface{}) error {
	gtp, ok := obj.(*v1.GlobalTrafficPolicy)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.GlobalTrafficPolicy", obj)
	}
	d.Cache.Delete(gtp)
	return d.GlobalTrafficHandler.Deleted(ctx, gtp)
}

func (d *GlobalTrafficController) GetProcessItemStatus(obj interface{}) (string, error) {
	gtp, ok := obj.(*v1.GlobalTrafficPolicy)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1.GlobalTrafficPolicy", obj)
	}
	return d.Cache.GetGTPProcessStatus(gtp), nil
}

func (d *GlobalTrafficController) UpdateProcessItemStatus(obj interface{}, status string) error {
	gtp, ok := obj.(*v1.GlobalTrafficPolicy)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.GlobalTrafficPolicy", obj)
	}
	return d.Cache.UpdateGTPProcessStatus(gtp, status)
}

func (d *GlobalTrafficController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	gtp, ok := obj.(*v1.GlobalTrafficPolicy)
	if !ok {
		return
	}
	metadata := gtp.ObjectMeta
	if metadata.Annotations[common.AdmiralIgnoreAnnotation] == "true" || metadata.Labels[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.GlobalTrafficPolicyResourceType,
			gtp.Name, gtp.Namespace, "", "Value=true")
	}
}

func (d *GlobalTrafficController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	gtp, ok := obj.(*v1.GlobalTrafficPolicy)
	if ok && isRetry {
		orderedGtps := d.Cache.Get(common.GetGtpKey(gtp), gtp.Namespace)
		if len(orderedGtps) == 0 {
			return nil, fmt.Errorf("no gtps found for identity=%s, namespace=%s", common.GetGtpIdentity(gtp), gtp.Namespace)
		}
		common.SortGtpsByPriorityAndCreationTime(orderedGtps, common.GetGtpIdentity(gtp), common.GetGtpEnv(gtp))
		return orderedGtps[0], nil
	}
	if ok && d.CrdClient != nil {
		return d.CrdClient.AdmiralV1alpha1().GlobalTrafficPolicies(gtp.Namespace).Get(ctx, gtp.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("kubernetes client is not initialized, txId=%s", ctx.Value("txId"))
}
