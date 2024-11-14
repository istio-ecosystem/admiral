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

//MonoVertex controller discovers monoVertexs as mesh clients (its assumed that k8s MonoVertex doesn't have any ingress communication)

type MonoVertexController struct {
	NumaflowClient    numaflow.Interface
	MonoVertexHandler ClientDiscoveryHandler
	informer          cache.SharedIndexInformer
	Cache             *monoVertexCache
}

type MonoVertexEntry struct {
	Identity     string
	MonoVertices map[string]*common.K8sObject
}

type monoVertexCache struct {
	//map of dependencies key=identity value array of onboarded identities
	cache map[string]*MonoVertexEntry
	mutex *sync.Mutex
}

func newMonoVertexCache() *monoVertexCache {
	return &monoVertexCache{
		cache: make(map[string]*MonoVertexEntry),
		mutex: &sync.Mutex{},
	}
}

func getK8sObjectFromMonoVertex(monoVertex *v1alpha1.MonoVertex) *common.K8sObject {
	labels := make(map[string]string)
	annotations := make(map[string]string)
	if monoVertex.Spec.Metadata != nil {
		labels = monoVertex.Spec.Metadata.Labels
		annotations = monoVertex.Spec.Metadata.Annotations
	}
	return &common.K8sObject{
		Name:        monoVertex.Name,
		Namespace:   monoVertex.Namespace,
		Annotations: annotations,
		Labels:      labels,
		Status:      common.NotProcessed,
		Type:        common.MonoVertex,
	}
}

func (p *monoVertexCache) Put(monoVertex *common.K8sObject) *common.K8sObject {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	identity := common.GetGlobalIdentifier(monoVertex.Annotations, monoVertex.Labels)
	existingMonoVertices := p.cache[identity]
	if existingMonoVertices == nil {
		existingMonoVertices = &MonoVertexEntry{
			Identity:     identity,
			MonoVertices: map[string]*common.K8sObject{monoVertex.Namespace: monoVertex},
		}
		p.cache[identity] = existingMonoVertices
		return monoVertex
	} else {
		monoVertexInCache := existingMonoVertices.MonoVertices[monoVertex.Namespace]
		if monoVertexInCache == nil {
			existingMonoVertices.MonoVertices[monoVertex.Namespace] = monoVertex
			p.cache[identity] = existingMonoVertices
			return monoVertex
		}
	}
	return monoVertex
}

func (p *monoVertexCache) Get(key string, namespace string) *common.K8sObject {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	jce, ok := p.cache[key]
	if ok {
		j, ok := jce.MonoVertices[namespace]
		if ok {
			return j
		}
	}
	return nil
}

func (p *monoVertexCache) GetMonoVertexProcessStatus(monoVertex *v1alpha1.MonoVertex) (string, error) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	monoVertexObj := getK8sObjectFromMonoVertex(monoVertex)
	identity := common.GetGlobalIdentifier(monoVertexObj.Annotations, monoVertexObj.Labels)

	jce, ok := p.cache[identity]
	if ok {
		monoVertexFromNamespace, ok := jce.MonoVertices[monoVertexObj.Namespace]
		if ok {
			return monoVertexFromNamespace.Status, nil
		}
	}

	return common.NotProcessed, nil
}

func (p *monoVertexCache) UpdateMonoVertexProcessStatus(monoVertex *v1alpha1.MonoVertex, status string) error {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	monoVertexObj := getK8sObjectFromMonoVertex(monoVertex)
	identity := common.GetGlobalIdentifier(monoVertexObj.Annotations, monoVertexObj.Labels)

	jce, ok := p.cache[identity]
	if ok {
		monoVertexFromNamespace, ok := jce.MonoVertices[monoVertex.Namespace]
		if ok {
			monoVertexFromNamespace.Status = status
			p.cache[jce.Identity] = jce
			return nil
		} else {
			newMonoVertex := getK8sObjectFromMonoVertex(monoVertex)
			newMonoVertex.Status = status
			jce.MonoVertices[monoVertex.Namespace] = newMonoVertex
			p.cache[jce.Identity] = jce
			return nil
		}
	}

	return fmt.Errorf(LogCacheFormat, "UpdateStatus", "MonoVertex",
		monoVertex.Name, monoVertex.Namespace, "", "nothing to update, monoVertex not found in cache")
}

func (p *MonoVertexController) DoesGenerationMatch(ctxLogger *log.Entry, obj interface{}, oldObj interface{}) (bool, error) {
	if !common.DoGenerationCheck() {
		ctxLogger.Debugf(ControllerLogFormat, "DoesGenerationMatch", "",
			fmt.Sprintf("generation check is disabled"))
		return false, nil
	}
	monoVertexNew, ok := obj.(*v1alpha1.MonoVertex)
	if !ok {
		return false, fmt.Errorf("type assertion failed, %v is not of type *MonoVertex", obj)
	}
	monoVertexOld, ok := oldObj.(*v1alpha1.MonoVertex)
	if !ok {
		return false, fmt.Errorf("type assertion failed, %v is not of type *MonoVertex", oldObj)
	}
	if monoVertexNew.Generation == monoVertexOld.Generation {
		ctxLogger.Infof(ControllerLogFormat, "DoesGenerationMatch", "",
			fmt.Sprintf("old and new generation matched for monoVertex %s", monoVertexNew.Name))
		return true, nil
	}
	return false, nil
}

func NewMonoVertexController(stopCh <-chan struct{}, handler ClientDiscoveryHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*MonoVertexController, error) {

	monoVertexController := MonoVertexController{}
	monoVertexController.MonoVertexHandler = handler

	var err error

	monoVertexController.NumaflowClient, err = clientLoader.LoadNumaflowClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller k8s client: %v", err)
	}

	monoVertexController.informer = v1alpha12.NewMonoVertexInformer(
		monoVertexController.NumaflowClient,
		meta_v1.NamespaceAll,
		resyncPeriod,
		cache.Indexers{},
	)

	monoVertexController.Cache = newMonoVertexCache()

	NewController("monoVertex-ctrl", config.Host, stopCh, &monoVertexController, monoVertexController.informer)

	return &monoVertexController, nil
}

func (d *MonoVertexController) Added(ctx context.Context, obj interface{}) error {
	return addUpdateMonoVertex(d, ctx, obj)
}

func (d *MonoVertexController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	//Not Required, this is a no-op as as Add event already handles registering this as a mesh client
	return nil
}

func addUpdateMonoVertex(j *MonoVertexController, ctx context.Context, obj interface{}) error {
	monoVertex, ok := obj.(*v1alpha1.MonoVertex)
	if !ok {
		return fmt.Errorf("failed to covert informer object to MonoVertex")
	}
	if monoVertex.Spec.Metadata != nil && !common.ShouldIgnore(monoVertex.Spec.Metadata.Annotations, monoVertex.Spec.Metadata.Labels) {
		k8sObj := getK8sObjectFromMonoVertex(monoVertex)
		newK8sObj := j.Cache.Put(k8sObj)
		newK8sObj.Status = common.ProcessingInProgress
		return j.MonoVertexHandler.Added(ctx, newK8sObj)
	}
	return nil
}

func (p *MonoVertexController) Deleted(ctx context.Context, obj interface{}) error {
	//Not Required (to be handled via asset off boarding)
	return nil
}

func (d *MonoVertexController) GetProcessItemStatus(obj interface{}) (string, error) {
	monoVertex, ok := obj.(*v1alpha1.MonoVertex)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *common.K8sObject", obj)
	}
	return d.Cache.GetMonoVertexProcessStatus(monoVertex)
}

func (d *MonoVertexController) UpdateProcessItemStatus(obj interface{}, status string) error {
	monoVertex, ok := obj.(*v1alpha1.MonoVertex)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *MonoVertex", obj)
	}
	return d.Cache.UpdateMonoVertexProcessStatus(monoVertex, status)
}

func (d *MonoVertexController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	monoVertex, ok := obj.(*v1alpha1.MonoVertex)
	if !ok {
		return
	}
	monoVertexObj := getK8sObjectFromMonoVertex(monoVertex)
	if monoVertexObj.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.MonoVertex,
			monoVertex.Name, monoVertex.Namespace, "", "Value=true")
	}
}

func (j *MonoVertexController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	monoVertex, ok := obj.(*v1alpha1.MonoVertex)
	monoVertexObj := getK8sObjectFromMonoVertex(monoVertex)
	identity := common.GetGlobalIdentifier(monoVertexObj.Annotations, monoVertexObj.Labels)
	if ok && isRetry {
		return j.Cache.Get(identity, monoVertex.Namespace), nil
	}
	if ok && j.NumaflowClient != nil {
		return j.NumaflowClient.NumaflowV1alpha1().MonoVertices(monoVertex.Namespace).Get(ctx, monoVertex.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("kubernetes client is not initialized, txId=%s", ctx.Value("txId"))
}
