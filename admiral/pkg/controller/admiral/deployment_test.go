package admiral

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	k8sAppsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd"
)

func TestDeploymentController_Added(t *testing.T) {
	common.ResetSync()
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(admiralParams)
	ctx := context.Background()
	ctx = context.WithValue(ctx, "clusterId", "test-cluster-k8s")
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
	deployment.Spec.Template.Labels = map[string]string{"identity": "deployment", "istio-injected": "true"}
	deployment.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}
	deployment.Namespace = "deployment-ns"
	deploymentWithBadLabels := k8sAppsV1.Deployment{}
	deploymentWithBadLabels.Spec.Template.Labels = map[string]string{"identity": "deploymentWithBadLabels", "random-label": "true"}
	deploymentWithBadLabels.Spec.Template.Annotations = map[string]string{"admiral.io/env": "dev"}
	deploymentWithBadLabels.Namespace = "deploymentWithBadLabels-ns"
	deploymentWithIgnoreLabels := k8sAppsV1.Deployment{}
	deploymentWithIgnoreLabels.Spec.Template.Labels = map[string]string{"identity": "deploymentWithIgnoreLabels", "istio-injected": "true", "admiral-ignore": "true"}
	deploymentWithIgnoreLabels.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}
	deploymentWithIgnoreLabels.Namespace = "deploymentWithIgnoreLabels-ns"
	deploymentWithIgnoreAnnotations := k8sAppsV1.Deployment{}
	deploymentWithIgnoreAnnotations.Spec.Template.Labels = map[string]string{"identity": "deploymentWithIgnoreAnnotations"}
	deploymentWithIgnoreAnnotations.Annotations = map[string]string{"admiral.io/ignore": "true"}
	deploymentWithIgnoreAnnotations.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}
	deploymentWithIgnoreAnnotations.Namespace = "deploymentWithIgnoreAnnotations-ns"
	deploymentWithNsIgnoreAnnotations := k8sAppsV1.Deployment{}
	deploymentWithNsIgnoreAnnotations.Spec.Template.Labels = map[string]string{"identity": "deploymentWithNsIgnoreAnnotations"}
	deploymentWithNsIgnoreAnnotations.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}
	deploymentWithNsIgnoreAnnotations.Namespace = "test-ns"

	testCases := []struct {
		name                  string
		deployment            *k8sAppsV1.Deployment
		expectedDeployment    *k8sAppsV1.Deployment
		id                    string
		expectedCacheContains bool
	}{
		{
			name:                  "Expects deployment to be added to the cache when the correct label is present",
			deployment:            &deployment,
			expectedDeployment:    &deployment,
			id:                    "deployment",
			expectedCacheContains: true,
		},
		{
			name:                  "Expects deployment to not be added to the cache when the correct label is not present",
			deployment:            &deploymentWithBadLabels,
			expectedDeployment:    nil,
			id:                    "deploymentWithBadLabels",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored deployment identified by label to not be added to the cache",
			deployment:            &deploymentWithIgnoreLabels,
			expectedDeployment:    nil,
			id:                    "deploymentWithIgnoreLabels",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored deployment identified by deployment annotation to not be added to the cache",
			deployment:            &deploymentWithIgnoreAnnotations,
			expectedDeployment:    nil,
			id:                    "deploymentWithIgnoreAnnotations",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored deployment identified by namespace annotation to not be added to the cache",
			deployment:            &deploymentWithNsIgnoreAnnotations,
			expectedDeployment:    nil,
			id:                    "deploymentWithNsIgnoreAnnotations",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored deployment identified by label to be removed from the cache",
			deployment:            &deployment,
			expectedDeployment:    nil,
			id:                    "deployment",
			expectedCacheContains: false,
		},
	}
	depController.K8sClient = fake.NewSimpleClientset()
	ns := coreV1.Namespace{}
	ns.Name = "test-ns"
	ns.Annotations = map[string]string{"admiral.io/ignore": "true"}
	depController.K8sClient.CoreV1().Namespaces().Create(ctx, &ns, metav1.CreateOptions{})
	depController.Cache.cache = map[string]*DeploymentClusterEntry{}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "Expects ignored deployment identified by label to be removed from the cache" {
				deployment.Spec.Template.Labels["admiral-ignore"] = "true"
			}
			depController.Added(ctx, c.deployment)
			deploymentClusterEntry := depController.Cache.cache[c.id]
			var deploymentsMap map[string]*DeploymentItem = nil
			if deploymentClusterEntry != nil {
				deploymentsMap = deploymentClusterEntry.Deployments
			}
			var deploymentObj *k8sAppsV1.Deployment = nil
			if deploymentsMap != nil && len(deploymentsMap) > 0 {
				deploymentObj = deploymentsMap["dev"].Deployment
			}
			if !reflect.DeepEqual(c.expectedDeployment, deploymentObj) {
				t.Errorf("Expected rollout %+v but got %+v", c.expectedDeployment, deploymentObj)
			}
		})
	}
}

func TestDeploymentController_Deleted(t *testing.T) {
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

	testCases := []struct {
		name               string
		deployment         *k8sAppsV1.Deployment
		expectedDeployment *k8sAppsV1.Deployment
	}{
		{
			name:               "Expects deployment to be deleted from the cache when the correct label is present",
			deployment:         &deployment,
			expectedDeployment: nil,
		},
		{
			name:               "Expects no error thrown if calling delete on an deployment not exist in cache",
			deployment:         &deployment,
			expectedDeployment: nil,
		},
	}

	ctx := context.Background()

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			depController.K8sClient = fake.NewSimpleClientset()
			depController.Cache.cache = map[string]*DeploymentClusterEntry{}
			if c.name == "Expects deployment to be deleted from the cache when the correct label is present" {
				deployItem := &DeploymentItem{
					Deployment: c.deployment,
				}
				depController.Cache.cache["id"] = &DeploymentClusterEntry{
					Identity: "id",
					Deployments: map[string]*DeploymentItem{
						"default": deployItem,
					},
				}
			}
			depController.Deleted(ctx, c.deployment)

			if c.expectedDeployment == nil {
				if len(depController.Cache.cache) > 0 && len(depController.Cache.cache["id"].Deployments) != 0 {
					t.Errorf("Cache should remain the key with empty value if expected deployment is nil")
				}
			}
		})
	}
}

func TestNewDeploymentController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	depHandler := test.MockDeploymentHandler{}

	depCon, _ := NewDeploymentController(stop, &depHandler, config, time.Duration(1000), loader.GetFakeClientLoader())

	if depCon == nil {
		t.Errorf("Deployment controller should not be nil")
	}
}

func TestDeploymentController_GetDeploymentBySelectorInNamespace(t *testing.T) {
	deployment := k8sAppsV1.Deployment{}
	deployment.Namespace = "namespace"
	deployment.Name = "fake-app-deployment-qal"
	deployment.Spec = k8sAppsV1.DeploymentSpec{
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"}},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "qal"},
			},
		},
	}

	deployment2 := k8sAppsV1.Deployment{}
	deployment2.Namespace = "namespace"
	deployment2.Name = "fake-app-deployment-e2e"
	deployment2.Spec = k8sAppsV1.DeploymentSpec{
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"}},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "e2e"},
			},
		},
	}

	deployment3 := k8sAppsV1.Deployment{}
	deployment3.Namespace = "namespace"
	deployment3.Name = "fake-app-deployment-prf-1"
	deployment3.CreationTimestamp = v1.Now()
	deployment3.Spec = k8sAppsV1.DeploymentSpec{
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"}},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "prf"},
			},
		},
	}

	deployment4 := k8sAppsV1.Deployment{}
	deployment4.Namespace = "namespace"
	deployment4.Name = "fake-app-deployment-prf-2"
	deployment4.CreationTimestamp = v1.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
	deployment4.Spec = k8sAppsV1.DeploymentSpec{
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app2"}},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app2", "env": "prf"},
			},
		},
	}

	oneDeploymentClient := fake.NewSimpleClientset(&deployment)

	allDeploymentsClient := fake.NewSimpleClientset(&deployment, &deployment2, &deployment3, &deployment4)

	noDeploymentsClient := fake.NewSimpleClientset()

	deploymentController := &DeploymentController{}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name                string
		expectedDeployments []k8sAppsV1.Deployment
		fakeClient          *fake.Clientset
		selector            map[string]string
	}{
		{
			name:                "Get one",
			expectedDeployments: []k8sAppsV1.Deployment{deployment},
			fakeClient:          oneDeploymentClient,
			selector:            map[string]string{"identity": "app1"},
		},
		{
			name:                "Get one from long list",
			expectedDeployments: []k8sAppsV1.Deployment{deployment4},
			fakeClient:          allDeploymentsClient,
			selector:            map[string]string{"identity": "app2"},
		},
		{
			name:                "Get many from long list",
			expectedDeployments: []k8sAppsV1.Deployment{deployment, deployment3, deployment2},
			fakeClient:          allDeploymentsClient,
			selector:            map[string]string{"identity": "app1"},
		},
		{
			name:                "Get none from long list",
			expectedDeployments: []k8sAppsV1.Deployment{},
			fakeClient:          allDeploymentsClient,
			selector:            map[string]string{"identity": "app3"},
		},
		{
			name:                "Get none from empty list",
			expectedDeployments: []k8sAppsV1.Deployment{},
			fakeClient:          noDeploymentsClient,
			selector:            map[string]string{"identity": "app1"},
		},
	}

	ctx := context.Background()
	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			deploymentController.K8sClient = c.fakeClient
			returnedDeployments := deploymentController.GetDeploymentBySelectorInNamespace(ctx, c.selector, "namespace")

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

func TestDeleteFromDeploymentClusterCache(t *testing.T) {
	var (
		identity = "app1"
		env      = "prd"

		deploymentWithoutEnvAnnotation1 = k8sAppsV1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "abc-service",
				Namespace: "namespace-" + env,
			},
			Spec: k8sAppsV1.DeploymentSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": identity}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": identity},
					},
				},
			},
		}
		deploymentWithoutEnvAnnotation2 = k8sAppsV1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "debug",
				Namespace: "namespace-" + env,
			},
			Spec: k8sAppsV1.DeploymentSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": identity}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": identity},
					},
				},
			},
		}
		deploymentWitEnvAnnotation1 = k8sAppsV1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "abc-service",
				Namespace: "namespace-" + env,
			},
			Spec: k8sAppsV1.DeploymentSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": identity}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": identity, "env": "prd"},
					},
				},
			},
		}
	)

	testCases := []struct {
		name                        string
		deploymentInCache           *k8sAppsV1.Deployment
		deploymentToDelete          *k8sAppsV1.Deployment
		expectedDeploymentCacheSize int
	}{
		{
			name: "Given deployment cache has a valid deployment in its cache, " +
				"And the deployment does not have an env annotation" +
				"And a debug deployment exists, without the env annotation" +
				"And the env key is derived from namespace " +
				"When DeleteFromDeploymentClusterCache is called for a debug deployment" +
				"Then, the valid deployment should not be deleted from the cache",
			deploymentInCache:           &deploymentWithoutEnvAnnotation1,
			deploymentToDelete:          &deploymentWithoutEnvAnnotation2,
			expectedDeploymentCacheSize: 1,
		},
		{
			name: "Given deployment cache has a valid deployment in its cache, " +
				"And the deployment does not have an env annotation" +
				"And the env key is derived from namespace " +
				"When DeleteFromDeploymentClusterCache is called for the same deployment" +
				"Then, the deployment should be deleted from the cache",
			deploymentInCache:           &deploymentWithoutEnvAnnotation1,
			deploymentToDelete:          &deploymentWithoutEnvAnnotation1,
			expectedDeploymentCacheSize: 0,
		},
		{
			name: "Given deployment cache has a valid deployment in its cache, " +
				"And the deployment as a valid env annotation" +
				"And a debug deployment exists, without an env annotation" +
				"When DeleteFromDeploymentClusterCache is called for a debug deployment" +
				"Then, the valid deployment should not be deleted from the cache",
			deploymentInCache:           &deploymentWitEnvAnnotation1,
			deploymentToDelete:          &deploymentWithoutEnvAnnotation2,
			expectedDeploymentCacheSize: 1,
		},
		{
			name: "Given deployment cache has a valid deployment in its cache, " +
				"And the deployment as a valid env annotation" +
				"And a debug deployment exists, without an env annotation" +
				"When DeleteFromDeploymentClusterCache is called for the same deployment" +
				"Then, the deployment should be deleted from the cache",
			deploymentInCache:           &deploymentWitEnvAnnotation1,
			deploymentToDelete:          &deploymentWitEnvAnnotation1,
			expectedDeploymentCacheSize: 0,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			deploymentCache := deploymentCache{
				cache: make(map[string]*DeploymentClusterEntry),
				mutex: &sync.Mutex{},
			}
			deploymentCache.UpdateDeploymentToClusterCache(identity, c.deploymentInCache)
			deploymentCache.DeleteFromDeploymentClusterCache(identity, c.deploymentToDelete)
			if len(deploymentCache.cache[identity].Deployments) != c.expectedDeploymentCacheSize {
				t.Fatalf("expected deployment cache size to have size: %v, but got: %v",
					c.expectedDeploymentCacheSize, len(deploymentCache.cache))
			}
		})
	}
}

func TestHandleAddUpdateDeploymentTypeAssertion(t *testing.T) {

	ctx := context.Background()
	deploymentController := &DeploymentController{
		Cache: &deploymentCache{
			cache: make(map[string]*DeploymentClusterEntry),
			mutex: &sync.Mutex{},
		},
	}

	testCases := []struct {
		name          string
		deployment    interface{}
		expectedError error
	}{
		{
			name: "Given context, Deployment and DeploymentController " +
				"When Deployment param is nil " +
				"Then func should return an error",
			deployment:    nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.Deployment"),
		},
		{
			name: "Given context, Deployment and DeploymentController " +
				"When Deployment param is not of type *v1.Deployment " +
				"Then func should return an error",
			deployment:    struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.Deployment"),
		},
		{
			name: "Given context, Deployment and DeploymentController " +
				"When Deployment param is of type *v1.Deployment " +
				"Then func should not return an error",
			deployment: &k8sAppsV1.Deployment{
				Spec: k8sAppsV1.DeploymentSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Labels: make(map[string]string),
						},
					},
				},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := HandleAddUpdateDeployment(ctx, tc.deployment, deploymentController)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				if err != nil {
					assert.Fail(t, "expected error to be nil but got %v", err)
				}
			}

		})
	}

}

func TestDeploymentDeleted(t *testing.T) {

	mockDeploymenttHandler := &test.MockDeploymentHandler{}
	ctx := context.Background()
	labelset := common.LabelSet{
		DeploymentAnnotation: "sidecar.istio.io/inject",
	}
	deploymentController := DeploymentController{
		DeploymentHandler: mockDeploymenttHandler,
		Cache: &deploymentCache{
			cache: make(map[string]*DeploymentClusterEntry),
			mutex: &sync.Mutex{},
		},
		labelSet:  &labelset,
		K8sClient: fake.NewSimpleClientset(),
	}

	deploymentControllerWithErrorHandler := DeploymentController{
		DeploymentHandler: &test.MockDeploymentHandlerError{},
		K8sClient:         fake.NewSimpleClientset(),
		Cache: &deploymentCache{
			cache: make(map[string]*DeploymentClusterEntry),
			mutex: &sync.Mutex{},
		},
		labelSet: &labelset,
	}

	testCases := []struct {
		name          string
		deployment    interface{}
		controller    *DeploymentController
		expectedError error
	}{
		{
			name: "Given context, Deployment " +
				"When Deployment param is nil " +
				"Then func should return an error",
			deployment:    nil,
			controller:    &deploymentController,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.Deployment"),
		},
		{
			name: "Given context, Deployment " +
				"When Deployment param is not of type *v1.Deployment " +
				"Then func should return an error",
			deployment:    struct{}{},
			controller:    &deploymentController,
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.Deployment"),
		},
		{
			name: "Given context, Deployment " +
				"When Deployment param is of type *v1.Deployment " +
				"Then func should not return an error",
			deployment: &k8sAppsV1.Deployment{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test-ns",
				},
				Spec: k8sAppsV1.DeploymentSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Namespace: "test-ns",
							Labels:    make(map[string]string),
						},
					},
				},
			},
			controller:    &deploymentController,
			expectedError: nil,
		},
		{
			name: "Given context, Deployment and DeploymentController " +
				"When Deployment param is of type *v1.Deployment with admiral.io/ignore annotation true" +
				"Then func should not return an error",
			deployment: &k8sAppsV1.Deployment{
				Spec: k8sAppsV1.DeploymentSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Namespace: "test-ns",
							Labels:    make(map[string]string),
							Annotations: map[string]string{
								common.AdmiralIgnoreAnnotation: "true",
							},
						},
					},
				},
			},
			controller:    &deploymentControllerWithErrorHandler,
			expectedError: nil,
		},
		{
			name: "Given context, Deployment and DeploymentController " +
				"When Deployment param is of type *v1.Deployment with admiral.io/ignore annotation false" +
				"Then func should not return an error",
			deployment: &k8sAppsV1.Deployment{
				Spec: k8sAppsV1.DeploymentSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Name:      "test",
							Namespace: "test-ns",
							Labels:    make(map[string]string),
							Annotations: map[string]string{
								common.AdmiralIgnoreAnnotation: "false",
								"sidecar.istio.io/inject":      "true",
							},
						},
					},
				},
			},
			controller:    &deploymentControllerWithErrorHandler,
			expectedError: errors.New("error while deleting deployment"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := tc.controller.Deleted(ctx, tc.deployment)
			if tc.expectedError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				if err != nil {
					assert.Fail(t, "expected error to be nil but got %v", err)
				}
			}

		})
	}

}

func TestUpdateProcessItemStatus(t *testing.T) {
	var (
		serviceAccount                     = &coreV1.ServiceAccount{}
		env                                = "prd"
		deploymentWithEnvAnnotationInCache = &k8sAppsV1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "abc-service-with-env",
				Namespace: "namespace-" + env,
			},
			Spec: k8sAppsV1.DeploymentSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app2"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app2", "env": "prd"},
					},
				},
			},
		}
		deploymentWithEnvAnnotationInCache2 = &k8sAppsV1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "abc-service2-with-env",
				Namespace: "namespace-" + env,
			},
			Spec: k8sAppsV1.DeploymentSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app2"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app4", "env": "prd"},
					},
				},
			},
		}
		deploymentNotInCache = &k8sAppsV1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "debug",
				Namespace: "namespace-" + env,
			},
			Spec: k8sAppsV1.DeploymentSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app3"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app3"},
					},
				},
			},
		}
		diffNSdeploymentNotInCache = &k8sAppsV1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "debug",
				Namespace: "namespace2-" + env,
			},
			Spec: k8sAppsV1.DeploymentSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app3"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app3"},
					},
				},
			},
		}
	)

	// Populating the deployment Cache
	deploymentCache := &deploymentCache{
		cache: make(map[string]*DeploymentClusterEntry),
		mutex: &sync.Mutex{},
	}

	deploymentController := &DeploymentController{
		Cache: deploymentCache,
	}

	deploymentCache.UpdateDeploymentToClusterCache("app2", deploymentWithEnvAnnotationInCache)
	deploymentCache.UpdateDeploymentToClusterCache("app4", deploymentWithEnvAnnotationInCache2)

	testCases := []struct {
		name           string
		obj            interface{}
		statusToSet    string
		expectedErr    error
		expectedStatus string
	}{
		{
			name: "Given deployment cache has a valid deployment in its cache, " +
				"And the deployment has an env annotation and is processed" +
				"Then, the status for the valid deployment should be updated to processed",
			obj:            deploymentWithEnvAnnotationInCache,
			statusToSet:    common.Processed,
			expectedErr:    nil,
			expectedStatus: common.Processed,
		},
		{
			name: "Given deployment cache has a valid deployment in its cache, " +
				"And the deployment has an env annotation and is processed" +
				"Then, the status for the valid deployment should be updated to not processed",
			obj:            deploymentWithEnvAnnotationInCache,
			statusToSet:    common.NotProcessed,
			expectedErr:    nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given deployment cache does not has a valid deployment in its cache, " +
				"Then, the status for the valid deployment should be not processed, " +
				"And an error should be returned with the deployment not found message",
			obj:            deploymentNotInCache,
			statusToSet:    common.NotProcessed,
			expectedErr:    fmt.Errorf(LogCacheFormat, "Update", "Deployment", "debug", "namespace-prd", "", "nothing to update, deployment not found in cache"),
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given deployment cache does not has a valid deployment in its cache, " +
				"And deployment is in a different namespace, " +
				"Then, the status for the valid deployment should be not processed, " +
				"And an error should be returned with the deployment not found message",
			obj:            diffNSdeploymentNotInCache,
			statusToSet:    common.NotProcessed,
			expectedErr:    fmt.Errorf(LogCacheFormat, "Update", "Deployment", "debug", "namespace2-prd", "", "nothing to update, deployment not found in cache"),
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:            serviceAccount,
			expectedErr:    fmt.Errorf("type assertion failed"),
			expectedStatus: common.NotProcessed,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := deploymentController.UpdateProcessItemStatus(c.obj, c.statusToSet)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			status, _ := deploymentController.GetProcessItemStatus(c.obj)
			assert.Equal(t, c.expectedStatus, status)
		})
	}
}

func TestGetProcessItemStatus(t *testing.T) {
	var (
		serviceAccount                     = &coreV1.ServiceAccount{}
		env                                = "prd"
		deploymentWithEnvAnnotationInCache = &k8sAppsV1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "abc-service-with-env",
				Namespace: "namespace-" + env,
			},
			Spec: k8sAppsV1.DeploymentSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app2"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app2", "env": "prd"},
					},
				},
			},
		}
		deploymentNotInCache = &k8sAppsV1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "debug",
				Namespace: "namespace-" + env,
			},
			Spec: k8sAppsV1.DeploymentSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app3"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app3"},
					},
				},
			},
		}
	)

	// Populating the deployment Cache
	deploymentCache := &deploymentCache{
		cache: make(map[string]*DeploymentClusterEntry),
		mutex: &sync.Mutex{},
	}

	deploymentController := &DeploymentController{
		Cache: deploymentCache,
	}

	deploymentCache.UpdateDeploymentToClusterCache(common.GetDeploymentGlobalIdentifier(deploymentWithEnvAnnotationInCache), deploymentWithEnvAnnotationInCache)
	deploymentCache.UpdateDeploymentProcessStatus(deploymentWithEnvAnnotationInCache, common.Processed)

	testCases := []struct {
		name           string
		obj            interface{}
		expectedErr    error
		expectedResult string
	}{
		{
			name: "Given deployment cache has a valid deployment in its cache, " +
				"And the deployment has an env annotation and is processed" +
				"Then, we should be able to get the status as processed",
			obj:            deploymentWithEnvAnnotationInCache,
			expectedResult: common.Processed,
		},
		{
			name: "Given deployment cache does not has a valid deployment in its cache, " +
				"Then, the status for the valid deployment should not be updated",
			obj:            deploymentNotInCache,
			expectedResult: common.NotProcessed,
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:            serviceAccount,
			expectedErr:    fmt.Errorf("type assertion failed"),
			expectedResult: common.NotProcessed,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			res, err := deploymentController.GetProcessItemStatus(c.obj)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			assert.Equal(t, c.expectedResult, res)
		})
	}
}

func TestGetByIdentity(t *testing.T) {
	var (
		env = "prd"

		deploymentWithEnvAnnotationInCache = &k8sAppsV1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "abc-service-with-env",
				Namespace: "namespace-" + env,
			},
			Spec: k8sAppsV1.DeploymentSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app2"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app2", "env": "prd"},
					},
				},
			},
		}
	)

	// Populating the deployment Cache
	deploymentCache := &deploymentCache{
		cache: make(map[string]*DeploymentClusterEntry),
		mutex: &sync.Mutex{},
	}

	deploymentCache.UpdateDeploymentToClusterCache("app2", deploymentWithEnvAnnotationInCache)

	testCases := []struct {
		name             string
		keyToGetIdentity string
		expectedResult   map[string]*DeploymentItem
	}{
		{
			name: "Given deployment cache has a deployment for the key in its cache, " +
				"Then, the function would return the Deployments",
			keyToGetIdentity: "app2",
			expectedResult:   map[string]*DeploymentItem{"prd": &DeploymentItem{Deployment: deploymentWithEnvAnnotationInCache, Status: common.ProcessingInProgress}},
		},
		{
			name: "Given deployment cache does not have a deployment for the key in its cache, " +
				"Then, the function would return nil",
			keyToGetIdentity: "app5",
			expectedResult:   nil,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			res := deploymentCache.GetByIdentity(c.keyToGetIdentity)
			assert.Equal(t, c.expectedResult, res)
		})
	}
}

func TestDeploymentLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a Deployment object
	d := &DeploymentController{}
	d.LogValueOfAdmiralIoIgnore("not a deployment")
	// No error should occur

	// Test case 2: K8sClient is nil
	d = &DeploymentController{}
	d.LogValueOfAdmiralIoIgnore(&k8sAppsV1.Deployment{})
	// No error should occur

	// Test case 3: Namespace is not found
	mockClient := fake.NewSimpleClientset()
	d = &DeploymentController{K8sClient: mockClient}
	d.LogValueOfAdmiralIoIgnore(&k8sAppsV1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}})
	// No error should occur

	// Test case 4: AdmiralIgnoreAnnotation is not set
	mockClient = fake.NewSimpleClientset(&coreV1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}})
	d = &DeploymentController{K8sClient: mockClient}
	d.LogValueOfAdmiralIoIgnore(&k8sAppsV1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}})
	// No error should occur

	// Test case 5: AdmiralIgnoreAnnotation is set in Deployment object
	mockClient = fake.NewSimpleClientset(&coreV1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}})
	d = &DeploymentController{K8sClient: mockClient}
	deployment := &k8sAppsV1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Annotations: map[string]string{
				common.AdmiralIgnoreAnnotation: "true",
			},
		},
	}
	d.LogValueOfAdmiralIoIgnore(deployment)
	// No error should occur

	// Test case 6: AdmiralIgnoreAnnotation is set in Namespace object
	mockClient = fake.NewSimpleClientset(&coreV1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
			Annotations: map[string]string{
				common.AdmiralIgnoreAnnotation: "true",
			},
		},
	})
	d = &DeploymentController{K8sClient: mockClient}
	deployment = &k8sAppsV1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}}
	d.LogValueOfAdmiralIoIgnore(deployment)
	// No error should occur
}
