package istio

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
	"time"
)

func TestNewDestinationRuleController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockDestinationRuleHandler{}

	destinationRuleController, err := NewDestinationRuleController(stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if destinationRuleController == nil {
		t.Errorf("DestinationRule controller should never be nil without an error thrown")
	}
}