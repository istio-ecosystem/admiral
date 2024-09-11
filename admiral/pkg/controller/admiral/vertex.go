package admiral

import (
	"context"
	"fmt"
	"github.com/numaproj/numaflow/pkg/apis/numaflow/v1alpha1"
	numaflow "github.com/numaproj/numaflow/pkg/client/clientset/versioned"
	v1alpha12 "github.com/numaproj/numaflow/pkg/client/informers/externalversions/numaflow/v1alpha1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sync"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"

	"k8s.io/client-go/tools/cache"
)

//Vertex controller discovers vertexs as mesh clients (its assumed that k8s Vertex doesn't have any ingress communication)

type VertexController struct {
	NumaflowClient   numaflow.Interface
	VertexHandler  ClientDiscoveryHandler
	informer    cache.SharedIndexInformer
	Cache       *vertexCache
}

type VertexEntry struct {
	Identity string
	Vertices map[string]*K8sObject
}

type vertexCache struct {
	//map of dependencies key=identity value array of onboarded identities
	cache map[string]*VertexEntry
	mutex *sync.Mutex
}

func NewVertexCache() *vertexCache {
	return &vertexCache{
		cache: make(map[string]*VertexEntry),
		mutex: &sync.Mutex{},
	}
}


func (p *vertexCache) getK8sObjectFromVertex(vertex v1alpha1.Vertex) *K8sObject{
	return &K8sObject{
		Name: vertex.Name,
		Namespace: vertex.Namespace,
		Annotations: vertex.Spec.Metadata.Annotations,
		Labels: vertex.Spec.Metadata.Labels,
		Status: common.NotProcessed,
		Type: common.Vertex,
	}
}

func (p *vertexCache) Put(vertex *K8sObject) (*K8sObject, bool) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	identity := common.GetGlobalIdentifier(vertex.Annotations, vertex.Labels)
	existingVertices := p.cache[identity]
	if existingVertices == nil {
		existingVertices = &VertexEntry{
			Identity: identity,
			Vertices: map[string]*K8sObject{vertex.Namespace: vertex},
		}
		p.cache[identity] = existingVertices
		return vertex, true
	} else {
		vertexInCache := existingVertices.Vertices[vertex.Namespace]
		if vertexInCache == nil {
			existingVertices.Vertices[vertex.Namespace] = vertex
			p.cache[identity] = existingVertices
			return vertex, true
		}
	}
	return nil, false
}

func (p *vertexCache) GetByIdentity(key string) *VertexEntry {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	jce := p.cache[key]
	if jce == nil {
		return nil
	} else {
		return jce
	}
}

func (p *vertexCache) Get(key string, namespace string) *K8sObject {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	jce, ok := p.cache[key]
	if ok {
		j, ok := jce.Vertices[namespace]
		if ok {
			return j
		}
	}
	return nil
}

func (p *vertexCache) GetVertexProcessStatus(vertex v1alpha1.Vertex) (string, error) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	identity := common.GetGlobalIdentifier(vertex.Annotations, vertex.Labels)

	jce, ok := p.cache[identity]
	if ok {
		vertexFromNamespace, ok := jce.Vertices[vertex.Namespace]
		if ok {
			return vertexFromNamespace.Status, nil
		}
	}

	return common.NotProcessed, nil
}

func (p *vertexCache) UpdateVertexProcessStatus(vertex v1alpha1.Vertex, status string) error {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	identity := common.GetGlobalIdentifier(vertex.Annotations, vertex.Labels)

	jce, ok := p.cache[identity]
	if ok {
		vertexFromNamespace, ok := jce.Vertices[vertex.Namespace]
		if ok {
			vertexFromNamespace.Status = status
			p.cache[jce.Identity] = jce
			return nil
		} else {
			newVertex := p.getK8sObjectFromVertex(vertex)
			newVertex.Status = status
			jce.Vertices[vertex.Namespace] = newVertex
			p.cache[jce.Identity] = jce
			return nil
		}
	}

	return fmt.Errorf(LogCacheFormat, "UpdateStatus", "Vertex",
		vertex.Name, vertex.Namespace, "", "nothing to update, vertex not found in cache")
}

func (p *VertexController) DoesGenerationMatch(ctxLogger *log.Entry, obj interface{}, oldObj interface{}) (bool, error) {
	if !common.DoGenerationCheck() {
		ctxLogger.Debugf(ControllerLogFormat, "DoesGenerationMatch", "",
			fmt.Sprintf("generation check is disabled"))
		return false, nil
	}
	vertexNew, ok := obj.(v1alpha1.Vertex)
	if !ok {
		return false, fmt.Errorf("type assertion failed, %v is not of type *Vertex", obj)
	}
	vertexOld, ok := oldObj.(v1alpha1.Vertex)
	if !ok {
		return false, fmt.Errorf("type assertion failed, %v is not of type *Vertex", oldObj)
	}
	if vertexNew.Generation == vertexOld.Generation {
		ctxLogger.Infof(ControllerLogFormat, "DoesGenerationMatch", "",
			fmt.Sprintf("old and new generation matched for vertex %s", vertexNew.Name))
		return true, nil
	}
	return false, nil
}

func NewVertexController(stopCh <-chan struct{}, handler ClientDiscoveryHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*VertexController, error) {

	vertexController := VertexController{}
	vertexController.VertexHandler = handler

	var err error

	vertexController.NumaflowClient, err = clientLoader.LoadNumaflowClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller k8s client: %v", err)
	}

	vertexController.informer = v1alpha12.NewVertexInformer(
		vertexController.NumaflowClient,
		meta_v1.NamespaceAll,
		resyncPeriod,
		cache.Indexers{},
	)

	vertexController.Cache = NewVertexCache()

	NewController("vertex-ctrl", config.Host, stopCh, &vertexController, vertexController.informer)

	return &vertexController, nil
}

func (d *VertexController) Added(ctx context.Context, obj interface{}) error {
	return addUpdateVertex(d, ctx, obj)
}

func (d *VertexController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	//Not Required, to be handled via off boarding
	return nil
}

func addUpdateVertex(j *VertexController, ctx context.Context, obj interface{}) error {
	vertex, ok := obj.(v1alpha1.Vertex)
	if !ok {
		return nil
	}
	if !common.ShouldIgnore(vertex.Annotations, vertex.Labels) {
		k8sObj := j.Cache.getK8sObjectFromVertex(vertex)
		newK8sObj, isNew := j.Cache.Put(k8sObj)
		if isNew {
			j.VertexHandler.Added(ctx, newK8sObj)
		} else {
			log.Infof("Ignoring vertex %v as it was already processed", vertex.Name)
		}
	}
	return nil
}

func (p *VertexController) Deleted(ctx context.Context, obj interface{}) error {
	//Not Required (to be handled via asset off boarding)
	return nil
}

func (d *VertexController) GetProcessItemStatus(obj interface{}) (string, error) {
	vertex, ok := obj.(v1alpha1.Vertex)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *K8sObject", obj)
	}
	return d.Cache.GetVertexProcessStatus(vertex)
}

func (d *VertexController) UpdateProcessItemStatus(obj interface{}, status string) error {
	vertex, ok := obj.(v1alpha1.Vertex)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *Vertex", obj)
	}
	return d.Cache.UpdateVertexProcessStatus(vertex, status)
}

func (d *VertexController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	vertex, ok := obj.(v1alpha1.Vertex)
	if !ok {
		return
	}

	if vertex.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.Vertex,
				vertex.Name, vertex.Namespace, "", "Value=true")
	}
}

func (j *VertexController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	vertex, ok := obj.(v1alpha1.Vertex)
	identity := common.GetGlobalIdentifier(vertex.Annotations, vertex.Labels)
	if ok && isRetry {
		return j.Cache.Get(identity, vertex.Namespace), nil
	}
	if ok && j.NumaflowClient != nil {
		return j.NumaflowClient.NumaflowV1alpha1().Vertices(vertex.Namespace).Get(ctx, vertex.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("kubernetes client is not initialized, txId=%s", ctx.Value("txId"))
}
