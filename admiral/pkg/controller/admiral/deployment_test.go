package admiral

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/api/apps/v1"
	"sync"
	"testing"
)

func TestDeploymentController_Added(t *testing.T) {
	//Deployments with the correct label are added to the cache
	mdh := test.MockDeploymentHandler{}
	cache := deploymentCache{
		cache: map[string]*DeploymentClusterEntry{},
		mutex: &sync.Mutex{},
	}
	labelset := common.LabelSet{
		DeploymentLabel: "istio-injected",
		AdmiralIgnoreLabel: "admiral-ignore",
	}
	depController := DeploymentController{
		DeploymentHandler: &mdh,
		Cache:             &cache,
		labelSet:          labelset,
	}
	deployment := v1.Deployment{}
	deployment.Spec.Template.Labels = map[string]string{"identity": "id", "istio-injected": "true"}
	deploymentWithBadLabels := v1.Deployment{}
	deploymentWithBadLabels.Spec.Template.Labels = map[string]string{"identity": "id", "random-label": "true"}
	deploymentWithIgnoreLabels := v1.Deployment{}
	deploymentWithIgnoreLabels.Spec.Template.Labels = map[string]string{"identity": "id", "istio-injected": "true", "admiral-ignore": "true"}

	testCases := []struct {
		name               string
		deployment         *v1.Deployment
		expectedDeployment *v1.Deployment
		expectedCacheSize  int
	}{
		{
			name:               "Expects deployment to be added to the cache when the correct label is present",
			deployment:         &deployment,
			expectedDeployment: &deployment,
			expectedCacheSize:  1,
		},
		{
			name:               "Expects deployment to not be added to the cache when the correct label is not present",
			deployment:         &deploymentWithBadLabels,
			expectedDeployment: nil,
			expectedCacheSize:  0,
		},
		{
			name:               "Expects ignored deployment to not be added to the cache",
			deployment:         &deploymentWithIgnoreLabels,
			expectedDeployment: nil,
			expectedCacheSize:  0,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			depController.Cache.cache = map[string]*DeploymentClusterEntry{}
			depController.Added(c.deployment)
			if c.expectedDeployment == nil {
				if len(depController.Cache.cache) != 0 {
					t.Errorf("Cache should be empty if expected deployment is nil")
				}
			} else if len(depController.Cache.cache)==0 && c.expectedCacheSize != 0 {
				t.Errorf("Unexpectedly empty cache. Length should have been %v but was 0", c.expectedCacheSize)
			}else if len(depController.Cache.cache["id"].Deployments) < 1 && len(depController.Cache.cache["id"].Deployments[""]) != c.expectedCacheSize {
				t.Errorf("Deployment controller cache the wrong size. Got %v, expected %v", len(depController.Cache.cache["id"].Deployments[""]), c.expectedCacheSize)
			} else if depController.Cache.cache["id"].Deployments[""][0] != &deployment {
				t.Errorf("Incorrect deployment added to deployment controller cache. Got %v expected %v", depController.Cache.cache["id"].Deployments[""][0], deployment)
			}

		})
	}

}
