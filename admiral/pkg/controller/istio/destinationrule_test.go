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

	dstRule := &v1alpha3.DestinationRule{Spec: v1alpha32.DestinationRule{}, ObjectMeta: v1.ObjectMeta{Name: "dr1", Namespace: "namespace1"}}

	destinationRuleController.Added(dstRule)

	if !cmp.Equal(dstRule.Spec, handler.Obj.Spec) {
		t.Errorf("Handler should have the added obj")
	}

	updatedDstRule := &v1alpha3.DestinationRule{Spec: v1alpha32.DestinationRule{Host: "hello.global"}, ObjectMeta: v1.ObjectMeta{Name: "dr1", Namespace: "namespace1"}}
	destinationRuleController.Updated(updatedDstRule, dstRule)

	if !cmp.Equal(updatedDstRule.Spec, handler.Obj.Spec) {
		t.Errorf("Handler should have the updated obj")
	}

	destinationRuleController.Deleted(dstRule)

	if handler.Obj != nil {
		t.Errorf("Handler should have no obj")
	}
}
