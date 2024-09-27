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

func getMonoVertex(namespace string, annotations map[string]string, labels map[string]string) *v1alpha1.MonoVertex {
	monomonoVertex := &v1alpha1.MonoVertex{}
	monomonoVertex.Namespace = namespace
	spec := v1alpha1.MonoVertexSpec{}
	spec.Metadata = &v1alpha1.Metadata{
		Annotations: annotations,
		Labels:      labels,
	}
	monomonoVertex.Spec = spec
	return monomonoVertex
}

func TestMonoVertexController_Added(t *testing.T) {
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
	//MonoVertexs with the correct label are added to the cache
	mdh := test.MockClientDiscoveryHandler{}
	cache := monoVertexCache{
		cache: map[string]*MonoVertexEntry{},
		mutex: &sync.Mutex{},
	}
	monomonoVertexController := MonoVertexController{
		MonoVertexHandler: &mdh,
		Cache:             &cache,
	}
	monomonoVertex := getMonoVertex("monomonoVertex-ns", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "monomonoVertex", "istio-injected": "true"})
	monomonoVertexWithBadLabels := getMonoVertex("monomonoVertexWithBadLabels-ns", map[string]string{"admiral.io/env": "dev"}, map[string]string{"identity": "monomonoVertexWithBadLabels", "random-label": "true"})
	monomonoVertexWithIgnoreLabels := getMonoVertex("monomonoVertexWithIgnoreLabels-ns", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "monomonoVertexWithIgnoreLabels", "istio-injected": "true", "admiral-ignore": "true"})
	monomonoVertexWithIgnoreAnnotations := getMonoVertex("monomonoVertexWithIgnoreAnnotations-ns", map[string]string{"admiral.io/ignore": "true"}, map[string]string{"identity": "monomonoVertexWithIgnoreAnnotations"})
	monomonoVertexWithIgnoreAnnotations.Annotations = map[string]string{"admiral.io/ignore": "true"}

	testCases := []struct {
		name                  string
		monomonoVertex        *v1alpha1.MonoVertex
		expectedMonoVertex    *common.K8sObject
		id                    string
		expectedCacheContains bool
	}{
		{
			name:                  "Expects monomonoVertex to be added to the cache when the correct label is present",
			monomonoVertex:        monomonoVertex,
			expectedMonoVertex:    getK8sObjectFromMonoVertex(monomonoVertex),
			id:                    "monomonoVertex",
			expectedCacheContains: true,
		},
		{
			name:                  "Expects monomonoVertex to not be added to the cache when the correct label is not present",
			monomonoVertex:        monomonoVertexWithBadLabels,
			expectedMonoVertex:    nil,
			id:                    "monomonoVertexWithBadLabels",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored monomonoVertex identified by label to not be added to the cache",
			monomonoVertex:        monomonoVertexWithIgnoreLabels,
			expectedMonoVertex:    nil,
			id:                    "monomonoVertexWithIgnoreLabels",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored monomonoVertex identified by monomonoVertex annotation to not be added to the cache",
			monomonoVertex:        monomonoVertexWithIgnoreAnnotations,
			expectedMonoVertex:    nil,
			id:                    "monomonoVertexWithIgnoreAnnotations",
			expectedCacheContains: false,
		},
	}
	ns := coreV1.Namespace{}
	ns.Name = "test-ns"
	ns.Annotations = map[string]string{"admiral.io/ignore": "true"}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "Expects ignored monomonoVertex identified by label to be removed from the cache" {
				monomonoVertex.Spec.Metadata.Labels["admiral-ignore"] = "true"
			}
			monomonoVertexController.Added(ctx, c.monomonoVertex)
			monomonoVertexClusterEntry := monomonoVertexController.Cache.cache[c.id]
			var monomonoVertexsMap map[string]*common.K8sObject = nil
			if monomonoVertexClusterEntry != nil {
				monomonoVertexsMap = monomonoVertexClusterEntry.MonoVertices
			}
			var monomonoVertexObj *common.K8sObject = nil
			if monomonoVertexsMap != nil && len(monomonoVertexsMap) > 0 {
				monomonoVertexObj = monomonoVertexsMap[c.monomonoVertex.Namespace]
			}
			if !reflect.DeepEqual(c.expectedMonoVertex, monomonoVertexObj) {
				t.Errorf("Expected rollout %+v but got %+v", c.expectedMonoVertex, monomonoVertexObj)
			}
		})
	}
}

func TestMonoVertexControlle_DoesGenerationMatch(t *testing.T) {
	dc := MonoVertexController{}

	admiralParams := common.AdmiralParams{}

	testCases := []struct {
		name                  string
		monomonoVertexNew     interface{}
		monomonoVertexOld     interface{}
		enableGenerationCheck bool
		expectedValue         bool
		expectedError         error
	}{
		{
			name: "Given context, new monomonoVertex and old monomonoVertex object " +
				"When new monomonoVertex is not of type *v1.MonoVertex " +
				"Then func should return an error",
			monomonoVertexNew:     struct{}{},
			monomonoVertexOld:     struct{}{},
			enableGenerationCheck: true,
			expectedError:         fmt.Errorf("type assertion failed, {} is not of type *MonoVertex"),
		},
		{
			name: "Given context, new monomonoVertex and old monomonoVertex object " +
				"When old monomonoVertex is not of type *v1.MonoVertex " +
				"Then func should return an error",
			monomonoVertexNew:     &v1alpha1.MonoVertex{},
			monomonoVertexOld:     struct{}{},
			enableGenerationCheck: true,
			expectedError:         fmt.Errorf("type assertion failed, {} is not of type *MonoVertex"),
		},
		{
			name: "Given context, new monomonoVertex and old monomonoVertex object " +
				"When monomonoVertex generation check is enabled but the generation does not match " +
				"Then func should return false ",
			monomonoVertexNew: &v1alpha1.MonoVertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			monomonoVertexOld: &v1alpha1.MonoVertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 1,
				},
			},
			enableGenerationCheck: true,
			expectedError:         nil,
		},
		{
			name: "Given context, new monomonoVertex and old monomonoVertex object " +
				"When monomonoVertex generation check is disabled " +
				"Then func should return false ",
			monomonoVertexNew: &v1alpha1.MonoVertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			monomonoVertexOld: &v1alpha1.MonoVertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 1,
				},
			},
			expectedError: nil,
		},
		{
			name: "Given context, new monomonoVertex and old monomonoVertex object " +
				"When monomonoVertex generation check is enabled and the old and new monomonoVertex generation is equal " +
				"Then func should just return true",
			monomonoVertexNew: &v1alpha1.MonoVertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			monomonoVertexOld: &v1alpha1.MonoVertex{
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
			actual, err := dc.DoesGenerationMatch(ctxLogger, tc.monomonoVertexNew, tc.monomonoVertexOld)
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

func TestNewMonoVertexController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	monomonoVertexHandler := test.MockClientDiscoveryHandler{}

	monomonoVertexCon, _ := NewMonoVertexController(stop, &monomonoVertexHandler, config, 0, loader.GetFakeClientLoader())

	if monomonoVertexCon == nil {
		t.Errorf("MonoVertex controller should not be nil")
	}
}

func TestMonoVertexUpdateProcessItemStatus(t *testing.T) {
	common.ResetSync()
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(admiralParams)
	monomonoVertexInCache := getMonoVertex("namespace-1", map[string]string{}, map[string]string{"identity": "monomonoVertex1", "env": "prd"})
	monomonoVertexInCache.Name = "monomonoVertex1"
	monomonoVertexInCache2 := getMonoVertex("namespace-2", map[string]string{}, map[string]string{"identity": "monomonoVertex2", "env": "prd"})
	monomonoVertexInCache2.Name = "monomonoVertex2"
	monomonoVertexNotInCache := getMonoVertex("namespace-3", map[string]string{}, map[string]string{"identity": "monomonoVertex3", "env": "prd"})
	monomonoVertexNotInCache.Name = "monomonoVertex3"
	var (
		serviceAccount = &coreV1.ServiceAccount{}
	)

	// Populating the monomonoVertex Cache
	monomonoVertexCache := &monoVertexCache{
		cache: make(map[string]*MonoVertexEntry),
		mutex: &sync.Mutex{},
	}

	monomonoVertexController := &MonoVertexController{
		Cache: monomonoVertexCache,
	}

	monomonoVertexCache.Put(getK8sObjectFromMonoVertex(monomonoVertexInCache))
	monomonoVertexCache.Put(getK8sObjectFromMonoVertex(monomonoVertexInCache2))

	testCases := []struct {
		name           string
		obj            interface{}
		statusToSet    string
		expectedErr    error
		expectedStatus string
	}{
		{
			name: "Given monomonoVertex cache has a valid monomonoVertex in its cache, " +
				"And is processed" +
				"Then, the status for the valid monomonoVertex should be updated to processed",
			obj:            monomonoVertexInCache,
			statusToSet:    common.Processed,
			expectedErr:    nil,
			expectedStatus: common.Processed,
		},
		{
			name: "Given monomonoVertex cache has a valid monomonoVertex in its cache, " +
				"And is processed" +
				"Then, the status for the valid monomonoVertex should be updated to not processed",
			obj:            monomonoVertexInCache2,
			statusToSet:    common.NotProcessed,
			expectedErr:    nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given monomonoVertex cache does not has a valid monomonoVertex in its cache, " +
				"Then, the status for the valid monomonoVertex should be not processed, " +
				"And an error should be returned with the monomonoVertex not found message",
			obj:            monomonoVertexNotInCache,
			statusToSet:    common.NotProcessed,
			expectedErr:    fmt.Errorf(LogCacheFormat, "UpdateStatus", "MonoVertex", monomonoVertexNotInCache.Name, monomonoVertexNotInCache.Namespace, "", "nothing to update, monoVertex not found in cache"),
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
			err := monomonoVertexController.UpdateProcessItemStatus(c.obj, c.statusToSet)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			status, _ := monomonoVertexController.GetProcessItemStatus(c.obj)
			assert.Equal(t, c.expectedStatus, status)
		})
	}
}

func TestMonoVertexGetProcessItemStatus(t *testing.T) {
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
	monomonoVertexInCache := getMonoVertex("namespace-1", map[string]string{}, map[string]string{"identity": "monomonoVertex1"})
	monomonoVertexInCache.Name = "debug-1"
	monomonoVertexNotInCache := getMonoVertex("namespace-2", map[string]string{}, map[string]string{"identity": "monomonoVertex2"})
	monomonoVertexNotInCache.Name = "debug-2"

	// Populating the monomonoVertex Cache
	monomonoVertexCache := &monoVertexCache{
		cache: make(map[string]*MonoVertexEntry),
		mutex: &sync.Mutex{},
	}

	monomonoVertexController := &MonoVertexController{
		Cache: monomonoVertexCache,
	}

	monomonoVertexCache.Put(getK8sObjectFromMonoVertex(monomonoVertexInCache))
	monomonoVertexCache.UpdateMonoVertexProcessStatus(monomonoVertexInCache, common.Processed)

	testCases := []struct {
		name           string
		obj            interface{}
		expectedErr    error
		expectedResult string
	}{
		{
			name: "Given monomonoVertex cache has a valid monomonoVertex in its cache, " +
				"And is processed" +
				"Then, we should be able to get the status as processed",
			obj:            monomonoVertexInCache,
			expectedResult: common.Processed,
		},
		{
			name: "Given monomonoVertex cache does not has a valid monomonoVertex in its cache, " +
				"Then, the status for the valid monomonoVertex should not be updated",
			obj:            monomonoVertexNotInCache,
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
			res, err := monomonoVertexController.GetProcessItemStatus(c.obj)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			assert.Equal(t, c.expectedResult, res)
		})
	}
}

func TestMonoVertexLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a MonoVertex object
	d := &MonoVertexController{}
	d.LogValueOfAdmiralIoIgnore("not a monomonoVertex")
	// No error should occur

	// Test case 2: K8sClient is nil
	d = &MonoVertexController{}
	d.LogValueOfAdmiralIoIgnore(&v1alpha1.MonoVertex{})
	// No error should occur

	d.LogValueOfAdmiralIoIgnore(&v1alpha1.MonoVertex{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}})
	// No error should occur

	// Test case 3: AdmiralIgnoreAnnotation is set in MonoVertex object
	d = &MonoVertexController{}
	monomonoVertex := &v1alpha1.MonoVertex{
		Spec: v1alpha1.MonoVertexSpec{
			AbstractPodTemplate: v1alpha1.AbstractPodTemplate{
				Metadata: &v1alpha1.Metadata{
					Annotations: map[string]string{
						common.AdmiralIgnoreAnnotation: "true",
					},
				},
			},
		},
	}
	d.LogValueOfAdmiralIoIgnore(monomonoVertex)
	// No error should occur
}

func TestMonoVertexController_CacheGet(t *testing.T) {
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

	cache := monoVertexCache{
		cache: map[string]*MonoVertexEntry{},
		mutex: &sync.Mutex{},
	}

	monoVertex := getMonoVertex("monoVertex-ns", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "monoVertex1", "istio-injected": "true"})
	cache.Put(getK8sObjectFromMonoVertex(monoVertex))
	monoVertexSameIdentityInDiffNamespace := getMonoVertex("monoVertex-ns-2", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "monoVertex1", "istio-injected": "true"})
	cache.Put(getK8sObjectFromMonoVertex(monoVertexSameIdentityInDiffNamespace))
	monoVertex2 := getMonoVertex("monoVertex-ns-3", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "monoVertex2", "istio-injected": "true"})
	cache.Put(getK8sObjectFromMonoVertex(monoVertex2))

	testCases := []struct {
		name           string
		expectedVertex *common.K8sObject
		identity       string
		namespace      string
	}{
		{
			name:           "Expects monoVertex to be in the cache when right identity and namespace are passed",
			expectedVertex: getK8sObjectFromMonoVertex(monoVertex),
			identity:       "monoVertex1",
			namespace:      "monoVertex-ns",
		},
		{
			name:           "Expects monoVertex to be in the cache when same identity and diff namespace are passed",
			expectedVertex: getK8sObjectFromMonoVertex(monoVertexSameIdentityInDiffNamespace),
			identity:       "monoVertex1",
			namespace:      "monoVertex-ns-2",
		},
		{
			name:           "Expects monoVertex to be in the cache when diff identity and diff namespace are passed",
			expectedVertex: getK8sObjectFromMonoVertex(monoVertex2),
			identity:       "monoVertex2",
			namespace:      "monoVertex-ns-3",
		},
		{
			name:           "Expects nil monoVertex in random namespace",
			expectedVertex: nil,
			identity:       "monoVertex2",
			namespace:      "random",
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "Expects ignored monoVertex identified by label to be removed from the cache" {
				monoVertex.Spec.Metadata.Labels["admiral-ignore"] = "true"
			}
			monoVertexObj := cache.Get(c.identity, c.namespace)
			if !reflect.DeepEqual(c.expectedVertex, monoVertexObj) {
				t.Errorf("Expected rollout %+v but got %+v", c.expectedVertex, monoVertexObj)
			}
		})
	}
}
