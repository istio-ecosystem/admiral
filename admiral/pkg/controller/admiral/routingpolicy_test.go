package admiral

import (
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
	"time"
)


func TestNewroutingPolicyController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockRoutingPolicyHandler{}

	routingPolicyController, err := NewRoutingPoliciesController(stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if routingPolicyController == nil {
		t.Errorf("RoutingPolicy controller should never be nil without an error thrown")
	}
}


func TestRoutingPolicyAddUpdateDelete(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockRoutingPolicyHandler{}
	routingPolicyController, err := NewRoutingPoliciesController(stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if routingPolicyController == nil {
		t.Errorf("RoutingPolicy controller should never be nil without an error thrown")
	}

	rpName := "greetingRoutingPolicy"
	rp := model.RoutingPolicy{
		Config: map[string]string{"cacheTTL": "86400", "dispatcherUrl": "stage.greeting.router.mesh", "pathPrefix": "/hello,/hello/v2/"},
		Plugin: "greeting",
		Hosts: []string{"stage.greeting.mesh"},
	}

	rpObj := makeK8sRoutingPolicyObj(rpName, "namespace1", rp)
	routingPolicyController.Added(rpObj)

	if !cmp.Equal(handler.Obj.Spec, rpObj.Spec) {
		t.Errorf("Add should call the handler with the object")
	}

	updatedrp := model.RoutingPolicy{
		Config: map[string]string{"cacheTTL": "86400", "dispatcherUrl": "e2e.greeting.router.mesh", "pathPrefix": "/hello,/hello/v2/"},
		Plugin: "greeting",
		Hosts: []string{"e2e.greeting.mesh"},
	}

	updatedRpObj := makeK8sRoutingPolicyObj(rpName, "namespace1", updatedrp)

	routingPolicyController.Updated(updatedRpObj, rpObj)

	if !cmp.Equal(handler.Obj.Spec, updatedRpObj.Spec) {
		t.Errorf("Update should call the handler with the updated object")
	}

	routingPolicyController.Deleted(updatedRpObj)

	if handler.Obj != nil {
		t.Errorf("Delete should delete the routing policy")
	}

}


func makeK8sRoutingPolicyObj(name string, namespace string, rp model.RoutingPolicy) *v1.RoutingPolicy {
	return &v1.RoutingPolicy{
		Spec:       rp,
		ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace},
		TypeMeta: v12.TypeMeta{
			APIVersion: "admiral.io/v1",
			Kind:       "RoutingPolicy",
		}}
}
