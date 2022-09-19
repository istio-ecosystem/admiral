package clusters

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	"istio.io/api/networking/v1alpha3"
	v1alpha32 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioFake "istio.io/client-go/pkg/clientset/versioned/fake"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestGetDependentClusters(t *testing.T) {
	identityClusterCache := common.NewMapOfMaps()
	identityClusterCache.Put("id1", "dep1", "cl1")
	identityClusterCache.Put("id2", "dep2", "cl2")
	identityClusterCache.Put("id3", "dep3", "cl3")

	testCases := []struct {
		name                 string
		dependents           map[string]string
		identityClusterCache *common.MapOfMaps
		sourceServices       map[string]*coreV1.Service
		expectedResult       map[string]string
	}{
		{
			name:           "nil dependents map",
			dependents:     nil,
			expectedResult: make(map[string]string),
		},
		{
			name:                 "empty dependents map",
			dependents:           map[string]string{},
			identityClusterCache: identityClusterCache,
			expectedResult:       map[string]string{},
		},
		{
			name: "no dependent match",
			dependents: map[string]string{
				"id99": "val1",
			},
			identityClusterCache: identityClusterCache,
			expectedResult:       map[string]string{},
		},
		{
			name: "no service for matched dep cluster",
			dependents: map[string]string{
				"id1": "val1",
			},
			identityClusterCache: identityClusterCache,
			sourceServices: map[string]*coreV1.Service{
				"cl1": &coreV1.Service{},
			},
			expectedResult: map[string]string{},
		},
		{
			name: "found service for matched dep cluster",
			dependents: map[string]string{
				"id1": "val1",
			},
			identityClusterCache: identityClusterCache,
			sourceServices: map[string]*coreV1.Service{
				"cl99": &coreV1.Service{
					ObjectMeta: metaV1.ObjectMeta{
						Name: "testservice",
					},
				},
			},
			expectedResult: map[string]string{
				"cl1": "cl1",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualResult := getDependentClusters(tc.dependents, tc.identityClusterCache, tc.sourceServices)
			assert.Equal(t, len(tc.expectedResult), len(actualResult))
			assert.True(t, reflect.DeepEqual(actualResult, tc.expectedResult))
		})
	}

}

func TestIgnoreIstioResource(t *testing.T) {
	//Struct of test case info. Name is required.
	testCases := []struct {
		name           string
		exportTo       []string
		annotations    map[string]string
		namespace      string
		expectedResult bool
	}{
		{
			name:           "Should return false when exportTo is not present",
			exportTo:       nil,
			annotations:    nil,
			namespace:      "random-ns",
			expectedResult: false,
		},
		{
			name:           "Should return false when its exported to *",
			exportTo:       []string{"*"},
			annotations:    nil,
			namespace:      "random-ns",
			expectedResult: false,
		},
		{
			name:           "Should return true when its exported to .",
			exportTo:       []string{"."},
			annotations:    nil,
			namespace:      "random-ns",
			expectedResult: true,
		},
		{
			name:           "Should return true when its exported to a handful of namespaces",
			exportTo:       []string{"namespace1", "namespace2"},
			annotations:    nil,
			namespace:      "random-ns",
			expectedResult: true,
		},
		{
			name:           "Should return true when admiral ignore annotation is set",
			exportTo:       []string{"*"},
			annotations:    map[string]string{common.AdmiralIgnoreAnnotation: "true"},
			namespace:      "random-ns",
			expectedResult: true,
		},
		{
			name:           "Should return true when its from istio-system namespace",
			exportTo:       []string{"*"},
			annotations:    nil,
			namespace:      "istio-system",
			expectedResult: true,
		},
		{
			name:           "Should return true when its from admiral sync namespace",
			exportTo:       []string{"*"},
			annotations:    nil,
			namespace:      "ns",
			expectedResult: true,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := IgnoreIstioResource(c.exportTo, c.annotations, c.namespace)
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
	outlierDetection := &v1alpha3.OutlierDetection{
		BaseEjectionTime:         &duration.Duration{Seconds: 300},
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 50},
		Interval:                 &duration.Duration{Seconds: 60},
		MaxEjectionPercent:       100,
	}
	mTLS := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		OutlierDetection: outlierDetection,
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				Http2MaxRequests:         DefaultHTTP2MaxRequests,
				MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
		},
	}
	se := &v1alpha3.ServiceEntry{Hosts: []string{"qa.myservice.global"}, Endpoints: []*v1alpha3.WorkloadEntry{
		{Address: "east.com", Locality: "us-east-2"}, {Address: "west.com", Locality: "us-west-2"},
	}}
	noGtpDr := v1alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLS,
	}

	basicGtpDr := v1alpha3.DestinationRule{
		Host: "qa.myservice.global",
		TrafficPolicy: &v1alpha3.TrafficPolicy{
			Tls: &v1alpha3.ClientTLSSettings{Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL},
			LoadBalancer: &v1alpha3.LoadBalancerSettings{
				LbPolicy:          &v1alpha3.LoadBalancerSettings_Simple{Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST},
				LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{},
			},
			OutlierDetection: outlierDetection,
			ConnectionPool: &v1alpha3.ConnectionPoolSettings{
				Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
					Http2MaxRequests:         DefaultHTTP2MaxRequests,
					MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
				},
			},
		},
	}

	failoverGtpDr := v1alpha3.DestinationRule{
		Host: "qa.myservice.global",
		TrafficPolicy: &v1alpha3.TrafficPolicy{
			Tls: &v1alpha3.ClientTLSSettings{Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL},
			LoadBalancer: &v1alpha3.LoadBalancerSettings{
				LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST},
				LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{
					Distribute: []*v1alpha3.LocalityLoadBalancerSetting_Distribute{
						{
							From: "uswest2/*",
							To:   map[string]uint32{"us-west-2": 100},
						},
					},
				},
			},
			OutlierDetection: outlierDetection,
			ConnectionPool: &v1alpha3.ConnectionPoolSettings{
				Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
					Http2MaxRequests:         DefaultHTTP2MaxRequests,
					MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
				},
			},
		},
	}

	topologyGTPPolicy := &model.TrafficPolicy{
		LbType: model.TrafficPolicy_TOPOLOGY,
		Target: []*model.TrafficGroup{
			{
				Region: "us-west-2",
				Weight: 100,
			},
		},
	}

	failoverGTPPolicy := &model.TrafficPolicy{
		LbType: model.TrafficPolicy_FAILOVER,
		Target: []*model.TrafficGroup{
			{
				Region: "us-west-2",
				Weight: 100,
			},
			{
				Region: "us-east-2",
				Weight: 0,
			},
		},
	}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name            string
		se              *v1alpha3.ServiceEntry
		locality        string
		gtpPolicy       *model.TrafficPolicy
		destinationRule *v1alpha3.DestinationRule
	}{
		{
			name:            "Should handle a nil GTP",
			se:              se,
			locality:        "uswest2",
			gtpPolicy:       nil,
			destinationRule: &noGtpDr,
		},
		{
			name:            "Should return default DR with empty locality",
			se:              se,
			locality:        "",
			gtpPolicy:       failoverGTPPolicy,
			destinationRule: &noGtpDr,
		},
		{
			name:            "Should handle a topology GTP",
			se:              se,
			locality:        "uswest2",
			gtpPolicy:       topologyGTPPolicy,
			destinationRule: &basicGtpDr,
		},
		{
			name:            "Should handle a failover GTP",
			se:              se,
			locality:        "uswest2",
			gtpPolicy:       failoverGTPPolicy,
			destinationRule: &failoverGtpDr,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getDestinationRule(c.se, c.locality, c.gtpPolicy)
			if !cmp.Equal(result, c.destinationRule, protocmp.Transform()) {
				t.Fatalf("DestinationRule Mismatch. Diff: %v", cmp.Diff(result, c.destinationRule))
			}
		})
	}
}

func TestGetOutlierDetection(t *testing.T) {
	//Do setup here
	outlierDetection := &v1alpha3.OutlierDetection{
		BaseEjectionTime:         &duration.Duration{Seconds: DefaultBaseEjectionTime},
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: DefaultConsecutiveGatewayErrors},
		Interval:                 &duration.Duration{Seconds: DefaultInterval},
		MaxEjectionPercent:       100,
	}

	outlierDetectionOneHostRemote := &v1alpha3.OutlierDetection{
		BaseEjectionTime:         &duration.Duration{Seconds: DefaultBaseEjectionTime},
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: DefaultConsecutiveGatewayErrors},
		Interval:                 &duration.Duration{Seconds: DefaultInterval},
		MaxEjectionPercent:       34,
	}

	topologyGTPPolicy := &model.TrafficPolicy{
		LbType: model.TrafficPolicy_TOPOLOGY,
		Target: []*model.TrafficGroup{
			{
				Region: "us-west-2",
				Weight: 100,
			},
		},
	}

	se := &v1alpha3.ServiceEntry{Hosts: []string{"qa.myservice.global"}, Endpoints: []*v1alpha3.WorkloadEntry{
		{Address: "east.com", Locality: "us-east-2"}, {Address: "west.com", Locality: "us-west-2"},
	}}

	seOneHostRemote := &v1alpha3.ServiceEntry{Hosts: []string{"qa.myservice.global"}, Endpoints: []*v1alpha3.WorkloadEntry{
		{Address: "east.com", Locality: "us-east-2"},
	}}

	seOneHostLocal := &v1alpha3.ServiceEntry{Hosts: []string{"qa.myservice.global"}, Endpoints: []*v1alpha3.WorkloadEntry{
		{Address: "hello.ns.svc.cluster.local", Locality: "us-east-2"},
	}}

	seOneHostRemoteIp := &v1alpha3.ServiceEntry{Hosts: []string{"qa.myservice.global"}, Endpoints: []*v1alpha3.WorkloadEntry{
		{Address: "95.45.25.34", Locality: "us-east-2"},
	}}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name             string
		se               *v1alpha3.ServiceEntry
		locality         string
		gtpPolicy        *model.TrafficPolicy
		outlierDetection *v1alpha3.OutlierDetection
	}{

		{
			name:             "Should return nil for cluster local only endpoint",
			se:               seOneHostLocal,
			locality:         "uswest2",
			gtpPolicy:        topologyGTPPolicy,
			outlierDetection: nil,
		},
		{
			name:             "Should return nil for one IP endpoint",
			se:               seOneHostRemoteIp,
			locality:         "uswest2",
			gtpPolicy:        topologyGTPPolicy,
			outlierDetection: nil,
		},
		{
			name:             "Should return 34% ejection for remote endpoint with one entry",
			se:               seOneHostRemote,
			locality:         "uswest2",
			gtpPolicy:        topologyGTPPolicy,
			outlierDetection: outlierDetectionOneHostRemote,
		},
		{
			name:             "Should return 100% ejection for two remote endpoints",
			se:               se,
			locality:         "uswest2",
			gtpPolicy:        topologyGTPPolicy,
			outlierDetection: outlierDetection,
		},
		{
			name:             "Should use the default outlier detection if gtpPolicy is nil",
			se:               se,
			locality:         "uswest2",
			gtpPolicy:        nil,
			outlierDetection: outlierDetection,
		},
		{
			name:             "Should use the default outlier detection if OutlierDetection is nil inside gtpPolicy",
			se:               se,
			locality:         "uswest2",
			gtpPolicy:        topologyGTPPolicy,
			outlierDetection: outlierDetection,
		},
		{
			name:     "Should apply the default BaseEjectionTime if it is not configured in the outlier detection config",
			se:       se,
			locality: "uswest2",
			gtpPolicy: &model.TrafficPolicy{
				LbType: model.TrafficPolicy_TOPOLOGY,
				Target: []*model.TrafficGroup{
					{
						Region: "us-west-2",
						Weight: 100,
					},
				},
				OutlierDetection: &model.TrafficPolicy_OutlierDetection{
					ConsecutiveGatewayErrors: 10,
					Interval:                 60,
				},
			},
			outlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:         &duration.Duration{Seconds: DefaultBaseEjectionTime},
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 10},
				Interval:                 &duration.Duration{Seconds: 60},
				MaxEjectionPercent:       100,
			},
		},
		{
			name:     "Should apply the default ConsecutiveGatewayErrors if it is not configured in the outlier detection config",
			se:       se,
			locality: "uswest2",
			gtpPolicy: &model.TrafficPolicy{
				LbType: model.TrafficPolicy_TOPOLOGY,
				Target: []*model.TrafficGroup{
					{
						Region: "us-west-2",
						Weight: 100,
					},
				},
				OutlierDetection: &model.TrafficPolicy_OutlierDetection{
					BaseEjectionTime: 600,
					Interval:         60,
				},
			},
			outlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:         &duration.Duration{Seconds: 600},
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: DefaultConsecutiveGatewayErrors},
				Interval:                 &duration.Duration{Seconds: 60},
				MaxEjectionPercent:       100,
			},
		},
		{
			name:     "Should apply the default Interval if it is not configured in the outlier detection config",
			se:       se,
			locality: "uswest2",
			gtpPolicy: &model.TrafficPolicy{
				LbType: model.TrafficPolicy_TOPOLOGY,
				Target: []*model.TrafficGroup{
					{
						Region: "us-west-2",
						Weight: 100,
					},
				},
				OutlierDetection: &model.TrafficPolicy_OutlierDetection{
					BaseEjectionTime:         600,
					ConsecutiveGatewayErrors: 50,
				},
			},
			outlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:         &duration.Duration{Seconds: 600},
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 50},
				Interval:                 &duration.Duration{Seconds: DefaultInterval},
				MaxEjectionPercent:       100,
			},
		},
		{
			name:     "Default outlier detection config should be overriden by the outlier detection config specified in the TrafficPolicy",
			se:       se,
			locality: "uswest2",
			gtpPolicy: &model.TrafficPolicy{
				LbType: model.TrafficPolicy_TOPOLOGY,
				Target: []*model.TrafficGroup{
					{
						Region: "us-west-2",
						Weight: 100,
					},
				},
				OutlierDetection: &model.TrafficPolicy_OutlierDetection{
					BaseEjectionTime:         600,
					ConsecutiveGatewayErrors: 10,
					Interval:                 60,
				},
			},
			outlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:         &duration.Duration{Seconds: 600},
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 10},
				Interval:                 &duration.Duration{Seconds: 60},
				MaxEjectionPercent:       100,
			},
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getOutlierDetection(c.se, c.locality, c.gtpPolicy)
			if !cmp.Equal(result, c.outlierDetection, protocmp.Transform()) {
				t.Fatalf("OutlierDetection Mismatch. Diff: %v", cmp.Diff(result, c.outlierDetection))
			}
		})
	}
}

func TestHandleVirtualServiceEvent(t *testing.T) {
	var (
		ctx                 = context.Background()
		cnameCache          = common.NewMapOfMaps()
		goodCnameCache      = common.NewMapOfMaps()
		rr                  = NewRemoteRegistry(context.TODO(), common.AdmiralParams{})
		rr1                 = NewRemoteRegistry(context.TODO(), common.AdmiralParams{})
		rr2                 = NewRemoteRegistry(context.TODO(), common.AdmiralParams{})
		fakeIstioClient     = istioFake.NewSimpleClientset()
		fullFakeIstioClient = istioFake.NewSimpleClientset()

		tooManyHosts = v1alpha32.VirtualService{
			Spec: v1alpha3.VirtualService{
				Hosts: []string{"qa.blah.global", "e2e.blah.global"},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "too-many-hosts",
				Namespace: "other-ns",
			},
		}
		happyPath = v1alpha32.VirtualService{
			Spec: v1alpha3.VirtualService{
				Hosts: []string{"e2e.blah.global"},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "vs-name",
				Namespace: "other-ns",
			},
		}
		nonExistentVs = v1alpha32.VirtualService{
			Spec: v1alpha3.VirtualService{
				Hosts: []string{"does-not-exist.com"},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "does-not-exist",
				Namespace: "other-ns",
			},
		}
		vsNotGeneratedByAdmiral = v1alpha32.VirtualService{
			Spec: v1alpha3.VirtualService{
				Hosts: []string{"e2e.blah.something"},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "vs-name-other-nss",
				Namespace: "other-ns",
			},
		}
	)

	rr.AdmiralCache = &AdmiralCache{
		CnameDependentClusterCache: cnameCache,
		SeClusterCache:             common.NewMapOfMaps(),
	}
	noDependentClustersHandler := VirtualServiceHandler{
		RemoteRegistry: rr,
	}

	goodCnameCache.Put("e2e.blah.global", "cluster.k8s.global", "cluster.k8s.global")
	rr1.AdmiralCache = &AdmiralCache{
		CnameDependentClusterCache: goodCnameCache,
		SeClusterCache:             common.NewMapOfMaps(),
	}
	rr1.PutRemoteController("cluster.k8s.global", &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fakeIstioClient,
		},
	})
	handlerEmptyClient := VirtualServiceHandler{
		RemoteRegistry: rr1,
	}
	fullFakeIstioClient.NetworkingV1alpha3().VirtualServices("ns").Create(ctx, &v1alpha32.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "vs-name",
		},
		Spec: v1alpha3.VirtualService{
			Hosts: []string{"e2e.blah.global"},
		},
	}, metaV1.CreateOptions{})
	rr2.AdmiralCache = &AdmiralCache{
		CnameDependentClusterCache: goodCnameCache,
		SeClusterCache:             common.NewMapOfMaps(),
	}
	rr2.PutRemoteController("cluster.k8s.global", &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{
			IstioClient: fullFakeIstioClient,
		},
	})
	handlerFullClient := VirtualServiceHandler{
		ClusterID:      "cluster2.k8s.global",
		RemoteRegistry: rr2,
	}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name          string
		vs            *v1alpha32.VirtualService
		handler       *VirtualServiceHandler
		expectedError error
		event         common.Event
	}{
		{
			name:          "Virtual Service with multiple hosts",
			vs:            &tooManyHosts,
			expectedError: nil,
			handler:       &noDependentClustersHandler,
			event:         0,
		},
		{
			name:          "No dependent clusters",
			vs:            &happyPath,
			expectedError: nil,
			handler:       &noDependentClustersHandler,
			event:         0,
		},
		{
			name:          "Add event for VS not generated by Admiral",
			vs:            &happyPath,
			expectedError: nil,
			handler:       &handlerFullClient,
			event:         0,
		},
		{
			name:          "Update event for VS not generated by Admiral",
			vs:            &vsNotGeneratedByAdmiral,
			expectedError: nil,
			handler:       &handlerFullClient,
			event:         1,
		},
		{
			name:          "Delete event for VS not generated by Admiral",
			vs:            &vsNotGeneratedByAdmiral,
			expectedError: nil,
			handler:       &handlerFullClient,
			event:         2,
		},
		{
			name:          "New Virtual Service",
			vs:            &happyPath,
			expectedError: nil,
			handler:       &handlerEmptyClient,
			event:         0,
		},
		{
			name:          "Existing Virtual Service",
			vs:            &happyPath,
			expectedError: nil,
			handler:       &handlerFullClient,
			event:         1,
		},
		{
			name:          "Deleted existing Virtual Service, should not return an error",
			vs:            &happyPath,
			expectedError: nil,
			handler:       &handlerFullClient,
			event:         2,
		},
		{
			name:          "Deleting virtual service which does not exist, should not return an error",
			vs:            &nonExistentVs,
			expectedError: nil,
			handler:       &handlerFullClient,
			event:         2,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := handleVirtualServiceEvent(ctx, c.vs, c.handler, c.event, common.VirtualService)
			if err != c.expectedError {
				t.Fatalf("Error mismatch, expected %v but got %v", c.expectedError, err)
			}
		})
	}
}

func TestGetServiceForRolloutCanary(t *testing.T) {
	//Struct of test case info. Name is required.
	const (
		Namespace                  = "namespace"
		ServiceName                = "serviceName"
		StableServiceName          = "stableserviceName"
		CanaryServiceName          = "canaryserviceName"
		GeneratedStableServiceName = "hello-" + common.RolloutStableServiceSuffix
		LatestMatchingService      = "hello-root-service"
		vsName1                    = "virtualservice1"
		vsName2                    = "virtualservice2"
		vsName3                    = "virtualservice3"
		vsName4                    = "virtualservice4"
		vsRoutePrimary             = "primary"
	)
	var (
		config = rest.Config{
			Host: "localhost",
		}
		stop            = make(chan struct{})
		fakeIstioClient = istioFake.NewSimpleClientset()
		selectorMap     = map[string]string{
			"app": "test",
		}
		ports = []coreV1.ServicePort{{Port: 8080}, {Port: 8081}}
	)
	s, err := admiral.NewServiceController("test", stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))
	if err != nil {
		t.Fatalf("failed to initialize service controller, err: %v", err)
	}
	r, err := admiral.NewRolloutsController("test", stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300))
	if err != nil {
		t.Fatalf("failed ot initialize rollout controller, err: %v", err)
	}
	v := &istio.VirtualServiceController{
		IstioClient: fakeIstioClient,
	}

	rcTemp := &RemoteController{
		VirtualServiceController: v,
		ServiceController:        s,
		RolloutController:        r,
	}

	service := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: ServiceName, Namespace: Namespace, CreationTimestamp: metaV1.NewTime(time.Now())},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports:    ports,
		},
	}

	// namespace1 Services
	service1 := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: "dummy1", Namespace: "namespace1", CreationTimestamp: metaV1.NewTime(time.Now())},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports: []coreV1.ServicePort{{
				Port: 8080,
				Name: "random3",
			}, {
				Port: 8081,
				Name: "random4",
			},
			},
		},
	}

	// namespace4 Services
	service3 := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: "dummy3", Namespace: "namespace4", CreationTimestamp: metaV1.NewTime(time.Now())},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports: []coreV1.ServicePort{{
				Port: 8080,
				Name: "random3",
			}, {
				Port: 8081,
				Name: "random4",
			},
			},
		},
	}

	service4 := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: "dummy4", Namespace: "namespace4", CreationTimestamp: metaV1.NewTime(time.Now())},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports: []coreV1.ServicePort{{
				Port: 8081,
				Name: "random4",
			},
			},
		},
	}

	// namespace Services
	stableService := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: StableServiceName, Namespace: Namespace, CreationTimestamp: metaV1.NewTime(time.Now().Add(time.Duration(-15)))},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports:    ports,
		},
	}

	generatedStableService := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: GeneratedStableServiceName, Namespace: Namespace, CreationTimestamp: metaV1.NewTime(time.Now().Add(time.Duration(-15)))},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports:    ports,
		},
	}

	canaryService := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: CanaryServiceName, Namespace: Namespace, CreationTimestamp: metaV1.NewTime(time.Now().Add(time.Duration(-15)))},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports:    ports,
		},
	}

	latestMatchingService := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: LatestMatchingService, Namespace: Namespace, CreationTimestamp: metaV1.NewTime(time.Now())},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports:    ports,
		},
	}

	rcTemp.ServiceController.Cache.Put(service)
	rcTemp.ServiceController.Cache.Put(service1)
	rcTemp.ServiceController.Cache.Put(service3)
	rcTemp.ServiceController.Cache.Put(service4)
	rcTemp.ServiceController.Cache.Put(stableService)
	rcTemp.ServiceController.Cache.Put(canaryService)
	rcTemp.ServiceController.Cache.Put(generatedStableService)
	rcTemp.ServiceController.Cache.Put(latestMatchingService)

	virtualService := &v1alpha32.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{Name: vsName1, Namespace: Namespace},
		Spec: v1alpha3.VirtualService{
			Http: []*v1alpha3.HTTPRoute{{Route: []*v1alpha3.HTTPRouteDestination{
				{Destination: &v1alpha3.Destination{Host: StableServiceName}, Weight: 80},
				{Destination: &v1alpha3.Destination{Host: CanaryServiceName}, Weight: 20},
			}}},
		},
	}

	vsMutipleRoutesWithMatch := &v1alpha32.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{Name: vsName2, Namespace: Namespace},
		Spec: v1alpha3.VirtualService{
			Http: []*v1alpha3.HTTPRoute{{Name: vsRoutePrimary, Route: []*v1alpha3.HTTPRouteDestination{
				{Destination: &v1alpha3.Destination{Host: StableServiceName}, Weight: 80},
				{Destination: &v1alpha3.Destination{Host: CanaryServiceName}, Weight: 20},
			}}},
		},
	}

	vsMutipleRoutesWithZeroWeight := &v1alpha32.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{Name: vsName4, Namespace: Namespace},
		Spec: v1alpha3.VirtualService{
			Http: []*v1alpha3.HTTPRoute{{Name: "random", Route: []*v1alpha3.HTTPRouteDestination{
				{Destination: &v1alpha3.Destination{Host: StableServiceName}, Weight: 100},
				{Destination: &v1alpha3.Destination{Host: CanaryServiceName}, Weight: 0},
			}}},
		},
	}
	ctx := context.Background()
	rcTemp.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(Namespace).Create(ctx, virtualService, metaV1.CreateOptions{})
	rcTemp.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(Namespace).Create(ctx, vsMutipleRoutesWithMatch, metaV1.CreateOptions{})
	rcTemp.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(Namespace).Create(ctx, vsMutipleRoutesWithZeroWeight, metaV1.CreateOptions{})

	canaryRollout := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	matchLabel := make(map[string]string)
	matchLabel["app"] = "test"

	labelSelector := metaV1.LabelSelector{
		MatchLabels: matchLabel,
	}
	canaryRollout.Spec.Selector = &labelSelector

	canaryRollout.Namespace = Namespace
	canaryRollout.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{},
	}

	canaryRolloutNS1 := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	matchLabel2 := make(map[string]string)
	matchLabel2["app"] = "test1"

	labelSelector2 := metaV1.LabelSelector{
		MatchLabels: matchLabel2,
	}
	canaryRolloutNS1.Spec.Selector = &labelSelector2

	canaryRolloutNS1.Namespace = "namespace1"
	canaryRolloutNS1.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{},
	}

	canaryRolloutNS4 := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{common.SidecarEnabledPorts: "8080"}},
		}}}
	matchLabel4 := make(map[string]string)
	matchLabel4["app"] = "test"
	labelSelector4 := metaV1.LabelSelector{
		MatchLabels: matchLabel4,
	}
	canaryRolloutNS4.Spec.Selector = &labelSelector4
	canaryRolloutNS4.Namespace = "namespace4"
	canaryRolloutNS4.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{},
	}

	canaryRolloutIstioVs := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	canaryRolloutIstioVs.Spec.Selector = &labelSelector

	canaryRolloutIstioVs.Namespace = Namespace
	canaryRolloutIstioVs.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
			TrafficRouting: &argo.RolloutTrafficRouting{
				Istio: &argo.IstioTrafficRouting{
					VirtualService: &argo.IstioVirtualService{Name: vsName1},
				},
			},
		},
	}

	canaryRolloutIstioVsRouteMatch := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	canaryRolloutIstioVsRouteMatch.Spec.Selector = &labelSelector

	canaryRolloutIstioVsRouteMatch.Namespace = Namespace
	canaryRolloutIstioVsRouteMatch.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
			TrafficRouting: &argo.RolloutTrafficRouting{
				Istio: &argo.IstioTrafficRouting{
					VirtualService: &argo.IstioVirtualService{Name: vsName2, Routes: []string{vsRoutePrimary}},
				},
			},
		},
	}

	canaryRolloutIstioVsRouteMisMatch := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	canaryRolloutIstioVsRouteMisMatch.Spec.Selector = &labelSelector

	canaryRolloutIstioVsRouteMisMatch.Namespace = Namespace
	canaryRolloutIstioVsRouteMisMatch.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
			TrafficRouting: &argo.RolloutTrafficRouting{
				Istio: &argo.IstioTrafficRouting{
					VirtualService: &argo.IstioVirtualService{Name: vsName2, Routes: []string{"random"}},
				},
			},
		},
	}

	canaryRolloutIstioVsZeroWeight := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	canaryRolloutIstioVsZeroWeight.Spec.Selector = &labelSelector

	canaryRolloutIstioVsZeroWeight.Namespace = Namespace
	canaryRolloutIstioVsZeroWeight.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
			TrafficRouting: &argo.RolloutTrafficRouting{
				Istio: &argo.IstioTrafficRouting{
					VirtualService: &argo.IstioVirtualService{Name: vsName4},
				},
			},
		},
	}

	canaryRolloutWithStableService := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	canaryRolloutWithStableService.Spec.Selector = &labelSelector

	canaryRolloutWithStableService.Namespace = Namespace
	canaryRolloutWithStableService.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
		},
	}

	canaryRolloutIstioVsMimatch := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	canaryRolloutIstioVsMimatch.Spec.Selector = &labelSelector

	canaryRolloutIstioVsMimatch.Namespace = Namespace
	canaryRolloutIstioVsMimatch.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
			TrafficRouting: &argo.RolloutTrafficRouting{
				Istio: &argo.IstioTrafficRouting{
					VirtualService: &argo.IstioVirtualService{Name: "random"},
				},
			},
		},
	}

	canaryRolloutWithStableServiceNS4 := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{common.SidecarEnabledPorts: "8080"}},
		}}}
	canaryRolloutWithStableServiceNS4.Spec.Selector = &labelSelector

	canaryRolloutWithStableServiceNS4.Namespace = "namespace4"
	canaryRolloutWithStableServiceNS4.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
		},
	}

	resultForDummy := map[string]*WeightedService{service3.Name: {Weight: 1, Service: service3}}

	resultForEmptyStableServiceOnRollout := map[string]*WeightedService{LatestMatchingService: {Weight: 1, Service: latestMatchingService}}

	resultForCanaryWithIstio := map[string]*WeightedService{StableServiceName: {Weight: 80, Service: stableService},
		CanaryServiceName: {Weight: 20, Service: canaryService}}

	resultForCanaryWithStableService := map[string]*WeightedService{StableServiceName: {Weight: 1, Service: stableService}}

	resultForCanaryWithStableServiceWeight := map[string]*WeightedService{StableServiceName: {Weight: 100, Service: stableService}}

	resultRolloutWithOneServiceHavingMeshPort := map[string]*WeightedService{service3.Name: {Weight: 1, Service: service3}}

	testCases := []struct {
		name    string
		rollout *argo.Rollout
		rc      *RemoteController
		result  map[string]*WeightedService
	}{
		{
			name:    "canaryRolloutHappyCaseMeshPortAnnotationOnRollout",
			rollout: &canaryRolloutNS4,
			rc:      rcTemp,
			result:  resultForDummy,
		}, {
			name:    "canaryRolloutWithoutSelectorMatch",
			rollout: &canaryRolloutNS1,
			rc:      rcTemp,
			result:  make(map[string]*WeightedService, 0),
		}, {
			name:    "canaryRolloutHappyCase",
			rollout: &canaryRollout,
			rc:      rcTemp,
			result:  resultForEmptyStableServiceOnRollout,
		}, {
			name:    "canaryRolloutWithIstioVsMimatch",
			rollout: &canaryRolloutIstioVsMimatch,
			rc:      rcTemp,
			result:  resultForCanaryWithStableService,
		}, {
			name:    "canaryRolloutWithIstioVirtualService",
			rollout: &canaryRolloutIstioVs,
			rc:      rcTemp,
			result:  resultForCanaryWithIstio,
		}, {
			name:    "canaryRolloutWithIstioVirtualServiceZeroWeight",
			rollout: &canaryRolloutIstioVsZeroWeight,
			rc:      rcTemp,
			result:  resultForCanaryWithStableServiceWeight,
		}, {
			name:    "canaryRolloutWithIstioRouteMatch",
			rollout: &canaryRolloutIstioVsRouteMatch,
			rc:      rcTemp,
			result:  resultForCanaryWithIstio,
		}, {
			name:    "canaryRolloutWithIstioRouteMisMatch",
			rollout: &canaryRolloutIstioVsRouteMisMatch,
			rc:      rcTemp,
			result:  resultForCanaryWithStableService,
		},
		{
			name:    "canaryRolloutWithStableServiceName",
			rollout: &canaryRolloutWithStableService,
			rc:      rcTemp,
			result:  resultForCanaryWithStableService,
		},
		{
			name:    "canaryRolloutWithOneServiceHavingMeshPort",
			rollout: &canaryRolloutWithStableServiceNS4,
			rc:      rcTemp,
			result:  resultRolloutWithOneServiceHavingMeshPort,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getServiceForRollout(ctx, c.rc, c.rollout)
			if len(c.result) == 0 {
				if len(result) != 0 {
					t.Fatalf("Service expected to be nil")
				}
			} else {
				for key, wanted := range c.result {
					if got, ok := result[key]; ok {
						if !cmp.Equal(got.Service.Name, wanted.Service.Name) {
							t.Fatalf("Service Mismatch. Diff: %v", cmp.Diff(got.Service.Name, wanted.Service.Name))
						}
						if !cmp.Equal(got.Weight, wanted.Weight) {
							t.Fatalf("Service Weight Mismatch. Diff: %v", cmp.Diff(got.Weight, wanted.Weight))
						}
					} else {
						t.Fatalf("Expected a service with name=%s but none returned", key)
					}
				}
			}
		})
	}
}

func TestGetServiceForRolloutBlueGreen(t *testing.T) {
	//Struct of test case info. Name is required.
	const (
		namespace                         = "namespace"
		serviceName                       = "serviceNameActive"
		generatedActiveServiceName        = "hello-" + common.RolloutActiveServiceSuffix
		rolloutPodHashLabel        string = "rollouts-pod-template-hash"
	)
	var (
		stop   = make(chan struct{})
		config = rest.Config{
			Host: "localhost",
		}
		matchLabel = map[string]string{
			"app": "test",
		}
		labelSelector = metaV1.LabelSelector{
			MatchLabels: matchLabel,
		}
		bgRollout = argo.Rollout{
			Spec: argo.RolloutSpec{
				Selector: &labelSelector,
				Strategy: argo.RolloutStrategy{
					BlueGreen: &argo.BlueGreenStrategy{
						ActiveService:  serviceName,
						PreviewService: "previewService",
					},
				},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
				},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Namespace: namespace,
			},
		}
		bgRolloutNoActiveService = argo.Rollout{
			Spec: argo.RolloutSpec{
				Selector: &labelSelector,
				Strategy: argo.RolloutStrategy{
					BlueGreen: &argo.BlueGreenStrategy{},
				},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
				},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Namespace: namespace,
			},
		}
		selectorMap = map[string]string{
			"app":               "test",
			rolloutPodHashLabel: "hash",
		}
		activeService = &coreV1.Service{
			Spec: coreV1.ServiceSpec{
				Selector: selectorMap,
			},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      serviceName,
				Namespace: namespace,
			},
		}
	)
	s, err := admiral.NewServiceController("test", stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))
	if err != nil {
		t.Fatalf("failed to initialize service controller, err: %v", err)
	}
	r, err := admiral.NewRolloutsController("test", stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300))
	if err != nil {
		t.Fatalf("failed to initialize rollout controller, err: %v", err)
	}

	emptyCacheService, err := admiral.NewServiceController("test", stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))
	if err != nil {
		t.Fatalf("failed to initialize empty service controller, err: %v", err)
	}

	rc := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{},
		ServiceController:        s,
		RolloutController:        r,
	}

	port1 := coreV1.ServicePort{
		Port: 8080,
		Name: "random1",
	}

	port2 := coreV1.ServicePort{
		Port: 8081,
		Name: "random2",
	}

	ports := []coreV1.ServicePort{port1, port2}
	activeService.Spec.Ports = ports

	generatedActiveService := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	generatedActiveService.Name = generatedActiveServiceName
	generatedActiveService.Namespace = namespace
	generatedActiveService.Spec.Ports = ports

	selectorMap1 := make(map[string]string)
	selectorMap1["app"] = "test1"

	service1 := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	service1.Name = "dummy"
	service1.Namespace = namespace
	port3 := coreV1.ServicePort{
		Port: 8080,
		Name: "random3",
	}

	port4 := coreV1.ServicePort{
		Port: 8081,
		Name: "random4",
	}

	ports1 := []coreV1.ServicePort{port3, port4}
	service1.Spec.Ports = ports1

	selectorMap2 := make(map[string]string)
	selectorMap2["app"] = "test"
	selectorMap2[rolloutPodHashLabel] = "hash"
	previewService := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	previewService.Name = "previewService"
	previewService.Namespace = namespace
	port5 := coreV1.ServicePort{
		Port: 8080,
		Name: "random3",
	}

	port6 := coreV1.ServicePort{
		Port: 8081,
		Name: "random4",
	}

	ports2 := []coreV1.ServicePort{port5, port6}

	previewService.Spec.Ports = ports2

	serviceNS1 := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	serviceNS1.Name = "dummy"
	serviceNS1.Namespace = "namespace1"
	port8 := coreV1.ServicePort{
		Port: 8080,
		Name: "random3",
	}

	port9 := coreV1.ServicePort{
		Port: 8081,
		Name: "random4",
	}

	ports12 := []coreV1.ServicePort{port8, port9}
	serviceNS1.Spec.Ports = ports12

	rc.ServiceController.Cache.Put(service1)
	rc.ServiceController.Cache.Put(previewService)
	rc.ServiceController.Cache.Put(activeService)
	rc.ServiceController.Cache.Put(serviceNS1)
	rc.ServiceController.Cache.Put(generatedActiveService)

	noStratergyRollout := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	noStratergyRollout.Namespace = namespace

	noStratergyRollout.Spec.Strategy = argo.RolloutStrategy{}

	bgRolloutNs1 := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}

	matchLabel1 := make(map[string]string)
	matchLabel1["app"] = "test"

	labelSelector1 := metaV1.LabelSelector{
		MatchLabels: matchLabel,
	}
	bgRolloutNs1.Spec.Selector = &labelSelector1

	bgRolloutNs1.Namespace = "namespace1"
	bgRolloutNs1.Spec.Strategy = argo.RolloutStrategy{
		BlueGreen: &argo.BlueGreenStrategy{
			ActiveService:  serviceName,
			PreviewService: "previewService",
		},
	}

	resultForBlueGreen := map[string]*WeightedService{serviceName: {Weight: 1, Service: activeService}}
	resultForNoActiveService := map[string]*WeightedService{generatedActiveServiceName: {Weight: 1, Service: generatedActiveService}}

	testCases := []struct {
		name    string
		rollout *argo.Rollout
		rc      *RemoteController
		result  map[string]*WeightedService
	}{
		{
			name:    "canaryRolloutNoLabelMatch",
			rollout: &bgRolloutNs1,
			rc:      rc,
			result:  make(map[string]*WeightedService, 0),
		}, {
			name:    "canaryRolloutNoStratergy",
			rollout: &noStratergyRollout,
			rc:      rc,
			result:  make(map[string]*WeightedService, 0),
		}, {
			name:    "canaryRolloutHappyCase",
			rollout: &bgRollout,
			rc:      rc,
			result:  resultForBlueGreen,
		}, {
			name:    "rolloutWithNoActiveService",
			rollout: &bgRolloutNoActiveService,
			rc:      rc,
			result:  resultForNoActiveService,
		},
		{
			name:    "canaryRolloutNilRollout",
			rollout: nil,
			rc:      rc,
			result:  make(map[string]*WeightedService, 0),
		},
		{
			name:    "canaryRolloutEmptyServiceCache",
			rollout: &bgRollout,
			rc: &RemoteController{
				ServiceController: emptyCacheService,
			},
			result: make(map[string]*WeightedService, 0),
		},
	}

	ctx := context.Background()

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getServiceForRollout(ctx, c.rc, c.rollout)
			if len(c.result) == 0 {
				if len(result) > 0 {
					t.Fatalf("Service expected to be nil")
				}
			} else {
				for key, service := range c.result {
					if val, ok := result[key]; ok {
						if !cmp.Equal(val.Service.Name, service.Service.Name) {
							t.Fatalf("Service Mismatch. Diff: %v", cmp.Diff(val.Service.Name, service.Service.Name))
						}
					} else {
						t.Fatalf("Expected a service with name=%s but none returned", key)
					}
				}
			}
		})
	}
}

func TestSkipDestructiveUpdate(t *testing.T) {
	twoEndpointSe := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global-west", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global-east", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}

	twoEndpointSeUpdated := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global-west", Ports: map[string]uint32{"http": 90}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global-east", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}

	oneEndpointSe := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global-west", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		},
	}

	newSeTwoEndpoints := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "random"},
		//nolint
		Spec: twoEndpointSe,
	}

	newSeTwoEndpointsUpdated := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "random"},
		//nolint
		Spec: twoEndpointSeUpdated,
	}

	newSeOneEndpoint := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "random"},
		//nolint
		Spec: oneEndpointSe,
	}

	oldSeTwoEndpoints := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "random"},
		//nolint
		Spec: twoEndpointSe,
	}

	oldSeOneEndpoint := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "random"},
		//nolint
		Spec: oneEndpointSe,
	}

	rcWarmupPhase := &RemoteController{
		StartTime: time.Now(),
	}

	rcNotinWarmupPhase := &RemoteController{
		StartTime: time.Now().Add(time.Duration(-21) * time.Minute),
	}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name            string
		rc              *RemoteController
		newSe           *v1alpha32.ServiceEntry
		oldSe           *v1alpha32.ServiceEntry
		skipDestructive bool
		diff            string
	}{
		{
			name:            "Should return false when in warm up phase but not destructive",
			rc:              rcWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           oldSeOneEndpoint,
			skipDestructive: false,
			diff:            "",
		},
		{
			name:            "Should return true when in warm up phase but is destructive",
			rc:              rcWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: true,
			diff:            "Delete",
		},
		{
			name:            "Should return false when not in warm up phase but is destructive",
			rc:              rcNotinWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: false,
			diff:            "Delete",
		},
		{
			name:            "Should return false when in warm up phase but is constructive",
			rc:              rcWarmupPhase,
			newSe:           newSeTwoEndpoints,
			oldSe:           oldSeOneEndpoint,
			skipDestructive: false,
			diff:            "Add",
		},
		{
			name:            "Should return false when not in warm up phase but endpoints updated",
			rc:              rcNotinWarmupPhase,
			newSe:           newSeTwoEndpointsUpdated,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: false,
			diff:            "Update",
		},
		{
			name:            "Should return true when in warm up phase but endpoints are updated (destructive)",
			rc:              rcWarmupPhase,
			newSe:           newSeTwoEndpointsUpdated,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: true,
			diff:            "Update",
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			skipDestructive, diff := skipDestructiveUpdate(c.rc, c.newSe, c.oldSe)
			if skipDestructive == c.skipDestructive {
				//perfect
			} else {
				t.Errorf("Result Failed. Got %v, expected %v", skipDestructive, c.skipDestructive)
			}
			if c.diff == "" || (c.diff != "" && strings.Contains(diff, c.diff)) {
				//perfect
			} else {
				t.Errorf("Diff Failed. Got %v, expected %v", diff, c.diff)
			}
		})
	}
}

func TestAddUpdateServiceEntry(t *testing.T) {
	var (
		ctx             = context.Background()
		fakeIstioClient = istioFake.NewSimpleClientset()
		seCtrl          = &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
		}
	)
	twoEndpointSe := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global-west", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global-east", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}

	oneEndpointSe := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global-west", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		},
	}

	invalidEndpoint := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.test-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "test.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}

	invalidEndpointSe := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se3", Namespace: "namespace"},
		//nolint
		Spec: invalidEndpoint,
	}

	newSeOneEndpoint := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "namespace"},
		//nolint
		Spec: oneEndpointSe,
	}

	oldSeTwoEndpoints := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se2", Namespace: "namespace"},
		//nolint
		Spec: twoEndpointSe,
	}

	_, err := seCtrl.IstioClient.NetworkingV1alpha3().ServiceEntries("namespace").Create(ctx, oldSeTwoEndpoints, metaV1.CreateOptions{})
	if err != nil {
		t.Error(err)
	}

	rcWarmupPhase := &RemoteController{
		ServiceEntryController: seCtrl,
		StartTime:              time.Now(),
	}

	rcNotInWarmupPhase := &RemoteController{
		ServiceEntryController: seCtrl,
		StartTime:              time.Now().Add(time.Duration(-21) * time.Minute),
	}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name            string
		rc              *RemoteController
		newSe           *v1alpha32.ServiceEntry
		oldSe           *v1alpha32.ServiceEntry
		skipDestructive bool
	}{
		{
			name:            "Should add a new SE",
			rc:              rcWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           nil,
			skipDestructive: false,
		},
		{
			name:            "Should not update SE when in warm up mode and the update is destructive",
			rc:              rcWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: true,
		},
		{
			name:            "Should update an SE",
			rc:              rcNotInWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: false,
		},
		{
			name:            "Should create an SE with one endpoint",
			rc:              rcNotInWarmupPhase,
			newSe:           invalidEndpointSe,
			oldSe:           nil,
			skipDestructive: false,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			addUpdateServiceEntry(ctx, c.newSe, c.oldSe, "namespace", c.rc)
			if c.skipDestructive {
				//verify the update did not go through
				se, err := c.rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries("namespace").Get(ctx, c.oldSe.Name, metaV1.GetOptions{})
				if err != nil {
					t.Error(err)
				}
				_, diff := getServiceEntryDiff(c.oldSe, se)
				if diff != "" {
					t.Errorf("Failed. Got %v, expected %v", se.Spec.String(), c.oldSe.Spec.String())
				}
			}
		})
	}
}

func TestValidateServiceEntryEndpoints(t *testing.T) {

	twoValidEndpoints := []*v1alpha3.WorkloadEntry{
		{Address: "valid1.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		{Address: "valid2.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
	}

	oneValidEndpoints := []*v1alpha3.WorkloadEntry{
		{Address: "valid1.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
	}

	dummyEndpoints := []*v1alpha3.WorkloadEntry{
		{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
	}

	validAndInvalidEndpoints := []*v1alpha3.WorkloadEntry{
		{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		{Address: "valid2.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
	}

	twoValidEndpointsSe := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "twoValidEndpointsSe", Namespace: "namespace"},
		Spec: v1alpha3.ServiceEntry{
			Hosts:     []string{"e2e.my-first-service.mesh"},
			Addresses: []string{"240.10.1.1"},
			Ports: []*v1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
				Name: "http", Protocol: "http"}},
			Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
			Resolution:      v1alpha3.ServiceEntry_DNS,
			SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
			Endpoints:       twoValidEndpoints,
		},
	}

	oneValidEndpointsSe := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "twoValidEndpointsSe", Namespace: "namespace"},
		Spec: v1alpha3.ServiceEntry{
			Hosts:     []string{"e2e.my-first-service.mesh"},
			Addresses: []string{"240.10.1.1"},
			Ports: []*v1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
				Name: "http", Protocol: "http"}},
			Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
			Resolution:      v1alpha3.ServiceEntry_DNS,
			SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
			Endpoints:       oneValidEndpoints,
		},
	}

	dummyEndpointsSe := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "twoValidEndpointsSe", Namespace: "namespace"},
		Spec: v1alpha3.ServiceEntry{
			Hosts:     []string{"e2e.my-first-service.mesh"},
			Addresses: []string{"240.10.1.1"},
			Ports: []*v1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
				Name: "http", Protocol: "http"}},
			Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
			Resolution:      v1alpha3.ServiceEntry_DNS,
			SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
			Endpoints:       dummyEndpoints,
		},
	}

	validAndInvalidEndpointsSe := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "twoValidEndpointsSe", Namespace: "namespace"},
		Spec: v1alpha3.ServiceEntry{
			Hosts:     []string{"e2e.my-first-service.mesh"},
			Addresses: []string{"240.10.1.1"},
			Ports: []*v1alpha3.Port{{Number: uint32(common.DefaultServiceEntryPort),
				Name: "http", Protocol: "http"}},
			Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
			Resolution:      v1alpha3.ServiceEntry_DNS,
			SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
			Endpoints:       validAndInvalidEndpoints,
		},
	}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name                      string
		serviceEntry              *v1alpha32.ServiceEntry
		expectedAreEndpointsValid bool
		expectedValidEndpoints    []*v1alpha3.WorkloadEntry
	}{
		{
			name:                      "Validate SE with dummy endpoint",
			serviceEntry:              dummyEndpointsSe,
			expectedAreEndpointsValid: false,
			expectedValidEndpoints:    []*v1alpha3.WorkloadEntry{},
		},
		{
			name:                      "Validate SE with valid endpoint",
			serviceEntry:              oneValidEndpointsSe,
			expectedAreEndpointsValid: true,
			expectedValidEndpoints:    []*v1alpha3.WorkloadEntry{{Address: "valid1.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"}},
		},
		{
			name:                      "Validate endpoint with multiple valid endpoints",
			serviceEntry:              twoValidEndpointsSe,
			expectedAreEndpointsValid: true,
			expectedValidEndpoints: []*v1alpha3.WorkloadEntry{
				{Address: "valid1.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
				{Address: "valid2.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"}},
		},
		{
			name:                      "Validate endpoint with mix of valid and dummy endpoints",
			serviceEntry:              validAndInvalidEndpointsSe,
			expectedAreEndpointsValid: false,
			expectedValidEndpoints: []*v1alpha3.WorkloadEntry{
				{Address: "valid2.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"}},
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			areValidEndpoints := validateAndProcessServiceEntryEndpoints(c.serviceEntry)

			if areValidEndpoints != c.expectedAreEndpointsValid {
				t.Errorf("Failed. Got %v, expected %v", areValidEndpoints, c.expectedAreEndpointsValid)
			}

			if len(c.serviceEntry.Spec.Endpoints) != len(c.expectedValidEndpoints) {
				t.Errorf("Failed. Got %v, expected %v", len(c.serviceEntry.Spec.Endpoints), len(c.expectedValidEndpoints))
			}
		})
	}
}
