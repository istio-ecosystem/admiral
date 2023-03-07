package common

import (
	"time"

	"github.com/matryer/resync"
	log "github.com/sirupsen/logrus"
)

var admiralParams = AdmiralParams{
	LabelSet: &LabelSet{},
}

var once resync.Once

func ResetSync() {
	once.Reset()
}

func InitializeConfig(params AdmiralParams) {
	var initHappened = false
	once.Do(func() {
		admiralParams = params
		initHappened = true
		InitializeMetrics()
	})
	if initHappened {
		log.Info("InitializeConfig was called.")
	} else {
		log.Warn("InitializeConfig was called but didn't take effect. It can only be called once, and thus has already been initialized. Please ensure you aren't re-initializing the config.")
	}
}

func GetAdmiralParams() AdmiralParams {
	return admiralParams
}

func GetArgoRolloutsEnabled() bool {
	return admiralParams.ArgoRolloutsEnabled
}

func GetKubeconfigPath() string {
	return admiralParams.KubeconfigPath
}

func GetCacheRefreshDuration() time.Duration {
	return admiralParams.CacheRefreshDuration
}

func GetClusterRegistriesNamespace() string {
	return admiralParams.ClusterRegistriesNamespace
}

func GetDependenciesNamespace() string {
	return admiralParams.DependenciesNamespace
}

func GetSyncNamespace() string {
	return admiralParams.SyncNamespace
}

func GetEnableSAN() bool {
	return admiralParams.EnableSAN
}

func GetSANPrefix() string {
	return admiralParams.SANPrefix
}

func GetSecretResolver() string {
	return admiralParams.SecretResolver
}

func GetLabelSet() *LabelSet {
	return admiralParams.LabelSet
}

func GetAdditionalEndpointSuffixes() []string {
	return admiralParams.AdditionalEndpointSuffixes
}

func GetAdditionalEndpointLabelFilters() []string {
	return admiralParams.AdditionalEndpointLabelFilters
}

func GetHostnameSuffix() string {
	return admiralParams.HostnameSuffix
}

func GetWorkloadIdentifier() string {
	return admiralParams.LabelSet.WorkloadIdentityKey
}

func GetGlobalTrafficDeploymentLabel() string {
	return admiralParams.LabelSet.GlobalTrafficDeploymentLabel
}

func GetRoutingPolicyLabel() string {
	return admiralParams.LabelSet.WorkloadIdentityKey
}

func GetWorkloadSidecarUpdate() string {
	return admiralParams.WorkloadSidecarUpdate
}

func GetEnvoyFilterVersion() string {
	return admiralParams.EnvoyFilterVersion
}

func GetEnvoyFilterAdditionalConfig() string {
	return admiralParams.EnvoyFilterAdditionalConfig
}

func GetEnableRoutingPolicy() bool {
	return admiralParams.EnableRoutingPolicy
}

func GetWorkloadSidecarName() string {
	return admiralParams.WorkloadSidecarName
}

func GetEnvKey() string {
	return admiralParams.LabelSet.EnvKey
}

func GetMetricsEnabled() bool {
	return admiralParams.MetricsEnabled
}

///Setters - be careful

func SetKubeconfigPath(path string) {
	admiralParams.KubeconfigPath = path
}

// for unit test only
func SetEnablePrometheus(value bool) {
	admiralParams.MetricsEnabled = value
}
