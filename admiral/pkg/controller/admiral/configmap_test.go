package admiral

import (
	"errors"
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
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

func TestConfigMapController_GetConfigMap(t *testing.T) {
	configmapController := ConfigMapController{
		ConfigmapNamespace: "admiral",
	}

	client := fake.NewSimpleClientset()
	cm := v1.ConfigMap{}
	cm.Name = "se-address-configmap"
	cm.Namespace="admiral"
	cm.Labels= map[string]string{"foo":"bar"}//differentiating from a new/empty cm
	_, err := client.CoreV1().ConfigMaps("admiral").Create(&cm)
	if err != nil {
		t.Errorf("%v", err)
	}
	configmapController.K8sClient = client


	emptyConfigmapController := ConfigMapController{
		ConfigmapNamespace: "admiral",
	}

	emptyClient := fake.NewSimpleClientset()
	emptyCM := v1.ConfigMap{}
	emptyCM.Name = "se-address-configmap"
	emptyCM.Namespace="admiral"
	emptyConfigmapController.K8sClient = emptyClient

	testCases := []struct{
		name string
		configMapController	 *ConfigMapController
		expectedConfigMap 	*v1.ConfigMap
		expectedError	error
	}{
		{
			name: "should return confirmap",
			configMapController:     &configmapController,
			expectedConfigMap: &cm,
			expectedError: nil,
		},
		{
			name: "should return newly created configmap",
			configMapController:     &emptyConfigmapController,
			expectedConfigMap: &emptyCM,
			expectedError: nil,
		},

	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			cm, err := c.configMapController.GetConfigMap()
			if err==nil && c.expectedError==nil {
				//we're fine
			} else if err.Error() != c.expectedError.Error() {
				t.Errorf("Error mismatch. Expected %v but got %v", c.expectedError, err)
			}
			if !cmp.Equal(cm, c.expectedConfigMap) {
				logrus.Info("Object Diff: " + cmp.Diff(cm, c.expectedConfigMap))
				t.Errorf("Configmap Mismatch. Expected %v but got %v", c.expectedConfigMap, cm)
			}

		})
	}
}