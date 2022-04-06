package clusters

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
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
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
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

	admiralCache.SeClusterCache = common.NewMapOfMaps()

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

	emptyEndpointSe := istionetworkingv1alpha3.ServiceEntry{
		Hosts:     []string{"dev.bar.global"},
		Endpoints: []*istionetworkingv1alpha3.ServiceEntry_Endpoint{},
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
	AddServiceEntriesWithDr(&admiralCache, map[string]string{"cl1": "cl1"}, map[string]*RemoteController{"cl1": rc}, map[string]*istionetworkingv1alpha3.ServiceEntry{"se1": &emptyEndpointSe})
}

func TestCreateSeAndDrSetFromGtp(t *testing.T) {

	host := "dev.bar.global"
	west := "west"
	east := "east"
	eastWithCaps := "East"

	admiralCache := AdmiralCache{}

	admiralCache.ServiceEntryAddressStore = &ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses:      []string{},
	}

	cacheWithEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses:      []string{},
	}

	cacheController := &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	admiralCache.ConfigMapController = cacheController

	se := &istionetworkingv1alpha3.ServiceEntry{
		Addresses: []string{"240.10.1.0"},
		Hosts:     []string{host},
		Endpoints: []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{}, Locality: "us-west-2"},
			{Address: "240.20.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{}, Locality: "us-east-2"},
		},
	}

	defaultPolicy := &model.TrafficPolicy{
		LbType: model.TrafficPolicy_TOPOLOGY,
		Dns:    host,
	}

	trafficPolicyDefaultOverride := &model.TrafficPolicy{
		LbType:    model.TrafficPolicy_FAILOVER,
		DnsPrefix: common.Default,
		Target: []*model.TrafficGroup{
			{
				Region: "us-west-2",
				Weight: 100,
			},
		},
	}

	trafficPolicyWest := &model.TrafficPolicy{
		LbType:    model.TrafficPolicy_FAILOVER,
		DnsPrefix: west,
		Target: []*model.TrafficGroup{
			{
				Region: "us-west-2",
				Weight: 100,
			},
		},
	}

	trafficPolicyEast := &model.TrafficPolicy{
		LbType:    model.TrafficPolicy_FAILOVER,
		DnsPrefix: east,
		Target: []*model.TrafficGroup{
			{
				Region: "us-east-2",
				Weight: 100,
			},
		},
	}

	gTPDefaultOverride := &v13.GlobalTrafficPolicy{
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				trafficPolicyDefaultOverride,
			},
		},
	}

	gTPMultipleDns := &v13.GlobalTrafficPolicy{
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				defaultPolicy, trafficPolicyWest, trafficPolicyEast,
			},
		},
	}

	testCases := []struct {
		name     string
		env      string
		locality string
		se       *istionetworkingv1alpha3.ServiceEntry
		gtp      *v13.GlobalTrafficPolicy
		seDrSet  map[string]*SeDrTuple
	}{
		{
			name:     "Should handle a nil GTP",
			env:      "dev",
			locality: "us-west-2",
			se:       se,
			gtp:      nil,
			seDrSet:  map[string]*SeDrTuple{host: nil},
		},
		{
			name:     "Should handle a GTP with default overide",
			env:      "dev",
			locality: "us-west-2",
			se:       se,
			gtp:      gTPDefaultOverride,
			seDrSet:  map[string]*SeDrTuple{host: nil},
		},
		{
			name:     "Should handle a GTP with multiple Dns",
			env:      "dev",
			locality: "us-west-2",
			se:       se,
			gtp:      gTPMultipleDns,
			seDrSet: map[string]*SeDrTuple{host: nil, common.GetCnameVal([]string{west, host}): nil,
				common.GetCnameVal([]string{east, host}): nil},
		},
		{
			name:     "Should handle a GTP with Dns prefix with Caps",
			env:      "dev",
			locality: "us-west-2",
			se:       se,
			gtp:      gTPMultipleDns,
			seDrSet: map[string]*SeDrTuple{host: nil, common.GetCnameVal([]string{west, host}): nil,
				strings.ToLower(common.GetCnameVal([]string{eastWithCaps, host})): nil},
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := createSeAndDrSetFromGtp(c.env, c.locality, c.se, c.gtp, &admiralCache)
			generatedHosts := make([]string, 0, len(result))
			for generatedHost := range result {
				generatedHosts = append(generatedHosts, generatedHost)
			}
			for host, _ := range c.seDrSet {
				if _, ok := result[host]; !ok {
					t.Fatalf("Generated hosts %v is missing the required host: %v", generatedHosts, host)
				} else if !isLower(result[host].SeName) || !isLower(result[host].DrName) {
					t.Fatalf("Generated istio resource names %v %v are not all lowercase", result[host].SeName, result[host].DrName)
				}
			}
		})
	}

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

	rr.RemoteControllers["test.cluster"] = rc
	modifyServiceEntryForNewServiceOrPod(admiral.Add, "test", "bar", rr)

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
				listener.Hosts = listener.Hosts[:0]
				matched.Hosts = matched.Hosts[:0]
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

	// the second deployment will be add with us-east-2 region remote controller
	secondDeployment := v14.Deployment{}
	secondDeployment.Spec.Template.Labels = map[string]string{"env": "e2e", "identity": "my-first-service"}

	se := istionetworkingv1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istionetworkingv1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istionetworkingv1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istionetworkingv1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		},
	}

	oneEndpointSe := istionetworkingv1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istionetworkingv1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istionetworkingv1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istionetworkingv1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		},
	}

	twoEndpointSe := istionetworkingv1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istionetworkingv1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istionetworkingv1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istionetworkingv1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}

	threeEndpointSe := istionetworkingv1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istionetworkingv1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istionetworkingv1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istionetworkingv1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}
	eastEndpointSe := istionetworkingv1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istionetworkingv1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istionetworkingv1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istionetworkingv1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}

	emptyEndpointSe := istionetworkingv1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istionetworkingv1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istionetworkingv1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istionetworkingv1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints:       []*istionetworkingv1alpha3.ServiceEntry_Endpoint{},
	}

	grpcSe := istionetworkingv1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istionetworkingv1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "grpc", Protocol: "grpc"}},
		Location:        istionetworkingv1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istionetworkingv1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"grpc": 0}, Locality: "us-west-2"},
		},
	}

	deploymentSeCreationTestCases := []struct {
		name           string
		action         admiral.EventType
		rc             *RemoteController
		admiralCache   AdmiralCache
		meshPorts      map[string]uint32
		deployment     v14.Deployment
		serviceEntries map[string]*istionetworkingv1alpha3.ServiceEntry
		expectedResult *istionetworkingv1alpha3.ServiceEntry
	}{
		{
			name:           "Should return a created service entry with grpc protocol",
			action:         admiral.Add,
			rc:             rc,
			admiralCache:   admiralCache,
			meshPorts:      map[string]uint32{"grpc": uint32(80)},
			deployment:     deployment,
			serviceEntries: map[string]*istionetworkingv1alpha3.ServiceEntry{},
			expectedResult: &grpcSe,
		},
		{
			name:           "Should return a created service entry with http protocol",
			action:         admiral.Add,
			rc:             rc,
			admiralCache:   admiralCache,
			meshPorts:      map[string]uint32{"http": uint32(80)},
			deployment:     deployment,
			serviceEntries: map[string]*istionetworkingv1alpha3.ServiceEntry{},
			expectedResult: &se,
		},
		{
			name:         "Delete the service entry with one endpoint",
			action:       admiral.Delete,
			rc:           rc,
			admiralCache: admiralCache,
			meshPorts:    map[string]uint32{"http": uint32(80)},
			deployment:   deployment,
			serviceEntries: map[string]*istionetworkingv1alpha3.ServiceEntry{
				"e2e.my-first-service.mesh": &oneEndpointSe,
			},
			expectedResult: &emptyEndpointSe,
		},
		{
			name:         "Delete the service entry with two endpoints",
			action:       admiral.Delete,
			rc:           rc,
			admiralCache: admiralCache,
			meshPorts:    map[string]uint32{"http": uint32(80)},
			deployment:   deployment,
			serviceEntries: map[string]*istionetworkingv1alpha3.ServiceEntry{
				"e2e.my-first-service.mesh": &twoEndpointSe,
			},
			expectedResult: &eastEndpointSe,
		},
		{
			name:         "Delete the service entry with three endpoints",
			action:       admiral.Delete,
			rc:           rc,
			admiralCache: admiralCache,
			meshPorts:    map[string]uint32{"http": uint32(80)},
			deployment:   deployment,
			serviceEntries: map[string]*istionetworkingv1alpha3.ServiceEntry{
				"e2e.my-first-service.mesh": &threeEndpointSe,
			},
			expectedResult: &eastEndpointSe,
		},
	}

	//Run the test for every provided case
	for _, c := range deploymentSeCreationTestCases {
		t.Run(c.name, func(t *testing.T) {
			var createdSE *istionetworkingv1alpha3.ServiceEntry
			createdSE = createServiceEntry(c.action, c.rc, &c.admiralCache, c.meshPorts, &c.deployment, c.serviceEntries)
			if !reflect.DeepEqual(createdSE, c.expectedResult) {
				t.Errorf("Test %s failed, expected: %v got %v", c.name, c.expectedResult, createdSE)
			}
		})
	}

	// Test for Rollout
	rollout := argo.Rollout{}
	rollout.Spec.Template.Labels = map[string]string{"env": "e2e", "identity": "my-first-service"}

	rolloutSeCreationTestCases := []struct {
		name           string
		rc             *RemoteController
		admiralCache   AdmiralCache
		meshPorts      map[string]uint32
		rollout        argo.Rollout
		expectedResult *istionetworkingv1alpha3.ServiceEntry
	}{
		{
			name:           "Should return a created service entry with grpc protocol",
			rc:             rc,
			admiralCache:   admiralCache,
			meshPorts:      map[string]uint32{"grpc": uint32(80)},
			rollout:        rollout,
			expectedResult: &grpcSe,
		},
		{
			name:           "Should return a created service entry with http protocol",
			rc:             rc,
			admiralCache:   admiralCache,
			meshPorts:      map[string]uint32{"http": uint32(80)},
			rollout:        rollout,
			expectedResult: &se,
		},
	}

	//Run the test for every provided case
	for _, c := range rolloutSeCreationTestCases {
		t.Run(c.name, func(t *testing.T) {
			createdSE := createServiceEntryForRollout(admiral.Add, c.rc, &c.admiralCache, c.meshPorts, &c.rollout, map[string]*istionetworkingv1alpha3.ServiceEntry{})
			if !reflect.DeepEqual(createdSE, c.expectedResult) {
				t.Errorf("Test %s failed, expected: %v got %v", c.name, c.expectedResult, createdSE)
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
	rr.RemoteControllers["test.cluster"] = rc

	admiralCache := &AdmiralCache{
		IdentityClusterCache:       common.NewMapOfMaps(),
		ServiceEntryAddressStore:   &cacheWithEntry,
		CnameClusterCache:          common.NewMapOfMaps(),
		CnameIdentityCache:         &sync.Map{},
		CnameDependentClusterCache: common.NewMapOfMaps(),
		IdentityDependencyCache:    common.NewMapOfMaps(),
		GlobalTrafficCache:         &globalTrafficCache{},
		DependencyNamespaceCache:   common.NewSidecarEgressMap(),
		SeClusterCache:             common.NewMapOfMaps(),
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

	r.Cache.UpdateRolloutToClusterCache("bar", &rollout)

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
	se := modifyServiceEntryForNewServiceOrPod(admiral.Add, "test", "bar", rr)
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

func TestCreateServiceEntryForBlueGreenRolloutsUsecase(t *testing.T) {

	const NAMESPACE = "test-test"
	const ACTIVE_SERVICENAME = "serviceNameActive"
	const PREVIEW_SERVICENAME = "serviceNamePreview"
	const ROLLOUT_POD_HASH_LABEL string = "rollouts-pod-template-hash"

	p := common.AdmiralParams{
		KubeconfigPath:        "testdata/fake.config",
		PreviewHostnamePrefix: "preview",
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
		EntryAddresses: map[string]string{
			"test.test.mesh-se":         common.LocalAddressPrefix + ".10.1",
			"preview.test.test.mesh-se": common.LocalAddressPrefix + ".10.2",
		},
		Addresses: []string{common.LocalAddressPrefix + ".10.1", common.LocalAddressPrefix + ".10.2"},
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
	rr.RemoteControllers["test.cluster"] = rc

	admiralCache := &AdmiralCache{
		IdentityClusterCache:       common.NewMapOfMaps(),
		ServiceEntryAddressStore:   &cacheWithEntry,
		CnameClusterCache:          common.NewMapOfMaps(),
		CnameIdentityCache:         &sync.Map{},
		CnameDependentClusterCache: common.NewMapOfMaps(),
		IdentityDependencyCache:    common.NewMapOfMaps(),
		GlobalTrafficCache:         &globalTrafficCache{},
		DependencyNamespaceCache:   common.NewSidecarEgressMap(),
		SeClusterCache:             common.NewMapOfMaps(),
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
		BlueGreen: &argo.BlueGreenStrategy{ActiveService: ACTIVE_SERVICENAME, PreviewService: PREVIEW_SERVICENAME},
	}
	labelMap := make(map[string]string)
	labelMap["identity"] = "test"

	matchLabel4 := make(map[string]string)
	matchLabel4["app"] = "test"

	labelSelector4 := v12.LabelSelector{
		MatchLabels: matchLabel4,
	}
	rollout.Spec.Selector = &labelSelector4

	r.Cache.UpdateRolloutToClusterCache("bar", &rollout)

	selectorMap := make(map[string]string)
	selectorMap["app"] = "test"
	selectorMap[ROLLOUT_POD_HASH_LABEL] = "hash"

	port1 := coreV1.ServicePort{
		Port: 8080,
		Name: "random1",
	}

	port2 := coreV1.ServicePort{
		Port: 8081,
		Name: "random2",
	}

	ports := []coreV1.ServicePort{port1, port2}

	activeService := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	activeService.Name = ACTIVE_SERVICENAME
	activeService.Namespace = NAMESPACE
	activeService.Spec.Ports = ports

	s.Cache.Put(activeService)

	previewService := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	previewService.Name = PREVIEW_SERVICENAME
	previewService.Namespace = NAMESPACE
	previewService.Spec.Ports = ports

	s.Cache.Put(previewService)

	se := modifyServiceEntryForNewServiceOrPod(admiral.Add, "test", "bar", rr)

	if nil == se {
		t.Fatalf("no service entries found")
	}
	if len(se) != 2 {
		t.Fatalf("Expected 2 service entries to be created but found %d", len(se))
	}
	serviceEntryResp := se["test.test.mesh"]
	if nil == serviceEntryResp {
		t.Fatalf("Service entry returned should not be empty")
	}
	previewServiceEntryResp := se["preview.test.test.mesh"]
	if nil == previewServiceEntryResp {
		t.Fatalf("Preview Service entry returned should not be empty")
	}

	// When Preview service is not defined in BlueGreen strategy
	rollout.Spec.Strategy = argo.RolloutStrategy{
		BlueGreen: &argo.BlueGreenStrategy{ActiveService: ACTIVE_SERVICENAME},
	}

	se = modifyServiceEntryForNewServiceOrPod(admiral.Add, "test", "bar", rr)

	if len(se) != 1 {
		t.Fatalf("Expected 1 service entries to be created but found %d", len(se))
	}
	serviceEntryResp = se["test.test.mesh"]

	if nil == serviceEntryResp {
		t.Fatalf("Service entry returned should not be empty")
	}
}

func TestUpdateEndpointsForBlueGreen(t *testing.T) {
	const CLUSTER_INGRESS_1 = "ingress1.com"
	const ACTIVE_SERVICE = "activeService"
	const PREVIEW_SERVICE = "previewService"
	const NAMESPACE = "namespace"
	const ACTIVE_MESH_HOST = "qal.example.mesh"
	const PREVIEW_MESH_HOST = "preview.qal.example.mesh"

	rollout := argo.Rollout{}
	rollout.Spec.Strategy = argo.RolloutStrategy{
		BlueGreen: &argo.BlueGreenStrategy{
			ActiveService:  ACTIVE_SERVICE,
			PreviewService: PREVIEW_SERVICE,
		},
	}
	rollout.Spec.Template.Annotations = map[string]string{}
	rollout.Spec.Template.Annotations[common.SidecarEnabledPorts] = "8080"

	endpoint := istionetworkingv1alpha3.ServiceEntry_Endpoint{
		Labels: map[string]string{}, Address: CLUSTER_INGRESS_1, Ports: map[string]uint32{"http": 15443},
	}

	meshPorts := map[string]uint32{"http": 8080}

	weightedServices := map[string]*WeightedService{
		ACTIVE_SERVICE:  {Service: &v1.Service{ObjectMeta: v12.ObjectMeta{Name: ACTIVE_SERVICE, Namespace: NAMESPACE}}},
		PREVIEW_SERVICE: {Service: &v1.Service{ObjectMeta: v12.ObjectMeta{Name: PREVIEW_SERVICE, Namespace: NAMESPACE}}},
	}

	activeWantedEndpoints := istionetworkingv1alpha3.ServiceEntry_Endpoint{
		Address: ACTIVE_SERVICE + common.Sep + NAMESPACE + common.DotLocalDomainSuffix, Ports: meshPorts,
	}

	previewWantedEndpoints := istionetworkingv1alpha3.ServiceEntry_Endpoint{
		Address: PREVIEW_SERVICE + common.Sep + NAMESPACE + common.DotLocalDomainSuffix, Ports: meshPorts,
	}

	testCases := []struct {
		name             string
		rollout          argo.Rollout
		inputEndpoint    istionetworkingv1alpha3.ServiceEntry_Endpoint
		weightedServices map[string]*WeightedService
		clusterIngress   string
		meshPorts        map[string]uint32
		meshHost         string
		wantedEndpoints  istionetworkingv1alpha3.ServiceEntry_Endpoint
	}{
		{
			name:             "should return endpoint with active service address",
			rollout:          rollout,
			inputEndpoint:    endpoint,
			weightedServices: weightedServices,
			meshPorts:        meshPorts,
			meshHost:         ACTIVE_MESH_HOST,
			wantedEndpoints:  activeWantedEndpoints,
		},
		{
			name:             "should return endpoint with preview service address",
			rollout:          rollout,
			inputEndpoint:    endpoint,
			weightedServices: weightedServices,
			meshPorts:        meshPorts,
			meshHost:         PREVIEW_MESH_HOST,
			wantedEndpoints:  previewWantedEndpoints,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			updateEndpointsForBlueGreen(&c.rollout, c.weightedServices, map[string]string{}, &c.inputEndpoint, "test", c.meshHost)
			if c.inputEndpoint.Address != c.wantedEndpoints.Address {
				t.Errorf("Wanted %s endpoint, got: %s", c.wantedEndpoints.Address, c.inputEndpoint.Address)
			}
		})
	}
}

func TestUpdateEndpointsForWeightedServices(t *testing.T) {
	t.Parallel()

	const CLUSTER_INGRESS_1 = "ingress1.com"
	const CLUSTER_INGRESS_2 = "ingress2.com"
	const CANARY_SERVICE = "canaryService"
	const STABLE_SERVICE = "stableService"
	const NAMESPACE = "namespace"

	se := &istionetworkingv1alpha3.ServiceEntry{
		Endpoints: []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
			{Labels: map[string]string{}, Address: CLUSTER_INGRESS_1, Weight: 10, Ports: map[string]uint32{"http": 15443}},
			{Labels: map[string]string{}, Address: CLUSTER_INGRESS_2, Weight: 10, Ports: map[string]uint32{"http": 15443}},
		},
	}

	meshPorts := map[string]uint32{"http": 8080}

	weightedServices := map[string]*WeightedService{
		CANARY_SERVICE: {Weight: 10, Service: &v1.Service{ObjectMeta: v12.ObjectMeta{Name: CANARY_SERVICE, Namespace: NAMESPACE}}},
		STABLE_SERVICE: {Weight: 90, Service: &v1.Service{ObjectMeta: v12.ObjectMeta{Name: STABLE_SERVICE, Namespace: NAMESPACE}}},
	}
	weightedServicesZeroWeight := map[string]*WeightedService{
		CANARY_SERVICE: {Weight: 0, Service: &v1.Service{ObjectMeta: v12.ObjectMeta{Name: CANARY_SERVICE, Namespace: NAMESPACE}}},
		STABLE_SERVICE: {Weight: 100, Service: &v1.Service{ObjectMeta: v12.ObjectMeta{Name: STABLE_SERVICE, Namespace: NAMESPACE}}},
	}

	wantedEndpoints := []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
		{Address: CLUSTER_INGRESS_2, Weight: 10, Ports: map[string]uint32{"http": 15443}},
		{Address: STABLE_SERVICE + common.Sep + NAMESPACE + common.DotLocalDomainSuffix, Weight: 90, Ports: meshPorts},
		{Address: CANARY_SERVICE + common.Sep + NAMESPACE + common.DotLocalDomainSuffix, Weight: 10, Ports: meshPorts},
	}

	wantedEndpointsZeroWeights := []*istionetworkingv1alpha3.ServiceEntry_Endpoint{
		{Address: CLUSTER_INGRESS_2, Weight: 10, Ports: map[string]uint32{"http": 15443}},
		{Address: STABLE_SERVICE + common.Sep + NAMESPACE + common.DotLocalDomainSuffix, Weight: 100, Ports: meshPorts},
	}

	testCases := []struct {
		name              string
		inputServiceEntry *istionetworkingv1alpha3.ServiceEntry
		weightedServices  map[string]*WeightedService
		clusterIngress    string
		meshPorts         map[string]uint32
		wantedEndpoints   []*istionetworkingv1alpha3.ServiceEntry_Endpoint
	}{
		{
			name:              "should return endpoints with assigned weights",
			inputServiceEntry: copyServiceEntry(se),
			weightedServices:  weightedServices,
			clusterIngress:    CLUSTER_INGRESS_1,
			meshPorts:         meshPorts,
			wantedEndpoints:   wantedEndpoints,
		},
		{
			name:              "should return endpoints as is",
			inputServiceEntry: copyServiceEntry(se),
			weightedServices:  weightedServices,
			clusterIngress:    "random",
			meshPorts:         meshPorts,
			wantedEndpoints:   copyServiceEntry(se).Endpoints,
		},
		{
			name:              "should not return endpoints with zero weight",
			inputServiceEntry: copyServiceEntry(se),
			weightedServices:  weightedServicesZeroWeight,
			clusterIngress:    CLUSTER_INGRESS_1,
			meshPorts:         meshPorts,
			wantedEndpoints:   wantedEndpointsZeroWeights,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			updateEndpointsForWeightedServices(c.inputServiceEntry,
				c.weightedServices, c.clusterIngress, c.meshPorts)
			if len(c.inputServiceEntry.Endpoints) != len(c.wantedEndpoints) {
				t.Errorf("Wanted %d endpoints, got: %d", len(c.wantedEndpoints), len(c.inputServiceEntry.Endpoints))
			}
			for _, ep := range c.wantedEndpoints {
				for _, epResult := range c.inputServiceEntry.Endpoints {
					if ep.Address == epResult.Address {
						if ep.Weight != epResult.Weight {
							t.Errorf("Wanted endpoint weight %d, got: %d for Address %s", ep.Weight, epResult.Weight, ep.Address)
						}
					}
				}
			}
		})
	}

}

func isLower(s string) bool {
	for _, r := range s {
		if !unicode.IsLower(r) && unicode.IsLetter(r) {
			return false
		}
	}
	return true
}
