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
	sidecarController := SidecarController{}
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
			err := sidecarController.UpdateProcessItemStatus(tc.obj, common.NotProcessed)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}
