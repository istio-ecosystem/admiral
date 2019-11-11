package admiral

import (
	"fmt"
	"github.com/sirupsen/logrus"

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
	Deleted(interface{})
}

type Controller struct {
	delegator Delegator
	queue     workqueue.RateLimitingInterface
	informer  cache.SharedIndexInformer
}

func NewController(stopCh <-chan struct{}, delegator Delegator, informer cache.SharedIndexInformer) Controller {

	controller := Controller{
		informer:  informer,
		delegator: delegator,
		queue:     workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}

	controller.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			logrus.Debugf("Informer: add : %v", obj)
			key, err := cache.MetaNamespaceKeyFunc(obj)

			if err == nil {
				controller.queue.Add(key)
			}

		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			logrus.Debugf("Informer Update: %v", newObj)
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				controller.queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			logrus.Debugf("Informer Delete: %v", obj)
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				controller.queue.Add(key)
			}
		},
	})

	go controller.Run(stopCh)

	return controller
}

// Run starts the controller until it receves a message over stopCh
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	logrus.Info("Starting controller")

	go c.informer.Run(stopCh)

	// Wait for the caches to be synced before starting workers
	logrus.Info(" Waiting for informer caches to sync")
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		utilruntime.HandleError(fmt.Errorf(" timed out waiting for caches to sync"))
		return
	}

	logrus.Info("informer caches synced")
	wait.Until(c.runWorker, 5*time.Second, stopCh)
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
		// continue looping
	}
}

func (c *Controller) processNextItem() bool {
	depName, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(depName)

	err := c.processItem(depName.(string))
	if err == nil {
		// No error, reset the ratelimit counters
		c.queue.Forget(depName)
	} else if c.queue.NumRequeues(depName) < maxRetries {
		logrus.Errorf("Error processing %s (will retry): %v", depName, err)
		c.queue.AddRateLimited(depName)
	} else {
		logrus.Errorf("Error processing %s (giving up): %v", depName, err)
		c.queue.Forget(depName)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) processItem(name string) error {
	obj, exists, err := c.informer.GetIndexer().GetByKey(name)
	if err != nil {
		return fmt.Errorf("controller: error fetching object %s error: %v", name, err)
	}

	if exists {
		c.delegator.Added(obj)
	} else {
		c.delegator.Deleted(obj)
	}

	return nil
}
