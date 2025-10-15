package admiral

import (
	"context"
	"fmt"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/numaproj/numaflow/pkg/apis/numaflow/v1alpha1"
	numaflow "github.com/numaproj/numaflow/pkg/client/clientset/versioned"
	v1alpha12 "github.com/numaproj/numaflow/pkg/client/informers/externalversions/numaflow/v1alpha1"
	"k8s.io/client-go/rest"

	"sync"

	log "github.com/sirupsen/logrus"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	vertexControllerPrefix = "vertex-workload-ctrl"
)

// VertexWorkloadHandler interface contains the methods that are required
type VertexWorkloadHandler interface {
	Added(ctx context.Context, obj *v1alpha1.Vertex) error
	Deleted(ctx context.Context, obj *v1alpha1.Vertex) error
}

type VertexItem struct {
	Vertex *v1alpha1.Vertex
	Status string
}

type VertexClusterEntry struct {
	Identity string
	Vertices map[string]*VertexItem
}

type VertexWorkloadController struct {
	K8sClient             kubernetes.Interface
	NumaflowClient        numaflow.Interface
	VertexWorkloadHandler VertexWorkloadHandler
	Cache                 *vertexWorkloadCache
	informer              cache.SharedIndexInformer
	labelSet              *common.LabelSet
}

func (d *VertexWorkloadController) DoesGenerationMatch(ctxLogger *log.Entry, obj interface{}, oldObj interface{}) (bool, error) {
	if !common.DoGenerationCheck() {
		ctxLogger.Debugf(ControllerLogFormat, "DoesGenerationMatch", "",
			fmt.Sprintf("generation check is disabled"))
		return false, nil
	}
	vertexNew, ok := obj.(*v1alpha1.Vertex)
	if !ok {
		return false, fmt.Errorf("type assertion failed, %v is not of type *v1alpha1.Vertex", obj)
	}
	vertexOld, ok := oldObj.(*v1alpha1.Vertex)
	if !ok {
		return false, fmt.Errorf("type assertion failed, %v is not of type *v1alpha1.Vertex", oldObj)
	}
	if vertexNew.Generation == vertexOld.Generation {
		ctxLogger.Infof(ControllerLogFormat, "DoesGenerationMatch", "",
			fmt.Sprintf("old and new generation matched for vertex %s", vertexNew.Name))
		return true, nil
	}
	return false, nil
}

func (d *VertexWorkloadController) IsOnlyReplicaCountChanged(ctxLogger *log.Entry, obj interface{}, oldObj interface{}) (bool, error) {
	// Vertex doesn't have replica count, so this is always false
	return false, nil
}

type vertexWorkloadCache struct {
	cache map[string]*VertexClusterEntry
	mutex *sync.Mutex
}

func NewVertexWorkloadCache() *vertexWorkloadCache {
	return &vertexWorkloadCache{
		cache: make(map[string]*VertexClusterEntry),
		mutex: &sync.Mutex{},
	}
}

func (p *vertexWorkloadCache) getKey(vertex *v1alpha1.Vertex) string {
	return common.GetVertexGlobalIdentifier(vertex)
}

func (p *vertexWorkloadCache) Get(key string, env string) *v1alpha1.Vertex {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	vce, ok := p.cache[key]
	if ok {
		vceEnv, ok := vce.Vertices[env]
		if ok {
			return vceEnv.Vertex
		}
	}

	return nil
}

func (d *vertexWorkloadCache) List() []v1alpha1.Vertex {
	var vertexList []v1alpha1.Vertex
	d.mutex.Lock()
	defer d.mutex.Unlock()
	for _, vertexClusterEntry := range d.cache {
		for _, vertexItem := range vertexClusterEntry.Vertices {
			if vertexItem != nil && vertexItem.Vertex != nil {
				vertexList = append(vertexList, *vertexItem.Vertex)
			}
		}
	}
	return vertexList
}

func (p *vertexWorkloadCache) GetVertexProcessStatus(vertex *v1alpha1.Vertex) string {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	env := common.GetEnvForVertex(vertex)
	key := p.getKey(vertex)

	vce, ok := p.cache[key]
	if ok {
		vceEnv, ok := vce.Vertices[env]
		if ok {
			return vceEnv.Status
		}
	}

	return common.NotProcessed
}

func (p *vertexWorkloadCache) UpdateVertexProcessStatus(vertex *v1alpha1.Vertex, status string) error {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	env := common.GetEnvForVertex(vertex)
	key := p.getKey(vertex)

	vce, ok := p.cache[key]
	if ok {
		vceEnv, ok := vce.Vertices[env]
		if ok {
			vceEnv.Status = status
			p.cache[vce.Identity] = vce
			return nil
		} else {
			vce.Vertices[env] = &VertexItem{
				Status: status,
			}

			p.cache[vce.Identity] = vce
			return nil
		}
	}

	return fmt.Errorf(LogCacheFormat, "Update", "Vertex",
		vertex.Name, vertex.Namespace, "", "nothing to update, vertex not found in cache")
}

func (p *vertexWorkloadCache) UpdateVertexToClusterCache(key string, vertex *v1alpha1.Vertex) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	env := common.GetEnvForVertex(vertex)

	vce := p.cache[key]
	if vce == nil {
		vce = &VertexClusterEntry{
			Identity: key,
			Vertices: make(map[string]*VertexItem),
		}
	}

	vce.Vertices[env] = &VertexItem{
		Vertex: vertex,
		Status: common.ProcessingInProgress,
	}
	p.cache[vce.Identity] = vce
}

func (p *vertexWorkloadCache) DeleteFromVertexClusterCache(key string, vertex *v1alpha1.Vertex) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	var (
		env = common.GetEnvForVertex(vertex)
		vce = p.cache[key]
	)

	if vce != nil {
		if vce.Vertices[env] != nil && vce.Vertices[env].Vertex != nil && vertex.Name == vce.Vertices[env].Vertex.Name {
			log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Delete", "Vertex",
				vertex.Name, vertex.Namespace, "", "ignoring vertex and deleting from cache")
			delete(vce.Vertices, env)
		} else {
			log.Warnf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Get", "Vertex",
				vertex.Name, vertex.Namespace, "", "ignoring vertex delete as it doesn't match the one in cache")
		}
	} else {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Delete", "Vertex",
			vertex.Name, vertex.Namespace, "", "nothing to delete, vertex not found in cache")
	}
}

func NewVertexWorkloadController(stopCh <-chan struct{}, handler VertexWorkloadHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*VertexWorkloadController, error) {

	vertexController := VertexWorkloadController{}
	vertexController.VertexWorkloadHandler = handler
	vertexController.labelSet = common.GetLabelSet()

	vertexController.Cache = NewVertexWorkloadCache()
	var err error
	vertexController.K8sClient, err = clientLoader.LoadKubeClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vertex controller k8s client: %v", err)
	}

	vertexController.NumaflowClient, err = clientLoader.LoadNumaflowClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vertex controller numaflow client: %v", err)
	}

	vertexController.informer = v1alpha12.NewVertexInformer(
		vertexController.NumaflowClient,
		meta_v1.NamespaceAll,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController(vertexControllerPrefix, config.Host, stopCh, &vertexController, vertexController.informer)

	return &vertexController, nil
}

func (d *VertexWorkloadController) Added(ctx context.Context, obj interface{}) error {
	return HandleAddUpdateVertex(ctx, obj, d)
}

func (d *VertexWorkloadController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	return HandleAddUpdateVertex(ctx, obj, d)
}

func (d *VertexWorkloadController) GetProcessItemStatus(obj interface{}) (string, error) {
	vertex, ok := obj.(*v1alpha1.Vertex)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1alpha1.Vertex", obj)
	}
	return d.Cache.GetVertexProcessStatus(vertex), nil
}

func (d *VertexWorkloadController) UpdateProcessItemStatus(obj interface{}, status string) error {
	vertex, ok := obj.(*v1alpha1.Vertex)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha1.Vertex", obj)
	}
	return d.Cache.UpdateVertexProcessStatus(vertex, status)
}

func HandleAddUpdateVertex(ctx context.Context, obj interface{}, d *VertexWorkloadController) error {
	vertex, ok := obj.(*v1alpha1.Vertex)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha1.Vertex", obj)
	}
	key := d.Cache.getKey(vertex)
	defer util.LogElapsedTime("HandleAddUpdateVertex", key, vertex.Name+"_"+vertex.Namespace, "")()
	if len(key) > 0 {
		if !d.shouldIgnoreBasedOnLabels(ctx, vertex) {
			d.Cache.UpdateVertexToClusterCache(key, vertex)
			return d.VertexWorkloadHandler.Added(ctx, vertex)
		} else {
			ns, err := d.K8sClient.CoreV1().Namespaces().Get(ctx, vertex.Namespace, meta_v1.GetOptions{})
			if err != nil {
				log.Warnf("Failed to get namespace object for vertex with namespace %v, err: %v", vertex.Namespace, err)
			} else if (ns != nil && ns.Annotations[common.AdmiralIgnoreAnnotation] == "true") || (vertex.Annotations != nil && vertex.Annotations[common.AdmiralIgnoreAnnotation] == "true") {
				log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.Vertex,
					vertex.Name, vertex.Namespace, "", "Value=true")
			}
			d.Cache.DeleteFromVertexClusterCache(key, vertex)
		}
	}
	return nil
}

func (d *VertexWorkloadController) Deleted(ctx context.Context, obj interface{}) error {
	vertex, ok := obj.(*v1alpha1.Vertex)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha1.Vertex", obj)
	}
	if d.shouldIgnoreBasedOnLabels(ctx, vertex) {
		ns, err := d.K8sClient.CoreV1().Namespaces().Get(ctx, vertex.Namespace, meta_v1.GetOptions{})
		if err != nil {
			log.Warnf("Failed to get namespace object for vertex with namespace %v, err: %v", vertex.Namespace, err)
		} else if (ns != nil && ns.Annotations[common.AdmiralIgnoreAnnotation] == "true") || (vertex.Annotations != nil && vertex.Annotations[common.AdmiralIgnoreAnnotation] == "true") {
			log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.Vertex,
				vertex.Name, vertex.Namespace, "", "Value=true")
		}
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Delete", common.Vertex,
			vertex.Name, vertex.Namespace, "", "ignoring vertex on basis of labels/annotation")
		return nil
	}
	key := d.Cache.getKey(vertex)
	err := d.VertexWorkloadHandler.Deleted(ctx, vertex)
	if err == nil && len(key) > 0 {
		d.Cache.DeleteFromVertexClusterCache(key, vertex)
		d.Cache.DeleteFromVertexClusterCache(common.GetVertexOriginalIdentifier(vertex), vertex)
	}
	return err
}

func (d *VertexWorkloadController) shouldIgnoreBasedOnLabels(ctx context.Context, vertex *v1alpha1.Vertex) bool {
	if vertex.Spec.Metadata != nil {
		if vertex.Spec.Metadata.Labels != nil && vertex.Spec.Metadata.Labels[d.labelSet.AdmiralIgnoreLabel] == "true" {
			return true
		}

		if vertex.Spec.Metadata.Annotations != nil && vertex.Spec.Metadata.Annotations[d.labelSet.DeploymentAnnotation] != "true" {
			return true
		}
	}

	if vertex.Annotations != nil && vertex.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		return true
	}

	ns, err := d.K8sClient.CoreV1().Namespaces().Get(ctx, vertex.Namespace, meta_v1.GetOptions{})
	if err != nil {
		log.Warnf("Failed to get namespace object for vertex with namespace %v, err: %v", vertex.Namespace, err)
		return false
	}

	if ns.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		return true
	}
	return false
}

func (d *VertexWorkloadController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	vertex, ok := obj.(*v1alpha1.Vertex)
	if !ok {
		return
	}
	if d.K8sClient != nil {
		ns, err := d.K8sClient.CoreV1().Namespaces().Get(context.Background(), vertex.Namespace, meta_v1.GetOptions{})
		if err != nil {
			log.Warnf("Failed to get namespace object for vertex with namespace %v, err: %v", vertex.Namespace, err)
		} else if (ns != nil && ns.Annotations[common.AdmiralIgnoreAnnotation] == "true") || (vertex.Annotations != nil && vertex.Annotations[common.AdmiralIgnoreAnnotation] == "true") {
			log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.Vertex,
				vertex.Name, vertex.Namespace, "", "Value=true")
		}
	}
}

func (d *VertexWorkloadController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	vertex, ok := obj.(*v1alpha1.Vertex)
	if ok && isRetry {
		return d.Cache.Get(common.GetVertexGlobalIdentifier(vertex), common.GetEnvForVertex(vertex)), nil
	}
	if ok && d.NumaflowClient != nil {
		return d.NumaflowClient.NumaflowV1alpha1().Vertices(vertex.Namespace).Get(ctx, vertex.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("numaflow client is not initialized, txId=%s", ctx.Value("txId"))
}
