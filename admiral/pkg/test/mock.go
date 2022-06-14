package test

import (
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
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

type MockRolloutHandler struct {
}

func (m *MockRolloutHandler) Added(obj *argo.Rollout) {

}

func (m *MockRolloutHandler) Deleted(obj *argo.Rollout) {

}

func (m *MockRolloutHandler) Updated(obj *argo.Rollout) {

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
	Obj *k8sCoreV1.Node
}

func (m *MockNodeHandler) Added(obj *k8sCoreV1.Node) {
	m.Obj = obj
}

func (m *MockNodeHandler) Deleted(obj *k8sCoreV1.Node) {
	m.Obj = nil
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
	Obj *v1.GlobalTrafficPolicy
}

func (m *MockGlobalTrafficHandler) Added(obj *v1.GlobalTrafficPolicy) {
	m.Obj = obj
}

func (m *MockGlobalTrafficHandler) Updated(obj *v1.GlobalTrafficPolicy) {
	m.Obj = obj
}

func (m *MockGlobalTrafficHandler) Deleted(obj *v1.GlobalTrafficPolicy) {
	m.Obj = nil
}

type MockServiceEntryHandler struct {
	Obj *v1alpha32.ServiceEntry
}

func (m *MockServiceEntryHandler) Added(obj *v1alpha32.ServiceEntry) {
	m.Obj = obj
}

func (m *MockServiceEntryHandler) Updated(obj *v1alpha32.ServiceEntry) {
	m.Obj = obj
}

func (m *MockServiceEntryHandler) Deleted(obj *v1alpha32.ServiceEntry) {
	m.Obj = nil
}

type MockVirtualServiceHandler struct {
	Obj *v1alpha32.VirtualService
}

func (m *MockVirtualServiceHandler) Added(obj *v1alpha32.VirtualService) {
	m.Obj = obj
}

func (m *MockVirtualServiceHandler) Updated(obj *v1alpha32.VirtualService) {
	m.Obj = obj
}

func (m *MockVirtualServiceHandler) Deleted(obj *v1alpha32.VirtualService) {
	m.Obj = nil
}

type MockDestinationRuleHandler struct {
	Obj *v1alpha32.DestinationRule
}

func (m *MockDestinationRuleHandler) Added(obj *v1alpha32.DestinationRule) {
	m.Obj = obj
}

func (m *MockDestinationRuleHandler) Updated(obj *v1alpha32.DestinationRule) {
	m.Obj = obj
}

func (m *MockDestinationRuleHandler) Deleted(obj *v1alpha32.DestinationRule) {
	m.Obj = nil
}

type MockSidecarHandler struct {
	Obj *v1alpha32.Sidecar
}

func (m *MockSidecarHandler) Added(obj *v1alpha32.Sidecar) {
	m.Obj = obj
}

func (m *MockSidecarHandler) Updated(obj *v1alpha32.Sidecar) {
	m.Obj = obj
}

func (m *MockSidecarHandler) Deleted(obj *v1alpha32.Sidecar) {
	m.Obj = nil
}

type MockRoutingPolicyHandler struct {
	Obj *v1.RoutingPolicy
}

func (m *MockRoutingPolicyHandler) Added(obj *v1.RoutingPolicy) {
	m.Obj = obj
}

func (m *MockRoutingPolicyHandler) Deleted(obj *v1.RoutingPolicy) {
	m.Obj = nil
}

func (m *MockRoutingPolicyHandler) Updated(obj *v1.RoutingPolicy) {
	m.Obj = obj
}