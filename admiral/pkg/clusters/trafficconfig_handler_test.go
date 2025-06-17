package clusters

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	log "github.com/sirupsen/logrus"
	"istio.io/api/meta/v1alpha1"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	stop                 = make(chan struct{})
	admiralParams        = common.AdmiralParams{}
	handler              = &TrafficConfigHandler{}
	mockProcessor        = &MockTrafficConfigProcessor{}
	defaultTrafficConfig = v1.TrafficConfig{}
)

const (
	namespace           = "namespace"
	namespaceNoDeps     = "namespace-no-deps"
	namespaceForRollout = "rollout-namespace"
	cluster1            = "cluster1"
	cluster2            = "cluster2"
	identity1           = "identity1"
	identity2           = "identity2"
	identity4           = "identity4"
	ACTIVE_SERVICE      = "activeService"
	PREVIEW_SERVICE     = "previewService"
)

type MockTrafficConfigProcessor struct {
	invocation int
	err        error
}

func (m *MockTrafficConfigProcessor) Process(ctx context.Context, tc *v1.TrafficConfig,
	remoteRegistry *RemoteRegistry, eventType admiral.EventType,
	modifySE ModifySEFunc) error {
	m.invocation++
	return m.err
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	shutdown()
	os.Exit(code)
}

func setup() {
	admiralParams = common.AdmiralParams{
		CacheReconcileDuration:                    1 * time.Minute,
		EnableTrafficConfigProcessingForSlowStart: true,
		LabelSet: &common.LabelSet{
			EnvKey:              "env",
			WorkloadIdentityKey: "identity",
		},
		EnableSAN:                          true,
		SANPrefix:                          "prefix",
		HostnameSuffix:                     "mesh",
		SyncNamespace:                      "ns",
		ClusterRegistriesNamespace:         "default",
		DependenciesNamespace:              "default",
		WorkloadSidecarName:                "default",
		Profile:                            common.AdmiralProfileDefault,
		DependentClusterWorkerConcurrency:  5,
		PreventSplitBrain:                  true,
		VSRoutingGateways:                  []string{"istio-system/passthrough-gateway"},
		EnableVSRoutingInCluster:           true,
		VSRoutingInClusterEnabledResources: map[string]string{"cluster1": identity1},
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	rr := initializeControllers()

	mockProcessor := &MockTrafficConfigProcessor{}
	handler = &TrafficConfigHandler{
		RemoteRegistry:         rr,
		TrafficConfigProcessor: mockProcessor,
	}
	tcEntries := []TrafficConfigEntry{
		{
			Workloadenvs:     []string{"e2e-usw2", "e2e-use2"},
			DurationInterval: "30s",
		},
		{
			Workloadenvs:     []string{"e2e-apse2"},
			DurationInterval: "20s",
		},
	}
	defaultTrafficConfig = getTrafficConfigForAssetAndWorkloadEnv(identity1, "e2e", tcEntries)

}

func initializeControllers() *RemoteRegistry {
	rr := NewRemoteRegistry(nil, common.AdmiralParams{})
	rr.StartTime = time.Now().Add(-60 * time.Second)
	rr.AdmiralCache = &AdmiralCache{
		SlowStartConfigCache:       common.NewMapOfMapOfMaps(),
		IdentityClusterCache:       common.NewMapOfMaps(),
		IdentityDependencyCache:    common.NewMapOfMaps(),
		CnameClusterCache:          common.NewMapOfMaps(),
		CnameIdentityCache:         &sync.Map{},
		CnameDependentClusterCache: common.NewMapOfMaps(),
		GlobalTrafficCache: &globalTrafficCache{
			mutex: &sync.Mutex{},
		},
		ClientConnectionConfigCache: NewClientConnectionConfigCache(),
		DependencyNamespaceCache:    common.NewSidecarEgressMap(),
		SeClusterCache:              common.NewMapOfMaps(),
		DynamoDbEndpointUpdateCache: &sync.Map{},
		ServiceEntryAddressStore: &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{
				"test." + identity1 + ".mesh-se": "127.0.0.1",
			},
			Addresses: []string{},
		},
		OutlierDetectionCache: &outlierDetectionCache{
			identityCache: make(map[string]*v1.OutlierDetection),
			mutex:         &sync.Mutex{},
		},
	}

	cacheController := &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(rr.AdmiralCache.ServiceEntryAddressStore, "123"),
	}

	rr.AdmiralCache.ConfigMapController = cacheController
	config := rest.Config{
		Host: "localhost",
	}

	r, err := admiral.NewRolloutsController(stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		log.Fatalf("failed to initialize rollout controller, err: %v", err)
	}
	rollout := &argo.Rollout{
		Spec: argo.RolloutSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Strategy: argo.RolloutStrategy{
				BlueGreen: &argo.BlueGreenStrategy{
					ActiveService:  ACTIVE_SERVICE,
					PreviewService: PREVIEW_SERVICE,
				},
			},
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"sidecar.istio.io/inject": "true",
						"identity":                identity1,
					},
					Labels: map[string]string{
						"identity": identity1,
						"env":      "e2e-usw2",
					}},
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespaceForRollout,
			Labels: map[string]string{
				"env": "e2e-usw2",
			},
		},
	}

	serviceForRollout := &coreV1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ACTIVE_SERVICE,
			Namespace: namespaceForRollout,
		},
		Spec: coreV1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []coreV1.ServicePort{
				{
					Name: "http",
					Port: 8090,
				},
			},
		},
	}

	entry := &admiral.RolloutClusterEntry{
		Identity: identity1,
		Rollouts: map[string]*admiral.RolloutItem{identity1: {Rollout: rollout}},
	}
	r.Cache = admiral.NewRolloutCache()
	r.Cache.Put(entry)
	r.Cache.UpdateRolloutToClusterCache(identity1, rollout)

	rcCluster1 := &RemoteController{
		ClusterID:         cluster1,
		RolloutController: r,
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
	}

	rcCluster2 := &RemoteController{
		ClusterID:         cluster2,
		RolloutController: r,
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-east-2",
			},
		},
	}
	rr.remoteControllers[cluster1] = rcCluster1
	rr.remoteControllers[cluster2] = rcCluster2

	rr.AdmiralCache.IdentityClusterCache.Put(identity1, cluster1, cluster1)

	serviceController1, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, admiralParams.CacheReconcileDuration, loader.GetFakeClientLoader())
	if err != nil {
		log.Fatalf("%v", err)
	}
	rcCluster1.ServiceController = serviceController1
	serviceController1.Cache.Put(serviceForRollout)

	serviceController2, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, admiralParams.CacheReconcileDuration, loader.GetFakeClientLoader())
	if err != nil {
		log.Fatalf("%v", err)
	}
	rcCluster2.ServiceController = serviceController2

	virtualServiceController, err := istio.NewVirtualServiceController(make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, admiralParams.CacheReconcileDuration, loader.GetFakeClientLoader())
	if err != nil {
		log.Fatalf("%v", err)
	}

	rcCluster1.VirtualServiceController = virtualServiceController
	rcCluster2.VirtualServiceController = virtualServiceController

	destinationRuleController1, err := istio.NewDestinationRuleController(make(<-chan struct{}), &test.MockDestinationRuleHandler{}, cluster1, &config, admiralParams.CacheReconcileDuration, loader.GetFakeClientLoader())
	if err != nil {
		log.Fatalf("%v", err)
	}

	rcCluster1.DestinationRuleController = destinationRuleController1

	destinationRuleController2, err := istio.NewDestinationRuleController(make(<-chan struct{}), &test.MockDestinationRuleHandler{}, cluster2, &config, admiralParams.CacheReconcileDuration, loader.GetFakeClientLoader())
	if err != nil {
		log.Fatalf("%v", err)
	}
	rcCluster2.DestinationRuleController = destinationRuleController2

	serviceEntryController1, err := istio.NewServiceEntryController(make(<-chan struct{}), &test.MockServiceEntryHandler{}, cluster1, &config, admiralParams.CacheReconcileDuration, loader.GetFakeClientLoader())
	if err != nil {
		log.Fatalf("%v", err)
	}

	rcCluster1.ServiceEntryController = serviceEntryController1

	serviceEntryController2, err := istio.NewServiceEntryController(make(<-chan struct{}), &test.MockServiceEntryHandler{}, cluster2, &config, admiralParams.CacheReconcileDuration, loader.GetFakeClientLoader())
	if err != nil {
		log.Fatalf("%v", err)
	}

	rcCluster1.ServiceEntryController = serviceEntryController2

	gtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, admiralParams.CacheReconcileDuration, loader.GetFakeClientLoader())
	if err != nil {
		log.Fatalf("%v", err)
	}
	rcCluster1.GlobalTraffic = gtpc
	rcCluster2.GlobalTraffic = gtpc
	return rr
}

func shutdown() {
	log.Info("shutdown")
}

type TrafficConfigEntry struct {
	Workloadenvs     []string
	DurationInterval string
}

func getTrafficConfigForAssetAndWorkloadEnv(asset string, env string, trafficConfigEntries []TrafficConfigEntry) v1.TrafficConfig {
	// generate

	obj := v1.TrafficConfig{}
	obj.ObjectMeta.Labels = make(map[string]string)
	obj.ObjectMeta.Labels["asset"] = asset
	obj.ObjectMeta.Labels["env"] = env

	var slowStartConfig []*v1.SlowStartConfig
	for _, entry := range trafficConfigEntries {
		slowStartEntry := v1.SlowStartConfig{
			Duration: entry.DurationInterval,
		}

		for _, workloadEnv := range entry.Workloadenvs {
			slowStartEntry.WorkloadEnvSelectors = append(slowStartEntry.WorkloadEnvSelectors, workloadEnv)
			handler.RemoteRegistry.AdmiralCache.SlowStartConfigCache.Put(asset, env, workloadEnv, entry.DurationInterval)
		}
		slowStartConfig = append(slowStartConfig, &slowStartEntry)

	}

	obj.Spec.EdgeService = &v1.EdgeService{}
	obj.Spec.EdgeService.SlowStartConfig = slowStartConfig

	return obj

}

func TestTrafficConfigHandler_Added_WhenNoSourceClusterExistsForIdentity(t *testing.T) {
	admiralParams.EnableTrafficConfigProcessingForSlowStart = true
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	initializeControllers()
	err := handler.Added(context.Background(), &defaultTrafficConfig)

	// Verify the processor was invoked and no error returned
	assert.NoError(t, err)
	destinationRuleControllerCache := handler.RemoteRegistry.remoteControllers[cluster1].DestinationRuleController.Cache
	drNotExpected := destinationRuleControllerCache.Get(identity1, common.NamespaceIstioSystem)
	assert.Panics(t, func() {
		getDrLabels(drNotExpected)
	}, "This should have panicked")
}

func getDrLabels(dr *v1alpha3.DestinationRule) map[string]string {
	return dr.ObjectMeta.Labels
}

func TestTrafficConfigHandler_Added(t *testing.T) {
	inClusterDr := &v1alpha3.DestinationRule{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s.svc.cluster.local-incluster-dr", namespaceForRollout),
			Namespace: common.NamespaceIstioSystem,
		},
		Spec: networkingV1Alpha3.DestinationRule{
			Host: "",
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					WarmupDurationSecs: &duration.Duration{
						Seconds: 0,
					},
				},
			},
			Subsets:          nil,
			ExportTo:         nil,
			WorkloadSelector: nil,
		},
		Status: v1alpha1.IstioStatus{},
	}
	handler.RemoteRegistry.remoteControllers[cluster1].DestinationRuleController.Cache.Put(inClusterDr)
	admiralParams.EnableTrafficConfigProcessingForSlowStart = false
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	handler.RemoteRegistry.AdmiralCache.IdentityDependencyCache.Put(identity1, cluster1, cluster1)
	err := handler.Added(context.Background(), &defaultTrafficConfig)
	assert.NoError(t, err)

	destinationRuleControllerCache := handler.RemoteRegistry.remoteControllers[cluster1].DestinationRuleController.Cache
	dr := destinationRuleControllerCache.Get(fmt.Sprintf("%s.svc.cluster.local-incluster-dr", namespaceForRollout), common.NamespaceIstioSystem)
	// validate that the warmupDuration is set to default when trafficConfig processing is not enabled
	assert.Equal(t, common.GetDefaultWarmupDurationSecs(), dr.Spec.TrafficPolicy.LoadBalancer.WarmupDurationSecs.Seconds)

	admiralParams.EnableTrafficConfigProcessingForSlowStart = true
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	initializeControllers()
	handler.RemoteRegistry.AdmiralCache.IdentityDependencyCache.Put(identity1, cluster1, cluster1)
	err = handler.Added(context.Background(), &defaultTrafficConfig)
	assert.NoError(t, err)

	destinationRuleControllerCache = handler.RemoteRegistry.remoteControllers[cluster1].DestinationRuleController.Cache
	dr = destinationRuleControllerCache.Get(fmt.Sprintf("%s.svc.cluster.local-incluster-dr", namespaceForRollout), common.NamespaceIstioSystem)
	// Validate that the DR was updated in the cache.
	assert.Equal(t, int64(30), dr.Spec.TrafficPolicy.LoadBalancer.WarmupDurationSecs.Seconds)

	routingDr := destinationRuleControllerCache.Get(fmt.Sprintf("%s.svc.cluster.local-routing-dr", namespaceForRollout), common.NamespaceIstioSystem)
	assert.Panics(t, func() { getDrLabels(routingDr) }, "The routing DR should not exist if EnableVSRouting is false")

	// Set VS based routing to true. We should get the routing DR as well this time.
	admiralParams.EnableVSRouting = true
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	initializeControllers()
	handler.RemoteRegistry.AdmiralCache.IdentityDependencyCache.Put(identity1, cluster1, cluster1)
	err = handler.Updated(context.Background(), &defaultTrafficConfig)
	assert.NoError(t, err)
	destinationRuleControllerCache = handler.RemoteRegistry.remoteControllers[cluster1].DestinationRuleController.Cache
	dr = destinationRuleControllerCache.Get(fmt.Sprintf("%s.svc.cluster.local-routing-dr", namespaceForRollout), common.NamespaceIstioSystem)
	// Validate that the DR was updated in the cache.
	assert.Equal(t, int64(30), dr.Spec.TrafficPolicy.LoadBalancer.WarmupDurationSecs.Seconds)
	//validate that the warmupDuration for default DR is not impacted by this.
	routingDr = destinationRuleControllerCache.Get(fmt.Sprintf("%s.svc.cluster.local-routing-dr", namespaceForRollout), common.NamespaceIstioSystem)
	getDrLabels(routingDr)
	assert.Equal(t, int64(30), routingDr.Spec.TrafficPolicy.LoadBalancer.WarmupDurationSecs.Seconds)

	disabledTrafficConfig := defaultTrafficConfig.DeepCopy()
	disabledTrafficConfig.Annotations = make(map[string]string)
	disabledTrafficConfig.Annotations["isSlowStartDisabled"] = "true"
	err = handler.Updated(context.Background(), disabledTrafficConfig)
	assert.NoError(t, err)
	destinationRuleControllerCache = handler.RemoteRegistry.remoteControllers[cluster1].DestinationRuleController.Cache
	dr = destinationRuleControllerCache.Get(fmt.Sprintf("%s.svc.cluster.local-routing-dr", namespaceForRollout), common.NamespaceIstioSystem)
	// Validate that the DR was updated in the cache.
	assert.Equal(t, common.GetDefaultWarmupDurationSecs(), dr.Spec.TrafficPolicy.LoadBalancer.WarmupDurationSecs.Seconds)
	//validate that the warmupDuration for default DR is not impacted by this.
	routingDr = destinationRuleControllerCache.Get(fmt.Sprintf("%s.svc.cluster.local-routing-dr", namespaceForRollout), common.NamespaceIstioSystem)
	getDrLabels(routingDr)
	assert.Equal(t, common.GetDefaultWarmupDurationSecs(), routingDr.Spec.TrafficPolicy.LoadBalancer.WarmupDurationSecs.Seconds)

}
func TestGetTrafficConfigLabel(t *testing.T) {
	testCases := []struct {
		name          string
		labels        map[string]string
		key           string
		expectedValue string
	}{
		{
			name: "Label exists",
			labels: map[string]string{
				"admiral.io/env":   "test-env",
				"admiral.io/asset": "test-asset",
			},
			key:           "admiral.io/asset",
			expectedValue: "test-asset",
		},
		{
			name: "Label doesn't exist",
			labels: map[string]string{
				"admiral.io/env": "test-env",
			},
			key:           "admiral.io/asset",
			expectedValue: "",
		},
		{
			name:          "Empty labels",
			labels:        map[string]string{},
			key:           "admiral.io/asset",
			expectedValue: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getTrafficConfigLabel(tc.labels, tc.key)
			assert.Equal(t, tc.expectedValue, result)
		})
	}
}

// Mock for modifyServiceEntryForNewServiceOrPod
var originalModifyServiceEntryFunc = modifyServiceEntryForNewServiceOrPodMock

func modifyServiceEntryForNewServiceOrPodMock(ctx context.Context, event admiral.EventType, env string,
	sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*networkingV1Alpha3.ServiceEntry, error) {
	// Check if the test is expecting this call for validation
	if ctx.Value("modifyServiceEntryInvoked") != nil {
		modifyServiceEntryInvoked := ctx.Value("modifyServiceEntryInvoked").(*bool)
		*modifyServiceEntryInvoked = true
	}
	if ctx.Value("modifyServiceEntrySourceEnv") != nil {
		modifyServiceEntrySourceEnv := ctx.Value("modifyServiceEntrySourceEnv").(*string)
		*modifyServiceEntrySourceEnv = env
	}
	if ctx.Value("modifyServiceEntryAssetAlias") != nil {
		modifyServiceEntryAssetAlias := ctx.Value("modifyServiceEntryAssetAlias").(*string)
		*modifyServiceEntryAssetAlias = sourceIdentity
	}
	return nil, nil
}

func TestHandleTrafficConfigRecord(t *testing.T) {
	admiralParams := common.AdmiralParams{
		EnableTrafficConfigProcessingForSlowStart: true,
		CacheReconcileDuration:                    5 * time.Minute,
		LabelSet: &common.LabelSet{
			EnvKey: "env",
		},
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	identityClusterCache := common.NewMapOfMaps()
	identityClusterCache.Put("test-asset", "cluster1", "cluster1")

	slowStartCache := common.NewMapOfMapOfMaps()
	slowStartCache.Put("test-asset", "test-env", "production", "30")

	modifyServiceEntryInvoked := false
	modifyServiceEntrySourceEnv := ""
	modifyServiceEntryAssetAlias := ""

	remoteRegistry := NewRemoteRegistry(nil, admiralParams)

	remoteRegistry.AdmiralCache.IdentityClusterCache = identityClusterCache
	remoteRegistry.AdmiralCache.SlowStartConfigCache = slowStartCache

	rcCluster1 := &RemoteController{
		ClusterID: cluster1,
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
	}

	remoteRegistry.remoteControllers[cluster1] = rcCluster1

	slowStartConfigs := common.NewMap()
	slowStartConfigs.Put("test-workload-env", "30")
	testCases := []struct {
		name                                 string
		trafficConfig                        *v1.TrafficConfig
		cacheWarmupTime                      bool
		enableTrafficConfigProcessing        bool
		expectedModifyServiceEntryInvoked    bool
		expectedModifyServiceEntrySourceEnv  string
		expectedModifyServiceEntryAssetAlias string
	}{
		{
			name: "Cache warmup time - skip processing",
			trafficConfig: &v1.TrafficConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-config",
					Labels: map[string]string{
						common.TrafficConfigAssetLabelKey: "test-asset",
						common.TrafficConfigEnvLabelKey:   "test-env",
					},
				},
				Spec: v1.TrafficConfigSpec{
					EdgeService: &v1.EdgeService{
						SlowStartConfig: []*v1.SlowStartConfig{
							{
								WorkloadEnvSelectors: []string{"production"},
								Duration:             "30s",
							},
						},
					},
				},
			},
			cacheWarmupTime:                   true,
			enableTrafficConfigProcessing:     true,
			expectedModifyServiceEntryInvoked: false,
		},
		{
			name: "Traffic config processing disabled",
			trafficConfig: &v1.TrafficConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-config",
					Labels: map[string]string{
						common.TrafficConfigAssetLabelKey: "test-asset",
						common.TrafficConfigEnvLabelKey:   "test-env",
					},
				},
				Spec: v1.TrafficConfigSpec{
					EdgeService: &v1.EdgeService{
						SlowStartConfig: []*v1.SlowStartConfig{
							{
								WorkloadEnvSelectors: []string{"production"},
								Duration:             "30s",
							},
						},
					},
				},
			},
			cacheWarmupTime:                   false,
			enableTrafficConfigProcessing:     false,
			expectedModifyServiceEntryInvoked: false,
		},
		{
			name: "No slow start config",
			trafficConfig: &v1.TrafficConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-config",
					Labels: map[string]string{
						common.TrafficConfigAssetLabelKey: "test-asset",
						common.TrafficConfigEnvLabelKey:   "test-env",
					},
				},
				Spec: v1.TrafficConfigSpec{
					EdgeService: &v1.EdgeService{},
				},
			},
			cacheWarmupTime:                   false,
			enableTrafficConfigProcessing:     true,
			expectedModifyServiceEntryInvoked: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			modifyServiceEntryInvoked = false
			modifyServiceEntrySourceEnv = ""
			modifyServiceEntryAssetAlias = ""

			// Mock cache warmup time if needed
			if tc.cacheWarmupTime {
				remoteRegistry.StartTime = time.Now()
			} else {
				remoteRegistry.StartTime = time.Now().Add(-10 * time.Minute)
			}

			handler := &TrafficConfigHandler{
				RemoteRegistry: remoteRegistry,
			}

			err := handler.HandleTrafficConfigRecord(context.Background(), tc.trafficConfig, remoteRegistry, admiral.Add, slowStartConfigs)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedModifyServiceEntryInvoked, modifyServiceEntryInvoked)

			if tc.expectedModifyServiceEntryInvoked {
				assert.Equal(t, tc.expectedModifyServiceEntrySourceEnv, modifyServiceEntrySourceEnv)
				assert.Equal(t, tc.expectedModifyServiceEntryAssetAlias, modifyServiceEntryAssetAlias)
			}
		})
	}
}

func TestIsCacheWarmupTime(t *testing.T) {
	testCases := []struct {
		name           string
		startTime      time.Time
		expectedResult bool
	}{
		{
			name:           "In warmup period",
			startTime:      time.Now().Add(-30 * time.Second),
			expectedResult: true,
		},
		{
			name:           "Outside warmup period",
			startTime:      time.Now().Add(-10 * time.Minute),
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			remoteRegistry := &RemoteRegistry{
				StartTime: tc.startTime,
			}
			result := IsCacheWarmupTime(remoteRegistry)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
