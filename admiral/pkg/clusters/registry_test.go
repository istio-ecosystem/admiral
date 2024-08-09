package clusters

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	depModel "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sCoreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var registryTestSingleton sync.Once

func admiralParamsForRegistryTests() common.AdmiralParams {
	return common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			AdmiralCRDIdentityLabel: "identity",
			PriorityKey:             "priority",
			EnvKey:                  "admiral.io/env",
		},
		KubeconfigPath:                "testdata/fake.config",
		EnableSAN:                     true,
		SANPrefix:                     "prefix",
		HostnameSuffix:                "mesh",
		SyncNamespace:                 "ns",
		CacheReconcileDuration:        1 * time.Minute,
		SeAndDrCacheReconcileDuration: 1 * time.Minute,
		ClusterRegistriesNamespace:    "default",
		DependenciesNamespace:         "default",
		WorkloadSidecarUpdate:         "enabled",
		WorkloadSidecarName:           "default",
		EnableRoutingPolicy:           true,
		EnvoyFilterVersion:            "1.13",
		Profile:                       common.AdmiralProfileDefault,
	}
}

func setupForRegistryTests() {
	registryTestSingleton.Do(func() {
		common.ResetSync()
		common.InitializeConfig(admiralParamsForRegistryTests())
	})
}

func TestDeleteCacheControllerThatDoesntExist(t *testing.T) {
	setupForRegistryTests()
	w := NewRemoteRegistry(nil, common.AdmiralParams{})
	err := w.deleteCacheController("I don't exit")
	if err != nil {
		t.Fail()
	}
}

func TestDeleteCacheController(t *testing.T) {
	setupForRegistryTests()
	w := NewRemoteRegistry(context.TODO(), common.AdmiralParams{})
	r := rest.Config{
		Host: "test.com",
	}
	cluster := "test.cluster"
	w.createCacheController(&r, cluster, util.ResyncIntervals{UniversalReconcileInterval: 300 * time.Second, SeAndDrReconcileInterval: 300 * time.Second})
	rc := w.GetRemoteController(cluster)

	if rc == nil {
		t.Fail()
	}

	err := w.deleteCacheController(cluster)

	if err != nil {
		t.Fail()
	}
	rc = w.GetRemoteController(cluster)

	if rc != nil {
		t.Fail()
	}
}

func TestCopyServiceEntry(t *testing.T) {
	setupForRegistryTests()
	se := networking.ServiceEntry{
		Hosts: []string{"test.com"},
	}

	r := copyServiceEntry(&se)

	if r.Hosts[0] != "test.com" {
		t.Fail()
	}
}

func TestCopyEndpoint(t *testing.T) {
	setupForRegistryTests()
	se := networking.WorkloadEntry{
		Address: "127.0.0.1",
	}

	r := copyEndpoint(&se)

	if r.Address != "127.0.0.1" {
		t.Fail()
	}

}

func TestCopySidecar(t *testing.T) {
	setupForRegistryTests()
	spec := networking.Sidecar{
		WorkloadSelector: &networking.WorkloadSelector{
			Labels: map[string]string{"TestLabel": "TestValue"},
		},
	}

	//nolint
	sidecar := v1alpha3.Sidecar{Spec: spec}

	newSidecar := copySidecar(&sidecar)

	if newSidecar.Spec.WorkloadSelector != spec.WorkloadSelector {
		t.Fail()
	}
}

func createMockRemoteController(f func(interface{})) (*RemoteController, error) {
	config := rest.Config{
		Host: "localhost",
	}
	stop := make(chan struct{})
	d, err := admiral.NewDeploymentController(stop, &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		return nil, err
	}
	s, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		return nil, err
	}
	n, err := admiral.NewNodeController(stop, &test.MockNodeHandler{}, &config, loader.GetFakeClientLoader())
	if err != nil {
		return nil, err
	}
	r, err := admiral.NewRolloutsController(stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		return nil, err
	}
	rpc, err := admiral.NewRoutingPoliciesController(stop, &test.MockRoutingPolicyHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		return nil, err
	}

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
			Template: k8sCoreV1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"sidecar.istio.io/inject": "true"},
					Labels:      map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
				},
			},
		},
	}
	ctx := context.Background()
	d.Added(ctx, &deployment)
	service := k8sCoreV1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: k8sCoreV1.ServiceSpec{
			Selector: map[string]string{"identity": "bar"},
			Ports: []k8sCoreV1.ServicePort{
				{Name: "http", Port: 8080},
			},
		},
	}
	s.Added(ctx, &service)

	rc := RemoteController{
		DeploymentController:    d,
		ServiceController:       s,
		NodeController:          n,
		ClusterID:               "test.cluster",
		RolloutController:       r,
		RoutingPolicyController: rpc,
	}
	return &rc, nil
}

func TestCreateSecretController(t *testing.T) {
	setupForRegistryTests()
	err := createSecretController(context.Background(), NewRemoteRegistry(nil, common.AdmiralParams{}))
	if err != nil {
		t.Fail()
	}

	common.SetKubeconfigPath("fail")

	err = createSecretController(context.Background(), NewRemoteRegistry(context.TODO(), common.AdmiralParams{}))

	common.SetKubeconfigPath("testdata/fake.config")

	if err == nil {
		t.Fail()
	}
}

func TestInitAdmiral(t *testing.T) {
	setupForRegistryTests()
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet:       &common.LabelSet{},
	}
	p.LabelSet.WorkloadIdentityKey = "overridden-key"
	rr, err := InitAdmiral(context.Background(), p)

	if err != nil {
		t.Fail()
	}
	if len(rr.GetClusterIds()) != 0 {
		t.Fail()
	}

	if common.GetWorkloadIdentifier() != "identity" {
		t.Errorf("Workload identity label override failed. Expected \"identity\", got %v", common.GetWorkloadIdentifier())
	}
}

func TestAdded(t *testing.T) {
	setupForRegistryTests()
	ctx := context.Background()
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}
	rr, _ := InitAdmiral(context.Background(), p)

	rc, _ := createMockRemoteController(func(i interface{}) {
		t.Fail()
	})
	rr.PutRemoteController("test.cluster", rc)
	d, e := admiral.NewDependencyController(make(chan struct{}), &test.MockDependencyHandler{}, p.KubeconfigPath, "dep-ns", time.Second*time.Duration(300), loader.GetFakeClientLoader())

	if e != nil {
		t.Fail()
	}

	dh := DependencyHandler{
		RemoteRegistry:              rr,
		DepController:               d,
		DestinationServiceProcessor: &MockDestinationServiceProcessor{},
	}

	depData := v1.Dependency{
		Spec: depModel.Dependency{
			IdentityLabel: "idenity",
			Destinations:  []string{"one", "two"},
			Source:        "bar",
		},
	}

	dh.Added(ctx, &depData)
	dh.Deleted(ctx, &depData)

}

func TestGetServiceForDeployment(t *testing.T) {
	setupForRegistryTests()
	baseRc, _ := createMockRemoteController(func(i interface{}) {
	})

	rcWithService, _ := createMockRemoteController(func(i interface{}) {
	})

	service := k8sCoreV1.Service{}
	service.Namespace = "under-test"
	service.Spec.Ports = []k8sCoreV1.ServicePort{
		{
			Name: "port1",
			Port: 8090,
		},
	}
	service.Spec.Selector = map[string]string{"under-test": "true"}
	rcWithService.ServiceController.Cache.Put(&service)

	deploymentWithNoSelector := k8sAppsV1.Deployment{}
	deploymentWithNoSelector.Name = "dep1"
	deploymentWithNoSelector.Namespace = "under-test"
	deploymentWithNoSelector.Spec.Selector = &metav1.LabelSelector{}

	deploymentWithSelector := k8sAppsV1.Deployment{}
	deploymentWithSelector.Name = "dep2"
	deploymentWithSelector.Namespace = "under-test"
	deploymentWithSelector.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"under-test": "true"}}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name            string
		controller      *RemoteController
		deployment      *k8sAppsV1.Deployment
		expectedService *k8sCoreV1.Service
	}{
		{
			name:            "Should return nil with nothing in the cache",
			controller:      baseRc,
			deployment:      nil,
			expectedService: nil,
		},
		{
			name:            "Should not match if selectors don't match",
			controller:      rcWithService,
			deployment:      &deploymentWithNoSelector,
			expectedService: nil,
		},
		{
			name:            "Should return proper service",
			controller:      rcWithService,
			deployment:      &deploymentWithSelector,
			expectedService: &service,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			resultingService, _ := getServiceForDeployment(c.controller, c.deployment)
			if resultingService == nil && c.expectedService == nil {
				//perfect
			} else {
				if !cmp.Equal(resultingService, c.expectedService) {
					logrus.Infof("Service diff: %v", cmp.Diff(resultingService, c.expectedService))
					t.Errorf("Service mismatch. Got %v, expected %v", resultingService, c.expectedService)
				}
			}
		})
	}
}

func TestUpdateCacheController(t *testing.T) {
	setupForRegistryTests()
	p := common.AdmiralParams{
		KubeconfigPath:                "testdata/fake.config",
		CacheReconcileDuration:        300 * time.Second,
		SeAndDrCacheReconcileDuration: 150 * time.Second,
	}
	originalConfig, err := clientcmd.BuildConfigFromFlags("", "testdata/fake.config")
	if err != nil {
		t.Fatalf("unexpected error when building client with testdata/fake.config, err: %v", err)
	}
	changedConfig, err := clientcmd.BuildConfigFromFlags("", "testdata/fake_2.config")
	if err != nil {
		t.Fatalf("Unexpected error getting client %v", err)
	}

	rr, _ := InitAdmiral(context.Background(), p)

	rc, _ := createMockRemoteController(func(i interface{}) {
		t.Fail()
	})
	rc.stop = make(chan struct{})
	rr.PutRemoteController("test.cluster", rc)

	//Struct of test case info. Name is required.
	testCases := []struct {
		name          string
		oldConfig     *rest.Config
		newConfig     *rest.Config
		clusterId     string
		shouldRefresh bool
	}{
		{
			name:          "Should update controller when kubeconfig changes",
			oldConfig:     originalConfig,
			newConfig:     changedConfig,
			clusterId:     "test.cluster",
			shouldRefresh: true,
		},
		{
			name:          "Should not update controller when kubeconfig doesn't change",
			oldConfig:     originalConfig,
			newConfig:     originalConfig,
			clusterId:     "test.cluster",
			shouldRefresh: false,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			rr.GetRemoteController(c.clusterId).ApiServer = c.oldConfig.Host
			d, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, c.oldConfig, time.Second*time.Duration(300), loader.GetFakeClientLoader())
			if err != nil {
				t.Fatalf("Unexpected error creating controller %v", err)
			}
			rc.DeploymentController = d

			err = rr.updateCacheController(c.newConfig, c.clusterId, common.GetResyncIntervals())
			if err != nil {
				t.Fatalf("Unexpected error doing update %v", err)
			}

			if rr.GetRemoteController(c.clusterId).ApiServer != c.newConfig.Host {
				t.Fatalf("Client mismatch. Updated controller has the wrong client. Expected %v got %v", c.newConfig.Host, rr.GetRemoteController(c.clusterId).ApiServer)
			}

			refreshed := checkIfLogged(hook.AllEntries(), "Client mismatch, recreating cache controllers for cluster")

			if refreshed != c.shouldRefresh {
				t.Fatalf("Refresh mismatch. Expected %v got %v", c.shouldRefresh, refreshed)
			}
		})
	}
}

func checkIfLogged(entries []*logrus.Entry, phrase string) bool {
	for _, entry := range entries {
		if strings.Contains(entry.Message, phrase) {
			return true
		}
	}
	return false
}

func TestInitAdmiralHA(t *testing.T) {
	var (
		ctx                 = context.TODO()
		dummyKubeConfig     = "./testdata/fake.config"
		dependencyNamespace = "dependency-ns"
	)
	testCases := []struct {
		name        string
		params      common.AdmiralParams
		assertFunc  func(rr *RemoteRegistry, t *testing.T)
		expectedErr error
	}{
		{
			name: "Given Admiral is running in HA mode for database builder, " +
				"When InitAdmiralHA is invoked with correct parameters, " +
				"Then, it should return RemoteRegistry with 3 controllers - DependencyController, " +
				"DeploymentController, and RolloutController",
			params: common.AdmiralParams{
				HAMode:                common.HAController,
				KubeconfigPath:        dummyKubeConfig,
				DependenciesNamespace: dependencyNamespace,
			},
			assertFunc: func(rr *RemoteRegistry, t *testing.T) {
				if rr == nil {
					t.Error("expected RemoteRegistry to be initialized, but got nil")
				}
				// check if it has DependencyController initialized
				if rr != nil && rr.DependencyController == nil {
					t.Error("expected DependencyController to be initialized, but it was not")
				}
			},
			expectedErr: nil,
		},
		{
			name: "Given Admiral is running in HA mode for database builder, " +
				"When InitAdmiralHA is invoked with invalid HAMode parameter, " +
				"Then InitAdmiralHA should return an expected error",
			params: common.AdmiralParams{
				KubeconfigPath:        dummyKubeConfig,
				DependenciesNamespace: dependencyNamespace,
			},
			assertFunc: func(rr *RemoteRegistry, t *testing.T) {
				if rr != nil {
					t.Error("expected RemoteRegistry to be uninitialized")
				}
			},
			expectedErr: fmt.Errorf("admiral HA only supports %s mode", common.HAController),
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			common.ResetSync()
			rr, err := InitAdmiralHA(ctx, c.params)
			if c.expectedErr == nil && err != nil {
				t.Errorf("expected: nil, got: %v", err)
			}
			if c.expectedErr != nil {
				if err == nil {
					t.Errorf("expected: %v, got: %v", c.expectedErr, err)
				}
				if err != nil && c.expectedErr.Error() != err.Error() {
					t.Errorf("expected: %v, got: %v", c.expectedErr, err)
				}
			}
			c.assertFunc(rr, t)
		})
	}
}
