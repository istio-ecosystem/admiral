package clusters

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1alpha12 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/core/vsrouting"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	apiNetworkingV1Alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioFake "istio.io/client-go/pkg/clientset/versioned/fake"
	v1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddUpdateVirtualServicesForIngress(t *testing.T) {

	vsLabels := map[string]string{
		vsRoutingLabel: "enabled",
	}

	existingVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-env.test-identity.global-routing-vs",
			Namespace: util.IstioSystemNamespace,
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
		LabelSet:                    &common.LabelSet{},
		EnableSWAwareNSCaches:       true,
		IngressVSExportToNamespaces: []string{"istio-system"},
		VSRoutingGateways:           []string{"istio-system/passthrough-gateway"},
		EnableVSRouting:             true,
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	istioClientWithExistingVS := istioFake.NewSimpleClientset()
	istioClientWithExistingVS.NetworkingV1alpha3().VirtualServices(util.IstioSystemNamespace).
		Create(context.Background(), existingVS, metaV1.CreateOptions{})

	istioClientWithNoExistingVS := istioFake.NewSimpleClientset()
	rc := &RemoteController{
		ClusterID:                "cluster-1",
		VirtualServiceController: &istio.VirtualServiceController{},
	}

	rr := NewRemoteRegistry(context.Background(), admiralParams)
	rr.PutRemoteController("cluster-1", rc)

	defaultFQDN := "outbound_.80_._.test-env.test-identity.global"
	previewFQDN := "outbound_.80_._.preview.test-env.test-identity.global"
	canaryFQDN := "outbound_.80_._.canary.test-env.test-identity.global"

	sourceDestinationsWithSingleDestinationSvc := map[string]map[string][]*vsrouting.RouteDestination{
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
	sourceDestinationsWithPreviewSvc := map[string]map[string][]*vsrouting.RouteDestination{
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
	sourceDestinationsWithCanarySvc := map[string]map[string][]*vsrouting.RouteDestination{
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
		vsName                      string
		sourceClusterToDestinations map[string]map[string][]*vsrouting.RouteDestination
		istioClient                 *istioFake.Clientset
		expectedError               error
		expectedVS                  *apiNetworkingV1Alpha3.VirtualService
	}{
		{
			name: "Given a nil remoteRegistry, " +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should return an error",
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               fmt.Errorf("remoteRegistry is nil"),
		},
		{
			name: "Given a empty vsName, " +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should return an error",
			remoteRegistry:              rr,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               fmt.Errorf("vsName is empty"),
		},
		{
			name: "Given a valid sourceClusterToDestinations " +
				"And the VS is a new VS" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should successfully create the VS",
			remoteRegistry:              rr,
			vsName:                      "test-env.test-identity.global",
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-env.test-identity.global-routing-vs",
					Namespace: util.IstioSystemNamespace,
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
			name: "Given a valid sourceClusterToDestination " +
				"And there is VS with same name already exists" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should successfully update the VS",
			remoteRegistry:              rr,
			vsName:                      "test-env.test-identity.global",
			istioClient:                 istioClientWithExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-env.test-identity.global-routing-vs",
					Namespace: util.IstioSystemNamespace,
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
			name: "Given a valid sourceClusterToDestination " +
				"And there is a preview endpoint in the sourceIngressVirtualService" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should successfully create a VS including the preview endpoint route",
			remoteRegistry:              rr,
			vsName:                      "test-env.test-identity.global",
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithPreviewSvc,
			expectedError:               nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-env.test-identity.global-routing-vs",
					Namespace: util.IstioSystemNamespace,
					Labels:    vsLabels,
				},
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{
						"outbound_.80_._.preview.test-env.test-identity.global",
						"outbound_.80_._.test-env.test-identity.global",
					},
					Gateways: []string{"istio-system/passthrough-gateway"},
					ExportTo: []string{"istio-system"},
					Tls: []*networkingV1Alpha3.TLSRoute{
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
					},
				},
			},
		},
		{
			name: "Given a valid sourceClusterToDestination " +
				"And there is a canary endpoint in the sourceIngressVirtualService" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then it should successfully create a VS including the canary endpoint routes with weights",
			remoteRegistry:              rr,
			vsName:                      "test-env.test-identity.global",
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithCanarySvc,
			expectedError:               nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-env.test-identity.global-routing-vs",
					Namespace: "test-sync-ns",
					Labels:    vsLabels,
				},
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{
						"outbound_.80_._.canary.test-env.test-identity.global",
						"outbound_.80_._.test-env.test-identity.global",
					},
					Gateways: []string{"istio-system/passthrough-gateway"},
					ExportTo: []string{"istio-system"},
					Tls: []*networkingV1Alpha3.TLSRoute{

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
					},
				},
			},
		},
		{
			name: "Given a valid sourceClusterToDestination " +
				"And there is a preview endpoint match in the VS but there is no coresponding svc found" +
				"When addUpdateVirtualServicesForSourceIngress is invoked, " +
				"Then the VS created should not have the preview sniHost match in the VS",
			remoteRegistry:              rr,
			vsName:                      "test-env.test-identity.global",
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
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
			err := addUpdateVirtualServicesForIngress(
				context.Background(),
				ctxLogger,
				tc.remoteRegistry,
				tc.sourceClusterToDestinations,
				tc.vsName)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				actualVS, err := tc.istioClient.
					NetworkingV1alpha3().
					VirtualServices(util.IstioSystemNamespace).
					Get(context.Background(), "test-env.test-identity.global-routing-vs", metaV1.GetOptions{})
				require.Nil(t, err)
				require.Equal(t, tc.expectedVS.ObjectMeta.Name, actualVS.ObjectMeta.Name)
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

	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	meshPort := uint32(8080)
	testCases := []struct {
		name                     string
		serviceInstance          map[string]*coreV1.Service
		destinations             map[string][]*vsrouting.RouteDestination
		deployment               *v1.Deployment
		expectedError            error
		expectedRouteDestination map[string][]*vsrouting.RouteDestination
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
			destinations: make(map[string][]*vsrouting.RouteDestination),
			deployment: &v1.Deployment{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.DeploymentSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
						},
					},
				},
			},
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
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
			deployment: &v1.Deployment{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.DeploymentSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
						},
					},
				},
			},
			destinations: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
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
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
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
			err := populateVSRouteDestinationForDeployment(tc.serviceInstance, meshPort, tc.deployment, tc.destinations)
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

	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	meshPort := uint32(8080)
	testCases := []struct {
		name                     string
		serviceInstance          map[string]*coreV1.Service
		weightedServices         map[string]*WeightedService
		rollout                  *v1alpha1.Rollout
		destinations             map[string][]*vsrouting.RouteDestination
		expectedError            error
		expectedRouteDestination map[string][]*vsrouting.RouteDestination
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
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: map[string]string{
						"identity": "test-identity",
						"env":      "test-env",
					},
				},
				Spec: v1alpha1.RolloutSpec{
					Strategy: v1alpha1.RolloutStrategy{
						BlueGreen: &v1alpha1.BlueGreenStrategy{
							ActiveService:  "active-svc",
							PreviewService: "preview-svc",
						},
					},
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
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
			destinations:  make(map[string][]*vsrouting.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"outbound_.80_._.preview.test-env.test-identity.global": {
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
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
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
			destinations:  make(map[string][]*vsrouting.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
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
				"outbound_.80_._.canary.test-env.test-identity.global": {
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
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
						},
					},
				},
			},
			destinations:  make(map[string][]*vsrouting.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
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

	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	meshPort := uint32(8080)
	testCases := []struct {
		name                     string
		weightedServices         map[string]*WeightedService
		rollout                  *v1alpha1.Rollout
		destinations             map[string][]*vsrouting.RouteDestination
		expectedError            error
		expectedRouteDestination map[string][]*vsrouting.RouteDestination
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
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
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
			destinations:  make(map[string][]*vsrouting.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"outbound_.80_._.preview.test-env.test-identity.global": {
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

	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	meshPort := uint32(8080)
	testCases := []struct {
		name                     string
		serviceInstance          *coreV1.Service
		weightedServices         map[string]*WeightedService
		rollout                  *v1alpha1.Rollout
		destinations             map[string][]*vsrouting.RouteDestination
		expectedError            error
		expectedRouteDestination map[string][]*vsrouting.RouteDestination
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
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: map[string]string{
						"identity": "test-identity",
						"env":      "test-env",
					},
				},
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
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
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
			destinations:  make(map[string][]*vsrouting.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
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
				"outbound_.80_._.canary.test-env.test-identity.global": {
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

func TestGetBaseVirtualServiceForIngress(t *testing.T) {

	admiralParams := common.AdmiralParams{
		IngressVSExportToNamespaces: []string{"istio-system"},
	}

	vsLabels := map[string]string{
		vsRoutingLabel: "enabled",
	}

	validVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: util.IstioSystemNamespace,
			Labels:    vsLabels,
		},
		Spec: networkingV1Alpha3.VirtualService{
			Gateways: []string{"istio-system/passthrough-gateway"},
			ExportTo: []string{"istio-system"},
		},
	}

	testCases := []struct {
		name            string
		routingGateways []string
		expectedVS      *apiNetworkingV1Alpha3.VirtualService
		expectedError   error
	}{
		{
			name: "Given a valid host and sniHost," +
				"And gateways are not configured " +
				"When getBaseVirtualServiceForIngress is invoked, " +
				"Then it should return an error",
			routingGateways: []string{},
			expectedError:   fmt.Errorf("no gateways configured for ingress virtual service"),
		},
		{
			name: "Given a valid host and sniHost," +
				"When getBaseVirtualServiceForIngress is invoked, " +
				"Then it should return the expected VirtualService",
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
			actual, err := getBaseVirtualServiceForIngress()
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
				common.Rollout: {"http2": 8090, "http": 0},
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
				common.Deployment: {"http2": 8090, "http": 0},
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

	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "env",
			AdmiralCRDIdentityLabel: "identity",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*v1alpha12.GlobalTrafficPolicy)
	gtpCache.mutex = &sync.Mutex{}

	gtpCache.Put(&v1alpha12.GlobalTrafficPolicy{
		ObjectMeta: metaV1.ObjectMeta{
			Name:        "test-gtp",
			Annotations: map[string]string{"env": "test-env"},
			Labels:      map[string]string{"identity": "test-identity"},
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					DnsPrefix: "default",
				},
				{
					DnsPrefix: "test-env",
				},
				{
					DnsPrefix: "west",
				},
				{
					DnsPrefix: "east",
				},
			},
		},
	})

	rr := NewRemoteRegistry(context.Background(), admiralParams)
	rr.AdmiralCache = &AdmiralCache{
		GlobalTrafficCache: gtpCache,
	}

	meshPort := uint32(8080)
	testCases := []struct {
		name                      string
		serviceInstance           map[string]*coreV1.Service
		weightedServices          map[string]*WeightedService
		meshDeployAndRolloutPorts map[string]map[string]uint32
		rollout                   *v1alpha1.Rollout
		deployment                *v1.Deployment
		expectedError             error
		expectedRouteDestination  map[string][]*vsrouting.RouteDestination
		sourceIdentity            string
		env                       string
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
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
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
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"outbound_.80_._.preview.test-env.test-identity.global": {
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
				"When getAllVSRouteDestinationsByCluster is invoked, " +
				"Then it should populate the destinations",
			meshDeployAndRolloutPorts: map[string]map[string]uint32{
				common.Deployment: {"http": meshPort},
			},
			deployment: &v1.Deployment{
				Spec: v1.DeploymentSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
						},
					},
				},
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
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
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
			name: "Given an empty route destinations map" +
				"And serviceInstance has both rollout" +
				"When getAllVSRouteDestinationsByCluster is invoked, " +
				"Then it should populate the destinations with deployment and rollout service",
			meshDeployAndRolloutPorts: map[string]map[string]uint32{
				common.Rollout:    {"http": meshPort},
				common.Deployment: {"http": meshPort},
			},
			serviceInstance: map[string]*coreV1.Service{
				common.Rollout: {},
				common.Deployment: {
					ObjectMeta: metaV1.ObjectMeta{
						Name:      "test-deployment-svc",
						Namespace: "test-ns",
					},
				},
			},
			deployment: &v1.Deployment{
				Spec: v1.DeploymentSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
						},
					},
				},
			},
			rollout: &v1alpha1.Rollout{
				Spec: v1alpha1.RolloutSpec{
					Strategy: v1alpha1.RolloutStrategy{
						BlueGreen: &v1alpha1.BlueGreenStrategy{
							ActiveService:  "active-svc",
							PreviewService: "preview-svc",
						},
					},
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
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
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 50,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-deployment-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 50,
					},
				},
				"outbound_.80_._.preview.test-env.test-identity.global": {
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
			name: "Given an empty route destinations map and valid sourceIdenity and env" +
				"And serviceInstance has both rollout and deployment" +
				"And there is corresponding GTP" +
				"When getAllVSRouteDestinationsByCluster is invoked, " +
				"Then it should populate the destinations with deployment, rollout service" +
				"And additional GTP dns prefixed endpoints sans preview",
			meshDeployAndRolloutPorts: map[string]map[string]uint32{
				common.Rollout:    {"http": meshPort},
				common.Deployment: {"http": meshPort},
			},
			serviceInstance: map[string]*coreV1.Service{
				common.Rollout: {},
				common.Deployment: {
					ObjectMeta: metaV1.ObjectMeta{
						Name:      "test-deployment-svc",
						Namespace: "test-ns",
					},
				},
			},
			deployment: &v1.Deployment{
				Spec: v1.DeploymentSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
						},
					},
				},
			},
			rollout: &v1alpha1.Rollout{
				Spec: v1alpha1.RolloutSpec{
					Strategy: v1alpha1.RolloutStrategy{
						BlueGreen: &v1alpha1.BlueGreenStrategy{
							ActiveService:  "active-svc",
							PreviewService: "preview-svc",
						},
					},
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
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
			sourceIdentity: "test-identity",
			env:            "test-env",
			expectedError:  nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 50,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-deployment-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 50,
					},
				},
				"outbound_.80_._.west.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 50,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-deployment-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 50,
					},
				},
				"outbound_.80_._.east.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 50,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-deployment-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 50,
					},
				},
				"outbound_.80_._.preview.test-env.test-identity.global": {
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

	ctxLogger := log.WithFields(log.Fields{})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getAllVSRouteDestinationsByCluster(
				ctxLogger,
				tc.serviceInstance,
				tc.meshDeployAndRolloutPorts,
				tc.weightedServices,
				tc.rollout,
				tc.deployment,
				rr,
				tc.sourceIdentity,
				tc.env)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				for fqdn, destinations := range tc.expectedRouteDestination {
					require.NotNil(t, actual[fqdn])
					require.Equal(t, len(destinations), len(actual[fqdn]))
					for i := 0; i < len(destinations); i++ {
						require.Equal(t, destinations[i].Destination.Host, actual[fqdn][i].Destination.Host)
						require.Equal(t, destinations[i].Destination.Port.Number, actual[fqdn][i].Destination.Port.Number)
						require.Equal(t, destinations[i].Weight, actual[fqdn][i].Weight)
					}
				}
			}
		})
	}

}

func TestGetDefaultSNIHostFromDeployment(t *testing.T) {

	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	testCases := []struct {
		name            string
		deployment      *v1.Deployment
		expectedError   error
		expectedSNIHost string
	}{
		{
			name: "Given a nil deployment " +
				"When getDefaultSNIHostFromDeployment is invoked, " +
				"Then it should return an error",
			deployment:    nil,
			expectedError: fmt.Errorf("deployment is nil"),
		},
		{
			name: "Given an invalid deployment " +
				"When getDefaultSNIHostFromDeployment is invoked, " +
				"Then it should return an error",
			deployment:    &v1.Deployment{},
			expectedError: fmt.Errorf("cname is empty"),
		},
		{
			name: "Given an valid deployment " +
				"When getDefaultSNIHostFromDeployment is invoked, " +
				"Then it should return a valid SNI host",
			deployment: &v1.Deployment{
				Spec: v1.DeploymentSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
						},
					},
				},
			},
			expectedError:   nil,
			expectedSNIHost: "outbound_.80_._.test-env.test-identity.global",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getDefaultSNIHostFromDeployment(tc.deployment)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedSNIHost, actual)
			}
		})
	}

}

func TestGetDefaultSNIHostFromRollout(t *testing.T) {

	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	testCases := []struct {
		name            string
		rollout         *v1alpha1.Rollout
		expectedError   error
		expectedSNIHost string
	}{
		{
			name: "Given a nil rollout " +
				"When getDefaultSNIHostFromRollout is invoked, " +
				"Then it should return an error",
			rollout:       nil,
			expectedError: fmt.Errorf("rollout is nil"),
		},
		{
			name: "Given an invalid rollout " +
				"When getDefaultSNIHostFromRollout is invoked, " +
				"Then it should return an error",
			rollout:       &v1alpha1.Rollout{},
			expectedError: fmt.Errorf("cname is empty"),
		},
		{
			name: "Given an valid rollout " +
				"When getDefaultSNIHostFromRollout is invoked, " +
				"Then it should return a valid SNI host",
			rollout: &v1alpha1.Rollout{
				Spec: v1alpha1.RolloutSpec{
					Strategy: v1alpha1.RolloutStrategy{
						BlueGreen: &v1alpha1.BlueGreenStrategy{
							ActiveService:  "active-svc",
							PreviewService: "preview-svc",
						},
					},
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
						},
					},
				},
			},
			expectedError:   nil,
			expectedSNIHost: "outbound_.80_._.test-env.test-identity.global",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getDefaultSNIHostFromRollout(tc.rollout)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedSNIHost, actual)
			}
		})
	}

}

func TestGetCanarySNIHostFromRollout(t *testing.T) {

	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	testCases := []struct {
		name            string
		rollout         *v1alpha1.Rollout
		expectedError   error
		expectedSNIHost string
	}{
		{
			name: "Given a nil rollout " +
				"When getCanarySNIHostFromRollout is invoked, " +
				"Then it should return an error",
			rollout:       nil,
			expectedError: fmt.Errorf("rollout is nil"),
		},
		{
			name: "Given an invalid rollout " +
				"When getCanarySNIHostFromRollout is invoked, " +
				"Then it should return an error",
			rollout:       &v1alpha1.Rollout{},
			expectedError: fmt.Errorf("getCanaryFQDNForRollout, unable to get cname for rollout "),
		},
		{
			name: "Given an valid rollout " +
				"When getCanarySNIHostFromRollout is invoked, " +
				"Then it should return a valid SNI host",
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
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
						},
					},
				},
			},
			expectedError:   nil,
			expectedSNIHost: "outbound_.80_._.canary.test-env.test-identity.global",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getCanarySNIHostFromRollout(tc.rollout)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedSNIHost, actual)
			}
		})
	}

}

func TestGetPreviewSNIHostFromRollout(t *testing.T) {

	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	testCases := []struct {
		name            string
		rollout         *v1alpha1.Rollout
		expectedError   error
		expectedSNIHost string
	}{
		{
			name: "Given a nil rollout " +
				"When getPreviewSNIHostFromRollout is invoked, " +
				"Then it should return an error",
			rollout:       nil,
			expectedError: fmt.Errorf("rollout is nil"),
		},
		{
			name: "Given an invalid rollout " +
				"When getPreviewSNIHostFromRollout is invoked, " +
				"Then it should return an error",
			rollout:       &v1alpha1.Rollout{},
			expectedError: fmt.Errorf("getPreviewFQDNForRollout, unable to get cname for rollout "),
		},
		{
			name: "Given an valid rollout " +
				"When getPreviewSNIHostFromRollout is invoked, " +
				"Then it should return a valid SNI host",
			rollout: &v1alpha1.Rollout{
				Spec: v1alpha1.RolloutSpec{
					Strategy: v1alpha1.RolloutStrategy{
						BlueGreen: &v1alpha1.BlueGreenStrategy{
							ActiveService:  "active-svc",
							PreviewService: "preview-svc",
						},
					},
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: metaV1.ObjectMeta{
							Annotations: map[string]string{
								"identity": "test-identity",
								"env":      "test-env",
							},
						},
					},
				},
			},
			expectedError:   nil,
			expectedSNIHost: "outbound_.80_._.preview.test-env.test-identity.global",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getPreviewSNIHostFromRollout(tc.rollout)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expectedSNIHost, actual)
			}
		})
	}

}

func TestGetIngressDRLoadBalancerPolicy(t *testing.T) {

	testCases := []struct {
		name           string
		admiralParams  common.AdmiralParams
		expectedPolicy networkingV1Alpha3.LoadBalancerSettings_SimpleLB
	}{
		{
			name: "Given a no ingressPolicy " +
				"When getIngressDRLoadBalancerPolicy is invoked, " +
				"Then it should return the default round robin LoadBalancerPolicy",
			admiralParams: common.AdmiralParams{
				IngressLBPolicy: "",
			},
			expectedPolicy: networkingV1Alpha3.LoadBalancerSettings_ROUND_ROBIN,
		},
		{
			name: "Given a random ingressPolicy " +
				"When getIngressDRLoadBalancerPolicy is invoked, " +
				"Then it should return random LoadBalancerPolicy",
			admiralParams: common.AdmiralParams{
				IngressLBPolicy: "random",
			},
			expectedPolicy: networkingV1Alpha3.LoadBalancerSettings_RANDOM,
		},
		{
			name: "Given a least request ingressPolicy " +
				"When getIngressDRLoadBalancerPolicy is invoked, " +
				"Then it should return least request LoadBalancerPolicy",
			admiralParams: common.AdmiralParams{
				IngressLBPolicy: "least_request",
			},
			expectedPolicy: networkingV1Alpha3.LoadBalancerSettings_LEAST_REQUEST,
		},
		{
			name: "Given a round robin ingressPolicy " +
				"When getIngressDRLoadBalancerPolicy is invoked, " +
				"Then it should return round robin LoadBalancerPolicy",
			admiralParams: common.AdmiralParams{
				IngressLBPolicy: "round_robin",
			},
			expectedPolicy: networkingV1Alpha3.LoadBalancerSettings_ROUND_ROBIN,
		},
		{
			name: "Given a passthrough ingressPolicy " +
				"When getIngressDRLoadBalancerPolicy is invoked, " +
				"Then it should return passthrough LoadBalancerPolicy",
			admiralParams: common.AdmiralParams{
				IngressLBPolicy: "passthrough",
			},
			expectedPolicy: networkingV1Alpha3.LoadBalancerSettings_PASSTHROUGH,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			common.ResetSync()
			common.InitializeConfig(tc.admiralParams)
			actual := getIngressDRLoadBalancerPolicy()
			require.Equal(t, tc.expectedPolicy, actual)
		})
	}

}

func TestAaddUpdateDestinationRuleForSourceIngress(t *testing.T) {

	existingDR := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-ns.svc.cluster.local-routing-dr",
			Namespace: util.IstioSystemNamespace,
		},
		Spec: networkingV1Alpha3.DestinationRule{
			Host:     "*.test-ns.svc.cluster.local",
			ExportTo: []string{util.IstioSystemNamespace},
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					LbPolicy: &networkingV1Alpha3.LoadBalancerSettings_Simple{
						Simple: networkingV1Alpha3.LoadBalancerSettings_LEAST_REQUEST,
					},
					WarmupDurationSecs: &duration.Duration{Seconds: common.GetDefaultWarmupDurationSecs()},
				},
				Tls: &networkingV1Alpha3.ClientTLSSettings{
					SubjectAltNames: []string{"spiffe://test-san-prefix/test-identity"},
				},
			},
		},
	}

	admiralParams := common.AdmiralParams{
		SANPrefix:                         "test-san-prefix",
		IngressVSExportToNamespaces:       []string{"istio-system"},
		EnableVSRouting:                   true,
		VSRoutingSlowStartEnabledClusters: []string{"cluster-1"},
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	istioClientWithExistingDR := istioFake.NewSimpleClientset()
	istioClientWithExistingDR.NetworkingV1alpha3().DestinationRules(util.IstioSystemNamespace).
		Create(context.Background(), existingDR, metaV1.CreateOptions{})

	istioClientWithNoExistingDR := istioFake.NewSimpleClientset()

	rr := NewRemoteRegistry(context.Background(), admiralParams)

	ctxLogger := log.WithFields(log.Fields{
		"type": "DestinationRule",
	})

	testCases := []struct {
		name                     string
		istioClient              *istioFake.Clientset
		drName                   string
		sourceClusterToDRHosts   map[string]map[string]string
		sourceIdentity           string
		expectedError            error
		expectedDestinationRules *apiNetworkingV1Alpha3.DestinationRule
	}{
		{
			name: "Given a empty sourceIdentity " +
				"When addUpdateDestinationRuleForSourceIngress is invoked, " +
				"Then it should return an error",
			drName: "test-ns.svc.cluster.local-routing-dr",
			sourceClusterToDRHosts: map[string]map[string]string{
				"cluster-1": {
					"test-ns.svc.cluster.local": "*.test-ns.svc.cluster.local",
				},
			},
			sourceIdentity: "",
			expectedError:  fmt.Errorf("sourceIdentity is empty"),
		},
		{
			name: "Given a valid sourceClusterToDRHosts " +
				"When addUpdateDestinationRuleForSourceIngress is invoked, " +
				"Then it should create the destination rules",
			drName:         "test-ns.svc.cluster.local-routing-dr",
			sourceIdentity: "test-identity",
			sourceClusterToDRHosts: map[string]map[string]string{
				"cluster-1": {
					"test-ns.svc.cluster.local": "*.test-ns.svc.cluster.local",
				},
			},
			istioClient:   istioClientWithNoExistingDR,
			expectedError: nil,
			expectedDestinationRules: &apiNetworkingV1Alpha3.DestinationRule{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-ns.svc.cluster.local-routing-dr",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.DestinationRule{
					Host:     "*.test-ns.svc.cluster.local",
					ExportTo: []string{util.IstioSystemNamespace},
					TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
						LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
							LbPolicy: &networkingV1Alpha3.LoadBalancerSettings_Simple{
								Simple: networkingV1Alpha3.LoadBalancerSettings_ROUND_ROBIN,
							},
							LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
								Enabled: &wrappers.BoolValue{Value: false},
							},
							WarmupDurationSecs: &duration.Duration{Seconds: common.GetDefaultWarmupDurationSecs()},
						},
						Tls: &networkingV1Alpha3.ClientTLSSettings{
							SubjectAltNames: []string{"spiffe://test-san-prefix/test-identity"},
						},
					},
				},
			},
		},
		{
			name: "Given a valid sourceClusterToDRHosts " +
				"When addUpdateDestinationRuleForSourceIngress is invoked, " +
				"Then it should create the destination rules",
			drName:         "test-ns.svc.cluster.local-routing-dr",
			sourceIdentity: "test-identity",
			sourceClusterToDRHosts: map[string]map[string]string{
				"cluster-1": {
					"test-ns.svc.cluster.local": "*.test-ns.svc.cluster.local",
				},
			},
			istioClient:   istioClientWithExistingDR,
			expectedError: nil,
			expectedDestinationRules: &apiNetworkingV1Alpha3.DestinationRule{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-ns.svc.cluster.local-routing-dr",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.DestinationRule{
					Host:     "*.test-ns.svc.cluster.local",
					ExportTo: []string{util.IstioSystemNamespace},
					TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
						LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
							LbPolicy: &networkingV1Alpha3.LoadBalancerSettings_Simple{
								Simple: networkingV1Alpha3.LoadBalancerSettings_ROUND_ROBIN,
							},
							LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
								Enabled: &wrappers.BoolValue{Value: false},
							},
							WarmupDurationSecs: &duration.Duration{Seconds: common.GetDefaultWarmupDurationSecs()},
						},
						Tls: &networkingV1Alpha3.ClientTLSSettings{
							SubjectAltNames: []string{"spiffe://test-san-prefix/test-identity"},
						},
					},
				},
			},
		},
		{
			name: "Given a valid sourceClusterToDRHosts " +
				"When addUpdateDestinationRuleForSourceIngress is invoked," +
				"And the cluster is not enabled with slow start " +
				"Then it should create the destination rules without warmupDurationSecs",
			drName:         "test-ns2.svc.cluster.local-routing-dr",
			sourceIdentity: "test-identity",
			sourceClusterToDRHosts: map[string]map[string]string{
				"cluster-2": {
					"test-ns2.svc.cluster.local": "*.test-ns2.svc.cluster.local",
				},
			},
			istioClient:   istioClientWithExistingDR,
			expectedError: nil,
			expectedDestinationRules: &apiNetworkingV1Alpha3.DestinationRule{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-ns2.svc.cluster.local-routing-dr",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.DestinationRule{
					Host:     "*.test-ns2.svc.cluster.local",
					ExportTo: []string{util.IstioSystemNamespace},
					TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
						LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
							LbPolicy: &networkingV1Alpha3.LoadBalancerSettings_Simple{
								Simple: networkingV1Alpha3.LoadBalancerSettings_ROUND_ROBIN,
							},
							LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
								Enabled: &wrappers.BoolValue{Value: false},
							},
						},
						Tls: &networkingV1Alpha3.ClientTLSSettings{
							SubjectAltNames: []string{"spiffe://test-san-prefix/test-identity"},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rc := &RemoteController{
				ClusterID:                 "cluster-1",
				DestinationRuleController: &istio.DestinationRuleController{},
			}
			rc2 := &RemoteController{
				ClusterID:                 "cluster-2",
				DestinationRuleController: &istio.DestinationRuleController{},
			}
			rc.DestinationRuleController.IstioClient = tc.istioClient
			rc2.DestinationRuleController.IstioClient = tc.istioClient
			rr.PutRemoteController("cluster-1", rc)
			rr.PutRemoteController("cluster-2", rc2)

			err := addUpdateDestinationRuleForSourceIngress(
				context.Background(),
				ctxLogger,
				rr,
				tc.sourceClusterToDRHosts,
				tc.sourceIdentity)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				actualDR, err := tc.istioClient.NetworkingV1alpha3().DestinationRules(util.IstioSystemNamespace).
					Get(context.Background(), tc.drName, metaV1.GetOptions{})
				require.Nil(t, err)
				require.Equal(t, tc.expectedDestinationRules.Spec.Host, actualDR.Spec.Host)
				require.Equal(t, tc.expectedDestinationRules.Spec.TrafficPolicy, actualDR.Spec.TrafficPolicy)
				require.Equal(t, tc.expectedDestinationRules.Spec.ExportTo, actualDR.Spec.ExportTo)
			}
		})
	}

}

func TestGetDestinationsForGTPDNSPrefixes(t *testing.T) {

	meshPort := uint32(8080)
	testCases := []struct {
		name                        string
		gtp                         *v1alpha12.GlobalTrafficPolicy
		routeDestination            map[string][]*vsrouting.RouteDestination
		expectedError               error
		expectedGTPRouteDestination map[string][]*vsrouting.RouteDestination
	}{
		{
			name: "Given nil gtp " +
				"When getDestinationsForGTPDNSPrefixes is invoked, " +
				"Then it should return an error",
			expectedError: fmt.Errorf("globaltrafficpolicy is nil"),
		},
		{
			name: "Given nil route destinations " +
				"When getDestinationsForGTPDNSPrefixes is invoked, " +
				"Then it should return an error",
			gtp:           &v1alpha12.GlobalTrafficPolicy{},
			expectedError: fmt.Errorf("destinations map is nil"),
		},
		{
			name: "Given empty route destinations " +
				"When getDestinationsForGTPDNSPrefixes is invoked, " +
				"Then it should return an empty gtpRouteDestinations",
			gtp:                         &v1alpha12.GlobalTrafficPolicy{},
			routeDestination:            make(map[string][]*vsrouting.RouteDestination),
			expectedError:               nil,
			expectedGTPRouteDestination: make(map[string][]*vsrouting.RouteDestination),
		},
		{
			name: "Given a valid params " +
				"When getDestinationsForGTPDNSPrefixes is invoked, " +
				"Then it should return an empty gtpRouteDestinations",
			gtp: &v1alpha12.GlobalTrafficPolicy{
				ObjectMeta: metaV1.ObjectMeta{
					Name:        "test-gtp",
					Annotations: map[string]string{"env": "test-env"},
					Labels:      map[string]string{"identity": "test-identity"},
				},
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{
							DnsPrefix: "default",
						},
						{
							DnsPrefix: "test-env",
						},
						{
							DnsPrefix: "west",
						},
						{
							DnsPrefix: "east",
						},
					},
				},
			},
			routeDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"outbound_.80_._.preview.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "preview-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"outbound_.80_._.canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
			expectedError: nil,
			expectedGTPRouteDestination: map[string][]*vsrouting.RouteDestination{
				"outbound_.80_._.east.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"outbound_.80_._.west.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"outbound_.80_._.east.canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"outbound_.80_._.west.canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
			},
		},
	}

	ctxLogger := log.WithFields(log.Fields{})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getDestinationsForGTPDNSPrefixes(
				ctxLogger,
				tc.gtp,
				tc.routeDestination,
				"test-env")
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				for fqdn, destinations := range tc.expectedGTPRouteDestination {
					require.NotNil(t, actual[fqdn])
					require.Equal(t, len(destinations), len(actual[fqdn]))
					for i := 0; i < len(destinations); i++ {
						require.Equal(t, destinations[i].Destination.Host, actual[fqdn][i].Destination.Host)
						require.Equal(t, destinations[i].Destination.Port.Number, actual[fqdn][i].Destination.Port.Number)
						require.Equal(t, destinations[i].Weight, actual[fqdn][i].Weight)
					}
				}
			}
		})
	}

}

func TestGetWeightSplits(t *testing.T) {

	testCases := []struct {
		name            string
		trafficSplit    int
		expectedWeights []int32
	}{
		{
			name: "Given 0 trafficSplit " +
				"When getWeightSplits is invoked, " +
				"Then it should return an empty array of weights",
			trafficSplit:    0,
			expectedWeights: []int32{},
		},
		{
			name: "Given 1 trafficSplit " +
				"When getWeightSplits is invoked, " +
				"Then it should return array of []int{100}",
			trafficSplit:    1,
			expectedWeights: []int32{100},
		},
		{
			name: "Given 2 trafficSplit " +
				"When getWeightSplits is invoked, " +
				"Then it should return array of []int{50, 50}",
			trafficSplit:    2,
			expectedWeights: []int32{50, 50},
		},
		{
			name: "Given 3 trafficSplit " +
				"When getWeightSplits is invoked, " +
				"Then it should return array of []int{34, 33,33}",
			trafficSplit:    3,
			expectedWeights: []int32{34, 33, 33},
		},
		{
			name: "Given 4 trafficSplit " +
				"When getWeightSplits is invoked, " +
				"Then it should return array of []int{25, 25,25,25}",
			trafficSplit:    4,
			expectedWeights: []int32{25, 25, 25, 25},
		},
		{
			name: "Given 9 trafficSplit " +
				"When getWeightSplits is invoked, " +
				"Then it should return array of []int{12, 11, 11, 11, 11, 11, 11, 11, 11}",
			trafficSplit:    9,
			expectedWeights: []int32{12, 11, 11, 11, 11, 11, 11, 11, 11},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := getWeightSplits(tc.trafficSplit)
			require.Equal(t, tc.expectedWeights, actual)
		})
	}
}

func TestAddWeightsToRouteDestinations(t *testing.T) {

	testCases := []struct {
		name                     string
		routeDestinations        map[string][]*vsrouting.RouteDestination
		expectedRouteDestination map[string][]*vsrouting.RouteDestination
		expectedError            error
	}{
		{
			name: "Given nil routeDestinations param " +
				"When addWeightsToRouteDestinations is invoked, " +
				"Then it should return an error",
			routeDestinations: nil,
			expectedError:     fmt.Errorf("route destinations map is nil"),
		},
		{
			name: "Given a routeDestination with a single destination " +
				"When addWeightsToRouteDestinations is invoked, " +
				"Then it should return without adding any weights",
			routeDestinations: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-svc.test-ns.svc.cluster.local",
					},
				},
			}},
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-svc.test-ns.svc.cluster.local",
					},
				},
			}},
		},
		{
			name: "Given a routeDestination with a two destinations with weights=100" +
				"When addWeightsToRouteDestinations is invoked, " +
				"Then it should return with the same weights",
			routeDestinations: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-svc.test-ns.svc.cluster.local",
					},
					Weight: 98,
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "canary.test-svc.test-ns.svc.cluster.local",
					},
					Weight: 2,
				},
			}},
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-svc.test-ns.svc.cluster.local",
					},
					Weight: 98,
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "canary.test-svc.test-ns.svc.cluster.local",
					},
					Weight: 2,
				},
			}},
		},
		{
			name: "Given a routeDestination with a two destinations with weights=50" +
				"When addWeightsToRouteDestinations is invoked, " +
				"Then it should return with the same weights",
			routeDestinations: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-svc.test-ns.svc.cluster.local",
					},
					Weight: 25,
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "canary.test-svc.test-ns.svc.cluster.local",
					},
					Weight: 25,
				},
			}},
			expectedError: fmt.Errorf("total weight is 50, expected 100 or 0"),
		},
		{
			name: "Given a routeDestination with a two destinations with weights=0" +
				"When addWeightsToRouteDestinations is invoked, " +
				"Then it should return with the same weights 50-50",
			routeDestinations: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-svc.test-ns.svc.cluster.local",
					},
					Weight: 0,
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "canary.test-svc.test-ns.svc.cluster.local",
					},
					Weight: 0,
				},
			}},
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-svc.test-ns.svc.cluster.local",
					},
					Weight: 50,
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "canary.test-svc.test-ns.svc.cluster.local",
					},
					Weight: 50,
				},
			}},
		},
		{
			name: "Given a routeDestination with a three destinations with no weights" +
				"When addWeightsToRouteDestinations is invoked, " +
				"Then it should return with weights 34-33-33",
			routeDestinations: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-svc.test-ns.svc.cluster.local",
					},
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "active.test-svc.test-ns.svc.cluster.local",
					},
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "preview.test-svc.test-ns.svc.cluster.local",
					},
				},
			}},
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "test-svc.test-ns.svc.cluster.local",
					},
					Weight: 34,
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "active.test-svc.test-ns.svc.cluster.local",
					},
					Weight: 33,
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "preview.test-svc.test-ns.svc.cluster.local",
					},
					Weight: 33,
				},
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := addWeightsToRouteDestinations(tc.routeDestinations)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, len(tc.expectedRouteDestination), len(tc.routeDestinations))
			}
		})
	}
}
