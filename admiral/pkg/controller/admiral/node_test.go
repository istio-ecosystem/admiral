package admiral

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
)

func TestNewNodeController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockNodeHandler{}

	nodeController, err := NewNodeController(stop, &handler, config)

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if nodeController == nil {
		t.Errorf("Node controller should never be nil without an error thrown")
	}
}
