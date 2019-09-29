package clusters

import (
	"github.com/admiral/admiral/pkg/controller/common"
	k8sAppsV1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
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
		Spec: k8sAppsV1.DeploymentSpec{Template:v12.PodTemplateSpec{
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
				Spec: k8sAppsV1.DeploymentSpec{Template:v12.PodTemplateSpec{
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
				Spec: k8sAppsV1.DeploymentSpec{Template:v12.PodTemplateSpec{
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