package admiral

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/clientcmd"
)

func TestServiceAdded(t *testing.T) {

	serviceHandler := &test.MockServiceHandler{}
	ctx := context.Background()
	serviceController := ServiceController{
		ServiceHandler: serviceHandler,
		Cache: &serviceCache{
			cache: make(map[string]*ServiceClusterEntry),
			mutex: &sync.Mutex{},
		},
	}

	testCases := []struct {
		name          string
		service       interface{}
		expectedError error
	}{
		{
			name: "Given context and Service " +
				"When Service param is nil " +
				"Then func should return an error",
			service:       nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.Service"),
		},
		{
			name: "Given context and Service " +
				"When Service param is not of type *v1.Service " +
				"Then func should return an error",
			service:       struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.Service"),
		},
		{
			name: "Given context and Service " +
				"When Service param is of type *v1.Service " +
				"Then func should not return an error",
			service:       &coreV1.Service{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := serviceController.Added(ctx, tc.service)
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

func TestServiceUpdated(t *testing.T) {

	serviceHandler := &test.MockServiceHandler{}
	ctx := context.Background()
	serviceController := ServiceController{
		ServiceHandler: serviceHandler,
		Cache: &serviceCache{
			cache: make(map[string]*ServiceClusterEntry),
			mutex: &sync.Mutex{},
		},
	}

	testCases := []struct {
		name          string
		service       interface{}
		expectedError error
	}{
		{
			name: "Given context and Service " +
				"When Service param is nil " +
				"Then func should return an error",
			service:       nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.Service"),
		},
		{
			name: "Given context and Service " +
				"When Service param is not of type *v1.Service " +
				"Then func should return an error",
			service:       struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.Service"),
		},
		{
			name: "Given context and Service " +
				"When Service param is of type *v1.Service " +
				"Then func should not return an error",
			service:       &coreV1.Service{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := serviceController.Updated(ctx, tc.service, nil)
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

func TestServiceDeleted(t *testing.T) {

	serviceHandler := &test.MockServiceHandler{}
	ctx := context.Background()
	serviceController := ServiceController{
		ServiceHandler: serviceHandler,
		Cache: &serviceCache{
			cache: make(map[string]*ServiceClusterEntry),
			mutex: &sync.Mutex{},
		},
	}

	testCases := []struct {
		name          string
		service       interface{}
		expectedError error
	}{
		{
			name: "Given context and Service " +
				"When Service param is nil " +
				"Then func should return an error",
			service:       nil,
			expectedError: fmt.Errorf("type assertion failed, <nil> is not of type *v1.Service"),
		},
		{
			name: "Given context and Service " +
				"When Service param is not of type *v1.Service " +
				"Then func should return an error",
			service:       struct{}{},
			expectedError: fmt.Errorf("type assertion failed, {} is not of type *v1.Service"),
		},
		{
			name: "Given context and Service " +
				"When Service param is of type *v1.Service " +
				"Then func should not return an error",
			service:       &coreV1.Service{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := serviceController.Deleted(ctx, tc.service)
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

func TestNewServiceController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockServiceHandler{}

	serviceController, err := NewServiceController(stop, &handler, config, time.Duration(1000), loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if serviceController == nil {
		t.Errorf("Service controller should never be nil without an error thrown")
	}
}

// Doing triple duty - also testing get/delete
func TestServiceCache_Put(t *testing.T) {
	serviceCache := serviceCache{}
	serviceCache.cache = make(map[string]*ServiceClusterEntry)
	serviceCache.mutex = &sync.Mutex{}

	//test service cache empty with admiral ignore should skip to save in cache

	firstSvc := &coreV1.Service{}
	firstSvc.Name = "First Test Service"
	firstSvc.Namespace = "ns"
	firstSvc.Annotations = map[string]string{"admiral.io/ignore": "true"}

	serviceCache.Put(firstSvc)

	if serviceCache.Get("ns") != nil {
		t.Errorf("Service with admiral.io/ignore annotation should not be in cache")
	}

	secondSvc := &coreV1.Service{}
	secondSvc.Name = "First Test Service"
	secondSvc.Namespace = "ns"
	secondSvc.Labels = map[string]string{"admiral.io/ignore": "true"}

	serviceCache.Put(secondSvc)

	if serviceCache.Get("ns") != nil {
		t.Errorf("Service with admiral.io/ignore label should not be in cache")
	}

	service := &coreV1.Service{}
	service.Name = "Test Service"
	service.Namespace = "ns"

	serviceCache.Put(service)

	if serviceCache.getKey(service) != "ns" {
		t.Errorf("Incorrect key. Got %v, expected ns", serviceCache.getKey(service))
	}
	if !cmp.Equal(serviceCache.Get("ns")[0], service) {
		t.Errorf("Incorrect service found. Diff: %v", cmp.Diff(serviceCache.Get("ns")[0], service))
	}

	length := len(serviceCache.Get("ns"))

	serviceCache.Put(service)

	if serviceCache.getKey(service) != "ns" {
		t.Errorf("Incorrect key. Got %v, expected ns", serviceCache.getKey(service))
	}
	if !cmp.Equal(serviceCache.Get("ns")[0], service) {
		t.Errorf("Incorrect service found. Diff: %v", cmp.Diff(serviceCache.Get("ns")[0], service))
	}
	if (length) != len(serviceCache.Get("ns")) {
		t.Errorf("Re-added the same service. Cache length expected %v, got %v", length, len(serviceCache.Get("ns")))
	}

	serviceCache.Delete(service)

	if serviceCache.Get("ns") != nil {
		t.Errorf("Didn't delete successfully, expected nil, got %v", serviceCache.Get("ns"))
	}

}

func TestServiceCache_GetLoadBalancer(t *testing.T) {

	setAbsoluteFQDN(false)

	sc := serviceCache{}
	sc.cache = make(map[string]*ServiceClusterEntry)
	sc.mutex = &sync.Mutex{}

	service := &coreV1.Service{}
	service.Name = "test-service"
	service.Namespace = "ns"
	service.Status = coreV1.ServiceStatus{}
	service.Status.LoadBalancer = coreV1.LoadBalancerStatus{}
	service.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, coreV1.LoadBalancerIngress{Hostname: "hostname.com"})
	service.Labels = map[string]string{"app": "test-service"}

	s2 := &coreV1.Service{}
	s2.Name = "test-service-ip"
	s2.Namespace = "ns"
	s2.Status = coreV1.ServiceStatus{}
	s2.Status.LoadBalancer = coreV1.LoadBalancerStatus{}
	s2.Status.LoadBalancer.Ingress = append(s2.Status.LoadBalancer.Ingress, coreV1.LoadBalancerIngress{IP: "1.2.3.4"})
	s2.Labels = map[string]string{"app": "test-service-ip"}

	// The primary use case is to support ingress gateways for local development
	externalIPService := &coreV1.Service{}
	externalIPService.Name = "test-service-externalip"
	externalIPService.Namespace = "ns"
	externalIPService.Spec = coreV1.ServiceSpec{}
	externalIPService.Spec.ExternalIPs = []string{"1.2.3.4"}
	externalIPService.Spec.Ports = []coreV1.ServicePort{
		{
			Name:       "http",
			Protocol:   coreV1.ProtocolTCP,
			Port:       common.DefaultMtlsPort,
			TargetPort: intstr.FromInt(80),
			NodePort:   30800,
		},
	}
	externalIPService.Labels = map[string]string{"app": "test-service-externalip"}

	ignoreService := &coreV1.Service{}
	ignoreService.Name = "test-service-ignored"
	ignoreService.Namespace = "ns"
	ignoreService.Status = coreV1.ServiceStatus{}
	ignoreService.Status.LoadBalancer = coreV1.LoadBalancerStatus{}
	ignoreService.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, coreV1.LoadBalancerIngress{Hostname: "hostname.com"})
	ignoreService.Annotations = map[string]string{"admiral.io/ignore": "true"}
	ignoreService.Labels = map[string]string{"app": "test-service-ignored"}

	ignoreService2 := &coreV1.Service{}
	ignoreService2.Name = "test-service-ignored-later"
	ignoreService2.Namespace = "ns"
	ignoreService2.Status = coreV1.ServiceStatus{}
	ignoreService2.Status.LoadBalancer = coreV1.LoadBalancerStatus{}
	ignoreService2.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, coreV1.LoadBalancerIngress{Hostname: "hostname.com"})
	ignoreService2.Labels = map[string]string{"app": "test-service-ignored-later"}

	ignoreService3 := &coreV1.Service{}
	ignoreService3.Name = "test-service-unignored-later"
	ignoreService3.Namespace = "ns"
	ignoreService3.Status = coreV1.ServiceStatus{}
	ignoreService3.Status.LoadBalancer = coreV1.LoadBalancerStatus{}
	ignoreService3.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, coreV1.LoadBalancerIngress{Hostname: "hostname.com"})
	ignoreService3.Annotations = map[string]string{"admiral.io/ignore": "true"}
	ignoreService3.Labels = map[string]string{"app": "test-service-unignored-later"}

	sc.Put(service)
	sc.Put(s2)
	sc.Put(externalIPService)
	sc.Put(ignoreService)
	sc.Put(ignoreService2)
	sc.Put(ignoreService3)

	ignoreService2.Annotations = map[string]string{"admiral.io/ignore": "true"}
	ignoreService3.Annotations = map[string]string{"admiral.io/ignore": "false"}

	sc.Put(ignoreService2) //Ensuring that if the ignore label is added to a service, it's no longer found
	sc.Put(ignoreService3) //And ensuring that if the ignore label is removed from a service, it becomes found

	testCases := []struct {
		name           string
		cache          *serviceCache
		key            string
		ns             string
		expectedReturn string
		expectedPort   int
	}{
		{
			name:           "Find service load balancer when present",
			cache:          &sc,
			key:            "test-service",
			ns:             "ns",
			expectedReturn: "hostname.com",
			expectedPort:   common.DefaultMtlsPort,
		},
		{
			name:           "Return default when service not present",
			cache:          &sc,
			key:            "test-service",
			ns:             "ns-incorrect",
			expectedReturn: "dummy.admiral.global",
			expectedPort:   0,
		},
		{
			name:           "Falls back to IP",
			cache:          &sc,
			key:            "test-service-ip",
			ns:             "ns",
			expectedReturn: "1.2.3.4",
			expectedPort:   common.DefaultMtlsPort,
		},
		{
			name:           "Falls back to externalIP",
			cache:          &sc,
			key:            "test-service-externalip",
			ns:             "ns",
			expectedReturn: "1.2.3.4",
			expectedPort:   30800,
		},
		{
			name:           "Successfully ignores services with the ignore label",
			cache:          &sc,
			key:            "test-service-ignored",
			ns:             "ns",
			expectedReturn: "dummy.admiral.global",
			expectedPort:   common.DefaultMtlsPort,
		},
		{
			name:           "Successfully ignores services when the ignore label is added after the service had been added to the cache for the first time",
			cache:          &sc,
			key:            "test-service-ignored-later",
			ns:             "ns",
			expectedReturn: "dummy.admiral.global",
			expectedPort:   common.DefaultMtlsPort,
		},
		{
			name:           "Successfully finds services when the ignore label is added initially, then removed",
			cache:          &sc,
			key:            "test-service-unignored-later",
			ns:             "ns",
			expectedReturn: "hostname.com",
			expectedPort:   common.DefaultMtlsPort,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			loadBalancer, port := c.cache.GetLoadBalancer(c.key, c.ns)
			if loadBalancer != c.expectedReturn || port != c.expectedPort {
				t.Errorf("Unexpected load balancer returned. Got %v:%v, expected %v:%v", loadBalancer, port, c.expectedReturn, c.expectedPort)
			}
		})
	}
}

func TestServiceCache_GetLoadBalancerWithAbsoluteFQDN(t *testing.T) {

	setAbsoluteFQDN(true)

	sc := serviceCache{}
	sc.cache = make(map[string]*ServiceClusterEntry)
	sc.mutex = &sync.Mutex{}

	service := &coreV1.Service{}
	service.Name = "test-service"
	service.Namespace = "ns"
	service.Status = coreV1.ServiceStatus{}
	service.Status.LoadBalancer = coreV1.LoadBalancerStatus{}
	service.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, coreV1.LoadBalancerIngress{Hostname: "hostname.com"})
	service.Labels = map[string]string{"app": "test-service"}

	s2 := &coreV1.Service{}
	s2.Name = "test-service-ip"
	s2.Namespace = "ns"
	s2.Status = coreV1.ServiceStatus{}
	s2.Status.LoadBalancer = coreV1.LoadBalancerStatus{}
	s2.Status.LoadBalancer.Ingress = append(s2.Status.LoadBalancer.Ingress, coreV1.LoadBalancerIngress{IP: "1.2.3.4"})
	s2.Labels = map[string]string{"app": "test-service-ip"}

	// The primary use case is to support ingress gateways for local development
	externalIPService := &coreV1.Service{}
	externalIPService.Name = "test-service-externalip"
	externalIPService.Namespace = "ns"
	externalIPService.Spec = coreV1.ServiceSpec{}
	externalIPService.Spec.ExternalIPs = []string{"1.2.3.4"}
	externalIPService.Spec.Ports = []coreV1.ServicePort{
		{
			Name:       "http",
			Protocol:   coreV1.ProtocolTCP,
			Port:       common.DefaultMtlsPort,
			TargetPort: intstr.FromInt(80),
			NodePort:   30800,
		},
	}
	externalIPService.Labels = map[string]string{"app": "test-service-externalip"}

	ignoreService := &coreV1.Service{}
	ignoreService.Name = "test-service-ignored"
	ignoreService.Namespace = "ns"
	ignoreService.Status = coreV1.ServiceStatus{}
	ignoreService.Status.LoadBalancer = coreV1.LoadBalancerStatus{}
	ignoreService.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, coreV1.LoadBalancerIngress{Hostname: "hostname.com"})
	ignoreService.Annotations = map[string]string{"admiral.io/ignore": "true"}
	ignoreService.Labels = map[string]string{"app": "test-service-ignored"}

	ignoreService2 := &coreV1.Service{}
	ignoreService2.Name = "test-service-ignored-later"
	ignoreService2.Namespace = "ns"
	ignoreService2.Status = coreV1.ServiceStatus{}
	ignoreService2.Status.LoadBalancer = coreV1.LoadBalancerStatus{}
	ignoreService2.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, coreV1.LoadBalancerIngress{Hostname: "hostname.com"})
	ignoreService2.Labels = map[string]string{"app": "test-service-ignored-later"}

	ignoreService3 := &coreV1.Service{}
	ignoreService3.Name = "test-service-unignored-later"
	ignoreService3.Namespace = "ns"
	ignoreService3.Status = coreV1.ServiceStatus{}
	ignoreService3.Status.LoadBalancer = coreV1.LoadBalancerStatus{}
	ignoreService3.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, coreV1.LoadBalancerIngress{Hostname: "hostname.com"})
	ignoreService3.Annotations = map[string]string{"admiral.io/ignore": "true"}
	ignoreService3.Labels = map[string]string{"app": "test-service-unignored-later"}

	sc.Put(service)
	sc.Put(s2)
	sc.Put(externalIPService)
	sc.Put(ignoreService)
	sc.Put(ignoreService2)
	sc.Put(ignoreService3)

	ignoreService2.Annotations = map[string]string{"admiral.io/ignore": "true"}
	ignoreService3.Annotations = map[string]string{"admiral.io/ignore": "false"}

	sc.Put(ignoreService2) //Ensuring that if the ignore label is added to a service, it's no longer found
	sc.Put(ignoreService3) //And ensuring that if the ignore label is removed from a service, it becomes found

	testCases := []struct {
		name           string
		cache          *serviceCache
		key            string
		ns             string
		expectedReturn string
		expectedPort   int
	}{
		{
			name:           "Given service and loadbalancer, should return endpoint with dot in the end",
			cache:          &sc,
			key:            "test-service",
			ns:             "ns",
			expectedReturn: "hostname.com.",
			expectedPort:   common.DefaultMtlsPort,
		},
		{
			name:           "Given service not present, should return dummy",
			cache:          &sc,
			key:            "test-service",
			ns:             "ns-incorrect",
			expectedReturn: "dummy.admiral.global",
			expectedPort:   0,
		},
		{
			name:           "Given host not present in load balancer, should fallback to IP without dot at the end",
			cache:          &sc,
			key:            "test-service-ip",
			ns:             "ns",
			expectedReturn: "1.2.3.4",
			expectedPort:   common.DefaultMtlsPort,
		},
		{
			name:           "Given ignore label, should return dummy",
			cache:          &sc,
			key:            "test-service-ignored",
			ns:             "ns",
			expectedReturn: "dummy.admiral.global",
			expectedPort:   common.DefaultMtlsPort,
		},
		{
			name:           "Successfully ignores services when the ignore label is added after the service had been added to the cache for the first time",
			cache:          &sc,
			key:            "test-service-ignored-later",
			ns:             "ns",
			expectedReturn: "dummy.admiral.global",
			expectedPort:   common.DefaultMtlsPort,
		},
		{
			name:           "Successfully finds services when the ignore label is added initially, then removed",
			cache:          &sc,
			key:            "test-service-unignored-later",
			ns:             "ns",
			expectedReturn: "hostname.com.",
			expectedPort:   common.DefaultMtlsPort,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			loadBalancer, port := c.cache.GetLoadBalancer(c.key, c.ns)
			if loadBalancer != c.expectedReturn || port != c.expectedPort {
				t.Errorf("Unexpected load balancer returned. Got %v:%v, expected %v:%v", loadBalancer, port, c.expectedReturn, c.expectedPort)
			}
		})
	}
}

func setAbsoluteFQDN(flag bool) {
	admiralParams := common.GetAdmiralParams()
	admiralParams.EnableAbsoluteFQDN = flag
	common.ResetSync()
	common.InitializeConfig(admiralParams)
}

func TestConcurrentGetAndPut(t *testing.T) {
	serviceCache := serviceCache{}
	serviceCache.cache = make(map[string]*ServiceClusterEntry)
	serviceCache.mutex = &sync.Mutex{}

	serviceCache.Put(&coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{Name: "testname", Namespace: "testns"},
	})

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(3*time.Second))
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	// Producer go routine
	go func(ctx context.Context) {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				serviceCache.Put(&coreV1.Service{
					ObjectMeta: metaV1.ObjectMeta{Name: "testname", Namespace: string(uuid.NewUUID())},
				})
			}
		}
	}(ctx)

	// Consumer go routine
	go func(ctx context.Context) {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				assert.NotNil(t, serviceCache.Get("testns"))
			}
		}
	}(ctx)

	wg.Wait()

}

func TestGetOrderedServices(t *testing.T) {

	//Struct of test case info. Name is required.
	testCases := []struct {
		name           string
		services       map[string]*ServiceItem
		expectedResult string
	}{
		{
			name:           "Should return nil for nil input",
			services:       nil,
			expectedResult: "",
		},
		{
			name: "Should return the only service",
			services: map[string]*ServiceItem{
				"s1": {Service: &coreV1.Service{ObjectMeta: metaV1.ObjectMeta{Name: "s1", Namespace: "ns1", CreationTimestamp: metaV1.NewTime(time.Now())}}},
			},
			expectedResult: "s1",
		},
		{
			name: "Should return the latest service by creationTime",
			services: map[string]*ServiceItem{
				"s1": {Service: &coreV1.Service{ObjectMeta: metaV1.ObjectMeta{Name: "s1", Namespace: "ns1", CreationTimestamp: metaV1.NewTime(time.Now().Add(time.Duration(-15)))}}},
				"s2": {Service: &coreV1.Service{ObjectMeta: metaV1.ObjectMeta{Name: "s2", Namespace: "ns1", CreationTimestamp: metaV1.NewTime(time.Now())}}},
			},
			expectedResult: "s2",
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getOrderedServices(c.services)
			if c.expectedResult == "" && len(result) > 0 {
				t.Errorf("Failed. Got %v, expected no service", result[0].Name)
			} else if c.expectedResult != "" {
				if len(result) > 0 && result[0].Name == c.expectedResult {
					//perfect
				} else {
					t.Errorf("Failed. Got %v, expected %v", result[0].Name, c.expectedResult)
				}
			}
		})
	}
}

func TestServiceGetProcessItemStatus(t *testing.T) {
	var (
		serviceAccount = &coreV1.ServiceAccount{}
		svcInCache     = &coreV1.Service{
			ObjectMeta: metaV1.ObjectMeta{
				Name:              "s1",
				Namespace:         "ns1",
				CreationTimestamp: metaV1.NewTime(time.Now()),
			},
		}
		svcInCache2 = &coreV1.Service{
			ObjectMeta: metaV1.ObjectMeta{
				Name:              "s2",
				Namespace:         "ns1",
				CreationTimestamp: metaV1.NewTime(time.Now()),
			},
		}
		svcNotInCache = &coreV1.Service{
			ObjectMeta: metaV1.ObjectMeta{
				Name:              "s3",
				Namespace:         "ns1",
				CreationTimestamp: metaV1.NewTime(time.Now()),
			},
		}
	)

	// Populating the deployment Cache
	svcCache := &serviceCache{
		cache: make(map[string]*ServiceClusterEntry),
		mutex: &sync.Mutex{},
	}

	svcController := &ServiceController{
		Cache: svcCache,
	}

	svcCache.Put(svcInCache)
	svcCache.UpdateSvcProcessStatus(svcInCache, common.Processed)
	svcCache.UpdateSvcProcessStatus(svcInCache2, common.NotProcessed)

	cases := []struct {
		name        string
		obj         interface{}
		expectedRes string
		expectedErr error
	}{
		{
			name: "Given service cache has a valid service in its cache, " +
				"And the service is processed" +
				"Then, we should be able to get the status as true",
			obj:         svcInCache,
			expectedRes: common.Processed,
		},
		{
			name: "Given service cache has a valid service in its cache, " +
				"And the service is processed" +
				"Then, we should be able to get the status as false",
			obj:         svcInCache2,
			expectedRes: common.NotProcessed,
		},
		{
			name: "Given service cache does not has a valid service in its cache, " +
				"Then, the function would return false",
			obj:         svcNotInCache,
			expectedRes: common.NotProcessed,
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:         serviceAccount,
			expectedErr: fmt.Errorf("type assertion failed"),
			expectedRes: common.NotProcessed,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := svcController.GetProcessItemStatus(c.obj)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			assert.Equal(t, c.expectedRes, res)
		})
	}
}

func TestServiceUpdateProcessItemStatus(t *testing.T) {
	var (
		serviceAccount = &coreV1.ServiceAccount{}
		svcInCache     = &coreV1.Service{
			ObjectMeta: metaV1.ObjectMeta{
				Name:              "s1",
				Namespace:         "ns1",
				CreationTimestamp: metaV1.NewTime(time.Now()),
			},
		}
		svcInCache2 = &coreV1.Service{
			ObjectMeta: metaV1.ObjectMeta{
				Name:              "s2",
				Namespace:         "ns1",
				CreationTimestamp: metaV1.NewTime(time.Now()),
			},
		}
		svcNotInCache = &coreV1.Service{
			ObjectMeta: metaV1.ObjectMeta{
				Name:              "s3",
				Namespace:         "ns1",
				CreationTimestamp: metaV1.NewTime(time.Now()),
			},
		}
		diffNsSvcNotInCache = &coreV1.Service{
			ObjectMeta: metaV1.ObjectMeta{
				Name:              "s4",
				Namespace:         "ns2",
				CreationTimestamp: metaV1.NewTime(time.Now()),
			},
		}
	)

	// Populating the deployment Cache
	svcCache := &serviceCache{
		cache: make(map[string]*ServiceClusterEntry),
		mutex: &sync.Mutex{},
	}

	svcController := &ServiceController{
		Cache: svcCache,
	}

	svcCache.Put(svcInCache)
	svcCache.Put(svcInCache2)

	cases := []struct {
		name           string
		obj            interface{}
		statusToSet    string
		expectedStatus string
		expectedErr    error
	}{
		{
			name: "Given service cache has a valid service in its cache, " +
				"Then, the status for the valid service should be updated to true",
			obj:            svcInCache,
			statusToSet:    common.Processed,
			expectedErr:    nil,
			expectedStatus: common.Processed,
		},
		{
			name: "Given service cache has a valid service in its cache, " +
				"Then, the status for the valid service should be updated to false",
			obj:            svcInCache2,
			statusToSet:    common.NotProcessed,
			expectedErr:    nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given service cache does not has a valid service in its cache, " +
				"Then, an error should be returned with the service not found message",
			obj:         svcNotInCache,
			statusToSet: common.NotProcessed,
			expectedErr: fmt.Errorf(LogCacheFormat, "Update", "Service",
				"s3", "ns1", "", "nothing to update, service not found in cache"),
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given service cache does not has a valid service in its cache, " +
				"And service is in a different namespace, " +
				"Then, an error should be returned with the service not found message",
			obj:         diffNsSvcNotInCache,
			statusToSet: common.NotProcessed,
			expectedErr: fmt.Errorf(LogCacheFormat, "Update", "Service",
				"s4", "ns2", "", "nothing to update, service not found in cache"),
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

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := svcController.UpdateProcessItemStatus(c.obj, c.statusToSet)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			status, _ := svcController.GetProcessItemStatus(c.obj)
			assert.Equal(t, c.expectedStatus, status)
		})
	}
}

func TestServiceLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a Service object
	d := &ServiceController{}
	d.LogValueOfAdmiralIoIgnore("not a service")
	// No error should occur

	// Test case 2: Service has no annotations or labels
	d = &ServiceController{}
	d.LogValueOfAdmiralIoIgnore(&coreV1.Service{})
	// No error should occur

	// Test case 3: AdmiralIgnoreAnnotation is not set
	d = &ServiceController{}
	s := &coreV1.Service{ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{"other-annotation": "value"}}}
	d.LogValueOfAdmiralIoIgnore(s)
	// No error should occur

	// Test case 4: AdmiralIgnoreAnnotation is set in annotations
	d = &ServiceController{}
	s = &coreV1.Service{ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}}
	d.LogValueOfAdmiralIoIgnore(s)
	// No error should occur

	// Test case 5: AdmiralIgnoreAnnotation is set in labels
	d = &ServiceController{}
	s = &coreV1.Service{ObjectMeta: metaV1.ObjectMeta{Labels: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}}
	d.LogValueOfAdmiralIoIgnore(s)
	// No error should occur
}
