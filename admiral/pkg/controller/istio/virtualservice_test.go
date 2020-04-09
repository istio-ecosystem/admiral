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

	vs := &v1alpha3.VirtualService{Spec: v1alpha32.VirtualService{}, ObjectMeta: v1.ObjectMeta{Name: "vs1", Namespace: "namespace1"}}

	virtualServiceController.Added(vs)

	virtualServiceController.Updated(vs, vs)

	virtualServiceController.Deleted(vs)
}