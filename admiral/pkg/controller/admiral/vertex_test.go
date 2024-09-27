package admiral

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/numaproj/numaflow/pkg/apis/numaflow/v1alpha1"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"reflect"
	"sync"
	"testing"
)

func getVertex(namespace string, annotations map[string]string, labels map[string]string) *v1alpha1.Vertex {
	vertex := &v1alpha1.Vertex{}
	vertex.Namespace = namespace
	spec := v1alpha1.VertexSpec{}
	spec.Metadata = &v1alpha1.Metadata{
		Annotations: annotations,
		Labels:      labels,
	}
	vertex.Spec = spec
	return vertex
}

func TestVertexController_Added(t *testing.T) {
	common.ResetSync()
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
			DeploymentAnnotation:    "sidecar.istio.io/inject",
			AdmiralIgnoreLabel:      "admiral-ignore",
		},
	}
	common.InitializeConfig(admiralParams)
	ctx := context.Background()
	ctx = context.WithValue(ctx, "clusterId", "test-cluster-k8s")
	//Vertexs with the correct label are added to the cache
	mdh := test.MockClientDiscoveryHandler{}
	cache := vertexCache{
		cache: map[string]*VertexEntry{},
		mutex: &sync.Mutex{},
	}
	vertexController := VertexController{
		VertexHandler: &mdh,
		Cache:         &cache,
	}
	vertex := getVertex("vertex-ns", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "vertex", "istio-injected": "true"})
	vertexWithBadLabels := getVertex("vertexWithBadLabels-ns", map[string]string{"admiral.io/env": "dev"}, map[string]string{"identity": "vertexWithBadLabels", "random-label": "true"})
	vertexWithIgnoreLabels := getVertex("vertexWithIgnoreLabels-ns", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "vertexWithIgnoreLabels", "istio-injected": "true", "admiral-ignore": "true"})
	vertexWithIgnoreAnnotations := getVertex("vertexWithIgnoreAnnotations-ns", map[string]string{"admiral.io/ignore": "true"}, map[string]string{"identity": "vertexWithIgnoreAnnotations"})
	vertexWithIgnoreAnnotations.Annotations = map[string]string{"admiral.io/ignore": "true"}

	testCases := []struct {
		name                  string
		vertex                *v1alpha1.Vertex
		expectedVertex        *common.K8sObject
		id                    string
		expectedCacheContains bool
	}{
		{
			name:                  "Expects vertex to be added to the cache when the correct label is present",
			vertex:                vertex,
			expectedVertex:        getK8sObjectFromVertex(vertex),
			id:                    "vertex",
			expectedCacheContains: true,
		},
		{
			name:                  "Expects vertex to not be added to the cache when the correct label is not present",
			vertex:                vertexWithBadLabels,
			expectedVertex:        nil,
			id:                    "vertexWithBadLabels",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored vertex identified by label to not be added to the cache",
			vertex:                vertexWithIgnoreLabels,
			expectedVertex:        nil,
			id:                    "vertexWithIgnoreLabels",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored vertex identified by vertex annotation to not be added to the cache",
			vertex:                vertexWithIgnoreAnnotations,
			expectedVertex:        nil,
			id:                    "vertexWithIgnoreAnnotations",
			expectedCacheContains: false,
		},
	}
	ns := coreV1.Namespace{}
	ns.Name = "test-ns"
	ns.Annotations = map[string]string{"admiral.io/ignore": "true"}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "Expects ignored vertex identified by label to be removed from the cache" {
				vertex.Spec.Metadata.Labels["admiral-ignore"] = "true"
			}
			vertexController.Added(ctx, c.vertex)
			vertexClusterEntry := vertexController.Cache.cache[c.id]
			var vertexsMap map[string]*common.K8sObject = nil
			if vertexClusterEntry != nil {
				vertexsMap = vertexClusterEntry.Vertices
			}
			var vertexObj *common.K8sObject = nil
			if vertexsMap != nil && len(vertexsMap) > 0 {
				vertexObj = vertexsMap[c.vertex.Namespace]
			}
			if !reflect.DeepEqual(c.expectedVertex, vertexObj) {
				t.Errorf("Expected rollout %+v but got %+v", c.expectedVertex, vertexObj)
			}
		})
	}
}

func TestVertexController_CacheGet(t *testing.T) {
	common.ResetSync()
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
			DeploymentAnnotation:    "sidecar.istio.io/inject",
			AdmiralIgnoreLabel:      "admiral-ignore",
		},
	}
	common.InitializeConfig(admiralParams)
	ctx := context.Background()
	ctx = context.WithValue(ctx, "clusterId", "test-cluster-k8s")

	cache := vertexCache{
		cache: map[string]*VertexEntry{},
		mutex: &sync.Mutex{},
	}

	vertex := getVertex("vertex-ns", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "vertex1", "istio-injected": "true"})
	cache.Put(getK8sObjectFromVertex(vertex))
	vertexSameIdentityInDiffNamespace := getVertex("vertex-ns-2", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "vertex1", "istio-injected": "true"})
	cache.Put(getK8sObjectFromVertex(vertexSameIdentityInDiffNamespace))
	vertex2 := getVertex("vertex-ns-3", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "vertex2", "istio-injected": "true"})
	cache.Put(getK8sObjectFromVertex(vertex2))

	testCases := []struct {
		name           string
		expectedVertex *common.K8sObject
		identity       string
		namespace      string
	}{
		{
			name:           "Expects vertex to be in the cache when right identity and namespace are passed",
			expectedVertex: getK8sObjectFromVertex(vertex),
			identity:       "vertex1",
			namespace:      "vertex-ns",
		},
		{
			name:           "Expects vertex to be in the cache when same identity and diff namespace are passed",
			expectedVertex: getK8sObjectFromVertex(vertexSameIdentityInDiffNamespace),
			identity:       "vertex1",
			namespace:      "vertex-ns-2",
		},
		{
			name:           "Expects vertex to be in the cache when diff identity and diff namespace are passed",
			expectedVertex: getK8sObjectFromVertex(vertex2),
			identity:       "vertex2",
			namespace:      "vertex-ns-3",
		},
		{
			name:           "Expects nil vertex in random namespace",
			expectedVertex: nil,
			identity:       "vertex2",
			namespace:      "random",
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "Expects ignored vertex identified by label to be removed from the cache" {
				vertex.Spec.Metadata.Labels["admiral-ignore"] = "true"
			}
			vertexObj := cache.Get(c.identity, c.namespace)
			if !reflect.DeepEqual(c.expectedVertex, vertexObj) {
				t.Errorf("Expected rollout %+v but got %+v", c.expectedVertex, vertexObj)
			}
		})
	}
}

func TestVertexControlle_DoesGenerationMatch(t *testing.T) {
	dc := VertexController{}

	admiralParams := common.AdmiralParams{}

	testCases := []struct {
		name                  string
		vertexNew             interface{}
		vertexOld             interface{}
		enableGenerationCheck bool
		expectedValue         bool
		expectedError         error
	}{
		{
			name: "Given context, new vertex and old vertex object " +
				"When new vertex is not of type *v1.Vertex " +
				"Then func should return an error",
			vertexNew:             struct{}{},
			vertexOld:             struct{}{},
			enableGenerationCheck: true,
			expectedError:         fmt.Errorf("type assertion failed, {} is not of type *Vertex"),
		},
		{
			name: "Given context, new vertex and old vertex object " +
				"When old vertex is not of type *v1.Vertex " +
				"Then func should return an error",
			vertexNew:             &v1alpha1.Vertex{},
			vertexOld:             struct{}{},
			enableGenerationCheck: true,
			expectedError:         fmt.Errorf("type assertion failed, {} is not of type *Vertex"),
		},
		{
			name: "Given context, new vertex and old vertex object " +
				"When vertex generation check is enabled but the generation does not match " +
				"Then func should return false ",
			vertexNew: &v1alpha1.Vertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			vertexOld: &v1alpha1.Vertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 1,
				},
			},
			enableGenerationCheck: true,
			expectedError:         nil,
		},
		{
			name: "Given context, new vertex and old vertex object " +
				"When vertex generation check is disabled " +
				"Then func should return false ",
			vertexNew: &v1alpha1.Vertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			vertexOld: &v1alpha1.Vertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 1,
				},
			},
			expectedError: nil,
		},
		{
			name: "Given context, new vertex and old vertex object " +
				"When vertex generation check is enabled and the old and new vertex generation is equal " +
				"Then func should just return true",
			vertexNew: &v1alpha1.Vertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			vertexOld: &v1alpha1.Vertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			enableGenerationCheck: true,
			expectedError:         nil,
			expectedValue:         true,
		},
	}

	ctxLogger := log.WithFields(log.Fields{
		"txId": "abc",
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			admiralParams.EnableGenerationCheck = tc.enableGenerationCheck
			common.ResetSync()
			common.InitializeConfig(admiralParams)
			actual, err := dc.DoesGenerationMatch(ctxLogger, tc.vertexNew, tc.vertexOld)
			if !ErrorEqualOrSimilar(err, tc.expectedError) {
				t.Errorf("expected: %v, got: %v", tc.expectedError, err)
			}
			if err == nil {
				if tc.expectedValue != actual {
					t.Errorf("expected: %v, got: %v", tc.expectedValue, actual)
				}
			}
		})
	}

}

func TestNewVertexController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	vertexHandler := test.MockClientDiscoveryHandler{}

	vertexCon, _ := NewVertexController(stop, &vertexHandler, config, 0, loader.GetFakeClientLoader())

	if vertexCon == nil {
		t.Errorf("Vertex controller should not be nil")
	}
}

func TestVertexUpdateProcessItemStatus(t *testing.T) {
	common.ResetSync()
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(admiralParams)
	vertexInCache := getVertex("namespace-1", map[string]string{}, map[string]string{"identity": "vertex1", "env": "prd"})
	vertexInCache.Name = "vertex1"
	vertexInCache2 := getVertex("namespace-2", map[string]string{}, map[string]string{"identity": "vertex2", "env": "prd"})
	vertexInCache.Name = "vertex2"
	vertexNotInCache := getVertex("namespace-3", map[string]string{}, map[string]string{"identity": "vertex3", "env": "prd"})
	vertexInCache.Name = "vertex2"
	var (
		serviceAccount = &coreV1.ServiceAccount{}
	)

	// Populating the vertex Cache
	vertexCache := &vertexCache{
		cache: make(map[string]*VertexEntry),
		mutex: &sync.Mutex{},
	}

	vertexController := &VertexController{
		Cache: vertexCache,
	}

	vertexCache.Put(getK8sObjectFromVertex(vertexInCache))
	vertexCache.Put(getK8sObjectFromVertex(vertexInCache2))

	testCases := []struct {
		name           string
		obj            interface{}
		statusToSet    string
		expectedErr    error
		expectedStatus string
	}{
		{
			name: "Given vertex cache has a valid vertex in its cache, " +
				"And is processed" +
				"Then, the status for the valid vertex should be updated to processed",
			obj:            vertexInCache,
			statusToSet:    common.Processed,
			expectedErr:    nil,
			expectedStatus: common.Processed,
		},
		{
			name: "Given vertex cache has a valid vertex in its cache, " +
				"And is processed" +
				"Then, the status for the valid vertex should be updated to not processed",
			obj:            vertexInCache2,
			statusToSet:    common.NotProcessed,
			expectedErr:    nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given vertex cache does not has a valid vertex in its cache, " +
				"Then, the status for the valid vertex should be not processed, " +
				"And an error should be returned with the vertex not found message",
			obj:            vertexNotInCache,
			statusToSet:    common.NotProcessed,
			expectedErr:    fmt.Errorf(LogCacheFormat, "UpdateStatus", "Vertex", vertexNotInCache.Name, vertexNotInCache.Namespace, "", "nothing to update, vertex not found in cache"),
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:            serviceAccount,
			expectedErr:    fmt.Errorf("type assertion failed"),
			expectedStatus: common.NotProcessed,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := vertexController.UpdateProcessItemStatus(c.obj, c.statusToSet)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			status, _ := vertexController.GetProcessItemStatus(c.obj)
			assert.Equal(t, c.expectedStatus, status)
		})
	}
}

func TestVertexGetProcessItemStatus(t *testing.T) {
	common.ResetSync()
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(admiralParams)
	var (
		serviceAccount = &coreV1.ServiceAccount{}
	)
	vertexInCache := getVertex("namespace-1", map[string]string{}, map[string]string{"identity": "vertex1"})
	vertexInCache.Name = "debug-1"
	vertexNotInCache := getVertex("namespace-2", map[string]string{}, map[string]string{"identity": "vertex2"})
	vertexNotInCache.Name = "debug-2"

	// Populating the vertex Cache
	vertexCache := &vertexCache{
		cache: make(map[string]*VertexEntry),
		mutex: &sync.Mutex{},
	}

	vertexController := &VertexController{
		Cache: vertexCache,
	}

	vertexCache.Put(getK8sObjectFromVertex(vertexInCache))
	vertexCache.UpdateVertexProcessStatus(vertexInCache, common.Processed)

	testCases := []struct {
		name           string
		obj            interface{}
		expectedErr    error
		expectedResult string
	}{
		{
			name: "Given vertex cache has a valid vertex in its cache, " +
				"And is processed" +
				"Then, we should be able to get the status as processed",
			obj:            vertexInCache,
			expectedResult: common.Processed,
		},
		{
			name: "Given vertex cache does not has a valid vertex in its cache, " +
				"Then, the status for the valid vertex should not be updated",
			obj:            vertexNotInCache,
			expectedResult: common.NotProcessed,
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:            serviceAccount,
			expectedErr:    fmt.Errorf("type assertion failed"),
			expectedResult: common.NotProcessed,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			res, err := vertexController.GetProcessItemStatus(c.obj)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			assert.Equal(t, c.expectedResult, res)
		})
	}
}

func TestVertexLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a Vertex object
	d := &VertexController{}
	d.LogValueOfAdmiralIoIgnore("not a vertex")
	// No error should occur

	// Test case 2: K8sClient is nil
	d = &VertexController{}
	d.LogValueOfAdmiralIoIgnore(&v1alpha1.Vertex{})
	// No error should occur

	d.LogValueOfAdmiralIoIgnore(&v1alpha1.Vertex{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}})
	// No error should occur

	// Test case 3: AdmiralIgnoreAnnotation is set in Vertex object
	d = &VertexController{}
	vertex := &v1alpha1.Vertex{
		Spec: v1alpha1.VertexSpec{
			AbstractVertex: v1alpha1.AbstractVertex{
				AbstractPodTemplate: v1alpha1.AbstractPodTemplate{
					Metadata: &v1alpha1.Metadata{
						Annotations: map[string]string{
							common.AdmiralIgnoreAnnotation: "true",
						},
					},
				},
			},
		},
	}
	d.LogValueOfAdmiralIoIgnore(vertex)
	// No error should occur
}
