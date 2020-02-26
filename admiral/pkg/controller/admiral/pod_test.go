package admiral

import (
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd"
	"sync"
	"testing"
	"time"
)

func TestNewPodController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockPodHandler{}

	podController, err := NewPodController(stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if podController == nil {
		t.Errorf("Pod controller should never be nil without an error thrown")
	}
}

func TestPodController_GetPods(t *testing.T) {
	controller := PodController{
		labelSet: &common.LabelSet{
			DeploymentAnnotation:                "sidecar.istio.io/inject",
			NamespaceSidecarInjectionLabel:      "istio-injection",
			NamespaceSidecarInjectionLabelValue: "enabled",
			AdmiralIgnoreLabel:                  "admiral-ignore",
		},
	}

	client := fake.NewSimpleClientset()
	ns := v1.Namespace{}
	ns.Labels = map[string]string{"istio-injection": "enabled"}
	ns.Name = "test-ns"

	_, err := client.CoreV1().Namespaces().Create(&ns)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	pod := v1.Pod{}
	pod.Namespace = "test-ns"
	pod.Name="pod"
	pod.Labels = map[string]string{"identity": "id", "istio-injected": "true"}
	pod.Annotations = map[string]string{"sidecar.istio.io/inject": "true"}
	podWithBadLabels := v1.Pod{}
	podWithBadLabels.Namespace = "test-ns"
	podWithBadLabels.Name="podWithBadLabels"
	podWithBadLabels.Labels = map[string]string{"identity": "id", "random-label": "true"}
	podWithBadLabels.Annotations = map[string]string{"woo": "yay"}
	_, err = client.CoreV1().Pods("test-ns").Create(&pod)
	_, err = client.CoreV1().Pods("test-ns").Create(&podWithBadLabels)

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}
	
	controller.K8sClient = client

	podsList, err := controller.GetPods()

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}
	if !cmp.Equal(podsList[0].Name, pod.Name) || !cmp.Equal(podsList[0].Annotations, pod.Annotations) || !cmp.Equal(podsList[0].Labels, pod.Labels) {
		t.Errorf("Incorrect pod found. Mismatch: %v", cmp.Diff(podsList[0], pod))
	}
	if len(podsList) != 1 {
		t.Errorf("Too many pods found. Expected 1, found %v", podsList)
	}

}

func TestPodCache_AppendPodToCluster(t *testing.T) {
	podCache := podCache{}
	podCache.cache = make(map[string]*PodClusterEntry)
	podCache.mutex = &sync.Mutex{}

	pod := &v1.Pod{}
	pod.Name="foobar"
	pod.Namespace = "ns"
	pod.Labels = map[string]string{"identity":"my-first-pod"}

	podCache.AppendPodToCluster("ns", pod)

	if podCache.getKey(pod) != "my-first-pod" {
		t.Errorf("Incorrect key. Got %v, expected ns", podCache.getKey(pod))
	}
	if !cmp.Equal(podCache.Get("ns").Pods["ns"][0], pod) {
		t.Errorf("Incorrect pod fount. Diff: %v", cmp.Diff(podCache.Get("ns").Pods["ns"], pod))
	}

	length := len(podCache.Get("ns").Pods["ns"])

	podCache.AppendPodToCluster("ns", pod)

	if podCache.getKey(pod) != "my-first-pod" {
		t.Errorf("Incorrect key. Got %v, expected ns", podCache.getKey(pod))
	}
	if !cmp.Equal(podCache.Get("ns").Pods["ns"][0], pod) {
		t.Errorf("Incorrect pod fount. Diff: %v", cmp.Diff(podCache.Get("ns").Pods["ns"], pod))
	}
	if (length+1) != len(podCache.Get("ns").Pods["ns"]) {
		t.Errorf("Didn't add a second pod, expected %v, got %v", length+1, len(podCache.Get("ns").Pods["ns"]))
	}

	podCache.Delete(podCache.Get("ns"))

	if podCache.Get("ns") != nil {
		t.Errorf("Didn't delete successfully, expected nil, got %v", podCache.Get("ns"))
	}
}