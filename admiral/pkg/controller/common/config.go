package common

import (
	"sync"
	"time"
)

var admiralParams = AdmiralParams{
	LabelSet: &LabelSet{},
}

var once sync.Once

func InitializeConfig(params AdmiralParams) {
	once.Do(func() {
		admiralParams = params
	})
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