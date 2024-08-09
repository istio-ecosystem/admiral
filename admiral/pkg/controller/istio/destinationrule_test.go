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
	v1alpha32 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestDestinationRuleAdded(t *testing.T) {

	mockDestinationRuleHandler := &test.MockDestinationRuleHandler{}
	ctx := context.Background()
	destinationRuleController := DestinationRuleController{
		DestinationRuleHandler: mockDestinationRuleHandler,
		Cache:                  NewDestinationRuleCache(),
	}

	testCases := []struct {
		name            string
		destinationRule interface{}
		expectedError   error
	}{
		{
			name: "Given context and DestinationRule " +
				"When DestinationRule param is nil " +
				"Then func should return an error",
			destinationRule: nil,
			expectedError:   fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.DestinationRule"),
		},
		{
			name: "Given context and DestinationRule " +
				"When DestinationRule param is not of type *v1alpha3.DestinationRule " +
				"Then func should return an error",
			destinationRule: struct{}{},
			expectedError:   fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.DestinationRule"),
		},
		{
			name: "Given context and DestinationRule " +
				"When DestinationRule param is of type *v1alpha3.DestinationRule " +
				"Then func should not return an error",
			destinationRule: &networking.DestinationRule{},
			expectedError:   nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := destinationRuleController.Added(ctx, tc.destinationRule)
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

func TestDestinationRuleUpdated(t *testing.T) {

	mockDestinationRuleHandler := &test.MockDestinationRuleHandler{}
	ctx := context.Background()
	destinationRuleController := DestinationRuleController{
		DestinationRuleHandler: mockDestinationRuleHandler,
		Cache:                  NewDestinationRuleCache(),
	}

	testCases := []struct {
		name            string
		destinationRule interface{}
		expectedError   error
	}{
		{
			name: "Given context and DestinationRule " +
				"When DestinationRule param is nil " +
				"Then func should return an error",
			destinationRule: nil,
			expectedError:   fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.DestinationRule"),
		},
		{
			name: "Given context and DestinationRule " +
				"When DestinationRule param is not of type *v1alpha3.DestinationRule " +
				"Then func should return an error",
			destinationRule: struct{}{},
			expectedError:   fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.DestinationRule"),
		},
		{
			name: "Given context and DestinationRule " +
				"When DestinationRule param is of type *v1alpha3.DestinationRule " +
				"Then func should not return an error",
			destinationRule: &networking.DestinationRule{},
			expectedError:   nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := destinationRuleController.Updated(ctx, tc.destinationRule, nil)
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

func TestDestinationRuleDeleted(t *testing.T) {

	mockDestinationRuleHandler := &test.MockDestinationRuleHandler{}
	ctx := context.Background()
	destinationRuleController := DestinationRuleController{
		DestinationRuleHandler: mockDestinationRuleHandler,
		Cache:                  NewDestinationRuleCache(),
	}

	testCases := []struct {
		name            string
		destinationRule interface{}
		expectedError   error
	}{
		{
			name: "Given context and DestinationRule " +
				"When DestinationRule param is nil " +
				"Then func should return an error",
			destinationRule: nil,
			expectedError:   fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.DestinationRule"),
		},
		{
			name: "Given context and DestinationRule " +
				"When DestinationRule param is not of type *v1alpha3.DestinationRule " +
				"Then func should return an error",
			destinationRule: struct{}{},
			expectedError:   fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.DestinationRule"),
		},
		{
			name: "Given context and DestinationRule " +
				"When DestinationRule param is of type *v1alpha3.DestinationRule " +
				"Then func should not return an error",
			destinationRule: &networking.DestinationRule{},
			expectedError:   nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := destinationRuleController.Deleted(ctx, tc.destinationRule)
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

func TestNewDestinationRuleController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockDestinationRuleHandler{}

	destinationRuleController, err := NewDestinationRuleController(stop, &handler, "cluster-id1", config, time.Duration(1000), loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if destinationRuleController == nil {
		t.Errorf("DestinationRule controller should never be nil without an error thrown")
	}

	dstRule := &v1alpha3.DestinationRule{Spec: v1alpha32.DestinationRule{}, ObjectMeta: v1.ObjectMeta{Name: "dr1", Namespace: "namespace1"}}

	ctx := context.Background()

	destinationRuleController.Added(ctx, dstRule)
	if !cmp.Equal(&dstRule.Spec, &handler.Obj.Spec, protocmp.Transform()) {
		t.Errorf("Handler should have the added obj")
	}

	updatedDstRule := &v1alpha3.DestinationRule{Spec: v1alpha32.DestinationRule{Host: "hello.global"}, ObjectMeta: v1.ObjectMeta{Name: "dr1", Namespace: "namespace1"}}
	destinationRuleController.Updated(ctx, updatedDstRule, dstRule)

	if !cmp.Equal(&updatedDstRule.Spec, &handler.Obj.Spec, protocmp.Transform()) {
		t.Errorf("Handler should have the updated obj")
	}

	destinationRuleController.Deleted(ctx, dstRule)

	if handler.Obj != nil {
		t.Errorf("Handler should have no obj")
	}
}

// TODO: This is just a placeholder for when we add diff check for other types
func TestDestinationRuleGetProcessItemStatus(t *testing.T) {
	destinationRuleController := DestinationRuleController{}
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
			res, _ := destinationRuleController.GetProcessItemStatus(tc.obj)
			assert.Equal(t, tc.expectedRes, res)
		})
	}
}

func TestDestinationRuleUpdateProcessItemStatus(t *testing.T) {
	var (
		serviceAccount = &coreV1.ServiceAccount{}

		dr1 = &networking.DestinationRule{
			ObjectMeta: v1.ObjectMeta{
				Name:        "debug-incache",
				Namespace:   "namespace1",
				Annotations: map[string]string{"other-annotation": "value"}}}

		dr2 = &networking.DestinationRule{
			ObjectMeta: v1.ObjectMeta{
				Name:        "debug2-incache",
				Namespace:   "namespace1",
				Annotations: map[string]string{"other-annotation": "value"}}}

		drNotInCache = &networking.DestinationRule{
			ObjectMeta: v1.ObjectMeta{
				Name:        "debug",
				Namespace:   "namespace1",
				Annotations: map[string]string{"other-annotation": "value"}}}

		diffNsDrNotInCache = &networking.DestinationRule{
			ObjectMeta: v1.ObjectMeta{
				Name:        "debug",
				Namespace:   "namespace2",
				Annotations: map[string]string{"other-annotation": "value"}}}
	)

	drCache := &DestinationRuleCache{
		cache: make(map[string]*DestinationRuleItem),
		mutex: &sync.RWMutex{},
	}

	destinationRuleController := &DestinationRuleController{
		Cache: drCache,
	}

	drCache.Put(dr1)
	drCache.Put(dr2)

	cases := []struct {
		name           string
		obj            interface{}
		statusToSet    string
		expectedErr    error
		expectedStatus string
	}{
		{
			name: "Given dr cache has a valid dr in its cache, " +
				"Then, the status for the valid dr should be updated to processed",
			obj:            dr1,
			statusToSet:    common.Processed,
			expectedErr:    nil,
			expectedStatus: common.Processed,
		},
		{
			name: "Given dr cache has a valid dr in its cache, " +
				"Then, the status for the valid dr should be updated to not processed",
			obj:            dr2,
			statusToSet:    common.NotProcessed,
			expectedErr:    nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given dr cache does not has a valid dr in its cache, " +
				"Then, the status for the valid dr should be not processed, " +
				"And an error should be returned with the dr not found message",
			obj:         drNotInCache,
			statusToSet: common.NotProcessed,
			expectedErr: fmt.Errorf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Update", "DestinationRule",
				drNotInCache.Name, drNotInCache.Namespace, "", "nothing to update, destinationrule not found in cache"),
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given dr cache does not has a valid dr in its cache, " +
				"And dr is in a different namespace, " +
				"Then, the status for the valid dr should be not processed, " +
				"And an error should be returned with the dr not found message",
			obj:         diffNsDrNotInCache,
			statusToSet: common.NotProcessed,
			expectedErr: fmt.Errorf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Update", "DestinationRule",
				diffNsDrNotInCache.Name, diffNsDrNotInCache.Namespace, "", "nothing to update, destinationrule not found in cache"),
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
			err := destinationRuleController.UpdateProcessItemStatus(c.obj, c.statusToSet)
			if err != nil && c.expectedErr == nil {
				t.Errorf("unexpected error: %v", err)
			}
			if err == nil && c.expectedErr != nil {
				t.Errorf("expected error: %v", c.expectedErr)
			}
			if err != nil && c.expectedErr != nil && !strings.Contains(err.Error(), c.expectedErr.Error()) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			status, _ := destinationRuleController.GetProcessItemStatus(c.obj)
			assert.Equal(t, c.expectedStatus, status)
		})
	}
}

func TestLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a DestinationRule object
	sec := &DestinationRuleController{}
	sec.LogValueOfAdmiralIoIgnore("not a destination rule")
	// No error should occur

	// Test case 2: DestinationRule has no annotations
	sec = &DestinationRuleController{}
	sec.LogValueOfAdmiralIoIgnore(&networking.DestinationRule{})
	// No error should occur

	// Test case 3: AdmiralIgnoreAnnotation is not set
	sec = &DestinationRuleController{}
	dr := &networking.DestinationRule{
		ObjectMeta: v1.ObjectMeta{
			Annotations: map[string]string{"other-annotation": "value"}}}
	sec.LogValueOfAdmiralIoIgnore(dr)
	// No error should occur

	// Test case 4: AdmiralIgnoreAnnotation is set
	sec = &DestinationRuleController{}
	dr = &networking.DestinationRule{ObjectMeta: v1.ObjectMeta{
		Annotations: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}}
	sec.LogValueOfAdmiralIoIgnore(dr)
	// No error should occur
}
