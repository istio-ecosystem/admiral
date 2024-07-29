package registry

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func GetSampleIdentityConfigEnvironment(env string, namespace string) IdentityConfigEnvironment {
	identityConfigEnvironment := IdentityConfigEnvironment{
		Name:        env,
		Namespace:   namespace,
		ServiceName: "partner-data-to-tax-spk-root-service",
		Type:        "rollout",
		Selectors:   map[string]string{"app": "partner-data-to-tax"},
		Ports:       []coreV1.ServicePort{{Name: "http-service-mesh", Port: int32(8090), Protocol: coreV1.ProtocolTCP, TargetPort: intstr.FromInt(8090)}},
		TrafficPolicy: TrafficPolicy{
			ClientConnectionConfig: v1alpha1.ClientConnectionConfig{
				ObjectMeta: v1.ObjectMeta{
					Name: "sampleCCC",
				},
				Spec: v1alpha1.ClientConnectionConfigSpec{
					ConnectionPool: model.ConnectionPool{Http: &model.ConnectionPool_HTTP{
						Http2MaxRequests:         1000,
						MaxRequestsPerConnection: 5,
					}},
					Tunnel: model.Tunnel{},
				},
			},
			GlobalTrafficPolicy: v1alpha1.GlobalTrafficPolicy{
				ObjectMeta: v1.ObjectMeta{
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
			},
			OutlierDetection: v1alpha1.OutlierDetection{
				ObjectMeta: v1.ObjectMeta{
					Name: "sampleOD",
				},
				Spec: model.OutlierDetection{
					OutlierConfig: &model.OutlierConfig{
						ConsecutiveGatewayErrors: 10,
						Interval:                 10,
					},
					Selector: nil,
				},
			},
		},
	}
	return identityConfigEnvironment
}

func GetSampleIdentityConfig() IdentityConfig {
	prfEnv := GetSampleIdentityConfigEnvironment("prf", "ctg-taxprep-partnerdatatotax-usw2-prf")
	e2eEnv := GetSampleIdentityConfigEnvironment("e2e", "ctg-taxprep-partnerdatatotax-usw2-e2e")
	qalEnv := GetSampleIdentityConfigEnvironment("qal", "ctg-taxprep-partnerdatatotax-usw2-qal")
	environments := []IdentityConfigEnvironment{prfEnv, e2eEnv, qalEnv}
	clientAssets := []map[string]string{{"name": "intuit.cto.dev_portal"}, {"name": "intuit.ctg.tto.browserclient"}, {"name": "intuit.ctg.taxprep.partnerdatatotaxtestclient"}, {"name": "intuit.productmarketing.ipu.pmec"}, {"name": "intuit.tax.taxdev.txo"}, {"name": "intuit.CTO.oauth2"}, {"name": "intuit.platform.servicesgateway.servicesgateway"}, {"name": "intuit.ctg.taxprep.partnerdatatotax"}, {"name": "sample"}}
	cluster := IdentityConfigCluster{
		Name:            "cg-tax-ppd-usw2-k8s",
		Locality:        "us-west-2",
		IngressEndpoint: "internal-a96ffe9cdbb4c4d81b796cc6a37d3e1d-2123389388.us-west-2.elb.amazonaws.com.",
		IngressPort:     "15443",
		IngressPortName: "http",
		Environment:     environments,
	}
	identityConfig := IdentityConfig{
		IdentityName: "Intuit.ctg.taxprep.partnerdatatotax",
		Clusters:     []IdentityConfigCluster{cluster},
		ClientAssets: clientAssets,
	}
	return identityConfig
}
