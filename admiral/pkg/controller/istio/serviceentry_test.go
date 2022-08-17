package istio

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"google.golang.org/protobuf/testing/protocmp"
	"istio.io/api/networking/v1alpha3"
	v1alpha32 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestNewServiceEntryController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockServiceEntryHandler{}

	serviceEntryController, err := NewServiceEntryController("test", stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if serviceEntryController == nil {
		t.Errorf("ServiceEntry controller should never be nil without an error thrown")
	}

	serviceEntry := &v1alpha32.ServiceEntry{Spec: v1alpha3.ServiceEntry{}, ObjectMeta: v1.ObjectMeta{Name: "se1", Namespace: "namespace1"}}

	ctx := context.Background()

	serviceEntryController.Added(ctx, serviceEntry)

	if !cmp.Equal(&serviceEntry.Spec, &handler.Obj.Spec, protocmp.Transform()) {
		t.Errorf("Handler should have the added obj")
	}

	updatedServiceEntry := &v1alpha32.ServiceEntry{Spec: v1alpha3.ServiceEntry{Hosts: []string{"hello.global"}}, ObjectMeta: v1.ObjectMeta{Name: "se1", Namespace: "namespace1"}}
	serviceEntryController.Updated(ctx, updatedServiceEntry, serviceEntry)

	if !cmp.Equal(&updatedServiceEntry.Spec, &handler.Obj.Spec, protocmp.Transform()) {
		t.Errorf("Handler should have the updated obj")
	}

	serviceEntryController.Deleted(ctx, serviceEntry)

	if handler.Obj != nil {
		t.Errorf("Handler should have no obj")
	}
}
