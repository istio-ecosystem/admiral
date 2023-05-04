package clusters

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	"istio.io/client-go/pkg/clientset/versioned/typed/networking/v1alpha3/fake"
	time2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	testing2 "k8s.io/client-go/testing"
)

func TestCreateOrUpdateEnvoyFilter(t *testing.T) {
	p := common.AdmiralParams{
		KubeconfigPath:             "testdata/fake.config",
		LabelSet:                   &common.LabelSet{},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheRefreshDuration:       time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace:      "default",
		SecretResolver:             "",
		EnvoyFilterVersion:         "1.13",
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.GlobalTrafficDeploymentLabel = "identity"

	common.ResetSync()
	registry, _ := InitAdmiral(context.Background(), p)

	handler := RoutingPolicyHandler{}

	rpFilterCache := &routingPolicyFilterCache{}
	rpFilterCache.filterCache = make(map[string]map[string]map[string]string)
	rpFilterCache.mutex = &sync.Mutex{}

	routingPolicyController := &admiral.RoutingPolicyController{IstioClient: istiofake.NewSimpleClientset()}
	remoteController, _ := createMockRemoteController(func(i interface{}) {

	})

	remoteController.RoutingPolicyController = routingPolicyController

	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}
	registry.AdmiralCache.RoutingPolicyFilterCache = rpFilterCache

	// foo is dependent upon bar and bar has a deployment in the same cluster.
	registry.AdmiralCache.IdentityDependencyCache.Put("foo", "bar", "bar")
	registry.AdmiralCache.IdentityClusterCache.Put("bar", remoteController.ClusterID, remoteController.ClusterID)

	handler.RemoteRegistry = registry

	routingPolicyFoo := &v1.RoutingPolicy{
		TypeMeta: time2.TypeMeta{},
		ObjectMeta: time2.ObjectMeta{
			Labels: map[string]string{
				"identity":       "foo",
				"admiral.io/env": "stage",
			},
		},
		Spec: model.RoutingPolicy{
			Plugin: "test",
			Hosts:  []string{"e2e.testservice.mesh"},
			Config: map[string]string{
				"cachePrefix":       "cache-v1",
				"cachettlSec":       "86400",
				"routingServiceUrl": "e2e.test.routing.service.mesh",
				"pathPrefix":        "/sayhello,/v1/company/{id}/",
			},
		},
		Status: v1.RoutingPolicyStatus{},
	}

	selectors := map[string]string{"one": "test1", "two": "test2"}

	getSha1 = getSha1Error

	ctx := context.Background()

	envoyfilter, err := createOrUpdateEnvoyFilter(ctx, remoteController, routingPolicyFoo, admiral.Add, "barstage", registry.AdmiralCache, selectors)

	assert.NotNil(t, err)
	assert.Nil(t, envoyfilter)

	getSha1 = common.GetSha1

	envoyfilter, err = createOrUpdateEnvoyFilter(ctx, remoteController, routingPolicyFoo, admiral.Add, "bar", registry.AdmiralCache, selectors)
	assert.Equal(t, "test1", envoyfilter.Spec.WorkloadSelector.GetLabels()["one"])
	assert.Equal(t, "test2", envoyfilter.Spec.WorkloadSelector.GetLabels()["two"])
	assert.Equal(t, "test-dynamicrouting-d0fdd-1.13", envoyfilter.Name)

	envoyfilter, err = createOrUpdateEnvoyFilter(ctx, remoteController, routingPolicyFoo, admiral.Update, "bar", registry.AdmiralCache, selectors)
	assert.Nil(t, err)

	remoteController.RoutingPolicyController.IstioClient.NetworkingV1alpha3().(*fake.FakeNetworkingV1alpha3).PrependReactor("create", "envoyfilters",
		func(action testing2.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("error creating envoyfilter")
		},
	)
	envoyfilter3, err := createOrUpdateEnvoyFilter(ctx, remoteController, routingPolicyFoo, admiral.Add, "bar2", registry.AdmiralCache, selectors)
	assert.NotNil(t, err)
	assert.Nil(t, envoyfilter3)

	//TODO: Convert to table tests
	envoyfiltervmid, err := createOrUpdateEnvoyFilter(ctx, remoteController, routingPolicyFoo, admiral.Add, "bar", registry.AdmiralCache, selectors)
	vm_id := envoyfiltervmid.Spec.ConfigPatches[0].Patch.Value.Fields["typed_config"].GetStructValue().Fields["value"].GetStructValue().Fields["config"].GetStructValue().Fields["vm_config"].GetStructValue().Fields["vm_id"].GetStringValue()
	assert.Equal(t, "test", vm_id)
}

func getSha1Error(key interface{}) (string, error) {
	return "", errors.New("error occured while computing the sha")
}

func TestGetHosts(t *testing.T) {
	routingPolicyFoo := &v1.RoutingPolicy{
		TypeMeta: time2.TypeMeta{},
		ObjectMeta: time2.ObjectMeta{
			Labels: map[string]string{
				"identity":       "foo",
				"admiral.io/env": "stage",
			},
		},
		Spec: model.RoutingPolicy{
			Plugin: "test",
			Hosts:  []string{"e2e.testservice.mesh,e2e2.testservice.mesh"},
			Config: map[string]string{
				"cachePrefix":       "cache-v1",
				"cachettlSec":       "86400",
				"routingServiceUrl": "e2e.test.routing.service.mesh",
				"pathPrefix":        "/sayhello,/v1/company/{id}/",
			},
		},
		Status: v1.RoutingPolicyStatus{},
	}

	hosts, err := getHosts(routingPolicyFoo)
	if err != nil {
		assert.Fail(t, err.Error())
	}
	assert.Equal(t, "hosts: e2e.testservice.mesh,e2e2.testservice.mesh", hosts)
}

func TestGetPlugin(t *testing.T) {
	routingPolicyFoo := &v1.RoutingPolicy{
		TypeMeta: time2.TypeMeta{},
		ObjectMeta: time2.ObjectMeta{
			Labels: map[string]string{
				"identity":       "foo",
				"admiral.io/env": "stage",
			},
		},
		Spec: model.RoutingPolicy{
			Plugin: "test",
			Hosts:  []string{"e2e.testservice.mesh,e2e2.testservice.mesh"},
			Config: map[string]string{
				"cachePrefix":       "cache-v1",
				"cachettlSec":       "86400",
				"routingServiceUrl": "e2e.test.routing.service.mesh",
				"pathPrefix":        "/sayhello,/v1/company/{id}/",
			},
		},
		Status: v1.RoutingPolicyStatus{},
	}

	plugin, err := getPlugin(routingPolicyFoo)
	if err != nil {
		assert.Fail(t, err.Error())
	}
	assert.Equal(t, "plugin: test", plugin)
}
