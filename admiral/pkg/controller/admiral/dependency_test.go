package admiral

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"testing"
	"time"
)

func TestNewDependencyController(t *testing.T) {
	stop := make(chan struct{})
	handler := test.MockDependencyHandler{}

	dependencyController, err := NewDependencyController(stop, &handler, "../../test/resources/admins@fake-cluster.k8s.local", "ns", time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if dependencyController == nil {
		t.Errorf("Dependency controller should never be nil without an error thrown")
	}
}
