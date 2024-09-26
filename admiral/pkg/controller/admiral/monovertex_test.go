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
	monovertex := &v1alpha1.MonoVertex{}
	monovertex.Namespace = namespace
	spec := v1alpha1.MonoVertexSpec{}
	spec.Metadata = &v1alpha1.Metadata{
		Annotations: annotations,
		Labels:      labels,
	}
	monovertex.Spec = spec
	return monovertex
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
	monovertexController := MonoVertexController{
		MonoVertexHandler: &mdh,
		Cache:             &cache,
	}
	monovertex := getMonoVertex("monovertex-ns", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "monovertex", "istio-injected": "true"})
	monovertexWithBadLabels := getMonoVertex("monovertexWithBadLabels-ns", map[string]string{"admiral.io/env": "dev"}, map[string]string{"identity": "monovertexWithBadLabels", "random-label": "true"})
	monovertexWithIgnoreLabels := getMonoVertex("monovertexWithIgnoreLabels-ns", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "monovertexWithIgnoreLabels", "istio-injected": "true", "admiral-ignore": "true"})
	monovertexWithIgnoreAnnotations := getMonoVertex("monovertexWithIgnoreAnnotations-ns", map[string]string{"admiral.io/ignore": "true"}, map[string]string{"identity": "monovertexWithIgnoreAnnotations"})
	monovertexWithIgnoreAnnotations.Annotations = map[string]string{"admiral.io/ignore": "true"}

	testCases := []struct {
		name                  string
		monovertex            *v1alpha1.MonoVertex
		expectedMonoVertex    *common.K8sObject
		id                    string
		expectedCacheContains bool
	}{
		{
			name:                  "Expects monovertex to be added to the cache when the correct label is present",
			monovertex:            monovertex,
			expectedMonoVertex:    getK8sObjectFromMonoVertex(monovertex),
			id:                    "monovertex",
			expectedCacheContains: true,
		},
		{
			name:                  "Expects monovertex to not be added to the cache when the correct label is not present",
			monovertex:            monovertexWithBadLabels,
			expectedMonoVertex:    nil,
			id:                    "monovertexWithBadLabels",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored monovertex identified by label to not be added to the cache",
			monovertex:            monovertexWithIgnoreLabels,
			expectedMonoVertex:    nil,
			id:                    "monovertexWithIgnoreLabels",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored monovertex identified by monovertex annotation to not be added to the cache",
			monovertex:            monovertexWithIgnoreAnnotations,
			expectedMonoVertex:    nil,
			id:                    "monovertexWithIgnoreAnnotations",
			expectedCacheContains: false,
		},
	}
	ns := coreV1.Namespace{}
	ns.Name = "test-ns"
	ns.Annotations = map[string]string{"admiral.io/ignore": "true"}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "Expects ignored monovertex identified by label to be removed from the cache" {
				monovertex.Spec.Metadata.Labels["admiral-ignore"] = "true"
			}
			monovertexController.Added(ctx, c.monovertex)
			monovertexClusterEntry := monovertexController.Cache.cache[c.id]
			var monovertexsMap map[string]*common.K8sObject = nil
			if monovertexClusterEntry != nil {
				monovertexsMap = monovertexClusterEntry.MonoVertices
			}
			var monovertexObj *common.K8sObject = nil
			if monovertexsMap != nil && len(monovertexsMap) > 0 {
				monovertexObj = monovertexsMap[c.monovertex.Namespace]
			}
			if !reflect.DeepEqual(c.expectedMonoVertex, monovertexObj) {
				t.Errorf("Expected rollout %+v but got %+v", c.expectedMonoVertex, monovertexObj)
			}
		})
	}
}

func TestMonoVertexControlle_DoesGenerationMatch(t *testing.T) {
	dc := MonoVertexController{}

	admiralParams := common.AdmiralParams{}

	testCases := []struct {
		name                  string
		monovertexNew         interface{}
		monovertexOld         interface{}
		enableGenerationCheck bool
		expectedValue         bool
		expectedError         error
	}{
		{
			name: "Given context, new monovertex and old monovertex object " +
				"When new monovertex is not of type *v1.MonoVertex " +
				"Then func should return an error",
			monovertexNew:         struct{}{},
			monovertexOld:         struct{}{},
			enableGenerationCheck: true,
			expectedError:         fmt.Errorf("type assertion failed, {} is not of type *MonoVertex"),
		},
		{
			name: "Given context, new monovertex and old monovertex object " +
				"When old monovertex is not of type *v1.MonoVertex " +
				"Then func should return an error",
			monovertexNew:         &v1alpha1.MonoVertex{},
			monovertexOld:         struct{}{},
			enableGenerationCheck: true,
			expectedError:         fmt.Errorf("type assertion failed, {} is not of type *MonoVertex"),
		},
		{
			name: "Given context, new monovertex and old monovertex object " +
				"When monovertex generation check is enabled but the generation does not match " +
				"Then func should return false ",
			monovertexNew: &v1alpha1.MonoVertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			monovertexOld: &v1alpha1.MonoVertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 1,
				},
			},
			enableGenerationCheck: true,
			expectedError:         nil,
		},
		{
			name: "Given context, new monovertex and old monovertex object " +
				"When monovertex generation check is disabled " +
				"Then func should return false ",
			monovertexNew: &v1alpha1.MonoVertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			monovertexOld: &v1alpha1.MonoVertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 1,
				},
			},
			expectedError: nil,
		},
		{
			name: "Given context, new monovertex and old monovertex object " +
				"When monovertex generation check is enabled and the old and new monovertex generation is equal " +
				"Then func should just return true",
			monovertexNew: &v1alpha1.MonoVertex{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			monovertexOld: &v1alpha1.MonoVertex{
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
			actual, err := dc.DoesGenerationMatch(ctxLogger, tc.monovertexNew, tc.monovertexOld)
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
	monovertexHandler := test.MockClientDiscoveryHandler{}

	monovertexCon, _ := NewMonoVertexController(stop, &monovertexHandler, config, 0, loader.GetFakeClientLoader())

	if monovertexCon == nil {
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
	monovertexInCache := getMonoVertex("namespace-1", map[string]string{}, map[string]string{"identity": "monovertex1", "env": "prd"})
	monovertexInCache.Name = "monovertex1"
	monovertexInCache2 := getMonoVertex("namespace-2", map[string]string{}, map[string]string{"identity": "monovertex2", "env": "prd"})
	monovertexInCache2.Name = "monovertex2"
	monovertexNotInCache := getMonoVertex("namespace-3", map[string]string{}, map[string]string{"identity": "monovertex3", "env": "prd"})
	monovertexNotInCache.Name = "monovertex3"
	var (
		serviceAccount = &coreV1.ServiceAccount{}
	)

	// Populating the monovertex Cache
	monovertexCache := &monoVertexCache{
		cache: make(map[string]*MonoVertexEntry),
		mutex: &sync.Mutex{},
	}

	monovertexController := &MonoVertexController{
		Cache: monovertexCache,
	}

	monovertexCache.Put(getK8sObjectFromMonoVertex(monovertexInCache))
	monovertexCache.Put(getK8sObjectFromMonoVertex(monovertexInCache2))

	testCases := []struct {
		name           string
		obj            interface{}
		statusToSet    string
		expectedErr    error
		expectedStatus string
	}{
		{
			name: "Given monovertex cache has a valid monovertex in its cache, " +
				"And is processed" +
				"Then, the status for the valid monovertex should be updated to processed",
			obj:            monovertexInCache,
			statusToSet:    common.Processed,
			expectedErr:    nil,
			expectedStatus: common.Processed,
		},
		{
			name: "Given monovertex cache has a valid monovertex in its cache, " +
				"And is processed" +
				"Then, the status for the valid monovertex should be updated to not processed",
			obj:            monovertexInCache2,
			statusToSet:    common.NotProcessed,
			expectedErr:    nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given monovertex cache does not has a valid monovertex in its cache, " +
				"Then, the status for the valid monovertex should be not processed, " +
				"And an error should be returned with the monovertex not found message",
			obj:            monovertexNotInCache,
			statusToSet:    common.NotProcessed,
			expectedErr:    fmt.Errorf(LogCacheFormat, "UpdateStatus", "MonoVertex", monovertexNotInCache.Name, monovertexNotInCache.Namespace, "", "nothing to update, monoVertex not found in cache"),
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
			err := monovertexController.UpdateProcessItemStatus(c.obj, c.statusToSet)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			status, _ := monovertexController.GetProcessItemStatus(c.obj)
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
	monovertexInCache := getMonoVertex("namespace-1", map[string]string{}, map[string]string{"identity": "monovertex1"})
	monovertexInCache.Name = "debug-1"
	monovertexNotInCache := getMonoVertex("namespace-2", map[string]string{}, map[string]string{"identity": "monovertex2"})
	monovertexNotInCache.Name = "debug-2"

	// Populating the monovertex Cache
	monovertexCache := &monoVertexCache{
		cache: make(map[string]*MonoVertexEntry),
		mutex: &sync.Mutex{},
	}

	monovertexController := &MonoVertexController{
		Cache: monovertexCache,
	}

	monovertexCache.Put(getK8sObjectFromMonoVertex(monovertexInCache))
	monovertexCache.UpdateMonoVertexProcessStatus(monovertexInCache, common.Processed)

	testCases := []struct {
		name           string
		obj            interface{}
		expectedErr    error
		expectedResult string
	}{
		{
			name: "Given monovertex cache has a valid monovertex in its cache, " +
				"And is processed" +
				"Then, we should be able to get the status as processed",
			obj:            monovertexInCache,
			expectedResult: common.Processed,
		},
		{
			name: "Given monovertex cache does not has a valid monovertex in its cache, " +
				"Then, the status for the valid monovertex should not be updated",
			obj:            monovertexNotInCache,
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
			res, err := monovertexController.GetProcessItemStatus(c.obj)
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
	d.LogValueOfAdmiralIoIgnore("not a monovertex")
	// No error should occur

	// Test case 2: K8sClient is nil
	d = &MonoVertexController{}
	d.LogValueOfAdmiralIoIgnore(&v1alpha1.MonoVertex{})
	// No error should occur

	d.LogValueOfAdmiralIoIgnore(&v1alpha1.MonoVertex{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}})
	// No error should occur

	// Test case 3: AdmiralIgnoreAnnotation is set in MonoVertex object
	d = &MonoVertexController{}
	monovertex := &v1alpha1.MonoVertex{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Annotations: map[string]string{
				common.AdmiralIgnoreAnnotation: "true",
			},
		},
	}
	d.LogValueOfAdmiralIoIgnore(monovertex)
	// No error should occur
}
