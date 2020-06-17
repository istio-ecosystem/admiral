package istio

import (
	"github.com/google/go-cmp/cmp"
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

	if !cmp.Equal(sc.Spec, handler.Obj.Spec) {
		t.Errorf("Handler should have the added obj")
	}

	updatedSc := &v1alpha3.Sidecar{Spec: v1alpha32.Sidecar{WorkloadSelector: &v1alpha32.WorkloadSelector{Labels: map[string]string{"this": "that"}}}, ObjectMeta: v1.ObjectMeta{Name: "sc1", Namespace: "namespace1"}}
	sidecarController.Updated(updatedSc, sc)

	if !cmp.Equal(updatedSc.Spec, handler.Obj.Spec) {
		t.Errorf("Handler should have the updated obj")
	}

	sidecarController.Deleted(sc)

	if handler.Obj != nil {
		t.Errorf("Handler should have no obj")
	}
}
