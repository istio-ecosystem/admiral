package resolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDefaultResolver(t *testing.T) {
	resolver, err := NewDefaultResolver()
	assert.NotNil(t, resolver, "DefaultResolver should not be nil")
	assert.Nil(t, err, "Error while new instance creation should be nil")
}

func TestDefaultResolver_FetchKubeConfig(t *testing.T) {
	expectedKubeConfig := `
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: ca_data
    server: https://example.com
  name: example-cluster
contexts:
- context:
    cluster: example-cluster
    user: example-user
  name: example-context
current-context: example-context
kind: Config
preferences: {}
users:
- name: example-user
  user:
    client-certificate-data: cert_data
    client-key-data: key_data
`
	resolver, _ := NewDefaultResolver()
	kconfig, err := resolver.FetchKubeConfig("", []byte(expectedKubeConfig))
	assert.Equal(t, []byte(expectedKubeConfig), kconfig)
	assert.Nil(t, err, "Expected error to be nil")
}
