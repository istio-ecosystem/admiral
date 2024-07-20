package admiral

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"

	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestRoutingPolicyAdded(t *testing.T) {

	routingPolicyHandler := &test.MockRoutingPolicyHandler{}
	ctx := context.Background()
	routingPolicyController := RoutingPolicyController{
		RoutingPolicyHandler: routingPolicyHandler,
	}

	testCases := []struct {
		name          string
		routingPolicy interface{}
		expectedError error
	}{
		{
			name: "Given context and RoutingPolicy " +
				"When RoutingPolicy param is nil " +
				"Then func should return an error",
			routingPolicy: nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.RoutingPolicy"),
		},
		{
			name: "Given context and RoutingPolicy " +
				"When RoutingPolicy param is not of type *v1.RoutingPolicy " +
				"Then func should return an error",
			routingPolicy: struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.RoutingPolicy"),
		},
		{
			name: "Given context and RoutingPolicy " +
				"When RoutingPolicy param is of type *v1.RoutingPolicy " +
				"Then func should not return an error",
			routingPolicy: &v1.RoutingPolicy{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := routingPolicyController.Added(ctx, tc.routingPolicy)
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

func TestRoutingPolicyUpdated(t *testing.T) {

	routingPolicyHandler := &test.MockRoutingPolicyHandler{}
	ctx := context.Background()
	routingPolicyController := RoutingPolicyController{
		RoutingPolicyHandler: routingPolicyHandler,
	}

	testCases := []struct {
		name          string
		routingPolicy interface{}
		expectedError error
	}{
		{
			name: "Given context and RoutingPolicy " +
				"When RoutingPolicy param is nil " +
				"Then func should return an error",
			routingPolicy: nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.RoutingPolicy"),
		},
		{
			name: "Given context and RoutingPolicy " +
				"When RoutingPolicy param is not of type *v1.RoutingPolicy " +
				"Then func should return an error",
			routingPolicy: struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.RoutingPolicy"),
		},
		{
			name: "Given context and RoutingPolicy " +
				"When RoutingPolicy param is of type *v1.RoutingPolicy " +
				"Then func should not return an error",
			routingPolicy: &v1.RoutingPolicy{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := routingPolicyController.Updated(ctx, tc.routingPolicy, nil)
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

func TestRoutingPolicyDeleted(t *testing.T) {

	routingPolicyHandler := &test.MockRoutingPolicyHandler{}
	ctx := context.Background()
	routingPolicyController := RoutingPolicyController{
		RoutingPolicyHandler: routingPolicyHandler,
	}

	testCases := []struct {
		name          string
		routingPolicy interface{}
		expectedError error
	}{
		{
			name: "Given context and RoutingPolicy " +
				"When RoutingPolicy param is nil " +
				"Then func should return an error",
			routingPolicy: nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.RoutingPolicy"),
		},
		{
			name: "Given context and RoutingPolicy " +
				"When RoutingPolicy param is not of type *v1.RoutingPolicy " +
				"Then func should return an error",
			routingPolicy: struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.RoutingPolicy"),
		},
		{
			name: "Given context and RoutingPolicy " +
				"When RoutingPolicy param is of type *v1.RoutingPolicy " +
				"Then func should not return an error",
			routingPolicy: &v1.RoutingPolicy{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := routingPolicyController.Deleted(ctx, tc.routingPolicy)
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

func TestNewroutingPolicyController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockRoutingPolicyHandler{}

	routingPolicyController, err := NewRoutingPoliciesController(stop, &handler, config, time.Duration(1000), loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if routingPolicyController == nil {
		t.Errorf("RoutingPolicy controller should never be nil without an error thrown")
	}
}

func TestRoutingPolicyAddUpdateDelete(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockRoutingPolicyHandler{}
	routingPolicyController, err := NewRoutingPoliciesController(stop, &handler, config, time.Duration(1000), loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if routingPolicyController == nil {
		t.Errorf("RoutingPolicy controller should never be nil without an error thrown")
	}

	rpName := "greetingRoutingPolicy"
	rp := model.RoutingPolicy{
		Config: map[string]string{"cacheTTL": "86400", "dispatcherUrl": "stage.greeting.router.mesh", "pathPrefix": "/hello,/hello/v2/"},
		Plugin: "greeting",
		Hosts:  []string{"stage.greeting.mesh"},
	}

	ctx := context.Background()

	rpObj := makeK8sRoutingPolicyObj(rpName, "namespace1", rp)
	routingPolicyController.Added(ctx, rpObj)

	if !cmp.Equal(handler.Obj.Spec, rpObj.Spec) {
		t.Errorf("Add should call the handler with the object")
	}

	updatedrp := model.RoutingPolicy{
		Config: map[string]string{"cacheTTL": "86400", "dispatcherUrl": "e2e.greeting.router.mesh", "pathPrefix": "/hello,/hello/v2/"},
		Plugin: "greeting",
		Hosts:  []string{"e2e.greeting.mesh"},
	}

	updatedRpObj := makeK8sRoutingPolicyObj(rpName, "namespace1", updatedrp)

	routingPolicyController.Updated(ctx, updatedRpObj, rpObj)

	if !cmp.Equal(handler.Obj.Spec, updatedRpObj.Spec) {
		t.Errorf("Update should call the handler with the updated object")
	}

	routingPolicyController.Deleted(ctx, updatedRpObj)

	if handler.Obj != nil {
		t.Errorf("Delete should delete the routing policy")
	}

}

func makeK8sRoutingPolicyObj(name string, namespace string, rp model.RoutingPolicy) *v1.RoutingPolicy {
	return &v1.RoutingPolicy{
		Spec:       rp,
		ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace},
		TypeMeta: v12.TypeMeta{
			APIVersion: "admiral.io/v1",
			Kind:       "RoutingPolicy",
		}}
}

// TODO: This is just a placeholder for when we add diff check for other types
func TestRoutingPolicyGetProcessItemStatus(t *testing.T) {
	routingPolicyController := RoutingPolicyController{}
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
			res, _ := routingPolicyController.GetProcessItemStatus(tc.obj)
			assert.Equal(t, tc.expectedRes, res)
		})
	}
}

// TODO: This is just a placeholder for when we add diff check for other types
func TestRoutingPolicyUpdateProcessItemStatus(t *testing.T) {
	routingPolicyController := RoutingPolicyController{}
	testCases := []struct {
		name        string
		obj         interface{}
		expectedErr error
	}{
		{
			name:        "TODO: Currently always returns nil",
			obj:         nil,
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := routingPolicyController.UpdateProcessItemStatus(tc.obj, common.NotProcessed)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}

func TestRoutingPolicyLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a RoutingPolicy object
	r := &RoutingPolicyController{}
	r.LogValueOfAdmiralIoIgnore("not a routing policy")
	// No error should occur

	// Test case 2: RoutingPolicy has no annotations or labels
	r = &RoutingPolicyController{}
	r.LogValueOfAdmiralIoIgnore(&v1.RoutingPolicy{})
	// No error should occur

	// Test case 3: AdmiralIgnoreAnnotation is not set
	r = &RoutingPolicyController{}
	rp := &v1.RoutingPolicy{ObjectMeta: v12.ObjectMeta{Annotations: map[string]string{"other-annotation": "value"}}}
	r.LogValueOfAdmiralIoIgnore(rp)
	// No error should occur

	// Test case 4: AdmiralIgnoreAnnotation is set in annotations
	r = &RoutingPolicyController{}
	rp = &v1.RoutingPolicy{ObjectMeta: v12.ObjectMeta{Annotations: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}}
	r.LogValueOfAdmiralIoIgnore(rp)
	// No error should occur

	// Test case 5: AdmiralIgnoreAnnotation is set in labels
	r = &RoutingPolicyController{}
	rp = &v1.RoutingPolicy{ObjectMeta: v12.ObjectMeta{Labels: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}}
	r.LogValueOfAdmiralIoIgnore(rp)
	// No error should occur
}
