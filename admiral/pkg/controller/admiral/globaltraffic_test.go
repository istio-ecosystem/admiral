package admiral

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestGlobalTrafficPolicyAddedTypeAssertion(t *testing.T) {

	mockGTPHandler := &test.MockGlobalTrafficHandler{}
	ctx := context.Background()
	gtpController := GlobalTrafficController{
		GlobalTrafficHandler: mockGTPHandler,
		Cache: &gtpCache{
			cache: make(map[string]map[string]map[string]*gtpItem),
			mutex: &sync.Mutex{},
		},
	}

	testCases := []struct {
		name          string
		gtp           interface{}
		expectedError error
	}{
		{
			name: "Given context and GlobalTrafficPolicy " +
				"When GlobalTrafficPolicy param is nil " +
				"Then func should return an error",
			gtp:           nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.GlobalTrafficPolicy"),
		},
		{
			name: "Given context and GlobalTrafficPolicy " +
				"When GlobalTrafficPolicy param is not of type *v1.GlobalTrafficPolicy " +
				"Then func should return an error",
			gtp:           struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.GlobalTrafficPolicy"),
		},
		{
			name: "Given context and GlobalTrafficPolicy " +
				"When GlobalTrafficPolicy param is of type *v1.GlobalTrafficPolicy " +
				"Then func should not return an error",
			gtp:           &v1.GlobalTrafficPolicy{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := gtpController.Added(ctx, tc.gtp)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				if err != nil {
					assert.Fail(t, "expected error to be nil but got %v", err)
				}
			}

		})
	}

}

func TestGlobalTrafficPolicyUpdatedTypeAssertion(t *testing.T) {

	mockGTPHandler := &test.MockGlobalTrafficHandler{}
	ctx := context.Background()
	gtpController := GlobalTrafficController{
		GlobalTrafficHandler: mockGTPHandler,
		Cache: &gtpCache{
			cache: make(map[string]map[string]map[string]*gtpItem),
			mutex: &sync.Mutex{},
		},
	}

	testCases := []struct {
		name          string
		gtp           interface{}
		expectedError error
	}{
		{
			name: "Given context and GlobalTrafficPolicy " +
				"When GlobalTrafficPolicy param is nil " +
				"Then func should return an error",
			gtp:           nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.GlobalTrafficPolicy"),
		},
		{
			name: "Given context and GlobalTrafficPolicy " +
				"When GlobalTrafficPolicy param is not of type *v1.GlobalTrafficPolicy " +
				"Then func should return an error",
			gtp:           struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.GlobalTrafficPolicy"),
		},
		{
			name: "Given context and GlobalTrafficPolicy " +
				"When GlobalTrafficPolicy param is of type *v1.GlobalTrafficPolicy " +
				"Then func should not return an error",
			gtp:           &v1.GlobalTrafficPolicy{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := gtpController.Updated(ctx, tc.gtp, nil)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				if err != nil {
					assert.Fail(t, "expected error to be nil but got %v", err)
				}
			}

		})
	}

}

func TestGlobalTrafficPolicyDeletedTypeAssertion(t *testing.T) {

	mockGTPHandler := &test.MockGlobalTrafficHandler{}
	ctx := context.Background()
	gtpController := GlobalTrafficController{
		GlobalTrafficHandler: mockGTPHandler,
		Cache: &gtpCache{
			cache: make(map[string]map[string]map[string]*gtpItem),
			mutex: &sync.Mutex{},
		},
	}

	testCases := []struct {
		name          string
		gtp           interface{}
		expectedError error
	}{
		{
			name: "Given context and GlobalTrafficPolicy " +
				"When GlobalTrafficPolicy param is nil " +
				"Then func should return an error",
			gtp:           nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.GlobalTrafficPolicy"),
		},
		{
			name: "Given context and GlobalTrafficPolicy " +
				"When GlobalTrafficPolicy param is not of type *v1.GlobalTrafficPolicy " +
				"Then func should return an error",
			gtp:           struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.GlobalTrafficPolicy"),
		},
		{
			name: "Given context and GlobalTrafficPolicy " +
				"When GlobalTrafficPolicy param is of type *v1.GlobalTrafficPolicy " +
				"Then func should not return an error",
			gtp:           &v1.GlobalTrafficPolicy{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := gtpController.Deleted(ctx, tc.gtp)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				if err != nil {
					assert.Fail(t, "expected error to be nil but got %v", err)
				}
			}

		})
	}

}

func TestNewGlobalTrafficController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockGlobalTrafficHandler{}

	globalTrafficController, err := NewGlobalTrafficController(stop, &handler, config, time.Duration(1000), loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if globalTrafficController == nil {
		t.Errorf("GlobalTraffic controller should never be nil without an error thrown")
	}
}

func TestGlobalTrafficAddUpdateDelete(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockGlobalTrafficHandler{}

	globalTrafficController, err := NewGlobalTrafficController(stop, &handler, config, time.Duration(1000), loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if globalTrafficController == nil {
		t.Errorf("GlobalTraffic controller should never be nil without an error thrown")
	}

	ctx := context.Background()

	gtpName := "gtp1"
	gtp := model.GlobalTrafficPolicy{Selector: map[string]string{"identity": "payments", "env": "e2e"}, Policy: []*model.TrafficPolicy{}}
	gtpObj := makeK8sGtpObj(gtpName, "namespace1", gtp)
	globalTrafficController.Added(ctx, gtpObj)

	if !cmp.Equal(handler.Obj.Spec, gtpObj.Spec) {
		t.Errorf("Add should call the handler with the object")
	}

	updatedGtp := model.GlobalTrafficPolicy{Selector: map[string]string{"identity": "payments", "env": "qa"}, Policy: []*model.TrafficPolicy{}}
	updatedGtpObj := makeK8sGtpObj(gtpName, "namespace1", updatedGtp)

	globalTrafficController.Updated(ctx, updatedGtpObj, gtpObj)

	if !cmp.Equal(handler.Obj.Spec, updatedGtpObj.Spec) {
		t.Errorf("Update should call the handler with the updated object")
	}

	globalTrafficController.Deleted(ctx, updatedGtpObj)

	if handler.Obj != nil {
		t.Errorf("Delete should delete the gtp")
	}

}

func TestGlobalTrafficController_Updated(t *testing.T) {

	var (
		gth   = test.MockGlobalTrafficHandler{}
		cache = gtpCache{
			cache: make(map[string]map[string]map[string]*gtpItem),
			mutex: &sync.Mutex{},
		}
		gtpController = GlobalTrafficController{
			GlobalTrafficHandler: &gth,
			Cache:                &cache,
		}
		gtp = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hello"}},
		}}
		gtpUpdated = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "helloUpdated"}},
		}}
		gtpUpdatedToIgnore = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}, Annotations: map[string]string{"admiral.io/ignore": "true"}}}
	)

	ctx := context.Background()

	//add the base object to cache
	gtpController.Added(ctx, &gtp)

	testCases := []struct {
		name         string
		gtp          *v1.GlobalTrafficPolicy
		expectedGtps []*v1.GlobalTrafficPolicy
	}{
		{
			name:         "Gtp with should be updated",
			gtp:          &gtpUpdated,
			expectedGtps: []*v1.GlobalTrafficPolicy{&gtpUpdated},
		},
		{
			name:         "Should remove gtp from cache when update with Ignore annotation",
			gtp:          &gtpUpdatedToIgnore,
			expectedGtps: []*v1.GlobalTrafficPolicy{},
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtpController.Updated(ctx, c.gtp, gtp)
			gtpKey := common.GetGtpKey(c.gtp)
			matchedGtps := gtpController.Cache.Get(gtpKey, c.gtp.Namespace)
			if !reflect.DeepEqual(c.expectedGtps, matchedGtps) {
				t.Errorf("Test %s failed; expected %v, got %v", c.name, c.expectedGtps, matchedGtps)
			}
		})
	}
}

func TestGlobalTrafficController_Deleted(t *testing.T) {

	var (
		gth   = test.MockGlobalTrafficHandler{}
		cache = gtpCache{
			cache: make(map[string]map[string]map[string]*gtpItem),
			mutex: &sync.Mutex{},
		}
		gtpController = GlobalTrafficController{
			GlobalTrafficHandler: &gth,
			Cache:                &cache,
		}
		gtp = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hello"}},
		}}

		gtp2 = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp2", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp2"}},
		}}

		gtp3 = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp3", Namespace: "namespace2", Labels: map[string]string{"identity": "id2", "admiral.io/env": "stage"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp3"}},
		}}
	)

	ctx := context.Background()

	//add the base object to cache
	gtpController.Added(ctx, &gtp)
	gtpController.Added(ctx, &gtp2)

	testCases := []struct {
		name         string
		gtp          *v1.GlobalTrafficPolicy
		expectedGtps []*v1.GlobalTrafficPolicy
	}{
		{
			name:         "Should delete gtp",
			gtp:          &gtp,
			expectedGtps: []*v1.GlobalTrafficPolicy{&gtp2},
		},
		{
			name:         "Deleting non existing gtp should be a no-op",
			gtp:          &gtp3,
			expectedGtps: []*v1.GlobalTrafficPolicy{},
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtpController.Deleted(ctx, c.gtp)
			gtpKey := common.GetGtpKey(c.gtp)
			matchedGtps := gtpController.Cache.Get(gtpKey, c.gtp.Namespace)
			if !reflect.DeepEqual(c.expectedGtps, matchedGtps) {
				t.Errorf("Test %s failed; expected %v, got %v", c.name, c.expectedGtps, matchedGtps)
			}
		})
	}
}

func TestGlobalTrafficController_Added(t *testing.T) {
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
		gth                 = test.MockGlobalTrafficHandler{}
		gtp                 = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}}
		gtpWithIgnoreLabels = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtpWithIgnoreLabels", Namespace: "namespace2", Labels: map[string]string{"identity": "id2", "admiral.io/env": "stage"}, Annotations: map[string]string{"admiral.io/ignore": "true"}}}

		gtp2 = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp2", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}}

		gtp3 = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp3", Namespace: "namespace3", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}}
	)

	ctx := context.Background()

	testCases := []struct {
		name         string
		gtpKey       string
		namespace    string
		gtp          []*v1.GlobalTrafficPolicy
		expectedGtps []*v1.GlobalTrafficPolicy
	}{
		{
			name:         "Gtp should be added to the cache",
			gtpKey:       "stage.id",
			namespace:    "namespace1",
			gtp:          []*v1.GlobalTrafficPolicy{&gtp},
			expectedGtps: []*v1.GlobalTrafficPolicy{&gtp},
		},
		{
			name:         "Gtp with ignore annotation should not be added to the cache",
			gtpKey:       "stage.id2",
			namespace:    "namespace2",
			gtp:          []*v1.GlobalTrafficPolicy{&gtpWithIgnoreLabels},
			expectedGtps: []*v1.GlobalTrafficPolicy{},
		},
		{
			name:         "Should cache multiple gtps in a namespace",
			gtpKey:       "stage.id",
			namespace:    "namespace1",
			gtp:          []*v1.GlobalTrafficPolicy{&gtp, &gtp2},
			expectedGtps: []*v1.GlobalTrafficPolicy{&gtp, &gtp2},
		},
		{
			name:         "Should cache gtps in from multiple namespaces",
			gtpKey:       "stage.id",
			namespace:    "namespace3",
			gtp:          []*v1.GlobalTrafficPolicy{&gtp, &gtp3},
			expectedGtps: []*v1.GlobalTrafficPolicy{&gtp3},
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtpController := GlobalTrafficController{
				GlobalTrafficHandler: &gth,
				Cache: &gtpCache{
					cache: make(map[string]map[string]map[string]*gtpItem),
					mutex: &sync.Mutex{},
				},
			}
			for _, g := range c.gtp {
				gtpController.Added(ctx, g)
			}
			matchedGtps := gtpController.Cache.Get(c.gtpKey, c.namespace)
			sort.Slice(matchedGtps, func(i, j int) bool {
				return matchedGtps[i].Name < matchedGtps[j].Name
			})
			if !reflect.DeepEqual(c.expectedGtps, matchedGtps) {
				t.Errorf("Test %s failed; expected %v, got %v", c.name, c.expectedGtps, matchedGtps)
			}
		})
	}
}

func makeK8sGtpObj(name string, namespace string, gtp model.GlobalTrafficPolicy) *v1.GlobalTrafficPolicy {
	return &v1.GlobalTrafficPolicy{
		Spec:       gtp,
		ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace},
		TypeMeta: v12.TypeMeta{
			APIVersion: "admiral.io/v1",
			Kind:       "GlobalTrafficPolicy",
		}}
}

func TestGlobalTrafficGetProcessItemStatus(t *testing.T) {
	var (
		serviceAccount = &coreV1.ServiceAccount{}
		gtpInCache     = &v1.GlobalTrafficPolicy{
			ObjectMeta: v12.ObjectMeta{
				Name:      "gtp-in-cache",
				Namespace: "ns-1",
				Labels:    map[string]string{"identity": "id", "admiral.io/env": "stage"},
			},
		}
		gtpInCache2 = &v1.GlobalTrafficPolicy{
			ObjectMeta: v12.ObjectMeta{
				Name:      "gtp-in-cache2",
				Namespace: "ns-1",
				Labels:    map[string]string{"identity": "id", "admiral.io/env": "stage"},
			},
		}
		gtpNotInCache = &v1.GlobalTrafficPolicy{
			ObjectMeta: v12.ObjectMeta{
				Name:      "gtp-not-in-cache",
				Namespace: "ns-2",
				Labels:    map[string]string{"identity": "id1", "admiral.io/env": "stage1"},
			},
		}
	)

	// Populating the deployment Cache
	gtpCache := &gtpCache{
		cache: make(map[string]map[string]map[string]*gtpItem),
		mutex: &sync.Mutex{},
	}

	gtpController := &GlobalTrafficController{
		Cache: gtpCache,
	}

	gtpCache.Put(gtpInCache)
	gtpCache.UpdateGTPProcessStatus(gtpInCache, common.Processed)
	gtpCache.UpdateGTPProcessStatus(gtpInCache2, common.NotProcessed)

	cases := []struct {
		name        string
		obj         interface{}
		expectedRes string
		expectedErr error
	}{
		{
			name: "Given gtp cache has a valid gtp in its cache, " +
				"And the gtp is processed" +
				"Then, we should be able to get the status as processed",
			obj:         gtpInCache,
			expectedRes: common.Processed,
		},
		{
			name: "Given gtp cache has a valid gtp in its cache, " +
				"And the gtp is processed" +
				"Then, we should be able to get the status as not processed",
			obj:         gtpInCache2,
			expectedRes: common.NotProcessed,
		},
		{
			name: "Given dependency cache does not has a valid dependency in its cache, " +
				"Then, the function would return not processed",
			obj:         gtpNotInCache,
			expectedRes: common.NotProcessed,
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:         serviceAccount,
			expectedErr: fmt.Errorf("type assertion failed"),
			expectedRes: common.NotProcessed,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := gtpController.GetProcessItemStatus(c.obj)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			assert.Equal(t, c.expectedRes, res)
		})
	}
}

func TestGlobalTrafficUpdateProcessItemStatus(t *testing.T) {
	var (
		serviceAccount = &coreV1.ServiceAccount{}
		gtpInCache     = &v1.GlobalTrafficPolicy{
			ObjectMeta: v12.ObjectMeta{
				Name:      "gtp-in-cache",
				Namespace: "ns-1",
				Labels:    map[string]string{"identity": "id", "admiral.io/env": "stage"},
			},
		}
		gtpInCache2 = &v1.GlobalTrafficPolicy{
			ObjectMeta: v12.ObjectMeta{
				Name:      "gtp-in-cache2",
				Namespace: "ns-1",
				Labels:    map[string]string{"identity": "id", "admiral.io/env": "stage"},
			},
		}
		gtpNotInCache = &v1.GlobalTrafficPolicy{
			ObjectMeta: v12.ObjectMeta{
				Name:      "gtp-not-in-cache",
				Namespace: "ns-2",
				Labels:    map[string]string{"identity": "id1", "admiral.io/env": "stage1"},
			},
		}
		diffNsGtpNotInCache = &v1.GlobalTrafficPolicy{
			ObjectMeta: v12.ObjectMeta{
				Name:      "gtp-not-in-cache-2",
				Namespace: "ns-4",
				Labels:    map[string]string{"identity": "id1", "admiral.io/env": "stage1"},
			},
		}
	)

	// Populating the deployment Cache
	gtpCache := &gtpCache{
		cache: make(map[string]map[string]map[string]*gtpItem),
		mutex: &sync.Mutex{},
	}

	gtpController := &GlobalTrafficController{
		Cache: gtpCache,
	}

	gtpCache.Put(gtpInCache)
	gtpCache.Put(gtpInCache2)

	cases := []struct {
		name           string
		obj            interface{}
		statusToSet    string
		expectedStatus string
		expectedErr    error
	}{
		{
			name: "Given gtp cache has a valid gtp in its cache, " +
				"Then, the status for the valid gtp should be updated to true",
			obj:            gtpInCache,
			statusToSet:    common.Processed,
			expectedErr:    nil,
			expectedStatus: common.Processed,
		},
		{
			name: "Given gtp cache has a valid gtp in its cache, " +
				"Then, the status for the valid gtp should be updated to false",
			obj:            gtpInCache2,
			statusToSet:    common.NotProcessed,
			expectedErr:    nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given gtp cache does not has a valid gtp in its cache, " +
				"Then, an error should be returned with the gtp not found message, " +
				"And the status should be false",
			obj:         gtpNotInCache,
			statusToSet: common.NotProcessed,
			expectedErr: fmt.Errorf(LogCacheFormat, "Update", "GTP",
				"gtp-not-in-cache", "ns-2", "", "nothing to update, gtp not found in cache"),
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given gtp cache does not has a valid gtp in its cache, " +
				"And gtp is in a different namespace, " +
				"Then, an error should be returned with the gtp not found message, " +
				"And the status should be false",
			obj:         diffNsGtpNotInCache,
			statusToSet: common.NotProcessed,
			expectedErr: fmt.Errorf(LogCacheFormat, "Update", "GTP",
				"gtp-not-in-cache-2", "ns-4", "", "nothing to update, gtp not found in cache"),
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

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := gtpController.UpdateProcessItemStatus(c.obj, c.statusToSet)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			status, _ := gtpController.GetProcessItemStatus(c.obj)
			assert.Equal(t, c.expectedStatus, status)
		})
	}
}

func TestGlobalTrafficLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a GlobalTrafficPolicy object
	d := &GlobalTrafficController{}
	d.LogValueOfAdmiralIoIgnore("not a global traffic policy")
	// No error should occur

	// Test case 2: GlobalTrafficPolicy has no annotations or labels
	d = &GlobalTrafficController{}
	d.LogValueOfAdmiralIoIgnore(&v1.GlobalTrafficPolicy{})
	// No error should occur

	// Test case 3: AdmiralIgnoreAnnotation is not set
	d = &GlobalTrafficController{}
	gtp := &v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Annotations: map[string]string{"other-annotation": "value"}}}
	d.LogValueOfAdmiralIoIgnore(gtp)
	// No error should occur

	// Test case 4: AdmiralIgnoreAnnotation is set in annotations
	d = &GlobalTrafficController{}
	gtp = &v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Annotations: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}}
	d.LogValueOfAdmiralIoIgnore(gtp)
	// No error should occur

	// Test case 5: AdmiralIgnoreAnnotation is set in labels
	d = &GlobalTrafficController{}
	gtp = &v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Labels: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}}
	d.LogValueOfAdmiralIoIgnore(gtp)
	// No error should occur
}
