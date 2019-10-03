package clusters

import (
	"github.com/admiral/admiral/pkg/controller/admiral"
	"github.com/admiral/admiral/pkg/controller/common"
	"github.com/admiral/admiral/pkg/controller/istio"
	"github.com/admiral/admiral/pkg/test"
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

	config := rest.Config{
		Host: "localhost",
	}

	d, e := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300))

	s, e := admiral.NewServiceController(make(chan struct{}), &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))

	if e != nil {
		t.Fail()
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

			TestHook: func(i interface{}) {
				res := i.(*k8sAppsV1.Deployment)
				if res.Name != "test" {
					t.Fail()
				}
			},
		},
		DeploymentController: d,
		ServiceController:    s,
	}

	des := networking.DestinationRule{
		Host: "dev.bar.global",
		Subsets: []*networking.Subset{
			{Name: "subset1", Labels: map[string]string{"foo": "bar"}, TrafficPolicy: nil},
		},
	}

	createDestinationRuleForLocal(&rc, "local.name", "bar", "cluster1", &des, "sync")

}
