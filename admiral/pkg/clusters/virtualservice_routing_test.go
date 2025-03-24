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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	apiNetworkingV1Alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioFake "istio.io/client-go/pkg/clientset/versioned/fake"
	v1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddUpdateInClusterVirtualServices(t *testing.T) {

	existingVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-env.test-identity.global-incluster-vs",
			Namespace: util.IstioSystemNamespace,
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts:    []string{"test-env.test-identity.global"},
			ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
			Http: []*networkingV1Alpha3.HTTPRoute{
				{
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "test-env.test-identity.global",
								},
							},
						},
					},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
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
		SyncNamespace:                      "admiral-sync",
		LabelSet:                           &common.LabelSet{},
		ExportToIdentityList:               []string{"*"},
		ExportToMaxNamespaces:              100,
		EnableSWAwareNSCaches:              true,
		EnableVSRoutingInCluster:           true,
		VSRoutingInClusterEnabledResources: map[string]string{"cluster-1": "test-identity"},
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	istioClientWithExistingVS := istioFake.NewSimpleClientset()
	istioClientWithExistingVS.NetworkingV1alpha3().VirtualServices(util.IstioSystemNamespace).
		Create(context.Background(), existingVS, metaV1.CreateOptions{})

	istioClientWithNoExistingVS := istioFake.NewSimpleClientset()
	rc := &RemoteController{
		ClusterID: "cluster-1",
		VirtualServiceController: &istio.VirtualServiceController{
			VirtualServiceCache: istio.NewVirtualServiceCache(),
		},
	}

	rc1 := &RemoteController{
		ClusterID:                "cluster-2",
		VirtualServiceController: &istio.VirtualServiceController{},
	}

	rr := NewRemoteRegistry(context.Background(), admiralParams)
	rr.PutRemoteController("cluster-1", rc)
	rr.PutRemoteController("cluster-2", rc1)

	rr.AdmiralCache.CnameIdentityCache = &sync.Map{}
	rr.AdmiralCache.CnameIdentityCache.Store("test-env.test-identity.global", "test-identity")

	rr.AdmiralCache.IdentityClusterCache = common.NewMapOfMaps()
	rr.AdmiralCache.IdentityClusterCache.Put("test-identity", "cluster-1", "cluster-1")

	rr.AdmiralCache.IdentityClusterNamespaceCache = common.NewMapOfMapOfMaps()
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put(
		"test-identity", "cluster-1", "test-ns", "test-ns")

	rr.AdmiralCache.CnameDependentClusterNamespaceCache = common.NewMapOfMapOfMaps()
	rr.AdmiralCache.CnameDependentClusterNamespaceCache.Put(
		"test-env.test-identity.global", "cluster-1", "test-dependent-ns0", "test-dependent-ns0")
	rr.AdmiralCache.CnameDependentClusterNamespaceCache.Put(
		"test-env.test-identity.global", "cluster-1", "test-dependent-ns1", "test-dependent-ns1")

	defaultFQDNFoIdentity1 := "test-env.test-identity1.global"
	defaultFQDN := "test-env.test-identity.global"
	previewFQDN := "preview.test-env.test-identity.global"
	canaryFQDN := "canary.test-env.test-identity.global"

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
	cluster2SourceDestinationsWithSingleDestinationSvc := map[string]map[string][]*vsrouting.RouteDestination{
		"cluster-2": {
			defaultFQDNFoIdentity1: {
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
		sourceIdentity              string
		expectedError               error
		expectedVS                  *apiNetworkingV1Alpha3.VirtualService
	}{
		{
			name: "Given a empty sourceIdentity, " +
				"When addUpdateInClusterVirtualServices is invoked, " +
				"Then it should return an error",
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               fmt.Errorf("identity is empty"),
		},
		{
			name: "Given a nil remoteRegistry, " +
				"When addUpdateInClusterVirtualServices is invoked, " +
				"Then it should return an error",
			sourceIdentity:              "test-identity",
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               fmt.Errorf("remoteRegistry is nil"),
		},
		{
			name: "Given a empty vsName, " +
				"When addUpdateInClusterVirtualServices is invoked, " +
				"Then it should return an error",
			sourceIdentity:              "test-identity",
			remoteRegistry:              rr,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               fmt.Errorf("vsName is empty"),
		},
		{
			name: "Given a valid sourceClusterToDestinations " +
				"And the VS is a new VS" +
				"When addUpdateInClusterVirtualServices is invoked, " +
				"Then it should successfully create the VS",
			sourceIdentity:              "test-identity",
			remoteRegistry:              rr,
			vsName:                      "test-env.test-identity.global",
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-env.test-identity.global-incluster-vs",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.VirtualService{
					Hosts:    []string{"test-env.test-identity.global"},
					ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
					Http: []*networkingV1Alpha3.HTTPRoute{
						{
							Name: "test-env.test-identity.global",
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.test-identity.global",
										},
									},
								},
							},
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
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
			name: "Given a valid sourceClusterToDestinations " +
				"And the VS is a new VS" +
				"And vs routing is not enabled for the cluster" +
				"When addUpdateInClusterVirtualServices is invoked, " +
				"Then it should successfully create the VS",
			sourceIdentity:              "test-identity1",
			remoteRegistry:              rr,
			vsName:                      "test-env.test-identity1.global",
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: cluster2SourceDestinationsWithSingleDestinationSvc,
			expectedError:               nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-env.test-identity1.global-incluster-vs",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.VirtualService{
					Hosts:    []string{"test-env.test-identity1.global"},
					ExportTo: []string{"admiral-sync"},
					Http: []*networkingV1Alpha3.HTTPRoute{
						{
							Name: "test-env.test-identity1.global",
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.test-identity1.global",
										},
									},
								},
							},
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
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
				"When addUpdateInClusterVirtualServices is invoked, " +
				"Then it should successfully update the VS",
			sourceIdentity:              "test-identity",
			remoteRegistry:              rr,
			vsName:                      "test-env.test-identity.global",
			istioClient:                 istioClientWithExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-env.test-identity.global-incluster-vs",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.VirtualService{
					Hosts:    []string{"test-env.test-identity.global"},
					ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
					Http: []*networkingV1Alpha3.HTTPRoute{
						{
							Name: "test-env.test-identity.global",
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.test-identity.global",
										},
									},
								},
							},
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
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
				"When addUpdateInClusterVirtualServices is invoked, " +
				"Then it should successfully create a VS including the preview endpoint route",
			sourceIdentity:              "test-identity",
			remoteRegistry:              rr,
			vsName:                      "test-env.test-identity.global",
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithPreviewSvc,
			expectedError:               nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-env.test-identity.global-incluster-vs",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{
						"preview.test-env.test-identity.global",
						"test-env.test-identity.global",
					},
					ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
					Http: []*networkingV1Alpha3.HTTPRoute{
						{
							Name: "preview.test-env.test-identity.global",
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "preview.test-env.test-identity.global",
										},
									},
								},
							},
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
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
							Name: "test-env.test-identity.global",
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.test-identity.global",
										},
									},
								},
							},
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
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
				"And there is a canary endpoint" +
				"When addUpdateInClusterVirtualServices is invoked, " +
				"Then it should successfully create a VS including the canary endpoint routes with weights",
			sourceIdentity:              "test-identity",
			remoteRegistry:              rr,
			vsName:                      "test-env.test-identity.global",
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithCanarySvc,
			expectedError:               nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-env.test-identity.global-incluster-vs",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{
						"canary.test-env.test-identity.global",
						"test-env.test-identity.global",
					},
					ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
					Http: []*networkingV1Alpha3.HTTPRoute{
						{
							Name: "canary.test-env.test-identity.global",
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "canary.test-env.test-identity.global",
										},
									},
								},
							},
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
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
							Name: "test-env.test-identity.global",
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.test-identity.global",
										},
									},
								},
							},
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
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
				"And there is a preview endpoint match in the VS but there is no corresponding svc found" +
				"When addUpdateInClusterVirtualServices is invoked, " +
				"Then the VS created should not have the preview match in the VS",
			sourceIdentity:              "test-identity",
			remoteRegistry:              rr,
			vsName:                      "test-env.test-identity.global",
			istioClient:                 istioClientWithNoExistingVS,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               nil,
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-env.test-identity.global-incluster-vs",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.VirtualService{
					Hosts:    []string{"test-env.test-identity.global"},
					ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
					Http: []*networkingV1Alpha3.HTTPRoute{
						{
							Name: "test-env.test-identity.global",
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.test-identity.global",
										},
									},
								},
							},
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
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
			rc1 := rr.GetRemoteController("cluster-2")
			rc1.VirtualServiceController.IstioClient = tc.istioClient
			rr.PutRemoteController("cluster-1", rc)
			err := addUpdateInClusterVirtualServices(
				context.Background(),
				ctxLogger,
				tc.remoteRegistry,
				tc.sourceClusterToDestinations,
				tc.vsName,
				tc.sourceIdentity, "")
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				actualVS, err := tc.istioClient.
					NetworkingV1alpha3().
					VirtualServices(util.IstioSystemNamespace).
					Get(context.Background(), tc.vsName+"-incluster-vs", metaV1.GetOptions{})
				require.Nil(t, err)
				require.Equal(t, tc.expectedVS.ObjectMeta.Name, actualVS.ObjectMeta.Name)
				require.Equal(t, tc.expectedVS.Spec.Http, actualVS.Spec.Http)
				require.Equal(t, tc.expectedVS.Spec.ExportTo, actualVS.Spec.ExportTo)
				require.Equal(t, tc.expectedVS.Spec.Hosts, actualVS.Spec.Hosts)
			}
		})
	}

}

func TestAddUpdateVirtualServicesForIngress(t *testing.T) {

	vsLabels := map[string]string{
		common.VSRoutingLabel: "enabled",
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
		ClusterID: "cluster-1",
		VirtualServiceController: &istio.VirtualServiceController{
			VirtualServiceCache: istio.NewVirtualServiceCache(),
		},
	}

	rr := NewRemoteRegistry(context.Background(), admiralParams)
	rr.PutRemoteController("cluster-1", rc)

	defaultFQDN := "test-env.test-identity.global"
	previewFQDN := "preview.test-env.test-identity.global"
	canaryFQDN := "canary.test-env.test-identity.global"

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
				"test-env.test-identity.global": {
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
				"test-env.test-identity.global": {
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
				"test-env.test-identity.global": {
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
				"test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"preview.test-env.test-identity.global": {
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
				"test-env.test-identity.global": {
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
				"canary.test-env.test-identity.global": {
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
			name: "Given an empty route destinations map and a rollout with canary strategy and only one service as prt of weighted service " +
				"When populateVSRouteDestinationForRollout is invoked, " +
				"Then it should populate the destinations with the stable",
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
								Istio: &v1alpha1.IstioTrafficRouting{VirtualService: &v1alpha1.IstioVirtualService{Name: "test"}},
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
					Weight: 1,
					Service: &coreV1.Service{
						ObjectMeta: metaV1.ObjectMeta{
							Name:      "stable-svc",
							Namespace: "test-ns",
						},
					},
				},
			},
			destinations:  make(map[string][]*vsrouting.RouteDestination),
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "stable-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"canary.test-env.test-identity.global": {
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
				"test-env.test-identity.global": {
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
				"test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"preview.test-env.test-identity.global": {
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
				"test-env.test-identity.global": {
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
				"canary.test-env.test-identity.global": {
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
		common.VSRoutingLabel: "enabled",
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
				"test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"preview.test-env.test-identity.global": {
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
				"test-env.test-identity.global": {
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
				"test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-deployment-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"preview.test-env.test-identity.global": {
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
			actual, err := getAllVSRouteDestinationsByCluster(tc.serviceInstance, tc.meshDeployAndRolloutPorts, tc.weightedServices, tc.rollout, tc.deployment)
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

func TestProcessGTPAndAddWeightsByCluster(t *testing.T) {

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
					LbType:    model.TrafficPolicy_FAILOVER,
					Target: []*model.TrafficGroup{
						{
							Region: "us-east-2",
							Weight: 100,
						},
					},
				},
				{
					DnsPrefix: "west",
				},
				{
					DnsPrefix: "east",
					LbType:    model.TrafficPolicy_FAILOVER,
					Target: []*model.TrafficGroup{
						{
							Region: "us-east-2",
							Weight: 80,
						},
						{
							Region: "us-west-2",
							Weight: 20,
						},
					},
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
		name                     string
		destinations             map[string][]*vsrouting.RouteDestination
		expectedError            error
		expectedRouteDestination map[string][]*vsrouting.RouteDestination
		sourceIdentity           string
		env                      string
		updateWeights            bool
		sourceClusterLocality    string
	}{
		{
			name: "Given valid empty route destinations and valid GTP" +
				"When processGTPAndAddWeightsByCluster is invoked, " +
				"Then it should return error",
			sourceIdentity: "test-identity",
			env:            "test-env",
			expectedError:  fmt.Errorf("destinations map is nil"),
			updateWeights:  true,
		},
		{
			name: "Given valid sourceIdenity, sourceClusterLocality, env and route destinations" +
				"And there is corresponding GTP" +
				"When processGTPAndAddWeightsByCluster is invoked, " +
				"Then it should populate the destinations with additional GTP dns prefixed endpoints and weights",
			sourceIdentity:        "test-identity",
			env:                   "test-env",
			sourceClusterLocality: "us-east-2",
			expectedError:         nil,
			updateWeights:         true,
			destinations: map[string][]*vsrouting.RouteDestination{
				"test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 80,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 20,
					},
				},
				"preview.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "preview-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"canary.test-env.test-identity.global": {
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
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 80,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 20,
					},
				},
				"preview.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "preview-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"west.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 80,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 20,
					},
				},
				"west.canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"east.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 64,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 16,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "east.test-env.test-identity.global",
							Port: &networkingV1Alpha3.PortSelector{
								Number: 80,
							},
						},
						Weight: 20,
					},
				},
				"east.canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 80,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "east.canary.test-env.test-identity.global",
							Port: &networkingV1Alpha3.PortSelector{
								Number: 80,
							},
						},
						Weight: 20,
					},
				},
			},
		},
		{
			name: "Given valid sourceIdenity, sourceClusterLocality, env and route destinations" +
				"And there is corresponding GTP" +
				"When processGTPAndAddWeightsByCluster is invoked, " +
				"Then it should populate the destinations with additional GTP dns prefixed endpoints",
			sourceIdentity:        "test-identity",
			env:                   "test-env",
			expectedError:         nil,
			updateWeights:         false,
			sourceClusterLocality: "us-east-2",
			destinations: map[string][]*vsrouting.RouteDestination{
				"test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"preview.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "preview-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"canary.test-env.test-identity.global": {
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
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{
				"test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"preview.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "preview-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"east.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"west.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"east.canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"west.canary.test-env.test-identity.global": {
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
			err := processGTPAndAddWeightsByCluster(
				ctxLogger,
				rr,
				tc.sourceIdentity,
				tc.env,
				tc.sourceClusterLocality,
				tc.destinations,
				tc.updateWeights)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				for fqdn, destinations := range tc.destinations {
					require.NotNil(t, destinations)
					require.Equal(t, len(destinations), len(tc.expectedRouteDestination[fqdn]))
					for i := 0; i < len(destinations); i++ {
						require.Equal(t, destinations[i].Destination.Host, tc.expectedRouteDestination[fqdn][i].Destination.Host)
						require.Equal(t, destinations[i].Destination.Port.Number, tc.expectedRouteDestination[fqdn][i].Destination.Port.Number)
						require.Equal(t, destinations[i].Weight, tc.expectedRouteDestination[fqdn][i].Weight)
					}
				}
			}
		})
	}

}

func TestGetDefaultFQDNFromDeployment(t *testing.T) {

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
			expectedSNIHost: "test-env.test-identity.global",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getDefaultFQDNFromDeployment(tc.deployment)
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

func TestGetDefaultFQDNFromRollout(t *testing.T) {

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
			expectedSNIHost: "test-env.test-identity.global",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getDefaultFQDNFromRollout(tc.rollout)
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

func TestGetCanaryFQDNFromRollout(t *testing.T) {

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
			expectedSNIHost: "canary.test-env.test-identity.global",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getCanaryFQDNFromRollout(tc.rollout)
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

func TestGetPreviewFQDNFromRollout(t *testing.T) {

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
			expectedSNIHost: "preview.test-env.test-identity.global",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getPreviewFQDNFromRollout(tc.rollout)
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

func TestAddUpdateInClusterDestinationRule(t *testing.T) {

	existingDR := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-ns.svc.cluster.local-incluster-dr",
			Namespace: util.IstioSystemNamespace,
		},
		Spec: networkingV1Alpha3.DestinationRule{
			Host:     "*.test-ns.svc.cluster.local",
			ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					LbPolicy: &networkingV1Alpha3.LoadBalancerSettings_Simple{
						Simple: networkingV1Alpha3.LoadBalancerSettings_LEAST_REQUEST,
					},
					WarmupDurationSecs: &duration.Duration{Seconds: common.GetDefaultWarmupDurationSecs()},
				},
				Tls: &networkingV1Alpha3.ClientTLSSettings{
					Mode:            networkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
					SubjectAltNames: []string{"spiffe://test-san-prefix/test-identity"},
				},
			},
		},
	}

	admiralParams := common.AdmiralParams{
		SANPrefix:                          "test-san-prefix",
		EnableVSRoutingInCluster:           true,
		VSRoutingInClusterEnabledResources: map[string]string{"cluster-1": "test-identity", "cluster-2": "*"},
		VSRoutingSlowStartEnabledClusters:  []string{"cluster-1"},
		ExportToIdentityList:               []string{"*"},
		ExportToMaxNamespaces:              100,
		EnableSWAwareNSCaches:              true,
	}

	common.ResetSync()
	common.InitializeConfig(admiralParams)

	istioClientWithExistingDR := istioFake.NewSimpleClientset()
	istioClientWithExistingDR.NetworkingV1alpha3().DestinationRules(util.IstioSystemNamespace).
		Create(context.Background(), existingDR, metaV1.CreateOptions{})

	istioClientWithNoExistingDR := istioFake.NewSimpleClientset()

	rr := NewRemoteRegistry(context.Background(), admiralParams)

	rr.AdmiralCache.CnameIdentityCache = &sync.Map{}
	rr.AdmiralCache.CnameIdentityCache.Store("test-env.test-identity.global", "test-identity")

	rr.AdmiralCache.IdentityClusterCache = common.NewMapOfMaps()
	rr.AdmiralCache.IdentityClusterCache.Put("test-identity", "cluster-1", "cluster-1")
	rr.AdmiralCache.IdentityClusterCache.Put("test-identity", "cluster-2", "cluster-2")

	rr.AdmiralCache.IdentityClusterNamespaceCache = common.NewMapOfMapOfMaps()
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put(
		"test-identity", "cluster-1", "test-ns", "test-ns")
	rr.AdmiralCache.IdentityClusterNamespaceCache.Put(
		"test-identity", "cluster-2", "test-ns2", "test-ns2")

	rr.AdmiralCache.CnameDependentClusterNamespaceCache = common.NewMapOfMapOfMaps()
	rr.AdmiralCache.CnameDependentClusterNamespaceCache.Put(
		"test-env.test-identity.global", "cluster-1", "test-dependent-ns0", "test-dependent-ns0")
	rr.AdmiralCache.CnameDependentClusterNamespaceCache.Put(
		"test-env.test-identity.global", "cluster-1", "test-dependent-ns1", "test-dependent-ns1")
	rr.AdmiralCache.CnameDependentClusterNamespaceCache.Put(
		"test-env.test-identity.global", "cluster-2", "test-dependent-ns0", "test-dependent-ns0")
	rr.AdmiralCache.CnameDependentClusterNamespaceCache.Put(
		"test-env.test-identity.global", "cluster-2", "test-dependent-ns1", "test-dependent-ns1")

	ctxLogger := log.WithFields(log.Fields{
		"type": "DestinationRule",
	})

	testCases := []struct {
		name                     string
		istioClient              *istioFake.Clientset
		drName                   string
		sourceClusterToDRHosts   map[string]map[string]string
		sourceIdentity           string
		cname                    string
		expectedError            error
		expectedDestinationRules *apiNetworkingV1Alpha3.DestinationRule
	}{
		{
			name: "Given a empty sourceIdentity " +
				"When addUpdateInClusterDestinationRule is invoked, " +
				"Then it should return an error",
			drName: "test-ns.svc.cluster.local-incluster-dr",
			sourceClusterToDRHosts: map[string]map[string]string{
				"cluster-1": {
					"test-ns.svc.cluster.local": "*.test-ns.svc.cluster.local",
				},
			},
			sourceIdentity: "",
			expectedError:  fmt.Errorf("sourceIdentity is empty"),
		},
		{
			name: "Given a empty cname " +
				"When addUpdateInClusterDestinationRule is invoked, " +
				"Then it should return an error",
			drName: "test-ns.svc.cluster.local-incluster-dr",
			sourceClusterToDRHosts: map[string]map[string]string{
				"cluster-1": {
					"test-ns.svc.cluster.local": "*.test-ns.svc.cluster.local",
				},
			},
			sourceIdentity: "test-identity",
			expectedError:  fmt.Errorf("cname is empty"),
		},
		{
			name: "Given a valid sourceClusterToDRHosts " +
				"When addUpdateInClusterDestinationRule is invoked, " +
				"Then it should create the destination rules",
			drName:         "test-ns.svc.cluster.local-incluster-dr",
			sourceIdentity: "test-identity",
			cname:          "test-env.test-identity.global",
			sourceClusterToDRHosts: map[string]map[string]string{
				"cluster-1": {
					"test-ns.svc.cluster.local": "*.test-ns.svc.cluster.local",
				},
			},
			istioClient:   istioClientWithNoExistingDR,
			expectedError: nil,
			expectedDestinationRules: &apiNetworkingV1Alpha3.DestinationRule{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-ns.svc.cluster.local-incluster-dr",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.DestinationRule{
					Host:     "*.test-ns.svc.cluster.local",
					ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
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
							Mode:            networkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
							SubjectAltNames: []string{"spiffe://test-san-prefix/test-identity"},
						},
					},
				},
			},
		},
		{
			name: "Given a valid sourceClusterToDRHosts " +
				"When addUpdateInClusterDestinationRule is invoked, " +
				"Then it should create the destination rules",
			drName:         "test-ns.svc.cluster.local-incluster-dr",
			sourceIdentity: "test-identity",
			cname:          "test-env.test-identity.global",
			sourceClusterToDRHosts: map[string]map[string]string{
				"cluster-1": {
					"test-ns.svc.cluster.local": "*.test-ns.svc.cluster.local",
				},
			},
			istioClient:   istioClientWithExistingDR,
			expectedError: nil,
			expectedDestinationRules: &apiNetworkingV1Alpha3.DestinationRule{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-ns.svc.cluster.local-incluster-dr",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.DestinationRule{
					Host:     "*.test-ns.svc.cluster.local",
					ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
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
							Mode:            networkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
							SubjectAltNames: []string{"spiffe://test-san-prefix/test-identity"},
						},
					},
				},
			},
		},
		{
			name: "Given a valid sourceClusterToDRHosts " +
				"When addUpdateInClusterDestinationRule is invoked," +
				"And the cluster is not enabled with slow start " +
				"Then it should create the destination rules without warmupDurationSecs",
			drName:         "test-ns2.svc.cluster.local-incluster-dr",
			sourceIdentity: "test-identity",
			cname:          "test-env.test-identity.global",
			sourceClusterToDRHosts: map[string]map[string]string{
				"cluster-2": {
					"test-ns2.svc.cluster.local": "*.test-ns2.svc.cluster.local",
				},
			},
			istioClient:   istioClientWithExistingDR,
			expectedError: nil,
			expectedDestinationRules: &apiNetworkingV1Alpha3.DestinationRule{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-ns2.svc.cluster.local-incluster-dr",
					Namespace: util.IstioSystemNamespace,
				},
				Spec: networkingV1Alpha3.DestinationRule{
					Host:     "*.test-ns2.svc.cluster.local",
					ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns2"},
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
							Mode:            networkingV1Alpha3.ClientTLSSettings_ISTIO_MUTUAL,
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

			err := addUpdateInClusterDestinationRule(
				context.Background(),
				ctxLogger,
				rr,
				tc.sourceClusterToDRHosts,
				tc.sourceIdentity,
				tc.cname)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				actualDR, err := tc.istioClient.NetworkingV1alpha3().DestinationRules(util.IstioSystemNamespace).
					Get(context.Background(), tc.drName, metaV1.GetOptions{})
				require.Nil(t, err)
				require.Equal(t, tc.expectedDestinationRules.Spec.Host, actualDR.Spec.Host)
				require.Equal(t,
					tc.expectedDestinationRules.Spec.TrafficPolicy.LoadBalancer.LbPolicy,
					actualDR.Spec.TrafficPolicy.LoadBalancer.LbPolicy)
				require.Equal(
					t,
					tc.expectedDestinationRules.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting,
					actualDR.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting)
				require.Equal(
					t, tc.expectedDestinationRules.Spec.TrafficPolicy.Tls, actualDR.Spec.TrafficPolicy.Tls)
				require.Equal(t, tc.expectedDestinationRules.Spec.ExportTo, actualDR.Spec.ExportTo)
			}
		})
	}

}

func TestAddUpdateDestinationRuleForSourceIngress(t *testing.T) {

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
		sourceClusterLocality       string
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
			name: "Given nil routes for a globalFQDN within route destinations " +
				"When getDestinationsForGTPDNSPrefixes is invoked, " +
				"Then it should return an empty gtpRouteDestinations",
			gtp: &v1alpha12.GlobalTrafficPolicy{},
			routeDestination: map[string][]*vsrouting.RouteDestination{
				"test-env.test-identity.global": nil,
			},
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
							LbType:    model.TrafficPolicy_FAILOVER,
							Target: []*model.TrafficGroup{
								{
									Region: "us-east-2",
									Weight: 100,
								},
							},
						},
						{
							DnsPrefix: "test-env",
							LbType:    model.TrafficPolicy_TOPOLOGY,
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: 100,
								},
							},
						},
						{
							DnsPrefix: "west",
						},
						{
							DnsPrefix: "east",
						},
						{
							DnsPrefix: "test-failover",
							LbType:    model.TrafficPolicy_FAILOVER,
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: 100,
								},
							},
						},
						{
							DnsPrefix: "test-traffic-split",
							LbType:    model.TrafficPolicy_FAILOVER,
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: 10,
								},
								{
									Region: "us-east-2",
									Weight: 90,
								},
							},
						},
					},
				},
			},
			sourceClusterLocality: "us-west-2",
			routeDestination: map[string][]*vsrouting.RouteDestination{
				"test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"preview.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "preview-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"canary.test-env.test-identity.global": {
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
				"east.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"west.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"east.canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"west.canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"test-failover.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"test-failover.canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
					},
				},
				"test-traffic-split.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 10,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-traffic-split.test-env.test-identity.global",
							Port: &networkingV1Alpha3.PortSelector{
								Number: 80,
							},
						},
						Weight: 90,
					},
				},
				"test-traffic-split.canary.test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "desired-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 10,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-traffic-split.canary.test-env.test-identity.global",
							Port: &networkingV1Alpha3.PortSelector{
								Number: 80,
							},
						},
						Weight: 90,
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
				"test-env",
				tc.sourceClusterLocality, true)
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

func TestPerformInVSRoutingRollback(t *testing.T) {

	existingVS0 := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-env.test-identity1.global-incluster-vs",
			Namespace: util.IstioSystemNamespace,
			Labels:    map[string]string{common.VSRoutingType: common.VSRoutingTypeInCluster},
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts:    []string{"test-env.test-identity1.global"},
			ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
			Http: []*networkingV1Alpha3.HTTPRoute{
				{
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "test-env.test-identity1.global",
								},
							},
						},
					},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
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

	existingVS1 := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-env.test-identity2.global-incluster-vs",
			Namespace: util.IstioSystemNamespace,
			Labels:    map[string]string{common.VSRoutingType: common.VSRoutingTypeInCluster},
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts:    []string{"test-env.test-identity2.global"},
			ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
			Http: []*networkingV1Alpha3.HTTPRoute{
				{
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "test-env.test-identity2.global",
								},
							},
						},
					},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
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
	existingVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "test-env.test-identity.global-incluster-vs",
			Namespace: util.IstioSystemNamespace,
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts:    []string{"test-env.test-identity.global"},
			ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
			Http: []*networkingV1Alpha3.HTTPRoute{
				{
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "test-env.test-identity.global",
								},
							},
						},
					},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
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
		LabelSet:                            &common.LabelSet{},
		SyncNamespace:                       "test-sync-ns",
		ExportToIdentityList:                []string{"*"},
		ExportToMaxNamespaces:               100,
		EnableSWAwareNSCaches:               true,
		EnableVSRoutingInCluster:            true,
		VSRoutingInClusterDisabledResources: map[string]string{"cluster-2": "*", "cluster-1": "test-identity"},
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	istioClientCluster1 := istioFake.NewSimpleClientset()
	istioClientCluster1.NetworkingV1alpha3().VirtualServices(util.IstioSystemNamespace).
		Create(context.Background(), existingVS, metaV1.CreateOptions{})
	istioClientCluster2 := istioFake.NewSimpleClientset()
	istioClientCluster2.NetworkingV1alpha3().VirtualServices(util.IstioSystemNamespace).
		Create(context.Background(), existingVS0, metaV1.CreateOptions{})
	istioClientCluster2.NetworkingV1alpha3().VirtualServices(util.IstioSystemNamespace).
		Create(context.Background(), existingVS1, metaV1.CreateOptions{})

	rc := &RemoteController{
		ClusterID:                "cluster-1",
		VirtualServiceController: &istio.VirtualServiceController{IstioClient: istioClientCluster1},
	}
	rc1 := &RemoteController{
		ClusterID:                "cluster-2",
		VirtualServiceController: &istio.VirtualServiceController{IstioClient: istioClientCluster2},
	}

	rr := NewRemoteRegistry(context.Background(), admiralParams)
	rr.PutRemoteController("cluster-1", rc)
	rr.PutRemoteController("cluster-2", rc1)

	ctxLogger := log.WithFields(log.Fields{
		"type": "VirtualService",
	})

	testCases := []struct {
		name                        string
		remoteRegistry              *RemoteRegistry
		vsName                      string
		sourceClusterToEventNsCache map[string]string
		sourceIdentity              string
		cluster                     string
		vsNames                     []string
		expectedError               error
		expectedVSExportTo          []string
	}{
		{
			name: "Given a nil remoteRegistry, " +
				"When performInVSRoutingRollback is invoked, " +
				"Then it should return an error",
			sourceIdentity: "test-identity",
			expectedError:  fmt.Errorf("remoteRegistry is nil"),
		},
		{
			name: "Given a empty sourceIdentity, " +
				"When performInVSRoutingRollback is invoked, " +
				"Then it should return an error",
			remoteRegistry: rr,
			expectedError:  fmt.Errorf("source identity is empty"),
		},
		{
			name: "Given a nil sourceClusterToEventNsCache, " +
				"When performInVSRoutingRollback is invoked, " +
				"Then it should return an error",
			sourceIdentity: "test-identity",
			remoteRegistry: rr,
			expectedError:  fmt.Errorf("sourceClusterToEventNsCache is nil"),
		},
		{
			name: "Given a empty vsName, " +
				"When performInVSRoutingRollback is invoked, " +
				"Then it should return an error",
			sourceIdentity:              "test-identity",
			remoteRegistry:              rr,
			sourceClusterToEventNsCache: map[string]string{"cluster-1": "test-ns"},
			expectedError:               fmt.Errorf("vsname is empty"),
		},
		{
			name: "Given an identity which needs to be rolled back " +
				"When performInVSRoutingRollback is invoked, " +
				"Then it should successfully update the VS of the identity with sync namespace in exportTo",
			sourceIdentity: "test-identity",
			remoteRegistry: rr,
			vsName:         "test-env.test-identity.global",
			cluster:        "cluster-1",
			vsNames: []string{
				"test-env.test-identity.global-incluster-vs",
			},
			sourceClusterToEventNsCache: map[string]string{"cluster-1": "test-ns"},
			expectedError:               nil,
			expectedVSExportTo:          []string{"test-sync-ns"},
		},
		{
			name: "Given a cluster where all incluster VSs should be rolled back " +
				"When performInVSRoutingRollback is invoked, " +
				"Then it should successfully update all VSs in the given cluster with sync namespace in exportTo",
			sourceIdentity: "test-identity1",
			remoteRegistry: rr,
			vsName:         "test-env.test-identity1.global",
			cluster:        "cluster-2",
			vsNames: []string{
				"test-env.test-identity1.global-incluster-vs",
				"test-env.test-identity2.global-incluster-vs",
			},
			sourceClusterToEventNsCache: map[string]string{"cluster-2": "test-ns"},
			expectedError:               nil,
			expectedVSExportTo:          []string{"test-sync-ns"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := performInVSRoutingRollback(
				context.Background(),
				ctxLogger,
				tc.remoteRegistry,
				tc.sourceIdentity,
				tc.sourceClusterToEventNsCache,
				tc.vsName)
			if tc.expectedError != nil {
				require.NotNil(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.Nil(t, err)
				istioClient := tc.remoteRegistry.GetRemoteController(tc.cluster).VirtualServiceController.IstioClient
				for _, vsName := range tc.vsNames {
					actualVS, err := istioClient.
						NetworkingV1alpha3().
						VirtualServices(util.IstioSystemNamespace).
						Get(context.Background(), vsName, metaV1.GetOptions{})
					require.Nil(t, err)
					require.Equal(t, tc.expectedVSExportTo, actualVS.Spec.ExportTo)
				}
			}
		})
	}

}

func TestDoReconcileVirtualService(t *testing.T) {

	cachedInClusterVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "host1.test.global-incluster-vs",
		},
		Spec: networkingV1Alpha3.VirtualService{
			ExportTo: []string{"ns1"},
			Hosts:    []string{"host1.test.global"},
			Http: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "host1.test.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "host1.svc.cluster.local",
							},
						},
					},
				},
			},
		},
	}
	cachedIngressVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "host1.test.global-routing-vs",
		},
		Spec: networkingV1Alpha3.VirtualService{
			ExportTo: []string{"ns1"},
			Hosts:    []string{"host1.test.global"},
			Gateways: []string{"test-gateway"},
			Tls: []*networkingV1Alpha3.TLSRoute{
				{
					Match: []*networkingV1Alpha3.TLSMatchAttributes{
						{
							SniHosts: []string{"host1.test.global"},
						},
					},
					Route: []*networkingV1Alpha3.RouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "host1.svc.cluster.local",
							},
						},
					},
				},
			},
		},
	}

	vsCache := istio.NewVirtualServiceCache()
	vsCache.Put(cachedIngressVS)
	vsCache.Put(cachedInClusterVS)

	testCases := []struct {
		name           string
		rc             *RemoteController
		desiredVS      *apiNetworkingV1Alpha3.VirtualService
		comparator     VSRouteComparator
		expectedResult bool
		expectedError  error
	}{
		{
			name: "Given nil remoteController" +
				"When doReconcileVirtualService func is called" +
				"Then it should return an error",
			rc:            nil,
			expectedError: fmt.Errorf("remoteController is nil"),
		},
		{
			name: "Given nil vs" +
				"When doReconcileVirtualService func is called" +
				"Then it should return an error",
			rc:            &RemoteController{},
			expectedError: fmt.Errorf("virtualService is nil"),
		},
		{
			name: "Given nil vs controller" +
				"When doReconcileVirtualService func is called" +
				"Then it should return an error",
			rc:            &RemoteController{},
			desiredVS:     cachedInClusterVS,
			expectedError: fmt.Errorf("virtualService controller is nil"),
		},
		{
			name: "Given nil vs controller cache" +
				"When doReconcileVirtualService func is called" +
				"Then it should return an error",
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{},
			},
			desiredVS:     cachedInClusterVS,
			expectedError: fmt.Errorf("virtualServiceCache is nil"),
		},
		{
			name: "Given valid params" +
				"And there is no VS in cache" +
				"When doReconcileVirtualService func is called" +
				"Then it should return true and no error",
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					VirtualServiceCache: istio.NewVirtualServiceCache(),
				},
			},
			desiredVS:      cachedInClusterVS,
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name: "Given valid params" +
				"And the exportTo doesn't match" +
				"When doReconcileVirtualService func is called" +
				"Then it should return true and no error",
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					VirtualServiceCache: vsCache,
				},
			},
			desiredVS: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					ExportTo: []string{"ns1, ns2"},
				},
			},
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name: "Given valid params" +
				"And the hosts doesn't match" +
				"When doReconcileVirtualService func is called" +
				"Then it should return true and no error",
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					VirtualServiceCache: vsCache,
				},
			},
			desiredVS: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					ExportTo: []string{"ns1"},
					Hosts:    []string{"host1.test.global", "host2.test.global"},
				},
			},
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name: "Given valid params" +
				"And the http routes doesn't match" +
				"When doReconcileVirtualService func is called" +
				"Then it should return true and no error",
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					VirtualServiceCache: vsCache,
				},
			},
			desiredVS: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					ExportTo: []string{"ns1"},
					Hosts:    []string{"host1.test.global"},
					Http: []*networkingV1Alpha3.HTTPRoute{
						{
							Name: "host1.test.global",
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "host1.svc.cluster.local",
									},
								},
							},
						},
						{
							Name: "stage.host1.test.global",
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "stage.host1.svc.cluster.local",
									},
								},
							},
						},
					},
				},
			},
			comparator:     httpRoutesComparator,
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name: "Given valid params" +
				"And the tls routes doesn't match" +
				"When doReconcileVirtualService func is called" +
				"Then it should return true and no error",
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					VirtualServiceCache: vsCache,
				},
			},
			desiredVS: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					ExportTo: []string{"ns1"},
					Hosts:    []string{"host1.test.global"},
					Tls: []*networkingV1Alpha3.TLSRoute{
						{
							Match: []*networkingV1Alpha3.TLSMatchAttributes{
								{
									SniHosts: []string{"host1.test.global"},
								},
							},
							Route: []*networkingV1Alpha3.RouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "host1.svc.cluster.local",
									},
								},
							},
						},
						{
							Match: []*networkingV1Alpha3.TLSMatchAttributes{
								{
									SniHosts: []string{"stage.host1.test.global"},
								},
							},
							Route: []*networkingV1Alpha3.RouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "stage.host1.svc.cluster.local",
									},
								},
							},
						},
					},
				},
			},
			comparator:     tlsRoutesComparator,
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name: "Given valid params" +
				"And the gateways doesn't match" +
				"When doReconcileVirtualService func is called" +
				"Then it should return true and no error",
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					VirtualServiceCache: vsCache,
				},
			},
			desiredVS: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					ExportTo: []string{"ns1"},
					Hosts:    []string{"host1.test.global"},
					Gateways: []string{"new-gateway"},
					Tls: []*networkingV1Alpha3.TLSRoute{
						{
							Match: []*networkingV1Alpha3.TLSMatchAttributes{
								{
									SniHosts: []string{"host1.test.global"},
								},
							},
							Route: []*networkingV1Alpha3.RouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "host1.svc.cluster.local",
									},
								},
							},
						},
					},
				},
			},
			comparator:     tlsRoutesComparator,
			expectedResult: true,
			expectedError:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := doReconcileVirtualService(
				tc.rc,
				tc.desiredVS,
				tc.comparator)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedResult, actual)
			}
		})
	}

}

func TestHttpRoutesComparator(t *testing.T) {

	testCases := []struct {
		name           string
		vs1            *networkingV1Alpha3.VirtualService
		vs2            *networkingV1Alpha3.VirtualService
		expectedResult bool
		expectedError  error
	}{
		{
			name: "Given nil vs1" +
				"When httpRoutesComparator func is called" +
				"Then it should return an error",
			vs1:           nil,
			expectedError: fmt.Errorf("vs1Spec is nil"),
		},
		{
			name: "Given nil vs2" +
				"When httpRoutesComparator func is called" +
				"Then it should return an error",
			vs1:           &networkingV1Alpha3.VirtualService{},
			vs2:           nil,
			expectedError: fmt.Errorf("vs2Spec is nil"),
		},
		{
			name: "Given nil vs1.Spec.http" +
				"When httpRoutesComparator func is called" +
				"Then it should return an error",
			vs1:           &networkingV1Alpha3.VirtualService{},
			vs2:           &networkingV1Alpha3.VirtualService{},
			expectedError: fmt.Errorf("vs1.Spec.Http is nil"),
		},
		{
			name: "Given nil vs2.Spec.http" +
				"When httpRoutesComparator func is called" +
				"Then it should return an error",
			vs1: &networkingV1Alpha3.VirtualService{
				Http: []*networkingV1Alpha3.HTTPRoute{},
			},
			vs2:           &networkingV1Alpha3.VirtualService{},
			expectedError: fmt.Errorf("vs2.Spec.Http is nil"),
		},
		{
			name: "Given equal virtualservices http routes" +
				"When httpRoutesComparator func is called" +
				"Then it should return true and no error",
			vs1: &networkingV1Alpha3.VirtualService{
				Http: []*networkingV1Alpha3.HTTPRoute{
					{
						Name: "stage.host1.test.global",
						Route: []*networkingV1Alpha3.HTTPRouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "canary.host1.svc.cluster.local",
								},
							},
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "stage.host1.svc.cluster.local",
								},
							},
						},
					},
					{
						Name: "preview.stage.host1.test.global",
						Route: []*networkingV1Alpha3.HTTPRouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "preview.host2.svc.cluster.local",
								},
							},
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "stage.host2.svc.cluster.local",
								},
							},
						},
					},
				},
			},
			vs2: &networkingV1Alpha3.VirtualService{
				Http: []*networkingV1Alpha3.HTTPRoute{
					{
						Name: "preview.stage.host1.test.global",
						Route: []*networkingV1Alpha3.HTTPRouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "stage.host2.svc.cluster.local",
								},
							},
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "preview.host2.svc.cluster.local",
								},
							},
						},
					},
					{
						Name: "stage.host1.test.global",
						Route: []*networkingV1Alpha3.HTTPRouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "stage.host1.svc.cluster.local",
								},
							},
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "canary.host1.svc.cluster.local",
								},
							},
						},
					},
				},
			},
			expectedError:  nil,
			expectedResult: true,
		},
		{
			name: "Given un-equal virtualservices http routes" +
				"When httpRoutesComparator func is called" +
				"Then it should return true and no error",
			vs1: &networkingV1Alpha3.VirtualService{
				Http: []*networkingV1Alpha3.HTTPRoute{
					{
						Route: []*networkingV1Alpha3.HTTPRouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "stage.host2.svc.cluster.local",
								},
							},
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "preview.host2.svc.cluster.local",
								},
							},
						},
					},
				},
			},
			vs2: &networkingV1Alpha3.VirtualService{
				Http: []*networkingV1Alpha3.HTTPRoute{
					{
						Route: []*networkingV1Alpha3.HTTPRouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "stage.host1.svc.cluster.local",
								},
							},
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "canary.host1.svc.cluster.local",
								},
							},
						},
					},
					{
						Route: []*networkingV1Alpha3.HTTPRouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "stage.host2.svc.cluster.local",
								},
							},
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "preview.host2.svc.cluster.local",
								},
							},
						},
					},
				},
			},
			expectedError:  nil,
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := httpRoutesComparator(tc.vs1, tc.vs2)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedResult, actual)
			}
		})
	}

}

func TestTlsRoutesComparator(t *testing.T) {

	testCases := []struct {
		name           string
		vs1            *networkingV1Alpha3.VirtualService
		vs2            *networkingV1Alpha3.VirtualService
		expectedResult bool
		expectedError  error
	}{
		{
			name: "Given nil vs1" +
				"When tlsRoutesComparator func is called" +
				"Then it should return an error",
			vs1:           nil,
			expectedError: fmt.Errorf("vs1Spec is nil"),
		},
		{
			name: "Given nil vs2" +
				"When tlsRoutesComparator func is called" +
				"Then it should return an error",
			vs1:           &networkingV1Alpha3.VirtualService{},
			vs2:           nil,
			expectedError: fmt.Errorf("vs2Spec is nil"),
		},
		{
			name: "Given nil vs1.Spec.Tls" +
				"When tlsRoutesComparator func is called" +
				"Then it should return an error",
			vs1:           &networkingV1Alpha3.VirtualService{},
			vs2:           &networkingV1Alpha3.VirtualService{},
			expectedError: fmt.Errorf("vs1.Spec.Tls is nil"),
		},
		{
			name: "Given nil vs2.Spec.Tls" +
				"When tlsRoutesComparator func is called" +
				"Then it should return an error",
			vs1: &networkingV1Alpha3.VirtualService{
				Tls: []*networkingV1Alpha3.TLSRoute{},
			},
			vs2:           &networkingV1Alpha3.VirtualService{},
			expectedError: fmt.Errorf("vs2.Spec.Tls is nil"),
		},
		{
			name: "Given equal virtualservices http routes" +
				"When tlsRoutesComparator func is called" +
				"Then it should return true and no error",
			vs1: &networkingV1Alpha3.VirtualService{
				Tls: []*networkingV1Alpha3.TLSRoute{
					{
						Match: []*networkingV1Alpha3.TLSMatchAttributes{
							{
								SniHosts: []string{"stage.host1.global"},
							},
						},
						Route: []*networkingV1Alpha3.RouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "stage.host1.svc.cluster.local",
								},
							},
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "canary.host1.svc.cluster.local",
								},
							},
						},
					},
					{
						Match: []*networkingV1Alpha3.TLSMatchAttributes{
							{
								SniHosts: []string{"canary.stage.host1.global"},
							},
						},
						Route: []*networkingV1Alpha3.RouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "canary.host1.svc.cluster.local",
								},
							},
						},
					},
				},
			},
			vs2: &networkingV1Alpha3.VirtualService{
				Tls: []*networkingV1Alpha3.TLSRoute{
					{
						Match: []*networkingV1Alpha3.TLSMatchAttributes{
							{
								SniHosts: []string{"stage.host1.global"},
							},
						},
						Route: []*networkingV1Alpha3.RouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "canary.host1.svc.cluster.local",
								},
							},
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "stage.host1.svc.cluster.local",
								},
							},
						},
					},
					{
						Match: []*networkingV1Alpha3.TLSMatchAttributes{
							{
								SniHosts: []string{"canary.stage.host1.global"},
							},
						},
						Route: []*networkingV1Alpha3.RouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "canary.host1.svc.cluster.local",
								},
							},
						},
					},
				},
			},
			expectedError:  nil,
			expectedResult: true,
		},
		{
			name: "Given un-equal virtualservices http routes" +
				"When tlsRoutesComparator func is called" +
				"Then it should return true and no error",
			vs1: &networkingV1Alpha3.VirtualService{
				Tls: []*networkingV1Alpha3.TLSRoute{
					{
						Match: []*networkingV1Alpha3.TLSMatchAttributes{
							{
								SniHosts: []string{"stage.host1.global"},
							},
						},
						Route: []*networkingV1Alpha3.RouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "stage.host1.svc.cluster.local",
								},
							},
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "canary.host1.svc.cluster.local",
								},
							},
						},
					},
					{
						Match: []*networkingV1Alpha3.TLSMatchAttributes{
							{
								SniHosts: []string{"canary.stage.host1.global"},
							},
						},
						Route: []*networkingV1Alpha3.RouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "canary.host1.svc.cluster.local",
								},
							},
						},
					},
				},
			},
			vs2: &networkingV1Alpha3.VirtualService{
				Tls: []*networkingV1Alpha3.TLSRoute{
					{
						Match: []*networkingV1Alpha3.TLSMatchAttributes{
							{
								SniHosts: []string{"stage.host1.global"},
							},
						},
						Route: []*networkingV1Alpha3.RouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "canary.host1.svc.cluster.local",
								},
							},
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "stage.host1.svc.cluster.local",
								},
							},
						},
					},
					{
						Match: []*networkingV1Alpha3.TLSMatchAttributes{
							{
								SniHosts: []string{"canary.stage.host1.global"},
							},
						},
						Route: []*networkingV1Alpha3.RouteDestination{
							{
								Destination: &networkingV1Alpha3.Destination{
									Host: "canary1.host1.svc.cluster.local",
								},
							},
						},
					},
				},
			},
			expectedError:  nil,
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := tlsRoutesComparator(tc.vs1, tc.vs2)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedResult, actual)
			}
		})
	}

}

func TestMergeHosts(t *testing.T) {

	testCases := []struct {
		name                string
		hosts1              []string
		hosts2              []string
		expectedMergedHosts []string
	}{
		{
			name: "Given empty hosts params" +
				"And mergeHosts func is called" +
				"Then the fund should return empty merged hosts",
			hosts1:              []string{},
			hosts2:              []string{},
			expectedMergedHosts: []string{},
		},
		{
			name: "Given empty hosts1 param" +
				"And mergeHosts func is called" +
				"Then the fund should return  merged hosts containing only hosts2",
			hosts1:              []string{},
			hosts2:              []string{"stage1.host1.global", "stage2.host2.global"},
			expectedMergedHosts: []string{"stage1.host1.global", "stage2.host2.global"},
		},
		{
			name: "Given empty hosts1 param" +
				"And mergeHosts func is called" +
				"Then the fund should return  merged hosts containing only hosts2",
			hosts1:              []string{},
			hosts2:              []string{"stage1.host1.global", "stage2.host2.global"},
			expectedMergedHosts: []string{"stage1.host1.global", "stage2.host2.global"},
		},
		{
			name: "Given empty hosts2 param" +
				"And mergeHosts func is called" +
				"Then the fund should return  merged hosts containing only hosts2",
			hosts1:              []string{"stage1.host1.global", "stage2.host2.global"},
			hosts2:              []string{},
			expectedMergedHosts: []string{"stage1.host1.global", "stage2.host2.global"},
		},
		{
			name: "Given valid hosts1 hosts2 param" +
				"And mergeHosts func is called" +
				"Then the fund should return merged hosts",
			hosts1:              []string{"stage1.host1.global", "stage2.host2.global"},
			hosts2:              []string{"stage3.host3.global"},
			expectedMergedHosts: []string{"stage1.host1.global", "stage2.host2.global", "stage3.host3.global"},
		},
		{
			name: "Given valid hosts1 hosts2 param with duplicate hosts" +
				"And mergeHosts func is called" +
				"Then the func should return dedup merged hosts",
			hosts1:              []string{"stage1.host1.global", "stage2.host2.global"},
			hosts2:              []string{"stage1.host1.global"},
			expectedMergedHosts: []string{"stage1.host1.global", "stage2.host2.global"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := mergeHosts(tc.hosts1, tc.hosts2)
			assert.Equal(t, tc.expectedMergedHosts, actual)
		})
	}

}

func TestMergeHTTPRoutes(t *testing.T) {

	testCases := []struct {
		name                 string
		vsroutes1            []*networkingV1Alpha3.HTTPRoute
		vsroutes2            []*networkingV1Alpha3.HTTPRoute
		expectedMergedRoutes []*networkingV1Alpha3.HTTPRoute
		expectedError        error
	}{
		{
			name: "Given nil vsroutes1 params" +
				"And mergeHTTPRoutes func is called" +
				"Then the fund should return empty merged routes",
			vsroutes1:            nil,
			vsroutes2:            []*networkingV1Alpha3.HTTPRoute{},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{},
			expectedError:        fmt.Errorf("custom VS HTTPRoutes is nil"),
		},
		{
			name: "Given nil vsroutes2 params" +
				"And mergeHTTPRoutes func is called" +
				"Then the fund should return empty merged routes",
			vsroutes1:            []*networkingV1Alpha3.HTTPRoute{},
			vsroutes2:            nil,
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{},
			expectedError:        fmt.Errorf("incluster VS HTTPRoutes is nil"),
		},
		{
			name: "Given empty vsroutes params" +
				"And mergeHTTPRoutes func is called" +
				"Then the fund should return empty merged routes",
			vsroutes1:            []*networkingV1Alpha3.HTTPRoute{},
			vsroutes2:            []*networkingV1Alpha3.HTTPRoute{},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{},
		},
		{
			name: "Given empty vsroutes1 params" +
				"And mergeHTTPRoutes func is called" +
				"Then the fund should return empty merged routes",
			vsroutes1: []*networkingV1Alpha3.HTTPRoute{},
			vsroutes2: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
			},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Given empty vsroutes2 params" +
				"And mergeHTTPRoutes func is called" +
				"Then the fund should return empty merged routes",
			vsroutes1: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
			},
			vsroutes2: []*networkingV1Alpha3.HTTPRoute{},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Given empty vsroutes2 params" +
				"And mergeHTTPRoutes func is called" +
				"Then the fund should return empty merged routes",
			vsroutes1: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
			},
			vsroutes2: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
			},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
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
			actual, err := mergeHTTPRoutes(tc.vsroutes1, tc.vsroutes2)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedMergedRoutes, actual)
			}
		})
	}

}

func TestSortVSRoutes(t *testing.T) {

	testCases := []struct {
		name                 string
		customVSRoutes       []*networkingV1Alpha3.HTTPRoute
		inclusterVSRoutes    []*networkingV1Alpha3.HTTPRoute
		env                  string
		expectedMergedRoutes []*networkingV1Alpha3.HTTPRoute
		expectedError        error
	}{
		{
			name: "Given empty customVSRoutes and inclusterVSRoutes params" +
				"And sortVSRoutes func is called" +
				"Then the func should return empty merged routes",
			customVSRoutes:       []*networkingV1Alpha3.HTTPRoute{},
			inclusterVSRoutes:    []*networkingV1Alpha3.HTTPRoute{},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{},
		},
		{
			name: "Given empty customVSRoutes params" +
				"And sortVSRoutes func is called" +
				"Then the func should return merged routes only from inclusterVSRoutes",
			env:            "qal",
			customVSRoutes: []*networkingV1Alpha3.HTTPRoute{},
			inclusterVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "qal.stage1.host1.global",
								},
							},
						},
					},
				},
				{
					Name: "canary.qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "canary.qal.stage1.host1.global",
								},
							},
						},
					},
				},
			},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "canary.qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "canary.qal.stage1.host1.global",
								},
							},
						},
					},
				},
				{
					Name: "qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "qal.stage1.host1.global",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Given empty vsroutes2 params" +
				"And mergeHTTPRoutes func is called" +
				"Then the fund should return empty merged routes",
			env: "qal",
			customVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
			},
			inclusterVSRoutes: []*networkingV1Alpha3.HTTPRoute{},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Given valid params" +
				"And mergeHTTPRoutes func is called" +
				"Then the fund should return sorted routes",
			env: "qal",
			customVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
			},
			inclusterVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "qal.stage1.host1.global",
								},
							},
						},
					},
				},
				{
					Name: "canary.qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "canary.qal.stage1.host1.global",
								},
							},
						},
					},
				},
			},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "canary.qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "canary.qal.stage1.host1.global",
								},
							},
						},
					},
				},
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
				{
					Name: "qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "qal.stage1.host1.global",
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
			actual := sortVSRoutes(tc.customVSRoutes, tc.inclusterVSRoutes, tc.env)
			assert.Equal(t, tc.expectedMergedRoutes, actual)
		})
	}

}

func TestModifyCustomVSHTTPRoutes(t *testing.T) {

	vs := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:   "qal-air.stage1.host1.global-incluster-vs",
			Labels: map[string]string{common.VSRoutingType: common.VSRoutingTypeInCluster},
		},
		Spec: networkingV1Alpha3.VirtualService{
			ExportTo: []string{"ns1"},
			Hosts:    []string{"qal-air.stage1.host1.global"},
			Http: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "qal-air.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
						},
					},
				},
			},
		},
	}

	hostToRouteDestinationCache := istio.NewHostToRouteDestinationCache()
	hostToRouteDestinationCache.Put(vs)

	rc := &RemoteController{
		ClusterID: "cluster-1",
		VirtualServiceController: &istio.VirtualServiceController{
			HostToRouteDestinationCache: hostToRouteDestinationCache,
		},
	}

	testCases := []struct {
		name                 string
		customVSRoutes       []*networkingV1Alpha3.HTTPRoute
		inclusterVSRoutes    []*networkingV1Alpha3.HTTPRoute
		env                  string
		expectedMergedRoutes []*networkingV1Alpha3.HTTPRoute
		expectedError        error
	}{
		{
			name: "Given empty customVSRoutes and inclusterVSRoutes params" +
				"And sortVSRoutes func is called" +
				"Then the func should return empty merged routes",
			customVSRoutes:       []*networkingV1Alpha3.HTTPRoute{},
			inclusterVSRoutes:    []*networkingV1Alpha3.HTTPRoute{},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{},
		},
		{
			name: "Given empty customVSRoutes params" +
				"And sortVSRoutes func is called" +
				"Then the func should return merged routes only from inclusterVSRoutes",
			env:            "qal",
			customVSRoutes: []*networkingV1Alpha3.HTTPRoute{},
			inclusterVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "qal.stage1.host1.global",
								},
							},
						},
					},
				},
				{
					Name: "canary.qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "canary.qal.stage1.host1.global",
								},
							},
						},
					},
				},
			},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "canary.qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "canary.qal.stage1.host1.global",
								},
							},
						},
					},
				},
				{
					Name: "qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 50,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "qal.stage1.host1.global",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Given empty vsroutes2 params" +
				"And mergeHTTPRoutes func is called" +
				"Then the fund should return empty merged routes",
			env: "qal",
			customVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
			},
			inclusterVSRoutes: []*networkingV1Alpha3.HTTPRoute{},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1.host1.global",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Given a custom vs with fqdn that exists in the incluster cache" +
				"And modifyCustomVSHTTPRoutes func is called" +
				"Then the func should successfully modify the customVS",
			env: "qal",
			customVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Timeout: &duration.Duration{Seconds: 10},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "Health Check",
									},
								},
							},
						},
					},
				},
			},
			inclusterVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 50,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 50,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "qal.stage1.host1.global",
								},
							},
						},
					},
				},
			},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Timeout: &duration.Duration{Seconds: 10},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 50,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 50,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "Health Check",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Given a custom vs with fqdn that exists in the incluster cache" +
				"And also contains a route to a different fqdn that is in the hostToRouteDestinationCache cache" +
				"And modifyCustomVSHTTPRoutes func is called" +
				"Then the func should successfully modify the customVS",
			env: "qal",
			customVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Timeout: &duration.Duration{Seconds: 10},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "Health Check",
									},
								},
							},
						},
					},
				},
				{
					Timeout: &duration.Duration{Seconds: 50},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "qal-air",
									},
								},
							},
						},
					},
				},
			},
			inclusterVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 50,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 50,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "qal.stage1.host1.global",
								},
							},
						},
					},
				},
			},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Timeout: &duration.Duration{Seconds: 10},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 50,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 50,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "Health Check",
									},
								},
							},
						},
					},
				},
				{
					Timeout: &duration.Duration{Seconds: 50},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "qal-air",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Given a custom vs with fqdn that exists in the incluster cache" +
				"And also contains a route to a different fqdn that is in the hostToRouteDestinationCache cache" +
				"And the route in custom vs has a traffic split" +
				"And modifyCustomVSHTTPRoutes func is called" +
				"Then the func should successfully modify the customVS",
			env: "qal",
			customVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Timeout: &duration.Duration{Seconds: 10},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 90,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 10,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "Health Check",
									},
								},
							},
						},
					},
				},
				{
					Timeout: &duration.Duration{Seconds: 50},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "qal-air",
									},
								},
							},
						},
					},
				},
			},
			inclusterVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 90,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 10,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "qal.stage1.host1.global",
								},
							},
						},
					},
				},
			},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Timeout: &duration.Duration{Seconds: 10},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 81,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 9,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 10,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "Health Check",
									},
								},
							},
						},
					},
				},
				{
					Timeout: &duration.Duration{Seconds: 50},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "qal-air",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Given a custom vs with fqdn that exists in the incluster cache" +
				"And contains a route to a different fqdn that is in the hostToRouteDestinationCache cache" +
				"And contains a route that is not present in either" +
				"And modifyCustomVSHTTPRoutes func is called" +
				"Then the func should successfully modify the customVS as leave the route with no" +
				"FQDN mapping, as is",
			env: "qal",
			customVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Timeout: &duration.Duration{Seconds: 10},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "Health Check",
									},
								},
							},
						},
					},
				},
				{
					Timeout: &duration.Duration{Seconds: 50},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.stage1.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "qal-air",
									},
								},
							},
						},
					},
				},
				{
					Timeout: &duration.Duration{Seconds: 30},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.greeting.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "qal-greeting",
									},
								},
							},
						},
					},
				},
			},
			inclusterVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "qal.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 90,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 10,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "qal.stage1.host1.global",
								},
							},
						},
					},
				},
			},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Timeout: &duration.Duration{Seconds: 10},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 90,
						},
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "canary.qal.stage1.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
							Weight: 10,
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "Health Check",
									},
								},
							},
						},
					},
				},
				{
					Timeout: &duration.Duration{Seconds: 50},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.svc.cluster.local",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 8090,
								},
							},
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "qal-air",
									},
								},
							},
						},
					},
				},
				{
					Timeout: &duration.Duration{Seconds: 30},
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal.greeting.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
						},
					},
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Headers: map[string]*networkingV1Alpha3.StringMatch{
								"x-intuit-route-name": {
									MatchType: &networkingV1Alpha3.StringMatch_Exact{
										Exact: "qal-greeting",
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

			actual, err := modifyCustomVSHTTPRoutes(tc.customVSRoutes, tc.inclusterVSRoutes, rc)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				for i := 0; i < len(tc.expectedMergedRoutes); i++ {
					for j := 0; j < len(tc.expectedMergedRoutes[i].Route); j++ {
						assert.Equal(t, tc.expectedMergedRoutes[i].Route[j].Destination, actual[i].Route[j].Destination)
						assert.Equal(t, tc.expectedMergedRoutes[i].Route[j].Weight, actual[i].Route[j].Weight)
					}
					assert.Equal(t, tc.expectedMergedRoutes[i].Timeout, actual[i].Timeout)
					assert.Equal(t, tc.expectedMergedRoutes[i].Match, actual[i].Match)
				}
			}
		})
	}

}
