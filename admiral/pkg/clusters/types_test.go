package clusters

import (
	"context"
	"sync"
	"testing"
	"time"

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
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.GlobalTrafficDeploymentLabel = "identity"
	p.LabelSet.PriorityKey = "priority"

	common.InitializeConfig(p)
}

func TestDeploymentHandler(t *testing.T) {

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

	registry.RemoteControllers = map[string]*RemoteController{"cluster-1": remoteController}

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

			handler.Added(&deployment)
			handler.Deleted(&deployment)
		})
	}
}

func TestRolloutHandler(t *testing.T) {

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

	registry.RemoteControllers = map[string]*RemoteController{"cluster-1": remoteController}

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
			handler.Added(c.addedRolout)
			handler.Deleted(c.addedRolout)
			handler.Updated(c.addedRolout)
		})
	}
}

func TestHandleEventForGlobalTrafficPolicy(t *testing.T) {
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
			err := HandleEventForGlobalTrafficPolicy(c.gtp, registry, "testcluster")
			assert.Equal(t, err != nil, c.doesError)
		})
	}

}
