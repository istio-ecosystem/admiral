package admiral

import (
	"context"
	"testing"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	k8sV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestNewNodeController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockNodeHandler{}

	nodeController, err := NewNodeController("", stop, &handler, config)

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

	nodeController, err := NewNodeController("", stop, &handler, config)

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
