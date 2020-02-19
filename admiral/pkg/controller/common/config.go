package common

import (
	"istio.io/istio/pkg/log"
	"sync"
	"time"
)

var admiralParams = AdmiralParams{
	LabelSet: &LabelSet{},
}

var once sync.Once

func InitializeConfig(params AdmiralParams) {
	var initHappened = false
	once.Do(func() {
		admiralParams = params
		initHappened = true
	})
	if !initHappened {
		log.Warn("InitializeConfig was called but didn't take effect. It can only be called once, and thus has already been initialized. Please ensure you aren't re-initializing the config.")
	}
}


func GetAdmiralParams() AdmiralParams {
	return admiralParams
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

func GetHostnameSuffix() string {
	return admiralParams.HostnameSuffix
}

func GetWorkloadIdentifier() string {
	return admiralParams.LabelSet.WorkloadIdentityKey
}

///Setters - be careful

func SetKubeconfigPath(path string) {
	admiralParams.KubeconfigPath = path
}