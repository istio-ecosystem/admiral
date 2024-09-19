package common

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	log "github.com/sirupsen/logrus"
)

var configTestSingleton sync.Once

func setupForConfigTests() {
	var initHappened bool
	configTestSingleton.Do(func() {
		p := AdmiralParams{
			KubeconfigPath: "testdata/fake.config",
			LabelSet: &LabelSet{
				WorkloadIdentityKey:     "identity",
				AdmiralCRDIdentityLabel: "identity",
				IdentityPartitionKey:    "admiral.io/identityPartition",
			},
			EnableSAN:                    true,
			SANPrefix:                    "prefix",
			HostnameSuffix:               "mesh",
			SyncNamespace:                "admiral-sync",
			SecretFilterTags:             "admiral/sync",
			CacheReconcileDuration:       time.Minute,
			ClusterRegistriesNamespace:   "default",
			DependenciesNamespace:        "default",
			Profile:                      "default",
			WorkloadSidecarName:          "default",
			WorkloadSidecarUpdate:        "disabled",
			MetricsEnabled:               true,
			DeprecatedEnvoyFilterVersion: "1.10,1.17",
			EnvoyFilterVersion:           "1.10,1.13,1.17",
			CartographerFeatures:         map[string]string{"throttle_filter_gen": "disabled"},
			DisableIPGeneration:          false,
			EnableSWAwareNSCaches:        true,
			ExportToIdentityList:         []string{"*"},
			ExportToMaxNamespaces:        35,
			AdmiralOperatorMode:          false,
			OperatorSyncNamespace:        "admiral-sync",
			OperatorSecretFilterTags:     "admiral/syncoperator",
		}
		ResetSync()
		initHappened = true
		InitializeConfig(p)
	})
	if !initHappened {
		log.Warn("InitializeConfig was NOT called from setupForConfigTests")
	} else {
		log.Info("InitializeConfig was called setupForConfigTests")
	}
}

func TestDoVSRoutingForCluster(t *testing.T) {
	p := AdmiralParams{}

	testCases := []struct {
		name                      string
		cluster                   string
		enableVSRouting           bool
		enableVSRoutingForCluster []string
		expected                  bool
	}{
		{
			name: "Given enableVSRouting is false, enableVSRoutingForCluster is empty" +
				"When DoVSRoutingForCluster is called" +
				"Then it should return false",
			cluster:                   "cluster1",
			enableVSRouting:           false,
			enableVSRoutingForCluster: []string{},
			expected:                  false,
		},
		{
			name: "Given enableVSRouting is true, enableVSRoutingForCluster is empty" +
				"When DoVSRoutingForCluster is called" +
				"Then it should return false",
			cluster:                   "cluster1",
			enableVSRouting:           true,
			enableVSRoutingForCluster: []string{},
			expected:                  false,
		},
		{
			name: "Given enableVSRouting is true, and given cluster doesn't exists in the list" +
				"When DoVSRoutingForCluster is called" +
				"Then it should return false",
			cluster:                   "cluster2",
			enableVSRouting:           true,
			enableVSRoutingForCluster: []string{"cluster1"},
			expected:                  false,
		},
		{
			name: "Given enableVSRouting is true, and given cluster does exists in the list" +
				"When DoVSRoutingForCluster is called" +
				"Then it should return false",
			cluster:                   "cluster1",
			enableVSRouting:           true,
			enableVSRoutingForCluster: []string{"cluster1"},
			expected:                  true,
		},
		{
			name: "Given enableVSRouting is true, and all VS routing is enabled in all clusters using '*'" +
				"When DoVSRoutingForCluster is called" +
				"Then it should return false",
			cluster:                   "cluster1",
			enableVSRouting:           true,
			enableVSRoutingForCluster: []string{"*"},
			expected:                  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.EnableVSRouting = tc.enableVSRouting
			p.VSRoutingEnabledClusters = tc.enableVSRoutingForCluster
			ResetSync()
			InitializeConfig(p)

			assert.Equal(t, tc.expected, DoVSRoutingForCluster(tc.cluster))
		})
	}

}

func TestConfigManagement(t *testing.T) {
	setupForConfigTests()

	if GetWorkloadIdentifier() != "identity" {
		t.Errorf("Workload identifier mismatch, expected identity, got %v", GetWorkloadIdentifier())
	}
	if GetKubeconfigPath() != "testdata/fake.config" {
		t.Errorf("Kubeconfig path mismatch, expected testdata/fake.config, got %v", GetKubeconfigPath())
	}
	if GetSecretFilterTags() != "admiral/sync" {
		t.Errorf("Filter tags mismatch, expected admiral/sync, got %v", GetSecretFilterTags())
	}
	if GetSANPrefix() != "prefix" {
		t.Errorf("San prefix mismatch, expected prefix, got %v", GetSANPrefix())
	}
	if GetHostnameSuffix() != "mesh" {
		t.Errorf("Hostname suffix mismatch, expected mesh, got %v", GetHostnameSuffix())
	}
	if GetSyncNamespace() != "admiral-sync" {
		t.Errorf("Sync namespace mismatch, expected ns, got %v", GetSyncNamespace())
	}
	if GetEnableSAN() != true {
		t.Errorf("Enable SAN mismatch, expected true, got %v", GetEnableSAN())
	}
	if GetCacheRefreshDuration() != time.Minute {
		t.Errorf("Cache refresh duration mismatch, expected %v, got %v", time.Minute, GetCacheRefreshDuration())
	}
	if GetClusterRegistriesNamespace() != "default" {
		t.Errorf("Cluster registry namespace mismatch, expected default, got %v", GetClusterRegistriesNamespace())
	}
	if GetDependenciesNamespace() != "default" {
		t.Errorf("Dependency namespace mismatch, expected default, got %v", GetDependenciesNamespace())
	}
	if GetAdmiralProfile() != "default" {
		t.Errorf("Secret resolver mismatch, expected empty string, got %v", GetAdmiralProfile())
	}
	if GetAdmiralCRDIdentityLabel() != "identity" {
		t.Fatalf("Admiral CRD Identity label mismatch. Expected identity, got %v", GetAdmiralCRDIdentityLabel())
	}
	if GetWorkloadSidecarName() != "default" {
		t.Fatalf("Workload Sidecar Name mismatch. Expected default, got %v", GetWorkloadSidecarName())
	}
	if GetWorkloadSidecarUpdate() != "disabled" {
		t.Fatalf("Workload Sidecar Update mismatch. Expected disabled, got %v", GetWorkloadSidecarUpdate())
	}

	SetKubeconfigPath("/mypath/custom/kubeconfig")

	if GetKubeconfigPath() != "/mypath/custom/kubeconfig" {
		t.Fatalf("Workload Sidecar Name mismatch. Expected /mypath/custom/kubeconfig, got %v", GetKubeconfigPath())
	}

	if GetMetricsEnabled() != true {
		t.Errorf("Enable Prometheus mismatch, expected false, got %v", GetMetricsEnabled())
	}

	if IsPersonaTrafficConfig() != false {
		t.Errorf("Enable Traffic Persona mismatch, expected false, got %v", IsPersonaTrafficConfig())
	}

	if IsDefaultPersona() != true {
		t.Errorf("Enable Default Persona mismatch, expected false, got %v", IsDefaultPersona())
	}

	if len(GetDeprecatedEnvoyFilterVersion()) != 2 {
		t.Errorf("Get deprecated envoy filter version by splitting with ',',  expected 2, got %v", len(GetDeprecatedEnvoyFilterVersion()))
	}

	if len(GetEnvoyFilterVersion()) != 3 {
		t.Errorf("Get envoy filter version by splitting with ',', expected 3, got %v", len(GetEnvoyFilterVersion()))
	}

	if IsCartographerFeatureDisabled("router_filter_gen") {
		t.Errorf("If the feature is not present in the list should be assumed as enabled/true ',', expected false, got %v", IsCartographerFeatureDisabled("router_filter_gen"))
	}

	if !IsCartographerFeatureDisabled("throttle_filter_gen") {
		t.Errorf("If the feature is present in the list with valure disabled. ',', expected true, got %v", IsCartographerFeatureDisabled("throttle_filter_gen"))
	}

	if DisableIPGeneration() {
		t.Errorf("Disable IP Address Generation mismatch, expected false, got %v", DisableIPGeneration())
	}

	if GetPartitionIdentifier() != "admiral.io/identityPartition" {
		t.Errorf("Get identity partition mismatch, expected admiral.io/identityPartition, got %v", GetPartitionIdentifier())
	}

	if !EnableSWAwareNSCaches() {
		t.Errorf("enable SW aware namespace caches mismatch, expected true, got %v", EnableSWAwareNSCaches())
	}

	if !EnableExportTo("fakeIdentity") {
		t.Errorf("enable exportTo mismatch, expected true, got %v", EnableExportTo("fakeIdentity"))
	}

	if GetExportToMaxNamespaces() != 35 {
		t.Errorf("exportTo max namespaces mismatch, expected 35, got %v", GetExportToMaxNamespaces())
	}

	if IsAdmiralOperatorMode() {
		t.Errorf("enable operator mode mismatch, expected false, got %v", IsAdmiralOperatorMode())
	}

	if GetOperatorSyncNamespace() != "admiral-sync" {
		t.Errorf("operator sync namespace mismatch, expected admiral-sync, got %v", GetOperatorSyncNamespace())
	}

	if GetOperatorSecretFilterTags() != "admiral/syncoperator" {
		t.Errorf("operator secret filter tags mismatch, expected admiral/syncoperator, got %s", GetOperatorSecretFilterTags())
	}
}

func TestGetCRDIdentityLabelWithCRDIdentity(t *testing.T) {

	admiralParams := GetAdmiralParams()
	backOldIdentity := admiralParams.LabelSet.AdmiralCRDIdentityLabel
	admiralParams.LabelSet.AdmiralCRDIdentityLabel = "identityOld"

	assert.Equalf(t, "identityOld", GetAdmiralCRDIdentityLabel(), "GetCRDIdentityLabel()")

	admiralParams.LabelSet.AdmiralCRDIdentityLabel = backOldIdentity
}

//func TestGetCRDIdentityLabelWithLabel(t *testing.T) {
//
//	admiralParams := GetAdmiralParams()
//	backOldIdentity := admiralParams.LabelSet.AdmiralCRDIdentityLabel
//	backOldGTPLabel := admiralParams.LabelSet.GlobalTrafficDeploymentLabel
//	admiralParams.LabelSet.GlobalTrafficDeploymentLabel = "identityGTP"
//
//	assert.Equalf(t, "identityGTP", GetAdmiralCRDIdentityLabel(), "GetAdmiralCRDIdentityLabel()")
//
//	admiralParams.LabelSet.CRDIdentityLabel = backOldIdentity
//	admiralParams.LabelSet.GlobalTrafficDeploymentLabel = backOldGTPLabel
//}

//func TestGetCRDIdentityLabelWithEmptyLabel(t *testing.T) {
//
//	admiralParams := GetAdmiralParams()
//	backOldIdentity := admiralParams.LabelSet.CRDIdentityLabel
//	backOldGTPLabel := admiralParams.LabelSet.GlobalTrafficDeploymentLabel
//	admiralParams.LabelSet.GlobalTrafficDeploymentLabel = ""
//
//	assert.Equalf(t, "", GetCRDIdentityLabel(), "GetCRDIdentityLabel()")
//
//	admiralParams.LabelSet.GlobalTrafficDeploymentLabel = ""
//	admiralParams.LabelSet.CRDIdentityLabel = backOldIdentity
//	admiralParams.LabelSet.GlobalTrafficDeploymentLabel = backOldGTPLabel
//}
