package test

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sCoreV1 "k8s.io/api/core/v1"
)

type MockIstioConfigStore struct {
	TestHook func(interface{})
}

func (m *MockIstioConfigStore) RegisterEventHandler(typ string, handler func(istio.Config, istio.Event)) {

}
func (m *MockIstioConfigStore) HasSynced() bool {

	return false
}
func (m *MockIstioConfigStore) Run(stop <-chan struct{}) {

}
func (m *MockIstioConfigStore) ConfigDescriptor() istio.ConfigDescriptor {
	return nil
}
func (m *MockIstioConfigStore) Get(typ, name, namespace string) *istio.Config {
	return nil
}

func (m *MockIstioConfigStore) List(typ, namespace string) ([]istio.Config, error) {
	return nil, nil
}

func (m *MockIstioConfigStore) Create(config istio.Config) (revision string, err error) {

	if m.TestHook != nil {
		m.TestHook(config)
	}
	return "", nil
}
func (m *MockIstioConfigStore) Update(config istio.Config) (newRevision string, err error) {

	if m.TestHook != nil {
		m.TestHook(config)
	}
	return "", nil
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

type MockPodHandler struct {
}

func (m MockPodHandler) Added (obj *k8sCoreV1.Pod) {

}

func (m MockPodHandler) Deleted (obj *k8sCoreV1.Pod) {

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

type MockGlobalTrafficHandler struct {
}

func (m *MockGlobalTrafficHandler) Added(obj *v1.GlobalTrafficPolicy) {

}

func (m *MockGlobalTrafficHandler) Deleted(obj *v1.GlobalTrafficPolicy) {

}
