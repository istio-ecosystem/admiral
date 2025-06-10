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
		VirtualServiceHandler:       mockVirtualServiceHandler,
		VirtualServiceCache:         NewVirtualServiceCache(),
		HostToRouteDestinationCache: NewHostToRouteDestinationCache(),
		IdentityVirtualServiceCache: MockIdentityNamespaceVirtualServiceCache{},
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
		VirtualServiceHandler:       mockVirtualServiceHandler,
		VirtualServiceCache:         NewVirtualServiceCache(),
		HostToRouteDestinationCache: NewHostToRouteDestinationCache(),
		IdentityVirtualServiceCache: MockIdentityNamespaceVirtualServiceCache{},
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
		VirtualServiceHandler:       mockVirtualServiceHandler,
		VirtualServiceCache:         NewVirtualServiceCache(),
		HostToRouteDestinationCache: NewHostToRouteDestinationCache(),
		IdentityVirtualServiceCache: MockIdentityNamespaceVirtualServiceCache{},
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

func TestVirtualServiceGetProcessItemStatus(t *testing.T) {
	virtualServiceController := VirtualServiceController{
		VirtualServiceCache: &MockVirtualServiceCache{
			Status: common.Processed,
		},
	}
	testCases := []struct {
		name           string
		obj            interface{}
		expectedStatus string
		expectedErr    error
	}{
		{
			name: "Given an invalid vs" +
				"When UpdateProcessItemStatus is called" +
				"Then func should return error",
			obj:         nil,
			expectedErr: fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.VirtualService"),
		},
		{
			name: "Given an valid vs" +
				"When UpdateProcessItemStatus is called" +
				"Then func should return the status",
			obj: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{Name: "vs1"},
			},
			expectedStatus: common.Processed,
			expectedErr:    nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := virtualServiceController.GetProcessItemStatus(tc.obj)
			if tc.expectedErr != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedErr.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedStatus, res)
			}
		})
	}
}

func TestVirtualServiceUpdateProcessItemStatus(t *testing.T) {
	virtualServiceController := VirtualServiceController{
		VirtualServiceCache: &MockVirtualServiceCache{},
	}
	testCases := []struct {
		name        string
		obj         interface{}
		status      string
		mockedError error
		expectedErr error
	}{
		{
			name: "Given an invalid vs" +
				"When UpdateProcessItemStatus is called" +
				"Then func should return error",
			obj:         nil,
			expectedErr: fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.VirtualService"),
		},
		{
			name: "Given a valid vs and status" +
				"When UpdateProcessItemStatus is called" +
				"Then func should not return error",
			obj: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{Name: "vs1"},
			},
			status:      common.Processed,
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := virtualServiceController.UpdateProcessItemStatus(tc.obj, tc.status)
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

func TestHostToRouteDestinationCacheDelete(t *testing.T) {

	h := NewHostToRouteDestinationCache()
	h.cache["stage.host.global"] = []*networkingv1alpha3.HTTPRouteDestination{{
		Destination: &networkingv1alpha3.Destination{
			Host: "stage.host.svc.cluster.local",
		},
	}}
	h.cache["test-env.test-identity.global"] = []*networkingv1alpha3.HTTPRouteDestination{{
		Destination: &networkingv1alpha3.Destination{
			Host: "test-env.test-identity.svc.cluster.local",
		},
	}}

	testCases := []struct {
		name          string
		vs            *v1alpha3.VirtualService
		expectedCache map[string][]*networkingv1alpha3.HTTPRouteDestination
		expectedErr   error
	}{
		{
			name: "Given VS is nil," +
				"When HostToRouteDestinationCache.Delete func is called" +
				"Then the func should return error",
			expectedErr: fmt.Errorf("failed HostToRouteDestinationCache.Delete as virtualService is nil"),
		},
		{
			name: "Given a routing virtual service with valid http routes" +
				"When HostToRouteDestinationCache.Delete func is called" +
				"Then the VS should be cached",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{common.VSRoutingType: common.VSRoutingTypeInCluster},
				},
				Spec: networkingv1alpha3.VirtualService{
					Hosts: []string{"stage.host.global"},
					Http: []*networkingv1alpha3.HTTPRoute{
						{
							Name: "stage.host.global",
							Route: []*networkingv1alpha3.HTTPRouteDestination{{
								Destination: &networkingv1alpha3.Destination{
									Host: "stage.host.svc.cluster.local",
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
			err := h.Delete(tc.vs)
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
			name: "Given a virtual service which is not in-cluster vs" +
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
					Annotations: map[string]string{common.VSRoutingType: common.VSRoutingTypeInCluster},
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
					Labels: map[string]string{common.VSRoutingType: common.VSRoutingTypeInCluster},
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
		{
			name: "Given a routing virtual service with valid http routes" +
				"And if the VS contains names that are not FQDNs" +
				"When HostToRouteDestinationCache.Put func is called" +
				"Then the VS should should only cache the names with FQDNs",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{common.VSRoutingType: common.VSRoutingTypeInCluster},
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
						{
							Name: "all",
							Route: []*networkingv1alpha3.HTTPRouteDestination{{
								Destination: &networkingv1alpha3.Destination{
									Host: "test-all.test-identity.svc.cluster.local",
								},
							},
							},
						},
						{
							Name: "health check",
							Route: []*networkingv1alpha3.HTTPRouteDestination{{
								Destination: &networkingv1alpha3.Destination{
									Host: "test-health-check.test-identity.svc.cluster.local",
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

	admiralParams := common.AdmiralParams{
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

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

func TestHostToRouteDestinationCacheGet(t *testing.T) {

	h := NewHostToRouteDestinationCache()
	h.cache["stage.host.global"] = []*networkingv1alpha3.HTTPRouteDestination{{
		Destination: &networkingv1alpha3.Destination{
			Host: "stage.host.svc.cluster.local",
		},
	}}

	testCases := []struct {
		name          string
		routeName     string
		expectedCache []*networkingv1alpha3.HTTPRouteDestination
	}{
		{
			name: "Given a route name that doesn't exist" +
				"When HostToRouteDestinationCache.Get func is called" +
				"Then the cache should return nil",
			routeName:     "test.host.global",
			expectedCache: nil,
		},
		{
			name: "Given a route name that does exist" +
				"When HostToRouteDestinationCache.Get func is called" +
				"Then the cache should return the cached value",
			routeName: "stage.host.global",
			expectedCache: []*networkingv1alpha3.HTTPRouteDestination{{
				Destination: &networkingv1alpha3.Destination{
					Host: "stage.host.svc.cluster.local",
				},
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := h.Get(tc.routeName)
			assert.Equal(t, tc.expectedCache, actual)
		})
	}

}

func TestVirtualServiceCachePut(t *testing.T) {

	validVS := &v1alpha3.VirtualService{
		ObjectMeta: v1.ObjectMeta{
			Name:   "stage.host.global-incluster-vs",
			Labels: map[string]string{common.VSRoutingLabel: "enabled"},
		},
	}

	testCases := []struct {
		name          string
		vs            *v1alpha3.VirtualService
		expectedCache map[string]*VirtualServiceItem
		expectedErr   error
	}{
		{
			name: "Given a nil vs" +
				"When VirtualServiceCache.Put func is called" +
				"Then the func should return an error",
			vs:          nil,
			expectedErr: fmt.Errorf("vs is nil"),
		},
		{
			name: "Given a vs which is not vs routing enabled" +
				"When VirtualServiceCache.Put func is called" +
				"Then the func should not cache it",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{"other-annotation": "true"},
				},
			},
			expectedCache: map[string]*VirtualServiceItem{},
		},
		{
			name: "Given a vs which is vs routing enabled" +
				"When VirtualServiceCache.Put func is called" +
				"Then the func should cache it",
			vs: validVS,
			expectedCache: map[string]*VirtualServiceItem{
				"stage.host.global-incluster-vs": {
					VirtualService: validVS,
					Status:         common.ProcessingInProgress,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cache := NewVirtualServiceCache()
			err := cache.Put(tc.vs)
			if tc.expectedErr != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedErr.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedCache[tc.vs.Name], cache.cache[tc.vs.Name])
			}
		})
	}

}

func TestVirtualServiceCacheGet(t *testing.T) {

	validVS := &v1alpha3.VirtualService{
		ObjectMeta: v1.ObjectMeta{
			Name:        "stage.host.global-incluster-vs",
			Annotations: map[string]string{common.VSRoutingLabel: "enabled"},
		},
	}
	vsCache := NewVirtualServiceCache()
	vsCache.cache["stage.host.global-incluster-vs"] = &VirtualServiceItem{
		VirtualService: validVS,
		Status:         common.ProcessingInProgress,
	}

	testCases := []struct {
		name       string
		vsName     string
		expectedVS *v1alpha3.VirtualService
	}{
		{
			name: "Given a vs name which does not exists in cache" +
				"When VirtualServiceCache.Get func is called" +
				"Then the func should return nil",
			vsName:     "test.host.global-incluster-vs",
			expectedVS: nil,
		},
		{
			name: "Given a vs name which does exists in cache" +
				"When VirtualServiceCache.Get func is called" +
				"Then the func should return VirtualService",
			vsName:     "stage.host.global-incluster-vs",
			expectedVS: validVS,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vs := vsCache.Get(tc.vsName)
			assert.Equal(t, tc.expectedVS, vs)
		})
	}

}

func TestVirtualServiceCacheDelete(t *testing.T) {

	validVS := &v1alpha3.VirtualService{
		ObjectMeta: v1.ObjectMeta{
			Name:        "stage.host.global-incluster-vs",
			Annotations: map[string]string{common.VSRoutingLabel: "enabled"},
		},
	}
	vsCache := NewVirtualServiceCache()
	vsCache.cache["stage.host.global-incluster-vs"] = &VirtualServiceItem{
		VirtualService: validVS,
		Status:         common.ProcessingInProgress,
	}
	vsCache.cache["test.host.global-incluster-vs"] = &VirtualServiceItem{
		VirtualService: validVS,
		Status:         common.ProcessingInProgress,
	}

	testCases := []struct {
		name          string
		vs            *v1alpha3.VirtualService
		expectedCache map[string]*VirtualServiceItem
	}{
		{
			name: "Given a vs is nil" +
				"When VirtualServiceCache.Delete func is called" +
				"Then the func should do nothing",
			expectedCache: vsCache.cache,
		},
		{
			name: "Given a vs is not vs routing enabled" +
				"When VirtualServiceCache.Delete func is called" +
				"Then the func should do nothing",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{"other-annotation": "disabled"},
				},
			},
			expectedCache: vsCache.cache,
		},
		{
			name: "Given a vs is not in the cache" +
				"When VirtualServiceCache.Delete func is called" +
				"Then the func should do nothing",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name:        "dummy.host.global-incluster-vs",
					Annotations: map[string]string{common.VSRoutingLabel: "enabled"},
				},
			},
			expectedCache: vsCache.cache,
		},
		{
			name: "Given a vs is in the cache" +
				"When VirtualServiceCache.Delete func is called" +
				"Then the func delete the VS from the cache",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name:   "stage.host.global-incluster-vs",
					Labels: map[string]string{common.VSRoutingLabel: "enabled"},
				},
			},
			expectedCache: map[string]*VirtualServiceItem{
				"test.host.global-incluster-vs": {
					VirtualService: validVS,
					Status:         common.ProcessingInProgress,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vsCache.Delete(tc.vs)
			assert.Equal(t, tc.expectedCache, vsCache.cache)
		})
	}

}

func TestGetVSProcessStatus(t *testing.T) {

	vsCache := NewVirtualServiceCache()
	vsCache.cache["stage.host.global-incluster-vs"] = &VirtualServiceItem{
		Status: common.ProcessingInProgress,
	}

	testCases := []struct {
		name           string
		vs             *v1alpha3.VirtualService
		expectedStatus string
	}{
		{
			name: "Given a vs is nil" +
				"When GetVSProcessStatus func is called" +
				"Then the func should return NotProcessed",
			vs:             nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given a vs is not in the cache" +
				"When GetVSProcessStatus func is called" +
				"Then the func should return NotProcessed",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name: "dummy.host.global-incluster-vs",
				},
			},
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given a vs is in the cache" +
				"When GetVSProcessStatus func is called" +
				"Then the func should return NotProcessed",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name: "stage.host.global-incluster-vs",
				},
			},
			expectedStatus: common.ProcessingInProgress,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := vsCache.GetVSProcessStatus(tc.vs)
			assert.Equal(t, tc.expectedStatus, status)
		})
	}

}

func TestUpdateVSProcessStatus(t *testing.T) {

	vsCache := NewVirtualServiceCache()
	vsCache.cache["stage.host.global-incluster-vs"] = &VirtualServiceItem{
		Status: common.ProcessingInProgress,
	}

	testCases := []struct {
		name          string
		vs            *v1alpha3.VirtualService
		status        string
		expectedCache map[string]*VirtualServiceItem
		expectedErr   error
	}{
		{
			name: "Given a vs is nil" +
				"When UpdateVSProcessStatus func is called" +
				"Then the func should return an error",
			vs:          nil,
			expectedErr: fmt.Errorf("vs is nil"),
		},
		{
			name: "Given a vs is not in the cache" +
				"When UpdateVSProcessStatus func is called" +
				"Then the func should do nothing",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name: "dummy.host.global-incluster-vs",
				},
			},
			expectedCache: vsCache.cache,
		},
		{
			name: "Given a vs is in the cache" +
				"When UpdateVSProcessStatus func is called" +
				"Then the func should update the status",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name: "stage.host.global-incluster-vs",
				},
			},
			status: common.Processed,
			expectedCache: map[string]*VirtualServiceItem{
				"stage.host.global-incluster-vs": {
					Status: common.Processed,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := vsCache.UpdateVSProcessStatus(tc.vs, tc.status)
			if tc.expectedErr != nil {
				assert.NotNil(t, err)
				assert.Error(t, err, tc.expectedErr)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedCache, vsCache.cache)
			}
		})
	}

}

func TestIdentityVirtualServiceCachePut(t *testing.T) {

	ap := common.AdmiralParams{
		SyncNamespace: "sync-ns",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	validVS := &v1alpha3.VirtualService{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-vs",
			Namespace: "test-ns",
		},
		Spec: networkingv1alpha3.VirtualService{
			Hosts: []string{"stage.foo.host.global"},
		},
	}

	testCases := []struct {
		name          string
		vs            *v1alpha3.VirtualService
		expectedCache map[string]map[string]*v1alpha3.VirtualService
		expectedError error
	}{
		{
			name: "Given a vs with no name" +
				"When IdentityVirtualServiceCache.Put func is called" +
				"Then the func should return an error",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test-ns",
				},
			},
			expectedError: fmt.Errorf(
				"failed to put virtualService in IdentityVirtualServiceCache as vs name is empty"),
		},
		{
			name: "Given a vs with no namespace" +
				"When IdentityVirtualServiceCache.Put func is called" +
				"Then the func should return an error",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-vs",
				},
			},
			expectedError: fmt.Errorf(
				"failed to put virtualService in IdentityVirtualServiceCache as vs namespace is empty"),
		},
		{
			name: "Given a vs with no hosts" +
				"When IdentityVirtualServiceCache.Put func is called" +
				"Then the func should not add the VS to the cache",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-vs",
					Namespace: "test-ns",
				},
			},
			expectedCache: make(map[string]map[string]*v1alpha3.VirtualService),
			expectedError: nil,
		},
		{
			name: "Given a vs with dummy prefixed host" +
				"When IdentityVirtualServiceCache.Put func is called" +
				"Then the func should not add the VS to the cache",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-vs",
					Namespace: "test-ns",
				},
				Spec: networkingv1alpha3.VirtualService{
					Hosts: []string{"dummy.host.global"},
				},
			},
			expectedCache: make(map[string]map[string]*v1alpha3.VirtualService),
			expectedError: nil,
		},
		{
			name: "Given a valid vs" +
				"When IdentityVirtualServiceCache.Put func is called" +
				"Then the func should add the VS to the cache",
			vs: validVS,
			expectedCache: map[string]map[string]*v1alpha3.VirtualService{
				"foo.host": {
					"test-vs": validVS,
				},
				"host": {
					"test-vs": validVS,
				},
			},
			expectedError: nil,
		},
		{
			name: "Given a vs which was created in sync NS" +
				"When IdentityVirtualServiceCache.Put func is called" +
				"Then the func should not add the VS to the cache",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-vs",
					Namespace: "sync-ns",
				},
			},
			expectedCache: make(map[string]map[string]*v1alpha3.VirtualService),
			expectedError: nil,
		},
		{
			name: "Given a vs which was created in istio-system NS" +
				"When IdentityVirtualServiceCache.Put func is called" +
				"Then the func should not add the VS to the cache",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-vs",
					Namespace: "istio-system",
				},
			},
			expectedCache: make(map[string]map[string]*v1alpha3.VirtualService),
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			idNSCache := NewIdentityVirtualServiceCache()
			err := idNSCache.Put(tc.vs)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedCache, idNSCache.cache)
			}
		})
	}

}

func TestIdentityVirtualServiceCacheDelete(t *testing.T) {

	ap := common.AdmiralParams{
		SyncNamespace: "sync-ns",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	validVS := &v1alpha3.VirtualService{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-vs",
			Namespace: "test-ns",
		},
		Spec: networkingv1alpha3.VirtualService{
			Hosts: []string{"stage.foo.host.global"},
		},
	}

	idNSCache := NewIdentityVirtualServiceCache()
	idNSCache.Put(validVS)

	testCases := []struct {
		name          string
		vs            *v1alpha3.VirtualService
		expectedCache map[string]map[string]*v1alpha3.VirtualService
		expectedError error
	}{
		{
			name: "Given a vs with no name" +
				"When IdentityVirtualServiceCache.Delete func is called" +
				"Then the func should return an error",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test-ns",
				},
			},
			expectedError: fmt.Errorf(
				"failed to delete virtualService in IdentityVirtualServiceCache as vs name is empty"),
		},
		{
			name: "Given a vs with no namespace" +
				"When IdentityVirtualServiceCache.Delete func is called" +
				"Then the func should return an error",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-vs",
				},
			},
			expectedError: fmt.Errorf(
				"failed to delete virtualService in IdentityVirtualServiceCache as vs namespace is empty"),
		},
		{
			name: "Given a vs on some other host" +
				"When IdentityVirtualServiceCache.Delete func is called" +
				"Then the func should not delete any VS from the cache",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-vs",
					Namespace: "test-someotherns",
				},
				Spec: networkingv1alpha3.VirtualService{
					Hosts: []string{"foo.bar.global"},
				},
			},
			expectedCache: idNSCache.cache,
			expectedError: nil,
		},
		{
			name: "Given a vs on someother name" +
				"When IdentityVirtualServiceCache.Delete func is called" +
				"Then the func should not delete any VS from the cache",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-foo-vs",
					Namespace: "test-ns",
				},
				Spec: networkingv1alpha3.VirtualService{
					Hosts: []string{"dummy.host.global"},
				},
			},
			expectedCache: idNSCache.cache,
			expectedError: nil,
		},
		{
			name: "Given a valid vs" +
				"When IdentityVirtualServiceCache.Delete func is called" +
				"Then the func should delete the VS from the cache",
			vs: validVS,
			expectedCache: map[string]map[string]*v1alpha3.VirtualService{
				"foo.host": {},
				"host":     {},
			},
			expectedError: nil,
		},
		{
			name: "Given a vs created in sync NS" +
				"When IdentityVirtualServiceCache.Delete func is called" +
				"Then the func should not delete any VS from the cache",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-bar-vs",
					Namespace: "sync-ns",
				},
				Spec: networkingv1alpha3.VirtualService{
					Hosts: []string{"bar.host.global"},
				},
			},
			expectedCache: idNSCache.cache,
			expectedError: nil,
		},
		{
			name: "Given a vs created in istio-system NS" +
				"When IdentityVirtualServiceCache.Delete func is called" +
				"Then the func should not delete any VS from the cache",
			vs: &v1alpha3.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-bar-vs",
					Namespace: "istio-system",
				},
				Spec: networkingv1alpha3.VirtualService{
					Hosts: []string{"bar.host.global"},
				},
			},
			expectedCache: idNSCache.cache,
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := idNSCache.Delete(tc.vs)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedCache, idNSCache.cache)
			}
		})
	}

}

func TestGetIdentitiesFromVSHostName(t *testing.T) {

	testCases := []struct {
		name               string
		hostName           string
		expectedIdentities []string
	}{
		{
			name: "Given an empty host name" +
				"When getIdentitiesFromVSHostName func is called" +
				"Then the func should return nil",
			hostName:           "",
			expectedIdentities: nil,
		},
		{
			name: "Given an invalid host name" +
				"When getIdentitiesFromVSHostName func is called" +
				"Then the func should return nil",
			hostName:           "foo.global",
			expectedIdentities: nil,
		},
		{
			name: "Given a valid host name with a canary prefix" +
				"When getIdentitiesFromVSHostName func is called" +
				"Then the func should return a slice of identities",
			hostName:           "canary.stage.foo.global",
			expectedIdentities: []string{"stage.foo", "foo"},
		},
		{
			name: "Given a valid default host name" +
				"When getIdentitiesFromVSHostName func is called" +
				"Then the func should return a slice of identities",
			hostName:           "stage.foo.global",
			expectedIdentities: []string{"foo"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := getIdentitiesFromVSHostName(tc.hostName)
			assert.Equal(t, tc.expectedIdentities, actual)
		})
	}

}

type MockVirtualServiceCache struct {
	Status string
	Error  error
}

func (m *MockVirtualServiceCache) Put(vs *v1alpha3.VirtualService) error {
	return m.Error
}

func (m *MockVirtualServiceCache) Get(vsName string) *v1alpha3.VirtualService {
	return nil
}

func (m *MockVirtualServiceCache) Delete(vs *v1alpha3.VirtualService) {
}

func (m *MockVirtualServiceCache) GetVSProcessStatus(*v1alpha3.VirtualService) string {
	return m.Status
}

func (m *MockVirtualServiceCache) UpdateVSProcessStatus(*v1alpha3.VirtualService, string) error {
	return m.Error
}

type MockIdentityNamespaceVirtualServiceCache struct {
}

func (m MockIdentityNamespaceVirtualServiceCache) Put(*v1alpha3.VirtualService) error {
	return nil
}
func (m MockIdentityNamespaceVirtualServiceCache) Delete(*v1alpha3.VirtualService) error {
	return nil
}
func (m MockIdentityNamespaceVirtualServiceCache) Get(string) map[string]*v1alpha3.VirtualService {
	return nil
}
