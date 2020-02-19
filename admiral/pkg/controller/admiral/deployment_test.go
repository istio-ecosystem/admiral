package admiral

import (
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/sirupsen/logrus"
	k8sAppsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd"
	"sync"
	"testing"
	"time"
)

func TestDeploymentController_Added(t *testing.T) {
	//Deployments with the correct label are added to the cache
	mdh := test.MockDeploymentHandler{}
	cache := deploymentCache{
		cache: map[string]*DeploymentClusterEntry{},
		mutex: &sync.Mutex{},
	}
	labelset := common.LabelSet{
		DeploymentAnnotation: "sidecar.istio.io/inject",
		AdmiralIgnoreLabel:   "admiral-ignore",
	}
	depController := DeploymentController{
		DeploymentHandler: &mdh,
		Cache:             &cache,
		labelSet:          &labelset,
	}
	deployment := k8sAppsV1.Deployment{}
	deployment.Spec.Template.Labels = map[string]string{"identity": "id", "istio-injected": "true"}
	deployment.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}
	deploymentWithBadLabels := k8sAppsV1.Deployment{}
	deploymentWithBadLabels.Spec.Template.Labels = map[string]string{"identity": "id", "random-label": "true"}
	deploymentWithIgnoreLabels := k8sAppsV1.Deployment{}
	deploymentWithIgnoreLabels.Spec.Template.Labels = map[string]string{"identity": "id", "istio-injected": "true", "admiral-ignore": "true"}
	deploymentWithIgnoreLabels.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}

	testCases := []struct {
		name               string
		deployment         *k8sAppsV1.Deployment
		expectedDeployment *k8sAppsV1.Deployment
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
			}else if len(depController.Cache.cache["id"].Deployments) < 1 && len(depController.Cache.cache["id"].Deployments[common.Default]) != c.expectedCacheSize {
				t.Errorf("Deployment controller cache the wrong size. Got %v, expected %v", len(depController.Cache.cache["id"].Deployments[""]), c.expectedCacheSize)
			} else if depController.Cache.cache["id"].Deployments[common.Default][0] != &deployment {
				t.Errorf("Incorrect deployment added to deployment controller cache. Got %v expected %v", depController.Cache.cache["id"].Deployments[""][0], deployment)
			}

		})
	}

}



func TestDeploymentController_GetDeployments(t *testing.T) {

	depController := DeploymentController{
		labelSet: &common.LabelSet{
			DeploymentAnnotation:                "sidecar.istio.io/inject",
			NamespaceSidecarInjectionLabel:      "istio-injection",
			NamespaceSidecarInjectionLabelValue: "enabled",
			AdmiralIgnoreLabel:                  "admiral-ignore",
		},
	}


	client := fake.NewSimpleClientset()

	ns := coreV1.Namespace{}
	ns.Labels = map[string]string{"istio-injection": "enabled"}
	ns.Name = "test-ns"

	_, err := client.CoreV1().Namespaces().Create(&ns)
	if err != nil {
		t.Errorf("%v", err)
	}

	deployment := k8sAppsV1.Deployment{}
	deployment.Namespace = "test-ns"
	deployment.Name="deployment"
	deployment.Spec.Template.Labels = map[string]string{"identity": "id", "istio-injected": "true"}
	deployment.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}
	deploymentWithBadLabels := k8sAppsV1.Deployment{}
	deploymentWithBadLabels.Namespace = "test-ns"
	deploymentWithBadLabels.Name="deploymentWithBadLabels"
	deploymentWithBadLabels.Spec.Template.Labels = map[string]string{"identity": "id", "random-label": "true"}
	deploymentWithBadLabels.Spec.Template.Annotations = map[string]string{"woo": "yay"}
	deploymentWithIgnoreLabels := k8sAppsV1.Deployment{}
	deploymentWithIgnoreLabels.Namespace = "test-ns"
	deploymentWithIgnoreLabels.Name="deploymentWithIgnoreLabels"
	deploymentWithIgnoreLabels.Spec.Template.Labels = map[string]string{"identity": "id", "istio-injected": "true", "admiral-ignore": "true"}
	deploymentWithIgnoreLabels.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}
	_, err = client.AppsV1().Deployments("test-ns").Create(&deployment)
	_, err = client.AppsV1().Deployments("test-ns").Create(&deploymentWithBadLabels)
	_, err = client.AppsV1().Deployments("test-ns").Create(&deploymentWithIgnoreLabels)

	if err != nil {
		t.Errorf("%v", err)
	}

	depController.K8sClient = client
	resultingDeps, _ := depController.GetDeployments()

	if len(resultingDeps) != 1 {
		t.Errorf("Get Deployments returned too many values. Expected 1, got %v", len(resultingDeps))
	}
	if  !cmp.Equal(resultingDeps[0], &deployment) {
		logrus.Info("Object Diff: " + cmp.Diff(resultingDeps[0], &deployment))
		t.Errorf("Get Deployments returned the incorrect value. Got %v, expected %v", resultingDeps[0], deployment)
	}

}

func TestNewDeploymentController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	depHandler := test.MockDeploymentHandler{}

	depCon, err := NewDeploymentController(stop, &depHandler, config, time.Duration(1000))

	if depCon == nil {
		t.Errorf("Deployment controller should not be nil")
	}
}
