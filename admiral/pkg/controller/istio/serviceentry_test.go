package istio

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	coreV1 "k8s.io/api/core/v1"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"

	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	"istio.io/api/networking/v1alpha3"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1alpha32 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestServiceEntryAdded(t *testing.T) {

	mockServiceEntryHandler := &test.MockServiceEntryHandler{}
	ctx := context.Background()
	serviceEntryController := ServiceEntryController{
		ServiceEntryHandler: mockServiceEntryHandler,
		Cluster:             "testCluster",
		Cache: &ServiceEntryCache{
			cache: map[string]map[string]*ServiceEntryItem{},
			mutex: &sync.RWMutex{},
		},
	}

	testCases := []struct {
		name          string
		serviceEntry  interface{}
		expectedError error
	}{
		{
			name: "Given context and ServiceEntry " +
				"When ServiceEntry param is nil " +
				"Then func should return an error",
			serviceEntry:  nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.ServiceEntry"),
		},
		{
			name: "Given context and ServiceEntry " +
				"When ServiceEntry param is not of type *v1alpha3.ServiceEntry " +
				"Then func should return an error",
			serviceEntry:  struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.ServiceEntry"),
		},
		{
			name: "Given context and ServiceEntry " +
				"When ServiceEntry param is of type *v1alpha3.ServiceEntry " +
				"Then func should not return an error",
			serviceEntry:  &networking.ServiceEntry{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := serviceEntryController.Added(ctx, tc.serviceEntry)
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

func TestServiceEntryUpdated(t *testing.T) {

	mockServiceEntryHandler := &test.MockServiceEntryHandler{}
	ctx := context.Background()
	serviceEntryController := ServiceEntryController{
		ServiceEntryHandler: mockServiceEntryHandler,
		Cluster:             "testCluster",
		Cache: &ServiceEntryCache{
			cache: map[string]map[string]*ServiceEntryItem{},
			mutex: &sync.RWMutex{},
		},
	}

	testCases := []struct {
		name          string
		serviceEntry  interface{}
		expectedError error
	}{
		{
			name: "Given context and ServiceEntry " +
				"When ServiceEntry param is nil " +
				"Then func should return an error",
			serviceEntry:  nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.ServiceEntry"),
		},
		{
			name: "Given context and ServiceEntry " +
				"When ServiceEntry param is not of type *v1alpha3.ServiceEntry " +
				"Then func should return an error",
			serviceEntry:  struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.ServiceEntry"),
		},
		{
			name: "Given context and ServiceEntry " +
				"When ServiceEntry param is of type *v1alpha3.ServiceEntry " +
				"Then func should not return an error",
			serviceEntry:  &networking.ServiceEntry{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := serviceEntryController.Updated(ctx, tc.serviceEntry, nil)
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

func TestServiceEntryDeleted(t *testing.T) {

	mockServiceEntryHandler := &test.MockServiceEntryHandler{}
	ctx := context.Background()
	serviceEntryController := ServiceEntryController{
		ServiceEntryHandler: mockServiceEntryHandler,
		Cluster:             "testCluster",
		Cache: &ServiceEntryCache{
			cache: map[string]map[string]*ServiceEntryItem{},
			mutex: &sync.RWMutex{},
		},
	}

	testCases := []struct {
		name          string
		serviceEntry  interface{}
		expectedError error
	}{
		{
			name: "Given context and ServiceEntry " +
				"When ServiceEntry param is nil " +
				"Then func should return an error",
			serviceEntry:  nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.ServiceEntry"),
		},
		{
			name: "Given context and ServiceEntry " +
				"When ServiceEntry param is not of type *v1alpha3.ServiceEntry " +
				"Then func should return an error",
			serviceEntry:  struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.ServiceEntry"),
		},
		{
			name: "Given context and ServiceEntry " +
				"When ServiceEntry param is of type *v1alpha3.ServiceEntry " +
				"Then func should not return an error",
			serviceEntry:  &networking.ServiceEntry{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := serviceEntryController.Deleted(ctx, tc.serviceEntry)
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

func TestNewServiceEntryController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockServiceEntryHandler{}

	serviceEntryController, err := NewServiceEntryController(stop, &handler, "testCluster", config, time.Duration(1000), loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if serviceEntryController == nil {
		t.Errorf("ServiceEntry controller should never be nil without an error thrown")
	}

	serviceEntry := &v1alpha32.ServiceEntry{Spec: v1alpha3.ServiceEntry{}, ObjectMeta: v1.ObjectMeta{Name: "se1", Namespace: "namespace1"}}

	ctx := context.Background()

	serviceEntryController.Added(ctx, serviceEntry)

	if !cmp.Equal(&serviceEntry.Spec, &handler.Obj.Spec, protocmp.Transform()) {
		t.Errorf("Handler should have the added obj")
	}

	updatedServiceEntry := &v1alpha32.ServiceEntry{Spec: v1alpha3.ServiceEntry{Hosts: []string{"hello.global"}}, ObjectMeta: v1.ObjectMeta{Name: "se1", Namespace: "namespace1"}}
	serviceEntryController.Updated(ctx, updatedServiceEntry, serviceEntry)

	if !cmp.Equal(&updatedServiceEntry.Spec, &handler.Obj.Spec, protocmp.Transform()) {
		t.Errorf("Handler should have the updated obj")
	}

	serviceEntryController.Deleted(ctx, serviceEntry)

	if handler.Obj != nil {
		t.Errorf("Handler should have no obj")
	}
}

// TODO: This is just a placeholder for when we add diff check for other types
func TestServiceEntryGetProcessItemStatus(t *testing.T) {
	serviceEntryController := ServiceEntryController{}
	testCases := []struct {
		name        string
		obj         interface{}
		expectedRes string
	}{
		{
			name:        "TODO: Currently always returns false",
			obj:         nil,
			expectedRes: common.NotProcessed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, _ := serviceEntryController.GetProcessItemStatus(tc.obj)
			assert.Equal(t, tc.expectedRes, res)
		})
	}
}

func TestServiceEntryUpdateProcessItemStatus(t *testing.T) {
	var (
		serviceAccount = &coreV1.ServiceAccount{}

		se1 = &networking.ServiceEntry{
			ObjectMeta: v1.ObjectMeta{
				Name:        "debug-incache",
				Namespace:   "namespace1",
				Annotations: map[string]string{"other-annotation": "value"}}}

		se2 = &networking.ServiceEntry{
			ObjectMeta: v1.ObjectMeta{
				Name:        "debug2-incache",
				Namespace:   "namespace1",
				Annotations: map[string]string{"other-annotation": "value"}}}

		seNotInCache = &networking.ServiceEntry{
			ObjectMeta: v1.ObjectMeta{
				Name:        "debug",
				Namespace:   "namespace1",
				Annotations: map[string]string{"other-annotation": "value"}}}

		diffNsSeNotInCache = &networking.ServiceEntry{
			ObjectMeta: v1.ObjectMeta{
				Name:        "debug",
				Namespace:   "namespace2",
				Annotations: map[string]string{"other-annotation": "value"}}}
	)

	seCache := &ServiceEntryCache{
		cache: make(map[string]map[string]*ServiceEntryItem),
		mutex: &sync.RWMutex{},
	}

	serviceentryController := &ServiceEntryController{
		Cluster: "cluster1",
		Cache:   seCache,
	}

	seCache.Put(se1, "cluster1")
	seCache.Put(se2, "cluster1")

	cases := []struct {
		name           string
		obj            interface{}
		statusToSet    string
		expectedErr    error
		expectedStatus string
	}{
		{
			name: "Given se cache has a valid se in its cache, " +
				"Then, the status for the valid se should be updated to processed",
			obj:            se1,
			statusToSet:    common.Processed,
			expectedErr:    nil,
			expectedStatus: common.Processed,
		},
		{
			name: "Given se cache has a valid se in its cache, " +
				"Then, the status for the valid se should be updated to not processed",
			obj:            se2,
			statusToSet:    common.NotProcessed,
			expectedErr:    nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given se cache does not has a valid se in its cache, " +
				"Then, the status for the valid se should be not processed, " +
				"And an error should be returned with the se not found message",
			obj:         seNotInCache,
			statusToSet: common.NotProcessed,
			expectedErr: fmt.Errorf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Update", "ServiceEntry",
				seNotInCache.Name, seNotInCache.Namespace, "", "nothing to update, serviceentry not found in cache"),
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given se cache does not has a valid se in its cache, " +
				"And se is in a different namespace, " +
				"Then, the status for the valid se should be not processed, " +
				"And an error should be returned with the se not found message",
			obj:         diffNsSeNotInCache,
			statusToSet: common.NotProcessed,
			expectedErr: fmt.Errorf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Update", "ServiceEntry",
				diffNsSeNotInCache.Name, diffNsSeNotInCache.Namespace, "", "nothing to update, serviceentry not found in cache"),
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
			err := serviceentryController.UpdateProcessItemStatus(c.obj, c.statusToSet)
			if err != nil && c.expectedErr == nil {
				t.Errorf("unexpected error: %v", err)
			}
			if err == nil && c.expectedErr != nil {
				t.Errorf("expected error: %v", c.expectedErr)
			}
			if err != nil && c.expectedErr != nil && !strings.Contains(err.Error(), c.expectedErr.Error()) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			status, _ := serviceentryController.GetProcessItemStatus(c.obj)
			assert.Equal(t, c.expectedStatus, status)
		})
	}
}

func TestServiceEntryLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a ServiceEntry object
	sec := &ServiceEntryController{}
	sec.LogValueOfAdmiralIoIgnore("not a service entry")
	// No error should occur

	// Test case 2: ServiceEntry has no annotations
	sec = &ServiceEntryController{}
	sec.LogValueOfAdmiralIoIgnore(&networking.ServiceEntry{})
	// No error should occur

	// Test case 3: AdmiralIgnoreAnnotation is not set
	sec = &ServiceEntryController{}
	se := &networking.ServiceEntry{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"other-annotation": "value"}}}
	sec.LogValueOfAdmiralIoIgnore(se)
	// No error should occur

	// Test case 4: AdmiralIgnoreAnnotation is set in annotations
	sec = &ServiceEntryController{}
	se = &networking.ServiceEntry{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}}
	sec.LogValueOfAdmiralIoIgnore(se)
	// No error should occur
}
