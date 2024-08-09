package clusters

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"istio.io/api/networking/v1alpha3"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	k8sAppsV1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sCorev1 "k8s.io/api/core/v1"
)

func TestCreateOrUpdateEnvoyFilter(t *testing.T) {
	registry := getRegistry("1.13,1.17")

	handler := RoutingPolicyHandler{}

	rpFilterCache := &routingPolicyFilterCache{}
	rpFilterCache.filterCache = make(map[string]map[string]map[string]string)
	rpFilterCache.mutex = &sync.Mutex{}

	routingPolicyController := &admiral.RoutingPolicyController{IstioClient: istiofake.NewSimpleClientset()}
	remoteController, _ := createMockRemoteController(func(i interface{}) {

	})

	deployment := k8sAppsV1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
			Labels:    map[string]string{"sidecar.istio.io/inject": "true", "identity": "bar", "env": "dev"},
		},
		Spec: k8sAppsV1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"identity": "bar"},
			},
			Template: k8sCorev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"sidecar.istio.io/inject": "true"},
					Labels:      map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
				},
			},
		},
	}

	rollout := v1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
			Labels:    map[string]string{"sidecar.istio.io/inject": "true", "identity": "bar", "env": "dev"},
		},
		Spec: v1alpha1.RolloutSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"identity": "bar"},
			},
			Template: k8sCorev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"sidecar.istio.io/inject": "true"},
					Labels:      map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
				},
			},
		},
	}
	ctx := context.Background()
	remoteController.RolloutController.Added(ctx, &rollout)
	remoteController.RoutingPolicyController = routingPolicyController

	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}
	registry.AdmiralCache.RoutingPolicyFilterCache = rpFilterCache

	// foo is dependent upon bar and bar has a deployment in the same cluster.
	registry.AdmiralCache.IdentityDependencyCache.Put("foo", "bar", "bar")
	registry.AdmiralCache.IdentityClusterCache.Put("bar", remoteController.ClusterID, remoteController.ClusterID)

	handler.RemoteRegistry = registry

	routingPolicyFoo := &v1.RoutingPolicy{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "routingpolicy-foo",
			Labels: map[string]string{
				"identity":       "foo",
				"admiral.io/env": "dev",
			},
		},
		Spec: model.RoutingPolicy{
			Plugin: "test",
			Hosts:  []string{"e2e.testservice.mesh"},
			Config: map[string]string{
				"cachePrefix":       "cache-v1",
				"cachettlSec":       "86400",
				"routingServiceUrl": "e2e.test.routing.service.mesh",
				"pathPrefix":        "/sayhello,/v1/company/{id}/",
				"wasmPath":          "dummyPath",
			},
		},
		Status: v1.RoutingPolicyStatus{},
	}

	envoyFilter_113 := &networking.EnvoyFilter{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-dr-70395ba3470fd8ce6062-f6ce3712830af1b15625-1.13",
		},
		Spec: v1alpha3.EnvoyFilter{
			ConfigPatches: nil,
			Priority:      0,
		},
	}
	envoyFilter_117 := &networking.EnvoyFilter{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-dr-70395ba3470fd8ce6062-f6ce3712830af1b15625-1.17",
		},
		Spec: v1alpha3.EnvoyFilter{
			ConfigPatches: nil,
			Priority:      0,
		},
	}

	getSha1 = common.GetSha1

	//Struct of test case info. Name is required.
	testCases := []struct {
		name                string
		workloadKey         string
		routingPolicy       *v1.RoutingPolicy
		eventType           admiral.EventType
		expectedEnvoyFilter *networking.EnvoyFilter
		filterCount         int
		registry            *AdmiralCache
		shaMethod           func(interface{}) (string, error)
		matchingRollout     bool
	}{
		{
			name: "Given dynamic routing is enabled in admiral startup params, " +
				"When an ADD event for routing policy is received but sha1 calculation fails" +
				"Then 0 envoy filters are created and error is thrown",
			workloadKey:         "bar",
			routingPolicy:       routingPolicyFoo,
			eventType:           admiral.Add,
			expectedEnvoyFilter: nil,
			filterCount:         0,
			registry:            registry.AdmiralCache,
			shaMethod:           getSha1Error,
		},
		{
			name: "Given 2 envoy filter versions are specified in Admiral startup params, " +
				"And there exists a dependent service, which has a deployment, " +
				"When an ADD event is received for routing policy" +
				"Then 2 envoy filters are created, one for each version in each dependent cluster's istio-system ns",
			workloadKey:         "bar",
			routingPolicy:       routingPolicyFoo,
			eventType:           admiral.Add,
			expectedEnvoyFilter: envoyFilter_113,
			filterCount:         2,
			registry:            registry.AdmiralCache,
		},
		{
			name: "Given 2 envoy filter versions are specified in Admiral startup params, " +
				"When an UPDATE event is received for routing policy" +
				"Then 2 envoy filters are created, one for each version in each dependent's ns",
			workloadKey:         "bar",
			routingPolicy:       routingPolicyFoo,
			eventType:           admiral.Update,
			expectedEnvoyFilter: envoyFilter_113,
			filterCount:         2,
			registry:            registry.AdmiralCache,
		},
		{
			name: "Given 2 envoy filter versions are specified in Admiral startup params, " +
				"And there exists a dependent service, which has a rollout, " +
				"When an ADD event is received for routing policy" +
				"Then 2 envoy filters are created, one for each version in dependent cluster's istio-system ns",
			workloadKey:         "bar",
			routingPolicy:       routingPolicyFoo,
			eventType:           admiral.Add,
			expectedEnvoyFilter: envoyFilter_113,
			filterCount:         2,
			registry:            registry.AdmiralCache,
			matchingRollout:     true,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.shaMethod != nil {
				getSha1 = c.shaMethod
			} else {
				getSha1 = common.GetSha1
			}
			if c.matchingRollout {
				remoteController.DeploymentController.Deleted(ctx, &deployment)
			} else {
				remoteController.DeploymentController.Added(ctx, &deployment)
			}
			if c.eventType == admiral.Update {
				remoteController.RoutingPolicyController.IstioClient.NetworkingV1alpha3().
					EnvoyFilters(common.NamespaceIstioSystem).Create(context.Background(), envoyFilter_113, metav1.CreateOptions{})
				remoteController.RoutingPolicyController.IstioClient.NetworkingV1alpha3().
					EnvoyFilters(common.NamespaceIstioSystem).Create(context.Background(), envoyFilter_117, metav1.CreateOptions{})

			}
			envoyfilterList, err := createOrUpdateEnvoyFilter(ctx, remoteController, c.routingPolicy, c.eventType, c.workloadKey, c.registry)

			if err != nil && c.expectedEnvoyFilter != nil {
				t.Fatalf("EnvoyFilter error: %v", err)
			}

			if c.expectedEnvoyFilter != nil && c.filterCount == len(envoyfilterList) && !cmp.Equal(envoyfilterList[0].Name, c.expectedEnvoyFilter.Name, protocmp.Transform()) {
				t.Fatalf("EnvoyFilter Mismatch. Diff: %v", cmp.Diff(envoyfilterList[0], c.expectedEnvoyFilter, protocmp.Transform()))
			}

			for _, ef := range envoyfilterList {
				assert.Equal(t, "bar", ef.Spec.WorkloadSelector.Labels[common.AssetAlias])
				assert.Equal(t, c.routingPolicy.Name, ef.Annotations[envoyfilterAssociatedRoutingPolicyNameAnnotation])
				assert.Equal(t, common.GetRoutingPolicyIdentity(c.routingPolicy), ef.Annotations[envoyfilterAssociatedRoutingPolicyIdentityeAnnotation])
				assert.Equal(t, "istio-system", ef.ObjectMeta.Namespace)
				// assert filename in vm_config
				assert.Contains(t, ef.Spec.ConfigPatches[0].Patch.Value.String(), common.WasmPathValue)
			}
		})
		t.Cleanup(func() {
			remoteController.RoutingPolicyController.IstioClient.NetworkingV1alpha3().
				EnvoyFilters(common.NamespaceIstioSystem).Delete(context.Background(), "test-dr-70395ba3470fd8ce6062-f6ce3712830af1b15625-1.13", metav1.DeleteOptions{})
			remoteController.RoutingPolicyController.IstioClient.NetworkingV1alpha3().
				EnvoyFilters(common.NamespaceIstioSystem).Delete(context.Background(), "test-dr-70395ba3470fd8ce6062-f6ce3712830af1b15625-1.17", metav1.DeleteOptions{})

		})
	}
}

func getRegistry(filterVersion string) *RemoteRegistry {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			DeploymentAnnotation: "sidecar.istio.io/inject",
		},
		KubeconfigPath:             "testdata/fake.config",
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheReconcileDuration:     time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace:      "default",
		EnvoyFilterVersion:         filterVersion,
		Profile:                    common.AdmiralProfileDefault,
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"

	common.ResetSync()
	registry, _ := InitAdmiral(context.Background(), p)
	return registry
}

func getSha1Error(key interface{}) (string, error) {
	return "", errors.New("error occured while computing the sha")
}

func TestGetHosts(t *testing.T) {
	routingPolicyFoo := &v1.RoutingPolicy{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"identity":       "foo",
				"admiral.io/env": "stage",
			},
		},
		Spec: model.RoutingPolicy{
			Plugin: "test",
			Hosts:  []string{"e2e.testservice.mesh,e2e2.testservice.mesh"},
			Config: map[string]string{
				"cachePrefix":       "cache-v1",
				"cachettlSec":       "86400",
				"routingServiceUrl": "e2e.test.routing.service.mesh",
				"pathPrefix":        "/sayhello,/v1/company/{id}/",
			},
		},
		Status: v1.RoutingPolicyStatus{},
	}

	hosts := getHosts(routingPolicyFoo)
	assert.Equal(t, "hosts: e2e.testservice.mesh,e2e2.testservice.mesh", hosts)
}

func TestGetPlugin(t *testing.T) {
	routingPolicyFoo := &v1.RoutingPolicy{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"identity":       "foo",
				"admiral.io/env": "stage",
			},
		},
		Spec: model.RoutingPolicy{
			Plugin: "test",
			Hosts:  []string{"e2e.testservice.mesh,e2e2.testservice.mesh"},
			Config: map[string]string{
				"cachePrefix":       "cache-v1",
				"cachettlSec":       "86400",
				"routingServiceUrl": "e2e.test.routing.service.mesh",
				"pathPrefix":        "/sayhello,/v1/company/{id}/",
			},
		},
		Status: v1.RoutingPolicyStatus{},
	}

	plugin := getPlugin(routingPolicyFoo)
	assert.Equal(t, "plugin: test", plugin)
}
