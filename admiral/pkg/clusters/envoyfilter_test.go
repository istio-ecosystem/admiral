package clusters

import (
	"context"
	"errors"
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
	"sync"
	"testing"
	"time"
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
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.GlobalTrafficDeploymentLabel = "identity"

	registry, _ := InitAdmiral(context.Background(), p)

	handler := RoutingPolicyHandler{}

	rpFilterCache := &routingPolicyFilterCache{}
	rpFilterCache.filterCache = make(map[string]map[string]map[string]string)
	rpFilterCache.mutex = &sync.Mutex{}

	routingPolicyController := &admiral.RoutingPolicyController{IstioClient: istiofake.NewSimpleClientset()}
	remoteController, _ := createMockRemoteController(func(i interface{}) {

	})

	remoteController.RoutingPolicyController = routingPolicyController

	registry.RemoteControllers = map[string]*RemoteController{"cluster-1": remoteController}
	registry.AdmiralCache.RoutingPolicyFilterCache = rpFilterCache

	// foo is dependent upon bar and bar has a deployment in the same cluster.
	registry.AdmiralCache.IdentityDependencyCache.Put("foo", "bar", "bar")
	registry.AdmiralCache.IdentityClusterCache.Put("bar", remoteController.ClusterID, remoteController.ClusterID)


	handler.RemoteRegistry = registry

	routingPolicyFoo := &v1.RoutingPolicy{
		TypeMeta:   time2.TypeMeta{},
		ObjectMeta: time2.ObjectMeta{
			Labels: map[string]string{
				"identity": "foo",
				"admiral.io/env": "stage",
			},
		},
		Spec:       model.RoutingPolicy{
			Plugin:               "test",
			Hosts:                []string{"e2e.testservice.mesh"},
			Config: map[string]string{
				"cachePrefix": "cache-v1",
				"cachettlSec": "86400",
				"routingServiceUrl": "e2e.test.routing.service.mesh",
				"pathPrefix": "/sayhello,/v1/company/{id}/",
			},
		},
		Status:     v1.RoutingPolicyStatus{},
	}

	getSha1 = getSha1Error

	envoyfilter, err := createOrUpdateEnvoyFilter(remoteController, routingPolicyFoo, admiral.Add, "barstage", registry.AdmiralCache)

	assert.NotNil(t, err)
	assert.Nil(t, envoyfilter)

	getSha1 = common.GetSha1

	envoyfilter, err = createOrUpdateEnvoyFilter(remoteController, routingPolicyFoo, admiral.Add, "bar", registry.AdmiralCache)
	assert.Equal(t, "bar", envoyfilter.Spec.WorkloadSelector.GetLabels()["identity"])
	assert.Equal(t, "stage", envoyfilter.Spec.WorkloadSelector.GetLabels()["admiral.io/env"])
	assert.Equal(t, "test-dynamicrouting-d0fdd-1.10", envoyfilter.Name)

	envoyfilter, err = createOrUpdateEnvoyFilter(remoteController, routingPolicyFoo, admiral.Update, "bar", registry.AdmiralCache)
	assert.Nil(t, err)


	remoteController.RoutingPolicyController.IstioClient.NetworkingV1alpha3().(*fake.FakeNetworkingV1alpha3).PrependReactor("create", "envoyfilters",
		func(action testing2.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("error creating envoyfilter")
		},
	)
	envoyfilter3, err := createOrUpdateEnvoyFilter(remoteController, routingPolicyFoo, admiral.Add, "bar2", registry.AdmiralCache)
	assert.NotNil(t, err)
	assert.Nil(t, envoyfilter3)


}

func getSha1Error (key interface{}) (string, error) {
	return "", errors.New("error occured while computing the sha")
}
