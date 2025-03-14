package istio

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	v1alpha32 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestAdded(t *testing.T) {

	mockVirtualServiceHandler := &test.MockVirtualServiceHandler{}
	ctx := context.Background()
	virtualServiceController := VirtualServiceController{
		VirtualServiceHandler: mockVirtualServiceHandler,
	}

	testCases := []struct {
		name           string
		virtualService interface{}
		expectedError  error
	}{
		{
			name: "Given context and virtualService " +
				"When virtualservice param is nil " +
				"Then func should return an error",
			virtualService: nil,
			expectedError:  fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.VirtualService"),
		},
		{
			name: "Given context and virtualService " +
				"When virtualservice param is not of type *v1alpha3.VirtualService " +
				"Then func should return an error",
			virtualService: struct{}{},
			expectedError:  fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.VirtualService"),
		},
		{
			name: "Given context and virtualService " +
				"When virtualservice param is of type *v1alpha3.VirtualService " +
				"Then func should not return an error",
			virtualService: &v1alpha3.VirtualService{},
			expectedError:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := virtualServiceController.Added(ctx, tc.virtualService)
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

	mockVirtualServiceHandler := &test.MockVirtualServiceHandler{}
	ctx := context.Background()
	virtualServiceController := VirtualServiceController{
		VirtualServiceHandler: mockVirtualServiceHandler,
	}

	testCases := []struct {
		name           string
		virtualService interface{}
		expectedError  error
	}{
		{
			name: "Given context and virtualService " +
				"When virtualservice param is nil " +
				"Then func should return an error",
			virtualService: nil,
			expectedError:  fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.VirtualService"),
		},
		{
			name: "Given context and virtualService " +
				"When virtualservice param is not of type *v1alpha3.VirtualService " +
				"Then func should return an error",
			virtualService: struct{}{},
			expectedError:  fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.VirtualService"),
		},
		{
			name: "Given context and virtualService " +
				"When virtualservice param is of type *v1alpha3.VirtualService " +
				"Then func should not return an error",
			virtualService: &v1alpha3.VirtualService{},
			expectedError:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := virtualServiceController.Updated(ctx, tc.virtualService, nil)
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

	mockVirtualServiceHandler := &test.MockVirtualServiceHandler{}
	ctx := context.Background()
	virtualServiceController := VirtualServiceController{
		VirtualServiceHandler: mockVirtualServiceHandler,
	}

	testCases := []struct {
		name           string
		virtualService interface{}
		expectedError  error
	}{
		{
			name: "Given context and virtualService " +
				"When virtualservice param is nil " +
				"Then func should return an error",
			virtualService: nil,
			expectedError:  fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.VirtualService"),
		},
		{
			name: "Given context and virtualService " +
				"When virtualservice param is not of type *v1alpha3.VirtualService " +
				"Then func should return an error",
			virtualService: struct{}{},
			expectedError:  fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.VirtualService"),
		},
		{
			name: "Given context and virtualService " +
				"When virtualservice param is of type *v1alpha3.VirtualService " +
				"Then func should not return an error",
			virtualService: &v1alpha3.VirtualService{},
			expectedError:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := virtualServiceController.Deleted(ctx, tc.virtualService)
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

func TestNewVirtualServiceController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockVirtualServiceHandler{}

	virtualServiceController, err := NewVirtualServiceController(stop, &handler, config, time.Duration(1000), loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if virtualServiceController == nil {
		t.Errorf("VirtualService controller should never be nil without an error thrown")
	}

	vs := &v1alpha3.VirtualService{Spec: v1alpha32.VirtualService{}, ObjectMeta: v1.ObjectMeta{Name: "vs1", Namespace: "namespace1"}}

	ctx := context.Background()

	virtualServiceController.Added(ctx, vs)

	if !cmp.Equal(&vs.Spec, &handler.Obj.Spec, protocmp.Transform()) {
		t.Errorf("Handler should have the added obj")
	}

	updatedVs := &v1alpha3.VirtualService{Spec: v1alpha32.VirtualService{Hosts: []string{"hello.global"}}, ObjectMeta: v1.ObjectMeta{Name: "vs1", Namespace: "namespace1"}}

	virtualServiceController.Updated(ctx, updatedVs, vs)

	if !cmp.Equal(&updatedVs.Spec, &handler.Obj.Spec, protocmp.Transform()) {
		t.Errorf("Handler should have the updated obj")
	}

	virtualServiceController.Deleted(ctx, vs)

	if handler.Obj != nil {
		t.Errorf("Handler should have no obj")
	}
}

// TODO: This is just a placeholder for when we add diff check for other types
func TestVirtualServiceGetProcessItemStatus(t *testing.T) {
	virtualServiceController := VirtualServiceController{}
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
			res, _ := virtualServiceController.GetProcessItemStatus(tc.obj)
			assert.Equal(t, tc.expectedRes, res)
		})
	}
}

// TODO: This is just a placeholder for when we add diff check for other types
func TestVirtualServiceUpdateProcessItemStatus(t *testing.T) {
	virtualServiceController := VirtualServiceController{}
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
			err := virtualServiceController.UpdateProcessItemStatus(tc.obj, common.NotProcessed)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}

func TestVirtualServiceLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a VirtualService object
	sec := &VirtualServiceController{}
	sec.LogValueOfAdmiralIoIgnore("not a virtual service")
	// No error should occur

	// Test case 2: VirtualService has no annotations
	sec = &VirtualServiceController{}
	sec.LogValueOfAdmiralIoIgnore(&v1alpha3.VirtualService{})
	// No error should occur

	// Test case 3: AdmiralIgnoreAnnotation is not set
	sec = &VirtualServiceController{}
	vs := &v1alpha3.VirtualService{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"other-annotation": "value"}}}
	sec.LogValueOfAdmiralIoIgnore(vs)
	// No error should occur

	// Test case 4: AdmiralIgnoreAnnotation is set in annotations
	sec = &VirtualServiceController{}
	vs = &v1alpha3.VirtualService{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}}
	sec.LogValueOfAdmiralIoIgnore(vs)
	// No error should occur
}

func TestHostToRouteDestinationCachePut(t *testing.T) {

	testCases := []struct {
		name          string
		vs            *v1alpha3.VirtualService
		expectedCache map[string][]*networkingv1alpha3.HTTPRouteDestination
		expectedErr   error
	}{
		{
			name: "Given VS is nil," +
				"When HostToRouteDestinationCache.Put func is called" +
				"Then the func should return error",
			expectedErr: fmt.Errorf("failed HostToRouteDestinationCache.Put as virtualService is nil"),
		},
		{
			name: "Given a virtual service which is not VS routing enabled" +
				"When HostToRouteDestinationCache.Put func is called" +
				"Then the VS should not be cached",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{"other-annotation": "value"},
				},
				Spec: networkingv1alpha3.VirtualService{
					Hosts: []string{"new-env.test-identity.global"},
					Http: []*networkingv1alpha3.HTTPRoute{
						{
							Name: "new-env.test-identity.global",
							Route: []*networkingv1alpha3.HTTPRouteDestination{{
								Destination: &networkingv1alpha3.Destination{
									Host: "new-env.test-identity.global",
								},
							},
							},
						},
					},
				},
			},
			expectedCache: map[string][]*networkingv1alpha3.HTTPRouteDestination{},
			expectedErr:   nil,
		},
		{
			name: "Given a virtual service which contains no http routes" +
				"When HostToRouteDestinationCache.Put func is called" +
				"Then the VS should not be cached",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{"other-annotation": "value"},
				},
				Spec: networkingv1alpha3.VirtualService{
					Hosts: []string{"new-env.test-identity.global"},
				},
			},
			expectedCache: map[string][]*networkingv1alpha3.HTTPRouteDestination{},
			expectedErr:   nil,
		},
		{
			name: "Given a routing virtual service with valid http routes" +
				"When HostToRouteDestinationCache.Put func is called" +
				"Then the VS should be cached",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{common.VSRoutingLabel: "enabled"},
				},
				Spec: networkingv1alpha3.VirtualService{
					Hosts: []string{"test-env.test-identity.global"},
					Http: []*networkingv1alpha3.HTTPRoute{
						{
							Name: "test-env.test-identity.global",
							Route: []*networkingv1alpha3.HTTPRouteDestination{{
								Destination: &networkingv1alpha3.Destination{
									Host: "test-env.test-identity.svc.cluster.local",
								},
							},
							},
						},
					},
				},
			},
			expectedCache: map[string][]*networkingv1alpha3.HTTPRouteDestination{
				"test-env.test-identity.global": {
					&networkingv1alpha3.HTTPRouteDestination{
						Destination: &networkingv1alpha3.Destination{
							Host: "test-env.test-identity.svc.cluster.local",
						},
					},
				},
			},
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHostToRouteDestinationCache()
			err := h.Put(tc.vs)
			if tc.expectedErr != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedErr.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.True(t, reflect.DeepEqual(tc.expectedCache, h.cache))
			}
		})
	}

}
