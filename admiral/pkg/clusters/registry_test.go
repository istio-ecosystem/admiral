package clusters

import (
	"context"
	depModel "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	networking "istio.io/api/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sCoreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"testing"
	"time"
)

func TestDeleteCacheControllerThatDoesntExist(t *testing.T) {

	w := RemoteRegistry{
		remoteControllers: make(map[string]*RemoteController),
	}

	err := w.deleteCacheController("I don't exit")

	if err != nil {
		t.Fail()
	}
}

func TestDeleteCacheController(t *testing.T) {

	w := RemoteRegistry{
		remoteControllers: make(map[string]*RemoteController),
	}

	r := rest.Config{
		Host: "test.com",
	}

	cluster := "test.cluster"
	w.createCacheController(&r, cluster, time.Second*time.Duration(300))
	_, ok := w.remoteControllers[cluster]

	if !ok {
		t.Fail()
	}

	err := w.deleteCacheController(cluster)

	if err != nil {
		t.Fail()
	}
	_, ok = w.remoteControllers[cluster]

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

func TestCreateDestinationRuleForLocalNoDeployLabel(t *testing.T) {

	config := rest.Config{
		Host: "localhost",
	}

	d, e := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300), &common.LabelSet{})

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

	createDestinationRuleForLocal(&rc, "local.name", "identity", "cluster1", &des, "sync", ".global", "identity")

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

	createDestinationRuleForLocal(rc, "local.name", "bar", "cluster1", &des, "sync", ".global", "identity")

}

func createMockRemoteController(f func(interface{})) (*RemoteController, error) {
	config := rest.Config{
		Host: "localhost",
	}
	stop := make(chan struct{})
	d, e := admiral.NewDeploymentController(stop, &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300), &common.LabelSet{})
	s, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))
	n, e := admiral.NewNodeController(stop, &test.MockNodeHandler{}, &config)

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
	}
	return &rc, nil
}

func TestCreateSecretController(t *testing.T) {

	p := AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	rr := RemoteRegistry{}
	err := createSecretController(context.Background(), &rr, p)

	if err != nil {
		t.Fail()
	}

	p = AdmiralParams{
		KubeconfigPath: "fail",
	}

	rr = RemoteRegistry{}
	err = createSecretController(context.Background(), &rr, p)

	if err == nil {
		t.Fail()
	}
}

func TestInitAdmiral(t *testing.T) {

	p := AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	rr, err := InitAdmiral(context.Background(), p)

	if err != nil {
		t.Fail()
	}
	if len(rr.remoteControllers) != 0 {
		t.Fail()
	}
}

func TestAdded(t *testing.T) {

	p := AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}
	rr, _ := InitAdmiral(context.Background(), p)

	rc, _ := createMockRemoteController(func(i interface{}) {
		t.Fail()
	})
	rr.remoteControllers["test.cluster"] = rc
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

func TestMakeVirtualService(t *testing.T) {
	vs := makeVirtualService("test.local", "dest", 8080)
	if vs.Hosts[0] != "test.local" {
		t.Fail()
	}
	if vs.Http[0].Route[0].Destination.Host != "dest" {
		t.Fail()
	}
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
	service.Spec.Selector = map[string]string{"under-test":"true"}
	rcWithService.ServiceController.Cache.Put(&service)

	deploymentWithNoSelector := k8sAppsV1.Deployment{}
	deploymentWithNoSelector.Name = "dep1"
	deploymentWithNoSelector.Namespace ="under-test"
	deploymentWithNoSelector.Spec.Selector = &metav1.LabelSelector{}

	deploymentWithSelector := k8sAppsV1.Deployment{}
	deploymentWithSelector.Name = "dep2"
	deploymentWithSelector.Namespace = "under-test"
	deploymentWithSelector.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"under-test":"true"}}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name string
		controller *RemoteController
		deployment *k8sAppsV1.Deployment
		expectedService *k8sCoreV1.Service
	}{
		{
			name: "Should return nil with nothing in the cache",
			controller:baseRc,
			deployment:nil,
			expectedService:nil,
		},
		{
			name: "Should not match if selectors don't match",
			controller:rcWithService,
			deployment:&deploymentWithNoSelector,
			expectedService:nil,
		},
		{
			name: "Should return proper service",
			controller:rcWithService,
			deployment:&deploymentWithSelector,
			expectedService:&service,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			//resultingService := getServiceForDeployment(c.controller, c.deployment)
			//if resultingService == nil && c.expectedService == nil {
			//	//perfect
			//} else {
			//	if !cmp.Equal(resultingService, c.expectedService) {
			//		logrus.Infof("Service diff: %v", cmp.Diff(resultingService, c.expectedService))
			//		t.Errorf("Service mismatch. Got %v, expected %v",resultingService, c.expectedService)
			//	}
			//}
		})
	}
}
