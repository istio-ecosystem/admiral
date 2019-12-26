package admiral

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sAppsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/rest"
	"time"

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
	Deployments map[string][]*k8sAppsV1.Deployment
}

type DeploymentController struct {
	K8sClient         kubernetes.Interface
	DeploymentHandler DeploymentHandler
	Cache             *deploymentCache
	informer          cache.SharedIndexInformer
	ctl               *Controller
	labelSet 		  common.LabelSet
}

type deploymentCache struct {
	//map of dependencies key=identity value array of onboarded identitys
	cache map[string]*DeploymentClusterEntry
	mutex *sync.Mutex
}

func (p *deploymentCache) Put(deploymentEntry *DeploymentClusterEntry) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	p.cache[deploymentEntry.Identity] = deploymentEntry
}

func (p *deploymentCache) getKey(deployment *k8sAppsV1.Deployment) string {
	return common.GetDeploymentGlobalIdentifier(deployment)
}

func (p *deploymentCache) Get(key string) *DeploymentClusterEntry {
	return p.cache[key]
}

func (p *deploymentCache) Delete(pod *DeploymentClusterEntry) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	delete(p.cache, pod.Identity)
}

func (p *deploymentCache) AppendDeploymentToCluster(key string, deployment *k8sAppsV1.Deployment) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	v := p.Get(key)

	if v == nil {
		v = &DeploymentClusterEntry{
			Identity:    key,
			Deployments: make(map[string][]*k8sAppsV1.Deployment),
		}
		p.cache[v.Identity] = v
	}

	//TODO this is assuming globally unquie names name which might not alway be the case.  This would need a cluster name too
	namespaceDeployments := v.Deployments[deployment.Namespace]

	if namespaceDeployments == nil {
		namespaceDeployments = make([]*k8sAppsV1.Deployment, 0)
	}

	namespaceDeployments = append(namespaceDeployments, deployment)

	v.Deployments[deployment.Namespace] = namespaceDeployments

}

func (d *DeploymentController) GetDeployments() ([]*k8sAppsV1.Deployment, error) {

	ns := d.K8sClient.CoreV1().Namespaces()

	namespaceSidecarInjectionLabelFilter := d.labelSet.NamespaceSidecarInjectionLabel+"="+d.labelSet.NamespaceSidecarInjectionLabelValue
	istioEnabledNs, err := ns.List(meta_v1.ListOptions{LabelSelector: namespaceSidecarInjectionLabelFilter})

	if err != nil {
		return nil, fmt.Errorf("error getting istio labled namespaces: %v", err)
	}

	var res []*k8sAppsV1.Deployment

	for _, v := range istioEnabledNs.Items {

		deployments := d.K8sClient.AppsV1().Deployments(v.Name)
		admiralEnabledLabelFilter := d.labelSet.DeploymentLabel+"=true"
		deploymentsList, err := deployments.List(meta_v1.ListOptions{LabelSelector: admiralEnabledLabelFilter})

		if err != nil {
			return nil, fmt.Errorf("error getting istio labled namespaces: %v", err)
		}

		for _, pi := range deploymentsList.Items {
			res = append(res, &pi)
		}
	}

	return res, nil
}

func NewDeploymentController(stopCh <-chan struct{}, handler DeploymentHandler, config *rest.Config, resyncPeriod time.Duration) (*DeploymentController, error) {

	deploymentController := DeploymentController{}
	deploymentController.DeploymentHandler = handler

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

	NewController(stopCh, &deploymentController, deploymentController.informer)

	return &deploymentController, nil
}

func (d *DeploymentController) Added(ojb interface{}) {
	deployment := ojb.(*k8sAppsV1.Deployment)
	key := d.Cache.getKey(deployment)
	if len(key) > 0 && deployment.Spec.Template.Labels[d.labelSet.DeploymentLabel] == "true" {
		d.Cache.AppendDeploymentToCluster(key, deployment)
		d.DeploymentHandler.Added(deployment)
	}

}

func (d *DeploymentController) Deleted(name string) {
	//TODO deal with this
}
