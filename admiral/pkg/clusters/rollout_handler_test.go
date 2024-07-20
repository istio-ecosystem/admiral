package clusters

import (
	"context"
	"sync"
	"testing"
	"time"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	admiralFake "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned/fake"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var rolloutHandlerTestSingleton sync.Once

func admiralParamsForRolloutHandlerTests() common.AdmiralParams {
	return common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
			PriorityKey:             "priority",
			IdentityPartitionKey:    "admiral.io/identityPartition",
		},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheReconcileDuration:     time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace:      "default",
		EnableRoutingPolicy:        true,
		EnvoyFilterVersion:         "1.13",
		Profile:                    common.AdmiralProfileDefault,
		EnableSWAwareNSCaches:      true,
		ExportToIdentityList:       []string{"*"},
		ExportToMaxNamespaces:      35,
	}
}

func setupForRolloutHandlerTests() {
	rolloutHandlerTestSingleton.Do(func() {
		common.ResetSync()
		common.InitializeConfig(admiralParamsForRolloutHandlerTests())
	})
}

func TestRolloutHandlerPartitionCache(t *testing.T) {
	setupForRolloutHandlerTests()
	admiralParams := admiralParamsForRolloutHandlerTests()
	ctx := context.Background()
	remoteRegistry, _ := InitAdmiral(ctx, admiralParams)
	remoteRegistry.AdmiralCache.PartitionIdentityCache = common.NewMap()
	partitionIdentifier := "admiral.io/identityPartition"
	clusterName := "test-k8s"

	testCases := []struct {
		name     string
		rollout  argo.Rollout
		expected string
	}{
		{
			name: "Given the rollout has the partition label, " +
				"Then the PartitionIdentityCache should contain an entry for that rollout",
			rollout:  argo.Rollout{Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{ObjectMeta: metaV1.ObjectMeta{Labels: map[string]string{partitionIdentifier: "sw1", "env": "stage", "identity": "services.gateway"}}}}},
			expected: "services.gateway",
		},
		{
			name: "Given the rollout has the partition annotation, " +
				"Then the PartitionIdentityCache should contain an entry for that rollout",
			rollout:  argo.Rollout{Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{partitionIdentifier: "sw2", "env": "stage", "identity": "services.gateway"}}}}},
			expected: "services.gateway",
		},
		{
			name: "Given the rollout doesn't have the partition label or annotation, " +
				"Then the PartitionIdentityCache should not contain an entry for that rollout",
			rollout:  argo.Rollout{Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{ObjectMeta: metaV1.ObjectMeta{Labels: map[string]string{"identity": "services.gateway"}, Annotations: map[string]string{}}}}},
			expected: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			_ = HandleEventForRollout(ctx, admiral.Add, &c.rollout, remoteRegistry, clusterName)
			iVal := ""
			if len(c.expected) > 0 {
				globalIdentifier := common.GetRolloutGlobalIdentifier(&c.rollout)
				iVal = remoteRegistry.AdmiralCache.PartitionIdentityCache.Get(globalIdentifier)
			}
			if !(iVal == c.expected) {
				t.Errorf("Expected cache to contain: %s, got: %s", c.expected, iVal)
			}
		})
	}
}

func TestRolloutHandler(t *testing.T) {
	setupForRolloutHandlerTests()
	ctx := context.Background()
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.mutex = &sync.Mutex{}

	fakeCrdClient := admiralFake.NewSimpleClientset()
	gtpController := &admiral.GlobalTrafficController{CrdClient: fakeCrdClient}

	remoteController, _ := createMockRemoteController(func(i interface{}) {
	})
	remoteController.GlobalTraffic = gtpController
	registry, _ := InitAdmiral(context.Background(), p)
	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}
	registry.AdmiralCache.GlobalTrafficCache = gtpCache

	handler := RolloutHandler{}
	handler.RemoteRegistry = registry
	handler.ClusterID = "cluster-1"

	rollout := argo.Rollout{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test",
			Namespace: "namespace",
			Labels:    map[string]string{"identity": "app1"},
		},
		Spec: argo.RolloutSpec{
			Selector: &metaV1.LabelSelector{
				MatchLabels: map[string]string{"identity": "bar"},
			},
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
				},
			},
		},
	}

	testCases := []struct {
		name                       string
		addedRollout               *argo.Rollout
		expectedRolloutCacheKey    string
		expectedIdentityCacheValue *v1.GlobalTrafficPolicy
		expectedRolloutCacheValue  *argo.Rollout
	}{{
		name:                       "Shouldn't throw errors when called",
		addedRollout:               &rollout,
		expectedRolloutCacheKey:    "myGTP1",
		expectedIdentityCacheValue: nil,
		expectedRolloutCacheValue:  nil,
	}, {
		name:                       "Shouldn't throw errors when called-no identity",
		addedRollout:               &argo.Rollout{},
		expectedRolloutCacheKey:    "myGTP1",
		expectedIdentityCacheValue: nil,
		expectedRolloutCacheValue:  nil,
	},
	}

	//Rather annoying, but wasn't able to get the autogenerated fake k8s client for GTP objects to allow me to list resources, so this test is only for not throwing errors. I'll be testing the rest of the fucntionality picemeal.
	//Side note, if anyone knows how to fix `level=error msg="Failed to list rollouts in cluster, error: no kind \"GlobalTrafficPolicyList\" is registered for version \"admiral.io/v1\" in scheme \"pkg/runtime/scheme.go:101\""`, I'd love to hear it!
	//Already tried working through this: https://github.com/camilamacedo86/operator-sdk/blob/e40d7db97f0d132333b1e46ddf7b7f3cab1e379f/doc/user/unit-testing.md with no luck

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtpCache = &globalTrafficCache{}
			gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
			gtpCache.mutex = &sync.Mutex{}
			handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache
			handler.Added(ctx, c.addedRollout)
			ns := handler.RemoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Get("bar").Get("cluster-1").GetKeys()[0]
			if ns != "namespace" {
				t.Errorf("expected namespace: %v but got %v", "namespace", ns)
			}
			handler.Deleted(ctx, c.addedRollout)
			handler.Updated(ctx, c.addedRollout)
		})
	}
}

func newFakeRollout(name, namespace string, matchLabels map[string]string) *argo.Rollout {
	return &argo.Rollout{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: argo.RolloutSpec{
			Selector: &metaV1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}
}

type fakeHandleEventForRollout struct {
	handleEventForRolloutFunc func() HandleEventForRolloutFunc
	calledByRolloutName       map[string]bool
	calledRolloutByNamespace  map[string]map[string]bool
}

func (f *fakeHandleEventForRollout) CalledRolloutForNamespace(name, namespace string) bool {
	if f.calledRolloutByNamespace[namespace] != nil {
		return f.calledRolloutByNamespace[namespace][name]
	}
	return false
}

func newFakeHandleEventForRolloutsByError(errByRollout map[string]map[string]error) *fakeHandleEventForRollout {
	f := &fakeHandleEventForRollout{
		calledRolloutByNamespace: make(map[string]map[string]bool, 0),
	}
	f.handleEventForRolloutFunc = func() HandleEventForRolloutFunc {
		return func(
			ctx context.Context,
			event admiral.EventType,
			rollout *argo.Rollout,
			remoteRegistry *RemoteRegistry,
			clusterName string) error {
			if f.calledRolloutByNamespace[rollout.Namespace] == nil {
				f.calledRolloutByNamespace[rollout.Namespace] = map[string]bool{
					rollout.Name: true,
				}
			} else {
				f.calledRolloutByNamespace[rollout.Namespace][rollout.Name] = true
			}

			return errByRollout[rollout.Namespace][rollout.Name]
		}
	}
	return f
}
