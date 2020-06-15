package clusters

import (
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	argofake "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/fake"
	"github.com/gogo/protobuf/types"
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"istio.io/api/networking/v1alpha3"
	v1alpha32 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	coreV1 "k8s.io/api/core/v1"
	k8sv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"sync"
	"testing"
	"time"
	k8sAppsV1 "k8s.io/api/apps/v1"
)

func TestIgnoreIstioResource(t *testing.T) {

	//Struct of test case info. Name is required.
	testCases := []struct {
		name           string
		exportTo       []string
		expectedResult bool
	}{
		{
			name:           "Should return false when exportTo is not present",
			exportTo:       nil,
			expectedResult: false,
		},
		{
			name:           "Should return false when its exported to *",
			exportTo:       []string{"*"},
			expectedResult: false,
		},
		{
			name:           "Should return true when its exported to .",
			exportTo:       []string{"."},
			expectedResult: true,
		},
		{
			name:           "Should return true when its exported to a handful of namespaces",
			exportTo:       []string{"namespace1", "namespace2"},
			expectedResult: true,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := IgnoreIstioResource(c.exportTo)
			if result == c.expectedResult {
				//perfect
			} else {
				t.Errorf("Failed. Got %v, expected %v", result, c.expectedResult)
			}
		})
	}
}

func TestGetDestinationRule(t *testing.T) {
	//Do setup here
	outlierDetection := &v1alpha3.OutlierDetection{
		BaseEjectionTime:  &types.Duration{Seconds: 120},
		ConsecutiveErrors: int32(10),
		Interval:          &types.Duration{Seconds: 5}}
	mTLS := &v1alpha3.TrafficPolicy{Tls: &v1alpha3.TLSSettings{Mode: v1alpha3.TLSSettings_ISTIO_MUTUAL}, OutlierDetection: outlierDetection,}

	noGtpDr := v1alpha3.DestinationRule{
		Host:          "qa.myservice.global",
		TrafficPolicy: mTLS,
	}

	basicGtpDr := v1alpha3.DestinationRule{
		Host: "qa.myservice.global",
		TrafficPolicy: &v1alpha3.TrafficPolicy{
			Tls: &v1alpha3.TLSSettings{Mode: v1alpha3.TLSSettings_ISTIO_MUTUAL},
			LoadBalancer: &v1alpha3.LoadBalancerSettings{
				LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{Simple: v1alpha3.LoadBalancerSettings_ROUND_ROBIN},
				LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{},
			},
			OutlierDetection: outlierDetection,
		},
	}

	failoverGtpDr := v1alpha3.DestinationRule{
		Host: "qa.myservice.global",
		TrafficPolicy: &v1alpha3.TrafficPolicy{
			Tls: &v1alpha3.TLSSettings{Mode: v1alpha3.TLSSettings_ISTIO_MUTUAL},
			LoadBalancer: &v1alpha3.LoadBalancerSettings{
				LbPolicy: &v1alpha3.LoadBalancerSettings_Simple{Simple: v1alpha3.LoadBalancerSettings_ROUND_ROBIN},
				LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{
					Distribute: []*v1alpha3.LocalityLoadBalancerSetting_Distribute{
						{
							From: "uswest2/*",
							To: map[string]uint32{"us-west-2": 100},
						},
					},
				},
			},
			OutlierDetection: outlierDetection,
		},
	}

	topologyGTPBody := model.GlobalTrafficPolicy{
		Policy: []*model.TrafficPolicy{
			{
				LbType: model.TrafficPolicy_TOPOLOGY,
				Target: []*model.TrafficGroup{
					{
						Region: "us-west-2",
						Weight: 100,
					},
				},
			},
		},
	}

	topologyGTP := v1.GlobalTrafficPolicy{
		Spec: topologyGTPBody,
	}
	topologyGTP.Name = "myGTP"
	topologyGTP.Namespace = "myNS"

	failoverGTPBody := model.GlobalTrafficPolicy{
		Policy: []*model.TrafficPolicy{
			{
				LbType: model.TrafficPolicy_FAILOVER,
				Target: []*model.TrafficGroup{
					{
						Region: "us-west-2",
						Weight: 100,
					},
					{
						Region: "us-east-2",
						Weight: 0,
					},
				},
			},
		},
	}

	failoverGTP := v1.GlobalTrafficPolicy{
		Spec: failoverGTPBody,
	}
	failoverGTP.Name = "myGTP"
	failoverGTP.Namespace = "myNS"

	//Struct of test case info. Name is required.
	testCases := []struct {
		name            string
		host            string
		locality        string
		gtp             *v1.GlobalTrafficPolicy
		destinationRule *v1alpha3.DestinationRule
	}{
		{
			name:            "Should handle a nil GTP",
			host:            "qa.myservice.global",
			locality:        "uswest2",
			gtp:             nil,
			destinationRule: &noGtpDr,
		},
		{
			name:            "Should handle a topology GTP",
			host:            "qa.myservice.global",
			locality:        "uswest2",
			gtp:             &topologyGTP,
			destinationRule: &basicGtpDr,
		},
		{
			name:            "Should handle a failover GTP",
			host:            "qa.myservice.global",
			locality:        "uswest2",
			gtp:             &failoverGTP,
			destinationRule: &failoverGtpDr,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getDestinationRule(c.host, c.locality, c.gtp)
			if !cmp.Equal(result, c.destinationRule) {
				t.Fatalf("DestinationRule Mismatch. Diff: %v", cmp.Diff(result, c.destinationRule))
			}
		})
	}
}


func TestHandleVirtualServiceEvent(t *testing.T) {
	//Do setup here
	syncNs := v1alpha32.VirtualService{}
	syncNs.Namespace = "ns"

	tooManyHosts := v1alpha32.VirtualService{
		Spec: v1alpha3.VirtualService{
			Hosts: []string{"qa.blah.global", "e2e.blah.global"},
		},
	}
	tooManyHosts.Namespace = "other-ns"

	happyPath := v1alpha32.VirtualService{
		Spec: v1alpha3.VirtualService{
			Hosts: []string{"e2e.blah.global"},
		},
	}
	happyPath.Namespace = "other-ns"
	happyPath.Name = "vs-name"

	cnameCache := common.NewMapOfMaps()
	noDependencClustersHandler := VirtualServiceHandler{
		RemoteRegistry: &RemoteRegistry{
			remoteControllers: map[string]*RemoteController{},
			AdmiralCache: &AdmiralCache{
				CnameDependentClusterCache: cnameCache,
			},
		},
	}

	fakeIstioClient := istiofake.NewSimpleClientset()
	goodCnameCache := common.NewMapOfMaps()
	goodCnameCache.Put("e2e.blah.global", "cluster.k8s.global", "cluster.k8s.global")
	handlerEmptyClient := VirtualServiceHandler{
		RemoteRegistry: &RemoteRegistry{
			remoteControllers: map[string]*RemoteController{
				"cluster.k8s.global": &RemoteController{
					VirtualServiceController: &istio.VirtualServiceController{
						IstioClient: fakeIstioClient,
					},
				},
			},
			AdmiralCache: &AdmiralCache{
				CnameDependentClusterCache: goodCnameCache,
			},
		},
	}

	fullFakeIstioClient := istiofake.NewSimpleClientset()
	fullFakeIstioClient.NetworkingV1alpha3().VirtualServices("ns").Create(&v1alpha32.VirtualService{
		ObjectMeta: v12.ObjectMeta{
			Name: "vs-name",
		},
		Spec: v1alpha3.VirtualService{
			Hosts: []string{"e2e.blah.global"},
		},
	})
	handlerFullClient := VirtualServiceHandler{
		RemoteRegistry: &RemoteRegistry{
			remoteControllers: map[string]*RemoteController{
				"cluster.k8s.global": &RemoteController{
					VirtualServiceController: &istio.VirtualServiceController{
						IstioClient: fullFakeIstioClient,
					},
				},
			},
			AdmiralCache: &AdmiralCache{
				CnameDependentClusterCache: goodCnameCache,
			},
		},
	}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name string
		vs *v1alpha32.VirtualService
		handler *VirtualServiceHandler
		expectedError error
		event common.Event
	}{
		{
			name: "Virtual Service in sync namespace",
			vs: &syncNs,
			expectedError: nil,
			handler: &noDependencClustersHandler,
			event: 0,
		},
		{
			name: "Virtual Service with multiple hosts",
			vs: &tooManyHosts,
			expectedError: nil,
			handler: &noDependencClustersHandler,
			event: 0,
		},
		{
			name: "No dependent clusters",
			vs: &happyPath,
			expectedError: nil,
			handler: &noDependencClustersHandler,
			event: 0,
		},
		{
			name: "New Virtual Service",
			vs: &happyPath,
			expectedError: nil,
			handler: &handlerEmptyClient,
			event: 0,
		},
		{
			name: "Existing Virtual Service",
			vs: &happyPath,
			expectedError: nil,
			handler: &handlerFullClient,
			event: 1,
		},
		{
			name: "Deleted Virtual Service",
			vs: &happyPath,
			expectedError: nil,
			handler: &handlerFullClient,
			event: 2,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := handleVirtualServiceEvent(c.vs, c.handler, c.event, common.VirtualService)
			if err != c.expectedError {
				t.Fatalf("Error mismatch, expected %v but got %v", c.expectedError, err)
			}
		})
	}
}


func TestGetServiceForRolloutCanary(t *testing.T){
	//Struct of test case info. Name is required.
	const  NAMESPACE = "namespace"
	const  SERVICENAME  = "serviceName"
	config := rest.Config{
		Host: "localhost",
	}
	stop := make(chan struct{})

	s, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))
	r, e := admiral.NewRolloutsController(stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300))

	if e != nil {
		t.Fatalf("Inititalization failed")
	}

	rcTemp := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{},
		ServiceController: s,
		RolloutController:r}

	selectorMap := make(map[string] string)
	selectorMap["app"] ="test"

	service := &coreV1.Service{
		Spec : coreV1.ServiceSpec{
			Selector:selectorMap,
		},
	}
	service.Name = SERVICENAME
	service.Namespace = NAMESPACE
	port1 := coreV1.ServicePort {
		Port : 8080,
	}

	port2 := coreV1.ServicePort {
		Port : 8081,
	}

	ports := []coreV1.ServicePort{port1, port2}
	service.Spec.Ports = ports


	selectorMap1 := make(map[string] string)
	selectorMap1["app"] ="test1"
	service1 := &coreV1.Service{
		Spec : coreV1.ServiceSpec{
			Selector:selectorMap,
		},
	}
	service1.Name = "dummy"
	service1.Namespace = "namespace1"
	port3 := coreV1.ServicePort {
		Port : 8080,
		Name: "random3",
	}

	port4 := coreV1.ServicePort {
		Port : 8081,
		Name: "random4",
	}

	ports1 := []coreV1.ServicePort{port3, port4}
	service1.Spec.Ports = ports1


	selectorMap4 := make(map[string] string)
	selectorMap4["app"] ="test"
	service4 := &coreV1.Service{
		Spec : coreV1.ServiceSpec{
			Selector:selectorMap4,
		},
	}
	service4.Name = "dummy"
	service4.Namespace = "namespace4"
	port11 := coreV1.ServicePort {
		Port : 8080,
		Name: "random3",
	}

	port12 := coreV1.ServicePort {
		Port : 8081,
		Name: "random4",
	}

	ports11 := []coreV1.ServicePort{port11, port12}
	service4.Spec.Ports = ports11

	rcTemp.ServiceController.Cache.Put(service)
	rcTemp.ServiceController.Cache.Put(service1)
	rcTemp.ServiceController.Cache.Put(service4)


	canaryRollout:= argo.Rollout{
		Spec: argo.RolloutSpec{Template:coreV1.PodTemplateSpec{
			ObjectMeta: k8sv1.ObjectMeta{Annotations:map[string]string{}},
		}}};
	matchLabel := make(map[string] string)
	matchLabel["app"] ="test"

	labelSelector := v12.LabelSelector{
		MatchLabels:matchLabel,
	}
	canaryRollout.Spec.Selector = &labelSelector

	canaryRollout.Namespace =NAMESPACE
	canaryRollout.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{},
	}


	canaryRolloutNS1:= argo.Rollout{
		Spec: argo.RolloutSpec{Template:coreV1.PodTemplateSpec{
			ObjectMeta: k8sv1.ObjectMeta{Annotations:map[string]string{}},
		}}};
	matchLabel2 := make(map[string] string)
	matchLabel2["app"] ="test1"

	labelSelector2 := v12.LabelSelector{
		MatchLabels:matchLabel2,
	}
	canaryRolloutNS1.Spec.Selector = &labelSelector2

	canaryRolloutNS1.Namespace ="namespace1"
	canaryRolloutNS1.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{},
	}


	canaryRolloutNS4:= argo.Rollout{
		Spec: argo.RolloutSpec{Template:coreV1.PodTemplateSpec{
			ObjectMeta: k8sv1.ObjectMeta{Annotations:map[string]string{}},
		}}};
	matchLabel4 := make(map[string] string)
	matchLabel4["app"] ="test"

	labelSelector4 := v12.LabelSelector{
		MatchLabels:matchLabel4,
	}
	canaryRolloutNS4.Spec.Selector = &labelSelector4

	canaryRolloutNS4.Namespace ="namespace4"
	canaryRolloutNS4.Spec.Strategy = argo.RolloutStrategy{
		Canary: &argo.CanaryStrategy{},
	}

	anotationsNS4Map := make(map[string]string)
	anotationsNS4Map[common.SidecarEnabledPorts] ="8080"

	canaryRolloutNS4.Spec.Template.Annotations = anotationsNS4Map

	testCases := []struct {
		name string
		rollout         *argo.Rollout
		rc             	*RemoteController
		result          string
	}{
		{
			name : "canaryRolloutHappyCaseMeshPortAnnotationOnRollout",
			rollout :&canaryRolloutNS4,
			rc : rcTemp,
			result: "dummy",
		},{
			name : "canaryRolloutWithoutSelectorMatch",
			rollout :&canaryRolloutNS1,
			rc : rcTemp,
			result: "",
		},{
			name : "canaryRolloutHappyCase",
			rollout :&canaryRollout,
			rc : rcTemp,
			result: SERVICENAME,
		},

	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getServiceForRollout(c.rc,c.rollout)
			if len(c.result) ==0 {
				if result != nil {
					t.Fatalf("Service expected to be nil. Value is = %v" , result)
				}
			}else
			if !cmp.Equal(result.Name, c.result) {
				t.Fatalf("Service Mismatch. Diff: %v", cmp.Diff(result.Name, c.name))
			}
		})
	}
}


func TestGetServiceForRolloutBlueGreen(t *testing.T){
	//Struct of test case info. Name is required.
	const  NAMESPACE = "namespace"
	const  SERVICENAME  = "serviceNameActive"
	const  ROLLOUT_POD_HASH_LABEL string = "rollouts-pod-template-hash"

	config := rest.Config{
		Host: "localhost",
	}
	stop := make(chan struct{})

	s, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))
	r, e := admiral.NewRolloutsController(stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300))


	emptyCacheService, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))

	if e != nil {
		t.Fatalf("Inititalization failed")
	}

	rc := &RemoteController{
		VirtualServiceController: &istio.VirtualServiceController{},
		ServiceController: s,
		RolloutController:r}


	bgRollout:= argo.Rollout{
		Spec: argo.RolloutSpec{Template:coreV1.PodTemplateSpec{
			ObjectMeta: k8sv1.ObjectMeta{Annotations:map[string]string{}},
		}}};

	matchLabel := make(map[string] string)
	matchLabel["app"] ="test"

	labelSelector := v12.LabelSelector{
		MatchLabels:matchLabel,
	}
	bgRollout.Spec.Selector = &labelSelector

	bgRollout.Namespace =NAMESPACE
	bgRollout.Spec.Strategy = argo.RolloutStrategy{
		BlueGreen: &argo.BlueGreenStrategy{
			ActiveService:SERVICENAME,
			PreviewService:"previewService",
		},
	}

	selectorMap := make(map[string] string)
	selectorMap["app"] ="test"
	selectorMap[ROLLOUT_POD_HASH_LABEL] ="hash"

	activeService := &coreV1.Service{
		Spec : coreV1.ServiceSpec{
			Selector:selectorMap,
		},
	}
	activeService.Name = SERVICENAME
	activeService.Namespace = NAMESPACE
	port1 := coreV1.ServicePort {
		Port : 8080,
		Name: "random1",
	}

	port2 := coreV1.ServicePort {
		Port : 8081,
		Name: "random2",
	}

	ports := []coreV1.ServicePort{port1, port2}
	activeService.Spec.Ports = ports



	selectorMap1 := make(map[string] string)
	selectorMap1["app"] ="test1"

	service1 := &coreV1.Service{
		Spec : coreV1.ServiceSpec{
			Selector:selectorMap,
		},
	}
	service1.Name = "dummy"
	service1.Namespace = NAMESPACE
	port3 := coreV1.ServicePort {
		Port : 8080,
		Name: "random3",
	}

	port4 := coreV1.ServicePort {
		Port : 8081,
		Name: "random4",
	}

	ports1 := []coreV1.ServicePort{port3, port4}
	service1.Spec.Ports = ports1




	selectorMap2 := make(map[string] string)
	selectorMap2["app"] ="test"
	selectorMap2[ROLLOUT_POD_HASH_LABEL] ="hash"
	previewService := &coreV1.Service{
		Spec : coreV1.ServiceSpec{
			Selector:selectorMap,
		},
	}
	previewService.Name = "previewService"
	previewService.Namespace = NAMESPACE
	port5 := coreV1.ServicePort {
		Port : 8080,
		Name: "random3",
	}

	port6 := coreV1.ServicePort {
		Port : 8081,
		Name: "random4",
	}

	ports2 := []coreV1.ServicePort{port5, port6}

	previewService.Spec.Ports = ports2



	serviceNS1 := &coreV1.Service{
		Spec : coreV1.ServiceSpec{
			Selector:selectorMap,
		},
	}
	serviceNS1.Name = "dummy"
	serviceNS1.Namespace = "namespace1"
	port8 := coreV1.ServicePort {
		Port : 8080,
		Name: "random3",
	}

	port9 := coreV1.ServicePort {
		Port : 8081,
		Name: "random4",
	}

	ports12 := []coreV1.ServicePort{port8, port9}
	serviceNS1.Spec.Ports = ports12


	rc.ServiceController.Cache.Put(service1)
	rc.ServiceController.Cache.Put(previewService)
	rc.ServiceController.Cache.Put(activeService)
	rc.ServiceController.Cache.Put(serviceNS1)



	noStratergyRollout:= argo.Rollout{
		Spec: argo.RolloutSpec{Template:coreV1.PodTemplateSpec{
			ObjectMeta: k8sv1.ObjectMeta{Annotations:map[string]string{}},
		}}};
	noStratergyRollout.Namespace =NAMESPACE


	noStratergyRollout.Spec.Strategy = argo.RolloutStrategy{
	}



	bgRolloutNs1:= argo.Rollout{
		Spec: argo.RolloutSpec{Template:coreV1.PodTemplateSpec{
			ObjectMeta: k8sv1.ObjectMeta{Annotations:map[string]string{}},
		}}};

	matchLabel1 := make(map[string] string)
	matchLabel1["app"] ="test"

	labelSelector1 := v12.LabelSelector{
		MatchLabels:matchLabel,
	}
	bgRolloutNs1.Spec.Selector = &labelSelector1

	bgRolloutNs1.Namespace ="namespace1"
	bgRolloutNs1.Spec.Strategy = argo.RolloutStrategy{
		BlueGreen: &argo.BlueGreenStrategy{
			ActiveService:SERVICENAME,
			PreviewService:"previewService",
		},
	}

	testCases := []struct {
		name string
		rollout         *argo.Rollout
		rc             	*RemoteController
		result          string
	}{
		{
			name : "canaryRolloutNoLabelMatch",
			rollout :&bgRolloutNs1,
			rc : rc,
			result: "",
		},{
			name : "canaryRolloutNoStratergy",
			rollout :&noStratergyRollout,
			rc : rc,
			result: "",
		},{
			name : "canaryRolloutHappyCase",
			rollout :&bgRollout,
			rc : rc,
			result: SERVICENAME,
		},
		{
			name : "canaryRolloutNilRollout",
			rollout :nil,
			rc : rc,
			result: "",
		},
		{
			name : "canaryRolloutEmptyServiceCache",
			rollout :&bgRollout,
			rc : &RemoteController{
				ServiceController:emptyCacheService,
			},
			result: "",
		},

	}


	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := getServiceForRollout(c.rc,c.rollout)
			if len(c.result) ==0 {
				if result != nil {
					t.Fatalf("Service expected to be nil")
				}
			}else
			if !cmp.Equal(result.Name, c.result) {
				t.Fatalf("Service Mismatch. Diff: %v", cmp.Diff(result, c.name))
			}
		})
	}
}

func TestHandleDependencyRecord(t *testing.T){

	fakeClient := fake.NewSimpleClientset()

	stop := make(chan struct{})
	config := rest.Config{
		Host: "localhost",
	}

	labelset := common.LabelSet{
		DeploymentAnnotation: "sidecar.istio.io/inject",
		AdmiralIgnoreLabel:   "admiral-ignore",
	}

	ns := coreV1.Namespace{}
	ns.Name ="namespace"
	ns.Namespace ="namespace"


	r, e := admiral.NewRolloutsControllerWithLabelOverride(stop, &test.MockRolloutHandler{}, &config, time.Second*time.Duration(300),&labelset)
	s, e := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300))
	d,e := admiral.NewDeploymentControllerWithLabelOverride(stop, &test.MockDeploymentHandler{},&config,time.Second*time.Duration(300),&labelset)
	n,e := admiral.NewNodeController(stop,&test.MockNodeHandler{},&config)
	noRolloutsClient := argofake.NewSimpleClientset().ArgoprojV1alpha1()
	fakeClient.CoreV1().Namespaces().Create(&ns)

	if e != nil {
		t.Fatalf("Inititalization failed")
	}

	r.K8sClient =fakeClient
	r.RolloutClient =noRolloutsClient
	d.K8sClient = fakeClient

	remoteController := &RemoteController{}
	remoteController.DeploymentController = d
	remoteController.RolloutController = r
	remoteController.ServiceController = s
	remoteController.NodeController = n

	usecase1Rcs := make(map[string]*RemoteController)
	usecase1Rcs["cluster-1"] =remoteController

	registry := &RemoteRegistry{}
	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}

	cacheWithEntry := ServiceEntryAddressStore{
		EntryAddresses: map[string]string{"qal.greeting.mesh-se": common.LocalAddressPrefix + ".10.1"},
		Addresses: []string{common.LocalAddressPrefix + ".10.1"},
	}
	cacheWithEntry.EntryAddresses["qal.payments.mesh-se"] =common.LocalAddressPrefix+"10.2"

	admiralCache := &AdmiralCache{
		IdentityClusterCache: common.NewMapOfMaps(),
		ServiceEntryAddressStore : &cacheWithEntry,
		CnameClusterCache: common.NewMapOfMaps(),
		CnameIdentityCache: & sync.Map{},
		CnameDependentClusterCache: common.NewMapOfMaps(),
	}

	registry.AdmiralCache = admiralCache

	rollout := argo.Rollout{}
	rollout.Namespace = "namespace"
	rollout.Name = "fake-app-rollout-qal"
	rollout.Spec = argo.RolloutSpec{
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: k8sv1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env":"qal"},
				Annotations: map[string]string{"sidecar.istio.io/inject":"true"},
			},
		},
	}
	rollout.Labels = map[string]string{"identity": "app1"}



	greetingRollout := argo.Rollout{}
	greetingRollout.Namespace = "namespace"
	greetingRollout.Name = "greetingRollout-qal"
	greetingRollout.Spec = argo.RolloutSpec{
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: k8sv1.ObjectMeta{
				Labels: map[string]string{"identity": "greeting", "env":"qal"},
				Annotations: map[string]string{"sidecar.istio.io/inject":"true"},
			},
		},
	}
	greetingRollout.Labels = map[string]string{"identity": "greeting"}


	paymentDeployment := k8sAppsV1.Deployment{}
	paymentDeployment.Namespace = "namespace"
	paymentDeployment.Name = "payments-qal"
	paymentDeployment.Spec = k8sAppsV1.DeploymentSpec{
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: k8sv1.ObjectMeta{
				Labels: map[string]string{"identity": "payments", "env":"qal"},
				Annotations: map[string]string{"sidecar.istio.io/inject":"true"},
			},
		},
	}
	paymentDeployment.Labels = map[string]string{"identity": "payments"}


	r.Added(&rollout)
	r.Added(&greetingRollout)

	d.Added(&paymentDeployment)

	dependency := model.Dependency{IdentityLabel: "identity", Destinations:[]string{"greeting", "payments", "newservice"}, Source: "webapp"}


	testCases := []struct {
		name string
		sourceIdentity string
		dep             *v1.Dependency
		remoteRegistry *RemoteRegistry
	}{
		{
			name : "handleDependencyRecord-NoDependency",
			sourceIdentity:"app1",
			dep: &v1.Dependency{ Spec:dependency},
			remoteRegistry:registry,
		},

	}
	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			handleDependencyRecord(c.sourceIdentity,c.remoteRegistry,c.remoteRegistry.remoteControllers,c.dep)
		})
	}
}