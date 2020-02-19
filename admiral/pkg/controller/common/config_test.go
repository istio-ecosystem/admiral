package common

import "testing"

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
	}

	p.LabelSet.WorkloadIdentityKey="BAD_LABEL"

	InitializeConfig(p)

	if GetKubeconfigPath() != "testdata/fake.config" {
		t.Errorf("Kubeconfig path mismatch, expected testdata/fake.config, got %v", GetKubeconfigPath())
	}
	if GetWorkloadIdentifier() != "identity" {
		t.Errorf("Kubeconfig path mismatch, expected identity, got %v", GetKubeconfigPath())
	}
	if GetKubeconfigPath() != "testdata/fake.config" {
		t.Errorf("Kubeconfig path mismatch, expected testdata/fake.config, got %v", GetKubeconfigPath())
	}
	if GetSANPrefix() != "prefix" {
		t.Errorf("Kubeconfig path mismatch, expected prefix, got %v", GetKubeconfigPath())
	}
	if GetHostnameSuffix() != "mesh" {
		t.Errorf("Kubeconfig path mismatch, expected mesh, got %v", GetKubeconfigPath())
	}
	if GetSyncNamespace() != "ns" {
		t.Errorf("Kubeconfig path mismatch, expected ns, got %v", GetKubeconfigPath())
	}
	if GetEnableSAN() != true {
		t.Errorf("Kubeconfig path mismatch, expected true, got %v", GetKubeconfigPath())
	}


}