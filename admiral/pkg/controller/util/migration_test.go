package util

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	networking "istio.io/api/networking/v1alpha3"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateEndpointsForDeployToRolloutMigration(t *testing.T) {
	var (
		foobarMetadataName        = "foobar"
		foobarMetadataNamespace   = "foobar-ns"
		identity                  = "identity"
		meshPorts                 = make(map[string]map[string]uint32)
		serviceInstanceDeployNil  = make(map[string]*coreV1.Service)
		serviceInstanceRolloutNil = make(map[string]*coreV1.Service)
		serviceInstance           = make(map[string]*coreV1.Service)
		clusterName               = "test-k8s"
	)

	localAddress := common.LocalAddressPrefix + ".10.1"

	seDeployment := &networking.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*networking.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        networking.ServiceEntry_MESH_INTERNAL,
		Resolution:      networking.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*networking.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Deployment}},
		},
	}

	seRollout := &networking.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*networking.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        networking.ServiceEntry_MESH_INTERNAL,
		Resolution:      networking.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*networking.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Rollout}},
		},
	}

	seDeployAndRolloutSingleCluster := &networking.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*networking.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        networking.ServiceEntry_MESH_INTERNAL,
		Resolution:      networking.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*networking.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Rollout}},
		},
	}

	seDeployAndRolloutMulticluster := &networking.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{localAddress},
		Ports: []*networking.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        networking.ServiceEntry_MESH_INTERNAL,
		Resolution:      networking.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*networking.WorkloadEntry{
			{Address: "east.elb.aws.com", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2", Labels: map[string]string{"type": common.Deployment}},
			{Address: "west.elb.aws.com", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2", Labels: map[string]string{"type": common.Rollout}},
		},
	}

	seDeployAndRolloutMulticluster1 := seDeployAndRolloutMulticluster.DeepCopy()
	seDeployAndRolloutMulticluster2 := seDeployAndRolloutMulticluster.DeepCopy()

	service := &coreV1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      foobarMetadataName,
			Namespace: foobarMetadataNamespace,
		},
		Spec: coreV1.ServiceSpec{
			Selector: map[string]string{"app": identity},
			Ports: []coreV1.ServicePort{
				{
					Name: "http",
					Port: 8090,
				},
			},
		},
	}

	meshPorts[common.Deployment] = map[string]uint32{"http": uint32(8090)}
	meshPorts[common.Rollout] = map[string]uint32{"http": uint32(8090)}

	serviceInstanceDeployNil[common.Deployment] = nil
	serviceInstanceRolloutNil[common.Rollout] = nil
	serviceInstance[common.Deployment] = service
	serviceInstance[common.Rollout] = service

	clusterDeployRolloutPresent := make(map[string]map[string]bool)
	clusterDeployRolloutPresent[clusterName] = make(map[string]bool)
	clusterDeployRolloutPresent[clusterName][common.Deployment] = true
	clusterDeployRolloutPresent[clusterName][common.Rollout] = true

	testCases := []struct {
		name                        string
		serviceInstance             map[string]*coreV1.Service
		serviceEntry                *networking.ServiceEntry
		clusterAppDeleteMap         map[string]string
		clusterIngress              string
		clusterDeployRolloutPresent map[string]map[string]bool
		expectedSeEndpoints         []*networking.WorkloadEntry
		expectedErr                 error
	}{
		{
			name: "Given service resource for the deployment type is nil," +
				"Then there is an error returned",
			serviceInstance:             serviceInstanceDeployNil,
			serviceEntry:                seDeployment,
			clusterAppDeleteMap:         nil,
			clusterIngress:              "dummy.admiral.global",
			clusterDeployRolloutPresent: nil,
			expectedSeEndpoints:         nil,
			expectedErr:                 fmt.Errorf("serviceInstance for Deployment/Rollout is nil as the service cache has not updated yet"),
		},
		{
			name: "Given service resource for the rollout type is nil," +
				"Then there is an error returned",
			serviceInstance:             serviceInstanceRolloutNil,
			serviceEntry:                seRollout,
			clusterAppDeleteMap:         nil,
			clusterIngress:              "dummy.admiral.global",
			clusterDeployRolloutPresent: nil,
			expectedSeEndpoints:         nil,
			expectedErr:                 fmt.Errorf("serviceInstance for Deployment/Rollout is nil as the service cache has not updated yet"),
		},
		{
			name: "Given all valid parameters," +
				"And there is a deployment and rollout in a single cluster," +
				"Then there is no error returned and 2 endpoints for deployment and rollout",
			serviceInstance:             serviceInstance,
			serviceEntry:                seDeployAndRolloutSingleCluster,
			clusterIngress:              "dummy.admiral.global",
			clusterAppDeleteMap:         nil,
			clusterDeployRolloutPresent: clusterDeployRolloutPresent,
			expectedSeEndpoints: []*networking.WorkloadEntry{
				{
					Address:  "foobar.foobar-ns.svc.cluster.local",
					Locality: "us-west-2",
					Ports:    meshPorts[common.Deployment],
					Labels:   map[string]string{"type": common.Deployment},
				},
				{
					Address:  "foobar.foobar-ns.svc.cluster.local",
					Locality: "us-west-2",
					Ports:    meshPorts[common.Rollout],
					Labels:   map[string]string{"type": common.Rollout},
				},
			},
			expectedErr: nil,
		},
		{
			name: "Given all valid parameters," +
				"And there is a deployment and rollout in a multi cluster," +
				"Then there is no error returned and 3 endpoints for deployment, rollout and LB",
			serviceInstance:             serviceInstance,
			serviceEntry:                seDeployAndRolloutMulticluster,
			clusterIngress:              "east.elb.aws.com",
			clusterAppDeleteMap:         nil,
			clusterDeployRolloutPresent: clusterDeployRolloutPresent,
			expectedSeEndpoints: []*networking.WorkloadEntry{
				{
					Address:  "foobar.foobar-ns.svc.cluster.local",
					Locality: "us-east-2",
					Ports:    meshPorts[common.Deployment],
					Labels:   map[string]string{"type": common.Deployment},
				},
				{
					Address:  "foobar.foobar-ns.svc.cluster.local",
					Locality: "us-east-2",
					Ports:    meshPorts[common.Rollout],
					Labels:   map[string]string{"type": common.Rollout},
				},
				{
					Address:  "west.elb.aws.com",
					Locality: "us-west-2",
					Ports:    map[string]uint32{"http": 0},
				},
			},
			expectedErr: nil,
		},
		{
			name: "Given all valid parameters," +
				"And there is a deployment and rollout in a multi cluster," +
				"And there is a delete for a deployment in one of the cluster," +
				"When we are computing the SE for the source cluster," +
				"Then there is no error returned and 2 endpoints for rollout and LB in that cluster",
			serviceInstance:             serviceInstance,
			serviceEntry:                seDeployAndRolloutMulticluster2,
			clusterIngress:              "east.elb.aws.com",
			clusterAppDeleteMap:         map[string]string{"test-k8s": common.Deployment},
			clusterDeployRolloutPresent: clusterDeployRolloutPresent,
			expectedSeEndpoints: []*networking.WorkloadEntry{
				{
					Address:  "foobar.foobar-ns.svc.cluster.local",
					Locality: "us-east-2",
					Ports:    meshPorts[common.Rollout],
					Labels:   map[string]string{"type": common.Rollout},
				},
				{
					Address:  "west.elb.aws.com",
					Locality: "us-west-2",
					Ports:    map[string]uint32{"http": 0},
				},
			},
			expectedErr: nil,
		},
		{
			name: "Given all valid parameters," +
				"And there is a deployment and rollout in a multi cluster," +
				"And there is a delete for a deployment in one of the cluster," +
				"When we are computing the SE for the other cluster," +
				"Then there is no error returned and still 3 endpoints for deployment, rollout and LB",
			serviceInstance:             serviceInstance,
			serviceEntry:                seDeployAndRolloutMulticluster1,
			clusterIngress:              "east.elb.aws.com",
			clusterAppDeleteMap:         nil,
			clusterDeployRolloutPresent: clusterDeployRolloutPresent,
			expectedSeEndpoints: []*networking.WorkloadEntry{
				{
					Address:  "foobar.foobar-ns.svc.cluster.local",
					Locality: "us-east-2",
					Ports:    meshPorts[common.Deployment],
					Labels:   map[string]string{"type": common.Deployment},
				},
				{
					Address:  "foobar.foobar-ns.svc.cluster.local",
					Locality: "us-east-2",
					Ports:    meshPorts[common.Rollout],
					Labels:   map[string]string{"type": common.Rollout},
				},
				{
					Address:  "west.elb.aws.com",
					Locality: "us-west-2",
					Ports:    map[string]uint32{"http": 0},
				},
			},
			expectedErr: nil,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := UpdateEndpointsForDeployToRolloutMigration(c.serviceInstance, c.serviceEntry, meshPorts, c.clusterIngress, c.clusterAppDeleteMap, clusterName, c.clusterDeployRolloutPresent)
			assert.Equal(t, c.expectedErr, err)
			if err == nil {
				if !reflect.DeepEqual(c.expectedSeEndpoints, c.serviceEntry.Endpoints) {
					t.Errorf("Expected endpoints: %v, got: %v", c.expectedSeEndpoints, c.serviceEntry.Endpoints)
				}
			}
		})
	}
}
