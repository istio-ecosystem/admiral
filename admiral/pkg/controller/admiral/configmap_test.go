package admiral

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	_ "github.com/istio-ecosystem/admiral/admiral/pkg/test"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func init() {
	initConfig("se-address-configmap")
}

func initConfig(seAdressCM string) {
	p := common.AdmiralParams{
		KubeconfigPath:             "testdata/fake.config",
		LabelSet:                   &common.LabelSet{},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheReconcileDuration:     time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace:      "default",
		Profile:                    common.AdmiralProfileDefault,
		SeAddressConfigmap:         seAdressCM,
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"

	p.LabelSet.EnvKey = "admiral.io/env"
	common.InitializeConfig(p)
}

func TestConfigMapController_GetConfigMap(t *testing.T) {
	configmapController := ConfigMapController{
		ConfigmapNamespace: "admiral",
	}

	client := fake.NewSimpleClientset()
	cm := createConfigMap("se-address-configmap", "admiral", map[string]string{"foo": "bar"}) //differentiating from a new/empty cm
	ctx := context.Background()
	_, err := client.CoreV1().ConfigMaps("admiral").Create(ctx, &cm, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("%v", err)
	}
	configmapController.K8sClient = client

	configmapController2 := ConfigMapController{
		ConfigmapNamespace: "admiral",
	}

	client2 := fake.NewSimpleClientset()
	cm2 := createConfigMap("se-address-configmap2", "admiral", map[string]string{"foo": "bar"}) //differentiating from a new/empty cm
	ctx2 := context.Background()
	_, err = client2.CoreV1().ConfigMaps("admiral").Create(ctx2, &cm2, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("%v", err)
	}
	configmapController2.K8sClient = client2

	emptyConfigmapController := ConfigMapController{
		ConfigmapNamespace: "admiral",
	}
	emptyClient := fake.NewSimpleClientset()
	emptyCM := createConfigMap("se-address-configmap", "admiral", nil)
	emptyConfigmapController.K8sClient = emptyClient

	emptyConfigmapController2 := ConfigMapController{
		ConfigmapNamespace: "admiral",
	}
	emptyClient2 := fake.NewSimpleClientset()
	emptyCM2 := createConfigMap("se-address-configmap2", "admiral", nil)
	emptyConfigmapController2.K8sClient = emptyClient2

	testCases := []struct {
		name                string
		configMapController *ConfigMapController
		expectedConfigMap   *v1.ConfigMap
		seAdressCMName      string
		expectedError       error
	}{
		{
			name:                "given default configmap name in AdmiralParams, should return configmap",
			configMapController: &configmapController,
			expectedConfigMap:   &cm,
			expectedError:       nil,
		},
		{
			name:                "given default configmap name in AdmiralParams, should return newly created configmap",
			configMapController: &emptyConfigmapController,
			expectedConfigMap:   &emptyCM,
			expectedError:       nil,
		},
		{
			name:                "given se-address-configmap2 in AdmiralParams, should return configmap with addressconfigmap2",
			configMapController: &configmapController2,
			expectedConfigMap:   &cm2,
			seAdressCMName:      "se-address-configmap2",
			expectedError:       nil,
		},
		{
			name:                "given se-address-configmap2 in AdmiralParams, should return newly created configmap with addressconfigmap2",
			configMapController: &emptyConfigmapController2,
			expectedConfigMap:   &emptyCM2,
			seAdressCMName:      "se-address-configmap2",
			expectedError:       nil,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if len(c.seAdressCMName) > 0 {
				common.ResetSync()
				initConfig(c.seAdressCMName)
			}
			cm, err := c.configMapController.GetConfigMap(ctx)
			if err == nil && c.expectedError == nil {
				//we're fine
			} else if c.expectedError == nil && err != nil {
				t.Errorf("Unexpected error. Err: %v", err)
			} else if err.Error() != c.expectedError.Error() {
				t.Errorf("Error mismatch. Expected %v but got %v", c.expectedError, err)
			}
			if !cmp.Equal(cm, c.expectedConfigMap) {
				log.Info("Object Diff: " + cmp.Diff(cm, c.expectedConfigMap))
				t.Errorf("Configmap Mismatch. Expected %v but got %v", c.expectedConfigMap, cm)
			}

		})
	}
}

func createConfigMap(name string, namespace string, labels map[string]string) v1.ConfigMap {
	cm := v1.ConfigMap{}
	cm.Name = name
	cm.Namespace = namespace
	cm.Labels = labels
	return cm
}

func TestConfigMapController_PutConfigMap(t *testing.T) {
	configmapController := ConfigMapController{
		ConfigmapNamespace: "admiral-remote-ctx",
	}

	client := fake.NewSimpleClientset()
	cm := v1.ConfigMap{}
	cm.Name = "se-address-configmap"
	cm.Namespace = "admiral-remote-ctx"
	ctx := context.Background()
	_, err := client.CoreV1().ConfigMaps("admiral-remote-ctx").Create(ctx, &cm, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("%v", err)
	}
	configmapController.K8sClient = client

	cm.Data = map[string]string{"Foo": "Bar"}

	err = configmapController.PutConfigMap(ctx, &cm)

	if err != nil {
		t.Errorf("No error expected. Err: %v", err)
	}

}
