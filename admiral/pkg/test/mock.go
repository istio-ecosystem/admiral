package test

import (
	"context"
	"errors"
	admiralapiv1 "github.com/istio-ecosystem/admiral-api/pkg/apis/admiral/v1"

	argoprojv1alpha1 "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/typed/rollouts/v1alpha1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	v1alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	admiralV1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	v1alpha32 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sCoreV1 "k8s.io/api/core/v1"
)

var (
	RolloutNamespace = "test-ns"
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
	Obj *k8sAppsV1.Deployment
}

func (m *MockDeploymentHandler) Added(ctx context.Context, obj *k8sAppsV1.Deployment) error {
	m.Obj = obj
	return nil
}

func (m *MockDeploymentHandler) Deleted(ctx context.Context, obj *k8sAppsV1.Deployment) error {
	return nil
}

type MockDeploymentHandlerError struct {
}

func (m *MockDeploymentHandlerError) Added(ctx context.Context, obj *k8sAppsV1.Deployment) error {
	return nil
}

func (m *MockDeploymentHandlerError) Deleted(ctx context.Context, obj *k8sAppsV1.Deployment) error {
	return errors.New("error while deleting deployment")
}

type MockRolloutHandler struct {
	Obj *argo.Rollout
}

func (m *MockRolloutHandler) Added(ctx context.Context, obj *argo.Rollout) error {
	m.Obj = obj
	return nil
}

func (m *MockRolloutHandler) Deleted(ctx context.Context, obj *argo.Rollout) error {
	return nil
}

func (m *MockRolloutHandler) Updated(ctx context.Context, obj *argo.Rollout) error {
	return nil
}

type MockRolloutHandlerError struct {
	Obj *argo.Rollout
}

func (m *MockRolloutHandlerError) Added(ctx context.Context, obj *argo.Rollout) error {
	m.Obj = obj
	return nil
}

func (m *MockRolloutHandlerError) Deleted(ctx context.Context, obj *argo.Rollout) error {
	return errors.New("error while deleting rollout")
}

func (m *MockRolloutHandlerError) Updated(ctx context.Context, obj *argo.Rollout) error {
	return nil
}

type MockServiceHandler struct {
}

func (m *MockServiceHandler) Added(ctx context.Context, obj *k8sCoreV1.Service) error {
	return nil
}

func (m *MockServiceHandler) Updated(ctx context.Context, obj *k8sCoreV1.Service) error {
	return nil
}

func (m *MockServiceHandler) Deleted(ctx context.Context, obj *k8sCoreV1.Service) error {
	return nil
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

func (m *MockDependencyHandler) Added(ctx context.Context, obj *admiralV1.Dependency) error {
	return nil
}

func (m *MockDependencyHandler) Updated(ctx context.Context, obj *admiralV1.Dependency) error {
	return nil
}

func (m *MockDependencyHandler) Deleted(ctx context.Context, obj *admiralV1.Dependency) error {
	return nil
}

type MockGlobalTrafficHandler struct {
	Obj *admiralV1.GlobalTrafficPolicy
}

func (m *MockGlobalTrafficHandler) Added(ctx context.Context, obj *admiralV1.GlobalTrafficPolicy) error {
	m.Obj = obj
	return nil
}

func (m *MockGlobalTrafficHandler) Updated(ctx context.Context, obj *admiralV1.GlobalTrafficPolicy) error {
	m.Obj = obj
	return nil
}

func (m *MockGlobalTrafficHandler) Deleted(ctx context.Context, obj *admiralV1.GlobalTrafficPolicy) error {
	m.Obj = nil
	return nil
}

type MockServiceEntryHandler struct {
	Obj *v1alpha32.ServiceEntry
}

func (m *MockServiceEntryHandler) Added(obj *v1alpha32.ServiceEntry) error {
	m.Obj = obj
	return nil
}

func (m *MockServiceEntryHandler) Updated(obj *v1alpha32.ServiceEntry) error {
	m.Obj = obj
	return nil
}

func (m *MockServiceEntryHandler) Deleted(obj *v1alpha32.ServiceEntry) error {
	m.Obj = nil
	return nil
}

type MockVirtualServiceHandler struct {
	Obj *v1alpha32.VirtualService
}

func (m *MockVirtualServiceHandler) Added(ctx context.Context, obj *v1alpha32.VirtualService) error {
	m.Obj = obj
	return nil
}

func (m *MockVirtualServiceHandler) Updated(ctx context.Context, obj *v1alpha32.VirtualService) error {
	m.Obj = obj
	return nil
}

func (m *MockVirtualServiceHandler) Deleted(ctx context.Context, obj *v1alpha32.VirtualService) error {
	m.Obj = nil
	return nil
}

type MockDestinationRuleHandler struct {
	Obj *v1alpha32.DestinationRule
}

func (m *MockDestinationRuleHandler) Added(ctx context.Context, obj *v1alpha32.DestinationRule) error {
	m.Obj = obj
	return nil
}

func (m *MockDestinationRuleHandler) Updated(ctx context.Context, obj *v1alpha32.DestinationRule) error {
	m.Obj = obj
	return nil
}

func (m *MockDestinationRuleHandler) Deleted(ctx context.Context, obj *v1alpha32.DestinationRule) error {
	m.Obj = nil
	return nil
}

type MockSidecarHandler struct {
	Obj *v1alpha32.Sidecar
}

func (m *MockSidecarHandler) Added(ctx context.Context, obj *v1alpha32.Sidecar) error {
	m.Obj = obj
	return nil
}

func (m *MockSidecarHandler) Updated(ctx context.Context, obj *v1alpha32.Sidecar) error {
	m.Obj = obj
	return nil
}

func (m *MockSidecarHandler) Deleted(ctx context.Context, obj *v1alpha32.Sidecar) error {
	m.Obj = nil
	return nil
}

type MockRoutingPolicyHandler struct {
	Obj *admiralV1.RoutingPolicy
}

func (m *MockRoutingPolicyHandler) Added(ctx context.Context, obj *admiralV1.RoutingPolicy) error {
	m.Obj = obj
	return nil
}

func (m *MockRoutingPolicyHandler) Deleted(ctx context.Context, obj *admiralV1.RoutingPolicy) error {
	m.Obj = nil
	return nil
}

func (m *MockRoutingPolicyHandler) Updated(ctx context.Context, obj *admiralV1.RoutingPolicy) error {
	m.Obj = obj
	return nil
}

type MockTrafficConfigHandler struct {
	Obj *admiralV1.TrafficConfig
}

func (m *MockTrafficConfigHandler) Added(ctx context.Context, obj *admiralV1.TrafficConfig) {
	m.Obj = obj
}

func (m *MockTrafficConfigHandler) Deleted(ctx context.Context, obj *admiralV1.TrafficConfig) {
	m.Obj = nil
}

func (m *MockTrafficConfigHandler) Updated(ctx context.Context, obj *admiralV1.TrafficConfig) {
	m.Obj = obj
}

type MockEnvoyFilterHandler struct {
}

func (m *MockEnvoyFilterHandler) Added(context.Context, *v1alpha32.EnvoyFilter) {
}

func (m *MockEnvoyFilterHandler) Deleted(context.Context, *v1alpha32.EnvoyFilter) {
}

func (m *MockEnvoyFilterHandler) Updated(context.Context, *v1alpha32.EnvoyFilter) {
}

type MockRolloutsGetter struct{}
type FakeRolloutsImpl struct{}

func (f FakeRolloutsImpl) Create(ctx context.Context, rollout *v1alpha1.Rollout, opts metaV1.CreateOptions) (*v1alpha1.Rollout, error) {
	return nil, nil
}

func (f FakeRolloutsImpl) Update(ctx context.Context, rollout *v1alpha1.Rollout, opts metaV1.UpdateOptions) (*v1alpha1.Rollout, error) {
	return nil, nil
}

func (f FakeRolloutsImpl) UpdateStatus(ctx context.Context, rollout *v1alpha1.Rollout, opts metaV1.UpdateOptions) (*v1alpha1.Rollout, error) {
	return nil, nil
}

func (f FakeRolloutsImpl) Delete(ctx context.Context, name string, opts metaV1.DeleteOptions) error {
	return nil
}

func (f FakeRolloutsImpl) DeleteCollection(ctx context.Context, opts metaV1.DeleteOptions, listOpts metaV1.ListOptions) error {
	return nil
}

func (f FakeRolloutsImpl) Get(ctx context.Context, name string, opts metaV1.GetOptions) (*v1alpha1.Rollout, error) {
	return nil, nil
}

func (f FakeRolloutsImpl) List(ctx context.Context, opts metaV1.ListOptions) (*v1alpha1.RolloutList, error) {
	rollout1 := v1alpha1.Rollout{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "rollout-name",
			Namespace: RolloutNamespace,
		},
		Spec: v1alpha1.RolloutSpec{
			Strategy: v1alpha1.RolloutStrategy{
				Canary: &v1alpha1.CanaryStrategy{
					TrafficRouting: &v1alpha1.RolloutTrafficRouting{
						Istio: &v1alpha1.IstioTrafficRouting{
							VirtualService: &v1alpha1.IstioVirtualService{
								Name: "virtual-service-1",
							},
						},
					},
				},
			},
		},
	}
	rollout2 := v1alpha1.Rollout{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "rollout-name2",
			Namespace: RolloutNamespace,
		},
		Spec: v1alpha1.RolloutSpec{
			Strategy: v1alpha1.RolloutStrategy{
				Canary: &v1alpha1.CanaryStrategy{
					TrafficRouting: &v1alpha1.RolloutTrafficRouting{
						Istio: &v1alpha1.IstioTrafficRouting{
							VirtualService: &v1alpha1.IstioVirtualService{
								Name: "virtual-service-1",
							},
						},
					},
				},
			},
		},
	}
	list := &v1alpha1.RolloutList{Items: []v1alpha1.Rollout{rollout1, rollout2}}
	return list, nil
}

func (f FakeRolloutsImpl) Watch(ctx context.Context, opts metaV1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f FakeRolloutsImpl) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metaV1.PatchOptions, subresources ...string) (result *v1alpha1.Rollout, err error) {
	return nil, nil
}

func (m MockRolloutsGetter) RESTClient() rest.Interface {
	return nil
}

func (m MockRolloutsGetter) AnalysisRuns(namespace string) argoprojv1alpha1.AnalysisRunInterface {
	return nil
}

func (m MockRolloutsGetter) AnalysisTemplates(namespace string) argoprojv1alpha1.AnalysisTemplateInterface {
	return nil
}

func (m MockRolloutsGetter) ClusterAnalysisTemplates() argoprojv1alpha1.ClusterAnalysisTemplateInterface {
	return nil
}

func (m MockRolloutsGetter) Experiments(namespace string) argoprojv1alpha1.ExperimentInterface {
	return nil
}

func (m MockRolloutsGetter) Rollouts(namespace string) argoprojv1alpha1.RolloutInterface {
	return FakeRolloutsImpl{}
}

type MockOutlierDetectionHandler struct {
	Obj *admiralV1.OutlierDetection
}

func (m *MockOutlierDetectionHandler) Added(ctx context.Context, obj *admiralV1.OutlierDetection) error {
	m.Obj = obj
	return nil
}

func (m *MockOutlierDetectionHandler) Updated(ctx context.Context, obj *admiralV1.OutlierDetection) error {
	m.Obj = obj
	return nil
}

func (m *MockOutlierDetectionHandler) Deleted(ctx context.Context, obj *admiralV1.OutlierDetection) error {
	m.Obj = nil
	return nil
}

type MockShardHandler struct {
}

func (m *MockShardHandler) Added(ctx context.Context, obj *admiralapiv1.Shard) error {
	return nil
}

func (m *MockShardHandler) Deleted(ctx context.Context, obj *admiralapiv1.Shard) error {
	return nil
}
