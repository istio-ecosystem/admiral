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

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	argofake "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/fake"
	argoprojv1alpha1 "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/typed/rollouts/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd"
)

func TestNewRolloutController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	rolHandler := test.MockRolloutHandler{}

	depCon, _ := NewRolloutsController(stop, &rolHandler, config, time.Duration(1000), loader.GetFakeClientLoader())

	if depCon == nil {
		t.Errorf("Rollout controller should not be nil")
	}
}

func TestRolloutController_Added(t *testing.T) {
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
	//Rollouts with the correct label are added to the cache
	mdh := test.MockRolloutHandler{}
	cache := rolloutCache{
		cache: map[string]*RolloutClusterEntry{},
		mutex: &sync.Mutex{},
	}
	labelset := common.LabelSet{
		DeploymentAnnotation: "sidecar.istio.io/inject",
		AdmiralIgnoreLabel:   "admiral-ignore",
	}
	depController := RolloutController{
		RolloutHandler: &mdh,
		Cache:          &cache,
		labelSet:       &labelset,
	}
	rollout := argo.Rollout{}
	rollout.Spec.Template.Labels = map[string]string{"identity": "rollout", "istio-injected": "true"}
	rollout.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}
	rollout.Namespace = "rolloutns"
	rolloutWithBadLabels := argo.Rollout{}
	rolloutWithBadLabels.Spec.Template.Labels = map[string]string{"identity": "rolloutWithBadLabels", "random-label": "true"}
	rolloutWithBadLabels.Spec.Template.Annotations = map[string]string{"admiral.io/env": "dev"}
	rolloutWithBadLabels.Namespace = "rolloutWithBadLabelsns"
	rolloutWithIgnoreLabels := argo.Rollout{}
	rolloutWithIgnoreLabels.Spec.Template.Labels = map[string]string{"identity": "rolloutWithIgnoreLabels", "istio-injected": "true", "admiral-ignore": "true"}
	rolloutWithIgnoreLabels.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}
	rolloutWithIgnoreLabels.Namespace = "rolloutWithIgnoreLabelsns"
	rolloutWithIgnoreAnnotations := argo.Rollout{}
	rolloutWithIgnoreAnnotations.Spec.Template.Labels = map[string]string{"identity": "rolloutWithIgnoreAnnotations"}
	rolloutWithIgnoreAnnotations.Annotations = map[string]string{"admiral.io/ignore": "true"}
	rolloutWithIgnoreAnnotations.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}
	rolloutWithIgnoreAnnotations.Namespace = "rolloutWithIgnoreAnnotationsns"
	rolloutWithNsIgnoreAnnotations := argo.Rollout{}
	rolloutWithNsIgnoreAnnotations.Spec.Template.Labels = map[string]string{"identity": "rolloutWithNsIgnoreAnnotations"}
	rolloutWithNsIgnoreAnnotations.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}
	rolloutWithNsIgnoreAnnotations.Namespace = "test-ns"

	testCases := []struct {
		name                  string
		rollout               *argo.Rollout
		expectedRollout       *argo.Rollout
		id                    string
		expectedCacheContains bool
	}{
		{
			name:                  "Expects rollout to be added to the cache when the correct label is present",
			rollout:               &rollout,
			expectedRollout:       &rollout,
			id:                    "rollout",
			expectedCacheContains: true,
		},
		{
			name:                  "Expects rollout to not be added to the cache when the correct label is not present",
			rollout:               &rolloutWithBadLabels,
			expectedRollout:       nil,
			id:                    "rolloutWithBadLabels",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored rollout identified by label to not be added to the cache",
			rollout:               &rolloutWithIgnoreLabels,
			expectedRollout:       nil,
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored rollout identified by rollout annotation to not be added to the cache",
			rollout:               &rolloutWithIgnoreAnnotations,
			expectedRollout:       nil,
			id:                    "rolloutWithIgnoreAnnotations",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored rollout identified by namespace annotation to not be added to the cache",
			rollout:               &rolloutWithNsIgnoreAnnotations,
			expectedRollout:       nil,
			id:                    "rolloutWithNsIgnoreAnnotations",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored rollout identified by label to be removed from the cache",
			rollout:               &rollout,
			expectedRollout:       nil,
			id:                    "rollout",
			expectedCacheContains: false,
		},
	}
	depController.K8sClient = fake.NewSimpleClientset()
	ns := coreV1.Namespace{}
	ns.Name = "test-ns"
	ns.Annotations = map[string]string{"admiral.io/ignore": "true"}
	depController.K8sClient.CoreV1().Namespaces().Create(ctx, &ns, metav1.CreateOptions{})
	depController.Cache.cache = map[string]*RolloutClusterEntry{}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "Expects ignored rollout identified by label to be removed from the cache" {
				rollout.Spec.Template.Labels["admiral-ignore"] = "true"
			}
			depController.Added(ctx, c.rollout)
			rolloutClusterEntry := depController.Cache.cache[c.id]
			var rolloutsMap map[string]*RolloutItem = nil
			if rolloutClusterEntry != nil {
				rolloutsMap = rolloutClusterEntry.Rollouts
			}
			var rolloutObj *argo.Rollout = nil
			if rolloutsMap != nil && len(rolloutsMap) > 0 {
				rolloutObj = rolloutsMap["dev"].Rollout
			}
			if !reflect.DeepEqual(c.expectedRollout, rolloutObj) {
				t.Errorf("Expected rollout %+v but got %+v", c.expectedRollout, rolloutObj)
			}
		})
	}
}

func TestRolloutController_Deleted(t *testing.T) {
	ctx := context.Background()
	//Rollouts with the correct label are added to the cache
	mdh := test.MockRolloutHandler{}
	cache := rolloutCache{
		cache: map[string]*RolloutClusterEntry{},
		mutex: &sync.Mutex{},
	}
	labelset := common.LabelSet{
		DeploymentAnnotation: "sidecar.istio.io/inject",
		AdmiralIgnoreLabel:   "admiral-ignore",
	}
	depController := RolloutController{
		RolloutHandler: &mdh,
		Cache:          &cache,
		labelSet:       &labelset,
	}
	rollout := argo.Rollout{}
	rollout.Spec.Template.Labels = map[string]string{"identity": "id", "istio-injected": "true"}
	rollout.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}

	testCases := []struct {
		name            string
		rollout         *argo.Rollout
		expectedRollout *argo.Rollout
	}{
		{
			name:            "Expects rollout to be deleted from the cache when the correct label is present",
			rollout:         &rollout,
			expectedRollout: nil,
		},
		{
			name:            "Expects no error thrown if calling delete on an rollout not exist in cache",
			rollout:         &rollout,
			expectedRollout: nil,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			depController.K8sClient = fake.NewSimpleClientset()
			depController.Cache.cache = map[string]*RolloutClusterEntry{}
			if c.name == "Expects rollout to be deleted from the cache when the correct label is present" {
				rolItem := &RolloutItem{
					Rollout: c.rollout,
				}
				depController.Cache.cache["id"] = &RolloutClusterEntry{
					Identity: "id",
					Rollouts: map[string]*RolloutItem{
						"default": rolItem,
					},
				}
			}
			depController.Deleted(ctx, c.rollout)

			if c.expectedRollout == nil {
				if len(depController.Cache.cache) > 0 && len(depController.Cache.cache["id"].Rollouts) != 0 {
					t.Errorf("Cache should remain the key with empty value if expected rollout is nil")
				}
			}
		})
	}

}

func TestRolloutController_GetRolloutBySelectorInNamespace(t *testing.T) {

	rollout := argo.Rollout{}
	rollout.Namespace = "namespace"
	rollout.Name = "fake-app-rollout-qal"
	rollout.Spec = argo.RolloutSpec{
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"}},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "qal"},
			},
		},
	}

	rollout2 := argo.Rollout{}
	rollout2.Namespace = "namespace"
	rollout2.Name = "fake-app-rollout-e2e"
	rollout2.Spec = argo.RolloutSpec{
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"}},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "e2e"},
			},
		},
	}

	rollout3 := argo.Rollout{}
	rollout3.Namespace = "namespace"
	rollout3.Name = "fake-app-rollout-prf-1"
	rollout3.CreationTimestamp = v1.Now()
	rollout3.Spec = argo.RolloutSpec{
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"}},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "prf"},
			},
		},
	}

	rollout4 := argo.Rollout{}
	rollout4.Namespace = "namespace"
	rollout4.Name = "fake-app-rollout-prf-2"
	rollout4.CreationTimestamp = v1.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
	rollout4.Spec = argo.RolloutSpec{
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app2"}},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app2", "env": "prf"},
			},
		},
	}

	oneRolloutClient := argofake.NewSimpleClientset(&rollout).ArgoprojV1alpha1()

	allRolloutsClient := argofake.NewSimpleClientset(&rollout, &rollout2, &rollout3, &rollout4).ArgoprojV1alpha1()

	noRolloutsClient := argofake.NewSimpleClientset().ArgoprojV1alpha1()

	rolloutController := &RolloutController{}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name             string
		expectedRollouts []argo.Rollout
		fakeClient       argoprojv1alpha1.ArgoprojV1alpha1Interface
		selector         map[string]string
	}{
		{
			name:             "Get one",
			expectedRollouts: []argo.Rollout{rollout},
			fakeClient:       oneRolloutClient,
			selector:         map[string]string{"identity": "app1", common.RolloutPodHashLabel: "random-hash"},
		},
		{
			name:             "Get one from long list",
			expectedRollouts: []argo.Rollout{rollout4},
			fakeClient:       allRolloutsClient,
			selector:         map[string]string{"identity": "app2", common.RolloutPodHashLabel: "random-hash"},
		},
		{
			name:             "Get many from long list",
			expectedRollouts: []argo.Rollout{rollout, rollout3, rollout2},
			fakeClient:       allRolloutsClient,
			selector:         map[string]string{"identity": "app1", common.RolloutPodHashLabel: "random-hash"},
		},
		{
			name:             "Get none from long list",
			expectedRollouts: []argo.Rollout{},
			fakeClient:       allRolloutsClient,
			selector:         map[string]string{"identity": "app3", common.RolloutPodHashLabel: "random-hash"},
		},
		{
			name:             "Get none from empty list",
			expectedRollouts: []argo.Rollout{},
			fakeClient:       noRolloutsClient,
			selector:         map[string]string{"identity": "app1"},
		},
	}

	ctx := context.Background()

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			rolloutController.RolloutClient = c.fakeClient
			returnedRollouts := rolloutController.GetRolloutBySelectorInNamespace(ctx, c.selector, "namespace")

			sort.Slice(returnedRollouts, func(i, j int) bool {
				return returnedRollouts[i].Name > returnedRollouts[j].Name
			})

			sort.Slice(c.expectedRollouts, func(i, j int) bool {
				return c.expectedRollouts[i].Name > c.expectedRollouts[j].Name
			})

			if len(returnedRollouts) != len(c.expectedRollouts) {
				t.Fatalf("Returned the wrong number of deploymenrs. Found %v but expected %v", len(returnedRollouts), len(c.expectedRollouts))
			}

			if !cmp.Equal(returnedRollouts, c.expectedRollouts) {
				t.Fatalf("Rollout mismatch. Diff: %v", cmp.Diff(returnedRollouts, c.expectedRollouts))
			}

		})
	}
}

func TestHandleAddUpdateRolloutTypeAssertion(t *testing.T) {

	ctx := context.Background()
	rolloutController := &RolloutController{
		Cache: &rolloutCache{
			cache: make(map[string]*RolloutClusterEntry),
			mutex: &sync.Mutex{},
		},
	}

	testCases := []struct {
		name          string
		rollout       interface{}
		expectedError error
	}{
		{
			name: "Given context, Rollout and RolloutController " +
				"When Rollout param is nil " +
				"Then func should return an error",
			rollout:       nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *argo.Rollout"),
		},
		{
			name: "Given context, Rollout and RolloutController " +
				"When sidecar param is not of type *argo.Rollout " +
				"Then func should return an error",
			rollout:       struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *argo.Rollout"),
		},
		{
			name: "Given context, Rollout and RolloutController " +
				"When Rollout param is of type *argo.Rollout " +
				"Then func should not return an error",
			rollout: &argo.Rollout{
				Spec: argo.RolloutSpec{
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

			err := HandleAddUpdateRollout(ctx, tc.rollout, rolloutController)
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

func TestRolloutDeleted(t *testing.T) {

	mockRolloutHandler := &test.MockRolloutHandler{}
	ctx := context.Background()
	labelset := common.LabelSet{
		DeploymentAnnotation: "sidecar.istio.io/inject",
	}
	rolloutController := &RolloutController{
		RolloutHandler: mockRolloutHandler,
		Cache: &rolloutCache{
			cache: make(map[string]*RolloutClusterEntry),
			mutex: &sync.Mutex{},
		},
		labelSet:  &labelset,
		K8sClient: fake.NewSimpleClientset(),
	}

	rolloutControllerWithErrorHandler := &RolloutController{
		RolloutHandler: &test.MockRolloutHandlerError{},
		K8sClient:      fake.NewSimpleClientset(),
		Cache: &rolloutCache{
			cache: make(map[string]*RolloutClusterEntry),
			mutex: &sync.Mutex{},
		},
		labelSet: &labelset,
	}
	testCases := []struct {
		name          string
		rollout       interface{}
		controller    *RolloutController
		expectedError error
	}{
		{
			name: "Given context, Rollout " +
				"When Rollout param is nil " +
				"Then func should return an error",
			rollout:       nil,
			controller:    rolloutController,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *argo.Rollout"),
		},
		{
			name: "Given context, Rollout " +
				"When Rollout param is not of type *argo.Rollout " +
				"Then func should return an error",
			rollout:       struct{}{},
			controller:    rolloutController,
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *argo.Rollout"),
		},
		{
			name: "Given context, Rollout " +
				"When Rollout param is of type *argo.Rollout " +
				"Then func should not return an error",
			rollout: &argo.Rollout{
				Spec: argo.RolloutSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Name:      "test",
							Namespace: "test-ns",
							Labels:    make(map[string]string),
						},
					},
				},
			},
			controller:    rolloutController,
			expectedError: nil,
		},
		{
			name: "Given context, Deployment and DeploymentController " +
				"When Deployment param is of type *argo.Rollout with admiral.io/ignore annotation true" +
				"Then func should not return an error",
			rollout: &argo.Rollout{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test",
					Namespace: "test-ns",
					Annotations: map[string]string{
						common.AdmiralIgnoreAnnotation: "true",
						"sidecar.istio.io/inject":      "true",
					},
				},
				Spec: argo.RolloutSpec{
					Template: coreV1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Name:      "test",
							Namespace: "test-ns",
							Labels:    make(map[string]string),
							Annotations: map[string]string{
								common.AdmiralIgnoreAnnotation: "true",
								"sidecar.istio.io/inject":      "true",
							},
						},
					},
				},
			},
			controller:    rolloutControllerWithErrorHandler,
			expectedError: nil,
		},
		{
			name: "Given context, Rollout and RolloutController " +
				"When Rollout param is of type *argo.Rollout with admiral.io/ignore annotation false" +
				"Then func should not return an error",
			rollout: &argo.Rollout{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test",
					Namespace: "test-ns",
				},
				Spec: argo.RolloutSpec{
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
			controller:    rolloutControllerWithErrorHandler,
			expectedError: errors.New("error while deleting rollout"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := tc.controller.Deleted(ctx, tc.rollout)
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

func TestUpdateRolloutProcessStatus(t *testing.T) {
	var (
		serviceAccount                  = &coreV1.ServiceAccount{}
		env                             = "prd"
		rolloutWithEnvAnnotationInCache = &argo.Rollout{
			ObjectMeta: v1.ObjectMeta{
				Name:      "debug-incache",
				Namespace: "namespace-" + env,
			},
			Spec: argo.RolloutSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app1", "env": "prd"},
					},
				},
			},
		}
		rolloutWithEnvAnnotationInCache2 = &argo.Rollout{
			ObjectMeta: v1.ObjectMeta{
				Name:      "debug2-incache",
				Namespace: "namespace-" + env,
			},
			Spec: argo.RolloutSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app2", "env": "prd"},
					},
				},
			},
		}
		rolloutNotInCache = &argo.Rollout{
			ObjectMeta: v1.ObjectMeta{
				Name:      "debug",
				Namespace: "namespace-" + env,
			},
			Spec: argo.RolloutSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app2"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app3", "env": "prd"},
					},
				},
			},
		}
		diffNsRolloutNotInCache = &argo.Rollout{
			ObjectMeta: v1.ObjectMeta{
				Name:      "debug",
				Namespace: "namespace2-" + env,
			},
			Spec: argo.RolloutSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app2"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app4", "env": "prd"},
					},
				},
			},
		}
	)

	// Populating the deployment Cache
	rolloutCache := &rolloutCache{
		cache: make(map[string]*RolloutClusterEntry),
		mutex: &sync.Mutex{},
	}

	rolloutController := &RolloutController{
		Cache: rolloutCache,
	}

	rolloutCache.UpdateRolloutToClusterCache("app1", rolloutWithEnvAnnotationInCache)
	rolloutCache.UpdateRolloutToClusterCache("app2", rolloutWithEnvAnnotationInCache2)

	cases := []struct {
		name           string
		obj            interface{}
		statusToSet    bool
		expectedErr    error
		expectedStatus bool
	}{
		{
			name: "Given rollout cache has a valid rollout in its cache, " +
				"And the rollout has an env annotation" +
				"Then, the status for the valid rollout should be updated with true",
			obj:            rolloutWithEnvAnnotationInCache,
			statusToSet:    true,
			expectedErr:    nil,
			expectedStatus: true,
		},
		{
			name: "Given rollout cache has a valid rollout in its cache, " +
				"And the rollout has an env annotation" +
				"Then, the status for the valid rollout should be updated with false",
			obj:            rolloutWithEnvAnnotationInCache2,
			statusToSet:    false,
			expectedErr:    nil,
			expectedStatus: false,
		},
		{
			name: "Given rollout cache does not has a valid rollout in its cache, " +
				"Then, the status for the valid deployment should be false, " +
				"And an error should be returned with the rollout not found message",
			obj:            rolloutNotInCache,
			statusToSet:    false,
			expectedErr:    fmt.Errorf(LogCacheFormat, "Update", "Rollout", "debug", "namespace-prd", "", "nothing to update, rollout not found in cache"),
			expectedStatus: false,
		},
		{
			name: "Given rollout cache does not has a valid rollout in its cache, " +
				"Then, the status for the valid deployment should be false, " +
				"And an error should be returned with the rollout not found message",
			obj:            diffNsRolloutNotInCache,
			statusToSet:    false,
			expectedErr:    fmt.Errorf(LogCacheFormat, "Update", "Rollout", "debug", "namespace2-prd", "", "nothing to update, rollout not found in cache"),
			expectedStatus: false,
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:         serviceAccount,
			expectedErr: fmt.Errorf("type assertion failed"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := rolloutController.UpdateProcessItemStatus(c.obj, common.Processed)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
		})
	}
}

func TestGetRolloutProcessStatus(t *testing.T) {
	var (
		serviceAccount                  = &coreV1.ServiceAccount{}
		env                             = "prd"
		rolloutWithEnvAnnotationInCache = &argo.Rollout{
			ObjectMeta: v1.ObjectMeta{
				Name:      "debug-incache",
				Namespace: "namespace-" + env,
			},
			Spec: argo.RolloutSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app1", "env": "prd"},
					},
				},
			},
		}
		rolloutNotInCache = &argo.Rollout{
			ObjectMeta: v1.ObjectMeta{
				Name:      "debug",
				Namespace: "namespace-" + env,
			},
			Spec: argo.RolloutSpec{
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
	rolloutCache := &rolloutCache{
		cache: make(map[string]*RolloutClusterEntry),
		mutex: &sync.Mutex{},
	}

	rolloutController := &RolloutController{
		Cache: rolloutCache,
	}

	rolloutCache.UpdateRolloutToClusterCache("app1", rolloutWithEnvAnnotationInCache)
	rolloutCache.UpdateRolloutProcessStatus(rolloutWithEnvAnnotationInCache, common.Processed)

	cases := []struct {
		name           string
		obj            interface{}
		expectedResult string
		expectedErr    error
	}{
		{
			name: "Given rollout cache has a valid rollout in its cache, " +
				"And the rollout has an env annotation and is processed" +
				"Then, the status for the valid rollout should be updated to processed",
			obj:            rolloutWithEnvAnnotationInCache,
			expectedResult: common.Processed,
		},
		{
			name: "Given rollout cache does not has a valid rollout in its cache, " +
				"Then, the status for the valid rollout should be false",
			obj:            rolloutNotInCache,
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

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := rolloutController.GetProcessItemStatus(c.obj)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			assert.Equal(t, c.expectedResult, res)
		})
	}
}

func TestRolloutGetByIdentity(t *testing.T) {
	var (
		env                             = "prd"
		rolloutWithEnvAnnotationInCache = &argo.Rollout{
			ObjectMeta: v1.ObjectMeta{
				Name:      "debug-incache",
				Namespace: "namespace-" + env,
			},
			Spec: argo.RolloutSpec{
				Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"}},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"identity": "app2", "env": "prd"},
					},
				},
			},
		}
	)

	// Populating the deployment Cache
	rolloutCache := &rolloutCache{
		cache: make(map[string]*RolloutClusterEntry),
		mutex: &sync.Mutex{},
	}

	rolloutCache.UpdateRolloutToClusterCache("app2", rolloutWithEnvAnnotationInCache)

	testCases := []struct {
		name             string
		keyToGetIdentity string
		expectedResult   map[string]*RolloutItem
	}{
		{
			name: "Given rollout cache has a rollout for the key in its cache, " +
				"Then, the function would return the Rollouts",
			keyToGetIdentity: "app2",
			expectedResult:   map[string]*RolloutItem{"prd": &RolloutItem{Rollout: rolloutWithEnvAnnotationInCache, Status: common.ProcessingInProgress}},
		},
		{
			name: "Given rollout cache does not have a rollout for the key in its cache, " +
				"Then, the function would return nil",
			keyToGetIdentity: "app5",
			expectedResult:   nil,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			res := rolloutCache.GetByIdentity(c.keyToGetIdentity)
			assert.Equal(t, c.expectedResult, res)
		})
	}
}

func TestRolloutLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a Rollout object
	d := &RolloutController{}
	d.LogValueOfAdmiralIoIgnore("not a rollout")
	// No error should occur

	// Test case 2: K8sClient is nil
	d = &RolloutController{}
	d.LogValueOfAdmiralIoIgnore(&argo.Rollout{})
	// No error should occur

	// Test case 3: Namespace has no annotations and Rollout has no annotations
	d = &RolloutController{K8sClient: fake.NewSimpleClientset()}
	d.LogValueOfAdmiralIoIgnore(&argo.Rollout{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}})
	// No error should occur

	// Test case 4: Namespace has AdmiralIgnoreAnnotation set and Rollout has no annotations
	d = &RolloutController{K8sClient: fake.NewSimpleClientset(&coreV1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns", Annotations: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}})}
	d.LogValueOfAdmiralIoIgnore(&argo.Rollout{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}})
	// No error should occur

	// Test case 5: Namespace has no annotations and Rollout has AdmiralIgnoreAnnotation set
	d = &RolloutController{K8sClient: fake.NewSimpleClientset()}
	d.LogValueOfAdmiralIoIgnore(&argo.Rollout{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Annotations: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}})
	// No error should occur

	// Test case 6: Namespace has AdmiralIgnoreAnnotation set and Rollout has AdmiralIgnoreAnnotation set
	d = &RolloutController{K8sClient: fake.NewSimpleClientset(&coreV1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns", Annotations: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}})}
	d.LogValueOfAdmiralIoIgnore(&argo.Rollout{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Annotations: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}})
	// No error should occur
}
