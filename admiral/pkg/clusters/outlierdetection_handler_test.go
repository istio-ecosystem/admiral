package clusters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	networkingAlpha3 "istio.io/api/networking/v1alpha3"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandleEventForOutlierDetection(t *testing.T) {
	ctx := context.Background()

	admiralParamTest := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			AdmiralCRDIdentityLabel: "assetAlias",
		},
		Profile: common.AdmiralProfileDefault,
	}

	common.ResetSync()
	registryTest, _ := InitAdmiral(ctx, admiralParamTest)

	type args struct {
		event       admiral.EventType
		od          *v1.OutlierDetection
		clusterName string
		modifySE    ModifySEFunc
	}

	odConfig := model.OutlierConfig{
		BaseEjectionTime:         0,
		ConsecutiveGatewayErrors: 0,
		Interval:                 0,
		XXX_NoUnkeyedLiteral:     struct{}{},
		XXX_unrecognized:         nil,
		XXX_sizecache:            0,
	}

	od := v1.OutlierDetection{
		TypeMeta:   metaV1.TypeMeta{},
		ObjectMeta: metaV1.ObjectMeta{},
		Spec: model.OutlierDetection{
			OutlierConfig:        &odConfig,
			Selector:             map[string]string{"identity": "payments", "env": "e2e"},
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_unrecognized:     nil,
			XXX_sizecache:        0,
		},
		Status: v1.OutlierDetectionStatus{},
	}

	seFunc := func(ctx context.Context, event admiral.EventType, env string, sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*networkingAlpha3.ServiceEntry, error) {
		return nil, nil
	}

	testArg1 := args{
		event: admiral.Add,
		od: &v1.OutlierDetection{
			Spec:       od.Spec,
			ObjectMeta: metaV1.ObjectMeta{Name: "od1", Namespace: "ns1", Labels: map[string]string{"assetAlias": "Intuit.devx.supercar", "identity": "id", "admiral.io/env": "stage"}},
			TypeMeta: metaV1.TypeMeta{
				Kind:       "admiral.io/v1",
				APIVersion: common.OutlierDetection,
			},
		},
		clusterName: "test",
		modifySE:    seFunc,
	}

	testArg2 := args{
		event: admiral.Add,
		od: &v1.OutlierDetection{
			Spec:       od.Spec,
			ObjectMeta: metaV1.ObjectMeta{Name: "od1", Namespace: "ns1", Labels: map[string]string{"foo": "bar"}},
			TypeMeta: metaV1.TypeMeta{
				Kind:       "admiral.io/v1",
				APIVersion: common.OutlierDetection,
			},
		},
		clusterName: "test",
		modifySE:    seFunc,
	}

	errors.New("foo")

	tests := []struct {
		name   string
		args   args
		expErr error
	}{
		{"identity label missing", testArg2, errors.New("Skipped as label assetAlias was not found")},
		{"happy path", testArg1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := HandleEventForOutlierDetection(ctx, tt.args.event, tt.args.od, registryTest, tt.args.clusterName, tt.args.modifySE)
			if tt.expErr != nil {
				assert.Contains(t, err.Error(), tt.expErr.Error())
			} else {
				assert.Nil(t, err, "Not expecting error")
			}
		})
	}
}

func TestOutlierDetectionHandler_Added(t *testing.T) {
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
			PriorityKey:             "priority",
		},
		Profile:                    common.AdmiralProfileDefault,
		AdmiralStateSyncerMode:     true,
		AdmiralStateSyncerClusters: []string{"test-k8s"},
	}
	common.ResetSync()
	common.InitializeConfig(p)
	remoteRegistry, _ := InitAdmiral(context.Background(), p)
	validRegistryClient, invalidRegistryClient := getBasicRegistryClients()
	od := getBasicOd()

	testCases := []struct {
		name             string
		ctx              context.Context
		outlierDetection *v1.OutlierDetection
		registryClient   *registry.RegistryClient
		expectedError    error
	}{
		{
			name: "Given valid params to Added func " +
				"When no func returns an error " +
				"Then the func proceed to modifySe",
			outlierDetection: od,
			ctx:              context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient:   validRegistryClient,
			expectedError:    fmt.Errorf("op=Add type=OutlierDetection name= cluster=test-k8s error=skipped processing as cname is empty"),
		},
		{
			name: "Given valid params to Added func " +
				"When registryClient func returns an error " +
				"Then the func should proceed to modifySe and return an error",
			outlierDetection: od,
			ctx:              context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient:   invalidRegistryClient,
			expectedError:    fmt.Errorf("op=Add type=OutlierDetection name= cluster=test-k8s error=skipped processing as cname is empty"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			remoteRegistry.RegistryClient = tc.registryClient
			odHandler := OutlierDetectionHandler{
				RemoteRegistry: remoteRegistry,
				ClusterID:      "test-k8s",
			}
			actualError := odHandler.Added(tc.ctx, tc.outlierDetection)
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

func TestOutlierDetectionHandler_Updated(t *testing.T) {
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
			PriorityKey:             "priority",
		},
		Profile:                    common.AdmiralProfileDefault,
		AdmiralStateSyncerMode:     true,
		AdmiralStateSyncerClusters: []string{"test-k8s"},
	}
	common.ResetSync()
	common.InitializeConfig(p)
	remoteRegistry, _ := InitAdmiral(context.Background(), p)
	validRegistryClient, invalidRegistryClient := getBasicRegistryClients()
	od := getBasicOd()

	testCases := []struct {
		name             string
		ctx              context.Context
		outlierDetection *v1.OutlierDetection
		registryClient   *registry.RegistryClient
		expectedError    error
	}{
		{
			name: "Given valid params to Update func " +
				"When no func returns an error " +
				"Then the func proceed to modifySe",
			outlierDetection: od,
			ctx:              context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient:   validRegistryClient,
			expectedError:    fmt.Errorf("op=Update type=OutlierDetection name= cluster=test-k8s error=skipped processing as cname is empty"),
		},
		{
			name: "Given valid params to Update func " +
				"When registryClient func returns an error " +
				"Then the func should proceed to modifySe and return an error",
			outlierDetection: od,
			ctx:              context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient:   invalidRegistryClient,
			expectedError:    fmt.Errorf("op=Update type=OutlierDetection name= cluster=test-k8s error=skipped processing as cname is empty"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			remoteRegistry.RegistryClient = tc.registryClient
			odHandler := OutlierDetectionHandler{
				RemoteRegistry: remoteRegistry,
				ClusterID:      "test-k8s",
			}
			actualError := odHandler.Updated(tc.ctx, tc.outlierDetection)
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

func TestOutlierDetectionHandler_Deleted(t *testing.T) {
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
			PriorityKey:             "priority",
		},
		Profile:                    common.AdmiralProfileDefault,
		AdmiralStateSyncerMode:     true,
		AdmiralStateSyncerClusters: []string{"test-k8s"},
	}
	common.ResetSync()
	common.InitializeConfig(p)
	remoteRegistry, _ := InitAdmiral(context.Background(), p)
	validRegistryClient, invalidRegistryClient := getBasicRegistryClients()
	od := getBasicOd()

	testCases := []struct {
		name             string
		ctx              context.Context
		outlierDetection *v1.OutlierDetection
		registryClient   *registry.RegistryClient
		expectedError    error
	}{
		{
			name: "Given valid params to Delete func " +
				"When no func returns an error " +
				"Then the func proceed to modifySe",
			outlierDetection: od,
			ctx:              context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient:   validRegistryClient,
			expectedError:    fmt.Errorf("op=Delete type=OutlierDetection name= cluster=test-k8s error=skipped processing as cname is empty"),
		},
		{
			name: "Given valid params to Delete func " +
				"When registryClient func returns an error " +
				"Then the func should proceed to modifySe and return an error",
			outlierDetection: od,
			ctx:              context.WithValue(context.Background(), "txId", "txidvalue"),
			registryClient:   invalidRegistryClient,
			expectedError:    fmt.Errorf("op=Delete type=OutlierDetection name= cluster=test-k8s error=skipped processing as cname is empty"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			remoteRegistry.RegistryClient = tc.registryClient
			odHandler := OutlierDetectionHandler{
				RemoteRegistry: remoteRegistry,
				ClusterID:      "test-k8s",
			}
			actualError := odHandler.Deleted(tc.ctx, tc.outlierDetection)
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

func getBasicRegistryClients() (*registry.RegistryClient, *registry.RegistryClient) {
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
	return validRegistryClient, invalidRegistryClient
}

func getBasicOd() *v1.OutlierDetection {
	odConfig := model.OutlierConfig{
		BaseEjectionTime:         0,
		ConsecutiveGatewayErrors: 0,
		Interval:                 0,
		XXX_NoUnkeyedLiteral:     struct{}{},
		XXX_unrecognized:         nil,
		XXX_sizecache:            0,
	}

	od := &v1.OutlierDetection{
		TypeMeta:   metaV1.TypeMeta{},
		ObjectMeta: metaV1.ObjectMeta{},
		Spec: model.OutlierDetection{
			OutlierConfig:        &odConfig,
			Selector:             map[string]string{"identity": "payments", "env": "e2e"},
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_unrecognized:     nil,
			XXX_sizecache:        0,
		},
		Status: v1.OutlierDetectionStatus{},
	}
	return od
}
