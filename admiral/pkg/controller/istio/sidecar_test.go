package istio

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	v1alpha32 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	sc := &v1alpha3.Sidecar{Spec: v1alpha32.Sidecar{}, ObjectMeta: v1.ObjectMeta{Name: "sc1", Namespace: "namespace1"}}

	sidecarController.Added(sc)

	sidecarController.Updated(sc, sc)

	sidecarController.Deleted(sc)
}
