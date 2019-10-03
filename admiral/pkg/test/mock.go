package test

import (
	"github.com/admiral/admiral/pkg/controller/istio"
	k8sAppsV1 "k8s.io/api/apps/v1"
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
	return "", nil
}
func (m *MockIstioConfigStore) Update(config istio.Config) (newRevision string, err error) {

	m.TestHook(config)

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
