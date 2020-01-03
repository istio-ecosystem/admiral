package admiral

import (
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/api/core/v1"
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

	service := &v1.Service{}
	service.Name = "Test Service"
	service.Namespace = "ns"

	serviceCache.Put(service)

	if serviceCache.getKey(service) != "ns" {
		t.Errorf("Incorrect key. Got %v, expected ns", serviceCache.getKey(service))
	}
	if !cmp.Equal(serviceCache.Get("ns").Service["ns"][0], service) {
		t.Errorf("Incorrect service fount. Diff: %v", cmp.Diff(serviceCache.Get("ns").Service["ns"], service))
	}

	length := len(serviceCache.Get("ns").Service["ns"])

	serviceCache.Put(service)

	if serviceCache.getKey(service) != "ns" {
		t.Errorf("Incorrect key. Got %v, expected ns", serviceCache.getKey(service))
	}
	if !cmp.Equal(serviceCache.Get("ns").Service["ns"][0], service) {
		t.Errorf("Incorrect service fount. Diff: %v", cmp.Diff(serviceCache.Get("ns").Service["ns"], service))
	}
	if (length+1) != len(serviceCache.Get("ns").Service["ns"]) {
		t.Errorf("Didn't add a second service, expected %v, got %v", length+1, len(serviceCache.Get("ns").Service["ns"]))
	}

	serviceCache.Delete(serviceCache.Get("ns"))

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
	service.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, v1.LoadBalancerIngress{Hostname:"hostname.com"})

	s2 := &v1.Service{}
	s2.Name = "test-service-ip"
	s2.Namespace = "ns"
	s2.Status = v1.ServiceStatus{}
	s2.Status.LoadBalancer = v1.LoadBalancerStatus{}
	s2.Status.LoadBalancer.Ingress = append(s2.Status.LoadBalancer.Ingress, v1.LoadBalancerIngress{IP:"1.2.3.4"})

	sc.Put(service)
	sc.Put(s2)


	testCases := []struct {
		name string
		cache *serviceCache
		key string
		ns string
		expectedReturn string
	}{
		{
			name: "Find service load balancer when present",
			cache: &sc,
			key: "test-service",
			ns: "ns",
			expectedReturn: "hostname.com",
		},
		{
			name: "Return default when service not present",
			cache: &sc,
			key: "test-service",
			ns: "ns-incorrect",
			expectedReturn: "admiral_dummy.com",
		},
		{
			name: "Falls back to IP",
			cache: &sc,
			key: "test-service-ip",
			ns: "ns",
			expectedReturn: "1.2.3.4",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			loadBalancer := c.cache.GetLoadBalancer(c.key, c.ns)
			if loadBalancer != c.expectedReturn {
				t.Errorf("Unexpected load balancer returned. Got %v, expected %v", loadBalancer, c.expectedReturn)
			}
		})
	}
}

