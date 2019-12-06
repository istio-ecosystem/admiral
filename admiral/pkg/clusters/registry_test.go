package clusters

import (
	"context"
	"fmt"
	depModel "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"gotest.tools/assert"
	networking "istio.io/api/networking/v1alpha3"
	istioKube "istio.io/istio/pilot/pkg/serviceregistry/kube"
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

func TestAddUpdateIstioResource(t *testing.T) {

	rc := RemoteController{
		IstioConfigStore: &test.MockIstioConfigStore{},
	}

	new := istio.Config{}

	existing := istio.Config{}

	addUpdateIstioResource(&rc, new, nil, "VirtualService", "default")

	addUpdateIstioResource(&rc, new, &existing, "VirtualService", "default")
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

func TestCreateSeWithDrLabels(t *testing.T) {

	se := networking.ServiceEntry{
		Hosts: []string{"test.com"},
		Endpoints: []*networking.ServiceEntry_Endpoint{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	des := networking.DestinationRule{
		Host: "test.com",
		Subsets: []*networking.Subset{
			{Name: "subset1", Labels: map[string]string{"foo": "bar"}, TrafficPolicy: nil},
		},
	}

	address := common.NewMap()

	res := createSeWithDrLabels(nil, false, "", "test-se", &se, &des, address)

	if res == nil {
		t.Fail()
	}

	newSe := res["test-se"]

	value := newSe.Endpoints[0].Labels["foo"]

	if value != "bar" {
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
		IstioConfigStore: &test.MockIstioConfigStore{

			TestHook: func(i interface{}) {
				t.Fail()
			},
		},
		DeploymentController: d,
	}

	des := networking.DestinationRule{
		Host: "test.com",
		Subsets: []*networking.Subset{
			{Name: "subset1", Labels: map[string]string{"foo": "bar"}, TrafficPolicy: nil},
		},
	}

	createDestinationRuleForLocal(&rc, "local.name", "identity", "cluster1", &des, "sync")

}

func TestCreateDestinationRuleForLocal(t *testing.T) {

	rc, err := createMockRemoteController(
		func(i interface{}) {
			res := i.(istio.Config)
			if res.Name != "local.name" {
				t.Fail()
			}
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

	createDestinationRuleForLocal(rc, "local.name", "bar", "cluster1", &des, "sync")

}

func createMockRemoteController(f func(interface{})) (*RemoteController, error) {
	config := rest.Config{
		Host: "localhost",
	}
	stop := make(chan struct{})
	d, e := admiral.NewDeploymentController(stop, &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300))
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
		IstioConfigStore: &test.MockIstioConfigStore{
			TestHook: f,
		},
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

func TestCreateServiceEntryForNewServiceOrPod(t *testing.T) {

	p := AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}
	rr, _ := InitAdmiral(context.Background(), p)

	rc, _ := createMockRemoteController(func(i interface{}) {
		res := i.(istio.Config)
		se, ok := res.Spec.(*networking.ServiceEntry)
		if ok {
			if se.Hosts[0] != "dev.bar.global" {
				t.Fail()
			}
		}
	})

	rr.remoteControllers["test.cluster"] = rc
	createServiceEntryForNewServiceOrPod("test", "bar", rr, "sync")

}


func TestCreateServiceEntryUseClusterIP(t *testing.T) {
	p := AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}
	rr, _ := InitAdmiral(context.Background(), p)

	rc, _ := createMockRemoteController(func(i interface{}) {
		res := i.(istio.Config)
		se, ok := res.Spec.(*networking.ServiceEntry)
		if ok {
			if se.Hosts[0] != "dev.bar.global" {
				t.Fail()
			}
		}
	})

	rr.remoteControllers["test.cluster"] = rc
	rc.ServiceController.Cache.Get("test").Service["test"][0].Spec.ClusterIP = "1.2.3.4"

	serviceentries := createServiceEntryForNewServiceOrPod("test", "bar", rr, "sync")


	for key, value := range serviceentries{
		fmt.Println("Key:", key, "Value:", value.Addresses[0])
		assert.Equal(t, value.Addresses[0], "1.2.3.4", "Expected SE address to be equal to the cluster IP but it was not")
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

func TestCreateIstioController(t *testing.T) {

	p := AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}
	rr, _ := InitAdmiral(context.Background(), p)

	rc, _ := createMockRemoteController(func(i interface{}) {
		t.Fail()
	})
	rr.remoteControllers["test.cluster"] = rc
	config := rest.Config{
		Host: "localhost",
	}

	opts := istioKube.ControllerOptions{
		WatchedNamespace: metav1.NamespaceAll,
		ResyncPeriod:     time.Second * time.Duration(300),
		DomainSuffix:     ".cluster",
	}

	rr.createIstioController(&config, opts, rc, make(chan struct{}), "test.cluster")

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

func TestDeploymentHandler(t *testing.T) {

	p := AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	rr, _ := InitAdmiral(context.Background(), p)

	rc, _ := createMockRemoteController(func(i interface{}) {
		res := i.(istio.Config)
		se, ok := res.Spec.(*networking.ServiceEntry)
		if ok {
			if se.Hosts[0] != "dev.bar.global" {
				t.Fail()
			}
		}
	})
	rr.remoteControllers["test.cluster"] = rc

	dh := DeploymentHandler{
		RemoteRegistry: rr,
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

	dh.Added(&deployment)

	dh.Deleted(&deployment)
}

func TestPodHandler(t *testing.T) {

	p := AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	rr, _ := InitAdmiral(context.Background(), p)

	rc, _ := createMockRemoteController(func(i interface{}) {
		res := i.(istio.Config)
		se, ok := res.Spec.(*networking.ServiceEntry)
		if ok {
			if se.Hosts[0] != "dev.bar.global" {
				t.Fail()
			}
		}
	})
	rr.remoteControllers["test.cluster"] = rc

	ph := PodHandler{
		RemoteRegistry: rr,
	}

	pod := k8sCoreV1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: k8sCoreV1.PodSpec{
			Hostname: "test.local",
		},
	}

	ph.Added(&pod)

	ph.Deleted(&pod)
}
