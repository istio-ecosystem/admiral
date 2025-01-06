package clusters

import (
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	admiralV1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
)

var ignoreUnexported = cmpopts.IgnoreUnexported(admiralV1.GlobalTrafficPolicy{}.Status)

var typeTestSingleton sync.Once

func admiralParamsForTypesTests() common.AdmiralParams {
	return common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
			PriorityKey:             "priority",
		},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheReconcileDuration:     time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace:      "default",
		EnableRoutingPolicy:        true,
		EnvoyFilterVersion:         "1.13",
		Profile:                    common.AdmiralProfileDefault,
	}
}

func setupForTypeTests() {
	typeTestSingleton.Do(func() {
		common.ResetSync()
		common.InitializeConfig(admiralParamsForTypesTests())
	})
}

func TestRangeRemoteControllers(t *testing.T) {
	setupForTypeTests()

	rr := &RemoteRegistry{
		Mutex:             sync.Mutex{},
		remoteControllers: map[string]*RemoteController{"test": &RemoteController{}},
	}

	counter := 0
	// testFunc is a function that tests the RemoteController
	testFunc := func(k string, v *RemoteController) {
		counter = counter + 1
	}

	// TestRangeRemoteControllers is a table-driven test for RangeRemoteControllers
	tests := []struct {
		name     string
		expected int
	}{
		{
			name:     "TestRangeRemoteControllers",
			expected: 1,
		},
	}
	for _, tt := range tests {
		counter = 0
		t.Run(tt.name, func(t *testing.T) {
			rr.RangeRemoteControllers(testFunc)
			assert.Equal(t, tt.expected, counter)
		})
	}
}
