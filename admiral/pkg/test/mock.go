package test

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sCoreV1 "k8s.io/api/core/v1"
)

type MockIstioConfigStore struct {
	TestHook func(interface{})
}

func (m *MockIstioConfigStore) HasSynced() bool {

	return false
}
func (m *MockIstioConfigStore) Run(stop <-chan struct{}) {

}

func (m *MockIstioConfigStore) Delete(typ, name, namespace string) error {

	return nil
}

type MockDeploymentHandler struct {
}

func (m *MockDeploymentHandler) Added(obj *k8sAppsV1.Deployment) {

}

func (m *MockDeploymentHandler) Deleted(obj *k8sAppsV1.Deployment) {

}

type MockServiceHandler struct {
}

func (m *MockServiceHandler) Added(obj *k8sCoreV1.Service) {

}

func (m *MockServiceHandler) Deleted(obj *k8sCoreV1.Service) {

}

type MockNodeHandler struct {
}

func (m *MockNodeHandler) Added(obj *k8sCoreV1.Node) {

}

func (m *MockNodeHandler) Deleted(obj *k8sCoreV1.Node) {

}

type MockDependencyHandler struct {
}

func (m *MockDependencyHandler) Added(obj *v1.Dependency) {

}

func (m *MockDependencyHandler) Deleted(obj *v1.Dependency) {

}
