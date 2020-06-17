package admiral

import (
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestDependencyAddUpdateAndDelete(t *testing.T) {
	stop := make(chan struct{})
	handler := test.MockDependencyHandler{}

	dependencyController, err := NewDependencyController(stop, &handler, "../../test/resources/admins@fake-cluster.k8s.local", "ns", time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if dependencyController == nil {
		t.Errorf("Dependency controller should never be nil without an error thrown")
	}

	//test add
	depName := "dep1"
	dep := model.Dependency{IdentityLabel: "identity", Destinations: []string{"greeting", "payments"}, Source: "webapp"}
	depObj := makeK8sDependencyObj(depName, "namespace1", dep)
	dependencyController.Added(depObj)

	newDepObj := dependencyController.Cache.Get(depName)

	if !cmp.Equal(depObj.Spec, newDepObj.Spec) {
		t.Errorf("dep update failed, expected: %v got %v", depObj, newDepObj)
	}

	//test update
	updatedDep := model.Dependency{IdentityLabel: "identity", Destinations: []string{"greeting", "payments", "newservice"}, Source: "webapp"}
	updatedObj := makeK8sDependencyObj(depName, "namespace1", updatedDep)
	dependencyController.Updated(makeK8sDependencyObj(depName, "namespace1", updatedDep), depObj)

	updatedDepObj := dependencyController.Cache.Get(depName)

	if !cmp.Equal(updatedObj.Spec, updatedDepObj.Spec) {
		t.Errorf("dep update failed, expected: %v got %v", updatedObj, updatedDepObj)
	}

	//test delete
	dependencyController.Deleted(updatedDepObj)

	deletedDepObj := dependencyController.Cache.Get(depName)

	if deletedDepObj != nil {
		t.Errorf("dep delete failed")
	}

}

func makeK8sDependencyObj(name string, namespace string, dependency model.Dependency) *v1.Dependency {
	return &v1.Dependency{Spec: dependency, ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace}}
}
