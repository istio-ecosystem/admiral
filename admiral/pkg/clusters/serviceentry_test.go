package clusters

import (
	"context"
	"errors"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"gopkg.in/yaml.v2"
	"k8s.io/api/core/v1"
	"reflect"
	"testing"

	networking "istio.io/api/networking/v1alpha3"
	"strconv"
)

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

	cacheWithNoEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"test-se": "1.2.3.4"},
		Addresses: []string{"1.2.3.4"},
	}

	emptyCacheController := test.FakeConfigMapController{
		GetError: nil,
		PutError: nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithNoEntry),
	}


	res := createSeWithDrLabels(nil, false, "", "test-se", &se, &des, &cacheWithNoEntry, &emptyCacheController)

	if res == nil {
		t.Fail()
	}

	newSe := res["test-se"]

	value := newSe.Endpoints[0].Labels["foo"]

	if value != "bar" {
		t.Fail()
	}

	if newSe.Addresses[0] != "1.2.3.4" {
		t.Errorf("Address set incorrectly from cache, expected 1.2.3.4, got %v", newSe.Addresses[0])
	}
}

//todo make this pass
//func TestAddServiceEntriesWithDr(t *testing.T) {
//	admiralCache := AdmiralCache{}
//
//	se := networking.ServiceEntry{
//		Hosts: []string{"test.com"},
//		Endpoints: []*networking.ServiceEntry_Endpoint{
//			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{}, Network: "mesh1", Locality: "us-west", Weight: 100},
//		},
//	}
//
//	rc, _ := createMockRemoteController(func(i interface{}) {
//		res := i.(istio.Config)
//		se, ok := res.Spec.(*networking.ServiceEntry)
//		if ok {
//			if se.Hosts[0] != "dev.bar.global" {
//				t.Fail()
//			}
//		}
//	})
//
//	seConfig, _ := createIstioConfig(istio.ServiceEntryProto, &se, "se1", "admiral-sync")
//	_, err := rc.IstioConfigStore.Create(*seConfig)
//	if err != nil {
//		log.Infof("%v", err)
//	}
//
//	AddServiceEntriesWithDr(&admiralCache, map[string]string{"cl1":"cl1"}, map[string]*RemoteController{"cl1":rc}, map[string]*networking.ServiceEntry{"se1": &se}, "admiral-sync")
//}

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

func TestGetLocalAddressForSe(t *testing.T) {
	t.Parallel()
	cacheWithEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.a.mesh": common.LocalAddressPrefix + ".10.1"},
		Addresses: []string{common.LocalAddressPrefix + ".10.1"},
	}
	cacheWithNoEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses: []string{},
	}
	cacheWith255Entries := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses: []string{},
	}

	for i := 1; i <= 255; i++ {
		address :=  common.LocalAddressPrefix + ".10." + strconv.Itoa(i)
		cacheWith255Entries.EntryAddresses[strconv.Itoa(i) + ".mesh"] = address
		cacheWith255Entries.Addresses = append(cacheWith255Entries.Addresses, address)
	}

	emptyCacheController := test.FakeConfigMapController{
		GetError: nil,
		PutError: nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithNoEntry),
	}

	cacheController := test.FakeConfigMapController{
		GetError: nil,
		PutError: nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry),
	}

	cacheControllerWith255Entries := test.FakeConfigMapController{
		GetError: nil,
		PutError: nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWith255Entries),
	}

	cacheControllerGetError := test.FakeConfigMapController{
		GetError: errors.New("BAD THINGS HAPPENED"),
		PutError: nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry),
	}

	cacheControllerPutError := test.FakeConfigMapController{
		PutError: errors.New("BAD THINGS HAPPENED"),
		GetError: nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry),
	}


	testCases := []struct {
		name   string
		seName   string
		seAddressCache  ServiceEntryAddressStore
		wantAddess string
		cacheController admiral.ConfigMapControllerInterface
		expectedCacheUpdate bool
		wantedError error
	}{
		{
			name: "should return new available address",
			seName: "e2e.a.mesh",
			seAddressCache: cacheWithNoEntry,
			wantAddess: common.LocalAddressPrefix + ".10.1",
			cacheController: &emptyCacheController,
			expectedCacheUpdate: true,
			wantedError: nil,
		},
		{
			name: "should return address from map",
			seName: "e2e.a.mesh",
			seAddressCache: cacheWithEntry,
			wantAddess: common.LocalAddressPrefix + ".10.1",
			cacheController: &cacheController,
			expectedCacheUpdate: false,
			wantedError: nil,
		},
		{
			name: "should return new available address",
			seName: "e2e.b.mesh",
			seAddressCache: cacheWithEntry,
			wantAddess: common.LocalAddressPrefix + ".10.2",
			cacheController: &cacheController,
			expectedCacheUpdate: true,
			wantedError: nil,
		},
		{
			name: "should return new available address in higher subnet",
			seName: "e2e.a.mesh",
			seAddressCache: cacheWith255Entries,
			wantAddess: common.LocalAddressPrefix + ".11.1",
			cacheController: &cacheControllerWith255Entries,
			expectedCacheUpdate: true,
			wantedError: nil,
		},
		{
			name: "should gracefully propagate get error",
			seName: "e2e.a.mesh",
			seAddressCache: cacheWith255Entries,
			wantAddess: "",
			cacheController: &cacheControllerGetError,
			expectedCacheUpdate: true,
			wantedError: errors.New("BAD THINGS HAPPENED"),
		},
		{
			name: "Should not return address on put error",
			seName: "e2e.abcdefghijklmnop.mesh",
			seAddressCache: cacheWith255Entries,
			wantAddess: "",
			cacheController: &cacheControllerPutError,
			expectedCacheUpdate: true,
			wantedError: errors.New("BAD THINGS HAPPENED"),
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			seAddress, needsCacheUpdate, err := GetLocalAddressForSe(c.seName, &c.seAddressCache, c.cacheController)
			if c.wantAddess != "" {
				if !reflect.DeepEqual(seAddress, c.wantAddess) {
					t.Errorf("Wanted se address: %s, got: %s", c.wantAddess, seAddress)
				}
				if err==nil && c.wantedError==nil {
					//we're fine
				} else if err.Error() != c.wantedError.Error() {
					t.Errorf("Error mismatch. Expected %v but got %v", c.wantedError, err)
				}
				if needsCacheUpdate != c.expectedCacheUpdate {
					t.Errorf("Expected %v, got %v for needs cache update", c.expectedCacheUpdate, needsCacheUpdate)
				}
			} else {
				if seAddress != "" {
					t.Errorf("Unexpectedly found address: %s", seAddress)
				}
			}
		})
	}

}

func buildFakeConfigMapFromAddressStore(addressStore *ServiceEntryAddressStore) *v1.ConfigMap{
	bytes,_ := yaml.Marshal(addressStore)

	cm := v1.ConfigMap{
		Data: map[string]string{"serviceEntryAddressStore": string(bytes)},
	}
	cm.Name="se-address-configmap"
	cm.Namespace="admiral-remote-ctx"
	cm.ResourceVersion="1234"
	return &cm
}
