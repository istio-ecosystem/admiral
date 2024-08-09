package admiral

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/informers/externalversions/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type ClientConnectionConfigHandlerInterface interface {
	Added(ctx context.Context, obj *v1.ClientConnectionConfig) error
	Updated(ctx context.Context, obj *v1.ClientConnectionConfig) error
	Deleted(ctx context.Context, obj *v1.ClientConnectionConfig) error
}

type ClientConnectionConfigController struct {
	crdClient                       clientset.Interface
	informer                        cache.SharedIndexInformer
	clientConnectionSettingsHandler ClientConnectionConfigHandlerInterface
	Cache                           *clientConnectionSettingsCache
}

type clientConnectionSettingsItem struct {
	clientConnectionSettings *v1.ClientConnectionConfig
	status                   string
}

type clientConnectionSettingsCache struct {
	cache map[string]map[string]map[string]*clientConnectionSettingsItem
	mutex *sync.RWMutex
}

func (c *clientConnectionSettingsCache) Get(key, namespace string) []*v1.ClientConnectionConfig {
	defer c.mutex.RUnlock()
	c.mutex.RLock()
	namespacesWithClientConnectionConfig := c.cache[key]
	matchedClientConnectionConfig := make([]*v1.ClientConnectionConfig, 0)
	for ns, clientConnectionSettingsItem := range namespacesWithClientConnectionConfig {
		if namespace != ns {
			continue
		}
		for _, item := range clientConnectionSettingsItem {
			matchedClientConnectionConfig = append(matchedClientConnectionConfig, item.clientConnectionSettings.DeepCopy())
		}
	}
	return matchedClientConnectionConfig
}

func (c *clientConnectionSettingsCache) Put(clientConnectionSettings *v1.ClientConnectionConfig) {
	defer c.mutex.Unlock()
	c.mutex.Lock()
	key := common.ConstructKeyWithEnvAndIdentity(common.GetClientConnectionConfigEnv(clientConnectionSettings),
		common.GetClientConnectionConfigIdentity(clientConnectionSettings))
	namespacesWithClientConnectionConfig := c.cache[key]
	if namespacesWithClientConnectionConfig == nil {
		namespacesWithClientConnectionConfig = make(map[string]map[string]*clientConnectionSettingsItem)
	}
	namespaces := namespacesWithClientConnectionConfig[clientConnectionSettings.Namespace]
	if namespaces == nil {
		namespaces = make(map[string]*clientConnectionSettingsItem)
	}
	if common.ShouldIgnoreResource(clientConnectionSettings.ObjectMeta) {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s",
			"admiralIoIgnoreAnnotationCheck", common.ClientConnectionConfig,
			clientConnectionSettings.Name, clientConnectionSettings.Namespace, "", "Value=true")
		delete(namespaces, clientConnectionSettings.Name)
	} else {
		namespaces[clientConnectionSettings.Name] = &clientConnectionSettingsItem{
			clientConnectionSettings: clientConnectionSettings,
			status:                   common.ProcessingInProgress,
		}
	}

	namespacesWithClientConnectionConfig[clientConnectionSettings.Namespace] = namespaces
	c.cache[key] = namespacesWithClientConnectionConfig

	logrus.Infof("%s cache for key=%s gtp=%v", common.ClientConnectionConfig, key, namespacesWithClientConnectionConfig)
}

func (c *clientConnectionSettingsCache) Delete(clientConnectionSettings *v1.ClientConnectionConfig) {
	defer c.mutex.Unlock()
	c.mutex.Lock()
	key := common.ConstructKeyWithEnvAndIdentity(common.GetClientConnectionConfigEnv(clientConnectionSettings),
		common.GetClientConnectionConfigIdentity(clientConnectionSettings))
	namespacesWithClientConnectionConfig := c.cache[key]
	if namespacesWithClientConnectionConfig == nil {
		return
	}
	namespaces := namespacesWithClientConnectionConfig[clientConnectionSettings.Namespace]
	if namespaces == nil {
		return
	}
	delete(namespaces, clientConnectionSettings.Name)
	namespacesWithClientConnectionConfig[clientConnectionSettings.Namespace] = namespaces
	c.cache[key] = namespacesWithClientConnectionConfig
}

func (c *clientConnectionSettingsCache) GetStatus(clientConnectionSettings *v1.ClientConnectionConfig) string {
	defer c.mutex.RUnlock()
	c.mutex.RLock()

	key := common.ConstructKeyWithEnvAndIdentity(common.GetClientConnectionConfigEnv(clientConnectionSettings),
		common.GetClientConnectionConfigIdentity(clientConnectionSettings))

	namespacesWithClientConnectionConfig, ok := c.cache[key]
	if !ok {
		return common.NotProcessed
	}
	namespaces, ok := namespacesWithClientConnectionConfig[clientConnectionSettings.Namespace]
	if !ok {
		return common.NotProcessed
	}
	cachedClientConnectionConfig, ok := namespaces[clientConnectionSettings.Name]
	if !ok {
		return common.NotProcessed
	}

	return cachedClientConnectionConfig.status
}

func (c *clientConnectionSettingsCache) UpdateStatus(
	clientConnectionSettings *v1.ClientConnectionConfig, status string) error {
	defer c.mutex.Unlock()
	c.mutex.Lock()

	key := common.ConstructKeyWithEnvAndIdentity(common.GetClientConnectionConfigEnv(clientConnectionSettings),
		common.GetClientConnectionConfigIdentity(clientConnectionSettings))

	namespacesWithClientConnectionConfig, ok := c.cache[key]
	if !ok {
		return fmt.Errorf(LogCacheFormat, common.Update, common.ClientConnectionConfig,
			clientConnectionSettings.Name, clientConnectionSettings.Namespace,
			"", "skipped updating status in cache, clientConnectionSettings not found in cache")
	}
	namespaces, ok := namespacesWithClientConnectionConfig[clientConnectionSettings.Namespace]
	if !ok {
		return fmt.Errorf(LogCacheFormat, common.Update, common.ClientConnectionConfig,
			clientConnectionSettings.Name, clientConnectionSettings.Namespace,
			"", "skipped updating status in cache, clientConnectionSettings namespace not found in cache")
	}
	cachedClientConnectionConfig, ok := namespaces[clientConnectionSettings.Name]
	if !ok {
		return fmt.Errorf(LogCacheFormat, common.Update, common.ClientConnectionConfig,
			clientConnectionSettings.Name, clientConnectionSettings.Namespace,
			"", "skipped updating status in cache, clientConnectionSettings not found in cache with the specified name")
	}
	cachedClientConnectionConfig.status = status
	c.cache[key] = namespacesWithClientConnectionConfig
	return nil
}

func (c *ClientConnectionConfigController) Added(ctx context.Context, obj interface{}) error {
	clientConnectionSettings, ok := obj.(*v1.ClientConnectionConfig)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.ClientConnectionConfig", obj)
	}
	c.Cache.Put(clientConnectionSettings)
	return c.clientConnectionSettingsHandler.Added(ctx, clientConnectionSettings)
}

func (c *ClientConnectionConfigController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	clientConnectionSettings, ok := obj.(*v1.ClientConnectionConfig)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.ClientConnectionConfig", obj)
	}
	c.Cache.Put(clientConnectionSettings)
	return c.clientConnectionSettingsHandler.Updated(ctx, clientConnectionSettings)
}

func (c *ClientConnectionConfigController) Deleted(ctx context.Context, obj interface{}) error {
	clientConnectionSettings, ok := obj.(*v1.ClientConnectionConfig)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.ClientConnectionConfig", obj)
	}
	c.Cache.Delete(clientConnectionSettings)
	return c.clientConnectionSettingsHandler.Deleted(ctx, clientConnectionSettings)
}

func (c *ClientConnectionConfigController) UpdateProcessItemStatus(obj interface{}, status string) error {
	clientConnectionSettings, ok := obj.(*v1.ClientConnectionConfig)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.ClientConnectionConfig", obj)
	}
	return c.Cache.UpdateStatus(clientConnectionSettings, status)
}

func (c *ClientConnectionConfigController) GetProcessItemStatus(obj interface{}) (string, error) {
	clientConnectionSettings, ok := obj.(*v1.ClientConnectionConfig)
	if !ok {
		return common.NotProcessed,
			fmt.Errorf("type assertion failed, %v is not of type *v1.ClientConnectionConfig", obj)
	}
	return c.Cache.GetStatus(clientConnectionSettings), nil
}

func (c *ClientConnectionConfigController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	clientConnectionSettings, ok := obj.(*v1.ClientConnectionConfig)
	if !ok {
		return
	}
	metadata := clientConnectionSettings.ObjectMeta
	if metadata.Annotations[common.AdmiralIgnoreAnnotation] == "true" || metadata.Labels[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s",
			"admiralIoIgnoreAnnotationCheck", common.ClientConnectionConfig,
			clientConnectionSettings.Name, clientConnectionSettings.Namespace, "", "Value=true")
	}
}

func (c *ClientConnectionConfigController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	clientConnectionSettings, ok := obj.(*v1.ClientConnectionConfig)
	if !ok {
		return nil, fmt.Errorf("type assertion failed, %v is not of type *v1.ClientConnectionConfig", obj)
	}
	if c.crdClient == nil {
		return nil, fmt.Errorf("crd client is not initialized, txId=%s", ctx.Value("txId"))
	}
	return c.crdClient.AdmiralV1alpha1().
		ClientConnectionConfigs(clientConnectionSettings.Namespace).
		Get(ctx, clientConnectionSettings.Name, meta_v1.GetOptions{})
}

// NewClientConnectionConfigController creates a new instance of ClientConnectionConfigController
func NewClientConnectionConfigController(stopCh <-chan struct{}, handler ClientConnectionConfigHandlerInterface,
	config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*ClientConnectionConfigController, error) {

	crdClient, err := clientLoader.LoadAdmiralClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientconnectionsettings controller crd client: %w", err)
	}

	clientConnectionCache := &clientConnectionSettingsCache{}
	clientConnectionCache.cache = make(map[string]map[string]map[string]*clientConnectionSettingsItem)
	clientConnectionCache.mutex = &sync.RWMutex{}

	clientConnectionSettingsController := ClientConnectionConfigController{
		clientConnectionSettingsHandler: handler,
		crdClient:                       crdClient,
		Cache:                           clientConnectionCache,
	}

	clientConnectionSettingsController.informer = informerV1.NewClientConnectionConfigInformer(
		crdClient,
		meta_v1.NamespaceAll,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController("clientconnectionsettings-ctrl", config.Host, stopCh,
		&clientConnectionSettingsController, clientConnectionSettingsController.informer)

	return &clientConnectionSettingsController, nil
}
