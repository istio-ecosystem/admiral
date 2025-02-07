package clusters

import (
	"bytes"
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	networkingAlpha3 "istio.io/api/networking/v1alpha3"
	apiMachineryMetaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func setupForGlobalTrafficHandlerTests() {
	typeTestSingleton.Do(func() {
		common.ResetSync()
		common.InitializeConfig(admiralParamsForTypesTests())
	})
}

func TestHandleEventForGlobalTrafficPolicy(t *testing.T) {
	setupForGlobalTrafficHandlerTests()
	ctx := context.Background()
	event := admiral.EventType("Add")
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}
	remoteRegistry, _ := InitAdmiral(context.Background(), p)

	seFunc := func(ctx context.Context, event admiral.EventType, env string, sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*networkingAlpha3.ServiceEntry, error) {
		return nil, nil
	}

	seErrFunc := func(ctx context.Context, event admiral.EventType, env string, sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*networkingAlpha3.ServiceEntry, error) {
		return nil, fmt.Errorf("Error")
	}
	cases := []struct {
		name      string
		gtp       *v1.GlobalTrafficPolicy
		seFunc    ModifySEFunc
		doesError bool
	}{
		{
			name: "missing identity label in GTP should result in error being returned by the handler",
			gtp: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			seFunc:    seFunc,
			doesError: true,
		},
		{
			name: "empty identity label in GTP should result in error being returned by the handler",
			gtp: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": ""},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			seFunc:    seFunc,
			doesError: true,
		},
		{
			name: "valid GTP config which is expected to pass",
			gtp: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": "testapp"},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			seFunc:    seFunc,
			doesError: false,
		},
		{
			name: "Given a valid GTP config, " +
				"And modifyServiceEntryForNewServiceOrPod returns an error" +
				"Then, the function would return an error",
			gtp: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": "testapp"},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			seFunc:    seErrFunc,
			doesError: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := HandleEventForGlobalTrafficPolicy(ctx, event, c.gtp, remoteRegistry, "testcluster", c.seFunc)
			assert.Equal(t, err != nil, c.doesError)
		})
	}
}

func TestGlobalTrafficHandler_Added(t *testing.T) {
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
			PriorityKey:             "priority",
		},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheReconcileDuration:     10 * time.Second,
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
		ExpectedConfig: &util.Config{Host: "host", BaseURI: "v1"},
	}
	validRegistryClient.Client = &validClient
	invalidRegistryClient := registry.NewDefaultRegistryClient()
	invalidClient := test.MockClient{
		ExpectedPutResponse: &http.Response{
			StatusCode: 404,
			Body:       dummyRespBody,
		},
		ExpectedPutErr: fmt.Errorf("failed private auth call"),
		ExpectedConfig: &util.Config{Host: "host", BaseURI: "v1"},
	}
	invalidRegistryClient.Client = &invalidClient

	testCases := []struct {
		name                string
		ctx                 context.Context
		globalTrafficPolicy *v1.GlobalTrafficPolicy
		registryClient      *registry.RegistryClient
		expectedError       error
	}{
		{
			name: "Given valid params to Added func " +
				"When no func returns an error " +
				"Then the func proceed to modifySe",
			globalTrafficPolicy: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": "testapp"},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: validRegistryClient,
			expectedError:  fmt.Errorf("op=Added type=globaltrafficpolicy name=testgtp cluster=test-k8s error=task=Update name=testenv namespace=testapp cluster= message=processing skipped during cache warm up state for env=testenv identity=testapp"),
		},
		{
			name: "Given valid params to Added func " +
				"When handleEvent func returns an error " +
				"Then the func should return an error",
			globalTrafficPolicy: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: validRegistryClient,
			expectedError:  fmt.Errorf("op=Added type=globaltrafficpolicy name=testgtp cluster=test-k8s error=op=Event type=globaltrafficpolicy name=testgtp cluster=test-k8s message=Skipped as 'identity was not found', namespace="),
		},
		{
			name: "Given valid params to Added func " +
				"When registry func returns an error " +
				"Then the func should proceed",
			globalTrafficPolicy: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": "testapp"},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: invalidRegistryClient,
			expectedError:  fmt.Errorf("op=Added type=globaltrafficpolicy name=testgtp cluster=test-k8s error=task=Update name=testenv namespace=testapp cluster= message=processing skipped during cache warm up state for env=testenv identity=testapp"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			remoteRegistry.RegistryClient = tc.registryClient
			gtpHandler := GlobalTrafficHandler{
				RemoteRegistry: remoteRegistry,
				ClusterID:      "test-k8s",
			}
			actualError := gtpHandler.Added(tc.ctx, tc.globalTrafficPolicy)
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

func TestGlobalTrafficHandler_Updated(t *testing.T) {
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
			PriorityKey:             "priority",
		},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheReconcileDuration:     10 * time.Second,
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
		ExpectedConfig: &util.Config{Host: "host", BaseURI: "v1"},
	}
	validRegistryClient.Client = &validClient
	invalidRegistryClient := registry.NewDefaultRegistryClient()
	invalidClient := test.MockClient{
		ExpectedPutResponse: &http.Response{
			StatusCode: 404,
			Body:       dummyRespBody,
		},
		ExpectedPutErr: fmt.Errorf("failed private auth call"),
		ExpectedConfig: &util.Config{Host: "host", BaseURI: "v1"},
	}
	invalidRegistryClient.Client = &invalidClient

	testCases := []struct {
		name                string
		ctx                 context.Context
		globalTrafficPolicy *v1.GlobalTrafficPolicy
		registryClient      *registry.RegistryClient
		expectedError       error
	}{
		{
			name: "Given valid params to Updated func " +
				"When no func returns an error " +
				"Then the func proceed to modifySe",
			globalTrafficPolicy: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": "testapp"},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: validRegistryClient,
			expectedError:  fmt.Errorf("op=Updated type=globaltrafficpolicy name=testgtp cluster=test-k8s error=task=Update name=testenv namespace=testapp cluster= message=processing skipped during cache warm up state for env=testenv identity=testapp"),
		},
		{
			name: "Given valid params to Updated func " +
				"When handleEvent func returns an error " +
				"Then the func should return an error",
			globalTrafficPolicy: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: validRegistryClient,
			expectedError:  fmt.Errorf("op=Updated type=globaltrafficpolicy name=testgtp cluster=test-k8s error=op=Event type=globaltrafficpolicy name=testgtp cluster=test-k8s message=Skipped as 'identity was not found', namespace="),
		},
		{
			name: "Given valid params to Updated func " +
				"When registry func returns an error " +
				"Then the func should proceed",
			globalTrafficPolicy: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": "testapp"},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: invalidRegistryClient,
			expectedError:  fmt.Errorf("op=Updated type=globaltrafficpolicy name=testgtp cluster=test-k8s error=task=Update name=testenv namespace=testapp cluster= message=processing skipped during cache warm up state for env=testenv identity=testapp"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			remoteRegistry.RegistryClient = tc.registryClient
			gtpHandler := GlobalTrafficHandler{
				RemoteRegistry: remoteRegistry,
				ClusterID:      "test-k8s",
			}
			actualError := gtpHandler.Updated(tc.ctx, tc.globalTrafficPolicy)
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

func TestGlobalTrafficHandler_Deleted(t *testing.T) {
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
			PriorityKey:             "priority",
		},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheReconcileDuration:     10 * time.Second,
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
		ExpectedDeleteResponse: &http.Response{
			StatusCode: 200,
			Body:       dummyRespBody,
		},
		ExpectedDeleteErr: nil,
		ExpectedConfig:    &util.Config{Host: "host", BaseURI: "v1"},
	}
	validRegistryClient.Client = &validClient
	invalidRegistryClient := registry.NewDefaultRegistryClient()
	invalidClient := test.MockClient{
		ExpectedDeleteResponse: &http.Response{
			StatusCode: 404,
			Body:       dummyRespBody,
		},
		ExpectedDeleteErr: fmt.Errorf("failed private auth call"),
		ExpectedConfig:    &util.Config{Host: "host", BaseURI: "v1"},
	}
	invalidRegistryClient.Client = &invalidClient

	testCases := []struct {
		name                string
		ctx                 context.Context
		globalTrafficPolicy *v1.GlobalTrafficPolicy
		registryClient      *registry.RegistryClient
		expectedError       error
	}{
		{
			name: "Given valid params to Deleted func " +
				"When no func returns an error " +
				"Then the func proceed to modifySe",
			globalTrafficPolicy: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": "testapp"},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: validRegistryClient,
			expectedError:  fmt.Errorf("op=Deleted type=globaltrafficpolicy name=testgtp cluster=test-k8s error=task=Update name=testenv namespace=testapp cluster= message=processing skipped during cache warm up state for env=testenv identity=testapp"),
		},
		{
			name: "Given valid params to Updated func " +
				"When handleEvent func returns an error " +
				"Then the func should return an error",
			globalTrafficPolicy: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: validRegistryClient,
			expectedError:  fmt.Errorf("op=Deleted type=globaltrafficpolicy name=testgtp cluster=test-k8s error=op=Event type=globaltrafficpolicy name=testgtp cluster=test-k8s message=Skipped as 'identity was not found', namespace="),
		},
		{
			name: "Given valid params to Updated func " +
				"When registry func returns an error " +
				"Then the func should proceed",
			globalTrafficPolicy: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": "testapp"},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			ctx:            context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient: invalidRegistryClient,
			expectedError:  fmt.Errorf("op=Deleted type=globaltrafficpolicy name=testgtp cluster=test-k8s error=task=Update name=testenv namespace=testapp cluster= message=processing skipped during cache warm up state for env=testenv identity=testapp"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			remoteRegistry.RegistryClient = tc.registryClient
			gtpHandler := GlobalTrafficHandler{
				RemoteRegistry: remoteRegistry,
				ClusterID:      "test-k8s",
			}
			actualError := gtpHandler.Deleted(tc.ctx, tc.globalTrafficPolicy)
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
