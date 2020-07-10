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

func TestGlobalTrafficAddUpdateDelete(t *testing.T) {
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

	gtpName := "gtp1"
	gtp := model.GlobalTrafficPolicy{Selector: map[string]string{"identity": "payments", "env": "e2e"}, Policy: []*model.TrafficPolicy{}}
	gtpObj := makeK8sGtpObj(gtpName, "namespace1", gtp)
	globalTrafficController.Added(gtpObj)

	if !cmp.Equal(handler.Obj.Spec, gtpObj.Spec) {
		t.Errorf("Add should call the handler with the object")
	}

	updatedGtp := model.GlobalTrafficPolicy{Selector: map[string]string{"identity": "payments", "env": "qa"}, Policy: []*model.TrafficPolicy{}}
	updatedGtpObj := makeK8sGtpObj(gtpName, "namespace1", updatedGtp)

	globalTrafficController.Updated(updatedGtpObj, gtpObj)

	if !cmp.Equal(handler.Obj.Spec, updatedGtpObj.Spec) {
		t.Errorf("Update should call the handler with the updated object")
	}

	globalTrafficController.Deleted(updatedGtpObj)

	if handler.Obj != nil {
		t.Errorf("Delete should delete the gtp")
	}

}

func TestGlobalTrafficGetByLabel(t *testing.T) {
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

	gtps := globalTrafficController.GetGTPByLabel("payments", "namespace1")

	if gtps != nil || len(gtps) > 0 {
		t.Errorf("gtps is not empty")
	}
}

func makeK8sGtpObj(name string, namespace string, gtp model.GlobalTrafficPolicy) *v1.GlobalTrafficPolicy {
	return &v1.GlobalTrafficPolicy{
		Spec:       gtp,
		ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace},
		TypeMeta: v12.TypeMeta{
			APIVersion: "admiral.io/v1",
			Kind:       "GlobalTrafficPolicy",
		}}
}
