package clusters

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	argofake "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/fake"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	admiralFake "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned/fake"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	v12 "k8s.io/api/apps/v1"
	v13 "k8s.io/api/core/v1"
	time2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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

	common.InitializeConfig(p)
}

func TestGlobalTrafficHandler(t *testing.T) {
	//lots of setup
	handler := GlobalTrafficHandler{}
	registry := &RemoteRegistry{}
	admiralCacle := &AdmiralCache{}

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.dependencyCache = make(map[string]*v12.Deployment)
	gtpCache.dependencyRolloutCache = make(map[string]*argo.Rollout)
	gtpCache.mutex = &sync.Mutex{}

	fakeClient := fake.NewSimpleClientset()

	deployment := v12.Deployment{}
	deployment.Namespace = "namespace"
	deployment.Name = "fake-app-deployment-qal"
	deployment.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "qal"},
			},
		},
	}
	deployment.Labels = map[string]string{"identity": "app1"}

	deployment2 := v12.Deployment{}
	deployment2.Namespace = "namespace"
	deployment2.Name = "fake-app-deployment-e2e"
	deployment2.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "e2e"},
			},
		},
	}
	deployment2.Labels = map[string]string{"identity": "app1"}

	deployment3 := v12.Deployment{}
	deployment3.Namespace = "namespace"
	deployment3.Name = "fake-app-deployment-prf-1"
	deployment3.CreationTimestamp = time2.Now()
	deployment3.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "prf"},
			},
		},
	}
	deployment3.Labels = map[string]string{"identity": "app1"}

	deployment4 := v12.Deployment{}
	deployment4.Namespace = "namespace"
	deployment4.Name = "fake-app-deployment-prf-2"
	deployment4.CreationTimestamp = time2.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
	deployment4.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "prf"},
			},
		},
	}
	deployment4.Labels = map[string]string{"identity": "app1"}

	_, err := fakeClient.AppsV1().Deployments("namespace").Create(&deployment)
	_, err = fakeClient.AppsV1().Deployments("namespace").Create(&deployment2)
	_, err = fakeClient.AppsV1().Deployments("namespace").Create(&deployment3)
	_, err = fakeClient.AppsV1().Deployments("namespace").Create(&deployment4)
	if err != nil {
		log.Fatalf("Failed to set up mock k8s client. Failing test. Error=%v", err)
	}

	deploymentController := &admiral.DeploymentController{K8sClient: fakeClient}
	remoteController := &RemoteController{}
	remoteController.DeploymentController = deploymentController

	noRolloutsClient := argofake.NewSimpleClientset().ArgoprojV1alpha1()
	rolloutController := &admiral.RolloutController{K8sClient: fakeClient, RolloutClient: noRolloutsClient}
	remoteController.RolloutController = rolloutController

	registry.RemoteControllers = map[string]*RemoteController{"cluster-1": remoteController}

	admiralCacle.GlobalTrafficCache = gtpCache
	registry.AdmiralCache = admiralCacle
	handler.RemoteRegistry = registry

	e2eGtp := v1.GlobalTrafficPolicy{}
	e2eGtp.Labels = map[string]string{"identity": "app1", "env": "e2e"}
	e2eGtp.Namespace = "namespace"
	e2eGtp.Name = "myGTP"

	noMatchGtp := v1.GlobalTrafficPolicy{}
	noMatchGtp.Labels = map[string]string{"identity": "app2", "env": "e2e"}
	noMatchGtp.Namespace = "namespace"
	noMatchGtp.Name = "myGTP"

	prfGtp := v1.GlobalTrafficPolicy{}
	prfGtp.Labels = map[string]string{"identity": "app1", "env": "prf"}
	prfGtp.Namespace = "namespace"
	prfGtp.Name = "myGTP"

	testCases := []struct {
		name                         string
		gtp                          *v1.GlobalTrafficPolicy
		expectedIdentity             string
		expectedEnv                  string
		expectedIdentityCacheValue   *v1.GlobalTrafficPolicy
		expectedDeploymentCacheValue *v12.Deployment
	}{
		{
			name:                         "Should return matching environment",
			gtp:                          &e2eGtp,
			expectedIdentity:             "app1",
			expectedEnv:                  "e2e",
			expectedDeploymentCacheValue: &deployment2,
			expectedIdentityCacheValue:   &e2eGtp,
		},
		{
			name:                         "Should return nothing when identity labels don't match",
			gtp:                          &noMatchGtp,
			expectedIdentity:             "app1",
			expectedEnv:                  "e2e",
			expectedDeploymentCacheValue: nil,
			expectedIdentityCacheValue:   nil,
		},
		{
			name:                         "Should return oldest deployment when multiple match",
			gtp:                          &prfGtp,
			expectedIdentity:             "app1",
			expectedEnv:                  "prf",
			expectedDeploymentCacheValue: &deployment4,
			expectedIdentityCacheValue:   &prfGtp,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			//clearing admiralCache
			gtpCache = &globalTrafficCache{}
			gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
			gtpCache.dependencyCache = make(map[string]*v12.Deployment)
			gtpCache.dependencyRolloutCache = make(map[string]*argo.Rollout)
			gtpCache.mutex = &sync.Mutex{}
			handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache

			//run the method under test then make assertions
			handler.Added(c.gtp)
			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv), c.expectedIdentityCacheValue, ignoreUnexported) {
				t.Fatalf("GTP Mismatch. Diff: %v", cmp.Diff(c.expectedIdentityCacheValue, handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv), ignoreUnexported))
			}
			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetDeployment(c.gtp.Name), c.expectedDeploymentCacheValue, ignoreUnexported) {
				t.Fatalf("Deployment Mismatch. Diff: %v", cmp.Diff(c.expectedDeploymentCacheValue, handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetDeployment(c.gtp.Name), ignoreUnexported))
			}

			handler.Deleted(c.gtp)

			if handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv) != nil {
				t.Fatalf("Delete failed for Identity Cache")
			}
			if handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetDeployment(c.gtp.Name) != nil {
				t.Fatalf("Delete failed for Dependency Cache")
			}

		})
	}
}

func TestGlobalTrafficHandler_Updated(t *testing.T) {
	//lots of setup
	handler := GlobalTrafficHandler{}
	registry := &RemoteRegistry{}
	admiralCacle := &AdmiralCache{}

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.dependencyCache = make(map[string]*v12.Deployment)
	gtpCache.dependencyRolloutCache = make(map[string]*argo.Rollout)
	gtpCache.mutex = &sync.Mutex{}

	fakeClient := fake.NewSimpleClientset()

	deployment := v12.Deployment{}
	deployment.Namespace = "namespace"
	deployment.Name = "fake-app-deployment-qal"
	deployment.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "qal"},
			},
		},
	}
	deployment.Labels = map[string]string{"identity": "app1"}

	deployment2 := v12.Deployment{}
	deployment2.Namespace = "namespace"
	deployment2.Name = "fake-app-deployment-e2e"
	deployment2.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "e2e"},
			},
		},
	}
	deployment2.Labels = map[string]string{"identity": "app1"}

	deployment3 := v12.Deployment{}
	deployment3.Namespace = "namespace"
	deployment3.Name = "fake-app-deployment-prf-1"
	deployment3.CreationTimestamp = time2.Now()
	deployment3.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "prf"},
			},
		},
	}
	deployment3.Labels = map[string]string{"identity": "app1"}

	deployment4 := v12.Deployment{}
	deployment4.Namespace = "namespace"
	deployment4.Name = "fake-app-deployment-prf-2"
	deployment4.CreationTimestamp = time2.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
	deployment4.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "prf"},
			},
		},
	}
	deployment4.Labels = map[string]string{"identity": "app1"}

	_, err := fakeClient.AppsV1().Deployments("namespace").Create(&deployment)
	_, err = fakeClient.AppsV1().Deployments("namespace").Create(&deployment2)
	_, err = fakeClient.AppsV1().Deployments("namespace").Create(&deployment3)
	_, err = fakeClient.AppsV1().Deployments("namespace").Create(&deployment4)
	if err != nil {
		log.Fatalf("Failed to set up mock k8s client. Failing test. Error=%v", err)
	}

	deploymentController := &admiral.DeploymentController{K8sClient: fakeClient}
	remoteController := &RemoteController{}
	remoteController.DeploymentController = deploymentController

	noRolloutsClient := argofake.NewSimpleClientset().ArgoprojV1alpha1()
	rolloutController := &admiral.RolloutController{K8sClient: fakeClient, RolloutClient: noRolloutsClient}
	remoteController.RolloutController = rolloutController

	registry.RemoteControllers = map[string]*RemoteController{"cluster-1": remoteController}

	admiralCacle.GlobalTrafficCache = gtpCache
	registry.AdmiralCache = admiralCacle
	handler.RemoteRegistry = registry

	e2eGtp := v1.GlobalTrafficPolicy{}
	e2eGtp.Labels = map[string]string{"identity": "app1", "env": "e2e"}
	e2eGtp.Namespace = "namespace"
	e2eGtp.Name = "myGTP"

	e2eGtpExtraLabel := v1.GlobalTrafficPolicy{}
	e2eGtpExtraLabel.Labels = map[string]string{"identity": "app1", "env": "e2e", "random": "foobar"}
	e2eGtpExtraLabel.Namespace = "namespace"
	e2eGtpExtraLabel.Name = "myGTP"

	noMatchGtp := v1.GlobalTrafficPolicy{}
	noMatchGtp.Labels = map[string]string{"identity": "app2", "env": "e2e"}
	noMatchGtp.Namespace = "namespace"
	noMatchGtp.Name = "myGTP"

	prfGtp := v1.GlobalTrafficPolicy{}
	prfGtp.Labels = map[string]string{"identity": "app1", "env": "prf"}
	prfGtp.Namespace = "namespace"
	prfGtp.Name = "myGTP"

	testCases := []struct {
		name                         string
		gtp                          *v1.GlobalTrafficPolicy
		updatedGTP                   *v1.GlobalTrafficPolicy
		expectedIdentity             string
		expectedEnv                  string
		expectedIdentityCacheValue   *v1.GlobalTrafficPolicy
		expectedDeploymentCacheValue *v12.Deployment
	}{
		{
			name:                         "Should return matching environment",
			gtp:                          &e2eGtp,
			updatedGTP:                   &e2eGtpExtraLabel,
			expectedIdentity:             "app1",
			expectedEnv:                  "e2e",
			expectedDeploymentCacheValue: &deployment2,
			expectedIdentityCacheValue:   &e2eGtpExtraLabel,
		},
		{
			name:                         "Should return nothing when identity labels don't match after update",
			gtp:                          &e2eGtp,
			updatedGTP:                   &noMatchGtp,
			expectedIdentity:             "app1",
			expectedEnv:                  "e2e",
			expectedDeploymentCacheValue: nil,
			expectedIdentityCacheValue:   nil,
		},
		{
			name:                         "Should return oldest deployment when multiple match",
			gtp:                          &e2eGtp,
			updatedGTP:                   &prfGtp,
			expectedIdentity:             "app1",
			expectedEnv:                  "prf",
			expectedDeploymentCacheValue: &deployment4,
			expectedIdentityCacheValue:   &prfGtp,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			//clearing admiralCache
			gtpCache = &globalTrafficCache{}
			gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
			gtpCache.dependencyCache = make(map[string]*v12.Deployment)
			gtpCache.dependencyRolloutCache = make(map[string]*argo.Rollout)
			gtpCache.mutex = &sync.Mutex{}
			handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache

			//run the method under test then make assertions
			handler.Added(c.gtp)
			handler.Updated(c.updatedGTP)
			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv), c.expectedIdentityCacheValue, ignoreUnexported) {
				t.Fatalf("GTP Mismatch. Diff: %v", cmp.Diff(c.expectedIdentityCacheValue, handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv), ignoreUnexported))
			}
			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetDeployment(c.gtp.Name), c.expectedDeploymentCacheValue, ignoreUnexported) {
				t.Fatalf("Deployment Mismatch. Diff: %v", cmp.Diff(c.expectedDeploymentCacheValue, handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetDeployment(c.gtp.Name), ignoreUnexported))
			}

			handler.Deleted(c.updatedGTP)

			if handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv) != nil {
				t.Fatalf("Delete failed for Identity Cache")
			}
			if handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetDeployment(c.gtp.Name) != nil {
				t.Fatalf("Delete failed for Dependency Cache")
			}

		})
	}
}

func TestDeploymentHandler(t *testing.T) {

	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	registry, _ := InitAdmiral(context.Background(), p)

	handler := DeploymentHandler{}

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.dependencyCache = make(map[string]*v12.Deployment)
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
			gtpCache.dependencyCache = make(map[string]*v12.Deployment)
			gtpCache.mutex = &sync.Mutex{}
			handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache

			handler.Added(&deployment)
			handler.Deleted(&deployment)
		})
	}
}

func TestGlobalTrafficCache(t *testing.T) {
	deployment := v12.Deployment{}
	deployment.Namespace = "namespace"
	deployment.Name = "fake-app-deployment-qal"
	deployment.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "e2e"},
			},
		},
	}
	deployment.Labels = map[string]string{"identity": "app1"}

	deploymentNoEnv := v12.Deployment{}
	deploymentNoEnv.Namespace = "namespace"
	deploymentNoEnv.Name = "fake-app-deployment-qal"
	deploymentNoEnv.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1"},
			},
		},
	}
	deploymentNoEnv.Labels = map[string]string{"identity": "app1"}

	e2eGtp := v1.GlobalTrafficPolicy{}
	e2eGtp.Labels = map[string]string{"identity": "app1", "env": "e2e"}
	e2eGtp.Namespace = "namespace"
	e2eGtp.Name = "myGTP"

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.dependencyCache = make(map[string]*v12.Deployment)
	gtpCache.mutex = &sync.Mutex{}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name        string
		gtp         *v1.GlobalTrafficPolicy
		deployment  *v12.Deployment
		identity    string
		environment string
		gtpName     string
	}{
		{
			name:        "Base case",
			gtp:         &e2eGtp,
			deployment:  &deployment,
			identity:    "app1",
			environment: "e2e",
			gtpName:     "myGTP",
		},
		{
			name:        "No Deployment",
			gtp:         &e2eGtp,
			deployment:  nil,
			identity:    "app1",
			environment: "e2e",
			gtpName:     "myGTP",
		},
		{
			name:        "Handles lack of environment label properly",
			gtp:         &e2eGtp,
			deployment:  &deploymentNoEnv,
			identity:    "app1",
			environment: "default",
			gtpName:     "myGTP",
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {

			gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
			gtpCache.dependencyCache = make(map[string]*v12.Deployment)
			gtpCache.mutex = &sync.Mutex{}
			err := gtpCache.Put(c.gtp, c.deployment)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			foundGtp := gtpCache.GetFromIdentity(c.identity, c.environment)
			if c.deployment != nil && !cmp.Equal(foundGtp, c.gtp, ignoreUnexported) {
				t.Fatalf("GTP Mismatch: %v", cmp.Diff(foundGtp, c.gtp, ignoreUnexported))
			}

			//no deployment means there should be nothing in the identity cache
			if c.deployment == nil && foundGtp != nil {
				t.Fatalf("GTP Mismatch: %v", cmp.Diff(foundGtp, nil, ignoreUnexported))
			}

			foundDeployment := gtpCache.GetDeployment(c.gtpName)
			if !cmp.Equal(foundDeployment, c.deployment, ignoreUnexported) {
				t.Fatalf("Deployment Mismatch: %v", cmp.Diff(foundDeployment, c.deployment, ignoreUnexported))
			}

			gtpCache.Delete(c.gtp)
			if len(gtpCache.dependencyCache) != 0 {
				t.Fatalf("Dependency cache not cleared properly on delete")
			}
			if len(gtpCache.identityCache) != 0 {
				t.Fatalf("Identity cache not cleared properly on delete")
			}
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
	gtpCache.dependencyRolloutCache = make(map[string]*argo.Rollout)
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
			gtpCache.dependencyRolloutCache = make(map[string]*argo.Rollout)
			gtpCache.mutex = &sync.Mutex{}
			handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache

			handler.Added(c.addedRolout)
			handler.Deleted(c.addedRolout)
			handler.Updated(c.addedRolout)
		})
	}
}

func TestGlobalTrafficCacheForRollout(t *testing.T) {
	rollout := argo.Rollout{}
	rollout.Namespace = "namespace"
	rollout.Name = "fake-app-rollout-qal"
	rollout.Spec = argo.RolloutSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "e2e"},
			},
		},
	}
	rollout.Labels = map[string]string{"identity": "app1"}

	rolloutNoEnv := argo.Rollout{}
	rolloutNoEnv.Namespace = "namespace"
	rolloutNoEnv.Name = "fake-app-rollout-qal"
	rolloutNoEnv.Spec = argo.RolloutSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1"},
			},
		},
	}
	rolloutNoEnv.Labels = map[string]string{"identity": "app1"}

	e2eGtp := v1.GlobalTrafficPolicy{}
	e2eGtp.Labels = map[string]string{"identity": "app1", "env": "e2e"}
	e2eGtp.Namespace = "namespace"
	e2eGtp.Name = "myGTP"

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.dependencyRolloutCache = make(map[string]*argo.Rollout)
	gtpCache.mutex = &sync.Mutex{}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name        string
		gtp         *v1.GlobalTrafficPolicy
		rollout     *argo.Rollout
		identity    string
		environment string
		gtpName     string
	}{
		{
			name:        "Base case",
			gtp:         &e2eGtp,
			rollout:     &rollout,
			identity:    "app1",
			environment: "e2e",
			gtpName:     "myGTP",
		},
		{
			name:        "No rollout",
			gtp:         &e2eGtp,
			rollout:     nil,
			identity:    "app1",
			environment: "e2e",
			gtpName:     "myGTP",
		},
		{
			name:        "Handles lack of environment label properly",
			gtp:         &e2eGtp,
			rollout:     &rolloutNoEnv,
			identity:    "app1",
			environment: "default",
			gtpName:     "myGTP",
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {

			gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
			gtpCache.dependencyRolloutCache = make(map[string]*argo.Rollout)
			gtpCache.mutex = &sync.Mutex{}
			err := gtpCache.PutRollout(c.gtp, c.rollout)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			foundGtp := gtpCache.GetFromIdentity(c.identity, c.environment)
			if c.rollout != nil && !cmp.Equal(foundGtp, c.gtp, ignoreUnexported) {
				t.Fatalf("GTP Mismatch: %v", cmp.Diff(foundGtp, c.gtp, ignoreUnexported))
			}

			//no rollout means there should be nothing in the identity cache
			if c.rollout == nil && foundGtp != nil {
				t.Fatalf("GTP Mismatch: %v", cmp.Diff(foundGtp, nil, ignoreUnexported))
			}

			foundRollout := gtpCache.GetRollout(c.gtpName)
			if !cmp.Equal(foundRollout, c.rollout, ignoreUnexported) {
				t.Fatalf("Rollout Mismatch: %v", cmp.Diff(foundRollout, c.rollout, ignoreUnexported))
			}

			gtpCache.Delete(c.gtp)
			if len(gtpCache.dependencyCache) != 0 {
				t.Fatalf("Dependency cache not cleared properly on delete")
			}
			if len(gtpCache.identityCache) != 0 {
				t.Fatalf("Identity cache not cleared properly on delete")
			}
		})
	}

}

func TestGlobalTrafficHandler_Updated_ForRollouts(t *testing.T) {
	//lots of setup
	handler := GlobalTrafficHandler{}
	registry := &RemoteRegistry{}
	admiralCacle := &AdmiralCache{}

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.dependencyRolloutCache = make(map[string]*argo.Rollout)
	gtpCache.dependencyCache = make(map[string]*v12.Deployment)
	gtpCache.mutex = &sync.Mutex{}

	fakeClient := fake.NewSimpleClientset()

	rollout := argo.Rollout{}
	rollout.Namespace = "namespace"
	rollout.Name = "fake-app-rollout-qal"
	rollout.Spec = argo.RolloutSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "qal"},
			},
		},
	}
	rollout.Labels = map[string]string{"identity": "app1"}

	rollout2 := argo.Rollout{}
	rollout2.Namespace = "namespace"
	rollout2.Name = "fake-app-rollout-e2e"
	rollout2.Spec = argo.RolloutSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "e2e"},
			},
		},
	}
	rollout2.Labels = map[string]string{"identity": "app1"}

	rollout3 := argo.Rollout{}
	rollout3.Namespace = "namespace"
	rollout3.Name = "fake-app-rollout-prf-1"
	rollout3.CreationTimestamp = time2.Now()
	rollout3.Spec = argo.RolloutSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "prf"},
			},
		},
	}
	rollout3.Labels = map[string]string{"identity": "app1"}

	rollout4 := argo.Rollout{}
	rollout4.Namespace = "namespace"
	rollout4.Name = "fake-app-rollout-prf-2"
	rollout4.CreationTimestamp = time2.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
	rollout4.Spec = argo.RolloutSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "prf"},
			},
		},
	}
	rollout4.Labels = map[string]string{"identity": "app1"}

	deploymentController := &admiral.DeploymentController{K8sClient: fakeClient}
	remoteController := &RemoteController{}
	remoteController.DeploymentController = deploymentController

	noRolloutsClient := argofake.NewSimpleClientset().ArgoprojV1alpha1()
	noRolloutsClient.Rollouts("namespace").Create(&rollout)

	_, err := noRolloutsClient.Rollouts("namespace").Create(&rollout)
	_, err = noRolloutsClient.Rollouts("namespace").Create(&rollout2)
	_, err = noRolloutsClient.Rollouts("namespace").Create(&rollout3)
	_, err = noRolloutsClient.Rollouts("namespace").Create(&rollout4)
	if err != nil {
		log.Fatalf("Failed to set up mock k8s client. Failing test. Error=%v", err)
	}

	rolloutController := &admiral.RolloutController{K8sClient: fakeClient, RolloutClient: noRolloutsClient}
	remoteController.RolloutController = rolloutController

	registry.RemoteControllers = map[string]*RemoteController{"cluster-1": remoteController}

	admiralCacle.GlobalTrafficCache = gtpCache
	registry.AdmiralCache = admiralCacle
	admiralCacle.argoRolloutsEnabled = true
	handler.RemoteRegistry = registry

	e2eGtp := v1.GlobalTrafficPolicy{}
	e2eGtp.Labels = map[string]string{"identity": "app1", "env": "e2e"}
	e2eGtp.Namespace = "namespace"
	e2eGtp.Name = "myGTP"

	e2eGtpExtraLabel := v1.GlobalTrafficPolicy{}
	e2eGtpExtraLabel.Labels = map[string]string{"identity": "app1", "env": "e2e", "random": "foobar"}
	e2eGtpExtraLabel.Namespace = "namespace"
	e2eGtpExtraLabel.Name = "myGTP"

	noMatchGtp := v1.GlobalTrafficPolicy{}
	noMatchGtp.Labels = map[string]string{"identity": "app2", "env": "e2e"}
	noMatchGtp.Namespace = "namespace"
	noMatchGtp.Name = "myGTP"

	prfGtp := v1.GlobalTrafficPolicy{}
	prfGtp.Labels = map[string]string{"identity": "app1", "env": "prf"}
	prfGtp.Namespace = "namespace"
	prfGtp.Name = "myGTP"

	testCases := []struct {
		name                       string
		gtp                        *v1.GlobalTrafficPolicy
		updatedGTP                 *v1.GlobalTrafficPolicy
		expectedIdentity           string
		expectedEnv                string
		expectedIdentityCacheValue *v1.GlobalTrafficPolicy
		expectedRolloutCacheValue  *argo.Rollout
	}{
		{
			name:                       "Should return matching environment",
			gtp:                        &e2eGtp,
			updatedGTP:                 &e2eGtpExtraLabel,
			expectedIdentity:           "app1",
			expectedEnv:                "e2e",
			expectedRolloutCacheValue:  &rollout2,
			expectedIdentityCacheValue: &e2eGtpExtraLabel,
		},
		{
			name:                       "Should return nothing when identity labels don't match after update",
			gtp:                        &e2eGtp,
			updatedGTP:                 &noMatchGtp,
			expectedIdentity:           "app1",
			expectedEnv:                "e2e",
			expectedRolloutCacheValue:  nil,
			expectedIdentityCacheValue: nil,
		},
		{
			name:                       "Should return oldest rollout when multiple match",
			gtp:                        &e2eGtp,
			updatedGTP:                 &prfGtp,
			expectedIdentity:           "app1",
			expectedEnv:                "prf",
			expectedRolloutCacheValue:  &rollout4,
			expectedIdentityCacheValue: &prfGtp,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			//clearing admiralCache
			gtpCache = &globalTrafficCache{}
			gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
			gtpCache.dependencyRolloutCache = make(map[string]*argo.Rollout)
			gtpCache.dependencyCache = make(map[string]*v12.Deployment)
			gtpCache.mutex = &sync.Mutex{}
			handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache

			//run the method under test then make assertions
			handler.Added(c.gtp)
			handler.Updated(c.updatedGTP)
			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv), c.expectedIdentityCacheValue, ignoreUnexported) {
				t.Fatalf("GTP Mismatch. Diff: %v", cmp.Diff(c.expectedIdentityCacheValue, handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv), ignoreUnexported))
			}
			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetRollout(c.gtp.Name), c.expectedRolloutCacheValue, ignoreUnexported) {
				t.Fatalf("Rollout Mismatch. Diff: %v", cmp.Diff(c.expectedRolloutCacheValue, handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetRollout(c.gtp.Name), ignoreUnexported))
			}

			handler.Deleted(c.updatedGTP)

			if handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv) != nil {
				t.Fatalf("Delete failed for Identity Cache")
			}
			if handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetRollout(c.gtp.Name) != nil {
				t.Fatalf("Delete failed for Dependency Cache")
			}

		})
	}
}
