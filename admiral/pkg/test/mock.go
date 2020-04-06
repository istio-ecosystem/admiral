package test

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	v1alpha32 "istio.io/client-go/pkg/apis/networking/v1alpha3"
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

type MockPodHandler struct {
}

func (m MockPodHandler) Added(obj *k8sCoreV1.Pod) {

}

func (m MockPodHandler) Deleted(obj *k8sCoreV1.Pod) {

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

func (m *MockDependencyHandler) Updated(obj *v1.Dependency) {

}

func (m *MockDependencyHandler) Deleted(obj *v1.Dependency) {

}

type MockGlobalTrafficHandler struct {
}

func (m *MockGlobalTrafficHandler) Added(obj *v1.GlobalTrafficPolicy) {

}

func (m *MockGlobalTrafficHandler) Updated(obj *v1.GlobalTrafficPolicy) {

}

func (m *MockGlobalTrafficHandler) Deleted(obj *v1.GlobalTrafficPolicy) {

}

type MockServiceEntryHandler struct {
}

func (m *MockServiceEntryHandler) Added(obj *v1alpha32.ServiceEntry) {

}

func (m *MockServiceEntryHandler) Updated(obj *v1alpha32.ServiceEntry) {

}

func (m *MockServiceEntryHandler) Deleted(obj *v1alpha32.ServiceEntry) {

}

type MockVirtualServiceHandler struct {
}

func (m *MockVirtualServiceHandler) Added(obj *v1alpha32.VirtualService) {

}

func (m *MockVirtualServiceHandler) Updated(obj *v1alpha32.VirtualService) {

}

func (m *MockVirtualServiceHandler) Deleted(obj *v1alpha32.VirtualService) {

}

type MockDestinationRuleHandler struct {
}

func (m *MockDestinationRuleHandler) Added(obj *v1alpha32.DestinationRule) {

}

func (m *MockDestinationRuleHandler) Updated(obj *v1alpha32.DestinationRule) {

}

func (m *MockDestinationRuleHandler) Deleted(obj *v1alpha32.DestinationRule) {

}

type MockSidecarHandler struct {
}

func (m *MockSidecarHandler) Added(obj *v1alpha32.Sidecar) {

}

func (m *MockSidecarHandler) Updated(obj *v1alpha32.Sidecar) {

}

func (m *MockSidecarHandler) Deleted(obj *v1alpha32.Sidecar) {

}
