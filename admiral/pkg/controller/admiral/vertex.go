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
	NumaflowClient numaflow.Interface
	VertexHandler  ClientDiscoveryHandler
	informer       cache.SharedIndexInformer
	Cache          *vertexCache
}

type VertexEntry struct {
	Identity string
	Vertices map[string]*common.K8sObject
}

type vertexCache struct {
	//map of dependencies key=identity value array of onboarded identities
	cache map[string]*VertexEntry
	mutex *sync.Mutex
}

func newVertexCache() *vertexCache {
	return &vertexCache{
		cache: make(map[string]*VertexEntry),
		mutex: &sync.Mutex{},
	}
}

func getK8sObjectFromVertex(vertex *v1alpha1.Vertex) *common.K8sObject {
	labels := make(map[string]string)
	annotations := make(map[string]string)
	if vertex.Spec.Metadata != nil {
		labels = vertex.Spec.Metadata.Labels
		annotations = vertex.Spec.Metadata.Annotations
	}
	return &common.K8sObject{
		Name:        vertex.Name,
		Namespace:   vertex.Namespace,
		Annotations: annotations,
		Labels:      labels,
		Status:      common.NotProcessed,
		Type:        common.Vertex,
	}
}

func (p *vertexCache) Put(vertex *common.K8sObject) *common.K8sObject {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	identity := common.GetGlobalIdentifier(vertex.Annotations, vertex.Labels)
	existingVertices := p.cache[identity]
	if existingVertices == nil {
		existingVertices = &VertexEntry{
			Identity: identity,
			Vertices: map[string]*common.K8sObject{vertex.Namespace: vertex},
		}
		p.cache[identity] = existingVertices
		return vertex
	} else {
		vertexInCache := existingVertices.Vertices[vertex.Namespace]
		if vertexInCache == nil {
			existingVertices.Vertices[vertex.Namespace] = vertex
			p.cache[identity] = existingVertices
			return vertex
		}
	}
	return vertex
}

func (p *vertexCache) Get(key string, namespace string) *common.K8sObject {
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

func (p *vertexCache) GetVertexProcessStatus(vertex *v1alpha1.Vertex) (string, error) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	vertexObj := getK8sObjectFromVertex(vertex)
	identity := common.GetGlobalIdentifier(vertexObj.Annotations, vertexObj.Labels)

	jce, ok := p.cache[identity]
	if ok {
		vertexFromNamespace, ok := jce.Vertices[vertexObj.Namespace]
		if ok {
			return vertexFromNamespace.Status, nil
		}
	}

	return common.NotProcessed, nil
}

func (p *vertexCache) UpdateVertexProcessStatus(vertex *v1alpha1.Vertex, status string) error {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	vertexObj := getK8sObjectFromVertex(vertex)

	identity := common.GetGlobalIdentifier(vertexObj.Annotations, vertexObj.Labels)

	jce, ok := p.cache[identity]
	if ok {
		vertexFromNamespace, ok := jce.Vertices[vertexObj.Namespace]
		if ok {
			vertexFromNamespace.Status = status
			p.cache[jce.Identity] = jce
			return nil
		} else {
			vertexObj.Status = status
			jce.Vertices[vertex.Namespace] = vertexObj
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
	vertexNew, ok := obj.(*v1alpha1.Vertex)
	if !ok {
		return false, fmt.Errorf("type assertion failed, %v is not of type *Vertex", obj)
	}
	vertexOld, ok := oldObj.(*v1alpha1.Vertex)
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

func (p *VertexController) IsOnlyReplicaCountChanged(*log.Entry, interface{}, interface{}) (bool, error) {
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

	vertexController.Cache = newVertexCache()

	NewController("vertex-ctrl", config.Host, stopCh, &vertexController, vertexController.informer)

	return &vertexController, nil
}

func (d *VertexController) Added(ctx context.Context, obj interface{}) error {
	return addUpdateVertex(d, ctx, obj)
}

func (d *VertexController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	//Not Required, this is a no-op as Add event already handles registering this as a mesh client
	return nil
}

func addUpdateVertex(j *VertexController, ctx context.Context, obj interface{}) error {
	vertex, ok := obj.(*v1alpha1.Vertex)
	if !ok {
		return fmt.Errorf("failed to covert informer object to Vertex")
	}
	k8sObj := getK8sObjectFromVertex(vertex)
	if vertex.Spec.Metadata != nil && !common.ShouldIgnore(vertex.Spec.Metadata.Annotations, vertex.Spec.Metadata.Labels) {
		newK8sObj := j.Cache.Put(k8sObj)
		newK8sObj.Status = common.ProcessingInProgress
		return j.VertexHandler.Added(ctx, newK8sObj)
	}
	return nil
}

func (p *VertexController) Deleted(ctx context.Context, obj interface{}) error {
	//Not Required (to be handled via asset off boarding)
	return nil
}

func (d *VertexController) GetProcessItemStatus(obj interface{}) (string, error) {
	vertex, ok := obj.(*v1alpha1.Vertex)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *common.K8sObject", obj)
	}
	return d.Cache.GetVertexProcessStatus(vertex)
}

func (d *VertexController) UpdateProcessItemStatus(obj interface{}, status string) error {
	vertex, ok := obj.(*v1alpha1.Vertex)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *Vertex", obj)
	}
	return d.Cache.UpdateVertexProcessStatus(vertex, status)
}

func (d *VertexController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	vertex, ok := obj.(*v1alpha1.Vertex)
	if !ok {
		return
	}
	vetexObj := getK8sObjectFromVertex(vertex)
	if vetexObj.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.Vertex,
			vertex.Name, vertex.Namespace, "", "Value=true")
	}
}

func (j *VertexController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	vertex, ok := obj.(*v1alpha1.Vertex)
	vertexObj := getK8sObjectFromVertex(vertex)
	identity := common.GetGlobalIdentifier(vertexObj.Annotations, vertexObj.Labels)
	if ok && isRetry {
		return j.Cache.Get(identity, vertex.Namespace), nil
	}
	if ok && j.NumaflowClient != nil {
		return j.NumaflowClient.NumaflowV1alpha1().Vertices(vertex.Namespace).Get(ctx, vertex.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("kubernetes client is not initialized, txId=%s", ctx.Value("txId"))
}
