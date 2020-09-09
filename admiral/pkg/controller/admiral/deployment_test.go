package admiral

import (
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	log "github.com/sirupsen/logrus"
	k8sAppsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd"
	"sort"
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
	deploymentWithIgnoreAnnotations := k8sAppsV1.Deployment{}
	deploymentWithIgnoreAnnotations.Spec.Template.Labels = map[string]string{"identity": "id"}
	deploymentWithIgnoreAnnotations.Annotations = map[string]string{"admiral.io/ignore": "true"}
	deploymentWithIgnoreAnnotations.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}
	deploymentWithNsIgnoreAnnotations := k8sAppsV1.Deployment{}
	deploymentWithNsIgnoreAnnotations.Spec.Template.Labels = map[string]string{"identity": "id"}
	deploymentWithNsIgnoreAnnotations.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}
	deploymentWithNsIgnoreAnnotations.Namespace = "test-ns"

	testCases := []struct {
		name                  string
		deployment            *k8sAppsV1.Deployment
		expectedDeployment    *k8sAppsV1.Deployment
		expectedCacheContains bool
	}{
		{
			name:                  "Expects deployment to be added to the cache when the correct label is present",
			deployment:            &deployment,
			expectedDeployment:    &deployment,
			expectedCacheContains: true,
		},
		{
			name:                  "Expects deployment to not be added to the cache when the correct label is not present",
			deployment:            &deploymentWithBadLabels,
			expectedDeployment:    nil,
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored deployment identified by label to not be added to the cache",
			deployment:            &deploymentWithIgnoreLabels,
			expectedDeployment:    nil,
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored deployment identified by deployment annotation to not be added to the cache",
			deployment:            &deploymentWithIgnoreAnnotations,
			expectedDeployment:    nil,
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored deployment identified by namespace annotation to not be added to the cache",
			deployment:            &deploymentWithNsIgnoreAnnotations,
			expectedDeployment:    nil,
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored deployment identified by label to be removed from the cache",
			deployment:            &deploymentWithIgnoreLabels,
			expectedDeployment:    &deploymentWithIgnoreLabels,
			expectedCacheContains: false,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			depController.K8sClient = fake.NewSimpleClientset()
			if c.name == "Expects ignored deployment identified by namespace annotation to not be added to the cache" {
				ns := coreV1.Namespace{}
				ns.Name = "test-ns"
				ns.Annotations = map[string]string{"admiral.io/ignore": "true"}
				depController.K8sClient.CoreV1().Namespaces().Create(&ns)
			}
			depController.Cache.cache = map[string]*DeploymentClusterEntry{}

			if c.name == "Expects ignored deployment identified by label to be removed from the cache" {
				depController.Cache.UpdateDeploymentToClusterCache("id", &deployment)
			}
			depController.Added(c.deployment)

			if c.expectedDeployment == nil {
				if len(depController.Cache.cache) != 0 || (depController.Cache.cache["id"] != nil && len(depController.Cache.cache["id"].Deployments) != 0) {
					t.Errorf("Cache should be empty if expected deployment is nil")
				}
			} else if len(depController.Cache.cache) == 0 && c.expectedCacheContains != false {
				t.Errorf("Unexpectedly empty cache. Cache was expected to have the key")
			} else if len(depController.Cache.cache["id"].Deployments) == 0 && c.expectedCacheContains != false {
				t.Errorf("Deployment controller cache has wrong size. Cached was expected to have deployment for environment %v but was not present.", common.Default)
			} else if depController.Cache.cache["id"].Deployments[common.Default] != nil && depController.Cache.cache["id"].Deployments[common.Default] != &deployment {
				t.Errorf("Incorrect deployment added to deployment controller cache. Got %v expected %v", depController.Cache.cache["id"].Deployments[common.Default], deployment)
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
	deployment.Name = "deployment"
	deployment.Spec.Template.Labels = map[string]string{"identity": "id", "istio-injected": "true"}
	deployment.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}
	deploymentWithBadLabels := k8sAppsV1.Deployment{}
	deploymentWithBadLabels.Namespace = "test-ns"
	deploymentWithBadLabels.Name = "deploymentWithBadLabels"
	deploymentWithBadLabels.Spec.Template.Labels = map[string]string{"identity": "id", "random-label": "true"}
	deploymentWithBadLabels.Spec.Template.Annotations = map[string]string{"woo": "yay"}
	deploymentWithIgnoreLabels := k8sAppsV1.Deployment{}
	deploymentWithIgnoreLabels.Namespace = "test-ns"
	deploymentWithIgnoreLabels.Name = "deploymentWithIgnoreLabels"
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
	if !cmp.Equal(resultingDeps[0], &deployment) {
		log.Info("Object Diff: " + cmp.Diff(resultingDeps[0], &deployment))
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

func TestDeploymentController_GetDeploymentByLabel(t *testing.T) {
	deployment := k8sAppsV1.Deployment{}
	deployment.Namespace = "namespace"
	deployment.Name = "fake-app-deployment-qal"
	deployment.Spec = k8sAppsV1.DeploymentSpec{
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "qal"},
			},
		},
	}
	deployment.Labels = map[string]string{"identity": "app1"}

	deployment2 := k8sAppsV1.Deployment{}
	deployment2.Namespace = "namespace"
	deployment2.Name = "fake-app-deployment-e2e"
	deployment2.Spec = k8sAppsV1.DeploymentSpec{
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "e2e"},
			},
		},
	}
	deployment2.Labels = map[string]string{"identity": "app1"}

	deployment3 := k8sAppsV1.Deployment{}
	deployment3.Namespace = "namespace"
	deployment3.Name = "fake-app-deployment-prf-1"
	deployment3.CreationTimestamp = v1.Now()
	deployment3.Spec = k8sAppsV1.DeploymentSpec{
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "prf"},
			},
		},
	}
	deployment3.Labels = map[string]string{"identity": "app1"}

	deployment4 := k8sAppsV1.Deployment{}
	deployment4.Namespace = "namespace"
	deployment4.Name = "fake-app-deployment-prf-2"
	deployment4.CreationTimestamp = v1.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
	deployment4.Spec = k8sAppsV1.DeploymentSpec{
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app2", "env": "prf"},
			},
		},
	}
	deployment4.Labels = map[string]string{"identity": "app2"}

	oneDeploymentClient := fake.NewSimpleClientset(&deployment)

	allDeploymentsClient := fake.NewSimpleClientset(&deployment, &deployment2, &deployment3, &deployment4)

	noDeploymentsClient := fake.NewSimpleClientset()

	deploymentController := &DeploymentController{}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name                string
		expectedDeployments []k8sAppsV1.Deployment
		fakeClient          *fake.Clientset
		labelValue          string
	}{
		{
			name:                "Get one",
			expectedDeployments: []k8sAppsV1.Deployment{deployment},
			fakeClient:          oneDeploymentClient,
			labelValue:          "app1",
		},
		{
			name:                "Get one from long list",
			expectedDeployments: []k8sAppsV1.Deployment{deployment4},
			fakeClient:          allDeploymentsClient,
			labelValue:          "app2",
		},
		{
			name:                "Get many from long list",
			expectedDeployments: []k8sAppsV1.Deployment{deployment, deployment3, deployment2},
			fakeClient:          allDeploymentsClient,
			labelValue:          "app1",
		},
		{
			name:                "Get none from long list",
			expectedDeployments: []k8sAppsV1.Deployment{},
			fakeClient:          allDeploymentsClient,
			labelValue:          "app3",
		},
		{
			name:                "Get none from empty list",
			expectedDeployments: []k8sAppsV1.Deployment{},
			fakeClient:          noDeploymentsClient,
			labelValue:          "app1",
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			deploymentController.K8sClient = c.fakeClient
			returnedDeployments := deploymentController.GetDeploymentByLabel(c.labelValue, "namespace")

			sort.Slice(returnedDeployments, func(i, j int) bool {
				return returnedDeployments[i].Name > returnedDeployments[j].Name
			})

			sort.Slice(c.expectedDeployments, func(i, j int) bool {
				return c.expectedDeployments[i].Name > c.expectedDeployments[j].Name
			})

			if len(returnedDeployments) != len(c.expectedDeployments) {
				t.Fatalf("Returned the wrong number of deploymenrs. Found %v but expected %v", len(returnedDeployments), len(c.expectedDeployments))
			}

			if !cmp.Equal(returnedDeployments, c.expectedDeployments) {
				t.Fatalf("Deployment mismatch. Diff: %v", cmp.Diff(returnedDeployments, c.expectedDeployments))
			}

		})
	}
}
