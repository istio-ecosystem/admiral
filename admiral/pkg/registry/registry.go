package registry

// IdentityConfiguration is an interface to fetch configuration from a registry
// backend. The backend can provide an API to give configurations per identity,
// or if given a cluster name, it will provide the configurations for all
// the identities present in that cluster.
type IdentityConfiguration interface {
	GetByIdentityByName(identityAlias string) error
	GetByClusterName(clusterName string) error
}
