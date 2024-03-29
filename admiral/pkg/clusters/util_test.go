package clusters

import (
	"errors"
	"reflect"
	"strconv"
	"testing"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	k8sAppsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	k8sV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestGetMeshPorts(t *testing.T) {
	var (
		annotatedPort               = 8090
		annotatedSecondPort         = 8091
		defaultServicePort          = uint32(8080)
		ports                       = map[string]uint32{"http": uint32(annotatedPort)}
		portsDiffTargetPort         = map[string]uint32{"http": uint32(80)}
		grpcPorts                   = map[string]uint32{"grpc": uint32(annotatedPort)}
		grpcWebPorts                = map[string]uint32{"grpc-web": uint32(annotatedPort)}
		http2Ports                  = map[string]uint32{"http2": uint32(annotatedPort)}
		portsFromDefaultSvcPort     = map[string]uint32{"http": defaultServicePort}
		emptyPorts                  = map[string]uint32{}
		defaultK8sSvcPortNoName     = k8sV1.ServicePort{Port: int32(defaultServicePort)}
		defaultK8sSvcPort           = k8sV1.ServicePort{Name: "default", Port: int32(defaultServicePort)}
		meshK8sSvcPort              = k8sV1.ServicePort{Name: "mesh", Port: int32(annotatedPort)}
		serviceMeshPorts            = []k8sV1.ServicePort{defaultK8sSvcPort, meshK8sSvcPort}
		serviceMeshPortsOnlyDefault = []k8sV1.ServicePort{defaultK8sSvcPortNoName}
		service                     = &k8sV1.Service{
			ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
			Spec:       k8sV1.ServiceSpec{Ports: serviceMeshPorts},
		}
		deployment = &k8sAppsV1.Deployment{
			Spec: k8sAppsV1.DeploymentSpec{Template: coreV1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{common.SidecarEnabledPorts: strconv.Itoa(annotatedPort)}},
			}}}
		deploymentWithMultipleMeshPorts = &k8sAppsV1.Deployment{
			Spec: k8sAppsV1.DeploymentSpec{Template: coreV1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{common.SidecarEnabledPorts: strconv.Itoa(annotatedPort) + "," + strconv.Itoa(annotatedSecondPort)}},
			}}}
	)

	testCases := []struct {
		name        string
		clusterName string
		service     *k8sV1.Service
		deployment  *k8sAppsV1.Deployment
		expected    map[string]uint32
	}{
		{
			name:       "should return a port based on annotation",
			service:    service,
			deployment: deployment,
			expected:   ports,
		},
		{
			name: "should return a http port if no port name is specified",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: []k8sV1.ServicePort{{Port: int32(80), TargetPort: intstr.FromInt(annotatedPort)}}},
			},
			deployment: deployment,
			expected:   portsDiffTargetPort,
		},
		{
			name: "should return a http port if the port name doesn't start with a protocol name",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: []k8sV1.ServicePort{{Name: "hello-grpc", Port: int32(annotatedPort)}}},
			},
			deployment: deployment,
			expected:   ports,
		},
		{
			name: "should return a grpc port based on annotation",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: []k8sV1.ServicePort{{Name: "grpc-service", Port: int32(annotatedPort)}}},
			},
			deployment: deployment,
			expected:   grpcPorts,
		},
		{
			name: "should return a grpc-web port based on annotation",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: []k8sV1.ServicePort{{Name: "grpc-web", Port: int32(annotatedPort)}}},
			},
			deployment: deployment,
			expected:   grpcWebPorts,
		},
		{
			name: "should return a http2 port based on annotation",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: []k8sV1.ServicePort{{Name: "http2", Port: int32(annotatedPort)}}},
			},
			deployment: deployment,
			expected:   http2Ports,
		},
		{
			name: "should return a default port",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: serviceMeshPortsOnlyDefault},
			},
			deployment: &k8sAppsV1.Deployment{
				Spec: k8sAppsV1.DeploymentSpec{Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}},
				}}},
			expected: portsFromDefaultSvcPort,
		},
		{
			name: "should return empty ports",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: nil},
			},
			deployment: &k8sAppsV1.Deployment{
				Spec: k8sAppsV1.DeploymentSpec{Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}},
				}}},
			expected: emptyPorts,
		},
		{
			name: "should return a http port if the port name doesn't start with a protocol name",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec: k8sV1.ServiceSpec{Ports: []k8sV1.ServicePort{{Name: "http", Port: int32(annotatedPort)},
					{Name: "grpc", Port: int32(annotatedSecondPort)}}},
			},
			deployment: deploymentWithMultipleMeshPorts,
			expected:   ports,
		},
		{
			name: "should not panic when deployment is empty",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec: k8sV1.ServiceSpec{Ports: []k8sV1.ServicePort{{Name: "http", Port: int32(annotatedPort)},
					{Name: "grpc", Port: int32(annotatedSecondPort)}}},
			},
			deployment: nil,
			expected:   ports,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			meshPorts := GetMeshPortsForDeployment(c.clusterName, c.service, c.deployment)
			if !reflect.DeepEqual(meshPorts, c.expected) {
				t.Errorf("Wanted meshPorts: %v, got: %v", c.expected, meshPorts)
			}
		})
	}
}

func TestValidateConfigmapBeforePutting(t *testing.T) {

	legalStore := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.a.mesh": common.LocalAddressPrefix + ".10.1"},
		Addresses:      []string{common.LocalAddressPrefix + ".10.1"},
	}

	illegalStore := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.a.mesh": common.LocalAddressPrefix + ".10.1"},
		Addresses:      []string{common.LocalAddressPrefix + ".10.1", "1.2.3.4"},
	}

	emptyCM := coreV1.ConfigMap{}
	emptyCM.ResourceVersion = "123"

	testCases := []struct {
		name          string
		configMap     *coreV1.ConfigMap
		expectedError error
	}{
		{
			name:          "should not throw error on legal configmap",
			configMap:     buildFakeConfigMapFromAddressStore(&legalStore, "123"),
			expectedError: nil,
		},
		{
			name:          "should not throw error on empty configmap",
			configMap:     &emptyCM,
			expectedError: nil,
		},
		{
			name:          "should throw error on no resourceversion",
			configMap:     buildFakeConfigMapFromAddressStore(&legalStore, ""),
			expectedError: errors.New("resourceversion required"),
		},
		{
			name:          "should throw error on length mismatch",
			configMap:     buildFakeConfigMapFromAddressStore(&illegalStore, "123"),
			expectedError: errors.New("address cache length mismatch"),
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			errorResult := ValidateConfigmapBeforePutting(c.configMap)
			if errorResult == nil && c.expectedError == nil {
				//we're fine
			} else if c.expectedError == nil && errorResult != nil {
				t.Errorf("Unexpected error. Err: %v", errorResult)
			} else if errorResult.Error() != c.expectedError.Error() {
				t.Errorf("Error mismatch. Expected %v but got %v", c.expectedError, errorResult)
			}
		})
	}

}

func TestGetServiceSelector(t *testing.T) {

	selector := map[string]string{"app": "test1"}

	testCases := []struct {
		name        string
		clusterName string
		service     k8sV1.Service
		expected    map[string]string
	}{
		{
			name:        "should return a selectors based on service",
			clusterName: "test-cluster",
			service: k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Selector: selector},
			},
			expected: selector,
		},
		{
			name:        "should return empty selectors",
			clusterName: "test-cluster",
			service: k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Selector: map[string]string{}},
			},
			expected: nil,
		},
		{
			name:        "should return nil",
			clusterName: "test-cluster",
			service: k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Selector: nil},
			},
			expected: nil,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			selectors := GetServiceSelector(c.clusterName, &c.service)
			if selectors == nil {
				if c.expected != nil {
					t.Errorf("Wanted selectors: %v, got: %v", c.expected, selectors)
				}
			} else if !reflect.DeepEqual(selectors.Copy(), c.expected) {
				t.Errorf("Wanted selectors: %v, got: %v", c.expected, selectors)
			}
		})
	}
}

func TestGetMeshPortsForRollout(t *testing.T) {

	annotatedPort := 8090
	defaultServicePort := uint32(8080)

	defaultK8sSvcPortNoName := k8sV1.ServicePort{Port: int32(defaultServicePort)}
	defaultK8sSvcPort := k8sV1.ServicePort{Name: "default", Port: int32(defaultServicePort)}
	meshK8sSvcPort := k8sV1.ServicePort{Name: "mesh", Port: int32(annotatedPort)}

	serviceMeshPorts := []k8sV1.ServicePort{defaultK8sSvcPort, meshK8sSvcPort}

	serviceMeshPortsOnlyDefault := []k8sV1.ServicePort{defaultK8sSvcPortNoName}

	service := &k8sV1.Service{
		ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
		Spec:       k8sV1.ServiceSpec{Ports: serviceMeshPorts},
	}
	rollout := &argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{common.SidecarEnabledPorts: strconv.Itoa(annotatedPort)}},
		}}}

	ports := map[string]uint32{"http": uint32(annotatedPort)}

	portsFromDefaultSvcPort := map[string]uint32{"http": defaultServicePort}

	emptyPorts := map[string]uint32{}

	testCases := []struct {
		name        string
		clusterName string
		service     *k8sV1.Service
		rollout     *argo.Rollout
		expected    map[string]uint32
	}{
		{
			name:     "should return a port based on annotation",
			service:  service,
			rollout:  rollout,
			expected: ports,
		},
		{
			name: "should return a default port",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: serviceMeshPortsOnlyDefault},
			},
			rollout: &argo.Rollout{
				Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}},
				}}},
			expected: portsFromDefaultSvcPort,
		},
		{
			name: "should return empty ports",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: nil},
			},
			rollout: &argo.Rollout{
				Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}},
				}}},
			expected: emptyPorts,
		},
		{
			name: "should not panic when rollout is nil",
			service: &k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: nil},
			},
			rollout:  nil,
			expected: emptyPorts,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			meshPorts := GetMeshPortsForRollout(c.clusterName, c.service, c.rollout)
			if !reflect.DeepEqual(meshPorts, c.expected) {
				t.Errorf("Wanted meshPorts: %v, got: %v", c.expected, meshPorts)
			}
		})
	}
}

func TestGetMeshPortAndLabelsFromDeploymentOrRollout(t *testing.T) {
	var (
		service = &k8sV1.Service{
			Spec: k8sV1.ServiceSpec{
				Ports: []k8sV1.ServicePort{
					{
						Name: common.Http,
						Port: 8090,
					},
				},
			},
		}
		clusterNameWithExistingDeployment                       = "cluster_with_deployment-ppd-k8s"
		clusterNameWithExistingRollout                          = "cluster_with_rollout-ppd-k8s"
		clusterNameWithoutExistingRolloutOrDeployment           = "cluster_without_deployment_rollout-ppd-k8s"
		deploymentByClusterNameForExistingClusterWithDeployment = map[string]*k8sAppsV1.Deployment{
			clusterNameWithExistingDeployment: {
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"key": "value",
					},
				},
			},
		}
		rolloutByClusterNameForExistingClusterWithRollout = map[string]*argo.Rollout{
			clusterNameWithExistingRollout: {
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"key": "value",
					},
				},
			},
		}
	)
	cases := []struct {
		name                string
		cluster             string
		serviceInstance     *k8sV1.Service
		deploymentByCluster map[string]*k8sAppsV1.Deployment
		rolloutsByCluster   map[string]*argo.Rollout
		expectedMeshPort    map[string]uint32
		expectedLabels      map[string]string
	}{
		{
			name: "Given a deployment with labels exists in a cluster, " +
				"When GetMeshPortAndLabelsFromDeploymentOrRollout is called with," +
				"this cluster, with a valid service, " +
				"Then, it should return mesh ports and labels",
			cluster:             clusterNameWithExistingDeployment,
			serviceInstance:     service,
			deploymentByCluster: deploymentByClusterNameForExistingClusterWithDeployment,
			rolloutsByCluster:   rolloutByClusterNameForExistingClusterWithRollout,
			expectedMeshPort: map[string]uint32{
				common.Http: 8090,
			},
			expectedLabels: map[string]string{
				"key": "value",
			},
		},
		{
			name: "Given a rollout with labels exists in a cluster, " +
				"When GetMeshPortAndLabelsFromDeploymentOrRollout is called with," +
				"this cluster, with a valid service, " +
				"Then, it should return mesh ports and labels",
			cluster:             clusterNameWithExistingRollout,
			serviceInstance:     service,
			deploymentByCluster: deploymentByClusterNameForExistingClusterWithDeployment,
			rolloutsByCluster:   rolloutByClusterNameForExistingClusterWithRollout,
			expectedMeshPort: map[string]uint32{
				common.Http: 8090,
			},
			expectedLabels: map[string]string{
				"key": "value",
			},
		},
		{
			name: "Given neither a deployment nor a rollout with labels exists in a cluster, " +
				"When GetMeshPortAndLabelsFromDeploymentOrRollout is called with," +
				"this cluster, with a valid service, " +
				"Then, it should return nil for mesh ports, and nil for labels",
			cluster:             clusterNameWithoutExistingRolloutOrDeployment,
			serviceInstance:     service,
			deploymentByCluster: deploymentByClusterNameForExistingClusterWithDeployment,
			rolloutsByCluster:   rolloutByClusterNameForExistingClusterWithRollout,
			expectedMeshPort:    nil,
			expectedLabels:      nil,
		},
		{
			name: "Given neither a deployment nor a rollout with labels exists in a cluster, " +
				"When GetMeshPortAndLabelsFromDeploymentOrRollout is called with," +
				"this cluster, with a valid service, but empty deployment by cluster and rollout by cluster maps " +
				"Then, it should return nil for mesh ports, and nil for labels",
			cluster:             clusterNameWithoutExistingRolloutOrDeployment,
			serviceInstance:     service,
			deploymentByCluster: nil,
			rolloutsByCluster:   nil,
			expectedMeshPort:    nil,
			expectedLabels:      nil,
		},
	}

	for _, c := range cases {
		meshPort, labels := GetMeshPortAndLabelsFromDeploymentOrRollout(
			c.cluster, c.serviceInstance, c.deploymentByCluster, c.rolloutsByCluster,
		)
		if !reflect.DeepEqual(meshPort, c.expectedMeshPort) {
			t.Errorf("expected: %v, got: %v", c.expectedMeshPort, meshPort)
		}
		if !reflect.DeepEqual(labels, c.expectedLabels) {
			t.Errorf("expected: %v, got: %v", c.expectedLabels, labels)
		}
	}
}
