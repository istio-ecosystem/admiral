package clusters

import (
	"context"
	"errors"
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/google/go-cmp/cmp"
	v13 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	istionetworkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	v14 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	coreV1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestCreateSeWithDrLabels(t *testing.T) {

	se := istionetworkingv1alpha3.ServiceEntry{
		Hosts: []string{"test.com"},
		Endpoints: []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	des := istionetworkingv1alpha3.DestinationRule{
		Host: "test.com",
		Subsets: []*istionetworkingv1alpha3.Subset{
			{Name: "subset1", Labels: map[string]string{"foo": "bar"}, TrafficPolicy: nil},
		},
	}

	cacheWithNoEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"test-se": "1.2.3.4"},
		Addresses:      []string{"1.2.3.4"},
	}

	emptyCacheController := test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithNoEntry, "123"),
	}

	res := createSeWithDrLabels(nil, false, "", "test-se", &se, &des, &cacheWithNoEntry, &emptyCacheController)

	if res == nil {
		t.Fail()
	}

	newSe := res["test-se"] // Test for Rollout

	value := newSe.Endpoints[0].Labels["foo"]

	if value != "bar" {
		t.Fail()
	}

	if newSe.Addresses[0] != "1.2.3.4" {
		t.Errorf("Address set incorrectly from cache, expected 1.2.3.4, got %v", newSe.Addresses[0])
	}
}

func TestAddServiceEntriesWithDr(t *testing.T) {
	admiralCache := AdmiralCache{}

	cnameIdentityCache := sync.Map{}
	cnameIdentityCache.Store("dev.bar.global", "bar")
	admiralCache.CnameIdentityCache = &cnameIdentityCache

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v13.GlobalTrafficPolicy)
	gtpCache.dependencyCache = make(map[string]*v14.Deployment)
	gtpCache.mutex = &sync.Mutex{}
	admiralCache.GlobalTrafficCache = gtpCache

	se := istionetworkingv1alpha3.ServiceEntry{
		Hosts: []string{"dev.bar.global"},
		Endpoints: []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	seConfig := v1alpha3.ServiceEntry{
		Spec: se,
	}
	seConfig.Name = "se1"
	seConfig.Namespace = "admiral-sync"

	fakeIstioClient := istiofake.NewSimpleClientset()
	fakeIstioClient.NetworkingV1alpha3().ServiceEntries("admiral-sync").Create(&seConfig)
	rc := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
	}

	AddServiceEntriesWithDr(&admiralCache, map[string]string{"cl1": "cl1"}, map[string]*RemoteController{"cl1": rc}, map[string]*istionetworkingv1alpha3.ServiceEntry{"se1": &se})
}

func TestCreateServiceEntryForNewServiceOrPod(t *testing.T) {

	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}
	rr, _ := InitAdmiral(context.Background(), p)

	config := rest.Config{
		Host: "localhost",
	}

	d, e := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300))

	r, e := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300))

	if e != nil {
		t.Fail()
	}

	fakeIstioClient := istiofake.NewSimpleClientset()
	rc := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		DeploymentController: d,
		RolloutController:    r,
	}

	rr.remoteControllers["test.cluster"] = rc
	createServiceEntryForNewServiceOrPod("test", "bar", rr)

}

func TestGetLocalAddressForSe(t *testing.T) {
	t.Parallel()
	cacheWithEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.a.mesh": common.LocalAddressPrefix + ".10.1"},
		Addresses:      []string{common.LocalAddressPrefix + ".10.1"},
	}
	cacheWithNoEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses:      []string{},
	}
	cacheWith255Entries := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses:      []string{},
	}

	for i := 1; i <= 255; i++ {
		address := common.LocalAddressPrefix + ".10." + strconv.Itoa(i)
		cacheWith255Entries.EntryAddresses[strconv.Itoa(i)+".mesh"] = address
		cacheWith255Entries.Addresses = append(cacheWith255Entries.Addresses, address)
	}

	emptyCacheController := test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithNoEntry, "123"),
	}

	cacheController := test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	cacheControllerWith255Entries := test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWith255Entries, "123"),
	}

	cacheControllerGetError := test.FakeConfigMapController{
		GetError:          errors.New("BAD THINGS HAPPENED"),
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	cacheControllerPutError := test.FakeConfigMapController{
		PutError:          errors.New("BAD THINGS HAPPENED"),
		GetError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	testCases := []struct {
		name                string
		seName              string
		seAddressCache      ServiceEntryAddressStore
		wantAddess          string
		cacheController     admiral.ConfigMapControllerInterface
		expectedCacheUpdate bool
		wantedError         error
	}{
		{
			name:                "should return new available address",
			seName:              "e2e.a.mesh",
			seAddressCache:      cacheWithNoEntry,
			wantAddess:          common.LocalAddressPrefix + ".10.1",
			cacheController:     &emptyCacheController,
			expectedCacheUpdate: true,
			wantedError:         nil,
		},
		{
			name:                "should return address from map",
			seName:              "e2e.a.mesh",
			seAddressCache:      cacheWithEntry,
			wantAddess:          common.LocalAddressPrefix + ".10.1",
			cacheController:     &cacheController,
			expectedCacheUpdate: false,
			wantedError:         nil,
		},
		{
			name:                "should return new available address",
			seName:              "e2e.b.mesh",
			seAddressCache:      cacheWithEntry,
			wantAddess:          common.LocalAddressPrefix + ".10.2",
			cacheController:     &cacheController,
			expectedCacheUpdate: true,
			wantedError:         nil,
		},
		{
			name:                "should return new available address in higher subnet",
			seName:              "e2e.a.mesh",
			seAddressCache:      cacheWith255Entries,
			wantAddess:          common.LocalAddressPrefix + ".11.1",
			cacheController:     &cacheControllerWith255Entries,
			expectedCacheUpdate: true,
			wantedError:         nil,
		},
		{
			name:                "should gracefully propagate get error",
			seName:              "e2e.a.mesh",
			seAddressCache:      cacheWith255Entries,
			wantAddess:          "",
			cacheController:     &cacheControllerGetError,
			expectedCacheUpdate: true,
			wantedError:         errors.New("BAD THINGS HAPPENED"),
		},
		{
			name:                "Should not return address on put error",
			seName:              "e2e.abcdefghijklmnop.mesh",
			seAddressCache:      cacheWith255Entries,
			wantAddess:          "",
			cacheController:     &cacheControllerPutError,
			expectedCacheUpdate: true,
			wantedError:         errors.New("BAD THINGS HAPPENED"),
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			seAddress, needsCacheUpdate, err := GetLocalAddressForSe(c.seName, &c.seAddressCache, c.cacheController)
			if c.wantAddess != "" {
				if !reflect.DeepEqual(seAddress, c.wantAddess) {
					t.Errorf("Wanted se address: %s, got: %s", c.wantAddess, seAddress)
				}
				if err == nil && c.wantedError == nil {
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

func TestMakeRemoteEndpointForServiceEntry(t *testing.T) {
	address := "1.2.3.4"
	locality := "us-west-2"
	portName := "port"

	endpoint := makeRemoteEndpointForServiceEntry(address, locality, portName, common.DefaultMtlsPort)

	if endpoint.Address != address {
		t.Errorf("Address mismatch. Got: %v, expected: %v", endpoint.Address, address)
	}
	if endpoint.Locality != locality {
		t.Errorf("Locality mismatch. Got: %v, expected: %v", endpoint.Locality, locality)
	}
	if endpoint.Ports[portName] != 15443 {
		t.Errorf("Incorrect port found")
	}
}

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
		sidecarEgressMap["test-dependency-namespace"] = common.SidecarEgress{Namespace: "test-dependency-namespace", FQDN: "test-local-fqdn", CNAMEs: map[string]string{"test.myservice.global": "1"}}
		modifySidecarForLocalClusterCommunication("test-sidecar-namespace", sidecarEgressMap, remoteController)

		updatedSidecar, err := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars("test-sidecar-namespace").Get("default", v12.GetOptions{})

		if err != nil || updatedSidecar == nil {
			t.Fail()
		}

		hostList := append(createdSidecar.Spec.Egress[0].Hosts, "test-dependency-namespace/test-local-fqdn", "test-dependency-namespace/test.myservice.global")
		createdSidecar.Spec.Egress[0].Hosts = hostList

		// Egress host order doesn't matter but will cause tests to fail. Move these values to their own lists for comparision
		createdSidecarEgress := createdSidecar.Spec.Egress
		updatedSidecarEgress := updatedSidecar.Spec.Egress
		createdSidecar.Spec.Egress = createdSidecar.Spec.Egress[:0]
		updatedSidecar.Spec.Egress = updatedSidecar.Spec.Egress[:0]

		if !cmp.Equal(updatedSidecar, createdSidecar) {
			t.Fatalf("Modify existing sidecar failed as configuration is not same. Details - %v", cmp.Diff(updatedSidecar, createdSidecar))
		}
		var matched *istionetworkingv1alpha3.IstioEgressListener
		for _, listener := range createdSidecarEgress {
			matched = nil

			for j, newListener := range updatedSidecarEgress {
				if listener.Bind == newListener.Bind && listener.Port == newListener.Port && listener.CaptureMode == newListener.CaptureMode {
					matched = newListener
					updatedSidecarEgress = append(updatedSidecarEgress[:j], updatedSidecarEgress[j+1:]...)
				}
			}
			if matched != nil {
				oldHosts := listener.Hosts
				newHosts := matched.Hosts
				oldHosts = oldHosts[:0]
				newHosts = newHosts[:0]
				assert.ElementsMatch(t, oldHosts, newHosts, "hosts should match")
				if !cmp.Equal(listener, matched) {
					t.Fatalf("Listeners do not match. Details - %v", cmp.Diff(listener, matched))
				}
			} else {
				t.Fatalf("Corresponding listener on updated sidecar not found. Details - %v", cmp.Diff(createdSidecarEgress, updatedSidecarEgress))
			}
		}
	} else {
		t.Error("sidecar resource could not be created")
	}
}

func TestCreateServiceEntry(t *testing.T) {

	config := rest.Config{
		Host: "localhost",
	}
	stop := make(chan struct{})
	s, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))

	if e != nil {
		t.Fatalf("%v", e)
	}

	admiralCache := AdmiralCache{}

	localAddress := common.LocalAddressPrefix + ".10.1"

	cnameIdentityCache := sync.Map{}
	cnameIdentityCache.Store("dev.bar.global", "bar")
	admiralCache.CnameIdentityCache = &cnameIdentityCache

	admiralCache.ServiceEntryAddressStore = &ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.my-first-service.mesh-se": localAddress},
		Addresses:      []string{localAddress},
	}

	admiralCache.CnameClusterCache = common.NewMapOfMaps()

	fakeIstioClient := istiofake.NewSimpleClientset()
	rc := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		ServiceController: s,
	}

	cacheWithEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.my-first-service.mesh": localAddress},
		Addresses:      []string{localAddress},
	}

	cacheController := &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	admiralCache.ConfigMapController = cacheController

	deployment := v14.Deployment{}
	deployment.Spec.Template.Labels = map[string]string{"env": "e2e", "identity": "my-first-service"}

	resultingEntry := createServiceEntry(rc, &admiralCache, &deployment, map[string]*istionetworkingv1alpha3.ServiceEntry{})

	if resultingEntry.Hosts[0] != "e2e.my-first-service.mesh" {
		t.Errorf("Host mismatch. Got: %v, expected: e2e.my-first-service.mesh", resultingEntry.Hosts[0])
	}

	if resultingEntry.Addresses[0] != localAddress {
		t.Errorf("Address mismatch. Got: %v, expected: "+localAddress, resultingEntry.Addresses[0])
	}

	if resultingEntry.Endpoints[0].Address != "admiral_dummy.com" {
		t.Errorf("Endpoint mismatch. Got %v, expected: %v", resultingEntry.Endpoints[0].Address, "admiral_dummy.com")
	}

	if resultingEntry.Endpoints[0].Locality != "us-west-2" {
		t.Errorf("Locality mismatch. Got %v, expected: %v", resultingEntry.Endpoints[0].Locality, "us-west-2")
	}

	// Test for Rollout
	rollout := argo.Rollout{}
	rollout.Spec.Template.Labels = map[string]string{"env": "e2e", "identity": "my-first-service"}

	resultingEntry = createServiceEntryForRollout(rc, &admiralCache, &rollout, map[string]*istionetworkingv1alpha3.ServiceEntry{})

	if resultingEntry.Hosts[0] != "e2e.my-first-service.mesh" {
		t.Errorf("Host mismatch. Got: %v, expected: e2e.my-first-service.mesh", resultingEntry.Hosts[0])
	}

	if resultingEntry.Addresses[0] != localAddress {
		t.Errorf("Address mismatch. Got: %v, expected: "+localAddress, resultingEntry.Addresses[0])
	}

	if resultingEntry.Endpoints[0].Address != "admiral_dummy.com" {
		t.Errorf("Endpoint mismatch. Got %v, expected: %v", resultingEntry.Endpoints[0].Address, "admiral_dummy.com")
	}

	if resultingEntry.Endpoints[0].Locality != "us-west-2" {
		t.Errorf("Locality mismatch. Got %v, expected: %v", resultingEntry.Endpoints[0].Locality, "us-west-2")
	}

}

func TestCreateIngressOnlyVirtualService(t *testing.T) {

	fakeIstioClientCreate := istiofake.NewSimpleClientset()
	rcCreate := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClientCreate,
		},
	}
	fakeIstioClientUpdate := istiofake.NewSimpleClientset()
	rcUpdate := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClientUpdate,
		},
	}

	cname := "qa.mysvc.global"

	vsname := cname + "-default-vs"

	localFqdn := "mysvc.newmynamespace.svc.cluster.local"
	localFqdn2 := "mysvc.mynamespace.svc.cluster.local"

	vsTobeUpdated := makeVirtualService(cname, []string{common.MulticlusterIngressGateway}, localFqdn, 80)

	rcUpdate.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(common.GetSyncNamespace()).Create(&v1alpha3.VirtualService{
		Spec:       *vsTobeUpdated,
		ObjectMeta: v12.ObjectMeta{Name: vsname, Namespace: common.GetSyncNamespace()}})

	testCases := []struct {
		name           string
		rc             *RemoteController
		localFqdn      string
		expectedResult string
	}{
		{
			name:           "Should return a created virtual service",
			rc:             rcCreate,
			localFqdn:      localFqdn,
			expectedResult: localFqdn,
		},
		{
			name:           "Should return an updated virtual service",
			rc:             rcCreate,
			localFqdn:      localFqdn2,
			expectedResult: localFqdn2,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			createIngressOnlyVirtualService(c.rc, cname, &istionetworkingv1alpha3.ServiceEntry{Hosts: []string{"qa.mysvc.global"}}, c.localFqdn, map[string]uint32{common.Http: 80})
			vs, err := c.rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(common.GetSyncNamespace()).Get(vsname, v12.GetOptions{})
			if err != nil {
				t.Errorf("Test %s failed, expected: %v got %v", c.name, c.expectedResult, err)
			}
			if vs == nil && vs.Spec.Http[0].Route[0].Destination.Host != c.expectedResult {
				if vs != nil {
					t.Errorf("Virtual service update failed with expected local fqdn: %v, got: %v", localFqdn2, vs.Spec.Http[0].Route[0].Destination.Host)
				}
				t.FailNow()
			}
			if len(vs.Spec.Gateways) > 1 || vs.Spec.Gateways[0] != common.MulticlusterIngressGateway {
				t.Errorf("Virtual service gateway expected: %v, got: %v", common.MulticlusterIngressGateway, vs.Spec.Gateways)
			}
		})
	}
}

func TestCreateServiceEntryForNewServiceOrPodRolloutsUsecase(t *testing.T) {

	const NAMESPACE = "test-test"
	const SERVICENAME = "serviceNameActive"
	const ROLLOUT_POD_HASH_LABEL string = "rollouts-pod-template-hash"

	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	rr, _ := InitAdmiral(context.Background(), p)

	config := rest.Config{
		Host: "localhost",
	}

	d, e := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300))

	r, e := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300))
	v, e := istio.NewVirtualServiceController(make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, time.Second*time.Duration(300))

	if e != nil {
		t.Fail()
	}
	s, e := admiral.NewServiceController(make(chan struct{}), &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))

	cacheWithEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"test.test.mesh-se": common.LocalAddressPrefix + ".10.1"},
		Addresses:      []string{common.LocalAddressPrefix + ".10.1"},
	}

	fakeIstioClient := istiofake.NewSimpleClientset()
	rc := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		DeploymentController:     d,
		RolloutController:        r,
		ServiceController:        s,
		VirtualServiceController: v,
	}
	rc.ClusterID = "test.cluster"
	rr.remoteControllers["test.cluster"] = rc

	admiralCache := &AdmiralCache{
		IdentityClusterCache:       common.NewMapOfMaps(),
		ServiceEntryAddressStore:   &cacheWithEntry,
		CnameClusterCache:          common.NewMapOfMaps(),
		CnameIdentityCache:         &sync.Map{},
		CnameDependentClusterCache: common.NewMapOfMaps(),
		IdentityDependencyCache:    common.NewMapOfMaps(),
		GlobalTrafficCache:         &globalTrafficCache{},
		DependencyNamespaceCache:   common.NewSidecarEgressMap(),
	}
	rr.AdmiralCache = admiralCache

	rollout := argo.Rollout{}

	rollout.Spec = argo.RolloutSpec{
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v12.ObjectMeta{
				Labels: map[string]string{"identity": "test"},
			},
		},
	}

	rollout.Namespace = NAMESPACE
	rollout.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{},
	}
	labelMap := make(map[string]string)
	labelMap["identity"] = "test"

	matchLabel4 := make(map[string]string)
	matchLabel4["app"] = "test"

	labelSelector4 := v12.LabelSelector{
		MatchLabels: matchLabel4,
	}
	rollout.Spec.Selector = &labelSelector4

	r.Cache.AppendRolloutToCluster("bar", &rollout)

	selectorMap := make(map[string]string)
	selectorMap["app"] = "test"
	selectorMap[ROLLOUT_POD_HASH_LABEL] = "hash"

	activeService := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	activeService.Name = SERVICENAME
	activeService.Namespace = NAMESPACE
	port1 := coreV1.ServicePort{
		Port: 8080,
		Name: "random1",
	}

	port2 := coreV1.ServicePort{
		Port: 8081,
		Name: "random2",
	}

	ports := []coreV1.ServicePort{port1, port2}
	activeService.Spec.Ports = ports

	s.Cache.Put(activeService)
	se := createServiceEntryForNewServiceOrPod("test", "bar", rr)
	if nil == se {
		t.Fatalf("no service entries found")
	}
	if len(se) != 1 {
		t.Fatalf("More than 1 service entries found. Expected 1")
	}
	serviceEntryResp := se["test.test.mesh"]
	if nil == serviceEntryResp {
		t.Fatalf("Service entry returned should not be empty")
	}

}
