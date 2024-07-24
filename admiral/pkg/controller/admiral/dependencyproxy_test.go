package admiral

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	admiralV1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAdded(t *testing.T) {

	mockDependencyProxyHandler := &test.MockDependencyProxyHandler{}
	ctx := context.Background()
	dependencyProxyController := DependencyProxyController{
		Cache: &dependencyProxyCache{
			cache: make(map[string]*DependencyProxyItem),
			mutex: &sync.Mutex{},
		},
		DependencyProxyHandler: mockDependencyProxyHandler,
	}

	testCases := []struct {
		name            string
		dependencyProxy interface{}
		expectedError   error
	}{
		{
			name: "Given context and DependencyProxy " +
				"When DependencyProxy param is nil " +
				"Then func should return an error",
			dependencyProxy: nil,
			expectedError:   fmt.Errorf("type assertion failed, <nil> is not of type *v1.DependencyProxy"),
		},
		{
			name: "Given context and DependencyProxy " +
				"When DependencyProxy param is not of type *v1.DependencyProxy " +
				"Then func should return an error",
			dependencyProxy: struct{}{},
			expectedError:   fmt.Errorf("type assertion failed, {} is not of type *v1.DependencyProxy"),
		},
		{
			name: "Given context and DependencyProxy " +
				"When DependencyProxy param is of type *v1.DependencyProxy " +
				"Then func should not return an error",
			dependencyProxy: &v1.DependencyProxy{},
			expectedError:   nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := dependencyProxyController.Added(ctx, tc.dependencyProxy)
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

func TestUpdated(t *testing.T) {

	mockDependencyProxyHandler := &test.MockDependencyProxyHandler{}
	ctx := context.Background()
	dependencyProxyController := DependencyProxyController{
		Cache: &dependencyProxyCache{
			cache: make(map[string]*DependencyProxyItem),
			mutex: &sync.Mutex{},
		},
		DependencyProxyHandler: mockDependencyProxyHandler,
	}

	testCases := []struct {
		name            string
		dependencyProxy interface{}
		expectedError   error
	}{
		{
			name: "Given context and DependencyProxy " +
				"When DependencyProxy param is nil " +
				"Then func should return an error",
			dependencyProxy: nil,
			expectedError:   fmt.Errorf("type assertion failed, <nil> is not of type *v1.DependencyProxy"),
		},
		{
			name: "Given context and DependencyProxy " +
				"When DependencyProxy param is not of type *v1.DependencyProxy " +
				"Then func should return an error",
			dependencyProxy: struct{}{},
			expectedError:   fmt.Errorf("type assertion failed, {} is not of type *v1.DependencyProxy"),
		},
		{
			name: "Given context and DependencyProxy " +
				"When DependencyProxy param is of type *v1.DependencyProxy " +
				"Then func should not return an error",
			dependencyProxy: &v1.DependencyProxy{},
			expectedError:   nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := dependencyProxyController.Updated(ctx, tc.dependencyProxy, nil)
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

func TestDeleted(t *testing.T) {

	mockDependencyProxyHandler := &test.MockDependencyProxyHandler{}
	ctx := context.Background()
	dependencyProxyController := DependencyProxyController{
		Cache: &dependencyProxyCache{
			cache: make(map[string]*DependencyProxyItem),
			mutex: &sync.Mutex{},
		},
		DependencyProxyHandler: mockDependencyProxyHandler,
	}

	testCases := []struct {
		name            string
		dependencyProxy interface{}
		expectedError   error
	}{
		{
			name: "Given context and DependencyProxy " +
				"When DependencyProxy param is nil " +
				"Then func should return an error",
			dependencyProxy: nil,
			expectedError:   fmt.Errorf("type assertion failed, <nil> is not of type *v1.DependencyProxy"),
		},
		{
			name: "Given context and DependencyProxy " +
				"When DependencyProxy param is not of type *v1.DependencyProxy " +
				"Then func should return an error",
			dependencyProxy: struct{}{},
			expectedError:   fmt.Errorf("type assertion failed, {} is not of type *v1.DependencyProxy"),
		},
		{
			name: "Given context and DependencyProxy " +
				"When DependencyProxy param is of type *v1.DependencyProxy " +
				"Then func should not return an error",
			dependencyProxy: &v1.DependencyProxy{},
			expectedError:   nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := dependencyProxyController.Deleted(ctx, tc.dependencyProxy)
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

func TestDependencyProxyGetProcessItemStatus(t *testing.T) {
	var (
		serviceAccount         = &coreV1.ServiceAccount{}
		dependencyProxyInCache = &admiralV1.DependencyProxy{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "dp-in-cache",
				Namespace: "ns-1",
			},
		}
		dependencyProxyNotInCache = &v1.DependencyProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dp-not-in-cache",
				Namespace: "ns-2",
			},
		}
	)

	// Populating the deployment Cache
	dependencyProxyCache := &dependencyProxyCache{
		cache: make(map[string]*DependencyProxyItem),
		mutex: &sync.Mutex{},
	}

	dependencyProxyController := &DependencyProxyController{
		Cache: dependencyProxyCache,
	}

	dependencyProxyCache.Put(dependencyProxyInCache)
	dependencyProxyCache.UpdateDependencyProxyProcessStatus(dependencyProxyInCache, common.Processed)

	testCases := []struct {
		name                       string
		dependencyProxyToGetStatus interface{}
		expectedErr                error
		expectedResult             string
	}{
		{
			name: "Given dependency proxy cache has a valid dependency proxy in its cache, " +
				"And the dependency proxy is processed" +
				"Then, we should be able to get the status as processed",
			dependencyProxyToGetStatus: dependencyProxyInCache,
			expectedResult:             common.Processed,
		},
		{
			name: "Given dependency proxy cache does not has a valid dependency proxy in its cache, " +
				"Then, the function would return not processed",
			dependencyProxyToGetStatus: dependencyProxyNotInCache,
			expectedResult:             common.NotProcessed,
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			dependencyProxyToGetStatus: serviceAccount,
			expectedErr:                fmt.Errorf("type assertion failed"),
			expectedResult:             common.NotProcessed,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			res, err := dependencyProxyController.GetProcessItemStatus(c.dependencyProxyToGetStatus)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			assert.Equal(t, c.expectedResult, res)
		})
	}
}

func TestDependencyProxyUpdateProcessItemStatus(t *testing.T) {
	var (
		serviceAccount         = &coreV1.ServiceAccount{}
		dependencyProxyInCache = &v1.DependencyProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dp-in-cache",
				Namespace: "ns-1",
			},
		}
		dependencyProxyNotInCache = &v1.DependencyProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dp-not-in-cache",
				Namespace: "ns-2",
			},
		}
	)

	// Populating the deployment Cache
	dependencyProxyCache := &dependencyProxyCache{
		cache: make(map[string]*DependencyProxyItem),
		mutex: &sync.Mutex{},
	}

	dependencyProxyController := &DependencyProxyController{
		Cache: dependencyProxyCache,
	}

	dependencyProxyCache.Put(dependencyProxyInCache)

	cases := []struct {
		name        string
		obj         interface{}
		expectedErr error
	}{
		{
			name: "Given dependency proxy cache has a valid dependency proxy in its cache, " +
				"Then, the status for the valid dependency proxy should be updated to processed",
			obj:         dependencyProxyInCache,
			expectedErr: nil,
		},
		{
			name: "Given dependency proxy cache does not has a valid dependency proxy in its cache, " +
				"Then, an error should be returned with the dependency proxy not found message",
			obj: dependencyProxyNotInCache,
			expectedErr: fmt.Errorf(LogCacheFormat, "Update", "DependencyProxy",
				"dp-not-in-cache", "ns-2", "", "nothing to update, dependency proxy not found in cache"),
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:         serviceAccount,
			expectedErr: fmt.Errorf("type assertion failed"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := dependencyProxyController.UpdateProcessItemStatus(c.obj, common.Processed)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
		})
	}
}

func TestGet(t *testing.T) {
	var (
		dependencyProxyInCache = &v1.DependencyProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dp-in-cache",
				Namespace: "ns-1",
			},
		}
	)

	// Populating the deployment Cache
	dependencyProxyCache := &dependencyProxyCache{
		cache: make(map[string]*DependencyProxyItem),
		mutex: &sync.Mutex{},
	}

	dependencyProxyCache.Put(dependencyProxyInCache)

	testCases := []struct {
		name                 string
		dependencyProxyToGet string
		expectedResult       *v1.DependencyProxy
	}{
		{
			name: "Given dependency proxy cache has a valid dependency proxy in its cache, " +
				"Then, the function should be able to get the dependency proxy",
			dependencyProxyToGet: "dp-in-cache",
			expectedResult:       dependencyProxyInCache,
		},
		{
			name: "Given dependency proxy cache does not has a valid dependency proxy in its cache, " +
				"Then, the function should not be able to get the dependency proxy",
			dependencyProxyToGet: "dp-not-in-cache",
			expectedResult:       nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dependencyProxy := dependencyProxyCache.Get(tc.dependencyProxyToGet)
			assert.Equal(t, tc.expectedResult, dependencyProxy)
		})
	}
}

func TestNewDependencyProxyController(t *testing.T) {
	stop := make(chan struct{})
	handler := test.MockDependencyProxyHandler{}

	dependencyProxyController, err := NewDependencyProxyController(stop, &handler, "../../test/resources/admins@fake-cluster.k8s.local", "ns", time.Duration(1000), loader.GetFakeClientLoader())
	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if dependencyProxyController == nil {
		t.Errorf("Dependency proxy controller should never be nil without an error thrown")
	}
}
