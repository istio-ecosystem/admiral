// Copyright 2018 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secret

import (
	"context"
	"errors"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret/resolver"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
	"time"

	corev1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

const (
	filterLabel = "admiral/sync"
	maxRetries  = 5
)

// LoadKubeConfig is a unit test override variable for loading the k8s config.
// DO NOT USE - TEST ONLY.
var LoadKubeConfig = clientcmd.Load

var remoteClustersMetric common.Gauge

// addSecretCallback prototype for the add secret callback function.
type addSecretCallback func(config *rest.Config, dataKey string, resyncPeriod time.Duration) error

// updateSecretCallback prototype for the update secret callback function.
type updateSecretCallback func(config *rest.Config, dataKey string, resyncPeriod time.Duration) error

// removeSecretCallback prototype for the remove secret callback function.
type removeSecretCallback func(dataKey string) error

// Controller is the controller implementation for Secret resources
type Controller struct {
	kubeclientset  kubernetes.Interface
	namespace      string
	Cs             *ClusterStore
	queue          workqueue.RateLimitingInterface
	informer       cache.SharedIndexInformer
	addCallback    addSecretCallback
	updateCallback updateSecretCallback
	removeCallback removeSecretCallback
	secretResolver resolver.SecretResolver
}

// RemoteCluster defines cluster structZZ
type RemoteCluster struct {
	secretName string
}

// ClusterStore is a collection of clusters
type ClusterStore struct {
	RemoteClusters map[string]*RemoteCluster
}

// newClustersStore initializes data struct to store clusters information
func newClustersStore() *ClusterStore {
	remoteClusters := make(map[string]*RemoteCluster)
	return &ClusterStore{
		RemoteClusters: remoteClusters,
	}
}

// NewController returns a new secret controller
func NewController(
	kubeclientset kubernetes.Interface,
	namespace string,
	cs *ClusterStore,
	addCallback addSecretCallback,
	updateCallback updateSecretCallback,
	removeCallback removeSecretCallback,
	secretResolverType string) *Controller {

	secretsInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(opts meta_v1.ListOptions) (runtime.Object, error) {
				opts.LabelSelector = filterLabel + "=true"
				return kubeclientset.CoreV1().Secrets(namespace).List(opts)
			},
			WatchFunc: func(opts meta_v1.ListOptions) (watch.Interface, error) {
				opts.LabelSelector = filterLabel + "=true"
				return kubeclientset.CoreV1().Secrets(namespace).Watch(opts)
			},
		},
		&corev1.Secret{}, 0, cache.Indexers{},
	)

	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	var secretResolver resolver.SecretResolver
	var err error
	if len(secretResolverType) == 0 {
		log.Info("Initializing default secret resolver")
		secretResolver, err = resolver.NewDefaultResolver()
	} else {
		err = fmt.Errorf("unrecognized secret resolver type %v specified", secretResolverType)
	}

	if err != nil {
		log.Errorf("Failed to initialize secret resolver: %v", err)
		return nil
	}

	controller := &Controller{
		kubeclientset:  kubeclientset,
		namespace:      namespace,
		Cs:             cs,
		informer:       secretsInformer,
		queue:          queue,
		addCallback:    addCallback,
		updateCallback: updateCallback,
		removeCallback: removeCallback,
		secretResolver: secretResolver,
	}

	log.Info("Setting up event handlers")
	secretsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			log.Infof("Processing cluster add: %s", key)
			if err == nil {
				queue.Add(key)
			}
		},
		UpdateFunc: func(oldObj interface{}, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			log.Infof("Processing cluster update: %s", key)
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			log.Infof("Processing cluster delete: %s", key)
			if err == nil {
				queue.Add(key)
			}
		},
	})

	remoteClustersMetric = common.NewGaugeFrom(common.ClustersMonitoredMetricName, "Gauge for the clusters monitored by Admiral")
	return controller
}

// Run starts the controller until it receives a message over stopCh
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	log.Info("Starting Secrets controller")

	go c.informer.Run(stopCh)

	// Wait for the caches to be synced before starting workers
	log.Info("Waiting for informer caches to sync")
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	log.Info("secret informer caches synced")
	wait.Until(c.runWorker, 5*time.Second, stopCh)
}

// StartSecretController creates the secret controller.
func StartSecretController(
	k8s kubernetes.Interface,
	addCallback addSecretCallback,
	updateCallback updateSecretCallback,
	removeCallback removeSecretCallback,
	namespace string,
	ctx context.Context,
	secretResolverType string) (*Controller, error) {

	clusterStore := newClustersStore()
	controller := NewController(k8s, namespace, clusterStore, addCallback, updateCallback, removeCallback, secretResolverType)

	go controller.Run(ctx.Done())

	return controller, nil
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
		// continue looping
	}
}

func (c *Controller) processNextItem() bool {
	secretName, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(secretName)

	err := c.processItem(secretName.(string))
	if err == nil {
		// No error, reset the ratelimit counters
		c.queue.Forget(secretName)
	} else if c.queue.NumRequeues(secretName) < maxRetries {
		log.Errorf("Error processing %s (will retry): %v", secretName, err)
		c.queue.AddRateLimited(secretName)
	} else {
		log.Errorf("Error processing %s (giving up): %v", secretName, err)
		c.queue.Forget(secretName)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) processItem(secretName string) error {
	obj, exists, err := c.informer.GetIndexer().GetByKey(secretName)
	if err != nil {
		return fmt.Errorf("error fetching object %s error: %v", secretName, err)
	}

	if exists {
		c.addMemberCluster(secretName, obj.(*corev1.Secret))
	} else {
		c.deleteMemberCluster(secretName)
	}

	return nil
}

func (c *Controller) createRemoteCluster(kubeConfig []byte, secretName string, clusterID string, namespace string) (*RemoteCluster, *rest.Config, error) {
	if len(kubeConfig) == 0 {
		log.Infof("Data '%s' in the secret %s in namespace %s is empty, and disregarded ",
			clusterID, secretName, namespace)
		return nil, nil, errors.New("kubeconfig is empty")
	}

	kubeConfig, err := c.secretResolver.FetchKubeConfig(clusterID, kubeConfig)

	if err != nil {
		log.Errorf("Failed to fetch kubeconfig for cluster '%s' using secret resolver: %v, err: %v",
			clusterID, c.secretResolver, err)
		return nil, nil, errors.New("kubeconfig cannot be fetched")
	}

	clusterConfig, err := LoadKubeConfig(kubeConfig)

	if err != nil {
		log.Infof("Data '%s' in the secret %s in namespace %s is not a kubeconfig: %v",
			clusterID, secretName, namespace, err)
		log.Infof("KubeConfig: '%s'", string(kubeConfig))
		return nil, nil, errors.New("clusterConfig cannot be loaded")
	}

	clientConfig := clientcmd.NewDefaultClientConfig(*clusterConfig, &clientcmd.ConfigOverrides{})

	var restConfig *rest.Config
	restConfig, err = clientConfig.ClientConfig()

	if err != nil {
		log.Errorf("error during conversion of secret to client config: %v", err)
		return nil, nil, errors.New("restConfig cannot be built")
	}

	return &RemoteCluster{
		secretName: secretName,
	}, restConfig, nil
}

func (c *Controller) addMemberCluster(secretName string, s *corev1.Secret) {
	for clusterID, kubeConfig := range s.Data {
		// clusterID must be unique even across multiple secrets
		if prev, ok := c.Cs.RemoteClusters[clusterID]; !ok {
			log.Infof("Adding cluster_id=%v from secret=%v", clusterID, secretName)

			remoteCluster, restConfig, err := c.createRemoteCluster(kubeConfig, secretName, clusterID, s.ObjectMeta.Namespace)

			if err != nil {
				log.Errorf("Failed to add remote cluster from secret=%v for cluster_id=%v: %v",
					secretName, clusterID, err)
				continue
			}

			c.Cs.RemoteClusters[clusterID] = remoteCluster

			if err := c.addCallback(restConfig, clusterID, common.GetAdmiralParams().CacheRefreshDuration); err != nil {
				log.Errorf("error during secret loading for clusterID: %s %v", clusterID, err)
				continue
			}

			log.Infof("Secret loaded for cluster %s in the secret %s in namespace %s.", clusterID, c.Cs.RemoteClusters[clusterID].secretName, s.ObjectMeta.Namespace)

		} else {
			if prev.secretName != secretName {
				log.Errorf("ClusterID reused in two different secrets: %v and %v. ClusterID "+
					"must be unique across all secrets", prev.secretName, secretName)
				continue
			}

			log.Infof("Updating cluster %v from secret %v", clusterID, secretName)

			remoteCluster, restConfig, err := c.createRemoteCluster(kubeConfig, secretName, clusterID, s.ObjectMeta.Namespace)
			if err != nil {
				log.Errorf("Error updating cluster_id=%v from secret=%v: %v",
					clusterID, secretName, err)
				continue
			}

			c.Cs.RemoteClusters[clusterID] = remoteCluster
			if err := c.updateCallback(restConfig, clusterID, common.GetAdmiralParams().CacheRefreshDuration); err != nil {
				log.Errorf("Error updating cluster_id from secret=%v: %s %v",
					clusterID, secretName, err)
			}
		}

	}
	remoteClustersMetric.Set(float64(len(c.Cs.RemoteClusters)))
	log.Infof("Number of remote clusters: %d", len(c.Cs.RemoteClusters))
}

func (c *Controller) deleteMemberCluster(secretName string) {
	for clusterID, cluster := range c.Cs.RemoteClusters {
		if cluster.secretName == secretName {
			log.Infof("Deleting cluster member: %s", clusterID)
			err := c.removeCallback(clusterID)
			if err != nil {
				log.Errorf("error during cluster delete: %s %v", clusterID, err)
			}
			delete(c.Cs.RemoteClusters, clusterID)
		}
	}
	remoteClustersMetric.Set(float64(len(c.Cs.RemoteClusters)))
	log.Infof("Number of remote clusters: %d", len(c.Cs.RemoteClusters))
}
