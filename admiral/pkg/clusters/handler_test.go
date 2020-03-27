package clusters

import (
	"github.com/gogo/protobuf/types"
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"istio.io/api/networking/v1alpha3"
	"testing"
)

func TestIgnoreIstioResource(t *testing.T) {

	//Struct of test case info. Name is required.
	testCases := []struct {
		name           string
		exportTo       []string
		expectedResult bool
	}{
		{
			name:           "Should return false when exportTo is not present",
			exportTo:       nil,
			expectedResult: false,
		},
		{
			name:           "Should return false when its exported to *",
			exportTo:       []string{"*"},
			expectedResult: false,
		},
		{
			name:           "Should return true when its exported to .",
			exportTo:       []string{"."},
			expectedResult: true,
		},
		{
			name:           "Should return true when its exported to a handful of namespaces",
			exportTo:       []string{"namespace1", "namespace2"},
			expectedResult: true,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := IgnoreIstioResource(c.exportTo)
			if result == c.expectedResult {
				//perfect
			} else {
				t.Errorf("Failed. Got %v, expected %v", result, c.expectedResult)
			}
		})
	}
}

func TestGetDestinationRule(t *testing.T) {
	//Do setup here
	mTLS := &v1alpha3.TrafficPolicy{Tls: &v1alpha3.TLSSettings{Mode: v1alpha3.TLSSettings_ISTIO_MUTUAL}}

	noGtpDr := v1alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLS,
	}

	basicGtpDr := v1alpha3.DestinationRule{
		Host: "qa.myservice.global",
		TrafficPolicy: &v1alpha3.TrafficPolicy{
			Tls: &v1alpha3.TLSSettings{Mode: v1alpha3.TLSSettings_ISTIO_MUTUAL},
			LoadBalancer: &v1alpha3.LoadBalancerSettings{
				LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{Simple: v1alpha3.LoadBalancerSettings_ROUND_ROBIN},
				LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{},
			},
			OutlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:  &types.Duration{Seconds: 120},
				ConsecutiveErrors: 10,
				Interval:          &types.Duration{Seconds: 60},
			},
		},
	}

	failoverGtpDr := v1alpha3.DestinationRule{
		Host: "qa.myservice.global",
		TrafficPolicy: &v1alpha3.TrafficPolicy{
			Tls: &v1alpha3.TLSSettings{Mode: v1alpha3.TLSSettings_ISTIO_MUTUAL},
			LoadBalancer: &v1alpha3.LoadBalancerSettings{
				LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{Simple: v1alpha3.LoadBalancerSettings_ROUND_ROBIN},
				LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{
					Distribute: []*v1alpha3.LocalityLoadBalancerSetting_Distribute{
						{
							From: "uswest2/*",
							To: map[string]uint32{"us-west-2": 100},
						},
					},
				},
			},
			OutlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:  &types.Duration{Seconds: 120},
				ConsecutiveErrors: 10,
				Interval:          &types.Duration{Seconds: 60},
			},
		},
	}

	topologyGTPBody := model.GlobalTrafficPolicy{
		Policy: []*model.TrafficPolicy{
			{
				LbType: model.TrafficPolicy_TOPOLOGY,
				Target: []*model.TrafficGroup{
					{
						Region: "us-west-2",
						Weight: 100,
					},
				},
			},
		},
	}

	topologyGTP := v1.GlobalTrafficPolicy{
		Spec: topologyGTPBody,
	}
	topologyGTP.Name = "myGTP"
	topologyGTP.Namespace = "myNS"

	failoverGTPBody := model.GlobalTrafficPolicy{
		Policy: []*model.TrafficPolicy{
			{
				LbType: model.TrafficPolicy_FAILOVER,
				Target: []*model.TrafficGroup{
					{
						Region: "us-west-2",
						Weight: 100,
					},
				},
			},
		},
	}

	failoverGTP := v1.GlobalTrafficPolicy{
		Spec: failoverGTPBody,
	}
	failoverGTP.Name = "myGTP"
	failoverGTP.Namespace = "myNS"

	//Struct of test case info. Name is required.
	testCases := []struct {
		name            string
		host            string
		locality        string
		gtp             *v1.GlobalTrafficPolicy
		destinationRule *v1alpha3.DestinationRule
	}{
		{
			name:            "Should handle a nil GTP",
			host:            "qa.myservice.global",
			locality:        "uswest2",
			gtp:             nil,
			destinationRule: &noGtpDr,
		},
		{
			name:            "Should handle a topology GTP",
			host:            "qa.myservice.global",
			locality:        "uswest2",
			gtp:             &topologyGTP,
			destinationRule: &basicGtpDr,
		},
		{
			name:            "Should handle a failover GTP",
			host:            "qa.myservice.global",
			locality:        "uswest2",
			gtp:             &failoverGTP,
			destinationRule: &failoverGtpDr,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getDestinationRule(c.host, c.locality, c.gtp)
			if !cmp.Equal(result, c.destinationRule) {
				t.Fatalf("DestinationRule Mismatch. Diff: %v", cmp.Diff(result, c.destinationRule))
			}
		})
	}
}
