package common

import "time"

//Because the admiralParams are not a global singleton, we'll initialize them here and then use setters as needed throughout the tests. Import this package to ensure the initialization happens in your test
//If you're not using anything from the package (and getting an unused import error), import it with the name of _ (_ "github.com/istio-ecosystem/admiral/admiral/pkg/controller/common")
func init() {
	p := AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &LabelSet{},
		EnableSAN: true,
		SANPrefix: "prefix",
		HostnameSuffix: "mesh",
		SyncNamespace: "ns",
		CacheRefreshDuration: time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace: "default",
		SecretResolver: "",

	}

	p.LabelSet.WorkloadIdentityKey="identity"

	InitializeConfig(p)
}