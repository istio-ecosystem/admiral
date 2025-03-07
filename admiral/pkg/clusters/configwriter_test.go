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
		RegistryClientHost:                  "registry.com",
		RegistryClientAppId:                 "registry-appid",
		RegistryClientAppSecret:             "registry-appsecret",
		RegistryClientBaseURI:               "v1",
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
			Labels:   map[string]string{"security.istio.io/tlsMode": "istio"}}},
		WorkloadSelector: nil,
		ExportTo:         exportTo,
		SubjectAltNames:  []string{"spiffe://prefix/" + identity},
	}
	return serviceEntry
}

func createMockServiceEntryWithTwoEndpoints(env string, identity string, endpointAddress string, endpointPort int, exportTo []string) networkingV1Alpha3.ServiceEntry {
	serviceEntry := networkingV1Alpha3.ServiceEntry{
		Hosts:      []string{env + "." + strings.ToLower(identity) + ".mesh"},
		Addresses:  nil,
		Ports:      []*networkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort), Name: util.Http, Protocol: util.Http}},
		Location:   1,
		Resolution: 2,
		Endpoints: []*networkingV1Alpha3.WorkloadEntry{{Address: endpointAddress,
			Locality: "us-west-2",
			Ports:    map[string]uint32{"http": uint32(endpointPort)},
			Labels:   map[string]string{"security.istio.io/tlsMode": "istio"}},
			{
				Address:  "def-elb.us-east-2.elb.amazonaws.com.",
				Locality: "us-east-2",
				Ports:    map[string]uint32{"http": uint32(15443)},
				Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
			}},
		WorkloadSelector: nil,
		ExportTo:         exportTo,
		SubjectAltNames:  []string{"spiffe://prefix/" + identity},
	}
	return serviceEntry
}

func createMockServiceEntryWithTwoLocalEndpoints(env string, identity string, endpointAddress1, endpointAddress2 string, endpointPort int, exportTo []string) networkingV1Alpha3.ServiceEntry {
	serviceEntry := networkingV1Alpha3.ServiceEntry{
		Hosts:      []string{env + "." + strings.ToLower(identity) + ".mesh"},
		Addresses:  nil,
		Ports:      []*networkingV1Alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort), Name: util.Http, Protocol: util.Http}},
		Location:   1,
		Resolution: 2,
		Endpoints: []*networkingV1Alpha3.WorkloadEntry{
			{
				Address:  endpointAddress1,
				Locality: "us-west-2",
				Ports:    map[string]uint32{"http": uint32(endpointPort)},
				Labels:   map[string]string{"security.istio.io/tlsMode": "istio"}},
			{
				Address:  endpointAddress2,
				Locality: "us-west-2",
				Ports:    map[string]uint32{"http": uint32(endpointPort)},
				Labels:   map[string]string{"security.istio.io/tlsMode": "istio"}},
			{
				Address:  "def-elb.us-east-2.elb.amazonaws.com.",
				Locality: "us-east-2",
				Ports:    map[string]uint32{"http": uint32(15443)},
				Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
			}},
		WorkloadSelector: nil,
		ExportTo:         exportTo,
		SubjectAltNames:  []string{"spiffe://prefix/" + identity},
	}
	sort.Sort(WorkloadEntrySorted(serviceEntry.Endpoints))
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

func TestGetServiceEntryEndpoints(t *testing.T) {
	admiralParams := admiralParamsForConfigWriterTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	e2eEnv := registry.GetSampleIdentityConfigEnvironment("e2e", "ns-1-usw2-e2e", "sample")
	host := "e2e.sample.mesh"
	unweightedDeployment := registry.GetSampleIdentityConfigEnvironment("e2e", "ns-1-usw2-e2e", "sample")
	unweightedDeployment.Type = map[string]*registry.TypeConfig{
		"deployment": {
			Selectors: map[string]string{"app": "app1"},
		},
	}
	weightedServices := map[string][]*registry.RegistryServiceConfig{
		"default": {

			{
				Name: "app-1-spk-stable-service",
				Ports: map[string]uint32{
					"http": 8090,
				},
				Selectors: map[string]string{"app": "app1"},
				Weight:    75,
			},
		},
		"test": {
			{
				Name: "app-1-spk-desired-service",
				Ports: map[string]uint32{
					"http": 8090,
				},
				Selectors: map[string]string{"app": "app1"},
				Weight:    25,
			},
		},
	}
	weightedRollout := registry.GetSampleIdentityConfigEnvironment("e2e", "ns-1-usw2-e2e", "sample")
	weightedRollout.Services = weightedServices
	weightedRollout.Type = map[string]*registry.TypeConfig{
		"rollout": {
			Selectors: map[string]string{"app": "app1"},
			Strategy:  canaryStrategy,
		},
	}
	ingressEndpoints := map[string]*networkingV1Alpha3.WorkloadEntry{"cluster1": {
		Address:  "abc-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}, "cluster2": {
		Address:  "def-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	remoteEndpoints := []*networkingV1Alpha3.WorkloadEntry{{
		Address:  "def-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	remoteDeploymentEndpoints := []*networkingV1Alpha3.WorkloadEntry{{
		Address:  "def-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	localEndpoints := []*networkingV1Alpha3.WorkloadEntry{{
		Address:  "app-1-spk-root-service.ns-1-usw2-e2e.svc.cluster.local.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(8090)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	weightedEndpoints := []*networkingV1Alpha3.WorkloadEntry{
		{
			Address:  "app-1-spk-desired-service.ns-1-usw2-e2e.svc.cluster.local.",
			Locality: "us-west-2",
			Ports:    map[string]uint32{"http": uint32(8090)},
			Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
			Weight:   25,
		},
		{
			Address:  "app-1-spk-stable-service.ns-1-usw2-e2e.svc.cluster.local.",
			Locality: "us-west-2",
			Ports:    map[string]uint32{"http": uint32(8090)},
			Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
			Weight:   75,
		},
	}
	ctx := context.Background()
	ctxLogger := common.GetCtxLogger(ctx, "sample", "")
	testCases := []struct {
		name                      string
		identityConfigEnvironment *registry.IdentityConfigEnvironment
		ingressEndpoints          map[string]*networkingV1Alpha3.WorkloadEntry
		clientCluster             string
		serverCluster             string
		expectedSEEndpoints       []*networkingV1Alpha3.WorkloadEntry
	}{
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint, " +
				"When the client cluster is not the same as the server cluster" +
				"Then the constructed endpoint should be a remote endpoint",
			identityConfigEnvironment: e2eEnv,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cluster1",
			serverCluster:             "cluster2",
			expectedSEEndpoints:       remoteEndpoints,
		},
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint, " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed endpoint should be a local endpoint",
			identityConfigEnvironment: e2eEnv,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cluster1",
			serverCluster:             "cluster1",
			expectedSEEndpoints:       localEndpoints,
		},
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint, " +
				"When the service is an unweighted deployment on a remote cluster" +
				"Then the constructed endpoint should be a remote endpoint",
			identityConfigEnvironment: unweightedDeployment,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cluster1",
			serverCluster:             "cluster2",
			expectedSEEndpoints:       remoteDeploymentEndpoints,
		},
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint, " +
				"When the rollout has weighted services on a local cluster" +
				"Then the constructed endpoints should weighted local endpoints",
			identityConfigEnvironment: weightedRollout,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cluster1",
			serverCluster:             "cluster1",
			expectedSEEndpoints:       weightedEndpoints,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			seEndpoints, err := getServiceEntryEndpoints(ctxLogger, c.clientCluster, c.serverCluster, host, c.ingressEndpoints, c.identityConfigEnvironment)
			if err != nil {
				t.Errorf("want=nil, got=%v", err)
			}
			opts := cmpopts.IgnoreUnexported(networkingV1Alpha3.WorkloadEntry{})
			if !cmp.Equal(seEndpoints, c.expectedSEEndpoints, opts) {
				t.Errorf("want=%v, got=%v", c.expectedSEEndpoints, seEndpoints)
			}
		})
	}
}

func TestGetServiceEntryEndpointsForCanaryAndBlueGreen(t *testing.T) {
	admiralParams := admiralParamsForConfigWriterTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	host := "e2e.sample.mesh"
	canaryRollout := registry.GetSampleIdentityConfigEnvironment("e2e", "ns-1-usw2-e2e", "sample")
	canaryServices := map[string][]*registry.RegistryServiceConfig{
		"default": {
			{
				Name: "app-1-spk-root-service",
				Ports: map[string]uint32{
					"http": 8090,
				},
				Selectors: map[string]string{"app": "app1"},
			},
			{
				Name: "app-1-spk-stable-service",
				Ports: map[string]uint32{
					"http": 8090,
				},
				Selectors: map[string]string{"app": "app1"},
			},
		},
		"test": {
			{
				Name: "app-1-spk-desired-service",
				Ports: map[string]uint32{
					"http": 8090,
				},
				Selectors: map[string]string{"app": "app1"},
			},
		},
	}
	canaryRollout.Services = canaryServices
	canaryRollout.Type = map[string]*registry.TypeConfig{
		"rollout": {
			Selectors: map[string]string{"app": "app1"},
			Strategy:  canaryStrategy,
		},
	}

	blueGreenRollout := registry.GetSampleIdentityConfigEnvironment("e2e", "ns-1-usw2-e2e", "sample")
	blueGreenServices := map[string][]*registry.RegistryServiceConfig{
		"test": {
			{Name: "app-1-spk-preview-service",
				Ports: map[string]uint32{
					"http": 8090,
				},
				Selectors: map[string]string{"app": "app1"}},
		},
		"default": {
			{Name: "app-1-spk-active-service",
				Ports: map[string]uint32{
					"http": 8090,
				},
				Selectors: map[string]string{"app": "app1"}},
		},
	}
	blueGreenRollout.Services = blueGreenServices
	blueGreenRollout.Type = map[string]*registry.TypeConfig{
		"rollout": {
			Selectors: map[string]string{"app": "app1"},
			Strategy:  bluegreenStrategy,
		},
	}
	ingressEndpoints := map[string]*networkingV1Alpha3.WorkloadEntry{"cluster1": {
		Address:  "abc-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}, "cluster2": {
		Address:  "def-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}

	localEndpointsWithDesiredService := []*networkingV1Alpha3.WorkloadEntry{{
		Address:  "app-1-spk-desired-service.ns-1-usw2-e2e.svc.cluster.local.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(8090)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	localEndpointsWithPreviewService := []*networkingV1Alpha3.WorkloadEntry{{
		Address:  "app-1-spk-preview-service.ns-1-usw2-e2e.svc.cluster.local.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(8090)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	localEndpointsWithActiveService := []*networkingV1Alpha3.WorkloadEntry{{
		Address:  "app-1-spk-active-service.ns-1-usw2-e2e.svc.cluster.local.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(8090)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	remoteEndpoints := []*networkingV1Alpha3.WorkloadEntry{{
		Address:  "def-elb.us-west-2.elb.amazonaws.com.",
		Locality: "us-west-2",
		Ports:    map[string]uint32{"http": uint32(15443)},
		Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
	}}
	ctx := context.Background()
	ctxLogger := common.GetCtxLogger(ctx, "sample", "")
	testCases := []struct {
		name                      string
		identityConfigEnvironment *registry.IdentityConfigEnvironment
		ingressEndpoints          map[string]*networkingV1Alpha3.WorkloadEntry
		clientCluster             string
		serverCluster             string
		host                      string
		expectedSEEndpoints       []*networkingV1Alpha3.WorkloadEntry
	}{
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint and a desired service, " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed endpoint for canary host should be a local endpoint with desired service",
			identityConfigEnvironment: canaryRollout,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cluster1",
			serverCluster:             "cluster1",
			host:                      "canary.e2e.sample.mesh",
			expectedSEEndpoints:       localEndpointsWithDesiredService,
		},
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint and a desired service, " +
				"When the client cluster is the different from the server cluster" +
				"Then the constructed endpoint for canary host should be a remote endpoint of cluster 2",
			identityConfigEnvironment: canaryRollout,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cluster1",
			serverCluster:             "cluster2",
			host:                      host,
			expectedSEEndpoints:       remoteEndpoints,
		},
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint and a preview and active service for a bluegreen rollout, " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed endpoint for preview host should be a local endpoint with preview service",
			identityConfigEnvironment: blueGreenRollout,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cluster1",
			serverCluster:             "cluster1",
			host:                      "preview.e2e.sample.mesh",
			expectedSEEndpoints:       localEndpointsWithPreviewService,
		},
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint and a preview and active service for a bluegreen rollout, " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed endpoint for host should be a local endpoint with active service",
			identityConfigEnvironment: blueGreenRollout,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cluster1",
			serverCluster:             "cluster1",
			host:                      host,
			expectedSEEndpoints:       localEndpointsWithActiveService,
		},
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint and a preview and active service for a bluegreen rollout, " +
				"When the client cluster is the different from the server cluster" +
				"Then the constructed endpoint for  host should be a remote endpoint of cluster 2",
			identityConfigEnvironment: blueGreenRollout,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cluster1",
			serverCluster:             "cluster2",
			host:                      host,
			expectedSEEndpoints:       remoteEndpoints,
		},
		{
			name: "Given an IdentityConfigEnvironment and ingressEndpoint and a preview and active service for a bluegreen rollout, " +
				"When the client cluster is the different from the server cluster" +
				"Then the constructed endpoint for  host should be a remote endpoint of cluster 2",
			identityConfigEnvironment: blueGreenRollout,
			ingressEndpoints:          ingressEndpoints,
			clientCluster:             "cluster1",
			serverCluster:             "cluster2",
			host:                      "preview.e2e.sample.mesh",
			expectedSEEndpoints:       remoteEndpoints,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			seEndpoints, err := getServiceEntryEndpoints(ctxLogger, c.clientCluster, c.serverCluster, c.host, c.ingressEndpoints, c.identityConfigEnvironment)
			if err != nil {
				t.Errorf("want=nil, got=%v", err)
			}
			opts := cmpopts.IgnoreUnexported(networkingV1Alpha3.WorkloadEntry{})
			if !cmp.Equal(seEndpoints, c.expectedSEEndpoints, opts) {
				t.Errorf("want=%v, got=%v", c.expectedSEEndpoints, seEndpoints)
			}
		})
	}
}

func TestGetExportTo(t *testing.T) {
	admiralParams := admiralParamsForConfigWriterTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	registryClientParams := common.GetRegistryClientConfig()
	defaultRegistryClientConfig := &util.Config{
		Host:      registryClientParams["Host"],
		AppId:     registryClientParams["AppId"],
		AppSecret: registryClientParams["AppSecret"],
		BaseURI:   registryClientParams["BaseURI"],
	}
	defaultRegistryClient := registry.NewRegistryClient(registry.WithBaseClientConfig(defaultRegistryClientConfig))
	ctxLogger := common.GetCtxLogger(context.Background(), "test", "")
	testCases := []struct {
		name                    string
		registryClient          registry.ClientAPI
		clientCluster           string
		isServerOnClientCluster bool
		clientAssets            map[string]string
		expectedNamespaces      []string
	}{
		{
			name: "Given asset info, cluster info, and client info, " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed dependent namespaces should include istio-system",
			registryClient:          defaultRegistryClient,
			clientCluster:           "cluster1",
			isServerOnClientCluster: true,
			clientAssets:            map[string]string{"sample": "sample"},
			expectedNamespaces:      []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"},
		},
		{
			name: "Given asset info, cluster info, and client info, " +
				"When the client cluster is not the same as the server cluster" +
				"Then the constructed dependent namespaces should not include istio-system",
			registryClient:          defaultRegistryClient,
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
	identityConfigFailsServiceEntryEndpoint.Clusters["cluster1"].Environment["e2e"].Services = make(map[string][]*registry.RegistryServiceConfig)
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

func TestBuildServiceEntriesFromIdentityConfig_MultipleEndpoints(t *testing.T) {
	admiralParams := admiralParamsForConfigWriterTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	rr := NewRemoteRegistry(context.Background(), admiralParams)
	ctxLogger := common.GetCtxLogger(context.Background(), "test", "")
	identityConfig := registry.GetSampleIdentityConfigWithRemoteEndpoints("sample")
	expectedLocalServiceEntryPRF := createMockServiceEntryWithTwoEndpoints("prf", "sample", "app-1-spk-root-service.ns-1-usw2-prf.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntryE2E := createMockServiceEntryWithTwoEndpoints("e2e", "sample", "app-1-spk-root-service.ns-1-usw2-e2e.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntryQAL := createMockServiceEntryWithTwoEndpoints("qal", "sample", "app-1-spk-root-service.ns-1-usw2-qal.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntries := []*networkingV1Alpha3.ServiceEntry{&expectedLocalServiceEntryQAL, &expectedLocalServiceEntryPRF, &expectedLocalServiceEntryE2E}

	identityConfigForMigration := registry.GetSampleIdentityConfigWithRolloutAndDeployment("sample")
	expectedLocalServiceEntryPRFForMigration := createMockServiceEntryWithTwoLocalEndpoints("prf", "sample", "app-1-spk-root-service.ns-1-usw2-prf.svc.cluster.local.", "app-1-spk-deploy-service.ns-1-usw2-prf.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntryE2EForMigration := createMockServiceEntryWithTwoLocalEndpoints("e2e", "sample", "app-1-spk-root-service.ns-1-usw2-e2e.svc.cluster.local.", "app-1-spk-deploy-service.ns-1-usw2-e2e.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntryQALForMigration := createMockServiceEntryWithTwoLocalEndpoints("qal", "sample", "app-1-spk-root-service.ns-1-usw2-qal.svc.cluster.local.", "app-1-spk-deploy-service.ns-1-usw2-qal.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntriesForMigration := []*networkingV1Alpha3.ServiceEntry{&expectedLocalServiceEntryQALForMigration, &expectedLocalServiceEntryPRFForMigration, &expectedLocalServiceEntryE2EForMigration}

	identityConfigForMigration2 := registry.GetSampleIdentityConfigWithRolloutAndDeployment("sample")
	identityConfigForMigration2.Clusters["cluster2"].Environment["prf"].Type = map[string]*registry.TypeConfig{
		"deployment": {
			Selectors: map[string]string{"app": "app1"},
		},
	}
	identityConfigForMigration2.Clusters["cluster2"].Environment["e2e"].Type = map[string]*registry.TypeConfig{
		"deployment": {
			Selectors: map[string]string{"app": "app1"},
		},
	}
	identityConfigForMigration2.Clusters["cluster2"].Environment["qal"].Type = map[string]*registry.TypeConfig{
		"deployment": {
			Selectors: map[string]string{"app": "app1"},
		},
	}
	expectedLocalServiceEntryPRFForMigration2 := createMockServiceEntryWithTwoLocalEndpoints("prf", "sample", "app-1-spk-root-service.ns-1-usw2-prf.svc.cluster.local.", "app-1-spk-deploy-service.ns-1-usw2-prf.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntryPRFForMigration2.Endpoints[2].Labels = map[string]string{"security.istio.io/tlsMode": "istio"}
	expectedLocalServiceEntryE2EForMigration2 := createMockServiceEntryWithTwoLocalEndpoints("e2e", "sample", "app-1-spk-root-service.ns-1-usw2-e2e.svc.cluster.local.", "app-1-spk-deploy-service.ns-1-usw2-e2e.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntryE2EForMigration2.Endpoints[2].Labels = map[string]string{"security.istio.io/tlsMode": "istio"}
	expectedLocalServiceEntryQALForMigration2 := createMockServiceEntryWithTwoLocalEndpoints("qal", "sample", "app-1-spk-root-service.ns-1-usw2-qal.svc.cluster.local.", "app-1-spk-deploy-service.ns-1-usw2-qal.svc.cluster.local.", 8090, []string{"istio-system", "ns-1-usw2-e2e", "ns-1-usw2-prf", "ns-1-usw2-qal"})
	expectedLocalServiceEntryQALForMigration2.Endpoints[2].Labels = map[string]string{"security.istio.io/tlsMode": "istio"}
	expectedLocalServiceEntriesForMigration2 := []*networkingV1Alpha3.ServiceEntry{&expectedLocalServiceEntryQALForMigration2, &expectedLocalServiceEntryPRFForMigration2, &expectedLocalServiceEntryE2EForMigration2}

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
				"Then the constructed se should have local endpoint and a remote endpoint and istio-system in exportTo",
			clientCluster:          "cluster1",
			event:                  admiral.Add,
			identityConfig:         identityConfig,
			expectedServiceEntries: expectedLocalServiceEntries,
			expectedErr:            false,
		},
		{
			name: "Given information to build an se has a rollout and deployment in the same namespace, and remote cluster having a rollout " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed se should have 2 local endpoints and a remote endpoint and istio-system in exportTo",
			clientCluster:          "cluster1",
			event:                  admiral.Add,
			identityConfig:         identityConfigForMigration,
			expectedServiceEntries: expectedLocalServiceEntriesForMigration,
			expectedErr:            false,
		},
		{
			name: "Given information to build an se has a rollout and deployment in the same namespace, and remote cluster having a deployment " +
				"When the client cluster is the same as the server cluster" +
				"Then the constructed se should have 2 local endpoints and a remote endpoint and istio-system in exportTo",
			clientCluster:          "cluster1",
			event:                  admiral.Add,
			identityConfig:         identityConfigForMigration2,
			expectedServiceEntries: expectedLocalServiceEntriesForMigration2,
			expectedErr:            false,
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
