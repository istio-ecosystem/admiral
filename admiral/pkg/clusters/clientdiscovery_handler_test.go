package clusters

import (
	"context"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestClientDiscoveryHandler_Added(t *testing.T) {
	//Struct of test case info. Name is required.
	const (
		namespace           = "namespace"
		namespace_no_deps   = "namespace-no-deps"
		namespaceForRollout = "rollout-namespace"
		cluster1            = "cluster1"
		cluster2            = "cluster2"
		cluster3            = "cluster3"
		identity1           = "identity1"
		identity2           = "identity2"
		identity3           = "identity3"
		identity4           = "identity4"
	)
	var (
		stop   = make(chan struct{})
		config = rest.Config{
			Host: "localhost",
		}
		matchLabel = map[string]string{
			"app": "test",
		}
		labelSelector = metaV1.LabelSelector{
			MatchLabels: matchLabel,
		}
		dep = v1alpha1.Dependency{
			Spec: model.Dependency{
				Source:       identity1,
				Destinations: []string{identity2},
			},
		}
		rollout = argo.Rollout{
			Spec: argo.RolloutSpec{
				Selector: &labelSelector,
				Strategy: argo.RolloutStrategy{
					BlueGreen: &argo.BlueGreenStrategy{},
				},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{
						"sidecar.istio.io/inject": "true",
						"identity":                identity1,
					}, Labels: map[string]string{
						"env": "stage",
					}},
				},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Namespace: namespaceForRollout,
			},
		}
		objRolloutNamespace = &common.K8sObject{
			Namespace:   namespaceForRollout,
			Name:        "obj-rollout-namespace",
			Annotations: map[string]string{"identity": identity1},
		}
		objValid = &common.K8sObject{
			Namespace:   namespace,
			Name:        "obj",
			Annotations: map[string]string{"identity": identity1},
		}
		objValidWithNoDeps = &common.K8sObject{
			Namespace:   namespace_no_deps,
			Name:        "obj-no-deps",
			Annotations: map[string]string{"identity": identity4},
		}
	)

	setupForHandlerTests(true)

	depController, err := admiral.NewDependencyController(stop, &test.MockDependencyHandler{}, loader.FakeKubeconfigPath, "", time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("failed to initialize dependency controller, err: %v", err)
	}
	r, err := admiral.NewRolloutsController(stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("failed to initialize rollout controller, err: %v", err)
	}
	d, err := admiral.NewDeploymentController(stop, &test.MockDeploymentHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("failed to initialize rollout controller, err: %v", err)
	}

	rcCluster1 := &RemoteController{
		RolloutController:    r,
		DeploymentController: d,
	}

	rcCluster2 := &RemoteController{
		RolloutController:    r,
		DeploymentController: d,
	}

	ctx := context.Background()

	remoteRegistry := NewRemoteRegistry(context.TODO(), common.AdmiralParams{})
	remoteRegistry.DependencyController = depController
	remoteRegistry.remoteControllers[cluster1] = rcCluster1
	remoteRegistry.remoteControllers[cluster2] = rcCluster2

	remoteRegistry.AdmiralCache = &AdmiralCache{
		IdentityClusterCache:          common.NewMapOfMaps(),
		IdentityClusterNamespaceCache: common.NewMapOfMapOfMaps(),
		PartitionIdentityCache:        common.NewMap(),
	}

	err = r.Added(ctx, &rollout)
	assert.NoError(t, err)
	rolloutFromCache := r.Cache.Get(identity1, "stage")
	assert.NotNil(t, rolloutFromCache)
	err = depController.Added(ctx, &dep)
	assert.NoError(t, err)
	depFromCache := depController.Cache.Get(identity1)
	assert.NotNil(t, depFromCache)

	testCases := []struct {
		name          string
		obj           *common.K8sObject
		rr            *RemoteRegistry
		clusterName   string
		expectedError error
	}{
		{
			name:          "No global identifier on obj results in no errors",
			obj:           &common.K8sObject{Annotations: map[string]string{}, Labels: map[string]string{}},
			rr:            remoteRegistry,
			clusterName:   cluster1,
			expectedError: nil,
		}, {
			name: "Rollout with same identifier present in the namespace should " +
				"return without processing and no errors",
			obj:           objRolloutNamespace,
			rr:            remoteRegistry,
			clusterName:   cluster1,
			expectedError: nil,
		}, {
			name:          "Valid client with no dependency record returns without processing",
			obj:           objValidWithNoDeps,
			rr:            remoteRegistry,
			clusterName:   cluster1,
			expectedError: nil,
		}, {
			name:          "Valid client with valid dependency record results in writing all dependencies endpoints to this cluster",
			obj:           objValid,
			rr:            remoteRegistry,
			clusterName:   cluster1,
			expectedError: nil,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := HandleEventForClientDiscovery(ctx, admiral.Add, c.obj, remoteRegistry, c.clusterName)
			if c.expectedError == nil && result != nil {
				t.Fatalf("Expected error %v got %v", c.expectedError, result)
			}
		})
	}
}
