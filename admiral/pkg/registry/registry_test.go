package registry

import (
	"bytes"
	"context"
	json "encoding/json"
	"errors"
	"fmt"
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	numa "github.com/numaproj/numaflow/pkg/apis/numaflow/v1alpha1"
	"io/ioutil"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	apiNetworkingV1Alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	appsV1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	log "github.com/sirupsen/logrus"
)

type MockClient struct {
	expectedResponse       *http.Response
	expectedPostResponse   *http.Response
	expectedPutResponse    *http.Response
	expectedDeleteResponse *http.Response
	expectedGetErr         error
	expectedPutErr         error
	expectedPostErr        error
	expectedDeleteErr      error
	expectedConfig         *Config
	Body                   []byte
}

func (m *MockClient) MakePrivateAuthCall(url string, tid string, method string, body []byte) (*http.Response, error) {
	m.Body = body
	switch method {
	case "GET":
		return m.expectedResponse, m.expectedGetErr
	case "PUT":
		return m.expectedPutResponse, m.expectedPutErr
	case "POST":
		return m.expectedPostResponse, m.expectedPostErr
	case "DELETE":
		return m.expectedDeleteResponse, m.expectedDeleteErr
	}
	return m.expectedResponse, nil
}

func (m *MockClient) GetConfig() *Config {
	return m.expectedConfig
}

func admiralParamsForRegistryClientTests() common.AdmiralParams {
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
		AdmiralStateSyncerMode:              true,
		RegistryClientHost:                  "registry.com",
		RegistryClientAppId:                 "registry-appid",
		RegistryClientAppSecret:             "registry-appsecret",
		RegistryClientBaseURI:               "v1",
		AdmiralAppEnv:                       "preProd",
	}
}

func newDefaultRegistryClient() *registryClient {
	registryClientParams := common.GetRegistryClientConfig()
	defaultRegistryClientConfig := &Config{
		Host:      registryClientParams["Host"],
		AppId:     registryClientParams["AppId"],
		AppSecret: registryClientParams["AppSecret"],
		BaseURI:   registryClientParams["BaseURI"],
	}
	return NewRegistryClient(WithBaseClientConfig(defaultRegistryClientConfig))
}

func TestParseIdentityConfigJSON(t *testing.T) {
	identityConfig := GetSampleIdentityConfig("sample")
	testCases := []struct {
		name           string
		identityConfig IdentityConfig
	}{
		{
			name: "Given a JSON identity configuration file, " +
				"When the file is parsed, " +
				"Then the file should be read into the IdentityConfig struct",
			identityConfig: identityConfig,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			jsonResult, err := json.MarshalIndent(c.identityConfig, "", "    ")
			if err != nil {
				t.Errorf("While marshaling IdentityConfig struct into JSON, got error: %s", err)
			}
			var identityConfigUnmarshalResult IdentityConfig
			err = json.Unmarshal(jsonResult, &identityConfigUnmarshalResult)
			if err != nil {
				t.Errorf("While unmarshaling JSON into IdentityConfig struct, got error: %s", err)
			}
			if !reflect.DeepEqual(identityConfigUnmarshalResult, c.identityConfig) {
				t.Errorf("Mismatch between original IdentityConfig and unmarshaled IdentityConfig")
			}
		})
	}
}

func TestIdentityConfigGetByIdentityName(t *testing.T) {
	sampleIdentityConfig := GetSampleIdentityConfig("sample")
	rc := NewRegistryClient()
	var jsonErr *json.SyntaxError
	ctxLogger := log.WithContext(context.Background())
	testCases := []struct {
		name                   string
		expectedIdentityConfig IdentityConfig
		expectedError          any
		identityAlias          string
	}{
		{
			name: "Given an identity, " +
				"When the identity config JSON is parsed, " +
				"Then the resulting struct should match the expected config",
			expectedIdentityConfig: sampleIdentityConfig,
			expectedError:          nil,
			identityAlias:          "sample",
		},
		{
			name: "Given an identity, " +
				"When the identity config JSON doesn't exist for it, " +
				"Then there should be a non-nil error",
			expectedIdentityConfig: IdentityConfig{},
			expectedError:          jsonErr,
			identityAlias:          "failed",
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			identityConfig, err := rc.GetIdentityConfigByIdentityName(c.identityAlias, ctxLogger)
			if err != nil && c.expectedError == nil {
				t.Errorf("error while getting identityConfig by name with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			} else {
				opts := getUnexportedProperties()
				if !cmp.Equal(identityConfig, c.expectedIdentityConfig, opts) {
					t.Errorf("want=%v, got=%v", c.expectedIdentityConfig, identityConfig)
					t.Errorf(cmp.Diff(identityConfig, c.expectedIdentityConfig, opts))
				}
			}
		})
	}
}

func TestGetIdentityConfigByClusterName(t *testing.T) {
	sampleIdentityConfig := GetSampleIdentityConfig("sample")
	rc := NewRegistryClient()
	var jsonErr *json.SyntaxError
	ctxLogger := log.WithContext(context.Background())
	testCases := []struct {
		name                   string
		expectedIdentityConfig IdentityConfig
		expectedError          any
		clusterName            string
	}{
		{
			name: "Given a cluster name, " +
				"When all the identity configs for the identities in that cluster are processed, " +
				"Then the structs returned should match the expected configs",
			expectedIdentityConfig: sampleIdentityConfig,
			expectedError:          nil,
			clusterName:            "sample",
		},
		{
			name: "Given a cluster name, " +
				"When there exists no identity config for that cluster, " +
				"Then there should be a non-nil error",
			expectedIdentityConfig: IdentityConfig{},
			expectedError:          jsonErr,
			clusterName:            "failed",
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			identityConfigs, err := rc.GetIdentityConfigByClusterName(c.clusterName, ctxLogger)
			if err != nil && c.expectedError == nil {
				t.Errorf("error while getting identityConfigs by cluster name with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			} else {
				opts := getUnexportedProperties()
				if !cmp.Equal(identityConfigs[0], c.expectedIdentityConfig, opts) {
					t.Errorf("mismatch between parsed JSON file and expected identity config for file: %s", c.clusterName)
					t.Errorf(cmp.Diff(identityConfigs[0], c.expectedIdentityConfig, opts))
				}
			}
		})
	}
}

func TestMarshalDataForRegistry(t *testing.T) {
	invalidData := map[string]interface{}{
		"name":  "name",
		"url":   "ingressURL",
		"label": strings.Join([]string{"test", "labels"}, ", "),
		"notes": make(chan int),
		"type":  "resourceType",
	}
	registryUrl := "registry.com"
	testCases := []struct {
		name          string
		expectedBody  []byte
		expectedError any
		data          map[string]interface{}
	}{
		{
			name: "Given a invalid data format to marshal, " +
				"Then the err returned should match the expected err",
			expectedBody:  nil,
			expectedError: fmt.Errorf("failed to marshal json body for http request to %s", registryUrl),
			data:          invalidData,
		},
		{
			name: "Given a empty data format to marshal, " +
				"Then the err returned should match the expected err",
			expectedBody:  nil,
			expectedError: fmt.Errorf("json body for request to %s was nil", registryUrl),
			data:          nil,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			body, err := marshalDataForRegistry(c.data, registryUrl)
			if err != nil && c.expectedError == nil {
				t.Errorf("error while marshaling data with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			} else {
				if !bytes.Equal(body, c.expectedBody) {
					t.Errorf("expected body was not same as returned body")
				}
			}
		})
	}
}

func TestMakeCallToRegistry(t *testing.T) {
	invalidData := map[string]interface{}{
		"name":  "name",
		"url":   "ingressURL",
		"label": strings.Join([]string{"test", "labels"}, ", "),
		"notes": make(chan int),
		"type":  "resourceType",
	}
	validData := map[string]interface{}{
		"name":  "name",
		"url":   "ingressURL",
		"label": strings.Join([]string{"test", "labels"}, ", "),
		"notes": "",
		"type":  "resourceType",
	}
	registryUrl := "registry.com"
	tid := "txId"
	method := "PUT"
	dummyRespBody := ioutil.NopCloser(bytes.NewBufferString("dummyRespBody"))
	privateAuthCallErr := fmt.Errorf("failed private auth call")
	baseClientPrivateAuthCallFailed := MockClient{
		expectedPutResponse: &http.Response{
			StatusCode: 404,
			Body:       dummyRespBody,
		},
		expectedPutErr: privateAuthCallErr,
		expectedConfig: &Config{Host: "host", BaseURI: "v1"},
	}
	baseClientResponseEmpty := MockClient{
		expectedPutResponse: nil,
		expectedPutErr:      nil,
		expectedConfig:      &Config{Host: "host", BaseURI: "v1"},
	}
	baseClientResponseCodeNot200 := MockClient{
		expectedPutResponse: &http.Response{
			StatusCode: 404,
			Body:       dummyRespBody,
		},
		expectedPutErr: nil,
		expectedConfig: &Config{Host: "host", BaseURI: "v1"},
	}
	validClient := MockClient{
		expectedPutResponse: &http.Response{
			StatusCode: 200,
			Body:       dummyRespBody,
		},
		expectedPutErr: nil,
		expectedConfig: &Config{Host: "host", BaseURI: "v1"},
	}
	testCases := []struct {
		name          string
		client        BaseClient
		expectedError any
		data          map[string]interface{}
	}{
		{
			name: "Given a invalid data format to marshal, " +
				"Then the err returned should match the expected err",
			expectedError: fmt.Errorf("failed to marshal json body for http request to %s", registryUrl),
			data:          invalidData,
			client:        &validClient,
		},
		{
			name: "Given a valid data format to marshal, " +
				"When the private auth call fails, " +
				"Then the err returned should match the expected err",
			expectedError: privateAuthCallErr,
			data:          validData,
			client:        &baseClientPrivateAuthCallFailed,
		},
		{
			name: "Given a valid data format to marshal, " +
				"When the response body is nil, " +
				"Then the err returned should match the expected err",
			expectedError: fmt.Errorf("response for request to %s was nil", registryUrl),
			data:          validData,
			client:        &baseClientResponseEmpty,
		},
		{
			name: "Given a valid data format to marshal, " +
				"When the response code is not 200, " +
				"Then the err returned should match the expected err",
			expectedError: fmt.Errorf("response code for request to %s was %v", registryUrl, 404),
			data:          validData,
			client:        &baseClientResponseCodeNot200,
		},
		{
			name: "Given a valid data format to marshal, " +
				"When the registry call succeeds, " +
				"Then no err should be returned",
			expectedError: nil,
			data:          validData,
			client:        &validClient,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := makeCallToRegistry(registryUrl, tid, method, c.data, c.client)
			if err != nil && c.expectedError == nil {
				t.Errorf("error while making call to registry with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			}
		})
	}
}

func TestDeleteClusterGateway(t *testing.T) {
	admiralParams := admiralParamsForRegistryClientTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	dummyRespBody := ioutil.NopCloser(bytes.NewBufferString("dummyRespBody"))
	validClient := MockClient{
		expectedDeleteResponse: &http.Response{
			StatusCode: 200,
			Body:       dummyRespBody,
		},
		expectedDeleteErr: nil,
		expectedConfig:    &Config{Host: "host", BaseURI: "v1"},
	}
	rc := newDefaultRegistryClient()
	rc.client = &validClient
	testCases := []struct {
		name          string
		expectedError any
	}{
		{
			name: "Given a valid DELETE request, " +
				"Then the registry call should succeed",
			expectedError: nil,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := rc.DeleteClusterGateway("clusterName", "gatewayName", "gateway-type", "tid")
			if err != nil && c.expectedError == nil {
				t.Errorf("error while making delete cluster gateway call with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			}
		})
	}
}

func TestPutClusterGateway(t *testing.T) {
	admiralParams := admiralParamsForRegistryClientTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	dummyRespBody := ioutil.NopCloser(bytes.NewBufferString("dummyRespBody"))
	validClient := MockClient{
		expectedPutResponse: &http.Response{
			StatusCode: 200,
			Body:       dummyRespBody,
		},
		expectedPutErr: nil,
		expectedConfig: &Config{Host: "host", BaseURI: "v1"},
	}
	rc := newDefaultRegistryClient()
	rc.client = &validClient
	testCases := []struct {
		name          string
		expectedError any
	}{
		{
			name: "Given a valid request body, " +
				"Then the registry call should succeed",
			expectedError: nil,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := rc.PutClusterGateway("clusterName", "gatewayName", "ingress.com", "", "gateway-type", "tid", nil)
			if err != nil && c.expectedError == nil {
				t.Errorf("error while making put cluster gateway call with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			}
		})
	}
}

func TestPutCustomData(t *testing.T) {
	admiralParams := admiralParamsForRegistryClientTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	dummyRespBody := ioutil.NopCloser(bytes.NewBufferString("dummyRespBody"))
	validClient := MockClient{
		expectedPutResponse: &http.Response{
			StatusCode: 200,
			Body:       dummyRespBody,
		},
		expectedPutErr: nil,
		expectedConfig: &Config{Host: "host", BaseURI: "v1"},
	}
	rc := newDefaultRegistryClient()
	rc.client = &validClient
	dummyVS := &apiNetworkingV1Alpha3.VirtualService{
		Spec: networkingV1Alpha3.VirtualService{
			Hosts: []string{"hostname"},
		},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "virtual-service-1",
			Namespace: "ns-1",
		},
	}
	dummyCCC := v1alpha1.ClientConnectionConfig{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "sampleCCC",
		},
		Spec: v1alpha1.ClientConnectionConfigSpec{
			ConnectionPool: model.ConnectionPool{Http: &model.ConnectionPool_HTTP{
				Http2MaxRequests:         1000,
				MaxRequestsPerConnection: 5,
			}},
			Tunnel: model.Tunnel{},
		},
	}
	dummyGTP := v1alpha1.GlobalTrafficPolicy{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "sampleGTP",
		},
		Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy{
				{
					LbType: 0,
					Target: []*model.TrafficGroup{
						{
							Region: "us-west-2",
							Weight: 50,
						},
						{
							Region: "us-east-2",
							Weight: 50,
						},
					},
					DnsPrefix: "testDnsPrefix",
					OutlierDetection: &model.TrafficPolicy_OutlierDetection{
						ConsecutiveGatewayErrors: 5,
						Interval:                 5,
					},
				},
			},
			Selector: nil,
		},
	}
	dummyOD := v1alpha1.OutlierDetection{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "sampleOD",
		},
		Spec: model.OutlierDetection{
			OutlierConfig: &model.OutlierConfig{
				ConsecutiveGatewayErrors: 10,
				Interval:                 10,
			},
			Selector: nil,
		},
	}
	testCases := []struct {
		name          string
		expectedError any
		resourceType  string
		value         interface{}
	}{
		{
			name: "Given a valid request body with VS, " +
				"Then the registry call should succeed",
			expectedError: nil,
			resourceType:  "vs",
			value:         dummyVS,
		},
		{
			name: "Given a valid request body with CCC, " +
				"Then the registry call should succeed",
			expectedError: nil,
			resourceType:  "cc",
			value:         dummyCCC,
		},
		{
			name: "Given a valid request body with GTP, " +
				"Then the registry call should succeed",
			expectedError: nil,
			resourceType:  "gtp",
			value:         dummyGTP,
		},
		{
			name: "Given a valid request body with OD, " +
				"Then the registry call should succeed",
			expectedError: nil,
			resourceType:  "od",
			value:         dummyOD,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := rc.PutCustomData("clusterName", "namespace", "customdata-name", c.resourceType, "tid", c.value)
			if err != nil && c.expectedError == nil {
				t.Errorf("error while making put cluster customdata call with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			}
		})
	}
}

func TestDeleteCustomData(t *testing.T) {
	admiralParams := admiralParamsForRegistryClientTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	dummyRespBody := ioutil.NopCloser(bytes.NewBufferString("dummyRespBody"))
	validClient := MockClient{
		expectedDeleteResponse: &http.Response{
			StatusCode: 200,
			Body:       dummyRespBody,
		},
		expectedDeleteErr: nil,
		expectedConfig:    &Config{Host: "host", BaseURI: "v1"},
	}
	rc := newDefaultRegistryClient()
	rc.client = &validClient
	testCases := []struct {
		name          string
		expectedError any
	}{
		{
			name: "Given a valid DELETE request, " +
				"Then the registry call should succeed",
			expectedError: nil,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := rc.DeleteCustomData("clusterName", "namespace", "customdata-name", "resourceType", "tid")
			if err != nil && c.expectedError == nil {
				t.Errorf("error while making put cluster customdata call with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			}
		})
	}
}

func TestPutHostingData(t *testing.T) {
	admiralParams := admiralParamsForRegistryClientTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	dummyRespBody := ioutil.NopCloser(bytes.NewBufferString("dummyRespBody"))
	validClient := MockClient{
		expectedPutResponse: &http.Response{
			StatusCode: 200,
			Body:       dummyRespBody,
		},
		expectedPutErr: nil,
		expectedConfig: &Config{Host: "host", BaseURI: "v1"},
	}
	errorClient := MockClient{
		expectedPutResponse: &http.Response{
			StatusCode: 404,
			Body:       dummyRespBody,
		},
		expectedPutErr: fmt.Errorf("error executing http request err:No Namespace Found with name=namespace and cluster=test-usw2-k8s"),
		expectedConfig: &Config{Host: "host", BaseURI: "v1"},
	}
	vc := newDefaultRegistryClient()
	vc.client = &validClient
	ec := newDefaultRegistryClient()
	ec.client = &errorClient
	dummyObjectMeta := metaV1.ObjectMeta{
		Name:      "test",
		Namespace: "namespace",
		Labels:    map[string]string{"identity": "bar"},
	}
	dummyDeployment := &appsV1.Deployment{
		ObjectMeta: dummyObjectMeta,
		Spec: appsV1.DeploymentSpec{
			Selector: &metaV1.LabelSelector{
				MatchLabels: map[string]string{"identity": "bar"},
			},
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
				},
			},
		},
	}
	dummyRollout := &argo.Rollout{
		ObjectMeta: dummyObjectMeta,
		Spec: argo.RolloutSpec{
			Selector: &metaV1.LabelSelector{
				MatchLabels: map[string]string{"identity": "bar"},
			},
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
				},
			},
		},
	}
	dummyJob := &batchv1.Job{
		ObjectMeta: dummyObjectMeta,
		Spec: batchv1.JobSpec{
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
				},
			},
		},
	}
	dummyVertex := &numa.Vertex{
		ObjectMeta: dummyObjectMeta,
		Spec:       numa.VertexSpec{},
	}
	dummyVertex.Spec.Metadata = &numa.Metadata{
		Labels: map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
	}
	dummyMonoVertex := &numa.MonoVertex{
		ObjectMeta: dummyObjectMeta,
		Spec:       numa.MonoVertexSpec{},
	}
	dummyMonoVertex.Spec.Metadata = &numa.Metadata{
		Labels: map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
	}
	dummyService := &coreV1.Service{
		ObjectMeta: dummyObjectMeta,
		Spec: coreV1.ServiceSpec{
			Selector: map[string]string{"app": "app"},
		},
	}
	testCases := []struct {
		name          string
		expectedError any
		resourceType  string
		rc            *registryClient
		value         interface{}
	}{
		{
			name: "Given a valid request body with Deployment, " +
				"Then the registry call should succeed",
			expectedError: nil,
			resourceType:  common.Deployment,
			rc:            vc,
			value:         dummyDeployment,
		},
		{
			name: "Given a valid request body with Rollout, " +
				"Then the registry call should succeed",
			expectedError: nil,
			resourceType:  common.Rollout,
			rc:            vc,
			value:         dummyRollout,
		},
		{
			name: "Given a valid request body with Job, " +
				"Then the registry call should succeed",
			expectedError: nil,
			resourceType:  common.Job,
			rc:            vc,
			value:         dummyJob,
		},
		{
			name: "Given a valid request body with Vertex, " +
				"Then the registry call should succeed",
			expectedError: nil,
			resourceType:  common.Vertex,
			rc:            vc,
			value:         dummyVertex,
		},
		{
			name: "Given a valid request body with MonoVertex, " +
				"Then the registry call should succeed",
			expectedError: nil,
			resourceType:  common.MonoVertex,
			rc:            vc,
			value:         dummyMonoVertex,
		},
		{
			name: "Given a valid request body with Service, " +
				"Then the registry call should succeed",
			expectedError: nil,
			resourceType:  "service",
			rc:            vc,
			value:         dummyService,
		},
		{
			name: "Given a valid request body with Service, " +
				"When the registry call fails with No Namespace Found, " +
				"Then the hosting route should be called",
			expectedError: fmt.Errorf("error executing http request err:No Namespace Found with name=namespace and cluster=test-usw2-k8s"),
			resourceType:  "service",
			rc:            ec,
			value:         dummyService,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := c.rc.PutHostingData("clusterName", "namespace", "hostingdata-name", "asset.alias", c.resourceType, "tid", c.value)
			if err != nil && c.expectedError == nil {
				t.Errorf("error while making put cluster hostingdata call with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			}
		})
	}
}

func TestDeleteHostingData(t *testing.T) {
	admiralParams := admiralParamsForRegistryClientTests()
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	dummyRespBody := ioutil.NopCloser(bytes.NewBufferString("dummyRespBody"))
	validClient := MockClient{
		expectedDeleteResponse: &http.Response{
			StatusCode: 200,
			Body:       dummyRespBody,
		},
		expectedDeleteErr: nil,
		expectedConfig:    &Config{Host: "host", BaseURI: "v1"},
	}
	rc := newDefaultRegistryClient()
	rc.client = &validClient
	dummyObjectMeta := metaV1.ObjectMeta{
		Name:      "test",
		Namespace: "namespace",
		Labels:    map[string]string{"identity": "bar"},
	}
	dummyDeployment := &appsV1.Deployment{
		ObjectMeta: dummyObjectMeta,
		Spec: appsV1.DeploymentSpec{
			Selector: &metaV1.LabelSelector{
				MatchLabels: map[string]string{"identity": "bar"},
			},
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: map[string]string{"identity": "bar", "istio-injected": "true", "env": "dev"},
				},
			},
		},
	}
	testCases := []struct {
		name          string
		expectedError any
		resourceType  string
		value         interface{}
	}{
		{
			name: "Given a valid request body with Deployment, " +
				"Then the registry call should succeed",
			expectedError: nil,
			resourceType:  common.Deployment,
			value:         dummyDeployment,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := rc.DeleteHostingData("clusterName", "namespace", "hostingdata-name", "asset.alias", c.resourceType, "tid")
			if err != nil && c.expectedError == nil {
				t.Errorf("error while making delete cluster hostingdata call with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			}
		})
	}
}

func getUnexportedProperties() cmp.Option {
	return cmpopts.IgnoreUnexported(
		networkingV1Alpha3.ServicePort{},
		networkingV1Alpha3.TrafficPolicy{},
		networkingV1Alpha3.LoadBalancerSettings{},
		networkingV1Alpha3.LocalityLoadBalancerSetting{},
		networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{},
		duration.Duration{},
		networkingV1Alpha3.ConnectionPoolSettings{},
		networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{},
		networkingV1Alpha3.OutlierDetection{},
		wrappers.UInt32Value{})
}
