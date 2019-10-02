package clusters

import (
	"github.com/admiral/admiral/pkg/controller/istio"
	networking "istio.io/api/networking/v1alpha3"
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
		IstioConfigStore: &MockIstioConfigStore{},
	}

	new := istio.Config{}

	existing := istio.Config{}

	addUpdateIstioResource(&rc, new, nil, "VirtualService", "default")

	addUpdateIstioResource(&rc, new, &existing, "VirtualService", "default")
}

type MockIstioConfigStore struct {
}

func (m *MockIstioConfigStore) RegisterEventHandler(typ string, handler func(istio.Config, istio.Event)) {

}
func (m *MockIstioConfigStore) HasSynced() bool {

	return false
}
func (m *MockIstioConfigStore) Run(stop <-chan struct{}) {

}
func (m *MockIstioConfigStore) ConfigDescriptor() istio.ConfigDescriptor {
	return nil
}
func (m *MockIstioConfigStore) Get(typ, name, namespace string) *istio.Config {
	return nil
}

func (m *MockIstioConfigStore) List(typ, namespace string) ([]istio.Config, error) {
	return nil, nil
}

func (m *MockIstioConfigStore) Create(config istio.Config) (revision string, err error) {
	return "", nil
}
func (m *MockIstioConfigStore) Update(config istio.Config) (newRevision string, err error) {
	return "", nil
}
func (m *MockIstioConfigStore) Delete(typ, name, namespace string) error {

	return nil
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
