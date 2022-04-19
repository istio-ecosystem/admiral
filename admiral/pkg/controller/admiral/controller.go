package admiral

import (
	"fmt"
	log "github.com/sirupsen/logrus"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"time"
)

const (
	maxRetries = 5
)

// Handler interface contains the methods that are required
type Delegator interface {
	Added(interface{})
	Updated(interface{}, interface{})
	Deleted(interface{})
}

type EventType string

const (
	Add    EventType = "Add"
	Update EventType = "Update"
	Delete EventType = "Delete"
)

type InformerCacheObj struct {
	key       string
	eventType EventType
	obj       interface{}
	oldObj    interface{}
}

type Controller struct {
	name      string
	delegator Delegator
	queue     workqueue.RateLimitingInterface
	informer  cache.SharedIndexInformer
}

func NewController(name string, stopCh <-chan struct{}, delegator Delegator, informer cache.SharedIndexInformer) Controller {

	controller := Controller{
		name: name,
		informer:  informer,
		delegator: delegator,
		queue:     workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}

	controller.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				log.Infof("Informer Add controller=%v obj=%v", controller.name, key)
				controller.queue.Add(InformerCacheObj{key: key, eventType: Add, obj: obj})
			}

		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				log.Infof("Informer Update controller=%v obj=%v", controller.name, key)
				controller.queue.Add(InformerCacheObj{key: key, eventType: Update, obj: newObj, oldObj: oldObj})
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				log.Infof("Informer Delete controller=%v obj=%v", controller.name, key)
				controller.queue.Add(InformerCacheObj{key: key, eventType: Delete, obj: obj})
			}
		},
	})

	go controller.Run(stopCh)

	return controller
}

// Run starts the controller until it receives a message over stopCh
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	log.Infof("Starting controller=%v", c.name)

	go c.informer.Run(stopCh)

	// Wait for the caches to be synced before starting workers
	log.Infof(" Waiting for informer caches to sync for controller=%v", c.name)
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		utilruntime.HandleError(fmt.Errorf(" timed out waiting for caches to sync for controller=%v", c.name))
		return
	}

	log.Infof("Informer caches synced for controller=%v, current keys=%v", c.name, c.informer.GetStore().ListKeys())
	wait.Until(c.runWorker, 5*time.Second, stopCh)
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
		// continue looping
	}
}

func (c *Controller) processNextItem() bool {
	item, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(item)

	err := c.processItem(item.(InformerCacheObj))
	if err == nil {
		// No error, reset the ratelimit counters
		c.queue.Forget(item)
	} else if c.queue.NumRequeues(item) < maxRetries {
		log.Errorf("Error processing %s (will retry): %v", item, err)
		c.queue.AddRateLimited(item)
	} else {
		log.Errorf("Error processing %s (giving up): %v", item, err)
		c.queue.Forget(item)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) processItem(informerCacheObj InformerCacheObj) error {

	if informerCacheObj.eventType == Delete {
		c.delegator.Deleted(informerCacheObj.obj)
	} else if informerCacheObj.eventType == Update {
		c.delegator.Updated(informerCacheObj.obj, informerCacheObj.oldObj)
	} else if informerCacheObj.eventType == Add {
		c.delegator.Added(informerCacheObj.obj)
	}
	return nil
}
