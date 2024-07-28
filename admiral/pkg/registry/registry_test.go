package registry

import (
	json "encoding/json"
	"reflect"
	"testing"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
<<<<<<< HEAD
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func getSampleIdentityConfigEnvironment(env string, namespace string) IdentityConfigEnvironment {
	identityConfigEnvironment := IdentityConfigEnvironment{
		Name:        env,
		Namespace:   namespace,
		ServiceName: "partner-data-to-tax-spk-root-service",
		Type:        "rollout",
		Selectors:   map[string]string{"app": "partner-data-to-tax"},
		Ports:       []coreV1.ServicePort{{Name: "http-service-mesh", Port: int32(8090), Protocol: coreV1.ProtocolTCP, TargetPort: intstr.FromInt(8090)}},
		TrafficPolicy: networkingV1Alpha3.TrafficPolicy{
			LoadBalancer: &networkingV1Alpha3.LoadBalancerSettings{
				LbPolicy: &networkingV1Alpha3.LoadBalancerSettings_Simple{Simple: networkingV1Alpha3.LoadBalancerSettings_LEAST_REQUEST},
				LocalityLbSetting: &networkingV1Alpha3.LocalityLoadBalancerSetting{
					Distribute: []*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{{
						From: "*",
						To:   map[string]uint32{"us-west-2": 100},
					}},
				},
				WarmupDurationSecs: &duration.Duration{Seconds: 45},
			},
			ConnectionPool: &networkingV1Alpha3.ConnectionPoolSettings{
				Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
					Http2MaxRequests:         1000,
					MaxRequestsPerConnection: 5,
				},
			},
			OutlierDetection: &networkingV1Alpha3.OutlierDetection{
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 0},
				Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
			},
		},
	}
	return identityConfigEnvironment
}

func getSampleIdentityConfig() IdentityConfig {
	prfEnv := getSampleIdentityConfigEnvironment("prf", "ctg-taxprep-partnerdatatotax-usw2-prf")
	e2eEnv := getSampleIdentityConfigEnvironment("e2e", "ctg-taxprep-partnerdatatotax-usw2-e2e")
	qalEnv := getSampleIdentityConfigEnvironment("qal", "ctg-taxprep-partnerdatatotax-usw2-qal")
	environments := []IdentityConfigEnvironment{prfEnv, e2eEnv, qalEnv}
	clientAssets := []map[string]string{{"name": "intuit.cto.dev_portal"}, {"name": "intuit.ctg.tto.browserclient"}, {"name": "intuit.ctg.taxprep.partnerdatatotaxtestclient"}, {"name": "intuit.productmarketing.ipu.pmec"}, {"name": "intuit.tax.taxdev.txo"}, {"name": "intuit.CTO.oauth2"}, {"name": "intuit.platform.servicesgateway.servicesgateway"}, {"name": "intuit.ctg.taxprep.partnerdatatotax"}, {"name": "sample"}}
	cluster := IdentityConfigCluster{
		Name:            "cg-tax-ppd-usw2-k8s",
		Locality:        "us-west-2",
		IngressEndpoint: "a-elb.us-west-2.elb.amazonaws.com.",
		IngressPort:     "15443",
		IngressPortName: "http",
		Environment:     environments,
		ClientAssets:    clientAssets,
	}
	identityConfig := IdentityConfig{
		Assetname: "Intuit.ctg.taxprep.partnerdatatotax",
		Clusters:  []IdentityConfigCluster{cluster},
	}
	return identityConfig
}

=======
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	log "github.com/sirupsen/logrus"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	"reflect"
	"testing"
)

>>>>>>> 508caceb (MESH-5069: Operator Shards (#749))
func TestParseIdentityConfigJSON(t *testing.T) {
	identityConfig := GetSampleIdentityConfig()
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
<<<<<<< HEAD
=======

func TestIdentityConfigGetByIdentityName(t *testing.T) {
	sampleIdentityConfig := GetSampleIdentityConfig()
	registryClient := NewRegistryClient(WithRegistryEndpoint("endpoint"))
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

			identityConfig, err := registryClient.GetIdentityConfigByIdentityName(c.identityAlias, ctxLogger)
			if err != nil && c.expectedError == nil {
				t.Errorf("error while getting identityConfig by name with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			} else {
				opts := cmpopts.IgnoreUnexported(networkingV1Alpha3.TrafficPolicy{}, networkingV1Alpha3.LoadBalancerSettings{}, networkingV1Alpha3.LocalityLoadBalancerSetting{}, networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{}, duration.Duration{}, networkingV1Alpha3.ConnectionPoolSettings{}, networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{}, networkingV1Alpha3.OutlierDetection{}, wrappers.UInt32Value{})
				if !cmp.Equal(identityConfig, c.expectedIdentityConfig, opts) {
					t.Errorf("mismatch between parsed JSON file and expected identity config for alias: %s", c.identityAlias)
					t.Errorf(cmp.Diff(identityConfig, c.expectedIdentityConfig, opts))
				}
			}
		})
	}
}

func TestGetIdentityConfigByClusterName(t *testing.T) {
	sampleIdentityConfig := GetSampleIdentityConfig()
	registryClient := NewRegistryClient(WithRegistryEndpoint("endpoint"))
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
			identityConfigs, err := registryClient.GetIdentityConfigByClusterName(c.clusterName, ctxLogger)
			if err != nil && c.expectedError == nil {
				t.Errorf("error while getting identityConfigs by cluster name with error: %v", err)
			} else if err != nil && c.expectedError != nil && !errors.As(err, &c.expectedError) {
				t.Errorf("failed to get correct error: %v, instead got error: %v", c.expectedError, err)
			} else {
				opts := cmpopts.IgnoreUnexported(networkingV1Alpha3.TrafficPolicy{}, networkingV1Alpha3.LoadBalancerSettings{}, networkingV1Alpha3.LocalityLoadBalancerSetting{}, networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute{}, duration.Duration{}, networkingV1Alpha3.ConnectionPoolSettings{}, networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{}, networkingV1Alpha3.OutlierDetection{}, wrappers.UInt32Value{})
				if !cmp.Equal(identityConfigs[0], c.expectedIdentityConfig, opts) {
					t.Errorf("mismatch between parsed JSON file and expected identity config for file: %s", c.clusterName)
					t.Errorf(cmp.Diff(identityConfigs[0], c.expectedIdentityConfig, opts))
				}
			}
		})
	}
}
>>>>>>> 508caceb (MESH-5069: Operator Shards (#749))
