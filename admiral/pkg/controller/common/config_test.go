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
				ShardIdentityLabelKey:   "admiral.io/shardIdentity",
			},
			EnableSAN:                              true,
			SANPrefix:                              "prefix",
			HostnameSuffix:                         "mesh",
			SyncNamespace:                          "admiral-sync",
			SecretFilterTags:                       "admiral/sync",
			CacheReconcileDuration:                 time.Minute,
			ClusterRegistriesNamespace:             "default",
			DependenciesNamespace:                  "default",
			Profile:                                "default",
			WorkloadSidecarName:                    "default",
			WorkloadSidecarUpdate:                  "disabled",
			MetricsEnabled:                         true,
			DeprecatedEnvoyFilterVersion:           "1.10,1.17",
			EnvoyFilterVersion:                     "1.10,1.13,1.17",
			CartographerFeatures:                   map[string]string{"throttle_filter_gen": "disabled"},
			DisableIPGeneration:                    false,
			EnableSWAwareNSCaches:                  true,
			ExportToIdentityList:                   []string{"*"},
			ExportToMaxNamespaces:                  35,
			AdmiralOperatorMode:                    false,
			OperatorSyncNamespace:                  "admiral-sync",
			OperatorIdentityValue:                  "operator",
			ShardIdentityValue:                     "shard",
			OperatorSecretFilterTags:               "admiral/syncoperator",
			DiscoveryClustersForNumaflow:           make([]string, 0),
			ClientDiscoveryClustersForJobs:         make([]string, 0),
			EnableClientDiscovery:                  true,
			ArgoRolloutsEnabled:                    true,
			EnvoyFilterAdditionalConfig:            "additional",
			EnableRoutingPolicy:                    true,
			HAMode:                                 "true",
			EnableDiffCheck:                        true,
			EnableProxyEnvoyFilter:                 true,
			EnableDependencyProcessing:             true,
			SeAddressConfigmap:                     "configmap",
			DeploymentOrRolloutWorkerConcurrency:   10,
			DependentClusterWorkerConcurrency:      10,
			DependencyWarmupMultiplier:             10,
			MaxRequestsPerConnection:               10,
			EnableClientConnectionConfigProcessing: true,
			DisableDefaultAutomaticFailover:        true,
			EnableServiceEntryCache:                true,
			EnableDestinationRuleCache:             true,
			AlphaIdentityList:                      []string{"identity1", "identity2"},
			EnableActivePassive:                    true,
			ClientInitiatedProcessingEnabled:       true,
			IngressLBPolicy:                        "policy",
			IngressVSExportToNamespaces:            []string{"namespace"},
			VSRoutingGateways:                      []string{"gateway"},
			EnableGenerationCheck:                  true,
			EnableIsOnlyReplicaCountChangedCheck:   true,
			PreventSplitBrain:                      true,
			AdmiralStateSyncerMode:                 true,
			DefaultWarmupDurationSecs:              10,
			AdmiralConfig:                          "someConfig",
			AdditionalEndpointSuffixes:             []string{"suffix1", "suffix2"},
			AdditionalEndpointLabelFilters:         []string{"label1", "label2"},
			EnableWorkloadDataStorage:              true,
			IgnoreLabelsAnnotationsVSCopyList:      []string{"applications.argoproj.io/app-name", "app.kubernetes.io/instance", "argocd.argoproj.io/tracking-id"},
			AdmiralStateSyncerClusters:             []string{"test-k8s"},
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

func TestDoRoutingPolicyForCluster(t *testing.T) {
	p := AdmiralParams{}
	type args struct {
		cluster               string
		enableRoutingPolicy   bool
		routingPolicyClusters []string
	}
	testCases := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Given enableRoutingPolicy is false" +
				"Then it should return false irrespective of the routingPolicyClusters",
			args: args{
				cluster:             "cluster1",
				enableRoutingPolicy: false,
			},
			want: false,
		},
		{
			name: "Given enableRoutingPolicy is false" +
				"Then it should return false irrespective of the routingPolicyClusters",
			args: args{
				cluster:               "cluster1",
				enableRoutingPolicy:   false,
				routingPolicyClusters: []string{"cluster1"},
			},
			want: false,
		},
		{
			name: "Given enableRoutingPolicy is true " +
				"Then it should return true if routingPolicyClusters contains '*'",
			args: args{
				cluster:               "cluster1",
				enableRoutingPolicy:   true,
				routingPolicyClusters: []string{"*"},
			},
			want: true,
		},
		{
			name: "Given enableRoutingPolicy is true " +
				"Then it should return false if routingPolicyClusters is empty",
			args: args{
				cluster:               "cluster1",
				enableRoutingPolicy:   true,
				routingPolicyClusters: []string{},
			},
			want: false,
		},
		{
			name: "Given enableRoutingPolicy is true " +
				"Then it should return true if routingPolicyClusters contains exact match",
			args: args{
				cluster:               "cluster1",
				enableRoutingPolicy:   true,
				routingPolicyClusters: []string{"cluster1"},
			},
			want: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.EnableRoutingPolicy = tc.args.enableRoutingPolicy
			p.RoutingPolicyClusters = tc.args.routingPolicyClusters
			ResetSync()
			InitializeConfig(p)

			assert.Equal(t, tc.want, DoRoutingPolicyForCluster(tc.args.cluster))
		})
	}
}

func TestDoVSRoutingForCluster(t *testing.T) {
	p := AdmiralParams{}

	testCases := []struct {
		name                        string
		cluster                     string
		enableVSRouting             bool
		disabledVSRoutingForCluster []string
		expected                    bool
	}{
		{
			name: "Given enableVSRouting is false, disabledVSRoutingForCluster is empty" +
				"When DoVSRoutingForCluster is called" +
				"Then it should return false",
			cluster:                     "cluster1",
			enableVSRouting:             false,
			disabledVSRoutingForCluster: []string{},
			expected:                    false,
		},
		{
			name: "Given enableVSRouting is true, disabledVSRoutingForCluster is empty" +
				"When DoVSRoutingForCluster is called" +
				"Then it should return true",
			cluster:                     "cluster1",
			enableVSRouting:             true,
			disabledVSRoutingForCluster: []string{},
			expected:                    true,
		},
		{
			name: "Given enableVSRouting is true, and given cluster doesn't exists in the list" +
				"When DoVSRoutingForCluster is called" +
				"Then it should return true",
			cluster:                     "cluster2",
			enableVSRouting:             true,
			disabledVSRoutingForCluster: []string{"cluster1"},
			expected:                    true,
		},
		{
			name: "Given enableVSRouting is true, and given cluster does exists in the list" +
				"When DoVSRoutingForCluster is called" +
				"Then it should return false",
			cluster:                     "cluster1",
			enableVSRouting:             true,
			disabledVSRoutingForCluster: []string{"cluster1"},
			expected:                    false,
		},
		{
			name: "Given enableVSRouting is true, and VS routing is disabled in all clusters using '*'" +
				"When DoVSRoutingForCluster is called" +
				"Then it should return false",
			cluster:                     "cluster1",
			enableVSRouting:             true,
			disabledVSRoutingForCluster: []string{"*"},
			expected:                    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.EnableVSRouting = tc.enableVSRouting
			p.VSRoutingDisabledClusters = tc.disabledVSRoutingForCluster
			ResetSync()
			InitializeConfig(p)

			assert.Equal(t, tc.expected, DoVSRoutingForCluster(tc.cluster))
		})
	}

}

func TestIsSlowStartEnabledForCluster(t *testing.T) {
	p := AdmiralParams{}

	testCases := []struct {
		name                               string
		cluster                            string
		enableVSRouting                    bool
		enableVSRoutingSlowStartForCluster []string
		expected                           bool
	}{
		{
			name: "Given enableVSRouting is false, enableVSRoutingSlowStartForCluster is empty" +
				"When IsSlowStartEnabledForCluster is called" +
				"Then it should return false",
			cluster:                            "cluster1",
			enableVSRouting:                    false,
			enableVSRoutingSlowStartForCluster: []string{},
			expected:                           false,
		},
		{
			name: "Given enableVSRouting is true, enableVSRoutingSlowStartForCluster is empty" +
				"When IsSlowStartEnabledForCluster is called" +
				"Then it should return false",
			cluster:                            "cluster1",
			enableVSRouting:                    true,
			enableVSRoutingSlowStartForCluster: []string{},
			expected:                           false,
		},
		{
			name: "Given enableVSRouting is true, and given cluster doesn't exists in the list" +
				"When IsSlowStartEnabledForCluster is called" +
				"Then it should return false",
			cluster:                            "cluster2",
			enableVSRouting:                    true,
			enableVSRoutingSlowStartForCluster: []string{"cluster1"},
			expected:                           false,
		},
		{
			name: "Given enableVSRouting is true, and given cluster does exists in the list" +
				"When IsSlowStartEnabledForCluster is called" +
				"Then it should return true",
			cluster:                            "cluster1",
			enableVSRouting:                    true,
			enableVSRoutingSlowStartForCluster: []string{"cluster1"},
			expected:                           true,
		},
		{
			name: "Given enableVSRouting is true, and all slow start is enabled in all clusters using '*'" +
				"When IsSlowStartEnabledForCluster is called" +
				"Then it should return false",
			cluster:                            "cluster1",
			enableVSRouting:                    true,
			enableVSRoutingSlowStartForCluster: []string{"*"},
			expected:                           true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.EnableVSRouting = tc.enableVSRouting
			p.VSRoutingSlowStartEnabledClusters = tc.enableVSRoutingSlowStartForCluster
			ResetSync()
			InitializeConfig(p)

			assert.Equal(t, tc.expected, IsSlowStartEnabledForCluster(tc.cluster))
		})
	}

}

func TestDoDRUpdateForInClusterVSRouting(t *testing.T) {
	p := AdmiralParams{}

	testCases := []struct {
		name                                   string
		cluster                                string
		identity                               string
		isSourceCluster                        bool
		enableVSRoutingInCluster               bool
		enabledVSRoutingInClusterForCluster    []string
		enabledVSRoutingInClusterForIdentities []string
		expected                               bool
	}{
		{
			name: "Given VSRoutingInCluster is enabled for cluster1 and identity1, cluster is source cluster" +
				"When DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return true",
			cluster:                                "cluster1",
			identity:                               "identity1",
			isSourceCluster:                        true,
			enableVSRoutingInCluster:               true,
			enabledVSRoutingInClusterForCluster:    []string{"cluster1"},
			enabledVSRoutingInClusterForIdentities: []string{"identity1"},
			expected:                               true,
		},
		{
			name: "Given VSRoutingInCluster is enabled for cluster1 and identity1, cluster is remote cluster" +
				"When DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return true",
			cluster:                                "cluster1",
			identity:                               "identity1",
			isSourceCluster:                        false,
			enableVSRoutingInCluster:               true,
			enabledVSRoutingInClusterForCluster:    []string{"cluster1"},
			enabledVSRoutingInClusterForIdentities: []string{"identity1"},
			expected:                               false,
		},
		{
			name: "Given VSRoutingInCluster is not enabled for cluster1, cluster is source cluster" +
				"When DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return true",
			cluster:                                "cluster1",
			identity:                               "identity1",
			isSourceCluster:                        true,
			enableVSRoutingInCluster:               true,
			enabledVSRoutingInClusterForCluster:    []string{"cluster2"},
			enabledVSRoutingInClusterForIdentities: []string{},
			expected:                               false,
		},
		{
			name: "Given VSRoutingInCluster is not enabled, cluster is source cluster" +
				"When DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return true",
			cluster:                                "cluster1",
			identity:                               "identity1",
			isSourceCluster:                        true,
			enableVSRoutingInCluster:               false,
			enabledVSRoutingInClusterForCluster:    []string{},
			enabledVSRoutingInClusterForIdentities: []string{},
			expected:                               false,
		},
		{
			name: "Given VSRoutingInCluster is enabled for cluster1,  VSRoutingInCluster not enabled for identity1, cluster is source cluster" +
				"When DoDRUpdateForInClusterVSRouting is called" +
				"Then it should return true",
			cluster:                                "cluster1",
			identity:                               "identity1",
			isSourceCluster:                        true,
			enableVSRoutingInCluster:               true,
			enabledVSRoutingInClusterForCluster:    []string{"cluster1"},
			enabledVSRoutingInClusterForIdentities: []string{},
			expected:                               false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.EnableVSRoutingInCluster = tc.enableVSRoutingInCluster
			p.VSRoutingInClusterEnabledClusters = tc.enabledVSRoutingInClusterForCluster
			p.VSRoutingInClusterEnabledIdentities = tc.enabledVSRoutingInClusterForIdentities
			ResetSync()
			InitializeConfig(p)

			assert.Equal(t, tc.expected, DoDRUpdateForInClusterVSRouting(tc.cluster, tc.identity, tc.isSourceCluster))
		})
	}

}

func TestIsVSRoutingInClusterDisabledForCluster(t *testing.T) {
	p := AdmiralParams{}

	testCases := []struct {
		name                                 string
		cluster                              string
		disabledVSRoutingInClusterForCluster []string
		expected                             bool
	}{
		{
			name: "Given disabledVSRoutingInClusterForCluster is empty" +
				"When IsVSRoutingInClusterDisabledForCluster is called" +
				"Then it should return false",
			cluster:                              "cluster1",
			disabledVSRoutingInClusterForCluster: []string{},
			expected:                             false,
		},
		{
			name: "Given cluster doesn't exists in the list" +
				"When IsVSRoutingInClusterDisabledForCluster is called" +
				"Then it should return false",
			cluster:                              "cluster2",
			disabledVSRoutingInClusterForCluster: []string{"cluster1"},
			expected:                             false,
		},
		{
			name: "Given cluster does exists in the list" +
				"When IsVSRoutingInClusterDisabledForCluster is called" +
				"Then it should return true",
			cluster:                              "cluster1",
			disabledVSRoutingInClusterForCluster: []string{"cluster1"},
			expected:                             true,
		},
		{
			name: "Given VS routing is disabled in all clusters using '*'" +
				"When IsVSRoutingInClusterDisabledForCluster is called" +
				"Then it should return true",
			cluster:                              "cluster1",
			disabledVSRoutingInClusterForCluster: []string{"*"},
			expected:                             true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.VSRoutingInClusterDisabledClusters = tc.disabledVSRoutingInClusterForCluster
			ResetSync()
			InitializeConfig(p)

			assert.Equal(t, tc.expected, IsVSRoutingInClusterDisabledForCluster(tc.cluster))
		})
	}

}

func TestIsVSRoutingInClusterDisabledForIdentity(t *testing.T) {
	p := AdmiralParams{}

	testCases := []struct {
		name                                    string
		identity                                string
		disabledVSRoutingInClusterForIdentities []string
		expected                                bool
	}{
		{
			name: "Given disabledVSRoutingInClusterForIdentities is empty" +
				"When IsVSRoutingInClusterDisabledForIdentity is called" +
				"Then it should return false",
			identity:                                "testIdentity1",
			disabledVSRoutingInClusterForIdentities: []string{},
			expected:                                false,
		},
		{
			name: "Given identity doesn't exists in the list" +
				"When IsVSRoutingInClusterDisabledForIdentity is called" +
				"Then it should return false",
			identity:                                "testIdentity2",
			disabledVSRoutingInClusterForIdentities: []string{"testIdentity1"},
			expected:                                false,
		},
		{
			name: "Given identity does exists in the list" +
				"When IsVSRoutingInClusterDisabledForIdentity is called" +
				"Then it should return true",
			identity:                                "testIdentity1",
			disabledVSRoutingInClusterForIdentities: []string{"testIdentity1"},
			expected:                                true,
		},
		{
			name: "Given  VS routing is disabled for all identities using '*'" +
				"When IsVSRoutingInClusterDisabledForIdentity is called" +
				"Then it should return true",
			identity:                                "testIdentity1",
			disabledVSRoutingInClusterForIdentities: []string{"*"},
			expected:                                true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.VSRoutingInClusterDisabledIdentities = tc.disabledVSRoutingInClusterForIdentities
			ResetSync()
			InitializeConfig(p)

			assert.Equal(t, tc.expected, IsVSRoutingInClusterDisabledForIdentity(tc.identity))
		})
	}

}

func TestDoVSRoutingInClusterForCluster(t *testing.T) {
	p := AdmiralParams{}

	testCases := []struct {
		name                                string
		cluster                             string
		enableVSRoutingInCluster            bool
		enabledVSRoutingInClusterForCluster []string
		expected                            bool
	}{
		{
			name: "Given enableVSRoutingInCluster is false, enabledVSRoutingInClusterForCluster is empty" +
				"When DoVSRoutingInClusterForCluster is called" +
				"Then it should return false",
			cluster:                             "cluster1",
			enableVSRoutingInCluster:            false,
			enabledVSRoutingInClusterForCluster: []string{},
			expected:                            false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, enabledVSRoutingInClusterForCluster is empty" +
				"When DoVSRoutingInClusterForCluster is called" +
				"Then it should return false",
			cluster:                             "cluster1",
			enableVSRoutingInCluster:            true,
			enabledVSRoutingInClusterForCluster: []string{},
			expected:                            false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and given cluster doesn't exists in the list" +
				"When DoVSRoutingInClusterForCluster is called" +
				"Then it should return false",
			cluster:                             "cluster2",
			enableVSRoutingInCluster:            true,
			enabledVSRoutingInClusterForCluster: []string{"cluster1"},
			expected:                            false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and given cluster does exists in the list" +
				"When DoVSRoutingInClusterForCluster is called" +
				"Then it should return true",
			cluster:                             "cluster1",
			enableVSRoutingInCluster:            true,
			enabledVSRoutingInClusterForCluster: []string{"cluster1"},
			expected:                            true,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and all VS routing is enabled in all clusters using '*'" +
				"When DoVSRoutingInClusterForCluster is called" +
				"Then it should return true",
			cluster:                             "cluster1",
			enableVSRoutingInCluster:            true,
			enabledVSRoutingInClusterForCluster: []string{"*"},
			expected:                            true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.EnableVSRoutingInCluster = tc.enableVSRoutingInCluster
			p.VSRoutingInClusterEnabledClusters = tc.enabledVSRoutingInClusterForCluster
			ResetSync()
			InitializeConfig(p)

			assert.Equal(t, tc.expected, DoVSRoutingInClusterForCluster(tc.cluster))
		})
	}

}

func TestDoVSRoutingInClusterForIdentity(t *testing.T) {
	p := AdmiralParams{}

	testCases := []struct {
		name                                   string
		identity                               string
		enableVSRoutingInCluster               bool
		enabledVSRoutingInClusterForIdentities []string
		expected                               bool
	}{
		{
			name: "Given enableVSRoutingInCluster is false, enabledVSRoutingInClusterForIdentities is empty" +
				"When DoVSRoutingInClusterForIdentity is called" +
				"Then it should return false",
			identity:                               "testIdentity1",
			enableVSRoutingInCluster:               false,
			enabledVSRoutingInClusterForIdentities: []string{},
			expected:                               false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, enabledVSRoutingInClusterForIdentities is empty" +
				"When DoVSRoutingInClusterForIdentity is called" +
				"Then it should return false",
			identity:                               "testIdentity1",
			enableVSRoutingInCluster:               true,
			enabledVSRoutingInClusterForIdentities: []string{},
			expected:                               false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and given cluster doesn't exists in the list" +
				"When DoVSRoutingInClusterForIdentity is called" +
				"Then it should return false",
			identity:                               "testIdentity2",
			enableVSRoutingInCluster:               true,
			enabledVSRoutingInClusterForIdentities: []string{"testIdentity1"},
			expected:                               false,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and given cluster does exists in the list" +
				"When DoVSRoutingInClusterForIdentity is called" +
				"Then it should return true",
			identity:                               "testIdentity1",
			enableVSRoutingInCluster:               true,
			enabledVSRoutingInClusterForIdentities: []string{"testIdentity1"},
			expected:                               true,
		},
		{
			name: "Given enableVSRoutingInCluster is true, and all VS routing is enabled in all clusters using '*'" +
				"When DoVSRoutingInClusterForIdentity is called" +
				"Then it should return true",
			identity:                               "testIdentity1",
			enableVSRoutingInCluster:               true,
			enabledVSRoutingInClusterForIdentities: []string{"*"},
			expected:                               true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.EnableVSRoutingInCluster = tc.enableVSRoutingInCluster
			p.VSRoutingInClusterEnabledIdentities = tc.enabledVSRoutingInClusterForIdentities
			ResetSync()
			InitializeConfig(p)

			assert.Equal(t, tc.expected, DoVSRoutingInClusterForIdentity(tc.identity))
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

	if IsAdmiralDynamicConfigEnabled() != false {
		t.Errorf("Enable Dynamic Config mismatch, expected false, got %v", IsAdmiralDynamicConfigEnabled())
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

	if GetOperatorIdentityLabelValue() != "operator" {
		t.Errorf("operator identity label value mismatch, expected operator but got %s", GetOperatorIdentityLabelValue())
	}

	shardIdentityLabelKey, shardIdentityLabelValue := GetShardIdentityLabelKeyValueSet()
	if shardIdentityLabelKey != "admiral.io/shardIdentity" && shardIdentityLabelValue != "shard" {
		t.Errorf("shard identity label key value set mismatched, expected admiral.io/shardIdentity and shard but got %s and %s", shardIdentityLabelKey, shardIdentityLabelValue)
	}

	if GetOperatorSecretFilterTags() != "admiral/syncoperator" {
		t.Errorf("operator secret filter tags mismatch, expected admiral/syncoperator, got %s", GetOperatorSecretFilterTags())
	}

	if IsClientDiscoveryEnabled() != true {
		t.Errorf("client discovery enabled mismatch, expected true, got %v", IsClientDiscoveryEnabled())
	}

	if len(GetClientDiscoveryClustersForJobs()) != 0 {
		t.Errorf("clusters for jobs client discovery mismatch, expected 0, got %v", GetClientDiscoveryClustersForJobs())
	}

	if len(GetClientDiscoveryClustersForNumaflow()) != 0 {
		t.Errorf("clusters for numaflow client discovery mismatch, expected 0, got %v", GetClientDiscoveryClustersForNumaflow())
	}

	if !GetArgoRolloutsEnabled() {
		t.Errorf("Argo rollouts enabled mismatch, expected true, got %v", GetArgoRolloutsEnabled())
	}

	if GetEnvoyFilterAdditionalConfig() != "additional" {
		t.Errorf("Envoy filter additional config mismatch, expected additional, got %v", GetEnvoyFilterAdditionalConfig())
	}

	if !GetEnableRoutingPolicy() {
		t.Errorf("Enable routing policy mismatch, expected true, got %v", GetEnableRoutingPolicy())
	}

	if GetHAMode() != "true" {
		t.Errorf("HA mode mismatch, expected true, got %v", GetHAMode())
	}

	if !GetDiffCheckEnabled() {
		t.Errorf("Diff check enabled mismatch, expected true, got %v", GetDiffCheckEnabled())
	}

	if !IsProxyEnvoyFilterEnabled() {
		t.Errorf("Proxy Envoy Filter enabled mismatch, expected true, got %v", IsProxyEnvoyFilterEnabled())
	}

	if !IsDependencyProcessingEnabled() {
		t.Errorf("Dependency processing enabled mismatch, expected true, got %v", IsDependencyProcessingEnabled())
	}

	if GetSeAddressConfigMap() != "configmap" {
		t.Errorf("SE address config map mismatch, expected configmap, got %v", GetSeAddressConfigMap())
	}

	if DeploymentOrRolloutWorkerConcurrency() != 10 {
		t.Errorf("Deployment or rollout worker concurrency mismatch, expected 10, got %v", DeploymentOrRolloutWorkerConcurrency())
	}

	if DependentClusterWorkerConcurrency() != 10 {
		t.Errorf("Dependent cluster worker concurrency mismatch, expected 10, got %v", DependentClusterWorkerConcurrency())
	}

	if DependencyWarmupMultiplier() != 10 {
		t.Errorf("Dependency warmup multiplier mismatch, expected 10, got %v", DependencyWarmupMultiplier())
	}

	if MaxRequestsPerConnection() != 10 {
		t.Errorf("Max requests per connection mismatch, expected 10, got %v", MaxRequestsPerConnection())
	}

	if !IsClientConnectionConfigProcessingEnabled() {
		t.Errorf("Client connection config processing enabled mismatch, expected true, got %v", IsClientConnectionConfigProcessingEnabled())
	}

	if DisableDefaultAutomaticFailover() != true {
		t.Errorf("Disable default automatic failover mismatch, expected true, got %v", DisableDefaultAutomaticFailover())
	}

	if !EnableServiceEntryCache() {
		t.Errorf("Enable service entry cache mismatch, expected true, got %v", EnableServiceEntryCache())
	}

	if !EnableDestinationRuleCache() {
		t.Errorf("Enable destination rule cache mismatch, expected true, got %v", EnableDestinationRuleCache())
	}

	if len(AlphaIdentityList()) != 2 {
		t.Errorf("Alpha identity list mismatch, expected 2, got %v", len(AlphaIdentityList()))
	}

	if AlphaIdentityList()[0] != "identity1" && AlphaIdentityList()[1] != "identity2" {
		t.Errorf("Alpha identity list mismatch, expected identity1 and identity2, got %v and %v", AlphaIdentityList()[0], AlphaIdentityList()[1])
	}

	if !IsOnlyReplicaCountChanged() {
		t.Errorf("Is only replica count changed mismatch, expected true, got %v", IsOnlyReplicaCountChanged())
	}

	if !EnableActivePassive() {
		t.Errorf("Enable active passive mismatch, expected true, got %v", EnableActivePassive())
	}

	if PreventSplitBrain() != true {
		t.Errorf("Prevent split brain mismatch, expected true, got %v", PreventSplitBrain())
	}

	if IsAdmiralStateSyncerMode() != true {
		t.Errorf("Admiral state syncer mode mismatch, expected true, got %v", IsAdmiralStateSyncerMode())
	}

	if GetDefaultWarmupDurationSecs() != int64(10) {
		t.Errorf("Default warmup duration mismatch, expected 10, got %v", GetDefaultWarmupDurationSecs())
	}

	if !DoGenerationCheck() {
		t.Errorf("Do generation check mismatch, expected true, got %v", DoGenerationCheck())
	}

	if GetVSRoutingGateways()[0] != "gateway" {
		t.Errorf("VS routing gateways mismatch, expected gateway, got %v", GetVSRoutingGateways())
	}

	if GetIngressVSExportToNamespace()[0] != "namespace" {
		t.Errorf("Ingress VS export to namespace mismatch, expected namespace, got %v", GetIngressVSExportToNamespace()[0])
	}

	if GetIngressLBPolicy() != "policy" {
		t.Errorf("Ingress LB policy mismatch, expected policy, got %v", GetIngressLBPolicy())
	}

	if ClientInitiatedProcessingEnabled() != true {
		t.Errorf("Client initiated processing enabled mismatch, expected true, got %v", ClientInitiatedProcessingEnabled())
	}

	if GetAdmiralConfigPath() != "someConfig" {
		t.Errorf("Admiral config path mismatch, expected someConfig, got %v", GetAdmiralConfigPath())
	}

	if GetAdditionalEndpointSuffixes()[0] != "suffix1" && GetAdditionalEndpointSuffixes()[1] != "suffix2" {
		t.Errorf("Additional endpoint suffixes mismatch, expected [suffix1, suffix2], got %v", GetAdditionalEndpointSuffixes())
	}

	if GetAdditionalEndpointLabelFilters()[0] != "label1" && GetAdditionalEndpointLabelFilters()[1] != "label2" {
		t.Errorf("Additional endpoint label filters mismatch, expected [label1, label2], got %v", GetAdditionalEndpointLabelFilters())
	}

	if !GetEnableWorkloadDataStorage() {
		t.Errorf("Enable workload data storage mismatch, expected true, got %v", GetEnableWorkloadDataStorage())
	}

	if len(GetIgnoreLabelsAnnotationsVSCopy()) != 3 {
		t.Errorf("ignored labels and annotations for VS copy mismatch, expected 3, got %v", GetIgnoreLabelsAnnotationsVSCopy())
	}

	if !IsStateSyncerCluster("test-k8s") {
		t.Errorf("state syncer cluster mismatch, expected true, got false")
	}

	if IsStateSyncerCluster("not-test-k8s") {
		t.Errorf("state syncer cluster mismatch, expected false, got true")
	}

}

func TestGetCRDIdentityLabelWithCRDIdentity(t *testing.T) {

	admiralParams := GetAdmiralParams()
	backOldIdentity := admiralParams.LabelSet.AdmiralCRDIdentityLabel
	admiralParams.LabelSet.AdmiralCRDIdentityLabel = "identityOld"

	assert.Equalf(t, "identityOld", GetAdmiralCRDIdentityLabel(), "GetCRDIdentityLabel()")

	admiralParams.LabelSet.AdmiralCRDIdentityLabel = backOldIdentity
}

func TestSetArgoRolloutsEnabled(t *testing.T) {
	p := AdmiralParams{}
	p.ArgoRolloutsEnabled = true
	ResetSync()
	InitializeConfig(p)

	SetArgoRolloutsEnabled(true)
	assert.Equal(t, true, GetArgoRolloutsEnabled())
}

func TestSetCartographerFeature(t *testing.T) {
	p := AdmiralParams{}
	ResetSync()
	InitializeConfig(p)

	SetCartographerFeature("feature", "enabled")
	assert.Equal(t, "enabled", wrapper.params.CartographerFeatures["feature"])
}

func TestGetResyncIntervals(t *testing.T) {
	p := AdmiralParams{}
	p.CacheReconcileDuration = time.Minute
	p.SeAndDrCacheReconcileDuration = time.Minute
	ResetSync()
	InitializeConfig(p)

	actual := GetResyncIntervals()

	assert.Equal(t, time.Minute, actual.SeAndDrReconcileInterval)
	assert.Equal(t, time.Minute, actual.UniversalReconcileInterval)
}

func TestShouldPerformRollback(t *testing.T) {
	p := AdmiralParams{}
	testCases := []struct {
		name                        string
		vsRoutingDisabledClusters   []string
		vsRoutingDisabledIdentities []string
		expectedResult              bool
	}{
		{
			name: "Given empty vs routing disabled cluster and disabled identities" +
				"When func shouldPerformRollback is called" +
				"Then the func should return false",
			expectedResult: false,
		},
		{
			name: "Given empty vs routing disabled cluster" +
				"And non-empty vs routing disabled identities" +
				"When func shouldPerformRollback is called" +
				"Then the func should return true",
			expectedResult:              true,
			vsRoutingDisabledIdentities: []string{"identity1", "identity2"},
		},
		{
			name: "Given non-empty vs routing disabled cluster" +
				"And empty vs routing disabled identities" +
				"When func shouldPerformRollback is called" +
				"Then the func should return true",
			expectedResult:            true,
			vsRoutingDisabledClusters: []string{"cluster1", "cluster2"},
		},
		{
			name: "Given non-empty vs routing disabled cluster" +
				"And non-empty vs routing disabled identities" +
				"When func shouldPerformRollback is called" +
				"Then the func should return true",
			expectedResult:              true,
			vsRoutingDisabledIdentities: []string{"identity1", "identity2"},
			vsRoutingDisabledClusters:   []string{"cluster1", "cluster2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.VSRoutingInClusterDisabledClusters = tc.vsRoutingDisabledClusters
			p.VSRoutingInClusterDisabledIdentities = tc.vsRoutingDisabledIdentities
			ResetSync()
			InitializeConfig(p)
			actual := ShouldInClusterVSRoutingPerformRollback()
			assert.Equal(t, tc.expectedResult, actual)
		})
	}

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
