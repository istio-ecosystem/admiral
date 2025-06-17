package clusters

import (
	"context"
	"errors"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/client-go/rest"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"

	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	log "github.com/sirupsen/logrus"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	"istio.io/api/networking/v1alpha3"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	v1alpha32 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioFake "istio.io/client-go/pkg/clientset/versioned/fake"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	fakenetworkingv1alpha3 "istio.io/client-go/pkg/clientset/versioned/typed/networking/v1alpha3/fake"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRetryUpdatingDR(t *testing.T) {
	// Create a mock logger
	logger := log.New()
	admiralParams := common.AdmiralParams{
		LabelSet:      &common.LabelSet{},
		SyncNamespace: "test-sync-ns",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	//Create a context with timeout for testing
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	admiralParams = common.GetAdmiralParams()
	log.Info("admiralSyncNS: " + admiralParams.SyncNamespace)
	// Create mock objects

	exist := &v1alpha32.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: admiralParams.SyncNamespace,
			Name:      "test-serviceentry-seRetriesTest",
			Annotations: map[string]string{
				"admiral.istio.io/ignore": "true",
			},
			ResourceVersion: "12345",
		},
		Spec: v1alpha3.DestinationRule{
			Host: "test-host",
		},
	}
	namespace := admiralParams.SyncNamespace
	rc := &RemoteController{
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: istioFake.NewSimpleClientset(),
		},
	}

	_, err := rc.DestinationRuleController.IstioClient.
		NetworkingV1alpha3().
		DestinationRules(namespace).
		Create(ctx, exist, metaV1.CreateOptions{})
	if err != nil {
		t.Error(err)
	}
	errConflict := k8sErrors.NewConflict(schema.GroupResource{}, "", nil)
	errOther := errors.New("Some other error")

	// Test when err is nil
	err = retryUpdatingDR(logger.WithField("test", "success"), ctx, exist, namespace, rc, nil)
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}

	// get the SE here, it should still have the old resource version.
	se, err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Get(ctx, exist.Name, metaV1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "12345", se.ObjectMeta.ResourceVersion)

	// Test when err is a conflict error
	err = retryUpdatingDR(logger.WithField("test", "conflict"), ctx, exist, namespace, rc, errConflict)
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}

	// get the SE and the resourceVersion should have been updated to 12345
	se, err = rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(admiralParams.SyncNamespace).Get(ctx, exist.Name, metaV1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "12345", se.ObjectMeta.ResourceVersion)

	// Test when err is a non-conflict error
	err = retryUpdatingDR(logger.WithField("test", "error"), ctx, exist, namespace, rc, errOther)
	if err == nil {
		t.Error("Expected non-nil error, got nil")
	}
}

func TestRetryUpdatingDROnConflict(t *testing.T) {
	// Create a mock logger
	logger := log.New()
	admiralParams := common.AdmiralParams{
		LabelSet:      &common.LabelSet{},
		SyncNamespace: "test-sync-ns",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	//Create a context with timeout for testing
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	admiralParams = common.GetAdmiralParams()
	log.Info("admiralSyncNS: " + admiralParams.SyncNamespace)

	// Create mock objects
	existVersion := "12345"
	exist := &v1alpha32.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: admiralParams.SyncNamespace,
			Name:      "test-serviceentry-seRetriesTest",
			Annotations: map[string]string{
				"admiral.istio.io/ignore": "true",
			},
			ResourceVersion: existVersion,
		},
		Spec: v1alpha3.DestinationRule{
			Host: "test-host",
		},
	}
	namespace := admiralParams.SyncNamespace
	rc := &RemoteController{
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: istioFake.NewSimpleClientset(),
		},
	}

	_, err := rc.DestinationRuleController.IstioClient.
		NetworkingV1alpha3().
		DestinationRules(namespace).
		Create(ctx, exist, metaV1.CreateOptions{})
	if err != nil {
		t.Error(err)
	}

	updatedObj := &v1alpha32.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: admiralParams.SyncNamespace,
			Name:      "test-serviceentry-seRetriesTest",
			Annotations: map[string]string{
				"admiral.istio.io/ignore": "true",
			},
			ResourceVersion: "12344", // has different version than existing object
		},
		Spec: v1alpha3.DestinationRule{
			Host: "test-host-other",
		},
	}

	// workaround because fake client does not correctly handle resource version
	conflictErr := k8sErrors.NewConflict(schema.GroupResource{}, "", nil)
	rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().(*fakenetworkingv1alpha3.FakeNetworkingV1alpha3).PrependReactor("update", "destinationrules", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		updateAction, ok := action.(k8stesting.UpdateAction)
		if !ok {
			return false, nil, nil
		}
		drObj, ok := updateAction.GetObject().(*v1alpha32.DestinationRule)
		if !ok {
			return false, nil, nil
		}
		if drObj.ResourceVersion != existVersion {
			return true, drObj, k8sErrors.NewConflict(schema.GroupResource{}, fmt.Sprintf("object resource version actual: %s, expected: %s", drObj.ResourceVersion, existVersion), nil)
		}
		return false, nil, nil
	})
	err = retryUpdatingDR(logger.WithField("test", "conflict"), ctx, updatedObj, namespace, rc, conflictErr)
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
		t.FailNow()
	}
	// check resource is updated properly with right spec
	dr, err := rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(namespace).Get(ctx, exist.Name, metaV1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "test-host-other", dr.Spec.Host)
}

func TestGetDestinationRule(t *testing.T) {
	admiralParams := common.AdmiralParams{
		LabelSet:                  &common.LabelSet{},
		SyncNamespace:             "test-sync-ns",
		DefaultWarmupDurationSecs: 45,
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	ctxLogger := log.WithFields(log.Fields{
		"type": "destinationRule",
	})
	//Do setup here
	outlierDetection := &v1alpha3.OutlierDetection{
		BaseEjectionTime:         &duration.Duration{Seconds: 300},
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 50},
		Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
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
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	se := &v1alpha3.ServiceEntry{Hosts: []string{"qa.myservice.global"}, Endpoints: []*v1alpha3.WorkloadEntry{
		{Address: "east.com", Locality: "us-east-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}}, {Address: "west.com", Locality: "us-west-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
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
				LbPolicy:           &v1alpha3.LoadBalancerSettings_Simple{Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST},
				LocalityLbSetting:  &v1alpha3.LocalityLoadBalancerSetting{},
				WarmupDurationSecs: &duration.Duration{Seconds: 45},
			},
			OutlierDetection: outlierDetection,
			ConnectionPool: &v1alpha3.ConnectionPoolSettings{
				Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
					MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
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
				WarmupDurationSecs: &duration.Duration{Seconds: 45},
			},
			OutlierDetection: outlierDetection,
			ConnectionPool: &v1alpha3.ConnectionPoolSettings{
				Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
					MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
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
			result := getDestinationRule(c.se, c.locality, c.gtpPolicy, nil, nil, nil, common.GTP, ctxLogger, admiral.Add, false)
			if !cmp.Equal(result, c.destinationRule, protocmp.Transform()) {
				t.Fatalf("DestinationRule Mismatch. Diff: %v", cmp.Diff(result, c.destinationRule, protocmp.Transform()))
			}
		})
	}
}

func TestGetDestinationRuleActivePassive(t *testing.T) {
	ctxLogger := log.WithFields(log.Fields{
		"type": "destinationRule",
	})
	// Enable Active-Passive
	admiralParams := common.AdmiralParams{
		CacheReconcileDuration: 10 * time.Minute,
		LabelSet: &common.LabelSet{
			EnvKey: "env",
		},
		DefaultWarmupDurationSecs: 45,
	}
	admiralParams.EnableActivePassive = true
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	mTLSWestNoDistribution := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	mTLSWest := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{
				Distribute: []*v1alpha3.LocalityLoadBalancerSetting_Distribute{
					{
						From: "*",
						To:   map[string]uint32{"us-west-2": 100},
					},
				},
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	mTLSAAWest := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	mTLSWestAfterGTP := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{
				Distribute: []*v1alpha3.LocalityLoadBalancerSetting_Distribute{
					{
						From: "us-west-2/*",
						To:   map[string]uint32{"us-west-2": 70, "us-east-2": 30},
					},
				},
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	mTLSSingleEndpointWest := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{
				Distribute: []*v1alpha3.LocalityLoadBalancerSetting_Distribute{
					{
						From: "*",
						To:   map[string]uint32{"us-west-2": 100},
					},
				},
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	mTLSEast := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{
				Distribute: []*v1alpha3.LocalityLoadBalancerSetting_Distribute{
					{
						From: "*",
						To:   map[string]uint32{"us-east-2": 100},
					},
				},
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	mTLSEastNoLocalityLbSetting := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	seSingleEndpoint := &v1alpha3.ServiceEntry{
		Hosts: []string{"qa.myservice.global"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "west.com", Locality: "us-west-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
		}}

	noGtpDrSingleEndpoint := v1alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLSSingleEndpointWest,
	}

	noGtpDrInCacheSingleEndpointWest := v1alpha32.DestinationRule{
		Spec: v1alpha3.DestinationRule{
			Host:          "qa.myservice.global",
			TrafficPolicy: mTLSWest,
		},
	}

	noGtpAADrInCacheSingleEndpointWest := v1alpha32.DestinationRule{
		Spec: v1alpha3.DestinationRule{
			Host:          "qa.myservice.global",
			TrafficPolicy: mTLSAAWest,
		},
	}

	noGtpDrInCacheSingleEndpointEast := v1alpha32.DestinationRule{
		Spec: v1alpha3.DestinationRule{
			Host:          "qa.myservice.global",
			TrafficPolicy: mTLSEast,
		},
	}

	outlierDetectionSingleEndpoint := &v1alpha3.OutlierDetection{
		BaseEjectionTime:         &duration.Duration{Seconds: 300},
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 50},
		Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
		Interval:                 &duration.Duration{Seconds: 60},
		MaxEjectionPercent:       33,
	}

	noGtpDrSingleEndpoint.TrafficPolicy.OutlierDetection = outlierDetectionSingleEndpoint

	seMultipleEndpoint := &v1alpha3.ServiceEntry{
		Hosts: []string{"qa.myservice.global"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "east.com", Locality: "us-east-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
			{Address: "west.com", Locality: "us-west-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
		}}

	noGtpDrMultipleEndpointWest := v1alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLSWest,
	}

	noGtpDrMultipleEndpointDeleteWest := v1alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLSWestNoDistribution,
	}

	noGtpDrMultipleEndpointEast := v1alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLSEast,
	}

	noGtpDrMultipleEndpointEastNoLocalityLbSetting := v1alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLSEastNoLocalityLbSetting,
	}

	DrWithGTPMultipleEndpointWest := v1alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLSWestAfterGTP,
	}

	outlierDetectionMultipleEndpoint := &v1alpha3.OutlierDetection{
		BaseEjectionTime:         &duration.Duration{Seconds: 300},
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 50},
		Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
		Interval:                 &duration.Duration{Seconds: 60},
		MaxEjectionPercent:       100,
	}

	noGtpDrMultipleEndpointWest.TrafficPolicy.OutlierDetection = outlierDetectionMultipleEndpoint
	noGtpDrMultipleEndpointEast.TrafficPolicy.OutlierDetection = outlierDetectionMultipleEndpoint
	DrWithGTPMultipleEndpointWest.TrafficPolicy.OutlierDetection = outlierDetectionMultipleEndpoint
	noGtpDrMultipleEndpointDeleteWest.TrafficPolicy.OutlierDetection = outlierDetectionMultipleEndpoint
	noGtpDrMultipleEndpointEastNoLocalityLbSetting.TrafficPolicy.OutlierDetection = outlierDetectionMultipleEndpoint

	GTPPolicy := &model.TrafficPolicy{
		LbType: model.TrafficPolicy_FAILOVER,
		Target: []*model.TrafficGroup{
			{
				Region: "us-west-2",
				Weight: 70,
			},
			{
				Region: "us-east-2",
				Weight: 30,
			},
		},
	}

	GTPPolicyNoTargets := &model.TrafficPolicy{
		LbType: model.TrafficPolicy_TOPOLOGY,
	}

	testCases := []struct {
		name                    string
		se                      *v1alpha3.ServiceEntry
		locality                string
		gtpPolicy               *model.TrafficPolicy
		destinationRuleInCache  *v1alpha32.DestinationRule
		eventResourceType       string
		eventType               admiral.EventType
		expectedDestinationRule *v1alpha3.DestinationRule
	}{
		{
			name: "Given the application is onboarding for the first time in west" +
				"And the DR cache does not have this entry" +
				"And there is no GTP" +
				"Then the DR should have the traffic distribution set to 100% to west",
			se:                      seSingleEndpoint,
			locality:                "us-west-2",
			gtpPolicy:               nil,
			destinationRuleInCache:  nil,
			eventResourceType:       common.Deployment,
			eventType:               admiral.Add,
			expectedDestinationRule: &noGtpDrSingleEndpoint,
		},

		{
			name: "Given the application is is Active-Passive and only in one region" +
				"And the DR cache does have this entry" +
				"And there is no GTP" +
				"Then the DR should have the traffic distribution as it was before",
			se:                      seSingleEndpoint,
			locality:                "us-west-2",
			gtpPolicy:               nil,
			destinationRuleInCache:  &noGtpDrInCacheSingleEndpointWest,
			eventResourceType:       common.Deployment,
			eventType:               admiral.Add,
			expectedDestinationRule: &noGtpDrSingleEndpoint,
		},

		{
			name: "Given the application is Active-Active and only in west region" +
				"And the DR cache does have this entry" +
				"And there is no GTP" +
				"Then the DR should have the traffic distribution set to 100% to west",
			se:                      seSingleEndpoint,
			locality:                "us-west-2",
			gtpPolicy:               nil,
			destinationRuleInCache:  &noGtpAADrInCacheSingleEndpointWest,
			eventResourceType:       common.Deployment,
			eventType:               admiral.Add,
			expectedDestinationRule: &noGtpDrSingleEndpoint,
		},
		{
			name: "Given the application is onboarding to east region" +
				"And was first onboarded to west" +
				"And the DR cache does have an entry" +
				"And there is no GTP" +
				"Then the DR should still have the traffic distribution set to 100% to west",
			se:                      seMultipleEndpoint,
			locality:                "us-west-2",
			gtpPolicy:               nil,
			destinationRuleInCache:  &noGtpDrInCacheSingleEndpointWest,
			eventResourceType:       common.Deployment,
			eventType:               admiral.Add,
			expectedDestinationRule: &noGtpDrMultipleEndpointWest,
		},
		{
			name: "Given the application is onboarding to west region" +
				"And was first onboarded to east" +
				"And the DR cache does have an entry" +
				"And there is no GTP" +
				"Then the DR should still have the traffic distribution set to 100% to east",
			se:                      seMultipleEndpoint,
			locality:                "us-west-2",
			gtpPolicy:               nil,
			destinationRuleInCache:  &noGtpDrInCacheSingleEndpointEast,
			eventResourceType:       common.Deployment,
			eventType:               admiral.Add,
			expectedDestinationRule: &noGtpDrMultipleEndpointEast,
		},
		{
			name: "Given the application is onboarding to west region" +
				"And was first onboarded to east" +
				"And the DR cache does have an entry" +
				"And there is a GTP being applied" +
				"Then the DR should still have the traffic distribution set to that defined by the GTP",
			se:                      seMultipleEndpoint,
			locality:                "us-west-2",
			gtpPolicy:               GTPPolicy,
			destinationRuleInCache:  &noGtpDrInCacheSingleEndpointWest,
			eventResourceType:       common.Deployment,
			eventType:               admiral.Add,
			expectedDestinationRule: &DrWithGTPMultipleEndpointWest,
		},
		{
			name: "Given the application is onboarding to west region" +
				"And was first onboarded to east" +
				"And the DR cache does have an entry" +
				"And there is a GTP being applied with no targets" +
				"Then the DR should change to Active-Active behavior",
			se:                      seMultipleEndpoint,
			locality:                "us-west-2",
			gtpPolicy:               GTPPolicyNoTargets,
			destinationRuleInCache:  &noGtpDrInCacheSingleEndpointWest,
			eventResourceType:       common.Deployment,
			eventType:               admiral.Add,
			expectedDestinationRule: &noGtpDrMultipleEndpointEastNoLocalityLbSetting,
		},
		{
			name: "Given the application is onboarding to west region" +
				"And was first onboarded to east" +
				"And the DR cache does have an entry" +
				"And there is a GTP being applied with no targets" +
				"Then the DR should change to Active-Active behavior",
			se:                      seMultipleEndpoint,
			locality:                "us-west-2",
			gtpPolicy:               GTPPolicyNoTargets,
			destinationRuleInCache:  &noGtpDrInCacheSingleEndpointWest,
			eventResourceType:       common.GTP,
			eventType:               admiral.Add,
			expectedDestinationRule: &noGtpDrMultipleEndpointEastNoLocalityLbSetting,
		},
		{
			name: "Given the application is onboarding to west region" +
				"And was first onboarded to east" +
				"And the DR cache does have an entry" +
				"And there is a GTP being applied with no targets" +
				"Then the DR should change to Active-Active behavior",
			se:                      seMultipleEndpoint,
			locality:                "us-west-2",
			gtpPolicy:               GTPPolicyNoTargets,
			destinationRuleInCache:  &noGtpDrInCacheSingleEndpointWest,
			eventResourceType:       common.GTP,
			eventType:               admiral.Update,
			expectedDestinationRule: &noGtpDrMultipleEndpointEastNoLocalityLbSetting,
		},
		{
			name: "Given the application is onboarding to west region" +
				"And was first onboarded to east" +
				"And the DR cache does have an entry" +
				"And the GTP is being deleted" +
				"Then the DR should not have any traffic distribution set",
			se:                      seMultipleEndpoint,
			locality:                "us-west-2",
			gtpPolicy:               nil,
			destinationRuleInCache:  &noGtpDrInCacheSingleEndpointWest,
			eventResourceType:       common.GTP,
			eventType:               admiral.Delete,
			expectedDestinationRule: &noGtpDrMultipleEndpointDeleteWest,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getDestinationRule(c.se, c.locality, c.gtpPolicy, nil, nil, c.destinationRuleInCache, c.eventResourceType, ctxLogger, c.eventType, false)
			if !cmp.Equal(result, c.expectedDestinationRule, protocmp.Transform()) {
				t.Fatalf("DestinationRule Mismatch. Diff: %v", cmp.Diff(result, c.expectedDestinationRule, protocmp.Transform()))
			}
		})
	}
}

func TestCalculateDistribution(t *testing.T) {
	mTLSWest := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{
				Distribute: []*v1alpha3.LocalityLoadBalancerSetting_Distribute{
					{
						From: "*",
						To:   map[string]uint32{"us-west-2": 100},
					},
				},
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	mTLSWestNoDistribution := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
	}

	dRInCache := v1alpha32.DestinationRule{
		Spec: v1alpha3.DestinationRule{
			Host:          "qa.myservice.global",
			TrafficPolicy: mTLSWest,
		},
	}

	dRInCacheNoDistribution := v1alpha32.DestinationRule{
		Spec: v1alpha3.DestinationRule{
			Host:          "qa.myservice.global",
			TrafficPolicy: mTLSWestNoDistribution,
		},
	}

	seSingleEndpoint := &v1alpha3.ServiceEntry{
		Hosts: []string{"qa.myservice.global"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "west.com", Locality: "us-west-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
		}}

	singleEndpointDistribution := []*v1alpha3.LocalityLoadBalancerSetting_Distribute{
		{From: "*",
			To: map[string]uint32{"us-west-2": 100},
		},
	}

	seMultipleEndpoint := &v1alpha3.ServiceEntry{
		Hosts: []string{"qa.myservice.global"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "east.com", Locality: "us-east-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
			{Address: "west.com", Locality: "us-west-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
		}}

	multipleEndpointDistribution := []*v1alpha3.LocalityLoadBalancerSetting_Distribute{
		{From: "*",
			To: map[string]uint32{"us-west-2": 100},
		},
	}

	seDeleted := &v1alpha3.ServiceEntry{
		Hosts: []string{"qa.myservice.global"},
	}

	testCases := []struct {
		name                   string
		se                     *v1alpha3.ServiceEntry
		destinationRuleInCache *v1alpha32.DestinationRule
		expectedDistribution   []*v1alpha3.LocalityLoadBalancerSetting_Distribute
	}{
		{
			name: "Given the SE of the application is only present in 1 region" +
				"And this is a new application" +
				"And the locality for that west" +
				"Then the traffic distribution should be set to 100% to west",
			se:                     seSingleEndpoint,
			destinationRuleInCache: nil,
			expectedDistribution:   singleEndpointDistribution,
		},
		{
			name: "Given the SE of the application is only present in 1 region" +
				"And the locality for that west" +
				"And is currently Active-Active" +
				"Then the traffic distribution should be set to 100% to west",
			se:                     seSingleEndpoint,
			destinationRuleInCache: &dRInCacheNoDistribution,
			expectedDistribution:   singleEndpointDistribution,
		},
		{
			name: "Given the SE of the application is only present in 1 region" +
				"And the locality for that west" +
				"And is currently Active-Passive" +
				"Then the traffic distribution should be set to 100% to west",
			se:                     seSingleEndpoint,
			destinationRuleInCache: &dRInCache,
			expectedDistribution:   singleEndpointDistribution,
		},
		{
			name: "Given the SE of the application is present in multiple regions" +
				"And the DR is present in the cache" +
				"Then the traffic distribution should be set what is present in the cache",
			se:                     seMultipleEndpoint,
			destinationRuleInCache: &dRInCache,
			expectedDistribution:   multipleEndpointDistribution,
		},
		{
			name: "Given the SE of the application is present in multiple regions" +
				"And the DR is not present in cache" +
				"Then the traffic distribution should be set to empty",
			se:                     seMultipleEndpoint,
			destinationRuleInCache: nil,
			expectedDistribution:   make([]*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute, 0),
		},
		{
			name: "Given the SE of the application is present in multiple regions" +
				"And the DR is present in the cache but no distribution is set" +
				"Then the traffic distribution should be set to empty",
			se:                     seMultipleEndpoint,
			destinationRuleInCache: &dRInCacheNoDistribution,
			expectedDistribution:   make([]*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute, 0),
		},
		{
			name: "Given the application is being deleted" +
				"Then the traffic distribution should be set to empty",
			se:                     seDeleted,
			destinationRuleInCache: &dRInCache,
			expectedDistribution:   make([]*networkingV1Alpha3.LocalityLoadBalancerSetting_Distribute, 0),
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := calculateDistribution(c.se, c.destinationRuleInCache)
			if !cmp.Equal(result, c.expectedDistribution, protocmp.Transform()) {
				t.Fatalf("Distribution Mismatch. Diff: %v", cmp.Diff(result, c.expectedDistribution, protocmp.Transform()))
			}
		})
	}
}

func TestGetOutlierDetection(t *testing.T) {
	outlierDetectionDisabledSpec := &v1alpha3.OutlierDetection{
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 0},
		Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
	}
	outlierDetectionFromGTP := &v1alpha3.OutlierDetection{
		BaseEjectionTime:         &duration.Duration{Seconds: 100},
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 100},
		Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: DefaultConsecutive5xxErrors},
		Interval:                 &duration.Duration{Seconds: 100},
		MaxEjectionPercent:       100,
	}

	outlierDetectionFromOutlierCRD := &v1alpha3.OutlierDetection{
		BaseEjectionTime:         &duration.Duration{Seconds: 10},
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 10},
		Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: DefaultConsecutive5xxErrors},
		Interval:                 &duration.Duration{Seconds: 10},
		MaxEjectionPercent:       100,
	}

	outlierDetectionWithRemoteHostUsingGTP := &v1alpha3.OutlierDetection{
		BaseEjectionTime:         &duration.Duration{Seconds: 100},
		ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 100},
		Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: DefaultConsecutive5xxErrors},
		Interval:                 &duration.Duration{Seconds: 100},
		MaxEjectionPercent:       33,
	}

	gtpPolicyWithOutlierDetection := &model.TrafficPolicy{
		OutlierDetection: &model.TrafficPolicy_OutlierDetection{
			BaseEjectionTime:         100,
			ConsecutiveGatewayErrors: 100,
			Interval:                 100,
		},
	}

	se := &v1alpha3.ServiceEntry{Hosts: []string{"qa.myservice.global"}, Endpoints: []*v1alpha3.WorkloadEntry{
		{Address: "east.com", Locality: "us-east-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}}, {Address: "west.com", Locality: "us-west-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
	}}

	seOneHostRemote := &v1alpha3.ServiceEntry{Hosts: []string{"qa.myservice.global"}, Endpoints: []*v1alpha3.WorkloadEntry{
		{Address: "east.com", Locality: "us-east-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
	}}

	seOneHostLocal := &v1alpha3.ServiceEntry{Hosts: []string{"qa.myservice.global"}, Endpoints: []*v1alpha3.WorkloadEntry{
		{Address: "hello.ns.svc.cluster.local", Locality: "us-east-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
	}}

	seOneHostRemoteIp := &v1alpha3.ServiceEntry{Hosts: []string{"qa.myservice.global"}, Endpoints: []*v1alpha3.WorkloadEntry{
		{Address: "95.45.25.34", Locality: "us-east-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
	}}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name                            string
		se                              *v1alpha3.ServiceEntry
		gtpPolicy                       *model.TrafficPolicy
		expectedOutlierDetection        *v1alpha3.OutlierDetection
		admiralOutlierDetectionCRD      *v1.OutlierDetection
		disableDefaultAutomaticFailover bool
	}{
		{
			name: "Given both outlier detection and global traffic policy exists, " +
				"When GTP contains configurations for outlier detection, " +
				"When both specs are passed to the function, " +
				"Then outlier configurations should be derived from outlier detection, " +
				"and not from global traffic policy",
			se:                       se,
			gtpPolicy:                gtpPolicyWithOutlierDetection,
			expectedOutlierDetection: outlierDetectionFromOutlierCRD,
			admiralOutlierDetectionCRD: &v1.OutlierDetection{
				TypeMeta:   metaV1.TypeMeta{},
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: model.OutlierDetection{
					OutlierConfig: &model.OutlierConfig{
						BaseEjectionTime:         10,
						ConsecutiveGatewayErrors: 10,
						Interval:                 10,
					},
				},
				Status: v1.OutlierDetectionStatus{},
			},
		},
		{
			name: "Given outlier detection policy exists, " +
				"And there is no GTP policy, " +
				"Then outlier configurations should be derived from outlier detection, " +
				"and not from global traffic policy",
			se:                       se,
			gtpPolicy:                nil,
			expectedOutlierDetection: outlierDetectionFromOutlierCRD,
			admiralOutlierDetectionCRD: &v1.OutlierDetection{
				TypeMeta:   metaV1.TypeMeta{},
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: model.OutlierDetection{
					OutlierConfig: &model.OutlierConfig{
						BaseEjectionTime:         10,
						ConsecutiveGatewayErrors: 10,
						Interval:                 10,
					},
				},
				Status: v1.OutlierDetectionStatus{},
			},
		},
		{
			name: "Given an asset is deployed only in one region, " +
				"And, a GTP exists for this asset, " +
				"And the associated service entry only has the local endpoint, " +
				"When the function is called, " +
				"Then, it should not return any outlier configuration",
			se:                         seOneHostLocal,
			gtpPolicy:                  gtpPolicyWithOutlierDetection,
			expectedOutlierDetection:   nil,
			admiralOutlierDetectionCRD: nil,
		},
		{
			name: "Given an asset is deployed only in one region, " +
				"And, a GTP exists for this asset, " +
				"And the associated service entry only has the remote IP endpoint, " +
				"When the function is called, " +
				"Then, it should not return any outlier configuration",
			se:                         seOneHostRemoteIp,
			gtpPolicy:                  gtpPolicyWithOutlierDetection,
			expectedOutlierDetection:   nil,
			admiralOutlierDetectionCRD: nil,
		},
		{
			name: "Given an asset is deployed only in one region, " +
				"And the associated service entry has an endpoint, which is neither an IP nor a local endpoint, " +
				"Then the the max ejection percentage should be set to 33%",
			se:                         seOneHostRemote,
			gtpPolicy:                  gtpPolicyWithOutlierDetection,
			expectedOutlierDetection:   outlierDetectionWithRemoteHostUsingGTP,
			admiralOutlierDetectionCRD: nil,
		},
		{
			name: "Given an asset is deployed in two regions, " +
				"And the associated service entry has two endpoints, " +
				"Then the max ejection percentage should be set to 100%",
			se:                         se,
			gtpPolicy:                  gtpPolicyWithOutlierDetection,
			expectedOutlierDetection:   outlierDetectionFromGTP,
			admiralOutlierDetectionCRD: nil,
		},
		{
			name: "Given there is neither outlier custom resource, nor any GTP for a given asset, " +
				"And default automatic failover is not enabled, " +
				"Then, the outlier detection property should exist but should be empty",
			se:                              se,
			gtpPolicy:                       nil,
			expectedOutlierDetection:        outlierDetectionDisabledSpec,
			admiralOutlierDetectionCRD:      nil,
			disableDefaultAutomaticFailover: true,
		},
		{
			name: "Given there is neither outlier custom resource, nor any GTP for a given asset, " +
				"And default automatic failover is not disabled, " +
				"Then, the outlier detection should return with default values",
			se:        se,
			gtpPolicy: nil,
			expectedOutlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:         &duration.Duration{Seconds: DefaultBaseEjectionTime},
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: DefaultConsecutiveGatewayErrors},
				// The default Consecutive5XXErrors is set to 5 in envoy, setting to 0 disables 5XX error outlier detection so that ConsecutiveGatewayErrors rule can get evaluated
				Consecutive_5XxErrors: &wrappers.UInt32Value{Value: DefaultConsecutive5xxErrors},
				Interval:              &duration.Duration{Seconds: DefaultInterval},
				MaxEjectionPercent:    100,
			},
			admiralOutlierDetectionCRD:      nil,
			disableDefaultAutomaticFailover: false,
		},
		{
			name: "Given base ejection is not configured in the Global Traffic Policy, " +
				"When there is no outlier resource, " +
				"Then the default value of BaseEjectionTime should be used",
			se: se,
			gtpPolicy: &model.TrafficPolicy{
				OutlierDetection: &model.TrafficPolicy_OutlierDetection{
					ConsecutiveGatewayErrors: 10,
					Interval:                 60,
				},
			},
			expectedOutlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:         &duration.Duration{Seconds: DefaultBaseEjectionTime},
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 10},
				Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
				Interval:                 &duration.Duration{Seconds: 60},
				MaxEjectionPercent:       100,
			},
			admiralOutlierDetectionCRD: nil,
		},
		{
			name: "Given base ejection is not configured in the Global Traffic Policy, " +
				"When there is no outlier resource, " +
				"Then the default value of ConsecutiveGatewayErrors should be used",
			se: se,
			gtpPolicy: &model.TrafficPolicy{
				OutlierDetection: &model.TrafficPolicy_OutlierDetection{
					BaseEjectionTime: 600,
					Interval:         60,
				},
			},
			expectedOutlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:         &duration.Duration{Seconds: 600},
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: DefaultConsecutiveGatewayErrors},
				Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: DefaultConsecutive5xxErrors},
				Interval:                 &duration.Duration{Seconds: 60},
				MaxEjectionPercent:       100,
			},
			admiralOutlierDetectionCRD: nil,
		},
		{
			name: "Given base ejection is not configured in the Global Traffic Policy, " +
				"When there is no outlier resource, " +
				"Then the default value of Interval should be used",
			se: se,
			gtpPolicy: &model.TrafficPolicy{
				OutlierDetection: &model.TrafficPolicy_OutlierDetection{
					BaseEjectionTime:         600,
					ConsecutiveGatewayErrors: 50,
				},
			},
			expectedOutlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:         &duration.Duration{Seconds: 600},
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 50},
				Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
				Interval:                 &duration.Duration{Seconds: DefaultInterval},
				MaxEjectionPercent:       100,
			},
			admiralOutlierDetectionCRD: nil,
		},
		{
			name: "Given there is a GTP for an asset, " +
				"When the GTP contains overrides for BaseEjectionTime, ConsecutiveGatewayErrors, and Interval, " +
				"Then the overrides should be used for the outlier detection configuration",
			se: se,
			gtpPolicy: &model.TrafficPolicy{
				OutlierDetection: &model.TrafficPolicy_OutlierDetection{
					BaseEjectionTime:         600,
					ConsecutiveGatewayErrors: 10,
					Interval:                 60,
				},
			},
			expectedOutlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:         &duration.Duration{Seconds: 600},
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 10},
				Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
				Interval:                 &duration.Duration{Seconds: 60},
				MaxEjectionPercent:       100,
			},
			admiralOutlierDetectionCRD: nil,
		},
		{
			name: "Given there is a GTP for an asset, " +
				"When the GTP contains all possible overrides, " +
				"Then the Consecutive_5XxErrors should be 0",
			se: se,
			gtpPolicy: &model.TrafficPolicy{
				OutlierDetection: &model.TrafficPolicy_OutlierDetection{
					BaseEjectionTime:         600,
					ConsecutiveGatewayErrors: 10,
					Interval:                 60,
				},
			},
			expectedOutlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:         &duration.Duration{Seconds: 600},
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 10},
				Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
				Interval:                 &duration.Duration{Seconds: 60},
				MaxEjectionPercent:       100,
			},
			admiralOutlierDetectionCRD: nil,
		},
		{
			name: "Given outlier detection policy exists, " +
				"When outlier contains all possible configurations, " +
				"Then the Consecutive_5XxErrors should be 0",
			se:        se,
			gtpPolicy: nil,
			expectedOutlierDetection: &v1alpha3.OutlierDetection{
				BaseEjectionTime:         &duration.Duration{Seconds: 10},
				ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 10},
				Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
				Interval:                 &duration.Duration{Seconds: 10},
				MaxEjectionPercent:       100,
			},
			admiralOutlierDetectionCRD: &v1.OutlierDetection{
				TypeMeta:   metaV1.TypeMeta{},
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: model.OutlierDetection{
					OutlierConfig: &model.OutlierConfig{
						BaseEjectionTime:         10,
						ConsecutiveGatewayErrors: 10,
						Interval:                 10,
					},
				},
				Status: v1.OutlierDetectionStatus{},
			},
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getOutlierDetection(c.se, c.gtpPolicy, c.admiralOutlierDetectionCRD, c.disableDefaultAutomaticFailover)
			if c.expectedOutlierDetection != nil {
				assert.Equal(t, result.BaseEjectionTime, c.expectedOutlierDetection.BaseEjectionTime, "BaseEjectionTime for Outlier Detection for "+c.name)
				assert.Equal(t, result.Interval, c.expectedOutlierDetection.Interval, "Interval for Outlier Detection for "+c.name)
				assert.Equal(t, result.ConsecutiveGatewayErrors, c.expectedOutlierDetection.ConsecutiveGatewayErrors, "ConsecutiveGatewayErrors for Outlier Detection for "+c.name)
				assert.Equal(t, result.Consecutive_5XxErrors, c.expectedOutlierDetection.Consecutive_5XxErrors, "Consecutive_5XxErrors for Outlier Detection for "+c.name)
				assert.Equal(t, result.MaxEjectionPercent, c.expectedOutlierDetection.MaxEjectionPercent, "MaxEjectionPercent for Outlier Detection for "+c.name)
			} else {
				assert.Equal(t, result, c.expectedOutlierDetection)
			}
		})
	}
}

func TestDestRuleHandlerCUDScenarios(t *testing.T) {
	dr := &v1alpha32.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "my-dr",
			Namespace: "test-ns",
		},
		Spec: v1alpha3.DestinationRule{
			Host:          "e2e.blah.global",
			TrafficPolicy: &v1alpha3.TrafficPolicy{},
		},
	}

	admiralParams := common.AdmiralParams{
		LabelSet:      &common.LabelSet{},
		SyncNamespace: "test-sync-ns",
	}
	common.InitializeConfig(admiralParams)

	var (
		goodCnameCache      = common.NewMapOfMaps()
		fullFakeIstioClient = istioFake.NewSimpleClientset()
	)
	goodCnameCache.Put("e2e.blah.global", "cluster.k8s.global", "cluster.k8s.global")

	ctx := context.Background()
	r := NewRemoteRegistry(ctx, admiralParams)
	r.AdmiralCache = &AdmiralCache{
		CnameDependentClusterCache: goodCnameCache,
		SeClusterCache:             common.NewMapOfMaps(),
	}

	r.PutRemoteController("cluster.k8s.global", &RemoteController{
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fullFakeIstioClient,
		},
	})

	drHandler := &DestinationRuleHandler{
		ClusterID:      "cluster.k8s.global",
		RemoteRegistry: r,
	}

	rr := NewRemoteRegistry(ctx, admiralParams)
	rr.PutRemoteController("diff.cluster.k8s.global", &RemoteController{
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: fullFakeIstioClient,
		},
	})
	drHandler2 := &DestinationRuleHandler{
		ClusterID:      "cluster.k8s.global",
		RemoteRegistry: rr,
	}

	testcases := []struct {
		name             string
		admiralReadState bool
		ns               string
		druleHandler     *DestinationRuleHandler
	}{
		{
			name:             "Encountered non-istio resource in RW state- No dependent clusters case",
			admiralReadState: false,
			ns:               "test-ns",
			druleHandler:     drHandler2,
		},
		{
			name:             "Admiral in read-only state",
			admiralReadState: true,
			ns:               "test-ns",
			druleHandler:     drHandler,
		},
		{
			name:             "Encountered istio resource",
			admiralReadState: false,
			ns:               "istio-system",
			druleHandler:     drHandler,
		},
		{
			name:             "Encountered non-istio resource in RW state",
			admiralReadState: false,
			ns:               "test-ns",
			druleHandler:     drHandler,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			commonUtil.CurrentAdmiralState.ReadOnly = tc.admiralReadState
			dr.ObjectMeta.Namespace = tc.ns

			err := tc.druleHandler.Added(ctx, dr)
			assert.NoError(t, err)

			dr.ObjectMeta.Namespace = tc.ns
			err = tc.druleHandler.Updated(ctx, dr)
			assert.NoError(t, err)

			err = tc.druleHandler.Deleted(ctx, dr)
			assert.NoError(t, err)
		})
	}
}

func TestDestinationRuleHandlerError(t *testing.T) {
	ctxLogger := log.WithFields(log.Fields{
		"type": "destinationRule",
	})
	dr := &v1alpha32.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "my-dr",
			Namespace: "test-ns",
		},
		Spec: v1alpha3.DestinationRule{
			Host:          "env.blah.global",
			TrafficPolicy: &v1alpha3.TrafficPolicy{},
		},
	}

	admiralParams := common.AdmiralParams{
		LabelSet:      &common.LabelSet{},
		SyncNamespace: "test-sync-ns",
	}

	common.ResetSync()
	common.InitializeConfig(admiralParams)

	var (
		ctx           = context.Background()
		rr1           = NewRemoteRegistry(ctx, admiralParams)
		rr2           = NewRemoteRegistry(ctx, admiralParams)
		rr3           = NewRemoteRegistry(ctx, admiralParams)
		rr4           = NewRemoteRegistry(ctx, admiralParams)
		badCnameCache = common.NewMapOfMaps()
	)

	badCnameCache.Put("env.blah.global", "fakecluster.k8s.global", "fakecluster.k8s.global")

	rr1.AdmiralCache = &AdmiralCache{
		CnameDependentClusterCache: badCnameCache,
		SeClusterCache:             common.NewMapOfMaps(),
	}

	rr2.AdmiralCache = &AdmiralCache{
		CnameDependentClusterCache: badCnameCache,
		SeClusterCache:             common.NewMapOfMaps(),
	}
	rr2.PutRemoteController("fakecluster.k8s.global", &RemoteController{
		DestinationRuleController: nil,
	})

	rr3.PutRemoteController("fakecluster.k8s.global", nil)

	rr4.PutRemoteController("fakecluster.k8s.global", &RemoteController{
		DestinationRuleController: nil,
	})

	drHandler1 := &DestinationRuleHandler{
		ClusterID:      "fakecluster.k8s.global",
		RemoteRegistry: rr2,
	}

	drHandler2 := &DestinationRuleHandler{
		ClusterID:      "fakecluster.k8s.global",
		RemoteRegistry: rr1,
	}

	drHandler3 := &DestinationRuleHandler{
		ClusterID:      "foobar",
		RemoteRegistry: rr3,
	}

	drHandler4 := &DestinationRuleHandler{
		ClusterID:      "foobar",
		RemoteRegistry: rr4,
	}

	cases := []struct {
		name             string
		admiralReadState bool
		ns               string
		druleHandler     *DestinationRuleHandler
		expectedError    error
	}{
		{
			name:             "Destination controller for a given dependent cluster is not initialized",
			admiralReadState: false,
			ns:               "test-ns",
			druleHandler:     drHandler1,
			expectedError:    fmt.Errorf("op=Event type=DestinationRule name=my-dr cluster=fakecluster.k8s.global message=DestinationRule controller not initialized for cluster"),
		},
		{
			name:             "Remote controller for a given dependent cluster is not initialized",
			admiralReadState: false,
			ns:               "test-ns",
			druleHandler:     drHandler2,
			expectedError:    fmt.Errorf("op=Event type=DestinationRule name=my-dr cluster=fakecluster.k8s.global message=remote controller not initialized for cluster"),
		},
		{
			name:             "Remote controller for a given remote cluster is not initialized",
			admiralReadState: false,
			ns:               "test-ns",
			druleHandler:     drHandler3,
			expectedError:    fmt.Errorf("op=Event type=DestinationRule name=my-dr cluster=fakecluster.k8s.global message=remote controller not initialized for cluster"),
		},
		{
			name: "Remote controller for a given remote cluster is initialized, " +
				"And Destination controller for a given dependent cluster is not initialized",
			admiralReadState: false,
			ns:               "test-ns",
			druleHandler:     drHandler4,
			expectedError:    fmt.Errorf("op=Event type=DestinationRule name=my-dr cluster=fakecluster.k8s.global message=DestinationRule controller not initialized for cluster"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			commonUtil.CurrentAdmiralState.ReadOnly = c.admiralReadState
			dr.ObjectMeta.Namespace = c.ns
			err := handleDestinationRuleEvent(ctxLogger, ctx, dr, c.druleHandler, common.Add, common.DestinationRuleResourceType)
			if err != nil && c.expectedError == nil {
				t.Errorf("expected error to be nil but got %v", err)
			}
			if err != nil && c.expectedError != nil {
				if !(err.Error() == c.expectedError.Error()) {
					t.Errorf("error mismatch, expected %v but got %v", c.expectedError, err)
				}
			}
			if err == nil && c.expectedError != nil {
				t.Errorf("expected error %v but got %v", c.expectedError, err)
			}
		})
	}
}

func TestDeleteDestinationRule(t *testing.T) {
	ctxLogger := log.WithFields(log.Fields{
		"type": "destinationRule",
	})
	dr := &v1alpha32.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "my-dr",
			Namespace: "test-ns",
		},
		Spec: v1alpha3.DestinationRule{
			Host:          "e2e.blah.global",
			TrafficPolicy: &v1alpha3.TrafficPolicy{},
		},
	}

	admiralParams := common.AdmiralParams{
		LabelSet:      &common.LabelSet{},
		SyncNamespace: "test-sync-ns",
	}
	common.InitializeConfig(admiralParams)

	ctx := context.Background()

	rc := &RemoteController{
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: istioFake.NewSimpleClientset(),
		},
	}
	rr := NewRemoteRegistry(ctx, admiralParams)
	err := deleteDestinationRule(ctx, dr, admiralParams.SyncNamespace, rc)
	assert.Nil(t, err)

	addUpdateDestinationRule(ctxLogger, ctx, dr, nil, admiralParams.SyncNamespace, rc, rr)
	assert.Nil(t, err)

	err = deleteDestinationRule(ctx, dr, admiralParams.SyncNamespace, rc)
	assert.Nil(t, err)

	rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().(*fakenetworkingv1alpha3.FakeNetworkingV1alpha3).PrependReactor("delete", "destinationrules", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &v1alpha32.DestinationRule{}, errors.New("Error deleting destination rule")
	})
	err = deleteDestinationRule(ctx, dr, admiralParams.SyncNamespace, rc)

	assert.NotNil(t, err, "should return the error for any error apart from not found")
}

func TestAddUpdateDestinationRule(t *testing.T) {
	ctxLogger := log.WithFields(log.Fields{
		"type": "destinationRule",
	})
	dr := &v1alpha32.DestinationRule{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "my-dr",
			Namespace: "test-ns",
		},
		Spec: v1alpha3.DestinationRule{
			Host:          "e2e.blah.global",
			TrafficPolicy: &v1alpha3.TrafficPolicy{},
		},
	}

	admiralParams := common.AdmiralParams{
		LabelSet:      &common.LabelSet{},
		SyncNamespace: "test-sync-ns",
	}
	common.InitializeConfig(admiralParams)

	ctx := context.Background()

	rc := &RemoteController{
		DestinationRuleController: &istio.DestinationRuleController{
			IstioClient: istioFake.NewSimpleClientset(),
		},
	}
	rr := NewRemoteRegistry(ctx, admiralParams)
	rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().(*fakenetworkingv1alpha3.FakeNetworkingV1alpha3).PrependReactor("create", "destinationrules", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &v1alpha32.DestinationRule{}, errors.New("Error creating destination rule")
	})

	err := addUpdateDestinationRule(ctxLogger, ctx, dr, nil, admiralParams.SyncNamespace, rc, rr)
	assert.NotNil(t, err, "should return the error if not success")
}

func TestAddUpdateDestinationRule2(t *testing.T) {
	var (
		namespace = "test-ns"
		ctxLogger = log.WithFields(log.Fields{
			"type": "destinationRule",
		})
		dr = &v1alpha32.DestinationRule{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "my-dr",
				Namespace: "test-ns",
			},
			Spec: v1alpha3.DestinationRule{
				Host:          "e2e.blah.global",
				TrafficPolicy: &v1alpha3.TrafficPolicy{},
			},
		}
		ctx = context.Background()
		rc  = &RemoteController{
			ClusterID: "test-cluster",
			DestinationRuleController: &istio.DestinationRuleController{
				IstioClient: istioFake.NewSimpleClientset(),
			},
		}
		admiralParams = common.AdmiralParams{
			LabelSet:              &common.LabelSet{},
			SyncNamespace:         "test-sync-ns",
			EnableSWAwareNSCaches: true,
			ExportToIdentityList:  []string{"blah"},
			ExportToMaxNamespaces: 35,
		}
	)
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	rr := NewRemoteRegistry(ctx, admiralParams)
	rr.AdmiralCache.CnameDependentClusterNamespaceCache.Put(dr.Spec.Host, rc.ClusterID, "dep-ns", "dep-ns")
	_, err := rc.DestinationRuleController.IstioClient.
		NetworkingV1alpha3().
		DestinationRules(namespace).
		Create(ctx, dr, metaV1.CreateOptions{})
	if err != nil {
		t.Error(err)
	}

	cases := []struct {
		name       string
		newDR      *v1alpha32.DestinationRule
		existingDR *v1alpha32.DestinationRule
		expErr     error
	}{
		{
			name: "Given destinationrule does not exist, " +
				"And the existing object obtained from Get is nil, " +
				"When another thread create the destinationrule, " +
				"When this thread attempts to create destinationrule and fails, " +
				"Then, then an Update operation should be run, " +
				"And there should be no panic," +
				"And no errors should be returned",
			newDR:      dr,
			existingDR: nil,
			expErr:     nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := addUpdateDestinationRule(ctxLogger, ctx, c.newDR, c.existingDR, namespace, rc, rr)
			if c.expErr == nil {
				assert.Equal(t, c.expErr, err)
			}
			if c.expErr != nil {
				assert.Equal(t, c.expErr, err)
			}
		})
	}
}

func TestAddNLBIdleTimeout(t *testing.T) {
	var (
		ctxLogger = log.WithFields(log.Fields{
			"type": "destinationRule",
		})
		dr = &v1alpha32.DestinationRule{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "my-dr",
				Namespace: "test-ns",
			},
			Spec: v1alpha3.DestinationRule{
				Host: "e2e.blah.global",
			},
		}
		ctx = context.Background()
		rc  = &RemoteController{
			ClusterID: "test-cluster",
			DestinationRuleController: &istio.DestinationRuleController{
				IstioClient: istioFake.NewSimpleClientset(),
			},
		}
		admiralParams = common.AdmiralParams{
			LabelSet:              &common.LabelSet{},
			SyncNamespace:         "test-sync-ns",
			EnableSWAwareNSCaches: true,
			ExportToMaxNamespaces: 35,
		}
		stop         = make(chan struct{})
		config       = rest.Config{Host: "test-nlb"}
		resyncPeriod = time.Millisecond * 1
		nlbService   = newFakeService(common.NLBIstioIngressGatewayLabelValue, common.NamespaceIstioSystem, map[string]string{"app": common.NLBIstioIngressGatewayLabelValue})
	)
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	rr := NewRemoteRegistry(ctx, admiralParams)
	serviceControllerWithOneMatchingService, _ := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	rc.ServiceController = serviceControllerWithOneMatchingService
	serviceControllerWithOneMatchingService.K8sClient.CoreV1().Services(common.NamespaceIstioSystem).Create(ctx, nlbService, metaV1.CreateOptions{})
	rr.PutRemoteController("test-cluster", rc)
	rr.AdmiralCache.CnameClusterCache.Put(dr.Spec.Host, "test-cluster", "test-cluster")
	rr.AdmiralCache.CnameDependentClusterNamespaceCache.Put(dr.Spec.Host, rc.ClusterID, "dep-ns", "dep-ns")
	rr.AdmiralCache.NLBEnabledCluster = []string{"test-cluster"}

	//Before State
	beforeDefaultLB := common.GetAdmiralParams().LabelSet.GatewayApp

	//Set to default to CLB
	common.GetAdmiralParams().LabelSet.GatewayApp = common.IstioIngressGatewayLabelValue

	cases := []struct {
		name            string
		newDR           *v1alpha32.DestinationRule
		VSSourceCluster string
		svcTimeout      string
		expTimeout      string
	}{
		{
			name: "Given the nlb svc has timeout value less than 350, " +
				"When we create a dr for a service in that cluster, " +
				"Then the created dr should have TCP idle timeout of 350s",
			newDR:      dr,
			svcTimeout: "330",
			expTimeout: "seconds:350",
		},
		{
			name: "Given the nlb svc has timeout value greater than 350, " +
				"When we create a dr for a service in that cluster, " +
				"Then the created dr should have TCP idle timeout of the same value as the nlb svc",
			newDR:      dr,
			svcTimeout: "2100",
			expTimeout: "seconds:2100",
		},
		{
			name: "Given the update is for a vs routing dr, " +
				"When the source cluster is nlb enabled, " +
				"Then the idle timeout should be correct",
			newDR:      dr,
			svcTimeout: "2100",
			expTimeout: "seconds:2100",
		},
		{
			name: "Given the update is for a vs routing dr, " +
				"When the source cluster is nlb enabled, " +
				"Then the idle timeout should be correct",
			newDR:           dr,
			svcTimeout:      "2100",
			expTimeout:      "seconds:2100",
			VSSourceCluster: "test-cluster",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nlbService.Annotations = map[string]string{common.NLBIdleTimeoutAnnotation: c.svcTimeout}
			serviceControllerWithOneMatchingService.K8sClient.CoreV1().Services(common.NamespaceIstioSystem).Update(ctx, nlbService, metaV1.UpdateOptions{})
			updatedDr := addNLBIdleTimeout(ctx, ctxLogger, rr, rc, c.newDR.Spec.DeepCopy(), c.VSSourceCluster, "")
			if updatedDr.TrafficPolicy.ConnectionPool.Tcp.IdleTimeout.String() != c.expTimeout {
				t.Errorf("got tcp idle timeout: %s, expected %s", updatedDr.TrafficPolicy.ConnectionPool.Tcp.IdleTimeout, c.expTimeout)
			}
		})
	}

	//restore
	common.GetAdmiralParams().LabelSet.GatewayApp = beforeDefaultLB
}

// write test for getClientConnectionPoolOverrides
func TestGetClientConnectionPoolOverrides(t *testing.T) {

	admiralParams := common.AdmiralParams{
		MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	cases := []struct {
		name             string
		overrides        *v1.ClientConnectionConfig
		expectedSettings *v1alpha3.ConnectionPoolSettings
	}{
		{
			name: "Given overrides is nil, " +
				"When getClientConnectionPoolOverrides is called, " +
				"Then, the default settings should be returned",
			overrides: nil,
			expectedSettings: &v1alpha3.ConnectionPoolSettings{
				Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
					MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
				},
			},
		},
		{
			name: "Given overrides is not nil, " +
				"When getClientConnectionPoolOverrides is called, " +
				"And the ClientConnectionConfig spec is empty" +
				"Then, the default overrides should be returned",
			overrides: &v1.ClientConnectionConfig{
				Spec: v1.ClientConnectionConfigSpec{},
			},
			expectedSettings: &v1alpha3.ConnectionPoolSettings{
				Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
					MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
				},
			},
		},
		{
			name: "Given overrides is not nil, " +
				"When getClientConnectionPoolOverrides is called, " +
				"And the ClientConnectionConfig ConnectionPool settings are empty" +
				"Then, the default overrides should be returned",
			overrides: &v1.ClientConnectionConfig{
				Spec: v1.ClientConnectionConfigSpec{},
			},
			expectedSettings: &v1alpha3.ConnectionPoolSettings{
				Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
					MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
				},
			},
		},
		{
			name: "Given overrides is not nil, " +
				"When getClientConnectionPoolOverrides is called, " +
				"And the ClientConnectionConfig's only ConnectionPool.Http.Http2MaxRequests is being overwritten " +
				"Then, only the ConnectionPool.Http.Http2MaxRequests should be overwritten",
			overrides: &v1.ClientConnectionConfig{
				Spec: v1.ClientConnectionConfigSpec{
					ConnectionPool: model.ConnectionPool{
						Http: &model.ConnectionPool_HTTP{
							Http2MaxRequests: 100,
						},
					},
				},
			},
			expectedSettings: &v1alpha3.ConnectionPoolSettings{
				Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
					Http2MaxRequests:         100,
					MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
				},
			},
		},
		{
			name: "Given overrides is not nil, " +
				"When getClientConnectionPoolOverrides is called, " +
				"And the ClientConnectionConfig's only ConnectionPool.Http.MaxRequestsPerConnection is being overwritten " +
				"Then, only the ConnectionPool.Http.MaxRequestsPerConnection should be overwritten",
			overrides: &v1.ClientConnectionConfig{
				Spec: v1.ClientConnectionConfigSpec{
					ConnectionPool: model.ConnectionPool{
						Http: &model.ConnectionPool_HTTP{
							MaxRequestsPerConnection: 5,
						},
					},
				},
			},
			expectedSettings: &v1alpha3.ConnectionPoolSettings{
				Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
					MaxRequestsPerConnection: 5,
				},
			},
		},
		{
			name: "Given overrides is not nil, " +
				"When getClientConnectionPoolOverrides is called, " +
				"And the ClientConnectionConfig's only ConnectionPool.Http.IdleTimeout is being overwritten " +
				"Then, only the ConnectionPool.Http.IdleTimeout should be overwritten",
			overrides: &v1.ClientConnectionConfig{
				Spec: v1.ClientConnectionConfigSpec{
					ConnectionPool: model.ConnectionPool{
						Http: &model.ConnectionPool_HTTP{
							IdleTimeout: "1s",
						},
					},
				},
			},
			expectedSettings: &v1alpha3.ConnectionPoolSettings{
				Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
					MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
					IdleTimeout:              &duration.Duration{Seconds: 1},
				},
			},
		},
		{
			name: "Given overrides is not nil, " +
				"When getClientConnectionPoolOverrides is called, " +
				"And the ClientConnectionConfig's only ConnectionPool.TCP.MaxConnectionDuration is being overwritten " +
				"Then, only the ConnectionPool.TCP.MaxConnectionDuration should be overwritten",
			overrides: &v1.ClientConnectionConfig{
				Spec: v1.ClientConnectionConfigSpec{
					ConnectionPool: model.ConnectionPool{
						Tcp: &model.ConnectionPool_TCP{
							MaxConnectionDuration: "1s",
						},
					},
				},
			},
			expectedSettings: &v1alpha3.ConnectionPoolSettings{
				Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
					MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
				},
				Tcp: &networkingV1Alpha3.ConnectionPoolSettings_TCPSettings{
					MaxConnectionDuration: &duration.Duration{Seconds: 1},
				},
			},
		},
		{
			name: "Given overrides is not nil, " +
				"When getClientConnectionPoolOverrides is called, " +
				"And the ConnectionPool.TCP.MaxConnectionDuration is set to 0 " +
				"And the ConnectionPool.Http.Http2MaxRequests is set to 0 " +
				"And the ConnectionPool.Http.MaxRequestsPerConnection is set to 0 " +
				"And the ConnectionPool.Http.IdleTimeout is set to 0 " +
				"Then, all the overrides should be set to 0",
			overrides: &v1.ClientConnectionConfig{
				Spec: v1.ClientConnectionConfigSpec{
					ConnectionPool: model.ConnectionPool{
						Tcp: &model.ConnectionPool_TCP{
							MaxConnectionDuration: "0s",
						},
						Http: &model.ConnectionPool_HTTP{
							IdleTimeout:              "0s",
							MaxRequestsPerConnection: 0,
							Http2MaxRequests:         0,
						},
					},
				},
			},
			expectedSettings: &v1alpha3.ConnectionPoolSettings{
				Http: &networkingV1Alpha3.ConnectionPoolSettings_HTTPSettings{
					MaxRequestsPerConnection: DefaultMaxRequestsPerConnection,
					IdleTimeout:              &duration.Duration{Seconds: 0},
				},
				Tcp: &networkingV1Alpha3.ConnectionPoolSettings_TCPSettings{
					MaxConnectionDuration: &duration.Duration{Seconds: 0},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := getClientConnectionPoolOverrides(c.overrides)
			assert.Equal(t, c.expectedSettings, actual)
		})
	}
}

func TestGetDestinationRuleVSRoutingInCluster(t *testing.T) {
	ctxLogger := log.WithFields(log.Fields{
		"type": "destinationRule",
	})
	// Enable Active-Passive
	admiralParams := common.AdmiralParams{
		CacheReconcileDuration: 10 * time.Minute,
		LabelSet: &common.LabelSet{
			EnvKey: "env",
		},
		DefaultWarmupDurationSecs: 45,
	}
	admiralParams.EnableActivePassive = false
	admiralParams.EnableVSRoutingInCluster = true
	admiralParams.VSRoutingInClusterEnabledResources = map[string]string{"*": "*"}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	se := &v1alpha3.ServiceEntry{
		Hosts: []string{"qa.myservice.global"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "east.com", Locality: "us-east-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
			{Address: "west.com", Locality: "us-west-2", Labels: map[string]string{"security.istio.io/tlsMode": "istio"}},
		}}

	gtp := &model.TrafficPolicy{
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

	trafficPolicy := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{
				Distribute: []*v1alpha3.LocalityLoadBalancerSetting_Distribute{
					{
						From: "*",
						To:   map[string]uint32{"us-east-2": 100},
					},
				},
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
		OutlierDetection: &v1alpha3.OutlierDetection{
			BaseEjectionTime:         &duration.Duration{Seconds: 300},
			ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 50},
			Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
			Interval:                 &duration.Duration{Seconds: 60},
			MaxEjectionPercent:       100,
		},
	}

	dr := v1alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: trafficPolicy,
	}

	trafficPolicy1 := &v1alpha3.TrafficPolicy{
		Tls: &v1alpha3.ClientTLSSettings{
			Mode: v1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
		},
		ConnectionPool: &v1alpha3.ConnectionPoolSettings{
			Http: &v1alpha3.ConnectionPoolSettings_HTTPSettings{
				MaxRequestsPerConnection: common.MaxRequestsPerConnection(),
			},
		},
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{
				Simple: v1alpha3.LoadBalancerSettings_LEAST_REQUEST,
			},
			WarmupDurationSecs: &duration.Duration{Seconds: 45},
		},
		OutlierDetection: &v1alpha3.OutlierDetection{
			BaseEjectionTime:         &duration.Duration{Seconds: 300},
			ConsecutiveGatewayErrors: &wrappers.UInt32Value{Value: 50},
			Consecutive_5XxErrors:    &wrappers.UInt32Value{Value: 0},
			Interval:                 &duration.Duration{Seconds: 60},
			MaxEjectionPercent:       100,
		},
	}

	dr1 := v1alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: trafficPolicy1,
	}

	testCases := []struct {
		name                          string
		se                            *v1alpha3.ServiceEntry
		locality                      string
		gtpPolicy                     *model.TrafficPolicy
		destinationRuleInCache        *v1alpha32.DestinationRule
		eventResourceType             string
		updateDRForInClusterVSRouting bool
		eventType                     admiral.EventType
		expectedDestinationRule       *v1alpha3.DestinationRule
	}{
		{
			name: "Given vs based routing is enabled for cluster1 and identity1" +
				"locality of the cluster is us-west-2" +
				"Then the DR should have the traffic distribution set to 100% to east remote region",
			se:                            se,
			locality:                      "us-west-2",
			gtpPolicy:                     gtp,
			updateDRForInClusterVSRouting: true,
			expectedDestinationRule:       &dr,
		},
		{
			name: "Given vs based routing is not enabled for cluster1 and identity1" +
				"locality of the cluster is us-west-2" +
				"Then the DR should have the traffic distribution set to 100% to east remote region",
			se:                            se,
			locality:                      "us-west-2",
			gtpPolicy:                     nil,
			updateDRForInClusterVSRouting: false,
			expectedDestinationRule:       &dr1,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getDestinationRule(c.se, c.locality, c.gtpPolicy, nil, nil, nil, "", ctxLogger, "", c.updateDRForInClusterVSRouting)
			if !cmp.Equal(result, c.expectedDestinationRule, protocmp.Transform()) {
				t.Fatalf("DestinationRule Mismatch. Diff: %v", cmp.Diff(result, c.expectedDestinationRule, protocmp.Transform()))
			}
		})
	}
}

func TestIsNLBTimeoutNeeded(t *testing.T) {
	type args struct {
		sourceClusters []string
		nlbOverwrite   []string
		clbOverwrite   []string
		sourceIdentity string
	}

	//Store previous state
	beforeRunningtest := common.GetAdmiralParams().LabelSet.GatewayApp

	updateAdmiralParam := common.GetAdmiralParams()
	updateAdmiralParam.NLBEnabledIdentityList = []string{"intuit.test.asset"}

	common.UpdateAdmiralParams(updateAdmiralParam)

	//common.GetAdmiralParams().LabelSet.GatewayApp = common.NLBIstioIngressGatewayLabelValue
	//NLB is default. SourceCluster doen't have clb overwrite
	nlbDefaultWithNoCLBOverwrite := args{
		sourceClusters: []string{"test1-k8s", "test2-k8s"},
		nlbOverwrite:   []string{"nlb1-k8s", "test2-k8s"},
		clbOverwrite:   []string{"clb1-k8s", "test3-k8s"},
	}

	//NLB is default. SourceCluster does have clb overwrite partially
	nlbDefaultWithCLBOverwrite := args{
		sourceClusters: []string{"test1-k8s", "clb1-k8s"},
		nlbOverwrite:   []string{"nlb1-k8s", "test2-k8s"},
		clbOverwrite:   []string{"clb1-k8s", "test3-k8s"},
	}

	//NLB is default. All sourcecluster belongs to clb overwrite
	nlbDefaultWithAllCLBOverwrite := args{
		sourceClusters: []string{"test1-k8s", "clb1-k8s"},
		nlbOverwrite:   []string{"nlb1-k8s", "test2-k8s"},
		clbOverwrite:   []string{"test1-k8s", "clb1-k8s"},
	}

	//CLB is default. SourceCluster does have partial nlb overwrite
	clbDefaultWithPartialNLBOverwrite := args{
		sourceClusters: []string{"test1-k8s", "nlb1-k8s"},
		nlbOverwrite:   []string{"nlb1-k8s", "test2-k8s"},
		clbOverwrite:   []string{"clb1-k8s", "test3-k8s"},
	}

	//CLB is default. SourceCluster doen't have nlb overwrite, SourceCluster doesn't have clb overwrite
	clbDefaultWithNoOverwrite := args{
		sourceClusters: []string{"test1-k8s", "test2-k8s"},
		nlbOverwrite:   []string{"nlb1-k8s", "nlb2-k8s"},
		clbOverwrite:   []string{"clb1-k8s", "test3-k8s"},
	}

	//CLB is default, Source Asset found
	withSourceAsset := args{
		sourceClusters: []string{"test1-k8s", "test2-k8s"},
		nlbOverwrite:   []string{"nlb1-k8s", "nlb2-k8s"},
		clbOverwrite:   []string{"clb1-k8s", "test3-k8s"},
		sourceIdentity: "intuit.test.asset",
	}

	withSourceAssetCamelCase := args{
		sourceClusters: []string{"test1-k8s", "test2-k8s"},
		nlbOverwrite:   []string{"nlb1-k8s", "nlb2-k8s"},
		clbOverwrite:   []string{"clb1-k8s", "test3-k8s"},
		sourceIdentity: "Intuit.test.asset",
	}

	withSourceAssetEmpty := args{
		sourceClusters: []string{"test1-k8s", "test2-k8s"},
		nlbOverwrite:   []string{"nlb1-k8s", "nlb2-k8s"},
		clbOverwrite:   []string{"clb1-k8s", "test3-k8s"},
		sourceIdentity: "",
	}

	withSourceAssetNotMatching := args{
		sourceClusters: []string{"test1-k8s", "test2-k8s"},
		nlbOverwrite:   []string{"nlb1-k8s", "nlb2-k8s"},
		clbOverwrite:   []string{"clb1-k8s", "test3-k8s"},
		sourceIdentity: "intuit.test.asset.trymatchme",
	}

	tests := []struct {
		name      string
		args      args
		defaultLB string
		want      bool
	}{
		//NLB Default
		{"NLB is default. SourceCluster doen't have clb overwrite", nlbDefaultWithNoCLBOverwrite, common.NLBIstioIngressGatewayLabelValue, true},
		{"NLB is default. SourceCluster does have clb overwrite partially", nlbDefaultWithCLBOverwrite, common.NLBIstioIngressGatewayLabelValue, true},
		{"NLB is default. All sourcecluster belongs to clb overwrite", nlbDefaultWithAllCLBOverwrite, common.NLBIstioIngressGatewayLabelValue, false},

		//CLB Default
		{"CLB is default. SourceCluster does have partial nlb overwrite", clbDefaultWithPartialNLBOverwrite, common.IstioIngressGatewayLabelValue, true},
		{"CLB is default. SourceCluster doen't have nlb overwrite, SourceCluster doesn't have clb overwrite", clbDefaultWithNoOverwrite, common.IstioIngressGatewayLabelValue, false},

		//Overwrite source asset
		{"CLB is default. Source Asset overwrite is present", withSourceAsset, common.IstioIngressGatewayLabelValue, true},
		{"NLB is default. Source Asset overwrite is present", withSourceAsset, common.NLBIstioIngressGatewayLabelValue, true},
		{"CLB is default. Source Asset overwrite is present - Camel Case", withSourceAssetCamelCase, common.IstioIngressGatewayLabelValue, true},
		{"NLB is default. Source Asset overwrite is present - Camel Case", withSourceAssetCamelCase, common.NLBIstioIngressGatewayLabelValue, true},
		{"CLB is default. Source Asset overwrite is empty", withSourceAssetEmpty, common.IstioIngressGatewayLabelValue, false},
		{"NLB is default. Source Asset overwrite is empty", withSourceAssetEmpty, common.NLBIstioIngressGatewayLabelValue, true},
		{"CLB is default. Source Asset overwrite is notmatching", withSourceAssetNotMatching, common.IstioIngressGatewayLabelValue, false},
		{"NLB is default. Source Asset overwrite is notmatching", withSourceAssetNotMatching, common.NLBIstioIngressGatewayLabelValue, true},

		//Cluster Specific Asset migration. CLB is default
		{"CLB is default. Source Asset overwrite for a matching cluster", args{
			sourceClusters: []string{"nlb1-k8s", "test2-k8s"},
			nlbOverwrite:   []string{"intuit.test.asset.trymatchme:nlb1-k8s", "nlb2-k8s"},
			clbOverwrite:   []string{"clb1-k8s", "clb2-k8s"},
			sourceIdentity: "intuit.test.asset.trymatchme",
		}, common.IstioIngressGatewayLabelValue, true},

		{"CLB is default. Source Asset overwrite for a non matching cluster", args{
			sourceClusters: []string{"test1-k8s", "test2-k8s"},
			nlbOverwrite:   []string{"intuit.test.asset.trymatchme:nlb1-k8s", "nlb2-k8s"},
			clbOverwrite:   []string{"clb1-k8s", "clb2-k8s"},
			sourceIdentity: "intuit.test.asset.trymatchme",
		}, common.IstioIngressGatewayLabelValue, false},
	}
	for _, tt := range tests {
		common.GetAdmiralParams().LabelSet.GatewayApp = tt.defaultLB
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, IsNLBTimeoutNeeded(tt.args.sourceClusters, tt.args.nlbOverwrite, tt.args.clbOverwrite, tt.args.sourceIdentity), "IsNLBTimeoutNeeded(%v, %v, %v)", tt.args.sourceClusters, tt.args.nlbOverwrite, tt.args.clbOverwrite)
		})
	}

	//Restore previous state
	common.GetAdmiralParams().LabelSet.GatewayApp = beforeRunningtest
}
