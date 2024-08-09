package registry

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
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
		EnableSAN:                           true,
		SANPrefix:                           "prefix",
		HostnameSuffix:                      "mesh",
		SyncNamespace:                       "ns",
		CacheReconcileDuration:              0,
		ClusterRegistriesNamespace:          "default",
		DependenciesNamespace:               "default",
		WorkloadSidecarName:                 "default",
		Profile:                             common.AdmiralProfileDefault,
		DependentClusterWorkerConcurrency:   5,
		EnableSWAwareNSCaches:               true,
		ExportToIdentityList:                []string{"*"},
		ExportToMaxNamespaces:               35,
		EnableAbsoluteFQDN:                  true,
		EnableAbsoluteFQDNForLocalEndpoints: true,
	}
}

func createMockServiceEntry(env string, identity string, endpointAddress string, endpointPort int, exportTo []string) networkingV1Alpha3.ServiceEntry {
	serviceEntry := networkingV1Alpha3.ServiceEntry{
		Hosts:      []string{env + "." + strings.ToLower(identity) + ".mesh"},
		Addresses:  nil,
		Ports:      []*networkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort), Name: util.Http, Protocol: util.Http}},
		Location:   1,
		Resolution: 2,
		Endpoints: []*networkingV1Alpha3.WorkloadEntry{{Address: endpointAddress,
			Locality: "us-west-2",
			Ports:    map[string]uint32{"http": uint32(endpointPort)},
			Labels:   map[string]string{"security.istio.io/tlsMode": "istio", "type": "rollout"}}},
		WorkloadSelector: nil,
		ExportTo:         exportTo,
		SubjectAltNames:  []string{"spiffe://prefix/" + identity},
	}
	return serviceEntry
}

func TestGetIngressEndpoints(t *testing.T) {
	identityConfig := getSampleIdentityConfig()
	expectedIngressEndpoints := []*networkingV1Alpha3.WorkloadEntry{{
		Address:  "a-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	testCases := []struct {
		name                     string
		identityConfigClusters   []IdentityConfigCluster
		expectedIngressEndpoints []*networkingV1Alpha3.WorkloadEntry
	}{
		{
			name: "Given an IdentityConfigCluster, " +
				"Then the constructed endpoint should be the ingress endpoint",
			identityConfigClusters:   identityConfig.Clusters,
			expectedIngressEndpoints: expectedIngressEndpoints,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			ingressEndpoints, err := getIngressEndpoints(c.identityConfigClusters)
			if err != nil {
				t.Errorf("While constructing ingressEndpoint, got error: %v", err)
			}
			if !reflect.DeepEqual(ingressEndpoints, c.expectedIngressEndpoints) {
				t.Errorf("Mismatch between constructed ingressEndpoint and expected ingressEndpoint")
			}
		})
	}
}

func TestGetServiceEntryPorts(t *testing.T) {
	e2eEnv := getSampleIdentityConfigEnvironment("e2e", "ctg-taxprep-partnerdatatotax-usw2-e2e")
	expectedSEPorts := []*networkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort), Name: util.Http, Protocol: util.Http}}
	testCases := []struct {
		name                      string
		identityConfigEnvironment IdentityConfigEnvironment
		expectedSEPorts           []*networkingV1Alpha3.ServicePort
	}{
		{
			name: "Given an IdentityConfigEnvironment, " +
				"Then the constructed ServiceEntryPorts should be as expected",
			identityConfigEnvironment: e2eEnv,
			expectedSEPorts:           expectedSEPorts,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			sePorts, err := getServiceEntryPorts(e2eEnv)
			if err != nil {
				t.Errorf("While constructing serviceEntryPorts, got error: %v", err)
			}
			if !reflect.DeepEqual(sePorts, c.expectedSEPorts) {
				t.Errorf("Mismatch between constructed ingressEndpoint and expected ingressEndpoint")
			}
		})
	}
}

func TestGetServiceEntryEndpoints(t *testing.T) {
	admiralParams := admiralParamsForServiceEntryTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	e2eEnv := getSampleIdentityConfigEnvironment("e2e", "ctg-taxprep-partnerdatatotax-usw2-e2e")
	ingressEndpoints := []*networkingV1Alpha3.WorkloadEntry{{
		Address:  "a-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	remoteEndpoint := []*networkingV1Alpha3.WorkloadEntry{{
		Address:  "a-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio", "type": "rollout"},
	}}
	localEndpoint := []*networkingV1Alpha3.WorkloadEntry{{
		Address:  "partner-data-to-tax-spk-root-service.ctg-taxprep-partnerdatatotax-usw2-e2e.svc.cluster.local.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(8090)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio", "type": "rollout"},
	}}
	ctx := context.Background()
	ctxLogger := common.GetCtxLogger(ctx, "ctg-taxprep-partnerdatatotax", "")
	testCases := []struct {
		name                      string
		identityConfigEnvironment IdentityConfigEnvironment
		ingressEndpoints          []*networkingV1Alpha3.WorkloadEntry
		operatorCluster           string
		sourceCluster             string
		remoteEndpointAddress     string
		expectedSEEndpoints       []*networkingV1Alpha3.WorkloadEntry
	}{
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint, " +
				"When the operator cluster is not the same as the source cluster" +
				"Then the constructed endpoint should be a remote endpoint",
			identityConfigEnvironment: e2eEnv,
			ingressEndpoints:          ingressEndpoints,
			operatorCluster:           "cg-tax-ppd-usw2-k8s",
			sourceCluster:             "apigw-cx-ppd-usw2-k8s",
			remoteEndpointAddress:     "a-elb.us-west-2.elb.amazonaws.com.",
			expectedSEEndpoints:       remoteEndpoint,
		},
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint, " +
				"When the operator cluster is the same as the source cluster" +
				"Then the constructed endpoint should be a local endpoint",
			identityConfigEnvironment: e2eEnv,
			ingressEndpoints:          ingressEndpoints,
			operatorCluster:           "cg-tax-ppd-usw2-k8s",
			sourceCluster:             "cg-tax-ppd-usw2-k8s",
			remoteEndpointAddress:     "a-elb.us-west-2.elb.amazonaws.com.",
			expectedSEEndpoints:       localEndpoint,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			seEndpoint, err := getServiceEntryEndpoints(ctxLogger, c.operatorCluster, c.sourceCluster, c.ingressEndpoints, c.remoteEndpointAddress, c.identityConfigEnvironment)
			if err != nil {
				t.Errorf("While constructing serviceEntryPortEndpoint, got error: %v", err)
			}
			opts := cmpopts.IgnoreUnexported(networkingV1Alpha3.WorkloadEntry{})
			if !cmp.Equal(seEndpoint, c.expectedSEEndpoints, opts) {
				t.Errorf("Mismatch between constructed ingressEndpoint and expected ingressEndpoint")
				t.Errorf(cmp.Diff(seEndpoint, c.expectedSEEndpoints, opts))
			}
		})
	}
}

func TestBuildServiceEntriesFromIdentityConfig(t *testing.T) {

}
