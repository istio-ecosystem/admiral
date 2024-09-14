package clusters

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sync"
	"testing"
	"time"

	admiralapiv1 "github.com/istio-ecosystem/admiral-api/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/sirupsen/logrus"
	istioNetworkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var shardTestSingleton sync.Once

func setupForShardTests() common.AdmiralParams {
	var initHappened bool
	admiralParams := admiralParamsForServiceEntryTests()
	admiralParams.EnableAbsoluteFQDN = true
	admiralParams.EnableAbsoluteFQDNForLocalEndpoints = true
	admiralParams.DisableIPGeneration = true
	admiralParams.SANPrefix = "pre-prod.api.org.com"
	admiralParams.ExportToMaxNamespaces = 35
	admiralParams.AdmiralOperatorMode = true
	admiralParams.OperatorSyncNamespace = "admiral-operator-sync"
	admiralParams.AdditionalEndpointSuffixes = []string{"org"}
	admiralParams.SecretFilterTags = "admiral/sync"
	admiralParams.OperatorSecretFilterTags = "admiral/syncoperator"
	shardTestSingleton.Do(func() {
		common.ResetSync()
		initHappened = true
		common.InitializeConfig(admiralParams)
	})
	if !initHappened {
		logrus.Warn("InitializeConfig was NOT called from setupForShardTests")
	} else {
		logrus.Info("InitializeConfig was called setupForShardTests")
	}
	return admiralParams
}

func createRemoteControllerForShardTests(cluster string) *RemoteController {
	return &RemoteController{
		ClusterID: cluster,
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: istiofake.NewSimpleClientset(),
			Cache:       istio.NewServiceEntryCache(),
		},
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: istiofake.NewSimpleClientset(),
			Cache:       istio.NewDestinationRuleCache(),
		},
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient:           istiofake.NewSimpleClientset(),
			VirtualServiceHandler: &test.MockVirtualServiceHandler{},
		},
		SidecarController: &istio.SidecarController{
			IstioClient:    istiofake.NewSimpleClientset(),
			SidecarHandler: &test.MockSidecarHandler{},
		},
	}
}

func createMockShard(shardName string, clusterName string, identityName string, identityEnv string) *admiralapiv1.Shard {
	identityItem := admiralapiv1.IdentityItem{
		Name:        identityName,
		Environment: identityEnv,
	}
	clusterShard := admiralapiv1.ClusterShards{
		Name:       clusterName,
		Locality:   "us-west-2",
		Identities: []admiralapiv1.IdentityItem{identityItem},
	}
	shardStatusCondition := admiralapiv1.ShardStatusCondition{
		Message:         "sync not started",
		Reason:          "notStarted",
		Status:          "false",
		Type:            "SyncComplete",
		LastUpdatedTime: v1.Now(),
	}
	shard := admiralapiv1.Shard{
		ObjectMeta: v1.ObjectMeta{Name: shardName, Namespace: "shard-namespace"},
		Spec:       admiralapiv1.ShardSpec{Clusters: []admiralapiv1.ClusterShards{clusterShard}},
		Status: admiralapiv1.ShardStatus{
			ClustersMonitored: 1,
			Conditions:        []admiralapiv1.ShardStatusCondition{shardStatusCondition},
			FailureDetails:    admiralapiv1.FailureDetails{},
			LastUpdatedTime:   v1.Now(),
		},
	}
	return &shard
}

func jsonPrint(v any) string {
	s, _ := json.MarshalIndent(v, "", "\t")
	return fmt.Sprintln(string(s))
}

func TestShardHandler_Added(t *testing.T) {
	admiralParams := setupForShardTests()
	rr, _ := InitAdmiralOperator(context.Background(), admiralParams)
	rr.AdmiralDatabaseClient = &mockDatabaseClientWithError{}
	rc1 := createRemoteControllerForShardTests("cluster1")
	rc2 := createRemoteControllerForShardTests("cluster-usw2-k8s")
	rr.PutRemoteController("cluster1", rc1)
	rr.PutRemoteController("cluster-usw2-k8s", rc2)

	//sampleShard1 := createMockShard("shard-sample", "cluster1", "sample", "e2e")
	sampleShard2 := createMockShard("blackhole-shard", "cluster-usw2-k8s", "ppdmeshtestblackhole", "ppd")
	shardHandler := &ShardHandler{RemoteRegistry: rr}
	sc, _ := admiral.NewShardController(make(chan struct{}), shardHandler, "../../test/resources/admins@fake-cluster.k8s.local", "ns", time.Duration(1000), loader.GetFakeClientLoader())
	sampleShard2, _ = sc.CrdClient.AdmiralV1().Shards(sampleShard2.Namespace).Create(context.Background(), sampleShard2, v1.CreateOptions{})
	defaultSidecar := &v1alpha3.Sidecar{
		ObjectMeta: v1.ObjectMeta{
			Name: "default",
		},
		Spec: istioNetworkingV1Alpha3.Sidecar{
			Egress: []*istioNetworkingV1Alpha3.IstioEgressListener{
				{
					Hosts: []string{"./*", common.NamespaceIstioSystem + "/*", common.GetOperatorSyncNamespace() + "/*"},
				},
			},
		},
	}
	//se1 := &istioNetworkingV1Alpha3.ServiceEntry{
	//	Hosts:      []string{"testDnsPrefix.e2e.sample.mesh"},
	//	Ports:      []*istioNetworkingV1Alpha3.ServicePort{{Number: 80, Protocol: "http", Name: "http"}},
	//	Location:   istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
	//	Resolution: istioNetworkingV1Alpha3.ServiceEntry_DNS,
	//	Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
	//		{Address: "app-1-spk-root-service.ns-1-usw2-e2e.svc.cluster.local.",
	//			Ports:    map[string]uint32{"http": 8090},
	//			Labels:   map[string]string{"security.istio.io/tlsMode": "istio", "type": "rollout"},
	//			Locality: "us-west-2"}},
	//	ExportTo:        []string{common.NamespaceIstioSystem, "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"},
	//	SubjectAltNames: []string{"spiffe://pre-prod.api.org.com/sample"},
	//}
	se2 := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:      []string{"testDnsPrefix.ppd.ppdmeshtestblackhole.mesh"},
		Ports:      []*istioNetworkingV1Alpha3.ServicePort{{Number: 80, Protocol: "http", Name: "http"}},
		Location:   istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution: istioNetworkingV1Alpha3.ServiceEntry_DNS,
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "abc-elb.us-east-2.elb.amazonaws.com.",
				Ports:    map[string]uint32{"http": 15443},
				Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
				Locality: "us-east-2"},
		},
		ExportTo:        []string{common.NamespaceIstioSystem, "services-inboundd268-usw2-dev"},
		SubjectAltNames: []string{"spiffe://pre-prod.api.org.com/ppdmeshtestblackhole"},
	}
	sidecar2 := &istioNetworkingV1Alpha3.Sidecar{
		Egress: []*istioNetworkingV1Alpha3.IstioEgressListener{
			{
				Hosts: []string{"./*", common.NamespaceIstioSystem + "/*", common.GetOperatorSyncNamespace() + "/*"},
			},
		},
	}
	rc1.SidecarController.IstioClient.NetworkingV1alpha3().Sidecars("ns-1-usw2-prf").Create(context.Background(), defaultSidecar, v1.CreateOptions{})
	rc1.SidecarController.IstioClient.NetworkingV1alpha3().Sidecars("ns-1-usw2-e2e").Create(context.Background(), defaultSidecar, v1.CreateOptions{})
	rc1.SidecarController.IstioClient.NetworkingV1alpha3().Sidecars("ns-1-usw2-qal").Create(context.Background(), defaultSidecar, v1.CreateOptions{})
	rc2.SidecarController.IstioClient.NetworkingV1alpha3().Sidecars("services-blackholed268-usw2-dev").Create(context.Background(), defaultSidecar, v1.CreateOptions{})
	dr2 := &istioNetworkingV1Alpha3.DestinationRule{
		Host: "ppd.ppdmeshtestblackhole.mesh",
		TrafficPolicy: &istioNetworkingV1Alpha3.TrafficPolicy{
			LoadBalancer: &istioNetworkingV1Alpha3.LoadBalancerSettings{
				LbPolicy: &istioNetworkingV1Alpha3.LoadBalancerSettings_Simple{
					Simple: istioNetworkingV1Alpha3.LoadBalancerSettings_LEAST_REQUEST,
				},
				WarmupDurationSecs: &duration.Duration{Seconds: 0},
			},
			ConnectionPool: &istioNetworkingV1Alpha3.ConnectionPoolSettings{
				Http: &istioNetworkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
					Http2MaxRequests:         1000,
					MaxRequestsPerConnection: 5,
				},
			},
			OutlierDetection: &istioNetworkingV1Alpha3.OutlierDetection{
				ConsecutiveGatewayErrors: wrapperspb.UInt32(10),
				Consecutive_5XxErrors:    wrapperspb.UInt32(0),
				Interval:                 &duration.Duration{Seconds: 10},
				BaseEjectionTime:         &duration.Duration{Seconds: 0},
				MaxEjectionPercent:       33,
			},
			Tls: &istioNetworkingV1Alpha3.ClientTLSSettings{
				Mode: istioNetworkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
			},
		},
		ExportTo: []string{common.NamespaceIstioSystem, "services-inboundd268-usw2-dev"},
	}
	vs2 := &istioNetworkingV1Alpha3.VirtualService{
		Hosts: []string{"ppd.ppdmeshtestblackhole.org"},
		Http: []*istioNetworkingV1Alpha3.HTTPRoute{
			{Route: []*istioNetworkingV1Alpha3.HTTPRouteDestination{
				{Destination: &istioNetworkingV1Alpha3.Destination{
					Host: "ppd.ppdmeshtestblackhole.mesh",
					Port: &istioNetworkingV1Alpha3.PortSelector{Number: 80},
				},
				},
			},
			},
		},
	}
	testCases := []struct {
		name              string
		rc                *RemoteController
		shard             *admiralapiv1.Shard
		expectedSEName    string
		expectedSE        *istioNetworkingV1Alpha3.ServiceEntry
		expectedSidecarNS string
		expectedSidecar   *istioNetworkingV1Alpha3.Sidecar
		expectedDRName    string
		expectedDR        *istioNetworkingV1Alpha3.DestinationRule
		expectedVSName    string
		expectedVS        *istioNetworkingV1Alpha3.VirtualService
	}{
		//{
		//	name: "Given the server asset we want to write resources for is deployed on the client cluster " +
		//		"And it is a client of itself " +
		//		"Then an SE with local endpoint and istio-system in exportTo should be built",
		//	rc:                rc1,
		//	shard:             sampleShard1,
		//	expectedSEName:    "testdnsprefix.e2e.sample.mesh-se",
		//	expectedSE:        se1,
		//	expectedSidecarNS: "ns-1-usw2-e2e",
		//},
		{
			name: "Given the server asset we want to write resources for is deployed on a remote cluster in env A and a client cluster in env B" +
				"Then an SE with only remote endpoint and istio-system in exportTo should be built for env B",
			rc:                rc2,
			shard:             sampleShard2,
			expectedSEName:    "testdnsprefix.ppd.ppdmeshtestblackhole.mesh-se",
			expectedSE:        se2,
			expectedSidecarNS: "services-blackholed268-usw2-dev",
			expectedSidecar:   sidecar2,
			expectedDRName:    "ppd.ppdmeshtestblackhole.mesh-default-dr",
			expectedDR:        dr2,
			expectedVSName:    "ppd.ppdmeshtestblackhole.org-vs",
			expectedVS:        vs2,
		},
		//TODO: Given the server asset we want to write resources for is deployed remotely and locally in the same env, se should have local and remote endpoint and istio-system
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			shErr := shardHandler.Added(context.Background(), tt.shard, sc.CrdClient)
			if shErr != nil {
				t.Errorf("failed to handle Shard with err: %v", shErr)
			}
			// Check that expected SE matches the produced SE
			actualSE, seErr := tt.rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(common.GetOperatorSyncNamespace()).Get(context.Background(), tt.expectedSEName, v1.GetOptions{})
			if seErr != nil {
				t.Errorf("failed to get ServiceEntry with err %v", seErr)
			}
			if actualSE == nil {
				t.Errorf("expected ServiceEntry to not be nil")
			} else if !compareServiceEntries(&actualSE.Spec, tt.expectedSE) {
				t.Errorf("got=%v, want=%v", jsonPrint(actualSE.Spec), jsonPrint(tt.expectedSE))
			}
			// Check that the expected Sidecar matches the produced Sidecar
			actualSidecar, sidecarErr := tt.rc.SidecarController.IstioClient.NetworkingV1alpha3().Sidecars(tt.expectedSidecarNS).Get(context.Background(), common.GetWorkloadSidecarName(), v1.GetOptions{})
			if actualSidecar == nil {
				t.Errorf("expected Sidecar to not be nil")
			} else if !cmp.Equal(&actualSidecar.Spec, tt.expectedSidecar, protocmp.Transform()) {
				t.Errorf("got=%v, want=%v", jsonPrint(actualSidecar), jsonPrint(tt.expectedSidecar))
			}
			if sidecarErr != nil {
				t.Errorf("failed to get Sidecar with err %v", sidecarErr)
			}
			// Check that the expected DR matches the produced DR
			actualDR, drErr := tt.rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(common.GetOperatorSyncNamespace()).Get(context.Background(), tt.expectedDRName, v1.GetOptions{})
			if actualDR == nil {
				t.Errorf("expected DestinationRule to not be nil")
			} else if !cmp.Equal(&actualDR.Spec, tt.expectedDR, protocmp.Transform()) {
				t.Errorf("got=%v, want=%v", jsonPrint(actualDR), jsonPrint(tt.expectedDR))
			}
			if drErr != nil {
				t.Errorf("failed to get DestinationRule with err %v", drErr)
			}
			// Check that the expected VS matches the produced VS
			actualVS, vsErr := tt.rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(common.GetOperatorSyncNamespace()).Get(context.Background(), tt.expectedVSName, v1.GetOptions{})
			if actualVS == nil {
				t.Errorf("expected VirtualService to not be nil")
			} else if !cmp.Equal(&actualVS.Spec, tt.expectedVS, protocmp.Transform()) {
				t.Errorf("got=%v, want=%v", jsonPrint(actualVS), jsonPrint(tt.expectedVS))
			}
			if vsErr != nil {
				t.Errorf("failed to get VirtualService with err %v", vsErr)
			}
		})
	}
}

func TestShardHandler_Deleted(t *testing.T) {
	admiralParams := setupForShardTests()
	rr, _ := InitAdmiralOperator(context.Background(), admiralParams)
	shardHandler := &ShardHandler{
		RemoteRegistry: rr,
	}
	shard := createMockShard("testShard", "testCluster", "testIdentity", "testEnv")
	err := shardHandler.Deleted(context.Background(), shard)
	if err != nil {
		t.Errorf("expected nil err for delete, for %v", err)
	}
}

func TestRetryUpdatingShard(t *testing.T) {
	admiralParams := setupForShardTests()
	rr, _ := InitAdmiralOperator(context.Background(), admiralParams)
	sampleShard := createMockShard("blackhole-shard", "cluster-usw2-k8s", "ppdmeshtestblackhole", "ppd")
	shardHandler := &ShardHandler{RemoteRegistry: rr}
	sc, _ := admiral.NewShardController(make(chan struct{}), shardHandler, "../../test/resources/admins@fake-cluster.k8s.local", "ns", time.Duration(1000), loader.GetFakeClientLoader())
	sc.CrdClient.AdmiralV1().Shards(sampleShard.Namespace).Create(context.Background(), sampleShard, v1.CreateOptions{})
	sampleShard2 := sampleShard.DeepCopy()
	newFailedCluster := admiralapiv1.FailedCluster{
		Name: "cluster-usw2-k8s",
		FailedIdentities: []admiralapiv1.FailedIdentity{{
			Name:    "ppdmeshtestblackhole",
			Message: "test failure message",
		}},
	}
	sampleShard2.Status.FailureDetails.FailedClusters = append(sampleShard2.Status.FailureDetails.FailedClusters, newFailedCluster)
	ctxLogger := common.GetCtxLogger(context.Background(), sampleShard2.Name, "")
	testCases := []struct {
		name          string
		sc            *admiral.ShardController
		newShard      *admiralapiv1.Shard
		existingShard *admiralapiv1.Shard
		expectedShard *admiralapiv1.Shard
		err           *k8sErrors.StatusError
	}{
		{
			name: "Given the server asset we want to write resources for is deployed on a remote cluster in env A and a client cluster in env B" +
				"Then an SE with only remote endpoint and istio-system in exportTo should be built for env B",
			sc:            sc,
			newShard:      sampleShard2,
			existingShard: sampleShard,
			expectedShard: sampleShard2,
			err:           k8sErrors.NewConflict(schema.GroupResource{}, "", nil),
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			err := retryUpdatingShard(context.Background(), ctxLogger, tt.newShard, tt.existingShard, tt.sc.CrdClient, tt.err)
			if err != nil {
				t.Errorf("failed to retry updating shard with err: %v", err)
			}
			// Check that the expected Shard matches the produced Shard
			actualShard, shardErr := tt.sc.CrdClient.AdmiralV1().Shards(tt.newShard.Namespace).Get(context.Background(), tt.newShard.Name, v1.GetOptions{})
			if actualShard == nil {
				t.Errorf("expected Shard to not be nil")
			} else if !cmp.Equal(actualShard.Status, tt.expectedShard.Status, protocmp.Transform()) {
				t.Errorf("got=%v, want=%v", jsonPrint(actualShard.Status), jsonPrint(tt.expectedShard.Status))
			}
			if shardErr != nil {
				t.Errorf("failed to get Shard with err %v", shardErr)
			}
		})
	}
}
