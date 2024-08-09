package resolver

// Interface for fetching kubeconfig of a remote cluster from k8s secret

type SecretResolver interface {
	FetchKubeConfig(secretName string, kubeConfig []byte) ([]byte, error)
	DeleteClusterFromCache(clusterName string) error
}
