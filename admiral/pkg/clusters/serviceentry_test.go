package clusters

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"k8s.io/client-go/rest"

	"github.com/golang/protobuf/ptypes/duration"

	"github.com/google/uuid"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"

	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/google/go-cmp/cmp"
	admiralapiv1 "github.com/istio-ecosystem/admiral/admiral/apis/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	admiralV1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	v13 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	istioNetworkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	k8sAppsV1 "k8s.io/api/apps/v1"
	v14 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func admiralParamsForServiceEntryTests() common.AdmiralParams {
	return common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			GatewayApp:              "gatewayapp",
			WorkloadIdentityKey:     "identity",
			PriorityKey:             "priority",
			EnvKey:                  "env",
			AdmiralCRDIdentityLabel: "identity",
		},
		EnableSAN:                         true,
		SANPrefix:                         "prefix",
		HostnameSuffix:                    "mesh",
		SyncNamespace:                     "ns",
		CacheReconcileDuration:            0,
		ClusterRegistriesNamespace:        "default",
		DependenciesNamespace:             "default",
		WorkloadSidecarName:               "default",
		Profile:                           common.AdmiralProfileDefault,
		DependentClusterWorkerConcurrency: 5,
		PreventSplitBrain:                 true,
		VSRoutingGateways:                 []string{"istio-system/passthrough-gateway"},
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
		logrus.Warn("InitializeConfig was NOT called from setupForServiceEntryTests")
	} else {
		logrus.Info("InitializeConfig was called setupForServiceEntryTests")
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
			Labels: map[string]string{
				"identity": identityLabelValue,
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
			Selector: &metav1.LabelSelector{
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
			Labels: map[string]string{
				"identity": identityLabelValue,
			},
		},
		Spec: argo.RolloutSpec{
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
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
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"identity": identityLabelValue,
					"app":      identityLabelValue,
				},
			},
		},
	}
}

func makeGTP(name, namespace, identity, env, dnsPrefix string, creationTimestamp metav1.Time) *v13.GlobalTrafficPolicy {
	return &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
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

func TestModifyServiceEntryForNewServiceOrPodForServiceEntryUpdateSuspension(t *testing.T) {
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
		resyncPeriod                        = time.Millisecond * 1
		expectedServiceEntriesForDeployment = map[string]*istioNetworkingV1Alpha3.ServiceEntry{
			"test." + deployment1Identity + ".mesh": &istioNetworkingV1Alpha3.ServiceEntry{
				Hosts:     []string{"test." + deployment1Identity + ".mesh"},
				Addresses: []string{"127.0.0.1"},
				Ports: []*istioNetworkingV1Alpha3.ServicePort{
					{
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
						Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
					},
				},
				SubjectAltNames: []string{"spiffe://prefix/" + deployment1Identity},
			},
		}
		serviceEntryAddressStore = &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{
				"test." + deployment1Identity + ".mesh-se": "127.0.0.1",
				"test." + rollout1Identity + ".mesh-se":    "127.0.0.1",
			},
			Addresses: []string{},
		}
		serviceForRollout = &coreV1.Service{
			ObjectMeta: metav1.ObjectMeta{
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
			ObjectMeta: metav1.ObjectMeta{
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
		rr3, _ = InitAdmiral(context.Background(), admiralParamsForServiceEntryTests())
	)
	deploymentController, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	deploymentController.Cache.UpdateDeploymentToClusterCache(deployment1Identity, testDeployment1)
	rolloutController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	rolloutController.Cache.UpdateRolloutToClusterCache(rollout1Identity, &testRollout1)
	serviceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	virtualServiceController, err := istio.NewVirtualServiceController(make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	gtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}

	od, err := admiral.NewOutlierDetectionController(make(chan struct{}), &test.MockOutlierDetectionHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
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
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic:              gtpc,
		OutlierDetectionController: od,
	}
	rr1.PutRemoteController(clusterID, rc)
	rr1.ServiceEntrySuspender = NewDefaultServiceEntrySuspender([]string{"asset1"})
	rr1.StartTime = time.Now()
	rr1.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	rr2.PutRemoteController(clusterID, rc)
	rr2.StartTime = time.Now()
	rr2.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	rr3.PutRemoteController(clusterID, rc)
	rr3.StartTime = time.Now()
	rr3.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore
	rr3.AdmiralDatabaseClient = nil

	testCases := []struct {
		name                   string
		assetIdentity          string
		readOnly               bool
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
				"And asset is NOT in the exclude list, " +
				"When modifyServiceEntryForNewServiceOrPod is called, " +
				"Then, corresponding service entry should be created, " +
				"And the function should return a map containing the created service entry",
			assetIdentity:          deployment1Identity,
			remoteRegistry:         rr2,
			expectedServiceEntries: expectedServiceEntriesForDeployment,
		},
		{
			name: "Given asset is using a deployment, " +
				"And asset is NOT in the exclude list and admiral database client is not initialized" +
				"When modifyServiceEntryForNewServiceOrPod is called, " +
				"Then, corresponding service entry should be created, " +
				"And the function should return a map containing the created service entry",
			assetIdentity:          deployment1Identity,
			remoteRegistry:         rr3,
			expectedServiceEntries: expectedServiceEntriesForDeployment,
		},
		{
			name: "Given admiral is running in read only mode," +
				"Service Entries should not get generated",
			assetIdentity:          deployment1Identity,
			remoteRegistry:         rr2,
			readOnly:               true,
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{},
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.readOnly {
				commonUtil.CurrentAdmiralState.ReadOnly = ReadOnlyEnabled
			}

			ctx := context.Background()
			ctx = context.WithValue(ctx, "clusterName", "clusterName")
			ctx = context.WithValue(ctx, "eventResourceType", common.Deployment)
			serviceEntries, _ := modifyServiceEntryForNewServiceOrPod(
				ctx,
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

func TestModifyServiceEntryForRolloutsMultipleEndpointsUseCase(t *testing.T) {
	var (
		env                     = "test"
		stop                    = make(chan struct{})
		foobarMetadataName      = "foobar"
		foobarMetadataNamespace = "foobar-ns"
		rollout1Identity        = "rollout1"
		testRollout1            = makeTestRollout(foobarMetadataName, foobarMetadataNamespace, rollout1Identity)
		testRollout2            = makeTestRollout(foobarMetadataName, foobarMetadataNamespace, rollout1Identity)
		clusterID               = "test-dev-k8s"
		fakeIstioClient         = istiofake.NewSimpleClientset()
		config                  = rest.Config{Host: "localhost"}
		resyncPeriod            = time.Millisecond * 0

		serviceEntryAddressStore = &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{
				"test." + rollout1Identity + ".mesh-se":        "127.0.0.1",
				"canary.test." + rollout1Identity + ".mesh-se": "127.0.0.1",
				"stable.test." + rollout1Identity + ".mesh-se": "127.0.0.1",
			},
			Addresses: []string{},
		}
		serviceEntryAddressStore2 = &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{
				"test." + rollout1Identity + ".mesh-se":        "127.0.0.1",
				"canary.test." + rollout1Identity + ".mesh-se": "127.0.0.1",
			},
			Addresses: []string{},
		}

		serviceEntryAddressStore3 = &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{
				"test." + rollout1Identity + ".mesh-se":        "127.0.0.1",
				"stable.test." + rollout1Identity + ".mesh-se": "127.0.0.1",
			},
			Addresses: []string{},
		}
		serviceForRollout = &coreV1.Service{
			ObjectMeta: metav1.ObjectMeta{
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
		serviceForRolloutCanary = &coreV1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      foobarMetadataName + "-canary",
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
		serviceForRolloutRoot = &coreV1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      foobarMetadataName + "-root",
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
		rr1 = NewRemoteRegistry(nil, admiralParamsForServiceEntryTests())
		rr2 = NewRemoteRegistry(nil, admiralParamsForServiceEntryTests())
		rr3 = NewRemoteRegistry(nil, admiralParamsForServiceEntryTests())
		rr4 = NewRemoteRegistry(nil, admiralParamsForServiceEntryTests())
		rr5 = NewRemoteRegistry(nil, admiralParamsForServiceEntryTests())
		rr6 = NewRemoteRegistry(nil, admiralParamsForServiceEntryTests())
	)

	testRollout2.Spec.Strategy.Canary.TrafficRouting = nil

	vsRoutes := []*istioNetworkingV1Alpha3.HTTPRouteDestination{
		{
			Destination: &istioNetworkingV1Alpha3.Destination{
				Host: foobarMetadataName + "-canary",
				Port: &istioNetworkingV1Alpha3.PortSelector{
					Number: common.DefaultServiceEntryPort,
				},
			},
			Weight: 30,
		},
		{
			Destination: &istioNetworkingV1Alpha3.Destination{
				Host: foobarMetadataName + "-stable",
				Port: &istioNetworkingV1Alpha3.PortSelector{
					Number: common.DefaultServiceEntryPort,
				},
			},
			Weight: 70,
		},
	}

	fooVS := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:   foobarMetadataName + "-canary",
			Labels: map[string]string{"admiral.io/env": "e2e", "identity": "my-first-service"},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{
			Hosts: []string{"stage.test00.foo"},
			Http: []*istioNetworkingV1Alpha3.HTTPRoute{
				{
					Route: vsRoutes,
				},
			},
		},
	}

	_, err := fakeIstioClient.NetworkingV1alpha3().VirtualServices(foobarMetadataNamespace).Create(context.Background(), fooVS, metav1.CreateOptions{})
	if err != nil {
		t.Error(err)
	}

	rolloutController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	rolloutController.Cache.UpdateRolloutToClusterCache(rollout1Identity, &testRollout1)
	rolloutController2, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	rolloutController2.Cache.UpdateRolloutToClusterCache(rollout1Identity, &testRollout2)
	serviceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	serviceController2, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	serviceController3, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	serviceController4, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	gtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
		t.FailNow()
	}
	serviceController.Cache.Put(serviceForRollout)
	serviceController.Cache.Put(serviceForRolloutCanary)
	serviceController.Cache.Put(serviceForRolloutRoot)
	serviceController2.Cache.Put(serviceForRollout)
	serviceController2.Cache.Put(serviceForRolloutCanary)
	serviceController3.Cache.Put(serviceForRollout)
	serviceController4.Cache.Put(serviceForRolloutRoot)
	rc := &RemoteController{
		ClusterID:         clusterID,
		RolloutController: rolloutController,
		ServiceController: serviceController,
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient,
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic: gtpc,
	}
	rc2 := &RemoteController{
		ClusterID:         clusterID,
		RolloutController: rolloutController,
		ServiceController: serviceController2,
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient,
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic: gtpc,
	}
	rc3 := &RemoteController{
		ClusterID:         clusterID,
		RolloutController: rolloutController,
		ServiceController: serviceController3,
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient,
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic: gtpc,
	}
	rc4 := &RemoteController{
		ClusterID:         clusterID,
		RolloutController: rolloutController2,
		ServiceController: serviceController4,
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient,
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic: gtpc,
	}
	cacheWithEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses:      []string{},
	}
	rr1.PutRemoteController(clusterID, rc)
	rr1.StartTime = time.Now()
	rr1.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	rr2.PutRemoteController(clusterID, rc2)
	rr2.StartTime = time.Now()
	rr2.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	rr3.PutRemoteController(clusterID, rc3)
	rr3.StartTime = time.Now()
	rr3.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	rr4.PutRemoteController(clusterID, rc)
	rr4.StartTime = time.Now()
	rr4.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore2
	rr4.AdmiralCache.ConfigMapController = &test.FakeConfigMapController{
		GetError:          errors.New("BAD THINGS HAPPENED"),
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	rr5.PutRemoteController(clusterID, rc)
	rr5.StartTime = time.Now()
	rr5.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore3
	rr5.AdmiralCache.ConfigMapController = &test.FakeConfigMapController{
		GetError:          errors.New("BAD THINGS HAPPENED"),
		PutError:          nil,
		ConfigmapToReturn: nil,
	}

	rr6.PutRemoteController(clusterID, rc4)
	rr6.StartTime = time.Now()
	rr6.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore
	rr6.AdmiralCache.ConfigMapController = &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "clusterName", "test-dev-k8s")
	ctx = context.WithValue(ctx, "eventResourceType", common.Rollout)

	testCases := []struct {
		name                      string
		assetIdentity             string
		remoteRegistry            *RemoteRegistry
		expectedServiceEntriesKey []string
	}{
		{
			name: "Given asset is using a rollout," +
				"When modifyServiceEntryForNewServiceOrPod is called, with 2 services (canary, root) " +
				"Then, it should return a map of 1 service entries ",
			assetIdentity:             "rollout1",
			remoteRegistry:            rr1,
			expectedServiceEntriesKey: []string{"test.rollout1.mesh", "canary.test.rollout1.mesh"},
		},
		{
			name: "Given asset is using a rollout," +
				"When modifyServiceEntryForNewServiceOrPod is called, with 1 service (canary) " +
				"Then, it should return a map of 1 service entries",
			assetIdentity:             "rollout1",
			remoteRegistry:            rr2,
			expectedServiceEntriesKey: []string{"test.rollout1.mesh", "canary.test.rollout1.mesh"},
		},
		{
			name: "Given asset is using a rollout," +
				"When modifyServiceEntryForNewServiceOrPod is called, with 2 services (canary, root) with address generation failure for stable " +
				"Then, it should return a map of 1 service entries",
			assetIdentity:             "rollout1",
			remoteRegistry:            rr4,
			expectedServiceEntriesKey: []string{"test.rollout1.mesh", "canary.test.rollout1.mesh"},
		},
		{
			name: "Given asset is using a rollout," +
				"When modifyServiceEntryForNewServiceOrPod is called, with 3 services (stable, canary, root) with address generation failure for canary " +
				"Then, it should no service entry",
			assetIdentity:             "rollout1",
			remoteRegistry:            rr5,
			expectedServiceEntriesKey: []string{},
		},
		{
			name: "Given asset is using a rollout," +
				"When modifyServiceEntryForNewServiceOrPod is called, with 1 services (root)" +
				"Then, it should return a map of 1 service entry",
			assetIdentity:             "rollout1",
			remoteRegistry:            rr6,
			expectedServiceEntriesKey: []string{"test.rollout1.mesh"},
		},
	}
	for _, c := range testCases {
		setupForServiceEntryTests()
		commonUtil.CurrentAdmiralState.ReadOnly = ReadWriteEnabled
		t.Run(c.name, func(t *testing.T) {
			serviceEntries, _ := modifyServiceEntryForNewServiceOrPod(
				ctx,
				admiral.Add,
				env,
				c.assetIdentity,
				c.remoteRegistry,
			)
			if len(serviceEntries) != len(c.expectedServiceEntriesKey) {
				t.Fatalf("expected service entries to be of length: %d, but got: %d", len(c.expectedServiceEntriesKey), len(serviceEntries))
			}
			if len(c.expectedServiceEntriesKey) > 0 {
				for _, k := range c.expectedServiceEntriesKey {
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

func TestIsGeneratedByAdmiral(t *testing.T) {

	testCases := []struct {
		name           string
		annotations    map[string]string
		expectedResult bool
	}{
		{
			name:           "given nil annotation, and isGeneratedByAdmiral is called, the func should return false",
			annotations:    nil,
			expectedResult: false,
		},
		{
			name:           "given empty annotation, and isGeneratedByAdmiral is called, the func should return false",
			annotations:    map[string]string{},
			expectedResult: false,
		},
		{
			name:           "given a annotations map, and the map does not contain the admiral created by annotation, and isGeneratedByAdmiral is called, the func should return false",
			annotations:    map[string]string{"test": "foobar"},
			expectedResult: false,
		},
		{
			name:           "given a annotations map, and the map contains the admiral created by annotation but value is not admiral, and isGeneratedByAdmiral is called, the func should return false",
			annotations:    map[string]string{resourceCreatedByAnnotationLabel: "foobar"},
			expectedResult: false,
		},
		{
			name:           "given a annotations map, and the map contains the admiral created by annotation, and isGeneratedByAdmiral is called, the func should return true",
			annotations:    map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			expectedResult: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			actual := isGeneratedByAdmiral(tt.annotations)
			if actual != tt.expectedResult {
				t.Errorf("expected %v but got %v", tt.expectedResult, actual)
			}
		})
	}

}

func TestAddServiceEntriesWithDr(t *testing.T) {
	var ctxLogger = logrus.WithFields(logrus.Fields{
		"type": "modifySE",
	})
	admiralCache := AdmiralCache{
		IdentityClusterCache:                common.NewMapOfMaps(),
		CnameDependentClusterNamespaceCache: common.NewMapOfMapOfMaps(),
		PartitionIdentityCache:              common.NewMap(),
	}

	cacheWithNoEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{
			"prefix.e2e.foo.global-se":    "test",
			"sw01.e2e.foo.global-se":      "test",
			"west.sw01.e2e.foo.global-se": "test",
			"east.sw01.e2e.foo.global-se": "test",
		},
		Addresses: []string{},
	}

	admiralCache.DynamoDbEndpointUpdateCache = &sync.Map{}
	admiralCache.DynamoDbEndpointUpdateCache.Store("dev.dummy.global", "")
	admiralCache.SeClusterCache = common.NewMapOfMaps()
	admiralCache.ServiceEntryAddressStore = &cacheWithNoEntry

	cnameIdentityCache := sync.Map{}
	cnameIdentityCache.Store("dev.bar.global", "bar")
	cnameIdentityCache.Store("dev.newse.global", "newse")
	cnameIdentityCache.Store("e2e.foo.global", "foo")
	cnameIdentityCache.Store("preview.dev.newse.global", "newse")
	cnameIdentityCache.Store("e2e.bar.global", "bar")
	admiralCache.CnameIdentityCache = &cnameIdentityCache

	trafficPolicyOverride := &model.TrafficPolicy{
		LbType:    model.TrafficPolicy_FAILOVER,
		DnsPrefix: common.Default,
		Target: []*model.TrafficGroup{
			{
				Region: "us-west-2",
				Weight: 100,
			},
		},
	}

	dnsPrefixedGTP := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dns-prefixed-gtp",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    0,
					DnsPrefix: "default",
				},
				{
					LbType:    1,
					DnsPrefix: "west",
				},
				{
					LbType:    1,
					DnsPrefix: "east",
				},
			},
		},
	}

	defaultGtp := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test.dev.bar-gtp",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				trafficPolicyOverride,
			},
		},
	}

	prefixedTrafficPolicy := &model.TrafficPolicy{
		LbType:    model.TrafficPolicy_TOPOLOGY,
		DnsPrefix: "prefix",
	}

	prefixedGtp := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test.e2e.foo-gtp",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				prefixedTrafficPolicy,
			},
		},
	}

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v13.GlobalTrafficPolicy)
	gtpCache.identityCache["dev.bar"] = defaultGtp
	gtpCache.identityCache["e2e.foo"] = prefixedGtp
	gtpCache.identityCache["sw01.e2e.foo"] = dnsPrefixedGTP
	gtpCache.identityCache["e2e.bar"] = prefixedGtp
	gtpCache.mutex = &sync.Mutex{}
	admiralCache.GlobalTrafficCache = gtpCache

	odCache := &outlierDetectionCache{}
	odCache.identityCache = make(map[string]*v13.OutlierDetection)
	odCache.mutex = &sync.Mutex{}
	admiralCache.OutlierDetectionCache = odCache

	clientConnectionSettingsCache := NewClientConnectionConfigCache()
	admiralCache.ClientConnectionConfigCache = clientConnectionSettingsCache

	dnsPrefixedSE := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"sw01.e2e.foo.global"},
		Addresses: []string{"240.0.0.1"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	newSE := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{"dev.newse.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	newCanarySE := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{"canary.dev.newse.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	newSeWithEmptyHosts := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	newPreviewSE := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{"preview.dev.newse.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}
	newPrefixedSE := istioNetworkingV1Alpha3.ServiceEntry{
		Addresses: []string{"240.10.1.0"},
		Hosts:     []string{"e2e.foo.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	prefixedSE := istioNetworkingV1Alpha3.ServiceEntry{
		Addresses: []string{"240.10.1.0"},
		Hosts:     []string{"e2e.bar.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	prefixedCanarySE := istioNetworkingV1Alpha3.ServiceEntry{
		Addresses: []string{"240.10.1.0"},
		Hosts:     []string{"canary.e2e.bar.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	canarySE := istioNetworkingV1Alpha3.ServiceEntry{
		Addresses: []string{"240.10.1.0"},
		Hosts:     []string{"canary.e2e.bar1.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	se := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{"dev.bar.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	emptyEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"dev.bar.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{},
	}

	dummyEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{"dev.dummy.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	dummyEndpointSeForNonSourceCluster := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{"dev.dummy.non.source.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	userGeneratedSE := v1alpha3.ServiceEntry{
		//nolint
		Spec: istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"dev.custom.global"},
			Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
				{
					Address:  "custom.svc.cluster.local",
					Ports:    map[string]uint32{"http": 80},
					Network:  "mesh1",
					Locality: "us-west",
					Weight:   100,
					Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
				},
			},
		},
	}
	userGeneratedSE.Name = "dev.custom.global-se"
	userGeneratedSE.Namespace = "ns"

	admiralOverrideSE := v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
		},
		//nolint
		Spec: istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"dev.custom.global"},
			Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
				{
					Address:  "override.svc.cluster.local",
					Ports:    map[string]uint32{"http": 80},
					Network:  "mesh1",
					Locality: "us-west",
					Weight:   100,
					Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
				},
			},
		},
	}

	seConfig := v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
		},
		//nolint
		Spec: se,
	}
	seConfig.Name = "dev.bar.global-se"
	seConfig.Namespace = "ns"

	dummySeConfig := v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
		},
		//nolint
		Spec: dummyEndpointSe,
	}
	dummySeConfig.Name = "dev.dummy.global-se"
	dummySeConfig.Namespace = "ns"

	dummySeConfigForNonSourceCluster := v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
		},
		//nolint
		Spec: dummyEndpointSeForNonSourceCluster,
	}
	dummySeConfigForNonSourceCluster.Name = "dev.dummy.non.source.global-se"
	dummySeConfigForNonSourceCluster.Namespace = "ns"

	dummyDRConfig := v1alpha3.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
		},
		Spec: istioNetworkingV1Alpha3.DestinationRule{
			Host: "dev.dummy.global",
		},
	}
	dummyDRConfig.Name = "dev.dummy.global-default-dr"
	dummyDRConfig.Namespace = "ns"

	emptyEndpointDR := v1alpha3.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
		},
		Spec: istioNetworkingV1Alpha3.DestinationRule{
			Host: "dev.bar.global",
		},
	}
	emptyEndpointDR.Name = "dev.bar.global-default-dr"
	emptyEndpointDR.Namespace = "ns"

	userGeneratedDestinationRule := v1alpha3.DestinationRule{
		Spec: istioNetworkingV1Alpha3.DestinationRule{
			Host: "dev.custom.global",
		},
	}
	userGeneratedDestinationRule.Name = "dev.custom.global-default-dr"
	userGeneratedDestinationRule.Namespace = "ns"

	ctx := context.Background()

	fakeIstioClient := istiofake.NewSimpleClientset()
	fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Create(ctx, &seConfig, metav1.CreateOptions{})
	fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Create(ctx, &dummySeConfig, metav1.CreateOptions{})
	fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Create(ctx, &dummySeConfigForNonSourceCluster, metav1.CreateOptions{})
	fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Create(ctx, &userGeneratedSE, metav1.CreateOptions{})

	fakeIstioClient.NetworkingV1alpha3().DestinationRules("ns").Create(ctx, &userGeneratedDestinationRule, metav1.CreateOptions{})
	fakeIstioClient.NetworkingV1alpha3().DestinationRules("ns").Create(ctx, &dummyDRConfig, metav1.CreateOptions{})
	fakeIstioClient.NetworkingV1alpha3().DestinationRules("ns").Create(ctx, &emptyEndpointDR, metav1.CreateOptions{})

	fakeIstioClient2 := istiofake.NewSimpleClientset()
	fakeIstioClient3 := istiofake.NewSimpleClientset()
	fakeIstioClient4 := istiofake.NewSimpleClientset()
	fakeIstioClient5 := istiofake.NewSimpleClientset()

	rc := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},

		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient,
		},
	}

	rc2 := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient2,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient2,
			Cache:       istio.NewDestinationRuleCache(),
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},

		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient2,
		},
	}

	rc3 := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient3,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient3,
			Cache:       istio.NewDestinationRuleCache(),
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},

		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient3,
		},
	}

	rc4 := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient4,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient4,
			Cache:       istio.NewDestinationRuleCache(),
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},

		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient4,
		},
	}

	rc5 := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient5,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient5,
			Cache:       istio.NewDestinationRuleCache(),
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},

		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient5,
		},
	}

	setupForServiceEntryTests()
	admiralParams := common.GetAdmiralParams()
	admiralParams.AdditionalEndpointSuffixes = []string{"intuit"}
	admiralParams.DependentClusterWorkerConcurrency = 5
	admiralParams.EnableSWAwareNSCaches = true
	admiralParams.ExportToIdentityList = []string{"*"}
	admiralParams.ExportToMaxNamespaces = 35
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	rr := NewRemoteRegistry(nil, admiralParams)
	rr.PutRemoteController("cl1", rc)
	rr.PutRemoteController("cl2", rc2)
	rr.PutRemoteController("cl3", rc3)
	rr.PutRemoteController("cl4", rc4)
	rr.PutRemoteController("cl5", rc5)
	rr.AdmiralCache = &admiralCache

	cacheWithEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"test.dev.bar": common.LocalAddressPrefix + ".10.1"},
		Addresses:      []string{common.LocalAddressPrefix + ".10.1"},
	}

	cacheController := &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	admiralCache.ConfigMapController = cacheController

	destinationRuleFoundAssertion := func(ctx context.Context, fakeIstioClient *istiofake.Clientset, serviceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry, expectedAnnotations map[string]string, dnsPrefix string) error {
		for _, serviceEntry := range serviceEntries {
			var drName string
			if dnsPrefix != "" && dnsPrefix != "default" {
				drName = getIstioResourceName(serviceEntry.Hosts[0], "-dr")
			} else {
				drName = getIstioResourceName(serviceEntry.Hosts[0], "-default-dr")
			}
			dr, err := fakeIstioClient.NetworkingV1alpha3().DestinationRules("ns").Get(ctx, drName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if dr == nil {
				return fmt.Errorf("expected the destinationRule %s but it wasn't found", drName)
			}
			if !reflect.DeepEqual(expectedAnnotations, dr.Annotations) {
				return fmt.Errorf("expected SE annotations %v but got %v", expectedAnnotations, dr.Annotations)
			}
		}
		return nil
	}

	destinationRuleNotFoundAssertion := func(ctx context.Context, fakeIstioClient *istiofake.Clientset, serviceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry, expectedAnnotations map[string]string, dnsPrefix string) error {
		for _, serviceEntry := range serviceEntries {
			drName := getIstioResourceName(serviceEntry.Hosts[0], "-default-dr")
			_, err := fakeIstioClient.NetworkingV1alpha3().DestinationRules("ns").Get(ctx, drName, metav1.GetOptions{})
			if err != nil && !k8sErrors.IsNotFound(err) {
				return err
			}
		}
		return nil
	}

	serviceEntryFoundAssertion := func(ctx context.Context, fakeIstioClient *istiofake.Clientset, serviceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry, expectedAnnotations map[string]string, expectedLabels map[string]string) error {
		for _, serviceEntry := range serviceEntries {
			seName := getIstioResourceName(serviceEntry.Hosts[0], "-se")
			se, err := fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Get(ctx, seName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if se == nil {
				return fmt.Errorf("expected the service entry %s but it wasn't found", seName)
			}
			for expectedAnnotationLabel, expectedAnnotationValue := range expectedAnnotations {
				actualVal, ok := se.Annotations[expectedAnnotationLabel]
				if !ok {
					return fmt.Errorf("expected SE annotation label %v", expectedAnnotationLabel)
				}
				if actualVal != expectedAnnotationValue {
					return fmt.Errorf("expected SE annotation label %v value %s but got %v", expectedAnnotationLabel, expectedAnnotationValue, actualVal)
				}
			}
			for expectedLabel, expectedLabelValue := range expectedLabels {
				actualVal, ok := se.Labels[expectedLabel]
				if !ok {
					return fmt.Errorf("expected SE label %v", expectedLabel)
				}
				if actualVal != expectedLabelValue {
					return fmt.Errorf("expected SE label %v value %s but got %v", expectedLabel, expectedLabelValue, actualVal)
				}
			}
		}
		return nil
	}
	serviceEntryNotFoundAssertion := func(ctx context.Context, fakeIstioClient *istiofake.Clientset, serviceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry, expectedAnnotations map[string]string, expectedLabels map[string]string) error {
		for _, serviceEntry := range serviceEntries {
			seName := getIstioResourceName(serviceEntry.Hosts[0], "-se")
			_, err := fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Get(ctx, seName, metav1.GetOptions{})
			if err != nil && !k8sErrors.IsNotFound(err) {
				return err
			}
		}
		return nil
	}

	virtualServiceAssertion := func(ctx context.Context, fakeIstioClient *istiofake.Clientset, expectedAnnotations map[string]string,
		expectedLabels map[string]string) error {
		labelSelector, err := labels.ValidatedSelectorFromSet(expectedLabels)
		if err != nil {
			return err
		}
		listOptions := metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		}
		vsList, err := fakeIstioClient.NetworkingV1alpha3().VirtualServices("ns").List(ctx, listOptions)
		if err != nil {
			return err
		}
		if vsList == nil {
			return fmt.Errorf("expected the virtualservice list but found nil")
		}
		if len(vsList.Items) == 0 {
			return fmt.Errorf("no matching virtualservices found")
		}
		if len(vsList.Items) > 1 {
			return fmt.Errorf("expected 1 matching virtualservice but found %d", len(vsList.Items))
		}
		vs := vsList.Items[0]
		if !reflect.DeepEqual(expectedAnnotations, vs.Annotations) {
			return fmt.Errorf("expected VS annotations %v but got %v", expectedAnnotations, vs.Annotations)
		}
		if !reflect.DeepEqual(expectedLabels, vs.Labels) {
			return fmt.Errorf("expected VS labels %v but got %v", expectedLabels, vs.Labels)
		}
		return nil
	}

	testCases := []struct {
		name                                       string
		serviceEntries                             map[string]*istioNetworkingV1Alpha3.ServiceEntry
		dnsPrefix                                  string
		identity                                   string
		serviceEntryAssertion                      func(ctx context.Context, fakeIstioClient *istiofake.Clientset, serviceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry, expectedAnnotations map[string]string, expectedLabels map[string]string) error
		virtualServiceAssertion                    func(ctx context.Context, fakeIstioClient *istiofake.Clientset, expectedAnnotations map[string]string, expectedLabels map[string]string) error
		destinationRuleAssertion                   func(ctx context.Context, fakeIstioClient *istiofake.Clientset, serviceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry, expectedAnnotations map[string]string, dnsPrefix string) error
		isServiceEntryModifyCalledForSourceCluster bool
		env                                        string
		expectedServiceEntries                     map[string]*istioNetworkingV1Alpha3.ServiceEntry
		expectedDRAnnotations                      map[string]string
		expectedSEAnnotations                      map[string]string
		expectedVSAnnotations                      map[string]string
		expectedLabels                             map[string]string
		expectedErr                                error
		expectedVSLabels                           map[string]string
		isAdditionalEndpointsEnabled               bool
		sourceClusters                             map[string]string
	}{
		{
			name: "Given an identity and env" +
				"When identity passed is empty" +
				"When AddServiceEntriesWithDrToAllCluster is called" +
				"Then the func should return an error",
			identity:                     "",
			env:                          "stage",
			expectedErr:                  fmt.Errorf("failed to process service entry as identity passed was empty"),
			isAdditionalEndpointsEnabled: false,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name: "Given an identity and env" +
				"When env passed is empty" +
				"When AddServiceEntriesWithDrToAllCluster is called" +
				"Then the func should return an error",
			identity:                     "foo",
			env:                          "",
			expectedErr:                  fmt.Errorf("failed to process service entry as env passed was empty for identity foo"),
			isAdditionalEndpointsEnabled: false,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name:                     "given a serviceEntry that does not exists, when AddServiceEntriesWithDrToAllCluster is called, then the se is created and the corresponding dr is created",
			serviceEntries:           map[string]*istioNetworkingV1Alpha3.ServiceEntry{"sw01.e2e.foo": &dnsPrefixedSE},
			serviceEntryAssertion:    serviceEntryFoundAssertion,
			destinationRuleAssertion: destinationRuleFoundAssertion,
			identity:                 "foo",
			env:                      "sw01.e2e",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"sw01.e2e.foo": &dnsPrefixedSE,
				"west.sw01.e2e.foo": {
					Hosts: []string{"west.sw01.e2e.foo.global"},
				},
				"east.sw01.e2e.foo": {
					Hosts: []string{"east.sw01.e2e.foo.global"},
				},
			},
			expectedDRAnnotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			expectedSEAnnotations: map[string]string{
				resourceCreatedByAnnotationLabel:         resourceCreatedByAnnotationValue,
				common.GetWorkloadIdentifier():           "foo",
				serviceEntryAssociatedGtpAnnotationLabel: "dns-prefixed-gtp",
			},
			expectedLabels:               map[string]string{"env": "sw01.e2e"},
			isAdditionalEndpointsEnabled: false,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name:                         "given a serviceEntry that does not exists, when AddServiceEntriesWithDrToAllCluster is called, then the se is created and the corresponding dr is created",
			serviceEntries:               map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &newSE},
			serviceEntryAssertion:        serviceEntryFoundAssertion,
			destinationRuleAssertion:     destinationRuleFoundAssertion,
			identity:                     "newse",
			env:                          "dev",
			expectedServiceEntries:       map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &newSE},
			expectedDRAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			expectedSEAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "newse"},
			expectedLabels:               map[string]string{"env": "dev"},
			isAdditionalEndpointsEnabled: false,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name:                         "given a serviceEntry that already exists in the sync ns, when AddServiceEntriesWithDrToAllCluster is called, then the se is updated and the corresponding dr is updated as well",
			serviceEntries:               map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &se},
			serviceEntryAssertion:        serviceEntryFoundAssertion,
			destinationRuleAssertion:     destinationRuleFoundAssertion,
			identity:                     "bar",
			env:                          "dev",
			expectedServiceEntries:       map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &se},
			expectedDRAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			expectedSEAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "bar", serviceEntryAssociatedGtpAnnotationLabel: "test.dev.bar-gtp"},
			expectedLabels:               map[string]string{"env": "dev"},
			isAdditionalEndpointsEnabled: false,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name:                         "given a serviceEntry that does not exists and gtp with dnsPrefix is configured, when AddServiceEntriesWithDrToAllCluster is called, then the se is created and the corresponding dr is created as well",
			serviceEntries:               map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &newPrefixedSE},
			serviceEntryAssertion:        serviceEntryFoundAssertion,
			destinationRuleAssertion:     destinationRuleFoundAssertion,
			identity:                     "foo",
			env:                          "e2e",
			dnsPrefix:                    "prefix",
			expectedServiceEntries:       map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &newPrefixedSE},
			expectedDRAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			expectedSEAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "foo", dnsPrefixAnnotationLabel: "prefix", serviceEntryAssociatedGtpAnnotationLabel: "test.e2e.foo-gtp"},
			expectedLabels:               map[string]string{"env": "e2e"},
			isAdditionalEndpointsEnabled: false,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name:                         "given a serviceEntry with empty hosts, when AddServiceEntriesWithDrToAllCluster is called, then error is expected",
			serviceEntries:               map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &newSeWithEmptyHosts},
			serviceEntryAssertion:        serviceEntryFoundAssertion,
			destinationRuleAssertion:     destinationRuleFoundAssertion,
			identity:                     "foo",
			env:                          "e2e",
			expectedErr:                  fmt.Errorf("failed to process service entry for identity foo and env e2e as it is nil or has empty hosts"),
			isAdditionalEndpointsEnabled: false,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name:                         "given a preview serviceEntry that does not exists, when AddServiceEntriesWithDrToAllCluster is called, then the se is created and the corresponding dr is created",
			serviceEntries:               map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &newPreviewSE},
			serviceEntryAssertion:        serviceEntryFoundAssertion,
			destinationRuleAssertion:     destinationRuleFoundAssertion,
			identity:                     "newse",
			env:                          "dev",
			expectedServiceEntries:       map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &newPreviewSE},
			expectedDRAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			expectedSEAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "newse"},
			expectedLabels:               map[string]string{"env": "dev"},
			isAdditionalEndpointsEnabled: false,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name:                     "given a serviceEntry that already exists in the sync ns and the serviceEntry does not have any valid endpoints, when AddServiceEntriesWithDrToAllCluster is called, then the se should be deleted along with the corresponding dr",
			serviceEntries:           map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &emptyEndpointSe},
			serviceEntryAssertion:    serviceEntryNotFoundAssertion,
			destinationRuleAssertion: destinationRuleNotFoundAssertion,
			identity:                 "newse",
			env:                      "dev",
			expectedServiceEntries:   map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &emptyEndpointSe},
			isServiceEntryModifyCalledForSourceCluster: true,
			isAdditionalEndpointsEnabled:               false,
			sourceClusters:                             map[string]string{"cl1": "cl1"},
		},
		{
			name:                     "given a serviceEntry that already exists in the sync ns, and the endpoints contain dummy addresses, and this is source cluster entry when AddServiceEntriesWithDrToAllCluster is called, then the se should be deleted",
			serviceEntries:           map[string]*istioNetworkingV1Alpha3.ServiceEntry{"dummySe": &dummyEndpointSe},
			serviceEntryAssertion:    serviceEntryNotFoundAssertion,
			destinationRuleAssertion: destinationRuleNotFoundAssertion,
			identity:                 "newse",
			env:                      "dev",
			expectedServiceEntries:   map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &dummyEndpointSe},
			isServiceEntryModifyCalledForSourceCluster: true,
			isAdditionalEndpointsEnabled:               false,
			sourceClusters:                             map[string]string{"cl1": "cl1"},
		},
		{
			name:                     "given a serviceEntry that already exists in the sync ns, and the endpoints contain dummy addresses, and this is not source cluster entry when AddServiceEntriesWithDrToAllCluster is called, then the se should be deleted",
			serviceEntries:           map[string]*istioNetworkingV1Alpha3.ServiceEntry{"dummySe": &dummyEndpointSeForNonSourceCluster},
			serviceEntryAssertion:    serviceEntryNotFoundAssertion,
			destinationRuleAssertion: destinationRuleNotFoundAssertion,
			identity:                 "newse",
			env:                      "dev",
			expectedServiceEntries:   map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &dummyEndpointSeForNonSourceCluster},
			isServiceEntryModifyCalledForSourceCluster: false,
			isAdditionalEndpointsEnabled:               false,
			sourceClusters:                             map[string]string{"cl1": "cl1"},
		},
		{
			name:                         "given a user generated custom serviceEntry that already exists in the sync ns, when AddServiceEntriesWithDrToAllCluster is called with a service entry on the same hostname, then the user generated SE will not be overriden",
			serviceEntries:               map[string]*istioNetworkingV1Alpha3.ServiceEntry{"admiralOverrideSE": &admiralOverrideSE.Spec},
			serviceEntryAssertion:        serviceEntryFoundAssertion,
			destinationRuleAssertion:     destinationRuleFoundAssertion,
			identity:                     "newse",
			env:                          "dev",
			expectedServiceEntries:       map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &admiralOverrideSE.Spec},
			expectedDRAnnotations:        nil,
			expectedSEAnnotations:        nil,
			expectedLabels:               nil,
			isAdditionalEndpointsEnabled: false,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name: "given a serviceEntry that does not exists and gtp with default dnsPrefix is configured, " +
				"when AddServiceEntriesWithDrToAllCluster is called, " +
				"then the se is created and the corresponding dr is created as well along with the additional VS endpoint with 'default' DNS prefix label",
			serviceEntries:               map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &se},
			serviceEntryAssertion:        serviceEntryFoundAssertion,
			destinationRuleAssertion:     destinationRuleFoundAssertion,
			expectedDRAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			expectedSEAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "bar", serviceEntryAssociatedGtpAnnotationLabel: "test.dev.bar-gtp"},
			expectedVSAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "bar"},
			identity:                     "bar",
			env:                          "dev",
			virtualServiceAssertion:      virtualServiceAssertion,
			expectedVSLabels:             map[string]string{common.GetEnvKey(): "dev", dnsPrefixAnnotationLabel: "default"},
			expectedLabels:               map[string]string{"env": "dev"},
			isAdditionalEndpointsEnabled: true,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name: "given a serviceEntry that does not exists and gtp with non-default dnsPrefix is configured, " +
				"when AddServiceEntriesWithDrToAllCluster is called, " +
				"then the se is created and the corresponding dr is created as well with the additional VS endpoint with non-default DNS prefix label",
			serviceEntries:               map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &prefixedSE},
			serviceEntryAssertion:        serviceEntryFoundAssertion,
			destinationRuleAssertion:     destinationRuleFoundAssertion,
			dnsPrefix:                    "prefix",
			identity:                     "bar",
			env:                          "e2e",
			expectedDRAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			expectedSEAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "bar", dnsPrefixAnnotationLabel: "prefix", serviceEntryAssociatedGtpAnnotationLabel: "test.e2e.foo-gtp"},
			expectedVSAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "bar"},
			virtualServiceAssertion:      virtualServiceAssertion,
			expectedLabels:               map[string]string{"env": "e2e"},
			expectedVSLabels:             map[string]string{common.GetEnvKey(): "e2e", dnsPrefixAnnotationLabel: "prefix"},
			isAdditionalEndpointsEnabled: true,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name: "given a canary serviceEntry that does not exists " +
				"when AddServiceEntriesWithDrToAllCluster is called, " +
				"then the se is created and the corresponding dr is created as well ",
			serviceEntries:               map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &canarySE},
			serviceEntryAssertion:        serviceEntryFoundAssertion,
			destinationRuleAssertion:     destinationRuleFoundAssertion,
			dnsPrefix:                    "default",
			identity:                     "bar1",
			env:                          "e2e",
			expectedDRAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			expectedSEAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "bar1", dnsPrefixAnnotationLabel: "canary", serviceEntryAssociatedGtpAnnotationLabel: "canary.test.e2e.foo-gtp"},
			expectedVSAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "bar1"},
			virtualServiceAssertion:      virtualServiceAssertion,
			expectedLabels:               map[string]string{"env": "e2e"},
			expectedVSLabels:             map[string]string{common.GetEnvKey(): "e2e", dnsPrefixAnnotationLabel: "canary"},
			isAdditionalEndpointsEnabled: true,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name: "given a canary serviceEntry that does not exists and gtp with non-default dnsPrefix is configured, " +
				"when AddServiceEntriesWithDrToAllCluster is called, " +
				"then the se is created and the corresponding dr is created as well with the additional VS endpoint with non-default DNS prefix label",
			serviceEntries:               map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &prefixedCanarySE},
			serviceEntryAssertion:        serviceEntryFoundAssertion,
			destinationRuleAssertion:     destinationRuleFoundAssertion,
			dnsPrefix:                    "prefix.canary",
			identity:                     "bar",
			env:                          "e2e",
			expectedDRAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			expectedSEAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "bar", dnsPrefixAnnotationLabel: "prefix.canary", serviceEntryAssociatedGtpAnnotationLabel: "canary.test.e2e.foo-gtp"},
			expectedVSAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "bar"},
			virtualServiceAssertion:      virtualServiceAssertion,
			expectedLabels:               map[string]string{"env": "e2e"},
			expectedVSLabels:             map[string]string{common.GetEnvKey(): "e2e", dnsPrefixAnnotationLabel: "prefix.canary"},
			isAdditionalEndpointsEnabled: true,
			sourceClusters:               map[string]string{"cl1": "cl1"},
		},
		{
			name:                         "given a serviceEntry that does not exists, when AddServiceEntriesWithDrToAllCluster is called, then the se is created and the corresponding dr is created",
			serviceEntries:               map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &newSE, "se2": &newCanarySE},
			serviceEntryAssertion:        serviceEntryFoundAssertion,
			destinationRuleAssertion:     destinationRuleFoundAssertion,
			identity:                     "newse",
			env:                          "dev",
			expectedServiceEntries:       map[string]*istioNetworkingV1Alpha3.ServiceEntry{"se1": &newSE, "se2": &newCanarySE},
			expectedDRAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			expectedSEAnnotations:        map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "newse"},
			expectedLabels:               map[string]string{"env": "dev"},
			isAdditionalEndpointsEnabled: false,
			sourceClusters: map[string]string{"cl1": "cl1",
				"cl2": "cl2",
				"cl3": "cl3",
				"cl4": "cl4",
				"cl5": "cl5",
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx = context.WithValue(ctx, common.EventResourceType, common.Rollout)
			ctx = context.WithValue(ctx, common.EventType, admiral.Add)
			err := AddServiceEntriesWithDrToAllCluster(ctxLogger, ctx, rr, tt.sourceClusters, tt.serviceEntries, tt.isAdditionalEndpointsEnabled, tt.isServiceEntryModifyCalledForSourceCluster, tt.identity, tt.env, "")

			if tt.dnsPrefix != "" && tt.dnsPrefix != "default" {
				tt.serviceEntries["se1"].Hosts = []string{tt.dnsPrefix + ".e2e." + tt.identity + ".global"}
			}

			if (tt.expectedErr != nil && err == nil) || (tt.expectedErr == nil && err != nil) {
				t.Fatalf("expected error and actual error do not match")
			} else if err != nil && err.Error() != tt.expectedErr.Error() {
				t.Fatalf("expected error %v and actual err %v do not match", tt.expectedErr.Error(), err.Error())
			} else if err == nil {
				for _, r := range tt.sourceClusters {
					fakeClient := rr.GetRemoteController(r).ServiceEntryController.IstioClient
					if err := tt.serviceEntryAssertion(context.Background(), fakeClient.(*istiofake.Clientset), tt.expectedServiceEntries, tt.expectedSEAnnotations, tt.expectedLabels); err != nil {
						t.Error(err)
					} else if err := tt.destinationRuleAssertion(context.Background(), fakeClient.(*istiofake.Clientset), tt.serviceEntries, tt.expectedDRAnnotations, tt.dnsPrefix); err != nil {
						t.Error(err)
					}
				}
			}
			if tt.isAdditionalEndpointsEnabled && tt.virtualServiceAssertion != nil {
				err = tt.virtualServiceAssertion(context.Background(), fakeIstioClient, tt.expectedVSAnnotations, tt.expectedVSLabels)
				if err != nil {
					t.Error(err)
				}
			}
		})
	}

}

func TestAddServiceEntriesWithDrWithoutDatabaseClient(t *testing.T) {
	var ctxLogger = logrus.WithFields(logrus.Fields{
		"type": "modifySE",
	})
	admiralCache := AdmiralCache{
		IdentityClusterCache: common.NewMapOfMaps(),
	}
	setupForServiceEntryTests()
	admiralParams := common.GetAdmiralParams()
	admiralParams.LabelSet.WorkloadIdentityKey = "identity"
	admiralParams.LabelSet.EnvKey = "env"
	admiralParams.DependentClusterWorkerConcurrency = 5
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	cacheWithNoEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"prefix.e2e.foo.global-se": "test", "prefix.e2e.bar.global-se": "test"},
		Addresses:      []string{},
	}

	admiralCache.DynamoDbEndpointUpdateCache = &sync.Map{}
	admiralCache.DynamoDbEndpointUpdateCache.Store("dev.dummy.global", "")
	admiralCache.SeClusterCache = common.NewMapOfMaps()
	admiralCache.ServiceEntryAddressStore = &cacheWithNoEntry

	cnameIdentityCache := sync.Map{}
	cnameIdentityCache.Store("dev.bar.global", "bar")
	cnameIdentityCache.Store("dev.newse.global", "newse")
	cnameIdentityCache.Store("e2e.foo.global", "foo")
	cnameIdentityCache.Store("e2e.bar.global", "bar")
	admiralCache.CnameIdentityCache = &cnameIdentityCache

	trafficPolicyOverride := &model.TrafficPolicy{
		LbType:    model.TrafficPolicy_FAILOVER,
		DnsPrefix: common.Default,
		Target: []*model.TrafficGroup{
			{
				Region: "us-west-2",
				Weight: 100,
			},
		},
	}

	defaultGtp := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test.dev.bar-gtp",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				trafficPolicyOverride,
			},
		},
	}

	prefixedTrafficPolicy := &model.TrafficPolicy{
		LbType:    model.TrafficPolicy_TOPOLOGY,
		DnsPrefix: "prefix",
	}

	prefixedGtp := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test.e2e.foo-gtp",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				prefixedTrafficPolicy,
			},
		},
	}

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v13.GlobalTrafficPolicy)
	gtpCache.identityCache["dev.bar"] = defaultGtp
	gtpCache.identityCache["e2e.foo"] = prefixedGtp
	gtpCache.identityCache["e2e.bar"] = prefixedGtp
	gtpCache.mutex = &sync.Mutex{}
	admiralCache.GlobalTrafficCache = gtpCache

	odCache := NewOutlierDetectionCache()
	admiralCache.OutlierDetectionCache = odCache

	clientConnectionSettingsCache := NewClientConnectionConfigCache()
	admiralCache.ClientConnectionConfigCache = clientConnectionSettingsCache

	dummyEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{"dev.dummy.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"https": 80}, Network: "mesh1", Locality: "us-west", Weight: 100, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
		},
	}

	dummySeConfig := v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
		},
		//nolint
		Spec: dummyEndpointSe,
	}
	dummySeConfig.Name = "dev.dummy.global-se"
	dummySeConfig.Namespace = "ns"

	dummyPrefixedEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Addresses: []string{"240.10.1.0"},
		Hosts:     []string{"e2e.bar.global"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Network: "mesh1", Locality: "us-west", Weight: 100},
		},
	}

	dummyPrefixedSeConfig := v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
		},
		//nolint
		Spec: dummyPrefixedEndpointSe,
	}
	dummySeConfig.Name = "prefix.e2e.bar.global-se"
	dummySeConfig.Namespace = "ns"

	dummyVirtualService := v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{common.GetWorkloadIdentifier(): "bar", common.GetEnvKey(): "e2e", dnsPrefixAnnotationLabel: "prefix"},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{
			Hosts: []string{"prefix.e2e.bar.global"},
		},
	}

	dummyDRConfig := v1alpha3.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
		},
		Spec: istioNetworkingV1Alpha3.DestinationRule{
			Host: "dev.dummy.global",
		},
	}
	dummyDRConfig.Name = "dev.dummy.global-default-dr"
	dummyDRConfig.Namespace = "ns"

	ctx := context.Background()
	fakeIstioClient := istiofake.NewSimpleClientset()
	fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Create(ctx, &dummySeConfig, metav1.CreateOptions{})
	fakeIstioClient.NetworkingV1alpha3().DestinationRules("ns").Create(ctx, &dummyDRConfig, metav1.CreateOptions{})
	fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Create(ctx, &dummyPrefixedSeConfig, metav1.CreateOptions{})
	fakeIstioClient.NetworkingV1alpha3().VirtualServices("ns").Create(ctx, &dummyVirtualService, metav1.CreateOptions{})

	rc := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient,
		},
	}
	setupForServiceEntryTests()

	rr := NewRemoteRegistry(nil, admiralParams)
	rr.PutRemoteController("cl1", rc)
	rr.AdmiralCache = &admiralCache

	destinationRuleNotFoundAssertion := func(ctx context.Context, fakeIstioClient *istiofake.Clientset, serviceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry, expectedAnnotations map[string]string, dnsPrefix string) error {
		for _, serviceEntry := range serviceEntries {
			drName := getIstioResourceName(serviceEntry.Hosts[0], "-default-dr")
			_, err := fakeIstioClient.NetworkingV1alpha3().DestinationRules("ns").Get(ctx, drName, metav1.GetOptions{})
			if err != nil && !k8sErrors.IsNotFound(err) {
				return err
			}
		}
		return nil
	}

	serviceEntryNotFoundAssertion := func(ctx context.Context, fakeIstioClient *istiofake.Clientset, serviceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry, expectedAnnotations map[string]string, expectedLabels map[string]string) error {
		for _, serviceEntry := range serviceEntries {
			seName := getIstioResourceName(serviceEntry.Hosts[0], "-se")
			_, err := fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Get(ctx, seName, metav1.GetOptions{})
			if err != nil && !k8sErrors.IsNotFound(err) {
				return err
			}
		}
		return nil
	}

	virtualServiceAssertion := func(ctx context.Context, fakeIstioClient *istiofake.Clientset,
		expectedLabels map[string]string) error {
		labelSelector, err := labels.ValidatedSelectorFromSet(expectedLabels)
		if err != nil {
			return err
		}
		listOptions := metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		}
		_, err = fakeIstioClient.NetworkingV1alpha3().VirtualServices("ns").List(ctx, listOptions)
		if err != nil && !k8sErrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	testCases := []struct {
		name                                       string
		serviceEntries                             map[string]*istioNetworkingV1Alpha3.ServiceEntry
		dnsPrefix                                  string
		serviceEntryAssertion                      func(ctx context.Context, fakeIstioClient *istiofake.Clientset, serviceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry, expectedAnnotations map[string]string, expectedLabels map[string]string) error
		destinationRuleAssertion                   func(ctx context.Context, fakeIstioClient *istiofake.Clientset, serviceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry, expectedAnnotations map[string]string, dnsPrefix string) error
		virtualServiceAssertion                    func(ctx context.Context, fakeIstioClient *istiofake.Clientset, expectedLabels map[string]string) error
		isServiceEntryModifyCalledForSourceCluster bool
		identity                                   string
		env                                        string
		expectedDRAnnotations                      map[string]string
		expectedSEAnnotations                      map[string]string
		expectedLabels                             map[string]string
		expectedVSLabels                           map[string]string
		isAdditionalEndpointsEnabled               bool
	}{
		{
			name: "given a serviceEntry that already exists in the sync ns, " +
				"and the endpoints contain dummy addresses, " +
				"when AddServiceEntriesWithDrToAllCluster is called, " +
				"then the se should be deleted",
			serviceEntries:                             map[string]*istioNetworkingV1Alpha3.ServiceEntry{"dummySe": &dummyEndpointSe},
			serviceEntryAssertion:                      serviceEntryNotFoundAssertion,
			destinationRuleAssertion:                   destinationRuleNotFoundAssertion,
			isServiceEntryModifyCalledForSourceCluster: true,
		},
		{
			name: "given a serviceEntry and additional endpoint generate VS that already exists in the sync ns, " +
				"and the endpoints contain dummy addresses, " +
				"when AddServiceEntriesWithDrToAllCluster is called, " +
				"then the se should be deleted along with the corresponding VS",
			serviceEntries:           map[string]*istioNetworkingV1Alpha3.ServiceEntry{"dummySe": &dummyPrefixedEndpointSe},
			serviceEntryAssertion:    serviceEntryNotFoundAssertion,
			destinationRuleAssertion: destinationRuleNotFoundAssertion,
			identity:                 "newse",
			env:                      "deb",
			virtualServiceAssertion:  virtualServiceAssertion,
			isServiceEntryModifyCalledForSourceCluster: true,
			isAdditionalEndpointsEnabled:               true,
			expectedVSLabels:                           map[string]string{common.GetEnvKey(): "dev", common.GetWorkloadIdentifier(): "bar", dnsPrefixAnnotationLabel: "prefix"},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			rr.AdmiralDatabaseClient = nil
			AddServiceEntriesWithDrToAllCluster(ctxLogger, ctx, rr, map[string]string{"cl1": "cl1"}, tt.serviceEntries, false, tt.isServiceEntryModifyCalledForSourceCluster, tt.identity, tt.env, "")
			if tt.dnsPrefix != "" && tt.dnsPrefix != "default" {
				tt.serviceEntries["dummySe"].Hosts = []string{tt.dnsPrefix + ".e2e." + tt.identity + ".global"}
			}
			err := tt.serviceEntryAssertion(context.Background(), fakeIstioClient, tt.serviceEntries, tt.expectedSEAnnotations, tt.expectedLabels)
			if err != nil {
				t.Error(err)
			}
			err = tt.destinationRuleAssertion(context.Background(), fakeIstioClient, tt.serviceEntries, tt.expectedDRAnnotations, tt.dnsPrefix)
			if err != nil {
				t.Error(err)
			}
			if tt.isAdditionalEndpointsEnabled {
				err = tt.virtualServiceAssertion(context.Background(), fakeIstioClient, tt.expectedVSLabels)
				if err != nil {
					t.Error(err)
				}
			}
		})
	}

}

func TestCreateSeAndDrSetFromGtp(t *testing.T) {
	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})
	host := "dev.bar.global"
	hostCanary := "canary.dev.bar.global"
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

	errorCacheController := &test.FakeConfigMapController{
		GetError:          errors.New("fake get error"),
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	admiralCache.ConfigMapController = cacheController

	se := &istioNetworkingV1Alpha3.ServiceEntry{
		Addresses: []string{"240.10.1.0"},
		Hosts:     []string{host},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Locality: "us-west-2"},
			{Address: "240.20.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Locality: "us-east-2"},
		},
	}

	canarySe := &istioNetworkingV1Alpha3.ServiceEntry{
		Addresses: []string{"240.10.1.0"},
		Hosts:     []string{hostCanary},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Locality: "us-west-2"},
			{Address: "240.20.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Locality: "us-east-2"},
		},
	}

	seGTPDeploymentSENotInConfigmap := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"test.bar.mesh"},
		Addresses: []string{"240.0.10.11"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "127.0.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Locality: "us-west-2"},
			{Address: "240.20.0.1", Ports: map[string]uint32{"https": 80}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Locality: "us-east-2"},
		},
	}

	defaultPolicy := &model.TrafficPolicy{
		LbType: model.TrafficPolicy_TOPOLOGY,
		Dns:    host,
	}

	canaryPolicy := &model.TrafficPolicy{
		LbType: model.TrafficPolicy_TOPOLOGY,
		Dns:    hostCanary,
	}

	canaryPolicyDefault := &model.TrafficPolicy{
		LbType:    model.TrafficPolicy_TOPOLOGY,
		DnsPrefix: common.Default,
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
		OutlierDetection: &model.TrafficPolicy_OutlierDetection{
			BaseEjectionTime: 300,
			Interval:         60,
		},
	}

	gTPDefaultOverride := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gTPDefaultOverrideName",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				trafficPolicyDefaultOverride,
			},
		},
	}

	gTPMultipleDns := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gTPMultipleDnsName",
			Labels: map[string]string{
				"identity": "mock-identity",
			},
			Namespace: "mock-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				defaultPolicy, trafficPolicyWest, trafficPolicyEast,
			},
		},
	}

	gTPCanaryDns := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gTPMultipleDnsName",
			Labels: map[string]string{
				"identity": "mock-identity",
			},
			Namespace: "mock-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				canaryPolicy, trafficPolicyWest, trafficPolicyEast,
			},
		},
	}

	gTPCanaryDnsDefault := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gTPMultipleDnsName",
			Labels: map[string]string{
				"identity": "mock-identity",
			},
			Namespace: "mock-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				canaryPolicyDefault, trafficPolicyWest, trafficPolicyEast,
			},
		},
	}

	dnsPrefixedGTPSENotInConfigmap := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dns-prefixed-gtp-senotinconfigmap",
			Annotations: map[string]string{"env": "test"},
			Labels:      map[string]string{"identity": "bar"},
			Namespace:   "bar-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    0,
					DnsPrefix: "default",
				},
				{
					LbType:    1,
					DnsPrefix: "west",
				},
				{
					LbType:    1,
					DnsPrefix: "east",
				},
			},
		},
	}

	admiralCache.CnameClusterCache = common.NewMapOfMaps()

	expectedCnameCache := common.NewMapOfMaps()
	expectedCnameCache.Put("east.dev.bar.global", "fake-cluster", "fake-cluster")
	expectedCnameCache.Put("west.dev.bar.global", "fake-cluster", "fake-cluster")

	testCases := []struct {
		name                                       string
		env                                        string
		locality                                   string
		se                                         *istioNetworkingV1Alpha3.ServiceEntry
		gtp                                        *v13.GlobalTrafficPolicy
		seDrSet                                    map[string]*SeDrTuple
		cc                                         admiral.ConfigMapControllerInterface
		disableIPGeneration                        bool
		isServiceEntryModifyCalledForSourceCluster bool
		cnameCache                                 *common.MapOfMaps
	}{
		{
			name:     "Should handle a nil GTP",
			env:      "dev",
			locality: "us-west-2",
			se:       se,
			gtp:      nil,
			seDrSet:  map[string]*SeDrTuple{host: &SeDrTuple{}},
			cc:       cacheController,
			isServiceEntryModifyCalledForSourceCluster: false,
			cnameCache: common.NewMapOfMaps(),
		},
		{
			name:     "Should handle a GTP with default overide",
			env:      "dev",
			locality: "us-west-2",
			se:       se,
			gtp:      gTPDefaultOverride,
			seDrSet:  map[string]*SeDrTuple{host: &SeDrTuple{SeDnsPrefix: "default", SeDrGlobalTrafficPolicyName: "gTPDefaultOverrideName"}},
			cc:       cacheController,
			isServiceEntryModifyCalledForSourceCluster: false,
			cnameCache: common.NewMapOfMaps(),
		},
		{
			name:     "Should handle a GTP with multiple Dns",
			env:      "dev",
			locality: "us-west-2",
			se:       se,
			gtp:      gTPMultipleDns,
			seDrSet: map[string]*SeDrTuple{host: &SeDrTuple{SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"}, common.GetCnameVal([]string{west, host}): &SeDrTuple{SeDnsPrefix: "west", SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"},
				common.GetCnameVal([]string{east, host}): &SeDrTuple{SeDnsPrefix: "east", SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"}},
			cc: cacheController,
			isServiceEntryModifyCalledForSourceCluster: true,
			cnameCache: expectedCnameCache,
		},
		{
			name:     "Should handle a GTP with Dns prefix with Caps",
			env:      "dev",
			locality: "us-west-2",
			se:       se,
			gtp:      gTPMultipleDns,
			seDrSet: map[string]*SeDrTuple{host: &SeDrTuple{SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"}, common.GetCnameVal([]string{west, host}): &SeDrTuple{SeDnsPrefix: "west", SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"},
				strings.ToLower(common.GetCnameVal([]string{eastWithCaps, host})): &SeDrTuple{SeDnsPrefix: "east", SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"}},
			cc: cacheController,
			isServiceEntryModifyCalledForSourceCluster: false,
			cnameCache: expectedCnameCache,
		},
		{
			name:     "Should handle a GTP with canary endpoint",
			env:      "dev",
			locality: "us-west-2",
			se:       canarySe,
			gtp:      gTPCanaryDns,
			seDrSet: map[string]*SeDrTuple{hostCanary: &SeDrTuple{SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"},
				common.GetCnameVal([]string{west, hostCanary}):                          &SeDrTuple{SeDnsPrefix: "west.canary", SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"},
				strings.ToLower(common.GetCnameVal([]string{eastWithCaps, hostCanary})): &SeDrTuple{SeDnsPrefix: "east.canary", SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"}},
			cc: cacheController,
			isServiceEntryModifyCalledForSourceCluster: false,
			cnameCache: expectedCnameCache,
		},
		{
			name:     "Should handle a GTP with canary endpoint ande default",
			env:      "dev",
			locality: "us-west-2",
			se:       canarySe,
			gtp:      gTPCanaryDnsDefault,
			seDrSet: map[string]*SeDrTuple{hostCanary: &SeDrTuple{SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"},
				common.GetCnameVal([]string{west, hostCanary}): &SeDrTuple{SeDnsPrefix: "west.canary", SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"},
				common.GetCnameVal([]string{east, hostCanary}): &SeDrTuple{SeDnsPrefix: "east.canary", SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"},
				hostCanary: &SeDrTuple{SeDnsPrefix: "canary", SeDrGlobalTrafficPolicyName: "gTPMultipleDnsName"}},
			cc: cacheController,
			isServiceEntryModifyCalledForSourceCluster: false,
			cnameCache: expectedCnameCache,
		},
		{
			name: "Given a SE is getting updated due to a GTP applied to a Deployment, " +
				"And configmap doesn't contain the corresponding address, " +
				"And disable IP feature is disabled, " +
				"And the configmap returns an error, " +
				"Then the SE is nil",
			env:                 "dev",
			locality:            "us-west-2",
			se:                  seGTPDeploymentSENotInConfigmap,
			gtp:                 dnsPrefixedGTPSENotInConfigmap,
			seDrSet:             nil,
			cc:                  errorCacheController,
			disableIPGeneration: false,
			isServiceEntryModifyCalledForSourceCluster: false,
			cnameCache: expectedCnameCache,
		},
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, "eventResourceType", common.Deployment)
	ctx = context.WithValue(ctx, common.EventType, admiral.Add)
	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			admiralParams := admiralParamsForServiceEntryTests()
			admiralParams.DisableIPGeneration = c.disableIPGeneration
			common.ResetSync()
			common.InitializeConfig(admiralParams)
			admiralCache.ConfigMapController = c.cc
			result, _ := createSeAndDrSetFromGtp(ctxLogger, ctx, c.env, c.locality, "fake-cluster", c.se, c.gtp, nil, nil, &admiralCache, nil, false, c.isServiceEntryModifyCalledForSourceCluster)
			if c.seDrSet == nil {
				if !reflect.DeepEqual(result, c.seDrSet) {
					t.Fatalf("Expected nil seDrSet but got %+v", result)
				}
			}
			generatedHosts := make([]string, 0, len(result))
			for generatedHost := range result {
				generatedHosts = append(generatedHosts, generatedHost)
			}
			for host, _ := range c.seDrSet {
				if _, ok := result[host]; !ok {
					t.Fatalf("Generated hosts %v is missing the required host: %v", generatedHosts, host)
				} else if !isLower(result[host].SeName) || !isLower(result[host].DrName) {
					t.Fatalf("Generated istio resource names %v %v are not all lowercase", result[host].SeName, result[host].DrName)
				} else if result[host].SeDnsPrefix != c.seDrSet[host].SeDnsPrefix {
					t.Fatalf("Expected seDrSet entry dnsPrefix %s does not match the result %s", c.seDrSet[host].SeDnsPrefix, result[host].SeDnsPrefix)
				} else if result[host].SeDrGlobalTrafficPolicyName != c.seDrSet[host].SeDrGlobalTrafficPolicyName {
					t.Fatalf("Expected seDrSet entry global traffic policy name %s does not match the result %s", c.seDrSet[host].SeDrGlobalTrafficPolicyName, result[host].SeDrGlobalTrafficPolicyName)
				}
			}

			assert.Equal(t, c.cnameCache, admiralCache.CnameClusterCache)
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

	d, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())

	r, e := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())

	if e != nil || err != nil {
		t.Fail()
	}

	fakeIstioClient := istiofake.NewSimpleClientset()
	rc := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
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

	ctx := context.Background()
	ctx = context.WithValue(ctx, "clusterName", "clusterName")
	ctx = context.WithValue(ctx, "eventResourceType", common.Deployment)
	modifyServiceEntryForNewServiceOrPod(ctx, admiral.Add, "test", "bar", rr)

}

func TestModifyServiceEntryForNewServiceOrPod(t *testing.T) {
	setupForServiceEntryTests()
	var (
		env                     = "test"
		stop                    = make(chan struct{})
		foobarMetadataName      = "foobar"
		foobarMetadataNamespace = "foobar-ns"
		rollout1Identity        = "rollout1"
		deployment1Identity     = "deployment1"
		testRollout1            = argo.Rollout{
			ObjectMeta: metav1.ObjectMeta{
				Name:      foobarMetadataName,
				Namespace: foobarMetadataNamespace,
				Annotations: map[string]string{
					"env": "test",
				},
				Labels: map[string]string{
					"identity": rollout1Identity,
				},
			},
			Spec: argo.RolloutSpec{
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"identity": rollout1Identity},
						Annotations: map[string]string{
							"env": "test",
							"traffic.sidecar.istio.io/includeInboundPorts": "abcd",
						},
					},
				},
				Strategy: argo.RolloutStrategy{
					Canary: &argo.CanaryStrategy{
						TrafficRouting: &argo.RolloutTrafficRouting{
							Istio: &argo.IstioTrafficRouting{
								VirtualService: &argo.IstioVirtualService{
									Name: foobarMetadataName + "-canary",
								},
							},
						},
						CanaryService: foobarMetadataName + "-canary",
						StableService: foobarMetadataName + "-stable",
					},
				},
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"identity": rollout1Identity,
						"app":      rollout1Identity,
					},
				},
			},
		}
		testDeployment1 = &k8sAppsV1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      foobarMetadataName,
				Namespace: foobarMetadataNamespace,
				Annotations: map[string]string{
					"env": "test",
					"traffic.sidecar.istio.io/includeInboundPorts": "8090",
				},
				Labels: map[string]string{
					"identity": deployment1Identity,
				},
			},
			Spec: k8sAppsV1.DeploymentSpec{
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"env": "test",
							"traffic.sidecar.istio.io/includeInboundPorts": "abcs",
						},
						Labels: map[string]string{
							"identity": deployment1Identity,
						},
					},
					Spec: coreV1.PodSpec{},
				},
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"identity": deployment1Identity,
						"app":      deployment1Identity,
					},
				},
			},
		}
		clusterID                           = "test-dev-k8s"
		fakeIstioClient                     = istiofake.NewSimpleClientset()
		config                              = rest.Config{Host: "localhost"}
		resyncPeriod                        = time.Millisecond * 1
		expectedServiceEntriesForDeployment = map[string]*istioNetworkingV1Alpha3.ServiceEntry{
			"test." + deployment1Identity + ".mesh": &istioNetworkingV1Alpha3.ServiceEntry{
				Hosts:     []string{"test." + deployment1Identity + ".mesh"},
				Addresses: []string{"127.0.0.1"},
				Ports: []*istioNetworkingV1Alpha3.ServicePort{
					&istioNetworkingV1Alpha3.ServicePort{
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
						Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
					},
				},
				SubjectAltNames: []string{"spiffe://prefix/" + deployment1Identity},
			},
		}
		serviceEntryAddressStore = &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{
				"test." + deployment1Identity + ".mesh-se": "127.0.0.1",
				"test." + rollout1Identity + ".mesh-se":    "127.0.0.1",
			},
			Addresses: []string{},
		}
		serviceForRollout = &coreV1.Service{
			ObjectMeta: metav1.ObjectMeta{
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
			ObjectMeta: metav1.ObjectMeta{
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
	deploymentController, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	deploymentController.Cache.UpdateDeploymentToClusterCache(deployment1Identity, testDeployment1)
	rolloutController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	rolloutController.Cache.UpdateRolloutToClusterCache(rollout1Identity, &testRollout1)
	serviceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	virtualServiceController, err := istio.NewVirtualServiceController(make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	gtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
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
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic: gtpc,
	}
	rr1.PutRemoteController(clusterID, rc)
	rr1.ServiceEntrySuspender = NewDefaultServiceEntrySuspender([]string{})
	rr1.StartTime = time.Now()
	rr1.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	rr2.PutRemoteController(clusterID, rc)
	rr2.StartTime = time.Now()
	rr2.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	testCases := []struct {
		name                   string
		assetIdentity          string
		trafficPersona         bool
		remoteRegistry         *RemoteRegistry
		expectedServiceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry
	}{
		{
			name:                   "Given a deployment with invalid includeInboundPorts annotation service entry should not get created",
			assetIdentity:          deployment1Identity,
			remoteRegistry:         rr1,
			expectedServiceEntries: nil,
		}, {
			name:                   "Given a rollout with invalid includeInboundPorts annotation service entry should not get created",
			assetIdentity:          rollout1Identity,
			remoteRegistry:         rr1,
			expectedServiceEntries: nil,
		}, {
			name:                   "Given a deployment with invalid assetId",
			assetIdentity:          "invalid_asset_id",
			remoteRegistry:         rr1,
			expectedServiceEntries: nil,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = context.WithValue(ctx, "clusterName", "clusterName")
			ctx = context.WithValue(ctx, "eventResourceType", common.Deployment)
			serviceEntries, _ := modifyServiceEntryForNewServiceOrPod(
				ctx,
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

func TestGetLocalAddressForSe(t *testing.T) {
	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})
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
		disableIPGeneration bool
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
		{
			name: "Given a valid ServiceEntry name" +
				"When the DisableIPGeneration is set to true" +
				"Then the GetLocalAddressForSe should return an empty string, false and no error",
			seName:              "e2e.testmesh.mesh",
			seAddressCache:      cacheWith255Entries,
			wantAddess:          "",
			cacheController:     &cacheControllerPutError,
			expectedCacheUpdate: false,
			wantedError:         nil,
			disableIPGeneration: true,
		},
	}
	ctx := context.Background()
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			admiralParams := admiralParamsForServiceEntryTests()
			admiralParams.DisableIPGeneration = c.disableIPGeneration
			common.ResetSync()
			common.InitializeConfig(admiralParams)
			seAddress, needsCacheUpdate, err := GetLocalAddressForSe(ctxLogger, ctx, c.seName, &c.seAddressCache, c.cacheController)
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

	endpoint := makeRemoteEndpointForServiceEntry(address, locality, portName, common.DefaultMtlsPort, common.Deployment)

	if endpoint.Address != address {
		t.Errorf("Address mismatch. Got: %v, expected: %v", endpoint.Address, address)
	}
	if endpoint.Locality != locality {
		t.Errorf("Locality mismatch. Got: %v, expected: %v", endpoint.Locality, locality)
	}
	if endpoint.Ports[portName] != 15443 {
		t.Errorf("Incorrect port found")
	}

	if endpoint.Labels["type"] != common.Deployment {
		t.Errorf("Type mismatch. Got: %v, expected: %v", endpoint.Labels["type"], common.Deployment)
	}

	if endpoint.Labels["security.istio.io/tlsMode"] != "istio" {
		t.Errorf("Type mismatch. Got: %v, expected: %v", endpoint.Labels["sidecar.istio.io/tlsMode"], "istio")
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
	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})
	setupForServiceEntryTests()
	var (
		assetIdentity     = "test-identity"
		identityNamespace = "test-dependency-namespace"
		assetFQDN         = "test-local-fqdn"
		sidecar           = &v1alpha3.Sidecar{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default",
				Namespace: identityNamespace,
			},
			Spec: istioNetworkingV1Alpha3.Sidecar{
				Egress: []*istioNetworkingV1Alpha3.IstioEgressListener{
					{
						Hosts: []string{"a"},
					},
				},
			},
		}
	)
	sidecarController := &istio.SidecarController{}
	sidecarController.IstioClient = istiofake.NewSimpleClientset()
	sidecarController.IstioClient.NetworkingV1alpha3().Sidecars(identityNamespace).Create(context.TODO(), sidecar, metav1.CreateOptions{})

	remoteController := &RemoteController{}
	remoteController.SidecarController = sidecarController

	sidecarCacheEgressMap := common.NewSidecarEgressMap()
	sidecarCacheEgressMap.Put(
		assetIdentity,
		identityNamespace,
		assetFQDN,
		nil,
	)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(2)
	go func(ctx context.Context) {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				sidecarCacheEgressMap.Put(
					assetIdentity,
					identityNamespace,
					assetFQDN,
					map[string]string{
						"key": "value",
					},
				)
			}
		}
	}(ctx)

	go func(ctx context.Context) {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				modifySidecarForLocalClusterCommunication(
					ctxLogger,
					ctx, identityNamespace, assetIdentity,
					sidecarCacheEgressMap, remoteController)
			}
		}
	}(ctx)
	wg.Wait()

	sidecarObj, err := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars("test-sidecar-namespace").Get(ctx, common.GetWorkloadSidecarName(), metav1.GetOptions{})
	if err == nil {
		t.Errorf("expected 404 not found error but got nil")
	}

	if sidecarObj != nil {
		t.Fatalf("Modify non existing resource failed, as no new resource should be created.")
	}
}

func TestModifyExistingSidecarForLocalClusterCommunication(t *testing.T) {
	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})
	setupForServiceEntryTests()
	var (
		assetIdentity     = "test-identity"
		identityNamespace = "test-sidecar-namespace"
		sidecarName       = "default"
		assetHostsList    = []string{"test-host"}
		sidecar           = &v1alpha3.Sidecar{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sidecarName,
				Namespace: identityNamespace,
			},
			Spec: istioNetworkingV1Alpha3.Sidecar{
				Egress: []*istioNetworkingV1Alpha3.IstioEgressListener{
					{
						Hosts: assetHostsList,
					},
				},
			},
		}

		sidecarController     = &istio.SidecarController{}
		remoteController      = &RemoteController{}
		sidecarCacheEgressMap = common.NewSidecarEgressMap()
	)
	sidecarCacheEgressMap.Put(
		assetIdentity,
		"test-dependency-namespace",
		"test-local-fqdn",
		map[string]string{
			"test.myservice.global": "1",
		},
	)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()

	err := modifySidecarForLocalClusterCommunication(ctxLogger, ctx, identityNamespace, assetIdentity, sidecarCacheEgressMap, nil)
	assert.NotNil(t, err)
	assert.Equal(t, "skipped modifying sidecar resource as remoteController object is nil", err.Error())

	remoteController.SidecarController = sidecarController
	sidecarController.IstioClient = istiofake.NewSimpleClientset()
	createdSidecar, err := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars(identityNamespace).Create(context.TODO(), sidecar, metav1.CreateOptions{})

	if err != nil {
		t.Errorf("unable to create sidecar using fake client, err: %v", err)
	}
	if createdSidecar != nil {
		sidecarEgressMap := make(map[string]common.SidecarEgress)
		cnameMap := common.NewMap()
		cnameMap.Put("test.myservice.global", "1")
		sidecarEgressMap["test-dependency-namespace"] = common.SidecarEgress{Namespace: "test-dependency-namespace", FQDN: "test-local-fqdn", CNAMEs: cnameMap}
		modifySidecarForLocalClusterCommunication(ctxLogger, ctx, identityNamespace, assetIdentity, sidecarCacheEgressMap, remoteController)

		updatedSidecar, err := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars("test-sidecar-namespace").Get(ctx, "default", metav1.GetOptions{})

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

func TestRetryUpdatingSidecar(t *testing.T) {
	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})
	setupForServiceEntryTests()
	var (
		assetIdentity     = "test-identity"
		identityNamespace = "test-sidecar-namespace"
		sidecarName       = "default"
		assetHostsList    = []string{"test-host"}
		sidecar           = &v1alpha3.Sidecar{
			ObjectMeta: metav1.ObjectMeta{
				Name:            sidecarName,
				Namespace:       identityNamespace,
				ResourceVersion: "12345",
			},
			Spec: istioNetworkingV1Alpha3.Sidecar{
				Egress: []*istioNetworkingV1Alpha3.IstioEgressListener{
					{
						Hosts: assetHostsList,
					},
				},
			},
		}
		sidecarController     = &istio.SidecarController{}
		remoteController      = &RemoteController{}
		sidecarCacheEgressMap = common.NewSidecarEgressMap()
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sidecarCacheEgressMap.Put(
		assetIdentity,
		"test-dependency-namespace",
		"test-local-fqdn",
		map[string]string{
			"test.myservice.global": "1",
		},
	)
	remoteController.SidecarController = sidecarController
	sidecarController.IstioClient = istiofake.NewSimpleClientset()
	createdSidecar, _ := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars(identityNamespace).Create(context.TODO(), sidecar, metav1.CreateOptions{})
	sidecarEgressMap := make(map[string]common.SidecarEgress)
	cnameMap := common.NewMap()
	cnameMap.Put("test.myservice.global", "1")
	sidecarEgressMap["test-dependency-namespace"] = common.SidecarEgress{Namespace: "test-dependency-namespace", FQDN: "test-local-fqdn", CNAMEs: cnameMap}
	newSidecar := copySidecar(createdSidecar)
	egressHosts := make(map[string]string)
	for _, sidecarEgress := range sidecarEgressMap {
		egressHost := sidecarEgress.Namespace + "/" + sidecarEgress.FQDN
		egressHosts[egressHost] = egressHost
		sidecarEgress.CNAMEs.Range(func(k, v string) {
			scopedCname := sidecarEgress.Namespace + "/" + k
			egressHosts[scopedCname] = scopedCname
		})
	}
	for egressHost := range egressHosts {
		if !util.Contains(newSidecar.Spec.Egress[0].Hosts, egressHost) {
			newSidecar.Spec.Egress[0].Hosts = append(newSidecar.Spec.Egress[0].Hosts, egressHost)
		}
	}
	newSidecarConfig := createSidecarSkeleton(newSidecar.Spec, common.GetWorkloadSidecarName(), identityNamespace)
	err := retryUpdatingSidecar(ctxLogger, ctx, newSidecarConfig, createdSidecar, identityNamespace, remoteController, k8sErrors.NewConflict(schema.GroupResource{}, "", nil), "Add")
	if err != nil {
		t.Errorf("failed to retry updating sidecar, got err: %v", err)
	}
	updatedSidecar, err := sidecarController.IstioClient.NetworkingV1alpha3().Sidecars("test-sidecar-namespace").Get(ctx, "default", metav1.GetOptions{})
	if err != nil {
		t.Errorf("failed to get updated sidecar, got err: %v", err)
	}
	if updatedSidecar.ResourceVersion != createdSidecar.ResourceVersion {
		t.Errorf("resource version check failed, expected %v but got %v", createdSidecar.ResourceVersion, updatedSidecar.ResourceVersion)
	}
}

func TestCreateServiceEntry(t *testing.T) {
	setupForServiceEntryTests()
	ctxLogger := logrus.WithFields(logrus.Fields{
		"type": "modifySE",
	})
	config := rest.Config{
		Host: "localhost",
	}
	stop := make(chan struct{})
	s, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())

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
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		ServiceController: s,
	}

	cacheWithEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.my-first-service.mesh-se": localAddress},
		Addresses:      []string{localAddress},
	}

	cacheController := &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	admiralCache.ConfigMapController = cacheController

	errorCacheController := &test.FakeConfigMapController{
		GetError:          fmt.Errorf("unable to reach to api server"),
		PutError:          nil,
		ConfigmapToReturn: nil,
	}

	errorAdmiralCache := AdmiralCache{}
	errorAdmiralCache.ServiceEntryAddressStore = &ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses:      []string{},
	}
	errorAdmiralCache.ConfigMapController = errorCacheController

	deployment := v14.Deployment{}
	deployment.Spec.Template.Labels = map[string]string{"env": "e2e", "identity": "my-first-service"}

	// the second deployment will be add with us-east-2 region remote controller
	secondDeployment := v14.Deployment{}
	secondDeployment.Spec.Template.Labels = map[string]string{"env": "e2e", "identity": "my-first-service"}

	deploymentWithoutIdentity := v14.Deployment{}
	deploymentWithoutIdentity.Spec.Template.Labels = map[string]string{}

	se := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Deployment, "security.istio.io/tlsMode": "istio"}},
		},
	}

	oneEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Deployment, "security.istio.io/tlsMode": "istio"}},
		},
	}

	twoEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Deployment, "security.istio.io/tlsMode": "istio"}},
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2", Labels: map[string]string{"type": common.Deployment, "security.istio.io/tlsMode": "istio"}},
		},
	}

	threeEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Deployment, "security.istio.io/tlsMode": "istio"}},
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Deployment, "security.istio.io/tlsMode": "istio"}},
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2", Labels: map[string]string{"type": common.Deployment, "security.istio.io/tlsMode": "istio"}},
		},
	}
	eastEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2", Labels: map[string]string{"type": common.Deployment, "security.istio.io/tlsMode": "istio"}},
		},
	}

	emptyEndpointSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints:       []*istioNetworkingV1Alpha3.WorkloadEntry{},
	}

	grpcSe := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "grpc", Protocol: "grpc"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"grpc": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Deployment, "security.istio.io/tlsMode": "istio"}},
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
		expectedError  error
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
		{
			name:         "Error getting unique address for SE",
			action:       admiral.Delete,
			rc:           rc,
			admiralCache: errorAdmiralCache,
			meshPorts:    map[string]uint32{"http": uint32(80)},
			deployment:   deployment,
			serviceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"e2e.my-first-service.mesh": &threeEndpointSe,
			},
			expectedResult: nil,
			expectedError:  errors.New("could not get unique address after 3 retries. Failing to create serviceentry name=e2e.my-first-service.mesh"),
		},
		{
			name:         "SE should not create for deployment without identity",
			action:       admiral.Delete,
			rc:           rc,
			admiralCache: admiralCache,
			meshPorts:    map[string]uint32{"http": uint32(80)},
			deployment:   deploymentWithoutIdentity,
			serviceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"e2e.my-first-service.mesh": &threeEndpointSe,
			},
			expectedResult: nil,
			expectedError:  nil,
		},
	}

	ctx := context.Background()

	//Run the test for every provided case
	for _, c := range deploymentSeCreationTestCases {
		t.Run(c.name, func(t *testing.T) {
			createdSE, err := createServiceEntryForDeployment(ctxLogger, ctx, c.action, c.rc, &c.admiralCache, c.meshPorts, &c.deployment, c.serviceEntries, "")
			if err != nil {
				assert.Equal(t, err.Error(), c.expectedError.Error())
			} else if !compareServiceEntries(createdSE, c.expectedResult) {
				t.Errorf("Test %s failed, expected: %v got %v", c.name, c.expectedResult, createdSE)
			}
		})
	}

	seRollout := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Rollout, "security.istio.io/tlsMode": "istio"}},
		},
	}

	grpcSeRollout := istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*istioNetworkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "grpc", Protocol: "grpc"}},
		Location:        istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      istioNetworkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"grpc": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Rollout, "security.istio.io/tlsMode": "istio"}},
		},
	}

	// Test for Rollout
	rollout := argo.Rollout{}
	rollout.Spec.Template.Labels = map[string]string{"env": "e2e", "identity": "my-first-service"}

	rolloutWithoutIdentity := argo.Rollout{}
	rolloutWithoutIdentity.Spec.Template.Labels = map[string]string{}

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
			expectedResult: &grpcSeRollout,
		},
		{
			name:           "Should return a created service entry with http protocol",
			rc:             rc,
			admiralCache:   admiralCache,
			meshPorts:      map[string]uint32{"http": uint32(80)},
			rollout:        rollout,
			expectedResult: &seRollout,
		},
		{
			name:           "Should not create a service entry when configmap controller fails",
			rc:             rc,
			admiralCache:   errorAdmiralCache,
			meshPorts:      map[string]uint32{"http": uint32(80)},
			rollout:        rollout,
			expectedResult: nil,
		},
		{
			name:           "Should not create a service entry for rollout without identity",
			rc:             rc,
			admiralCache:   admiralCache,
			meshPorts:      map[string]uint32{"http": uint32(80)},
			rollout:        rolloutWithoutIdentity,
			expectedResult: nil,
		},
	}

	//Run the test for every provided case
	for _, c := range rolloutSeCreationTestCases {
		t.Run(c.name, func(t *testing.T) {
			createdSE, _ := createServiceEntryForRollout(ctxLogger, ctx, admiral.Add, c.rc, &c.admiralCache, c.meshPorts, &c.rollout, map[string]*istioNetworkingV1Alpha3.ServiceEntry{}, "")
			if !compareServiceEntries(createdSE, c.expectedResult) {
				t.Errorf("Test %s failed, expected: %v got %v", c.name, c.expectedResult, createdSE)
			}
		})
	}
}

func generateRC(fakeIstioClient *istiofake.Clientset, s *admiral.ServiceController) *RemoteController {
	return &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient,
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		ServiceController: s,
		ClusterID:         "test",
	}
}

func generateService(name string, ns string, labels map[string]string, port int32) *v1.Service {
	return &coreV1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: coreV1.ServiceSpec{
			Selector: labels,
			Ports: []coreV1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromInt(8080),
				},
			},
		},
	}
}

func createTestRollout(selector metav1.LabelSelector, stable string, canary string) argo.Rollout {
	rollout := argo.Rollout{
		Spec: argo.RolloutSpec{
			Strategy: argo.RolloutStrategy{
				Canary: &argo.CanaryStrategy{
					CanaryService: canary,
					StableService: stable,
					TrafficRouting: &argo.RolloutTrafficRouting{
						Istio: &argo.IstioTrafficRouting{
							VirtualService: &argo.IstioVirtualService{Name: "virtualservice"},
						},
					},
				},
			},
			Selector: &selector,
		},
	}
	rollout.Namespace = "test-ns"
	rollout.Spec.Template.Annotations = map[string]string{}
	rollout.Spec.Template.Annotations[common.SidecarEnabledPorts] = "8080"
	rollout.Spec.Template.Labels = map[string]string{"env": "e2e", "identity": "my-first-service"}
	rollout.Spec.Template.Namespace = "test-ns"
	return rollout
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

	d, e := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if e != nil {
		t.Fail()
	}
	r, e := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if e != nil {
		t.Fail()
	}
	v, e := istio.NewVirtualServiceController(make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if e != nil {
		t.Fail()
	}
	s, e := admiral.NewServiceController(make(chan struct{}), &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if e != nil {
		t.Fail()
	}
	gtpc, e := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if e != nil {
		t.Fail()
	}

	odc, e := admiral.NewOutlierDetectionController(make(chan struct{}), &test.MockOutlierDetectionHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
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
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		DeploymentController:       d,
		RolloutController:          r,
		ServiceController:          s,
		VirtualServiceController:   v,
		GlobalTraffic:              gtpc,
		OutlierDetectionController: odc,
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
		GlobalTrafficCache: &globalTrafficCache{
			mutex: &sync.Mutex{},
		},
		OutlierDetectionCache: &outlierDetectionCache{
			identityCache: make(map[string]*v13.OutlierDetection),
			mutex:         &sync.Mutex{},
		},
		ClientConnectionConfigCache: NewClientConnectionConfigCache(),
		DependencyNamespaceCache:    common.NewSidecarEgressMap(),
		SeClusterCache:              common.NewMapOfMaps(),
		DynamoDbEndpointUpdateCache: &sync.Map{},
	}
	rr.AdmiralCache = admiralCache

	rollout := argo.Rollout{}

	rollout.Spec = argo.RolloutSpec{
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
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

	labelSelector4 := metav1.LabelSelector{
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

	ctx = context.WithValue(ctx, "clusterName", "clusterName")
	ctx = context.WithValue(ctx, "eventResourceType", common.Deployment)
	se, _ := modifyServiceEntryForNewServiceOrPod(ctx, admiral.Add, "test", "bar", rr)
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
	common.ResetSync()
	common.InitializeConfig(admiralParamsForServiceEntryTests())

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

	d, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}

	r, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}

	v, err := istio.NewVirtualServiceController(make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}

	s, err := admiral.NewServiceController(make(chan struct{}), &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}

	gtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}

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
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
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
		GlobalTrafficCache: &globalTrafficCache{
			mutex: &sync.Mutex{},
		},
		OutlierDetectionCache: &outlierDetectionCache{
			identityCache: make(map[string]*v13.OutlierDetection),
			mutex:         &sync.Mutex{},
		},
		ClientConnectionConfigCache: NewClientConnectionConfigCache(),
		DependencyNamespaceCache:    common.NewSidecarEgressMap(),
		SeClusterCache:              common.NewMapOfMaps(),
		DynamoDbEndpointUpdateCache: &sync.Map{},
	}
	rr.AdmiralCache = admiralCache

	rollout := argo.Rollout{}

	rollout.Spec = argo.RolloutSpec{
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
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

	labelSelector4 := metav1.LabelSelector{
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

	ctx = context.WithValue(ctx, "clusterName", "clusterName")
	ctx = context.WithValue(ctx, "eventResourceType", common.Deployment)
	se, _ := modifyServiceEntryForNewServiceOrPod(ctx, admiral.Add, "test", "bar", rr)

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

	ctx = context.WithValue(ctx, "clusterName", "clusterName")
	ctx = context.WithValue(ctx, "eventResourceType", common.Deployment)

	se, _ = modifyServiceEntryForNewServiceOrPod(ctx, admiral.Add, "test", "bar", rr)

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
		Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Address: CLUSTER_INGRESS_1, Ports: map[string]uint32{"http": 15443},
	}

	meshPorts := map[string]uint32{"http": 8080}

	weightedServices := map[string]*WeightedService{
		ACTIVE_SERVICE:  {Service: &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: ACTIVE_SERVICE, Namespace: NAMESPACE}}},
		PREVIEW_SERVICE: {Service: &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: PREVIEW_SERVICE, Namespace: NAMESPACE}}},
	}

	activeWantedEndpoints := &istioNetworkingV1Alpha3.WorkloadEntry{
		Address: ACTIVE_SERVICE + common.Sep + NAMESPACE + common.GetLocalDomainSuffix(), Ports: meshPorts, Labels: map[string]string{"security.istio.io/tlsMode": "istio"},
	}

	previewWantedEndpoints := &istioNetworkingV1Alpha3.WorkloadEntry{
		Address: PREVIEW_SERVICE + common.Sep + NAMESPACE + common.GetLocalDomainSuffix(), Ports: meshPorts, Labels: map[string]string{"security.istio.io/tlsMode": "istio"},
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
			{Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Address: CLUSTER_INGRESS_1, Weight: 10, Ports: map[string]uint32{"http": 15443}},
			{Labels: map[string]string{"security.istio.io/tlsMode": "istio"}, Address: CLUSTER_INGRESS_2, Weight: 10, Ports: map[string]uint32{"http": 15443}},
		},
	}

	meshPorts := map[string]uint32{"http": 8080}

	weightedServices := map[string]*WeightedService{
		CANARY_SERVICE: {Weight: 10, Service: &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: CANARY_SERVICE, Namespace: NAMESPACE}}},
		STABLE_SERVICE: {Weight: 90, Service: &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: STABLE_SERVICE, Namespace: NAMESPACE}}},
	}
	weightedServicesZeroWeight := map[string]*WeightedService{
		CANARY_SERVICE: {Weight: 0, Service: &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: CANARY_SERVICE, Namespace: NAMESPACE}}},
		STABLE_SERVICE: {Weight: 100, Service: &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: STABLE_SERVICE, Namespace: NAMESPACE}}},
	}

	wantedEndpoints := []*istioNetworkingV1Alpha3.WorkloadEntry{
		{Address: CLUSTER_INGRESS_2, Weight: 10, Ports: map[string]uint32{"http": 15443}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
		{Address: STABLE_SERVICE + common.Sep + NAMESPACE + common.GetLocalDomainSuffix(), Weight: 90, Ports: meshPorts, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
		{Address: CANARY_SERVICE + common.Sep + NAMESPACE + common.GetLocalDomainSuffix(), Weight: 10, Ports: meshPorts, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
	}

	wantedEndpointsZeroWeights := []*istioNetworkingV1Alpha3.WorkloadEntry{
		{Address: CLUSTER_INGRESS_2, Weight: 10, Ports: map[string]uint32{"http": 15443}, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
		{Address: STABLE_SERVICE + common.Sep + NAMESPACE + common.GetLocalDomainSuffix(), Weight: 100, Ports: meshPorts, Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
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

type mockDatabaseClient struct {
	dynamoClient *DynamoClient
	database     *admiralapiv1.DynamoDB
}

func (mockDatabaseClient) Get(env, identity string) (interface{}, error) {
	workloadDataItem := WorkloadData{
		AssetAlias:          "identity1",
		Env:                 "envStage",
		DnsPrefix:           "hellogtp7",
		Endpoint:            "hellogtp7.envStage.identity1.mesh",
		LbType:              "FAILOVER",
		TrafficDistribution: map[string]int32{},
		Aliases:             []string{"hellogtp7.envStage.identity1.intuit"},
		GtpManagedBy:        "github",
	}

	workloadDataItems := []WorkloadData{workloadDataItem}

	return workloadDataItems, nil
}

func (mockDatabaseClient) Update(data interface{}, logger *logrus.Entry) error {
	return nil
}

func (mockDatabaseClient) Delete(data interface{}, logger *logrus.Entry) error {
	return nil
}

func TestHandleDynamoDbUpdateForOldGtp(t *testing.T) {
	setupForServiceEntryTests()

	testCases := []struct {
		name           string
		oldGtp         *v13.GlobalTrafficPolicy
		remoteRegistry *RemoteRegistry
		expectedErrMsg string
		expectedErr    bool
		env            string
		identity       string
	}{
		{
			name: "Given globaltrafficpolicy as nil, " +
				"when HandleDynamoDbUpdateForOldGtp is called, " +
				"then it should return err",
			oldGtp:         nil,
			expectedErr:    true,
			expectedErrMsg: "provided globaltrafficpolicy is nil",
		},
		{
			name: "Given globaltrafficpolicy with nil spec, " +
				"when HandleDynamoDbUpdateForOldGtp is called, " +
				"then it should return err",
			oldGtp: &v13.GlobalTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "gtp"},
			},
			expectedErr:    true,
			expectedErrMsg: "globaltrafficpolicy gtp has a nil spec",
		},
		{
			name: "Given globaltrafficpolicy with nil spec policy, " +
				"when HandleDynamoDbUpdateForOldGtp is called, " +
				"then it should return err",
			oldGtp: &v13.GlobalTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "gtp1"},
				Spec: model.GlobalTrafficPolicy{
					Selector: map[string]string{"identity": "test.asset"},
				},
			},
			identity:       "test.asset",
			expectedErr:    true,
			expectedErrMsg: "policies are not defined in globaltrafficpolicy : gtp1",
		},
		{
			name: "Given globaltrafficpolicy with 0 configured policies, " +
				"when HandleDynamoDbUpdateForOldGtp is called, " +
				"then it should return err",
			oldGtp: &v13.GlobalTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "gtp1"},
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{},
				},
			},
			expectedErr:    true,
			expectedErrMsg: "0 policies configured on globaltrafficpolicy: gtp1",
		},
		{
			name: "Given globaltrafficpolicy and nil dynamodb client, " +
				"when HandleDynamoDbUpdateForOldGtp is called, " +
				"then it should return err",
			oldGtp: &v13.GlobalTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "gtp1"},
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{DnsPrefix: "default"},
					},
				},
			},
			remoteRegistry: &RemoteRegistry{},
			expectedErr:    true,
			expectedErrMsg: "dynamodb client for workload data table is not initialized",
		},
	}

	var ctxLogger = logrus.WithFields(logrus.Fields{
		"type": "handleDynamoDbUpdateForOldGtp",
	})

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := handleDynamoDbUpdateForOldGtp(c.oldGtp, c.remoteRegistry, "", c.env, c.identity, ctxLogger)
			if err == nil && c.expectedErr {
				assert.Fail(t, "expected error to be returned")
			} else if c.expectedErrMsg != "" && c.expectedErrMsg != err.Error() {
				assert.Failf(t, "actual and expected error do not match. actual - %v, expected %v", err.Error(), c.expectedErrMsg)
			}
		})
	}
}

func TestUpdateGlobalGtpCacheAndGetGtpPreferenceRegion(t *testing.T) {
	setupForServiceEntryTests()
	var (
		remoteRegistryWithoutGtpWithoutAdmiralClient = &RemoteRegistry{
			AdmiralCache: &AdmiralCache{GlobalTrafficCache: &globalTrafficCache{identityCache: make(map[string]*v13.GlobalTrafficPolicy), mutex: &sync.Mutex{}}},
		}
		remoteRegistryWithGtpAndAdmiralClient = &RemoteRegistry{
			AdmiralCache: &AdmiralCache{GlobalTrafficCache: &globalTrafficCache{identityCache: make(map[string]*v13.GlobalTrafficPolicy), mutex: &sync.Mutex{}},
				DynamoDbEndpointUpdateCache: &sync.Map{},
			},
			AdmiralDatabaseClient: mockDatabaseClient{},
		}

		remoteRegistryWithInvalidGtpCache = &RemoteRegistry{
			AdmiralCache: &AdmiralCache{GlobalTrafficCache: &globalTrafficCache{identityCache: make(map[string]*v13.GlobalTrafficPolicy), mutex: &sync.Mutex{}},
				DynamoDbEndpointUpdateCache: &sync.Map{},
			},
			AdmiralDatabaseClient: mockDatabaseClient{},
		}
		identity1 = "identity1"
		envStage  = "stage"

		gtp = &v13.GlobalTrafficPolicy{ObjectMeta: metav1.ObjectMeta{Name: "gtp", Namespace: "namespace1", CreationTimestamp: metav1.NewTime(time.Now().Add(time.Duration(-30))), Labels: map[string]string{"identity": identity1, "env": envStage}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hello"}},
		}}

		gtp2 = &v13.GlobalTrafficPolicy{ObjectMeta: metav1.ObjectMeta{Name: "gtp2", Namespace: "namespace1", CreationTimestamp: metav1.NewTime(time.Now().Add(time.Duration(-15))), Labels: map[string]string{"identity": identity1, "env": envStage}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp2"}},
		}}

		gtp3 = &v13.GlobalTrafficPolicy{ObjectMeta: metav1.ObjectMeta{Name: "gtp3", Namespace: "namespace2", CreationTimestamp: metav1.NewTime(time.Now()), Labels: map[string]string{"identity": identity1, "env": envStage}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp3"}},
		}}

		gtp4 = &v13.GlobalTrafficPolicy{ObjectMeta: metav1.ObjectMeta{Name: "gtp4", Namespace: "namespace1", CreationTimestamp: metav1.NewTime(time.Now().Add(time.Duration(-30))), Labels: map[string]string{"identity": identity1, "env": envStage, "priority": "10"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp4"}},
		}}

		gtp5 = &v13.GlobalTrafficPolicy{ObjectMeta: metav1.ObjectMeta{Name: "gtp5", Namespace: "namespace1", CreationTimestamp: metav1.NewTime(time.Now().Add(time.Duration(-15))), Labels: map[string]string{"identity": identity1, "env": envStage, "priority": "2"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp5"}},
		}}

		gtp6 = &v13.GlobalTrafficPolicy{ObjectMeta: metav1.ObjectMeta{Name: "gtp6", Namespace: "namespace3", CreationTimestamp: metav1.NewTime(time.Now()), Labels: map[string]string{"identity": identity1, "env": envStage, "priority": "1000"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp6"}},
		}}
		gtp7 = &v13.GlobalTrafficPolicy{ObjectMeta: metav1.ObjectMeta{Name: "gtp7", Namespace: "namespace1", CreationTimestamp: metav1.NewTime(time.Now().Add(time.Duration(-45))), Labels: map[string]string{"identity": identity1, "env": envStage, "priority": "2"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{{DnsPrefix: "hellogtp7"}},
		}}
	)

	remoteRegistryWithGtpAndAdmiralClient.AdmiralCache.GlobalTrafficCache.Put(gtp7)

	remoteRegistryWithInvalidGtpCache.AdmiralCache.GlobalTrafficCache.Put(
		&v13.GlobalTrafficPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "gtp7",
				Namespace:         "namespace1",
				CreationTimestamp: metav1.NewTime(time.Now().Add(time.Duration(-45))),
				Labels:            map[string]string{"identity": identity1, "env": envStage, "priority": "2"},
			},
		},
	)

	testCases := []struct {
		name                  string
		identity              string
		env                   string
		gtps                  map[string][]*v13.GlobalTrafficPolicy
		remoteRegistry        *RemoteRegistry
		cache                 *AdmiralCache
		admiralDatabaseClient AdmiralDatabaseManager
		expectedGtp           *v13.GlobalTrafficPolicy
		expectedErr           error
	}{
		{
			name:           "Should return nil when no GTP present",
			gtps:           map[string][]*v13.GlobalTrafficPolicy{},
			identity:       identity1,
			env:            envStage,
			remoteRegistry: remoteRegistryWithoutGtpWithoutAdmiralClient,
			expectedGtp:    nil,
		},
		{
			name:           "Should return nil when no GTP present, but cache has existing gtp",
			gtps:           map[string][]*v13.GlobalTrafficPolicy{},
			identity:       identity1,
			env:            envStage,
			remoteRegistry: remoteRegistryWithGtpAndAdmiralClient,
			expectedGtp:    nil,
		},
		{
			name:           "Should return error when invalid GTP is present in cache",
			gtps:           map[string][]*v13.GlobalTrafficPolicy{},
			identity:       identity1,
			env:            envStage,
			remoteRegistry: remoteRegistryWithInvalidGtpCache,
			expectedGtp:    nil,
			expectedErr:    fmt.Errorf("failed to update dynamodb data when GTP was deleted for identity=identity1 and env=stage, err=globaltrafficpolicy gtp7 has a nil spec"),
		},
		{
			name:           "Should return the only existing gtp",
			gtps:           map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp}},
			identity:       identity1,
			env:            envStage,
			remoteRegistry: remoteRegistryWithoutGtpWithoutAdmiralClient,
			expectedGtp:    gtp,
		},
		{
			name:           "Should return the gtp recently created within the cluster",
			gtps:           map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp, gtp2}},
			identity:       identity1,
			env:            envStage,
			remoteRegistry: remoteRegistryWithoutGtpWithoutAdmiralClient,
			expectedGtp:    gtp2,
		},
		{
			name:           "Should return the gtp recently created from another cluster",
			gtps:           map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp, gtp2}, "c2": {gtp3}},
			identity:       identity1,
			env:            envStage,
			remoteRegistry: remoteRegistryWithoutGtpWithoutAdmiralClient,
			expectedGtp:    gtp3,
		},
		{
			name:           "Should return the existing priority gtp within the cluster",
			gtps:           map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp, gtp2, gtp7}},
			identity:       identity1,
			env:            envStage,
			remoteRegistry: remoteRegistryWithoutGtpWithoutAdmiralClient,
			expectedGtp:    gtp7,
		},
		{
			name:           "Should return the recently created priority gtp within the cluster",
			gtps:           map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp5, gtp4, gtp, gtp2}},
			identity:       identity1,
			env:            envStage,
			remoteRegistry: remoteRegistryWithoutGtpWithoutAdmiralClient,
			expectedGtp:    gtp4,
		},
		{
			name:           "Should return the recently created priority gtp from another cluster",
			gtps:           map[string][]*v13.GlobalTrafficPolicy{"c1": {gtp, gtp2, gtp4, gtp5, gtp7}, "c2": {gtp6}, "c3": {gtp3}},
			identity:       identity1,
			env:            envStage,
			remoteRegistry: remoteRegistryWithoutGtpWithoutAdmiralClient,
			expectedGtp:    gtp6,
		},
	}

	var ctxLogger = logrus.WithFields(logrus.Fields{
		"type": "updateGlobalGtpCache",
	})

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := updateGlobalGtpCacheAndGetGtpPreferenceRegion(c.remoteRegistry, c.identity, c.env, c.gtps, "", ctxLogger)
			if c.expectedErr == nil {
				if err != nil {
					t.Errorf("expected error to be: nil, got: %v", err)
				}
			}
			if c.expectedErr != nil {
				if err == nil {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
				if err != nil && err.Error() != c.expectedErr.Error() {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
			}

			if err == nil {
				gtp, err := c.remoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(c.identity, c.env)
				if err != nil {
					t.Error(err)
				}
				if !reflect.DeepEqual(c.expectedGtp, gtp) {
					t.Errorf("Test %s failed expected gtp: %v got %v", c.name, c.expectedGtp, gtp)
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

func TestCreateAdditionalEndpoints(t *testing.T) {

	ctx := context.Background()
	namespace := "testns"
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
		},
		SyncNamespace: namespace,
	}
	rr := NewRemoteRegistry(ctx, admiralParams)
	admiralParams.LabelSet.EnvKey = "admiral.io/env"

	vsRoutes := []*istioNetworkingV1Alpha3.HTTPRouteDestination{
		{
			Destination: &istioNetworkingV1Alpha3.Destination{
				Host: "stage.test00.global",
				Port: &istioNetworkingV1Alpha3.PortSelector{
					Number: common.DefaultServiceEntryPort,
				},
			},
		},
	}

	fooVS := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "stage.test00.foo-vs",
			Labels:      map[string]string{"admiral.io/env": "stage", dnsPrefixAnnotationLabel: "default"},
			Annotations: map[string]string{"identity": "test00"},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{
			Hosts: []string{"stage.test00.foo", "stage.test00.bar"},
			Http: []*istioNetworkingV1Alpha3.HTTPRoute{
				{
					Route: vsRoutes,
				},
			},
		},
	}

	existingVS := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "stage.existing.foo-vs",
			Labels: map[string]string{"admiral.io/env": "stage", "identity": "existing", dnsPrefixAnnotationLabel: "default"},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{
			Hosts: []string{"stage.existing.foo"},
			Http: []*istioNetworkingV1Alpha3.HTTPRoute{
				{
					Route: []*istioNetworkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &istioNetworkingV1Alpha3.Destination{
								Host: "stage.existing.global",
								Port: &istioNetworkingV1Alpha3.PortSelector{
									Number: common.DefaultServiceEntryPort,
								},
							},
						},
					},
				},
			},
		},
	}

	validIstioClient := istiofake.NewSimpleClientset()
	validIstioClient.NetworkingV1alpha3().VirtualServices("testns").
		Create(ctx, existingVS, metav1.CreateOptions{})

	testcases := []struct {
		name                       string
		rc                         *RemoteController
		identity                   string
		env                        string
		destinationHostName        string
		additionalEndpointSuffixes []string
		virtualServiceHostName     []string
		dnsPrefix                  string
		expectedError              error
		expectedVS                 []*v1alpha3.VirtualService
		gatewayClusters            []string
		eventResourceType          string
	}{
		{
			name:                       "Given additional endpoint suffixes, when passed identity is empty, func should return an error",
			identity:                   "",
			additionalEndpointSuffixes: []string{"foo"},
			expectedError:              fmt.Errorf("identity passed is empty"),
			eventResourceType:          common.Rollout,
		},
		{
			name:                       "Given additional endpoint suffixes, when passed env is empty, func should return an error",
			identity:                   "test00",
			env:                        "",
			additionalEndpointSuffixes: []string{"foo"},
			expectedError:              fmt.Errorf("env passed is empty"),
			eventResourceType:          common.Rollout,
		},
		{
			name:                       "Given additional endpoint suffixes, when valid identity,env and additional suffix params are passed, func should not return any error and create desired virtualservices",
			additionalEndpointSuffixes: []string{"foo", "bar"},
			identity:                   "test00",
			env:                        "stage",
			destinationHostName:        "stage.test00.global",
			expectedError:              nil,
			expectedVS:                 []*v1alpha3.VirtualService{fooVS},
			virtualServiceHostName:     []string{"stage.test00.foo", "stage.test00.bar"},
			dnsPrefix:                  common.Default,
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
			eventResourceType: common.Rollout,
		},
		{
			name:                       "Given additional endpoint suffixes, when valid identity,env and additional suffix params are passed, func should not return any error and create desired virtualservices",
			additionalEndpointSuffixes: []string{"foo"},
			identity:                   "existing",
			env:                        "stage",
			destinationHostName:        "stage.existing.global",
			expectedError:              nil,
			expectedVS:                 []*v1alpha3.VirtualService{existingVS},
			virtualServiceHostName:     []string{"stage.existing.foo"},
			dnsPrefix:                  common.Default,
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
			eventResourceType: common.Rollout,
		},
		{
			name:                       "Given no additional endpoint suffixes are provided, when valid identity,env params are passed, func should not return any additional endpoints",
			additionalEndpointSuffixes: []string{},
			identity:                   "test00",
			env:                        "stage",
			destinationHostName:        "stage.test00.global",
			expectedError:              fmt.Errorf("failed generating additional endpoints for suffixes"),
			expectedVS:                 []*v1alpha3.VirtualService{},
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
			eventResourceType: common.Rollout,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			admiralParams.AdditionalEndpointSuffixes = tc.additionalEndpointSuffixes
			common.ResetSync()
			common.InitializeConfig(admiralParams)

			ctxLogger := logrus.WithFields(logrus.Fields{
				"type":     "createAdditionalEndpoints",
				"identity": tc.identity,
				"txId":     uuid.New().String(),
			})

			ctx = context.WithValue(ctx, common.EventResourceType, tc.eventResourceType)

			err := createAdditionalEndpoints(ctxLogger, ctx, tc.rc, rr, tc.virtualServiceHostName, tc.identity, tc.env,
				tc.destinationHostName, namespace, tc.dnsPrefix, tc.gatewayClusters, tc.env)

			if err != nil && tc.expectedError != nil {
				if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("expected %s, got %s", tc.expectedError.Error(), err.Error())
				}
			} else if err != tc.expectedError {
				t.Errorf("expected %v, got %v", tc.expectedError, err)
			}

			if err == nil {
				for _, vs := range tc.expectedVS {
					actualVS, err := tc.rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Get(context.Background(), vs.Name, metav1.GetOptions{})
					if err != nil {
						t.Errorf("test failed with error: %v", err)
					}
					if !reflect.DeepEqual(vs.Spec.Hosts, actualVS.Spec.Hosts) {
						t.Errorf("expected %v, got %v", vs.Spec.Hosts, actualVS.Spec.Hosts)
					}
					if !reflect.DeepEqual(vs.Spec.Http, actualVS.Spec.Http) {
						t.Errorf("expected %v, got %v", vs.Spec.Http, actualVS.Spec.Http)
					}
					if !reflect.DeepEqual(vs.Labels, actualVS.Labels) {
						t.Errorf("expected %v, got %v", vs.Labels, actualVS.Labels)
					}
				}
			}
		})
	}
}

func createVSSkeletonForIdentity(identity string) *v1alpha3.VirtualService {
	return &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("test.%s.buzz-vs", identity),
			Labels:      map[string]string{"admiral.io/env": "test", "identity": identity, dnsPrefixAnnotationLabel: "default"},
			Annotations: map[string]string{},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{

			Hosts: []string{fmt.Sprintf("test.%s.buzz", identity)},
			Http: []*istioNetworkingV1Alpha3.HTTPRoute{
				{
					Route: []*istioNetworkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &istioNetworkingV1Alpha3.Destination{
								Host: fmt.Sprintf("test.%s.global", identity),
								Port: &istioNetworkingV1Alpha3.PortSelector{
									Number: common.DefaultServiceEntryPort,
								},
							},
						},
					},
				},
			},
		},
	}
}
func TestCreateAdditionalEndpointsForGatewayCluster(t *testing.T) {
	ctx := context.Background()

	var (
		identity1 = "my.asset.identity1"
		identity2 = "my.asset.identity2"
		identity3 = "my.asset.identity3"
		identity4 = "my.asset.identity4"
	)

	admiralParams := common.AdmiralParams{
		LabelSet:                   &common.LabelSet{},
		SyncNamespace:              "admiral-sync",
		AdditionalEndpointSuffixes: []string{"buzz"},
	}
	admiralParams.LabelSet.EnvKey = "admiral.io/env"
	admiralParams.LabelSet.WorkloadIdentityKey = "identity"
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	rr := NewRemoteRegistry(ctx, admiralParams)

	existingVS := createVSSkeletonForIdentity(identity1)
	existingVSForAirAsset := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("test.%s.buzz-vs", identity2),
			Labels:      map[string]string{"admiral.io/env": "test", dnsPrefixAnnotationLabel: "default"},
			Annotations: map[string]string{"identity": "my.asset.identity"},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{

			Hosts: []string{fmt.Sprintf("test.%s.buzz", identity2)},
			Http: []*istioNetworkingV1Alpha3.HTTPRoute{
				{
					Route: []*istioNetworkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &istioNetworkingV1Alpha3.Destination{
								Host: fmt.Sprintf("test-air.%s.global", identity2),
								Port: &istioNetworkingV1Alpha3.PortSelector{
									Number: common.DefaultServiceEntryPort,
								},
							},
						},
					},
				},
			},
		},
	}

	existingVSId3 := createVSSkeletonForIdentity(identity3)
	existingVSId4 := createVSSkeletonForIdentity(identity4)

	nonExistingVS := &networking.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test.my.asset.identity3.buzz-vs",
			Labels: map[string]string{"admiral.io/env": "test", "identity": "my.asset.identity3", dnsPrefixAnnotationLabel: "default"},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{
			Hosts: []string{"test.my.asset.identity3.buzz"},
			Http: []*istioNetworkingV1Alpha3.HTTPRoute{
				{
					Route: []*istioNetworkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &istioNetworkingV1Alpha3.Destination{
								Host: "test-air.my.asset.identity3.global",
								Port: &istioNetworkingV1Alpha3.PortSelector{
									Number: common.DefaultServiceEntryPort,
								},
							},
						},
					},
				},
			},
		},
	}

	validIstioClient := istiofake.NewSimpleClientset()
	validIstioClient.NetworkingV1alpha3().VirtualServices(admiralParams.SyncNamespace).
		Create(ctx, existingVS, metav1.CreateOptions{})

	validIstioClient.NetworkingV1alpha3().VirtualServices(admiralParams.SyncNamespace).
		Create(ctx, existingVSForAirAsset, metav1.CreateOptions{})

	validIstioClient.NetworkingV1alpha3().VirtualServices(admiralParams.SyncNamespace).
		Create(ctx, existingVSId3, metav1.CreateOptions{})
	validIstioClient.NetworkingV1alpha3().VirtualServices(admiralParams.SyncNamespace).
		Create(ctx, existingVSId4, metav1.CreateOptions{})

	testcases := []struct {
		name                        string
		env                         string
		rc                          *RemoteController
		resourceType                string
		resourceAdmiralEnv          string
		hostNames                   []string
		identity                    string
		destinationHostname         string
		expectedDestination         string
		expectedVSHostNames         []string
		dnsPrefix                   string
		gatewayClusters             []string
		expectedVS                  *v1alpha3.VirtualService
		expectedError               error
		compareAnnotationsAndLabels bool
	}{
		{
			name: "given that the additionalEndpoint already exists, the destination host should be updated if the source rollout env has -air suffix",
			rc: &RemoteController{
				ClusterID: "gwCluster1",
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
			resourceType:                common.Rollout,
			gatewayClusters:             []string{"gwCluster1"},
			env:                         "test",
			resourceAdmiralEnv:          "test-air",
			identity:                    "my.asset.identity1",
			hostNames:                   []string{"test.my.asset.identity1.buzz"},
			destinationHostname:         "test-air.my.asset.identity1.global",
			compareAnnotationsAndLabels: true,
			dnsPrefix:                   "default",
			expectedVS: &networking.VirtualService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        fmt.Sprintf("test.%s.buzz-vs", identity1),
					Labels:      map[string]string{"admiral.io/env": "test", dnsPrefixAnnotationLabel: "default"},
					Annotations: map[string]string{"identity": identity1, "app.kubernetes.io/created-by": "admiral"},
				},
				Spec: istioNetworkingV1Alpha3.VirtualService{
					Hosts: existingVS.Spec.Hosts,
					Http: []*istioNetworkingV1Alpha3.HTTPRoute{
						{
							Route: []*istioNetworkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &istioNetworkingV1Alpha3.Destination{
										Host: "test-air.my.asset.identity1.global",
										Port: &istioNetworkingV1Alpha3.PortSelector{
											Number: common.DefaultServiceEntryPort,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "given that the additional endpoint already exists, the destination host should not be updated if the cluster ID is non-gateway", // this is handled by cartographer
			rc: &RemoteController{
				ClusterID: "nonGatewayCluster",
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
			resourceType:                common.Rollout,
			gatewayClusters:             []string{"gwCluster1"},
			env:                         "test",
			resourceAdmiralEnv:          "test-air",
			identity:                    "my.asset.identity3",
			hostNames:                   []string{"test.my.asset.identity3.buzz"},
			destinationHostname:         "test-air.my.asset.identity3.global",
			expectedVS:                  existingVSId3,
			compareAnnotationsAndLabels: false,
		},
		{
			name: "given that the additional endpoint already exists, the destination host should not be updated if the source rollout env does not have -air suffix",
			rc: &RemoteController{
				ClusterID: "nonGatewayCluster",
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
			resourceType:                common.Rollout,
			gatewayClusters:             []string{"gwCluster1"},
			env:                         "test",
			resourceAdmiralEnv:          "test",
			identity:                    "my.asset.identity2",
			hostNames:                   existingVSForAirAsset.Spec.Hosts,
			destinationHostname:         "test.my.asset.identity2.global",
			expectedVS:                  existingVSForAirAsset,
			compareAnnotationsAndLabels: false,
		},
		{
			name: "Given that the additional endpoint already exists, " +
				"When the source type is deployment, " +
				"Then, the destination host of the virtual service should not be updated",
			rc: &RemoteController{
				ClusterID: "gwCluster1",
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
			resourceType:        common.Deployment,
			gatewayClusters:     []string{"gwCluster1"},
			env:                 "test",
			resourceAdmiralEnv:  "test-air",
			identity:            "my.asset.identity4",
			hostNames:           existingVSId4.Spec.Hosts,
			destinationHostname: "test-air.my.asset.identity4.global",
			expectedVS:          existingVSId4,
		},
		{
			name: "given that the additional endpoint does not exist, it should be created when resource type is rollout",
			rc: &RemoteController{
				ClusterID: "gwCluster1",
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
			resourceType:        common.Rollout,
			gatewayClusters:     []string{"gwCluster1"},
			env:                 "test",
			resourceAdmiralEnv:  "test-air",
			identity:            "my.asset.identity3",
			hostNames:           nonExistingVS.Spec.Hosts,
			destinationHostname: "test-air.my.asset.identity3.global",
			expectedVS:          nonExistingVS,
		},
		{
			name: "given that the additional endpoint does not exist, it should be created when resource type is deployment",
			rc: &RemoteController{
				ClusterID: "gwCluster1",
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
			resourceType:        common.Deployment,
			gatewayClusters:     []string{"gwCluster1"},
			env:                 "test",
			resourceAdmiralEnv:  "test-air",
			identity:            "my.asset.identity3",
			hostNames:           nonExistingVS.Spec.Hosts,
			destinationHostname: "test-air.my.asset.identity3.global",
			expectedVS:          nonExistingVS,
		},
		{
			name: "given that the additionalEndpoint with identity label in the virtualservice labels already exists," +
				"when the VirtualService is updated," +
				"then the identityLabel is moved to annotations",
			rc: &RemoteController{
				ClusterID: "gwCluster1",
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
			dnsPrefix:                   "default",
			resourceType:                common.Rollout,
			gatewayClusters:             []string{"gwCluster1"},
			env:                         "test",
			resourceAdmiralEnv:          "test-air",
			identity:                    "my.asset.identity1",
			hostNames:                   []string{"test.my.asset.identity1.buzz"},
			destinationHostname:         "test-air.my.asset.identity1.global",
			compareAnnotationsAndLabels: true,
			expectedVS: &networking.VirtualService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        fmt.Sprintf("test.%s.buzz-vs", "my.asset.identity1"),
					Labels:      map[string]string{"admiral.io/env": "test", dnsPrefixAnnotationLabel: "default"},
					Annotations: map[string]string{common.GetWorkloadIdentifier(): "my.asset.identity1", "app.kubernetes.io/created-by": "admiral"},
				},
				Spec: istioNetworkingV1Alpha3.VirtualService{
					Hosts: existingVS.Spec.Hosts,
					Http: []*istioNetworkingV1Alpha3.HTTPRoute{
						{
							Route: []*istioNetworkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &istioNetworkingV1Alpha3.Destination{
										Host: "test-air.my.asset.identity1.global",
										Port: &istioNetworkingV1Alpha3.PortSelector{
											Number: common.DefaultServiceEntryPort,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx = context.WithValue(ctx, common.EventResourceType, tc.resourceType)
			common.ResetSync()
			common.InitializeConfig(admiralParams)

			ctxLogger := logrus.WithFields(logrus.Fields{
				"type": "createAdditionalEndpoints",
				"txId": uuid.New().String(),
			})

			err := createAdditionalEndpoints(ctxLogger, ctx, tc.rc, rr, tc.hostNames, tc.identity, tc.env, tc.destinationHostname, admiralParams.SyncNamespace, tc.dnsPrefix, tc.gatewayClusters, tc.resourceAdmiralEnv)
			if err != nil && tc.expectedError != nil {
				if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("expected %s, got %s", tc.expectedError.Error(), err.Error())
				}
			} else if err != tc.expectedError {
				t.Errorf("expected %v, got %v", tc.expectedError, err)
			}

			if err == nil {
				actualVS, err := tc.rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(admiralParams.SyncNamespace).Get(context.Background(), tc.expectedVS.Name, metav1.GetOptions{})
				if err != nil {
					t.Errorf("test failed with error: %v", err)
				}
				if !reflect.DeepEqual(tc.expectedVS.Spec.Hosts, actualVS.Spec.Hosts) {
					t.Errorf("expected %v, got %v", tc.expectedVS.Spec.Hosts, actualVS.Spec.Hosts)
				}
				if !reflect.DeepEqual(tc.expectedVS.Spec.Http, actualVS.Spec.Http) {
					t.Errorf("expected %v, got %v", tc.expectedVS.Spec.Http, actualVS.Spec.Http)
				}

				if tc.compareAnnotationsAndLabels {
					if !compareStringMaps(tc.expectedVS.Annotations, actualVS.Annotations) {
						t.Errorf("expected %v, got %v", tc.expectedVS.Annotations, actualVS.Annotations)

					}
					if !compareStringMaps(tc.expectedVS.Labels, actualVS.Labels) {
						t.Errorf("expected %v, got %v", tc.expectedVS.Annotations, actualVS.Annotations)
					}
				}
			}
		})
	}

}

func compareStringMaps(expected map[string]string, actual map[string]string) bool {
	if len(expected) != len(actual) {
		return false
	}

	for k, v := range expected {
		if actualValue, ok := actual[k]; !ok || actualValue != v {
			return false
		}
	}

	return true
}
func TestGetAdditionalEndpoints(t *testing.T) {

	namespace := "testns"
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
		},
		SyncNamespace: namespace,
	}
	admiralParams.LabelSet.EnvKey = "admiral.io/env"

	testcases := []struct {
		name                        string
		identity                    string
		env                         string
		additionalEndpointSuffixes  []string
		expectedError               error
		expectedAdditionalEndpoints map[string]bool
		dnsPrefix                   string
	}{
		{
			name: "Given additional endpoint suffixes and passed identity is empty, " +
				"When getAdditionalEndpoints is called, " +
				"it should return an error",
			identity:                   "",
			additionalEndpointSuffixes: []string{"foo"},
			expectedError:              fmt.Errorf("identity passed is empty"),
		},
		{
			name: "Given additional endpoint suffixes and passed env is empty, " +
				"When getAdditionalEndpoints is called, " +
				"it should return an error",
			identity:                   "test00",
			env:                        "",
			additionalEndpointSuffixes: []string{"foo"},
			expectedError:              fmt.Errorf("env passed is empty"),
		},
		{
			name: "Given additional endpoint suffixes and valid identity,env along with additional suffix params are passed, " +
				"When getAdditionalEndpoints is called, " +
				"it should not return any error and should return additional endpoints",
			additionalEndpointSuffixes:  []string{"foo", "bar"},
			identity:                    "test00",
			env:                         "stage",
			expectedAdditionalEndpoints: map[string]bool{"stage.test00.foo": true, "stage.test00.bar": true},
			expectedError:               nil,
		},
		{
			name: "Given additional endpoint suffixes and valid identity,env along with additional suffix params are passed, " +
				"When empty vsDNSPrefix is passed, " +
				"it should not return any error and should return additional endpoints with no dns prefix prepended",
			additionalEndpointSuffixes:  []string{"foo", "bar"},
			identity:                    "test00",
			env:                         "stage",
			dnsPrefix:                   "",
			expectedAdditionalEndpoints: map[string]bool{"stage.test00.foo": true, "stage.test00.bar": true},
			expectedError:               nil,
		},
		{
			name: "Given additional endpoint suffixes and valid identity,env along with additional suffix params are passed, " +
				"When default vsDNSPrefix is passed, " +
				"it should not return any error and should return additional endpoints with no dns prefix prepended",
			additionalEndpointSuffixes:  []string{"foo", "bar"},
			identity:                    "test00",
			env:                         "stage",
			dnsPrefix:                   common.Default,
			expectedAdditionalEndpoints: map[string]bool{"stage.test00.foo": true, "stage.test00.bar": true},
			expectedError:               nil,
		},
		{
			name: "Given additional endpoint suffixes and valid identity,env along with additional suffix params are passed, " +
				"When non-empty and non-default vsDNSPrefix is passed, " +
				"it should not return any error and should return additional endpoints with dns prefix prepended",
			additionalEndpointSuffixes:  []string{"foo", "bar"},
			identity:                    "test00",
			env:                         "stage",
			dnsPrefix:                   "west",
			expectedAdditionalEndpoints: map[string]bool{"west.stage.test00.foo": true, "west.stage.test00.bar": true},
			expectedError:               nil,
		},
		{
			name: "Given identity has an upper case" +
				"When getAdditionalEndpoints is called, " +
				"Then, it should return additional endpoints in lower case",
			additionalEndpointSuffixes: []string{"foo", "bar"},
			identity:                   "TEST00",
			env:                        "stage",
			expectedError:              nil,
			expectedAdditionalEndpoints: map[string]bool{
				"stage.test00.foo": true,
				"stage.test00.bar": true,
			},
		},
		{
			name:                       "Given the identity and valid intuit endpoint suffix and air env",
			additionalEndpointSuffixes: []string{"intuit"},
			identity:                   "TEST00",
			env:                        "stage-air",
			expectedError:              nil,
			expectedAdditionalEndpoints: map[string]bool{
				"stage.test00.intuit": true,
			},
		},
		{
			name:                       "Given the identity, valid intuit endpoint suffix, air env and valid dnsPrefix",
			additionalEndpointSuffixes: []string{"intuit"},
			identity:                   "TEST00",
			env:                        "stage-air",
			dnsPrefix:                  "west",
			expectedError:              nil,
			expectedAdditionalEndpoints: map[string]bool{
				"west.stage.test00.intuit": true,
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			admiralParams.AdditionalEndpointSuffixes = tc.additionalEndpointSuffixes
			common.ResetSync()
			common.InitializeConfig(admiralParams)

			additionalEndpoints, err := getAdditionalEndpoints(tc.identity, tc.env, tc.dnsPrefix)

			if err != nil && tc.expectedError != nil {
				if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("expected %s, got %s", tc.expectedError.Error(), err.Error())
				}
				if len(additionalEndpoints) != 0 {
					t.Errorf("expected additional endpoints length as 0 in case of error, but got %v", len(additionalEndpoints))
				}
			} else if err != tc.expectedError {
				t.Errorf("expected %v, got %v", tc.expectedError, err)
			}
			for _, additionalEndpoint := range additionalEndpoints {
				if tc.expectedAdditionalEndpoints != nil && !tc.expectedAdditionalEndpoints[additionalEndpoint] {
					t.Errorf("expected endpoints %s to be in %v", additionalEndpoint, tc.expectedAdditionalEndpoints)
				}
			}
		})
	}
}

func TestDeleteAdditionalEndpoints(t *testing.T) {
	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})
	ctx := context.Background()
	namespace := "testns"
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
		},
		SyncNamespace: namespace,
	}
	admiralParams.LabelSet.EnvKey = "admiral.io/env"

	fooVS := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "stage.test00.foo-vs",
			Labels:      map[string]string{"admiral.io/env": "stage", "identity": "test00", dnsPrefixAnnotationLabel: "default"},
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{
			Hosts: []string{"stage.test00.foo", "stage.test00.bar"},
		},
	}

	barVS := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "stage.test00.bar-vs",
			Labels:      map[string]string{"admiral.io/env": "stage", "identity": "test00", dnsPrefixAnnotationLabel: "default"},
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{
			Hosts: []string{"stage.test00.foo", "stage.test00.bar"},
		},
	}

	validIstioClient := istiofake.NewSimpleClientset()
	validIstioClient.NetworkingV1alpha3().VirtualServices(namespace).Create(ctx, fooVS, metav1.CreateOptions{})
	validIstioClient.NetworkingV1alpha3().VirtualServices(namespace).Create(ctx, barVS, metav1.CreateOptions{})

	testcases := []struct {
		name                       string
		identity                   string
		env                        string
		rc                         *RemoteController
		additionalEndpointSuffixes []string
		dnsPrefix                  string
		expectedError              error
		expectedDeletedVSNames     []string
	}{
		{
			name:                       "Given additional endpoint suffixes, when passed identity is empty, func should return an error",
			identity:                   "",
			additionalEndpointSuffixes: []string{"foo"},
			expectedError:              fmt.Errorf("identity passed is empty"),
		},
		{
			name:                       "Given additional endpoint suffixes, when passed env is empty, func should return an error",
			identity:                   "test00",
			env:                        "",
			additionalEndpointSuffixes: []string{"foo"},
			expectedError:              fmt.Errorf("env passed is empty"),
		},
		{
			name:                       "Given additional endpoint suffixes, when valid identity,env and additional suffix params are passed, func should not return any error and delete the desired virtualservices",
			identity:                   "test00",
			env:                        "stage",
			dnsPrefix:                  "",
			additionalEndpointSuffixes: []string{"foo", "bar"},
			expectedError:              nil,
			expectedDeletedVSNames:     []string{"stage.test00.foo-vs", "stage.test00.bar-vs"},
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			admiralParams.AdditionalEndpointSuffixes = tc.additionalEndpointSuffixes
			common.ResetSync()
			common.InitializeConfig(admiralParams)

			err := deleteAdditionalEndpoints(ctxLogger, ctx, tc.rc, tc.identity, tc.env, namespace, tc.dnsPrefix)

			if err != nil && tc.expectedError != nil {
				if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("expected %s, got %s", tc.expectedError.Error(), err.Error())
				}
			} else if err != tc.expectedError {
				t.Errorf("expected %v, got %v", tc.expectedError, err)
			}

			for _, expectedDeletedVSName := range tc.expectedDeletedVSNames {
				if err == nil && expectedDeletedVSName != "" {
					_, err := tc.rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Get(context.Background(), expectedDeletedVSName, metav1.GetOptions{})
					if err != nil && !k8sErrors.IsNotFound(err) {
						t.Errorf("test failed as VS should have been deleted. error: %v", err)
					}
				}
			}

		})
	}

}

func TestGetAdmiralGeneratedVirtualService(t *testing.T) {

	ctx := context.Background()
	namespace := "testns"

	fooVS := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name: "stage.test00.foo-vs",
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{
			Hosts: []string{"stage.test00.foo", "stage.test00.bar"},
		},
	}

	testcases := []struct {
		name             string
		labels           map[string]string
		annotations      map[string]string
		remoteController *RemoteController
		virtualService   *v1alpha3.VirtualService
		expectedError    error
		expectedVS       *v1alpha3.VirtualService
	}{
		{
			name:             "Given valid listOptions, when remoteController is nil, func should return an error",
			labels:           make(map[string]string),
			annotations:      make(map[string]string),
			virtualService:   fooVS,
			remoteController: nil,
			expectedError:    fmt.Errorf("error fetching admiral generated virtualservice as remote controller not initialized"),
		},
		{
			name:             "Given valid listOptions, when VirtualServiceController is nil, func should return an error",
			labels:           make(map[string]string),
			annotations:      make(map[string]string),
			virtualService:   fooVS,
			remoteController: &RemoteController{},
			expectedError:    fmt.Errorf("error fetching admiral generated virtualservice as VirtualServiceController controller not initialized"),
		},
		{
			name:           "Given valid listOptions, when VS matches the listOption labels and it is created by admiral, func should not return an error and return the VS",
			labels:         map[string]string{"admiral.io/env": "stage", "identity": "test00"},
			annotations:    map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			virtualService: fooVS,
			remoteController: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{},
			},
			expectedError: nil,
			expectedVS: &v1alpha3.VirtualService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "stage.test00.foo-vs",
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {

			tc.virtualService.Labels = tc.labels
			tc.virtualService.Annotations = tc.annotations
			validIstioClient := istiofake.NewSimpleClientset()
			validIstioClient.NetworkingV1alpha3().VirtualServices(namespace).Create(ctx, tc.virtualService, metav1.CreateOptions{})

			if tc.remoteController != nil && tc.remoteController.VirtualServiceController != nil {
				tc.remoteController.VirtualServiceController = &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				}
			}

			actualVS, err := getAdmiralGeneratedVirtualService(ctx, tc.remoteController, "stage.test00.foo-vs", namespace)

			if err != nil && tc.expectedError != nil {
				if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("expected %s, got %s", tc.expectedError.Error(), err.Error())
				}
			} else if err != tc.expectedError {
				t.Errorf("expected %v, got %v", tc.expectedError, err)
			}

			if err == nil && actualVS != nil {
				if actualVS.Name != tc.expectedVS.Name {
					t.Errorf("expected virtualservice %s got %s", tc.expectedVS.Name, actualVS.Name)
				}
			}
		})
	}
}

func TestDoGenerateAdditionalEndpoints(t *testing.T) {
	ctxLogger := logrus.WithFields(logrus.Fields{
		"type": "modifySE",
	})
	admiralCache := AdmiralCache{
		IdentityDependencyCache:           common.NewMapOfMaps(),
		IdentitiesWithAdditionalEndpoints: &sync.Map{},
	}
	testcases := []struct {
		name                           string
		labels                         map[string]string
		additionalEndpointSuffixes     []string
		additionalEndpointLabelFilters []string
		expectedResult                 bool
	}{
		{
			name:           "Given additional endpoint suffixes and labels, when no additional endpoint suffixes are set, then the func should return false",
			labels:         map[string]string{"foo": "bar"},
			expectedResult: false,
		},
		{
			name:                       "Given additional endpoint suffixes and labels, when no additional endpoint labels filters are set, then the func should return false",
			labels:                     map[string]string{"foo": "bar"},
			additionalEndpointSuffixes: []string{"fuzz"},
			expectedResult:             false,
		},
		{
			name:                           "Given additional endpoint suffixes and labels, when additional endpoint labels filters contains '*', then the func should return true",
			labels:                         map[string]string{"foo": "bar"},
			additionalEndpointSuffixes:     []string{"fuzz"},
			additionalEndpointLabelFilters: []string{"*"},
			expectedResult:                 true,
		},
		{
			name:                           "Given additional endpoint suffixes and labels, when additional endpoint labels filters contains is not in the rollout/deployment annotation, then the func should return false",
			labels:                         map[string]string{"foo": "bar"},
			additionalEndpointSuffixes:     []string{"fuzz"},
			additionalEndpointLabelFilters: []string{"baz"},
			expectedResult:                 false,
		},
		{
			name:                           "Given additional endpoint suffixes and labels, when additional endpoint labels filters contains the rollout/deployment annotation, then the func should return true",
			labels:                         map[string]string{"foo": "bar"},
			additionalEndpointSuffixes:     []string{"fuzz"},
			additionalEndpointLabelFilters: []string{"foo"},
			expectedResult:                 true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {

			admiralParams := common.AdmiralParams{
				AdditionalEndpointSuffixes:     tc.additionalEndpointSuffixes,
				AdditionalEndpointLabelFilters: tc.additionalEndpointLabelFilters,
			}
			common.ResetSync()
			common.InitializeConfig(admiralParams)

			actual := doGenerateAdditionalEndpoints(ctxLogger, tc.labels, "", &admiralCache)

			if actual != tc.expectedResult {
				t.Errorf("expected %t, got %t", tc.expectedResult, actual)
			}
		})
	}
}

func TestDoGenerateAdditionalEndpointsForDependencies(t *testing.T) {
	ctxLogger := logrus.WithFields(logrus.Fields{
		"type": "modifySE",
	})
	assetWithRequiredLabels := "assetWithRequiredLabels"
	assetWithoutRequiredLabelsAndNotADependency := "assetWithoutRequiredLabelsAndNotADependency"
	assetWithoutRequiredLabelsAndADependency := "assetWithoutRequiredLabelsAndADependency"

	admiralParams := common.AdmiralParams{
		AdditionalEndpointSuffixes:     []string{"fuzz"},
		AdditionalEndpointLabelFilters: []string{"foo", "bar"},
	}

	common.ResetSync()
	common.InitializeConfig(admiralParams)

	admiralCache := AdmiralCache{
		IdentityDependencyCache:           common.NewMapOfMaps(),
		IdentitiesWithAdditionalEndpoints: &sync.Map{},
	}

	admiralCache.IdentityDependencyCache.Put(assetWithoutRequiredLabelsAndADependency, assetWithRequiredLabels, assetWithRequiredLabels)

	testcases := []struct {
		name           string
		labels         map[string]string
		identity       string
		expectedResult bool
	}{
		{
			"AdditionalEndpoints should be generated for an asset with required labels",
			map[string]string{"foo": "baz"},
			assetWithRequiredLabels,
			true,
		},
		{
			"Additional endpoints should not be generated for an asset without the required labels and not a dependency of an asset whose additional endpoints have been generated",
			map[string]string{"unknown_label": "val"},
			assetWithoutRequiredLabelsAndNotADependency,
			false,
		},
		{
			"Additional endpoints should be generated for an asset that is a dependency of other asset with additional endpoints, even if it does not have the required labels.",
			map[string]string{"unknown_label": "val"},
			assetWithoutRequiredLabelsAndADependency,
			true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := doGenerateAdditionalEndpoints(ctxLogger, tc.labels, tc.identity, &admiralCache)
			if actual != tc.expectedResult {
				t.Errorf("expected %t, got %t", tc.expectedResult, actual)
			}
		})
	}

}

func TestFetchResourceLabels(t *testing.T) {
	var (
		deploymentName     = "test-deployment"
		rolloutName        = "test-rollout"
		namespace          = "test-namespace"
		identityLabel      = "foobar"
		existingCluster    = "existingCluster"
		nonExistingCluster = "nonExistingCluster"
		deployment1        = makeTestDeployment(deploymentName, namespace, identityLabel)
		rollout1           = makeTestRollout(rolloutName, namespace, identityLabel)
		labels             = map[string]string{
			"identity": identityLabel,
		}
	)
	cases := []struct {
		name              string
		cluster           string
		sourceDeployments map[string]*k8sAppsV1.Deployment
		sourceRollouts    map[string]*argo.Rollout
		expectedLabels    map[string]string
	}{
		{
			name: "Given cluster exists in sourceDeployments, " +
				"When, cluster contains a deployment with Labels, " +
				"When, fetchResourceLabel is called, " +
				"Then, it should return the expected label",
			cluster: existingCluster,
			sourceDeployments: map[string]*k8sAppsV1.Deployment{
				existingCluster: deployment1,
			},
			expectedLabels: labels,
		},
		{
			name: "Given cluster does not exist in sourceDeployments, " +
				"When, cluster contains a deployment with Labels, " +
				"When, fetchResourceLabel is called, " +
				"Then, it should return the expected label",
			cluster: nonExistingCluster,
			sourceDeployments: map[string]*k8sAppsV1.Deployment{
				existingCluster: deployment1,
			},
			expectedLabels: nil,
		},
		{
			name: "Given cluster exists in sourceRollouts, " +
				"When, cluster contains a rollout with Labels, " +
				"When, fetchResourceLabel is called, " +
				"Then, it should return the expected label",
			cluster: existingCluster,
			sourceRollouts: map[string]*argo.Rollout{
				existingCluster: &rollout1,
			},
			expectedLabels: labels,
		},
		{
			name: "Given cluster does not exist in sourceRollouts, " +
				"When, cluster contains a rollout with Labels, " +
				"When, fetchResourceLabel is called, " +
				"Then, it should return the expected label",
			cluster: nonExistingCluster,
			sourceRollouts: map[string]*argo.Rollout{
				existingCluster: &rollout1,
			},
			expectedLabels: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			label := fetchResourceLabel(
				c.sourceDeployments,
				c.sourceRollouts,
				c.cluster,
			)
			if !reflect.DeepEqual(label, c.expectedLabels) {
				t.Errorf("expected: %v, got: %v", c.expectedLabels, label)
			}
		})
	}
}

func TestGetWorkloadData(t *testing.T) {
	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})
	common.ResetSync()
	common.InitializeConfig(admiralParamsForServiceEntryTests())

	currentTime := time.Now().UTC().Format(time.RFC3339)

	var se = &v1alpha3.ServiceEntry{
		//nolint
		Spec: istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"dev.custom.global"},
			Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
				{
					Address:  "override.svc.cluster.local",
					Ports:    map[string]uint32{"http": 80},
					Network:  "mesh1",
					Locality: "us-west",
					Weight:   100,
					Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
				},
			},
		},
	}

	se.Annotations = map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "custom"}
	se.Labels = map[string]string{"env": "dev"}

	var failoverGtp = &admiralV1.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test.e2e.foo-gtp",
			Annotations: map[string]string{common.LastUpdatedAt: currentTime},
			ManagedFields: []metav1.ManagedFieldsEntry{
				{
					Manager: "ewok-mesh-agent",
				},
			},
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    model.TrafficPolicy_FAILOVER,
					DnsPrefix: common.Default,
					Target: []*model.TrafficGroup{
						{
							Region: "us-west-2",
							Weight: 100,
						},
					},
				},
			},
		},
	}

	var topologyGtp = &admiralV1.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test.e2e.foo-gtp",
			Annotations: map[string]string{common.LastUpdatedAt: currentTime},
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    model.TrafficPolicy_TOPOLOGY,
					DnsPrefix: common.Default,
				},
			},
		},
	}

	expectedWorkloadTid := WorkloadData{
		AssetAlias:          "custom",
		Endpoint:            "dev.custom.global",
		Env:                 "dev",
		DnsPrefix:           common.Default,
		TrafficDistribution: make(map[string]int32),
		LbType:              model.TrafficPolicy_TOPOLOGY.String(),
		Aliases:             nil,
		GtpManagedBy:        "github",
		GtpId:               "foo-bar",
		LastUpdatedAt:       currentTime,
		FailedClusters:      []string{"dev"},
	}

	expectedWorkloadVersion := WorkloadData{
		AssetAlias:          "custom",
		Endpoint:            "dev.custom.global",
		Env:                 "dev",
		DnsPrefix:           common.Default,
		TrafficDistribution: make(map[string]int32),
		LbType:              model.TrafficPolicy_TOPOLOGY.String(),
		Aliases:             nil,
		GtpManagedBy:        "github",
		GtpId:               "007",
		LastUpdatedAt:       currentTime,
		FailedClusters:      []string{"dev"},
	}

	var gtpWithIntuit_tid = &admiralV1.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gtp-tid-annotation",
			Annotations: map[string]string{
				common.IntuitTID:     "foo-bar",
				common.LastUpdatedAt: currentTime,
			},
		},

		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    model.TrafficPolicy_TOPOLOGY,
					DnsPrefix: common.Default,
				},
			},
		},
	}

	var gtpWithVersion = &admiralV1.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "gtp-version",
			ResourceVersion: "007",
			Annotations:     map[string]string{common.LastUpdatedAt: currentTime},
		},

		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    model.TrafficPolicy_TOPOLOGY,
					DnsPrefix: common.Default,
				},
			},
		},
	}

	var workloadDataWithoutGTP = WorkloadData{
		AssetAlias:          "custom",
		Endpoint:            "dev.custom.global",
		Env:                 "dev",
		Aliases:             []string{"dev.custom.testsuffix"},
		TrafficDistribution: map[string]int32{},
	}

	var workloadDataWithFailoverGTP = WorkloadData{
		AssetAlias: "custom",
		Endpoint:   "dev.custom.global",
		Env:        "dev",
		DnsPrefix:  "default",
		Aliases:    []string{"dev.custom.testsuffix"},
		LbType:     model.TrafficPolicy_FAILOVER.String(),
		TrafficDistribution: map[string]int32{
			"us-west-2": 100,
		},
		GtpManagedBy:   "mesh-agent",
		LastUpdatedAt:  currentTime,
		SuccessCluster: []string{"dev"},
	}

	var workloadDataWithTopologyGTP = WorkloadData{
		AssetAlias:          "custom",
		Endpoint:            "dev.custom.global",
		Env:                 "dev",
		DnsPrefix:           "default",
		Aliases:             []string{"dev.custom.testsuffix"},
		TrafficDistribution: map[string]int32{},
		LbType:              model.TrafficPolicy_TOPOLOGY.String(),
		GtpManagedBy:        "github",
		LastUpdatedAt:       currentTime,
		SuccessCluster:      []string{"dev"},
	}

	testCases := []struct {
		name                 string
		serviceEntry         *v1alpha3.ServiceEntry
		workloadData         WorkloadData
		globalTrafficPolicy  *admiralV1.GlobalTrafficPolicy
		additionalEndpoints  []string
		expectedWorkloadData WorkloadData
		isSuccess            bool
	}{
		{
			name: "Given serviceentry object and no globaltrafficpolicy, " +
				"When getWorkloadData is called, " +
				"Then it should return workloadData without global traffic policy data",
			serviceEntry:         se,
			globalTrafficPolicy:  nil,
			additionalEndpoints:  []string{"dev.custom.testsuffix"},
			expectedWorkloadData: workloadDataWithoutGTP,
			isSuccess:            true,
		},
		{
			name: "Given serviceentry object and failover globaltrafficpolicy object, " +
				"When getWorkloadData is called, " +
				"Then it should return workloadData with failover traffic configuration",
			serviceEntry:         se,
			globalTrafficPolicy:  failoverGtp,
			additionalEndpoints:  []string{"dev.custom.testsuffix"},
			expectedWorkloadData: workloadDataWithFailoverGTP,
			isSuccess:            true,
		},
		{
			name: "Given serviceentry object and topology globaltrafficpolicy object, " +
				"When getWorkloadData is called, " +
				"Then it should return workloadData with topology traffic configuration",
			serviceEntry:         se,
			globalTrafficPolicy:  topologyGtp,
			additionalEndpoints:  []string{"dev.custom.testsuffix"},
			expectedWorkloadData: workloadDataWithTopologyGTP,
			isSuccess:            true,
		},
		{
			name: "Given GTP contains intuit_tid in annotation, " +
				"When getWorkloadData is called, " +
				"Then it should return tid in workload object",
			serviceEntry:         se,
			globalTrafficPolicy:  gtpWithIntuit_tid,
			additionalEndpoints:  nil,
			expectedWorkloadData: expectedWorkloadTid,
			isSuccess:            false,
		},
		{
			name: "Given GTP intuit_tid annotation missing, " +
				"When getWorkloadData is called, " +
				"Then it should return k8s resource version in workload object",
			serviceEntry:         se,
			globalTrafficPolicy:  gtpWithVersion,
			additionalEndpoints:  nil,
			expectedWorkloadData: expectedWorkloadVersion,
			isSuccess:            false,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			workloadData := getWorkloadData(ctxLogger, c.serviceEntry, c.globalTrafficPolicy, c.additionalEndpoints, istioNetworkingV1Alpha3.DestinationRule{}, "dev", c.isSuccess)
			if !reflect.DeepEqual(workloadData, c.expectedWorkloadData) {
				assert.Fail(t, "actual and expected workload data do not match. Actual : %v. Expected : %v.", workloadData, c.expectedWorkloadData)
			}
		})
	}
}

func TestGetWorkloadDataActivePassiveEnabled(t *testing.T) {
	currentTime := time.Now().UTC().Format(time.RFC3339)
	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})
	// Enable Active-Passive
	admiralParams := common.AdmiralParams{
		CacheReconcileDuration: 10 * time.Minute,
		LabelSet: &common.LabelSet{
			EnvKey: "env",
		},
	}
	admiralParams.EnableActivePassive = true
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	var se = &v1alpha3.ServiceEntry{
		//nolint
		Spec: istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"dev.custom.global"},
			Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
				{
					Address:  "override.svc.cluster.local",
					Ports:    map[string]uint32{"http": 80},
					Network:  "mesh1",
					Locality: "us-west",
					Weight:   100,
					Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
				},
			},
		},
	}

	mTLSWestNoDistribution := &istioNetworkingV1Alpha3.TrafficPolicy{
		Tls: &istioNetworkingV1Alpha3.ClientTLSSettings{
			Mode: istioNetworkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &istioNetworkingV1Alpha3.ConnectionPoolSettings{
			Http: &istioNetworkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &istioNetworkingV1Alpha3.LoadBalancerSettings{
			LbPolicy: &istioNetworkingV1Alpha3.LoadBalancerSettings_Simple{
				Simple: istioNetworkingV1Alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	mTLSWest := &istioNetworkingV1Alpha3.TrafficPolicy{
		Tls: &istioNetworkingV1Alpha3.ClientTLSSettings{
			Mode: istioNetworkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &istioNetworkingV1Alpha3.ConnectionPoolSettings{
			Http: &istioNetworkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &istioNetworkingV1Alpha3.LoadBalancerSettings{
			LbPolicy: &istioNetworkingV1Alpha3.LoadBalancerSettings_Simple{
				Simple: istioNetworkingV1Alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			LocalityLbSetting: &istioNetworkingV1Alpha3.LocalityLoadBalancerSetting{
				Distribute: []*istioNetworkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
					{
						From: "*",
						To:   map[string]uint32{"us-west-2": 100},
					},
				},
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	mTLSWestAfterGTP := &istioNetworkingV1Alpha3.TrafficPolicy{
		Tls: &istioNetworkingV1Alpha3.ClientTLSSettings{
			Mode: istioNetworkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &istioNetworkingV1Alpha3.ConnectionPoolSettings{
			Http: &istioNetworkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &istioNetworkingV1Alpha3.LoadBalancerSettings{
			LbPolicy: &istioNetworkingV1Alpha3.LoadBalancerSettings_Simple{
				Simple: istioNetworkingV1Alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			LocalityLbSetting: &istioNetworkingV1Alpha3.LocalityLoadBalancerSetting{
				Distribute: []*istioNetworkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
					{
						From: "us-west-2/*",
						To:   map[string]uint32{"us-west-2": 70, "us-east-2": 30},
					},
				},
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	noGtpNoDistributionDr := istioNetworkingV1Alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLSWestNoDistribution,
	}

	noGtpDistributionDr := istioNetworkingV1Alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLSWest,
	}

	gtpDr := istioNetworkingV1Alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLSWestAfterGTP,
	}

	se.Annotations = map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue, common.GetWorkloadIdentifier(): "custom"}
	se.Labels = map[string]string{"env": "dev"}

	var failoverGtp = &admiralV1.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test.e2e.foo-gtp",
			Annotations: map[string]string{common.LastUpdatedAt: currentTime},
			ManagedFields: []metav1.ManagedFieldsEntry{
				{
					Manager: "ewok-mesh-agent",
				},
			},
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				&model.TrafficPolicy{
					LbType:    model.TrafficPolicy_FAILOVER,
					DnsPrefix: common.Default,
					Target: []*model.TrafficGroup{
						{
							Region: "us-west-2",
							Weight: 100,
						},
					},
				},
			},
		},
	}

	var topologyGtp = &admiralV1.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test.e2e.foo-gtp",
			Annotations: map[string]string{common.LastUpdatedAt: currentTime},
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				&model.TrafficPolicy{
					LbType:    model.TrafficPolicy_TOPOLOGY,
					DnsPrefix: common.Default,
				},
			},
		},
	}

	expectedWorkloadTid := WorkloadData{
		AssetAlias:          "custom",
		Endpoint:            "dev.custom.global",
		Env:                 "dev",
		DnsPrefix:           common.Default,
		TrafficDistribution: make(map[string]int32),
		LbType:              model.TrafficPolicy_TOPOLOGY.String(),
		Aliases:             nil,
		GtpManagedBy:        "github",
		GtpId:               "foo-bar",
		LastUpdatedAt:       currentTime,
		FailedClusters:      []string{"dev"},
	}

	expectedWorkloadVersion := WorkloadData{
		AssetAlias:          "custom",
		Endpoint:            "dev.custom.global",
		Env:                 "dev",
		DnsPrefix:           common.Default,
		TrafficDistribution: make(map[string]int32),
		LbType:              model.TrafficPolicy_TOPOLOGY.String(),
		Aliases:             nil,
		GtpManagedBy:        "github",
		GtpId:               "007",
		LastUpdatedAt:       currentTime,
		FailedClusters:      []string{"dev"},
	}

	var gtpWithIntuit_tid = &admiralV1.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gtp-tid-annotation",
			Annotations: map[string]string{
				common.IntuitTID: "foo-bar",
			},
		},

		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				&model.TrafficPolicy{
					LbType:    model.TrafficPolicy_TOPOLOGY,
					DnsPrefix: common.Default,
				},
			},
		},
	}

	var gtpWithVersion = &admiralV1.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "gtp-version",
			ResourceVersion: "007",
		},

		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				&model.TrafficPolicy{
					LbType:    model.TrafficPolicy_TOPOLOGY,
					DnsPrefix: common.Default,
				},
			},
		},
	}

	var workloadDataWithoutGTP = WorkloadData{
		AssetAlias:          "custom",
		Endpoint:            "dev.custom.global",
		Env:                 "dev",
		Aliases:             []string{"dev.custom.testsuffix"},
		TrafficDistribution: map[string]int32{},
	}

	var workloadDataWithoutGTPDefaultDistribution = WorkloadData{
		AssetAlias:          "custom",
		Endpoint:            "dev.custom.global",
		Env:                 "dev",
		Aliases:             []string{"dev.custom.testsuffix"},
		TrafficDistribution: map[string]int32{"us-west-2": 100},
		LbType:              model.TrafficPolicy_LbType_name[int32(model.TrafficPolicy_FAILOVER)],
	}

	var workloadDataWithFailoverGTP = WorkloadData{
		AssetAlias: "custom",
		Endpoint:   "dev.custom.global",
		Env:        "dev",
		DnsPrefix:  "default",
		Aliases:    []string{"dev.custom.testsuffix"},
		LbType:     model.TrafficPolicy_FAILOVER.String(),
		TrafficDistribution: map[string]int32{
			"us-west-2": 100,
		},
		GtpManagedBy:   "mesh-agent",
		LastUpdatedAt:  currentTime,
		SuccessCluster: []string{"dev"},
	}

	var workloadDataWithTopologyGTP = WorkloadData{
		AssetAlias:          "custom",
		Endpoint:            "dev.custom.global",
		Env:                 "dev",
		DnsPrefix:           "default",
		Aliases:             []string{"dev.custom.testsuffix"},
		TrafficDistribution: map[string]int32{},
		LbType:              model.TrafficPolicy_TOPOLOGY.String(),
		GtpManagedBy:        "github",
		LastUpdatedAt:       currentTime,
		SuccessCluster:      []string{"dev"},
	}

	testCases := []struct {
		name                 string
		serviceEntry         *v1alpha3.ServiceEntry
		workloadData         WorkloadData
		globalTrafficPolicy  *admiralV1.GlobalTrafficPolicy
		additionalEndpoints  []string
		dr                   istioNetworkingV1Alpha3.DestinationRule
		expectedWorkloadData WorkloadData
		isSuccess            bool
	}{
		{
			name: "Given serviceentry object and no globaltrafficpolicy, " +
				"And destinationRule is also not present" +
				"When getWorkloadData is called, " +
				"Then it should return workloadData with only the AssetAlias, Endpoint, Env and Aliases set",
			serviceEntry:         se,
			globalTrafficPolicy:  nil,
			additionalEndpoints:  []string{"dev.custom.testsuffix"},
			dr:                   istioNetworkingV1Alpha3.DestinationRule{},
			expectedWorkloadData: workloadDataWithoutGTP,
			isSuccess:            false,
		},
		{
			name: "Given serviceentry object and no globaltrafficpolicy, " +
				"And destinationRule present but does not have any distribution" +
				"When getWorkloadData is called, " +
				"Then it should return workloadData with only the AssetAlias, Endpoint, Env and Aliases set",
			serviceEntry:         se,
			globalTrafficPolicy:  nil,
			additionalEndpoints:  []string{"dev.custom.testsuffix"},
			dr:                   noGtpNoDistributionDr,
			expectedWorkloadData: workloadDataWithoutGTP,
			isSuccess:            false,
		},
		{
			name: "Given serviceentry object and no globaltrafficpolicy, " +
				"And destinationRule present and has the default distribution - From is set to *" +
				"When getWorkloadData is called, " +
				"Then it should return workloadData with only the AssetAlias, Endpoint, Env ,Aliases and TrafficDistribution set",
			serviceEntry:         se,
			globalTrafficPolicy:  nil,
			additionalEndpoints:  []string{"dev.custom.testsuffix"},
			dr:                   noGtpDistributionDr,
			expectedWorkloadData: workloadDataWithoutGTPDefaultDistribution,
			isSuccess:            false,
		},
		{
			name: "Given serviceentry object and no globaltrafficpolicy, " +
				"And destinationRule present without the default distribution - From is set to *" +
				"When getWorkloadData is called, " +
				"Then it should return workloadData with only the AssetAlias, Endpoint, Env and Aliases set",
			serviceEntry:         se,
			globalTrafficPolicy:  nil,
			additionalEndpoints:  []string{"dev.custom.testsuffix"},
			dr:                   gtpDr,
			expectedWorkloadData: workloadDataWithoutGTP,
			isSuccess:            false,
		},
		{
			name: "Given serviceentry object and failover globaltrafficpolicy object, " +
				"And destinationRule is also not present" +
				"When getWorkloadData is called, " +
				"Then it should return workloadData with failover traffic configuration",
			serviceEntry:         se,
			globalTrafficPolicy:  failoverGtp,
			additionalEndpoints:  []string{"dev.custom.testsuffix"},
			dr:                   istioNetworkingV1Alpha3.DestinationRule{},
			expectedWorkloadData: workloadDataWithFailoverGTP,
			isSuccess:            true,
		},
		{
			name: "Given serviceentry object and topology globaltrafficpolicy object, " +
				"And destinationRule is also not present" +
				"When getWorkloadData is called, " +
				"Then it should return workloadData with topology traffic configuration",
			serviceEntry:         se,
			globalTrafficPolicy:  topologyGtp,
			additionalEndpoints:  []string{"dev.custom.testsuffix"},
			dr:                   istioNetworkingV1Alpha3.DestinationRule{},
			expectedWorkloadData: workloadDataWithTopologyGTP,
			isSuccess:            true,
		},
		{
			name: "Given GTP contains intuit_tid in annotation, " +
				"And destinationRule is also not present" +
				"When getWorkloadData is called, " +
				"Then it should return tid in workload object",
			serviceEntry:         se,
			globalTrafficPolicy:  gtpWithIntuit_tid,
			additionalEndpoints:  nil,
			dr:                   istioNetworkingV1Alpha3.DestinationRule{},
			expectedWorkloadData: expectedWorkloadTid,
			isSuccess:            false,
		},
		{
			name: "Given GTP intuit_tid annotation missing, " +
				"And destinationRule is also not present" +
				"When getWorkloadData is called, " +
				"Then it should return k8s resource version in workload object",
			serviceEntry:         se,
			globalTrafficPolicy:  gtpWithVersion,
			additionalEndpoints:  nil,
			dr:                   istioNetworkingV1Alpha3.DestinationRule{},
			expectedWorkloadData: expectedWorkloadVersion,
			isSuccess:            false,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			workloadData := getWorkloadData(ctxLogger, c.serviceEntry, c.globalTrafficPolicy, c.additionalEndpoints, c.dr, "dev", c.isSuccess)
			if !reflect.DeepEqual(workloadData, c.expectedWorkloadData) {
				assert.Fail(t, "actual and expected workload data do not match.", "Actual : %v. Expected : %v.", workloadData, c.expectedWorkloadData)
			}
		})
	}
}

type mockDatabaseClientWithError struct {
	dynamoClient *DynamoClient
	database     *admiralapiv1.DynamoDB
}

func (mockDatabaseClientWithError) Update(data interface{}, logger *logrus.Entry) error {
	return fmt.Errorf("failed to update workloadData")
}

func (mockDatabaseClientWithError) Delete(data interface{}, logger *logrus.Entry) error {
	return fmt.Errorf("failed to delete workloadData")
}

func (mockDatabaseClientWithError) Get(env string, identity string) (interface{}, error) {
	return nil, fmt.Errorf("failed to get workloadData items")
}

func TestDeployRolloutMigration(t *testing.T) {
	common.ResetSync()
	common.InitializeConfig(admiralParamsForServiceEntryTests())
	var (
		env                     = "test"
		stop                    = make(chan struct{})
		foobarMetadataName      = "foobar"
		foobarMetadataNamespace = "foobar-ns"
		identity                = "identity"
		testRollout1            = makeTestRollout(foobarMetadataName, foobarMetadataNamespace, identity)
		testDeployment1         = makeTestDeployment(foobarMetadataName, foobarMetadataNamespace, identity)
		clusterID               = "test-dev-k8s"
		clusterDependentID      = "test-dev-dependent-k8s"
		fakeIstioClient         = istiofake.NewSimpleClientset()
		config                  = rest.Config{Host: "localhost"}
		resyncPeriod            = time.Millisecond * 1
		expectedServiceEntry    = &v1alpha3.ServiceEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test.identity.mesh-se",
				Namespace: "ns",
			},
			Spec: istioNetworkingV1Alpha3.ServiceEntry{
				Hosts:     []string{"test." + identity + ".mesh"},
				Addresses: []string{"127.0.0.1"},
				Ports: []*istioNetworkingV1Alpha3.ServicePort{
					{
						Number:   80,
						Protocol: "http",
						Name:     "http",
					},
				},
				Location:   istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
				Resolution: istioNetworkingV1Alpha3.ServiceEntry_DNS,
				Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
					{
						Address: "foobar.foobar-ns.svc.cluster.local",
						Ports: map[string]uint32{
							"http": 8090,
						},
						Labels:   map[string]string{"type": common.Deployment, "security.istio.io/tlsMode": "istio"},
						Locality: "us-west-2",
					},
					{
						Address: "foobar-stable.foobar-ns.svc.cluster.local",
						Ports: map[string]uint32{
							"http": 8090,
						},
						Labels:   map[string]string{"type": common.Rollout, "security.istio.io/tlsMode": "istio"},
						Locality: "us-west-2",
					},
				},
				SubjectAltNames: []string{"spiffe://prefix/" + identity},
			},
		}

		serviceEntryAddressStore = &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{
				"test." + identity + ".mesh-se": "127.0.0.1",
			},
			Addresses: []string{},
		}
		serviceForRollout = &coreV1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:              foobarMetadataName + "-stable",
				Namespace:         foobarMetadataNamespace,
				CreationTimestamp: metav1.NewTime(time.Now()),
			},
			Spec: coreV1.ServiceSpec{
				Selector: map[string]string{"app": identity},
				Ports: []coreV1.ServicePort{
					{
						Name: "http",
						Port: 8090,
					},
				},
			},
		}
		serviceForDeployment = &coreV1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:              foobarMetadataName,
				Namespace:         foobarMetadataNamespace,
				CreationTimestamp: metav1.NewTime(time.Now().AddDate(-1, 1, 1)),
			},
			Spec: coreV1.ServiceSpec{
				Selector: map[string]string{"app": identity},
				Ports: []coreV1.ServicePort{
					{
						Name: "http",
						Port: 8090,
					},
				},
			},
		}
		serviceForIngress = &coreV1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "east.aws.lb",
				Namespace: "istio-system",
				Labels:    map[string]string{"app": "gatewayapp"},
			},
			Spec: coreV1.ServiceSpec{
				Ports: []coreV1.ServicePort{
					{
						Name: "http",
						Port: 8090,
					},
				},
			},
			Status: coreV1.ServiceStatus{
				LoadBalancer: coreV1.LoadBalancerStatus{
					Ingress: []coreV1.LoadBalancerIngress{
						{
							Hostname: "east.aws.lb",
						},
					},
				},
			},
		}
		rr1, _ = InitAdmiral(context.Background(), admiralParamsForServiceEntryTests())
	)

	deploymentController, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	deploymentController.Cache.UpdateDeploymentToClusterCache(identity, testDeployment1)

	deploymentDependentController, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}

	rolloutController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	rolloutController.Cache.UpdateRolloutToClusterCache(identity, &testRollout1)

	rolloutDependentController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}

	serviceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}

	serviceDependentController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}

	virtualServiceController, err := istio.NewVirtualServiceController(make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}

	gtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
		t.FailNow()
	}

	serviceController.Cache.Put(serviceForDeployment)
	serviceController.Cache.Put(serviceForRollout)
	serviceController.Cache.Put(serviceForIngress)

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
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic: gtpc,
	}

	dependentRc := &RemoteController{
		ClusterID:                clusterDependentID,
		DeploymentController:     deploymentDependentController,
		RolloutController:        rolloutDependentController,
		ServiceController:        serviceDependentController,
		VirtualServiceController: virtualServiceController,
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic: gtpc,
	}

	rr1.PutRemoteController(clusterID, rc)
	rr1.PutRemoteController(clusterDependentID, dependentRc)
	rr1.StartTime = time.Now()
	rr1.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	rr1.AdmiralCache.IdentityClusterCache.Put("identity", clusterID, clusterID)

	testCases := []struct {
		name                   string
		assetIdentity          string
		readOnly               bool
		remoteRegistry         *RemoteRegistry
		expectedServiceEntries *v1alpha3.ServiceEntry
		expectedErr            error
	}{
		{
			name: "Given asset is using a deployment," +
				"And now to start migration starts using a rollout," +
				"Then, it should return an SE with the one endpoints for deployment and rollout",
			assetIdentity:          "identity",
			remoteRegistry:         rr1,
			expectedServiceEntries: expectedServiceEntry,
			expectedErr:            nil,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.readOnly {
				commonUtil.CurrentAdmiralState.ReadOnly = ReadOnlyEnabled
			}

			ctx := context.Background()
			ctx = context.WithValue(ctx, "clusterName", clusterID)
			ctx = context.WithValue(ctx, "eventResourceType", common.Deployment)

			_, err = modifyServiceEntryForNewServiceOrPod(
				ctx,
				admiral.Add,
				env,
				c.assetIdentity,
				c.remoteRegistry,
			)

			assert.Equal(t, err, c.expectedErr)

			seList, _ := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(common.GetSyncNamespace()).List(ctx, metav1.ListOptions{})
			if !reflect.DeepEqual(seList.Items[0].Spec.Endpoints, expectedServiceEntry.Spec.Endpoints) {
				t.Errorf("Expected SEs: %v Got: %v", expectedServiceEntry.Spec.Endpoints, seList.Items[0].Spec.Endpoints)
			}
		})
	}
}

// Helper function to create a fake VirtualService object with a given name and namespace
type vsOverrides func(vs *v1alpha3.VirtualService)

func createFakeVS(name string, opts ...vsOverrides) *v1alpha3.VirtualService {
	vs := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	for _, o := range opts {
		o(vs)
	}

	return vs
}

func TestGetExistingVS(t *testing.T) {
	tests := []struct {
		name        string
		existing    bool
		fakeVSFunc  func() *v1alpha3.VirtualService
		expectedErr error
	}{
		{
			name:     "VirtualService not found",
			existing: false,
			fakeVSFunc: func() *v1alpha3.VirtualService {
				return createFakeVS("test-vs", func(vs *v1alpha3.VirtualService) {
					vs.Namespace = common.GetAdmiralParams().SyncNamespace
				})
			},
			expectedErr: k8sErrors.NewNotFound(schema.GroupResource{Group: "networking.istio.io", Resource: "virtualservices"}, "test-vs"),
		},
		{
			name:     "VirtualService found",
			existing: true,
			fakeVSFunc: func() *v1alpha3.VirtualService {
				return createFakeVS("test-vs", func(vs *v1alpha3.VirtualService) {
					vs.Namespace = common.GetAdmiralParams().SyncNamespace
				})
			},
			expectedErr: nil,
		},
	}

	var ctxLogger = logrus.WithFields(logrus.Fields{
		"type": "getExistingVS",
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupForServiceEntryTests()
			rc := &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: istiofake.NewSimpleClientset(),
				},
			}
			var expectedVS *v1alpha3.VirtualService
			// Create the fake VirtualService obj
			fakeVS := tt.fakeVSFunc()

			// Set up a mock context with a short timeout
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			if tt.existing {
				rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(common.GetAdmiralParams().SyncNamespace).Create(ctx, fakeVS, metav1.CreateOptions{})
				expectedVS = fakeVS
			}

			// Get the existing VirtualService
			existingVS, err := getExistingVS(ctxLogger, ctx, rc, fakeVS.Name, common.GetSyncNamespace())

			// Check the results
			assert.Equal(t, expectedVS, existingVS, "Expected existingVS to match the fakeVS")
			assert.Equal(t, tt.expectedErr, err, "Expected error to match")
		})
	}
}
func TestGetDNSPrefixFromServiceEntry(t *testing.T) {

	testCases := []struct {
		name              string
		seDRTuple         *SeDrTuple
		expectedDNSPrefix string
	}{
		{
			name: "Given empty SeDRTuple " +
				"When getDNSPrefixFromServiceEntry is called " +
				"Then it should return the default DNS prefix",
			seDRTuple:         &SeDrTuple{},
			expectedDNSPrefix: common.Default,
		},
		{
			name: "Given SeDRTuple with default DNS prefix" +
				"When getDNSPrefixFromServiceEntry is called " +
				"Then it should return the default DNS prefix",
			seDRTuple: &SeDrTuple{
				SeDnsPrefix: common.Default,
			},
			expectedDNSPrefix: common.Default,
		},
		{
			name: "Given SeDRTuple with non-default DNS prefix" +
				"When getDNSPrefixFromServiceEntry is called " +
				"Then it should return the DNS prefix set on the SeDrTuple",
			seDRTuple: &SeDrTuple{
				SeDnsPrefix: "test",
			},
			expectedDNSPrefix: "test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actual := getDNSPrefixFromServiceEntry(tc.seDRTuple)
			if actual != tc.expectedDNSPrefix {
				t.Errorf("expected %s got %s", tc.expectedDNSPrefix, actual)
			}

		})
	}
}
func TestReconcileServiceEntry(t *testing.T) {
	var ctxLogger = logrus.WithFields(logrus.Fields{
		"type": "modifySE",
	})
	var seName = "foobar"
	var cluster = "test-cluster"
	alreadyUpdatedSESpecReverseOrder := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:      []string{"host-1"},
		Location:   istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution: istioNetworkingV1Alpha3.ServiceEntry_DNS,
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			&istioNetworkingV1Alpha3.WorkloadEntry{
				Address: "internal-lb-east.com",
				Ports: map[string]uint32{
					"http": 15443,
				},
				Labels: map[string]string{
					"deployment":                "deployment",
					"security.istio.io/tlsMode": "istio",
				},
				Locality: "us-east-2",
			},
			&istioNetworkingV1Alpha3.WorkloadEntry{
				Address: "internal-lb-west.com",
				Ports: map[string]uint32{
					"http": 15443,
				},
				Labels: map[string]string{
					"deployment":                "deployment",
					"security.istio.io/tlsMode": "istio",
				},
				Locality: "us-west-2",
			},
		},
	}
	alreadyUpdatedSESpec := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:      []string{"host-1"},
		Location:   istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution: istioNetworkingV1Alpha3.ServiceEntry_DNS,
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			&istioNetworkingV1Alpha3.WorkloadEntry{
				Address: "internal-lb-west.com",
				Ports: map[string]uint32{
					"http": 15443,
				},
				Labels: map[string]string{
					"deployment":                "deployment",
					"security.istio.io/tlsMode": "istio",
				},
				Locality: "us-west-2",
			},
			&istioNetworkingV1Alpha3.WorkloadEntry{
				Address: "internal-lb-east.com",
				Ports: map[string]uint32{
					"http": 15443,
				},
				Labels: map[string]string{
					"deployment":                "deployment",
					"security.istio.io/tlsMode": "istio",
				},
				Locality: "us-east-2",
			},
		},
	}
	notUpdatedSESpec := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:      []string{"host-1"},
		Location:   istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution: istioNetworkingV1Alpha3.ServiceEntry_DNS,
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			&istioNetworkingV1Alpha3.WorkloadEntry{
				Address: "internal-lb.com",
				Ports: map[string]uint32{
					"http": 15443,
				},
				Labels: map[string]string{
					"deployment": "deployment",
				},
				Locality: "us-west-2",
			},
		},
	}
	notUpdatedSE := &v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name: seName,
		},
		//nolint
		Spec: *notUpdatedSESpec,
	}
	alreadyUpdatedSE := &v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name: seName,
			Annotations: map[string]string{
				"a": "b",
			},
			Labels: map[string]string{
				"a": "b",
			},
		},
		//nolint
		Spec: *alreadyUpdatedSESpec,
	}
	alreadyUpdatedSEEndpointReversed := &v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name: seName,
			Annotations: map[string]string{
				"a": "b",
			},
			Labels: map[string]string{
				"a": "b",
			},
		},
		//nolint
		Spec: *alreadyUpdatedSESpecReverseOrder,
	}
	alreadyUpdatedSEButWithDifferentAnnotations := &v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name: seName,
			Annotations: map[string]string{
				"a": "c",
			},
		},
		//nolint
		Spec: *alreadyUpdatedSESpec,
	}
	alreadyUpdatedSEButWithDifferentLabels := &v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name: seName,
			Labels: map[string]string{
				"a": "c",
			},
		},
		//nolint
		Spec: *alreadyUpdatedSESpec,
	}
	rcWithSE := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			Cache: istio.NewServiceEntryCache(),
		},
	}
	rcWithSE.ServiceEntryController.Cache.Put(alreadyUpdatedSE, cluster)
	rcWithoutSE := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			Cache: istio.NewServiceEntryCache(),
		},
	}
	testCases := []struct {
		name                    string
		enableSECache           bool
		remoteController        *RemoteController
		desiredServiceEntry     *networking.ServiceEntry
		seName                  string
		cluster                 string
		annotationsKeyToCompare []string
		labelKeysToCompare      []string
		expectedResult          bool
	}{
		{
			name: "Given serviceEntry spec to be updated matches the serviceEntry cache," +
				"When reoncileServiceEntry is invoked, " +
				"It should return false",
			enableSECache:       true,
			remoteController:    rcWithSE,
			desiredServiceEntry: alreadyUpdatedSE,
			seName:              seName,
			expectedResult:      false,
		},
		{
			name: "Given serviceEntry spec to be updated matches the serviceEntry cache," +
				"And the annotations do not match, " +
				"When reoncileServiceEntry is invoked, " +
				"It should return true",
			enableSECache:           true,
			remoteController:        rcWithSE,
			desiredServiceEntry:     alreadyUpdatedSEButWithDifferentAnnotations,
			seName:                  seName,
			annotationsKeyToCompare: []string{"a"},
			expectedResult:          true,
		},
		{
			name: "Given serviceEntry spec to be updated matches the serviceEntry cache," +
				"And the labels do not match, " +
				"When reoncileServiceEntry is invoked, " +
				"It should return true",
			enableSECache:       true,
			remoteController:    rcWithSE,
			desiredServiceEntry: alreadyUpdatedSEButWithDifferentLabels,
			seName:              seName,
			labelKeysToCompare:  []string{"a"},
			expectedResult:      true,
		},
		{
			name: "Given serviceEntry spec to be updated does not match the serviceEntry cache," +
				"When reoncileServiceEntry is invoked, " +
				"It should return false",
			enableSECache:       true,
			remoteController:    rcWithoutSE,
			desiredServiceEntry: notUpdatedSE,
			seName:              seName,
			expectedResult:      true,
		},
		{
			name: "Given serviceEntry spec to be updated does not match the serviceEntry cache," +
				"When reoncileServiceEntry is invoked, " +
				"It should return false",
			enableSECache:       true,
			remoteController:    rcWithoutSE,
			desiredServiceEntry: notUpdatedSE,
			seName:              seName,
			expectedResult:      true,
		},
		{
			name: "Given reconcile se cache is disabled," +
				"When reoncileServiceEntry is invoked, " +
				"It should return true",
			enableSECache:       false,
			remoteController:    rcWithoutSE,
			desiredServiceEntry: notUpdatedSE,
			seName:              seName,
			expectedResult:      true,
		},
		{
			name: "Given serviceEntry spec to be updated " +
				"And the endpoints are in reverse order to the one that is in tha cache" +
				"When reoncileServiceEntry is invoked, " +
				"It should return false",
			enableSECache:       true,
			remoteController:    rcWithSE,
			desiredServiceEntry: alreadyUpdatedSEEndpointReversed,
			seName:              seName,
			expectedResult:      false,
		},
	}

	for _, c := range testCases {
		reconciliationRequired := reconcileServiceEntry(
			ctxLogger,
			c.enableSECache,
			c.remoteController,
			c.desiredServiceEntry,
			c.seName,
			cluster,
			c.annotationsKeyToCompare,
			c.labelKeysToCompare,
		)
		if reconciliationRequired != c.expectedResult {
			t.Errorf("expected: %v, got: %v", c.expectedResult, reconciliationRequired)
		}
	}
}

func buildServiceForDeployment(name string, namespace string, identity string) *coreV1.Service {
	service := &coreV1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.NewTime(time.Now().AddDate(-1, 1, 1)),
		},
		Spec: coreV1.ServiceSpec{
			Selector: map[string]string{"app": identity},
			Ports: []coreV1.ServicePort{
				{
					Name: "http",
					Port: 8090,
				},
			},
		},
	}
	return service
}

func buildServiceForRollout(name string, namespace string, identity string) *coreV1.Service {
	service := &coreV1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name + "-stable",
			Namespace:         namespace,
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: coreV1.ServiceSpec{
			Selector: map[string]string{"app": identity},
			Ports: []coreV1.ServicePort{
				{
					Name: "http",
					Port: 8090,
				},
			},
		},
	}
	return service
}

func TestSECreationDisableIP(t *testing.T) {
	admiralParams := admiralParamsForServiceEntryTests()
	admiralParams.DisableIPGeneration = true
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	var (
		env                      = "test"
		stop                     = make(chan struct{})
		clusterID                = "test-se-k8s"
		fakeIstioClient          = istiofake.NewSimpleClientset()
		config                   = rest.Config{Host: "localhost"}
		resyncPeriod             = time.Millisecond * 1000
		serviceEntryAddressStore = &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{
				"test.rseinconfigmap.mesh-se":         "239.0.0.1",
				"test.dseinconfigmap.mesh-se":         "239.0.0.2",
				"test.foo.mesh-se":                    "239.0.0.3",
				"west.test.foo.mesh-se":               "239.0.0.4",
				"east.test.foo.mesh-se":               "239.0.0.5",
				"test.rgtpseinconfigmap.mesh-se":      "239.0.0.6",
				"west.test.rgtpseinconfigmap.mesh-se": "239.0.0.7",
				"east.test.rgtpseinconfigmap.mesh-se": "239.0.0.8",
			},
			Addresses: []string{"239.0.0.1", "239.0.0.2", "239.0.0.3", "239.0.0.4", "239.0.0.5", "239.0.0.6", "239.0.0.7", "239.0.0.8"},
		}
		rolloutSEInConfigmap          = makeTestRollout("rseinconfigmapname", "rseinconfigmap-ns", "rseinconfigmap")
		deploymentSEInConfigmap       = makeTestDeployment("dseinconfigmapname", "dseinconfigmap-ns", "dseinconfigmap")
		rolloutSENotInConfigmap       = makeTestRollout("rsenotinconfigmapname", "rsenotinconfigmap-ns", "rsenotinconfigmap")
		deploymentSENotInConfigmap    = makeTestDeployment("dsenotinconfigmapname", "dsenotinconfigmap-ns", "dsenotinconfigmap")
		deploymentGTPSEInConfigmap    = makeTestDeployment("foo", "foo-ns", "foo")
		deploymentGTPSENotInConfigmap = makeTestDeployment("bar", "bar-ns", "bar")
		rolloutGTPSEInConfigmap       = makeTestRollout("rgtpseinconfigmapname", "rgtpseinconfigmap-ns", "rgtpseinconfigmap")
		rolloutGTPSENotInConfigmap    = makeTestRollout("rgtpsenotinconfigmapname", "rgtpsenotinconfigmap-ns", "rgtpsenotinconfigmap")
		seRolloutSEInConfigmap        = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.rseinconfigmap.mesh"},
			Addresses: []string{"239.0.0.1"},
		}
		seDeploymentSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.dseinconfigmap.mesh"},
			Addresses: []string{"239.0.0.2"},
		}
		seRolloutSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"test.rsenotinconfigmap.mesh"},
		}
		seDeploymentSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"test.dsenotinconfigmap.mesh"},
		}
		seGTPDeploymentSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.foo.mesh"},
			Addresses: []string{"239.0.0.3"},
		}
		seGTPWestSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"west.test.foo.mesh"},
			Addresses: []string{"239.0.0.4"},
		}
		seGTPEastSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"east.test.foo.mesh"},
			Addresses: []string{"239.0.0.5"},
		}
		seGTPDeploymentSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"test.bar.mesh"},
		}
		seGTPWestSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"west.test.bar.mesh"},
		}
		seGTPEastSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"east.test.bar.mesh"},
		}
		seGTPRolloutSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.rgtpseinconfigmap.mesh"},
			Addresses: []string{"239.0.0.6"},
		}
		seGTPRolloutWestSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"west.test.rgtpseinconfigmap.mesh"},
			Addresses: []string{"239.0.0.7"},
		}
		seGTPRolloutEastSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"east.test.rgtpseinconfigmap.mesh"},
			Addresses: []string{"239.0.0.8"},
		}
		seGTPRolloutSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"test.rgtpsenotinconfigmap.mesh"},
		}
		seGTPRolloutWestSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"west.test.rgtpsenotinconfigmap.mesh"},
		}
		seGTPRolloutEastSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"east.test.rgtpsenotinconfigmap.mesh"},
		}
		serviceRolloutSEInConfigmap          = buildServiceForRollout("rseinconfigmapname", "rseinconfigmap-ns", "rseinconfigmap")
		serviceDeploymentSEInConfigmap       = buildServiceForDeployment("dseinconfigmapname", "dseinconfigmap-ns", "dseinconfigmap")
		serviceRolloutSENotInConfigmap       = buildServiceForRollout("rsenotinconfigmapname", "rsenotinconfigmap-ns", "rsenotinconfigmap")
		serviceDeploymentSENotInConfigmap    = buildServiceForDeployment("dsenotinconfigmapname", "dsenotinconfigmap-ns", "dsenotinconfigmap")
		serviceGTPDeploymentSEInConfigmap    = buildServiceForDeployment("foo", "foo-ns", "foo")
		serviceGTPDeploymentSENotInConfigmap = buildServiceForDeployment("bar", "bar-ns", "bar")
		serviceGTPRolloutSEInConfigmap       = buildServiceForRollout("rgtpseinconfigmapname", "rgtpseinconfigmap-ns", "rgtpseinconfigmap")
		serviceGTPRolloutSENotInConfigmap    = buildServiceForRollout("rgtpsenotinconfigmapname", "rgtpsenotinconfigmap-ns", "rgtpsenotinconfigmap")
	)

	deploymentController, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}

	rolloutController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}

	serviceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}

	virtualServiceController, err := istio.NewVirtualServiceController(make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}

	gtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}

	cacheController := &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(serviceEntryAddressStore, "123"),
	}
	rr1 := NewRemoteRegistry(nil, admiralParams)
	rr1.AdmiralCache.ConfigMapController = cacheController
	rr1.AdmiralCache.SeClusterCache = common.NewMapOfMaps()
	cnameIdentityCache := sync.Map{}
	cnameIdentityCache.Store("test.foo.mesh", "foo")
	rr1.AdmiralCache.CnameIdentityCache = &cnameIdentityCache
	dnsPrefixedGTP := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dns-prefixed-gtp",
			Annotations: map[string]string{"env": "test"},
			Labels:      map[string]string{"identity": "foo"},
			Namespace:   "foo-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    0,
					DnsPrefix: "default",
				},
				{
					LbType:    1,
					DnsPrefix: "west",
				},
				{
					LbType:    1,
					DnsPrefix: "east",
				},
			},
		},
	}
	dnsPrefixedGTPSENotInConfigmap := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dns-prefixed-gtp-senotinconfigmap",
			Annotations: map[string]string{"env": "test"},
			Labels:      map[string]string{"identity": "bar"},
			Namespace:   "bar-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    0,
					DnsPrefix: "default",
				},
				{
					LbType:    1,
					DnsPrefix: "west",
				},
				{
					LbType:    1,
					DnsPrefix: "east",
				},
			},
		},
	}
	dnsPrefixedGTPRollout := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dns-prefixed-rollout-gtp",
			Annotations: map[string]string{"env": "test"},
			Labels:      map[string]string{"identity": "rgtpseinconfigmap"},
			Namespace:   "rgtpseinconfigmap-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    0,
					DnsPrefix: "default",
				},
				{
					LbType:    1,
					DnsPrefix: "west",
				},
				{
					LbType:    1,
					DnsPrefix: "east",
				},
			},
		},
	}
	dnsPrefixedGTPSENotInConfigmapRollout := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dns-prefixed-rollout-gtp-senotinconfigmap",
			Annotations: map[string]string{"env": "test"},
			Labels:      map[string]string{"identity": "rgtpsenotinconfigmap"},
			Namespace:   "rgtpsenotinconfigmap-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    0,
					DnsPrefix: "default",
				},
				{
					LbType:    1,
					DnsPrefix: "west",
				},
				{
					LbType:    1,
					DnsPrefix: "east",
				},
			},
		},
	}
	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v13.GlobalTrafficPolicy)
	gtpCache.identityCache["foo"] = dnsPrefixedGTP
	gtpCache.identityCache["bar"] = dnsPrefixedGTPSENotInConfigmap
	gtpCache.identityCache["rgtpseinconfigmap"] = dnsPrefixedGTPRollout
	gtpCache.identityCache["rgtpsenotinconfigmap"] = dnsPrefixedGTPSENotInConfigmapRollout
	gtpCache.mutex = &sync.Mutex{}
	rr1.AdmiralCache.GlobalTrafficCache = gtpCache
	odCache := &outlierDetectionCache{}
	odCache.identityCache = make(map[string]*v13.OutlierDetection)
	odCache.mutex = &sync.Mutex{}
	rr1.AdmiralCache.OutlierDetectionCache = odCache

	deploymentController.Cache.UpdateDeploymentToClusterCache("dseinconfigmap", deploymentSEInConfigmap)
	deploymentController.Cache.UpdateDeploymentToClusterCache("dsenotinconfigmap", deploymentSENotInConfigmap)
	deploymentController.Cache.UpdateDeploymentToClusterCache("foo", deploymentGTPSEInConfigmap)
	deploymentController.Cache.UpdateDeploymentToClusterCache("bar", deploymentGTPSENotInConfigmap)
	rolloutController.Cache.UpdateRolloutToClusterCache("rseinconfigmap", &rolloutSEInConfigmap)
	rolloutController.Cache.UpdateRolloutToClusterCache("rsenotinconfigmap", &rolloutSENotInConfigmap)
	rolloutController.Cache.UpdateRolloutToClusterCache("rgtpseinconfigmap", &rolloutGTPSEInConfigmap)
	rolloutController.Cache.UpdateRolloutToClusterCache("rgtpsenotinconfigmap", &rolloutGTPSENotInConfigmap)
	serviceController.Cache.Put(serviceDeploymentSEInConfigmap)
	serviceController.Cache.Put(serviceRolloutSEInConfigmap)
	serviceController.Cache.Put(serviceDeploymentSENotInConfigmap)
	serviceController.Cache.Put(serviceRolloutSENotInConfigmap)
	serviceController.Cache.Put(serviceGTPDeploymentSEInConfigmap)
	serviceController.Cache.Put(serviceGTPDeploymentSENotInConfigmap)
	serviceController.Cache.Put(serviceGTPRolloutSEInConfigmap)
	serviceController.Cache.Put(serviceGTPRolloutSENotInConfigmap)

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
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic: gtpc,
	}

	rc.GlobalTraffic.Cache.Put(dnsPrefixedGTP)
	rc.GlobalTraffic.Cache.Put(dnsPrefixedGTPSENotInConfigmap)
	rc.GlobalTraffic.Cache.Put(dnsPrefixedGTPRollout)
	rc.GlobalTraffic.Cache.Put(dnsPrefixedGTPSENotInConfigmapRollout)
	rr1.PutRemoteController(clusterID, rc)
	rr1.StartTime = time.Now()
	rr1.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	testCases := []struct {
		name                   string
		assetIdentity          string
		expectedServiceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry
		eventResourceType      string
	}{
		{
			name: "Given a SE is getting updated due to a Rollout, " +
				"And configmap contains the corresponding address, " +
				"And disable IP feature is enabled, " +
				"Then the SE Addresses field contains the address from the configmap",
			assetIdentity: "rseinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.rseinconfigmap.mesh": seRolloutSEInConfigmap,
			},
			eventResourceType: common.Rollout,
		},
		{
			name: "Given a SE is getting updated due to a Deployment, " +
				"And configmap contains the corresponding address, " +
				"And disable IP feature is enabled, " +
				"Then the SE Addresses field contains the address from the configmap",
			assetIdentity: "dseinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.dseinconfigmap.mesh": seDeploymentSEInConfigmap,
			},
			eventResourceType: common.Deployment,
		},
		{
			name: "Given a SE is getting updated due to a Rollout, " +
				"And configmap doesn't contain a corresponding address, " +
				"And disable IP feature is enabled, " +
				"Then the SE Addresses field is empty",
			assetIdentity: "rsenotinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.rsenotinconfigmap.mesh": seRolloutSENotInConfigmap,
			},
			eventResourceType: common.Rollout,
		},
		{
			name: "Given a SE is getting updated due to a Deployment, " +
				"And configmap doesn't contain a corresponding address, " +
				"And disable IP feature is enabled, " +
				"Then the SE Addresses field is empty",
			assetIdentity: "dsenotinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.dsenotinconfigmap.mesh": seDeploymentSENotInConfigmap,
			},
			eventResourceType: common.Deployment,
		},
		{
			name: "Given a SE is getting updated due to a GTP applied to a Deployment, " +
				"And configmap contains the corresponding address, " +
				"And disable IP feature is enabled, " +
				"Then the SE Addresses field contains the address from the configmap",
			assetIdentity: "foo",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.foo.mesh":      seGTPDeploymentSEInConfigmap,
				"west.test.foo.mesh": seGTPWestSEInConfigmap,
				"east.test.foo.mesh": seGTPEastSEInConfigmap,
			},
			eventResourceType: common.Deployment,
		},
		{
			name: "Given a SE is getting updated due to a GTP applied to a Deployment, " +
				"And configmap doesn't contain the corresponding address, " +
				"And disable IP feature is enabled, " +
				"Then the SE Addresses field is empty",
			assetIdentity: "bar",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.foo.mesh":      seGTPDeploymentSENotInConfigmap,
				"west.test.foo.mesh": seGTPWestSENotInConfigmap,
				"east.test.foo.mesh": seGTPEastSENotInConfigmap,
			},
			eventResourceType: common.Deployment,
		},
		{
			name: "Given a SE is getting updated due to a GTP applied to a Rollout, " +
				"And configmap contains the corresponding address, " +
				"And disable IP feature is enabled, " +
				"Then the SE Addresses field contains the address from the configmap",
			assetIdentity: "rgtpseinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.rgtpseinconfigmap.mesh":      seGTPRolloutSEInConfigmap,
				"west.test.rgtpseinconfigmap.mesh": seGTPRolloutWestSEInConfigmap,
				"east.test.rgtpseinconfigmap.mesh": seGTPRolloutEastSEInConfigmap,
			},
			eventResourceType: common.Rollout,
		},
		{
			name: "Given a SE is getting updated due to a GTP applied to a Rollout, " +
				"And configmap doesn't contain the corresponding address, " +
				"And disable IP feature is enabled, " +
				"Then the SE Addresses field is empty",
			assetIdentity: "rgtpsenotinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.rgtpsenotinconfigmap.mesh":      seGTPRolloutSENotInConfigmap,
				"west.test.rgtpsenotinconfigmap.mesh": seGTPRolloutWestSENotInConfigmap,
				"east.test.rgtpsenotinconfigmap.mesh": seGTPRolloutEastSENotInConfigmap,
			},
			eventResourceType: common.Rollout,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = context.WithValue(ctx, "clusterName", clusterID)
			ctx = context.WithValue(ctx, "eventResourceType", c.eventResourceType)

			_, err = modifyServiceEntryForNewServiceOrPod(
				ctx,
				admiral.Add,
				env,
				c.assetIdentity,
				rr1,
			)

			for _, expectedServiceEntry := range c.expectedServiceEntries {
				seName := getIstioResourceName(expectedServiceEntry.Hosts[0], "-se")
				createdSe, err := fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Get(ctx, seName, metav1.GetOptions{})
				if err != nil {
					logrus.Info(err)
					t.Error(err)
				}
				if createdSe == nil {
					logrus.Infof("expected the service entry %s but it wasn't found", seName)
					t.Errorf("expected the service entry %s but it wasn't found", seName)
				}
				if !reflect.DeepEqual(createdSe.Spec.Addresses, expectedServiceEntry.Addresses) {
					t.Errorf("expected SE Addresses %v of length %v but got %v of length %v", expectedServiceEntry.Addresses, len(expectedServiceEntry.Addresses), createdSe.Spec.Addresses, len(createdSe.Spec.Addresses))
				}
			}
		})
	}
}

func TestSECreation(t *testing.T) {
	admiralParams := admiralParamsForServiceEntryTests()
	admiralParams.EnableSWAwareNSCaches = true
	admiralParams.ExportToIdentityList = []string{"*"}
	admiralParams.ExportToMaxNamespaces = 35
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	var (
		env                      = "test"
		stop                     = make(chan struct{})
		clusterID                = "test-se-k8s"
		fakeIstioClient          = istiofake.NewSimpleClientset()
		config                   = rest.Config{Host: "localhost"}
		resyncPeriod             = time.Millisecond * 1000
		serviceEntryAddressStore = &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{
				"test.rseinconfigmap.mesh-se":         "239.0.0.1",
				"test.dseinconfigmap.mesh-se":         "239.0.0.2",
				"test.foo.mesh-se":                    "239.0.0.3",
				"west.test.foo.mesh-se":               "239.0.0.4",
				"east.test.foo.mesh-se":               "239.0.0.5",
				"test.rgtpseinconfigmap.mesh-se":      "239.0.0.6",
				"west.test.rgtpseinconfigmap.mesh-se": "239.0.0.7",
				"east.test.rgtpseinconfigmap.mesh-se": "239.0.0.8",
				"test.emptyaddress.mesh-se":           "",
				"test.emptyaddress1.mesh-se":          "",
				"test.emptyaddress2.mesh-se":          "",
			},
			Addresses: []string{"239.0.0.1", "239.0.0.2", "239.0.0.3", "239.0.0.4", "239.0.0.5", "239.0.0.6", "239.0.0.7", "239.0.0.8"},
		}
		rolloutSEInConfigmap              = makeTestRollout("rseinconfigmapname", "rseinconfigmap-ns", "rseinconfigmap")
		deploymentSEInConfigmap           = makeTestDeployment("dseinconfigmapname", "dseinconfigmap-ns", "dseinconfigmap")
		rolloutSENotInConfigmap           = makeTestRollout("rsenotinconfigmapname", "rsenotinconfigmap-ns", "rsenotinconfigmap")
		deploymentSENotInConfigmap        = makeTestDeployment("dsenotinconfigmapname", "dsenotinconfigmap-ns", "dsenotinconfigmap")
		deploymentGTPSEInConfigmap        = makeTestDeployment("foo", "foo-ns", "foo")
		deploymentGTPSENotInConfigmap     = makeTestDeployment("bar", "bar-ns", "bar")
		rolloutGTPSEInConfigmap           = makeTestRollout("rgtpseinconfigmapname", "rgtpseinconfigmap-ns", "rgtpseinconfigmap")
		rolloutGTPSENotInConfigmap        = makeTestRollout("rgtpsenotinconfigmapname", "rgtpsenotinconfigmap-ns", "rgtpsenotinconfigmap")
		deploymentEmptyAddressInConfigmap = makeTestDeployment("emptyaddressname", "emptyaddress-ns", "emptyaddress")
		rolloutEmptyAddressInConfigmap    = makeTestRollout("emptyaddress1name", "emptyaddress1-ns", "emptyaddress1")
		serverDeployment                  = makeTestDeployment("serverdeploymentname", "server-ns", "serveridentity")
		seRolloutSEInConfigmap            = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.rseinconfigmap.mesh"},
			Addresses: []string{"239.0.0.1"},
		}
		seDeploymentSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.dseinconfigmap.mesh"},
			Addresses: []string{"239.0.0.2"},
		}
		seRolloutSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.rsenotinconfigmap.mesh"},
			Addresses: []string{"240.0.10.9"},
		}
		seDeploymentSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.dsenotinconfigmap.mesh"},
			Addresses: []string{"240.0.10.10"},
		}
		seGTPDeploymentSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.foo.mesh"},
			Addresses: []string{"239.0.0.3"},
		}
		seGTPWestSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"west.test.foo.mesh"},
			Addresses: []string{"239.0.0.4"},
		}
		seGTPEastSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"east.test.foo.mesh"},
			Addresses: []string{"239.0.0.5"},
		}
		seGTPDeploymentSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.bar.mesh"},
			Addresses: []string{"240.0.10.11"},
		}
		seGTPWestSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"west.test.bar.mesh"},
			Addresses: []string{"240.0.10.12"},
		}
		seGTPEastSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"east.test.bar.mesh"},
			Addresses: []string{"240.0.10.13"},
		}
		seGTPRolloutSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.rgtpseinconfigmap.mesh"},
			Addresses: []string{"239.0.0.6"},
		}
		seGTPRolloutWestSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"west.test.rgtpseinconfigmap.mesh"},
			Addresses: []string{"239.0.0.7"},
		}
		seGTPRolloutEastSEInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"east.test.rgtpseinconfigmap.mesh"},
			Addresses: []string{"239.0.0.8"},
		}
		seGTPRolloutSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.rgtpsenotinconfigmap.mesh"},
			Addresses: []string{"240.0.10.14"},
		}
		seGTPRolloutWestSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"west.test.rgtpsenotinconfigmap.mesh"},
			Addresses: []string{"240.0.10.15"},
		}
		seGTPRolloutEastSENotInConfigmap = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"east.test.rgtpsenotinconfigmap.mesh"},
			Addresses: []string{"240.0.10.16"},
		}
		seServerDeployment = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:     []string{"test.serveridentity.mesh"},
			Addresses: []string{"240.0.10.17"},
		}
		serviceRolloutSEInConfigmap          = buildServiceForRollout("rseinconfigmapname", "rseinconfigmap-ns", "rseinconfigmap")
		serviceDeploymentSEInConfigmap       = buildServiceForDeployment("dseinconfigmapname", "dseinconfigmap-ns", "dseinconfigmap")
		serviceRolloutSENotInConfigmap       = buildServiceForRollout("rsenotinconfigmapname", "rsenotinconfigmap-ns", "rsenotinconfigmap")
		serviceDeploymentSENotInConfigmap    = buildServiceForDeployment("dsenotinconfigmapname", "dsenotinconfigmap-ns", "dsenotinconfigmap")
		serviceGTPDeploymentSEInConfigmap    = buildServiceForDeployment("foo", "foo-ns", "foo")
		serviceGTPDeploymentSENotInConfigmap = buildServiceForDeployment("bar", "bar-ns", "bar")
		serviceGTPRolloutSEInConfigmap       = buildServiceForRollout("rgtpseinconfigmapname", "rgtpseinconfigmap-ns", "rgtpseinconfigmap")
		serviceGTPRolloutSENotInConfigmap    = buildServiceForRollout("rgtpsenotinconfigmapname", "rgtpsenotinconfigmap-ns", "rgtpsenotinconfigmap")
		serviceEmptyAddressInConfigmap       = buildServiceForDeployment("emptyaddressname", "emptyaddress-ns", "emptyaddress")
		serviceEmptyAddress1InConfigmap      = buildServiceForRollout("emptyaddress1name", "emptyaddress1-ns", "emptyaddress1")
		serviceServerDeployment              = buildServiceForDeployment("serverdeploymentname", "server-ns", "serveridentity")
	)

	deploymentController, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}

	rolloutController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}

	serviceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}

	virtualServiceController, err := istio.NewVirtualServiceController(make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}

	gtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}

	cacheController := &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(serviceEntryAddressStore, "123"),
	}
	rr1 := NewRemoteRegistry(nil, admiralParams)
	rr1.AdmiralCache.ConfigMapController = cacheController
	rr1.AdmiralCache.SeClusterCache = common.NewMapOfMaps()
	rr1.AdmiralCache.IdentityDependencyCache.Put("serveridentity", "clientidentity", "clientidentity")
	rr1.AdmiralCache.IdentityClusterCache.Put("clientidentity", "client-cluster1-k8s", "client-cluster1-k8s")
	rr1.AdmiralCache.IdentityClusterCache.Put("clientidentity", "client-cluster2-k8s", "client-cluster2-k8s")
	rr1.AdmiralCache.IdentityClusterCache.Put("serveridentity", "server-cluster-k8s", "server-cluster-k8s")
	rr1.AdmiralCache.IdentityClusterNamespaceCache.Put("serveridentity", "server-cluster-k8s", "server-ns", "server-ns")
	rr1.AdmiralCache.IdentityClusterNamespaceCache.Put("clientidentity", "client-cluster1-k8s", "client-ns1", "client-ns1")
	rr1.AdmiralCache.IdentityClusterNamespaceCache.Put("clientidentity", "client-cluster2-k8s", "client-ns2", "client-ns2")
	rr1.AdmiralCache.IdentityDependencyCache.Put("dseinconfigmap", "clientidentity3", "clientidentity3")
	rr1.AdmiralCache.IdentityDependencyCache.Put("rseinconfigmap", "clientidentity4", "clientidentity4")
	rr1.AdmiralCache.IdentityClusterCache.Put("clientidentity4", "client-cluster4-k8s", "client-cluster4-k8s")
	expectedCnameDependentClusterNamespaceCache := common.NewMapOfMapOfMaps()
	expectedCnameDependentClusterNamespaceCache.Put("test.serveridentity.mesh", "client-cluster1-k8s", "client-ns1", "client-ns1")
	expectedCnameDependentClusterNamespaceCache.Put("test.serveridentity.mesh", "client-cluster2-k8s", "client-ns2", "client-ns2")
	cnameIdentityCache := sync.Map{}
	cnameIdentityCache.Store("test.foo.mesh", "foo")
	rr1.AdmiralCache.CnameIdentityCache = &cnameIdentityCache
	dnsPrefixedGTP := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dns-prefixed-gtp",
			Annotations: map[string]string{"env": "test"},
			Labels:      map[string]string{"identity": "foo"},
			Namespace:   "foo-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    0,
					DnsPrefix: "default",
				},
				{
					LbType:    1,
					DnsPrefix: "west",
				},
				{
					LbType:    1,
					DnsPrefix: "east",
				},
			},
		},
	}
	dnsPrefixedGTPSENotInConfigmap := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dns-prefixed-gtp-senotinconfigmap",
			Annotations: map[string]string{"env": "test"},
			Labels:      map[string]string{"identity": "bar"},
			Namespace:   "bar-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    0,
					DnsPrefix: "default",
				},
				{
					LbType:    1,
					DnsPrefix: "west",
				},
				{
					LbType:    1,
					DnsPrefix: "east",
				},
			},
		},
	}
	dnsPrefixedGTPRollout := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dns-prefixed-rollout-gtp",
			Annotations: map[string]string{"env": "test"},
			Labels:      map[string]string{"identity": "rgtpseinconfigmap"},
			Namespace:   "rgtpseinconfigmap-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    0,
					DnsPrefix: "default",
				},
				{
					LbType:    1,
					DnsPrefix: "west",
				},
				{
					LbType:    1,
					DnsPrefix: "east",
				},
			},
		},
	}
	dnsPrefixedGTPSENotInConfigmapRollout := &v13.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dns-prefixed-rollout-gtp-senotinconfigmap",
			Annotations: map[string]string{"env": "test"},
			Labels:      map[string]string{"identity": "rgtpsenotinconfigmap"},
			Namespace:   "rgtpsenotinconfigmap-ns",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType:    0,
					DnsPrefix: "default",
				},
				{
					LbType:    1,
					DnsPrefix: "west",
				},
				{
					LbType:    1,
					DnsPrefix: "east",
				},
			},
		},
	}
	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v13.GlobalTrafficPolicy)
	gtpCache.identityCache["foo"] = dnsPrefixedGTP
	gtpCache.identityCache["bar"] = dnsPrefixedGTPSENotInConfigmap
	gtpCache.identityCache["rgtpseinconfigmap"] = dnsPrefixedGTPRollout
	gtpCache.identityCache["rgtpsenotinconfigmap"] = dnsPrefixedGTPSENotInConfigmapRollout
	gtpCache.mutex = &sync.Mutex{}
	rr1.AdmiralCache.GlobalTrafficCache = gtpCache
	odCache := &outlierDetectionCache{}
	odCache.identityCache = make(map[string]*v13.OutlierDetection)
	odCache.mutex = &sync.Mutex{}
	rr1.AdmiralCache.OutlierDetectionCache = odCache

	deploymentController.Cache.UpdateDeploymentToClusterCache("dseinconfigmap", deploymentSEInConfigmap)
	deploymentController.Cache.UpdateDeploymentToClusterCache("dsenotinconfigmap", deploymentSENotInConfigmap)
	deploymentController.Cache.UpdateDeploymentToClusterCache("foo", deploymentGTPSEInConfigmap)
	deploymentController.Cache.UpdateDeploymentToClusterCache("bar", deploymentGTPSENotInConfigmap)
	deploymentController.Cache.UpdateDeploymentToClusterCache("emptyaddress", deploymentEmptyAddressInConfigmap)
	deploymentController.Cache.UpdateDeploymentToClusterCache("serveridentity", serverDeployment)
	rolloutController.Cache.UpdateRolloutToClusterCache("rseinconfigmap", &rolloutSEInConfigmap)
	rolloutController.Cache.UpdateRolloutToClusterCache("rsenotinconfigmap", &rolloutSENotInConfigmap)
	rolloutController.Cache.UpdateRolloutToClusterCache("rgtpseinconfigmap", &rolloutGTPSEInConfigmap)
	rolloutController.Cache.UpdateRolloutToClusterCache("rgtpsenotinconfigmap", &rolloutGTPSENotInConfigmap)
	rolloutController.Cache.UpdateRolloutToClusterCache("emptyaddress1", &rolloutEmptyAddressInConfigmap)
	serviceController.Cache.Put(serviceDeploymentSEInConfigmap)
	serviceController.Cache.Put(serviceRolloutSEInConfigmap)
	serviceController.Cache.Put(serviceDeploymentSENotInConfigmap)
	serviceController.Cache.Put(serviceRolloutSENotInConfigmap)
	serviceController.Cache.Put(serviceGTPDeploymentSEInConfigmap)
	serviceController.Cache.Put(serviceGTPDeploymentSENotInConfigmap)
	serviceController.Cache.Put(serviceGTPRolloutSEInConfigmap)
	serviceController.Cache.Put(serviceGTPRolloutSENotInConfigmap)
	serviceController.Cache.Put(serviceEmptyAddressInConfigmap)
	serviceController.Cache.Put(serviceEmptyAddress1InConfigmap)
	serviceController.Cache.Put(serviceServerDeployment)

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
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic: gtpc,
	}

	rc.GlobalTraffic.Cache.Put(dnsPrefixedGTP)
	rc.GlobalTraffic.Cache.Put(dnsPrefixedGTPSENotInConfigmap)
	rc.GlobalTraffic.Cache.Put(dnsPrefixedGTPRollout)
	rc.GlobalTraffic.Cache.Put(dnsPrefixedGTPSENotInConfigmapRollout)
	rr1.PutRemoteController(clusterID, rc)
	rr1.StartTime = time.Now()
	rr1.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	testCases := []struct {
		name                   string
		assetIdentity          string
		expectedServiceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry
		eventResourceType      string
		expectedCnameDCNSCache *common.MapOfMapOfMaps
	}{
		{
			name: "Given a SE is getting updated due to a Rollout, " +
				"And configmap contains the corresponding address, " +
				"And disable IP feature is disabled, " +
				"Then the SE Addresses field contains the address from the configmap",
			assetIdentity: "rseinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.rseinconfigmap.mesh": seRolloutSEInConfigmap,
			},
			eventResourceType:      common.Rollout,
			expectedCnameDCNSCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given a SE is getting updated due to a Deployment, " +
				"And configmap contains the corresponding address, " +
				"And disable IP feature is disabled, " +
				"Then the SE Addresses field contains the address from the configmap",
			assetIdentity: "dseinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.dseinconfigmap.mesh": seDeploymentSEInConfigmap,
			},
			eventResourceType:      common.Deployment,
			expectedCnameDCNSCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given a SE is getting updated due to a Rollout, " +
				"And configmap doesn't contain a corresponding address, " +
				"And disable IP feature is disabled, " +
				"Then the SE Addresses field has a newly created address",
			assetIdentity: "rsenotinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.rsenotinconfigmap.mesh": seRolloutSENotInConfigmap,
			},
			eventResourceType:      common.Rollout,
			expectedCnameDCNSCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given a SE is getting updated due to a Deployment, " +
				"And configmap doesn't contain a corresponding address, " +
				"And disable IP feature is disabled, " +
				"Then the SE Addresses field has a newly created address",
			assetIdentity: "dsenotinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.dsenotinconfigmap.mesh": seDeploymentSENotInConfigmap,
			},
			eventResourceType:      common.Deployment,
			expectedCnameDCNSCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given a SE is getting updated due to a GTP applied to a Deployment, " +
				"And configmap contains the corresponding address, " +
				"And disable IP feature is disabled, " +
				"Then the SE Addresses field contains the address from the configmap",
			assetIdentity: "foo",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.foo.mesh":      seGTPDeploymentSEInConfigmap,
				"west.test.foo.mesh": seGTPWestSEInConfigmap,
				"east.test.foo.mesh": seGTPEastSEInConfigmap,
			},
			eventResourceType:      common.Deployment,
			expectedCnameDCNSCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given a SE is getting updated due to a GTP applied to a Deployment, " +
				"And configmap doesn't contain the corresponding address, " +
				"And disable IP feature is disabled, " +
				"Then the SE Addresses field has a newly created address",
			assetIdentity: "bar",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.foo.mesh":      seGTPDeploymentSENotInConfigmap,
				"west.test.foo.mesh": seGTPWestSENotInConfigmap,
				"east.test.foo.mesh": seGTPEastSENotInConfigmap,
			},
			eventResourceType:      common.Deployment,
			expectedCnameDCNSCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given a SE is getting updated due to a GTP applied to a Rollout, " +
				"And configmap contains the corresponding address, " +
				"And disable IP feature is disabled, " +
				"Then the SE Addresses field contains the address from the configmap",
			assetIdentity: "rgtpseinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.rgtpseinconfigmap.mesh":      seGTPRolloutSEInConfigmap,
				"west.test.rgtpseinconfigmap.mesh": seGTPRolloutWestSEInConfigmap,
				"east.test.rgtpseinconfigmap.mesh": seGTPRolloutEastSEInConfigmap,
			},
			eventResourceType:      common.Rollout,
			expectedCnameDCNSCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given a SE is getting updated due to a GTP applied to a Rollout, " +
				"And configmap doesn't contain the corresponding address, " +
				"And disable IP feature is disabled, " +
				"Then the SE Addresses field has a newly created address",
			assetIdentity: "rgtpsenotinconfigmap",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.rgtpsenotinconfigmap.mesh":      seGTPRolloutSENotInConfigmap,
				"west.test.rgtpsenotinconfigmap.mesh": seGTPRolloutWestSENotInConfigmap,
				"east.test.rgtpsenotinconfigmap.mesh": seGTPRolloutEastSENotInConfigmap,
			},
			eventResourceType:      common.Rollout,
			expectedCnameDCNSCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given a SE is getting updated due to a Deployment, " +
				"And configmap contains empty address for that se, " +
				"And disable IP feature is disabled, " +
				"Then the SE should be nil",
			assetIdentity: "emptyaddress",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.emptyaddress.mesh": nil,
			},
			eventResourceType:      common.Deployment,
			expectedCnameDCNSCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given a SE is getting updated due to a Rollout, " +
				"And configmap contains empty address for that se, " +
				"And disable IP feature is disabled, " +
				"Then the SE should be nil",
			assetIdentity: "emptyaddress1",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.emptyaddress1.mesh": nil,
			},
			eventResourceType:      common.Rollout,
			expectedCnameDCNSCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given a SE is getting updated due to a Deployment, " +
				"And IdentityClusterNamespaceCache has entries for that identity, " +
				"And enable SW awareness feature is enabled, " +
				"Then the CnameDependentClusterNamespaceCache should be filled",
			assetIdentity: "serveridentity",
			expectedServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.serveridentity.mesh": seServerDeployment,
			},
			eventResourceType:      common.Deployment,
			expectedCnameDCNSCache: expectedCnameDependentClusterNamespaceCache,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = context.WithValue(ctx, "clusterName", clusterID)
			ctx = context.WithValue(ctx, "eventResourceType", c.eventResourceType)

			_, err = modifyServiceEntryForNewServiceOrPod(
				ctx,
				admiral.Add,
				env,
				c.assetIdentity,
				rr1,
			)

			for k, expectedServiceEntry := range c.expectedServiceEntries {
				if expectedServiceEntry == nil {
					fakeSeName := getIstioResourceName(k, "-se")
					_, err := fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Get(ctx, fakeSeName, metav1.GetOptions{})
					if err == nil {
						t.Errorf("Expected to have %v not found but there was no error", fakeSeName)
					}
					continue
				}
				seName := getIstioResourceName(expectedServiceEntry.Hosts[0], "-se")
				createdSe, err := fakeIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Get(ctx, seName, metav1.GetOptions{})
				if err != nil {
					logrus.Info(err)
					t.Error(err)
				}
				if createdSe == nil {
					logrus.Infof("expected the service entry %s but it wasn't found", seName)
					t.Errorf("expected the service entry %s but it wasn't found", seName)
				}
				if !reflect.DeepEqual(createdSe.Spec.Addresses, expectedServiceEntry.Addresses) {
					t.Errorf("expected SE Addresses %v of length %v but got %v of length %v", expectedServiceEntry.Addresses, len(expectedServiceEntry.Addresses), createdSe.Spec.Addresses, len(createdSe.Spec.Addresses))
				}
			}
			if c.expectedCnameDCNSCache.Len() > 0 {
				if !reflect.DeepEqual(c.expectedCnameDCNSCache, rr1.AdmiralCache.CnameDependentClusterNamespaceCache) {
					t.Error("expected CnameDependentClusterNamespaceCache did not match constructed CnameDependentClusterNamespaceCache")
				}
			}
		})
	}
}

func TestReconcileDestinationRule(t *testing.T) {
	var (
		ctxLogger = logrus.WithFields(logrus.Fields{
			"op": "ConfigWriter",
		})
		drName  = "foobar"
		cluster = "test-cluster"
	)

	ap := common.AdmiralParams{
		SyncNamespace: "admiral-sync",
	}

	common.ResetSync()
	common.InitializeConfig(ap)

	alreadyUpdatedDRSpec := &istioNetworkingV1Alpha3.DestinationRule{
		Host: "host-1",
		TrafficPolicy: &istioNetworkingV1Alpha3.TrafficPolicy{
			Tls: &istioNetworkingV1Alpha3.ClientTLSSettings{
				Mode: istioNetworkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
			},
		},
	}
	notUpdatedDRSpec := &istioNetworkingV1Alpha3.DestinationRule{
		Host: "host-1",
		TrafficPolicy: &istioNetworkingV1Alpha3.TrafficPolicy{
			Tls: &istioNetworkingV1Alpha3.ClientTLSSettings{
				Mode: istioNetworkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
			},
		},
	}
	alreadyUpdatedDR := &v1alpha3.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      drName,
			Namespace: "admiral-sync",
		},
		//nolint
		Spec: *alreadyUpdatedDRSpec,
	}
	rcWithDR := &RemoteController{
		DestinationRuleController: &istio.DestinationRuleController{
			Cache: istio.NewDestinationRuleCache(),
		},
	}
	rcWithDR.DestinationRuleController.Cache.Put(alreadyUpdatedDR)
	rcWithoutDR := &RemoteController{
		DestinationRuleController: &istio.DestinationRuleController{
			Cache: istio.NewDestinationRuleCache(),
		},
	}
	testCases := []struct {
		name             string
		enableDRCache    bool
		remoteController *RemoteController
		destinationRule  *istioNetworkingV1Alpha3.DestinationRule
		drName           string
		cluster          string
		expectedResult   bool
	}{
		{
			name: "Given destinationRule spec to be updated does not match the destinationRule cache," +
				"When reconcileDestinationRule is invoked, " +
				"It should return false",
			enableDRCache:    true,
			remoteController: rcWithoutDR,
			destinationRule:  notUpdatedDRSpec,
			drName:           drName,
			expectedResult:   true,
		},
		{
			name: "Given destinationRule spec to be updated does not match the destinationRule cache," +
				"When reconcileDestinationRule is invoked, " +
				"It should return false",
			enableDRCache:    true,
			remoteController: rcWithoutDR,
			destinationRule:  notUpdatedDRSpec,
			drName:           drName,
			expectedResult:   true,
		},
		{
			name: "Given dr cache is disabled," +
				"When reconcileDestinationRule is invoked, " +
				"It should return true",
			enableDRCache:    false,
			remoteController: rcWithoutDR,
			destinationRule:  notUpdatedDRSpec,
			drName:           drName,
			expectedResult:   true,
		},
	}

	for _, c := range testCases {
		reconciliationRequired := reconcileDestinationRule(
			ctxLogger, c.enableDRCache, c.remoteController, c.destinationRule, c.drName, cluster, admiralParams.SyncNamespace)
		if reconciliationRequired != c.expectedResult {
			t.Errorf("expected: %v, got: %v", c.expectedResult, reconciliationRequired)
		}
	}
}

func TestPopulateClientConnectionConfigCache(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.ResetSync()
	common.InitializeConfig(p)

	stop := make(chan struct{})
	clientConnectionSettingsController, _ := admiral.NewClientConnectionConfigController(
		stop, &ClientConnectionConfigHandler{}, &rest.Config{}, 0, loader.GetFakeClientLoader())

	clientConnectionSettingsController.Cache.Put(&admiralV1.ClientConnectionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ccsName",
			Namespace: "testns",
			Labels: map[string]string{
				"admiral.io/env": "testEnv",
				"identity":       "testId",
			},
		},
	})

	testCases := []struct {
		name                       string
		rc                         *RemoteController
		identity                   string
		namespace                  string
		clientConnectionSettingMap map[string][]*admiralV1.ClientConnectionConfig
		expectedError              error
	}{
		{
			name: "Given valid params to populateClientConnectionConfigCache func " +
				"When ClientConnectionConfigController is nil in the remoteController " +
				"Then the func should return an error",
			rc:            &RemoteController{},
			identity:      "testID",
			namespace:     "testNS",
			expectedError: fmt.Errorf("clientConnectionSettings controller is not initialized"),
		},
		{
			name: "Given valid params to populateClientConnectionConfigCache func " +
				"When ClientConnectionConfigController cache is nil in the remoteController " +
				"Then the func should return an error",
			rc: &RemoteController{
				ClientConnectionConfigController: &admiral.ClientConnectionConfigController{},
			},
			identity:      "testID",
			namespace:     "testNS",
			expectedError: fmt.Errorf("clientConnectionSettings controller is not initialized"),
		},
		{
			name: "Given valid params to populateClientConnectionConfigCache func " +
				"When there is no cache entry in the controller cache for the matching identity and namespace " +
				"Then the func should return an error",
			rc: &RemoteController{
				ClientConnectionConfigController: clientConnectionSettingsController,
			},
			identity:      "testID",
			namespace:     "testNS",
			expectedError: fmt.Errorf("clientConnectionSettings not found in controller cache"),
		},
		{
			name: "Given valid params to populateClientConnectionConfigCache func " +
				"When there is a cache entry in the controller cache for the matching identity and namespace " +
				"Then the entry should be added to the clientConnectionSettings cache",
			rc: &RemoteController{
				ClientConnectionConfigController: clientConnectionSettingsController,
				ClusterID:                        "testCluster",
			},
			identity:                   "testEnv.testId",
			namespace:                  "testns",
			clientConnectionSettingMap: make(map[string][]*admiralV1.ClientConnectionConfig),
			expectedError:              nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actualError := populateClientConnectionConfigCache(tc.rc, tc.identity, tc.namespace, tc.clientConnectionSettingMap)

			if tc.expectedError != nil {
				if actualError == nil {
					t.Fatalf("expected error %s got nil", tc.expectedError.Error())
				}
				assert.Equal(t, tc.expectedError.Error(), actualError.Error())
			} else {
				if actualError != nil {
					t.Fatalf("expected nil but got error %s", actualError.Error())
				}
				assert.NotNil(t, tc.clientConnectionSettingMap[tc.rc.ClusterID])
			}

		})
	}

}

// write test for updateGlobalClientConnectionConfigCache
func TestUpdateGlobalClientConnectionConfigCache(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
		EnableClientConnectionConfigProcessing: true,
	}
	common.ResetSync()
	common.InitializeConfig(p)

	creationTimeFirst := metav1.Now()
	creationTimeSecond := metav1.Now()

	testCases := []struct {
		name                           string
		admiralCache                   *AdmiralCache
		identity                       string
		env                            string
		clientConnectionSettings       map[string][]*admiralV1.ClientConnectionConfig
		expectedClientConnectionConfig *admiralV1.ClientConnectionConfig
		expectedError                  error
	}{
		{
			name: "Given valid params to updateGlobalClientConnectionConfigCache func " +
				"When clientConnectionSettings map is empty " +
				"Then the func should delete the entry from the global cache and not return any error",
			admiralCache: &AdmiralCache{
				ClientConnectionConfigCache: &clientConnectionSettingsCache{
					identityCache: map[string]*admiralV1.ClientConnectionConfig{
						"testEnv.testId": {
							ObjectMeta: metav1.ObjectMeta{
								Name:      "ccsName",
								Namespace: "testns",
								Labels: map[string]string{
									"admiral.io/env": "testEnv",
									"identity":       "testId",
								},
							},
						},
					},
					mutex: &sync.RWMutex{},
				},
			},
			identity:                       "testId",
			env:                            "testEnv",
			clientConnectionSettings:       map[string][]*admiralV1.ClientConnectionConfig{},
			expectedClientConnectionConfig: nil,
			expectedError:                  nil,
		},
		{
			name: "Given valid params to updateGlobalClientConnectionConfigCache func " +
				"When clientConnectionSettings map has two entries " +
				"Then the func should put the latest clientConnectionSettings in the cache",
			admiralCache: &AdmiralCache{
				ClientConnectionConfigCache: NewClientConnectionConfigCache(),
			},
			identity: "testId",
			env:      "testEnv",
			clientConnectionSettings: map[string][]*admiralV1.ClientConnectionConfig{
				"testEnv.testId": {
					&admiralV1.ClientConnectionConfig{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: creationTimeFirst,
							Name:              "ccsName",
							Namespace:         "testns0",
							Labels: map[string]string{
								"admiral.io/env": "testEnv",
								"identity":       "testId",
							},
						},
					},
					&admiralV1.ClientConnectionConfig{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: creationTimeSecond,
							Name:              "ccsName",
							Namespace:         "testns1",
							Labels: map[string]string{
								"admiral.io/env": "testEnv",
								"identity":       "testId",
							},
						},
					},
				},
			},
			expectedClientConnectionConfig: &admiralV1.ClientConnectionConfig{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: creationTimeSecond,
					Name:              "ccsName",
					Namespace:         "testns1",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "Given valid params to updateGlobalClientConnectionConfigCache func " +
				"When clientConnectionSettings map has two entries " +
				"Then the func should put the latest clientConnectionSettings in the cache",
			admiralCache: &AdmiralCache{
				ClientConnectionConfigCache: &MockClientConnectionConfigCache{},
			},
			identity: "testId",
			env:      "testEnv",
			clientConnectionSettings: map[string][]*admiralV1.ClientConnectionConfig{
				"testEnv.testId": {
					&admiralV1.ClientConnectionConfig{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: creationTimeFirst,
							Name:              "ccsName",
							Namespace:         "testns0",
							Labels: map[string]string{
								"admiral.io/env": "testEnv",
								"identity":       "testId",
							},
						},
					},
					&admiralV1.ClientConnectionConfig{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: creationTimeSecond,
							Name:              "ccsName",
							Namespace:         "testns1",
							Labels: map[string]string{
								"admiral.io/env": "testEnv",
								"identity":       "testId",
							},
						},
					},
				},
			},
			expectedClientConnectionConfig: nil,
			expectedError: fmt.Errorf(
				"error in updating ClientConnectionConfig global cache with name=ccsName in namespace=testns1 " +
					"as actively used for identity=testId with err=error adding to cache"),
		},
	}

	var ctxLogger = logrus.WithFields(logrus.Fields{
		"type": "modifySE",
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actualError := updateGlobalClientConnectionConfigCache(
				ctxLogger, tc.admiralCache, tc.identity, tc.env, tc.clientConnectionSettings)

			if tc.expectedError != nil {
				if actualError == nil {
					t.Fatalf("expected error %s got nil", tc.expectedError.Error())
				}
				assert.Equal(t, tc.expectedError.Error(), actualError.Error())
			} else {
				if actualError != nil {
					t.Fatalf("expected nil but got error %s", actualError.Error())
				}
				actualCacheEntry, _ := tc.admiralCache.ClientConnectionConfigCache.GetFromIdentity(tc.identity, tc.env)
				assert.Equal(t, tc.expectedClientConnectionConfig, actualCacheEntry)
			}

		})
	}
}

func TestAddServiceEntriesWithDrWorker(t *testing.T) {
	var namespace = "namespace-1"
	var ctxLogger = logrus.WithFields(logrus.Fields{
		"type": "modifySE",
	})
	seNotInCacheSpec := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{"test.mesh"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			&istioNetworkingV1Alpha3.WorkloadEntry{
				Address:  "aws-lb.1.com",
				Locality: "us-west-2",
				Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
			},
		},
	}
	existingAndDesiredSESpec := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts: []string{"test-existing-and-desired.mesh"},
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			&istioNetworkingV1Alpha3.WorkloadEntry{
				Address:  "aws-lb.1.com",
				Locality: "us-west-2",
			},
		},
	}
	drInCacheSpec := &istioNetworkingV1Alpha3.DestinationRule{
		Host: "test.mesh",
	}
	existingAndDesiredSE := &v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-existing-and-desired.mesh-se",
			Namespace: namespace,
		},
		//nolint
		Spec: *existingAndDesiredSESpec,
	}
	drInCache := &v1alpha3.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dr-1",
		},
		//nolint
		Spec: *drInCacheSpec,
	}

	ctx := context.TODO()
	ctx = context.WithValue(ctx, common.EventResourceType, common.Rollout)
	ctx = context.WithValue(ctx, common.EventType, admiral.Add)
	fakeIstioClient := istiofake.NewSimpleClientset()
	existingSE, err := fakeIstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Create(ctx, existingAndDesiredSE, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("failed to create mock se: %v", err)
	}
	existingSEResourceVersion := existingSE.ResourceVersion
	rc := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient,
		},
	}
	rc.DestinationRuleController.Cache.Put(
		drInCache,
	)
	rc.ServiceEntryController.Cache.Put(
		existingAndDesiredSE,
		"cluster1",
	)
	admiralParams := common.GetAdmiralParams()
	admiralParams.EnableServiceEntryCache = true
	admiralParams.AlphaIdentityList = []string{"*"}
	admiralParams.SyncNamespace = namespace
	admiralParams.AdditionalEndpointSuffixes = []string{"intuit"}
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	rr := NewRemoteRegistry(nil, admiralParams)
	rr.PutRemoteController("cluster1", rc)
	errors1 := make(chan error, 1)
	cases := []struct {
		name                       string
		remoteRegistry             *RemoteRegistry
		additionalEndpointsEnabled bool
		isSourceCluster            bool
		identity                   string
		env                        string
		se                         *istioNetworkingV1Alpha3.ServiceEntry
		clusters                   []string
		errors                     chan error
		assertionFunc              func() error
	}{
		{
			name: "Given desired service entry does not exist," +
				"When function is called," +
				"Then it creates the desired service entry",
			remoteRegistry:             rr,
			additionalEndpointsEnabled: false,
			isSourceCluster:            false,
			identity:                   "identity1",
			env:                        "qal",
			se:                         seNotInCacheSpec,
			clusters:                   []string{"cluster1"},
			errors:                     errors1,
			assertionFunc: func() error {
				se, err := rr.
					GetRemoteController("cluster1").
					ServiceEntryController.
					IstioClient.NetworkingV1alpha3().
					ServiceEntries(namespace).
					Get(ctx, "test.mesh-se", metav1.GetOptions{})
				if err != nil {
					return err
				}
				if se != nil {
					return nil
				}
				return fmt.Errorf("se was nil")
			},
		},
		{
			name: "Given desired service entry does not exist," +
				"And create additional endpoints is enable, " +
				"When function is called, " +
				"Then it creates the desired service entry",
			remoteRegistry:             rr,
			additionalEndpointsEnabled: true,
			isSourceCluster:            false,
			identity:                   "identity1",
			env:                        "qal",
			se:                         seNotInCacheSpec,
			clusters:                   []string{"cluster1"},
			errors:                     errors1,
			assertionFunc: func() error {
				vs, err := rr.
					GetRemoteController("cluster1").
					VirtualServiceController.
					IstioClient.NetworkingV1alpha3().
					VirtualServices(namespace).
					Get(ctx, "qal.identity1.intuit-vs", metav1.GetOptions{})
				if err != nil {
					return err
				}
				if vs != nil {
					t.Logf("vs.name=%s", vs.Name)
					return nil
				}
				return fmt.Errorf("vs was nil")
			},
		},
		{
			name: "Given current == desired service entry," +
				"When function is called," +
				"Then it does not update the service entry",
			remoteRegistry:             rr,
			additionalEndpointsEnabled: false,
			isSourceCluster:            false,
			identity:                   "identity1",
			env:                        "qal",
			se:                         existingAndDesiredSESpec,
			clusters:                   []string{"cluster1"},
			errors:                     errors1,
			assertionFunc: func() error {
				se, err := rr.
					GetRemoteController("cluster1").
					ServiceEntryController.
					IstioClient.NetworkingV1alpha3().
					ServiceEntries(namespace).
					Get(ctx, "test-existing-and-desired.mesh-se", metav1.GetOptions{})
				if err != nil {
					return err
				}
				if se != nil {
					resourceVersion := se.ResourceVersion
					if resourceVersion != existingSEResourceVersion {
						return fmt.Errorf("resource version of se changed")
					}
					return nil
				}
				return fmt.Errorf("se was nil")
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clusterChan := make(chan string, 1)
			go AddServiceEntriesWithDrWorker(
				ctxLogger,
				ctx,
				rr,
				c.additionalEndpointsEnabled,
				c.isSourceCluster,
				c.identity,
				c.env,
				"",
				c.se,
				clusterChan,
				c.errors,
			)
			for _, cluster := range c.clusters {
				clusterChan <- cluster
			}
			close(clusterChan)
			var resultingErrors error
			for i := 1; i <= 1; i++ {
				resultingErrors = common.AppendError(resultingErrors, <-c.errors)
			}
			assertion := c.assertionFunc()
			if resultingErrors != assertion {
				t.Errorf("expected=%v got=%v", assertion, resultingErrors)
			}
		})
	}
}

func TestGetCurrentDRForLocalityLbSetting(t *testing.T) {
	ap := common.AdmiralParams{
		SyncNamespace: "ns",
	}

	common.ResetSync()
	common.InitializeConfig(ap)

	var (
		fakeIstioClient             = istiofake.NewSimpleClientset()
		sourceCluster1clusterID     = "test-dev1-k8s"
		sourceCluster2clusterID     = "test-dev2-k8s"
		destinationClusterclusterID = "test-dev3-k8s"
		rr                          = NewRemoteRegistry(nil, admiralParamsForServiceEntryTests())
	)

	rr.AdmiralCache.IdentityClusterCache.Put("identity1", sourceCluster1clusterID, sourceCluster1clusterID)
	rr.AdmiralCache.IdentityClusterCache.Put("identity1", sourceCluster2clusterID, sourceCluster2clusterID)
	rr.AdmiralCache.IdentityClusterCache.Put("identity2", sourceCluster1clusterID, sourceCluster1clusterID)

	rc1 := &RemoteController{
		ClusterID: sourceCluster1clusterID,
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
	}

	rc2 := &RemoteController{
		ClusterID: sourceCluster2clusterID,
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
	}

	rc3 := &RemoteController{
		ClusterID: destinationClusterclusterID,
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fakeIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
	}

	dummyDRConfig := &v1alpha3.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
			Name:        "identity1-default-dr",
			Namespace:   "ns",
		},
		Spec: istioNetworkingV1Alpha3.DestinationRule{
			Host: "dev.dummy.global",
		},
	}

	rc1.DestinationRuleController.Cache.Put(dummyDRConfig)
	rc3.DestinationRuleController.Cache.Put(dummyDRConfig)

	rr.PutRemoteController(sourceCluster1clusterID, rc1)
	rr.PutRemoteController(sourceCluster2clusterID, rc2)
	rr.PutRemoteController(destinationClusterclusterID, rc3)

	se := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:      []string{"identity1"},
		Location:   istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution: istioNetworkingV1Alpha3.ServiceEntry_DNS,
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{
				Address: "internal-lb.com",
				Ports: map[string]uint32{
					"http": 15443,
				},
				Labels: map[string]string{
					"deployment": "deployment",
				},
				Locality: "us-west-2",
			},
		},
	}

	seNotInCache := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:      []string{"identity2"},
		Location:   istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution: istioNetworkingV1Alpha3.ServiceEntry_DNS,
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{
				Address: "internal-lb.com",
				Ports: map[string]uint32{
					"http": 15443,
				},
				Labels: map[string]string{
					"deployment": "deployment",
				},
				Locality: "us-west-2",
			},
		},
	}

	testCases := []struct {
		name                                       string
		isServiceEntryModifyCalledForSourceCluster bool
		cluster                                    string
		identityId                                 string
		se                                         *istioNetworkingV1Alpha3.ServiceEntry
		expectedDR                                 *v1alpha3.DestinationRule
	}{
		{
			name: "Given that the application is present in the cache " +
				"And this is the source cluster " +
				"Then the func should return the value in the cache",
			isServiceEntryModifyCalledForSourceCluster: true,
			cluster:    sourceCluster1clusterID,
			identityId: "identity1",
			se:         se,
			expectedDR: &v1alpha3.DestinationRule{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
					Name:        "identity1-default-dr",
					Namespace:   "ns",
				},
				Spec: istioNetworkingV1Alpha3.DestinationRule{
					Host: "dev.dummy.global",
				},
			},
		},
		{
			name: "Given that the application is present in the cache " +
				"And this is the dependent cluster " +
				"Then the func should return the value in the cache",
			isServiceEntryModifyCalledForSourceCluster: false,
			cluster:    destinationClusterclusterID,
			identityId: "identity1",
			se:         se,
			expectedDR: &v1alpha3.DestinationRule{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
					Name:        "identity1-default-dr",
					Namespace:   "ns",
				},
				Spec: istioNetworkingV1Alpha3.DestinationRule{
					Host: "dev.dummy.global",
				},
			},
		},
		{
			name: "Given that the application is not present in the cache " +
				"And this is a source cluster " +
				"And this is the second cluster where the application is onboarded " +
				"Then the func should return the DR in the other source cluster",
			isServiceEntryModifyCalledForSourceCluster: true,
			cluster:    sourceCluster2clusterID,
			identityId: "identity1",
			se:         se,
			expectedDR: &v1alpha3.DestinationRule{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
					Name:        "identity1-default-dr",
					Namespace:   "ns",
				},
				Spec: istioNetworkingV1Alpha3.DestinationRule{
					Host: "dev.dummy.global",
				},
			},
		},
		{
			name: "Given that the application is not present in the cache " +
				"And this is a source cluster " +
				"Then the func should return a nil",
			isServiceEntryModifyCalledForSourceCluster: true,
			cluster:    sourceCluster1clusterID,
			identityId: "identity2",
			se:         seNotInCache,
			expectedDR: nil,
		},
		{
			name: "Given that the application is not present in the cache " +
				"And this is a dependent cluster " +
				"Then the func should return a nil",
			isServiceEntryModifyCalledForSourceCluster: false,
			cluster:    sourceCluster2clusterID,
			identityId: "identity2",
			se:         seNotInCache,
			expectedDR: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualDR := getCurrentDRForLocalityLbSetting(rr, tc.isServiceEntryModifyCalledForSourceCluster, tc.cluster, tc.se, tc.identityId)
			if !reflect.DeepEqual(tc.expectedDR, actualDR) {
				t.Errorf("expected DR %v but got %v", tc.expectedDR, actualDR)
			}
		})
	}
}

type MockClientConnectionConfigCache struct {
}

func (m MockClientConnectionConfigCache) GetFromIdentity(identity string, environment string) (*v13.ClientConnectionConfig, error) {
	return nil, nil
}

func (m MockClientConnectionConfigCache) Put(clientConnectionSettings *v13.ClientConnectionConfig) error {
	return fmt.Errorf("error adding to cache")
}

func (m MockClientConnectionConfigCache) Delete(identity string, environment string) error {
	return nil
}

func TestUpdateCnameDependentClusterNamespaceCache(t *testing.T) {
	admiralParams := admiralParamsForServiceEntryTests()
	admiralParams.EnableSWAwareNSCaches = true
	admiralParams.ExportToIdentityList = []string{"*"}
	admiralParams.ExportToMaxNamespaces = 35
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	var ctxLogger = logrus.WithFields(logrus.Fields{
		"type": "modifySE",
	})
	rr := NewRemoteRegistry(nil, admiralParams)
	rr.AdmiralCache.IdentityDependencyCache.Put("serveridentity", "dependent1identity", "dependent1identity")
	rr.AdmiralCache.IdentityDependencyCache.Put("serveridentity", "dependent2identity", "dependent2identity")
	rr.AdmiralCache.IdentityClusterCache.Put("serveridentity", "servercluster-k8s", "servercluster-k8s")
	rr.AdmiralCache.IdentityClusterCache.Put("dependent1identity", "servercluster-k8s", "servercluster-k8s")
	rr.AdmiralCache.IdentityClusterCache.Put("dependent2identity", "dependent2cluster-k8s", "dependent2cluster-k8s")
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put("serveridentity", "servercluster-k8s", "serverns", "serverns")
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put("dependent1identity", "servercluster-k8s", "dependent1ns", "dependent1ns")
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put("dependent2identity", "dependent2cluster-k8s", "dependent2ns1", "dependent2ns1")
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put("dependent2identity", "dependent2cluster-k8s", "dependent2ns2", "dependent2ns2")
	rr.AdmiralCache.CnameDependentClusterCache.Put("test.serveridentity1.mesh", "servercluster-k8s", "servercluster-k8s")

	fakeService := buildServiceForDeployment("fakeservicename", "fakeservicens", "fakeserviceid")

	clusterResourcetypeServiceMap := make(map[string]map[string]*v1.Service)
	clusterResourcetypeServiceMap["servercluster-k8s"] = make(map[string]*v1.Service)
	clusterResourcetypeServiceMap["servercluster-k8s"][common.Deployment] = fakeService

	expectedCnameDependentClusterCache := common.NewMapOfMaps()
	expectedCnameDependentClusterCache.Put("test.serveridentity.mesh", "dependent2cluster-k8s", "dependent2cluster-k8s")

	expectedCnameDependentClusterNamespaceCache := common.NewMapOfMapOfMaps()
	expectedCnameDependentClusterNamespaceCache.Put("test.serveridentity.mesh", "servercluster-k8s", "dependent1ns", "dependent1ns")
	expectedCnameDependentClusterNamespaceCache.Put("test.serveridentity.mesh", "dependent2cluster-k8s", "dependent2ns1", "dependent2ns1")
	expectedCnameDependentClusterNamespaceCache.Put("test.serveridentity.mesh", "dependent2cluster-k8s", "dependent2ns2", "dependent2ns2")

	expectedCnameDependentClusterNamespaceCache1 := common.NewMapOfMapOfMaps()
	expectedCnameDependentClusterNamespaceCache1.Put("test.serveridentity1.mesh", "servercluster-k8s", "dependent1ns", "dependent1ns")

	testCases := []struct {
		name                                        string
		dependents                                  map[string]string
		deploymentOrRolloutName                     string
		deploymentOrRolloutNS                       string
		cname                                       string
		clusterResourcetypeServiceMap               map[string]map[string]*v1.Service
		expectedCnameDependentClusterCache          *common.MapOfMaps
		expectedCnameDependentClusterNamespaceCache *common.MapOfMapOfMaps
	}{
		{
			name: "Given nil dependents map " +
				"And we update the cname caches " +
				"Then the CnameDependentClusterCache should be empty " +
				"And the CnameDependentClusterNamespaceCache should be empty",
			dependents:                         nil,
			deploymentOrRolloutName:            "serverdeployment",
			deploymentOrRolloutNS:              "serverns",
			cname:                              "test.serveridentity.mesh",
			clusterResourcetypeServiceMap:      clusterResourcetypeServiceMap,
			expectedCnameDependentClusterCache: common.NewMapOfMaps(),
			expectedCnameDependentClusterNamespaceCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given empty dependents map " +
				"And we update the cname caches " +
				"Then the CnameDependentClusterCache should be empty " +
				"And the CnameDependentClusterNamespaceCache should be empty",
			dependents:                         map[string]string{},
			deploymentOrRolloutName:            "serverdeployment",
			deploymentOrRolloutNS:              "serverns",
			cname:                              "test.serveridentity.mesh",
			clusterResourcetypeServiceMap:      clusterResourcetypeServiceMap,
			expectedCnameDependentClusterCache: common.NewMapOfMaps(),
			expectedCnameDependentClusterNamespaceCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given no dependents match " +
				"And we update the cname caches " +
				"Then the CnameDependentClusterCache should be empty " +
				"And the CnameDependentClusterNamespaceCache should be empty",
			dependents:                         map[string]string{"dependent3identity": "dependent3identity", "dependent4identity": "dependent4identity"},
			deploymentOrRolloutName:            "serverdeployment",
			deploymentOrRolloutNS:              "serverns",
			cname:                              "test.serveridentity.mesh",
			clusterResourcetypeServiceMap:      clusterResourcetypeServiceMap,
			expectedCnameDependentClusterCache: common.NewMapOfMaps(),
			expectedCnameDependentClusterNamespaceCache: common.NewMapOfMapOfMaps(),
		},
		{
			name: "Given a service with dependents in the same cluster and other clusters " +
				"And we update the cname caches " +
				"Then the CnameDependentClusterCache should not contain the source cluster " +
				"And the CnameDependentClusterNamespaceCache should contain the source cluster",
			dependents:                         map[string]string{"dependent1identity": "dependent1identity", "dependent2identity": "dependent2identity"},
			deploymentOrRolloutName:            "serverdeployment",
			deploymentOrRolloutNS:              "serverns",
			cname:                              "test.serveridentity.mesh",
			clusterResourcetypeServiceMap:      clusterResourcetypeServiceMap,
			expectedCnameDependentClusterCache: expectedCnameDependentClusterCache,
			expectedCnameDependentClusterNamespaceCache: expectedCnameDependentClusterNamespaceCache,
		},
		{
			name: "Given a service has dependents in the same cluster " +
				"And CnameDependentClusterCache already contains the source cluster, " +
				"Then the source cluster should be removed from the CnameDependentClusterCache",
			dependents:                         map[string]string{"dependent1identity": "dependent1identity"},
			deploymentOrRolloutName:            "serverdeployment",
			deploymentOrRolloutNS:              "serverns",
			cname:                              "test.serveridentity1.mesh",
			clusterResourcetypeServiceMap:      clusterResourcetypeServiceMap,
			expectedCnameDependentClusterCache: common.NewMapOfMaps(),
			expectedCnameDependentClusterNamespaceCache: expectedCnameDependentClusterNamespaceCache1,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			updateCnameDependentClusterNamespaceCache(
				ctxLogger,
				rr,
				c.dependents,
				c.deploymentOrRolloutName,
				c.deploymentOrRolloutNS,
				c.cname,
				clusterResourcetypeServiceMap)
			expectedCnameDependentClusters := map[string]string{}
			cnameDependentClusters := map[string]string{}
			expectedCnameDependentClusterNamespaces := common.NewMapOfMaps()
			cnameDependentClusterNamespaces := common.NewMapOfMaps()

			if c.expectedCnameDependentClusterCache.Get(c.cname) != nil {
				expectedCnameDependentClusters = c.expectedCnameDependentClusterCache.Get(c.cname).Copy()
			}
			if rr.AdmiralCache.CnameDependentClusterCache.Get(c.cname) != nil {
				cnameDependentClusters = rr.AdmiralCache.CnameDependentClusterCache.Get(c.cname).Copy()
			}
			if c.expectedCnameDependentClusterNamespaceCache.Len() > 0 {
				expectedCnameDependentClusterNamespaces = c.expectedCnameDependentClusterNamespaceCache.Get(c.cname)
			}
			if rr.AdmiralCache.CnameDependentClusterNamespaceCache.Len() > 0 {
				cnameDependentClusterNamespaces = rr.AdmiralCache.CnameDependentClusterNamespaceCache.Get(c.cname)
			}

			if !reflect.DeepEqual(expectedCnameDependentClusters, cnameDependentClusters) {
				t.Errorf("expected dependent clusters: %v but got: %v", expectedCnameDependentClusters, cnameDependentClusters)
			}
			if !reflect.DeepEqual(expectedCnameDependentClusterNamespaces, cnameDependentClusterNamespaces) {
				t.Errorf("expected dependent cluster namespaces: %+v but got: %+v", expectedCnameDependentClusterNamespaces, cnameDependentClusterNamespaces)
			}
		})
	}
}

func TestPartitionAwarenessExportTo(t *testing.T) {
	admiralParams := admiralParamsForServiceEntryTests()
	admiralParams.DisableIPGeneration = true
	admiralParams.EnableSWAwareNSCaches = true
	admiralParams.ExportToIdentityList = []string{"*"}
	admiralParams.ExportToMaxNamespaces = 35
	admiralParams.AdditionalEndpointSuffixes = []string{"intuit"}
	admiralParams.AdditionalEndpointLabelFilters = []string{"foo"}
	admiralParams.CacheReconcileDuration = 10 * time.Minute
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	var (
		env                           = "test"
		stop                          = make(chan struct{})
		sourceIstioClient             = istiofake.NewSimpleClientset()
		remoteIstioClient             = istiofake.NewSimpleClientset()
		config                        = rest.Config{Host: "localhost"}
		resyncPeriod                  = time.Millisecond * 1000
		partitionedRollout            = makeTestRollout("partitionedrolloutname", "partitionedrollout-ns", "partitionedrolloutidentity")
		dependentInSourceCluster      = makeTestDeployment("dependentinsourceclustername", "dependentinsourcecluster-ns", "dependentinsourceclusteridentity")
		dependentInRemoteCluster      = makeTestRollout("dependentinremoteclustername", "dependentinremotecluster-ns", "dependentinremoteclusteridentity")
		dependentInBothClustersSrc    = makeTestDeployment("dependentinbothclustername", "dependentinbothsourcecluster-ns", "dependentinbothclusteridentity")
		dependentInBothClustersRem    = makeTestDeployment("dependentinbothclustername", "dependentinbothremotecluster-ns", "dependentinbothclusteridentity")
		partitionedRolloutSvc         = buildServiceForRollout("partitionedrolloutname", "partitionedrollout-ns", "partitionedrolloutidentity")
		dependentInSourceClusterSvc   = buildServiceForDeployment("dependentinsourceclustername", "dependentinsourcecluster-ns", "dependentinsourceclusteridentity")
		dependentInRemoteClusterSvc   = buildServiceForRollout("dependentinremoteclustername", "dependentinremotecluster-ns", "dependentinremoteclusteridentity")
		dependentInBothClustersSrcSvc = buildServiceForDeployment("dependentinbothclustername", "dependentinbothsourcecluster-ns", "dependentinbothclusteridentity")
		dependentInBothClustersRemSvc = buildServiceForDeployment("dependentinbothclustername", "dependentinbothremotecluster-ns", "dependentinbothclusteridentity")

		remoteClusterSE = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:    []string{"test.partitionedrolloutidentity.mesh"},
			ExportTo: []string{"dependentinbothremotecluster-ns", "dependentinremotecluster-ns", "fake-ns"},
		}
		sourceClusterSE = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:    []string{"test.partitionedrolloutidentity.mesh"},
			ExportTo: []string{"dependentinbothsourcecluster-ns", "dependentinsourcecluster-ns", common.NamespaceIstioSystem, "partitionedrollout-ns"},
		}
		existingSourceClusterSE = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:    []string{"test.partitionedrolloutidentity.mesh"},
			ExportTo: []string{"dependentinbothsourcecluster-ns"},
		}
		remoteClusterDR = &istioNetworkingV1Alpha3.DestinationRule{
			Host:     "test.partitionedrolloutidentity.mesh",
			ExportTo: []string{"dependentinbothremotecluster-ns", "dependentinremotecluster-ns", "fake-ns"},
		}
		updatedRemoteClusterDR = &istioNetworkingV1Alpha3.DestinationRule{
			Host:     "test.partitionedrolloutidentity.mesh",
			ExportTo: []string{"dependentinbothremotecluster-ns", "dependentinremotecluster-ns"},
		}
		sourceClusterDR = &istioNetworkingV1Alpha3.DestinationRule{
			Host:     "test.partitionedrolloutidentity.mesh",
			ExportTo: []string{"dependentinbothsourcecluster-ns", "dependentinsourcecluster-ns", common.NamespaceIstioSystem, "partitionedrollout-ns"},
		}
		remoteClusterVS = &istioNetworkingV1Alpha3.VirtualService{
			Hosts:    []string{"test.partitionedrolloutidentity.intuit"},
			ExportTo: []string{"dependentinbothremotecluster-ns", "dependentinremotecluster-ns"},
		}
		sourceClusterVS = &istioNetworkingV1Alpha3.VirtualService{
			Hosts:    []string{"test.partitionedrolloutidentity.intuit"},
			ExportTo: []string{"dependentinbothsourcecluster-ns", "dependentinsourcecluster-ns", common.NamespaceIstioSystem, "partitionedrollout-ns"},
		}
		serviceEntryAddressStore = &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{},
			Addresses:      []string{},
		}
	)
	partitionedRollout.Labels["foo"] = "bar"
	serviceForIngress := &coreV1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "east.aws.lb",
			Namespace: "istio-system",
			Labels:    map[string]string{"app": "gatewayapp"},
		},
		Spec: coreV1.ServiceSpec{
			Ports: []coreV1.ServicePort{
				{
					Name: "http",
					Port: 8090,
				},
			},
		},
		Status: coreV1.ServiceStatus{
			LoadBalancer: coreV1.LoadBalancerStatus{
				Ingress: []coreV1.LoadBalancerIngress{
					{
						Hostname: "east.aws.lb",
					},
				},
			},
		},
	}
	partitionedRollout.Spec.Template.Annotations[common.GetPartitionIdentifier()] = "partition"
	sourceDeploymentController, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	remoteDeploymentController, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	sourceRolloutController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	remoteRolloutController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	sourceServiceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	remoteServiceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	sourceGtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	remoteGtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	cacheController := &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(serviceEntryAddressStore, "123"),
	}

	rr := NewRemoteRegistry(nil, admiralParams)
	rr.AdmiralCache.ConfigMapController = cacheController
	rr.AdmiralCache.IdentityClusterCache.Put(common.GetRolloutGlobalIdentifier(&partitionedRollout), "source-cluster-k8s", "source-cluster-k8s")
	rr.AdmiralCache.IdentityClusterCache.Put(common.GetDeploymentGlobalIdentifier(dependentInSourceCluster), "source-cluster-k8s", "source-cluster-k8s")
	rr.AdmiralCache.IdentityClusterCache.Put(common.GetDeploymentGlobalIdentifier(dependentInBothClustersSrc), "source-cluster-k8s", "source-cluster-k8s")
	rr.AdmiralCache.IdentityClusterCache.Put(common.GetRolloutGlobalIdentifier(&dependentInRemoteCluster), "remote-cluster-k8s", "remote-cluster-k8s")
	rr.AdmiralCache.IdentityClusterCache.Put(common.GetDeploymentGlobalIdentifier(dependentInBothClustersRem), "remote-cluster-k8s", "remote-cluster-k8s")
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put(common.GetRolloutGlobalIdentifier(&partitionedRollout), "source-cluster-k8s", "partitionedrollout-ns", "partitionedrollout-ns")
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put(common.GetDeploymentGlobalIdentifier(dependentInSourceCluster), "source-cluster-k8s", "dependentinsourcecluster-ns", "dependentinsourcecluster-ns")
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put(common.GetDeploymentGlobalIdentifier(dependentInBothClustersSrc), "source-cluster-k8s", "dependentinbothsourcecluster-ns", "dependentinbothsourcecluster-ns")
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put(common.GetRolloutGlobalIdentifier(&dependentInRemoteCluster), "remote-cluster-k8s", "dependentinremotecluster-ns", "dependentinremotecluster-ns")
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put(common.GetDeploymentGlobalIdentifier(dependentInBothClustersRem), "remote-cluster-k8s", "dependentinbothremotecluster-ns", "dependentinbothremotecluster-ns")
	rr.AdmiralCache.IdentityDependencyCache.Put("partition.partitionedrolloutidentity", "dependentinsourceclusteridentity", "dependentinsourceclusteridentity")
	rr.AdmiralCache.IdentityDependencyCache.Put("partition.partitionedrolloutidentity", "dependentinremoteclusteridentity", "dependentinremoteclusteridentity")
	rr.AdmiralCache.IdentityDependencyCache.Put("partition.partitionedrolloutidentity", "dependentinbothclusteridentity", "dependentinbothclusteridentity")
	rr.AdmiralCache.PartitionIdentityCache.Put("partition.partitionedrolloutidentity", "partitionedrolloutidentity")

	sourceRolloutController.Cache.UpdateRolloutToClusterCache("partition.partitionedrolloutidentity", &partitionedRollout)
	remoteRolloutController.Cache.UpdateRolloutToClusterCache("dependentinremoteclusteridentity", &dependentInRemoteCluster)
	sourceDeploymentController.Cache.UpdateDeploymentToClusterCache("dependentinsourceclusteridentity", dependentInSourceCluster)
	sourceDeploymentController.Cache.UpdateDeploymentToClusterCache("dependentinbothclusteridentity", dependentInBothClustersSrc)
	remoteDeploymentController.Cache.UpdateDeploymentToClusterCache("dependentinbothclusteridentity", dependentInBothClustersRem)
	sourceServiceController.Cache.Put(partitionedRolloutSvc)
	sourceServiceController.Cache.Put(dependentInSourceClusterSvc)
	sourceServiceController.Cache.Put(dependentInBothClustersSrcSvc)
	sourceServiceController.Cache.Put(serviceForIngress)
	remoteServiceController.Cache.Put(dependentInRemoteClusterSvc)
	remoteServiceController.Cache.Put(dependentInBothClustersRemSvc)

	sourceRc := &RemoteController{
		ClusterID:            "source-cluster-k8s",
		DeploymentController: sourceDeploymentController,
		RolloutController:    sourceRolloutController,
		ServiceController:    sourceServiceController,
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: sourceIstioClient,
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: sourceIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: sourceIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic: sourceGtpc,
		StartTime:     time.Now(),
	}

	remoteRc := &RemoteController{
		ClusterID:            "remote-cluster-k8s",
		DeploymentController: remoteDeploymentController,
		RolloutController:    remoteRolloutController,
		ServiceController:    remoteServiceController,
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: remoteIstioClient,
		},
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-east-2",
			},
		},
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: remoteIstioClient,
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: remoteIstioClient,
			Cache:       istio.NewDestinationRuleCache(),
		},
		GlobalTraffic: remoteGtpc,
		StartTime:     time.Now(),
	}

	rr.PutRemoteController("source-cluster-k8s", sourceRc)
	rr.PutRemoteController("remote-cluster-k8s", remoteRc)
	existingSourceClusterSEv1 := createServiceEntrySkeleton(*existingSourceClusterSE, "test.partitionedrolloutidentity.mesh-se", common.GetSyncNamespace())
	existingSourceClusterSEv1.Annotations = map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue}
	sourceRc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries("ns").Create(context.Background(), existingSourceClusterSEv1, metav1.CreateOptions{})
	sourceRc.ServiceEntryController.Cache.Put(existingSourceClusterSEv1, "source-cluster-k8s")
	remoteClusterSEv1 := createServiceEntrySkeleton(*remoteClusterSE, "test.partitionedrolloutidentity.mesh-se", common.GetSyncNamespace())
	remoteClusterSEv1.Annotations = map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue}
	remoteRc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries("ns").Create(context.Background(), remoteClusterSEv1, metav1.CreateOptions{})
	remoteRc.ServiceEntryController.Cache.Put(remoteClusterSEv1, "remote-cluster-k8s")
	remoteClusterDRv1 := createDestinationRuleSkeleton(*remoteClusterDR, "test.partitionedrolloutidentity.mesh-se", common.GetSyncNamespace())
	remoteClusterDRv1.Annotations = map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue}
	remoteRc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules("ns").Create(context.Background(), remoteClusterDRv1, metav1.CreateOptions{})
	remoteRc.DestinationRuleController.Cache.Put(remoteClusterDRv1)
	rr.StartTime = time.Now().Add(-1 * common.GetAdmiralParams().CacheReconcileDuration)

	testCases := []struct {
		name                           string
		assetIdentity                  string
		expectedSourceServiceEntries   map[string]*istioNetworkingV1Alpha3.ServiceEntry
		expectedRemoteServiceEntries   map[string]*istioNetworkingV1Alpha3.ServiceEntry
		expectedSourceDestinationRules map[string]*istioNetworkingV1Alpha3.DestinationRule
		expectedRemoteDestinationRules map[string]*istioNetworkingV1Alpha3.DestinationRule
		expectedSourceVirtualServices  map[string]*istioNetworkingV1Alpha3.VirtualService
		expectedRemoteVirtualServices  map[string]*istioNetworkingV1Alpha3.VirtualService
		eventResourceType              string
	}{
		{
			name: "Given a SE is getting updated due to a Rollout, " +
				"And partition awareness feature is enabled, " +
				"Then the SE ExportTo field contains the dependent service namespaces for the appropriate cluster",
			assetIdentity: "partition.partitionedrolloutidentity",
			expectedSourceServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.partitionedrolloutidentity.mesh": sourceClusterSE,
			},
			expectedRemoteServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.partitionedrolloutidentity.mesh": remoteClusterSE,
			},
			expectedSourceDestinationRules: map[string]*istioNetworkingV1Alpha3.DestinationRule{
				"test.partitionedrolloutidentity.mesh": sourceClusterDR,
			},
			expectedRemoteDestinationRules: map[string]*istioNetworkingV1Alpha3.DestinationRule{
				"test.partitionedrolloutidentity.mesh": updatedRemoteClusterDR,
			},
			expectedSourceVirtualServices: map[string]*istioNetworkingV1Alpha3.VirtualService{
				"test.partitionedrolloutidentity.intuit": sourceClusterVS,
			},
			expectedRemoteVirtualServices: map[string]*istioNetworkingV1Alpha3.VirtualService{
				"test.partitionedrolloutidentity.intuit": remoteClusterVS,
			},
			eventResourceType: common.Rollout,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = context.WithValue(ctx, "clusterName", "source-cluster-k8s")
			ctx = context.WithValue(ctx, "eventResourceType", c.eventResourceType)

			_, err = modifyServiceEntryForNewServiceOrPod(ctx, admiral.Add, env, c.assetIdentity, rr)

			for _, expectedServiceEntry := range c.expectedSourceServiceEntries {
				seName := getIstioResourceName(expectedServiceEntry.Hosts[0], "-se")
				createdSe, err := sourceIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Get(ctx, seName, metav1.GetOptions{})
				if err != nil || createdSe == nil {
					t.Errorf("expected the service entry %s but it wasn't found", seName)
				} else if !reflect.DeepEqual(createdSe.Spec.ExportTo, expectedServiceEntry.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for sourceSE", expectedServiceEntry.ExportTo, createdSe.Spec.ExportTo)
				}
			}
			for _, expectedServiceEntry := range c.expectedRemoteServiceEntries {
				seName := getIstioResourceName(expectedServiceEntry.Hosts[0], "-se")
				createdSe, err := remoteIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Get(ctx, seName, metav1.GetOptions{})
				if err != nil || createdSe == nil {
					t.Errorf("expected the service entry %s but it wasn't found", seName)
				} else if !reflect.DeepEqual(createdSe.Spec.ExportTo, expectedServiceEntry.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for remoteSE", expectedServiceEntry.ExportTo, createdSe.Spec.ExportTo)
				}
			}
			for _, expectedDestinationRule := range c.expectedSourceDestinationRules {
				drName := getIstioResourceName(expectedDestinationRule.Host, "-default-dr")
				createdDr, err := sourceIstioClient.NetworkingV1alpha3().DestinationRules("ns").Get(ctx, drName, metav1.GetOptions{})
				if err != nil || createdDr == nil {
					t.Errorf("expected the destination rule %s but it wasn't found", drName)
				} else if !reflect.DeepEqual(createdDr.Spec.ExportTo, expectedDestinationRule.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for sourceDR", expectedDestinationRule.ExportTo, createdDr.Spec.ExportTo)
				}
			}
			for _, expectedDestinationRule := range c.expectedRemoteDestinationRules {
				drName := getIstioResourceName(expectedDestinationRule.Host, "-default-dr")
				createdDr, err := remoteIstioClient.NetworkingV1alpha3().DestinationRules("ns").Get(ctx, drName, metav1.GetOptions{})
				if err != nil || createdDr == nil {
					t.Errorf("expected the destination rule %s but it wasn't found", drName)
				} else if !reflect.DeepEqual(createdDr.Spec.ExportTo, expectedDestinationRule.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for remoteDR", expectedDestinationRule.ExportTo, createdDr.Spec.ExportTo)
				}
			}
			for _, expectedVirtualService := range c.expectedSourceVirtualServices {
				vsName := getIstioResourceName(expectedVirtualService.Hosts[0], "-vs")
				createdVs, err := sourceIstioClient.NetworkingV1alpha3().VirtualServices("ns").Get(ctx, vsName, metav1.GetOptions{})
				if err != nil || createdVs == nil {
					vs, err := sourceIstioClient.NetworkingV1alpha3().VirtualServices("ns").List(ctx, metav1.ListOptions{})
					t.Logf("vs %v with err %v", vs, err)
					t.Errorf("expected the virtual service %s but it wasn't found", vsName)
				} else if !reflect.DeepEqual(createdVs.Spec.ExportTo, expectedVirtualService.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for sourceVS", expectedVirtualService.ExportTo, createdVs.Spec.ExportTo)
				}
			}
			for _, expectedVirtualService := range c.expectedRemoteVirtualServices {
				vsName := getIstioResourceName(expectedVirtualService.Hosts[0], "-vs")
				createdVs, err := remoteIstioClient.NetworkingV1alpha3().VirtualServices("ns").Get(ctx, vsName, metav1.GetOptions{})
				if err != nil || createdVs == nil {
					t.Errorf("expected the virtual service %s but it wasn't found", vsName)
				} else if !reflect.DeepEqual(createdVs.Spec.ExportTo, expectedVirtualService.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for remoteVS", expectedVirtualService.ExportTo, createdVs.Spec.ExportTo)
				}
			}
		})
	}
}

func compareServiceEntries(se1, se2 *istioNetworkingV1Alpha3.ServiceEntry) bool {
	if se1 == se2 {
		return true
	}

	if se1 == nil || se2 == nil {
		return se1 == se2
	}

	if !reflect.DeepEqual(se1.Hosts, se2.Hosts) {
		return false
	}
	if !reflect.DeepEqual(se1.Addresses, se2.Addresses) {
		return false
	}
	if !reflect.DeepEqual(se1.Ports, se2.Ports) {
		return false
	}
	if se1.Location != se2.Location {
		return false
	}
	if se1.Resolution != se2.Resolution {
		return false
	}
	if !reflect.DeepEqual(se1.ExportTo, se2.ExportTo) {
		return false
	}
	if !reflect.DeepEqual(se1.SubjectAltNames, se2.SubjectAltNames) {
		return false
	}
	if !compareWorkloadEntries(se1.Endpoints, se2.Endpoints) {
		return false
	}

	return true
}

// compareWorkloadEntries compares two slices of WorkloadEntry objects.
func compareWorkloadEntries(wl1, wl2 []*istioNetworkingV1Alpha3.WorkloadEntry) bool {
	if len(wl1) != len(wl2) {
		return false
	}

	for i := range wl1 {
		if !compareWorkloadEntry(wl1[i], wl2[i]) {
			return false
		}
	}

	return true
}

// compareWorkloadEntry compares two WorkloadEntry objects.
func compareWorkloadEntry(w1, w2 *istioNetworkingV1Alpha3.WorkloadEntry) bool {
	if w1.Address != w2.Address {
		return false
	}
	if !reflect.DeepEqual(w1.Ports, w2.Ports) {
		return false
	}
	if w1.Locality != w2.Locality {
		return false
	}
	if !reflect.DeepEqual(w1.Labels, w2.Labels) {
		return false
	}

	return true
}

func TestPartitionAwarenessExportToMultipleRemote(t *testing.T) {
	admiralParams := admiralParamsForServiceEntryTests()
	admiralParams.DisableIPGeneration = true
	admiralParams.EnableSWAwareNSCaches = true
	admiralParams.ExportToIdentityList = []string{"*"}
	admiralParams.ExportToMaxNamespaces = 35
	admiralParams.AdditionalEndpointSuffixes = []string{"intuit"}
	admiralParams.AdditionalEndpointLabelFilters = []string{"foo"}
	admiralParams.CacheReconcileDuration = 0 * time.Minute
	admiralParams.DependentClusterWorkerConcurrency = 5
	admiralParams.SyncNamespace = "ns"
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	var (
		env                          = "test"
		stop                         = make(chan struct{})
		sourceIstioClient            = istiofake.NewSimpleClientset()
		gwIstioClient                = istiofake.NewSimpleClientset()
		remoteClusters               = 100
		config                       = rest.Config{Host: "localhost"}
		resyncPeriod                 = time.Millisecond * 1000
		dependentInSourceCluster     = makeTestDeployment("dependentinsourceclustername", "dependentinsourcecluster-ns", "dependentinsourceclusteridentity")
		dependentInSourceClusterSvc  = buildServiceForDeployment("dependentinsourceclustername", "dependentinsourcecluster-ns", "dependentinsourceclusteridentity")
		partitionedRollout           = makeTestRollout("partitionedrolloutname", "partitionedrollout-ns", "partitionedrolloutidentity")
		partitionedRolloutSvc        = buildServiceForRollout("partitionedrolloutname", "partitionedrollout-ns", "partitionedrolloutidentity")
		gwAsADependentInGWCluster    = makeTestDeployment("gatewayasdependentname", "gatewayasdependent-ns", "intuit.platform.servicesgateway.servicesgateway")
		gwAsADependentInGWClusterSvc = buildServiceForDeployment("gatewayasdependentname", "gatewayasdependent-ns", "intuit.platform.servicesgateway.servicesgateway")
		sourceClusterSE              = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:    []string{"test.partitionedrolloutidentity.mesh"},
			ExportTo: []string{"dependentinsourcecluster-ns", common.NamespaceIstioSystem, "partitionedrollout-ns"},
		}
		sourceClusterDR = &istioNetworkingV1Alpha3.DestinationRule{
			Host:     "test.partitionedrolloutidentity.mesh",
			ExportTo: []string{"dependentinsourcecluster-ns", common.NamespaceIstioSystem, "partitionedrollout-ns"},
		}
		sourceClusterVS = &istioNetworkingV1Alpha3.VirtualService{
			Hosts:    []string{"test.partitionedrolloutidentity.intuit"},
			ExportTo: []string{"dependentinsourcecluster-ns", common.NamespaceIstioSystem, "partitionedrollout-ns"},
		}
		clusterID = "source-cluster-k8s"

		gwRemoteClusterSE = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts: []string{"test.partitionedrolloutidentity.mesh"},
		}
		gwRemoteClusterDR = &istioNetworkingV1Alpha3.DestinationRule{
			Host: "test.partitionedrolloutidentity.mesh",
		}
		gwRemoteClusterVS = &istioNetworkingV1Alpha3.VirtualService{
			Hosts: []string{"test.partitionedrolloutidentity.intuit"},
		}
		gwClusterID = "gateway-cluster-k8s"
	)
	deploymentController, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("Failed to create deployment controller for %s: %v", clusterID, err)
	}

	rolloutController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("Failed to create rollout controller for %s: %v", clusterID, err)
	}

	serviceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("Failed to create service controller for %s: %v", clusterID, err)
	}

	gtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("Failed to create global traffic controller for %s: %v", clusterID, err)
	}
	// Virtual Service, Service Entry, and Destination Rule Controllers
	vsController := &istio.VirtualServiceController{IstioClient: sourceIstioClient}
	seController := &istio.ServiceEntryController{IstioClient: sourceIstioClient, Cache: istio.NewServiceEntryCache()}
	drController := &istio.DestinationRuleController{IstioClient: sourceIstioClient, Cache: istio.NewDestinationRuleCache()}

	gwdeploymentController, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("Failed to create deployment controller for %s: %v", gwClusterID, err)
	}

	gwrolloutController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("Failed to create rollout controller for %s: %v", gwClusterID, err)
	}

	gwserviceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("Failed to create service controller for %s: %v", gwClusterID, err)
	}

	gwgtpc, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("Failed to create global traffic controller for %s: %v", gwClusterID, err)
	}
	// Virtual Service, Service Entry, and Destination Rule Controllers
	gwvsController := &istio.VirtualServiceController{IstioClient: gwIstioClient}
	gwseController := &istio.ServiceEntryController{IstioClient: gwIstioClient, Cache: istio.NewServiceEntryCache()}
	gwdrController := &istio.DestinationRuleController{IstioClient: gwIstioClient, Cache: istio.NewDestinationRuleCache()}

	sourceRc := &RemoteController{
		ClusterID:                 "source-cluster-k8s",
		DeploymentController:      deploymentController,
		RolloutController:         rolloutController,
		ServiceController:         serviceController,
		VirtualServiceController:  vsController,
		ServiceEntryController:    seController,
		DestinationRuleController: drController,
		GlobalTraffic:             gtpc,
		StartTime:                 time.Now(),
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
	}

	gwRc := &RemoteController{
		ClusterID:                 gwClusterID,
		DeploymentController:      gwdeploymentController,
		RolloutController:         gwrolloutController,
		ServiceController:         gwserviceController,
		VirtualServiceController:  gwvsController,
		ServiceEntryController:    gwseController,
		DestinationRuleController: gwdrController,
		GlobalTraffic:             gwgtpc,
		StartTime:                 time.Now(),
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
	}
	// Global Traffic Controller
	partitionedRollout.Labels["foo"] = "bar"
	partitionedRollout.Spec.Template.Annotations[common.GetPartitionIdentifier()] = "partition"

	remoteControllers := make([]*RemoteController, remoteClusters)
	dependentInRemoteClusters := make([]argo.Rollout, remoteClusters)
	dependentInRemoteClustersSvc := make([]*coreV1.Service, remoteClusters)
	remoteClusterSEs := make([]*istioNetworkingV1Alpha3.ServiceEntry, remoteClusters)
	remoteClusterDRs := make([]*istioNetworkingV1Alpha3.DestinationRule, remoteClusters)
	remoteDeploymentController := make([]*admiral.DeploymentController, remoteClusters)
	remoteRolloutController := make([]*admiral.RolloutController, remoteClusters)
	remoteServiceController := make([]*admiral.ServiceController, remoteClusters)
	remoteVSController := make([]*istio.VirtualServiceController, remoteClusters)
	remoteSEController := make([]*istio.ServiceEntryController, remoteClusters)
	remoteDRController := make([]*istio.DestinationRuleController, remoteClusters)
	remoteGtpController := make([]*admiral.GlobalTrafficController, remoteClusters)
	remoteNodeController := make([]*admiral.NodeController, remoteClusters)
	clusterIDs := make([]string, remoteClusters)
	remoteIstioClient := make([]*istiofake.Clientset, remoteClusters)
	for i := 1; i <= remoteClusters; i++ {
		clusterIDs[i-1] = fmt.Sprintf("remote-cluster-%d", i-1)
		// Setup the controllers for this cluster
		remoteDeploymentController[i-1], _ = admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
		remoteRolloutController[i-1], _ = admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
		remoteServiceController[i-1], _ = admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
		remoteGtpController[i-1], _ = admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
		remoteIstioClient[i-1] = istiofake.NewSimpleClientset()
		remoteVSController[i-1] = &istio.VirtualServiceController{IstioClient: remoteIstioClient[i-1]}
		remoteSEController[i-1] = &istio.ServiceEntryController{IstioClient: remoteIstioClient[i-1], Cache: istio.NewServiceEntryCache()}
		remoteDRController[i-1] = &istio.DestinationRuleController{IstioClient: remoteIstioClient[i-1], Cache: istio.NewDestinationRuleCache()}
		remoteNodeController[i-1] = &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		}
		// Create the Remote Controller
		remoteControllers[i-1] = &RemoteController{
			ClusterID:                 clusterIDs[i-1],
			DeploymentController:      remoteDeploymentController[i-1],
			RolloutController:         remoteRolloutController[i-1],
			ServiceController:         remoteServiceController[i-1],
			VirtualServiceController:  remoteVSController[i-1],
			ServiceEntryController:    remoteSEController[i-1],
			DestinationRuleController: remoteDRController[i-1],
			GlobalTraffic:             remoteGtpController[i-1],
			NodeController:            remoteNodeController[i-1],
			StartTime:                 time.Now(),
		}
		dependentInRemoteClusters[i-1] = makeTestRollout(fmt.Sprintf("dependentinremoteclustername-%d", i-1), fmt.Sprintf("dependentinremotecluster-ns-%d", i-1), "dependentinremoteclusteridentity")
		dependentInRemoteClustersSvc[i-1] = buildServiceForRollout(fmt.Sprintf("dependentinremoteclustername-%d", i-1), fmt.Sprintf("dependentinremotecluster-ns-%d", i-1), "dependentinremoteclusteridentity")
		remoteClusterSEs[i-1] = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:    []string{fmt.Sprintf("test.partitionedrolloutidentity.mesh-%d", i-1)},
			ExportTo: []string{fmt.Sprintf("dependentinremotecluster-ns-%d", i-1)},
		}
		remoteClusterDRs[i-1] = &istioNetworkingV1Alpha3.DestinationRule{
			Host:     fmt.Sprintf("test.partitionedrolloutidentity.mesh-%d", i-1),
			ExportTo: []string{fmt.Sprintf("dependentinremotecluster-ns-%d", i-1)},
		}
	}
	serviceForIngress := &coreV1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "east.aws.lb",
			Namespace: "istio-system",
			Labels:    map[string]string{"app": "gatewayapp"},
		},
		Spec: coreV1.ServiceSpec{
			Ports: []coreV1.ServicePort{
				{
					Name: "http",
					Port: 8090,
				},
			},
		},
		Status: coreV1.ServiceStatus{
			LoadBalancer: coreV1.LoadBalancerStatus{
				Ingress: []coreV1.LoadBalancerIngress{
					{
						Hostname: "east.aws.lb",
					},
				},
			},
		},
	}
	serviceEntryAddressStore := &ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses:      []string{},
	}
	cacheController := &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(serviceEntryAddressStore, "123"),
	}

	rr := NewRemoteRegistry(nil, admiralParams)
	rr.AdmiralCache.ConfigMapController = cacheController
	rr.AdmiralCache.IdentityClusterCache.Put(common.GetDeploymentGlobalIdentifier(dependentInSourceCluster), "source-cluster-k8s", "source-cluster-k8s")
	rr.AdmiralCache.IdentityClusterCache.Put(common.GetDeploymentOriginalIdentifier(gwAsADependentInGWCluster), gwClusterID, gwClusterID)

	rr.AdmiralCache.IdentityClusterNamespaceCache.Put(common.GetDeploymentGlobalIdentifier(dependentInSourceCluster), "source-cluster-k8s", "dependentinsourcecluster-ns", "dependentinsourcecluster-ns")
	rr.AdmiralCache.IdentityDependencyCache.Put("partition.partitionedrolloutidentity", "dependentinsourceclusteridentity", "dependentinsourceclusteridentity")
	rr.AdmiralCache.IdentityDependencyCache.Put("partition.partitionedrolloutidentity", "dependentinremoteclusteridentity", "dependentinremoteclusteridentity")
	rr.AdmiralCache.IdentityDependencyCache.Put("partition.partitionedrolloutidentity", "intuit.platform.servicesgateway.servicesgateway", "intuit.platform.servicesgateway.servicesgateway")
	serviceController.Cache.Put(dependentInSourceClusterSvc)
	gwserviceController.Cache.Put(gwAsADependentInGWClusterSvc)
	deploymentController.Cache.UpdateDeploymentToClusterCache("dependentinsourceclusteridentity", dependentInSourceCluster)
	gwdeploymentController.Cache.UpdateDeploymentToClusterCache("intuit.platform.servicesgateway.servicesgateway", gwAsADependentInGWCluster)
	rr.AdmiralCache.IdentityClusterCache.Put(common.GetRolloutGlobalIdentifier(&partitionedRollout), "source-cluster-k8s", "source-cluster-k8s")
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put(common.GetRolloutGlobalIdentifier(&partitionedRollout), "source-cluster-k8s", "partitionedrollout-ns", "partitionedrollout-ns")
	rr.AdmiralCache.PartitionIdentityCache.Put("partition.partitionedrolloutidentity", "partitionedrolloutidentity")
	rolloutController.Cache.UpdateRolloutToClusterCache("partition.partitionedrolloutidentity", &partitionedRollout)
	serviceController.Cache.Put(partitionedRolloutSvc)
	serviceController.Cache.Put(dependentInSourceClusterSvc)
	serviceController.Cache.Put(serviceForIngress)
	gwserviceController.Cache.Put(gwAsADependentInGWClusterSvc)
	rolloutController.Cache.UpdateRolloutToClusterCache("partition.partitionedrolloutidentity", &partitionedRollout)
	serviceController.Cache.Put(serviceForIngress)
	rr.PutRemoteController("source-cluster-k8s", sourceRc)
	rr.PutRemoteController(gwClusterID, gwRc)
	existingSourceClusterSE := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:    []string{"test.partitionedrolloutidentity.mesh"},
		ExportTo: []string{"dummy-ns"},
	}

	existingSourceClusterSEv1 := createServiceEntrySkeleton(*existingSourceClusterSE, "test.partitionedrolloutidentity.mesh-se", common.GetSyncNamespace())
	existingSourceClusterSEv1.Annotations = map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue}
	sourceRc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries("ns").Create(context.Background(), existingSourceClusterSEv1, metav1.CreateOptions{})
	sourceRc.ServiceEntryController.Cache.Put(existingSourceClusterSEv1, "source-cluster-k8s")

	existingRemoteClusterSE := make([]*istioNetworkingV1Alpha3.ServiceEntry, remoteClusters)
	existingRemoteClusterDR := make([]*istioNetworkingV1Alpha3.DestinationRule, remoteClusters)
	//existingRemoteClusterVS := make([]*istioNetworkingV1Alpha3.VirtualService, remoteClusters)
	expectedRemoteClusterSE := make([]*istioNetworkingV1Alpha3.ServiceEntry, remoteClusters)
	expectedRemoteClusterDR := make([]*istioNetworkingV1Alpha3.DestinationRule, remoteClusters)
	expectedRemoteClusterVS := make([]*istioNetworkingV1Alpha3.VirtualService, remoteClusters)
	for i := 1; i <= remoteClusters; i++ {
		rr.AdmiralCache.IdentityClusterCache.Put(common.GetRolloutGlobalIdentifier(&dependentInRemoteClusters[i-1]), clusterIDs[i-1], clusterIDs[i-1])
		rr.AdmiralCache.IdentityClusterNamespaceCache.Put(common.GetRolloutGlobalIdentifier(&dependentInRemoteClusters[i-1]), clusterIDs[i-1], fmt.Sprintf("dependentinremotecluster-ns-%d", i-1), fmt.Sprintf("dependentinremotecluster-ns-%d", i-1))
		remoteRolloutController[i-1].Cache.UpdateRolloutToClusterCache("dependentinremoteclusteridentity", &dependentInRemoteClusters[i-1])
		remoteServiceController[i-1].Cache.Put(dependentInRemoteClustersSvc[i-1])
		rr.AdmiralCache.IdentityClusterCache.Put(common.GetRolloutGlobalIdentifier(&dependentInRemoteClusters[i-1]), clusterIDs[i-1], clusterIDs[i-1])
		remoteRolloutController[i-1].Cache.UpdateRolloutToClusterCache("dependentinremoteclusteridentity", &dependentInRemoteClusters[i-1])
		remoteServiceController[i-1].Cache.Put(dependentInRemoteClustersSvc[i-1])
		rr.PutRemoteController(clusterIDs[i-1], remoteControllers[i-1])

		existingRemoteClusterSE[i-1] = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:    []string{"test.partitionedrolloutidentity.mesh"},
			ExportTo: []string{"fake-ns"},
		}
		existingRemoteClusterSEv1 := createServiceEntrySkeleton(*existingRemoteClusterSE[i-1], fmt.Sprintf("test.partitionedrolloutidentity.mesh-se"), common.GetSyncNamespace())
		existingRemoteClusterSEv1.Annotations = map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue}
		remoteControllers[i-1].ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries("ns").Create(context.Background(), existingRemoteClusterSEv1, metav1.CreateOptions{})
		remoteControllers[i-1].ServiceEntryController.Cache.Put(existingRemoteClusterSEv1, clusterIDs[i-1])

		existingRemoteClusterDR[i-1] = &istioNetworkingV1Alpha3.DestinationRule{
			Host:     "test.partitionedrolloutidentity.mesh",
			ExportTo: []string{"fake-ns"},
		}
		existingRemoteClusterDRv1 := createDestinationRuleSkeleton(*existingRemoteClusterDR[i-1], fmt.Sprintf("test.partitionedrolloutidentity.mesh-default-dr"), common.GetSyncNamespace())
		existingRemoteClusterDRv1.Annotations = map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue}
		remoteControllers[i-1].DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules("ns").Create(context.Background(), existingRemoteClusterDRv1, metav1.CreateOptions{})
		remoteControllers[i-1].DestinationRuleController.Cache.Put(existingRemoteClusterDRv1)

		/*existingRemoteClusterVS[i-1] = &istioNetworkingV1Alpha3.VirtualService{
			Hosts:    []string{"test.partitionedrolloutidentity.intuit"},
			ExportTo: []string{"fake-ns"},
		}
		existingRemoteClusterVSv1 := createVirtualServiceSkeleton(*existingRemoteClusterVS[i-1], fmt.Sprintf("test.partitionedrolloutidentity.intuit-vs"), common.GetSyncNamespace())
		existingRemoteClusterVSv1.Annotations = map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue}
		remoteControllers[i-1].VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices("ns").Create(context.Background(), existingRemoteClusterVSv1, metav1.CreateOptions{})*/

		expectedRemoteClusterSE[i-1] = &istioNetworkingV1Alpha3.ServiceEntry{
			Hosts:    []string{fmt.Sprintf("test.partitionedrolloutidentity.mesh")},
			ExportTo: []string{"dependentinremotecluster-ns-" + fmt.Sprintf("%d", i-1)},
		}
		expectedRemoteClusterDR[i-1] = &istioNetworkingV1Alpha3.DestinationRule{
			Host:     fmt.Sprintf("test.partitionedrolloutidentity.mesh"),
			ExportTo: []string{"dependentinremotecluster-ns-" + fmt.Sprintf("%d", i-1)},
		}
		expectedRemoteClusterVS[i-1] = &istioNetworkingV1Alpha3.VirtualService{
			Hosts:    []string{fmt.Sprintf("test.partitionedrolloutidentity.intuit")},
			ExportTo: []string{"dependentinremotecluster-ns-" + fmt.Sprintf("%d", i-1)},
		}
	}

	existingGWSE := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:    []string{"test.partitionedrolloutidentity.mesh"},
		ExportTo: []string{"fake-ns"},
	}
	existingGWSEv1 := createServiceEntrySkeleton(*existingGWSE, fmt.Sprintf("test.partitionedrolloutidentity.mesh-se"), common.GetSyncNamespace())
	existingGWSEv1.Annotations = map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue}
	gwRc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries("ns").Create(context.Background(), existingGWSEv1, metav1.CreateOptions{})
	gwRc.ServiceEntryController.Cache.Put(existingGWSEv1, gwClusterID)

	existingGWDR := &istioNetworkingV1Alpha3.DestinationRule{
		Host:     "test.partitionedrolloutidentity.mesh",
		ExportTo: []string{"fake-ns"},
	}
	existingGWDRv1 := createDestinationRuleSkeleton(*existingGWDR, fmt.Sprintf("test.partitionedrolloutidentity.mesh-default-dr"), common.GetSyncNamespace())
	existingGWDRv1.Annotations = map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue}
	gwRc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules("ns").Create(context.Background(), existingGWDRv1, metav1.CreateOptions{})
	gwRc.DestinationRuleController.Cache.Put(existingGWDRv1)

	rr.StartTime = time.Now().Add(-1 * common.GetAdmiralParams().CacheReconcileDuration)

	testCases := []struct {
		name                                     string
		assetIdentity                            string
		expectedSourceServiceEntries             map[string]*istioNetworkingV1Alpha3.ServiceEntry
		expectedSourceDestinationRules           map[string]*istioNetworkingV1Alpha3.DestinationRule
		expectedSourceVirtualServices            map[string]*istioNetworkingV1Alpha3.VirtualService
		expectedRemoteServiceEntryInGWCluster    *istioNetworkingV1Alpha3.ServiceEntry
		expectedRemoteDestinationRuleInGWCluster *istioNetworkingV1Alpha3.DestinationRule
		expectedRemoteVirtualServiceInGWCluster  *istioNetworkingV1Alpha3.VirtualService
		eventResourceType                        string
	}{
		{
			name: "Given a SE is getting updated due to a Rollout, " +
				"And partition awareness feature is enabled, " +
				"Then the SE ExportTo field contains the dependent service namespaces for the appropriate cluster",
			assetIdentity: "partition.partitionedrolloutidentity",
			expectedSourceServiceEntries: map[string]*istioNetworkingV1Alpha3.ServiceEntry{
				"test.partitionedrolloutidentity.mesh": sourceClusterSE,
			},
			expectedSourceDestinationRules: map[string]*istioNetworkingV1Alpha3.DestinationRule{
				"test.partitionedrolloutidentity.mesh": sourceClusterDR,
			},
			expectedSourceVirtualServices: map[string]*istioNetworkingV1Alpha3.VirtualService{
				"test.partitionedrolloutidentity.intuit": sourceClusterVS,
			},
			expectedRemoteServiceEntryInGWCluster:    gwRemoteClusterSE,
			expectedRemoteDestinationRuleInGWCluster: gwRemoteClusterDR,
			expectedRemoteVirtualServiceInGWCluster:  gwRemoteClusterVS,
			eventResourceType:                        common.Rollout,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = context.WithValue(ctx, "clusterName", "source-cluster-k8s")
			ctx = context.WithValue(ctx, "eventResourceType", c.eventResourceType)

			_, err = modifyServiceEntryForNewServiceOrPod(ctx, admiral.Add, env, c.assetIdentity, rr)

			// Validating SEs
			for _, expectedServiceEntry := range c.expectedSourceServiceEntries {
				seName := getIstioResourceName(expectedServiceEntry.Hosts[0], "-se")
				createdSe, err := sourceIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Get(ctx, seName, metav1.GetOptions{})
				if err != nil || createdSe == nil {
					t.Errorf("expected the service entry %s but it wasn't found", seName)
				} else if !reflect.DeepEqual(createdSe.Spec.ExportTo, expectedServiceEntry.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for sourceSE", expectedServiceEntry.ExportTo, createdSe.Spec.ExportTo)
				}
			}
			var clientCount = 1
			for _, expectedServiceEntry := range expectedRemoteClusterSE {
				seName := getIstioResourceName(expectedServiceEntry.Hosts[0], "-se")
				createdSe, err := remoteIstioClient[clientCount-1].NetworkingV1alpha3().ServiceEntries("ns").Get(ctx, seName, metav1.GetOptions{})
				if err != nil || createdSe == nil {
					t.Errorf("expected the service entry %s but it wasn't found", seName)
				} else if !reflect.DeepEqual(createdSe.Spec.ExportTo, expectedServiceEntry.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for remoteSE", expectedServiceEntry.ExportTo, createdSe.Spec.ExportTo)
				}
				clientCount++
			}
			// For GW cluster - Empty ExportTo expected
			seName := getIstioResourceName(gwRemoteClusterSE.Hosts[0], "-se")
			createdSe, err := gwIstioClient.NetworkingV1alpha3().ServiceEntries("ns").Get(ctx, seName, metav1.GetOptions{})
			if err != nil || createdSe == nil {
				t.Errorf("expected the service entry %s but it wasn't found", seName)
			} else if !reflect.DeepEqual(createdSe.Spec.ExportTo, c.expectedRemoteServiceEntryInGWCluster.ExportTo) {
				t.Errorf("expected exportTo of %v but got %v for remoteSEInGWCluster", gwRemoteClusterSE.ExportTo, createdSe.Spec.ExportTo)
			}

			// Validating DRs
			for _, expectedDestinationRule := range c.expectedSourceDestinationRules {
				drName := getIstioResourceName(expectedDestinationRule.Host, "-default-dr")
				createdDr, err := sourceIstioClient.NetworkingV1alpha3().DestinationRules("ns").Get(ctx, drName, metav1.GetOptions{})
				if err != nil || createdDr == nil {
					t.Errorf("expected the destination rule %s but it wasn't found", drName)
				} else if !reflect.DeepEqual(createdDr.Spec.ExportTo, expectedDestinationRule.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for sourceDR", expectedDestinationRule.ExportTo, createdDr.Spec.ExportTo)
				}
			}
			clientCount = 1
			for _, expectedDestinationRule := range expectedRemoteClusterDR {
				drName := getIstioResourceName(expectedDestinationRule.Host, "-default-dr")
				createdDr, err := remoteIstioClient[clientCount-1].NetworkingV1alpha3().DestinationRules("ns").Get(ctx, drName, metav1.GetOptions{})
				if err != nil || createdDr == nil {
					t.Errorf("expected the service entry %s but it wasn't found", drName)
				} else if !reflect.DeepEqual(createdDr.Spec.ExportTo, expectedDestinationRule.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for remoteDR", expectedDestinationRule.ExportTo, createdDr.Spec.ExportTo)
				}
				clientCount++
			}
			// For GW cluster - Empty ExportTo expected
			drName := getIstioResourceName(gwRemoteClusterDR.Host, "-default-dr")
			createdDr, err := gwIstioClient.NetworkingV1alpha3().DestinationRules("ns").Get(ctx, drName, metav1.GetOptions{})
			if err != nil || createdDr == nil {
				t.Errorf("expected the destination rule %s but it wasn't found", drName)
			} else if !reflect.DeepEqual(createdDr.Spec.ExportTo, c.expectedRemoteServiceEntryInGWCluster.ExportTo) {
				t.Errorf("expected exportTo of %v but got %v for remoteDRInGWCluster", gwRemoteClusterDR.ExportTo, createdDr.Spec.ExportTo)
			}

			// Validating VSs
			for _, expectedVirtualService := range c.expectedSourceVirtualServices {
				vsName := getIstioResourceName(expectedVirtualService.Hosts[0], "-vs")
				createdVs, err := sourceIstioClient.NetworkingV1alpha3().VirtualServices("ns").Get(ctx, vsName, metav1.GetOptions{})
				if err != nil || createdVs == nil {
					t.Errorf("expected the virtual service %s but it wasn't found", vsName)
				} else if !reflect.DeepEqual(createdVs.Spec.ExportTo, expectedVirtualService.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for sourceVS", expectedVirtualService.ExportTo, createdVs.Spec.ExportTo)
				}
			}
			clientCount = 1
			for _, expectedVirtualService := range expectedRemoteClusterVS {
				vsName := getIstioResourceName(expectedVirtualService.Hosts[0], "-vs")
				createdVs, err := remoteIstioClient[clientCount-1].NetworkingV1alpha3().VirtualServices("ns").Get(ctx, vsName, metav1.GetOptions{})
				if err != nil || createdVs == nil {
					t.Errorf("expected the virtual service %s but it wasn't found", vsName)
				} else if !reflect.DeepEqual(createdVs.Spec.ExportTo, expectedVirtualService.ExportTo) {
					t.Errorf("expected exportTo of %v but got %v for remoteVS", expectedVirtualService.ExportTo, createdVs.Spec.ExportTo)
				}
				clientCount++
			}
			// For GW cluster - Empty ExportTo expected
			vsName := getIstioResourceName(gwRemoteClusterVS.Hosts[0], "-vs")
			createdVs, err := gwIstioClient.NetworkingV1alpha3().VirtualServices("ns").Get(ctx, vsName, metav1.GetOptions{})
			if err != nil || createdVs == nil {
				t.Errorf("expected the virtual service %s but it wasn't found", vsName)
			} else if !reflect.DeepEqual(createdVs.Spec.ExportTo, c.expectedRemoteVirtualServiceInGWCluster.ExportTo) {
				t.Errorf("expected exportTo of %v but got %v for remoteVSInGWCluster", gwRemoteClusterVS.ExportTo, createdVs.Spec.ExportTo)
			}
		})
	}
}

func TestGetClusters(t *testing.T) {
	admiralParams := admiralParamsForServiceEntryTests()
	admiralParams.AdmiralOperatorMode = true
	admiralParams.OperatorSecretFilterTags = "admiral/syncoperator"
	admiralParams.SecretFilterTags = "admiral/sync"
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	rr, _ := InitAdmiral(context.Background(), admiralParams)
	expectedgwClusterMap := common.NewMap()
	expectedgwClusterMap.Put("cluster1", "cluster1")
	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})
	testCases := []struct {
		name        string
		dependent   string
		expectedMap *common.Map
	}{
		{
			name: "Given that dependent is a valid identity, " +
				"When we call getClusters on it, " +
				"Then we get a map with the clusters it is deployed on",
			dependent:   "sample",
			expectedMap: expectedgwClusterMap,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gwClusterMap := getClusters(rr, c.dependent, ctxLogger)
			if !reflect.DeepEqual(gwClusterMap, c.expectedMap) {
				t.Errorf("got=%+v, want %+v", gwClusterMap, c.expectedMap)
			}
		})
	}
}

func TestValidateLocalityInServiceEntry(t *testing.T) {
	testCases := []struct {
		name        string
		entry       *v1alpha3.ServiceEntry
		expected    bool
		expectedErr interface{}
	}{
		{
			"AllEndpointsWithLocality",
			&v1alpha3.ServiceEntry{
				Spec: istioNetworkingV1Alpha3.ServiceEntry{
					Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
						{Locality: "us-west-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
						{Locality: "us-east-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
					},
				},
			},
			true,
			nil,
		},
		{
			"NoEndpoints",
			&v1alpha3.ServiceEntry{
				Spec: istioNetworkingV1Alpha3.ServiceEntry{
					Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{},
				},
			},
			true,
			nil,
		},
		{
			"SingleEndpointLocalitySet",
			&v1alpha3.ServiceEntry{
				Spec: istioNetworkingV1Alpha3.ServiceEntry{
					Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
						{Locality: "us-west-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
					},
				},
			},
			true,
			nil,
		},
		{
			"SomeEndpointsMissingLocality",
			&v1alpha3.ServiceEntry{
				Spec: istioNetworkingV1Alpha3.ServiceEntry{
					Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
						{Locality: "us-west-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
						{Address: "abc.foo.com.", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
					},
				},
			},
			false,
			[]string{"locality not set for endpoint with address abc.foo.com."},
		},
		{
			"AllEndpointsWithoutLocalityAndMode",
			&v1alpha3.ServiceEntry{
				Spec: istioNetworkingV1Alpha3.ServiceEntry{
					Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
						{Address: "abc.foo.com."},
						{Address: "def.foo.com."},
					},
				},
			},
			false,
			[]string{"locality not set for endpoint with address abc.foo.com.", "istio mode not set for endpoint with address abc.foo.com.", "locality not set for endpoint with address def.foo.com.", "istio mode not set for endpoint with address def.foo.com."},
		},
		{
			"AllEndpointsWithLocalityWithoutIstioModeLabel",
			&v1alpha3.ServiceEntry{
				Spec: istioNetworkingV1Alpha3.ServiceEntry{
					Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
						{Address: "abc.foo.com.", Locality: "us-west-2"},
						{Address: "def.foo.com.", Locality: "us-east-2"},
					},
				},
			},
			false,
			[]string{"istio mode not set for endpoint with address abc.foo.com.", "istio mode not set for endpoint with address def.foo.com."},
		},
		{
			"AllEndpointsWithLocalityWithPartiallyIstioModeLabel",
			&v1alpha3.ServiceEntry{
				Spec: istioNetworkingV1Alpha3.ServiceEntry{
					Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
						{Address: "abc.foo.com.", Locality: "us-west-2", Labels: map[string]string{"type": common.Rollout, "security.istio.io/tlsMode": "istio"}},
						{Address: "def.foo.com.", Locality: "us-east-2"},
					},
				},
			},
			false,
			[]string{"istio mode not set for endpoint with address def.foo.com."},
		},
		{
			"AllEndpointsWithLocalityWithIstioModeLabel",
			&v1alpha3.ServiceEntry{
				Spec: istioNetworkingV1Alpha3.ServiceEntry{
					Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
						{Address: "abc.foo.com.", Locality: "us-west-2", Labels: map[string]string{"type": common.Rollout, "security.istio.io/tlsMode": "istio"}},
						{Address: "def.foo.com.", Locality: "us-east-2", Labels: map[string]string{"type": common.Rollout, "security.istio.io/tlsMode": "istio"}},
					},
				},
			},
			true,
			nil,
		},
	}

	for _, tt := range testCases {
		result, err := validateServiceEntryEndpoints(tt.entry)
		if result != tt.expected {
			t.Errorf("Test failed: %s \nExpected: %v \nGot: %v", tt.name, tt.expected, result)
		}
		if tt.expectedErr == nil {
			assert.Nil(t, err)
		} else {
			for i, expectedErr := range tt.expectedErr.([]string) {
				assert.Contains(t, err.Error(), expectedErr, "Error %d: %s", i, expectedErr)
			}
		}
	}
}

func TestOrderSourceClusters(t *testing.T) {
	rc1 := &RemoteController{
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-east-2",
			},
		},
	}
	rc2 := &RemoteController{
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-west-2",
			},
		},
	}
	rc3 := &RemoteController{
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-apse-2",
			},
		},
	}

	tests := []struct {
		desc       string
		gtpPrefReg string
		rcMap      map[string]*RemoteController
		services   map[string]map[string]*v1.Service
		enabled    bool
		expected   string
	}{
		{
			desc:       "Empty services",
			gtpPrefReg: "",
			rcMap:      map[string]*RemoteController{},
			services:   map[string]map[string]*v1.Service{},
			expected:   "",
		},
		{
			desc:       "Cluster in gtp preference region",
			gtpPrefReg: "us-west-2",
			rcMap:      map[string]*RemoteController{"cluster2": rc2, "cluster1": rc1, "cluster3": rc3},
			services:   map[string]map[string]*v1.Service{"cluster1": {}, "cluster2": {}, "cluster3": {}},
			enabled:    true,
			expected:   "cluster2",
		},
	}

	for _, tC := range tests {
		t.Run(tC.desc, func(t *testing.T) {
			common.ResetSync()
			rr, _ := InitAdmiral(context.Background(), common.AdmiralParams{
				KubeconfigPath: "testdata/fake.config",
				LabelSet: &common.LabelSet{
					GatewayApp:              "gatewayapp",
					WorkloadIdentityKey:     "identity",
					PriorityKey:             "priority",
					EnvKey:                  "env",
					AdmiralCRDIdentityLabel: "identity",
				},
				EnableSAN:                         true,
				SANPrefix:                         "prefix",
				HostnameSuffix:                    "mesh",
				SyncNamespace:                     "ns",
				CacheReconcileDuration:            0,
				ClusterRegistriesNamespace:        "default",
				DependenciesNamespace:             "default",
				WorkloadSidecarName:               "default",
				Profile:                           common.AdmiralProfileDefault,
				DependentClusterWorkerConcurrency: 5,
				PreventSplitBrain:                 tC.enabled,
			})
			for c, rc := range tC.rcMap {
				rr.PutRemoteController(c, rc)
			}
			ctx := context.WithValue(context.Background(), common.GtpPreferenceRegion, tC.gtpPrefReg)
			result := orderSourceClusters(ctx, rr, tC.services)

			if tC.expected == "" {
				assert.Equal(t, 0, len(result))
			} else if !reflect.DeepEqual(result[0], tC.expected) {
				t.Fatalf("Expected %v, but got %v", tC.expected, result)
			}
		})
	}
}

func Test_getOverwrittenLoadBalancer(t *testing.T) {
	type args struct {
		ctx          *logrus.Entry
		rc           *RemoteController
		clusterName  string
		admiralCache *AdmiralCache
	}

	testArg := args{
		ctx:          logrus.New().WithContext(context.Background()),
		rc:           &RemoteController{},
		clusterName:  "cluster1",
		admiralCache: &AdmiralCache{NLBEnabledCluster: []string{"test-cluster1"}},
	}

	testArg1 := args{
		ctx:          logrus.New().WithContext(context.Background()),
		rc:           &RemoteController{},
		clusterName:  "cluster1",
		admiralCache: &AdmiralCache{NLBEnabledCluster: []string{"cluster1"}},
	}

	testArg2 := args{
		ctx:          logrus.New().WithContext(context.Background()),
		rc:           &RemoteController{},
		clusterName:  "cluster1",
		admiralCache: &AdmiralCache{NLBEnabledCluster: []string{"cluster1"}},
	}

	stop := make(chan struct{})
	config := rest.Config{
		Host: "localhost",
	}

	testAdmiralParam := common.GetAdmiralParams()
	testAdmiralParam.LabelSet.GatewayApp = common.IstioIngressGatewayLabelValue
	testAdmiralParam.NLBIngressLabel = common.NLBIstioIngressGatewayLabelValue

	common.UpdateAdmiralParams(testAdmiralParam)

	testService := v1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "clb",
			Namespace:         common.NamespaceIstioSystem,
			Generation:        0,
			CreationTimestamp: metav1.Time{},
			Labels:            map[string]string{common.App: common.IstioIngressGatewayLabelValue},
		},
		Spec: v1.ServiceSpec{},
		Status: v1.ServiceStatus{
			LoadBalancer: v1.LoadBalancerStatus{Ingress: make([]v1.LoadBalancerIngress, 0)},
			Conditions:   nil,
		},
	}

	portStatus := v1.PortStatus{
		Port:     007,
		Protocol: "HTTP",
		Error:    nil,
	}

	testLoadBalancerIngress := v1.LoadBalancerIngress{
		IP:       "007.007.007.007",
		Hostname: "clb.istio.com",
		IPMode:   nil,
		Ports:    make([]v1.PortStatus, 0),
	}
	testLoadBalancerIngress.Ports = append(testLoadBalancerIngress.Ports, portStatus)
	testService.Status.LoadBalancer.Ingress = append(testService.Status.LoadBalancer.Ingress, testLoadBalancerIngress)

	testArg.rc.ServiceController, _ = admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	testArg.rc.ServiceController.Cache.Put(&testService)

	testService1 := testService.DeepCopy()
	testService1.Name = "nlb"
	testService1.Labels[common.App] = common.NLBIstioIngressGatewayLabelValue
	testService1.Status.LoadBalancer.Ingress[0].Hostname = "nlb.istio.com"

	testArg1.rc.ServiceController, _ = admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	testArg1.rc.ServiceController.Cache.Put(&testService)
	testArg1.rc.ServiceController.Cache.Put(testService1)

	testService2 := testService1.DeepCopy()
	testService2.Labels[common.App] = common.NLBIstioIngressGatewayLabelValue + "TEST"

	testArg2.rc.ServiceController, _ = admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	testArg2.rc.ServiceController.Cache.Put(&testService)
	testArg2.rc.ServiceController.Cache.Put(testService2)

	tests := []struct {
		name             string
		args             args
		expectedEndpoint string
		expectedPort     int
	}{
		{"When NLB Cluster Overwrite is not then getOverwrittenLoadBalancer should return CLB value", testArg, "clb.istio.com", 15443},
		{"When NLB Cluster Overwrite is set then getOverwrittenLoadBalancer should return NLB value", testArg1, "nlb.istio.com", 15443},
		{"When NLB Cluster Overwrite is set but NLB is not present then getOverwrittenLoadBalancer should return CLB value", testArg2, "clb.istio.com", 15443},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualEndpoint, actualPort, _ := getOverwrittenLoadBalancer(tt.args.ctx, tt.args.rc, tt.args.clusterName, tt.args.admiralCache, "")
			assert.Equalf(t, tt.expectedEndpoint, actualEndpoint, fmt.Sprintf("getOverwrittenLoadBalancer should return endpoint %s, but got %s", tt.expectedEndpoint, actualEndpoint))
			assert.Equalf(t, tt.expectedPort, actualPort, fmt.Sprintf("getOverwrittenLoadBalancer should return port %d, but got %d", tt.expectedPort, actualPort))
		})
	}
}

func TestGetClusterIngressGateway(t *testing.T) {

	ctxLogger := logrus.New().WithContext(context.Background())
	stop := make(chan struct{})
	config := rest.Config{Host: "localhost"}

	serviceController, _ := admiral.NewServiceController(
		stop, &test.MockServiceHandler{}, &config, time.Second*1, loader.GetFakeClientLoader())

	invalidServiceController, _ := admiral.NewServiceController(
		stop, &test.MockServiceHandler{}, &config, time.Second*1, loader.GetFakeClientLoader())
	invalidServiceController.Cache = nil

	testCases := []struct {
		name            string
		rc              *RemoteController
		labelSet        *common.LabelSet
		expectedIngress string
		expectedError   error
	}{
		{
			name: "Given nil remoteController" +
				"When getClusterIngress func is called" +
				"Then it should return an error",
			rc:            nil,
			expectedError: fmt.Errorf("remote controller not initialized"),
		},
		{
			name: "Given nil ServiceController" +
				"When getClusterIngress func is called" +
				"Then it should return an error",
			rc:            &RemoteController{},
			expectedError: fmt.Errorf("service controller not initialized"),
		},
		{
			name: "Given nil ServiceControllerCache" +
				"When getClusterIngress func is called" +
				"Then it should return an error",
			rc: &RemoteController{
				ServiceController: invalidServiceController,
			},
			expectedError: fmt.Errorf("service controller cache not initialized"),
		},
		{
			name: "Given all valid params" +
				"When getClusterIngress func is called" +
				"Then it should return a valid cluster ingress and no errors",
			rc: &RemoteController{
				ServiceController: serviceController,
			},
			labelSet:        &common.LabelSet{GatewayApp: "gatewayapp"},
			expectedError:   nil,
			expectedIngress: "dummy.admiral.global",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, _, err := getOverwrittenLoadBalancer(ctxLogger, tc.rc, "TEST_CLUSTER", &AdmiralCache{}, "")
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedIngress, actual)
			}
		})
	}

}

func Test_getOverwrittenLoadBalancerSourceOverwrite(t *testing.T) {
	type args struct {
		ctx            *logrus.Entry
		rc             *RemoteController
		clusterName    string
		admiralCache   *AdmiralCache
		sourceIdentity string
		lbLabel        string
	}

	testAdmiralParam := common.GetAdmiralParams()
	testAdmiralParam.NLBEnabledIdentityList = append(testAdmiralParam.NLBEnabledIdentityList, "intuit.source.asset.match")
	testAdmiralParam.NLBIngressLabel = common.NLBIstioIngressGatewayLabelValue
	common.UpdateAdmiralParams(testAdmiralParam)

	//Before
	beforeDefaultLB := common.GetAdmiralParams().LabelSet.GatewayApp

	noSourceIdentityOnlyCLBFound := args{
		ctx:            logrus.New().WithContext(context.Background()),
		rc:             &RemoteController{},
		clusterName:    "test-cluster",
		admiralCache:   &AdmiralCache{NLBEnabledCluster: []string{"test-cluster1"}},
		sourceIdentity: "",
		lbLabel:        common.IstioIngressGatewayLabelValue,
	}

	testLoadBalancerIngressArgCLB := v1.LoadBalancerIngress{
		IP:       "007.007.007.007",
		Hostname: "clb.istio.com",
		IPMode:   nil,
		Ports:    make([]v1.PortStatus, 0),
	}

	testLoadBalancerIngressArgNLB := v1.LoadBalancerIngress{
		IP:       "007.007.007.007",
		Hostname: "nlb.istio.com",
		IPMode:   nil,
		Ports:    make([]v1.PortStatus, 0),
	}

	portStatus := v1.PortStatus{
		Port:     007,
		Protocol: "HTTP",
		Error:    nil,
	}

	testServiceCLB := v1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "clb",
			Namespace:         common.NamespaceIstioSystem,
			Generation:        0,
			CreationTimestamp: metav1.Time{},
			Labels:            map[string]string{common.App: common.IstioIngressGatewayLabelValue},
		},
		Spec: v1.ServiceSpec{},
		Status: v1.ServiceStatus{
			LoadBalancer: v1.LoadBalancerStatus{Ingress: make([]v1.LoadBalancerIngress, 0)},
			Conditions:   nil,
		},
	}

	testServiceNLB := v1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nlb",
			Namespace:         common.NamespaceIstioSystem,
			Generation:        0,
			CreationTimestamp: metav1.Time{},
			Labels:            map[string]string{common.App: common.NLBIstioIngressGatewayLabelValue},
		},
		Spec: v1.ServiceSpec{},
		Status: v1.ServiceStatus{
			LoadBalancer: v1.LoadBalancerStatus{Ingress: make([]v1.LoadBalancerIngress, 0)},
			Conditions:   nil,
		},
	}

	config := rest.Config{
		Host: "localhost",
	}

	testLoadBalancerIngressArgCLB.Ports = append(testLoadBalancerIngressArgCLB.Ports, portStatus)
	testServiceCLB.Status.LoadBalancer.Ingress = append(testServiceCLB.Status.LoadBalancer.Ingress, testLoadBalancerIngressArgCLB)

	noSourceIdentityOnlyCLBFound.rc.ServiceController, _ = admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	noSourceIdentityOnlyCLBFound.rc.ServiceController.Cache.Put(&testServiceCLB)

	sourceIdentityOnlyCLBFound := noSourceIdentityOnlyCLBFound
	sourceIdentityOnlyCLBFound.sourceIdentity = "intuit.source.asset.match"

	sourceIdentityBothLBFound := args{
		ctx:            logrus.New().WithContext(context.Background()),
		rc:             &RemoteController{},
		clusterName:    "test-cluster",
		admiralCache:   &AdmiralCache{NLBEnabledCluster: []string{"test-cluster1"}},
		sourceIdentity: "",
		lbLabel:        common.IstioIngressGatewayLabelValue,
	}

	sourceIdentityBothLBFound.sourceIdentity = "intuit.source.asset.match"
	sourceIdentityBothLBFound.rc.ServiceController, _ = admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	testServiceNLB.Status.LoadBalancer.Ingress = append(testServiceNLB.Status.LoadBalancer.Ingress, testLoadBalancerIngressArgNLB)
	noSourceIdentityOnlyCLBFound.rc.ServiceController.Cache.Put(&testServiceCLB)
	sourceIdentityBothLBFound.rc.ServiceController.Cache.Put(&testServiceNLB)

	tests := []struct {
		name     string
		args     args
		wantLB   string
		wantPort int
		//wantErr  assert.ErrorAssertionFunc
	}{
		{"No Source Asset overwrite", noSourceIdentityOnlyCLBFound, "clb.istio.com", 15443},
		{"Source Asset Overwrite with Only CLB found", sourceIdentityOnlyCLBFound, "clb.istio.com", 15443},
		{"Source Asset Overwrite with Both LB Found", sourceIdentityBothLBFound, "nlb.istio.com", 15443},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			common.GetAdmiralParams().LabelSet.GatewayApp = tt.args.lbLabel
			gotLB, gotPort, _ := getOverwrittenLoadBalancer(tt.args.ctx, tt.args.rc, tt.args.clusterName, tt.args.admiralCache, tt.args.sourceIdentity)
			assert.Equalf(t, tt.wantLB, gotLB, "getOverwrittenLoadBalancer(%v, %v, %v, %v, %v)", tt.args.ctx, tt.args.rc, tt.args.clusterName, tt.args.admiralCache, tt.args.sourceIdentity)
			assert.Equalf(t, tt.wantPort, gotPort, "getOverwrittenLoadBalancer(%v, %v, %v, %v, %v)", tt.args.ctx, tt.args.rc, tt.args.clusterName, tt.args.admiralCache, tt.args.sourceIdentity)
		})
	}

	//restore
	common.GetAdmiralParams().LabelSet.GatewayApp = beforeDefaultLB
}

func TestIsCartographerVSDisabled(t *testing.T) {

	testCases := []struct {
		name                    string
		rc                      *RemoteController
		env                     string
		identity                string
		getCustomVirtualService GetCustomVirtualService
		expectedResult          bool
		exptectedError          error
	}{
		{
			name: "Given a nil remotecontroller" +
				"When func DoDRPinning is called" +
				"Then the func should return an error",
			exptectedError: fmt.Errorf("remoteController is nil"),
		},
		{
			name: "Given empty env" +
				"When func DoDRPinning is called" +
				"Then the func should return an error",
			rc:             &RemoteController{},
			exptectedError: fmt.Errorf("env is empty"),
		},
		{
			name: "Given empty identity" +
				"When func DoDRPinning is called" +
				"Then the func should return an error",
			rc:             &RemoteController{},
			env:            "stage",
			exptectedError: fmt.Errorf("identity is empty"),
		},
		{
			name: "Given valid params" +
				"When getCustomVirtualService returns an error" +
				"And func DoDRPinning is called" +
				"Then the func should return an error",
			rc:       &RemoteController{},
			env:      "stage",
			identity: "testIdentity",
			getCustomVirtualService: func(
				ctx context.Context,
				entry *logrus.Entry,
				controller *RemoteController, env, identity string) ([]envCustomVSTuple, error) {
				return nil, fmt.Errorf("error getting custom virtualService")
			},
			exptectedError: fmt.Errorf("error getting custom virtualService"),
		},
		{
			name: "Given valid params" +
				"When getCustomVirtualService returns no matching VS" +
				"And func DoDRPinning is called" +
				"Then the func should return true",
			rc:       &RemoteController{},
			env:      "stage",
			identity: "testIdentity",
			getCustomVirtualService: func(
				ctx context.Context,
				entry *logrus.Entry,
				controller *RemoteController, env, identity string) ([]envCustomVSTuple, error) {
				return nil, nil
			},
			expectedResult: true,
		},
		{
			name: "Given valid params" +
				"When getCustomVirtualService returns matching VS" +
				"And the VS with no exportTo" +
				"And func DoDRPinning is called" +
				"Then the func should return false",
			rc:       &RemoteController{},
			env:      "stage",
			identity: "testIdentity",
			getCustomVirtualService: func(
				ctx context.Context,
				entry *logrus.Entry,
				controller *RemoteController, env, identity string) ([]envCustomVSTuple, error) {
				return []envCustomVSTuple{
					{
						customVS: &v1alpha3.VirtualService{
							Spec: istioNetworkingV1Alpha3.VirtualService{},
						},
						env: "stage",
					},
				}, nil
			},
			expectedResult: false,
		},
		{
			name: "Given valid params" +
				"When getCustomVirtualService returns matching VS" +
				"And the VS with no dot in exportTo" +
				"And func DoDRPinning is called" +
				"Then the func should return false",
			rc:       &RemoteController{},
			env:      "stage",
			identity: "testIdentity",
			getCustomVirtualService: func(
				ctx context.Context,
				entry *logrus.Entry,
				controller *RemoteController, env, identity string) ([]envCustomVSTuple, error) {
				return []envCustomVSTuple{
					{
						customVS: &v1alpha3.VirtualService{
							Spec: istioNetworkingV1Alpha3.VirtualService{
								ExportTo: []string{"testNS"},
							},
						},
						env: "stage",
					},
				}, nil
			},
			expectedResult: false,
		},
		{
			name: "Given valid params" +
				"When getCustomVirtualService returns matching VS" +
				"And the VS no dot in exportTo" +
				"And func DoDRPinning is called" +
				"Then the func should return true",
			rc:       &RemoteController{},
			env:      "stage",
			identity: "testIdentity",
			getCustomVirtualService: func(
				ctx context.Context,
				entry *logrus.Entry,
				controller *RemoteController, env, identity string) ([]envCustomVSTuple, error) {
				return []envCustomVSTuple{
					{
						customVS: &v1alpha3.VirtualService{
							Spec: istioNetworkingV1Alpha3.VirtualService{
								ExportTo: []string{"."},
							},
						},
						env: "stage",
					},
				}, nil
			},
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := IsCartographerVSDisabled(
				context.Background(), nil, tc.rc, tc.env, tc.identity, tc.getCustomVirtualService)
			if tc.exptectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.exptectedError, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedResult, actual)
			}
		})
	}

}

func TestHasInClusterVSWithValidExportToNS(t *testing.T) {

	vsWithValidNS := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "foo.testns.global-incluster-vs",
			Labels: map[string]string{common.VSRoutingLabel: "true"},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{
			ExportTo: []string{"testNS"},
		},
	}
	vsWithSyncNS := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "bar.testns.global-incluster-vs",
			Labels: map[string]string{common.VSRoutingLabel: "true"},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{
			ExportTo: []string{"sync-ns"},
		},
	}

	ap := common.AdmiralParams{
		SyncNamespace: "sync-ns",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	virtualServiceCache := istio.NewVirtualServiceCache()
	virtualServiceCache.Put(vsWithValidNS)
	virtualServiceCache.Put(vsWithSyncNS)

	testCases := []struct {
		name          string
		serviceEntry  *istioNetworkingV1Alpha3.ServiceEntry
		remoteCtrl    *RemoteController
		expected      bool
		expectedError error
	}{
		{
			name: "Given a nil remoteController" +
				"When hasInClusterVSWithValidExportToNS is called" +
				"Then it should return an error",
			expectedError: fmt.Errorf("remoteController is nil"),
		},
		{
			name: "Given a nil serviceEntry" +
				"When hasInClusterVSWithValidExportToNS is called" +
				"Then it should return an error",
			remoteCtrl:    &RemoteController{},
			expectedError: fmt.Errorf("serviceEntry is nil"),
		},
		{
			name: "Given an SE with multiple hosts" +
				"When hasInClusterVSWithValidExportToNS is called" +
				"Then it should return an error",
			remoteCtrl: &RemoteController{},
			serviceEntry: &istioNetworkingV1Alpha3.ServiceEntry{
				Hosts: []string{"host1", "host2"},
			},
			expectedError: fmt.Errorf("serviceEntry has more than one host"),
		},
		{
			name: "Given an SE with valid host" +
				"When hasInClusterVSWithValidExportToNS is called" +
				"And the in-cluster VS does not exists in the cache" +
				"Then it should return an error",
			remoteCtrl: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					VirtualServiceCache: virtualServiceCache,
				},
			},
			serviceEntry: &istioNetworkingV1Alpha3.ServiceEntry{
				Hosts: []string{"baz.testns.global"},
			},
			expectedError: fmt.Errorf("virtualService baz.testns.global-incluster-vs not found in cache"),
		},
		{
			name: "Given an SE with valid hosts" +
				"When hasInClusterVSWithValidExportToNS is called" +
				"And the in-cluster VS has valid NS" +
				"Then it should return true",
			remoteCtrl: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					VirtualServiceCache: virtualServiceCache,
				},
			},
			serviceEntry: &istioNetworkingV1Alpha3.ServiceEntry{
				Hosts: []string{"foo.testns.global"},
			},
			expected: true,
		},
		{
			name: "Given an SE with valid hosts" +
				"When hasInClusterVSWithValidExportToNS is called" +
				"And the in-cluster VS has sync NS" +
				"Then it should return false",
			remoteCtrl: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					VirtualServiceCache: virtualServiceCache,
				},
			},
			serviceEntry: &istioNetworkingV1Alpha3.ServiceEntry{
				Hosts: []string{"bar.testns.global"},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := hasInClusterVSWithValidExportToNS(tc.serviceEntry, tc.remoteCtrl)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestDoesIdentityHaveVS(t *testing.T) {

	identityClusterCache := common.NewMapOfMaps()
	identityClusterCache.Put("identity0", "cluster0", "cluster0")
	identityClusterCache.Put("identity1", "cluster1", "cluster1")
	identityClusterCache.Put("identity2", "cluster2", "cluster2")

	identityVirtualServiceCache := istio.NewIdentityVirtualServiceCache()
	identityVirtualServiceCache.Put(&v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vs1",
			Namespace: "test-ns",
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{Hosts: []string{"stage.identity0.global"}},
	})

	remoteController := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IdentityVirtualServiceCache: identityVirtualServiceCache,
		},
	}

	ap := common.AdmiralParams{
		SyncNamespace: "sync-ns",
	}

	remoteRegistry := NewRemoteRegistry(context.Background(), ap)
	remoteRegistry.AdmiralCache.IdentityClusterCache = identityClusterCache
	remoteRegistry.PutRemoteController("cluster0", remoteController)
	remoteRegistry.PutRemoteController("cluster2", remoteController)

	testCases := []struct {
		name           string
		remoteRegistry *RemoteRegistry
		identity       string
		expectedError  error
		expectedResult bool
	}{
		{
			name: "Given a nil remoteRegistry" +
				"When DoesIdentityHaveVS is called" +
				"Then it should return an error",
			remoteRegistry: nil,
			expectedError:  fmt.Errorf("remoteRegistry is nil"),
		},
		{
			name: "Given a nil admiralCache" +
				"When DoesIdentityHaveVS is called" +
				"Then it should return an error",
			remoteRegistry: &RemoteRegistry{},
			expectedError:  fmt.Errorf("AdmiralCache is nil in remoteRegistry"),
		},
		{
			name: "Given a nil IdentityClusterCache" +
				"When DoesIdentityHaveVS is called" +
				"Then it should return an error",
			remoteRegistry: &RemoteRegistry{
				AdmiralCache: &AdmiralCache{},
			},
			expectedError: fmt.Errorf("IdentityClusterCache is nil in AdmiralCache"),
		},
		{
			name: "Given an identity not in IdentityClusterCache" +
				"When DoesIdentityHaveVS is called" +
				"Then it should return an error",
			remoteRegistry: remoteRegistry,
			identity:       "identity9",
			expectedError:  fmt.Errorf("identityClustersMap is nil for identity identity9"),
		},
		{
			name: "Given an identity with no remoteController" +
				"When DoesIdentityHaveVS is called" +
				"Then it should return an error",
			remoteRegistry: remoteRegistry,
			identity:       "identity1",
			expectedError:  fmt.Errorf("remoteController is nil for cluster cluster1"),
		},
		{
			name: "Given an identity which has no VS in its NS" +
				"When DoesIdentityHaveVS is called" +
				"Then it should return false",
			remoteRegistry: remoteRegistry,
			identity:       "identity2",
			expectedError:  nil,
			expectedResult: false,
		},
		{
			name: "Given an identity which has VS in its NS" +
				"When DoesIdentityHaveVS is called" +
				"Then it should return true",
			remoteRegistry: remoteRegistry,
			identity:       "identity0",
			expectedError:  nil,
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := DoesIdentityHaveVS(tc.remoteRegistry, tc.identity)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedResult, actual)
			}
		})
	}
}

func Test_isNLBEnabled(t *testing.T) {
	type args struct {
		nlbClusters    []string
		clusterName    string
		sourceIdentity string
	}

	beforeState := common.GetAdmiralParams()
	admiralParamsTest := common.GetAdmiralParams()
	admiralParamsTest.NLBEnabledIdentityList = []string{"intuit.nlb.mesh.health"}
	common.UpdateAdmiralParams(admiralParamsTest)

	tests := []struct {
		name string
		args args
		want bool
	}{
		{"Identity & Cluster info present", args{
			nlbClusters:    []string{"intuit.mesh.health:test-k8s"},
			clusterName:    "test-k8s",
			sourceIdentity: "intuit.mesh.health",
		}, true},
		{"Empty NLB Clusters", args{
			nlbClusters:    nil,
			clusterName:    "test-k8s",
			sourceIdentity: "intuit.mesh.health",
		}, false},
		{"Identity Present in param as well as NLB overwrites and both match", args{
			nlbClusters:    []string{"intuit.nlb.mesh.health:test-k8s"},
			clusterName:    "test-k8s",
			sourceIdentity: "intuit.nlb.mesh.health",
		}, true},

		//Invalid input
		{"Identity Present in param as well as NLB overwrites and both match (case sensative", args{
			nlbClusters:    []string{"intuit.nlb.mesh.health:test-k8s"},
			clusterName:    "test-k8s",
			sourceIdentity: "Intuit.nlb.mesh.health",
		}, true},

		{"Identity not present in NLB overwrite", args{
			nlbClusters:    []string{":test-k8s"},
			clusterName:    "test-k8s",
			sourceIdentity: "intuit.mesh.health",
		}, false},
		{"Multiple identity present in NLB overwrite", args{
			nlbClusters:    []string{"intuit.nlb.mesh.health,intuit.elb.bmw,intuit.elb.tesla:test-k8s"},
			clusterName:    "test-k8s",
			sourceIdentity: "intuit.mesh.health",
		}, false},
		{"Multiple identity present in NLB overwrite with matching", args{
			nlbClusters:    []string{"intuit.mesh.health,intuit.elb.bmw,intuit.elb.tesla:test-k8s"},
			clusterName:    "test-k8s",
			sourceIdentity: "intuit.mesh.health",
		}, true},
		//Cluster Not matching
		{"Multiple identity present in NLB overwrite with non matching cluster", args{
			nlbClusters:    []string{"intuit.mesh.health,intuit.elb.bmw,intuit.elb.tesla:test-elb-k8s"},
			clusterName:    "test-k8s",
			sourceIdentity: "intuit.mesh.health",
		}, false},
		{"Identity not present in NLB overwrite with non matching cluster", args{
			nlbClusters:    []string{":test-elb-k8s"},
			clusterName:    "test-k8s",
			sourceIdentity: "intuit.mesh.health",
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, isNLBEnabled(tt.args.nlbClusters, tt.args.clusterName, tt.args.sourceIdentity), "isNLBEnabled(%v, %v, %v)", tt.args.nlbClusters, tt.args.clusterName, tt.args.sourceIdentity)
		})
	}

	common.UpdateAdmiralParams(beforeState)

}
