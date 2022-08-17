package clusters

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	admiralFake "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned/fake"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	v12 "k8s.io/api/apps/v1"
	v13 "k8s.io/api/core/v1"
	time2 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var ignoreUnexported = cmpopts.IgnoreUnexported(v1.GlobalTrafficPolicy{}.Status)

func init() {
	p := common.AdmiralParams{
		KubeconfigPath:             "testdata/fake.config",
		LabelSet:                   &common.LabelSet{},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheRefreshDuration:       time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace:      "default",
		SecretResolver:             "",
		EnableRoutingPolicy:        true,
		EnvoyFilterVersion:         "1.13",
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.GlobalTrafficDeploymentLabel = "identity"
	p.LabelSet.PriorityKey = "priority"

	common.InitializeConfig(p)
}

func TestDeploymentHandler(t *testing.T) {

	ctx := context.Background()

	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	registry, _ := InitAdmiral(context.Background(), p)

	handler := DeploymentHandler{}

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.mutex = &sync.Mutex{}

	fakeCrdClient := admiralFake.NewSimpleClientset()

	gtpController := &admiral.GlobalTrafficController{CrdClient: fakeCrdClient}
	remoteController, _ := createMockRemoteController(func(i interface{}) {

	})
	remoteController.GlobalTraffic = gtpController

	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}

	registry.AdmiralCache.GlobalTrafficCache = gtpCache
	handler.RemoteRegistry = registry

	deployment := v12.Deployment{
		ObjectMeta: time2.ObjectMeta{
			Name:      "test",
			Namespace: "namespace",
			Labels:    map[string]string{"identity": "app1"},
		},
		Spec: v12.DeploymentSpec{
			Selector: &time2.LabelSelector{
				MatchLabels: map[string]string{"identity": "bar"},
			},
			Template: v13.PodTemplateSpec{
				ObjectMeta: time2.ObjectMeta{
					Labels: map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
				},
			},
		},
	}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name                         string
		addedDeployment              *v12.Deployment
		expectedDeploymentCacheKey   string
		expectedIdentityCacheValue   *v1.GlobalTrafficPolicy
		expectedDeploymentCacheValue *v12.Deployment
	}{
		{
			name:                         "Shouldn't throw errors when called",
			addedDeployment:              &deployment,
			expectedDeploymentCacheKey:   "myGTP1",
			expectedIdentityCacheValue:   nil,
			expectedDeploymentCacheValue: nil,
		},
	}

	//Rather annoying, but wasn't able to get the autogenerated fake k8s client for GTP objects to allow me to list resources, so this test is only for not throwing errors. I'll be testing the rest of the fucntionality picemeal.
	//Side note, if anyone knows how to fix `level=error msg="Failed to list deployments in cluster, error: no kind \"GlobalTrafficPolicyList\" is registered for version \"admiral.io/v1\" in scheme \"pkg/runtime/scheme.go:101\""`, I'd love to hear it!
	//Already tried working through this: https://github.com/camilamacedo86/operator-sdk/blob/e40d7db97f0d132333b1e46ddf7b7f3cab1e379f/doc/user/unit-testing.md with no luck

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtpCache = &globalTrafficCache{}
			gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
			gtpCache.mutex = &sync.Mutex{}
			handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache

			handler.Added(ctx, &deployment)
			handler.Deleted(ctx, &deployment)
		})
	}
}

func TestRolloutHandler(t *testing.T) {

	ctx := context.Background()

	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	registry, _ := InitAdmiral(context.Background(), p)

	handler := RolloutHandler{}

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.mutex = &sync.Mutex{}

	fakeCrdClient := admiralFake.NewSimpleClientset()

	gtpController := &admiral.GlobalTrafficController{CrdClient: fakeCrdClient}
	remoteController, _ := createMockRemoteController(func(i interface{}) {

	})
	remoteController.GlobalTraffic = gtpController

	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}

	registry.AdmiralCache.GlobalTrafficCache = gtpCache
	handler.RemoteRegistry = registry

	rollout := argo.Rollout{
		ObjectMeta: time2.ObjectMeta{
			Name:      "test",
			Namespace: "namespace",
			Labels:    map[string]string{"identity": "app1"},
		},
		Spec: argo.RolloutSpec{
			Selector: &time2.LabelSelector{
				MatchLabels: map[string]string{"identity": "bar"},
			},
			Template: v13.PodTemplateSpec{
				ObjectMeta: time2.ObjectMeta{
					Labels: map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
				},
			},
		},
	}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name                       string
		addedRolout                *argo.Rollout
		expectedRolloutCacheKey    string
		expectedIdentityCacheValue *v1.GlobalTrafficPolicy
		expectedRolloutCacheValue  *argo.Rollout
	}{{
		name:                       "Shouldn't throw errors when called",
		addedRolout:                &rollout,
		expectedRolloutCacheKey:    "myGTP1",
		expectedIdentityCacheValue: nil,
		expectedRolloutCacheValue:  nil,
	}, {
		name:                       "Shouldn't throw errors when called-no identity",
		addedRolout:                &argo.Rollout{},
		expectedRolloutCacheKey:    "myGTP1",
		expectedIdentityCacheValue: nil,
		expectedRolloutCacheValue:  nil,
	},
	}

	//Rather annoying, but wasn't able to get the autogenerated fake k8s client for GTP objects to allow me to list resources, so this test is only for not throwing errors. I'll be testing the rest of the fucntionality picemeal.
	//Side note, if anyone knows how to fix `level=error msg="Failed to list rollouts in cluster, error: no kind \"GlobalTrafficPolicyList\" is registered for version \"admiral.io/v1\" in scheme \"pkg/runtime/scheme.go:101\""`, I'd love to hear it!
	//Already tried working through this: https://github.com/camilamacedo86/operator-sdk/blob/e40d7db97f0d132333b1e46ddf7b7f3cab1e379f/doc/user/unit-testing.md with no luck

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtpCache = &globalTrafficCache{}
			gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
			gtpCache.mutex = &sync.Mutex{}
			handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache
			handler.Added(ctx, c.addedRolout)
			handler.Deleted(ctx, c.addedRolout)
			handler.Updated(ctx, c.addedRolout)
		})
	}
}

func TestHandleEventForGlobalTrafficPolicy(t *testing.T) {
	ctx := context.Background()
	event := admiral.EventType("Add")
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}
	registry, _ := InitAdmiral(context.Background(), p)

	testcases := []struct {
		name      string
		gtp       *v1.GlobalTrafficPolicy
		doesError bool
	}{
		{
			name: "missing identity label in GTP should result in error being returned by the handler",
			gtp: &v1.GlobalTrafficPolicy{
				ObjectMeta: time2.ObjectMeta{
					Name:        "testgtp",
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			doesError: true,
		},
		{
			name: "empty identity label in GTP should result in error being returned by the handler",
			gtp: &v1.GlobalTrafficPolicy{
				ObjectMeta: time2.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": ""},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			doesError: true,
		},
		{
			name: "valid GTP config which is expected to pass",
			gtp: &v1.GlobalTrafficPolicy{
				ObjectMeta: time2.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": "testapp"},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			doesError: false,
		},
	}

	for _, c := range testcases {
		t.Run(c.name, func(t *testing.T) {
			err := HandleEventForGlobalTrafficPolicy(ctx, event, c.gtp, registry, "testcluster")
			assert.Equal(t, err != nil, c.doesError)
		})
	}
}

func TestRoutingPolicyHandler(t *testing.T) {
	p := common.AdmiralParams{
		KubeconfigPath:             "testdata/fake.config",
		LabelSet:                   &common.LabelSet{},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheRefreshDuration:       time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace:      "default",
		SecretResolver:             "",
		EnableRoutingPolicy:        true,
		EnvoyFilterVersion:         "1.13",
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.GlobalTrafficDeploymentLabel = "identity"

	registry, _ := InitAdmiral(context.Background(), p)

	handler := RoutingPolicyHandler{}

	rpFilterCache := &routingPolicyFilterCache{}
	rpFilterCache.filterCache = make(map[string]map[string]map[string]string)
	rpFilterCache.mutex = &sync.Mutex{}

	routingPolicyController := &admiral.RoutingPolicyController{IstioClient: istiofake.NewSimpleClientset()}
	remoteController, _ := createMockRemoteController(func(i interface{}) {

	})
	remoteController.RoutingPolicyController = routingPolicyController

	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}
	registry.AdmiralCache.RoutingPolicyFilterCache = rpFilterCache

	// foo is dependent upon bar and bar has a deployment in the same cluster.
	registry.AdmiralCache.IdentityDependencyCache.Put("foo", "bar", "bar")
	registry.AdmiralCache.IdentityClusterCache.Put("bar", remoteController.ClusterID, remoteController.ClusterID)

	// foo is also dependent upon bar2 but bar2 is in a different cluster, so this cluster should not have the envoyfilter created
	registry.AdmiralCache.IdentityDependencyCache.Put("foo", "bar2", "bar2")
	registry.AdmiralCache.IdentityClusterCache.Put("bar2", "differentCluster", "differentCluster")

	// foo1 is dependent upon bar 1 but bar1 does not have a deployment so it is missing from identityClusterCache
	registry.AdmiralCache.IdentityDependencyCache.Put("foo1", "bar1", "bar1")

	var mp = common.NewMap()
	mp.Put("k1", "v1")
	registry.AdmiralCache.WorkloadSelectorCache.PutMap("bar"+remoteController.ClusterID, mp)
	registry.AdmiralCache.WorkloadSelectorCache.PutMap("bar2differentCluster", mp)

	handler.RemoteRegistry = registry

	routingPolicyFoo := &v1.RoutingPolicy{
		TypeMeta: time2.TypeMeta{},
		ObjectMeta: time2.ObjectMeta{
			Labels: map[string]string{
				"identity":       "foo",
				"admiral.io/env": "stage",
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
			},
		},
		Status: v1.RoutingPolicyStatus{},
	}

	routingPolicyFoo1 := routingPolicyFoo.DeepCopy()
	routingPolicyFoo1.Labels[common.GetWorkloadIdentifier()] = "foo1"

	testCases := []struct {
		name                   string
		routingPolicy          *v1.RoutingPolicy
		expectedFilterCacheKey string
		valueExpected          bool
	}{
		{
			name:                   "If dependent deployment exists, should fetch filter from cache",
			routingPolicy:          routingPolicyFoo,
			expectedFilterCacheKey: "barstage",
			valueExpected:          true,
		},
		{
			name:                   "If dependent deployment does not exist, the filter should not be created",
			routingPolicy:          routingPolicyFoo1,
			expectedFilterCacheKey: "bar1stage",
			valueExpected:          false,
		},
		{
			name:                   "If dependent deployment exists in a different cluster, the filter should not be created",
			routingPolicy:          routingPolicyFoo,
			expectedFilterCacheKey: "bar2stage",
			valueExpected:          false,
		},
	}

	ctx := context.Background()

	time.Sleep(time.Second * 30)
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			handler.Added(ctx, c.routingPolicy)
			if c.valueExpected {
				filterCacheValue := registry.AdmiralCache.RoutingPolicyFilterCache.Get(c.expectedFilterCacheKey)
				assert.NotNil(t, filterCacheValue)
				selectorLabelsSha, err := common.GetSha1("bar" + common.GetRoutingPolicyEnv(c.routingPolicy))
				if err != nil {
					t.Error("Error ocurred while computing workload Labels sha1")
				}
				envoyFilterName := fmt.Sprintf("%s-dynamicrouting-%s-%s", strings.ToLower(c.routingPolicy.Spec.Plugin), selectorLabelsSha, "1.13")
				filterMap := filterCacheValue[remoteController.ClusterID]
				assert.NotNil(t, filterMap)
				assert.NotNil(t, filterMap[envoyFilterName])

				// once the routing policy is deleted, the corresponding filter should also be deleted
				handler.Deleted(ctx, c.routingPolicy)
				assert.Nil(t, registry.AdmiralCache.RoutingPolicyFilterCache.Get(c.expectedFilterCacheKey))
			} else {
				assert.Nil(t, registry.AdmiralCache.RoutingPolicyFilterCache.Get(c.expectedFilterCacheKey))
			}

		})
	}

	// Test for multiple filters
	registry.AdmiralCache.IdentityDependencyCache.Put("foo", "bar3", "bar3")
	registry.AdmiralCache.IdentityClusterCache.Put("bar3", remoteController.ClusterID, remoteController.ClusterID)
	registry.AdmiralCache.WorkloadSelectorCache.PutMap("bar3"+remoteController.ClusterID, mp)
	handler.Added(ctx, routingPolicyFoo)

	selectorLabelsShaBar3, err := common.GetSha1("bar3" + common.GetRoutingPolicyEnv(routingPolicyFoo))
	if err != nil {
		t.Error("Error ocurred while computing workload Labels sha1")
	}
	envoyFilterNameBar3 := fmt.Sprintf("%s-dynamicrouting-%s-%s", strings.ToLower(routingPolicyFoo.Spec.Plugin), selectorLabelsShaBar3, "1.13")

	filterCacheValue := registry.AdmiralCache.RoutingPolicyFilterCache.Get("bar3stage")
	assert.NotNil(t, filterCacheValue)
	filterMap := filterCacheValue[remoteController.ClusterID]
	assert.NotNil(t, filterMap)
	assert.NotNil(t, filterMap[envoyFilterNameBar3])

	registry.AdmiralCache.IdentityDependencyCache.Put("foo", "bar4", "bar4")
	registry.AdmiralCache.IdentityClusterCache.Put("bar4", remoteController.ClusterID, remoteController.ClusterID)
	registry.AdmiralCache.WorkloadSelectorCache.PutMap("bar4"+remoteController.ClusterID, mp)
	handler.Updated(ctx, routingPolicyFoo)

	selectorLabelsShaBar4, err := common.GetSha1("bar4" + common.GetRoutingPolicyEnv(routingPolicyFoo))
	if err != nil {
		t.Error("Error ocurred while computing workload Labels sha1")
	}
	envoyFilterNameBar4 := fmt.Sprintf("%s-dynamicrouting-%s-%s", strings.ToLower(routingPolicyFoo.Spec.Plugin), selectorLabelsShaBar4, "1.13")

	filterCacheValue = registry.AdmiralCache.RoutingPolicyFilterCache.Get("bar4stage")
	assert.NotNil(t, filterCacheValue)
	filterMap = filterCacheValue[remoteController.ClusterID]
	assert.NotNil(t, filterMap)
	assert.NotNil(t, filterMap[envoyFilterNameBar4])

	// ignore the routing policy
	annotations := routingPolicyFoo.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[common.AdmiralIgnoreAnnotation] = "true"
	routingPolicyFoo.SetAnnotations(annotations)

	handler.Updated(ctx, routingPolicyFoo)
	assert.Nil(t, registry.AdmiralCache.RoutingPolicyFilterCache.Get("bar4stage"))
	assert.Nil(t, registry.AdmiralCache.RoutingPolicyFilterCache.Get("bar3stage"))

}
