package clusters

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
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
	identityConfig := registry.GetSampleIdentityConfig("sample")
	expectedIngressEndpoints := map[string]*networkingV1Alpha3.WorkloadEntry{
		"cluster1": {
			Address:  "abc-elb.us-west-2.elb.amazonaws.com.",
			Locality: "us-west-2",
			Ports:    map[string]uint32{"http": uint32(15443)},
			Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
		},
	}
	testCases := []struct {
		name                     string
		identityConfigClusters   map[string]*registry.IdentityConfigCluster
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
				t.Errorf("want=nil, got=%v", err)
			}
			if !reflect.DeepEqual(ingressEndpoints, c.expectedIngressEndpoints) {
				t.Errorf("want=%v, got=%v", c.expectedIngressEndpoints, ingressEndpoints)
			}
		})
	}
}

func TestGetServiceEntryEndpoint(t *testing.T) {
	admiralParams := admiralParamsForConfigWriterTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	e2eEnv := registry.GetSampleIdentityConfigEnvironment("e2e", "ns-1-usw2-e2e", "sample")
	ingressEndpoints := map[string]*networkingV1Alpha3.WorkloadEntry{"cluster1": {
		Address:  "abc-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}, "apigw-cx-ppd-usw2-k8s": {
		Address:  "abc-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	remoteEndpoint := &networkingV1Alpha3.WorkloadEntry{
		Address:  "abc-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio", "type": "rollout"},
	}
	localEndpoint := &networkingV1Alpha3.WorkloadEntry{
		Address:  "app-1-spk-root-service.ns-1-usw2-e2e.svc.cluster.local.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(8090)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio", "type": "rollout"},
	}
	ctx := context.Background()
	ctxLogger := common.GetCtxLogger(ctx, "ctg-taxprep-partnerdatatotax", "")
	testCases := []struct {
		name                      string
		identityConfigEnvironment *registry.IdentityConfigEnvironment
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
			clientCluster:             "cluster1",
			serverCluster:             "apigw-cx-ppd-usw2-k8s",
			expectedSEEndpoint:        remoteEndpoint,
		},
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint, " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed endpoint should be a local endpoint",
			identityConfigEnvironment: e2eEnv,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cluster1",
			serverCluster:             "cluster1",
			expectedSEEndpoint:        localEndpoint,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			seEndpoint, err := getServiceEntryEndpoint(ctxLogger, c.clientCluster, c.serverCluster, c.ingressEndpoints, c.identityConfigEnvironment)
			if err != nil {
				t.Errorf("want=nil, got=%v", err)
			}
			opts := cmpopts.IgnoreUnexported(networkingV1Alpha3.WorkloadEntry{})
			if !cmp.Equal(seEndpoint, c.expectedSEEndpoint, opts) {
				t.Errorf("want=%v, got=%v", c.expectedSEEndpoint, seEndpoint)
			}
		})
	}
}

func TestGetExportTo(t *testing.T) {
	admiralParams := admiralParamsForConfigWriterTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	ctxLogger := common.GetCtxLogger(context.Background(), "test", "")
	testCases := []struct {
		name                    string
		registryClient          registry.IdentityConfiguration
		clientCluster           string
		isServerOnClientCluster bool
		clientAssets            map[string]string
		expectedNamespaces      []string
	}{
		{
			name: "Given asset info, cluster info, and client info, " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed dependent namespaces should include istio-system",
			registryClient:          registry.NewRegistryClient(registry.WithRegistryEndpoint("PLACEHOLDER")),
			clientCluster:           "cluster1",
			isServerOnClientCluster: true,
			clientAssets:            map[string]string{"sample": "sample"},
			expectedNamespaces:      []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"},
		},
		{
			name: "Given asset info, cluster info, and client info, " +
				"When the client cluster is not the same as the server cluster" +
				"Then the constructed dependent namespaces should not include istio-system",
			registryClient:          registry.NewRegistryClient(registry.WithRegistryEndpoint("PLACEHOLDER")),
			clientCluster:           "cluster1",
			isServerOnClientCluster: false,
			clientAssets:            map[string]string{"sample": "sample"},
			expectedNamespaces:      []string{"ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"},
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			namespaces, err := getExportTo(ctxLogger, c.registryClient, c.clientCluster, c.isServerOnClientCluster, c.clientAssets)
			if err != nil {
				t.Errorf("want=%v, got=%v", nil, err)
			}
			if !cmp.Equal(namespaces, c.expectedNamespaces) {
				t.Errorf("want=%v, got=%v", c.expectedNamespaces, namespaces)
			}
		})
	}
}

func TestBuildServiceEntriesFromIdentityConfig(t *testing.T) {
	admiralParams := admiralParamsForConfigWriterTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	rr, _ := InitAdmiralOperator(context.Background(), admiralParams)
	ctxLogger := common.GetCtxLogger(context.Background(), "test", "")
	identityConfig := registry.GetSampleIdentityConfig("sample")
	expectedLocalServiceEntryPRF := createMockServiceEntry("prf", "sample", "app-1-spk-root-service.ns-1-usw2-prf.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntryE2E := createMockServiceEntry("e2e", "sample", "app-1-spk-root-service.ns-1-usw2-e2e.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntryQAL := createMockServiceEntry("qal", "sample", "app-1-spk-root-service.ns-1-usw2-qal.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntries := []*networkingV1Alpha3.ServiceEntry{&expectedLocalServiceEntryQAL, &expectedLocalServiceEntryPRF, &expectedLocalServiceEntryE2E}
	identityConfigFailsIngressEndpoints := registry.GetSampleIdentityConfig("sample")
	identityConfigFailsIngressEndpoints.Clusters = map[string]*registry.IdentityConfigCluster{}
	identityConfigFailsExportTo := registry.GetSampleIdentityConfig("sample")
	identityConfigFailsExportTo.ClientAssets["fake"] = "fake"
	identityConfigFailsServiceEntryEndpoint := registry.GetSampleIdentityConfig("sample")
	identityConfigFailsServiceEntryEndpoint.Clusters["cluster1"].Environment["e2e"].Services = make(map[string]*registry.RegistryServiceConfig)
	testCases := []struct {
		name                   string
		clientCluster          string
		event                  admiral.EventType
		identityConfig         registry.IdentityConfig
		expectedServiceEntries []*networkingV1Alpha3.ServiceEntry
		expectedErr            bool
	}{
		{
			name: "Given information to build an se, " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed se should have local endpoint and istio-system in exportTo",
			clientCluster:          "cluster1",
			event:                  admiral.Add,
			identityConfig:         identityConfig,
			expectedServiceEntries: expectedLocalServiceEntries,
			expectedErr:            false,
		},
		{
			name: "Given getIngressEndpoints fails with a non-nil error or is empty, " +
				"Then there should be an empty array of returned service entries",
			clientCluster:          "cluster1",
			event:                  admiral.Add,
			identityConfig:         identityConfigFailsIngressEndpoints,
			expectedServiceEntries: make([]*networkingV1Alpha3.ServiceEntry, 0),
			expectedErr:            false,
		},
		{
			name: "Given getExportTo fails with a non-nil error, " +
				"Then there should be an empty array of returned service entries",
			clientCluster:          "cluster1",
			event:                  admiral.Add,
			identityConfig:         identityConfigFailsExportTo,
			expectedServiceEntries: make([]*networkingV1Alpha3.ServiceEntry, 0),
			expectedErr:            true,
		},
		{
			name: "Given getServiceEntryEndpoint fails with a non-nil error, " +
				"Then there should be an empty array of returned service entries",
			clientCluster:          "cluster1",
			event:                  admiral.Add,
			identityConfig:         identityConfigFailsServiceEntryEndpoint,
			expectedServiceEntries: make([]*networkingV1Alpha3.ServiceEntry, 0),
			expectedErr:            true,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			serviceEntryBuilder := ServiceEntryBuilder{ClientCluster: c.clientCluster, RemoteRegistry: rr}
			serviceEntries, err := serviceEntryBuilder.BuildServiceEntriesFromIdentityConfig(ctxLogger, c.identityConfig)
			if err != nil && !c.expectedErr {
				t.Errorf("want=%v, \ngot=%v", nil, err)
			}
			opts := cmpopts.IgnoreUnexported(networkingV1Alpha3.ServiceEntry{}, networkingV1Alpha3.ServicePort{}, networkingV1Alpha3.WorkloadEntry{})
			// sort the service entries by name
			sort.Sort(ServiceEntryListSorterByHost(serviceEntries))
			sort.Sort(ServiceEntryListSorterByHost(c.expectedServiceEntries))
			if !cmp.Equal(serviceEntries, c.expectedServiceEntries, opts) {
				t.Errorf("want=%v, \ngot=%v", c.expectedServiceEntries, serviceEntries)
			}
		})
	}
}

type ServiceEntryListSorterByHost []*networkingV1Alpha3.ServiceEntry

func (s ServiceEntryListSorterByHost) Len() int {
	return len(s)
}

func (w ServiceEntryListSorterByHost) Less(i, j int) bool {
	return w[i].Hosts[0] < w[j].Hosts[0]
}

func (w ServiceEntryListSorterByHost) Swap(i, j int) {
	w[i], w[j] = w[j], w[i]
}
