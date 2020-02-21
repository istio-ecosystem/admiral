package clusters

import (
	"errors"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	k8sAppsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"strconv"
	"testing"

	k8sV1 "k8s.io/api/core/v1"
)

func TestGetMeshPorts(t *testing.T) {

	annotatedPort := 8090
	defaultServicePort := uint32(8080)

	defaultK8sSvcPortNoName := k8sV1.ServicePort{Port: int32(defaultServicePort)}
	defaultK8sSvcPort := k8sV1.ServicePort{Name: "default", Port: int32(defaultServicePort)}
	meshK8sSvcPort := k8sV1.ServicePort{Name: "mesh", Port: int32(annotatedPort)}

	serviceMeshPorts := []k8sV1.ServicePort{defaultK8sSvcPort, meshK8sSvcPort}

	serviceMeshPortsOnlyDefault := []k8sV1.ServicePort{defaultK8sSvcPortNoName}

	service := k8sV1.Service{
		ObjectMeta: v1.ObjectMeta{Name: "server", Labels:map[string]string{"asset": "Intuit.platform.mesh.server"}},
		Spec: k8sV1.ServiceSpec{Ports: serviceMeshPorts},
	}
	deployment := k8sAppsV1.Deployment{
		Spec: k8sAppsV1.DeploymentSpec{Template:coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{Annotations:map[string]string{common.SidecarEnabledPorts: strconv.Itoa(annotatedPort)}},
		}}}

	ports := map[string]uint32{"http": uint32(annotatedPort)}

	portsFromDefaultSvcPort := map[string]uint32{"http": defaultServicePort}

	emptyPorts := map[string]uint32{}

	testCases := []struct {
		name   string
		clusterName   string
		service k8sV1.Service
		deployment   k8sAppsV1.Deployment
		expected map[string]uint32
	}{
		{
			name:    "should return a port based on annotation",
			service: service,
			deployment: deployment,
			expected: ports,
		},
		{
			name:    "should return a default port",
			service: k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels:map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec: k8sV1.ServiceSpec{Ports: serviceMeshPortsOnlyDefault},
			},
			deployment: k8sAppsV1.Deployment{
				Spec: k8sAppsV1.DeploymentSpec{Template:coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{Annotations:map[string]string{}},
				}}},
			expected: portsFromDefaultSvcPort,
		},
		{
			name:    "should return empty ports",
			service: k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels:map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec: k8sV1.ServiceSpec{Ports: nil},
			},
			deployment: k8sAppsV1.Deployment{
				Spec: k8sAppsV1.DeploymentSpec{Template:coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{Annotations:map[string]string{}},
				}}},
			expected: emptyPorts,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			meshPorts := GetMeshPorts(c.clusterName, &c.service, &c.deployment)
			if !reflect.DeepEqual(meshPorts, c.expected) {
				t.Errorf("Wanted meshPorts: %v, got: %v", c.expected, meshPorts)
			}
		})
	}
}

func TestValidateConfigmapBeforePutting(t *testing.T) {

	legalStore := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.a.mesh": common.LocalAddressPrefix + ".10.1"},
		Addresses: []string{common.LocalAddressPrefix + ".10.1"},
	}

	illegalStore := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.a.mesh": common.LocalAddressPrefix + ".10.1"},
		Addresses: []string{common.LocalAddressPrefix + ".10.1","1.2.3.4"},
	}

	emptyCM := coreV1.ConfigMap{}
	emptyCM.ResourceVersion = "123"

	testCases := []struct{
		name string
		configMap	 *coreV1.ConfigMap
		expectedError	error
	}{
		{
			name: "should not throw error on legal configmap",
			configMap:     buildFakeConfigMapFromAddressStore(&legalStore, "123"),
			expectedError: nil,
		},
		{
			name: "should not throw error on empty configmap",
			configMap:     &emptyCM,
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
			//errorResult := ValidateConfigmapBeforePutting(c.configMap)
			//if errorResult==nil && c.expectedError==nil {
			//	//we're fine
			//} else if c.expectedError == nil && errorResult != nil{
			//	t.Errorf("Unexpected error. Err: %v", errorResult)
			//} else if errorResult.Error() != c.expectedError.Error() {
			//	t.Errorf("Error mismatch. Expected %v but got %v", c.expectedError, errorResult)
			//}
		})
	}

}
