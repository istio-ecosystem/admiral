package istio

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
	"time"
)

func TestNewVirtualServiceController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockVirtualServiceHandler{}

	virtualServiceController, err := NewVirtualServiceController(stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if virtualServiceController == nil {
		t.Errorf("VirtualService controller should never be nil without an error thrown")
	}
}