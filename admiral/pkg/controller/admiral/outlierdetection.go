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
	"github.com/prometheus/common/log"
	"github.com/sirupsen/logrus"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type odItems struct {
	OutlierDetection *v1.OutlierDetection
	Status           string
}

type odCache struct {
	cache map[string]map[string]map[string]*odItems
	mutex *sync.RWMutex
}

type OutlierDetectionController struct {
	cache     *odCache
	informer  cache.SharedIndexInformer
	handler   OutlierDetectionControllerHandler
	crdclient clientset.Interface
}

func (c *odCache) Put(od *v1.OutlierDetection) {

	defer c.mutex.Unlock()
	c.mutex.Lock()

	key := common.ConstructKeyWithEnvAndIdentity(common.GetODEnv(od), common.GetODIdentity(od))

	namespaceWithOds := c.cache[key]

	if namespaceWithOds == nil {
		namespaceWithOds = make(map[string]map[string]*odItems)
	}

	namespaceOds := namespaceWithOds[od.Namespace]

	if namespaceOds == nil {
		namespaceOds = make(map[string]*odItems)
	}

	if common.ShouldIgnoreResource(od.ObjectMeta) {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.OutlierDetection,
			od.Name, od.Namespace, "", "Value=true")
		delete(namespaceWithOds, od.Name)
	} else {
		namespaceOds[od.Name] = &odItems{
			OutlierDetection: od,
			Status:           common.ProcessingInProgress,
		}
	}

	namespaceWithOds[od.Namespace] = namespaceOds
	c.cache[key] = namespaceWithOds

	logrus.Infof("OutlierDetection cache for key%s, gtp=%v", key, namespaceWithOds)

}

func (c *odCache) Delete(od *v1.OutlierDetection) {
	defer c.mutex.Unlock()
	c.mutex.Lock()

	key := common.ConstructKeyWithEnvAndIdentity(common.GetODEnv(od), common.GetODIdentity(od))

	namespaceWithOds := c.cache[key]
	if namespaceWithOds == nil {
		return
	}

	namespaceOd := namespaceWithOds[od.Namespace]
	if namespaceOd == nil {
		return
	}

	delete(namespaceOd, od.Name)
	namespaceWithOds[od.Namespace] = namespaceOd
	c.cache[key] = namespaceWithOds

}

func (c *odCache) Get(key, namespace string) []*v1.OutlierDetection {
	defer c.mutex.Unlock()
	c.mutex.Lock()
	namespaceWithOd := c.cache[key]
	result := make([]*v1.OutlierDetection, 0)

	for ns, ods := range namespaceWithOd {
		if namespace == ns {
			for _, i := range ods {
				result = append(result, i.OutlierDetection.DeepCopy())
			}
		}
	}
	return result
}

func (c *odCache) UpdateODProcessingStatus(od *v1.OutlierDetection, status string) error {
	defer c.mutex.Unlock()
	c.mutex.Lock()

	key := common.ConstructKeyWithEnvAndIdentity(common.GetODEnv(od), common.GetODIdentity(od))

	namespaceWithOds, ok := c.cache[key]
	if ok {
		namespaceOds, ok := namespaceWithOds[od.Namespace]
		if ok {
			nameOd, ok := namespaceOds[od.Name]
			if ok {
				nameOd.Status = status
				c.cache[key] = namespaceWithOds
				return nil
			}
		}
	}

	return fmt.Errorf(LogCacheFormat, common.Update, common.OutlierDetection,
		od.Name, od.Namespace, "", "nothing to update, "+common.OutlierDetection+" not found in cache")
}

func (c *odCache) GetODProcessStatus(od *v1.OutlierDetection) string {

	defer c.mutex.Unlock()
	c.mutex.Lock()

	key := common.ConstructKeyWithEnvAndIdentity(common.GetODEnv(od), common.GetODIdentity(od))

	namespaceWithOds, ok := c.cache[key]
	if ok {
		namespaceOds, ok := namespaceWithOds[od.Namespace]
		if ok {
			nameOd, ok := namespaceOds[od.Name]
			if ok {
				return nameOd.Status
			}
		}
	}

	return common.NotProcessed
}

func (c odCache) UpdateProcessingStatus(od *v1.OutlierDetection, status string) error {
	defer c.mutex.Unlock()
	c.mutex.Lock()

	key := common.ConstructKeyWithEnvAndIdentity(common.GetODEnv(od), common.GetODIdentity(od))

	namespaceWithOds, ok := c.cache[key]
	if ok {
		namespaceOds, ok := namespaceWithOds[od.Namespace]
		if ok {
			nameOd, ok := namespaceOds[od.Name]
			if ok {
				nameOd.Status = status
				c.cache[key] = namespaceWithOds
				return nil
			}
		}
	}

	return fmt.Errorf(LogCacheFormat, common.Update, common.OutlierDetection,
		od.Name, od.Namespace, "", "nothing to update, "+common.OutlierDetection+" not found in cache")
}

func (c odCache) GetProcessStatus(od *v1.OutlierDetection) string {

	defer c.mutex.Unlock()
	c.mutex.Lock()

	key := common.ConstructKeyWithEnvAndIdentity(common.GetODEnv(od), common.GetODIdentity(od))

	namespaceWithOds, ok := c.cache[key]
	if ok {
		namespaceOds, ok := namespaceWithOds[od.Namespace]
		if ok {
			nameOd, ok := namespaceOds[od.Name]
			if ok {
				return nameOd.Status
			}
		}
	}

	return common.NotProcessed
}

func (o *OutlierDetectionController) Added(ctx context.Context, i interface{}) error {
	od, ok := i.(*v1.OutlierDetection)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.OutlierDetection", i)
	}
	o.cache.Put(od)
	return o.handler.Added(ctx, od)
}

func (o *OutlierDetectionController) Updated(ctx context.Context, i interface{}, i2 interface{}) error {
	od, ok := i.(*v1.OutlierDetection)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.OutlierDetection", i)
	}
	o.cache.Put(od)
	return o.handler.Added(ctx, od)
}

func (o *OutlierDetectionController) Deleted(ctx context.Context, i interface{}) error {
	od, ok := i.(*v1.OutlierDetection)
	if !ok {
		//Validate if object is stale
		//Ref - https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/replicaset/replica_set.go#L356-L371
		staleObj, ok := i.(cache.DeletedFinalStateUnknown)
		if !ok {
			return fmt.Errorf("type assertion failed, %v is not of type *v1.OutlierDetection (Stale)", i)
		}
		od, ok = staleObj.Obj.(*v1.OutlierDetection)
		if !ok {
			return fmt.Errorf("type assertion failed, %v is not of type *v1.OutlierDetection", i)
		}
	}
	o.cache.Delete(od)
	return o.handler.Deleted(ctx, od)
}

func (o *OutlierDetectionController) UpdateProcessItemStatus(i interface{}, status string) error {
	od, ok := i.(*v1.OutlierDetection)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.OutlierDetection", i)
	}
	return o.cache.UpdateProcessingStatus(od, status)
}

func (o *OutlierDetectionController) GetProcessItemStatus(i interface{}) (string, error) {
	od, ok := i.(*v1.OutlierDetection)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1.OutlierDetection", i)
	}
	return o.cache.GetODProcessStatus(od), nil
}

func (o *OutlierDetectionController) GetCache() *odCache {
	return o.cache
}

type OutlierDetectionControllerHandler interface {
	Added(ctx context.Context, obj *v1.OutlierDetection) error
	Updated(ctx context.Context, obj *v1.OutlierDetection) error
	Deleted(ctx context.Context, obj *v1.OutlierDetection) error
}

func NewOutlierDetectionController(stopCh <-chan struct{}, handler OutlierDetectionControllerHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*OutlierDetectionController, error) {
	outlierDetectionController := OutlierDetectionController{}
	outlierDetectionController.handler = handler

	odCache := odCache{}
	odCache.cache = make(map[string]map[string]map[string]*odItems)
	odCache.mutex = &sync.RWMutex{}

	outlierDetectionController.cache = &odCache

	var err error

	outlierDetectionController.crdclient, err = clientLoader.LoadAdmiralClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create outlier detection controller crd client: %v", err)
	}

	outlierDetectionController.informer = informerV1.NewOutlierDetectionInformer(outlierDetectionController.crdclient, meta_v1.NamespaceAll,
		resyncPeriod, cache.Indexers{})

	NewController("od-ctrl", config.Host, stopCh, &outlierDetectionController, outlierDetectionController.informer)

	return &outlierDetectionController, nil

}

func (o *OutlierDetectionController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	od, ok := obj.(*v1.OutlierDetection)
	if !ok {
		return
	}
	metadata := od.ObjectMeta
	if metadata.Annotations[common.AdmiralIgnoreAnnotation] == "true" || metadata.Labels[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.OutlierDetection,
			od.Name, od.Namespace, "", "Value=true")
	}
}

func (o *OutlierDetectionController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	od, ok := obj.(*v1.OutlierDetection)
	if ok && o.crdclient != nil {
		return o.crdclient.AdmiralV1alpha1().OutlierDetections(od.Namespace).Get(ctx, od.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("crd client is not initialized, txId=%s", ctx.Value("txId"))
}
