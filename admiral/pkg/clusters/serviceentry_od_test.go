package clusters

import (
	"context"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	admiralV1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	v13 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	istioNetworkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func Test_updateGlobalOutlierDetectionCache(t *testing.T) {

	ctxLogger := logrus.WithFields(logrus.Fields{
		"txId": "abc",
	})
	common.ResetSync()

	remoteRegistryTest, _ := InitAdmiral(context.Background(), common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			AdmiralCRDIdentityLabel: "assetAlias",
		},
	})

	type args struct {
		cache             *AdmiralCache
		identity          string
		env               string
		outlierDetections map[string][]*admiralV1.OutlierDetection
	}

	testLabels := make(map[string]string)
	testLabels["identity"] = "foo"
	testLabels["assetAlias"] = "foo"

	outlierDetection1 := admiralV1.OutlierDetection{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "foo",
			Labels:    testLabels,
		},
		Spec:   makeOutlierDetectionTestModel(),
		Status: v13.OutlierDetectionStatus{},
	}

	outlierDetection1.ObjectMeta.CreationTimestamp = metav1.Now()

	odConfig1 := makeOutlierDetectionTestModel()
	odConfig1.OutlierConfig.ConsecutiveGatewayErrors = 100
	outlierDetection2 := admiralV1.OutlierDetection{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo1",
			Namespace: "foo1",
			Labels:    testLabels,
		},
		Spec:   odConfig1,
		Status: v13.OutlierDetectionStatus{},
	}

	outlierDetection2.ObjectMeta.CreationTimestamp = metav1.Now()

	arg1 := args{
		cache:             remoteRegistryTest.AdmiralCache,
		identity:          "foo",
		env:               "e2e",
		outlierDetections: nil,
	}
	arg1.outlierDetections = make(map[string][]*admiralV1.OutlierDetection)
	arg1.outlierDetections["test"] = append(arg1.outlierDetections["test"], &outlierDetection1)
	arg1.outlierDetections["test"] = append(arg1.outlierDetections["test"], &outlierDetection2)

	arg2 := args{
		cache:             remoteRegistryTest.AdmiralCache,
		identity:          "foo",
		env:               "e2e",
		outlierDetections: nil,
	}
	arg2.outlierDetections = make(map[string][]*admiralV1.OutlierDetection)

	arg2.cache.OutlierDetectionCache.Put(&outlierDetection1)
	arg2.cache.OutlierDetectionCache.Put(&outlierDetection2)

	tests := []struct {
		name      string
		args      args
		expected  *admiralV1.OutlierDetection
		wantedErr bool
	}{
		{"Validate only latest outlier detection object CRD present when more 2 object supplied", arg1, &outlierDetection2, false},
		{"Validate no object present when no outlier detection found", arg2, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateGlobalOutlierDetectionCache(ctxLogger, tt.args.cache, tt.args.identity, tt.args.env, tt.args.outlierDetections)
			actualOD, err := remoteRegistryTest.AdmiralCache.OutlierDetectionCache.GetFromIdentity("foo", "e2e")
			if tt.wantedErr {
				assert.NotNil(t, err, "Expected Error")
			}
			assert.Equal(t, tt.expected, actualOD)
			assert.Nil(t, err, "Expecting no errors")

		})
	}
}

func makeOutlierDetectionTestModel() model.OutlierDetection {
	odConfig := model.OutlierConfig{
		BaseEjectionTime:         0,
		ConsecutiveGatewayErrors: 0,
		Interval:                 0,
		XXX_NoUnkeyedLiteral:     struct{}{},
		XXX_unrecognized:         nil,
		XXX_sizecache:            0,
	}

	od := model.OutlierDetection{
		Selector:      map[string]string{"identity": "payments", "env": "e2e"},
		OutlierConfig: &odConfig,
	}

	return od
}

func Test_modifyServiceEntryForNewServiceOrPodForOutlierDetection(t *testing.T) {
	setupForServiceEntryTests()
	var (
		env                                 = "test"
		stop                                = make(chan struct{})
		foobarMetadataName                  = "foobar"
		foobarMetadataNamespace             = "foobar-ns"
		deployment1Identity                 = "deployment1"
		deployment1                         = makeTestDeployment(foobarMetadataName, foobarMetadataNamespace, deployment1Identity)
		cluster1ID                          = "test-dev-1-k8s"
		cluster2ID                          = "test-dev-2-k8s"
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
						Address: "internal-load-balancer-" + cluster1ID,
						Ports: map[string]uint32{
							"http": 0,
						},
						Locality: "us-west-2",
					},
				},
				SubjectAltNames: []string{"spiffe://prefix/" + deployment1Identity},
			},
		}
		serviceEntryAddressStore = &ServiceEntryAddressStore{
			EntryAddresses: map[string]string{
				"test." + deployment1Identity + ".mesh-se": "127.0.0.1",
			},
			Addresses: []string{},
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
		serviceForIngressInCluster1 = &coreV1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "istio-ingressgateway",
				Namespace: "istio-system",
				Labels: map[string]string{
					"app": "gatewayapp",
				},
			},
			Spec: coreV1.ServiceSpec{
				Selector: map[string]string{"app": "istio-ingressgateway"},
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
						coreV1.LoadBalancerIngress{
							Hostname: "internal-load-balancer-" + cluster1ID,
						},
					},
				},
			},
		}
		serviceForIngressInCluster2 = &coreV1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "istio-ingressgateway",
				Namespace: "istio-system",
				Labels: map[string]string{
					"app": "gatewayapp",
				},
			},
			Spec: coreV1.ServiceSpec{
				Selector: map[string]string{"app": "istio-ingressgateway"},
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
						coreV1.LoadBalancerIngress{
							Hostname: "internal-load-balancer-" + cluster2ID,
						},
					},
				},
			},
		}
		remoteRegistry, _ = InitAdmiral(context.Background(), admiralParamsForServiceEntryTests())
	)
	deploymentController, err := admiral.NewDeploymentController(make(chan struct{}), &test.MockDeploymentHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	deploymentController.Cache.UpdateDeploymentToClusterCache(deployment1Identity, deployment1)
	rolloutController, err := admiral.NewRolloutsController(make(chan struct{}), &test.MockRolloutHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fail()
	}
	serviceControllerCluster1, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	serviceControllerCluster2, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	virtualServiceController, err := istio.NewVirtualServiceController(make(chan struct{}), &test.MockVirtualServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	globalTrafficPolicyController, err := admiral.NewGlobalTrafficController(make(chan struct{}), &test.MockGlobalTrafficHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
		t.FailNow()
	}

	outlierDetectionPolicy := v13.OutlierDetection{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        foobarMetadataName,
			Namespace:   foobarMetadataNamespace,
			Annotations: map[string]string{"admiral.io/env": "test", "env": "test"},
			Labels:      map[string]string{"assetAlias": "deployment1", "identity": "deployment1"},
		},
		Spec: model.OutlierDetection{
			OutlierConfig: &model.OutlierConfig{
				BaseEjectionTime:         10,
				ConsecutiveGatewayErrors: 10,
				Interval:                 100,
			},
			Selector:             nil,
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_unrecognized:     nil,
			XXX_sizecache:        0,
		},
		Status: v13.OutlierDetectionStatus{},
	}

	outlierDetectionController, err := admiral.NewOutlierDetectionController(make(chan struct{}), &test.MockOutlierDetectionHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
		t.FailNow()
	}
	outlierDetectionController.GetCache().Put(&outlierDetectionPolicy)

	serviceControllerCluster1.Cache.Put(serviceForDeployment)
	serviceControllerCluster1.Cache.Put(serviceForIngressInCluster1)
	serviceControllerCluster2.Cache.Put(serviceForDeployment)
	serviceControllerCluster2.Cache.Put(serviceForIngressInCluster2)
	rcCluster1 := &RemoteController{
		ClusterID:                cluster1ID,
		DeploymentController:     deploymentController,
		RolloutController:        rolloutController,
		ServiceController:        serviceControllerCluster1,
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
		GlobalTraffic:              globalTrafficPolicyController,
		OutlierDetectionController: outlierDetectionController,
	}
	rcCluster2 := &RemoteController{
		ClusterID:                cluster2ID,
		DeploymentController:     deploymentController,
		RolloutController:        rolloutController,
		ServiceController:        serviceControllerCluster2,
		VirtualServiceController: virtualServiceController,
		NodeController: &admiral.NodeController{
			Locality: &admiral.Locality{
				Region: "us-east-2",
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
		GlobalTraffic:              globalTrafficPolicyController,
		OutlierDetectionController: outlierDetectionController,
	}

	remoteRegistry.PutRemoteController(cluster1ID, rcCluster1)
	remoteRegistry.PutRemoteController(cluster2ID, rcCluster2)
	remoteRegistry.ServiceEntrySuspender = NewDefaultServiceEntrySuspender([]string{"asset1"})
	remoteRegistry.StartTime = time.Now()
	remoteRegistry.AdmiralCache.ServiceEntryAddressStore = serviceEntryAddressStore

	testCases := []struct {
		name                   string
		assetIdentity          string
		readOnly               bool
		remoteRegistry         *RemoteRegistry
		expectedServiceEntries map[string]*istioNetworkingV1Alpha3.ServiceEntry
	}{
		//Both test case should return same service entry as outlier detection crd doesn't change Service Entry
		{
			name:                   "OutlierDetection present in namespace",
			assetIdentity:          deployment1Identity,
			remoteRegistry:         remoteRegistry,
			expectedServiceEntries: expectedServiceEntriesForDeployment,
		},
		{
			name:                   "OutlierDetection not present",
			assetIdentity:          deployment1Identity,
			remoteRegistry:         remoteRegistry,
			expectedServiceEntries: expectedServiceEntriesForDeployment,
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
			destinationRule, err := c.remoteRegistry.remoteControllers[cluster1ID].DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules("ns").Get(ctx, "test.deployment1.mesh-default-dr", metav1.GetOptions{})
			assert.Nil(t, err, "Expected no error for fetching outlier detection")
			assert.Equal(t, int(destinationRule.Spec.TrafficPolicy.OutlierDetection.Interval.Seconds), 100)
			assert.Equal(t, int(destinationRule.Spec.TrafficPolicy.OutlierDetection.BaseEjectionTime.Seconds), 10)
			assert.Equal(t, int(destinationRule.Spec.TrafficPolicy.OutlierDetection.ConsecutiveGatewayErrors.Value), 10)
			assert.Equal(t, int(destinationRule.Spec.TrafficPolicy.OutlierDetection.Consecutive_5XxErrors.Value), 0)
		})
	}
}
