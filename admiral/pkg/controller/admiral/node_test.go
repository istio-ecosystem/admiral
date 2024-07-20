package admiral

import (
	"context"
	"fmt"
	"testing"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	k8sV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestNodeAddedTypeAssertion(t *testing.T) {

	ctx := context.Background()
	nodeController := NodeController{
		Locality: &Locality{Region: "us-west-2"},
	}

	testCases := []struct {
		name          string
		node          interface{}
		expectedError error
	}{
		{
			name: "Given context and Node " +
				"When Node param is nil " +
				"Then func should return an error",
			node:          nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.Node"),
		},
		{
			name: "Given context and Node " +
				"When Node param is not of type *v1.Node " +
				"Then func should return an error",
			node:          struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.Node"),
		},
		{
			name: "Given context and Node " +
				"When Node param is of type *v1.Node " +
				"Then func should not return an error",
			node:          &k8sV1.Node{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := nodeController.Added(ctx, tc.node)
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

func TestNewNodeController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockNodeHandler{}

	nodeController, err := NewNodeController(stop, &handler, config, loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if nodeController == nil {
		t.Errorf("Node controller should never be nil without an error thrown")
	}
}

func TestNodeAddUpdateDelete(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockNodeHandler{}

	nodeController, err := NewNodeController(stop, &handler, config, loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if nodeController == nil {
		t.Errorf("Node controller should never be nil without an error thrown")
	}
	region := "us-west-2"
	nodeObj := &k8sV1.Node{Spec: k8sV1.NodeSpec{}, ObjectMeta: v1.ObjectMeta{Labels: map[string]string{common.NodeRegionLabel: region}}}

	ctx := context.Background()

	nodeController.Added(ctx, nodeObj)

	locality := nodeController.Locality

	if locality.Region != region {
		t.Errorf("region expected %v, got: %v", region, locality.Region)
	}

	nodeController.Updated(ctx, nodeObj, nodeObj)
	//update should make no difference
	if locality.Region != region {
		t.Errorf("region expected %v, got: %v", region, locality.Region)
	}

	nodeController.Deleted(ctx, nodeObj)
	//delete should make no difference
	if locality.Region != region {
		t.Errorf("region expected %v, got: %v", region, locality.Region)
	}
}

// TODO: This is just a placeholder for when we add diff check for other types
func TestNodeGetProcessItemStatus(t *testing.T) {
	nodeController := NodeController{}
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
			res, _ := nodeController.GetProcessItemStatus(tc.obj)
			assert.Equal(t, tc.expectedRes, res)
		})
	}
}

// TODO: This is just a placeholder for when we add diff check for other types
func TestNodeUpdateProcessItemStatus(t *testing.T) {
	nodeController := NodeController{}
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
			err := nodeController.UpdateProcessItemStatus(tc.obj, common.NotProcessed)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}
