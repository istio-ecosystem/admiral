package clusters

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	testMocks "github.com/istio-ecosystem/admiral/admiral/pkg/test"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	apiNetworkingV1Alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioFake "istio.io/client-go/pkg/clientset/versioned/fake"
	"k8s.io/apimachinery/pkg/runtime/schema"

	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestHandleVirtualServiceEvent(t *testing.T) {
	var (
		clusterID                                = "cluster-1"
		syncNamespace                            = "sync-namespace"
		updateResourceErr                        = fmt.Errorf("updateResource returned error")
		syncVirtualServiceForDependentClusterErr = fmt.Errorf("syncVirtualServiceForDependentCluster returned error")
		syncVirtualServiceForAllClustersErr      = fmt.Errorf("syncVirtualServiceForAllClusters returned error")
		cname1                                   = "one"
		cname2                                   = "two"
		dependentCluster1                        = "dep-cluster-1"
		ctx                                      = context.TODO()
		remoteRegistry                           = NewRemoteRegistry(ctx, common.AdmiralParams{})
		remoteRegistryWithDependents             = newRemoteRegistryWithDependents(ctx, cname1, dependentCluster1)
	)
	cases := []struct {
		name                                         string
		virtualService                               *apiNetworkingV1Alpha3.VirtualService
		params                                       common.AdmiralParams
		remoteRegistry                               *RemoteRegistry
		updateResource                               *fakeUpdateResource
		syncResourceForDependentClusters             *fakeSyncResource
		syncResourceForAllClusters                   *fakeSyncResource
		expectToCallUpdateResource                   bool
		expectToCallSyncResourceForDependentClusters bool
		expectToCallSyncResourceForAllClusters       bool
		expectedErr                                  error
	}{
		{
			name: "Given sync namespace is not configured, " +
				"When, handleVirtualServiceEvent is invoked" +
				"Then, it should return 'passed VirtualService object is nil' error",
			params: common.AdmiralParams{
				SyncNamespace: "",
			},
			remoteRegistry:                   remoteRegistry,
			virtualService:                   nil,
			updateResource:                   newFakeUpdateResource(false, nil),
			syncResourceForDependentClusters: newFakeSyncResource(nil),
			syncResourceForAllClusters:       newFakeSyncResource(nil),
			expectedErr:                      fmt.Errorf("expected valid value for sync namespace, got empty"),
		},
		{
			name: "Given an empty VirtualService object is passed, " +
				"When, handleVirtualServiceEvent is invoked" +
				"Then, it should return 'passed VirtualService object is nil' error",
			params: common.AdmiralParams{
				SyncNamespace: syncNamespace,
			},
			remoteRegistry:                   remoteRegistry,
			virtualService:                   nil,
			updateResource:                   newFakeUpdateResource(false, nil),
			syncResourceForDependentClusters: newFakeSyncResource(nil),
			syncResourceForAllClusters:       newFakeSyncResource(nil),
			expectedErr:                      fmt.Errorf("passed %s object is nil", common.VirtualServiceResourceType),
		},
		{
			name: "Given a VirtualService contains more than 1 host in its spec, " +
				"When, handleVirtualServiceEvent is invoked, " +
				"Then, it should return with a nil, and not call updateResource, and both the syncResource methods",
			params: common.AdmiralParams{
				SyncNamespace: syncNamespace,
			},
			remoteRegistry: remoteRegistry,
			virtualService: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{cname1, cname2},
				},
			},
			updateResource:                               newFakeUpdateResource(false, nil),
			syncResourceForDependentClusters:             newFakeSyncResource(nil),
			syncResourceForAllClusters:                   newFakeSyncResource(nil),
			expectToCallUpdateResource:                   false,
			expectToCallSyncResourceForDependentClusters: false,
			expectToCallSyncResourceForAllClusters:       false,
			expectedErr:                                  nil,
		},
		{
			name: "Given VirtualService is valid and argo is enabled, " +
				"When, handleVirtualServiceEvent is invoked, " +
				"When there are no dependents clusters," +
				"When updateResource returns an error" +
				"Then, it should return an '" + updateResourceErr.Error() + "'error",
			params: common.AdmiralParams{
				ArgoRolloutsEnabled: true,
				SyncNamespace:       syncNamespace,
			},
			remoteRegistry: remoteRegistry,
			virtualService: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{cname1},
				},
			},
			updateResource:                               newFakeUpdateResource(false, updateResourceErr),
			syncResourceForDependentClusters:             newFakeSyncResource(nil),
			syncResourceForAllClusters:                   newFakeSyncResource(nil),
			expectToCallUpdateResource:                   true,
			expectToCallSyncResourceForDependentClusters: false,
			expectToCallSyncResourceForAllClusters:       false,
			expectedErr:                                  updateResourceErr,
		},
		{
			name: "Given VirtualService is valid and argo is enabled, " +
				"When, handleVirtualServiceEvent is invoked, " +
				"When there are no dependents clusters," +
				"When updateResource returns true, and nil" +
				"Then, only updateResource should be called, " +
				"And, it should return nil, " +
				"And, syncVirtualServiceForDependentClusters & syncVirtualServiceForAllClusters should not be called",
			params: common.AdmiralParams{
				ArgoRolloutsEnabled: true,
				SyncNamespace:       syncNamespace,
			},
			remoteRegistry: remoteRegistry,
			virtualService: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{cname1},
				},
			},
			updateResource:                               newFakeUpdateResource(true, nil),
			syncResourceForDependentClusters:             newFakeSyncResource(nil),
			syncResourceForAllClusters:                   newFakeSyncResource(nil),
			expectToCallUpdateResource:                   true,
			expectToCallSyncResourceForDependentClusters: false,
			expectToCallSyncResourceForAllClusters:       false,
			expectedErr:                                  nil,
		},
		{
			name: "Given VirtualService is valid and argo is enabled, " +
				"When, handleVirtualServiceEvent is invoked, " +
				"When there are no dependents clusters," +
				"When updateResource returns true, and an error" +
				"Then, only updateResource should be called, " +
				"And, it should return the same error, " +
				"And, syncVirtualServiceForDependentClusters & syncVirtualServiceForAllClusters should not be called",
			params: common.AdmiralParams{
				ArgoRolloutsEnabled: true,
				SyncNamespace:       syncNamespace,
			},
			remoteRegistry: remoteRegistry,
			virtualService: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{cname1},
				},
			},
			updateResource:                               newFakeUpdateResource(true, updateResourceErr),
			syncResourceForDependentClusters:             newFakeSyncResource(nil),
			syncResourceForAllClusters:                   newFakeSyncResource(nil),
			expectToCallUpdateResource:                   true,
			expectToCallSyncResourceForDependentClusters: false,
			expectToCallSyncResourceForAllClusters:       false,
			expectedErr:                                  updateResourceErr,
		},
		{
			name: "Given VirtualService has 0 hosts in its spec, " +
				"And, argo is enabled, " +
				"When, handleVirtualServiceEvent is invoked, " +
				"When updateResource returns false, and nil" +
				"Then, only updateResource should be called, " +
				"And, it should return nil, " +
				"And, syncVirtualServiceForDependentClusters & syncVirtualServiceForAllClusters should not be called",
			params: common.AdmiralParams{
				ArgoRolloutsEnabled: true,
				SyncNamespace:       syncNamespace,
			},
			remoteRegistry: remoteRegistry,
			virtualService: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{},
				},
			},
			updateResource:                               newFakeUpdateResource(false, nil),
			syncResourceForDependentClusters:             newFakeSyncResource(nil),
			syncResourceForAllClusters:                   newFakeSyncResource(nil),
			expectToCallUpdateResource:                   true,
			expectToCallSyncResourceForDependentClusters: false,
			expectToCallSyncResourceForAllClusters:       false,
			expectedErr:                                  nil,
		},
		{
			name: "Given VirtualService is valid and argo is enabled, " +
				"When, handleVirtualServiceEvent is invoked, " +
				"When there are no dependents clusters," +
				"When updateResource, syncVirtualServiceForDependentClusters, and syncVirtualServiceForDependentClusters return nil" +
				"Then, only updateResource & syncVirtualServiceForAllClusters should be called, " +
				"And, syncVirtualServiceForDependentClusters should NOT be called",
			params: common.AdmiralParams{
				ArgoRolloutsEnabled: true,
				SyncNamespace:       syncNamespace,
			},
			remoteRegistry: remoteRegistry,
			virtualService: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{cname1},
				},
			},
			updateResource:                               newFakeUpdateResource(false, nil),
			syncResourceForDependentClusters:             newFakeSyncResource(nil),
			syncResourceForAllClusters:                   newFakeSyncResource(nil),
			expectToCallUpdateResource:                   true,
			expectToCallSyncResourceForDependentClusters: false,
			expectToCallSyncResourceForAllClusters:       true,
			expectedErr:                                  nil,
		},
		{
			name: "Given VirtualService is valid and argo is enabled, " +
				"When, handleVirtualServiceEvent is invoked, " +
				"When there are dependents clusters," +
				"When updateResource, syncVirtualServiceForDependentClusters, and syncVirtualServiceForDependentClusters return nil" +
				"Then, only updateResource & syncVirtualServiceForDependentClusters should be called, " +
				"And, syncVirtualServiceForAllClusters should NOT be called",
			params: common.AdmiralParams{
				ArgoRolloutsEnabled: true,
				SyncNamespace:       syncNamespace,
			},
			remoteRegistry: remoteRegistryWithDependents,
			virtualService: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{cname1},
				},
			},
			updateResource:                               newFakeUpdateResource(false, nil),
			syncResourceForDependentClusters:             newFakeSyncResource(nil),
			syncResourceForAllClusters:                   newFakeSyncResource(nil),
			expectToCallUpdateResource:                   true,
			expectToCallSyncResourceForDependentClusters: true,
			expectToCallSyncResourceForAllClusters:       false,
			expectedErr:                                  nil,
		},
		{
			name: "Given VirtualService is valid and argo is enabled, " +
				"When, handleVirtualServiceEvent is invoked, " +
				"When there are dependents clusters," +
				"When updateResource returns nil, " +
				"And syncVirtualServiceForDependentClusters returns an error" +
				"Then, it should return nil" +
				"And, syncVirtualServiceForDependentClusters should be called",
			params: common.AdmiralParams{
				ArgoRolloutsEnabled: true,
				SyncNamespace:       syncNamespace,
			},
			remoteRegistry: remoteRegistryWithDependents,
			virtualService: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{cname1},
				},
			},
			updateResource:                               newFakeUpdateResource(false, nil),
			syncResourceForDependentClusters:             newFakeSyncResource(syncVirtualServiceForDependentClusterErr),
			syncResourceForAllClusters:                   newFakeSyncResource(nil),
			expectToCallUpdateResource:                   true,
			expectToCallSyncResourceForDependentClusters: true,
			expectToCallSyncResourceForAllClusters:       false,
			expectedErr:                                  nil,
		},
		{
			name: "Given VirtualService is valid and argo is enabled, " +
				"When, handleVirtualServiceEvent is invoked, " +
				"When there are NOT dependents clusters," +
				"When updateResource syncVirtualServiceForAllClusters & return nil, " +
				"Then only updateResource & syncVirtualServiceForAllClusters should be called, " +
				"And, syncVirtualServiceForDependentClusters should NOT be called" +
				"And, it handleVirtualServiceEvent should return nil",
			params: common.AdmiralParams{
				ArgoRolloutsEnabled: true,
				SyncNamespace:       syncNamespace,
			},
			remoteRegistry: remoteRegistry,
			virtualService: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{cname1},
				},
			},
			updateResource:                               newFakeUpdateResource(false, nil),
			syncResourceForDependentClusters:             newFakeSyncResource(nil),
			syncResourceForAllClusters:                   newFakeSyncResource(nil),
			expectToCallUpdateResource:                   true,
			expectToCallSyncResourceForDependentClusters: false,
			expectToCallSyncResourceForAllClusters:       true,
			expectedErr:                                  nil,
		},
		{
			name: "Given VirtualService is valid and argo is enabled, " +
				"When, handleVirtualServiceEvent is invoked, " +
				"When there are NOT dependents clusters," +
				"When updateResource returns nil, " +
				"When syncVirtualServiceForAllClusters returns an error" +
				"Then only updateResource & syncVirtualServiceForAllClusters should be called, " +
				"And, syncVirtualServiceForDependentClusters should NOT be called" +
				"And, it handleVirtualServiceEvent should return nil",
			params: common.AdmiralParams{
				ArgoRolloutsEnabled: true,
				SyncNamespace:       syncNamespace,
			},
			remoteRegistry: remoteRegistry,
			virtualService: &apiNetworkingV1Alpha3.VirtualService{
				Spec: networkingV1Alpha3.VirtualService{
					Hosts: []string{cname1},
				},
			},
			updateResource:                               newFakeUpdateResource(false, nil),
			syncResourceForDependentClusters:             newFakeSyncResource(nil),
			syncResourceForAllClusters:                   newFakeSyncResource(syncVirtualServiceForAllClustersErr),
			expectToCallUpdateResource:                   true,
			expectToCallSyncResourceForDependentClusters: false,
			expectToCallSyncResourceForAllClusters:       true,
			expectedErr:                                  nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			virtualServiceHandler := &VirtualServiceHandler{
				remoteRegistry:                         c.remoteRegistry,
				clusterID:                              clusterID,
				updateResource:                         c.updateResource.updateResourceFunc(),
				syncVirtualServiceForDependentClusters: c.syncResourceForDependentClusters.syncResourceFunc(),
				syncVirtualServiceForAllClusters:       c.syncResourceForAllClusters.syncResourceFunc(),
			}
			common.ResetSync()
			common.InitializeConfig(c.params)
			err := virtualServiceHandler.handleVirtualServiceEvent(
				ctx,
				c.virtualService,
				common.Add,
			)
			if err != nil && c.expectedErr == nil {
				t.Errorf("expected error to be nil but got %v", err)
			}
			if err != nil && c.expectedErr != nil {
				if !(err.Error() == c.expectedErr.Error()) {
					t.Errorf("error mismatch, expected %v but got %v", c.expectedErr, err)
				}
			}
			if err == nil && c.expectedErr != nil {
				t.Errorf("expected error %v but got nil", c.expectedErr)
			}
			if c.expectToCallUpdateResource && !c.updateResource.called {
				t.Errorf("expected updateResource to be called, but it was not called")
			}
			if !c.expectToCallUpdateResource && c.updateResource.called {
				t.Errorf("expected updateResource to NOT be called, but it was called")
			}
			if c.expectToCallSyncResourceForDependentClusters && !c.syncResourceForDependentClusters.called {
				t.Errorf("expected syncForDependentClusters to be called, but it was not called")
			}
			if !c.expectToCallSyncResourceForDependentClusters && c.syncResourceForDependentClusters.called {
				t.Errorf("expected syncForDependentClusters to NOT be called, but it was called")
			}
			if c.expectToCallSyncResourceForAllClusters && !c.syncResourceForAllClusters.called {
				t.Errorf("expected syncForAllClusters to be called, but it was not called")
			}
			if !c.expectToCallSyncResourceForAllClusters && c.syncResourceForAllClusters.called {
				t.Errorf("expected syncForAllClusters to NOT be called, but it was called")
			}
		})
	}
}

func TestHandleVirtualServiceEventForRollout(t *testing.T) {
	var (
		ctx                                                   = context.TODO()
		cname1                                                = "cname-1"
		namespace1                                            = "namespace-1"
		rollout1                                              = "rollout-name"
		rollout2                                              = "rollout-name2"
		handleEventForRolloutErrForRollout2                   = fmt.Errorf("failed to update rollout")
		expectedHandleEventForRolloutErrForRollout2           = fmt.Errorf("op=Event type=Rollout name=rollout-name2 cluster=cluster-1 message=failed to update rollout")
		remoteRegistryWithRolloutControllerForExistingCluster = newRemoteRegistry(ctx, nil)
		workingVS                                             = &apiNetworkingV1Alpha3.VirtualService{
			Spec: networkingV1Alpha3.VirtualService{
				Hosts: []string{cname1},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "virtual-service-1",
				Namespace: namespace1,
			},
		}
		clusterID                          = "cluster-1"
		rolloutControllerNotInitializedErr = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, workingVS.Name,
			clusterID, "remote controller not initialized for cluster",
		)
		/* TODO:
		rolloutListClientErr = "failed listing rollouts"
			rolloutListErr       = fmt.Errorf(
				LogFormat, "Get", "Rollout",
				"Error finding rollouts in namespace="+workingVS.Namespace, clusterID, rolloutListClientErr,
			)
		*/
	)
	remoteRegistryWithRolloutControllerForExistingCluster.PutRemoteController(
		clusterID, &RemoteController{
			RolloutController: &admiral.RolloutController{
				RolloutClient: testMocks.MockRolloutsGetter{},
			},
		},
	)
	cases := []struct {
		name                                  string
		clusters                              []string
		virtualService                        *apiNetworkingV1Alpha3.VirtualService
		event                                 common.Event
		remoteRegistry                        *RemoteRegistry
		fakeHandleEventForRollout             *fakeHandleEventForRollout
		expectedRolloutVS                     bool
		expectHandleEventForRolloutToBeCalled bool
		expectedErr                           error
	}{
		{
			name: "Given virtualService passed is nil, " +
				"When, handleVirtualServicesForRollout is invoked, " +
				"Then, it should return 'VirtualService is nil' error",
			fakeHandleEventForRollout: newFakeHandleEventForRolloutsByError(nil),
			expectedErr:               fmt.Errorf("VirtualService is nil"),
		},
		{
			name: "Given remoteRegistry is nil, " +
				"When, handleVirtualServicesForRollout is invoked, " +
				"Then, it should return 'remoteRegistry is nil' error",
			virtualService:            &apiNetworkingV1Alpha3.VirtualService{},
			fakeHandleEventForRollout: newFakeHandleEventForRolloutsByError(nil),
			expectedErr:               fmt.Errorf("remoteRegistry is nil"),
		},
		{
			name: "Given remoteRegistry for cluster passed is nil, " +
				"When, handleVirtualServicesForRollout is invoked, " +
				"Then, it should return '" + rolloutControllerNotInitializedErr.Error() + "' error",
			virtualService:            workingVS,
			remoteRegistry:            newRemoteRegistry(ctx, nil),
			fakeHandleEventForRollout: newFakeHandleEventForRolloutsByError(nil),
			expectedErr:               rolloutControllerNotInitializedErr,
		},
		{
			name: "Given rollout a valid list of rollouts, " +
				"And, handleEventForRollout returns nil, " +
				"When, handleVirtualServicesForRollout is invoked, " +
				"When, a rollout matches the virtual service passed, " +
				"Then, it should return true, and nil, " +
				"And, it should call handleEventForRollout function",
			virtualService: workingVS,
			remoteRegistry: remoteRegistryWithRolloutControllerForExistingCluster,
			fakeHandleEventForRollout: newFakeHandleEventForRolloutsByError(map[string]map[string]error{
				testMocks.RolloutNamespace: map[string]error{
					"rollout-name":  nil,
					"rollout-name2": nil,
				},
			}),
			expectHandleEventForRolloutToBeCalled: true,
			expectedRolloutVS:                     true,
			expectedErr:                           nil,
		},
		// TODO: cannot mock return from List yet. Need more code changes
		/*
			{
				name: "Given rollout list returns an error, " +
					"When, handleVirtualServicesForRollout is invoked, " +
					"Then, it should return '" + rolloutListErr.Error() + "' error",
				virtualService:            workingVS,
				remoteRegistry:            remoteRegistryWithRolloutControllerWithListErr,
				fakeHandleEventForRollout: newFakeHandleEventForRollout(nil),
				expectedErr:               rolloutListErr,
			},
		*/
		{
			name: "Given there are multiple rollouts in the given namespace, " +
				"And, handleEventForRollout returns an error for one of them, " +
				"When, handleVirtualServicesForRollout is invoked, " +
				"When, both the rollout match the virtual service passed, " +
				"Then, it should return true, and an error, " +
				"And, it should call handleEventForRollout function",
			virtualService: workingVS,
			remoteRegistry: remoteRegistryWithRolloutControllerForExistingCluster,
			fakeHandleEventForRollout: newFakeHandleEventForRolloutsByError(map[string]map[string]error{
				testMocks.RolloutNamespace: map[string]error{
					"rollout-name":  nil,
					"rollout-name2": handleEventForRolloutErrForRollout2,
				},
			}),
			expectHandleEventForRolloutToBeCalled: true,
			expectedRolloutVS:                     true,
			expectedErr:                           expectedHandleEventForRolloutErrForRollout2,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isRolloutVS, err := handleVirtualServiceEventForRollout(
				ctx,
				c.virtualService,
				c.remoteRegistry,
				clusterID,
				c.fakeHandleEventForRollout.handleEventForRolloutFunc(),
			)
			if err != nil && c.expectedErr == nil {
				t.Errorf("expected error to be nil but got %v", err)
			}
			if err != nil && c.expectedErr != nil {
				if !(err.Error() == c.expectedErr.Error()) {
					t.Errorf("error mismatch, expected '%v' but got '%v'", c.expectedErr, err)
				}
			}
			if err == nil && c.expectedErr != nil {
				t.Errorf("expected error %v but got nil", c.expectedErr)
			}
			if isRolloutVS != c.expectedRolloutVS {
				t.Errorf("expected: %v, got: %v", c.expectedRolloutVS, isRolloutVS)
			}
			if c.expectHandleEventForRolloutToBeCalled && (c.fakeHandleEventForRollout.calledByRolloutName[rollout1] &&
				c.fakeHandleEventForRollout.calledByRolloutName[rollout2]) {
				t.Errorf("expected handleRollout to be called, but it was not")
			}
		})
	}
}

func TestSyncVirtualServicesToAllDependentClusters(t *testing.T) {
	var (
		ctx               = context.TODO()
		cname1            = "cname1"
		namespace1        = "namespace1"
		syncNamespace     = "sync-namespace"
		dependentCluster1 = "dep-cluster1"
		dependentCluster2 = "dep-cluster2"
		sourceCluster     = "cluster1"
		workingVS         = &apiNetworkingV1Alpha3.VirtualService{
			Spec: networkingV1Alpha3.VirtualService{
				Hosts: []string{cname1},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "virtual-service-1",
				Namespace: namespace1,
			},
		}

		fakeIstioClientWithoutAnyVirtualServices         = istioFake.NewSimpleClientset()
		fakeIstioClientWithoutKnownVirtualServices       = newFakeIstioClient(ctx, namespace1, workingVS)
		nilVirtualServiceControllerForDependencyCluster1 = map[string]*RemoteController{
			dependentCluster1: &RemoteController{},
		}
		virtualServiceControllerForDepCluster1AndNilForCluster2 = map[string]*RemoteController{
			dependentCluster1: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: istioFake.NewSimpleClientset(),
				},
			},
			dependentCluster2: &RemoteController{},
		}

		virtualServiceControllerForKnownClustersWithoutAnyVirtualServices = map[string]*RemoteController{
			dependentCluster1: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutAnyVirtualServices,
				},
			},
		}
		virtualServiceControllerForKnownClustersWithKnownVirtualServices = map[string]*RemoteController{
			dependentCluster1: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutKnownVirtualServices,
				},
			},
		}
		virtualServiceControllerForSourceClustersWithoutAnyVirtualServices = map[string]*RemoteController{
			sourceCluster: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutAnyVirtualServices,
				},
			},
			dependentCluster1: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutKnownVirtualServices,
				},
			},
		}
		virtualServiceControllerForSourceClustersWithKnownVirtualServices = map[string]*RemoteController{
			dependentCluster1: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutKnownVirtualServices,
				},
			},
			sourceCluster: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutKnownVirtualServices,
				},
			},
		}

		cluster1                        = []string{dependentCluster1}
		clusters1And2                   = []string{dependentCluster1, dependentCluster2}
		clustersContainingSourceCluster = []string{dependentCluster1, sourceCluster}
		emptyVSErr                      = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, "", sourceCluster,
			"VirtualService is nil",
		)
		emptyRemoteRegistryErr = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, "", sourceCluster,
			"remoteRegistry is nil",
		)
		nilRemoteControllerForDepCluster1Err = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, workingVS.Name, dependentCluster1,
			"dependent controller not initialized for cluster",
		)
		nilRemoteControllerForDepCluster2Err = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, workingVS.Name, dependentCluster2,
			"dependent controller not initialized for cluster",
		)
		virtualServiceControllerNotInitializedForCluster1Err = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, workingVS.Name, dependentCluster1,
			"VirtualService controller not initialized for cluster",
		)
		virtualServiceControllerNotInitializedForCluster2Err = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, workingVS.Name, dependentCluster2,
			"VirtualService controller not initialized for cluster",
		)
	)

	cases := []struct {
		name                    string
		clusters                []string
		virtualService          *apiNetworkingV1Alpha3.VirtualService
		event                   common.Event
		remoteRegistry          *RemoteRegistry
		sourceCluster           string
		syncNamespace           string
		assertFunc              func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T)
		doSyncVSToSourceCluster bool
		expectedErr             error
	}{
		{
			name: "Given a nil VirtualService is passed , " +
				"When, syncVirtualServicesToAllDependentClusters is invoked, " +
				"Then, it should return '" + emptyVSErr.Error() + "' error",
			sourceCluster: sourceCluster,
			expectedErr:   emptyVSErr,
		},
		{
			name: "Given a nil remoteRegistry is passed , " +
				"When, syncVirtualServicesToAllDependentClusters is invoked, " +
				"Then, it should return '" + emptyRemoteRegistryErr.Error() + "' error",
			sourceCluster:  sourceCluster,
			virtualService: workingVS,
			expectedErr:    emptyRemoteRegistryErr,
		},
		{
			name: "Given remote controller for cluster is not initialized , " +
				"When, syncVirtualServicesToAllDependentClusters is invoked, " +
				"Then, it should return '" + nilRemoteControllerForDepCluster1Err.Error() + "' error",
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, nil),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			expectedErr:    nilRemoteControllerForDepCluster1Err,
		},
		{
			name: "Given remote controller for one cluster is not initialized , " +
				"And, there is another cluster, which has remote controller initialized, " +
				"When, syncVirtualServicesToAllDependentClusters is invoked, " +
				"Then, it should return '" + nilRemoteControllerForDepCluster1Err.Error() + "' error" +
				"And, it creates VirtualService for cluster which has remote controller initialized",
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForKnownClustersWithoutAnyVirtualServices),
			clusters:       clusters1And2,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					// cluster with no nil pointer exception
					if cluster == dependentCluster1 {
						rc := remoteRegistry.GetRemoteController(cluster)
						vs, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
						if err != nil {
							t.Errorf("expected nil, but got error: %v", err)
							return
						}
						if vs == nil || vs.Name != workingVS.Name {
							t.Errorf("expected VirtualService to be created, but it was not")
						}
					}
				}
			},
			expectedErr: nilRemoteControllerForDepCluster2Err,
		},
		{
			name: "Given VirtualServiceController for cluster is not initialized , " +
				"When, syncVirtualServicesToAllDependentClusters is invoked, " +
				"Then, it should return '" + virtualServiceControllerNotInitializedForCluster1Err.Error() + "' error",
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, nilVirtualServiceControllerForDependencyCluster1),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			expectedErr:    virtualServiceControllerNotInitializedForCluster1Err,
		},
		{
			name: "Given VirtualServiceController for one cluster is not initialized , " +
				"And, there is another cluster, which has VirtualServiceController initialized, " +
				"When, syncVirtualServicesToAllDependentClusters is invoked, " +
				"Then, it should return '" + virtualServiceControllerNotInitializedForCluster2Err.Error() + "' error" +
				"And, it should create VirtualService for cluster which has the controller initialized",
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForDepCluster1AndNilForCluster2),
			clusters:       clusters1And2,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					// cluster with no nil pointer exception
					if cluster == dependentCluster1 {
						rc := remoteRegistry.GetRemoteController(cluster)
						vs, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
						if err != nil {
							t.Errorf("expected nil, but got error: %v", err)
							return
						}
						if vs == nil || vs.Name != workingVS.Name {
							t.Errorf("expected VirtualService to be created, but it was not")
						}
					}
				}
			},
			expectedErr: virtualServiceControllerNotInitializedForCluster2Err,
		},
		{
			name: "Given a valid VirtualService is passed for CREATE event, " +
				"And the VirtualService does not exist, " +
				"When, syncVirtualServicesToAllDependentClusters is invoked, " +
				"Then, the new VirtualService should be created" +
				"And, it should not return an error",
			event:          common.Add,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForKnownClustersWithoutAnyVirtualServices),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					vs, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if err != nil {
						t.Errorf("expected nil, but got error: %v", err)
						return
					}
					if vs == nil || vs.Name != workingVS.Name {
						t.Errorf("expected VirtualService to be created, but it was not")
					}
				}
			},
			expectedErr: nil,
		},
		{
			name: "Given a valid VirtualService is passed UPDATE event, " +
				"And the VirtualService already exists, " +
				"When, syncVirtualServicesToAllDependentClusters is invoked, " +
				"Then, the VirtualService should be updated" +
				"And, it should not return an error",
			event:          common.Update,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForKnownClustersWithKnownVirtualServices),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					vs, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if err != nil {
						t.Errorf("expected nil, but got error: %v", err)
						return
					}
					if vs == nil || vs.Name != workingVS.Name {
						t.Errorf("expected VirtualService to be created, but it was not")
					}
				}
			},
			expectedErr: nil,
		},

		{
			name: "Given a valid VirtualService is passed for DELETE event, " +
				"And the VirtualService exists, " +
				"When, syncVirtualServicesToAllDependentClusters is invoked, " +
				"Then, the VirtualService should be deleted, " +
				"And, it should not return an error",
			event:          common.Delete,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForKnownClustersWithKnownVirtualServices),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					_, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if !k8sErrors.IsNotFound(err) {
						t.Errorf("expected error to be Not Found, but got: %v", err)
					}
				}
			},
			expectedErr: nil,
		},
		{
			name: "Given a valid VirtualService is passed for DELETE event, " +
				"And the VirtualService does not exist, " +
				"When, syncVirtualServicesToAllDependentClusters is invoked, " +
				"Then, the VirtualService should be deleted",
			event:          common.Delete,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForKnownClustersWithoutAnyVirtualServices),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					_, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if !k8sErrors.IsNotFound(err) {
						t.Errorf("expected error to be Not Found, but got: %v", err)
					}
				}
			},
			expectedErr: nil,
		},
		{
			name: "Given a valid VirtualService is passed for CREATE event, " +
				"And the VirtualService does not exist, " +
				"When, an asset has a client in the source cluster, " +
				"Then, the new VirtualService should be created in source and dependent clusters" +
				"And, it should not return an error",
			event:          common.Add,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForSourceClustersWithoutAnyVirtualServices),
			clusters:       clustersContainingSourceCluster,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					vs, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if err != nil {
						t.Errorf("expected nil, but got error: %v", err)
						return
					}
					if vs == nil || vs.Name != workingVS.Name {
						t.Errorf("expected VirtualService to be created, but it was not")
					}
				}
			},
			doSyncVSToSourceCluster: true,
			expectedErr:             nil,
		},
		{
			name: "Given a valid VirtualService is passed for DELETE event, " +
				"And the VirtualService exist, " +
				"When, an asset has a client in the source cluster, " +
				"Then, the new VirtualService should be deleted in source and dependent clusters" +
				"And, it should not return an error",
			event:          common.Add,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForSourceClustersWithKnownVirtualServices),
			clusters:       clustersContainingSourceCluster,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					vs, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if err != nil {
						t.Errorf("expected nil, but got error: %v", err)
						return
					}
					if vs == nil || vs.Name != workingVS.Name {
						t.Errorf("expected VirtualService to be created, but it was not")
					}
				}
			},
			doSyncVSToSourceCluster: true,
			expectedErr:             nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			common.ResetSync()
			admiralParams := common.AdmiralParams{
				EnableSyncIstioResourcesToSourceClusters: c.doSyncVSToSourceCluster,
			}
			common.InitializeConfig(admiralParams)
			err := syncVirtualServicesToAllDependentClusters(
				ctx,
				c.clusters,
				c.virtualService,
				c.event,
				c.remoteRegistry,
				c.sourceCluster,
				syncNamespace,
			)
			if err != nil && c.expectedErr == nil {
				t.Errorf("expected error to be nil but got %v", err)
			}
			if err != nil && c.expectedErr != nil {
				if !(err.Error() == c.expectedErr.Error()) {
					t.Errorf("error mismatch, expected %v but got %v", c.expectedErr, err)
				}
			}
			if err == nil && c.expectedErr != nil {
				t.Errorf("expected error %v but got nil", c.expectedErr)
			}
			if c.assertFunc != nil {
				c.assertFunc(c.remoteRegistry, c.clusters, t)
			}
		})
	}
}

func TestSyncVirtualServicesToAllRemoteClusters(t *testing.T) {
	var (
		ctx               = context.TODO()
		cname1            = "cname1"
		namespace1        = "namespace1"
		syncNamespace     = "sync-namespace"
		dependentCluster1 = "dep-cluster1"
		dependentCluster2 = "dep-cluster2"
		sourceCluster     = "cluster1"
		workingVS         = &apiNetworkingV1Alpha3.VirtualService{
			Spec: networkingV1Alpha3.VirtualService{
				Hosts: []string{cname1},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "virtual-service-1",
				Namespace: namespace1,
			},
		}
		fakeIstioClientWithoutAnyVirtualServices    = istioFake.NewSimpleClientset()
		fakeIstioClientWithoutKnownVirtualServices  = newFakeIstioClient(ctx, namespace1, workingVS)
		nilVirtualServiceControllerForKnownClusters = map[string]*RemoteController{
			dependentCluster1: &RemoteController{},
		}
		virtualServiceControllerForDepCluster1AndNilForCluster2 = map[string]*RemoteController{
			dependentCluster1: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: istioFake.NewSimpleClientset(),
				},
			},
			dependentCluster2: &RemoteController{},
		}
		virtualServiceControllerForKnownClustersWithoutAnyVirtualServices = map[string]*RemoteController{
			dependentCluster1: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutAnyVirtualServices,
				},
			},
		}
		virtualServiceControllerForKnownClustersWithKnownVirtualServices = map[string]*RemoteController{
			dependentCluster1: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutKnownVirtualServices,
				},
			},
		}
		virtualServiceControllerForSourceClustersWithKnownVirtualServices = map[string]*RemoteController{
			dependentCluster1: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutKnownVirtualServices,
				},
			},
			sourceCluster: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutKnownVirtualServices,
				},
			},
		}
		virtualServiceControllerForSourceClustersWithoutAnyVirtualServices = map[string]*RemoteController{
			dependentCluster1: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutAnyVirtualServices,
				},
			},
			sourceCluster: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: fakeIstioClientWithoutAnyVirtualServices,
				},
			},
		}
		cluster1                        = []string{dependentCluster1}
		cluster1And2                    = []string{dependentCluster1, dependentCluster2}
		clustersContainingSourceCluster = []string{dependentCluster1, sourceCluster}
		emptyVSErr                      = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, "", sourceCluster,
			"VirtualService is nil",
		)
		emptyRemoteRegistryErr = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, "", sourceCluster,
			"remoteRegistry is nil",
		)
		nilRemoteControllerForDepCluster1Err = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, workingVS.Name, dependentCluster1,
			"remote controller not initialized for cluster",
		)
		nilRemoteControllerForDepCluster2Err = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, workingVS.Name, dependentCluster2,
			"remote controller not initialized for cluster",
		)
		virtualServiceControllerNotInitializedForCluster1Err = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, workingVS.Name, dependentCluster1,
			"VirtualService controller not initialized for cluster",
		)
		virtualServiceControllerNotInitializedForCluster2Err = fmt.Errorf(
			LogFormat, "Event", common.VirtualServiceResourceType, workingVS.Name, dependentCluster2,
			"VirtualService controller not initialized for cluster",
		)
	)

	cases := []struct {
		name                    string
		clusters                []string
		virtualService          *apiNetworkingV1Alpha3.VirtualService
		event                   common.Event
		remoteRegistry          *RemoteRegistry
		sourceCluster           string
		syncNamespace           string
		assertFunc              func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T)
		doSyncVSToSourceCluster bool
		expectedErr             error
	}{
		{
			name: "Given a nil VirtualService is passed , " +
				"When, syncVirtualServicesToAllRemoteClusters is invoked, " +
				"Then, it should return '" + emptyVSErr.Error() + "' error",
			sourceCluster: sourceCluster,
			expectedErr:   emptyVSErr,
		},
		{
			name: "Given a nil remoteRegistry is passed , " +
				"When, syncVirtualServicesToAllRemoteClusters is invoked, " +
				"Then, it should return '" + emptyRemoteRegistryErr.Error() + "' error",
			sourceCluster:  sourceCluster,
			virtualService: workingVS,
			expectedErr:    emptyRemoteRegistryErr,
		},
		{
			name: "Given remote controller for cluster is not initialized , " +
				"When, syncVirtualServicesToAllRemoteClusters is invoked, " +
				"Then, it should return '" + nilRemoteControllerForDepCluster1Err.Error() + "' error",
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, nil),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			expectedErr:    nilRemoteControllerForDepCluster1Err,
		},
		{
			name: "Given remote controller for one cluster is not initialized , " +
				"And, it is initialized for another cluster, " +
				"When, syncVirtualServicesToAllRemoteClusters is invoked, " +
				"Then, it should return '" + nilRemoteControllerForDepCluster1Err.Error() + "' error" +
				"And, it creates VirtualService for cluster which has remote controller initialized",
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForKnownClustersWithoutAnyVirtualServices),
			clusters:       cluster1And2,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					// cluster with no nil pointer exception
					if cluster == dependentCluster1 {
						rc := remoteRegistry.GetRemoteController(cluster)
						vs, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
						if err != nil {
							t.Errorf("expected nil, but got error: %v", err)
							return
						}
						if vs == nil || vs.Name != workingVS.Name {
							t.Errorf("expected VirtualService to be created, but it was not")
						}
					}
				}
			},
			expectedErr: nilRemoteControllerForDepCluster2Err,
		},
		{
			name: "Given VirtualServiceController for cluster is not initialized , " +
				"When, syncVirtualServicesToAllRemoteClusters is invoked, " +
				"Then, it should return '" + virtualServiceControllerNotInitializedForCluster1Err.Error() + "' error",
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, nilVirtualServiceControllerForKnownClusters),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			expectedErr:    virtualServiceControllerNotInitializedForCluster1Err,
		},
		{
			name: "Given VirtualServiceController for one cluster is not initialized , " +
				"And VirtualServiceController is initialized for another cluster, " +
				"When, syncVirtualServicesToAllRemoteClusters is invoked, " +
				"Then, it should return '" + virtualServiceControllerNotInitializedForCluster2Err.Error() + "' error" +
				"And, it should create VirtualService for cluster which has it initialized",
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForDepCluster1AndNilForCluster2),
			clusters:       cluster1And2,
			sourceCluster:  sourceCluster,
			expectedErr:    virtualServiceControllerNotInitializedForCluster2Err,
		},
		{
			name: "Given a valid VirtualService is passed for CREATE event, " +
				"And the VirtualService does not exist, " +
				"When, syncVirtualServicesToAllRemoteClusters is invoked, " +
				"Then, the new VirtualService should be created" +
				"And, it should not return an error",
			event:          common.Add,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForKnownClustersWithoutAnyVirtualServices),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					vs, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if err != nil {
						t.Errorf("expected nil, but got error: %v", err)
						return
					}
					if vs == nil || vs.Name != workingVS.Name {
						t.Errorf("expected VirtualService to be created, but it was not")
					}
				}
			},
			expectedErr: nil,
		},
		{
			name: "Given a valid VirtualService is passed UPDATE event, " +
				"And the VirtualService already exists, " +
				"When, syncVirtualServicesToAllRemoteClusters is invoked, " +
				"Then, the VirtualService should be updated" +
				"And, it should not return an error",
			event:          common.Update,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForKnownClustersWithKnownVirtualServices),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					vs, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if err != nil {
						t.Errorf("expected nil, but got error: %v", err)
						return
					}
					if vs == nil || vs.Name != workingVS.Name {
						t.Errorf("expected VirtualService to be created, but it was not")
					}
				}
			},
			expectedErr: nil,
		},

		{
			name: "Given a valid VirtualService is passed for DELETE event, " +
				"And the VirtualService exists, " +
				"When, syncVirtualServicesToAllRemoteClusters is invoked, " +
				"Then, the VirtualService should be deleted, " +
				"And, it should not return an error",
			event:          common.Delete,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForKnownClustersWithKnownVirtualServices),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					_, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if !k8sErrors.IsNotFound(err) {
						t.Errorf("expected error to be Not Found, but got: %v", err)
					}
				}
			},
			expectedErr: nil,
		},
		{
			name: "Given a valid VirtualService is passed for DELETE event, " +
				"And the VirtualService does not exist, " +
				"When, syncVirtualServicesToAllRemoteClusters is invoked, " +
				"Then, the VirtualService should be deleted",
			event:          common.Delete,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForKnownClustersWithoutAnyVirtualServices),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					_, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if !k8sErrors.IsNotFound(err) {
						t.Errorf("expected error to be Not Found, but got: %v", err)
					}
				}
			},
			expectedErr: nil,
		},
		{
			name: "Given a valid VirtualService is passed for CREATE event, " +
				"And the VirtualService does not exist, " +
				"When, an asset has a client in the source cluster, " +
				"Then, the new VirtualService should be created in source and dependent clusters" +
				"And, it should not return an error",
			event:          common.Add,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForSourceClustersWithoutAnyVirtualServices),
			clusters:       clustersContainingSourceCluster,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					vs, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if err != nil {
						t.Errorf("expected nil, but got error: %v", err)
						return
					}
					if vs == nil || vs.Name != workingVS.Name {
						t.Errorf("expected VirtualService to be created, but it was not")
					}
				}
			},
			doSyncVSToSourceCluster: true,
			expectedErr:             nil,
		},
		{
			name: "Given a valid VirtualService is passed for DELETE event, " +
				"And the VirtualService does not exist, " +
				"When, an asset has a client in the source cluster, " +
				"Then, the VirtualService should be deleted from source and dependent clusters",
			event:          common.Delete,
			virtualService: workingVS,
			remoteRegistry: newRemoteRegistry(ctx, virtualServiceControllerForSourceClustersWithKnownVirtualServices),
			clusters:       cluster1,
			sourceCluster:  sourceCluster,
			assertFunc: func(remoteRegistry *RemoteRegistry, clusters []string, t *testing.T) {
				for _, cluster := range clusters {
					rc := remoteRegistry.GetRemoteController(cluster)
					_, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(syncNamespace).Get(ctx, workingVS.Name, metaV1.GetOptions{})
					if !k8sErrors.IsNotFound(err) {
						t.Errorf("expected error to be Not Found, but got: %v", err)
					}
				}
			},
			doSyncVSToSourceCluster: true,
			expectedErr:             nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			common.ResetSync()
			admiralParams := common.AdmiralParams{
				EnableSyncIstioResourcesToSourceClusters: c.doSyncVSToSourceCluster,
			}
			common.InitializeConfig(admiralParams)
			err := syncVirtualServicesToAllRemoteClusters(
				ctx,
				c.clusters,
				c.virtualService,
				c.event,
				c.remoteRegistry,
				c.sourceCluster,
				syncNamespace,
			)
			if err != nil && c.expectedErr == nil {
				t.Errorf("expected error to be nil but got %v", err)
			}
			if err != nil && c.expectedErr != nil {
				if !(err.Error() == c.expectedErr.Error()) {
					t.Errorf("error mismatch, expected %v but got %v", c.expectedErr, err)
				}
			}
			if err == nil && c.expectedErr != nil {
				t.Errorf("expected error %v but got %v", c.expectedErr, err)
			}
			if c.assertFunc != nil {
				c.assertFunc(c.remoteRegistry, c.clusters, t)
			}
		})
	}
}

func TestVirtualSvcHandlerCUDScenarios(t *testing.T) {
	ctx := context.Background()

	admiralParams := common.AdmiralParams{
		LabelSet:            &common.LabelSet{},
		SyncNamespace:       "test-sync-ns",
		ArgoRolloutsEnabled: true,
	}
	common.InitializeConfig(admiralParams)

	vs := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{Name: "my-vs", Namespace: "test-ns"},
		Spec: networkingV1Alpha3.VirtualService{
			Http: []*networkingV1Alpha3.HTTPRoute{{Name: "random", Route: []*networkingV1Alpha3.HTTPRouteDestination{
				{Destination: &networkingV1Alpha3.Destination{Host: "stable-host"}, Weight: 100},
				{Destination: &networkingV1Alpha3.Destination{Host: "canary-host"}, Weight: 0},
			}}},
		},
	}

	var (
		config = rest.Config{
			Host: "localhost",
		}
		stop = make(chan struct{})
	)

	r, err := admiral.NewRolloutsController(stop, &testMocks.MockRolloutHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("failed ot initialize rollout controller, err: %v", err)
	}

	r.RolloutClient = testMocks.MockRolloutsGetter{}

	rr := NewRemoteRegistry(ctx, admiralParams)
	rr.PutRemoteController("test-cluster", &RemoteController{
		RolloutController: r,
	})

	vsHandler := &VirtualServiceHandler{
		clusterID:      "test-cluster",
		remoteRegistry: rr,
		updateResource: handleVirtualServiceEventForRollout,
	}

	admiralParams = common.AdmiralParams{
		LabelSet:            &common.LabelSet{},
		SyncNamespace:       "test-sync-ns",
		ArgoRolloutsEnabled: false,
	}
	common.InitializeConfig(admiralParams)
	vsHandler2 := &VirtualServiceHandler{
		clusterID:      "test-cluster",
		remoteRegistry: NewRemoteRegistry(ctx, admiralParams),
		updateResource: handleVirtualServiceEventForRollout,
	}

	cases := []struct {
		name                string
		admiralReadState    bool
		ns                  string
		handler             *VirtualServiceHandler
		argoRolloutsEnabled bool
	}{
		{
			name:                "virtual service used by Argo rollouts case",
			admiralReadState:    false,
			ns:                  "test-ns",
			handler:             vsHandler,
			argoRolloutsEnabled: true,
		},
		{
			name:                "Admiral in read-only state",
			admiralReadState:    true,
			ns:                  "test-ns",
			handler:             vsHandler2,
			argoRolloutsEnabled: false,
		},
		{
			name:                "Encountered istio resource",
			admiralReadState:    false,
			ns:                  "istio-system",
			handler:             vsHandler2,
			argoRolloutsEnabled: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			common.ResetSync()
			admiralParams = common.AdmiralParams{
				LabelSet:            &common.LabelSet{},
				SyncNamespace:       "test-sync-ns",
				ArgoRolloutsEnabled: c.argoRolloutsEnabled,
			}
			common.InitializeConfig(admiralParams)
			commonUtil.CurrentAdmiralState.ReadOnly = c.admiralReadState
			vs.ObjectMeta.Namespace = c.ns
			err := c.handler.Added(ctx, vs)
			assert.NoError(t, err)

			err = c.handler.Updated(ctx, vs)
			assert.NoError(t, err)

			err = c.handler.Deleted(ctx, vs)
			assert.NoError(t, err)

		})
	}
}

func TestDeleteVirtualService(t *testing.T) {
	ctx := context.Background()
	namespace := "testns"

	fooVS := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "stage.test00.foo-vs",
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts: []string{"stage.test00.foo", "stage.test00.bar"},
		},
	}

	validIstioClient := istioFake.NewSimpleClientset()
	validIstioClient.NetworkingV1alpha3().VirtualServices(namespace).Create(ctx, fooVS, metaV1.CreateOptions{})

	testcases := []struct {
		name                  string
		virtualService        *apiNetworkingV1Alpha3.VirtualService
		rc                    *RemoteController
		expectedError         error
		expectedDeletedVSName string
	}{
		{
			name:           "Given virtualservice to delete, when nil VS is passed, the func should return an error",
			virtualService: nil,
			expectedError:  fmt.Errorf("the VirtualService passed was nil"),
		},
		{
			name:           "Given virtualservice to delete, when VS passed does not exists, the func should return an error",
			virtualService: &apiNetworkingV1Alpha3.VirtualService{ObjectMeta: metaV1.ObjectMeta{Name: "vs-does-not-exists"}},
			expectedError:  fmt.Errorf("either VirtualService was already deleted, or it never existed"),
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
		},
		{
			name:           "Given virtualservice to delete, when VS exists, the func should delete the VS and not return any error",
			virtualService: fooVS,
			expectedError:  nil,
			rc: &RemoteController{
				VirtualServiceController: &istio.VirtualServiceController{
					IstioClient: validIstioClient,
				},
			},
			expectedDeletedVSName: "stage.test00.foo-vs",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {

			err := deleteVirtualService(ctx, tc.virtualService, namespace, tc.rc)

			if err != nil && tc.expectedError != nil {
				if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("expected %s, got %s", tc.expectedError.Error(), err.Error())
				}
			} else if err != tc.expectedError {
				t.Errorf("expected %v, got %v", tc.expectedError, err)
			}

			if err == nil && tc.expectedDeletedVSName != "" {
				_, err := tc.rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(namespace).Get(context.Background(), tc.expectedDeletedVSName, metaV1.GetOptions{})
				if err != nil && !k8sErrors.IsNotFound(err) {
					t.Errorf("test failed as VS should have been deleted. error: %v", err)
				}
			}

		})
	}

}

type fakeSyncResource struct {
	syncResourceFunc func() SyncVirtualServiceResource
	called           bool
}

func newFakeSyncResource(err error) *fakeSyncResource {
	f := &fakeSyncResource{}
	f.syncResourceFunc = func() SyncVirtualServiceResource {
		return func(
			ctx context.Context,
			dependentClusters []string,
			obj *apiNetworkingV1Alpha3.VirtualService,
			event common.Event,
			remoteRegistry *RemoteRegistry,
			clusterId string,
			syncNamespace string) error {
			f.called = true
			return err
		}
	}
	return f
}

type fakeUpdateResource struct {
	updateResourceFunc func() UpdateResourcesForVirtualService
	called             bool
}

func newFakeUpdateResource(isCanaryVS bool, err error) *fakeUpdateResource {
	f := &fakeUpdateResource{}
	f.updateResourceFunc = func() UpdateResourcesForVirtualService {
		return func(
			ctx context.Context,
			virtualService *apiNetworkingV1Alpha3.VirtualService,
			remoteRegistry *RemoteRegistry,
			clusterID string,
			handlerFunc HandleEventForRolloutFunc) (bool, error) {
			f.called = true
			return isCanaryVS, err
		}
	}
	return f
}

func newRemoteRegistryWithDependents(ctx context.Context, cname, clusterID string) *RemoteRegistry {
	remoteRegistry := NewRemoteRegistry(ctx, common.AdmiralParams{})
	remoteRegistry.AdmiralCache.CnameDependentClusterCache.Put(cname, clusterID, clusterID)
	return remoteRegistry
}

func newRemoteRegistry(ctx context.Context, clusters map[string]*RemoteController) *RemoteRegistry {
	remoteRegistry := NewRemoteRegistry(ctx, common.AdmiralParams{})
	for cluster, controller := range clusters {
		remoteRegistry.PutRemoteController(cluster, controller)
	}
	return remoteRegistry
}

func newFakeIstioClient(ctx context.Context, namespace string, vs *apiNetworkingV1Alpha3.VirtualService) *istioFake.Clientset {
	fakeIstioClientWithoutKnownVirtualServices := istioFake.NewSimpleClientset()
	fakeIstioClientWithoutKnownVirtualServices.
		NetworkingV1alpha3().
		VirtualServices(namespace).
		Create(ctx, vs, metaV1.CreateOptions{})
	return fakeIstioClientWithoutKnownVirtualServices
}

func TestRetryUpdatingVS(t *testing.T) {

	ctxLogger := log.WithFields(log.Fields{
		"type": "retryUpdatingVS",
	})
	ctx := context.TODO()

	vsDoesNotExists := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "vs-does-not-exists",
			Namespace: common.GetSyncNamespace(),
		},
	}
	vs := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:        "vsToBeUpdated",
			Namespace:   common.GetSyncNamespace(),
			Labels:      map[string]string{"updated": "false"},
			Annotations: map[string]string{"updated": "false"},
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts: []string{"old.host"},
		},
	}
	istioClient := istioFake.NewSimpleClientset()
	istioClient.
		NetworkingV1alpha3().
		VirtualServices(common.GetSyncNamespace()).
		Create(ctx, vs, metaV1.CreateOptions{})

	rc := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: istioClient,
		},
	}

	vsThatShouldBeUpdated := &apiNetworkingV1Alpha3.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name:        "vsToBeUpdated",
			Namespace:   common.GetSyncNamespace(),
			Labels:      map[string]string{"updated": "true"},
			Annotations: map[string]string{"updated": "true"},
		},
		Spec: networkingV1Alpha3.VirtualService{
			Hosts: []string{"new.host"},
		},
	}

	testCases := []struct {
		name          string
		newVS         *apiNetworkingV1Alpha3.VirtualService
		existingVS    *apiNetworkingV1Alpha3.VirtualService
		err           error
		expectedError error
		expectedVS    *apiNetworkingV1Alpha3.VirtualService
	}{
		{
			name: "Given valid params " +
				"When the error passed is nil" +
				"Then the func should not update the vs and return no errors",
			newVS:         vs,
			existingVS:    vs,
			err:           nil,
			expectedVS:    vs,
			expectedError: nil,
		},
		{
			name: "Given valid params " +
				"When the error is of not type IsConflict" +
				"Then the func should not update the vs and return no errors",
			newVS:         vs,
			existingVS:    vs,
			err:           fmt.Errorf("some other error"),
			expectedVS:    vs,
			expectedError: fmt.Errorf("some other error"),
		},
		{
			name: "Given valid params " +
				"When the passed VS does not exists" +
				"Then the func should not update the vs and return no errors",
			newVS:         vs,
			existingVS:    vsDoesNotExists,
			err:           k8sErrors.NewConflict(schema.GroupResource{}, "", fmt.Errorf("object already modified")),
			expectedVS:    vs,
			expectedError: k8sErrors.NewConflict(schema.GroupResource{}, "", fmt.Errorf("object already modified")),
		},
		{
			name: "Given valid params " +
				"When the passed VS exists" +
				"Then the func should update the vs and return no errors",
			newVS:         vsThatShouldBeUpdated,
			existingVS:    vsThatShouldBeUpdated,
			err:           k8sErrors.NewConflict(schema.GroupResource{}, "", fmt.Errorf("object already modified")),
			expectedVS:    vsThatShouldBeUpdated,
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actualError := retryUpdatingVS(ctxLogger, ctx, tc.newVS, tc.existingVS, common.GetSyncNamespace(), rc, tc.err, "Update")

			if tc.expectedError != nil {
				assert.NotNil(t, actualError)
				assert.Equal(t, tc.expectedError.Error(), actualError.Error())
			} else {
				assert.Nil(t, actualError)
				actualVS, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(common.GetSyncNamespace()).Get(ctx, tc.existingVS.Name, metaV1.GetOptions{})
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedVS.Labels, actualVS.Labels)
				assert.Equal(t, tc.expectedVS.Annotations, actualVS.Annotations)
				assert.Equal(t, tc.expectedVS.Spec.Hosts, actualVS.Spec.Hosts)
			}

		})
	}
}

func TestAddUpdateVirtualService(t *testing.T) {
	var (
		ctxLogger = log.WithFields(log.Fields{
			"type": "VirtualService",
		})
		ctx = context.Background()

		namespace = "testns"
		fooVS     = &apiNetworkingV1Alpha3.VirtualService{
			ObjectMeta: metaV1.ObjectMeta{
				Name: "stage.test00.foo-vs",
			},
			Spec: networkingV1Alpha3.VirtualService{
				Hosts: []string{"stage.test00.foo", "stage.test00.bar"},
			},
		}
		istioClientWithExistingVS = istioFake.NewSimpleClientset()
	)
	istioClientWithExistingVS.NetworkingV1alpha3().VirtualServices(namespace).Create(ctx, fooVS, metaV1.CreateOptions{})
	rc := &RemoteController{
		ClusterID: "cluster-1",
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: istioClientWithExistingVS,
		},
	}
	admiralParams := common.AdmiralParams{
		LabelSet:              &common.LabelSet{},
		SyncNamespace:         "test-sync-ns",
		EnableSWAwareNSCaches: true,
	}
	rr := NewRemoteRegistry(ctx, admiralParams)

	cases := []struct {
		name       string
		newVS      *apiNetworkingV1Alpha3.VirtualService
		existingVS *apiNetworkingV1Alpha3.VirtualService
		expErr     error
	}{
		{
			name: "Given virtual service does not exist, " +
				"And the existing object obtained from Get is nil, " +
				"When another thread create the virtualservice, " +
				"When this thread attempts to create virtualservice and fails, " +
				"Then, then an Update operation should be run, " +
				"And there should be no panic," +
				"And no errors should be returned",
			newVS:      fooVS,
			existingVS: nil,
			expErr:     nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			addUpdateVirtualService(ctxLogger, ctx, c.newVS, c.existingVS, namespace, rc, rr)
		})
	}
}
