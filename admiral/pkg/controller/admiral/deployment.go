package admiral

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/sirupsen/logrus"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sAppsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/rest"
	"time"

	log "github.com/sirupsen/logrus"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sync"
)

// Handler interface contains the methods that are required
type DeploymentHandler interface {
	Added(obj *k8sAppsV1.Deployment)
	Deleted(obj *k8sAppsV1.Deployment)
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

func (p *deploymentCache) Get(key string) *DeploymentClusterEntry {
	return p.cache[key]
}

func (p *deploymentCache) UpdateDeploymentToClusterCache(key string, deployment *k8sAppsV1.Deployment) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	v := p.Get(key)

	if v == nil {
		v = &DeploymentClusterEntry{
			Identity:    key,
			Deployments: make(map[string]*k8sAppsV1.Deployment),
		}
		p.cache[v.Identity] = v
	}
	env := common.GetEnv(deployment)
	v.Deployments[env] = deployment
}

func (p *deploymentCache) DeleteFromDeploymentClusterCache(key string, deployment *k8sAppsV1.Deployment) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	v := p.Get(key)

	if v != nil {
		env := common.GetEnv(deployment)
		delete(v.Deployments, env)
	}
}

func (d *DeploymentController) GetDeployments() ([]*k8sAppsV1.Deployment, error) {

	ns := d.K8sClient.CoreV1().Namespaces()

	namespaceSidecarInjectionLabelFilter := d.labelSet.NamespaceSidecarInjectionLabel + "=" + d.labelSet.NamespaceSidecarInjectionLabelValue
	istioEnabledNs, err := ns.List(meta_v1.ListOptions{LabelSelector: namespaceSidecarInjectionLabelFilter})

	if err != nil {
		return nil, fmt.Errorf("error getting istio labled namespaces: %v", err)
	}

	var res []*k8sAppsV1.Deployment

	for _, v := range istioEnabledNs.Items {

		deployments := d.K8sClient.AppsV1().Deployments(v.Name)
		deploymentsList, err := deployments.List(meta_v1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("error listing deployments: %v", err)
		}
		var admiralDeployments []k8sAppsV1.Deployment
		for _, deployment := range deploymentsList.Items {
			if !d.shouldIgnoreBasedOnLabels(&deployment) {
				admiralDeployments = append(admiralDeployments, deployment)
			}
		}

		if err != nil {
			return nil, fmt.Errorf("error getting istio labled namespaces: %v", err)
		}

		for _, pi := range admiralDeployments {
			res = append(res, &pi)
		}
	}

	return res, nil
}

func NewDeploymentController(stopCh <-chan struct{}, handler DeploymentHandler, config *rest.Config, resyncPeriod time.Duration) (*DeploymentController, error) {

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

	NewController("deployment-ctrl-" + config.Host, stopCh, &deploymentController, deploymentController.informer)

	return &deploymentController, nil
}

func NewDeploymentControllerWithLabelOverride(stopCh <-chan struct{}, handler DeploymentHandler, config *rest.Config, resyncPeriod time.Duration, labelSet *common.LabelSet) (*DeploymentController, error) {

	dc, err := NewDeploymentController(stopCh, handler, config, resyncPeriod)
	dc.labelSet = labelSet
	return dc, err
}

func (d *DeploymentController) Added(obj interface{}) {
	HandleAddUpdateDeployment(obj, d)
}

func (d *DeploymentController) Updated(obj interface{}, oldObj interface{}) {
	HandleAddUpdateDeployment(obj, d)
}

func HandleAddUpdateDeployment(ojb interface{}, d *DeploymentController) {
	deployment := ojb.(*k8sAppsV1.Deployment)
	key := d.Cache.getKey(deployment)
	if len(key) > 0 {
		if !d.shouldIgnoreBasedOnLabels(deployment) {
			d.Cache.UpdateDeploymentToClusterCache(key, deployment)
			d.DeploymentHandler.Added(deployment)
		} else {
			d.Cache.DeleteFromDeploymentClusterCache(key, deployment)
			log.Debugf("ignoring deployment %v based on labels", deployment.Name)
		}
	}
}

func (d *DeploymentController) Deleted(ojb interface{}) {
	deployment := ojb.(*k8sAppsV1.Deployment)
	key := d.Cache.getKey(deployment)
	d.DeploymentHandler.Deleted(deployment)
	if len(key) > 0 {
		d.Cache.DeleteFromDeploymentClusterCache(key, deployment)
	}
}

func (d *DeploymentController) shouldIgnoreBasedOnLabels(deployment *k8sAppsV1.Deployment) bool {
	if deployment.Spec.Template.Labels[d.labelSet.AdmiralIgnoreLabel] == "true" { //if we should ignore, do that and who cares what else is there
		return true
	}

	if deployment.Spec.Template.Annotations[d.labelSet.DeploymentAnnotation] != "true" { //Not sidecar injected, we don't want to inject
		return true
	}

	if deployment.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		return true
	}

	ns, err := d.K8sClient.CoreV1().Namespaces().Get(deployment.Namespace, meta_v1.GetOptions{})
	if err != nil {
		log.Warnf("Failed to get namespace object for deployment with namespace %v, err: %v", deployment.Namespace, err)
		return false
	}

	if ns.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		return true
	}
	return false //labels are fine, we should not ignore
}

func (d *DeploymentController) GetDeploymentByLabel(labelValue string, namespace string) []k8sAppsV1.Deployment {
	matchLabel := common.GetGlobalTrafficDeploymentLabel()
	labelOptions := meta_v1.ListOptions{}
	labelOptions.LabelSelector = fmt.Sprintf("%s=%s", matchLabel, labelValue)
	matchedDeployments, err := d.K8sClient.AppsV1().Deployments(namespace).List(labelOptions)

	if err != nil {
		logrus.Errorf("Failed to list deployments in cluster, error: %v", err)
		return nil
	}

	if matchedDeployments.Items == nil {
		return []k8sAppsV1.Deployment{}
	}

	return matchedDeployments.Items
}
