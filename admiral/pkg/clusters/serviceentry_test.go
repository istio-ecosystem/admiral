package clusters

import (
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"gopkg.in/yaml.v2"
	istionetworkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	"k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

//func TestCreateSeWithDrLabels(t *testing.T) {
//
//	se := networking.ServiceEntry{
//		Hosts: []string{"test.com"},
//		Endpoints: []*networking.ServiceEntry_Endpoint{
//			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{}, Network: "mesh1", Locality: "us-west", Weight: 100},
//		},
//	}
//
//	des := networking.DestinationRule{
//		Host: "test.com",
//		Subsets: []*networking.Subset{
//			{Name: "subset1", Labels: map[string]string{"foo": "bar"}, TrafficPolicy: nil},
//		},
//	}
//
//	cacheWithNoEntry := ServiceEntryAddressStore{
//		EntryAddresses: map[string]string{"test-se": "1.2.3.4"},
//		Addresses: []string{"1.2.3.4"},
//	}
//
//	emptyCacheController := test.FakeConfigMapController{
//		GetError: nil,
//		PutError: nil,
//		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithNoEntry, "123"),
//	}
//
//
//	res := createSeWithDrLabels(nil, false, "", "test-se", &se, &des, &cacheWithNoEntry, &emptyCacheController)
//
//	if res == nil {
//		t.Fail()
//	}
//
//	newSe := res["test-se"]
//
//	value := newSe.Endpoints[0].Labels["foo"]
//
//	if value != "bar" {
//		t.Fail()
//	}
//
//	if newSe.Addresses[0] != "1.2.3.4" {
//		t.Errorf("Address set incorrectly from cache, expected 1.2.3.4, got %v", newSe.Addresses[0])
//	}
//}
//
//func TestAddServiceEntriesWithDr(t *testing.T) {
//	admiralCache := AdmiralCache{}
//
//	cnameIdentityCache := sync.Map{}
//	cnameIdentityCache.Store("dev.bar.global", "bar")
//	admiralCache.CnameIdentityCache = &cnameIdentityCache
//
//	se := networking.ServiceEntry{
//		Hosts: []string{"dev.bar.global"},
//		Endpoints: []*networking.ServiceEntry_Endpoint{
//			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{}, Network: "mesh1", Locality: "us-west", Weight: 100},
//		},
//	}
//
//	rc, _ := createMockRemoteController(func(i interface{}) {
//		//res := i.(istio.Config)
//		//se, ok := res.Spec.(*networking.ServiceEntry)
//		//if ok {
//		//	if se.Hosts[0] != "dev.bar.global" {
//		//		t.Errorf("Host mismatch. Expected dev.bar.global, got %v", se.Hosts[0])
//		//	}
//		//}
//	})
//
//	seConfig, _ := createIstioConfig(istio.ServiceEntryProto, &se, "se1", "admiral-sync")
//	_, err := rc.IstioConfigStore.Create(*seConfig)
//	if err != nil {
//		t.Errorf("%v", err)
//
//	}
//
//	AddServiceEntriesWithDr(&admiralCache, map[string]string{"cl1":"cl1"}, map[string]*RemoteController{"cl1":rc}, map[string]*networking.ServiceEntry{"se1": &se}, "admiral-sync")
//	}
//
//func TestCreateServiceEntryForNewServiceOrPod(t *testing.T) {
//
//	p := AdmiralParams{
//		KubeconfigPath: "testdata/fake.config",
//	}
//	rr, _ := InitAdmiral(context.Background(), p)
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
//	rr.remoteControllers["test.cluster"] = rc
//	createServiceEntryForNewServiceOrPod("test", "bar", rr)
//
//}
//
//func TestGetLocalAddressForSe(t *testing.T) {
//	t.Parallel()
//	cacheWithEntry := ServiceEntryAddressStore{
//		EntryAddresses: map[string]string{"e2e.a.mesh": common.LocalAddressPrefix + ".10.1"},
//		Addresses: []string{common.LocalAddressPrefix + ".10.1"},
//	}
//	cacheWithNoEntry := ServiceEntryAddressStore{
//		EntryAddresses: map[string]string{},
//		Addresses: []string{},
//	}
//	cacheWith255Entries := ServiceEntryAddressStore{
//		EntryAddresses: map[string]string{},
//		Addresses: []string{},
//	}
//
//	for i := 1; i <= 255; i++ {
//		address :=  common.LocalAddressPrefix + ".10." + strconv.Itoa(i)
//		cacheWith255Entries.EntryAddresses[strconv.Itoa(i) + ".mesh"] = address
//		cacheWith255Entries.Addresses = append(cacheWith255Entries.Addresses, address)
//	}
//
//	emptyCacheController := test.FakeConfigMapController{
//		GetError: nil,
//		PutError: nil,
//		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithNoEntry, "123"),
//	}
//
//	cacheController := test.FakeConfigMapController{
//		GetError: nil,
//		PutError: nil,
//		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
//	}
//
//	cacheControllerWith255Entries := test.FakeConfigMapController{
//		GetError: nil,
//		PutError: nil,
//		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWith255Entries, "123"),
//	}
//
//	cacheControllerGetError := test.FakeConfigMapController{
//		GetError: errors.New("BAD THINGS HAPPENED"),
//		PutError: nil,
//		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
//	}
//
//	cacheControllerPutError := test.FakeConfigMapController{
//		PutError: errors.New("BAD THINGS HAPPENED"),
//		GetError: nil,
//		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
//	}
//
//
//	testCases := []struct {
//		name   string
//		seName   string
//		seAddressCache  ServiceEntryAddressStore
//		wantAddess string
//		cacheController admiral.ConfigMapControllerInterface
//		expectedCacheUpdate bool
//		wantedError error
//	}{
//		{
//			name: "should return new available address",
//			seName: "e2e.a.mesh",
//			seAddressCache: cacheWithNoEntry,
//			wantAddess: common.LocalAddressPrefix + ".10.1",
//			cacheController: &emptyCacheController,
//			expectedCacheUpdate: true,
//			wantedError: nil,
//		},
//		{
//			name: "should return address from map",
//			seName: "e2e.a.mesh",
//			seAddressCache: cacheWithEntry,
//			wantAddess: common.LocalAddressPrefix + ".10.1",
//			cacheController: &cacheController,
//			expectedCacheUpdate: false,
//			wantedError: nil,
//		},
//		{
//			name: "should return new available address",
//			seName: "e2e.b.mesh",
//			seAddressCache: cacheWithEntry,
//			wantAddess: common.LocalAddressPrefix + ".10.2",
//			cacheController: &cacheController,
//			expectedCacheUpdate: true,
//			wantedError: nil,
//		},
//		{
//			name: "should return new available address in higher subnet",
//			seName: "e2e.a.mesh",
//			seAddressCache: cacheWith255Entries,
//			wantAddess: common.LocalAddressPrefix + ".11.1",
//			cacheController: &cacheControllerWith255Entries,
//			expectedCacheUpdate: true,
//			wantedError: nil,
//		},
//		{
//			name: "should gracefully propagate get error",
//			seName: "e2e.a.mesh",
//			seAddressCache: cacheWith255Entries,
//			wantAddess: "",
//			cacheController: &cacheControllerGetError,
//			expectedCacheUpdate: true,
//			wantedError: errors.New("BAD THINGS HAPPENED"),
//		},
//		{
//			name: "Should not return address on put error",
//			seName: "e2e.abcdefghijklmnop.mesh",
//			seAddressCache: cacheWith255Entries,
//			wantAddess: "",
//			cacheController: &cacheControllerPutError,
//			expectedCacheUpdate: true,
//			wantedError: errors.New("BAD THINGS HAPPENED"),
//		},
//	}
//
//	for _, c := range testCases {
//		t.Run(c.name, func(t *testing.T) {
//			seAddress, needsCacheUpdate, err := GetLocalAddressForSe(c.seName, &c.seAddressCache, c.cacheController)
//			if c.wantAddess != "" {
//				if !reflect.DeepEqual(seAddress, c.wantAddess) {
//					t.Errorf("Wanted se address: %s, got: %s", c.wantAddess, seAddress)
//				}
//				if err==nil && c.wantedError==nil {
//					//we're fine
//				} else if err.Error() != c.wantedError.Error() {
//					t.Errorf("Error mismatch. Expected %v but got %v", c.wantedError, err)
//				}
//				if needsCacheUpdate != c.expectedCacheUpdate {
//					t.Errorf("Expected %v, got %v for needs cache update", c.expectedCacheUpdate, needsCacheUpdate)
//				}
//			} else {
//				if seAddress != "" {
//					t.Errorf("Unexpectedly found address: %s", seAddress)
//				}
//			}
//		})
//	}
//
//}
//
//func TestMakeRemoteEndpointForServiceEntry(t *testing.T) {
//	address := "1.2.3.4"
//	locality := "us-west-2"
//	portName := "port"
//
//	endpoint := makeRemoteEndpointForServiceEntry(address, locality, portName)
//
//	if endpoint.Address != address {
//		t.Errorf("Address mismatch. Got: %v, expected: %v", endpoint.Address, address)
//	}
//	if endpoint.Locality != locality {
//		t.Errorf("Locality mismatch. Got: %v, expected: %v", endpoint.Locality, locality)
//	}
//	if endpoint.Ports[portName] != 15443 {
//		t.Errorf("Incorrect port found")
//	}
//}
//
func buildFakeConfigMapFromAddressStore(addressStore *ServiceEntryAddressStore, resourceVersion string) *v1.ConfigMap {
	bytes, _ := yaml.Marshal(addressStore)

	cm := v1.ConfigMap{
		Data: map[string]string{"serviceEntryAddressStore": string(bytes)},
	}
	cm.Name = "se-address-configmap"
	cm.Namespace = "admiral-remote-ctx"
	cm.ResourceVersion = resourceVersion
	return &cm
}

func TestModifyNonExistingSidecarForLocalClusterCommunication(t *testing.T) {
	sidecarController := &istio.SidecarController{}
	sidecarController.IstioClient = istiofake.NewSimpleClientset()

	remoteController := &RemoteController{}
	remoteController.SidecarController = sidecarController

	sidecarEgressMap := make(map[string]common.SidecarEgress)
	sidecarEgressMap["test-dependency-namespace"] = common.SidecarEgress{Namespace: "test-dependency-namespace", FQDN: "test-local-fqdn"}

	modifySidecarForLocalClusterCommunication("test-sidecar-namespace", sidecarEgressMap, remoteController)

	sidecarObj, _ := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars("test-sidecar-namespace").Get(common.GetWorkloadSidecarName(), v12.GetOptions{})

	if sidecarObj != nil {
		t.Fatalf("Modify non existing resource failed, as no new resource should be created.")
	}
}

func TestModifyExistingSidecarForLocalClusterCommunication(t *testing.T) {

	sidecarController := &istio.SidecarController{}
	sidecarController.IstioClient = istiofake.NewSimpleClientset()

	remoteController := &RemoteController{}
	remoteController.SidecarController = sidecarController

	existingSidecarObj := &v1alpha3.Sidecar{}
	existingSidecarObj.ObjectMeta.Namespace = "test-sidecar-namespace"
	existingSidecarObj.ObjectMeta.Name = "default"

	istioEgress := istionetworkingv1alpha3.IstioEgressListener{
		Hosts: []string{"test-host"},
	}

	existingSidecarObj.Spec = istionetworkingv1alpha3.Sidecar{
		Egress: []*istionetworkingv1alpha3.IstioEgressListener{&istioEgress},
	}

	createdSidecar, _ := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars("test-sidecar-namespace").Create(existingSidecarObj)

	if createdSidecar != nil {

		sidecarEgressMap := make(map[string]common.SidecarEgress)
		sidecarEgressMap["test-dependency-namespace"] = common.SidecarEgress{Namespace: "test-dependency-namespace", FQDN: "test-local-fqdn"}
		modifySidecarForLocalClusterCommunication("test-sidecar-namespace", sidecarEgressMap, remoteController)

		updatedSidecar, error := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars("test-sidecar-namespace").Get("default", v12.GetOptions{})

		if error != nil || updatedSidecar == nil {
			t.Fail()
		}

		hostList := append(createdSidecar.Spec.Egress[0].Hosts, "test-dependency-namespace/test-local-fqdn")
		createdSidecar.Spec.Egress[0].Hosts = hostList

		if !cmp.Equal(updatedSidecar, createdSidecar) {
			t.Fatalf("Modify existing sidecar failed as configuration is not same. Details - %v", cmp.Diff(updatedSidecar, createdSidecar))
		}
	} else {
		t.Error("sidecar resource could not be created")
	}
}

//
//func TestCreateServiceEntry(t *testing.T) {
//	admiralCache := AdmiralCache{}
//
//	localAddress := common.LocalAddressPrefix + ".10.1"
//
//	cnameIdentityCache := sync.Map{}
//	cnameIdentityCache.Store("dev.bar.global", "bar")
//	admiralCache.CnameIdentityCache = &cnameIdentityCache
//
//	admiralCache.ServiceEntryAddressStore = &ServiceEntryAddressStore{
//		EntryAddresses: map[string]string{"e2e.my-first-service.mesh-se":localAddress},
//		Addresses: []string{localAddress},
//	}
//
//	admiralCache.CnameClusterCache = common.NewMapOfMaps()
//
//	rc, _ := createMockRemoteController(func(i interface{}) {
//		res := i.(istio.Config)
//		se, ok := res.Spec.(*networking.ServiceEntry)
//		if ok {
//			if se.Hosts[0] != "dev.bar.global" {
//				t.Errorf("Host mismatch. Expected dev.bar.global, got %v", se.Hosts[0])
//			}
//		}
//	})
//
//	params := AdmiralParams{
//		EnableSAN: true,
//		SANPrefix: "prefix",
//		LabelSet:&common.LabelSet{WorkloadIdentityLabel:"identity"},
//		HostnameSuffix: "mesh",
//	}
//
//	cacheWithEntry := ServiceEntryAddressStore{
//		EntryAddresses: map[string]string{"e2e.my-first-service.mesh": localAddress},
//		Addresses: []string{localAddress},
//	}
//
//	cacheController := &test.FakeConfigMapController{
//		GetError: nil,
//		PutError: nil,
//		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
//	}
//
//	admiralCache.ConfigMapController = cacheController
//
//	deployment := v12.Deployment{}
//	deployment.Spec.Template.Labels = map[string]string{"env":"e2e", "identity":"my-first-service", }
//
//	resultingEntry := createServiceEntry(rc, params, &admiralCache, &deployment, map[string]*networking.ServiceEntry{})
//
//	if resultingEntry.Hosts[0] != "e2e.my-first-service.mesh" {
//		t.Errorf("Host mismatch. Got: %v, expected: e2e.my-first-service.mesh", resultingEntry.Hosts[0])
//	}
//
//	if resultingEntry.Addresses[0] != localAddress {
//		t.Errorf("Address mismatch. Got: %v, expected: " + localAddress, resultingEntry.Addresses[0])
//	}
//
//}
