package admiral

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"testing"

	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
)

func TestEnvoyFilterAdded(t *testing.T) {

	mockEnvoyFilterHandler := &test.MockEnvoyFilterHandler{}
	ctx := context.Background()
	envoyFilterController := EnvoyFilterController{
		EnvoyFilterHandler: mockEnvoyFilterHandler,
	}

	testCases := []struct {
		name          string
		envoyFilter   interface{}
		expectedError error
	}{
		{
			name: "Given context and EnvoyFilter " +
				"When EnvoyFilter param is nil " +
				"Then func should return an error",
			envoyFilter:   nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.EnvoyFilter"),
		},
		{
			name: "Given context and EnvoyFilter " +
				"When EnvoyFilter param is not of type *v1alpha3.EnvoyFilter " +
				"Then func should return an error",
			envoyFilter:   struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.EnvoyFilter"),
		},
		{
			name: "Given context and EnvoyFilter " +
				"When EnvoyFilter param is of type *v1alpha3.EnvoyFilter " +
				"Then func should not return an error",
			envoyFilter:   &v1alpha3.EnvoyFilter{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := envoyFilterController.Added(ctx, tc.envoyFilter)
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

func TestEnvoyFilterUpdated(t *testing.T) {

	mockEnvoyFilterHandler := &test.MockEnvoyFilterHandler{}
	ctx := context.Background()
	envoyFilterController := EnvoyFilterController{
		EnvoyFilterHandler: mockEnvoyFilterHandler,
	}

	testCases := []struct {
		name          string
		envoyFilter   interface{}
		expectedError error
	}{
		{
			name: "Given context and EnvoyFilter " +
				"When EnvoyFilter param is nil " +
				"Then func should return an error",
			envoyFilter:   nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.EnvoyFilter"),
		},
		{
			name: "Given context and EnvoyFilter " +
				"When EnvoyFilter param is not of type *v1alpha3.EnvoyFilter " +
				"Then func should return an error",
			envoyFilter:   struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.EnvoyFilter"),
		},
		{
			name: "Given context and EnvoyFilter " +
				"When EnvoyFilter param is of type *v1alpha3.EnvoyFilter " +
				"Then func should not return an error",
			envoyFilter:   &v1alpha3.EnvoyFilter{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := envoyFilterController.Updated(ctx, tc.envoyFilter, nil)
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

func TestEnvoyFilterDeleted(t *testing.T) {

	mockEnvoyFilterHandler := &test.MockEnvoyFilterHandler{}
	ctx := context.Background()
	envoyFilterController := EnvoyFilterController{
		EnvoyFilterHandler: mockEnvoyFilterHandler,
	}

	testCases := []struct {
		name          string
		envoyFilter   interface{}
		expectedError error
	}{
		{
			name: "Given context and EnvoyFilter " +
				"When EnvoyFilter param is nil " +
				"Then func should return an error",
			envoyFilter:   nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1alpha3.EnvoyFilter"),
		},
		{
			name: "Given context and EnvoyFilter " +
				"When EnvoyFilter param is not of type *v1alpha3.EnvoyFilter " +
				"Then func should return an error",
			envoyFilter:   struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1alpha3.EnvoyFilter"),
		},
		{
			name: "Given context and EnvoyFilter " +
				"When EnvoyFilter param is of type *v1alpha3.EnvoyFilter " +
				"Then func should not return an error",
			envoyFilter:   &v1alpha3.EnvoyFilter{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := envoyFilterController.Deleted(ctx, tc.envoyFilter)
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

// TODO: This is just a placeholder for when we add diff check for other types
func TestEnvoyFilterGetProcessItemStatus(t *testing.T) {
	envoyFilterController := EnvoyFilterController{}
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
			res, _ := envoyFilterController.GetProcessItemStatus(tc.obj)
			assert.Equal(t, tc.expectedRes, res)
		})
	}
}

// TODO: This is just a placeholder for when we add diff check for other types
func TestEnvoyFilterUpdateProcessItemStatus(t *testing.T) {
	envoyFilterController := EnvoyFilterController{}
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
			err := envoyFilterController.UpdateProcessItemStatus(tc.obj, common.NotProcessed)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}
