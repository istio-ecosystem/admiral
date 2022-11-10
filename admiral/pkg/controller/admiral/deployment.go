package admiral

import (
	"context"
	"fmt"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sAppsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/rest"

	"sync"

	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// DeploymentHandler interface contains the methods that are required
type DeploymentHandler interface {
	Added(ctx context.Context, obj *k8sAppsV1.Deployment)
	Deleted(ctx context.Context, obj *k8sAppsV1.Deployment)
}

type DeploymentClusterEntry struct {
	Identity    string
	Deployments map[string]*k8sAppsV1.Deployment
}

type DeploymentController struct {
	K8sClient         kubernetes.Interface
	DeploymentHandler DeploymentHandler
	Cache             *deploymentCache
	informer          cache.SharedIndexInformer
	labelSet          *common.LabelSet
}

type deploymentCache struct {
	//map of dependencies key=identity value array of onboarded identities
	cache map[string]*DeploymentClusterEntry
	mutex *sync.Mutex
}

func (p *deploymentCache) getKey(deployment *k8sAppsV1.Deployment) string {
	return common.GetDeploymentGlobalIdentifier(deployment)
}

func (p *deploymentCache) Get(key string, env string) *k8sAppsV1.Deployment {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	dce := p.cache[key]
	if dce != nil {
		return dce.Deployments[env]
	} else {
		return nil
	}
}

func (p *deploymentCache) UpdateDeploymentToClusterCache(key string, deployment *k8sAppsV1.Deployment) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	env := common.GetEnv(deployment)

	dce := p.cache[key]

	if dce == nil {
		dce = &DeploymentClusterEntry{
			Identity:    key,
			Deployments: make(map[string]*k8sAppsV1.Deployment),
		}
	}
	dce.Deployments[env] = deployment
	p.cache[dce.Identity] = dce
}

func (p *deploymentCache) DeleteFromDeploymentClusterCache(key string, deployment *k8sAppsV1.Deployment) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	var (
		env = common.GetEnv(deployment)
		dce = p.cache[key]
	)

	if dce != nil {
		if dce.Deployments[env] != nil && deployment.Name == dce.Deployments[env].Name {
			log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Delete", "Deployment",
				deployment.Name, deployment.Namespace, "", "ignoring deployment and deleting from cache")
			delete(dce.Deployments, env)
		} else {
			log.Warnf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Get", "Deployment",
				deployment.Name, deployment.Namespace, "", "ignoring deployment delete as it doesn't match the one in cache")
		}
	} else {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Delete", "Deployment",
			deployment.Name, deployment.Namespace, "", "nothing to delete, deployment not found in cache")
	}
}

func NewDeploymentController(clusterID string, stopCh <-chan struct{}, handler DeploymentHandler, config *rest.Config, resyncPeriod time.Duration) (*DeploymentController, error) {

	deploymentController := DeploymentController{}
	deploymentController.DeploymentHandler = handler
	deploymentController.labelSet = common.GetLabelSet()

	deploymentCache := deploymentCache{}
	deploymentCache.cache = make(map[string]*DeploymentClusterEntry)
	deploymentCache.mutex = &sync.Mutex{}

	deploymentController.Cache = &deploymentCache
	var err error

	deploymentController.K8sClient, err = K8sClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller k8s client: %v", err)
	}

	deploymentController.informer = k8sAppsinformers.NewDeploymentInformer(
		deploymentController.K8sClient,
		meta_v1.NamespaceAll,
		resyncPeriod,
		cache.Indexers{},
	)

	wc := NewMonitoredDelegator(&deploymentController, clusterID, "deployment")
	NewController("deployment-ctrl-"+config.Host, stopCh, wc, deploymentController.informer)

	return &deploymentController, nil
}

func (d *DeploymentController) Added(ctx context.Context, obj interface{}) {
	HandleAddUpdateDeployment(ctx, obj, d)
}

func (d *DeploymentController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) {
	HandleAddUpdateDeployment(ctx, obj, d)
}

func HandleAddUpdateDeployment(ctx context.Context, ojb interface{}, d *DeploymentController) {
	deployment := ojb.(*k8sAppsV1.Deployment)
	key := d.Cache.getKey(deployment)
	if len(key) > 0 {
		if !d.shouldIgnoreBasedOnLabels(ctx, deployment) {
			d.Cache.UpdateDeploymentToClusterCache(key, deployment)
			d.DeploymentHandler.Added(ctx, deployment)
		} else {
			d.Cache.DeleteFromDeploymentClusterCache(key, deployment)
		}
	}
}

func (d *DeploymentController) Deleted(ctx context.Context, ojb interface{}) {
	deployment := ojb.(*k8sAppsV1.Deployment)
	key := d.Cache.getKey(deployment)
	d.DeploymentHandler.Deleted(ctx, deployment)
	if len(key) > 0 {
		d.Cache.DeleteFromDeploymentClusterCache(key, deployment)
	}
}

func (d *DeploymentController) shouldIgnoreBasedOnLabels(ctx context.Context, deployment *k8sAppsV1.Deployment) bool {
	if deployment.Spec.Template.Labels[d.labelSet.AdmiralIgnoreLabel] == "true" { //if we should ignore, do that and who cares what else is there
		return true
	}

	if deployment.Spec.Template.Annotations[d.labelSet.DeploymentAnnotation] != "true" { //Not sidecar injected, we don't want to inject
		return true
	}

	if deployment.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		return true
	}

	ns, err := d.K8sClient.CoreV1().Namespaces().Get(ctx, deployment.Namespace, meta_v1.GetOptions{})
	if err != nil {
		log.Warnf("Failed to get namespace object for deployment with namespace %v, err: %v", deployment.Namespace, err)
		return false
	}

	if ns.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		return true
	}
	return false //labels are fine, we should not ignore
}

func (d *DeploymentController) GetDeploymentBySelectorInNamespace(ctx context.Context, serviceSelector map[string]string, namespace string) []k8sAppsV1.Deployment {

	matchedDeployments, err := d.K8sClient.AppsV1().Deployments(namespace).List(ctx, meta_v1.ListOptions{})

	if err != nil {
		logrus.Errorf("Failed to list deployments in cluster, error: %v", err)
		return nil
	}

	if matchedDeployments.Items == nil {
		return []k8sAppsV1.Deployment{}
	}

	filteredDeployments := make([]k8sAppsV1.Deployment, 0)

	for _, deployment := range matchedDeployments.Items {
		if common.IsServiceMatch(serviceSelector, deployment.Spec.Selector) {
			filteredDeployments = append(filteredDeployments, deployment)
		}
	}

	return filteredDeployments
}
