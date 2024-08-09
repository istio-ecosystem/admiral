package clusters

import (
	"context"
	"encoding/json"
	"fmt"
	admiralapiv1 "github.com/istio-ecosystem/admiral-api/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/sirupsen/logrus"
	istioNetworkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sync"
	"testing"
)

var shardTestSingleton sync.Once

func setupForShardTests() common.AdmiralParams {
	var initHappened bool
	admiralParams := admiralParamsForServiceEntryTests()
	admiralParams.EnableAbsoluteFQDN = true
	admiralParams.EnableAbsoluteFQDNForLocalEndpoints = true
	admiralParams.SANPrefix = "pre-prod.api.intuit.com"
	admiralParams.ExportToMaxNamespaces = 35
	admiralParams.AdmiralOperatorMode = true
	admiralParams.OperatorSyncNamespace = "shard-namespace"
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
			LastUpdatedTime:   v1.Time{},
		},
	}
	return &shard
}

func jsonPrint(v any) {
	s, _ := json.MarshalIndent(v, "", "\t")
	fmt.Println(string(s))
}

func TestShardHandler_Added(t *testing.T) {
	admiralParams := setupForShardTests()
	rr, _ := InitAdmiralOperator(context.Background(), admiralParams)
	rc1 := &RemoteController{
		ClusterID: "cg-tax-ppd-usw2-k8s",
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: istiofake.NewSimpleClientset(),
			Cache:       istio.NewServiceEntryCache(),
		},
	}
	rc2 := &RemoteController{
		ClusterID: "multi-long-1026-usw2-k8s",
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: istiofake.NewSimpleClientset(),
			Cache:       istio.NewServiceEntryCache(),
		},
	}
	rr.PutRemoteController("cg-tax-ppd-usw2-k8s", rc1)
	rr.PutRemoteController("multi-long-1026-usw2-k8s", rc2)
	sampleShard1 := createMockShard("shard-sample", "cg-tax-ppd-usw2-k8s", "sample", "e2e")
	sampleShard2 := createMockShard("blackhole-shard", "multi-long-1026-usw2-k8s", "intuit.services.gateway.ppdmeshtestblackhole", "multi-long-1026-usw2-k8s")
	shardHandler := &ShardHandler{
		RemoteRegistry: rr,
	}
	se1 := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:           []string{"e2e.intuit.ctg.taxprep.partnerdatatotax.mesh"},
		Ports:           []*istioNetworkingV1Alpha3.ServicePort{{Number: 80, Protocol: "http", Name: "http"}},
		Location:        1,
		Resolution:      2,
		Endpoints:       []*istioNetworkingV1Alpha3.WorkloadEntry{{Address: "partner-data-to-tax-spk-root-service.ctg-taxprep-partnerdatatotax-usw2-e2e.svc.cluster.local.", Ports: map[string]uint32{"http": 8090}, Labels: map[string]string{"security.istio.io/tlsMode": "istio", "type": "rollout"}, Locality: "us-west-2"}},
		ExportTo:        []string{"ctg-taxprep-partnerdatatotax-usw2-e2e", "ctg-taxprep-partnerdatatotax-usw2-prf", "ctg-taxprep-partnerdatatotax-usw2-qal", common.NamespaceIstioSystem},
		SubjectAltNames: []string{"spiffe://pre-prod.api.intuit.com/Intuit.ctg.taxprep.partnerdatatotax"},
	}
	se2 := &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:      []string{"multi-long-1026-use2-k8s.intuit.services.gateway.ppdmeshtestblackhole.mesh"},
		Ports:      []*istioNetworkingV1Alpha3.ServicePort{{Number: 80, Protocol: "http", Name: "http"}},
		Location:   1,
		Resolution: 2,
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			{Address: "internal-ff96ae9cdbb4c4d81b796cc6a37d3e1d-2123389388.us-east-2.elb.amazonaws.com.", Ports: map[string]uint32{"http": 15443}, Labels: map[string]string{"security.istio.io/tlsMode": "istio", "type": "deployment"}, Locality: "us-east-2"},
		},
		ExportTo:        []string{common.NamespaceIstioSystem, "services-inboundd268-usw2-dev"},
		SubjectAltNames: []string{"spiffe://pre-prod.api.intuit.com/intuit.services.gateway.ppdmeshtestblackhole"},
	}
	testCases := []struct {
		name           string
		rc             *RemoteController
		shard          *admiralapiv1.Shard
		expectedSEName string
		expectedSE     *istioNetworkingV1Alpha3.ServiceEntry
	}{
		{
			name: "Given the server asset we want to write resources for is deployed on the client cluster " +
				"And it is a client of itself " +
				"Then an SE with local endpoint and istio-system in exportTo should be built",
			rc:             rc1,
			shard:          sampleShard1,
			expectedSEName: "e2e.intuit.ctg.taxprep.partnerdatatotax.mesh-se",
			expectedSE:     se1,
		},
		{
			name: "Given the server asset we want to write resources for is deployed on a remote cluster in env A and a client cluster in env B" +
				"Then an SE with only remote endpoint and istio-system in exportTo should be built for env B",
			rc:             rc2,
			shard:          sampleShard2,
			expectedSEName: "multi-long-1026-use2-k8s.intuit.services.gateway.ppdmeshtestblackhole.mesh-se",
			expectedSE:     se2,
		},
		//TODO: Given the server asset we want to write resources for is deployed remotely and locally in the same env, se should have local and remote endpoint and istio-system
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			shErr := shardHandler.Added(context.Background(), tt.shard)
			if shErr != nil {
				t.Errorf("failed to produce SE with err: %v", shErr)
			}
			actualSE, seErr := tt.rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(common.GetOperatorSyncNamespace()).Get(context.Background(), tt.expectedSEName, v1.GetOptions{})
			if seErr != nil {
				t.Errorf("failed to get SE with err %v", seErr)
			}
			if !compareServiceEntries(&actualSE.Spec, tt.expectedSE) {
				jsonPrint(actualSE.Spec)
				jsonPrint(tt.expectedSE)
				t.Errorf("expected se did not match actual se")
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
	err := shardHandler.Deleted(context.Background(), nil)
	if err != nil {
		t.Errorf("expected nil err for delete, for %v", err)
	}
}
