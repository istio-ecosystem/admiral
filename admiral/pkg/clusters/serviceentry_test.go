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
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	"gopkg.in/yaml.v2"
	istioNetworkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	k8sAppsV1 "k8s.io/api/apps/v1"
	v14 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func admiralParamsForServiceEntryTests() common.AdmiralParams {
	return common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			GatewayApp:                   "gatewayapp",
			WorkloadIdentityKey:          "identity",
			PriorityKey:                  "priority",
			EnvKey:                       "env",
			GlobalTrafficDeploymentLabel: "identity",
		},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheRefreshDuration:       0,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace:      "default",
		WorkloadSidecarName:        "default",
		SecretResolver:             "",
	}
}

var serviceEntryTestSingleton sync.Once

func setupForServiceEntryTests() {
	var initHappened bool
	serviceEntryTestSingleton.Do(func() {
		common.ResetSync()
		initHappened = true
		common.InitializeConfig(admiralParamsForServiceEntryTests())
	})
	if !initHappened {
		log.Warn("InitializeConfig was NOT called from setupForServiceEntryTests")
	} else {
		log.Info("InitializeConfig was called setupForServiceEntryTests")
	}
}

func makeTestDeployment(name, namespace, identityLabelValue string) *k8sAppsV1.Deployment {
	return &k8sAppsV1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"env": "test",
				"traffic.sidecar.istio.io/includeInboundPorts": "8090",
			},
		},
		Spec: k8sAppsV1.DeploymentSpec{
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"env": "test",
						"traffic.sidecar.istio.io/includeInboundPorts": "8090",
					},
					Labels: map[string]string{
						"identity": identityLabelValue,
					},
				},
				Spec: coreV1.PodSpec{},
			},
			Selector: &v12.LabelSelector{
				MatchLabels: map[string]string{
					"identity": identityLabelValue,
					"app":      identityLabelValue,
				},
			},
		},
	}
}

func makeTestRollout(name, namespace, identityLabelValue string) argo.Rollout {
	return argo.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"env": "test",
			},
		},
		Spec: argo.RolloutSpec{
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: v12.ObjectMeta{
					Labels: map[string]string{"identity": identityLabelValue},
					Annotations: map[string]string{
						"env": "test",
						"traffic.sidecar.istio.io/includeInboundPorts": "8090",
					},
				},
			},
			Strategy: argo.RolloutStrategy{
				Canary: &argo.CanaryStrategy{
					TrafficRouting: &argo.RolloutTrafficRouting{
						Istio: &argo.IstioTrafficRouting{
							VirtualService: &argo.IstioVirtualService{
								Name: name + "-canary",
							},
						},
					},
					CanaryService: name + "-canary",
					StableService: name + "-stable",
				},
			},
			Selector: &v12.LabelSelector{
				MatchLabels: map[string]string{
					"identity": identityLabelValue,
					"app":      identityLabelValue,
				},
			},
		},
	}
}

func makeGTP(name, namespace, identity, env, dnsPrefix string, creationTimestamp v12.Time) *v13.GlobalTrafficPolicy {
	return &v13.GlobalTrafficPolicy{
		ObjectMeta: v12.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: creationTimestamp,
			Labels:            map[string]string{"identity": identity, "env": env},
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: dnsPrefix}},
		},
	}
}

func TestModifyServiceEntryForNewServiceOrPodForExcludedIdentity(t *testing.T) {
	setupForServiceEntryTests()
	var (
		env                                 = "test"
		stop                                = make(chan struct{})
		foobarMetadataName                  = "foobar"
		foobarMetadataNamespace             = "foobar-ns"
		rollout1Identity                    = "rollout1"
		deployment1Identity                 = "deployment1"
		testRollout1                        = makeTestRollout(foobarMetadataName, foobarMetadataNamespace, rollout1Identity)
		testDeployment1                     = makeTestDeployment(foobarMetadataName, foobarMetadataNamespace, deployment1Identity)
		clusterID                           = "test-dev-k8s"
		fakeIstioClient                     = istiofake.NewSimpleClientset()
		config                              = rest.Config{Host: "localhost"}
		expectedServiceEntriesForDeployment = map[string]*istioNetworkingV1Alpha3.ServiceEntry{
			"test." + deployment1Identity + ".mesh": &istioNetworkingV1Alpha3.ServiceEntry{
				Hosts:     []string{"test." + deployment1Identity + ".mesh"},
				Addresses: []string{"127.0.0.1"},
				Ports: []*istioNetworkingV1Alpha3.Port{
					&istioNetworkingV1Alpha3.Port{
						Number:   80,
						Protocol: "http",
						Name:     "http",
					},
				},
				Location:   istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
				Resolution: istioNetworkingV1Alpha3.ServiceEntry_DNS,
				Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
					&istioNetworkingV1Alpha3.WorkloadEntry{
						Address: "dummy.admiral.global",
						Ports: map[string]uint32{
							"http": 0,
						},
						Locality: "us-west-2",
					},
				},
				SubjectAltNames: []string{"spiffe://prefix/" + deployment1Identity},
			},
		}
		/*
			expectedServiceEntriesForRollout = map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test." + deployment1Identity + ".mesh": &istioNetworkingV1Alpha3.ServiceEntry{
					Hosts:     []string{"test." + rollout1Identity + ".mesh"},
					Addresses: []string{"127.0.0.1"},
					Ports: []*istioNetworkingV1Alpha3.Port{
						&istioNetworkingV1Alpha3.Port{
							Number:   80,
							Protocol: "http",
							Name:     "http",
						},
					},
					Location:   istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution: istioNetworkingV1Alpha3.ServiceEntry_DNS,
					Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
						&istioNetworkingV1Alpha3.WorkloadEntry{
							Address: "dummy.admiral.global",
							Ports: map[string]uint32{
								"http": 0,
							},
							Locality: "us-west-2",
						},
					},
					SubjectAltNames: []string{"spiffe://prefix/" + rollout1Identity},
				},
			}
		*/
		serviceEntryAddressStore = &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{
				"test." + deployment1Identity + ".mesh-se": "127.0.0.1",
				"test." + rollout1Identity + ".mesh-se":    "127.0.0.1",
			},
			Addresses: []string{},
		}
		serviceForRollout = &coreV1.Service{
			ObjectMeta: v12.ObjectMeta{
				Name:      foobarMetadataName + "-stable",
				Namespace: foobarMetadataNamespace,
			},
			Spec: coreV1.ServiceSpec{
				Selector: map[string]string{"app": rollout1Identity},
				Ports: []coreV1.ServicePort{
					{
						Name: "http",
						Port: 8090,
					},
				},
			},
		}
		serviceForDeployment = &coreV1.Service{
			ObjectMeta: v12.ObjectMeta{
				Name:      foobarMetadataName,
				Namespace: foobarMetadataNamespace,
			},
			Spec: coreV1.ServiceSpec{
				Selector: map[string]string{"app": deployment1Identity},
				Ports: []coreV1.ServicePort{
					{
						Name: "http",
						Port: 8090,
					},
				},
			},
		}
		rr1, _ = InitAdmiral(context.Background(), admiralParamsForServiceEntryTests())
		rr2, _ = InitAdmiral(context.Background(), admiralParamsForServiceEntryTests())
	)
	deploymentController, err := admiral.NewDeploymentController(clusterID, make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300))
	if err != nil {
		t.Fail()
	}
	deploymentController.Cache.UpdateDeploymentToClusterCache(deployment1Identity, testDeployment1)
	rolloutController, err := admiral.NewRolloutsController(clusterID, make(chan struct{}), &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300))
	if err != nil {
		t.Fail()
	}
	rolloutController.Cache.UpdateRolloutToClusterCache(rollout1Identity, &testRollout1)
	serviceController, err := admiral.NewServiceController(clusterID, stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))
	if err != nil {
		t.Fatalf("%v", err)
	}
	virtualServiceController, err := istio.NewVirtualServiceController(clusterID, make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, time.Second*time.Duration(300))
	if err != nil {
		t.Fatalf("%v", err)
	}
	gtpc, err := admiral.NewGlobalTrafficController("", make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, time.Second*time.Duration(300))
	if err != nil {
		t.Fatalf("%v", err)
		t.FailNow()
	}
	t.Logf("expectedServiceEntriesForDeployment: %v\n", expectedServiceEntriesForDeployment)
	serviceController.Cache.Put(serviceForRollout)
	serviceController.Cache.Put(serviceForDeployment)
	rc := &RemoteController{
		ClusterID:                clusterID,
		DeploymentController:     deploymentController,
		RolloutController:        rolloutController,
		ServiceController:        serviceController,
		VirtualServiceController: virtualServiceController,
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
		},
		GlobalTraffic: gtpc,
	}
	rr1.PutRemoteController(clusterID, rc)
	rr1.ExcludedIdentityMap = map[string]bool{
		"asset1": true,
	}
	rr1.StartTime = time.Now()
	rr1.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	rr2.PutRemoteController(clusterID, rc)
	rr2.StartTime = time.Now()
	rr2.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	testCases := []struct {
		name                   string
		assetIdentity          string
		remoteRegistry         *RemoteRegistry
		expectedServiceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry
	}{
		{
			name: "Given asset is using a deployment," +
				"And asset is in the exclude list, " +
				"When modifyServiceEntryForNewServiceOrPod is called, " +
				"Then, it should skip creating service entries, and return an empty map of service entries",
			assetIdentity:          "asset1",
			remoteRegistry:         rr1,
			expectedServiceEntries: nil,
		},
		{
			name: "Given asset is using a rollout," +
				"And asset is in the exclude list, " +
				"When modifyServiceEntryForNewServiceOrPod is called, " +
				"Then, it should skip creating service entries, and return an empty map of service entries",
			assetIdentity:          "asset1",
			remoteRegistry:         rr1,
			expectedServiceEntries: nil,
		},
		{
			name: "Given asset is using a deployment, " +
				"And asset is NOT in the exclude list" +
				"When modifyServiceEntryForNewServiceOrPod is called, " +
				"Then, corresponding service entry should be created, " +
				"And the function should return a map containing the created service entry",
			assetIdentity:          deployment1Identity,
			remoteRegistry:         rr2,
			expectedServiceEntries: expectedServiceEntriesForDeployment,
		},
		/*
			{
				name: "Given asset is using a rollout, " +
					"And asset is NOT in the exclude list" +
					"When modifyServiceEntryForNewServiceOrPod is called, " +
					"Then, corresponding service entry should be created, " +
					"And the function should return a map containing the created service entry",
				assetIdentity:          rollout1Identity,
				remoteRegistry:         rr2,
				expectedServiceEntries: expectedServiceEntriesForRollout,
			},
		*/
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			serviceEntries := modifyServiceEntryForNewServiceOrPod(
				context.Background(),
				admiral.Add,
				env,
				c.assetIdentity,
				c.remoteRegistry,
			)
			if len(serviceEntries) != len(c.expectedServiceEntries) {
				t.Fatalf("expected service entries to be of length: %d, but got: %d", len(c.expectedServiceEntries), len(serviceEntries))
			}
			if len(c.expectedServiceEntries) > 0 {
				for k := range c.expectedServiceEntries {
					if serviceEntries[k] == nil {
						t.Fatalf(
							"expected service entries to contain service entry for: %s, "+
								"but did not find it. Got map: %v",
							k, serviceEntries,
						)
					}
				}
			}
		})
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
	gtpCache.mutex = &sync.Mutex{}
	admiralCache.GlobalTrafficCache = gtpCache

	se := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{"dev.bar.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	emptyEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"dev.bar.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{},
	}

	dummyEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{"dev.dummy.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	seConfig := v1alpha3.ServiceEntry{
		//nolint
		Spec: se,
	}
	seConfig.Name = "se1"
	seConfig.Namespace = "admiral-sync"

	dummySeConfig := v1alpha3.ServiceEntry{
		//nolint
		Spec: dummyEndpointSe,
	}
	dummySeConfig.Name = "dummySe"
	dummySeConfig.Namespace = "admiral-sync"

	ctx := context.Background()

	fakeIstioClient := istiofake.NewSimpleClientset()
	fakeIstioClient.NetworkingV1alpha3().ServiceEntries("admiral-sync").Create(ctx, &seConfig, v12.CreateOptions{})
	fakeIstioClient.NetworkingV1alpha3().ServiceEntries("admiral-sync").Create(ctx, &dummySeConfig, v12.CreateOptions{})

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
	rr := NewRemoteRegistry(nil, common.AdmiralParams{})
	rr.PutRemoteController("cl1", rc)
	rr.AdmiralCache = &admiralCache
	AddServiceEntriesWithDr(ctx, rr, map[string]string{"cl1": "cl1"}, map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &se})
	AddServiceEntriesWithDr(ctx, rr, map[string]string{"cl1": "cl1"}, map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &emptyEndpointSe})
	AddServiceEntriesWithDr(ctx, rr, map[string]string{"cl1": "cl1"}, map[string]*istioNetworkingV1Alpha3.ServiceEntry{"dummySe": &dummyEndpointSe})

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

	se := &istioNetworkingV1Alpha3.ServiceEntry{
		Addresses: []string{"240.10.1.0"},
		Hosts:     []string{host},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
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
		se       *istioNetworkingV1Alpha3.ServiceEntry
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
	ctx := context.Background()
	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := createSeAndDrSetFromGtp(ctx, c.env, c.locality, c.se, c.gtp, &admiralCache)
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
	rr.StartTime = time.Now().Add(-60 * time.Second)

	config := rest.Config{
		Host: "localhost",
	}

	d, e := admiral.NewDeploymentController("", make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300))

	r, e := admiral.NewRolloutsController("test", make(chan struct{}), &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300))

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

	rr.PutRemoteController("test.cluster", rc)
	modifyServiceEntryForNewServiceOrPod(context.Background(), admiral.Add, "test", "bar", rr)

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
	ctx := context.Background()
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			seAddress, needsCacheUpdate, err := GetLocalAddressForSe(ctx, c.seName, &c.seAddressCache, c.cacheController)
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
	ctx := context.Background()

	modifySidecarForLocalClusterCommunication(ctx, "test-sidecar-namespace", sidecarEgressMap, remoteController)

	sidecarObj, err := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars("test-sidecar-namespace").Get(ctx, common.GetWorkloadSidecarName(), v12.GetOptions{})
	if err == nil {
		t.Errorf("expected 404 not found error but got nil")
	}

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

	istioEgress := istioNetworkingV1Alpha3.IstioEgressListener{
		Hosts: []string{"test-host"},
	}

	existingSidecarObj.Spec = istioNetworkingV1Alpha3.Sidecar{
		Egress: []*istioNetworkingV1Alpha3.IstioEgressListener{&istioEgress},
	}

	ctx := context.Background()
	createdSidecar, err := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars("test-sidecar-namespace").Create(ctx, existingSidecarObj, v12.CreateOptions{})
	if err != nil {
		t.Error(err)
	}
	if createdSidecar != nil {

		sidecarEgressMap := make(map[string]common.SidecarEgress)
		sidecarEgressMap["test-dependency-namespace"] = common.SidecarEgress{Namespace: "test-dependency-namespace", FQDN: "test-local-fqdn", CNAMEs: map[string]string{"test.myservice.global": "1"}}
		modifySidecarForLocalClusterCommunication(ctx, "test-sidecar-namespace", sidecarEgressMap, remoteController)

		updatedSidecar, err := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars("test-sidecar-namespace").Get(ctx, "default", v12.GetOptions{})

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

		if !cmp.Equal(updatedSidecar, createdSidecar, protocmp.Transform()) {
			t.Fatalf("Modify existing sidecar failed as configuration is not same. Details - %v", cmp.Diff(updatedSidecar, createdSidecar))
		}
		var matched *istioNetworkingV1Alpha3.IstioEgressListener
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
				if !cmp.Equal(listener, matched, protocmp.Transform()) {
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
	s, e := admiral.NewServiceController("test", stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))

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

	se := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		},
	}

	oneEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		},
	}

	twoEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}

	threeEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}
	eastEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}

	emptyEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints:       []*istioNetworkingV1Alpha3.WorkloadEntry{},
	}

	grpcSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "grpc", Protocol: "grpc"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
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
		serviceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry
		expectedResult *istioNetworkingV1Alpha3.ServiceEntry
	}{
		{
			name:           "Should return a created service entry with grpc protocol",
			action:         admiral.Add,
			rc:             rc,
			admiralCache:   admiralCache,
			meshPorts:      map[string]uint32{"grpc": uint32(80)},
			deployment:     deployment,
			serviceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{},
			expectedResult: &grpcSe,
		},
		{
			name:           "Should return a created service entry with http protocol",
			action:         admiral.Add,
			rc:             rc,
			admiralCache:   admiralCache,
			meshPorts:      map[string]uint32{"http": uint32(80)},
			deployment:     deployment,
			serviceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{},
			expectedResult: &se,
		},
		{
			name:         "Delete the service entry with one endpoint",
			action:       admiral.Delete,
			rc:           rc,
			admiralCache: admiralCache,
			meshPorts:    map[string]uint32{"http": uint32(80)},
			deployment:   deployment,
			serviceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
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
			serviceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
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
			serviceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"e2e.my-first-service.mesh": &threeEndpointSe,
			},
			expectedResult: &eastEndpointSe,
		},
	}

	ctx := context.Background()

	//Run the test for every provided case
	for _, c := range deploymentSeCreationTestCases {
		t.Run(c.name, func(t *testing.T) {
			createdSE := createServiceEntryForDeployment(ctx, c.action, c.rc, &c.admiralCache, c.meshPorts, &c.deployment, c.serviceEntries)
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
		expectedResult *istioNetworkingV1Alpha3.ServiceEntry
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
			createdSE := createServiceEntryForRollout(ctx, admiral.Add, c.rc, &c.admiralCache, c.meshPorts, &c.rollout, map[string]*istioNetworkingV1Alpha3.ServiceEntry{})
			if !reflect.DeepEqual(createdSE, c.expectedResult) {
				t.Errorf("Test %s failed, expected: %v got %v", c.name, c.expectedResult, createdSE)
			}
		})
	}

}

func TestCreateServiceEntryForNewServiceOrPodRolloutsUsecase(t *testing.T) {
	const (
		namespace                  = "test-test"
		serviceName                = "serviceNameActive"
		rolloutPodHashLabel string = "rollouts-pod-template-hash"
	)
	ctx := context.Background()
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	rr, _ := InitAdmiral(context.Background(), p)

	rr.StartTime = time.Now().Add(-60 * time.Second)

	config := rest.Config{
		Host: "localhost",
	}

	d, e := admiral.NewDeploymentController("", make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300))
	if e != nil {
		t.Fail()
	}
	r, e := admiral.NewRolloutsController("test", make(chan struct{}), &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300))
	if e != nil {
		t.Fail()
	}
	v, e := istio.NewVirtualServiceController("", make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, time.Second*time.Duration(300))
	if e != nil {
		t.Fail()
	}
	s, e := admiral.NewServiceController("test", make(chan struct{}), &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))
	if e != nil {
		t.Fail()
	}
	gtpc, e := admiral.NewGlobalTrafficController("", make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, time.Second*time.Duration(300))
	if e != nil {
		t.Fail()
	}
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
		GlobalTraffic:            gtpc,
	}
	rc.ClusterID = "test.cluster"
	rr.PutRemoteController("test.cluster", rc)

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
		WorkloadSelectorCache:      common.NewMapOfMaps(),
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

	rollout.Namespace = namespace
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
	selectorMap[rolloutPodHashLabel] = "hash"

	activeService := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	activeService.Name = serviceName
	activeService.Namespace = namespace
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
	se := modifyServiceEntryForNewServiceOrPod(ctx, admiral.Add, "test", "bar", rr)
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

	ctx := context.Background()

	p := common.AdmiralParams{
		KubeconfigPath:        "testdata/fake.config",
		PreviewHostnamePrefix: "preview",
	}
	rr, _ := InitAdmiral(context.Background(), p)
	config := rest.Config{
		Host: "localhost",
	}
	rr.StartTime = time.Now().Add(-60 * time.Second)

	d, e := admiral.NewDeploymentController("", make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300))

	r, e := admiral.NewRolloutsController("test", make(chan struct{}), &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300))
	v, e := istio.NewVirtualServiceController("", make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, time.Second*time.Duration(300))

	if e != nil {
		t.Fail()
	}
	s, e := admiral.NewServiceController("test", make(chan struct{}), &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))
	gtpc, e := admiral.NewGlobalTrafficController("", make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, time.Second*time.Duration(300))

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
		GlobalTraffic:            gtpc,
	}
	rc.ClusterID = "test.cluster"
	rr.PutRemoteController("test.cluster", rc)

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
		WorkloadSelectorCache:      common.NewMapOfMaps(),
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

	se := modifyServiceEntryForNewServiceOrPod(ctx, admiral.Add, "test", "bar", rr)

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

	se = modifyServiceEntryForNewServiceOrPod(ctx, admiral.Add, "test", "bar", rr)

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

	rollout := &argo.Rollout{}
	rollout.Spec.Strategy = argo.RolloutStrategy{
		BlueGreen: &argo.BlueGreenStrategy{
			ActiveService:  ACTIVE_SERVICE,
			PreviewService: PREVIEW_SERVICE,
		},
	}
	rollout.Spec.Template.Annotations = map[string]string{}
	rollout.Spec.Template.Annotations[common.SidecarEnabledPorts] = "8080"

	endpoint := &istioNetworkingV1Alpha3.WorkloadEntry{
		Labels: map[string]string{}, Address: CLUSTER_INGRESS_1, Ports: map[string]uint32{"http": 15443},
	}

	meshPorts := map[string]uint32{"http": 8080}

	weightedServices := map[string]*WeightedService{
		ACTIVE_SERVICE:  {Service: &v1.Service{ObjectMeta: v12.ObjectMeta{Name: ACTIVE_SERVICE, Namespace: NAMESPACE}}},
		PREVIEW_SERVICE: {Service: &v1.Service{ObjectMeta: v12.ObjectMeta{Name: PREVIEW_SERVICE, Namespace: NAMESPACE}}},
	}

	activeWantedEndpoints := &istioNetworkingV1Alpha3.WorkloadEntry{
		Address: ACTIVE_SERVICE + common.Sep + NAMESPACE + common.DotLocalDomainSuffix, Ports: meshPorts,
	}

	previewWantedEndpoints := &istioNetworkingV1Alpha3.WorkloadEntry{
		Address: PREVIEW_SERVICE + common.Sep + NAMESPACE + common.DotLocalDomainSuffix, Ports: meshPorts,
	}

	testCases := []struct {
		name             string
		rollout          *argo.Rollout
		inputEndpoint    *istioNetworkingV1Alpha3.WorkloadEntry
		weightedServices map[string]*WeightedService
		clusterIngress   string
		meshPorts        map[string]uint32
		meshHost         string
		wantedEndpoints  *istioNetworkingV1Alpha3.WorkloadEntry
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
			updateEndpointsForBlueGreen(c.rollout, c.weightedServices, map[string]string{}, c.inputEndpoint, "test", c.meshHost)
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

	se := &istioNetworkingV1Alpha3.ServiceEntry{
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
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

	wantedEndpoints := []*istioNetworkingV1Alpha3.WorkloadEntry{
		{Address: CLUSTER_INGRESS_2, Weight: 10, Ports: map[string]uint32{"http": 15443}},
		{Address: STABLE_SERVICE + common.Sep + NAMESPACE + common.DotLocalDomainSuffix, Weight: 90, Ports: meshPorts},
		{Address: CANARY_SERVICE + common.Sep + NAMESPACE + common.DotLocalDomainSuffix, Weight: 10, Ports: meshPorts},
	}

	wantedEndpointsZeroWeights := []*istioNetworkingV1Alpha3.WorkloadEntry{
		{Address: CLUSTER_INGRESS_2, Weight: 10, Ports: map[string]uint32{"http": 15443}},
		{Address: STABLE_SERVICE + common.Sep + NAMESPACE + common.DotLocalDomainSuffix, Weight: 100, Ports: meshPorts},
	}

	testCases := []struct {
		name              string
		inputServiceEntry *istioNetworkingV1Alpha3.ServiceEntry
		weightedServices  map[string]*WeightedService
		clusterIngress    string
		meshPorts         map[string]uint32
		wantedEndpoints   []*istioNetworkingV1Alpha3.WorkloadEntry
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

func TestUpdateGlobalGtpCache(t *testing.T) {
	setupForServiceEntryTests()
	var (
		admiralCache = &AdmiralCache{GlobalTrafficCache: &globalTrafficCache{identityCache: make(map[string]*v13.GlobalTrafficPolicy), mutex: &sync.Mutex{}}}
		identity1    = "identity1"
		envStage     = "stage"

		gtp = &v13.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp", Namespace: "namespace1", CreationTimestamp: v12.NewTime(time.Now().Add(time.Duration(-30))), Labels: map[string]string{"identity": identity1, "env": envStage}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hello"}},
		}}

		gtp2 = &v13.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp2", Namespace: "namespace1", CreationTimestamp: v12.NewTime(time.Now().Add(time.Duration(-15))), Labels: map[string]string{"identity": identity1, "env": envStage}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp2"}},
		}}

		gtp7 = &v13.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp7", Namespace: "namespace1", CreationTimestamp: v12.NewTime(time.Now().Add(time.Duration(-45))), Labels: map[string]string{"identity": identity1, "env": envStage, "priority": "2"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp7"}},
		}}

		gtp3 = &v13.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp3", Namespace: "namespace2", CreationTimestamp: v12.NewTime(time.Now()), Labels: map[string]string{"identity": identity1, "env": envStage}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp3"}},
		}}

		gtp4 = &v13.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp4", Namespace: "namespace1", CreationTimestamp: v12.NewTime(time.Now().Add(time.Duration(-30))), Labels: map[string]string{"identity": identity1, "env": envStage, "priority": "10"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp4"}},
		}}

		gtp5 = &v13.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp5", Namespace: "namespace1", CreationTimestamp: v12.NewTime(time.Now().Add(time.Duration(-15))), Labels: map[string]string{"identity": identity1, "env": envStage, "priority": "2"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp5"}},
		}}

		gtp6 = &v13.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp6", Namespace: "namespace3", CreationTimestamp: v12.NewTime(time.Now()), Labels: map[string]string{"identity": identity1, "env": envStage, "priority": "1000"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp6"}},
		}}
	)

	testCases := []struct {
		name        string
		identity    string
		env         string
		gtps        map[string][]*v13.GlobalTrafficPolicy
		expectedGtp *v13.GlobalTrafficPolicy
	}{
		{
			name:        "Should return nil when no GTP present",
			gtps:        map[string][]*v13.GlobalTrafficPolicy{},
			identity:    identity1,
			env:         envStage,
			expectedGtp: nil,
		},
		{
			name:        "Should return the only existing gtp",
			gtps:        map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp}},
			identity:    identity1,
			env:         envStage,
			expectedGtp: gtp,
		},
		{
			name:        "Should return the gtp recently created within the cluster",
			gtps:        map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp, gtp2}},
			identity:    identity1,
			env:         envStage,
			expectedGtp: gtp2,
		},
		{
			name:        "Should return the gtp recently created from another cluster",
			gtps:        map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp, gtp2}, "c2": {gtp3}},
			identity:    identity1,
			env:         envStage,
			expectedGtp: gtp3,
		},
		{
			name:        "Should return the existing priority gtp within the cluster",
			gtps:        map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp, gtp2, gtp7}},
			identity:    identity1,
			env:         envStage,
			expectedGtp: gtp7,
		},
		{
			name:        "Should return the recently created priority gtp within the cluster",
			gtps:        map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp5, gtp4, gtp, gtp2}},
			identity:    identity1,
			env:         envStage,
			expectedGtp: gtp4,
		},
		{
			name:        "Should return the recently created priority gtp from another cluster",
			gtps:        map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp, gtp2, gtp4, gtp5, gtp7}, "c2": {gtp6}, "c3": {gtp3}},
			identity:    identity1,
			env:         envStage,
			expectedGtp: gtp6,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			updateGlobalGtpCache(admiralCache, c.identity, c.env, c.gtps)
			gtp := admiralCache.GlobalTrafficCache.GetFromIdentity(c.identity, c.env)
			if !reflect.DeepEqual(c.expectedGtp, gtp) {
				t.Errorf("Test %s failed expected gtp: %v got %v", c.name, c.expectedGtp, gtp)
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

func TestIsBlueGreenStrategy(t *testing.T) {
	var (
		emptyRollout                 *argo.Rollout
		rolloutWithBlueGreenStrategy = &argo.Rollout{
			Spec: argo.RolloutSpec{
				Strategy: argo.RolloutStrategy{
					BlueGreen: &argo.BlueGreenStrategy{
						ActiveService: "active",
					},
				},
			},
		}
		rolloutWithCanaryStrategy = &argo.Rollout{
			Spec: argo.RolloutSpec{
				Strategy: argo.RolloutStrategy{
					Canary: &argo.CanaryStrategy{
						CanaryService: "canaryservice",
					},
				},
			},
		}
		rolloutWithNoStrategy = &argo.Rollout{
			Spec: argo.RolloutSpec{},
		}
		rolloutWithEmptySpec = &argo.Rollout{}
	)
	cases := []struct {
		name           string
		rollout        *argo.Rollout
		expectedResult bool
	}{
		{
			name: "Given argo rollout is configured with blue green rollout strategy" +
				"When isBlueGreenStrategy is called" +
				"Then it should return true",
			rollout:        rolloutWithBlueGreenStrategy,
			expectedResult: true,
		},
		{
			name: "Given argo rollout is configured with canary rollout strategy" +
				"When isBlueGreenStrategy is called" +
				"Then it should return false",
			rollout:        rolloutWithCanaryStrategy,
			expectedResult: false,
		},
		{
			name: "Given argo rollout is configured without any rollout strategy" +
				"When isBlueGreenStrategy is called" +
				"Then it should return false",
			rollout:        rolloutWithNoStrategy,
			expectedResult: false,
		},
		{
			name: "Given argo rollout is nil" +
				"When isBlueGreenStrategy is called" +
				"Then it should return false",
			rollout:        emptyRollout,
			expectedResult: false,
		},
		{
			name: "Given argo rollout has an empty Spec" +
				"When isBlueGreenStrategy is called" +
				"Then it should return false",
			rollout:        rolloutWithEmptySpec,
			expectedResult: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := isBlueGreenStrategy(c.rollout)
			if result != c.expectedResult {
				t.Errorf("expected: %t, got: %t", c.expectedResult, result)
			}
		})
	}
}
