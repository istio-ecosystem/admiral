package common

import (
	"strings"
	"sync"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/monitoring"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"github.com/matryer/resync"
	log "github.com/sirupsen/logrus"
)

type admiralParamsWrapper struct {
	params AdmiralParams
	sync.RWMutex
	resync.Once
}

// Singleton
var wrapper = admiralParamsWrapper{
	params: AdmiralParams{
		LabelSet: &LabelSet{},
	},
}

func ResetSync() {
	wrapper.Reset()
}

func InitializeConfig(params AdmiralParams) {
	var initHappened = false
	wrapper.Do(func() {
		wrapper.Lock()
		defer wrapper.Unlock()
		wrapper.params = params
		if wrapper.params.LabelSet == nil {
			wrapper.params.LabelSet = &LabelSet{}
		}
		err := monitoring.InitializeMonitoring()
		if err != nil {
			log.Errorf("failed to setup monitoring: %v", err)
		}
		initHappened = true
	})
	if initHappened {
		log.Info("InitializeConfig was called.")
	} else {
		log.Warn("InitializeConfig was called but didn't take effect. It can only be called once, and thus has already been initialized. Please ensure you aren't re-initializing the config.")
	}
}

func GetAdmiralParams() AdmiralParams {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params
}

func GetAdmiralProfile() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.Profile
}

func GetArgoRolloutsEnabled() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.ArgoRolloutsEnabled
}

func GetSecretFilterTags() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.SecretFilterTags
}

func GetKubeconfigPath() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.KubeconfigPath
}

func GetCacheRefreshDuration() time.Duration {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.CacheReconcileDuration
}

func GetClusterRegistriesNamespace() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.ClusterRegistriesNamespace
}

func GetDependenciesNamespace() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.DependenciesNamespace
}

func GetSyncNamespace() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.SyncNamespace
}

func GetEnableSAN() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableSAN
}

func GetSANPrefix() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.SANPrefix
}

func GetAdmiralConfigPath() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.AdmiralConfig
}

func GetLabelSet() *LabelSet {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet
}

func GetAdditionalEndpointSuffixes() []string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.AdditionalEndpointSuffixes
}

func GetAdditionalEndpointLabelFilters() []string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.AdditionalEndpointLabelFilters
}

func GetEnableWorkloadDataStorage() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableWorkloadDataStorage
}

func GetHostnameSuffix() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.HostnameSuffix
}

func GetWorkloadIdentifier() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet.WorkloadIdentityKey
}

func GetPartitionIdentifier() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet.IdentityPartitionKey
}

func GetTrafficConfigIdentifier() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet.TrafficConfigIdentityKey
}

func GetAdmiralCRDIdentityLabel() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet.AdmiralCRDIdentityLabel
}

func GetRoutingPolicyLabel() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet.WorkloadIdentityKey
}

func GetWorkloadSidecarUpdate() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.WorkloadSidecarUpdate
}

func GetEnvoyFilterVersion() []string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	if len(strings.TrimSpace(wrapper.params.EnvoyFilterVersion)) == 0 {
		return []string{}
	}
	return strings.Split(wrapper.params.EnvoyFilterVersion, ",")
}

func GetDeprecatedEnvoyFilterVersion() []string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	if len(strings.TrimSpace(wrapper.params.DeprecatedEnvoyFilterVersion)) == 0 {
		return []string{}
	}
	return strings.Split(wrapper.params.DeprecatedEnvoyFilterVersion, ",")
}

func GetEnvoyFilterAdditionalConfig() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnvoyFilterAdditionalConfig
}

func GetEnableRoutingPolicy() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableRoutingPolicy
}

func GetWorkloadSidecarName() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.WorkloadSidecarName
}

func GetEnvKey() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet.EnvKey
}

func GetMetricsEnabled() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.MetricsEnabled
}

func IsPersonaTrafficConfig() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.TrafficConfigPersona
}

// This function is used to determine if a feature is enabled or not.
// If the feature is not present in the list, it is assumed to be enabled.
// Also any value other than "disabled" is assumed to be enabled.
func IsCartographerFeatureDisabled(featureName string) bool {
	wrapper.RLock()
	defer wrapper.RUnlock()

	if wrapper.params.CartographerFeatures == nil {
		return false
	}
	// If the feature exists in the list and is set to disabled, return true
	if val, ok := wrapper.params.CartographerFeatures[featureName]; ok {
		return val == "disabled"
	} else {
		return false
	}
}

func IsDefaultPersona() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return !wrapper.params.TrafficConfigPersona
}

func GetHAMode() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.HAMode
}

func GetDiffCheckEnabled() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableDiffCheck
}

func IsProxyEnvoyFilterEnabled() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableProxyEnvoyFilter
}

func IsDependencyProcessingEnabled() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableDependencyProcessing
}

func GetSeAddressConfigMap() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.SeAddressConfigmap
}

func DeploymentOrRolloutWorkerConcurrency() int {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.DeploymentOrRolloutWorkerConcurrency
}

func DependentClusterWorkerConcurrency() int {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.DependentClusterWorkerConcurrency
}

func DependencyWarmupMultiplier() int {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.DependencyWarmupMultiplier
}

func MaxRequestsPerConnection() int32 {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.MaxRequestsPerConnection
}

func IsAbsoluteFQDNEnabled() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableAbsoluteFQDN
}

func IsClientConnectionConfigProcessingEnabled() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableClientConnectionConfigProcessing
}

func IsAbsoluteFQDNEnabledForLocalEndpoints() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableAbsoluteFQDNForLocalEndpoints
}

func DisableDefaultAutomaticFailover() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.DisableDefaultAutomaticFailover
}

func EnableServiceEntryCache() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableServiceEntryCache
}

func EnableDestinationRuleCache() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableDestinationRuleCache
}

func AlphaIdentityList() []string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.AlphaIdentityList
}

func SetKubeconfigPath(path string) {
	wrapper.Lock()
	defer wrapper.Unlock()
	wrapper.params.KubeconfigPath = path
}

func SetEnablePrometheus(value bool) {
	wrapper.Lock()
	defer wrapper.Unlock()
	wrapper.params.MetricsEnabled = value
}

func SetArgoRolloutsEnabled(value bool) {
	wrapper.Lock()
	defer wrapper.Unlock()
	wrapper.params.ArgoRolloutsEnabled = value
}

func SetCartographerFeature(featureName string, val string) {
	wrapper.Lock()
	defer wrapper.Unlock()
	if wrapper.params.CartographerFeatures == nil {
		wrapper.params.CartographerFeatures = make(map[string]string)
	}
	wrapper.params.CartographerFeatures[featureName] = val
}

func GetGatewayAssetAliases() []string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.GatewayAssetAliases
}

func DisableIPGeneration() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.DisableIPGeneration
}

func EnableActivePassive() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableActivePassive
}

func EnableExportTo(identityOrCname string) bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	if wrapper.params.ExportToIdentityList != nil {
		for _, identity := range wrapper.params.ExportToIdentityList {
			if identity != "" && (identity == "*" || strings.Contains(strings.ToLower(identityOrCname), strings.ToLower(identity))) && wrapper.params.EnableSWAwareNSCaches {
				return true
			}
		}
	}
	return false
}

func EnableSWAwareNSCaches() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableSWAwareNSCaches
}

func GetIngressVSExportToNamespace() []string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.IngressVSExportToNamespaces
}

func IsVSBasedRoutingEnabled() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableVSRouting
}

func GetVSRoutingGateways() []string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.VSRoutingGateways
}

func DoGenerationCheck() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableGenerationCheck
}

func GetResyncIntervals() util.ResyncIntervals {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return util.ResyncIntervals{
		UniversalReconcileInterval: wrapper.params.CacheReconcileDuration,
		SeAndDrReconcileInterval:   wrapper.params.SeAndDrCacheReconcileDuration,
	}
}

func GetExportToMaxNamespaces() int {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.ExportToMaxNamespaces
}

func IsAdmiralStateSyncerMode() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.AdmiralStateSyncerMode
}

func GetDefaultWarmupDurationSecs() int64 {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.DefaultWarmupDurationSecs
}

func IsAdmiralOperatorMode() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.AdmiralOperatorMode
}

func GetOperatorSyncNamespace() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.OperatorSyncNamespace
}

func GetOperatorIdentityLabelValue() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.OperatorIdentityValue
}

func GetShardIdentityLabelKeyValueSet() (string, string) {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet.ShardIdentityLabelKey, wrapper.params.ShardIdentityValue
}

func GetOperatorSecretFilterTags() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.OperatorSecretFilterTags
}
