package admiral

import (
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	argofake "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/fake"
	argoprojv1alpha1 "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/typed/rollouts/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestNewRolloutController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	rolHandler := test.MockRolloutHandler{}

	depCon, err := NewRolloutsController("test", stop, &rolHandler, config, time.Duration(1000))

	if depCon == nil {
		t.Errorf("Rollout controller should not be nil")
	}
}

func TestRolloutController_Added(t *testing.T) {
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
	rolloutWithBadLabels := argo.Rollout{}
	rolloutWithBadLabels.Spec.Template.Labels = map[string]string{"identity": "id", "random-label": "true"}
	rolloutWithIgnoreLabels := argo.Rollout{}
	rolloutWithIgnoreLabels.Spec.Template.Labels = map[string]string{"identity": "id", "istio-injected": "true", "admiral-ignore": "true"}
	rolloutWithIgnoreLabels.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}
	rolloutWithIgnoreAnnotations := argo.Rollout{}
	rolloutWithIgnoreAnnotations.Spec.Template.Labels = map[string]string{"identity": "id"}
	rolloutWithIgnoreAnnotations.Annotations = map[string]string{"admiral.io/ignore": "true"}
	rolloutWithIgnoreAnnotations.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}
	rolloutWithNsIgnoreAnnotations := argo.Rollout{}
	rolloutWithNsIgnoreAnnotations.Spec.Template.Labels = map[string]string{"identity": "id"}
	rolloutWithNsIgnoreAnnotations.Spec.Template.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}
	rolloutWithNsIgnoreAnnotations.Namespace = "test-ns"

	testCases := []struct {
		name                  string
		rollout               *argo.Rollout
		expectedRollout       *argo.Rollout
		expectedCacheContains bool
	}{
		{
			name:                  "Expects rollout to be added to the cache when the correct label is present",
			rollout:               &rollout,
			expectedRollout:       &rollout,
			expectedCacheContains: true,
		},
		{
			name:                  "Expects rollout to not be added to the cache when the correct label is not present",
			rollout:               &rolloutWithBadLabels,
			expectedRollout:       nil,
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
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored rollout identified by namespace annotation to not be added to the cache",
			rollout:               &rolloutWithNsIgnoreAnnotations,
			expectedRollout:       nil,
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored rollout identified by label to be removed from the cache",
			rollout:               &rollout,
			expectedRollout:       &rollout,
			expectedCacheContains: false,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			depController.K8sClient = fake.NewSimpleClientset()
			if c.name == "Expects ignored rollout identified by namespace annotation to not be added to the cache" {
				ns := coreV1.Namespace{}
				ns.Name = "test-ns"
				ns.Annotations = map[string]string{"admiral.io/ignore": "true"}
				depController.K8sClient.CoreV1().Namespaces().Create(&ns)
			}
			depController.Cache.cache = map[string]*RolloutClusterEntry{}
			depController.Added(c.rollout)
			if c.expectedRollout == nil {
				if len(depController.Cache.cache) != 0 || (depController.Cache.cache["id"] != nil && len(depController.Cache.cache["id"].Rollouts) != 0) {
					t.Errorf("Cache should be empty if expected rollout is nil")
				}
			} else if len(depController.Cache.cache) == 0 && c.expectedCacheContains != false {
				t.Errorf("Unexpectedly empty cache. Expected cache to have entry for the given identifier")
			} else if len(depController.Cache.cache["id"].Rollouts) == 0 && c.expectedCacheContains != false {
				t.Errorf("Rollout controller cache has wrong size. Cached was expected to have rollout for environment %v but was not present.", common.Default)
			} else if depController.Cache.cache["id"].Rollouts[common.Default] != nil && depController.Cache.cache["id"].Rollouts[common.Default] != &rollout {
				t.Errorf("Incorrect rollout added to rollout controller cache. Got %v expected %v", depController.Cache.cache["id"].Rollouts[common.Default], rollout)
			}
		})
	}
}

func TestRolloutController_Deleted(t *testing.T) {
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
				depController.Cache.cache["id"] = &RolloutClusterEntry{
					Identity: "id",
					Rollouts: map[string]*argo.Rollout{
						"default": c.rollout,
					},
				}
			}
			depController.Deleted(c.rollout)

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
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"},},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "qal"},
			},
		},
	}
	rollout.Labels = map[string]string{"identity": "app1"}

	rollout2 := argo.Rollout{}
	rollout2.Namespace = "namespace"
	rollout2.Name = "fake-app-rollout-e2e"
	rollout2.Spec = argo.RolloutSpec{
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"},},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "e2e"},
			},
		},
	}
	rollout2.Labels = map[string]string{"identity": "app1"}

	rollout3 := argo.Rollout{}
	rollout3.Namespace = "namespace"
	rollout3.Name = "fake-app-rollout-prf-1"
	rollout3.CreationTimestamp = v1.Now()
	rollout3.Spec = argo.RolloutSpec{
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app1"},},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env": "prf"},
			},
		},
	}
	rollout3.Labels = map[string]string{"identity": "app1"}

	rollout4 := argo.Rollout{}
	rollout4.Namespace = "namespace"
	rollout4.Name = "fake-app-rollout-prf-2"
	rollout4.CreationTimestamp = v1.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
	rollout4.Spec = argo.RolloutSpec{
		Selector: &v1.LabelSelector{MatchLabels: map[string]string{"identity": "app2"},},
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app2", "env": "prf"},
			},
		},
	}
	rollout4.Labels = map[string]string{"identity": "app2"}

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
			selector:         map[string]string {"identity": "app1", common.RolloutPodHashLabel: "random-hash"},
		},
		{
			name:             "Get one from long list",
			expectedRollouts: []argo.Rollout{rollout4},
			fakeClient:       allRolloutsClient,
			selector:         map[string]string {"identity": "app2", common.RolloutPodHashLabel: "random-hash"},
		},
		{
			name:             "Get many from long list",
			expectedRollouts: []argo.Rollout{rollout, rollout3, rollout2},
			fakeClient:       allRolloutsClient,
			selector:         map[string]string {"identity": "app1", common.RolloutPodHashLabel: "random-hash"},
		},
		{
			name:             "Get none from long list",
			expectedRollouts: []argo.Rollout{},
			fakeClient:       allRolloutsClient,
			selector:         map[string]string {"identity": "app3", common.RolloutPodHashLabel: "random-hash"},
		},
		{
			name:             "Get none from empty list",
			expectedRollouts: []argo.Rollout{},
			fakeClient:       noRolloutsClient,
			selector:         map[string]string {"identity": "app1"},
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			rolloutController.RolloutClient = c.fakeClient
			returnedRollouts := rolloutController.GetRolloutBySelectorInNamespace(c.selector, "namespace")

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
