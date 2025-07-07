package istio

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"

	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	v1alpha32 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestSidecarAdded(t *testing.T) {

	mockSidecarHandler := &test.MockSidecarHandler{}
	ctx := context.Background()
	sidecarController := SidecarController{
		SidecarHandler: mockSidecarHandler,
		SidecarCache:   NewSidecarCache(),
	}

	testCases := []struct {
		name          string
		sidecar       interface{}
		expectedError error
	}{
		{
			name: "Given context and sidecar " +
				"When sidecar param is nil " +
				"Then func should return an error",
			sidecar:       nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.Sidecar"),
		},
		{
			name: "Given context and sidecar " +
				"When sidecar param is not of type *v1alpha3.Sidecar " +
				"Then func should return an error",
			sidecar:       struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.Sidecar"),
		},
		{
			name: "Given context and Sidecar " +
				"When Sidecar param is of type *v1alpha3.Sidecar " +
				"Then func should not return an error",
			sidecar:       &v1alpha3.Sidecar{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := sidecarController.Added(ctx, tc.sidecar)
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

func TestSidecarUpdated(t *testing.T) {

	mockSidecarHandler := &test.MockSidecarHandler{}
	ctx := context.Background()
	sidecarController := SidecarController{
		SidecarHandler: mockSidecarHandler,
		SidecarCache:   NewSidecarCache(),
	}

	testCases := []struct {
		name          string
		sidecar       interface{}
		expectedError error
	}{
		{
			name: "Given context and sidecar " +
				"When sidecar param is nil " +
				"Then func should return an error",
			sidecar:       nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.Sidecar"),
		},
		{
			name: "Given context and sidecar " +
				"When sidecar param is not of type *v1alpha3.Sidecar " +
				"Then func should return an error",
			sidecar:       struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.Sidecar"),
		},
		{
			name: "Given context and Sidecar " +
				"When Sidecar param is of type *v1alpha3.Sidecar " +
				"Then func should not return an error",
			sidecar:       &v1alpha3.Sidecar{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := sidecarController.Updated(ctx, tc.sidecar, nil)
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

func TestSidecarDeleted(t *testing.T) {

	mockSidecarHandler := &test.MockSidecarHandler{}
	ctx := context.Background()
	sidecarController := SidecarController{
		SidecarHandler: mockSidecarHandler,
		SidecarCache:   NewSidecarCache(),
	}

	testCases := []struct {
		name          string
		sidecar       interface{}
		expectedError error
	}{
		{
			name: "Given context and sidecar " +
				"When sidecar param is nil " +
				"Then func should return an error",
			sidecar:       nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.Sidecar"),
		},
		{
			name: "Given context and sidecar " +
				"When sidecar param is not of type *v1alpha3.Sidecar " +
				"Then func should return an error",
			sidecar:       struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.Sidecar"),
		},
		{
			name: "Given context and Sidecar " +
				"When Sidecar param is of type *v1alpha3.Sidecar " +
				"Then func should not return an error",
			sidecar:       &v1alpha3.Sidecar{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := sidecarController.Deleted(ctx, tc.sidecar)
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

func TestNewSidecarController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockSidecarHandler{}

	sidecarController, err := NewSidecarController(stop, &handler, config, time.Duration(1000), loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if sidecarController == nil {
		t.Errorf("Sidecar controller should never be nil without an error thrown")
	}

	sc := &v1alpha3.Sidecar{Spec: v1alpha32.Sidecar{}, ObjectMeta: v1.ObjectMeta{Name: "sc1", Namespace: "namespace1"}}

	ctx := context.Background()

	sidecarController.Added(ctx, sc)

	if !cmp.Equal(&sc.Spec, &handler.Obj.Spec, protocmp.Transform()) {
		t.Errorf("Handler should have the added obj")
	}

	updatedSc := &v1alpha3.Sidecar{Spec: v1alpha32.Sidecar{WorkloadSelector: &v1alpha32.WorkloadSelector{Labels: map[string]string{"this": "that"}}}, ObjectMeta: v1.ObjectMeta{Name: "sc1", Namespace: "namespace1"}}
	sidecarController.Updated(ctx, updatedSc, sc)

	if !cmp.Equal(&updatedSc.Spec, &handler.Obj.Spec, protocmp.Transform()) {
		t.Errorf("Handler should have the updated obj")
	}

	sidecarController.Deleted(ctx, sc)

	if handler.Obj != nil {
		t.Errorf("Handler should have no obj")
	}
}

// TODO: This is just a placeholder for when we add diff check for other types
func TestSideCarGetProcessItemStatus(t *testing.T) {
	sidecarController := SidecarController{}
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
			res, _ := sidecarController.GetProcessItemStatus(tc.obj)
			assert.Equal(t, tc.expectedRes, res)
		})
	}
}

// TODO: This is just a placeholder for when we add diff check for other types
func TestSideCarUpdateProcessItemStatus(t *testing.T) {
	sidecarController := SidecarController{
		SidecarCache: NewSidecarCache(),
	}
	testCases := []struct {
		name        string
		obj         interface{}
		expectedErr error
	}{
		{
			name:        "TODO: Currently always returns nil",
			obj:         &v1alpha3.Sidecar{},
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := sidecarController.UpdateProcessItemStatus(tc.obj, common.NotProcessed)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}

func TestSidecarCachePut(t *testing.T) {

	tests := []struct {
		name                              string
		sidecar                           *v1alpha3.Sidecar
		enableSidecarCaching              bool
		maxSidecarEgressHostsLimitToCache int
		expectedError                     string
		expectedCache                     map[string]*SidecarItem
	}{
		{
			name: "Given enableSidecarCaching is set to false " +
				"When SidecarCache.Put is called with a sidecar " +
				"Then it should return no error and an empty cache",
			sidecar: &v1alpha3.Sidecar{
				ObjectMeta: v1.ObjectMeta{
					Name:      "default",
					Namespace: "test-namespace",
				},
				Spec: v1alpha32.Sidecar{
					Egress: []*v1alpha32.IstioEgressListener{
						{
							Hosts: []string{"*.example.com"},
						},
					},
				},
			},
			maxSidecarEgressHostsLimitToCache: 100,
			enableSidecarCaching:              false,
			expectedCache:                     map[string]*SidecarItem{},
		},
		{
			name: "Given nil sidecar object" +
				"When SidecarCache.Put is called with a sidecar " +
				"Then it should return an error",
			sidecar:              nil,
			expectedError:        "sidecar is nil",
			enableSidecarCaching: true,
			expectedCache:        map[string]*SidecarItem{},
		},
		{
			name: "Given a non default sidecar" +
				"When SidecarCache.Put is called with a sidecar " +
				"Then it should return no error and an empty cache",
			sidecar: &v1alpha3.Sidecar{
				ObjectMeta: v1.ObjectMeta{
					Name:      "non-default-sidecar",
					Namespace: "test-namespace",
				},
				Spec: v1alpha32.Sidecar{
					Egress: []*v1alpha32.IstioEgressListener{
						{
							Hosts: []string{"*.example.com"},
						},
					},
				},
			},
			maxSidecarEgressHostsLimitToCache: 100,
			enableSidecarCaching:              true,
			expectedCache:                     map[string]*SidecarItem{},
		},
		{
			name: "Given default sidecar with no Egress config" +
				"When SidecarCache.Put is called with a sidecar " +
				"Then it should return an error",
			sidecar: &v1alpha3.Sidecar{
				ObjectMeta: v1.ObjectMeta{
					Name:      "default",
					Namespace: "test-namespace",
				},
				Spec: v1alpha32.Sidecar{},
			},
			expectedError:                     "sidecar has no egress",
			enableSidecarCaching:              true,
			maxSidecarEgressHostsLimitToCache: 100,
			expectedCache:                     map[string]*SidecarItem{},
		},
		{
			name: "Given default sidecar with empty Egress hosts" +
				"When SidecarCache.Put is called with a sidecar " +
				"Then it should return an error",
			sidecar: &v1alpha3.Sidecar{
				ObjectMeta: v1.ObjectMeta{
					Name:      "default",
					Namespace: "test-namespace",
				},
				Spec: v1alpha32.Sidecar{
					Egress: []*v1alpha32.IstioEgressListener{
						{},
					},
				},
			},
			expectedError:                     "sidecar has no hosts",
			enableSidecarCaching:              true,
			maxSidecarEgressHostsLimitToCache: 100,
			expectedCache:                     map[string]*SidecarItem{},
		},
		{
			name: "Given default sidecar with egress hosts more than the max limit" +
				"When SidecarCache.Put is called with a sidecar " +
				"Then it should not return an error and return an empty cache",
			sidecar: &v1alpha3.Sidecar{
				ObjectMeta: v1.ObjectMeta{
					Name:      "default",
					Namespace: "test-namespace",
				},
				Spec: v1alpha32.Sidecar{
					Egress: []*v1alpha32.IstioEgressListener{
						{
							Hosts: []string{"host1.example.com", "host2.example.com", "host3.example.com"},
						},
					},
				},
			},
			maxSidecarEgressHostsLimitToCache: 2,
			enableSidecarCaching:              true,
			expectedCache:                     map[string]*SidecarItem{},
		},
		{
			name: "Given a valid default sidecar" +
				"When SidecarCache.Put is called with a sidecar " +
				"Then it should not return an error and the sidecar should be cached",
			sidecar: &v1alpha3.Sidecar{
				ObjectMeta: v1.ObjectMeta{
					Name:      "default",
					Namespace: "test-namespace",
				},
				Spec: v1alpha32.Sidecar{
					Egress: []*v1alpha32.IstioEgressListener{
						{
							Hosts: []string{"host1.example.com", "host2.example.com", "host3.example.com"},
						},
					},
				},
			},
			maxSidecarEgressHostsLimitToCache: 100,
			enableSidecarCaching:              true,
			expectedCache: map[string]*SidecarItem{
				"test-namespace/default": {
					Sidecar: &v1alpha3.Sidecar{
						ObjectMeta: v1.ObjectMeta{
							Name:      "default",
							Namespace: "test-namespace",
						},
						Spec: v1alpha32.Sidecar{
							Egress: []*v1alpha32.IstioEgressListener{
								{
									Hosts: []string{"host1.example.com", "host2.example.com", "host3.example.com"},
								},
							},
						},
					},
					Status: common.ProcessingInProgress,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ap := common.AdmiralParams{
				WorkloadSidecarName:               "default",
				EnableSidecarCaching:              tc.enableSidecarCaching,
				MaxSidecarEgressHostsLimitToCache: tc.maxSidecarEgressHostsLimitToCache,
			}
			common.ResetSync()
			common.InitializeConfig(ap)

			s := NewSidecarCache()
			err := s.Put(tc.sidecar)
			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.Equal(t, err.Error(), tc.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedCache, s.cache)
			}
		})
	}
}

func TestSidecarCacheGet(t *testing.T) {

	ap := common.AdmiralParams{
		WorkloadSidecarName:               "default",
		EnableSidecarCaching:              true,
		MaxSidecarEgressHostsLimitToCache: 100,
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	sidecar := &v1alpha3.Sidecar{
		ObjectMeta: v1.ObjectMeta{
			Name:      "default",
			Namespace: "test-namespace",
		},
		Spec: v1alpha32.Sidecar{
			Egress: []*v1alpha32.IstioEgressListener{
				{
					Hosts: []string{"host1.example.com", "host2.example.com", "host3.example.com"},
				},
			},
		},
	}
	s := NewSidecarCache()
	s.Put(sidecar)
	tests := []struct {
		name            string
		sidecarName     string
		sidecarNS       string
		expectedSidecar *v1alpha3.Sidecar
	}{
		{
			name: "Given a sidecar with no name" +
				"When SidecarCache.Get is called " +
				"Then it should return nil",
			sidecarName:     "",
			sidecarNS:       "test-namespace",
			expectedSidecar: nil,
		},
		{
			name: "Given a sidecar with no namespace" +
				"When SidecarCache.Get is called " +
				"Then it should return nil",
			sidecarName:     "default",
			sidecarNS:       "",
			expectedSidecar: nil,
		},
		{
			name: "Given a sidecar name and ns not in cache" +
				"When SidecarCache.Get is called " +
				"Then it should return nil",
			sidecarName:     "non-default",
			sidecarNS:       "test-namespace",
			expectedSidecar: nil,
		},
		{
			name: "Given a sidecar in the cache " +
				"When SidecarCache.Get is called with its name and namespace " +
				"Then it should return the sidecar",
			sidecarName: "default",
			sidecarNS:   "test-namespace",
			expectedSidecar: &v1alpha3.Sidecar{
				ObjectMeta: v1.ObjectMeta{
					Name:      "default",
					Namespace: "test-namespace",
				},
				Spec: v1alpha32.Sidecar{
					Egress: []*v1alpha32.IstioEgressListener{
						{
							Hosts: []string{"host1.example.com", "host2.example.com", "host3.example.com"},
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := s.Get(tc.sidecarName, tc.sidecarNS)
			assert.Equal(t, tc.expectedSidecar, actual)
		})
	}
}

func TestSidecarCacheDelete(t *testing.T) {
	ap := common.AdmiralParams{
		WorkloadSidecarName:               "default",
		EnableSidecarCaching:              true,
		MaxSidecarEgressHostsLimitToCache: 100,
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	sidecar1 := &v1alpha3.Sidecar{
		ObjectMeta: v1.ObjectMeta{
			Name:      "default",
			Namespace: "test-namespace1",
		},
		Spec: v1alpha32.Sidecar{
			Egress: []*v1alpha32.IstioEgressListener{
				{
					Hosts: []string{"host1.example.com", "host2.example.com", "host3.example.com"},
				},
			},
		},
	}
	sidecar2 := &v1alpha3.Sidecar{
		ObjectMeta: v1.ObjectMeta{
			Name:      "default",
			Namespace: "test-namespace2",
		},
		Spec: v1alpha32.Sidecar{
			Egress: []*v1alpha32.IstioEgressListener{
				{
					Hosts: []string{"host1.example.com", "host2.example.com", "host3.example.com"},
				},
			},
		},
	}
	s := NewSidecarCache()
	s.Put(sidecar1)
	s.Put(sidecar2)

	tests := []struct {
		name             string
		sidecar          *v1alpha3.Sidecar
		expectecCacheLen int
	}{
		{
			name: "Given a nil sidecar" +
				"When SidecarCache.Delete is called " +
				"Then it should not return with noop",
			expectecCacheLen: 2,
		},
		{
			name: "Given a sidecar not in cache" +
				"When SidecarCache.Delete is called with a sidecar " +
				"Then it should not return with noop",
			sidecar:          &v1alpha3.Sidecar{ObjectMeta: v1.ObjectMeta{Name: "non-default", Namespace: "test-namespace"}},
			expectecCacheLen: 2,
		},
		{
			name: "Given a sidecar in cache" +
				"When SidecarCache.Delete is called with a sidecar " +
				"Then it should delete the sidecar from cache and not return an error",
			sidecar:          sidecar2,
			expectecCacheLen: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s.Delete(tc.sidecar)
			assert.Equal(t, tc.expectecCacheLen, len(s.cache))
		})
	}
}

func TestGetSidecarProcessStatus(t *testing.T) {
	ap := common.AdmiralParams{
		WorkloadSidecarName:               "default",
		EnableSidecarCaching:              true,
		MaxSidecarEgressHostsLimitToCache: 100,
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	sidecar := &v1alpha3.Sidecar{
		ObjectMeta: v1.ObjectMeta{
			Name:      "default",
			Namespace: "test-namespace",
		},
		Spec: v1alpha32.Sidecar{
			Egress: []*v1alpha32.IstioEgressListener{
				{
					Hosts: []string{"host1.example.com", "host2.example.com", "host3.example.com"},
				},
			},
		},
	}

	s := NewSidecarCache()
	s.Put(sidecar)

	tests := []struct {
		name           string
		sidecar        *v1alpha3.Sidecar
		expectedStatus string
	}{
		{
			name: "Given a nil sidecar" +
				"When GetSidecarProcessStatus is called, " +
				"Then it should return NotProcessed",
			sidecar:        nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given a sidecar that is not in cache" +
				"When GetSidecarProcessStatus is called, " +
				"Then it should return NotProcessed",
			sidecar:        &v1alpha3.Sidecar{ObjectMeta: v1.ObjectMeta{Name: "non-default", Namespace: "test-namespace"}},
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given a sidecar in cache," +
				"When GetSidecarProcessStatus is called, " +
				"Then it should return ProcessingInProgress",
			sidecar:        sidecar,
			expectedStatus: common.ProcessingInProgress,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := s.GetSidecarProcessStatus(tc.sidecar)
			assert.Equal(t, tc.expectedStatus, status)
		})
	}
}

func TestUpdateSidecarProcessStatus(t *testing.T) {

	ap := common.AdmiralParams{
		WorkloadSidecarName:               "default",
		EnableSidecarCaching:              true,
		MaxSidecarEgressHostsLimitToCache: 100,
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	sidecar := &v1alpha3.Sidecar{
		ObjectMeta: v1.ObjectMeta{
			Name:      "default",
			Namespace: "test-namespace",
		},
		Spec: v1alpha32.Sidecar{
			Egress: []*v1alpha32.IstioEgressListener{
				{
					Hosts: []string{"host1.example.com", "host2.example.com", "host3.example.com"},
				},
			},
		},
	}

	s := NewSidecarCache()
	s.Put(sidecar)

	tests := []struct {
		name          string
		sidecar       *v1alpha3.Sidecar
		status        string
		expectedError error
	}{
		{
			name: "Given a nil sidecar, " +
				"When UpdateSidecarProcessStatus is called, " +
				"Then it should return an error",
			sidecar:       nil,
			status:        common.Processed,
			expectedError: fmt.Errorf("sidecar is nil"),
		},
		{
			name: "Given a sidecar in cache, " +
				"When UpdateSidecarProcessStatus is called, " +
				"Then it should update the status without error",
			sidecar:       sidecar,
			status:        common.Processed,
			expectedError: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := s.UpdateSidecarProcessStatus(tc.sidecar, tc.status)
			if tc.expectedError != nil {
				assert.Equal(t, tc.expectedError, err)
			} else {
				assert.NoError(t, err)
				status := s.GetSidecarProcessStatus(tc.sidecar)
				assert.Equal(t, tc.status, status)
			}
		})
	}
}
