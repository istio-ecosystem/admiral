package clusters

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	testMocks "github.com/istio-ecosystem/admiral/admiral/pkg/test"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	apiNetworkingV1Alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioFake "istio.io/client-go/pkg/clientset/versioned/fake"
	v1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// TODO: Refactor this test
// The code has been refactored so this test will fail
func TestAddUpdateVirtualServicesForSourceIngress(t *testing.T) {

	vsLabels := map[string]string{
		vsRoutingLabel: "enabled",
	}

	newVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-env.test-identity.global-routing-vs",
			Namespace: "test-sync-ns",
			Labels:    vsLabels,
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts:    []string{"test-env.test-identity.global"},
			Gateways: []string{"istio-system/passthrough-gateway"},
			ExportTo: []string{"istio-system"},
			Tls: []*networkingV1Alpha3.TLSRoute{
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
						{
							Port:     common.DefaultMtlsPort,
							SniHosts: []string{"outbound_.80_._.test-env.test-identity.global"},
						},
					},
				},
			},
		},
	}

	existingVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-env.test-identity.global-routing-vs",
			Namespace: "test-sync-ns",
			Labels:    vsLabels,
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts:    []string{"test-env.test-identity.global"},
			Gateways: []string{"istio-system/passthrough-gateway"},
			ExportTo: []string{"istio-system"},
			Tls: []*networkingV1Alpha3.TLSRoute{
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
						{
							Port:     common.DefaultMtlsPort,
							SniHosts: []string{"outbound_.80_._.test-env.test-identity.global"},
						},
					},
					Route: []*networkingV1Alpha3.RouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "test-env.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8080,
								},
							},
						},
					},
				},
			},
		},
	}

	admiralParams := common.AdmiralParams{
		LabelSet:              &common.LabelSet{},
		SyncNamespace:         "test-sync-ns",
		EnableSWAwareNSCaches: true,
	}

	istioClientWithExistingVS := istioFake.NewSimpleClientset()
	istioClientWithExistingVS.NetworkingV1alpha3().VirtualServices(admiralParams.SyncNamespace).
		Create(context.Background(), existingVS, metaV1.CreateOptions{})

	istioClientWithNoExistingVS := istioFake.NewSimpleClientset()
	rc := &RemoteController{
		ClusterID:                "cluster-1",
		VirtualServiceController: &istio.VirtualServiceController{},
	}

	rr := NewRemoteRegistry(context.Background(), admiralParams)
	rr.PutRemoteController("cluster-1", rc)

	sourceServices := make(map[string]map[string]*coreV1.Service)
	sourceServices["cluster-1"] = make(map[string]*coreV1.Service)
	sourceServices["cluster-1"][common.Deployment] = &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-deployment-svc",
			Namespace: "test-ns",
		},
		Spec: coreV1.ServiceSpec{
			Ports: []coreV1.ServicePort{
				{
					Name: "http",
					Port: 8080,
				},
			},
		},
	}
	sourceServices["cluster-1"][common.Rollout] = &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-rollout-svc",
			Namespace: "test-ns",
		},
		Spec: coreV1.ServiceSpec{
			Ports: []coreV1.ServicePort{
				{
					Name: "http",
					Port: 8080,
				},
			},
		},
	}

	rollout := &v1alpha1.Rollout{
		Spec: v1alpha1.RolloutSpec{
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: map[string]string{
						"identity":                 "test-identity",
						"env":                      "test-env",
						common.SidecarEnabledPorts: "8080",
					},
				},
			},
		},
	}
	deployment := &v1.Deployment{
		Spec: v1.DeploymentSpec{
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: map[string]string{
						"identity":                 "test-identity",
						"env":                      "test-env",
						common.SidecarEnabledPorts: "8080",
					},
				},
			},
		},
	}

	ctxLogger := log.WithFields(log.Fields{
		"type": "VirtualService",
	})

	testCases := []struct {
		name                        string
		remoteRegistry              *RemoteRegistry
		sourceServices              map[string]map[string]*coreV1.Service
		sourceIngressVirtualService map[string]*apiNetworkingV1Alpha3.VirtualService
		sourceDeployment            map[string]*v1.Deployment
		sourceRollout               map[string]*v1alpha1.Rollout
		istioClient                 *istioFake.Clientset
		expectedError               error
		expectedVS                  *apiNetworkingV1Alpha3.VirtualService
	}{
		{
			name: "Given a nil remoteRegistry, " +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should return an error",
			expectedError: fmt.Errorf("remoteRegistry is nil"),
		},
		{
			name: "Given a no sourceServices, " +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should return an error",
			remoteRegistry: rr,
			sourceServices: map[string]map[string]*coreV1.Service{"cluster-1": {}},
			expectedError:  fmt.Errorf("no destination found for the ingress virtualservice"),
		},
		{
			name: "Given a invalid sourceIngressVirtualService, " +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should return an error",
			remoteRegistry:   rr,
			sourceServices:   sourceServices,
			sourceRollout:    map[string]*v1alpha1.Rollout{"cluster-1": rollout},
			sourceDeployment: map[string]*v1.Deployment{"cluster-1": deployment},
			sourceIngressVirtualService: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": {
					Spec: networkingV1Alpha3.VirtualService{},
				},
			},
			expectedError: fmt.Errorf("no TLSRoute found in the ingress virtualservice with host test-env.test-identity.global"),
		},
		{
			name: "Given a valid sourceIngressVirtualService, " +
				"And the VS is a new VS" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should successfully create the VS",
			remoteRegistry:   rr,
			sourceRollout:    map[string]*v1alpha1.Rollout{"cluster-1": rollout},
			sourceDeployment: map[string]*v1.Deployment{"cluster-1": deployment},
			istioClient:      istioClientWithNoExistingVS,
			sourceServices:   sourceServices,
			sourceIngressVirtualService: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": newVS,
			},
			expectedError: nil,
		},
		{
			name: "Given a valid sourceIngressVirtualService, " +
				"And there is VS with same name already exists" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should successfully update the VS",
			remoteRegistry:   rr,
			sourceRollout:    map[string]*v1alpha1.Rollout{"cluster-1": rollout},
			sourceDeployment: map[string]*v1.Deployment{"cluster-1": deployment},
			istioClient:      istioClientWithExistingVS,
			sourceServices:   sourceServices,
			sourceIngressVirtualService: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": newVS,
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rc := rr.GetRemoteController("cluster-1")
			rc.VirtualServiceController.IstioClient = tc.istioClient
			rr.PutRemoteController("cluster-1", rc)
			err := addUpdateVirtualServicesForSourceIngress(
				context.Background(),
				ctxLogger,
				tc.remoteRegistry,
				tc.sourceIngressVirtualService,
				nil)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
			}
		})
	}

}

func TestGenerateSNIHost(t *testing.T) {
	testCases := []struct {
		name            string
		host            string
		expectedSNIHost string
		expectedError   error
	}{
		{
			name: "Given an empty host, " +
				"When generateSNIHost is invoked, " +
				"Then it should return an error",
			host:          "",
			expectedError: fmt.Errorf("fqdn is empty"),
		},
		{
			name: "Given a host, " +
				"When generateSNIHost is invoked, " +
				"Then it should return the SNI host",
			host:            "test-env.test-identity.global",
			expectedSNIHost: "outbound_.80_._.test-env.test-identity.global",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sniHost, err := generateSNIHost(tc.host)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedSNIHost, sniHost)
			}
		})
	}
}

func TestGetFQDNFromSNIHost(t *testing.T) {

	testCases := []struct {
		name          string
		sniHost       string
		expectedHost  string
		expectedError error
	}{
		{
			name: "Given an empty sniHost, " +
				"When getFQDNFromSNIHost is invoked, " +
				"Then it should return an error",
			sniHost:       "",
			expectedError: fmt.Errorf("sniHost is empty"),
		},
		{
			name: "Given an invalid sniHost, " +
				"When getFQDNFromSNIHost is invoked, " +
				"Then it should return an error",
			sniHost:       "test-env.test-identity.global",
			expectedError: fmt.Errorf("invalid SNI host format, should start with outbound_"),
		},
		{
			name: "Given a valid sniHost, " +
				"When generateSNIHost is invoked, " +
				"Then it should return the host",
			sniHost:      "outbound_.80_._.test-env.test-identity.global",
			expectedHost: "test-env.test-identity.global",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host, err := getFQDNFromSNIHost(tc.sniHost)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedHost, host)
			}
		})
	}

}

func TestGenerateIngressVirtualServiceForRollout(t *testing.T) {

	admiralParams := common.AdmiralParams{
		SyncNamespace: "test-sync-ns",
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix:              "global",
		VSRoutingGateways:           []string{"istio-system/passthrough-gateway"},
		IngressVSExportToNamespaces: []string{"istio-system"},
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	vsLabels := map[string]string{
		vsRoutingLabel: "enabled",
	}

	validVSWithDefaultFQDN := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-env.test-identity.global-routing-vs",
			Namespace: "test-sync-ns",
			Labels:    vsLabels,
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts:    []string{"outbound_.80_._.test-env.test-identity.global"},
			Gateways: []string{"istio-system/passthrough-gateway"},
			ExportTo: []string{"istio-system"},
			Tls: []*networkingV1Alpha3.TLSRoute{
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
						{
							Port:     common.DefaultMtlsPort,
							SniHosts: []string{"outbound_.80_._.test-env.test-identity.global"},
						},
					},
				},
			},
		},
	}
	validVSWithPreviewFQDN := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-env.test-identity.global-routing-vs",
			Namespace: "test-sync-ns",
			Labels:    vsLabels,
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts: []string{
				"test-env.test-identity.global",
				"preview.test-env.test-identity.global",
			},
			Gateways: []string{"istio-system/passthrough-gateway"},
			ExportTo: []string{"istio-system"},
			Tls: []*networkingV1Alpha3.TLSRoute{
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
						{
							Port:     common.DefaultMtlsPort,
							SniHosts: []string{"outbound_.80_._.test-env.test-identity.global"},
						},
						{
							Port:     common.DefaultMtlsPort,
							SniHosts: []string{"outbound_.80_._.preview.test-env.test-identity.global"},
						},
					},
				},
			},
		},
	}
	validVSWithCanaryFQDN := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-env.test-identity.global-routing-vs",
			Namespace: "test-sync-ns",
			Labels:    vsLabels,
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts: []string{
				"test-env.test-identity.global",
				"canary.test-env.test-identity.global",
			},
			Gateways: []string{"istio-system/passthrough-gateway"},
			ExportTo: []string{"istio-system"},
			Tls: []*networkingV1Alpha3.TLSRoute{
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
						{
							Port:     common.DefaultMtlsPort,
							SniHosts: []string{"outbound_.80_._.test-env.test-identity.global"},
						},
						{
							Port:     common.DefaultMtlsPort,
							SniHosts: []string{"outbound_.80_._.canary.test-env.test-identity.global"},
						},
					},
				},
			},
		},
	}
	stop := make(chan struct{})
	config := rest.Config{
		Host: "localhost",
	}
	serviceController, _ := admiral.NewServiceController(
		stop, &testMocks.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	rc := &RemoteController{
		ServiceController: serviceController,
	}
	previewService := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "previewService",
			Namespace: "test-ns",
		},
		Spec: coreV1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []coreV1.ServicePort{
				{
					Name: "http",
					Port: 8080,
				},
			},
		},
	}
	canaryService := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "canaryService",
			Namespace: "test-ns",
		},
		Spec: coreV1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []coreV1.ServicePort{
				{
					Name: "http",
					Port: 8080,
				},
			},
		},
	}
	serviceController.Cache.Put(previewService)
	serviceController.Cache.Put(canaryService)

	testCases := []struct {
		name                    string
		rollout                 *v1alpha1.Rollout
		expectedSourceIngressVS map[string]*apiNetworkingV1Alpha3.VirtualService
		expectedError           error
	}{
		{
			name: "Given a empty rollout, " +
				"When generateIngressVirtualServiceForRollout is invoked, " +
				"Then it should return an error",
			expectedError: fmt.Errorf("rollout is nil"),
		},
		{
			name: "Given a invalid rollout," +
				"When generateIngressVirtualServiceForRollout is invoked, " +
				"Then it should return an error",
			rollout:       &v1alpha1.Rollout{},
			expectedError: fmt.Errorf("cname is empty"),
		},
		{
			name: "Given a valid rollout," +
				"And has no bluegreen or canary strategy" +
				"When generateIngressVirtualServiceForRollout is invoked, " +
				"Then it should populate the sourceIngressVS with only contain default hostname",
			rollout: &v1alpha1.Rollout{
				Spec: v1alpha1.RolloutSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{"identity": "test-identity", "env": "test-env"},
						},
					},
				},
			},
			expectedSourceIngressVS: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": validVSWithDefaultFQDN,
			},
			expectedError: nil,
		},
		{
			name: "Given a valid rollout," +
				"And has bluegreen strategy" +
				"When generateIngressVirtualServiceForRollout is invoked, " +
				"Then it should populate the sourceIngressVS with default and preview hosts",
			rollout: &v1alpha1.Rollout{
				ObjectMeta: metaV1.ObjectMeta{Namespace: "test-ns"},
				Spec: v1alpha1.RolloutSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity":                 "test-identity",
								"env":                      "test-env",
								common.SidecarEnabledPorts: "8080",
							},
						},
					},
					Strategy: v1alpha1.RolloutStrategy{
						BlueGreen: &v1alpha1.BlueGreenStrategy{
							PreviewService: "previewService",
						},
					},
					Selector: &metaV1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			},
			expectedSourceIngressVS: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": validVSWithPreviewFQDN,
			},
			expectedError: nil,
		},
		{
			name: "Given a valid rollout," +
				"And has canary strategy" +
				"When generateIngressVirtualServiceForRollout is invoked, " +
				"Then it should populate the sourceIngressVS with default and canary hosts",
			rollout: &v1alpha1.Rollout{
				ObjectMeta: metaV1.ObjectMeta{Namespace: "test-ns"},
				Spec: v1alpha1.RolloutSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity":                 "test-identity",
								"env":                      "test-env",
								common.SidecarEnabledPorts: "8080",
							},
						},
					},
					Strategy: v1alpha1.RolloutStrategy{
						Canary: &v1alpha1.CanaryStrategy{
							CanaryService: "canaryService",
							TrafficRouting: &v1alpha1.RolloutTrafficRouting{
								Istio: &v1alpha1.IstioTrafficRouting{},
							},
						},
					},
					Selector: &metaV1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			},
			expectedSourceIngressVS: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": validVSWithCanaryFQDN,
			},
			expectedError: nil,
		},
	}

	ctxLogger := log.WithFields(log.Fields{
		"type": "VirtualService",
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sourceIngressVS := make(map[string]*apiNetworkingV1Alpha3.VirtualService)
			err := generateIngressVirtualServiceForRollout(
				context.Background(),
				ctxLogger,
				tc.rollout,
				sourceIngressVS,
				rc)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedSourceIngressVS, sourceIngressVS)
			}
		})
	}

}

func TestGenerateIngressVirtualServiceForDeployment(t *testing.T) {

	admiralParams := common.AdmiralParams{
		SyncNamespace: "test-sync-ns",
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix:              "global",
		VSRoutingGateways:           []string{"istio-system/passthrough-gateway"},
		IngressVSExportToNamespaces: []string{"istio-system"},
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	vsLabels := map[string]string{
		vsRoutingLabel: "enabled",
	}

	validVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-env.test-identity.global-routing-vs",
			Namespace: "test-sync-ns",
			Labels:    vsLabels,
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts:    []string{"outbound_.80_._.test-env.test-identity.global"},
			Gateways: []string{"istio-system/passthrough-gateway"},
			ExportTo: []string{"istio-system"},
			Tls: []*networkingV1Alpha3.TLSRoute{
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
						{
							Port:     common.DefaultMtlsPort,
							SniHosts: []string{"outbound_.80_._.test-env.test-identity.global"},
						},
					},
				},
			},
		},
	}

	testCases := []struct {
		name                    string
		deployment              *v1.Deployment
		expectedSourceIngressVS map[string]*apiNetworkingV1Alpha3.VirtualService
		expectedError           error
	}{
		{
			name: "Given a empty deployment, " +
				"When generateIngressVirtualServiceForDeployment is invoked, " +
				"Then it should return an error",
			expectedError: fmt.Errorf("deployment is nil"),
		},
		{
			name: "Given a invalid deployment," +
				"When generateIngressVirtualServiceForDeployment is invoked, " +
				"Then it should return an error",
			deployment:    &v1.Deployment{},
			expectedError: fmt.Errorf("cname is empty"),
		},
		{
			name: "Given a valid deployment," +
				"When generateIngressVirtualServiceForDeployment is invoked, " +
				"Then it should populate the sourceIngressVS map correctly",
			deployment: &v1.Deployment{
				Spec: v1.DeploymentSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{"identity": "test-identity", "env": "test-env"},
						},
					},
				},
			},
			expectedSourceIngressVS: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": validVS,
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sourceIngressVS := make(map[string]*apiNetworkingV1Alpha3.VirtualService)
			err := generateIngressVirtualServiceForDeployment(tc.deployment, sourceIngressVS)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedSourceIngressVS, sourceIngressVS)
			}
		})
	}

}

func TestGetBaseVirtualServiceForIngress(t *testing.T) {

	admiralParams := common.AdmiralParams{
		SyncNamespace:               "test-sync-ns",
		IngressVSExportToNamespaces: []string{"istio-system"},
	}

	vsLabels := map[string]string{
		vsRoutingLabel: "enabled",
	}

	validVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-host-routing-vs",
			Namespace: "test-sync-ns",
			Labels:    vsLabels,
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts:    []string{"test-sni-host"},
			Gateways: []string{"istio-system/passthrough-gateway"},
			ExportTo: []string{"istio-system"},
			Tls: []*networkingV1Alpha3.TLSRoute{
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
						{
							Port:     common.DefaultMtlsPort,
							SniHosts: []string{"test-sni-host"},
						},
					},
				},
			},
		},
	}

	testCases := []struct {
		name            string
		hosts           []string
		sniHosts        []string
		routingGateways []string
		expectedVS      *apiNetworkingV1Alpha3.VirtualService
		expectedError   error
	}{
		{
			name: "Given a empty host, " +
				"When getBaseVirtualServiceForIngress is invoked, " +
				"Then it should return an error",
			sniHosts:      []string{"test-sni-host"},
			expectedError: fmt.Errorf("host is empty"),
		},
		{
			name: "Given a empty sniHost, " +
				"When getBaseVirtualServiceForIngress is invoked, " +
				"Then it should return an error",
			hosts:         []string{"test-host"},
			expectedError: fmt.Errorf("sniHost is empty"),
		},
		{
			name: "Given a valid host and sniHost," +
				"And gateways are not configured " +
				"When getBaseVirtualServiceForIngress is invoked, " +
				"Then it should return an error",
			hosts:           []string{"test-host"},
			sniHosts:        []string{"test-sni-host"},
			routingGateways: []string{},
			expectedError:   fmt.Errorf("no gateways configured for ingress virtual service"),
		},
		{
			name: "Given a valid host and sniHost," +
				"When getBaseVirtualServiceForIngress is invoked, " +
				"Then it should return the expected VirtualService",
			hosts:           []string{"test-host"},
			sniHosts:        []string{"test-sni-host"},
			routingGateways: []string{"istio-system/passthrough-gateway"},
			expectedError:   nil,
			expectedVS:      validVS,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			admiralParams.VSRoutingGateways = tc.routingGateways
			common.ResetSync()
			common.InitializeConfig(admiralParams)
			actual, err := getBaseVirtualServiceForIngress(tc.hosts, tc.sniHosts)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedVS, actual)
			}
		})
	}

}
