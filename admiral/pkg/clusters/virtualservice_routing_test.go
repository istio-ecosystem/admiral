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

	newVSWithPreviewEndpoint := &apiNetworkingV1Alpha3.VirtualService{
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
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
						{
							Port:     common.DefaultMtlsPort,
							SniHosts: []string{"outbound_.80_._.preview.test-env.test-identity.global"},
						},
					},
				},
			},
		},
	}
	newVSWithCanaryEndpoint := &apiNetworkingV1Alpha3.VirtualService{
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
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
						{
							Port:     common.DefaultMtlsPort,
							SniHosts: []string{"outbound_.80_._.canary.test-env.test-identity.global"},
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
								Host: "test-rollout-svc.test-ns.svc.cluster.local",
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
	common.ResetSync()
	common.InitializeConfig(admiralParams)

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

	sourceDestinationsWithSingleDestinationSvc := map[string]map[string][]*networkingV1Alpha3.RouteDestination{
		"cluster-1": {
			defaultFQDN: {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-deployment-svc.test-ns.svc.cluster.local",
						Port: &networkingV1Alpha3.PortSelector{
							Number: 8080,
						},
					},
				},
			},
		},
	}
	sourceDestinationsWithPreviewSvc := map[string]map[string][]*networkingV1Alpha3.RouteDestination{
		"cluster-1": {
			defaultFQDN: {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-rollout-active-svc.test-ns.svc.cluster.local",
						Port: &networkingV1Alpha3.PortSelector{
							Number: 8080,
						},
					},
				},
			},
			previewFQDN: {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-rollout-preview-svc.test-ns.svc.cluster.local",
						Port: &networkingV1Alpha3.PortSelector{
							Number: 8080,
						},
					},
				},
			},
		},
	}
	sourceDestinationsWithCanarySvc := map[string]map[string][]*networkingV1Alpha3.RouteDestination{
		"cluster-1": {
			defaultFQDN: {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-rollout-stable-svc.test-ns.svc.cluster.local",
						Port: &networkingV1Alpha3.PortSelector{
							Number: 8080,
						},
					},
					Weight: 90,
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-rollout-desired-svc.test-ns.svc.cluster.local",
						Port: &networkingV1Alpha3.PortSelector{
							Number: 8080,
						},
					},
					Weight: 10,
				},
			},
			canaryFQDN: {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-rollout-desired-svc.test-ns.svc.cluster.local",
						Port: &networkingV1Alpha3.PortSelector{
							Number: 8080,
						},
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
		sourceIngressVirtualService map[string]*apiNetworkingV1Alpha3.VirtualService
		sourceClusterToDestinations map[string]map[string][]*networkingV1Alpha3.RouteDestination
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
			name: "Given a no sourceClusterToDestinations, " +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should return an error",
			remoteRegistry:              rr,
			sourceClusterToDestinations: map[string]map[string][]*networkingV1Alpha3.RouteDestination{},
			expectedError:               fmt.Errorf("no route destination found for the ingress virtualservice"),
		},
		{
			name: "Given a invalid sourceIngressVirtualService, " +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should return an error",
			remoteRegistry:              rr,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			sourceIngressVirtualService: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": {
					Spec: networkingV1Alpha3.VirtualService{},
				},
			},
			expectedError: fmt.Errorf("no TLSRoute found in the ingress virtualservice with host test-env.test-identity.global"),
		},
		{
			name: "Given a invalid VS in sourceIngressVirtualService with missing sniHost " +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should return an error",
			remoteRegistry:              rr,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			sourceIngressVirtualService: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": {
					ObjectMeta: metaV1.ObjectMeta{
						Name: "test-env.test-identity.global-routing-vs",
					},
					Spec: networkingV1Alpha3.VirtualService{
						Tls: []*networkingV1Alpha3.TLSRoute{
							{
								Match: []*networkingV1Alpha3.TLSMatchAttributes{
									{
										Port: common.DefaultMtlsPort,
									},
								},
							},
						},
					},
				},
			},
			expectedError: fmt.Errorf(
				"no SNIHosts found in the ingress virtualservice test-env.test-identity.global-routing-vs"),
		},
		{
			name: "Given a invalid VS in sourceIngressVirtualService with multiple sniHost matches " +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should return an error",
			remoteRegistry:              rr,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			sourceIngressVirtualService: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": {
					ObjectMeta: metaV1.ObjectMeta{
						Name: "test-env.test-identity.global-routing-vs",
					},
					Spec: networkingV1Alpha3.VirtualService{
						Tls: []*networkingV1Alpha3.TLSRoute{
							{
								Match: []*networkingV1Alpha3.TLSMatchAttributes{
									{
										Port: common.DefaultMtlsPort,
										SniHosts: []string{
											"outbound_.80_._.test-env.test-identity.global",
											"outbound_.80_._.preview.test-env.test-identity.global",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedError: fmt.Errorf(
				"more than one SNIHosts found in the ingress virtualservice test-env.test-identity.global-routing-vs"),
		},
		{
			name: "Given a valid sourceIngressVirtualService and valid sourceClusterToDestinations " +
				"And the VS is a new VS" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should successfully create the VS",
			remoteRegistry:              rr,
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			sourceIngressVirtualService: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": newVS,
			},
			expectedError: nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
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
										Host: "test-deployment-svc.test-ns.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8080,
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
			name: "Given a valid sourceIngressVirtualService, and valid sourceClusterToDestination " +
				"And there is VS with same name already exists" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should successfully update the VS",
			remoteRegistry:              rr,
			istioClient:                 istioClientWithExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			sourceIngressVirtualService: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": existingVS,
			},
			expectedError: nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
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
										Host: "test-deployment-svc.test-ns.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8080,
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
			name: "Given a valid sourceIngressVirtualService, and valid sourceClusterToDestination " +
				"And there is a preview endpoint in the sourceIngressVirtualService" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should successfully create a VS including the preview endpoint route",
			remoteRegistry:              rr,
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithPreviewSvc,
			sourceIngressVirtualService: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": newVSWithPreviewEndpoint,
			},
			expectedError: nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
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
										Host: "test-rollout-active-svc.test-ns.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8080,
										},
									},
								},
							},
						},
						{
							Match: []*networkingV1Alpha3.TLSMatchAttributes{
								{
									Port:     common.DefaultMtlsPort,
									SniHosts: []string{"outbound_.80_._.preview.test-env.test-identity.global"},
								},
							},
							Route: []*networkingV1Alpha3.RouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "test-rollout-preview-svc.test-ns.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8080,
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
			name: "Given a valid sourceIngressVirtualService, and valid sourceClusterToDestination " +
				"And there is a canary endpoint in the sourceIngressVirtualService" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should successfully create a VS including the canary endpoint routes with weights",
			remoteRegistry:              rr,
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithCanarySvc,
			sourceIngressVirtualService: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": newVSWithCanaryEndpoint,
			},
			expectedError: nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
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
										Host: "test-rollout-stable-svc.test-ns.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8080,
										},
									},
									Weight: 90,
								},
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "test-rollout-desired-svc.test-ns.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8080,
										},
									},
									Weight: 10,
								},
							},
						},
						{
							Match: []*networkingV1Alpha3.TLSMatchAttributes{
								{
									Port:     common.DefaultMtlsPort,
									SniHosts: []string{"outbound_.80_._.canary.test-env.test-identity.global"},
								},
							},
							Route: []*networkingV1Alpha3.RouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "test-rollout-desired-svc.test-ns.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8080,
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
			name: "Given a sourceIngressVirtualService and  sourceClusterToDestination " +
				"And there is a preview endpoint match in the VS but there is no coresponding svc found" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then the VS created should not have the preview sniHost match in the VS",
			remoteRegistry:              rr,
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			sourceIngressVirtualService: map[string]*apiNetworkingV1Alpha3.VirtualService{
				"test-env.test-identity.global": newVSWithPreviewEndpoint,
			},
			expectedError: nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
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
										Host: "test-deployment-svc.test-ns.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8080,
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
				tc.sourceClusterToDestinations)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				actualVS, err := tc.istioClient.
					NetworkingV1alpha3().
					VirtualServices("test-sync-ns").
					Get(context.Background(), "test-env.test-identity.global-routing-vs", metaV1.GetOptions{})
				require.Nil(t, err)
				require.Equal(t, tc.expectedVS.Spec.Tls, actualVS.Spec.Tls)
				require.Equal(t, tc.expectedVS.Spec.ExportTo, actualVS.Spec.ExportTo)
				require.Equal(t, tc.expectedVS.Spec.Gateways, actualVS.Spec.Gateways)
				require.Equal(t, tc.expectedVS.Spec.Hosts, actualVS.Spec.Hosts)
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

func TestPopulateVSRouteDestinationForDeployment(t *testing.T) {

	meshPort := uint32(8080)
	testCases := []struct {
		name                     string
		serviceInstance          map[string]*coreV1.Service
		destinations             map[string][]*networkingV1Alpha3.RouteDestination
		expectedError            error
		expectedRouteDestination map[string][]*networkingV1Alpha3.RouteDestination
	}{
		{
			name: "Given nil serviceInstance " +
				"When populateVSRouteDestinationForDeployment is invoked, " +
				"Then it should return an error",
			serviceInstance: nil,
			expectedError:   fmt.Errorf("serviceInstance is nil"),
		},
		{
			name: "Given a serviceInstance not associated to a deployment " +
				"When populateVSRouteDestinationForDeployment is invoked, " +
				"Then it should return an error",
			serviceInstance: map[string]*coreV1.Service{
				common.Rollout: {},
			},
			expectedError: fmt.Errorf("service is not associated with a deployment"),
		},
		{
			name: "Given a nil destinations map" +
				"When populateVSRouteDestinationForDeployment is invoked, " +
				"Then it should return an error",
			serviceInstance: map[string]*coreV1.Service{
				common.Deployment: {},
			},
			destinations:  nil,
			expectedError: fmt.Errorf("destinations map is nil"),
		},
		{
			name: "Given an empty route destinations map, " +
				"When populateVSRouteDestinationForDeployment is invoked, " +
				"Then it should populate the destinations",
			serviceInstance: map[string]*coreV1.Service{
				common.Deployment: {
					ObjectMeta: metaV1.ObjectMeta{
						Name:      "test-deployment-svc",
						Namespace: "test-ns",
					},
				},
			},
			destinations:  make(map[string][]*networkingV1Alpha3.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*networkingV1Alpha3.RouteDestination{
				defaultFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-deployment-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
		},
		{
			name: "Given an non-empty route destinations map, " +
				"When populateVSRouteDestinationForDeployment is invoked, " +
				"Then it should append the new destination to the map",
			serviceInstance: map[string]*coreV1.Service{
				common.Deployment: {
					ObjectMeta: metaV1.ObjectMeta{
						Name:      "preview-svc",
						Namespace: "test-ns",
					},
				},
			},
			destinations: map[string][]*networkingV1Alpha3.RouteDestination{
				defaultFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-deployment-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
			expectedError: nil,
			expectedRouteDestination: map[string][]*networkingV1Alpha3.RouteDestination{
				defaultFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-deployment-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "preview-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := populateVSRouteDestinationForDeployment(tc.serviceInstance, meshPort, tc.destinations)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedRouteDestination, tc.destinations)
			}
		})
	}

}

func TestPopulateVSRouteDestinationForRollout(t *testing.T) {

	meshPort := uint32(8080)
	testCases := []struct {
		name                     string
		serviceInstance          map[string]*coreV1.Service
		weightedServices         map[string]*WeightedService
		rollout                  *v1alpha1.Rollout
		destinations             map[string][]*networkingV1Alpha3.RouteDestination
		expectedError            error
		expectedRouteDestination map[string][]*networkingV1Alpha3.RouteDestination
	}{
		{
			name: "Given nil serviceInstance " +
				"When populateVSRouteDestinationForRollout is invoked, " +
				"Then it should return an error",
			serviceInstance: nil,
			expectedError:   fmt.Errorf("serviceInstance is nil"),
		},
		{
			name: "Given a serviceInstance not associated to a rollout " +
				"When populateVSRouteDestinationForRollout is invoked, " +
				"Then it should return an error",
			serviceInstance: map[string]*coreV1.Service{
				common.Deployment: {},
			},
			expectedError: fmt.Errorf("service is not associated with a rollout"),
		},
		{
			name: "Given a nil rollout" +
				"When populateVSRouteDestinationForRollout is invoked, " +
				"Then it should return an error",
			serviceInstance: map[string]*coreV1.Service{
				common.Rollout: {},
			},
			destinations:  nil,
			expectedError: fmt.Errorf("rollout is nil"),
		},
		{
			name: "Given a nil destinations map" +
				"When populateVSRouteDestinationForRollout is invoked, " +
				"Then it should return an error",
			serviceInstance: map[string]*coreV1.Service{
				common.Rollout: {},
			},
			rollout:       &v1alpha1.Rollout{},
			destinations:  nil,
			expectedError: fmt.Errorf("destinations map is nil"),
		},
		{
			name: "Given an empty route destinations map and a rollout with bluegreen strategy " +
				"When populateVSRouteDestinationForRollout is invoked, " +
				"Then it should populate the destinations with the active and preview services",
			serviceInstance: map[string]*coreV1.Service{
				common.Rollout: {},
			},
			rollout: &v1alpha1.Rollout{
				Spec: v1alpha1.RolloutSpec{
					Strategy: v1alpha1.RolloutStrategy{
						BlueGreen: &v1alpha1.BlueGreenStrategy{
							ActiveService:  "active-svc",
							PreviewService: "preview-svc",
						},
					},
				},
			},
			weightedServices: map[string]*WeightedService{
				"preview-svc": {
					Service: &coreV1.Service{
						ObjectMeta: metaV1.ObjectMeta{
							Name:      "preview-svc",
							Namespace: "test-ns",
						},
					},
				},
				"active-svc": {
					Service: &coreV1.Service{
						ObjectMeta: metaV1.ObjectMeta{
							Name:      "active-svc",
							Namespace: "test-ns",
						},
					},
				},
			},
			destinations:  make(map[string][]*networkingV1Alpha3.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*networkingV1Alpha3.RouteDestination{
				defaultFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				previewFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "preview-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
		},
		{
			name: "Given an empty route destinations map and a rollout with canary strategy " +
				"When populateVSRouteDestinationForRollout is invoked, " +
				"Then it should populate the destinations with the stable and desired services",
			serviceInstance: map[string]*coreV1.Service{
				common.Rollout: {
					ObjectMeta: metaV1.ObjectMeta{
						Namespace: "test-ns",
					},
				},
			},
			rollout: &v1alpha1.Rollout{
				Spec: v1alpha1.RolloutSpec{
					Strategy: v1alpha1.RolloutStrategy{
						Canary: &v1alpha1.CanaryStrategy{
							StableService: "stable-svc",
							CanaryService: "canary-svc",
							TrafficRouting: &v1alpha1.RolloutTrafficRouting{
								Istio: &v1alpha1.IstioTrafficRouting{},
							},
						},
					},
				},
			},
			weightedServices: map[string]*WeightedService{
				"stable-svc": {
					Weight: 90,
					Service: &coreV1.Service{
						ObjectMeta: metaV1.ObjectMeta{
							Name:      "stable-svc",
							Namespace: "test-ns",
						},
					},
				},
				"canary-svc": {
					Weight: 10,
					Service: &coreV1.Service{
						ObjectMeta: metaV1.ObjectMeta{
							Name:      "canary-svc",
							Namespace: "test-ns",
						},
					},
				},
			},
			destinations:  make(map[string][]*networkingV1Alpha3.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*networkingV1Alpha3.RouteDestination{
				defaultFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "canary-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 10,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "stable-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 90,
					},
				},
				canaryFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "canary-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
		},
		{
			name: "Given an empty route destinations map and a rollout with non-istio canary strategy " +
				"When populateVSRouteDestinationForRollout is invoked, " +
				"Then it should populate the destinations with whatever service it got during discovery",
			serviceInstance: map[string]*coreV1.Service{
				common.Rollout: {
					ObjectMeta: metaV1.ObjectMeta{
						Name:      "test-rollout-svc",
						Namespace: "test-ns",
					},
				},
			},
			rollout: &v1alpha1.Rollout{
				Spec: v1alpha1.RolloutSpec{
					Strategy: v1alpha1.RolloutStrategy{
						Canary: &v1alpha1.CanaryStrategy{
							StableService: "stable-svc",
							CanaryService: "canary-svc",
						},
					},
				},
			},
			destinations:  make(map[string][]*networkingV1Alpha3.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*networkingV1Alpha3.RouteDestination{
				defaultFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-rollout-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := populateVSRouteDestinationForRollout(
				tc.serviceInstance,
				tc.weightedServices,
				tc.rollout,
				meshPort,
				tc.destinations)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedRouteDestination, tc.destinations)
			}
		})
	}

}

func TestPopulateDestinationsForBlueGreenStrategy(t *testing.T) {

	meshPort := uint32(8080)
	testCases := []struct {
		name                     string
		weightedServices         map[string]*WeightedService
		rollout                  *v1alpha1.Rollout
		destinations             map[string][]*networkingV1Alpha3.RouteDestination
		expectedError            error
		expectedRouteDestination map[string][]*networkingV1Alpha3.RouteDestination
	}{
		{
			name: "Given a nil rollout" +
				"When populateDestinationsForBlueGreenStrategy is invoked, " +
				"Then it should return an error",
			destinations:  nil,
			expectedError: fmt.Errorf("populateDestinationsForBlueGreenStrategy, rollout is nil"),
		},
		{
			name: "Given a nil weightedServices" +
				"When populateDestinationsForBlueGreenStrategy is invoked, " +
				"Then it should return an error",
			rollout: &v1alpha1.Rollout{
				ObjectMeta: metaV1.ObjectMeta{
					Name: "test-rollout",
				},
			},
			destinations: nil,
			expectedError: fmt.Errorf(
				"populateDestinationsForBlueGreenStrategy, weightedServices is nil for rollout test-rollout"),
		},
		{
			name: "Given a nil destinations map" +
				"When populateDestinationsForBlueGreenStrategy is invoked, " +
				"Then it should return an error",
			rollout: &v1alpha1.Rollout{
				ObjectMeta: metaV1.ObjectMeta{
					Name: "test-rollout",
				},
			},
			weightedServices: make(map[string]*WeightedService),
			destinations:     nil,
			expectedError: fmt.Errorf(
				"populateDestinationsForBlueGreenStrategy, destinations is nil for rollout test-rollout"),
		},
		{
			name: "Given an empty route destinations map and a rollout with bluegreen strategy " +
				"When populateDestinationsForBlueGreenStrategy is invoked, " +
				"Then it should populate the destinations with the active and preview services",
			rollout: &v1alpha1.Rollout{
				Spec: v1alpha1.RolloutSpec{
					Strategy: v1alpha1.RolloutStrategy{
						BlueGreen: &v1alpha1.BlueGreenStrategy{
							ActiveService:  "active-svc",
							PreviewService: "preview-svc",
						},
					},
				},
			},
			weightedServices: map[string]*WeightedService{
				"preview-svc": {
					Service: &coreV1.Service{
						ObjectMeta: metaV1.ObjectMeta{
							Name:      "preview-svc",
							Namespace: "test-ns",
						},
					},
				},
				"active-svc": {
					Service: &coreV1.Service{
						ObjectMeta: metaV1.ObjectMeta{
							Name:      "active-svc",
							Namespace: "test-ns",
						},
					},
				},
			},
			destinations:  make(map[string][]*networkingV1Alpha3.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*networkingV1Alpha3.RouteDestination{
				defaultFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				previewFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "preview-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := populateDestinationsForBlueGreenStrategy(
				tc.weightedServices,
				tc.rollout,
				meshPort,
				tc.destinations)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedRouteDestination, tc.destinations)
			}
		})
	}

}

func TestPopulateDestinationsForCanaryStrategy(t *testing.T) {

	meshPort := uint32(8080)
	testCases := []struct {
		name                     string
		serviceInstance          *coreV1.Service
		weightedServices         map[string]*WeightedService
		rollout                  *v1alpha1.Rollout
		destinations             map[string][]*networkingV1Alpha3.RouteDestination
		expectedError            error
		expectedRouteDestination map[string][]*networkingV1Alpha3.RouteDestination
	}{
		{
			name: "Given nil serviceInstance " +
				"When populateDestinationsForCanaryStrategy is invoked, " +
				"Then it should return an error",
			serviceInstance: nil,
			expectedError:   fmt.Errorf("populateDestinationsForCanaryStrategy, serviceInstance is nil"),
		},
		{
			name: "Given a nil rollout" +
				"When populateDestinationsForCanaryStrategy is invoked, " +
				"Then it should return an error",
			serviceInstance: &coreV1.Service{},
			expectedError:   fmt.Errorf("populateDestinationsForCanaryStrategy, rollout is nil"),
		},
		{
			name: "Given a nil destinations map" +
				"When populateVSRouteDestinationForRollout is invoked, " +
				"Then it should return an error",
			serviceInstance: &coreV1.Service{},
			rollout: &v1alpha1.Rollout{
				ObjectMeta: metaV1.ObjectMeta{
					Name: "test-rollout",
				},
			},
			weightedServices: make(map[string]*WeightedService),
			destinations:     nil,
			expectedError: fmt.Errorf(
				"populateDestinationsForCanaryStrategy, destinations is nil for rollout test-rollout"),
		},
		{
			name: "Given an empty route destinations map and a rollout with canary strategy " +
				"When populateDestinationsForCanaryStrategy is invoked, " +
				"Then it should populate the destinations with the stable and desired services",
			serviceInstance: &coreV1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Namespace: "test-ns",
				},
			},
			rollout: &v1alpha1.Rollout{
				Spec: v1alpha1.RolloutSpec{
					Strategy: v1alpha1.RolloutStrategy{
						Canary: &v1alpha1.CanaryStrategy{
							StableService: "stable-svc",
							CanaryService: "canary-svc",
							TrafficRouting: &v1alpha1.RolloutTrafficRouting{
								Istio: &v1alpha1.IstioTrafficRouting{},
							},
						},
					},
				},
			},
			weightedServices: map[string]*WeightedService{
				"stable-svc": {
					Weight: 90,
					Service: &coreV1.Service{
						ObjectMeta: metaV1.ObjectMeta{
							Name:      "stable-svc",
							Namespace: "test-ns",
						},
					},
				},
				"canary-svc": {
					Weight: 10,
					Service: &coreV1.Service{
						ObjectMeta: metaV1.ObjectMeta{
							Name:      "canary-svc",
							Namespace: "test-ns",
						},
					},
				},
			},
			destinations:  make(map[string][]*networkingV1Alpha3.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*networkingV1Alpha3.RouteDestination{
				defaultFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "canary-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 10,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "stable-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 90,
					},
				},
				canaryFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "canary-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := populateDestinationsForCanaryStrategy(
				tc.serviceInstance,
				tc.weightedServices,
				tc.rollout,
				meshPort,
				tc.destinations)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedRouteDestination, tc.destinations)
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
				"outbound_.80_._.test-env.test-identity.global",
				"outbound_.80_._.preview.test-env.test-identity.global",
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
					},
				},
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
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
				"outbound_.80_._.test-env.test-identity.global",
				"outbound_.80_._.canary.test-env.test-identity.global",
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
					},
				},
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
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

func TestGetMeshHTTPPortForRollout(t *testing.T) {

	testCases := []struct {
		name          string
		ports         map[string]map[string]uint32
		expectedError error
		expectedPort  uint32
	}{
		{
			name: "Given a nil ports map, " +
				"When getMeshHTTPPortForRollout is invoked, " +
				"Then it should return an error",
			expectedError: fmt.Errorf("ports map is nil"),
		},
		{
			name: "Given a empty ports map, " +
				"When getMeshHTTPPortForRollout is invoked, " +
				"Then it should return an error",
			ports:         make(map[string]map[string]uint32),
			expectedError: fmt.Errorf("ports map is empty"),
		},
		{
			name: "Given a ports map with no valid rollout port " +
				"When getMeshHTTPPortForRollout is invoked, " +
				"Then it should return an error",
			ports: map[string]map[string]uint32{
				common.Deployment: {},
			},
			expectedError: fmt.Errorf("rollout ports not found"),
		},
		{
			name: "Given a ports map with invalid port " +
				"When getMeshHTTPPortForRollout is invoked, " +
				"Then it should return an error",
			ports: map[string]map[string]uint32{
				common.Rollout: {"http": 0},
			},
			expectedError: fmt.Errorf("no valid port found for rollout"),
		},
		{
			name: "Given a ports map with valid port " +
				"When getMeshHTTPPortForRollout is invoked, " +
				"Then it should return the port",
			ports: map[string]map[string]uint32{
				common.Rollout: {"http": 8080},
			},
			expectedError: nil,
			expectedPort:  8080,
		},
		{
			name: "Given a ports map with multiple ports " +
				"When getMeshHTTPPortForRollout is invoked, " +
				"Then it should return the first port",
			ports: map[string]map[string]uint32{
				common.Rollout: {"http2": 8090, "http": 8080},
			},
			expectedError: nil,
			expectedPort:  8090,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getMeshHTTPPortForRollout(tc.ports)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedPort, actual)
			}
		})
	}

}

func TestGetMeshHTTPPortForDeployment(t *testing.T) {

	testCases := []struct {
		name          string
		ports         map[string]map[string]uint32
		expectedError error
		expectedPort  uint32
	}{
		{
			name: "Given a nil ports map, " +
				"When getMeshHTTPPortForDeployment is invoked, " +
				"Then it should return an error",
			expectedError: fmt.Errorf("ports map is nil"),
		},
		{
			name: "Given a empty ports map, " +
				"When getMeshHTTPPortForDeployment is invoked, " +
				"Then it should return an error",
			ports:         make(map[string]map[string]uint32),
			expectedError: fmt.Errorf("ports map is empty"),
		},
		{
			name: "Given a ports map with no valid rollout port " +
				"When getMeshHTTPPortForDeployment is invoked, " +
				"Then it should return an error",
			ports: map[string]map[string]uint32{
				common.Rollout: {},
			},
			expectedError: fmt.Errorf("deployment ports not found"),
		},
		{
			name: "Given a ports map with invalid port " +
				"When getMeshHTTPPortForDeployment is invoked, " +
				"Then it should return an error",
			ports: map[string]map[string]uint32{
				common.Deployment: {"http": 0},
			},
			expectedError: fmt.Errorf("no valid port found for deployment"),
		},
		{
			name: "Given a ports map with valid port " +
				"When getMeshHTTPPortForDeployment is invoked, " +
				"Then it should return the port",
			ports: map[string]map[string]uint32{
				common.Deployment: {"http": 8080},
			},
			expectedError: nil,
			expectedPort:  8080,
		},
		{
			name: "Given a ports map with multiple ports " +
				"When getMeshHTTPPortForDeployment is invoked, " +
				"Then it should return the first port",
			ports: map[string]map[string]uint32{
				common.Deployment: {"http2": 8090, "http": 8080},
			},
			expectedError: nil,
			expectedPort:  8090,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getMeshHTTPPortForDeployment(tc.ports)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedPort, actual)
			}
		})
	}

}

func TestGetAllVSRouteDestinationsByCluster(t *testing.T) {

	meshPort := uint32(8080)
	testCases := []struct {
		name                      string
		serviceInstance           map[string]*coreV1.Service
		weightedServices          map[string]*WeightedService
		meshDeployAndRolloutPorts map[string]map[string]uint32
		rollout                   *v1alpha1.Rollout
		expectedError             error
		expectedRouteDestination  map[string][]*networkingV1Alpha3.RouteDestination
	}{
		{
			name: "Given nil serviceInstance " +
				"When getAllVSRouteDestinationsByCluster is invoked, " +
				"Then it should return an error",
			serviceInstance: nil,
			expectedError:   fmt.Errorf("serviceInstance is nil"),
		},
		{
			name: "Given a rollout with bluegreen strategy " +
				"When getAllVSRouteDestinationsByCluster is invoked, " +
				"Then it should return destinations with the active and preview services",
			meshDeployAndRolloutPorts: map[string]map[string]uint32{
				common.Rollout: {"http": meshPort},
			},
			serviceInstance: map[string]*coreV1.Service{
				common.Rollout: {},
			},
			rollout: &v1alpha1.Rollout{
				Spec: v1alpha1.RolloutSpec{
					Strategy: v1alpha1.RolloutStrategy{
						BlueGreen: &v1alpha1.BlueGreenStrategy{
							ActiveService:  "active-svc",
							PreviewService: "preview-svc",
						},
					},
				},
			},
			weightedServices: map[string]*WeightedService{
				"preview-svc": {
					Service: &coreV1.Service{
						ObjectMeta: metaV1.ObjectMeta{
							Name:      "preview-svc",
							Namespace: "test-ns",
						},
					},
				},
				"active-svc": {
					Service: &coreV1.Service{
						ObjectMeta: metaV1.ObjectMeta{
							Name:      "active-svc",
							Namespace: "test-ns",
						},
					},
				},
			},
			expectedError: nil,
			expectedRouteDestination: map[string][]*networkingV1Alpha3.RouteDestination{
				defaultFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				previewFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "preview-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
		},
		{
			name: "Given an empty route destinations map, " +
				"When populateVSRouteDestinationForDeployment is invoked, " +
				"Then it should populate the destinations",
			meshDeployAndRolloutPorts: map[string]map[string]uint32{
				common.Deployment: {"http": meshPort},
			},
			serviceInstance: map[string]*coreV1.Service{
				common.Deployment: {
					ObjectMeta: metaV1.ObjectMeta{
						Name:      "test-deployment-svc",
						Namespace: "test-ns",
					},
				},
			},
			expectedError: nil,
			expectedRouteDestination: map[string][]*networkingV1Alpha3.RouteDestination{
				defaultFQDN: {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-deployment-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getAllVSRouteDestinationsByCluster(
				tc.serviceInstance,
				tc.meshDeployAndRolloutPorts,
				tc.weightedServices,
				tc.rollout)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedRouteDestination, actual)
			}
		})
	}

}
