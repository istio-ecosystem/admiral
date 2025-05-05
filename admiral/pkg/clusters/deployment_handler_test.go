package clusters

import (
	"bytes"
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"sync"
	"testing"
	"time"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	admiralFake "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned/fake"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"

	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var deploymentHandlerTestSingleton sync.Once

func admiralParamsForDeploymentHandlerTests() common.AdmiralParams {
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
		AdmiralStateSyncerMode:     true,
		AdmiralStateSyncerClusters: []string{"cluster-1"},
	}
}

func setupForDeploymentHandlerTests() {
	deploymentHandlerTestSingleton.Do(func() {
		common.ResetSync()
		common.InitializeConfig(admiralParamsForDeploymentHandlerTests())
	})
}

func TestDeploymentHandlerPartitionCache(t *testing.T) {
	setupForDeploymentHandlerTests()
	admiralParams := admiralParamsForDeploymentHandlerTests()
	ctx := context.Background()
	remoteRegistry, _ := InitAdmiral(ctx, admiralParams)
	remoteRegistry.AdmiralCache.PartitionIdentityCache = common.NewMap()
	partitionIdentifier := "admiral.io/identityPartition"
	clusterName := "test-k8s"

	testCases := []struct {
		name       string
		deployment appsV1.Deployment
		expected   string
	}{
		{
			name: "Given the deployment has the partition label, " +
				"Then the PartitionIdentityCache should contain an entry for that deployment",
			deployment: appsV1.Deployment{Spec: appsV1.DeploymentSpec{Template: coreV1.PodTemplateSpec{ObjectMeta: metaV1.ObjectMeta{Labels: map[string]string{partitionIdentifier: "sw1", "env": "stage", "identity": "services.gateway"}}}}},
			expected:   "services.gateway",
		},
		{
			name: "Given the deployment has the partition annotation, " +
				"Then the PartitionIdentityCache should contain an entry for that deployment",
			deployment: appsV1.Deployment{Spec: appsV1.DeploymentSpec{Template: coreV1.PodTemplateSpec{ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{partitionIdentifier: "sw1", "env": "stage", "identity": "services.gateway"}}}}},
			expected:   "services.gateway",
		},
		{
			name: "Given the deployment doesn't have the partition label or annotation, " +
				"Then the PartitionIdentityCache should not contain an entry for that deployment",
			deployment: appsV1.Deployment{Spec: appsV1.DeploymentSpec{Template: coreV1.PodTemplateSpec{ObjectMeta: metaV1.ObjectMeta{Labels: map[string]string{"identity": "services.gateway"}, Annotations: map[string]string{}}}}},
			expected:   "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			_ = HandleEventForDeployment(ctx, admiral.Add, &c.deployment, remoteRegistry, clusterName)
			iVal := ""
			if len(c.expected) > 0 {
				globalIdentifier := common.GetDeploymentGlobalIdentifier(&c.deployment)
				iVal = remoteRegistry.AdmiralCache.PartitionIdentityCache.Get(globalIdentifier)
			}
			if !(iVal == c.expected) {
				t.Errorf("Expected cache to contain: %s, got: %s", c.expected, iVal)
			}
		})
	}
}

func TestDeploymentHandler(t *testing.T) {
	setupForDeploymentHandlerTests()
	ctx := context.WithValue(context.Background(), "txId", "txidvalue")

	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	remoteRegistry, _ := InitAdmiral(context.Background(), p)

	handler := DeploymentHandler{}

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1.GlobalTrafficPolicy)
	gtpCache.mutex = &sync.Mutex{}

	fakeCrdClient := admiralFake.NewSimpleClientset()

	gtpController := &admiral.GlobalTrafficController{CrdClient: fakeCrdClient}
	remoteController, _ := createMockRemoteController(func(i interface{}) {

	})
	remoteController.GlobalTraffic = gtpController

	remoteRegistry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}

	remoteRegistry.AdmiralCache.GlobalTrafficCache = gtpCache
	handler.RemoteRegistry = remoteRegistry
	handler.ClusterID = "cluster-1"
	deployment := appsV1.Deployment{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test",
			Namespace: "namespace",
			Labels:    map[string]string{"identity": "app1"},
		},
		Spec: appsV1.DeploymentSpec{
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

	//Struct of test case info. Name is required.
	testCases := []struct {
		name                         string
		addedDeployment              *appsV1.Deployment
		expectedDeploymentCacheKey   string
		expectedIdentityCacheValue   *v1.GlobalTrafficPolicy
		expectedDeploymentCacheValue *appsV1.Deployment
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
			ns := handler.RemoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Get("bar").Get("cluster-1").GetValues()[0]
			if ns != "namespace" {
				t.Errorf("expected namespace: %v but got %v", "namespace", ns)
			}
			handler.Deleted(ctx, &deployment)
		})
	}
}

func TestCallRegistryForDeployment(t *testing.T) {
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
		Profile:                    common.AdmiralProfileDefault,
		AdmiralStateSyncerMode:     true,
		AdmiralStateSyncerClusters: []string{"test-k8s"},
	}
	common.ResetSync()
	common.InitializeConfig(p)
	remoteRegistry, _ := InitAdmiral(context.Background(), p)
	dummyRespBody := ioutil.NopCloser(bytes.NewBufferString("dummyRespBody"))
	validRegistryClient := registry.NewDefaultRegistryClient()
	validClient := test.MockClient{
		ExpectedPutResponse: &http.Response{
			StatusCode: 200,
			Body:       dummyRespBody,
		},
		ExpectedPutErr: nil,
		ExpectedConfig: &util.Config{Host: "host", BaseURI: "v1"},
	}
	validRegistryClient.Client = &validClient
	invalidRegistryClient := registry.NewDefaultRegistryClient()
	invalidClient := test.MockClient{
		ExpectedDeleteResponse: &http.Response{
			StatusCode: 404,
			Body:       dummyRespBody,
		},
		ExpectedDeleteErr: fmt.Errorf("failed private auth call"),
		ExpectedConfig:    &util.Config{Host: "host", BaseURI: "v1"},
	}
	invalidRegistryClient.Client = &invalidClient
	deploy := &appsV1.Deployment{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test",
			Namespace: "namespace",
			Labels:    map[string]string{"identity": "app1"},
		},
		Spec: appsV1.DeploymentSpec{
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
		name           string
		ctx            context.Context
		obj            *appsV1.Deployment
		registryClient *registry.RegistryClient
		event          admiral.EventType
		expectedError  error
	}{
		{
			name: "Given valid registry client " +
				"When calling for add event " +
				"Then error should be nil",
			obj:            deploy,
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: validRegistryClient,
			event:          admiral.Add,
			expectedError:  nil,
		},
		{
			name: "Given valid registry client " +
				"When calling for update event " +
				"Then error should be nil",
			obj:            deploy,
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: validRegistryClient,
			event:          admiral.Update,
			expectedError:  nil,
		},
		{
			name: "Given valid params to call registry func " +
				"When registry func returns an error " +
				"Then handler should receive an error",
			obj:            deploy,
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: invalidRegistryClient,
			event:          admiral.Delete,
			expectedError:  fmt.Errorf("op=Delete type=deployment name=test cluster=test-k8s message=failed to Delete deployment with err: failed private auth call"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			remoteRegistry.RegistryClient = tc.registryClient
			clusterName := "test-k8s"
			actualError := callRegistryForDeployment(tc.ctx, tc.event, remoteRegistry, "test.testId", clusterName, tc.obj)
			if tc.expectedError != nil {
				if actualError == nil {
					t.Fatalf("expected error %s but got nil", tc.expectedError.Error())
				}
				assert.Equal(t, tc.expectedError.Error(), actualError.Error())
			} else {
				if actualError != nil {
					t.Fatalf("expected error nil but got %s", actualError.Error())
				}
			}
		})
	}
}

type fakeHandleEventForDeployment struct {
	handleEventForDeploymentFunc func() HandleEventForDeploymentFunc
	calledDeploymentByNamespace  map[string]map[string]bool
}

func (f *fakeHandleEventForDeployment) CalledDeploymentForNamespace(name, namespace string) bool {
	if f.calledDeploymentByNamespace[namespace] != nil {
		return f.calledDeploymentByNamespace[namespace][name]
	}
	return false
}

func newFakeHandleEventForDeploymentsByError(errByDeployment map[string]map[string]error) *fakeHandleEventForDeployment {
	f := &fakeHandleEventForDeployment{
		calledDeploymentByNamespace: make(map[string]map[string]bool, 0),
	}
	f.handleEventForDeploymentFunc = func() HandleEventForDeploymentFunc {
		return func(
			ctx context.Context,
			event admiral.EventType,
			deployment *appsV1.Deployment,
			remoteRegistry *RemoteRegistry,
			clusterName string) error {
			if f.calledDeploymentByNamespace[deployment.Namespace] == nil {
				f.calledDeploymentByNamespace[deployment.Namespace] = map[string]bool{
					deployment.Name: true,
				}
			} else {
				f.calledDeploymentByNamespace[deployment.Namespace][deployment.Name] = true
			}
			return errByDeployment[deployment.Namespace][deployment.Name]
		}
	}
	return f
}

func newFakeDeployment(name, namespace string, matchLabels map[string]string) *appsV1.Deployment {
	return &appsV1.Deployment{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"identity": name},
		},
		Spec: appsV1.DeploymentSpec{
			Selector: &metaV1.LabelSelector{
				MatchLabels: matchLabels,
			},
			Template: coreV1.PodTemplateSpec{ObjectMeta: metaV1.ObjectMeta{Labels: map[string]string{"identity": name}}},
		},
	}
}
