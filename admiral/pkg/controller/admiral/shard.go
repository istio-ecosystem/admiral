package admiral

import (
	"context"
	"fmt"
	admiralapiv1 "github.com/istio-ecosystem/admiral-api/pkg/apis/admiral/v1"
	admiralapi "github.com/istio-ecosystem/admiral-api/pkg/client/clientset/versioned"
	v1 "github.com/istio-ecosystem/admiral-api/pkg/client/informers/externalversions/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sync"
	"time"
)

type ShardHandler interface {
	Added(ctx context.Context, obj *admiralapiv1.Shard) error
	Deleted(ctx context.Context, obj *admiralapiv1.Shard) error
}

type ShardItem struct {
	Shard  *admiralapiv1.Shard
	Status string
}

type ShardController struct {
	K8sClient    kubernetes.Interface
	CrdClient    admiralapi.Interface
	Cache        *shardCache
	informer     cache.SharedIndexInformer
	mutex        sync.Mutex
	ShardHandler ShardHandler
}

func (d *ShardController) DoesGenerationMatch(entry *log.Entry, i interface{}, i2 interface{}) (bool, error) {
	return false, nil
}

// shardCache is a map from shard name to corresponding ShardItem
type shardCache struct {
	cache map[string]*ShardItem
	mutex *sync.Mutex
}

func newShardCache() *shardCache {
	return &shardCache{
		cache: make(map[string]*ShardItem),
		mutex: &sync.Mutex{},
	}
}

func (p *shardCache) getKey(shard *admiralapiv1.Shard) string {
	return shard.Name
}

func (p *shardCache) Get(key string) *admiralapiv1.Shard {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	shardItem, ok := p.cache[key]
	if ok {
		return shardItem.Shard
	}

	return nil
}

func (p *shardCache) GetShardProcessStatus(shard *admiralapiv1.Shard) string {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	key := p.getKey(shard)

	shardItem, ok := p.cache[key]
	if ok {
		return shardItem.Status
	}

	return common.NotProcessed
}

func (p *shardCache) UpdateShardProcessStatus(shard *admiralapiv1.Shard, status string) error {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	key := p.getKey(shard)

	shardItem, ok := p.cache[key]
	if ok {
		shardItem.Status = status
		p.cache[key] = shardItem
		return nil
	} else {
		return fmt.Errorf(LogCacheFormat, "Update", "Shard",
			shard.Name, shard.Namespace, "", "nothing to update, shard not found in cache")
	}

}

func (p *shardCache) UpdateShardToClusterCache(key string, shard *admiralapiv1.Shard) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	shardItem := &ShardItem{
		Shard:  shard,
		Status: common.ProcessingInProgress,
	}
	p.cache[key] = shardItem
}

func (p *shardCache) DeleteFromShardClusterCache(key string, shard *admiralapiv1.Shard) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	shardItem := p.cache[key]

	if shardItem != nil {
		if shardItem.Shard != nil && shardItem.Shard.Name == shard.Name {
			log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Delete", "Shard",
				shard.Name, shard.Namespace, "", "ignoring shard and deleting from cache")
			delete(p.cache, key)
		} else {
			log.Warnf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Get", "Shard",
				shard.Name, shard.Namespace, "", "ignoring shard delete as it doesn't match the one in cache")
		}
	} else {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Delete", "Shard",
			shard.Name, shard.Namespace, "", "nothing to delete, shard not found in cache")
	}
}

func NewShardController(stopCh <-chan struct{}, handler ShardHandler, configPath string, namespace string, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*ShardController, error) {
	shardController := ShardController{
		K8sClient:    nil,
		CrdClient:    nil,
		Cache:        newShardCache(),
		informer:     nil,
		mutex:        sync.Mutex{},
		ShardHandler: handler,
	}
	var err error
	shardController.K8sClient, err = clientLoader.LoadKubeClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create shard controller k8s client: %v", err)
	}
	shardController.CrdClient, err = clientLoader.LoadAdmiralApiClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create shard controller crd client: %v", err)
	}
	labelOptions := informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
		opIdLabel, opIdValue := common.GetOperatorIdentityLabelKeyValueSet()
		shardIdLabel, shardIdValue := common.GetShardIdentityLabelKeyValueSet()
		opts.LabelSelector = fmt.Sprintf("%s=%s, %s=%s", opIdLabel, opIdValue, shardIdLabel, shardIdValue)
	})
	informerFactory := informers.NewSharedInformerFactoryWithOptions(shardController.K8sClient, resyncPeriod, labelOptions)
	informerFactory.Start(stopCh)
	shardController.informer = v1.NewShardInformer(shardController.CrdClient,
		namespace,
		resyncPeriod,
		cache.Indexers{})
	NewController("shard-ctrl", "", stopCh, &shardController, shardController.informer)
	return &shardController, nil
}

func (d *ShardController) Added(ctx context.Context, obj interface{}) error {
	return HandleAddUpdateShard(ctx, obj, d)
}

func (d *ShardController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	return HandleAddUpdateShard(ctx, obj, d)
}

func (d *ShardController) GetProcessItemStatus(obj interface{}) (string, error) {
	shard, ok := obj.(*admiralapiv1.Shard)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *admiralapiv1.Shard", obj)
	}
	return d.Cache.GetShardProcessStatus(shard), nil
}

func (d *ShardController) UpdateProcessItemStatus(obj interface{}, status string) error {
	shard, ok := obj.(*admiralapiv1.Shard)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *admiralapiv1.Shard", obj)
	}
	return d.Cache.UpdateShardProcessStatus(shard, status)
}

func HandleAddUpdateShard(ctx context.Context, obj interface{}, d *ShardController) error {
	shard, ok := obj.(*admiralapiv1.Shard)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *admiralapiv1.Shard", obj)
	}
	key := d.Cache.getKey(shard)
	defer util.LogElapsedTime("HandleAddUpdateShard", key, shard.Name+"_"+shard.Namespace, "")()
	if len(key) > 0 {
		d.Cache.UpdateShardToClusterCache(key, shard)
	}
	err := d.ShardHandler.Added(ctx, shard)
	return err
}

func (d *ShardController) Deleted(ctx context.Context, obj interface{}) error {
	shard, ok := obj.(*admiralapiv1.Shard)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *admiralapiv1.Shard", obj)
	}
	key := d.Cache.getKey(shard)
	var err error
	if err == nil && len(key) > 0 {
		d.Cache.DeleteFromShardClusterCache(key, shard)
	}
	return d.ShardHandler.Deleted(ctx, shard)
}

func (d *ShardController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	shard, ok := obj.(*admiralapiv1.Shard)
	if !ok {
		return
	}
	if d.K8sClient != nil {
		ns, err := d.K8sClient.CoreV1().Namespaces().Get(context.Background(), shard.Namespace, metav1.GetOptions{})
		if err != nil {
			log.Warnf("failed to get namespace object for shard with namespace %v, err: %v", shard.Namespace, err)
		} else if (ns != nil && ns.Annotations[common.AdmiralIgnoreAnnotation] == "true") || shard.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
			log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.ShardResourceType,
				shard.Name, shard.Namespace, "", "Value=true")
		}
	}
}

func (d *ShardController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	shard, ok := obj.(*admiralapiv1.Shard)
	if ok && isRetry {
		return d.Cache.Get(shard.Name), nil
	}
	if ok && d.CrdClient != nil {
		return d.CrdClient.AdmiralV1().Shards(shard.Namespace).Get(ctx, shard.Name, metav1.GetOptions{})
	}
	return nil, fmt.Errorf("kubernetes client is not initialized, txId=%s", ctx.Value("txId"))
}
