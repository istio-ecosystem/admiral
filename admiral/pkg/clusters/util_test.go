package clusters

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/sirupsen/logrus"
	istioNetworkingV1Alpha3 "istio.io/api/networking/v1alpha3"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	"k8s.io/client-go/rest"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	coreV1 "k8s.io/api/core/v1"
	k8sV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

	service := k8sV1.Service{
		ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
		Spec:       k8sV1.ServiceSpec{Ports: serviceMeshPorts},
	}
	rollout := argo.Rollout{
		Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{common.SidecarEnabledPorts: strconv.Itoa(annotatedPort)}},
		}}}

	ports := map[string]uint32{"http": uint32(annotatedPort)}

	portsFromDefaultSvcPort := map[string]uint32{"http": defaultServicePort}

	emptyPorts := map[string]uint32{}

	testCases := []struct {
		name        string
		clusterName string
		service     k8sV1.Service
		rollout     argo.Rollout
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
			service: k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: serviceMeshPortsOnlyDefault},
			},
			rollout: argo.Rollout{
				Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}},
				}}},
			expected: portsFromDefaultSvcPort,
		},
		{
			name: "should return empty ports",
			service: k8sV1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sV1.ServiceSpec{Ports: nil},
			},
			rollout: argo.Rollout{
				Spec: argo.RolloutSpec{Template: coreV1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}},
				}}},
			expected: emptyPorts,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			meshPorts := GetMeshPortsForRollout(c.clusterName, &c.service, &c.rollout)
			if !reflect.DeepEqual(meshPorts, c.expected) {
				t.Errorf("Wanted meshPorts: %v, got: %v", c.expected, meshPorts)
			}
		})
	}
}

func TestRemoveSeEndpoints(t *testing.T) {
	clusterName := "clusterForWhichEventWasSent"
	differentClusterName := "notSameClusterForWhichEventWasSent"
	testCases := []struct {
		name                     string
		event                    admiral.EventType
		clusterId                string
		deployToRolloutMigration bool
		appType                  string
		clusterAppDeleteMap      map[string]string
		expectedEvent            admiral.EventType
		expectedDeleteCluster    bool
	}{
		{
			name: "Given a delete event is received," +
				"And we are currently processing for the same cluster," +
				"Then we should return a delete event and true for deleting the endpoints for the cluster",
			event:                    admiral.Delete,
			clusterId:                clusterName,
			deployToRolloutMigration: false,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Delete,
			expectedDeleteCluster:    true,
		},
		{
			name: "Given a delete event is received," +
				"And we are currently processing for a different cluster," +
				"Then we should return a update event and false for deleting the endpoints for the cluster",
			event:                    admiral.Delete,
			clusterId:                differentClusterName,
			deployToRolloutMigration: false,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Update,
			expectedDeleteCluster:    false,
		},
		{
			name: "Given a add event is received," +
				"And we are currently processing for a different cluster," +
				"Then we should return a add event and false for deleting the endpoints for the cluster",
			event:                    admiral.Add,
			clusterId:                differentClusterName,
			deployToRolloutMigration: false,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Add,
			expectedDeleteCluster:    false,
		},
		{
			name: "Given a update event is received," +
				"And we are currently processing for a different cluster," +
				"Then we should return a update event and false for deleting the endpoints for the cluster",
			event:                    admiral.Update,
			clusterId:                differentClusterName,
			deployToRolloutMigration: false,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Update,
			expectedDeleteCluster:    false,
		},
		{
			name: "Given a add event is received," +
				"And we are currently processing for the same cluster," +
				"Then we should return a add event and false for deleting the endpoints for the cluster",
			event:                    admiral.Add,
			clusterId:                clusterName,
			deployToRolloutMigration: false,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Add,
			expectedDeleteCluster:    false,
		},
		{
			name: "Given a update event is received," +
				"And we are currently processing for the same cluster," +
				"Then we should return a update event and false for deleting the endpoints for the cluster",
			event:                    admiral.Update,
			clusterId:                clusterName,
			deployToRolloutMigration: false,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Update,
			expectedDeleteCluster:    false,
		},
		{
			name: "Given a add event is received," +
				"And we are currently processing for the same cluster," +
				"And an application is being migrated from deployment to rollout," +
				"Then we should return a add event and false for deleting the endpoints for the cluster",
			event:                    admiral.Add,
			clusterId:                clusterName,
			deployToRolloutMigration: true,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Add,
			expectedDeleteCluster:    false,
		},
		{
			name: "Given a update event is received," +
				"And we are currently processing for the same cluster," +
				"And an application is being migrated from deployment to rollout," +
				"Then we should return a update event and false for deleting the endpoints for the cluster",
			event:                    admiral.Update,
			clusterId:                clusterName,
			deployToRolloutMigration: true,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Update,
			expectedDeleteCluster:    false,
		},
		{
			name: "Given a delete event is received," +
				"And we are currently processing for the same cluster," +
				"And an application is being migrated from deployment to rollout," +
				"And an application is of deployment type," +
				"Then we should return a delete event and true for deleting the endpoints for the cluster",
			event:                    admiral.Delete,
			clusterId:                clusterName,
			deployToRolloutMigration: true,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      map[string]string{clusterName: common.Deployment},
			expectedEvent:            admiral.Delete,
			expectedDeleteCluster:    true,
		},
		{
			name: "Given a delete event is received," +
				"And we are currently processing for the same cluster," +
				"And an application is being migrated from deployment to rollout," +
				"And an application is of rollout type," +
				"Then we should return a update event and true for deleting the endpoints for the cluster",
			event:                    admiral.Delete,
			clusterId:                clusterName,
			deployToRolloutMigration: true,
			appType:                  common.Rollout,
			clusterAppDeleteMap:      map[string]string{clusterName: common.Deployment},
			expectedEvent:            admiral.Update,
			expectedDeleteCluster:    true,
		},
		{
			name: "Given a delete event is received," +
				"And we are currently processing for the same cluster," +
				"And an application is being migrated from rollout to deployment," +
				"And an application is of rollout type," +
				"Then we should return a update event and true for deleting the endpoints for the cluster",
			event:                    admiral.Delete,
			clusterId:                clusterName,
			deployToRolloutMigration: true,
			appType:                  common.Rollout,
			clusterAppDeleteMap:      map[string]string{clusterName: common.Rollout},
			expectedEvent:            admiral.Delete,
			expectedDeleteCluster:    true,
		},
		{
			name: "Given a add event is received," +
				"And we are currently processing for a different cluster," +
				"And an application is being migrated from deployment to rollout," +
				"Then we should return a add event and false for deleting the endpoints for the cluster",
			event:                    admiral.Add,
			clusterId:                differentClusterName,
			deployToRolloutMigration: true,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Add,
			expectedDeleteCluster:    false,
		},
		{
			name: "Given a update event is received," +
				"And we are currently processing for a different cluster," +
				"And an application is being migrated from deployment to rollout," +
				"Then we should return a update event and false for deleting the endpoints for the cluster",
			event:                    admiral.Update,
			clusterId:                differentClusterName,
			deployToRolloutMigration: false,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Update,
			expectedDeleteCluster:    false,
		},
		{
			name: "Given a delete event is received," +
				"And we are currently processing for a different cluster," +
				"And an application is being migrated from deployment to rollout," +
				"And an application is of deployment type," +
				"Then we should return a delete event and false for deleting the endpoints for the cluster",
			event:                    admiral.Delete,
			clusterId:                differentClusterName,
			deployToRolloutMigration: true,
			appType:                  common.Deployment,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Update,
			expectedDeleteCluster:    false,
		},
		{
			name: "Given a delete event is received," +
				"And we are currently processing for a different cluster," +
				"And an application is being migrated from deployment to rollout," +
				"And an application is of rollout type," +
				"Then we should return a delete event and false for deleting the endpoints for the cluster",
			event:                    admiral.Delete,
			clusterId:                differentClusterName,
			deployToRolloutMigration: true,
			appType:                  common.Rollout,
			clusterAppDeleteMap:      nil,
			expectedEvent:            admiral.Update,
			expectedDeleteCluster:    false,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			eventType, deleteCluster := removeSeEndpoints(clusterName, c.event, c.clusterId, c.deployToRolloutMigration, c.appType, c.clusterAppDeleteMap)
			if !reflect.DeepEqual(eventType, c.expectedEvent) {
				t.Errorf("wanted eventType: %v, got: %v", c.expectedEvent, eventType)
			}

			if !reflect.DeepEqual(deleteCluster, c.expectedDeleteCluster) {
				t.Errorf("wanted deleteCluster: %v, got: %v", c.expectedDeleteCluster, deleteCluster)
			}
		})
	}
}

func TestGetAllServicesForRollout(t *testing.T) {

	setupForServiceEntryTests()
	config := rest.Config{
		Host: "localhost",
	}

	stop := make(chan struct{})
	s, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if e != nil {
		t.Fatalf("%v", e)
	}
	sWithNolabels, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if e != nil {
		t.Fatalf("%v", e)
	}
	sWithRootLabels, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if e != nil {
		t.Fatalf("%v", e)
	}
	sWithNoService, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if e != nil {
		t.Fatalf("%v", e)
	}
	sWithRootMeshPorts, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	if e != nil {
		t.Fatalf("%v", e)
	}

	admiralCache := AdmiralCache{}

	cacheWithNoEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses:      []string{},
	}

	localAddress := common.LocalAddressPrefix + ".10.1"

	cnameIdentityCache := sync.Map{}
	cnameIdentityCache.Store("dev.bar.global", "bar")
	admiralCache.CnameIdentityCache = &cnameIdentityCache

	admiralCache.ServiceEntryAddressStore = &ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.my-first-service.mesh-se": localAddress},
		Addresses:      []string{localAddress},
	}

	admiralCache.CnameClusterCache = common.NewMapOfMaps()

	fakeIstioClient := istiofake.NewSimpleClientset()

	cacheWithEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses:      []string{},
	}

	cacheController := &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	admiralCache.ConfigMapController = cacheController
	admiralCache.ServiceEntryAddressStore = &cacheWithNoEntry

	rc := generateRC(fakeIstioClient, s)
	rcWithNolabels := generateRC(fakeIstioClient, sWithNolabels)
	rcWithOnlyRootLabels := generateRC(fakeIstioClient, sWithRootLabels)
	rcWithNoService := generateRC(fakeIstioClient, sWithNoService)
	rcWithRootMeshPorts := generateRC(fakeIstioClient, sWithRootMeshPorts)

	serviceRootForRollout := generateService("rootservice", "test-ns", map[string]string{"app": "test"}, 8090)
	serviceStableForRollout := generateService("stableservice", "test-ns", map[string]string{"app": "test"}, 8090)
	serviceStableForRolloutNoLabels := generateService("stableservicenolabels", "test-ns", map[string]string{}, 8090)
	serviceRootForRolloutNoLabels := generateService("rootservicenolabels", "test-ns", map[string]string{}, 8090)
	serviceStableForRolloutNoPorts := generateService("stablenoports", "test-ns", map[string]string{}, 1024)

	s.Cache.Put(serviceRootForRollout)
	s.Cache.Put(serviceStableForRollout)

	sWithRootLabels.Cache.Put(serviceStableForRolloutNoLabels)
	sWithRootLabels.Cache.Put(serviceRootForRollout)

	sWithNolabels.Cache.Put(serviceStableForRolloutNoLabels)
	sWithNolabels.Cache.Put(serviceRootForRolloutNoLabels)

	sWithRootMeshPorts.Cache.Put(serviceRootForRollout)
	sWithRootMeshPorts.Cache.Put(serviceStableForRolloutNoPorts)

	selector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "test"},
	}

	selectorEmpty := metav1.LabelSelector{
		MatchLabels: map[string]string{},
	}

	testRollout := createTestRollout(selector, "stableservice", "rootservice")
	testRolloutEmpty := createTestRollout(selectorEmpty, "stableservice", "rootservice")

	testCases := []struct {
		name                 string
		rc                   *RemoteController
		rollout              *argo.Rollout
		expectedServiceArray []string
	}{
		{
			name: "Should return root and stable services, " +
				"given the root and stable services match the rollout label spec and have mesh ports",
			rc:                   rc,
			rollout:              &testRollout,
			expectedServiceArray: []string{"stableservice", "rootservice"},
		},
		{
			name: "Should return root service " +
				"given root and stable services are present, only root matches rollout labels",
			rc:                   rcWithOnlyRootLabels,
			rollout:              &testRollout,
			expectedServiceArray: []string{"rootservice"},
		},
		{
			name: "Should return root service " +
				"given root and stable services are present, only root has mesh ports",
			rc:                   rcWithRootMeshPorts,
			rollout:              &testRollout,
			expectedServiceArray: []string{"rootservice"},
		},
		{
			name: "Should return no service " +
				"given root and stable services are present, no labels are matching rollout",
			rc:                   rcWithNolabels,
			rollout:              &testRollout,
			expectedServiceArray: []string{},
		},
		{
			name: "Should return no service " +
				"given no service is present in cache",
			rc:                   rcWithNoService,
			rollout:              &testRollout,
			expectedServiceArray: []string{},
		},
		{
			name: "Should return no service " +
				"given rollout is nil",
			rc:                   rc,
			rollout:              nil,
			expectedServiceArray: []string{},
		},
		{
			name: "Should return root service " +
				"given rollout does not have selector",
			rc:                   rc,
			rollout:              &testRolloutEmpty,
			expectedServiceArray: []string{},
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {

			serviceMap := GetAllServicesForRollout(c.rc, c.rollout)

			for _, key := range c.expectedServiceArray {
				if serviceMap[key] == nil {
					t.Errorf("Test %s failed, expected: %v got %v", c.name, c.expectedServiceArray, serviceMap)
				}
			}

			if len(c.expectedServiceArray) != len(serviceMap) {
				t.Errorf("Test %s failed, expected length: %v got %v", c.name, len(c.expectedServiceArray), len(serviceMap))
			}

		})
	}

}

func TestGenerateServiceEntryForCanary(t *testing.T) {
	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})
	setupForServiceEntryTests()
	ctx := context.Background()
	config := rest.Config{
		Host: "localhost",
	}

	stop := make(chan struct{})
	s, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())

	if e != nil {
		t.Fatalf("%v", e)
	}

	admiralCache := AdmiralCache{}

	cacheWithNoEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses:      []string{},
	}

	localAddress := common.LocalAddressPrefix + ".10.1"

	cnameIdentityCache := sync.Map{}
	cnameIdentityCache.Store("dev.bar.global", "bar")
	admiralCache.CnameIdentityCache = &cnameIdentityCache

	admiralCache.ServiceEntryAddressStore = &ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"e2e.my-first-service.mesh-se": localAddress},
		Addresses:      []string{localAddress},
	}

	admiralCache.CnameClusterCache = common.NewMapOfMaps()

	fakeIstioClient := istiofake.NewSimpleClientset()

	cacheWithEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{},
		Addresses:      []string{},
	}

	cacheController := &test.FakeConfigMapController{
		GetError:          nil,
		PutError:          nil,
		ConfigmapToReturn: buildFakeConfigMapFromAddressStore(&cacheWithEntry, "123"),
	}

	admiralCache.ConfigMapController = cacheController
	admiralCache.ServiceEntryAddressStore = &cacheWithNoEntry

	rc := generateRC(fakeIstioClient, s)

	serviceForRollout := generateService("stableservice", "test-ns", map[string]string{"app": "test"}, 8090)
	serviceCanaryForRollout := generateService("canaryservice", "test-ns", map[string]string{"app": "test"}, 8090)

	s.Cache.Put(serviceForRollout)
	s.Cache.Put(serviceCanaryForRollout)

	vsRoutes := []*istioNetworkingV1Alpha3.HTTPRouteDestination{
		{
			Destination: &istioNetworkingV1Alpha3.Destination{
				Host: "canaryservice",
				Port: &istioNetworkingV1Alpha3.PortSelector{
					Number: common.DefaultServiceEntryPort,
				},
			},
			Weight: 30,
		},
		{
			Destination: &istioNetworkingV1Alpha3.Destination{
				Host: "stableservice",
				Port: &istioNetworkingV1Alpha3.PortSelector{
					Number: common.DefaultServiceEntryPort,
				},
			},
			Weight: 70,
		},
	}

	fooVS := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "virtualservice",
			Labels: map[string]string{"admiral.io/env": "e2e", "identity": "my-first-service"},
		},
		Spec: istioNetworkingV1Alpha3.VirtualService{
			Hosts: []string{"stage.test00.foo"},
			Http: []*istioNetworkingV1Alpha3.HTTPRoute{
				{
					Route: vsRoutes,
				},
			},
		},
	}

	_, err := fakeIstioClient.NetworkingV1alpha3().VirtualServices("test-ns").Create(ctx, fooVS, metav1.CreateOptions{})
	if err != nil {
		t.Error(err)
	}
	selector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "test"},
	}

	workloadIdentityKey := "identity"
	rolloutSeCreationTestCases := []struct {
		name           string
		rc             *RemoteController
		rollout        argo.Rollout
		expectedResult int
	}{
		{
			name: "Should return a created service entry for canary, " +
				"given the 2 services exist and the VS has reference to both services",
			rc:             rc,
			rollout:        createTestRollout(selector, "stableservice", "canaryservice"),
			expectedResult: 1,
		},
		{
			name: "Should not create service entry for canary, " +
				"given both services exist and VS has reference to only canary ",
			rc:             rc,
			rollout:        createTestRollout(selector, "", "canaryservice"),
			expectedResult: 1,
		},
		{
			name: "Should not create service entry for stable, " +
				"given both services exist and VS has reference to only stable ",
			rc:             rc,
			rollout:        createTestRollout(selector, "stableservice", ""),
			expectedResult: 0,
		},
		{
			name: "Should not return created service entry for stable, " +
				"given only stable service exists and VS has reference to both ",
			rc:             rc,
			rollout:        createTestRollout(selector, "stableservice", "canaryservice2"),
			expectedResult: 0,
		},
		{
			name:           "Should not return SE, both services are missing",
			rc:             rc,
			rollout:        createTestRollout(selector, "stableservice2", "canaryservice2"),
			expectedResult: 0,
		},
		{
			name:           "Should not return SE, reference in VS are missing",
			rc:             rc,
			rollout:        createTestRollout(selector, "", ""),
			expectedResult: 0,
		},
		{
			name:           "Should not return SE, canary strategy is nil",
			rc:             rc,
			rollout:        createTestRollout(selector, "", ""),
			expectedResult: 0,
		},
	}

	//Run the test for every provided case
	for _, c := range rolloutSeCreationTestCases {
		t.Run(c.name, func(t *testing.T) {
			se := map[string]*istioNetworkingV1Alpha3.ServiceEntry{}
			san := getSanForRollout(&c.rollout, workloadIdentityKey)
			err := GenerateServiceEntryForCanary(ctxLogger, ctx, admiral.Add, rc, &admiralCache, map[string]uint32{"http": uint32(80)}, &c.rollout, se, workloadIdentityKey, san, "")
			if err != nil || len(se) != c.expectedResult {
				t.Errorf("Test %s failed, expected: %v got %v", c.name, c.expectedResult, len(se))
			}

		})
	}

}

func TestIsIstioCanaryStrategy(t *testing.T) {
	var (
		emptyRollout                 *argo.Rollout
		rolloutWithBlueGreenStrategy = &argo.Rollout{
			Spec: argo.RolloutSpec{
				Strategy: argo.RolloutStrategy{
					BlueGreen: &argo.BlueGreenStrategy{
						ActiveService: "active",
					},
				},
			},
		}
		rolloutWithSimpleCanaryStrategy = &argo.Rollout{
			Spec: argo.RolloutSpec{
				Strategy: argo.RolloutStrategy{
					Canary: &argo.CanaryStrategy{
						CanaryService: "canaryservice",
					},
				},
			},
		}
		rolloutWithIstioCanaryStrategy = &argo.Rollout{
			Spec: argo.RolloutSpec{
				Strategy: argo.RolloutStrategy{
					Canary: &argo.CanaryStrategy{
						CanaryService: "canaryservice",
						StableService: "stableservice",
						TrafficRouting: &argo.RolloutTrafficRouting{
							Istio: &argo.IstioTrafficRouting{
								VirtualService: &argo.IstioVirtualService{Name: "virtualservice"},
							},
						},
					},
				},
			},
		}
		rolloutWithNoStrategy = &argo.Rollout{
			Spec: argo.RolloutSpec{},
		}
		rolloutWithEmptySpec = &argo.Rollout{}
	)
	cases := []struct {
		name           string
		rollout        *argo.Rollout
		expectedResult bool
	}{
		{
			name: "Given argo rollout is configured with blue green rollout strategy" +
				"When isCanaryIstioStrategy is called" +
				"Then it should return false",
			rollout:        rolloutWithBlueGreenStrategy,
			expectedResult: false,
		},
		{
			name: "Given argo rollout is configured with canary rollout strategy" +
				"When isCanaryIstioStrategy is called" +
				"Then it should return false",
			rollout:        rolloutWithSimpleCanaryStrategy,
			expectedResult: false,
		},
		{
			name: "Given argo rollout is configured with istio canary rollout strategy" +
				"When isCanaryIstioStrategy is called" +
				"Then it should return true",
			rollout:        rolloutWithIstioCanaryStrategy,
			expectedResult: true,
		},
		{
			name: "Given argo rollout is configured without any rollout strategy" +
				"When isCanaryIstioStrategy is called" +
				"Then it should return false",
			rollout:        rolloutWithNoStrategy,
			expectedResult: false,
		},
		{
			name: "Given argo rollout is nil" +
				"When isCanaryIstioStrategy is called" +
				"Then it should return false",
			rollout:        emptyRollout,
			expectedResult: false,
		},
		{
			name: "Given argo rollout has an empty Spec" +
				"When isCanaryIstioStrategy is called" +
				"Then it should return false",
			rollout:        rolloutWithEmptySpec,
			expectedResult: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := IsCanaryIstioStrategy(c.rollout)
			if result != c.expectedResult {
				t.Errorf("expected: %t, got: %t", c.expectedResult, result)
			}
		})
	}
}

func generateSEGivenIdentity(deployment1Identity string) *istioNetworkingV1Alpha3.ServiceEntry {
	return &istioNetworkingV1Alpha3.ServiceEntry{
		Hosts:     []string{"test." + deployment1Identity + ".mesh"},
		Addresses: []string{"127.0.0.1"},
		Ports: []*istioNetworkingV1Alpha3.ServicePort{
			{
				Number:   80,
				Protocol: "http",
				Name:     "http",
			},
		},
		Location:   istioNetworkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution: istioNetworkingV1Alpha3.ServiceEntry_DNS,
		Endpoints: []*istioNetworkingV1Alpha3.WorkloadEntry{
			&istioNetworkingV1Alpha3.WorkloadEntry{
				Address: "dummy.admiral.global",
				Ports: map[string]uint32{
					"http": 0,
				},
				Locality: "us-west-2",
				Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
			},
		},
		SubjectAltNames: []string{"spiffe://prefix/" + deployment1Identity},
	}

}

func TestFilterClusters(t *testing.T) {
	var (
		sourceClusters               = common.NewMap()
		destinationClusters          = common.NewMap()
		destinationAllCommonClusters = common.NewMap()
		destinationMoreClusters      = common.NewMap()
		destinationNoCommonClusters  = common.NewMap()
	)
	sourceClusters.Put("A", "A")
	sourceClusters.Put("B", "B")
	destinationClusters.Put("A", "A")
	destinationAllCommonClusters.Put("A", "A")
	destinationAllCommonClusters.Put("B", "B")
	destinationMoreClusters.Put("A", "A")
	destinationMoreClusters.Put("B", "B")
	destinationMoreClusters.Put("C", "C")
	destinationNoCommonClusters.Put("E", "E")

	cases := []struct {
		name                string
		sourceClusters      *common.Map
		destinationClusters *common.Map
		expectedResult      map[string]string
	}{
		{
			name: "Given sourceClusters and destinationClusters" +
				"When there are common clusters between the two" +
				"Then it should only the clusters where the source is but not the destination",
			sourceClusters:      sourceClusters,
			destinationClusters: destinationClusters,
			expectedResult:      map[string]string{"B": "B"},
		},
		{
			name: "Given sourceClusters and destinationClusters" +
				"When all the cluster are common" +
				"Then it should return an empty map",
			sourceClusters:      sourceClusters,
			destinationClusters: destinationAllCommonClusters,
			expectedResult:      map[string]string{},
		},
		{
			name: "Given sourceClusters and destinationClusters" +
				"When all the cluster are common and destination has more clusters" +
				"Then it should return an empty map",
			sourceClusters:      sourceClusters,
			destinationClusters: destinationMoreClusters,
			expectedResult:      map[string]string{},
		},
		{
			name: "Given sourceClusters and destinationClusters" +
				"When no cluster are common" +
				"Then it should return all the clusters in the sourceClusters",
			sourceClusters:      sourceClusters,
			destinationClusters: destinationNoCommonClusters,
			expectedResult:      map[string]string{"A": "A", "B": "B"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := filterClusters(c.sourceClusters, c.destinationClusters)
			if !reflect.DeepEqual(result.Copy(), c.expectedResult) {
				t.Errorf("expected: %v, got: %v", c.expectedResult, result)
			}
		})
	}
}

func TestGetSortedDependentNamespaces(t *testing.T) {
	admiralParams := common.GetAdmiralParams()
	admiralParams.EnableSWAwareNSCaches = true
	admiralParams.ExportToIdentityList = []string{"*"}
	admiralParams.ExportToMaxNamespaces = 35
	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	emptynscache := common.NewMapOfMapOfMaps()
	emptycnameidcache := &sync.Map{}
	emptyidclustercache := common.NewMapOfMaps()
	cndepclusternscache := common.NewMapOfMapOfMaps()
	clusternscache := common.NewMapOfMaps()
	clusternscache.PutMap("cluster1", nil)
	cndepclusternscache.PutMapofMaps("cname", clusternscache)
	cndepclusternscache.Put("cname", "cluster2", "ns1", "ns1")
	cndepclusternscache.Put("cname", "cluster2", "ns2", "ns2")
	cndepclusternscache.Put("cname", "cluster3", "ns3", "ns3")
	cndepclusternscache.Put("cname", "cluster3", "ns4", "ns4")
	cndepclusternscache.Put("cname", "cluster3", "ns5", "ns5")
	cndepclusternscache.Put("cname", "cluster4", "ns6", "ns6")
	cndepclusternscache.Put("cname", "cluster4", "ns7", "ns7")
	cndepclusternscache.Put("cname", "cluster5", "ns1", "ns1")
	cndepclusternscache.Put("cname", "cluster5", "ns2", "ns2")
	cndepclusternscache.Put("cname", "cluster5", "istio-system", "istio-system")
	for i := range [35]int{} {
		nshash := "ns" + strconv.Itoa(i+3)
		cndepclusternscache.Put("cname", "cluster3", nshash, nshash)
	}
	idclustercache := common.NewMapOfMaps()
	idclustercache.Put("cnameid", "cluster1", "cluster1")
	idclustercache.Put("cnameid", "cluster2", "cluster2")
	idclustercache.Put("cnameid", "cluster3", "cluster3")
	cnameidcache := &sync.Map{}
	cnameidcache.Store("cname", "cnameid")
	var nilSlice []string
	cases := []struct {
		name                                string
		identityClusterCache                *common.MapOfMaps
		cnameIdentityCache                  *sync.Map
		cnameDependentClusterNamespaceCache *common.MapOfMapOfMaps
		cname                               string
		clusterId                           string
		skipIstioNSFromExportTo             bool
		expectedResult                      []string
	}{
		{
			name: "Given CnameDependentClusterNamespaceCache is nil " +
				"Then we should return nil slice",
			identityClusterCache:                nil,
			cnameIdentityCache:                  nil,
			cnameDependentClusterNamespaceCache: nil,
			cname:                               "cname",
			clusterId:                           "fake-cluster",
			expectedResult:                      nilSlice,
		},
		{
			name: "Given CnameDependentClusterNamespaceCache is filled and CnameIdentityCache is nil " +
				"Then we should return the dependent namespaces without istio-system",
			identityClusterCache:                nil,
			cnameIdentityCache:                  nil,
			cnameDependentClusterNamespaceCache: cndepclusternscache,
			cname:                               "cname",
			clusterId:                           "cluster2",
			expectedResult:                      []string{"ns1", "ns2"},
		},
		{
			name: "Given CnameDependentClusterNamespaceCache and CnameIdentityCache are filled but IdentityClusterCache is nil " +
				"Then we should return the dependent namespaces without istio-system",
			identityClusterCache:                nil,
			cnameIdentityCache:                  cnameidcache,
			cnameDependentClusterNamespaceCache: cndepclusternscache,
			cname:                               "cname",
			clusterId:                           "cluster2",
			expectedResult:                      []string{"ns1", "ns2"},
		},
		{
			name: "Given CnameDependentClusterNamespaceCache has no entries for the cname " +
				"Then we should return nil slice",
			identityClusterCache:                nil,
			cnameIdentityCache:                  nil,
			cnameDependentClusterNamespaceCache: emptynscache,
			cname:                               "cname-none",
			clusterId:                           "fake-cluster",
			expectedResult:                      nilSlice,
		},
		{
			name: "Given CnameDependentClusterNamespaceCache has no namespace entries for the cname and cluster " +
				"Then we should return nil slice",
			identityClusterCache:                nil,
			cnameIdentityCache:                  nil,
			cnameDependentClusterNamespaceCache: cndepclusternscache,
			cname:                               "cname",
			clusterId:                           "cluster1",
			expectedResult:                      nilSlice,
		},
		{
			name: "Given CnameDependentClusterNamespaceCache is filled and CnameIdentityCache has no entries for the cname" +
				"Then we should return the dependent namespaces without istio-system",
			identityClusterCache:                nil,
			cnameIdentityCache:                  emptycnameidcache,
			cnameDependentClusterNamespaceCache: cndepclusternscache,
			cname:                               "cname",
			clusterId:                           "cluster2",
			expectedResult:                      []string{"ns1", "ns2"},
		},
		{
			name: "Given CnameDependentClusterNamespaceCache and CnameIdentityCache are filled but IdentityClusterCache has no entries for the identity " +
				"Then we should return the dependent namespaces without istio-system",
			identityClusterCache:                emptyidclustercache,
			cnameIdentityCache:                  cnameidcache,
			cnameDependentClusterNamespaceCache: cndepclusternscache,
			cname:                               "cname",
			clusterId:                           "cluster2",
			expectedResult:                      []string{"ns1", "ns2"},
		},
		{
			name: "Given the cname has dependent cluster namespaces but no dependents are in the source cluster " +
				"Then we should return a sorted slice of the dependent cluster namespaces",
			identityClusterCache:                idclustercache,
			cnameIdentityCache:                  cnameidcache,
			cnameDependentClusterNamespaceCache: cndepclusternscache,
			cname:                               "cname",
			clusterId:                           "cluster4",
			expectedResult:                      []string{"ns6", "ns7"},
		},
		{
			name: "Given the cname has dependent cluster namespaces and some dependents in the source cluster " +
				"Then we should return a sorted slice of the dependent cluster namespaces including istio-system",
			identityClusterCache:                idclustercache,
			cnameIdentityCache:                  cnameidcache,
			cnameDependentClusterNamespaceCache: cndepclusternscache,
			cname:                               "cname",
			clusterId:                           "cluster2",
			expectedResult:                      []string{"istio-system", "ns1", "ns2"},
		},
		{
			name: "Given the cname has more dependent cluster namespaces than the maximum " +
				"Then we should return a slice containing *",
			identityClusterCache:                idclustercache,
			cnameIdentityCache:                  cnameidcache,
			cnameDependentClusterNamespaceCache: cndepclusternscache,
			cname:                               "cname",
			clusterId:                           "cluster3",
			expectedResult:                      []string{"*"},
		},
		{
			name: "Given the cname has dependent cluster namespaces and some dependents in the source cluster " +
				"And we skip adding istio-system NS to the exportTo list" +
				"Then we should return a sorted slice of the dependent cluster namespaces excluding istio-system",
			identityClusterCache:                idclustercache,
			cnameIdentityCache:                  cnameidcache,
			cnameDependentClusterNamespaceCache: cndepclusternscache,
			cname:                               "cname",
			clusterId:                           "cluster2",
			skipIstioNSFromExportTo:             true,
			expectedResult:                      []string{"ns1", "ns2"},
		},
		{
			name: "Given the cname has dependent cluster namespaces and some dependents in the source cluster " +
				"And there is a dependent in istio-system NS" +
				"When skipIstioNSFromExportTo is true" +
				"Then we should return a sorted slice of the dependent cluster namespaces excluding istio-system",
			identityClusterCache:                idclustercache,
			cnameIdentityCache:                  cnameidcache,
			cnameDependentClusterNamespaceCache: cndepclusternscache,
			cname:                               "cname",
			clusterId:                           "cluster5",
			skipIstioNSFromExportTo:             true,
			expectedResult:                      []string{"ns1", "ns2"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			admiralCache := &AdmiralCache{}
			admiralCache.IdentityClusterCache = c.identityClusterCache
			admiralCache.CnameIdentityCache = c.cnameIdentityCache
			admiralCache.CnameDependentClusterNamespaceCache = c.cnameDependentClusterNamespaceCache
			result := getSortedDependentNamespaces(
				admiralCache, c.cname, c.clusterId, ctxLogger, c.skipIstioNSFromExportTo)
			if !reflect.DeepEqual(result, c.expectedResult) {
				t.Errorf("expected: %v, got: %v", c.expectedResult, result)
			}
		})
	}
}

func TestGetDestinationsToBeProcessedForClientInitiatedProcessing(t *testing.T) {
	identityClusterCache := common.NewMapOfMaps()
	identityClusterCache.Put("foo", "cluster1", "cluster1")
	identityClusterCache.Put("bar", "cluster2", "cluster2")
	testCases := []struct {
		name                 string
		remoteRegistry       *RemoteRegistry
		globalIdentifier     string
		clusterName          string
		clientNs             string
		initialDestinations  []string
		expectedDestinations []string
	}{
		{
			name: "When global identifier does not exist, return nil",
			remoteRegistry: &RemoteRegistry{
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: make(map[string][]string),
						mutex:              &sync.Mutex{},
					},
					ClientClusterNamespaceServerCache: common.NewMapOfMapOfMaps(),
					IdentityClusterCache:              identityClusterCache,
				},
			},
			globalIdentifier:     "doesNotExist",
			clusterName:          "cluster1",
			clientNs:             "clientNs",
			initialDestinations:  []string{},
			expectedDestinations: []string{},
		},
		{
			name: "When processed client clusters are empty, process all actual server identities",
			remoteRegistry: &RemoteRegistry{
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"asset1": {"foo", "bar"}},
						mutex:              &sync.Mutex{},
					},
					ClientClusterNamespaceServerCache: common.NewMapOfMapOfMaps(),
					IdentityClusterCache:              identityClusterCache,
				},
			},
			globalIdentifier:     "asset1",
			clusterName:          "cluster1",
			clientNs:             "clientNs",
			initialDestinations:  []string{},
			expectedDestinations: []string{"bar", "foo"},
		},
		{
			name: "When some server identities are already processed, skip those",
			remoteRegistry: &RemoteRegistry{
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"asset1": {"foo", "bar"}},
						mutex:              &sync.Mutex{},
					},
					ClientClusterNamespaceServerCache: func() *common.MapOfMapOfMaps {
						m := common.NewMapOfMapOfMaps()
						m.Put("cluster1", "clientNs", "foo", "foo")
						return m
					}(),
					IdentityClusterCache: identityClusterCache,
				},
			},
			globalIdentifier:     "asset1",
			clusterName:          "cluster1",
			clientNs:             "clientNs",
			initialDestinations:  []string{},
			expectedDestinations: []string{"bar"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualDestinations := getDestinationsToBeProcessedForClientInitiatedProcessing(tc.remoteRegistry, tc.globalIdentifier, tc.clusterName, tc.clientNs, tc.initialDestinations, false)
			if len(actualDestinations) == 0 && len(tc.expectedDestinations) == 0 {
				return // both are empty, which is expected
			}
			if !reflect.DeepEqual(tc.expectedDestinations, actualDestinations) {
				t.Errorf("Test case %s failed: expected %v, got %v", tc.name, tc.expectedDestinations, actualDestinations)
			}
		})
	}
}
