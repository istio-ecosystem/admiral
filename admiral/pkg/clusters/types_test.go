package clusters

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	v12 "k8s.io/api/apps/v1"
	v13 "k8s.io/api/core/v1"
	time2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"log"
	"sync"
	"testing"
	"time"
)

var ignoreUnexported = cmpopts.IgnoreUnexported(v1.GlobalTrafficPolicy{}.Status)

func init() {
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{},
		EnableSAN: true,
		SANPrefix: "prefix",
		HostnameSuffix: "mesh",
		SyncNamespace: "ns",
		CacheRefreshDuration: time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace: "default",
		SecretResolver: "",

	}

	p.LabelSet.WorkloadIdentityKey="identity"
	p.LabelSet.GlobalTrafficDeploymentLabel="identity"

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
	gtpCache.mutex = &sync.Mutex{}


	fakeClient := fake.NewSimpleClientset()

	deployment := v12.Deployment{}
	deployment.Namespace = "namespace"
	deployment.Name = "fake-app-deployment-qal"
	deployment.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env":"qal"},
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
				Labels: map[string]string{"identity": "app1", "env":"e2e"},
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
				Labels: map[string]string{"identity": "app1", "env":"prf"},
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
				Labels: map[string]string{"identity": "app1", "env":"prf"},
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

	deploymentController := &admiral.DeploymentController{K8sClient:fakeClient}
	remoteController := &RemoteController{}
	remoteController.DeploymentController = deploymentController

	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}

	admiralCacle.GlobalTrafficCache = gtpCache
	registry.AdmiralCache = admiralCacle
	handler.RemoteRegistry = registry


	e2eGtp := v1.GlobalTrafficPolicy{}
	e2eGtp.Labels = map[string]string{"identity": "app1", "env":"e2e"}
	e2eGtp.Namespace = "namespace"
	e2eGtp.Name = "myGTP"

	noMatchGtp := v1.GlobalTrafficPolicy{}
	noMatchGtp.Labels = map[string]string{"identity": "app2", "env":"e2e"}
	noMatchGtp.Namespace = "namespace"
	noMatchGtp.Name = "myGTP"

	prfGtp := v1.GlobalTrafficPolicy{}
	prfGtp.Labels = map[string]string{"identity": "app1", "env":"prf"}
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
			gtpCache.mutex = &sync.Mutex{}
			handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache

			//run the method under test then make assertions
			handler.Added(c.gtp)
			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv), c.expectedIdentityCacheValue, ignoreUnexported) {
				t.Fatalf("GTP Mismatch. Diff: %v", cmp.Diff(c.expectedIdentityCacheValue, handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv), ignoreUnexported))
			}
			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetDeployment(c.gtp.Name),  c.expectedDeploymentCacheValue, ignoreUnexported) {
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
	gtpCache.mutex = &sync.Mutex{}


	fakeClient := fake.NewSimpleClientset()

	deployment := v12.Deployment{}
	deployment.Namespace = "namespace"
	deployment.Name = "fake-app-deployment-qal"
	deployment.Spec = v12.DeploymentSpec{
		Template: v13.PodTemplateSpec{
			ObjectMeta: time2.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env":"qal"},
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
				Labels: map[string]string{"identity": "app1", "env":"e2e"},
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
				Labels: map[string]string{"identity": "app1", "env":"prf"},
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
				Labels: map[string]string{"identity": "app1", "env":"prf"},
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

	deploymentController := &admiral.DeploymentController{K8sClient:fakeClient}
	remoteController := &RemoteController{}
	remoteController.DeploymentController = deploymentController

	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}

	admiralCacle.GlobalTrafficCache = gtpCache
	registry.AdmiralCache = admiralCacle
	handler.RemoteRegistry = registry


	e2eGtp := v1.GlobalTrafficPolicy{}
	e2eGtp.Labels = map[string]string{"identity": "app1", "env":"e2e"}
	e2eGtp.Namespace = "namespace"
	e2eGtp.Name = "myGTP"

	e2eGtpExtraLabel := v1.GlobalTrafficPolicy{}
	e2eGtpExtraLabel.Labels = map[string]string{"identity": "app1", "env":"e2e", "random": "foobar"}
	e2eGtpExtraLabel.Namespace = "namespace"
	e2eGtpExtraLabel.Name = "myGTP"

	noMatchGtp := v1.GlobalTrafficPolicy{}
	noMatchGtp.Labels = map[string]string{"identity": "app2", "env":"e2e"}
	noMatchGtp.Namespace = "namespace"
	noMatchGtp.Name = "myGTP"

	prfGtp := v1.GlobalTrafficPolicy{}
	prfGtp.Labels = map[string]string{"identity": "app1", "env":"prf"}
	prfGtp.Namespace = "namespace"
	prfGtp.Name = "myGTP"



	testCases := []struct {
		name                         string
		gtp                          *v1.GlobalTrafficPolicy
		updatedGTP                          *v1.GlobalTrafficPolicy
		expectedIdentity             string
		expectedEnv                  string
		expectedIdentityCacheValue   *v1.GlobalTrafficPolicy
		expectedDeploymentCacheValue *v12.Deployment

	}{
		{
			name:                         "Should return matching environment",
			gtp:                          &e2eGtp,
			updatedGTP:					  &e2eGtpExtraLabel,
			expectedIdentity:             "app1",
			expectedEnv:                  "e2e",
			expectedDeploymentCacheValue: &deployment2,
			expectedIdentityCacheValue:   &e2eGtpExtraLabel,
		},
		{
			name:                         "Should return nothing when identity labels don't match after update",
			gtp:                          &e2eGtp,
			updatedGTP:					  &noMatchGtp,
			expectedIdentity:             "app1",
			expectedEnv:                  "e2e",
			expectedDeploymentCacheValue: nil,
			expectedIdentityCacheValue:   nil,
		},
		{
			name:                         "Should return oldest deployment when multiple match",
			gtp:                          &e2eGtp,
			updatedGTP: 				  &prfGtp,
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
			gtpCache.mutex = &sync.Mutex{}
			handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache

			//run the method under test then make assertions
			handler.Added(c.gtp)
			handler.Updated(c.updatedGTP)
			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv), c.expectedIdentityCacheValue, ignoreUnexported) {
				t.Fatalf("GTP Mismatch. Diff: %v", cmp.Diff(c.expectedIdentityCacheValue, handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.expectedIdentity, c.expectedEnv), ignoreUnexported))
			}
			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetDeployment(c.gtp.Name),  c.expectedDeploymentCacheValue, ignoreUnexported) {
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

//todo figure out why the fake CRD client is unable to list k8s resources
//func TestDeploymentHandler(t *testing.T) {
//
//	p := common.AdmiralParams{
//		KubeconfigPath: "testdata/fake.config",
//	}
//
//	registry, _ := InitAdmiral(context.Background(), p)
//
//	handler := DeploymentHandler{}
//
//	gtpCache := &globalTrafficCache{}
//	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
//	gtpCache.dependencyCache = make(map[string]*v12.Deployment)
//	gtpCache.mutex = &sync.Mutex{}
//
//	fakeCrdClient := admiralFake.NewSimpleClientset()
//
//	//setting up fake resources
//	e2eGtp := v1.GlobalTrafficPolicy{}
//	e2eGtp.Labels = map[string]string{"identity": "app1", "env":"e2e"}
//	e2eGtp.Namespace = "namespace"
//	e2eGtp.Name = "myGTP1"
//
//	e2eGtpExtraLabel := v1.GlobalTrafficPolicy{}
//	e2eGtpExtraLabel.Labels = map[string]string{"identity": "app1", "env":"e2e", "random": "foobar"}
//	e2eGtpExtraLabel.Namespace = "namespace"
//	e2eGtpExtraLabel.Name = "myGTP2"
//
//	noMatchGtp := v1.GlobalTrafficPolicy{}
//	noMatchGtp.Labels = map[string]string{"identity": "app2", "env":"e2e"}
//	noMatchGtp.Namespace = "namespace"
//	noMatchGtp.Name = "myGTP3"
//
//	prfGtp := v1.GlobalTrafficPolicy{}
//	prfGtp.Labels = map[string]string{"identity": "app1", "env":"prf"}
//	prfGtp.Namespace = "namespace"
//	prfGtp.Name = "myGTP4"
//
//	_, err := fakeCrdClient.AdmiralV1().GlobalTrafficPolicies("namespace").Create(&e2eGtpExtraLabel)
//	_, err = fakeCrdClient.AdmiralV1().GlobalTrafficPolicies("namespace").Create(&e2eGtp)
//	_, err = fakeCrdClient.AdmiralV1().GlobalTrafficPolicies("namespace").Create(&noMatchGtp)
//	_, err = fakeCrdClient.AdmiralV1().GlobalTrafficPolicies("namespace").Create(&prfGtp)
//	if err != nil {
//		log.Fatalf("Failed to set up mock CRD client. Failing test. Error=%v", err)
//	}
//
//	gtpController := &admiral.GlobalTrafficController{CrdClient:fakeCrdClient}
//	remoteController, _ := createMockRemoteController(func(i interface{}) {
//
//	})
//	remoteController.GlobalTraffic = gtpController
//
//	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}
//
//	registry.AdmiralCache.GlobalTrafficCache = gtpCache
//	handler.RemoteRegistry = registry
//
//	deployment := v12.Deployment{
//		ObjectMeta: time2.ObjectMeta{
//			Name:      "test",
//			Namespace: "namespace",
//			Labels: map[string]string{"identity": "app1"},
//		},
//		Spec: v12.DeploymentSpec{
//			Selector: &time2.LabelSelector{
//				MatchLabels: map[string]string{"identity": "bar"},
//			},
//			Template: v13.PodTemplateSpec{
//				ObjectMeta: time2.ObjectMeta{
//					Labels: map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
//				},
//			},
//		},
//	}
//
//
//	//Struct of test case info. Name is required.
//	testCases := []struct {
//		name string
//		addedDeployment *v12.Deployment
//		expectedDeploymentCacheKey string
//		expectedIdentityCacheValue   *v1.GlobalTrafficPolicy
//		expectedDeploymentCacheValue *v12.Deployment
//
//	}{
//		{
//			name: "Should populate cache successfully on matching deployment",
//			addedDeployment: &deployment,
//			expectedDeploymentCacheKey: "myGTP1",
//			expectedIdentityCacheValue: &e2eGtp,
//			expectedDeploymentCacheValue: &deployment,
//
//		},
//	}
//
//	//Run the test for every provided case
//	for _, c := range testCases {
//		t.Run(c.name, func(t *testing.T) {
//			gtpCache = &globalTrafficCache{}
//			gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
//			gtpCache.dependencyCache = make(map[string]*v12.Deployment)
//			gtpCache.mutex = &sync.Mutex{}
//			handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache
//
//			handler.Added(&deployment)
//			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.addedDeployment.Spec.Template.Labels["identity"], c.addedDeployment.Spec.Template.Labels["env"]), c.expectedIdentityCacheValue, ignoreUnexported) {
//				t.Fatalf("GTP Mismatch. Diff: %v", cmp.Diff(c.expectedIdentityCacheValue, handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.addedDeployment.Spec.Template.Labels["identity"], c.addedDeployment.Spec.Template.Labels["env"]), ignoreUnexported))
//			}
//			if !cmp.Equal(handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetDeployment(c.expectedDeploymentCacheKey),  c.expectedDeploymentCacheValue, ignoreUnexported) {
//				t.Fatalf("Deployment Mismatch. Diff: %v", cmp.Diff(c.expectedDeploymentCacheValue, handler.RemoteRegistry.AdmiralCache.GlobalTrafficCache.GetDeployment(c.expectedDeploymentCacheKey), ignoreUnexported))
//			}
//
//			handler.Deleted(&deployment)
//		})
//	}
//}
