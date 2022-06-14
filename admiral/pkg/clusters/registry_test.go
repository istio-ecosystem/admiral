package clusters

import (
	"context"
	"github.com/google/go-cmp/cmp"
	depModel "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sCoreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"strings"
	"testing"
	"time"
)

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
		WorkloadSidecarUpdate:      "enabled",
		WorkloadSidecarName:        "default",
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.GlobalTrafficDeploymentLabel = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"

	common.InitializeConfig(p)
}

func TestDeleteCacheControllerThatDoesntExist(t *testing.T) {

	w := RemoteRegistry{
		RemoteControllers: make(map[string]*RemoteController),
	}

	err := w.deleteCacheController("I don't exit")

	if err != nil {
		t.Fail()
	}
}

func TestDeleteCacheController(t *testing.T) {

	w := RemoteRegistry{
		RemoteControllers: make(map[string]*RemoteController),
	}

	r := rest.Config{
		Host: "test.com",
	}

	cluster := "test.cluster"
	w.createCacheController(&r, cluster, time.Second*time.Duration(300))
	_, ok := w.RemoteControllers[cluster]

	if !ok {
		t.Fail()
	}

	err := w.deleteCacheController(cluster)

	if err != nil {
		t.Fail()
	}
	_, ok = w.RemoteControllers[cluster]

	if ok {
		t.Fail()
	}
}

func TestCopyServiceEntry(t *testing.T) {

	se := networking.ServiceEntry{
		Hosts: []string{"test.com"},
	}

	r := copyServiceEntry(&se)

	if r.Hosts[0] != "test.com" {
		t.Fail()
	}
}

func TestCopyEndpoint(t *testing.T) {

	se := networking.ServiceEntry_Endpoint{
		Address: "127.0.0.1",
	}

	r := copyEndpoint(&se)

	if r.Address != "127.0.0.1" {
		t.Fail()
	}

}

func TestCopySidecar(t *testing.T) {
	spec := networking.Sidecar{
		WorkloadSelector: &networking.WorkloadSelector{
			Labels: map[string]string{"TestLabel": "TestValue"},
		},
	}

	sidecar := v1alpha3.Sidecar{Spec: spec}

	newSidecar := copySidecar(&sidecar)

	if newSidecar.Spec.WorkloadSelector != spec.WorkloadSelector {
		t.Fail()
	}
}

func TestCreateDestinationRuleForLocalNoDeployLabel(t *testing.T) {

	config := rest.Config{
		Host: "localhost",
	}

	d, e := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300))

	if e != nil {
		t.Fail()
	}

	rc := RemoteController{
		DeploymentController: d,
	}

	des := networking.DestinationRule{
		Host: "test.com",
		Subsets: []*networking.Subset{
			{Name: "subset1", Labels: map[string]string{"foo": "bar"}, TrafficPolicy: nil},
		},
	}

	createDestinationRuleForLocal(&rc, "local.name", "identity", "cluster1", &des)

}

func TestCreateDestinationRuleForLocal(t *testing.T) {

	rc, err := createMockRemoteController(
		func(i interface{}) {

		},
	)

	if err != nil {
		t.Fail()
	}
	des := networking.DestinationRule{
		Host: "dev.bar.global",
		Subsets: []*networking.Subset{
			{Name: "subset1", Labels: map[string]string{"foo": "bar"}, TrafficPolicy: nil},
		},
	}

	createDestinationRuleForLocal(rc, "local.name", "bar", "cluster1", &des)

}

func createMockRemoteController(f func(interface{})) (*RemoteController, error) {
	config := rest.Config{
		Host: "localhost",
	}
	stop := make(chan struct{})
	d, e := admiral.NewDeploymentController(stop, &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300))
	s, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))
	n, e := admiral.NewNodeController(stop, &test.MockNodeHandler{}, &config)
	r, e := admiral.NewRolloutsController(stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300))
	rpc, e := admiral.NewRoutingPoliciesController(stop, &test.MockRoutingPolicyHandler{}, &config, time.Second*time.Duration(300))

	if e != nil {
		return nil, e
	}

	deployment := k8sAppsV1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: k8sAppsV1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"identity": "bar"},
			},
			Template: k8sCoreV1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
				},
			},
		},
	}
	d.Added(&deployment)
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
	s.Added(&service)

	rc := RemoteController{
		DeploymentController: d,
		ServiceController:    s,
		NodeController:       n,
		ClusterID:            "test.cluster",
		RolloutController:    r,
		RoutingPolicyController: rpc,
	}
	return &rc, nil
}

func TestCreateSecretController(t *testing.T) {
	rr := RemoteRegistry{}
	err := createSecretController(context.Background(), &rr)

	if err != nil {
		t.Fail()
	}

	common.SetKubeconfigPath("fail")

	rr = RemoteRegistry{}
	err = createSecretController(context.Background(), &rr)

	common.SetKubeconfigPath("testdata/fake.config")

	if err == nil {
		t.Fail()
	}
}

func TestInitAdmiral(t *testing.T) {

	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet:       &common.LabelSet{},
	}

	p.LabelSet.WorkloadIdentityKey = "overridden-key"

	rr, err := InitAdmiral(context.Background(), p)

	if err != nil {
		t.Fail()
	}
	if len(rr.RemoteControllers) != 0 {
		t.Fail()
	}

	if common.GetWorkloadIdentifier() != "identity" {
		t.Errorf("Workload identity label override failed. Expected \"identity\", got %v", common.GetWorkloadIdentifier())
	}
}

func TestAdded(t *testing.T) {

	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}
	rr, _ := InitAdmiral(context.Background(), p)

	rc, _ := createMockRemoteController(func(i interface{}) {
		t.Fail()
	})
	rr.RemoteControllers["test.cluster"] = rc
	d, e := admiral.NewDependencyController(make(chan struct{}), &test.MockDependencyHandler{}, p.KubeconfigPath, "dep-ns", time.Second*time.Duration(300))

	if e != nil {
		t.Fail()
	}

	dh := DependencyHandler{
		RemoteRegistry: rr,
		DepController:  d,
	}

	depData := v1.Dependency{
		Spec: depModel.Dependency{
			IdentityLabel: "idenity",
			Destinations:  []string{"one", "two"},
			Source:        "bar",
		},
	}

	dh.Added(&depData)
	dh.Deleted(&depData)

}

func TestGetServiceForDeployment(t *testing.T) {
	baseRc, _ := createMockRemoteController(func(i interface{}) {
		//res := i.(istio.Config)
		//se, ok := res.Spec.(*v1alpha3.ServiceEntry)
		//if ok {
		//	if se.Hosts[0] != "dev.bar.global" {
		//		t.Errorf("Host mismatch. Expected dev.bar.global, got %v", se.Hosts[0])
		//	}
		//}
	})

	rcWithService, _ := createMockRemoteController(func(i interface{}) {
		//res := i.(istio.Config)
		//se, ok := res.Spec.(*networking.ServiceEntry)
		//if ok {
		//	if se.Hosts[0] != "dev.bar.global" {
		//		t.Errorf("Host mismatch. Expected dev.bar.global, got %v", se.Hosts[0])
		//	}
		//}
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
			resultingService := getServiceForDeployment(c.controller, c.deployment)
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
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}
	originalConfig, err := clientcmd.BuildConfigFromFlags("", "testdata/fake.config")
	changedConfig, err := clientcmd.BuildConfigFromFlags("", "testdata/fake_2.config")
	if err != nil {
		t.Fatalf("Unexpected error getting client %v", err)
	}

	rr, _ := InitAdmiral(context.Background(), p)

	rc, _ := createMockRemoteController(func(i interface{}) {
		t.Fail()
	})
	rc.stop = make(chan struct{})
	rr.RemoteControllers["test.cluster"] = rc

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
			rr.RemoteControllers[c.clusterId].ApiServer = c.oldConfig.Host
			d, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, c.oldConfig, time.Second*time.Duration(300))
			if err != nil {
				t.Fatalf("Unexpected error creating controller %v", err)
			}
			rc.DeploymentController = d

			err = rr.updateCacheController(c.newConfig, c.clusterId, time.Second*time.Duration(300))
			if err != nil {
				t.Fatalf("Unexpected error doing update %v", err)
			}

			if rr.RemoteControllers[c.clusterId].ApiServer != c.newConfig.Host {
				t.Fatalf("Client mismatch. Updated controller has the wrong client. Expected %v got %v", c.newConfig.Host, rr.RemoteControllers[c.clusterId].ApiServer)
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
