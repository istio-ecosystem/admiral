package istio

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
	"time"
)

func TestNewServiceEntryController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockServiceEntryHandler{}

	serviceEntryController, err := NewServiceEntryController(stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if serviceEntryController == nil {
		t.Errorf("ServiceEntry controller should never be nil without an error thrown")
	}
}