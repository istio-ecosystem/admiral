package clusters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/client-go/rest"
)

func TestClientDiscoveryHandler_Added(t *testing.T) {
	//Struct of test case info. Name is required.
	const (
		namespace           = "namespace"
		namespaceNoDeps     = "namespace-no-deps"
		namespaceForRollout = "rollout-namespace"
		cluster1            = "cluster1"
		cluster2            = "cluster2"
		identity1           = "identity1"
		identity2           = "identity2"
		identity4           = "identity4"
	)
	var (
		stop = make(chan struct{})
	)

	type args struct {
		obj              *common.K8sObject
		options          []Option
		rollout          *argo.Rollout
		dep              *v1alpha1.Dependency
		readOnly         bool
		disableCache     bool
		admiralReadyOnly bool
		depCtrlAddErr    error
	}

	type want struct {
		identityClusterNSCacheLen int
		partitionIdentityCacheLen int
		err                       error
		depCtrlAddCalled          int
	}

	testCases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "No global identifier on obj results in no errors",
			args: args{
				obj: &common.K8sObject{Annotations: map[string]string{}, Labels: map[string]string{}},
			},
			want: want{},
		},
		{
			name: "Admiral Cache is not initialized returns error",
			args: args{
				obj: &common.K8sObject{
					Namespace:   namespaceForRollout,
					Name:        "obj-rollout-namespace",
					Annotations: map[string]string{"identity": identity1},
					Labels: map[string]string{
						"admiral.io/identityPartition": "test",
					},
				},
				disableCache: true,
			},
			want: want{
				err: errors.New("op=Add type=Job name=obj-rollout-namespace cluster=cluster1 error=task=Add name=obj-rollout-namespace namespace=rollout-namespace cluster=cluster1 message=processing skipped as Admiral cache is not initialized for identity=identity1"),
			},
		},
		{
			name: "Admiral in Read Only mode updates SW aware caches",
			args: args{
				obj: &common.K8sObject{
					Namespace:   namespaceForRollout,
					Name:        "obj-rollout-namespace",
					Annotations: map[string]string{"identity": identity1},
					Labels: map[string]string{
						"admiral.io/identityPartition": "test",
					},
				},
				options:  []Option{argoEnabled(true), swAwareNsCache(true)},
				readOnly: true,
			},
			want: want{
				identityClusterNSCacheLen: 1,
				partitionIdentityCacheLen: 1,
			},
		},
		{
			name: "Admiral during cache warmup returns early",
			args: args{
				obj: &common.K8sObject{
					Namespace:   namespaceForRollout,
					Name:        "obj-rollout-namespace",
					Annotations: map[string]string{"identity": identity1},
					Labels: map[string]string{
						"admiral.io/identityPartition": "test",
					},
				},
				options: []Option{
					argoEnabled(true),
					swAwareNsCache(true),
					cacheWarmupTime(1 * time.Minute),
				},
			},
			want: want{
				identityClusterNSCacheLen: 1,
				partitionIdentityCacheLen: 1,
			},
		},
		{
			name: "Rollout with same identifier present in the namespace should " +
				"return without processing and no errors",
			args: args{
				obj: &common.K8sObject{
					Namespace:   namespaceForRollout,
					Name:        "obj-rollout-namespace",
					Annotations: map[string]string{"identity": identity1},
					Labels: map[string]string{
						"admiral.io/identityPartition": "test",
					},
				},
				rollout: &argo.Rollout{
					Spec: argo.RolloutSpec{
						Selector: &metaV1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "test",
							},
						},
						Strategy: argo.RolloutStrategy{
							BlueGreen: &argo.BlueGreenStrategy{},
						},
						Template: coreV1.PodTemplateSpec{
							ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{
								"sidecar.istio.io/inject": "true",
								"identity":                identity1,
							}, Labels: map[string]string{
								"env": "stage",
							}},
						},
					},
					ObjectMeta: metaV1.ObjectMeta{
						Namespace: namespaceForRollout,
					},
				},
				options: []Option{argoEnabled(true), swAwareNsCache(true), dependencyProcessing(true)},
			},
			want: want{
				identityClusterNSCacheLen: 1,
				partitionIdentityCacheLen: 1,
			},
		},
		{
			name: "Valid client with no dependency record returns without processing",
			args: args{
				obj: &common.K8sObject{
					Namespace:   namespaceNoDeps,
					Name:        "obj-no-deps",
					Annotations: map[string]string{"identity": identity4},
					Labels: map[string]string{
						"admiral.io/identityPartition": "test",
					},
				},
				options: []Option{
					argoEnabled(true),
					swAwareNsCache(true)},
			},
			want: want{
				identityClusterNSCacheLen: 1,
				partitionIdentityCacheLen: 1,
			},
		},
		{
			name: "Valid client with valid dependency record results in calling depHandler Added()",
			args: args{
				obj: &common.K8sObject{
					Namespace:   namespace,
					Name:        "obj",
					Annotations: map[string]string{"identity": identity1},
					Labels: map[string]string{
						"admiral.io/identityPartition": "test",
					},
				},
				options: []Option{
					argoEnabled(true),
				},
				dep: &v1alpha1.Dependency{
					Spec: model.Dependency{
						Source:       identity1,
						Destinations: []string{identity2},
					},
				},
			},
			want: want{
				depCtrlAddCalled: 1,
			},
		},
		{
			name: "Valid client with valid dependency record with error in calling depHandler Added() returns error",
			args: args{
				obj: &common.K8sObject{
					Namespace:   namespace,
					Name:        "obj",
					Annotations: map[string]string{"identity": identity1},
					Labels: map[string]string{
						"admiral.io/identityPartition": "test",
					},
				},
				options: []Option{
					argoEnabled(true),
				},
				dep: &v1alpha1.Dependency{
					Spec: model.Dependency{
						Source:       identity1,
						Destinations: []string{identity2},
					},
				},
				depCtrlAddErr: errors.New("oops"),
			},
			want: want{
				depCtrlAddCalled: 1,
				err:              errors.New("op=Add type=Job name=obj cluster=cluster1 error=op=Process type= name=obj namespace=namespace cluster=cluster1 message=Error processing client discovery"),
			},
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {

			// Given
			ctx := context.Background()
			InitAdmiralForTesting(c.args.options...)

			mockDepCtrl := test.NewMockDependencyHandler(c.args.depCtrlAddErr, nil, nil)
			depController, err := admiral.NewDependencyController(stop, mockDepCtrl, loader.FakeKubeconfigPath, "", time.Second*time.Duration(300), loader.GetFakeClientLoader())
			if err != nil {
				t.Fatalf("failed to initialize dependency controller, err: %v", err)
			}
			config := rest.Config{
				Host: "localhost",
			}
			r, err := admiral.NewRolloutsController(stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
			if err != nil {
				t.Fatalf("failed to initialize rollout controller, err: %v", err)
			}
			if c.args.rollout != nil {
				entry := &admiral.RolloutClusterEntry{
					Identity: "test." + identity1,
					Rollouts: map[string]*admiral.RolloutItem{identity1: {Rollout: c.args.rollout}},
				}
				r.Cache.Put(entry)
				err = r.Added(ctx, c.args.rollout)
				assert.NoError(t, err)
			}
			if c.args.dep != nil {
				depController.Cache.Put(c.args.dep)
			}
			d, err := admiral.NewDeploymentController(stop, &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
			if err != nil {
				t.Fatalf("failed to initialize rollout controller, err: %v", err)
			}

			rcCluster1 := &RemoteController{
				RolloutController:    r,
				DeploymentController: d,
			}

			rcCluster2 := &RemoteController{
				RolloutController:    r,
				DeploymentController: d,
			}

			rr := NewRemoteRegistry(context.TODO(), common.AdmiralParams{})
			rr.DependencyController = depController
			rr.remoteControllers[cluster1] = rcCluster1
			rr.remoteControllers[cluster2] = rcCluster2

			rr.AdmiralCache = &AdmiralCache{
				IdentityClusterCache:          common.NewMapOfMaps(),
				IdentityClusterNamespaceCache: common.NewMapOfMapOfMaps(),
				PartitionIdentityCache:        common.NewMap(),
			}

			commonUtil.CurrentAdmiralState.ReadOnly = c.args.readOnly
			if c.args.disableCache {
				rr.AdmiralCache = nil
			}

			h := &ClientDiscoveryHandler{ClusterID: cluster1, RemoteRegistry: rr}
			// When
			result := h.Added(ctx, c.args.obj)

			// Then
			if c.want.err != nil && assert.Error(t, result) {
				// error validation
				assert.Equal(t, result, c.want.err)
			} else {
				assert.Equal(t, c.want.partitionIdentityCacheLen, rr.AdmiralCache.PartitionIdentityCache.Len())
				assert.Equal(t, c.want.identityClusterNSCacheLen, rr.AdmiralCache.IdentityClusterNamespaceCache.Len())
				if c.args.rollout != nil {
					rolloutFromCache := r.Cache.Get(identity1, "stage")
					assert.NotNil(t, rolloutFromCache)
				}
			}

			if c.want.depCtrlAddCalled > 0 {
				assert.Equal(t, c.want.depCtrlAddCalled, mockDepCtrl.AddedCnt)
			}
		})
	}
}

func TestCallRegistryForClientDiscovery(t *testing.T) {
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
		Profile:                    common.AdmiralProfileDefault,
		AdmiralStateSyncerMode:     true,
		AdmiralStateSyncerClusters: []string{"test-k8s"},
	}
	common.ResetSync()
	common.InitializeConfig(p)
	remoteRegistry, _ := InitAdmiral(context.Background(), p)
	dummyRespBody := ioutil.NopCloser(bytes.NewBufferString("dummyRespBody"))
	validRegistryClient := registry.NewDefaultRegistryClient()
	validClient := test.MockClient{
		ExpectedPutResponse: &http.Response{
			StatusCode: 200,
			Body:       dummyRespBody,
		},
		ExpectedPutErr: nil,
		ExpectedConfig: &commonUtil.Config{Host: "host", BaseURI: "v1"},
	}
	validRegistryClient.Client = &validClient
	invalidRegistryClient := registry.NewDefaultRegistryClient()
	invalidClient := test.MockClient{
		ExpectedDeleteResponse: &http.Response{
			StatusCode: 404,
			Body:       dummyRespBody,
		},
		ExpectedDeleteErr: fmt.Errorf("failed private auth call"),
		ExpectedConfig:    &commonUtil.Config{Host: "host", BaseURI: "v1"},
	}
	invalidRegistryClient.Client = &invalidClient
	k8sObj := &common.K8sObject{
		Namespace: "testns",
		Name:      "obj",
		Type:      "testtype",
		Labels: map[string]string{
			"admiral.io/identityPartition": "test",
			"admiral.io/env":               "testEnv",
			"identity":                     "testId",
		},
	}

	testCases := []struct {
		name           string
		ctx            context.Context
		obj            *common.K8sObject
		registryClient *registry.RegistryClient
		event          admiral.EventType
		expectedError  error
	}{
		{
			name: "Given valid registry client " +
				"When calling for add event " +
				"Then error should be nil",
			obj:            k8sObj,
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: validRegistryClient,
			event:          admiral.Add,
			expectedError:  nil,
		},
		{
			name: "Given valid registry client " +
				"When calling for update event " +
				"Then error should be nil",
			obj:            k8sObj,
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: validRegistryClient,
			event:          admiral.Update,
			expectedError:  nil,
		},
		{
			name: "Given valid params to call registry func " +
				"When registry func returns an error " +
				"Then handler should receive an error",
			obj:            k8sObj,
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: invalidRegistryClient,
			event:          admiral.Delete,
			expectedError:  fmt.Errorf("op=Delete type=testtype name=obj cluster=test-k8s message=failed to Delete testtype with err: failed private auth call"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			remoteRegistry.RegistryClient = tc.registryClient
			clusterName := "test-k8s"
			actualError := callRegistryForClientDiscovery(tc.ctx, tc.event, remoteRegistry, "test.testId", clusterName, tc.obj)
			if tc.expectedError != nil {
				if actualError == nil {
					t.Fatalf("expected error %s but got nil", tc.expectedError.Error())
				}
				assert.Equal(t, tc.expectedError.Error(), actualError.Error())
			} else {
				if actualError != nil {
					t.Fatalf("expected error nil but got %s", actualError.Error())
				}
			}
		})
	}
}
