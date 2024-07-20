package clusters

import (
	"sync"
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
