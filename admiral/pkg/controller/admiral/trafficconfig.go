package admiral

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/informers/externalversions/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sync"
	"time"
)

// TrafficConfigHandler interface contains the methods that are required
type TrafficConfigHandler interface {
	Added(ctx context.Context, obj *v1alpha1.TrafficConfig) error
	Updated(ctx context.Context, obj *v1alpha1.TrafficConfig) error
	Deleted(ctx context.Context, obj *v1alpha1.TrafficConfig) error
}

type TrafficConfigController struct {
	K8sClient            kubernetes.Interface
	CrdClient            clientset.Interface
	TrafficConfigHandler TrafficConfigHandler
	informer             cache.SharedIndexInformer
	labelSet             *common.LabelSet
	mutex                sync.Mutex
	Cache                *trafficConfigCache
}

func (c *TrafficConfigController) DoesGenerationMatch(*log.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

func (c *TrafficConfigController) IsOnlyReplicaCountChanged(*log.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

type TrafficConfigItem struct {
	trafficConfig *v1alpha1.TrafficConfig
	status        string
}

type trafficConfigCache struct {
	//map of dependencies key=identity value array of onboarded identitys
	cache map[string]*TrafficConfigItem
	mutex *sync.Mutex
}

func (tc *trafficConfigCache) Put(trafficConfig *v1alpha1.TrafficConfig) {
	defer tc.mutex.Unlock()
	tc.mutex.Lock()

	key := tc.getKey(trafficConfig)
	tc.cache[key] = &TrafficConfigItem{
		trafficConfig: trafficConfig,
		status:        common.ProcessingInProgress,
	}
}

func (tc *trafficConfigCache) getKey(trafficConfig *v1alpha1.TrafficConfig) string {
	return trafficConfig.Name
}

func (tc *trafficConfigCache) Get(identity string) *v1alpha1.TrafficConfig {
	defer tc.mutex.Unlock()
	tc.mutex.Lock()

	tcItem, ok := tc.cache[identity]
	if ok {
		return tcItem.trafficConfig
	}

	return nil
}

func (tc *trafficConfigCache) Delete(trafficConfig *v1alpha1.TrafficConfig) {
	defer tc.mutex.Unlock()
	tc.mutex.Lock()
	delete(tc.cache, tc.getKey(trafficConfig))
}

func (tc *trafficConfigCache) GetTrafficConfigProcessStatus(trafficConfig *v1alpha1.TrafficConfig) string {
	defer tc.mutex.Unlock()
	tc.mutex.Lock()

	key := tc.getKey(trafficConfig)

	tcItem, ok := tc.cache[key]
	if ok {
		return tcItem.status
	}

	return common.NotProcessed
}

func (tc *trafficConfigCache) UpdateTrafficConfigProcessStatus(trafficConfig *v1alpha1.TrafficConfig, status string) error {
	defer tc.mutex.Unlock()
	tc.mutex.Lock()

	key := tc.getKey(trafficConfig)

	tcItem, ok := tc.cache[key]
	if ok {
		tcItem.status = status
		tc.cache[key] = tcItem
		return nil
	}

	return fmt.Errorf(LogCacheFormat, "Update", "TrafficConfig",
		trafficConfig.Name, trafficConfig.Namespace, "", "nothing to update, traffic config not found in cache")
}

func NewTrafficConfigController(stopCh <-chan struct{}, handler TrafficConfigHandler, configPath string, namespace string, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*TrafficConfigController, error) {
	trafficConfigController := TrafficConfigController{}
	trafficConfigController.TrafficConfigHandler = handler

	tcCache := trafficConfigCache{}
	tcCache.cache = make(map[string]*TrafficConfigItem)
	tcCache.mutex = &sync.Mutex{}

	trafficConfigController.Cache = &tcCache

	trafficConfigController.mutex = sync.Mutex{}

	var err error

	trafficConfigController.K8sClient, err = clientLoader.LoadKubeClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create traffic config controller k8s client: %v", err)
	}

	trafficConfigController.CrdClient, err = clientLoader.LoadAdmiralClientFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create traffic config controller crd client: %v", err)

	}

	trafficConfigController.informer = informerV1.NewTrafficConfigInformer(
		trafficConfigController.CrdClient,
		namespace,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController("trafficconfig-ctrl-"+namespace, "", stopCh, &trafficConfigController, trafficConfigController.informer)

	return &trafficConfigController, nil
}

func (c *TrafficConfigController) Added(ctx context.Context, obj interface{}) error {
	trafficConfig, ok := obj.(*v1alpha1.TrafficConfig)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha1.TrafficConfig", obj)
	}

	c.Cache.Put(trafficConfig)
	return c.TrafficConfigHandler.Added(ctx, trafficConfig)
}

func (c *TrafficConfigController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	trafficConfig, ok := obj.(*v1alpha1.TrafficConfig)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha1.TrafficConfig", obj)
	}

	c.Cache.Put(trafficConfig)
	return c.TrafficConfigHandler.Updated(ctx, trafficConfig)
}

func (c *TrafficConfigController) Deleted(ctx context.Context, obj interface{}) error {
	trafficConfig, ok := obj.(*v1alpha1.TrafficConfig)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha1.TrafficConfig", obj)
	}
	c.Cache.Delete(trafficConfig)
	return c.TrafficConfigHandler.Deleted(ctx, trafficConfig)
}

func (c *TrafficConfigController) GetProcessItemStatus(obj interface{}) (string, error) {
	trafficConfig, ok := obj.(*v1alpha1.TrafficConfig)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1alpha1.TrafficConfig", obj)
	}
	return c.Cache.GetTrafficConfigProcessStatus(trafficConfig), nil
}

func (c *TrafficConfigController) UpdateProcessItemStatus(obj interface{}, status string) error {
	trafficConfig, ok := obj.(*v1alpha1.TrafficConfig)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha1.TrafficConfig", obj)
	}
	return c.Cache.UpdateTrafficConfigProcessStatus(trafficConfig, status)
}

func (c *TrafficConfigController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	trafficconfig, ok := obj.(*v1alpha1.TrafficConfig)
	if !ok {
		return
	}
	metadata := trafficconfig.ObjectMeta
	if metadata.Annotations[common.AdmiralIgnoreAnnotation] == "true" || metadata.Labels[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s",
			"admiralIoIgnoreAnnotationCheck", common.ClientConnectionConfig,
			trafficconfig.Name, trafficconfig.Namespace, "", "Value=true")
	}
}

func (c *TrafficConfigController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	trafficConfig, ok := obj.(*v1alpha1.TrafficConfig)
	if ok && isRetry {
		return c.Cache.Get(trafficConfig.Name), nil
	}
	if ok && c.CrdClient != nil {
		return c.CrdClient.AdmiralV1alpha1().TrafficConfigs(trafficConfig.Namespace).Get(ctx, trafficConfig.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("crd client is not initialized, txId=%s", ctx.Value("txId"))
}
