package admiral

import (
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/clientcmd"
	"sync"
	"testing"
	"time"
)

func TestNewServiceController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockServiceHandler{}

	serviceController, err := NewServiceController(stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if serviceController == nil {
		t.Errorf("Service controller should never be nil without an error thrown")
	}
}

//Doing triple duty - also testing get/delete
func TestServiceCache_Put(t *testing.T) {
	serviceCache := serviceCache{}
	serviceCache.cache = make(map[string]*ServiceClusterEntry)
	serviceCache.mutex = &sync.Mutex{}

	//test service cache empty with admiral ignore should skip to save in cache

	firstSvc := &v1.Service{}
	firstSvc.Name = "First Test Service"
	firstSvc.Namespace = "ns"
	firstSvc.Annotations = map[string]string{"admiral.io/ignore": "true"}

	serviceCache.Put(firstSvc)

	if serviceCache.Get("ns") != nil {
		t.Errorf("Service with admiral.io/ignore annotation should not be in cache")
	}

	secondSvc := &v1.Service{}
	secondSvc.Name = "First Test Service"
	secondSvc.Namespace = "ns"
	secondSvc.Labels = map[string]string{"admiral.io/ignore": "true"}

	serviceCache.Put(secondSvc)

	if serviceCache.Get("ns") != nil {
		t.Errorf("Service with admiral.io/ignore label should not be in cache")
	}

	service := &v1.Service{}
	service.Name = "Test Service"
	service.Namespace = "ns"

	serviceCache.Put(service)

	if serviceCache.getKey(service) != "ns" {
		t.Errorf("Incorrect key. Got %v, expected ns", serviceCache.getKey(service))
	}
	if !cmp.Equal(serviceCache.Get("ns").Service["ns"][service.Name], service) {
		t.Errorf("Incorrect service fount. Diff: %v", cmp.Diff(serviceCache.Get("ns").Service["ns"], service))
	}

	length := len(serviceCache.Get("ns").Service["ns"])

	serviceCache.Put(service)

	if serviceCache.getKey(service) != "ns" {
		t.Errorf("Incorrect key. Got %v, expected ns", serviceCache.getKey(service))
	}
	if !cmp.Equal(serviceCache.Get("ns").Service["ns"][service.Name], service) {
		t.Errorf("Incorrect service fount. Diff: %v", cmp.Diff(serviceCache.Get("ns").Service["ns"], service))
	}
	if (length) != len(serviceCache.Get("ns").Service["ns"]) {
		t.Errorf("Re-added the same service. Cache length expected %v, got %v", length, len(serviceCache.Get("ns").Service["ns"]))
	}

	serviceCache.Delete(service)

	if serviceCache.Get("ns") != nil {
		t.Errorf("Didn't delete successfully, expected nil, got %v", serviceCache.Get("ns"))
	}

}

func TestServiceCache_GetLoadBalancer(t *testing.T) {
	sc := serviceCache{}
	sc.cache = make(map[string]*ServiceClusterEntry)
	sc.mutex = &sync.Mutex{}

	service := &v1.Service{}
	service.Name = "test-service"
	service.Namespace = "ns"
	service.Status = v1.ServiceStatus{}
	service.Status.LoadBalancer = v1.LoadBalancerStatus{}
	service.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, v1.LoadBalancerIngress{Hostname: "hostname.com"})
	service.Labels = map[string]string{"app": "test-service"}

	s2 := &v1.Service{}
	s2.Name = "test-service-ip"
	s2.Namespace = "ns"
	s2.Status = v1.ServiceStatus{}
	s2.Status.LoadBalancer = v1.LoadBalancerStatus{}
	s2.Status.LoadBalancer.Ingress = append(s2.Status.LoadBalancer.Ingress, v1.LoadBalancerIngress{IP: "1.2.3.4"})
	s2.Labels = map[string]string{"app": "test-service-ip"}

	// The primary use case is to support ingress gateways for local development
	externalIPService := &v1.Service{}
	externalIPService.Name = "test-service-externalip"
	externalIPService.Namespace = "ns"
	externalIPService.Spec = v1.ServiceSpec{}
	externalIPService.Spec.ExternalIPs = []string{"1.2.3.4"}
	externalIPService.Spec.Ports = []v1.ServicePort{
		{
			"http",
			v1.ProtocolTCP,
			common.DefaultMtlsPort,
			intstr.FromInt(80),
			30800,
		},
	}
	externalIPService.Labels = map[string]string{"app": "test-service-externalip"}

	ignoreService := &v1.Service{}
	ignoreService.Name = "test-service-ignored"
	ignoreService.Namespace = "ns"
	ignoreService.Status = v1.ServiceStatus{}
	ignoreService.Status.LoadBalancer = v1.LoadBalancerStatus{}
	ignoreService.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, v1.LoadBalancerIngress{Hostname: "hostname.com"})
	ignoreService.Annotations = map[string]string{"admiral.io/ignore": "true"}
	ignoreService.Labels = map[string]string{"app": "test-service-ignored"}

	ignoreService2 := &v1.Service{}
	ignoreService2.Name = "test-service-ignored-later"
	ignoreService2.Namespace = "ns"
	ignoreService2.Status = v1.ServiceStatus{}
	ignoreService2.Status.LoadBalancer = v1.LoadBalancerStatus{}
	ignoreService2.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, v1.LoadBalancerIngress{Hostname: "hostname.com"})
	ignoreService2.Labels = map[string]string{"app": "test-service-ignored-later"}

	ignoreService3 := &v1.Service{}
	ignoreService3.Name = "test-service-unignored-later"
	ignoreService3.Namespace = "ns"
	ignoreService3.Status = v1.ServiceStatus{}
	ignoreService3.Status.LoadBalancer = v1.LoadBalancerStatus{}
	ignoreService3.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, v1.LoadBalancerIngress{Hostname: "hostname.com"})
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
