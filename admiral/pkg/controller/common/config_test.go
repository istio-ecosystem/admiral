package common

import (
	"testing"
	"time"
)

func TestConfigManagement(t *testing.T) {

	//Initial state comes from the init method in configInitializer.go
	//p := AdmiralParams{
	//	KubeconfigPath: "testdata/fake.config",
	//	LabelSet: &LabelSet{},
	//	EnableSAN: true,
	//	SANPrefix: "prefix",
	//	HostnameSuffix: "mesh",
	//	SyncNamespace: "ns",
	//}
	//
	//p.LabelSet.WorkloadIdentityKey="identity"

	//trying to initialize again. If the singleton pattern works, none of these will have changed
	p := AdmiralParams{
		KubeconfigPath: "DIFFERENT",
		LabelSet: &LabelSet{},
		EnableSAN: false,
		SANPrefix: "BAD_PREFIX",
		HostnameSuffix: "NOT_MESH",
		SyncNamespace: "NOT_A_NAMESPACE",
		CacheRefreshDuration: time.Hour,
		ClusterRegistriesNamespace: "NOT_DEFAULT",
		DependenciesNamespace: "NOT_DEFAULT",
		SecretResolver: "INSECURE_RESOLVER",

	}

	p.LabelSet.WorkloadIdentityKey="BAD_LABEL"

	InitializeConfig(p)

	if GetWorkloadIdentifier() != "identity" {
		t.Errorf("Workload identifier mismatch, expected identity, got %v", GetWorkloadIdentifier())
	}
	if GetKubeconfigPath() != "testdata/fake.config" {
		t.Errorf("Kubeconfig path mismatch, expected testdata/fake.config, got %v", GetKubeconfigPath())
	}
	if GetSANPrefix() != "prefix" {
		t.Errorf("San prefix mismatch, expected prefix, got %v", GetSANPrefix())
	}
	if GetHostnameSuffix() != "mesh" {
		t.Errorf("Hostname suffix mismatch, expected mesh, got %v", GetHostnameSuffix())
	}
	if GetSyncNamespace() != "ns" {
		t.Errorf("Sync namespace mismatch, expected ns, got %v", GetSyncNamespace())
	}
	if GetEnableSAN() != true {
		t.Errorf("Enable SAN mismatch, expected true, got %v", GetEnableSAN())
	}
	if GetCacheRefreshDuration() != time.Minute {
		t.Errorf("Cachee refresh duration mismatch, expected %v, got %v", time.Minute, GetCacheRefreshDuration())
	}
	if GetClusterRegistriesNamespace() != "default" {
		t.Errorf("Cluster registry namespace mismatch, expected default, got %v", GetClusterRegistriesNamespace())
	}
	if GetDependenciesNamespace() != "default" {
		t.Errorf("Dependency namespace mismatch, expected default, got %v", GetDependenciesNamespace())
	}
	if GetSecretResolver() != "" {
		t.Errorf("Secret resolver mismatch, expected empty string, got %v", GetSecretResolver())
	}

}