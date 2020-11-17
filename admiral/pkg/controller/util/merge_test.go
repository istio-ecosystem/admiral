package util

import (
	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
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

	//test idempotency
	mvs, mdr, err = Merge(sc, mvs, mdr, []string{"identity"})

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

	sdr, err := yaml.Marshal(mdr)
	if err != nil {
		t.Error(err)
		t.Fail()
	}
	t.Log(string(sdr[:]))

	assertVS, err := loadVirtualServiceMerge()
	if err != nil {
		t.Error(err)
		t.Fail()
	}

	if !cmp.Equal(mvs, assertVS) {
		t.Fail()
	}
}

func TestMergeNoSourceMatch(t *testing.T) {

	sc, err := loadServiceClientPopulated()

	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	sc.Labels["identity"] = "blah"

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

	_, _, err = Merge(sc, vs, dr, []string{"blah"})
	if err == nil {
		t.Error(err)
		t.Fail()
	}

	mvs, _, err := Merge(sc, vs, dr, []string{"identity"})

	assertVS, err := loadVirtualServiceMerge()
	if err != nil {
		t.Error(err)
		t.Fail()
	}

	if cmp.Equal(mvs, assertVS) {
		t.Fail()
	}
}

func TestMergeWithDeleteOfSCRule(t *testing.T) {

	sc, err := loadServiceClientPopulated()

	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	scwithoutv1, err := loadServiceClientPopulatedWithoutV1()

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

	assertVs, err := loadVirtualServiceMerge()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if !cmp.Equal(mvs, assertVs) {
		t.FailNow()
	}

	m2vs, m2dr, err := Merge(scwithoutv1, mvs, dr, []string{"identity"})

	assertVS2, err := loadVirtualServiceMergeWithoutSCV1()

	if !cmp.Equal(m2vs, assertVS2) {
		t.FailNow()
	}

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	m3vs, m3dr, err := Merge(sc, m2vs, dr, []string{"identity"})

	if !cmp.Equal(m3vs, assertVs) {
		printYaml(t, err, m3vs)
		printYaml(t, err, assertVs)
		t.FailNow()
	}

	if err != nil {
		t.Error(err)
		t.Fail()
	}

	if !cmp.Equal(m2dr, dr) {
		//dr should be modified by merge
		t.FailNow()
	}

	if !cmp.Equal(m3dr, dr) {
		//dr should be modified by merge
		t.FailNow()
	}

	if err != nil {
		t.Error(err)
		t.Fail()
	}

	printYaml(t, err, mvs)

	sdr, err := yaml.Marshal(mdr)
	if err != nil {
		t.Error(err)
		t.Fail()
	}
	t.Log(string(sdr[:]))

}

func TestMergeWith2SCRules(t *testing.T) {

	sc, err := loadServiceClientPopulated()

	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	sc2, err := loadServiceClientPopulated2()

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

	scwithoutv1, err := loadServiceClientPopulatedWithoutV1()

	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	assert, err := loadVirtualServiceAsset3()

	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	mvs, mdr, err := Merge(sc, vs, dr, []string{"identity"})

	m2vs, m2dr, err := Merge(sc2, mvs, mdr, []string{"identity"})

	m3vs, mdr, err := Merge(sc, m2vs, dr, []string{"identity"})

	m4vs, mdr, err := Merge(scwithoutv1, m2vs, dr, []string{"identity"})

	printYaml(t, err, m3vs)

	printYaml(t, err, m4vs)

	if !cmp.Equal(m4vs, assert) {
		//dr should be modified by merge
		t.FailNow()
	}

	sdr, err := yaml.Marshal(m2dr)
	if err != nil {
		t.Error(err)
		t.Fail()
	}
	t.Log(string(sdr[:]))

}

func printYaml(t *testing.T, err error, mvs *v1alpha3.VirtualService) {
	s, err := yaml.Marshal(mvs)
	if err != nil {
		t.Error(err)
		t.Fail()
	}
	t.Log(string(s[:]))
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
      timeout: 10s  #Duration
      retries: #HTTPRetry
        attempts: 3
        perTryTimeout: 2s
        retryOn: gateway-error,connect-failure,refused-stream 
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

func loadServiceClientPopulated2() (*v1.ServiceClient, error) {

	in := []byte(`
apiVersion: admiralproj.io/v1alpha1
kind: ServiceClient
metadata:
  name: rating-reviews
  labels:
    identity: store #client
spec:
  selector: #deployment to target
    identity: reviews
    version: v1
  Http:
    - match:
        - uri:
            prefix: "/v2/*"
      timeout: 10s
      retries:
        attempts: 3
        perTryTimeout: 2s
        retryOn: gateway-error,connect-failure,refused-stream
    - match:
        - uri:
            prefix: "/v1/*"
      timeout: 10s
      retries:
        attempts: 5
        perTryTimeout: 20s
        retryOn: gateway-error
    - match:
      - headers:
          end-user:
            exact: bob
      fault: 
        delay:
          fixedDelay: 10s
          percentage:
            value: 50`)

	sc := v1.ServiceClient{}
	err := yaml.Unmarshal(in, &sc)

	return &sc, err

}

func loadServiceClientPopulatedWithoutV1() (*v1.ServiceClient, error) {

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

func loadVirtualServiceMergeWithoutSCV1() (*v1alpha3.VirtualService, error) {

	in := []byte(`
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata: 
  name: reviews
spec:
  hosts:
  - reviews
  http:
          - fault:
              delay:
                fixedDelay: 7s
                percentage:
                  value: 100
            match:
            - headers:
                end-user:
                  exact: jason
              sourceLabels:
                identity: ratings
            route:
            - destination:
                host: reviews
                subset: v1 
          - match:
            - uri:
                prefix: /v1/*
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

func loadVirtualServiceMerge() (*v1alpha3.VirtualService, error) {

	in := []byte(`
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata: 
  name: reviews
spec:
  hosts:
  - reviews
  http:
          - fault:
              delay:
                fixedDelay: 7s
                percentage:
                  value: 100
            match:
            - headers:
                end-user:
                  exact: jason
              sourceLabels:
                identity: ratings
            route:
            - destination:
                host: reviews
                subset: v1
          - match:
            - sourceLabels:
                identity: ratings
              uri:
                prefix: /v1/*
            retries:
              attempts: 3
              perTryTimeout: 2s
              retryOn: gateway-error,connect-failure,refused-stream
            route:
            - destination:
                host: reviews
                subset: v2
            timeout: 10s
          - match:
            - uri:
                prefix: /v1/*
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

func loadVirtualServiceAsset3() (*v1alpha3.VirtualService, error) {

	in := []byte(`
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
          creationTimestamp: null
          name: reviews
spec:
          hosts:
          - reviews
          http:
          - fault:
              delay:
                fixedDelay: 7s
                percentage:
                  value: 100
            match:
            - headers:
                end-user:
                  exact: jason
              sourceLabels:
                identity: ratings
            route:
            - destination:
                host: reviews
                subset: v1
          - fault:
              delay:
                fixedDelay: 10s
                percentage:
                  value: 50
            match:
            - headers:
                end-user:
                  exact: bob
              sourceLabels:
                identity: store
            route:
            - destination:
                host: reviews
                subset: v1
          - match:
            - sourceLabels:
                identity: store
              uri:
                prefix: /v1/*
            retries:
              attempts: 5
              perTryTimeout: 20s
              retryOn: gateway-error
            route:
            - destination:
                host: reviews
                subset: v2
            timeout: 10s
          - match:
            - sourceLabels:
                identity: store
              uri:
                prefix: /v2/*
            retries:
              attempts: 3
              perTryTimeout: 2s
              retryOn: gateway-error,connect-failure,refused-stream
            route:
            - destination:
                host: reviews
                subset: v1
            timeout: 10s
          - match:
            - uri:
                prefix: /v1/*
            route:
            - destination:
                host: reviews
                subset: v2
          - route:
            - destination:
                host: reviews
                subset: v1
`)

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
