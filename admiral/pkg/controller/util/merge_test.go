package util

import (
	"github.com/ghodss/yaml"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"testing"
)

func TestMerge(t *testing.T) {

	sc, err := loadServiceClientPopulated()

	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	vs, err := loadVirtualServicePopulated()

	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	dr, err := loadDestinationRulePopulated()

	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	mvs, mdr, err := Merge(sc, vs, dr, []string{"identity"})

	if err != nil {
		t.Error(err)
		t.Fail()
	}

	s, err := yaml.Marshal(mvs)
	if err != nil {
		t.Error(err)
		t.Fail()
	}
	t.Log(string(s[:]))
	print(yaml.Marshal(mdr))
}

func loadServiceClientPopulated() (*v1.ServiceClient, error) {

	in := []byte(`
apiVersion: admiralproj.io/v1alpha1
kind: ServiceClient
metadata:
  name: rating-reviews
  labels:
    identity: ratings #client
spec:
  selector: #deployment to target
    identity: reviews
    version: v1
  Http:
    - match:  #HTTPMatchRequest[]
        - uri:
            prefix: "/v1/*"
      timeout: 1s  #Duration
      retries: #HTTPRetry
        attempts: 3
        perTryTimeout: 2s
        retryOn: gateway-error,connect-failure,refused-stream
      outlierDetection: #OutlierDetection
        baseEjectionTime: 3m
        consecutiveErrors: 1
        interval: 1s
        maxEjectionPercent: 100
    - match: #HTTPMatchRequest[]
        - headers:
            end-user:
              exact: jason
      fault: #HTTPFaultInjection
        delay:
          fixedDelay: 7s
          percentage:
            value: 100`)

	sc := v1.ServiceClient{}
	err := yaml.Unmarshal(in, &sc)

	return &sc, err

}

func loadVirtualServicePopulated() (*v1alpha3.VirtualService, error) {

	in := []byte(`
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: reviews
spec:
  hosts:
    - reviews
  http:
    - match:
        - uri:
            prefix: "/v1/*"
      route:
        - destination:
            host: reviews
            subset: v2
    - route:
        - destination:
            host: reviews
            subset: v1`)

	vs := v1alpha3.VirtualService{}
	err := yaml.Unmarshal(in, &vs)

	return &vs, err

}

func loadDestinationRulePopulated() (*v1alpha3.DestinationRule, error) {

	in := []byte(`
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: reviews-destination
  namespace: foo
spec:
  host: reviews # interpreted as reviews.foo.svc.cluster.local
  subsets:
    - name: v1
      labels:
        version: v1
    - name: v2
      labels:
        version: v2`)

	dr := v1alpha3.DestinationRule{}
	err := yaml.Unmarshal(in, &dr)

	return &dr, err

}

//func makeServiceClientFault() *v1.ServiceClient {
//	return &v1.ServiceClient{
//		Spec: v1.ClientSpec{
//
//			Http: []v1.ClientHttpRoute{
//				{
//					Match: &networkingv1alpha3.HTTPMatchRequest{
//						Uri: &networkingv1alpha3.StringMatch{
//							MatchType: &networkingv1alpha3.StringMatch_Regex{Regex: "/v1"},
//						},
//					},
//					Fault: &networkingv1alpha3.HTTPFaultInjection{
//						Abort: &networkingv1alpha3.HTTPFaultInjection_Abort{
//							ErrorType: &networkingv1alpha3.HTTPFaultInjection_Abort_HttpStatus{
//								HttpStatus: 500,
//							},
//						},
//						Delay: &networkingv1alpha3.HTTPFaultInjection_Delay{
//							Percentage: &networkingv1alpha3.Percent{
//								Value: 50,
//							},
//							HttpDelayType: &networkingv1alpha3.HTTPFaultInjection_Delay_FixedDelay{
//								FixedDelay: &types.Duration{
//									Seconds: 2,
//								},
//							},
//						},
//					},
//				},
//			},
//		},
//	}
//}
