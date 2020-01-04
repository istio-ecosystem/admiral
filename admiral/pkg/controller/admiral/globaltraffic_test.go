package admiral

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
	"time"
)

func TestNewGlobalTrafficController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockGlobalTrafficHandler{}

	globalTrafficController, err := NewGlobalTrafficController(stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if globalTrafficController == nil {
		t.Errorf("GlobalTraffic controller should never be nil without an error thrown")
	}
}
