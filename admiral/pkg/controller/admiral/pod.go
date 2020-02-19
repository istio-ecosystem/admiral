package admiral

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	k8sV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"time"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sync"
)

// Handler interface contains the methods that are required
type PodHandler interface {
	Added(obj *k8sV1.Pod)
	Deleted(obj *k8sV1.Pod)
}

type PodClusterEntry struct {
	Identity string
	Pods     map[string][]*k8sV1.Pod
}

type PodController struct {
	K8sClient  kubernetes.Interface
	PodHandler PodHandler
	Cache      *podCache
	informer   cache.SharedIndexInformer
	ctl        *Controller
	labelSet   *common.LabelSet
}

type podCache struct {
	//map of dependencies key=identity value array of onboarded identities
	cache map[string]*PodClusterEntry
	mutex *sync.Mutex
}

func (p *podCache) Put(podEntry *PodClusterEntry) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	p.cache[podEntry.Identity] = podEntry
}

func (p *podCache) getKey(pod *k8sV1.Pod) string {
	return common.GetPodGlobalIdentifier(pod)
}

func (p *podCache) Get(key string) *PodClusterEntry {
	return p.cache[key]
}

func (p *podCache) Delete(pod *PodClusterEntry) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	delete(p.cache, pod.Identity)
}

func (p *podCache) AppendPodToCluster(key string, pod *k8sV1.Pod) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	v := p.Get(key)

	if v == nil {
		v = &PodClusterEntry{
			Identity: key,
			Pods:     make(map[string][]*k8sV1.Pod),
		}
		p.cache[v.Identity] = v
	}

	namespacePods := v.Pods[pod.Namespace]

	if namespacePods == nil {
		namespacePods = make ([]*k8sV1.Pod, 0)
	}

	namespacePods = append(namespacePods, pod)

	v.Pods[pod.Namespace] = namespacePods

}

func (d *PodController) GetPods() ([]*k8sV1.Pod, error) {

	ns := d.K8sClient.CoreV1().Namespaces()

	namespaceSidecarInjectionLabelFilter := d.labelSet.NamespaceSidecarInjectionLabel+"="+d.labelSet.NamespaceSidecarInjectionLabelValue
	istioEnabledNs, err := ns.List(meta_v1.ListOptions{LabelSelector: namespaceSidecarInjectionLabelFilter})

	if err != nil {
		return nil, fmt.Errorf("error getting istio labled namespaces: %v", err)
	}

	var res []*k8sV1.Pod

	for _, v := range istioEnabledNs.Items {

		pods := d.K8sClient.CoreV1().Pods(v.Name)
		podsList, err := pods.List(meta_v1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("error listing pods: %v", err)
		}
		var admiralDeployments []k8sV1.Pod
		for _, pod := range podsList.Items {
			if pod.Annotations[d.labelSet.DeploymentAnnotation] == "true" {
				admiralDeployments = append(admiralDeployments, pod)
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

func NewPodController(stopCh <-chan struct{}, handler PodHandler, config *rest.Config, resyncPeriod time.Duration) (*PodController, error) {

	podController := PodController{}
	podController.PodHandler = handler
	podController.labelSet = common.GetLabelSet()

	podCache := podCache{}
	podCache.cache = make(map[string]*PodClusterEntry)
	podCache.mutex = &sync.Mutex{}

	podController.Cache = &podCache
	var err error

	podController.K8sClient, err = K8sClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pod controller k8s client: %v", err)
	}

	podController.informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(opts meta_v1.ListOptions) (runtime.Object, error) {
				return podController.K8sClient.CoreV1().Pods(meta_v1.NamespaceAll).List(opts)
			},
			WatchFunc: func(opts meta_v1.ListOptions) (watch.Interface, error) {
				return podController.K8sClient.CoreV1().Pods(meta_v1.NamespaceAll).Watch(opts)
			},
		},
		&k8sV1.Pod{}, resyncPeriod, cache.Indexers{},
	)

	NewController(stopCh, &podController, podController.informer)

	return &podController, nil
}

func (d *PodController) Added(ojb interface{}) {
	pod := ojb.(*k8sV1.Pod)
	key := d.Cache.getKey(pod)
	if len(key) > 0 && pod.Labels[d.labelSet.DeploymentAnnotation] == "true" {
		d.Cache.AppendPodToCluster(key, pod)
		d.PodHandler.Added(pod)
	}

}

func (d *PodController) Deleted(name string) {
	//TODO deal with this
}
