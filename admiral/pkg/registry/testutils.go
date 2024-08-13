package registry

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	networking "istio.io/api/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetSampleIdentityConfigEnvironment(env string, namespace string) *IdentityConfigEnvironment {
	identityConfigEnvironment := &IdentityConfigEnvironment{
		Name:        env,
		Namespace:   namespace,
		ServiceName: "app-1-spk-root-service",
		Services: map[string]*RegistryServiceConfig{
			"app-1-spk-root-service": &RegistryServiceConfig{
				Name:   "app-1-spk-root-service",
				Weight: -1,
				Ports: map[string]uint32{
					"http": 8090,
				},
			},
		},
		Type:      "rollout",
		Selectors: map[string]string{"app": "app-1"},
		Ports:     []*networking.ServicePort{{Name: "http", Number: uint32(80), Protocol: "http"}},
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
	prfEnv := GetSampleIdentityConfigEnvironment("prf", "ns-1-usw2-prf")
	e2eEnv := GetSampleIdentityConfigEnvironment("e2e", "ns-1-usw2-e2e")
	qalEnv := GetSampleIdentityConfigEnvironment("qal", "ns-1-usw2-qal")
	environments := map[string]*IdentityConfigEnvironment{
		"prf": prfEnv,
		"e2e": e2eEnv,
		"qal": qalEnv,
	}
	clientAssets := map[string]string{
		"sample": "sample",
	}
	cluster := IdentityConfigCluster{
		Name:            "cluster1",
		Locality:        "us-west-2",
		IngressEndpoint: "abc-elb.us-west-2.elb.amazonaws.com.",
		IngressPort:     "15443",
		IngressPortName: "http",
		Environment:     environments,
	}
	identityConfig := IdentityConfig{
		IdentityName: "sample",
		Clusters: map[string]*IdentityConfigCluster{
			"cluster1": &cluster},
		ClientAssets: clientAssets,
	}
	return identityConfig
}
