package clusters

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
)

func admiralParamsForConfigWriterTests() common.AdmiralParams {
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
		MetricsEnabled:                      true,
		SecretFilterTags:                    "admiral/sync",
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
		AdmiralOperatorMode:                 true,
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
	identityConfig := registry.GetSampleIdentityConfig()
	expectedIngressEndpoints := map[string]*networkingV1Alpha3.WorkloadEntry{"cg-tax-ppd-usw2-k8s": {
		Address:  "internal-a96ffe9cdbb4c4d81b796cc6a37d3e1d-2123389388.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	testCases := []struct {
		name                     string
		identityConfigClusters   []registry.IdentityConfigCluster
		expectedIngressEndpoints map[string]*networkingV1Alpha3.WorkloadEntry
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
	e2eEnv := registry.GetSampleIdentityConfigEnvironment("e2e", "ctg-taxprep-partnerdatatotax-usw2-e2e")
	expectedSEPorts := []*networkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort), Name: util.Http, Protocol: util.Http}}
	testCases := []struct {
		name                      string
		identityConfigEnvironment registry.IdentityConfigEnvironment
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

func TestGetServiceEntryEndpoint(t *testing.T) {
	admiralParams := admiralParamsForConfigWriterTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	e2eEnv := registry.GetSampleIdentityConfigEnvironment("e2e", "ctg-taxprep-partnerdatatotax-usw2-e2e")
	ingressEndpoints := map[string]*networkingV1Alpha3.WorkloadEntry{"cg-tax-ppd-usw2-k8s": {
		Address:  "internal-a96ffe9cdbb4c4d81b796cc6a37d3e1d-2123389388.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}, "apigw-cx-ppd-usw2-k8s": {
		Address:  "internal-a1cbfde75adbe1fed9763495dfd07960-2123389388.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	remoteEndpoint := &networkingV1Alpha3.WorkloadEntry{
		Address:  "internal-a1cbfde75adbe1fed9763495dfd07960-2123389388.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio", "type": "rollout"},
	}
	localEndpoint := &networkingV1Alpha3.WorkloadEntry{
		Address:  "partner-data-to-tax-spk-root-service.ctg-taxprep-partnerdatatotax-usw2-e2e.svc.cluster.local.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(8090)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio", "type": "rollout"},
	}
	ctx := context.Background()
	ctxLogger := common.GetCtxLogger(ctx, "ctg-taxprep-partnerdatatotax", "")
	testCases := []struct {
		name                      string
		identityConfigEnvironment registry.IdentityConfigEnvironment
		ingressEndpoints          map[string]*networkingV1Alpha3.WorkloadEntry
		clientCluster             string
		serverCluster             string
		expectedSEEndpoint        *networkingV1Alpha3.WorkloadEntry
	}{
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint, " +
				"When the client cluster is not the same as the server cluster" +
				"Then the constructed endpoint should be a remote endpoint",
			identityConfigEnvironment: e2eEnv,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cg-tax-ppd-usw2-k8s",
			serverCluster:             "apigw-cx-ppd-usw2-k8s",
			expectedSEEndpoint:        remoteEndpoint,
		},
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint, " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed endpoint should be a local endpoint",
			identityConfigEnvironment: e2eEnv,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cg-tax-ppd-usw2-k8s",
			serverCluster:             "cg-tax-ppd-usw2-k8s",
			expectedSEEndpoint:        localEndpoint,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			seEndpoint, err := getServiceEntryEndpoint(ctxLogger, c.clientCluster, c.serverCluster, c.ingressEndpoints, c.identityConfigEnvironment)
			if err != nil {
				t.Errorf("While constructing serviceEntryPortEndpoint, got error: %v", err)
			}
			opts := cmpopts.IgnoreUnexported(networkingV1Alpha3.WorkloadEntry{})
			if !cmp.Equal(seEndpoint, c.expectedSEEndpoint, opts) {
				t.Errorf("Mismatch between constructed ingressEndpoint and expected ingressEndpoint")
				t.Errorf(cmp.Diff(seEndpoint, c.expectedSEEndpoint, opts))
			}
		})
	}
}

func TestGetExportTo(t *testing.T) {
	admiralParams := admiralParamsForConfigWriterTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	ctxLogger := common.GetCtxLogger(context.Background(), "ctg-taxprep-partnerdatatotax", "")
	testCases := []struct {
		name                    string
		registryClient          registry.IdentityConfiguration
		clientCluster           string
		isServerOnClientCluster bool
		clientAssets            []map[string]string
		expectedNamespaces      []string
	}{
		{
			name: "Given asset info, cluster info, and client info, " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed dependent namespaces should include istio-system",
			registryClient:          registry.NewRegistryClient(registry.WithRegistryEndpoint("PLACEHOLDER")),
			clientCluster:           "cg-tax-ppd-usw2-k8s",
			isServerOnClientCluster: true,
			clientAssets:            []map[string]string{{"name": "sample"}},
			expectedNamespaces:      []string{"ctg-taxprep-partnerdatatotax-usw2-e2e", "ctg-taxprep-partnerdatatotax-usw2-prf", "ctg-taxprep-partnerdatatotax-usw2-qal", "istio-system"},
		},
		{
			name: "Given asset info, cluster info, and client info, " +
				"When the client cluster is not the same as the server cluster" +
				"Then the constructed dependent namespaces should not include istio-system",
			registryClient:          registry.NewRegistryClient(registry.WithRegistryEndpoint("PLACEHOLDER")),
			clientCluster:           "cg-tax-ppd-usw2-k8s",
			isServerOnClientCluster: false,
			clientAssets:            []map[string]string{{"name": "sample"}},
			expectedNamespaces:      []string{"ctg-taxprep-partnerdatatotax-usw2-e2e", "ctg-taxprep-partnerdatatotax-usw2-prf", "ctg-taxprep-partnerdatatotax-usw2-qal"},
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			namespaces, err := getExportTo(ctxLogger, c.registryClient, c.clientCluster, c.isServerOnClientCluster, c.clientAssets)
			if err != nil {
				t.Errorf("While constructing sorted dependent namespaces, got error: %v", err)
			}
			if !cmp.Equal(namespaces, c.expectedNamespaces) {
				t.Errorf("Mismatch between constructed sortedDependentNamespaces and expected sortedDependentNamespaces")
				t.Errorf(cmp.Diff(namespaces, c.expectedNamespaces))
			}
		})
	}
}

func TestBuildServiceEntriesFromIdentityConfig(t *testing.T) {
	admiralParams := admiralParamsForConfigWriterTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	rr, _ := InitAdmiralOperator(context.Background(), admiralParams)
	ctxLogger := common.GetCtxLogger(context.Background(), "ctg-taxprep-partnerdatatotax", "")
	identityConfig := registry.GetSampleIdentityConfig()
	expectedLocalServiceEntryprf := createMockServiceEntry("prf", "Intuit.ctg.taxprep.partnerdatatotax", "partner-data-to-tax-spk-root-service.ctg-taxprep-partnerdatatotax-usw2-prf.svc.cluster.local.", 8090, []string{"ctg-taxprep-partnerdatatotax-usw2-e2e", "ctg-taxprep-partnerdatatotax-usw2-prf", "ctg-taxprep-partnerdatatotax-usw2-qal", "istio-system"})
	expectedLocalServiceEntrye2e := createMockServiceEntry("e2e", "Intuit.ctg.taxprep.partnerdatatotax", "partner-data-to-tax-spk-root-service.ctg-taxprep-partnerdatatotax-usw2-e2e.svc.cluster.local.", 8090, []string{"ctg-taxprep-partnerdatatotax-usw2-e2e", "ctg-taxprep-partnerdatatotax-usw2-prf", "ctg-taxprep-partnerdatatotax-usw2-qal", "istio-system"})
	expectedLocalServiceEntryqal := createMockServiceEntry("qal", "Intuit.ctg.taxprep.partnerdatatotax", "partner-data-to-tax-spk-root-service.ctg-taxprep-partnerdatatotax-usw2-qal.svc.cluster.local.", 8090, []string{"ctg-taxprep-partnerdatatotax-usw2-e2e", "ctg-taxprep-partnerdatatotax-usw2-prf", "ctg-taxprep-partnerdatatotax-usw2-qal", "istio-system"})
	expectedLocalServiceEntries := []*networkingV1Alpha3.ServiceEntry{&expectedLocalServiceEntryprf, &expectedLocalServiceEntrye2e, &expectedLocalServiceEntryqal}
	testCases := []struct {
		name                   string
		clientCluster          string
		identityConfig         registry.IdentityConfig
		expectedServiceEntries []*networkingV1Alpha3.ServiceEntry
	}{
		{
			name: "Given information to build an se, " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed se should have local endpoint and istio-system in exportTo",
			clientCluster:          "cg-tax-ppd-usw2-k8s",
			identityConfig:         identityConfig,
			expectedServiceEntries: expectedLocalServiceEntries,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			serviceEntryBuilder := ServiceEntryBuilder{ClientCluster: c.clientCluster, RemoteRegistry: rr}
			serviceEntries, err := serviceEntryBuilder.BuildServiceEntriesFromIdentityConfig(ctxLogger, c.identityConfig)
			if err != nil {
				t.Errorf("While constructing service entries, got error: %v", err)
			}
			opts := cmpopts.IgnoreUnexported(networkingV1Alpha3.ServiceEntry{}, networkingV1Alpha3.ServicePort{}, networkingV1Alpha3.WorkloadEntry{})
			if !cmp.Equal(serviceEntries, c.expectedServiceEntries, opts) {
				t.Errorf("Mismatch between constructed sorted entries and expected service entries")
				t.Errorf(cmp.Diff(serviceEntries, c.expectedServiceEntries, opts))
			}
		})
	}
}
