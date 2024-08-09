package clusters

import (
	"context"
	"testing"
	"time"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	cmp "github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"istio.io/api/networking/v1alpha3"
	v1alpha32 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioFake "istio.io/client-go/pkg/clientset/versioned/fake"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func admiralParamsForHandlerTests(argoEnabled bool) common.AdmiralParams {
	return common.AdmiralParams{
		ArgoRolloutsEnabled: argoEnabled,
		LabelSet:            &common.LabelSet{},
	}
}

func setupForHandlerTests(argoEnabled bool) {
	common.ResetSync()
	common.InitializeConfig(admiralParamsForHandlerTests(argoEnabled))
}

func TestIgnoreIstioResource(t *testing.T) {

	admiralParams := common.AdmiralParams{
		LabelSet:             &common.LabelSet{},
		TrafficConfigPersona: false,
		SyncNamespace:        "ns",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)

	//Struct of test case info. Name is required.
	testCases := []struct {
		name           string
		exportTo       []string
		annotations    map[string]string
		namespace      string
		expectedResult bool
	}{
		{
			name:           "Should return false when exportTo is not present",
			exportTo:       nil,
			annotations:    nil,
			namespace:      "random-ns",
			expectedResult: false,
		},
		{
			name:           "Should return false when its exported to *",
			exportTo:       []string{"*"},
			annotations:    nil,
			namespace:      "random-ns",
			expectedResult: false,
		},
		{
			name:           "Should return true when its exported to .",
			exportTo:       []string{"."},
			annotations:    nil,
			namespace:      "random-ns",
			expectedResult: true,
		},
		{
			name:           "Should return true when its exported to a handful of namespaces",
			exportTo:       []string{"namespace1", "namespace2"},
			annotations:    nil,
			namespace:      "random-ns",
			expectedResult: true,
		},
		{
			name:           "Should return true when admiral ignore annotation is set",
			exportTo:       []string{"*"},
			annotations:    map[string]string{common.AdmiralIgnoreAnnotation: "true"},
			namespace:      "random-ns",
			expectedResult: true,
		},
		{
			name:           "Should return true when its from istio-system namespace",
			exportTo:       []string{"*"},
			annotations:    nil,
			namespace:      "istio-system",
			expectedResult: true,
		},
		{
			name:           "Should return true when its from admiral sync namespace",
			exportTo:       []string{"*"},
			annotations:    nil,
			namespace:      "ns",
			expectedResult: true,
		},
		{
			name:           "created by cartographer",
			exportTo:       []string{"namespace1", "namespace2"},
			annotations:    map[string]string{common.CreatedBy: common.Cartographer},
			namespace:      "random-namespace",
			expectedResult: true,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := IgnoreIstioResource(c.exportTo, c.annotations, c.namespace)
			if result == c.expectedResult {
				//perfect
			} else {
				t.Errorf("Failed. Got %v, expected %v", result, c.expectedResult)
			}
		})
	}
}

func TestGetServiceForRolloutCanary(t *testing.T) {
	const (
		Namespace                  = "namespace"
		ServiceName                = "serviceName"
		StableServiceName          = "stableserviceName"
		CanaryServiceName          = "canaryserviceName"
		GeneratedStableServiceName = "hello-" + common.RolloutStableServiceSuffix
		RootService                = "hello-root-service"
		vsName1                    = "virtualservice1"
		vsName2                    = "virtualservice2"
		vsName3                    = "virtualservice3"
		vsName4                    = "virtualservice4"
		vsRoutePrimary             = "primary"
	)
	var (
		config = rest.Config{
			Host: "localhost",
		}
		stop            = make(chan struct{})
		fakeIstioClient = istioFake.NewSimpleClientset()
		selectorMap     = map[string]string{
			"app": "test",
		}
		ports = []coreV1.ServicePort{{Port: 8080}, {Port: 8081}}
	)
	s, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("failed to initialize service controller, err: %v", err)
	}
	r, err := admiral.NewRolloutsController(stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("failed ot initialize rollout controller, err: %v", err)
	}

	v := &istio.VirtualServiceController{
		IstioClient: fakeIstioClient,
	}

	rcTemp := &RemoteController{
		VirtualServiceController: v,
		ServiceController:        s,
		RolloutController:        r,
	}

	service := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: ServiceName, Namespace: Namespace, CreationTimestamp: metaV1.NewTime(time.Now())},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports:    ports,
		},
	}

	// namespace1 Services
	service1 := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: "dummy1", Namespace: "namespace1", CreationTimestamp: metaV1.NewTime(time.Now())},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports: []coreV1.ServicePort{{
				Port: 8080,
				Name: "random3",
			}, {
				Port: 8081,
				Name: "random4",
			},
			},
		},
	}

	// namespace4 Services
	service3 := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: "dummy3", Namespace: "namespace4", CreationTimestamp: metaV1.NewTime(time.Now())},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports: []coreV1.ServicePort{{
				Port: 8080,
				Name: "random3",
			}, {
				Port: 8081,
				Name: "random4",
			},
			},
		},
	}

	service4 := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: "dummy4", Namespace: "namespace4", CreationTimestamp: metaV1.NewTime(time.Now())},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports: []coreV1.ServicePort{{
				Port: 8081,
				Name: "random4",
			},
			},
		},
	}

	service5 := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: GeneratedStableServiceName, Namespace: "namespace5", CreationTimestamp: metaV1.NewTime(time.Now())},
		Spec: coreV1.ServiceSpec{
			Selector: map[string]string{
				"app": "test5",
			},
			Ports: []coreV1.ServicePort{{
				Port: 8081,
				Name: "random5",
			},
			},
		},
	}

	// namespace Services
	stableService := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: StableServiceName, Namespace: Namespace, CreationTimestamp: metaV1.NewTime(time.Now().Add(time.Duration(-15)))},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports:    ports,
		},
	}

	generatedStableService := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: GeneratedStableServiceName, Namespace: Namespace, CreationTimestamp: metaV1.NewTime(time.Now().Add(time.Duration(-15)))},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports:    ports,
		},
	}

	canaryService := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: CanaryServiceName, Namespace: Namespace, CreationTimestamp: metaV1.NewTime(time.Now().Add(time.Duration(-15)))},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports:    ports,
		},
	}

	rootService := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: RootService, Namespace: Namespace, CreationTimestamp: metaV1.NewTime(time.Now())},
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
			Ports:    ports,
		},
	}

	rcTemp.ServiceController.Cache.Put(service)
	rcTemp.ServiceController.Cache.Put(service1)
	rcTemp.ServiceController.Cache.Put(service3)
	rcTemp.ServiceController.Cache.Put(service4)
	rcTemp.ServiceController.Cache.Put(service5)
	rcTemp.ServiceController.Cache.Put(stableService)
	rcTemp.ServiceController.Cache.Put(canaryService)
	rcTemp.ServiceController.Cache.Put(generatedStableService)
	rcTemp.ServiceController.Cache.Put(rootService)

	virtualService := &v1alpha32.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{Name: vsName1, Namespace: Namespace},
		Spec: v1alpha3.VirtualService{
			Http: []*v1alpha3.HTTPRoute{{Route: []*v1alpha3.HTTPRouteDestination{
				{Destination: &v1alpha3.Destination{Host: StableServiceName}, Weight: 80},
				{Destination: &v1alpha3.Destination{Host: CanaryServiceName}, Weight: 20},
			}}},
		},
	}

	vsMutipleRoutesWithMatch := &v1alpha32.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{Name: vsName2, Namespace: Namespace},
		Spec: v1alpha3.VirtualService{
			Http: []*v1alpha3.HTTPRoute{{Name: vsRoutePrimary, Route: []*v1alpha3.HTTPRouteDestination{
				{Destination: &v1alpha3.Destination{Host: StableServiceName}, Weight: 80},
				{Destination: &v1alpha3.Destination{Host: CanaryServiceName}, Weight: 20},
			}}},
		},
	}

	vsMutipleRoutesWithZeroWeight := &v1alpha32.VirtualService{
		ObjectMeta: metaV1.ObjectMeta{Name: vsName4, Namespace: Namespace},
		Spec: v1alpha3.VirtualService{
			Http: []*v1alpha3.HTTPRoute{{Name: "random", Route: []*v1alpha3.HTTPRouteDestination{
				{Destination: &v1alpha3.Destination{Host: StableServiceName}, Weight: 100},
				{Destination: &v1alpha3.Destination{Host: CanaryServiceName}, Weight: 0},
			}}},
		},
	}
	ctx := context.Background()
	rcTemp.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(Namespace).Create(ctx, virtualService, metaV1.CreateOptions{})
	rcTemp.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(Namespace).Create(ctx, vsMutipleRoutesWithMatch, metaV1.CreateOptions{})
	rcTemp.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(Namespace).Create(ctx, vsMutipleRoutesWithZeroWeight, metaV1.CreateOptions{})

	canaryRollout := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	matchLabel := make(map[string]string)
	matchLabel["app"] = "test"

	labelSelector := metaV1.LabelSelector{
		MatchLabels: matchLabel,
	}
	canaryRollout.Spec.Selector = &labelSelector

	canaryRollout.Namespace = Namespace
	canaryRollout.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{},
	}

	canaryRolloutNS1 := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	matchLabel2 := make(map[string]string)
	matchLabel2["app"] = "test1"

	labelSelector2 := metaV1.LabelSelector{
		MatchLabels: matchLabel2,
	}
	canaryRolloutNS1.Spec.Selector = &labelSelector2

	canaryRolloutNS1.Namespace = "namespace1"
	canaryRolloutNS1.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{},
	}

	canaryRolloutNS4 := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{common.SidecarEnabledPorts: "8080"}},
		}}}
	matchLabel4 := make(map[string]string)
	matchLabel4["app"] = "test"
	labelSelector4 := metaV1.LabelSelector{
		MatchLabels: matchLabel4,
	}
	canaryRolloutNS4.Spec.Selector = &labelSelector4
	canaryRolloutNS4.Namespace = "namespace4"
	canaryRolloutNS4.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{},
	}

	canaryRolloutIstioVs := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	canaryRolloutIstioVs.Spec.Selector = &labelSelector

	canaryRolloutIstioVs.Namespace = Namespace
	canaryRolloutIstioVs.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
			TrafficRouting: &argo.RolloutTrafficRouting{
				Istio: &argo.IstioTrafficRouting{
					VirtualService: &argo.IstioVirtualService{Name: vsName1},
				},
			},
		},
	}

	canaryRolloutIstioVsRouteMatch := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	canaryRolloutIstioVsRouteMatch.Spec.Selector = &labelSelector

	canaryRolloutIstioVsRouteMatch.Namespace = Namespace
	canaryRolloutIstioVsRouteMatch.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
			TrafficRouting: &argo.RolloutTrafficRouting{
				Istio: &argo.IstioTrafficRouting{
					VirtualService: &argo.IstioVirtualService{Name: vsName2, Routes: []string{vsRoutePrimary}},
				},
			},
		},
	}

	canaryRolloutIstioVsRouteMisMatch := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	canaryRolloutIstioVsRouteMisMatch.Spec.Selector = &labelSelector

	canaryRolloutIstioVsRouteMisMatch.Namespace = Namespace
	canaryRolloutIstioVsRouteMisMatch.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
			TrafficRouting: &argo.RolloutTrafficRouting{
				Istio: &argo.IstioTrafficRouting{
					VirtualService: &argo.IstioVirtualService{Name: vsName2, Routes: []string{"random"}},
				},
			},
		},
	}

	canaryRolloutIstioVsZeroWeight := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	canaryRolloutIstioVsZeroWeight.Spec.Selector = &labelSelector

	canaryRolloutIstioVsZeroWeight.Namespace = Namespace
	canaryRolloutIstioVsZeroWeight.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
			TrafficRouting: &argo.RolloutTrafficRouting{
				Istio: &argo.IstioTrafficRouting{
					VirtualService: &argo.IstioVirtualService{Name: vsName4},
				},
			},
		},
	}

	canaryRolloutWithRootService := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	canaryRolloutWithRootService.Spec.Selector = &labelSelector

	canaryRolloutWithRootService.Namespace = Namespace
	canaryRolloutWithRootService.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
		},
	}

	canaryRolloutWithoutRootService := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	matchLabel5 := make(map[string]string)
	matchLabel5["app"] = "test5"

	labelSelector5 := metaV1.LabelSelector{
		MatchLabels: matchLabel5,
	}
	canaryRolloutWithoutRootService.Spec.Selector = &labelSelector5

	canaryRolloutWithoutRootService.Namespace = "namespace5"
	canaryRolloutWithoutRootService.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{},
	}

	canaryRolloutNoStrategy := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	matchLabel6 := make(map[string]string)
	matchLabel6["app"] = "test6"

	labelSelector6 := metaV1.LabelSelector{
		MatchLabels: matchLabel6,
	}
	canaryRolloutNoStrategy.Spec.Selector = &labelSelector6

	canaryRolloutNoStrategy.Namespace = "namespace6"

	canaryRolloutWithoutIstioVS := argo.Rollout{
		ObjectMeta: metaV1.ObjectMeta{Namespace: Namespace},
		Spec: argo.RolloutSpec{
			Selector: &labelSelector,
			Strategy: argo.RolloutStrategy{
				Canary: &argo.CanaryStrategy{
					StableService: StableServiceName,
					CanaryService: CanaryServiceName,
					TrafficRouting: &argo.RolloutTrafficRouting{
						Istio: &argo.IstioTrafficRouting{},
					},
				},
			},
		},
	}

	canaryRolloutIstioVsMismatch := argo.Rollout{
		ObjectMeta: metaV1.ObjectMeta{Namespace: Namespace},
		Spec: argo.RolloutSpec{
			Selector: &labelSelector,
			Strategy: argo.RolloutStrategy{
				Canary: &argo.CanaryStrategy{
					StableService: StableServiceName,
					CanaryService: CanaryServiceName,
					TrafficRouting: &argo.RolloutTrafficRouting{
						Istio: &argo.IstioTrafficRouting{
							VirtualService: &argo.IstioVirtualService{Name: "random"},
						},
					},
				},
			},
		},
	}

	canaryRolloutWithStableServiceNS4 := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{common.SidecarEnabledPorts: "8080"}},
		}}}
	canaryRolloutWithStableServiceNS4.Spec.Selector = &labelSelector

	canaryRolloutWithStableServiceNS4.Namespace = "namespace4"
	canaryRolloutWithStableServiceNS4.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{
			StableService: StableServiceName,
			CanaryService: CanaryServiceName,
		},
	}

	resultForDummy := map[string]*WeightedService{service3.Name: {Weight: 1, Service: service3}}

	resultForEmptyStableServiceOnRollout := map[string]*WeightedService{RootService: {Weight: 1, Service: rootService}}

	resultForCanaryWithIstio := map[string]*WeightedService{StableServiceName: {Weight: 80, Service: stableService},
		CanaryServiceName: {Weight: 20, Service: canaryService}}

	resultForCanaryWithRootService := map[string]*WeightedService{RootService: {Weight: 1, Service: rootService}}

	resultForCanaryWithStableService := map[string]*WeightedService{StableServiceName: {Weight: 1, Service: stableService}}

	resultForCanaryWithoutRootService := map[string]*WeightedService{GeneratedStableServiceName: {Weight: 1, Service: service5}}

	resultForCanaryWithStableServiceWeight := map[string]*WeightedService{StableServiceName: {Weight: 100, Service: stableService}}

	resultRolloutWithOneServiceHavingMeshPort := map[string]*WeightedService{service3.Name: {Weight: 1, Service: service3}}

	testCases := []struct {
		name    string
		rollout *argo.Rollout
		rc      *RemoteController
		result  map[string]*WeightedService
	}{
		{
			name:    "canaryRolloutHappyCaseMeshPortAnnotationOnRollout",
			rollout: &canaryRolloutNS4,
			rc:      rcTemp,
			result:  resultForDummy,
		}, {
			name:    "canaryRolloutWithoutSelectorMatch",
			rollout: &canaryRolloutNS1,
			rc:      rcTemp,
			result:  make(map[string]*WeightedService, 0),
		}, {
			name:    "canaryRolloutHappyCase",
			rollout: &canaryRollout,
			rc:      rcTemp,
			result:  resultForEmptyStableServiceOnRollout,
		}, {
			name:    "canaryRolloutWithoutIstioVS",
			rollout: &canaryRolloutWithoutIstioVS,
			rc:      rcTemp,
			result:  resultForCanaryWithStableService,
		}, {
			name:    "canaryRolloutWithIstioVsMimatch",
			rollout: &canaryRolloutIstioVsMismatch,
			rc:      rcTemp,
			result:  resultForCanaryWithStableService,
		}, {
			name:    "canaryRolloutWithIstioVirtualService",
			rollout: &canaryRolloutIstioVs,
			rc:      rcTemp,
			result:  resultForCanaryWithIstio,
		}, {
			name:    "canaryRolloutWithIstioVirtualServiceZeroWeight",
			rollout: &canaryRolloutIstioVsZeroWeight,
			rc:      rcTemp,
			result:  resultForCanaryWithStableServiceWeight,
		}, {
			name:    "canaryRolloutWithIstioRouteMatch",
			rollout: &canaryRolloutIstioVsRouteMatch,
			rc:      rcTemp,
			result:  resultForCanaryWithIstio,
		}, {
			name:    "canaryRolloutWithIstioRouteMisMatch",
			rollout: &canaryRolloutIstioVsRouteMisMatch,
			rc:      rcTemp,
			result:  resultForCanaryWithStableService,
		},
		{
			name:    "canaryRolloutWithRootServiceName",
			rollout: &canaryRolloutWithRootService,
			rc:      rcTemp,
			result:  resultForCanaryWithRootService,
		},
		{
			name:    "canaryRolloutWithOneServiceHavingMeshPort",
			rollout: &canaryRolloutWithStableServiceNS4,
			rc:      rcTemp,
			result:  resultRolloutWithOneServiceHavingMeshPort,
		},
		{
			name:    "canaryRolloutWithRootServiceNameMissing",
			rollout: &canaryRolloutWithoutRootService,
			rc:      rcTemp,
			result:  resultForCanaryWithoutRootService,
		},
		{
			name:    "canaryRolloutEmptyStrategy",
			rollout: &canaryRolloutNoStrategy,
			rc:      rcTemp,
			result:  nil,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getServiceForRollout(ctx, c.rc, c.rollout)
			if len(c.result) == 0 {
				if len(result) != 0 {
					t.Fatalf("Service expected to be nil")
				}
			} else {
				for key, wanted := range c.result {
					if got, ok := result[key]; ok {
						if !cmp.Equal(got.Service.Name, wanted.Service.Name) {
							t.Fatalf("Service Mismatch. Diff: %v", cmp.Diff(got.Service.Name, wanted.Service.Name))
						}
						if !cmp.Equal(got.Weight, wanted.Weight) {
							t.Fatalf("Service Weight Mismatch. Diff: %v", cmp.Diff(got.Weight, wanted.Weight))
						}
					} else {
						t.Fatalf("Expected a service with name=%s but none returned", key)
					}
				}
			}
		})
	}
}

func TestGetServiceForRolloutBlueGreen(t *testing.T) {
	//Struct of test case info. Name is required.
	const (
		namespace   = "namespace"
		serviceName = "serviceNameActive"

		generatedActiveServiceName        = "hello-" + common.RolloutActiveServiceSuffix
		rolloutPodHashLabel        string = "rollouts-pod-template-hash"
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
		bgRollout = argo.Rollout{
			Spec: argo.RolloutSpec{
				Selector: &labelSelector,
				Strategy: argo.RolloutStrategy{
					BlueGreen: &argo.BlueGreenStrategy{
						ActiveService:  serviceName,
						PreviewService: "previewService",
					},
				},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
				},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Namespace: namespace,
			},
		}
		bgRolloutNoActiveService = argo.Rollout{
			Spec: argo.RolloutSpec{
				Selector: &labelSelector,
				Strategy: argo.RolloutStrategy{
					BlueGreen: &argo.BlueGreenStrategy{},
				},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
				},
			},
			ObjectMeta: metaV1.ObjectMeta{
				Namespace: namespace,
			},
		}
		selectorMap = map[string]string{
			"app":               "test",
			rolloutPodHashLabel: "hash",
		}
		activeService = &coreV1.Service{
			Spec: coreV1.ServiceSpec{
				Selector: selectorMap,
			},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      serviceName,
				Namespace: namespace,
			},
		}
	)
	s, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("failed to initialize service controller, err: %v", err)
	}
	r, err := admiral.NewRolloutsController(stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("failed to initialize rollout controller, err: %v", err)
	}

	emptyCacheService, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("failed to initialize empty service controller, err: %v", err)
	}

	rc := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{},
		ServiceController:        s,
		RolloutController:        r,
	}

	port1 := coreV1.ServicePort{
		Port: 8080,
		Name: "random1",
	}

	port2 := coreV1.ServicePort{
		Port: 8081,
		Name: "random2",
	}

	ports := []coreV1.ServicePort{port1, port2}
	activeService.Spec.Ports = ports

	generatedActiveService := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	generatedActiveService.Name = generatedActiveServiceName
	generatedActiveService.Namespace = namespace
	generatedActiveService.Spec.Ports = ports

	selectorMap1 := make(map[string]string)
	selectorMap1["app"] = "test1"

	service1 := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	service1.Name = "dummy"
	service1.Namespace = namespace
	port3 := coreV1.ServicePort{
		Port: 8080,
		Name: "random3",
	}

	port4 := coreV1.ServicePort{
		Port: 8081,
		Name: "random4",
	}

	ports1 := []coreV1.ServicePort{port3, port4}
	service1.Spec.Ports = ports1

	selectorMap2 := make(map[string]string)
	selectorMap2["app"] = "test"
	selectorMap2[rolloutPodHashLabel] = "hash"
	previewService := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	previewService.Name = "previewService"
	previewService.Namespace = namespace
	port5 := coreV1.ServicePort{
		Port: 8080,
		Name: "random3",
	}

	port6 := coreV1.ServicePort{
		Port: 8081,
		Name: "random4",
	}

	ports2 := []coreV1.ServicePort{port5, port6}

	previewService.Spec.Ports = ports2

	serviceNS1 := &coreV1.Service{
		Spec: coreV1.ServiceSpec{
			Selector: selectorMap,
		},
	}
	serviceNS1.Name = "dummy"
	serviceNS1.Namespace = "namespace1"
	port8 := coreV1.ServicePort{
		Port: 8080,
		Name: "random3",
	}

	port9 := coreV1.ServicePort{
		Port: 8081,
		Name: "random4",
	}

	ports12 := []coreV1.ServicePort{port8, port9}
	serviceNS1.Spec.Ports = ports12

	rc.ServiceController.Cache.Put(service1)
	rc.ServiceController.Cache.Put(previewService)
	rc.ServiceController.Cache.Put(activeService)
	rc.ServiceController.Cache.Put(serviceNS1)
	rc.ServiceController.Cache.Put(generatedActiveService)

	noStratergyRollout := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}
	noStratergyRollout.Namespace = namespace

	noStratergyRollout.Spec.Strategy = argo.RolloutStrategy{}

	bgRolloutNs1 := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{}},
		}}}

	matchLabel1 := make(map[string]string)
	matchLabel1["app"] = "test"

	labelSelector1 := metaV1.LabelSelector{
		MatchLabels: matchLabel,
	}
	bgRolloutNs1.Spec.Selector = &labelSelector1

	bgRolloutNs1.Namespace = "namespace1"
	bgRolloutNs1.Spec.Strategy = argo.RolloutStrategy{
		BlueGreen: &argo.BlueGreenStrategy{
			ActiveService:  serviceName,
			PreviewService: "previewService",
		},
	}

	resultForBlueGreen := map[string]*WeightedService{serviceName: {Weight: 1, Service: activeService}}
	resultForNoActiveService := map[string]*WeightedService{generatedActiveServiceName: {Weight: 1, Service: generatedActiveService}}

	testCases := []struct {
		name    string
		rollout *argo.Rollout
		rc      *RemoteController
		result  map[string]*WeightedService
	}{
		{
			name:    "canaryRolloutNoLabelMatch",
			rollout: &bgRolloutNs1,
			rc:      rc,
			result:  make(map[string]*WeightedService, 0),
		}, {
			name:    "canaryRolloutNoStratergy",
			rollout: &noStratergyRollout,
			rc:      rc,
			result:  make(map[string]*WeightedService, 0),
		}, {
			name:    "canaryRolloutHappyCase",
			rollout: &bgRollout,
			rc:      rc,
			result:  resultForBlueGreen,
		}, {
			name:    "rolloutWithNoActiveService",
			rollout: &bgRolloutNoActiveService,
			rc:      rc,
			result:  resultForNoActiveService,
		},
		{
			name:    "canaryRolloutNilRollout",
			rollout: nil,
			rc:      rc,
			result:  make(map[string]*WeightedService, 0),
		},
		{
			name:    "canaryRolloutEmptyServiceCache",
			rollout: &bgRollout,
			rc: &RemoteController{
				ServiceController: emptyCacheService,
			},
			result: make(map[string]*WeightedService, 0),
		},
	}

	ctx := context.Background()

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getServiceForRollout(ctx, c.rc, c.rollout)
			if len(c.result) == 0 {
				if len(result) > 0 {
					t.Fatalf("Service expected to be nil")
				}
			} else {
				for key, service := range c.result {
					if val, ok := result[key]; ok {
						if !cmp.Equal(val.Service.Name, service.Service.Name) {
							t.Fatalf("Service Mismatch. Diff: %v", cmp.Diff(val.Service.Name, service.Service.Name))
						}
					} else {
						t.Fatalf("Expected a service with name=%s but none returned", key)
					}
				}
			}
		})
	}
}

func makeRemoteRegistry(
	clusterNames []string, remoteController *RemoteController, cname string, dependentClusters []string) *RemoteRegistry {
	var (
		cache = common.NewMapOfMaps()
		rr    = NewRemoteRegistry(context.TODO(), common.AdmiralParams{})
	)
	rr.AdmiralCache = &AdmiralCache{
		CnameDependentClusterCache: cache,
	}
	for _, dependentCluster := range dependentClusters {
		rr.AdmiralCache.CnameDependentClusterCache.Put(cname, dependentCluster, dependentCluster)
	}
	for _, clusterName := range clusterNames {
		rr.PutRemoteController(
			clusterName,
			remoteController,
		)
	}

	return rr
}
