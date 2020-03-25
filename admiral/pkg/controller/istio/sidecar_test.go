package istio

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
	"time"
)

func TestNewSidecarController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockSidecarHandler{}

	sidecarController, err := NewSidecarController(stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if sidecarController == nil {
		t.Errorf("Sidecar controller should never be nil without an error thrown")
	}
}
