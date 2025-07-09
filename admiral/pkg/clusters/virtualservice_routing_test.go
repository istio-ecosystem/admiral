package clusters

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1alpha12 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
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

	ap := common.AdmiralParams{
		SyncNamespace:                      "admiral-sync",
		LabelSet:                           &common.LabelSet{},
		ExportToIdentityList:               []string{"*"},
		ExportToMaxNamespaces:              100,
		EnableSWAwareNSCaches:              true,
		EnableVSRoutingInCluster:           true,
		VSRoutingInClusterEnabledResources: map[string]string{"cluster-1": "test-identity"},
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	istioClientWithExistingVS := istioFake.NewSimpleClientset()
	istioClientWithExistingVS.NetworkingV1alpha3().VirtualServices(util.IstioSystemNamespace).
		Create(context.Background(), existingVS, metaV1.CreateOptions{})

	istioClientWithNoExistingVS := istioFake.NewSimpleClientset()
	rc := &RemoteController{
		ClusterID: "cluster-1",
		VirtualServiceController: &istio.VirtualServiceController{
			VirtualServiceCache:         istio.NewVirtualServiceCache(),
			IdentityVirtualServiceCache: istio.NewIdentityVirtualServiceCache(),
		},
		ServiceEntryController: &istio.ServiceEntryController{
			Cache: istio.NewServiceEntryCache(),
		},
	}

	rc1 := &RemoteController{
		ClusterID: "cluster-2",
		VirtualServiceController: &istio.VirtualServiceController{
			IdentityVirtualServiceCache: istio.NewIdentityVirtualServiceCache(),
		},
		ServiceEntryController: &istio.ServiceEntryController{
			Cache: istio.NewServiceEntryCache(),
		},
	}

	rr := NewRemoteRegistry(context.Background(), ap)
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
		env                         string
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
			name: "Given a empty cname, " +
				"When addUpdateInClusterVirtualServices is invoked, " +
				"Then it should return an error",
			sourceIdentity:              "test-identity",
			remoteRegistry:              rr,
			sourceClusterToDestinations: sourceDestinationsWithSingleDestinationSvc,
			expectedError:               fmt.Errorf("cname is empty"),
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
			env:                         "stage",
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
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.Test-identity.global",
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
			env:                         "stage",
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
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.Test-identity1.global",
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
			env:                         "stage",
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
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.Test-identity.global",
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
			env:                         "stage",
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
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "preview.test-env.Test-identity.global",
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
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.Test-identity.global",
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
			env:                         "stage",
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
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "canary.test-env.Test-identity.global",
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
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.Test-identity.global",
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
			env:                         "stage",
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
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "test-env.Test-identity.global",
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
				tc.sourceIdentity, tc.env, map[string]string{"cluster-1": "testns"})
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

	ap := common.AdmiralParams{
		LabelSet:                    &common.LabelSet{},
		EnableSWAwareNSCaches:       true,
		IngressVSExportToNamespaces: []string{"istio-system"},
		VSRoutingGateways:           []string{"istio-system/passthrough-gateway"},
		EnableVSRouting:             true,
	}
	common.ResetSync()
	common.InitializeConfig(ap)

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

	rr := NewRemoteRegistry(context.Background(), ap)
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

	ap := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

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

	ap := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

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

	ap := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

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

	ap := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

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

	ap := common.AdmiralParams{
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
			ap.VSRoutingGateways = tc.routingGateways
			common.ResetSync()
			common.InitializeConfig(ap)
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

	ap := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "env",
			AdmiralCRDIdentityLabel: "identity",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	meshPort := uint32(8080)
	testCases := []struct {
		name                      string
		serviceInstance           map[string]*coreV1.Service
		weightedServices          map[string]*WeightedService
		meshDeployAndRolloutPorts map[string]map[string]uint32
		rollout                   *v1alpha1.Rollout
		deployment                *v1.Deployment
		resourceTypeBeingDeleted  string
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
		{
			name: "Given an empty route destinations map" +
				"And serviceInstance has both rollout and deployment" +
				"And rollout is being deleted" +
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
			resourceTypeBeingDeleted: common.Rollout,
			expectedError:            nil,
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getAllVSRouteDestinationsByCluster(
				tc.serviceInstance, tc.meshDeployAndRolloutPorts, tc.weightedServices, tc.rollout, tc.deployment, tc.resourceTypeBeingDeleted)
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

	ap := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "env",
			AdmiralCRDIdentityLabel: "identity",
		},
		EnableActivePassive: true,
		HostnameSuffix:      "global",
		SyncNamespace:       "sync-ns",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

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

	passiveDR := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "stage.foo.passive.global-default-dr",
			Namespace: "sync-ns",
		},
		Spec: networkingV1Alpha3.DestinationRule{
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
						Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
							{
								From: "*",
								To:   map[string]uint32{"us-east-2": 100},
							},
						},
					},
				},
			},
		},
	}

	drCache := istio.NewDestinationRuleCache()
	drCache.Put(passiveDR)

	passiveRegionSE := &apiNetworkingV1Alpha3.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "stage.foo.passive.global-se",
		},
		Spec: networkingV1Alpha3.ServiceEntry{
			Endpoints: []*networkingV1Alpha3.WorkloadEntry{
				{
					Locality: "us-east-2",
				},
				{
					Locality: "us-west-2",
				},
			},
		},
	}

	seCache := istio.NewServiceEntryCache()
	seCache.Put(passiveRegionSE, cluster1)

	rrWithNoGTP := NewRemoteRegistry(context.Background(), admiralParams)
	rrWithNoGTP.remoteControllers = map[string]*RemoteController{
		"cluster1": {
			DestinationRuleController: &istio.DestinationRuleController{
				Cache: drCache,
			},
			ServiceEntryController: &istio.ServiceEntryController{
				Cache: seCache,
			},
		},
	}
	rrWithNoGTP.AdmiralCache = &AdmiralCache{
		GlobalTrafficCache: &globalTrafficCache{
			identityCache: make(map[string]*v1alpha12.GlobalTrafficPolicy),
			mutex:         &sync.Mutex{},
		},
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
		cname                    string
		sourceCluster            string
		remoteRegistry           *RemoteRegistry
	}{
		{
			name: "Given valid empty route destinations and valid GTP" +
				"When processGTPAndAddWeightsByCluster is invoked, " +
				"Then it should return error",
			sourceIdentity: "test-identity",
			env:            "test-env",
			expectedError:  fmt.Errorf("destinations map is nil"),
			updateWeights:  true,
			remoteRegistry: rr,
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
			remoteRegistry:        rr,
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
			remoteRegistry:        rr,
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
		{
			name: "Given valid params" +
				"And there is no corresponding GTP" +
				"And the func is called for in-cluster vs" +
				"And the active passive is enabled" +
				"When processGTPAndAddWeightsByCluster is invoked, " +
				"Then it should populate the destinations with one region's VS having SE endpoints",
			sourceIdentity:        "test-identity",
			env:                   "test-env",
			expectedError:         nil,
			updateWeights:         true,
			sourceClusterLocality: "us-west-2",
			sourceCluster:         "cluster1",
			cname:                 "stage.foo.passive.global",
			remoteRegistry:        rrWithNoGTP,
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
						Weight: 0,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-env.test-identity.global",
							Port: &networkingV1Alpha3.PortSelector{
								Number: 80,
							},
						},
						Weight: 100,
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
				tc.remoteRegistry,
				tc.sourceIdentity,
				tc.env,
				tc.sourceClusterLocality,
				tc.destinations,
				tc.updateWeights, tc.cname, tc.sourceCluster)
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

	ap := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

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

	ap := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

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

	ap := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

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

	ap := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

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
			Name:        "test-ns.svc.cluster.local-incluster-dr",
			Namespace:   util.IstioSystemNamespace,
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
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

	ap := common.AdmiralParams{
		SANPrefix:                          "test-san-prefix",
		EnableVSRoutingInCluster:           true,
		VSRoutingInClusterEnabledResources: map[string]string{"cluster-1": "test-identity", "cluster-2": "*"},
		VSRoutingSlowStartEnabledClusters:  []string{"cluster-1"},
		ExportToIdentityList:               []string{"*"},
		ExportToMaxNamespaces:              100,
		EnableSWAwareNSCaches:              true,
		MaxRequestsPerConnection:           DefaultMaxRequestsPerConnection,
		DisableDefaultAutomaticFailover:    true,
	}

	common.ResetSync()
	common.InitializeConfig(ap)

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

	rr.AdmiralCache.ClientConnectionConfigCache = NewClientConnectionConfigCache()

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
		env                      string
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
			env:            "stage",
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
			env:            "stage",
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
			env:           "stage",
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
						ConnectionPool: &networkingV1Alpha3.ConnectionPoolSettings{
							Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
								MaxRequestsPerConnection: 100,
							},
						},
						OutlierDetection: &networkingV1Alpha3.OutlierDetection{
							ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 0},
							Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
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
			env:            "stage",
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
						ConnectionPool: &networkingV1Alpha3.ConnectionPoolSettings{
							Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
								MaxRequestsPerConnection: 100,
							},
						},
						OutlierDetection: &networkingV1Alpha3.OutlierDetection{
							ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 0},
							Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
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
			env:            "stage",
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
						ConnectionPool: &networkingV1Alpha3.ConnectionPoolSettings{
							Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
								MaxRequestsPerConnection: 100,
							},
						},
						OutlierDetection: &networkingV1Alpha3.OutlierDetection{
							ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 0},
							Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rc := &RemoteController{
				ClusterID: "cluster-1",
				DestinationRuleController: &istio.DestinationRuleController{
					Cache: istio.NewDestinationRuleCache(),
				},
				VirtualServiceController: &istio.VirtualServiceController{
					IdentityVirtualServiceCache: istio.NewIdentityVirtualServiceCache(),
					IstioClient:                 istioFake.NewSimpleClientset(),
				},
			}
			rc2 := &RemoteController{
				ClusterID: "cluster-2",
				DestinationRuleController: &istio.DestinationRuleController{
					Cache: istio.NewDestinationRuleCache(),
				},
				VirtualServiceController: &istio.VirtualServiceController{
					IdentityVirtualServiceCache: istio.NewIdentityVirtualServiceCache(),
					IstioClient:                 istioFake.NewSimpleClientset(),
				},
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
				tc.cname, tc.env)
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
				require.Equal(t,
					tc.expectedDestinationRules.Spec.TrafficPolicy.ConnectionPool.Http.MaxRequestsPerConnection,
					actualDR.Spec.TrafficPolicy.ConnectionPool.Http.MaxRequestsPerConnection)
			}
		})
	}

}

func TestAddUpdateDestinationRuleForSourceIngress(t *testing.T) {

	existingDR := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:        "test-ns.svc.cluster.local-routing-dr",
			Namespace:   util.IstioSystemNamespace,
			Annotations: map[string]string{resourceCreatedByAnnotationLabel: resourceCreatedByAnnotationValue},
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

	ap := common.AdmiralParams{
		SANPrefix:                         "test-san-prefix",
		IngressVSExportToNamespaces:       []string{"istio-system"},
		EnableVSRouting:                   true,
		VSRoutingSlowStartEnabledClusters: []string{"cluster-1"},
		DisableDefaultAutomaticFailover:   true,
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	istioClientWithExistingDR := istioFake.NewSimpleClientset()
	istioClientWithExistingDR.NetworkingV1alpha3().DestinationRules(util.IstioSystemNamespace).
		Create(context.Background(), existingDR, metaV1.CreateOptions{})

	istioClientWithNoExistingDR := istioFake.NewSimpleClientset()

	rr := NewRemoteRegistry(context.Background(), ap)

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
						ConnectionPool: &networkingV1Alpha3.ConnectionPoolSettings{
							Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
								MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
							},
						},
						OutlierDetection: &networkingV1Alpha3.OutlierDetection{
							ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 0},
							Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
						},
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
						ConnectionPool: &networkingV1Alpha3.ConnectionPoolSettings{
							Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
								MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
							},
						},
						OutlierDetection: &networkingV1Alpha3.OutlierDetection{
							ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 0},
							Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
						},
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
						ConnectionPool: &networkingV1Alpha3.ConnectionPoolSettings{
							Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
								MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
							},
						},
						OutlierDetection: &networkingV1Alpha3.OutlierDetection{
							ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 0},
							Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
						},
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
				ClusterID: "cluster-1",
				DestinationRuleController: &istio.DestinationRuleController{
					Cache: istio.NewDestinationRuleCache(),
				},
			}
			rc2 := &RemoteController{
				ClusterID: "cluster-2",
				DestinationRuleController: &istio.DestinationRuleController{
					Cache: istio.NewDestinationRuleCache(),
				},
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
		{
			name: "Given a GTP with failover" +
				"When getDestinationsForGTPDNSPrefixes is invoked, " +
				"Then it should return an routeDestinations with .local to 0 and .global to 100",
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
								{
									Region: "us-west-2",
									Weight: 0,
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
			},
			expectedError: nil,
			expectedGTPRouteDestination: map[string][]*vsrouting.RouteDestination{
				"test-env.test-identity.global": {
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "active-svc.test-ns.svc.cluster.local",
							Port: &networkingV1Alpha3.PortSelector{
								Number: meshPort,
							},
						},
						Weight: 0,
					},
					{
						Destination: &networkingV1Alpha3.Destination{
							Host: "test-env.test-identity.global",
							Port: &networkingV1Alpha3.PortSelector{
								Number: 80,
							},
						},
						Weight: 100,
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
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
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
		{
			name: "Given a routeDestination with weights total over 100" +
				"When addWeightsToRouteDestinations is invoked, " +
				"Then it should return with weights the same weights",
			routeDestinations: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "active.test-svc.test-ns.svc.cluster.local",
					},
					Weight: 100,
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "preview.test-svc.test-ns.svc.cluster.local",
					},
					Weight: 1,
				},
			}},
			expectedError: nil,
			expectedRouteDestination: map[string][]*vsrouting.RouteDestination{"test-svc.test-ns.mesh": {
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "active.test-svc.test-ns.svc.cluster.local",
					},
					Weight: 100,
				},
				{
					Destination: &networkingV1Alpha3.Destination{
						Host: "preview.test-svc.test-ns.svc.cluster.local",
					},
					Weight: 1,
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

	ap := common.AdmiralParams{
		LabelSet:                            &common.LabelSet{},
		SyncNamespace:                       "test-sync-ns",
		ExportToIdentityList:                []string{"*"},
		ExportToMaxNamespaces:               100,
		EnableSWAwareNSCaches:               true,
		EnableVSRoutingInCluster:            true,
		VSRoutingInClusterDisabledResources: map[string]string{"cluster-2": "*", "cluster-1": "test-identity"},
	}
	common.ResetSync()
	common.InitializeConfig(ap)

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

	rr := NewRemoteRegistry(context.Background(), ap)
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
				"Then the func should return empty merged hosts",
			hosts1:              []string{},
			hosts2:              []string{},
			expectedMergedHosts: []string{},
		},
		{
			name: "Given empty hosts1 param" +
				"And mergeHosts func is called" +
				"Then the func should return  merged hosts containing only hosts2",
			hosts1:              []string{},
			hosts2:              []string{"stage1.host1.global", "stage2.host2.global"},
			expectedMergedHosts: []string{"stage1.host1.global", "stage2.host2.global"},
		},
		{
			name: "Given empty hosts2 param" +
				"And mergeHosts func is called" +
				"Then the func should return  merged hosts containing only hosts2",
			hosts1:              []string{"stage1.host1.global", "stage2.host2.global"},
			hosts2:              []string{},
			expectedMergedHosts: []string{"stage1.host1.global", "stage2.host2.global"},
		},
		{
			name: "Given valid hosts1 hosts2 param" +
				"And mergeHosts func is called" +
				"Then the func should return merged hosts",
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

func TestSortVSRoutes(t *testing.T) {

	testCases := []struct {
		name                 string
		customVSRoutes       []*networkingV1Alpha3.HTTPRoute
		inclusterVSRoutes    []*networkingV1Alpha3.HTTPRoute
		hostsNotInCustomVS   map[string]bool
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
			customVSRoutes: []*networkingV1Alpha3.HTTPRoute{},
			hostsNotInCustomVS: map[string]bool{
				"canary.qal.stage1.host1.global": true,
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
			name: "Given empty inclusterVSRoutes params" +
				"And sortVSRoutes func is called" +
				"Then the func should return only routes from customVSRoutes",
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
				"And sortVSRoutes func is called" +
				"Then the func should return sorted routes",
			hostsNotInCustomVS: map[string]bool{
				"canary.qal.stage1.host1.global": true,
			},
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
		{
			name: "Given valid params" +
				"And sortVSRoutes func is called" +
				"Then the func should return sorted routes",
			hostsNotInCustomVS: map[string]bool{
				"canary.qal.stage1.host1.global": true,
				"qal-air.stage1.host1.global":    true,
			},
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
				{
					Name: "qal-air.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.stage1.svc.cluster.local",
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
									Prefix: "qal-air.stage1.host1.global",
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
					Name: "qal-air.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.stage1.svc.cluster.local",
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
									Prefix: "qal-air.stage1.host1.global",
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
		{
			name: "Given valid params" +
				"And in-cluster VS has duplicate routes" +
				"And sortVSRoutes func is called" +
				"Then the func should return deduplicated sorted routes",
			hostsNotInCustomVS: map[string]bool{
				"canary.qal.stage1.host1.global": true,
				"qal-air.stage1.host1.global":    true,
			},
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
				{
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1-air.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Name: "air",
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1-air-new.host1.global",
								},
							},
						},
					},
				},
			},
			inclusterVSRoutes: []*networkingV1Alpha3.HTTPRoute{
				{
					Name: "air",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1-air.svc.cluster.local",
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
									Prefix: "stage1-air.host1.global",
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
					Name: "qal-air.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.stage1.svc.cluster.local",
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
									Prefix: "qal-air.stage1.host1.global",
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
					Name: "qal-air.stage1.host1.global",
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "qal-air.stage1.svc.cluster.local",
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
									Prefix: "qal-air.stage1.host1.global",
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
					Route: []*networkingV1Alpha3.HTTPRouteDestination{
						{
							Destination: &networkingV1Alpha3.Destination{
								Host: "stage1-air.host1.global",
								Port: &networkingV1Alpha3.PortSelector{
									Number: 80,
								},
							},
							Weight: 100,
						},
					},
					Name: "air",
					Match: []*networkingV1Alpha3.HTTPMatchRequest{
						{
							Authority: &networkingV1Alpha3.StringMatch{
								MatchType: &networkingV1Alpha3.StringMatch_Prefix{
									Prefix: "stage1-air-new.host1.global",
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
			actual := sortVSRoutes(tc.customVSRoutes, tc.inclusterVSRoutes, tc.hostsNotInCustomVS)
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
		expectedMergedRoutes []*networkingV1Alpha3.HTTPRoute
		expectedError        error
	}{
		{
			name: "Given empty customVSRoutes and inclusterVSRoutes params" +
				"And modifyCustomVSHTTPRoutes func is called" +
				"Then the func should return empty merged routes",
			customVSRoutes:       []*networkingV1Alpha3.HTTPRoute{},
			inclusterVSRoutes:    []*networkingV1Alpha3.HTTPRoute{},
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{},
		},
		{
			name: "Given empty customVSRoutes params" +
				"And modifyCustomVSHTTPRoutes func is called" +
				"Then the func should return empty merged custom vs routes",
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
			expectedMergedRoutes: []*networkingV1Alpha3.HTTPRoute{},
		},
		{
			name: "Given empty inclusterVSRoutes params" +
				"And modifyCustomVSHTTPRoutes func is called" +
				"Then the func should return the custom vs routes as is",
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

func TestMergeVS(t *testing.T) {

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
		name             string
		customVS         *apiNetworkingV1Alpha3.VirtualService
		inclusterVS      *apiNetworkingV1Alpha3.VirtualService
		remoteController *RemoteController
		expectedVS       *apiNetworkingV1Alpha3.VirtualService
		expectedError    error
	}{
		{
			name: "Given nil customVS" +
				"When mergeVS func is called" +
				"Then func should return an error",
			expectedError: fmt.Errorf("custom VS is nil"),
		},
		{
			name: "Given nil inclusterVS" +
				"When mergeVS func is called" +
				"Then func should return an error",
			customVS:      &apiNetworkingV1Alpha3.VirtualService{},
			expectedError: fmt.Errorf("incluster VS is nil"),
		},
		{
			name: "Given nil remoteController" +
				"When mergeVS func is called" +
				"Then func should return an error",
			customVS:      &apiNetworkingV1Alpha3.VirtualService{},
			inclusterVS:   &apiNetworkingV1Alpha3.VirtualService{},
			expectedError: fmt.Errorf("remote controller is nil"),
		},
		{
			name: "Given valid params" +
				"When mergeVS func is called" +
				"Then func should return a valid merged vs",
			customVS: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{
						"qal.stage1.host1.global",
						"qal-air.stage1.host1.global",
					},
					Http: []*networkingV1Alpha3.HTTPRoute{
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
							Timeout: &duration.Duration{Seconds: 10},
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "qal-air.stage1.host1.global",
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
												Exact: "qal-air",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			inclusterVS: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{
						"qal.stage1.host1.global",
						"canary.qal.stage1.host1.global",
						"east.qal.stage1.host1.global",
						"west.qal.stage1.host1.global",
					},
					Http: []*networkingV1Alpha3.HTTPRoute{
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
						{
							Name: "canary.qal.stage1.host1.global",
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "canary.qal.stage1.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8090,
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
							Name: "east.qal.stage1.host1.global",
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "east.qal.stage1.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8090,
										},
									},
									Weight: 100,
								},
							},
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "east.qal.stage1.host1.global",
										},
									},
								},
							},
						},
						{
							Name: "west.qal.stage1.host1.global",
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "west.qal.stage1.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8090,
										},
									},
									Weight: 100,
								},
							},
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "west.qal.stage1.host1.global",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedVS: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{
						"qal.stage1.host1.global",
						"qal-air.stage1.host1.global",
						"canary.qal.stage1.host1.global",
						"east.qal.stage1.host1.global",
						"west.qal.stage1.host1.global",
					},
					Http: []*networkingV1Alpha3.HTTPRoute{
						{
							Name: "canary.qal.stage1.host1.global",
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "canary.qal.stage1.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8090,
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
							Name: "east.qal.stage1.host1.global",
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "east.qal.stage1.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8090,
										},
									},
									Weight: 100,
								},
							},
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "east.qal.stage1.host1.global",
										},
									},
								},
							},
						},
						{
							Name: "west.qal.stage1.host1.global",
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "west.qal.stage1.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8090,
										},
									},
									Weight: 100,
								},
							},
							Match: []*networkingV1Alpha3.HTTPMatchRequest{
								{
									Authority: &networkingV1Alpha3.StringMatch{
										MatchType: &networkingV1Alpha3.StringMatch_Prefix{
											Prefix: "west.qal.stage1.host1.global",
										},
									},
								},
							},
						},
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
							Timeout: &duration.Duration{Seconds: 10},
							Route: []*networkingV1Alpha3.HTTPRouteDestination{
								{
									Destination: &networkingV1Alpha3.Destination{
										Host: "qal-air.svc.cluster.local",
										Port: &networkingV1Alpha3.PortSelector{
											Number: 8090,
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
												Exact: "qal-air",
											},
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
				},
			},
			remoteController: rc,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := mergeVS(tc.customVS, tc.inclusterVS, tc.remoteController)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedVS.Spec.Hosts, actual.Spec.Hosts)
				for i := 0; i < len(tc.expectedVS.Spec.Http); i++ {
					for j := 0; j < len(tc.expectedVS.Spec.Http[i].Route); j++ {
						assert.Equal(
							t, tc.expectedVS.Spec.Http[i].Route[j].Destination.Host, actual.Spec.Http[i].Route[j].Destination.Host)
						assert.Equal(
							t, tc.expectedVS.Spec.Http[i].Route[j].Weight, actual.Spec.Http[i].Route[j].Weight)
					}
					assert.Equal(t, tc.expectedVS.Spec.Http[i].Timeout, actual.Spec.Http[i].Timeout)
					assert.Equal(t, tc.expectedVS.Spec.Http[i].Match, actual.Spec.Http[i].Match)
				}
			}
		})
	}

}

func TestGetCustomVirtualService(t *testing.T) {

	syncNS := "test-ns"
	nonCustomVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "noncustom-vs",
			Namespace: syncNS,
		},
		Spec: networkingV1Alpha3.VirtualService{},
	}
	customVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "custom-vs",
			Namespace: syncNS,
			Annotations: map[string]string{
				common.CreatedForEnv: "stage",
			},
			Labels: map[string]string{
				common.CreatedBy:  "testCreatedBy",
				common.CreatedFor: "testidentity",
			},
		},
		Spec: networkingV1Alpha3.VirtualService{},
	}
	customVSWithMultipleEnvsStage := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "custom-vs-stage",
			Namespace: syncNS,
			Annotations: map[string]string{
				common.CreatedForEnv: "stage1_stage2_stage",
			},
			Labels: map[string]string{
				common.CreatedBy:  "testCreatedBy",
				common.CreatedFor: "testidentity",
			},
		},
		Spec: networkingV1Alpha3.VirtualService{},
	}
	customVSWithMultipleEnvsQAL := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "custom-vs-qal",
			Namespace: syncNS,
			Annotations: map[string]string{
				common.CreatedForEnv: "qal1_qal2_qal",
			},
			Labels: map[string]string{
				common.CreatedBy:  "testCreatedBy",
				common.CreatedFor: "testidentity",
			},
		},
		Spec: networkingV1Alpha3.VirtualService{},
	}

	istioClientClusterWithNoVS := istioFake.NewSimpleClientset()
	istioClientCluster := istioFake.NewSimpleClientset()
	istioClientClusterWithMultipleEnvs := istioFake.NewSimpleClientset()
	istioClientClusterWithMultipleVSWithMultipleEnvs := istioFake.NewSimpleClientset()
	istioClientCluster.NetworkingV1alpha3().VirtualServices(syncNS).
		Create(context.Background(), nonCustomVS, metaV1.CreateOptions{})
	istioClientCluster.NetworkingV1alpha3().VirtualServices(syncNS).
		Create(context.Background(), customVS, metaV1.CreateOptions{})
	istioClientClusterWithMultipleEnvs.NetworkingV1alpha3().VirtualServices(syncNS).
		Create(context.Background(), customVSWithMultipleEnvsStage, metaV1.CreateOptions{})
	istioClientClusterWithMultipleVSWithMultipleEnvs.NetworkingV1alpha3().VirtualServices(syncNS).
		Create(context.Background(), customVSWithMultipleEnvsQAL, metaV1.CreateOptions{})
	istioClientClusterWithMultipleVSWithMultipleEnvs.NetworkingV1alpha3().VirtualServices(syncNS).
		Create(context.Background(), customVSWithMultipleEnvsStage, metaV1.CreateOptions{})

	remoteControllerWithNoVS := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: istioClientClusterWithNoVS,
		},
	}
	remoteController := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: istioClientCluster,
		},
	}
	remoteControllerWithMultipleEnvs := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: istioClientClusterWithMultipleEnvs,
		},
	}
	remoteControllerWithMultipleVSMultipleEnvs := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: istioClientClusterWithMultipleVSWithMultipleEnvs,
		},
	}

	testCases := []struct {
		name             string
		remoteController *RemoteController
		env              string
		identity         string
		expectedVS       []envCustomVSTuple
		expectedError    error
	}{
		{
			name: "Given nil remote controller" +
				"When getCustomVirtualService func is called" +
				"Then the func should return an error",
			remoteController: nil,
			expectedError:    fmt.Errorf("remoteController is nil"),
		},
		{
			name: "Given empty env" +
				"When getCustomVirtualService func is called" +
				"Then the func should return an error",
			remoteController: &RemoteController{},
			expectedError:    fmt.Errorf("env is empty"),
		},
		{
			name: "Given empty identity" +
				"When getCustomVirtualService func is called" +
				"Then the func should return an error",
			remoteController: &RemoteController{},
			env:              "stage",
			expectedError:    fmt.Errorf("identity is empty"),
		},
		{
			name: "Given a cluster where there are no customVS" +
				"When getCustomVirtualService func is called" +
				"Then the func should return nil VS",
			remoteController: remoteControllerWithNoVS,
			env:              "stage",
			identity:         "testIdentity",
			expectedVS:       nil,
		},
		{
			name: "Given a cluster where there is customVS" +
				"When getCustomVirtualService func is called" +
				"Then the func should return the correct VS",
			remoteController: remoteController,
			env:              "stage",
			identity:         "testIdentity",
			expectedVS:       []envCustomVSTuple{{env: "stage", customVS: customVS}},
		},
		{
			name: "Given a cluster where there is customVS with multiple underscore separated envs" +
				"When getCustomVirtualService func is called" +
				"Then the func should return the correct VS",
			remoteController: remoteControllerWithMultipleEnvs,
			env:              "stage",
			identity:         "testIdentity",
			expectedVS: []envCustomVSTuple{
				{env: "stage", customVS: customVSWithMultipleEnvsStage},
				{env: "stage1", customVS: customVSWithMultipleEnvsStage},
				{env: "stage2", customVS: customVSWithMultipleEnvsStage},
			},
		},
		{
			name: "Given a cluster where there is customVS with multiple underscore separated envs" +
				"When getCustomVirtualService func is called" +
				"Then the func should return the correct VS",
			remoteController: remoteControllerWithMultipleEnvs,
			env:              "stage",
			identity:         "testIdentity",
			expectedVS: []envCustomVSTuple{
				{env: "stage", customVS: customVSWithMultipleEnvsStage},
				{env: "stage1", customVS: customVSWithMultipleEnvsStage},
				{env: "stage2", customVS: customVSWithMultipleEnvsStage},
			},
		},
		{
			name: "Given a cluster where there are multiple customVS with multiple underscore separated envs" +
				"When getCustomVirtualService func is called" +
				"Then the func should return the correct VS",
			remoteController: remoteControllerWithMultipleVSMultipleEnvs,
			env:              "qal",
			identity:         "testIdentity",
			expectedVS: []envCustomVSTuple{
				{env: "qal", customVS: customVSWithMultipleEnvsQAL},
				{env: "qal1", customVS: customVSWithMultipleEnvsQAL},
				{env: "qal2", customVS: customVSWithMultipleEnvsQAL},
			},
		},
	}

	ctxLogger := log.WithFields(log.Fields{
		"type": "VirtualService",
	})

	ap := common.AdmiralParams{
		SyncNamespace:      syncNS,
		ProcessVSCreatedBy: "testCreatedBy",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := getCustomVirtualService(
				context.Background(), ctxLogger, tc.remoteController, tc.env, tc.identity)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedVS, actual)
			}
		})
	}

}

func TestGetHostsDiff(t *testing.T) {

	testCases := []struct {
		name           string
		inclusterHosts []string
		customVSHosts  []string
		expectedHosts  map[string]bool
	}{
		{
			name: "Given empty customVSHosts and inclusterHosts" +
				"When getHostsDiff func is called" +
				"Then the func should return empty map",
			inclusterHosts: []string{},
			customVSHosts:  []string{},
			expectedHosts:  map[string]bool{},
		},
		{
			name: "Given empty customVSHosts" +
				"When getHostsDiff func is called" +
				"Then the func should return all hosts from inclusterVSHosts",
			inclusterHosts: []string{
				"canary.qal.stage.global",
				"east.qal.stage.global",
				"qal.stage.global",
			},
			customVSHosts: []string{},
			expectedHosts: map[string]bool{
				"canary.qal.stage.global": true,
				"east.qal.stage.global":   true,
				"qal.stage.global":        true,
			},
		},
		{
			name: "Given empty inclusterVSHosts" +
				"When getHostsDiff func is called" +
				"Then the func should return an empty map",
			inclusterHosts: []string{},
			customVSHosts: []string{
				"canary.qal.stage.global",
				"east.qal.stage.global",
				"qal.stage.global",
			},
			expectedHosts: map[string]bool{},
		},
		{
			name: "Given inclusterVSHosts and customVSHosts" +
				"And they have all common hosts" +
				"When getHostsDiff func is called" +
				"Then the func should return an empty map",
			inclusterHosts: []string{
				"canary.qal.stage.global",
				"east.qal.stage.global",
				"qal.stage.global",
			},
			customVSHosts: []string{
				"canary.qal.stage.global",
				"east.qal.stage.global",
				"qal.stage.global",
			},
			expectedHosts: map[string]bool{},
		},
		{
			name: "Given inclusterVSHosts and customVSHosts" +
				"When getHostsDiff func is called" +
				"Then the func should return valid diffs",
			inclusterHosts: []string{
				"canary.qal.stage.global",
				"east.qal.stage.global",
				"qal.stage.global",
			},
			customVSHosts: []string{
				"qal.stage.global",
			},
			expectedHosts: map[string]bool{
				"canary.qal.stage.global": true,
				"east.qal.stage.global":   true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := getHostsDiff(tc.inclusterHosts, tc.customVSHosts)
			assert.Equal(t, tc.expectedHosts, actual)
		})
	}

}

func TestIsSEMultiRegion(t *testing.T) {

	// write table test cases for IsSEMultiRegion function
	testCases := []struct {
		name           string
		serviceEntry   *networkingV1Alpha3.ServiceEntry
		expectedResult bool
	}{
		{
			name: "Given nil ServiceEntry" +
				"When IsSEMultiRegion is called" +
				"Then it should return false",
			serviceEntry:   nil,
			expectedResult: false,
		},
		{
			name: "Given ServiceEntry with no endpoints" +
				"When IsSEMultiRegion is called" +
				"Then it should return false",
			serviceEntry: &networkingV1Alpha3.ServiceEntry{
				Endpoints: nil,
			},
			expectedResult: false,
		},
		{
			name: "Given ServiceEntry with endpoints in a single region" +
				"When IsSEMultiRegion is called" +
				"Then it should return false",
			serviceEntry: &networkingV1Alpha3.ServiceEntry{
				Endpoints: []*networkingV1Alpha3.WorkloadEntry{
					{
						Locality: "us-west-2",
					},
					{
						Locality: "us-west-2",
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "Given ServiceEntry with endpoints in multiple regions" +
				"When IsSEMultiRegion is called" +
				"Then it should return true",
			serviceEntry: &networkingV1Alpha3.ServiceEntry{
				Endpoints: []*networkingV1Alpha3.WorkloadEntry{
					{
						Locality: "us-west-2",
					},
					{
						Locality: "us-east-2",
					},
				},
			},
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualResult := isSEMultiRegion(tc.serviceEntry)
			assert.Equal(t, tc.expectedResult, actualResult)
		})
	}

}

func TestPerformDRPinning(t *testing.T) {

	cachedAdditionalEndpointDR := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "west.foo.test-ns.global-dr",
			Namespace: "sync-ns",
		},
		Spec: networkingV1Alpha3.DestinationRule{
			Host:     "west.foo.test-ns.global",
			ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
						Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
							{
								From: "*",
								To:   map[string]uint32{"us-west-2": 100},
							},
						},
					},
				},
			},
		},
	}

	expectedAdditionalEndpointDR := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "west.foo.test-ns.global-dr",
			Namespace: "sync-ns",
		},
		Spec: networkingV1Alpha3.DestinationRule{
			Host:     "west.foo.test-ns.global",
			ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
						Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
							{
								From: "*",
								To:   map[string]uint32{"us-east-2": 100},
							},
						},
					},
				},
			},
		},
	}

	cachedDefaultDR := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "foo.test-ns.global-default-dr",
			Namespace: "sync-ns",
		},
		Spec: networkingV1Alpha3.DestinationRule{
			Host:     "foo.test-ns.global",
			ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
						Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
							{
								From: "*",
								To:   map[string]uint32{"us-west-2": 100},
							},
						},
					},
				},
			},
		},
	}

	expectedDefaultDR := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "foo.test-ns.global-default-dr",
			Namespace: "sync-ns",
		},
		Spec: networkingV1Alpha3.DestinationRule{
			Host:     "foo.test-ns.global",
			ExportTo: []string{"test-dependent-ns0", "test-dependent-ns1", "test-ns"},
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
						Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
							{
								From: "*",
								To:   map[string]uint32{"us-east-2": 100},
							},
						},
					},
				},
			},
		},
	}

	drCache := istio.NewDestinationRuleCache()
	drCache.Put(cachedDefaultDR)
	drCache.Put(cachedAdditionalEndpointDR)

	istioClient := istioFake.NewSimpleClientset()
	istioClient.NetworkingV1alpha3().DestinationRules("sync-ns").
		Create(context.Background(), cachedDefaultDR, metaV1.CreateOptions{})
	istioClient.NetworkingV1alpha3().DestinationRules("sync-ns").
		Create(context.Background(), cachedAdditionalEndpointDR, metaV1.CreateOptions{})

	ap := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey: "identity",
			EnvKey:              "env",
		},
		HostnameSuffix: "global",
		SyncNamespace:  "sync-ns",
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	testCases := []struct {
		name                  string
		remoteRegistry        *RemoteRegistry
		remoteController      *RemoteController
		vs                    *apiNetworkingV1Alpha3.VirtualService
		env                   string
		sourceIdentity        string
		sourceCluster         string
		drName                string
		expectedError         error
		expectDestinationRule *apiNetworkingV1Alpha3.DestinationRule
	}{
		{
			name: "Given nil remoteRegistry" +
				"When performDRPinning func is called" +
				"Then the func should return an error",
			remoteRegistry: nil,
			expectedError:  fmt.Errorf("remoteRegistry is nil"),
		},
		{
			name: "Given nil remoteController" +
				"When performDRPinning func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{},
			expectedError:  fmt.Errorf("remoteController is nil"),
		},
		{
			name: "Given nil vs" +
				"When performDRPinning func is called" +
				"Then the func should return an error",
			remoteRegistry:   &RemoteRegistry{},
			remoteController: &RemoteController{},
			expectedError:    fmt.Errorf("virtualService is nil"),
		},
		{
			name: "Given locality is missing" +
				"When performDRPinning func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{},
			remoteController: &RemoteController{
				ClusterID: "cluster1",
			},
			vs:            &apiNetworkingV1Alpha3.VirtualService{},
			sourceCluster: "cluster1",
			expectedError: fmt.Errorf("getClusterRegion failed due to failed to get region of cluster cluster1"),
		},
		{
			name: "Given for a VS host there is no DR in cache" +
				"When performDRPinning func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{},
			remoteController: &RemoteController{
				NodeController: &admiral.NodeController{
					Locality: &admiral.Locality{
						Region: "us-west-2",
					},
				},
				DestinationRuleController: &istio.DestinationRuleController{
					IstioClient: istioFake.NewSimpleClientset(),
					Cache:       istio.NewDestinationRuleCache(),
				},
			},
			vs: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{"foo.test-ns.global"},
				},
			},
			sourceCluster: "cluster1",
			expectedError: fmt.Errorf(
				"skipped pinning DR to remote region as no cached DR found with drName foo.test-ns.global-default-dr in cluster cluster1"),
		},
		{
			name: "Given for a VS host there is DR in cache with same region" +
				"When performDRPinning func is called" +
				"Then the func should not return an error but DR will not change as reconcile is not needed",
			remoteRegistry: &RemoteRegistry{},
			remoteController: &RemoteController{
				NodeController: &admiral.NodeController{
					Locality: &admiral.Locality{
						Region: "us-east-2",
					},
				},
				DestinationRuleController: &istio.DestinationRuleController{
					IstioClient: istioClient,
					Cache:       drCache,
				},
			},
			vs: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{"foo.test-ns.global"},
				},
			},
			drName:                "foo.test-ns.global-default-dr",
			sourceCluster:         "cluster1",
			sourceIdentity:        "foo.test-ns.global-default-dr",
			env:                   "foo",
			expectDestinationRule: cachedDefaultDR,
		},
		{
			name: "Given for a VS host there is DR in cache and region needs to be flipped" +
				"When performDRPinning func is called" +
				"Then the func should not return an error and DR will be updated with new region",
			remoteRegistry: &RemoteRegistry{},
			remoteController: &RemoteController{
				NodeController: &admiral.NodeController{
					Locality: &admiral.Locality{
						Region: "us-west-2",
					},
				},
				DestinationRuleController: &istio.DestinationRuleController{
					IstioClient: istioClient,
					Cache:       drCache,
				},
			},
			vs: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{"foo.test-ns.global"},
				},
			},
			drName:                "foo.test-ns.global-default-dr",
			sourceCluster:         "cluster1",
			sourceIdentity:        "foo.test-ns.global-default-dr",
			env:                   "foo",
			expectDestinationRule: expectedDefaultDR,
		},
		{
			name: "Given for a VS with additonal endpoint host with DR in cache and region needs to be flipped" +
				"When performDRPinning func is called" +
				"Then the func should not return an error and DR will be updated with new region",
			remoteRegistry: &RemoteRegistry{},
			remoteController: &RemoteController{
				NodeController: &admiral.NodeController{
					Locality: &admiral.Locality{
						Region: "us-west-2",
					},
				},
				DestinationRuleController: &istio.DestinationRuleController{
					IstioClient: istioClient,
					Cache:       drCache,
				},
			},
			vs: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{"west.foo.test-ns.global"},
				},
			},
			drName:                "west.foo.test-ns.global-dr",
			sourceCluster:         "cluster1",
			sourceIdentity:        "west.foo.test-ns.global-dr",
			env:                   "foo",
			expectDestinationRule: expectedAdditionalEndpointDR,
		},
	}

	ctxLogger := log.WithFields(log.Fields{
		"type": "VirtualService",
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := performDRPinning(
				context.Background(), ctxLogger, tc.remoteRegistry,
				tc.remoteController, tc.vs, tc.env, tc.sourceCluster)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				actualDr, err := tc.remoteController.DestinationRuleController.IstioClient.NetworkingV1alpha3().
					DestinationRules(common.GetSyncNamespace()).Get(context.Background(), tc.drName, metaV1.GetOptions{})
				assert.Nil(t, err)
				assert.True(t,
					reflect.DeepEqual(
						tc.expectDestinationRule.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute,
						actualDr.Spec.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute))
			}
		})
	}

}

func TestGetLocalityLBSettings(t *testing.T) {

	testCases := []struct {
		name           string
		locality       string
		expectedResult *networkingV1Alpha3.LocalityLoadBalancerSetting
		expectedError  error
	}{
		{
			name: "Given empty locality" +
				"When getLocalityLBSettings func is called" +
				"Then the func should return an error",
			locality:      "",
			expectedError: fmt.Errorf("currentLocality is empty"),
		},
		{
			name: "Given current locality is west" +
				"When getLocalityLBSettings func is called" +
				"Then the func should pin to east region",
			locality: "us-west-2",
			expectedResult: &networkingV1Alpha3.LocalityLoadBalancerSetting{
				Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
					{
						From: "*",
						To:   map[string]uint32{"us-east-2": 100},
					},
				},
			},
		},
		{
			name: "Given current locality is east" +
				"When getLocalityLBSettings func is called" +
				"Then the func should pin to west region",
			locality: "us-east-2",
			expectedResult: &networkingV1Alpha3.LocalityLoadBalancerSetting{
				Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
					{
						From: "*",
						To:   map[string]uint32{"us-west-2": 100},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualResult, err := getLocalityLBSettings(tc.locality)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.True(t, reflect.DeepEqual(tc.expectedResult, actualResult))
			}
		})
	}
}

func TestDoDRUpdateForInClusterVSRouting(t *testing.T) {

	identityClusterCache := common.NewMapOfMaps()
	identityClusterCache.Put("identity0", "cluster1", "cluster1")
	identityClusterCache.Put("identity1", "cluster1", "cluster1")

	identityVirtualServiceCache := istio.NewIdentityVirtualServiceCache()
	identityVirtualServiceCache.Put(&apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "vs1",
		},
		Spec: networkingV1Alpha3.VirtualService{Hosts: []string{"stage.identity0.global"}},
	})

	vsWithValidNS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:   "foo.testns.global-incluster-vs",
			Labels: map[string]string{common.VSRoutingLabel: "true"},
		},
		Spec: networkingV1Alpha3.VirtualService{
			ExportTo: []string{"testNS"},
		},
	}
	vsWithSyncNS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:   "bar.testns.global-incluster-vs",
			Labels: map[string]string{common.VSRoutingLabel: "true"},
		},
		Spec: networkingV1Alpha3.VirtualService{
			ExportTo: []string{"sync-ns"},
		},
	}

	virtualServiceCache := istio.NewVirtualServiceCache()
	virtualServiceCache.Put(vsWithValidNS)
	virtualServiceCache.Put(vsWithSyncNS)

	remoteController := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IdentityVirtualServiceCache: identityVirtualServiceCache,
			VirtualServiceCache:         virtualServiceCache,
			IstioClient:                 istioFake.NewSimpleClientset(),
		},
	}

	remoteRegistry := NewRemoteRegistry(context.Background(), common.AdmiralParams{})
	remoteRegistry.AdmiralCache.IdentityClusterCache = identityClusterCache
	remoteRegistry.PutRemoteController("cluster1", remoteController)

	testCases := []struct {
		name                               string
		cluster                            string
		identity                           string
		env                                string
		isSourceCluster                    bool
		enableVSRoutingInCluster           bool
		remoteRegistry                     *RemoteRegistry
		serviceEntry                       *networkingV1Alpha3.ServiceEntry
		enabledVSRoutingInClusterResources map[string]string
		expected                           bool
	}{
		{
			name: "Given nil remoteRegistry" +
				"When DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return false",
		},
		{
			name: "Given VSRoutingInCluster is enabled for cluster1 and identity1, cluster is source cluster" +
				"When corresponding incluster VS's exportTo has not been updated with valid namespaces" +
				"And DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return false",
			cluster:                            "cluster1",
			identity:                           "identity1",
			env:                                "stage",
			isSourceCluster:                    true,
			enableVSRoutingInCluster:           true,
			enabledVSRoutingInClusterResources: map[string]string{"cluster1": "identity1"},
			remoteRegistry:                     remoteRegistry,
			serviceEntry: &networkingV1Alpha3.ServiceEntry{
				Hosts: []string{"bar.testns.global"},
			},
			expected: false,
		},
		{
			name: "Given VSRoutingInCluster is enabled for cluster1 and identity1, cluster is source cluster" +
				"When DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return true",
			cluster:                            "cluster1",
			identity:                           "identity1",
			env:                                "stage",
			isSourceCluster:                    true,
			enableVSRoutingInCluster:           true,
			enabledVSRoutingInClusterResources: map[string]string{"cluster1": "identity1"},
			remoteRegistry:                     remoteRegistry,
			serviceEntry: &networkingV1Alpha3.ServiceEntry{
				Hosts: []string{"foo.testns.global"},
			},
			expected: true,
		},
		{
			name: "Given VSRoutingInCluster is enabled for cluster1 and identity1, cluster is remote cluster" +
				"When DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return true",
			cluster:                            "cluster1",
			identity:                           "identity1",
			env:                                "stage",
			isSourceCluster:                    false,
			enableVSRoutingInCluster:           true,
			enabledVSRoutingInClusterResources: map[string]string{"cluster1": "identity1"},
			remoteRegistry:                     remoteRegistry,
			serviceEntry: &networkingV1Alpha3.ServiceEntry{
				Hosts: []string{"foo.testns.global"},
			},
			expected: false,
		},
		{
			name: "Given VSRoutingInCluster is not enabled for cluster1, cluster is source cluster" +
				"When DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return true",
			cluster:                            "cluster1",
			identity:                           "identity1",
			env:                                "stage",
			isSourceCluster:                    true,
			enableVSRoutingInCluster:           true,
			enabledVSRoutingInClusterResources: map[string]string{"cluster2": ""},
			remoteRegistry:                     remoteRegistry,
			serviceEntry: &networkingV1Alpha3.ServiceEntry{
				Hosts: []string{"foo.testns.global"},
			},
			expected: false,
		},
		{
			name: "Given VSRoutingInCluster is not enabled, cluster is source cluster" +
				"When DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return true",
			cluster:                            "cluster1",
			identity:                           "identity1",
			env:                                "stage",
			isSourceCluster:                    true,
			enableVSRoutingInCluster:           false,
			enabledVSRoutingInClusterResources: map[string]string{"": ""},
			remoteRegistry:                     remoteRegistry,
			serviceEntry: &networkingV1Alpha3.ServiceEntry{
				Hosts: []string{"foo.testns.global"},
			},
			expected: false,
		},
		{
			name: "Given VSRoutingInCluster is enabled for cluster1,  VSRoutingInCluster not enabled for identity1, cluster is source cluster" +
				"When DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return true",
			cluster:                            "cluster1",
			identity:                           "identity1",
			env:                                "stage",
			isSourceCluster:                    true,
			enableVSRoutingInCluster:           true,
			enabledVSRoutingInClusterResources: map[string]string{"cluster1": ""},
			remoteRegistry:                     remoteRegistry,
			serviceEntry: &networkingV1Alpha3.ServiceEntry{
				Hosts: []string{"foo.testns.global"},
			},
			expected: false,
		},
	}

	ctxLogger := log.WithFields(log.Fields{
		"type": "VirtualService",
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := common.AdmiralParams{
				SyncNamespace: "sync-ns",
			}
			p.EnableVSRoutingInCluster = tc.enableVSRoutingInCluster
			p.VSRoutingInClusterEnabledResources = tc.enabledVSRoutingInClusterResources
			common.ResetSync()
			common.InitializeConfig(p)

			assert.Equal(t, tc.expected, DoDRUpdateForInClusterVSRouting(context.Background(),
				ctxLogger, tc.env, tc.cluster, tc.identity, tc.isSourceCluster, tc.remoteRegistry, tc.serviceEntry, true))
		})
	}

}

func TestIsVSRoutingInClusterDisabledForClusterAndIdentity(t *testing.T) {

	testCases := []struct {
		name                                string
		cluster                             string
		identity                            string
		disabledVSRoutingInClusterResources map[string]string
		expected                            bool
	}{
		{
			name: "Given disabledVSRoutingInClusterResources is empty" +
				"When IsVSRoutingInClusterDisabledForIdentity is called" +
				"Then it should return false",
			cluster:                             "cluster1",
			identity:                            "identity",
			disabledVSRoutingInClusterResources: map[string]string{},
			expected:                            false,
		},
		{
			name: "Given cluster doesn't exists in the disabledVSRoutingInClusterResources" +
				"When IsVSRoutingInClusterDisabledForIdentity is called" +
				"Then it should return false",
			cluster:                             "cluster2",
			identity:                            "identity",
			disabledVSRoutingInClusterResources: map[string]string{},
			expected:                            false,
		},
		{
			name: "Given cluster does exists in the disabledVSRoutingInClusterResources" +
				"When IsVSRoutingInClusterDisabledForIdentity is called" +
				"Then it should return true",
			cluster:                             "cluster1",
			identity:                            "identity",
			disabledVSRoutingInClusterResources: map[string]string{"cluster1": "*"},
			expected:                            true,
		},
		{
			name: "Given VS routing is disabled in all clusters using '*'" +
				"When IsVSRoutingInClusterDisabledForIdentity is called" +
				"Then it should return true",
			cluster:                             "cluster1",
			identity:                            "identity",
			disabledVSRoutingInClusterResources: map[string]string{"*": "*"},
			expected:                            true,
		},
		{
			name: "Given cluster does exists in the disabledVSRoutingInClusterResources and given identity does not exists" +
				"When IsVSRoutingInClusterDisabledForIdentity is called" +
				"Then it should return false",
			cluster:                             "cluster1",
			identity:                            "identity1",
			disabledVSRoutingInClusterResources: map[string]string{"cluster1": ""},
			expected:                            false,
		},
		{
			name: "Given cluster and identity does exists in the disabledVSRoutingInClusterResources" +
				"When IsVSRoutingInClusterDisabledForIdentity is called" +
				"Then it should return true",
			cluster:                             "cluster1",
			identity:                            "identity1",
			disabledVSRoutingInClusterResources: map[string]string{"cluster1": "identity1"},
			expected:                            true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := common.AdmiralParams{}
			p.VSRoutingInClusterDisabledResources = tc.disabledVSRoutingInClusterResources
			common.ResetSync()
			common.InitializeConfig(p)

			assert.Equal(t, tc.expected, IsVSRoutingInClusterDisabledForIdentity(tc.cluster, tc.identity))
		})
	}
}

func TestDoVSRoutingInClusterForClusterAndIdentity(t *testing.T) {

	customVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "custom-vs",
			Namespace: "sync-ns",
			Annotations: map[string]string{
				common.CreatedForEnv: "stage",
			},
			Labels: map[string]string{
				common.CreatedBy:  "testCreatedBy",
				common.CreatedFor: "identity1",
			},
		},
		Spec: networkingV1Alpha3.VirtualService{
			ExportTo: []string{"*"},
		},
	}

	istioClientCluster := istioFake.NewSimpleClientset()
	istioClientCluster.NetworkingV1alpha3().VirtualServices("sync-ns").
		Create(context.Background(), customVS, metaV1.CreateOptions{})

	identityClusterCache := common.NewMapOfMaps()
	identityClusterCache.Put("identity0", "cluster0", "cluster0")
	identityClusterCache.Put("identity1", "cluster1", "cluster1")

	identityVirtualServiceCache := istio.NewIdentityVirtualServiceCache()
	identityVirtualServiceCache.Put(&apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "vs1",
			Namespace: "ns1",
		},
		Spec: networkingV1Alpha3.VirtualService{Hosts: []string{"stage.identity0.global"}},
	})

	remoteController := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IdentityVirtualServiceCache: identityVirtualServiceCache,
			IstioClient:                 istioFake.NewSimpleClientset(),
		},
	}
	remoteControllerWithCustomVS := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IdentityVirtualServiceCache: identityVirtualServiceCache,
			IstioClient:                 istioClientCluster,
		},
	}

	p := common.AdmiralParams{
		SyncNamespace:      "sync-ns",
		ProcessVSCreatedBy: "testCreatedBy",
	}

	remoteRegistry := NewRemoteRegistry(context.Background(), p)
	remoteRegistry.AdmiralCache.IdentityClusterCache = identityClusterCache
	remoteRegistry.PutRemoteController("cluster1", remoteController)
	remoteRegistry.PutRemoteController("cluster0", remoteController)
	remoteRegistry.PutRemoteController("cluster9", remoteControllerWithCustomVS)

	testCases := []struct {
		name                                  string
		cluster                               string
		identity                              string
		env                                   string
		enableVSRoutingInCluster              bool
		enabledVSRoutingInClusterForResources map[string]string
		vsRoutingInClusterDisabledResources   map[string]string
		remoteRegistry                        *RemoteRegistry
		expected                              bool
		performCartographerVSCheck            bool
	}{
		{
			name: "Given enableVSRoutingInCluster is false, enabledVSRoutingInClusterForResources is empty" +
				"When DoVSRoutingInClusterForClusterAndIdentity is called" +
				"Then it should return false",
			cluster:                               "cluster1",
			env:                                   "stage",
			enableVSRoutingInCluster:              false,
			enabledVSRoutingInClusterForResources: map[string]string{},
			remoteRegistry:                        remoteRegistry,
			expected:                              false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, enabledVSRoutingInClusterForResources is empty" +
				"When DoVSRoutingInClusterForClusterAndIdentity is called" +
				"Then it should return false",
			cluster:                               "cluster1",
			identity:                              "identity1",
			env:                                   "stage",
			enableVSRoutingInCluster:              true,
			enabledVSRoutingInClusterForResources: map[string]string{},
			remoteRegistry:                        remoteRegistry,
			expected:                              false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and given cluster doesn't exists in the enabledVSRoutingInClusterForResources" +
				"When DoVSRoutingInClusterForClusterAndIdentity is called" +
				"Then it should return false",
			cluster:                               "cluster2",
			identity:                              "identity1",
			env:                                   "stage",
			enableVSRoutingInCluster:              true,
			enabledVSRoutingInClusterForResources: map[string]string{"cluster1": "test1"},
			remoteRegistry:                        remoteRegistry,
			expected:                              false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and there is a cartographer VS with valid(non-dot) NS" +
				"When DoVSRoutingInClusterForClusterAndIdentity is called" +
				"Then it should return false",
			cluster:                               "cluster9",
			identity:                              "identity1",
			env:                                   "stage",
			enableVSRoutingInCluster:              true,
			enabledVSRoutingInClusterForResources: map[string]string{"cluster9": "*"},
			remoteRegistry:                        remoteRegistry,
			expected:                              false,
			performCartographerVSCheck:            true,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and given cluster does exists in the map" +
				"When DoVSRoutingInClusterForClusterAndIdentity is called" +
				"Then it should return true",
			cluster:                               "cluster1",
			enableVSRoutingInCluster:              true,
			env:                                   "stage",
			identity:                              "identity1",
			enabledVSRoutingInClusterForResources: map[string]string{"cluster1": "*"},
			remoteRegistry:                        remoteRegistry,
			expected:                              true,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and all VS routing is enabled in all clusters using '*'" +
				"When DoVSRoutingInClusterForClusterAndIdentity is called" +
				"Then it should return true",
			cluster:                               "cluster1",
			enableVSRoutingInCluster:              true,
			env:                                   "stage",
			identity:                              "identity1",
			enabledVSRoutingInClusterForResources: map[string]string{"*": "*"},
			remoteRegistry:                        remoteRegistry,
			expected:                              true,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and given cluster and identity does exists in the map" +
				"When DoVSRoutingInClusterForClusterAndIdentity is called" +
				"Then it should return true",
			cluster:                               "cluster1",
			identity:                              "identity1",
			enableVSRoutingInCluster:              true,
			env:                                   "stage",
			enabledVSRoutingInClusterForResources: map[string]string{"cluster1": "identity1, identity2"},
			remoteRegistry:                        remoteRegistry,
			expected:                              true,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and given cluster does exists in the map, and given identity does not exists in the map" +
				"When DoVSRoutingInClusterForClusterAndIdentity is called" +
				"Then it should return false",
			cluster:                               "cluster1",
			identity:                              "identity3",
			enableVSRoutingInCluster:              true,
			env:                                   "stage",
			enabledVSRoutingInClusterForResources: map[string]string{"cluster1": "identity1, identity2"},
			remoteRegistry:                        remoteRegistry,
			expected:                              false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and given cluster does exists in the map, and given identity has a VS in its namespace" +
				"When DoVSRoutingInClusterForClusterAndIdentity is called" +
				"Then it should return false",
			cluster:                               "cluster0",
			identity:                              "identity0",
			enableVSRoutingInCluster:              true,
			env:                                   "stage",
			enabledVSRoutingInClusterForResources: map[string]string{"cluster0": "*"},
			remoteRegistry:                        remoteRegistry,
			expected:                              false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, but is disabled for all identities on the give cluster" +
				"When DoVSRoutingInClusterForClusterAndIdentity is called" +
				"Then it should return false",
			cluster:                             "cluster0",
			identity:                            "identity0",
			enableVSRoutingInCluster:            true,
			env:                                 "stage",
			vsRoutingInClusterDisabledResources: map[string]string{"cluster0": "*"},
			remoteRegistry:                      remoteRegistry,
			expected:                            false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, but is disabled for an identity on the given cluster" +
				"When DoVSRoutingInClusterForClusterAndIdentity is called" +
				"Then it should return false",
			cluster:                             "cluster0",
			identity:                            "identity0",
			enableVSRoutingInCluster:            true,
			env:                                 "stage",
			vsRoutingInClusterDisabledResources: map[string]string{"cluster0": "identity0"},
			remoteRegistry:                      remoteRegistry,
			expected:                            false,
		},
	}

	ctxLogger := log.WithFields(log.Fields{
		"type": "VirtualService",
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.EnableVSRoutingInCluster = tc.enableVSRoutingInCluster
			p.VSRoutingInClusterEnabledResources = tc.enabledVSRoutingInClusterForResources
			common.ResetSync()
			common.InitializeConfig(p)

			assert.Equal(t, tc.expected, DoVSRoutingInClusterForClusterAndIdentity(
				context.Background(), ctxLogger, tc.env, tc.cluster, tc.identity, tc.remoteRegistry, tc.performCartographerVSCheck))
		})
	}

}

func TestDoActivePassiveInClusterVS(t *testing.T) {

	passiveDR := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "stage.foo.passive.global-default-dr",
			Namespace: "sync-ns",
		},
		Spec: networkingV1Alpha3.DestinationRule{
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
						Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
							{
								From: "*",
								To:   map[string]uint32{"us-east-2": 100},
							},
						},
					},
				},
			},
		},
	}
	activeDR := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "stage.foo.active.global-default-dr",
			Namespace: "sync-ns",
		},
		Spec: networkingV1Alpha3.DestinationRule{
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
						Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
							{
								From: "*",
								To:   map[string]uint32{"us-west-2": 100},
							},
						},
					},
				},
			},
		},
	}
	drWithNoLBDistribution := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "stage.foo.nolb.global-default-dr",
			Namespace: "sync-ns",
		},
		Spec: networkingV1Alpha3.DestinationRule{
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{},
				},
			},
		},
	}
	splitDR := &apiNetworkingV1Alpha3.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "stage.foo.split.global-default-dr",
			Namespace: "sync-ns",
		},
		Spec: networkingV1Alpha3.DestinationRule{
			TrafficPolicy: &networkingV1Alpha3.TrafficPolicy{
				LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
					LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
						Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{
							{
								From: "*",
								To:   map[string]uint32{"us-west-2": 50},
							},
							{
								From: "*",
								To:   map[string]uint32{"us-east-2": 50},
							},
						},
					},
				},
			},
		},
	}
	drCache := istio.NewDestinationRuleCache()
	drCache.Put(passiveDR)
	drCache.Put(activeDR)
	drCache.Put(drWithNoLBDistribution)
	drCache.Put(splitDR)

	singleRegionSE := &apiNetworkingV1Alpha3.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "stage.foo.global-se",
		},
		Spec: networkingV1Alpha3.ServiceEntry{
			Endpoints: []*networkingV1Alpha3.WorkloadEntry{
				{
					Locality: "us-east-2",
				},
			},
		},
	}
	multiRegionSE := &apiNetworkingV1Alpha3.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "stage.foo.multi.global-se",
		},
		Spec: networkingV1Alpha3.ServiceEntry{
			Endpoints: []*networkingV1Alpha3.WorkloadEntry{
				{
					Locality: "us-east-2",
				},
				{
					Locality: "us-west-2",
				},
			},
		},
	}
	activeRegionSE := &apiNetworkingV1Alpha3.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "stage.foo.active.global-se",
		},
		Spec: networkingV1Alpha3.ServiceEntry{
			Endpoints: []*networkingV1Alpha3.WorkloadEntry{
				{
					Locality: "us-east-2",
				},
				{
					Locality: "us-west-2",
				},
			},
		},
	}
	passiveRegionSE := &apiNetworkingV1Alpha3.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "stage.foo.passive.global-se",
		},
		Spec: networkingV1Alpha3.ServiceEntry{
			Endpoints: []*networkingV1Alpha3.WorkloadEntry{
				{
					Locality: "us-east-2",
				},
				{
					Locality: "us-west-2",
				},
			},
		},
	}
	splitRegionSE := &apiNetworkingV1Alpha3.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "stage.foo.split.global-se",
		},
		Spec: networkingV1Alpha3.ServiceEntry{
			Endpoints: []*networkingV1Alpha3.WorkloadEntry{
				{
					Locality: "us-east-2",
				},
				{
					Locality: "us-west-2",
				},
			},
		},
	}
	barRegionSE := &apiNetworkingV1Alpha3.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "stage.bar.global-se",
		},
		Spec: networkingV1Alpha3.ServiceEntry{
			Endpoints: []*networkingV1Alpha3.WorkloadEntry{
				{
					Locality: "us-east-2",
				},
				{
					Locality: "us-west-2",
				},
			},
		},
	}
	noLBRegionSE := &apiNetworkingV1Alpha3.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "stage.foo.nolb.global-se",
		},
		Spec: networkingV1Alpha3.ServiceEntry{
			Endpoints: []*networkingV1Alpha3.WorkloadEntry{
				{
					Locality: "us-east-2",
				},
				{
					Locality: "us-west-2",
				},
			},
		},
	}
	seCache := istio.NewServiceEntryCache()
	seCache.Put(singleRegionSE, cluster1)
	seCache.Put(multiRegionSE, cluster1)
	seCache.Put(activeRegionSE, cluster1)
	seCache.Put(passiveRegionSE, cluster1)
	seCache.Put(splitRegionSE, cluster1)
	seCache.Put(barRegionSE, cluster1)
	seCache.Put(noLBRegionSE, cluster1)

	testCases := []struct {
		name                  string
		remoteRegistry        *RemoteRegistry
		cname                 string
		sourceCluster         string
		sourceClusterLocality string
		expectedGTP           *v1alpha12.GlobalTrafficPolicy
		expectedError         error
	}{
		{
			name: "Given a nil remoteRegistry" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: nil,
			expectedError:  fmt.Errorf("remoteRegistry is nil"),
		},
		{
			name: "Given a nil remoteController" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{},
			},
			sourceCluster: "cluster1",
			expectedError: fmt.Errorf("remotecontroller is nil for cluster cluster1"),
		},
		{
			name: "Given a nil destinationRuleController" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{
					"cluster1": {},
				},
			},
			sourceCluster: "cluster1",
			expectedError: fmt.Errorf("destinationRuleController is nil for cluster cluster1"),
		},
		{
			name: "Given a nil destinationRuleController.Cache" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{
					"cluster1": {
						DestinationRuleController: &istio.DestinationRuleController{},
					},
				},
			},
			sourceCluster: "cluster1",
			expectedError: fmt.Errorf("destinationRuleController.Cache is nil for cluster cluster1"),
		},
		{
			name: "Given a nil serviceEntryController" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{
					"cluster1": {
						DestinationRuleController: &istio.DestinationRuleController{
							Cache: istio.NewDestinationRuleCache(),
						},
					},
				},
			},
			sourceCluster: "cluster1",
			expectedError: fmt.Errorf("serviceEntryController is nil for cluster cluster1"),
		},
		{
			name: "Given a nil serviceEntryController.Cache" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{
					"cluster1": {
						DestinationRuleController: &istio.DestinationRuleController{
							Cache: istio.NewDestinationRuleCache(),
						},
						ServiceEntryController: &istio.ServiceEntryController{},
					},
				},
			},
			sourceCluster: "cluster1",
			expectedError: fmt.Errorf("serviceEntryController.Cache is nil for cluster cluster1"),
		},
		{
			name: "Given a SE missing in serviceEntryController.Cache" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{
					"cluster1": {
						DestinationRuleController: &istio.DestinationRuleController{
							Cache: istio.NewDestinationRuleCache(),
						},
						ServiceEntryController: &istio.ServiceEntryController{
							Cache: istio.NewServiceEntryCache(),
						},
					},
				},
			},
			sourceCluster: "cluster1",
			cname:         "stage.foo.global",
			expectedError: fmt.Errorf("no se found in cache for seName stage.foo.global-se"),
		},
		{
			name: "Given a SE in cache is not multiRegion" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{
					"cluster1": {
						DestinationRuleController: &istio.DestinationRuleController{
							Cache: istio.NewDestinationRuleCache(),
						},
						ServiceEntryController: &istio.ServiceEntryController{
							Cache: seCache,
						},
					},
				},
			},
			sourceCluster: "cluster1",
			cname:         "stage.foo.global",
			expectedError: fmt.Errorf(
				"skipped active passive for incluster as the SE is not multi-region stage.foo.global-se"),
		},
		{
			name: "Given there is no DR in the cache" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{
					"cluster1": {
						DestinationRuleController: &istio.DestinationRuleController{
							Cache: istio.NewDestinationRuleCache(),
						},
						ServiceEntryController: &istio.ServiceEntryController{
							Cache: seCache,
						},
					},
				},
			},
			sourceCluster: "cluster1",
			cname:         "stage.bar.global",
			expectedError: fmt.Errorf("no dr found in cache for drName stage.bar.global-default-dr"),
		},
		{
			name: "Given a DR with no load distribution" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{
					"cluster1": {
						DestinationRuleController: &istio.DestinationRuleController{
							Cache: drCache,
						},
						ServiceEntryController: &istio.ServiceEntryController{
							Cache: seCache,
						},
					},
				},
			},
			sourceCluster: "cluster1",
			cname:         "stage.foo.nolb.global",
			expectedError: fmt.Errorf(
				"skipped active passive for incluster as the DR has no localityLBSetting stage.foo.nolb.global-default-dr"),
		},
		{
			name: "Given a DR with traffic split" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{
					"cluster1": {
						DestinationRuleController: &istio.DestinationRuleController{
							Cache: drCache,
						},
						ServiceEntryController: &istio.ServiceEntryController{
							Cache: seCache,
						},
					},
				},
			},
			sourceCluster: "cluster1",
			cname:         "stage.foo.split.global",
			expectedError: fmt.Errorf(
				"distribution on the DR stage.foo.split.global-default-dr has a traffic split"),
		},
		{
			name: "Given a DR pointing to active region" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{
					"cluster1": {
						DestinationRuleController: &istio.DestinationRuleController{
							Cache: drCache,
						},
						ServiceEntryController: &istio.ServiceEntryController{
							Cache: seCache,
						},
					},
				},
			},
			sourceCluster:         "cluster1",
			cname:                 "stage.foo.active.global",
			sourceClusterLocality: "us-west-2",
			expectedError: fmt.Errorf(
				"the DR stage.foo.active.global-default-dr is pointing to the active cluster us-west-2 already"),
		},
		{
			name: "Given a DR pointing to passive region" +
				"When doActivePassiveInClusterVS func is called" +
				"Then the func should return a failover GTP",
			remoteRegistry: &RemoteRegistry{
				remoteControllers: map[string]*RemoteController{
					"cluster1": {
						DestinationRuleController: &istio.DestinationRuleController{
							Cache: drCache,
						},
						ServiceEntryController: &istio.ServiceEntryController{
							Cache: seCache,
						},
					},
				},
			},
			sourceCluster:         "cluster1",
			cname:                 "stage.foo.passive.global",
			sourceClusterLocality: "us-west-2",
			expectedGTP: &v1alpha12.GlobalTrafficPolicy{
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{
							DnsPrefix: common.Default,
							LbType:    model.TrafficPolicy_FAILOVER,
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: int32(0),
								},
								{
									Region: "us-east-2",
									Weight: int32(100),
								},
							},
						},
					},
				},
			},
		},
	}

	admiralParams := common.AdmiralParams{
		SyncNamespace: "sync-ns",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualGTP, err := doActivePassiveInClusterVS(tc.remoteRegistry,
				tc.cname, tc.sourceCluster, tc.sourceClusterLocality)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.True(t, reflect.DeepEqual(actualGTP, tc.expectedGTP))
			}
		})
	}
}

func TestIsVSRoutingInClusterDisabledForCluster(t *testing.T) {
	testCases := []struct {
		name                                string
		vsRoutingInClusterDisabledResources map[string]string
		cluster                             string
		expectedResult                      bool
	}{
		{
			name: "Given nil vs routing disabled resources" +
				"When func IsVSRoutingInClusterDisabledForCluster is called" +
				"Then the func should return false",
			expectedResult:                      false,
			vsRoutingInClusterDisabledResources: nil,
		},
		{
			name: "Given empty vs routing disabled resources" +
				"When func IsVSRoutingInClusterDisabledForCluster is called" +
				"Then the func should return false",
			expectedResult:                      false,
			vsRoutingInClusterDisabledResources: map[string]string{},
		},
		{
			name: "Given non-empty vs routing disabled resources but for unique identity" +
				"When func IsVSRoutingInClusterDisabledForCluster is called" +
				"Then the func should return false",
			expectedResult:                      false,
			vsRoutingInClusterDisabledResources: map[string]string{"cluster1": "identity1"},
			cluster:                             "cluster1",
		},
		{
			name: "Given non-empty vs routing disabled resources for all the cluster identities" +
				"When func IsVSRoutingInClusterDisabledForCluster is called" +
				"Then the func should return true",
			expectedResult:                      true,
			vsRoutingInClusterDisabledResources: map[string]string{"cluster1": "*"},
			cluster:                             "cluster1",
		},
		{
			name: "Given non-empty vs routing disabled resources for all the resources" +
				"When func IsVSRoutingInClusterDisabledForCluster is called" +
				"Then the func should return true",
			expectedResult:                      true,
			vsRoutingInClusterDisabledResources: map[string]string{"*": "*"},
			cluster:                             "cluster1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := common.AdmiralParams{}
			p.VSRoutingInClusterDisabledResources = tc.vsRoutingInClusterDisabledResources
			common.ResetSync()
			common.InitializeConfig(p)
			actual := IsVSRoutingInClusterDisabledForCluster(tc.cluster)
			assert.Equal(t, tc.expectedResult, actual)
		})
	}
}

func TestGenerateAuthorityMatches(t *testing.T) {

	testCases := []struct {
		name                     string
		globalFQDN               string
		identity                 string
		expectedAuthorityMatches []*networkingV1Alpha3.HTTPMatchRequest
	}{
		{
			name: "Given a global FQDN and identity does not exists in the FQDN" +
				"When generateAuthorityMatches is called" +
				"Then it should return a matches slice with just the global FQDN",
			globalFQDN: "foo.testns.global",
			identity:   "identity1",
			expectedAuthorityMatches: []*networkingV1Alpha3.HTTPMatchRequest{
				{
					Authority: &networkingV1Alpha3.StringMatch{
						MatchType: &networkingV1Alpha3.StringMatch_Prefix{
							Prefix: "foo.testns.global",
						},
					},
				},
			},
		},
		{
			name: "Given a global FQDN and identity with lower case is passed" +
				"When generateAuthorityMatches is called" +
				"Then it should return a matches slice with both upper and lower case identity",
			globalFQDN: "foo.identity1.global",
			identity:   "identity1",
			expectedAuthorityMatches: []*networkingV1Alpha3.HTTPMatchRequest{
				{
					Authority: &networkingV1Alpha3.StringMatch{
						MatchType: &networkingV1Alpha3.StringMatch_Prefix{
							Prefix: "foo.identity1.global",
						},
					},
				},
				{
					Authority: &networkingV1Alpha3.StringMatch{
						MatchType: &networkingV1Alpha3.StringMatch_Prefix{
							Prefix: "foo.Identity1.global",
						},
					},
				},
			},
		},
		{
			name: "Given a global FQDN and identity with upper case is passed" +
				"When generateAuthorityMatches is called" +
				"Then it should return a matches slice with both upper and lower case identity",
			globalFQDN: "foo.identity1.global",
			identity:   "Identity1",
			expectedAuthorityMatches: []*networkingV1Alpha3.HTTPMatchRequest{
				{
					Authority: &networkingV1Alpha3.StringMatch{
						MatchType: &networkingV1Alpha3.StringMatch_Prefix{
							Prefix: "foo.identity1.global",
						},
					},
				},
				{
					Authority: &networkingV1Alpha3.StringMatch{
						MatchType: &networkingV1Alpha3.StringMatch_Prefix{
							Prefix: "foo.Identity1.global",
						},
					},
				},
			},
		},
		{
			name: "Given a region specific global FQDN and identity with upper case is passed" +
				"When generateAuthorityMatches is called" +
				"Then it should return a matches slice with both upper and lower case identity",
			globalFQDN: "east.foo.identity1.global",
			identity:   "Identity1",
			expectedAuthorityMatches: []*networkingV1Alpha3.HTTPMatchRequest{
				{
					Authority: &networkingV1Alpha3.StringMatch{
						MatchType: &networkingV1Alpha3.StringMatch_Prefix{
							Prefix: "east.foo.identity1.global",
						},
					},
				},
				{
					Authority: &networkingV1Alpha3.StringMatch{
						MatchType: &networkingV1Alpha3.StringMatch_Prefix{
							Prefix: "east.foo.Identity1.global",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualAuthorityMatches := generateAuthorityMatches(tc.globalFQDN, tc.identity)
			assert.Equal(t, len(tc.expectedAuthorityMatches), len(actualAuthorityMatches))
			assert.True(t, reflect.DeepEqual(tc.expectedAuthorityMatches, actualAuthorityMatches))
		})
	}

}

func TestUpdateClientSidecarWithClusterLocalServices(t *testing.T) {

	clientSidecar1 := &apiNetworkingV1Alpha3.Sidecar{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "default",
			Namespace: "clientNS1",
		},
		Spec: networkingV1Alpha3.Sidecar{
			Egress: []*networkingV1Alpha3.IstioEgressListener{
				{
					Hosts: []string{"./*", "admiral-sync/*.mesh"},
				},
			},
		},
	}
	clientSidecar2 := &apiNetworkingV1Alpha3.Sidecar{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "default",
			Namespace: "clientNS2",
		},
		Spec: networkingV1Alpha3.Sidecar{
			Egress: []*networkingV1Alpha3.IstioEgressListener{
				{
					Hosts: []string{"./*", "admiral-sync/*.mesh", "identityNamespace/*.svc.cluster.local"},
				},
			},
		},
	}
	clientSidecar3 := &apiNetworkingV1Alpha3.Sidecar{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "default",
			Namespace: "clientNS3",
		},
		Spec: networkingV1Alpha3.Sidecar{
			Egress: []*networkingV1Alpha3.IstioEgressListener{
				{
					Hosts: []string{"./*", "admiral-sync/*.mesh", "xyz/*.svc.cluster.local"},
				},
			},
		},
	}

	ap := common.AdmiralParams{
		SyncNamespace:                     "admiral-sync",
		WorkloadSidecarName:               "default",
		EnableSidecarCaching:              true,
		MaxSidecarEgressHostsLimitToCache: 100,
	}
	common.ResetSync()
	common.InitializeConfig(ap)

	validSidecarCache := istio.NewSidecarCache()
	validSidecarCache.Put(clientSidecar1)
	validSidecarCache.Put(clientSidecar2)
	validSidecarCache.Put(clientSidecar3)

	testCases := []struct {
		name                        string
		rc                          *RemoteController
		vs                          *apiNetworkingV1Alpha3.VirtualService
		sourceCluster               string
		sourceClusterToEventNsCache map[string]string
		enableSidecarCaching        bool
		expectedError               error
		expectedEgressHosts         map[string][]string
	}{
		{
			name: "Given enableSidecarCaching is set to false" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return no error",
			enableSidecarCaching: false,
			vs: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "test-vs",
					Namespace: "identityNamespace",
				},
			},
		},
		{
			name: "Given enableSidecarCaching is set to true, but remote controller is nil" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return an error",
			enableSidecarCaching: true,
			expectedError:        fmt.Errorf("remoteController is nil"),
		},
		{
			name: "Given sideController is nil" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return an error",
			enableSidecarCaching: true,
			rc:                   &RemoteController{},
			expectedError:        fmt.Errorf("sidecarController is nil"),
		},
		{
			name: "Given sidecarCache is nil" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return an error",
			enableSidecarCaching: true,
			rc: &RemoteController{
				SidecarController: &istio.SidecarController{},
			},
			expectedError: fmt.Errorf("sidecarCache is nil"),
		},
		{
			name: "Given virtualService is nil" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return an error",
			enableSidecarCaching: true,
			rc: &RemoteController{
				SidecarController: &istio.SidecarController{
					SidecarCache: istio.NewSidecarCache(),
				},
			},
			expectedError: fmt.Errorf("virtualService is nil"),
		},
		{
			name: "Given sourceClusterToEventNsCache is nil" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return an error",
			enableSidecarCaching: true,
			rc: &RemoteController{
				SidecarController: &istio.SidecarController{
					SidecarCache: istio.NewSidecarCache(),
				},
			},
			vs:            &apiNetworkingV1Alpha3.VirtualService{},
			expectedError: fmt.Errorf("sourceClusterToEventNsCache is nil"),
		},
		{
			name: "Given identityNamespace is not in sourceClusterToEventNsCache" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return an error",
			enableSidecarCaching: true,
			rc: &RemoteController{
				SidecarController: &istio.SidecarController{
					SidecarCache: istio.NewSidecarCache(),
				},
			},
			vs:                          &apiNetworkingV1Alpha3.VirtualService{},
			sourceCluster:               "cluster1",
			sourceClusterToEventNsCache: map[string]string{"cluster2": "identityNamespace"},
			expectedError:               fmt.Errorf("identityNamespace is empty for sourceCluster cluster1"),
		},
		{
			name: "Given vs with no exportToNS" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return an error",
			enableSidecarCaching: true,
			rc: &RemoteController{
				SidecarController: &istio.SidecarController{
					SidecarCache: istio.NewSidecarCache(),
				},
			},
			vs: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name: "test-vs",
				},
				Spec: networkingV1Alpha3.VirtualService{
					ExportTo: []string{},
				},
			},
			sourceCluster:               "cluster1",
			sourceClusterToEventNsCache: map[string]string{"cluster1": "identityNamespace"},
			expectedError:               fmt.Errorf("exportToNamespaces is nil or empty for virtualService test-vs"),
		},
		{
			name: "Given vs with sync ns in the exportTo" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return",
			enableSidecarCaching: true,
			rc: &RemoteController{
				SidecarController: &istio.SidecarController{
					SidecarCache: istio.NewSidecarCache(),
				},
			},
			vs: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name: "test-vs",
				},
				Spec: networkingV1Alpha3.VirtualService{
					ExportTo: []string{"admiral-sync"},
				},
			},
			sourceCluster:               "cluster1",
			sourceClusterToEventNsCache: map[string]string{"cluster1": "identityNamespace"},
		},
		{
			name: "Given vs with only the identity's own NS" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return",
			enableSidecarCaching: true,
			rc: &RemoteController{
				SidecarController: &istio.SidecarController{
					SidecarCache: istio.NewSidecarCache(),
				},
			},
			vs: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name: "test-vs",
				},
				Spec: networkingV1Alpha3.VirtualService{
					ExportTo: []string{"identityNamespace"},
				},
			},
			sourceCluster:               "cluster1",
			sourceClusterToEventNsCache: map[string]string{"cluster1": "identityNamespace"},
		},
		{
			name: "Given vs with valid client NS but no cached sidecar" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return",
			enableSidecarCaching: true,
			rc: &RemoteController{
				SidecarController: &istio.SidecarController{
					SidecarCache: istio.NewSidecarCache(),
				},
			},
			vs: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name: "test-vs",
				},
				Spec: networkingV1Alpha3.VirtualService{
					ExportTo: []string{"identityNamespace", "clientNS"},
				},
			},
			sourceCluster:               "cluster1",
			sourceClusterToEventNsCache: map[string]string{"cluster1": "identityNamespace"},
		},
		{
			name: "Given vs with valid client NS but no cached sidecar" +
				"When updateClientSidecarWithClusterLocalServices func is called" +
				"Then the func should return",
			enableSidecarCaching: true,
			rc: &RemoteController{
				SidecarController: &istio.SidecarController{
					SidecarCache: validSidecarCache,
				},
			},
			vs: &apiNetworkingV1Alpha3.VirtualService{
				ObjectMeta: metaV1.ObjectMeta{
					Name: "test-vs",
				},
				Spec: networkingV1Alpha3.VirtualService{
					ExportTo: []string{"identityNamespace", "clientNS1", "clientNS2", "clientNS3"},
				},
			},
			sourceCluster:               "cluster1",
			sourceClusterToEventNsCache: map[string]string{"cluster1": "identityNamespace"},
			expectedEgressHosts: map[string][]string{
				"clientNS1": {"./*", "admiral-sync/*.mesh", "identityNamespace/*.svc.cluster.local"},
				"clientNS2": {"./*", "admiral-sync/*.mesh", "identityNamespace/*.svc.cluster.local"},
				"clientNS3": {"./*", "admiral-sync/*.mesh", "xyz/*.svc.cluster.local", "identityNamespace/*.svc.cluster.local"},
			},
		},
	}

	ctxLogger := log.WithFields(log.Fields{
		"type": "VirtualService",
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ap := common.AdmiralParams{
				SyncNamespace:        "admiral-sync",
				WorkloadSidecarName:  "default",
				EnableSidecarCaching: tc.enableSidecarCaching,
			}
			common.ResetSync()
			common.InitializeConfig(ap)

			addUpdateSidecar := func(ctxLogger *log.Entry,
				ctx context.Context,
				newSidecarConfig *apiNetworkingV1Alpha3.Sidecar,
				cachedSidecar *apiNetworkingV1Alpha3.Sidecar,
				clientNamespace string,
				rc *RemoteController) {
				sidecarName := newSidecarConfig.ObjectMeta.Namespace
				sidecarEgressHosts := newSidecarConfig.Spec.Egress
				assert.True(t, reflect.DeepEqual(tc.expectedEgressHosts[sidecarName], sidecarEgressHosts[0].Hosts))
			}

			err := updateClientSidecarWithClusterLocalServices(
				context.Background(), ctxLogger, tc.rc, tc.vs, tc.sourceCluster, tc.sourceClusterToEventNsCache, addUpdateSidecar)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			}

		})
	}

}
