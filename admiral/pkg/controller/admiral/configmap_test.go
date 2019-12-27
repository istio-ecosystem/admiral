package admiral

import (
	"errors"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"gopkg.in/yaml.v2"
	"k8s.io/api/core/v1"
	"testing"
)

func buildFakeConfigMapFromAddressStore(addressStore *common.ServiceEntryAddressStore, resourceVersion string) *v1.ConfigMap{
	bytes,_ := yaml.Marshal(addressStore)

	cm := v1.ConfigMap{
		Data: map[string]string{"serviceEntryAddressStore": string(bytes)},
	}
	cm.Name="se-address-configmap"
	cm.Namespace="admiral-remote-ctx"
	cm.ResourceVersion=resourceVersion
	return &cm
}

func TestValidateConfigmapBeforePutting(t *testing.T) {

	legalStore := common.ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.a.mesh": common.LocalAddressPrefix + ".10.1"},
		Addresses: []string{common.LocalAddressPrefix + ".10.1"},
	}

	illegalStore := common.ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.a.mesh": common.LocalAddressPrefix + ".10.1"},
		Addresses: []string{common.LocalAddressPrefix + ".10.1","1.2.3.4"},
	}

	testCases := []struct{
		name string
		configMap	 *v1.ConfigMap
		expectedError	error
	}{
		{
			name: "should not throw error on legal configmap",
			configMap:     buildFakeConfigMapFromAddressStore(&legalStore, "123"),
			expectedError: nil,
		},
		{
			name: "should throw error on no resourceversion",
			configMap:     buildFakeConfigMapFromAddressStore(&legalStore, ""),
			expectedError: errors.New("resourceversion required"),
		},
		{
			name: "should throw error on length mismatch",
			configMap:     buildFakeConfigMapFromAddressStore(&illegalStore, "123"),
			expectedError: errors.New("address cache length mismatch"),
		},

	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			errorResult := validateConfigmapBeforePutting(c.configMap)
			if errorResult==nil && c.expectedError==nil {
				//we're fine
			} else if errorResult.Error() != c.expectedError.Error() {
				t.Errorf("Error mismatch. Expected %v but got %v", c.expectedError, errorResult)
			}
		})
	}

}