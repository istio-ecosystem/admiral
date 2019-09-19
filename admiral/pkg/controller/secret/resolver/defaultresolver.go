package resolver

//The default resolver uses the k8s secret payload to be the valid kubeconfig for the remote cluster

func (r DefaultResolver) FetchKubeConfig(secretName string, kubeConfig []byte) ([]byte, error) {
	return kubeConfig, nil
}

type DefaultResolver struct {

}

func NewDefaultResolver() (SecretResolver, error) {
	resolver := DefaultResolver{}
	return resolver, nil
}
