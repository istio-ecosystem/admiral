package admiral

import (
	"errors"
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	_ "github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
)


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
			} else if c.expectedError == nil && err != nil {
				t.Errorf("Unexpected error. Err: %v", err)
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

func TestNewConfigMapController(t *testing.T) {
	testCases := []struct {
		name string
		kubeconfigPath string
		namespace string
		expectedError error
	}{
		{
			name: "Fails creating an in-cluster config while out of a cluster",
			kubeconfigPath: "",
			namespace: "ns",
			expectedError: errors.New("unable to load in-cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined"),
		},
		{
			name: "Kubeconfig config",
			kubeconfigPath: "../../test/resources/admins@fake-cluster.k8s.local",
			namespace: "ns",
			expectedError: nil,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			common.SetKubeconfigPath(c.kubeconfigPath)
			controller, err := NewConfigMapController()
			if err==nil && c.expectedError==nil {
				//only do these in an error-less context
				if c.namespace != controller.ConfigmapNamespace {
					t.Errorf("Namespace mismatch. Expected %v but got %v", c.namespace, controller.ConfigmapNamespace)
				}
				if controller.K8sClient.CoreV1() == nil {
					t.Errorf("Clientset is nil")
				}
			} else if err.Error() != c.expectedError.Error() {
				t.Errorf("Error mismatch. Expected %v but got %v", c.expectedError, err)
			}
		})
	}

}

func TestConfigMapController_PutConfigMap(t *testing.T) {
	configmapController := ConfigMapController{
		ConfigmapNamespace: "admiral-remote-ctx",
	}

	client := fake.NewSimpleClientset()
	cm := v1.ConfigMap{}
	cm.Name = "se-address-configmap"
	cm.Namespace="admiral-remote-ctx"
	_, err := client.CoreV1().ConfigMaps("admiral-remote-ctx").Create(&cm)
	if err != nil {
		t.Errorf("%v", err)
	}
	configmapController.K8sClient = client

	cm.Data = map[string]string{"Foo":"Bar"}

	err = configmapController.PutConfigMap(&cm)

	if err != nil {
		t.Errorf("No error expected. Err: %v", err)
	}

}