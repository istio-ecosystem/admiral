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

func TestGetDependencies(t *testing.T) {
	stop := make(chan struct{})
	handler := test.MockDependencyHandler{}

	dependencyController, err := NewDependencyController(stop, &handler, "../../test/resources/admins@fake-cluster.k8s.local", "ns", time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if dependencyController == nil {
		t.Errorf("Dependency controller should never be nil without an error thrown")
	}

	deps, _  := dependencyController.GetDependencies()

	if deps != nil {
		t.Errorf("deps is not nil")
	}

	dep := model.Dependency{IdentityLabel: "identity", Destinations:[]string{"greeting", "payments", "newservice"}, Source: "webapp"}
	depObj := makeK8sDependencyObj("mydep", "namespace1", dep)

	dependencyController.DepCrdClient.AdmiralV1().Dependencies("namespace1").Create(depObj);

	deps, _  = dependencyController.GetDependencies()

	if deps == nil || len(deps) == 0{
		t.Errorf("deps should be non empty")
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
	dep := model.Dependency{IdentityLabel: "identity", Destinations:[]string{"greeting", "payments"}, Source: "webapp"}
	depObj := makeK8sDependencyObj(depName, "namespace1", dep)
	dependencyController.Added(depObj)

	newDepObj := dependencyController.Cache.Get(depName)

	if !cmp.Equal(depObj, newDepObj) {
		t.Errorf("dep update failed, expected: %v got %v", depObj, newDepObj)
	}

	//test update
	updatedDep := model.Dependency{IdentityLabel: "identity", Destinations:[]string{"greeting", "payments", "newservice"}, Source: "webapp"}
	updatedObj := makeK8sDependencyObj(depName, "namespace1", updatedDep)
	dependencyController.Updated(makeK8sDependencyObj(depName, "namespace1", updatedDep), depObj)

	updatedDepObj := dependencyController.Cache.Get(depName)

	if !cmp.Equal(updatedObj, updatedDepObj) {
		t.Errorf("dep update failed, expected: %v got %v", updatedObj, updatedDepObj)
	}

	//test delete
	dependencyController.Deleted(updatedDepObj)

	deletedDepObj := dependencyController.Cache.Get(depName)

	if deletedDepObj != nil {
		t.Errorf("dep delete failed")
	}

}

func makeK8sDependencyObj (name string, namespace string, dependency model.Dependency) *v1.Dependency {
	return &v1.Dependency{Spec: dependency, ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace}}
}
