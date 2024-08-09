package admiral

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"

	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	admiralV1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDependencyAdded(t *testing.T) {

	mockDependencyHandler := &test.MockDependencyHandler{}
	ctx := context.Background()
	dependencyController := DependencyController{
		Cache: &depCache{
			cache: make(map[string]*DependencyItem),
			mutex: &sync.Mutex{},
		},
		DepHandler: mockDependencyHandler,
	}

	testCases := []struct {
		name          string
		Dependency    interface{}
		expectedError error
	}{
		{
			name: "Given context and Dependency " +
				"When Dependency param is nil " +
				"Then func should return an error",
			Dependency:    nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.Dependency"),
		},
		{
			name: "Given context and Dependency " +
				"When Dependency param is not of type *v1.Dependency " +
				"Then func should return an error",
			Dependency:    struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.Dependency"),
		},
		{
			name: "Given context and Dependency " +
				"When Dependency param is of type *v1.Dependency " +
				"Then func should not return an error",
			Dependency:    &v1.Dependency{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := dependencyController.Added(ctx, tc.Dependency)
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

func TestDependencyUpdated(t *testing.T) {

	mockDependencyHandler := &test.MockDependencyHandler{}
	ctx := context.Background()
	dependencyController := DependencyController{
		Cache: &depCache{
			cache: make(map[string]*DependencyItem),
			mutex: &sync.Mutex{},
		},
		DepHandler: mockDependencyHandler,
	}

	testCases := []struct {
		name          string
		Dependency    interface{}
		expectedError error
	}{
		{
			name: "Given context and Dependency " +
				"When Dependency param is nil " +
				"Then func should return an error",
			Dependency:    nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.Dependency"),
		},
		{
			name: "Given context and Dependency " +
				"When Dependency param is not of type *v1.Dependency " +
				"Then func should return an error",
			Dependency:    struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.Dependency"),
		},
		{
			name: "Given context and Dependency " +
				"When Dependency param is of type *v1.Dependency " +
				"Then func should not return an error",
			Dependency:    &v1.Dependency{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := dependencyController.Updated(ctx, tc.Dependency, nil)
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

func TestDependencyDeleted(t *testing.T) {

	mockDependencyHandler := &test.MockDependencyHandler{}
	ctx := context.Background()
	dependencyController := DependencyController{
		Cache: &depCache{
			cache: make(map[string]*DependencyItem),
			mutex: &sync.Mutex{},
		},
		DepHandler: mockDependencyHandler,
	}

	testCases := []struct {
		name          string
		Dependency    interface{}
		expectedError error
	}{
		{
			name: "Given context and Dependency " +
				"When Dependency param is nil " +
				"Then func should return an error",
			Dependency:    nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.Dependency"),
		},
		{
			name: "Given context and Dependency " +
				"When Dependency param is not of type *v1.Dependency " +
				"Then func should return an error",
			Dependency:    struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.Dependency"),
		},
		{
			name: "Given context and Dependency " +
				"When Dependency param is of type *v1.Dependency " +
				"Then func should not return an error",
			Dependency:    &v1.Dependency{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := dependencyController.Deleted(ctx, tc.Dependency)
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

func TestNewDependencyController(t *testing.T) {
	stop := make(chan struct{})
	handler := test.MockDependencyHandler{}

	dependencyController, err := NewDependencyController(stop, &handler, "../../test/resources/admins@fake-cluster.k8s.local", "ns", time.Duration(1000), loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if dependencyController == nil {
		t.Errorf("Dependency controller should never be nil without an error thrown")
	}
}

func makeK8sDependencyObj(name string, namespace string, dependency model.Dependency) *v1.Dependency {
	return &v1.Dependency{Spec: dependency, ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace}}
}

func TestDependencyAddUpdateAndDelete(t *testing.T) {
	stop := make(chan struct{})
	handler := test.MockDependencyHandler{}

	dependencyController, err := NewDependencyController(stop, &handler, "../../test/resources/admins@fake-cluster.k8s.local", "ns", time.Duration(1000), loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if dependencyController == nil {
		t.Errorf("Dependency controller should never be nil without an error thrown")
	}

	ctx := context.Background()

	//test add
	depName := "dep1"
	dep := model.Dependency{IdentityLabel: "identity", Destinations: []string{"greeting", "payments"}, Source: "webapp"}
	depObj := makeK8sDependencyObj(depName, "namespace1", dep)
	dependencyController.Added(ctx, depObj)

	newDepObj := dependencyController.Cache.Get(depName)

	if !cmp.Equal(depObj.Spec, newDepObj.Spec) {
		t.Errorf("dep update failed, expected: %v got %v", depObj, newDepObj)
	}

	//test update
	updatedDep := model.Dependency{IdentityLabel: "identity", Destinations: []string{"greeting", "payments", "newservice"}, Source: "webapp"}
	updatedObj := makeK8sDependencyObj(depName, "namespace1", updatedDep)
	dependencyController.Updated(ctx, makeK8sDependencyObj(depName, "namespace1", updatedDep), depObj)

	updatedDepObj := dependencyController.Cache.Get(depName)

	if !cmp.Equal(updatedObj.Spec, updatedDepObj.Spec) {
		t.Errorf("dep update failed, expected: %v got %v", updatedObj, updatedDepObj)
	}

	//test delete
	dependencyController.Deleted(ctx, updatedDepObj)

	deletedDepObj := dependencyController.Cache.Get(depName)

	if deletedDepObj != nil {
		t.Errorf("dep delete failed")
	}

}

func TestDependencyGetProcessItemStatus(t *testing.T) {
	var (
		serviceAccount    = &coreV1.ServiceAccount{}
		dependencyInCache = &admiralV1.Dependency{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "dep-in-cache",
				Namespace: "ns-1",
			},
		}
		dependencyNotInCache = &v1.Dependency{
			ObjectMeta: v12.ObjectMeta{
				Name:      "dep-not-in-cache",
				Namespace: "ns-2",
			},
		}
	)

	// Populating the deployment Cache
	depCache := &depCache{
		cache: make(map[string]*DependencyItem),
		mutex: &sync.Mutex{},
	}

	dependencyController := &DependencyController{
		Cache: depCache,
	}

	depCache.Put(dependencyInCache)
	depCache.UpdateDependencyProcessStatus(dependencyInCache, common.Processed)

	testCases := []struct {
		name             string
		dependencyRecord interface{}
		expectedResult   string
		expectedErr      error
	}{
		{
			name: "Given dependency cache has a valid dependency in its cache, " +
				"And the dependency is processed" +
				"Then, we should be able to get the status as processed",
			dependencyRecord: dependencyInCache,
			expectedResult:   common.Processed,
		},
		{
			name: "Given dependency cache does not has a valid dependency in its cache, " +
				"Then, the function would return not processed",
			dependencyRecord: dependencyNotInCache,
			expectedResult:   common.NotProcessed,
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			dependencyRecord: serviceAccount,
			expectedErr:      fmt.Errorf("type assertion failed"),
			expectedResult:   common.NotProcessed,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			res, err := dependencyController.GetProcessItemStatus(c.dependencyRecord)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			assert.Equal(t, c.expectedResult, res)
		})
	}
}

func TestDependencyUpdateProcessItemStatus(t *testing.T) {
	var (
		serviceAccount    = &coreV1.ServiceAccount{}
		dependencyInCache = &v1.Dependency{
			ObjectMeta: v12.ObjectMeta{
				Name:      "dep-in-cache",
				Namespace: "ns-1",
			},
		}
		dependencyNotInCache = &v1.Dependency{
			ObjectMeta: v12.ObjectMeta{
				Name:      "dep-not-in-cache",
				Namespace: "ns-2",
			},
		}
	)

	// Populating the deployment Cache
	depCache := &depCache{
		cache: make(map[string]*DependencyItem),
		mutex: &sync.Mutex{},
	}

	dependencyController := &DependencyController{
		Cache: depCache,
	}

	depCache.Put(dependencyInCache)

	cases := []struct {
		name        string
		obj         interface{}
		expectedErr error
	}{
		{
			name: "Given dependency cache has a valid dependency in its cache, " +
				"Then, the status for the valid dependency should be updated to processed",
			obj:         dependencyInCache,
			expectedErr: nil,
		},
		{
			name: "Given dependency cache does not has a valid dependency in its cache, " +
				"Then, an error should be returned with the dependency not found message",
			obj: dependencyNotInCache,
			expectedErr: fmt.Errorf(LogCacheFormat, "Update", "Dependency",
				"dep-not-in-cache", "ns-2", "", "nothing to update, dependency not found in cache"),
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
			err := dependencyController.UpdateProcessItemStatus(c.obj, common.Processed)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
		})
	}
}
