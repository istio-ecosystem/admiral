package admiral

import (
	"github.com/google/go-cmp/cmp"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestNewGlobalTrafficController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockGlobalTrafficHandler{}

	globalTrafficController, err := NewGlobalTrafficController(stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if globalTrafficController == nil {
		t.Errorf("GlobalTraffic controller should never be nil without an error thrown")
	}
}

func TestGlobalTrafficAddUpdateDelete(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockGlobalTrafficHandler{}

	globalTrafficController, err := NewGlobalTrafficController(stop, &handler, config, time.Duration(1000))

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	if globalTrafficController == nil {
		t.Errorf("GlobalTraffic controller should never be nil without an error thrown")
	}

	gtpName := "gtp1"
	gtp := model.GlobalTrafficPolicy{Selector: map[string]string{"identity": "payments", "env": "e2e"}, Policy: []*model.TrafficPolicy{}}
	gtpObj := makeK8sGtpObj(gtpName, "namespace1", gtp)
	globalTrafficController.Added(gtpObj)

	if !cmp.Equal(handler.Obj.Spec, gtpObj.Spec) {
		t.Errorf("Add should call the handler with the object")
	}

	updatedGtp := model.GlobalTrafficPolicy{Selector: map[string]string{"identity": "payments", "env": "qa"}, Policy: []*model.TrafficPolicy{}}
	updatedGtpObj := makeK8sGtpObj(gtpName, "namespace1", updatedGtp)

	globalTrafficController.Updated(updatedGtpObj, gtpObj)

	if !cmp.Equal(handler.Obj.Spec, updatedGtpObj.Spec) {
		t.Errorf("Update should call the handler with the updated object")
	}

	globalTrafficController.Deleted(updatedGtpObj)

	if handler.Obj != nil {
		t.Errorf("Delete should delete the gtp")
	}

}

func TestGlobalTrafficController_Updated(t *testing.T) {

	var (

		gth = test.MockGlobalTrafficHandler{}
		cache = gtpCache{
			cache: make(map[string]map[string]map[string]*v1.GlobalTrafficPolicy),
			mutex: &sync.Mutex{},
		}
		gtpController = GlobalTrafficController{
			GlobalTrafficHandler: &gth,
			Cache:             &cache,
		}
		gtp = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy {{DnsPrefix: "hello"}},
		},}
		gtpUpdated = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy {{DnsPrefix: "helloUpdated"}},
		},}
		gtpUpdatedToIgnore = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}, Annotations: map[string]string{"admiral.io/ignore": "true"}}}

	)

	//add the base object to cache
	gtpController.Added(&gtp)

	testCases := []struct {
		name                  string
		gtp            		  *v1.GlobalTrafficPolicy
		expectedGtps 		  []*v1.GlobalTrafficPolicy
	}{
		{
			name:                  	"Gtp with should be updated",
			gtp:            		&gtpUpdated,
			expectedGtps: 			[]*v1.GlobalTrafficPolicy{&gtpUpdated},
		},
		{
			name:                  	"Should remove gtp from cache when update with Ignore annotation",
			gtp:            		&gtpUpdatedToIgnore,
			expectedGtps: 			[]*v1.GlobalTrafficPolicy{},
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtpController.Updated(c.gtp, gtp)
			gtpKey := common.GetGtpKey(c.gtp)
			matchedGtps := gtpController.Cache.Get(gtpKey, c.gtp.Namespace)
			if !reflect.DeepEqual(c.expectedGtps, matchedGtps) {
				t.Errorf("Test %s failed; expected %v, got %v", c.name, c.expectedGtps, matchedGtps)
			}
		})
	}
}

func TestGlobalTrafficController_Deleted(t *testing.T) {

	var (
		gth = test.MockGlobalTrafficHandler{}
		cache = gtpCache{
			cache: make(map[string]map[string]map[string]*v1.GlobalTrafficPolicy),
			mutex: &sync.Mutex{},
		}
		gtpController = GlobalTrafficController{
			GlobalTrafficHandler: &gth,
			Cache:             &cache,
		}
		gtp = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy {{DnsPrefix: "hello"}},
		},}

		gtp2 = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp2", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy {{DnsPrefix: "hellogtp2"}},
		},}

		gtp3 = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp3", Namespace: "namespace2", Labels: map[string]string{"identity": "id2", "admiral.io/env": "stage"}}, Spec: model.GlobalTrafficPolicy{
			Policy: []*model.TrafficPolicy {{DnsPrefix: "hellogtp3"}},
		},}
	)

	//add the base object to cache
	gtpController.Added(&gtp)
	gtpController.Added(&gtp2)

	testCases := []struct {
		name                  string
		gtp            		  *v1.GlobalTrafficPolicy
		expectedGtps 		  []*v1.GlobalTrafficPolicy
	}{
		{
			name:                  	"Should delete gtp",
			gtp:            		&gtp,
			expectedGtps: 			[]*v1.GlobalTrafficPolicy{&gtp2},
		},
		{
			name:                  	"Deleting non existing gtp should be a no-op",
			gtp:            		&gtp3,
			expectedGtps: 			[]*v1.GlobalTrafficPolicy{},
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtpController.Deleted(c.gtp)
			gtpKey := common.GetGtpKey(c.gtp)
			matchedGtps := gtpController.Cache.Get(gtpKey, c.gtp.Namespace)
			if !reflect.DeepEqual(c.expectedGtps, matchedGtps) {
				t.Errorf("Test %s failed; expected %v, got %v", c.name, c.expectedGtps, matchedGtps)
			}
		})
	}
}

func TestGlobalTrafficController_Added(t *testing.T) {

	var (

		gth = test.MockGlobalTrafficHandler{}
		cache = gtpCache{
			cache: make(map[string]map[string]map[string]*v1.GlobalTrafficPolicy),
			mutex: &sync.Mutex{},
		}
		gtpController = GlobalTrafficController{
			GlobalTrafficHandler: &gth,
			Cache:             &cache,
		}
		gtp = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}},}
		gtpWithIgnoreLabels = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtpWithIgnoreLabels", Namespace: "namespace2", Labels: map[string]string{"identity": "id2", "admiral.io/env": "stage"}, Annotations: map[string]string{"admiral.io/ignore": "true"}}}

		gtp2 = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp2", Namespace: "namespace1", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}}

		gtp3 = v1.GlobalTrafficPolicy{ObjectMeta: v12.ObjectMeta{Name: "gtp3", Namespace: "namespace3", Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}}}
	)

	testCases := []struct {
		name                  string
		gtp            		  *v1.GlobalTrafficPolicy
		expectedGtps 		  []*v1.GlobalTrafficPolicy
	}{
		{
			name:                  	"Gtp should be added to the cache",
			gtp:            		&gtp,
			expectedGtps: 			[]*v1.GlobalTrafficPolicy{&gtp},
		},
		{
			name:                  	"Gtp with ignore annotation should not be added to the cache",
			gtp:            		&gtpWithIgnoreLabels,
			expectedGtps: 			[]*v1.GlobalTrafficPolicy{},
		},
		{
			name:                  	"Should cache multiple gtps in a namespace",
			gtp:            		&gtp2,
			expectedGtps: 			[]*v1.GlobalTrafficPolicy{&gtp, &gtp2},
		},
		{
			name:                  	"Should cache gtps in from multiple namespaces",
			gtp:            		&gtp3,
			expectedGtps: 			[]*v1.GlobalTrafficPolicy{&gtp3},
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtpController.Added(c.gtp)
			gtpKey := common.GetGtpKey(c.gtp)
			matchedGtps := gtpController.Cache.Get(gtpKey, c.gtp.Namespace)
			if !reflect.DeepEqual(c.expectedGtps, matchedGtps) {
				t.Errorf("Test %s failed; expected %v, got %v", c.name, c.expectedGtps, matchedGtps)
			}
		})
	}
}

func makeK8sGtpObj(name string, namespace string, gtp model.GlobalTrafficPolicy) *v1.GlobalTrafficPolicy {
	return &v1.GlobalTrafficPolicy{
		Spec:       gtp,
		ObjectMeta: v12.ObjectMeta{Name: name, Namespace: namespace},
		TypeMeta: v12.TypeMeta{
			APIVersion: "admiral.io/v1",
			Kind:       "GlobalTrafficPolicy",
		}}
}
