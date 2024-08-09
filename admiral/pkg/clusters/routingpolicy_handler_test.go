package clusters

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"

	admiralV1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRoutingPolicyHandler(t *testing.T) {
	common.ResetSync()
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			DeploymentAnnotation: "sidecar.istio.io/inject",
		},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheReconcileDuration:     time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace:      "default",
		EnableRoutingPolicy:        true,
		EnvoyFilterVersion:         "1.13",
		Profile:                    common.AdmiralProfileDefault,
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"

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

	// foo is also dependent upon bar2 but bar2 is in a different cluster, so this cluster should not have the envoyfilter created
	registry.AdmiralCache.IdentityDependencyCache.Put("foo2", "bar2", "bar2")
	registry.AdmiralCache.IdentityClusterCache.Put("bar2", "differentCluster", "differentCluster")

	// foo1 is dependent upon bar 1 but bar1 does not have a deployment so it is missing from identityClusterCache
	registry.AdmiralCache.IdentityDependencyCache.Put("foo1", "bar1", "bar1")

	handler.RemoteRegistry = registry

	routingPolicyFoo := &admiralV1.RoutingPolicy{
		TypeMeta: metaV1.TypeMeta{},
		ObjectMeta: metaV1.ObjectMeta{
			Name: "rpfoo",
			Labels: map[string]string{
				"identity":       "foo",
				"admiral.io/env": "dev",
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
		Status: admiralV1.RoutingPolicyStatus{},
	}

	routingPolicyFooTest := &admiralV1.RoutingPolicy{
		TypeMeta: metaV1.TypeMeta{},
		ObjectMeta: metaV1.ObjectMeta{
			Name: "rpfoo",
			Labels: map[string]string{
				"identity":       "foo",
				"admiral.io/env": "dev",
			},
		},
		Spec: model.RoutingPolicy{
			Plugin: "test",
			Hosts:  []string{"e2e.testservice.mesh"},
			Config: map[string]string{
				"routingServiceUrl": "e2e.test.routing.service.mesh",
			},
		},
		Status: admiralV1.RoutingPolicyStatus{},
	}

	routingPolicyFoo1 := routingPolicyFoo.DeepCopy()
	routingPolicyFoo1.Labels[common.GetWorkloadIdentifier()] = "foo1"

	routingPolicyFoo2 := routingPolicyFoo.DeepCopy()
	routingPolicyFoo2.Labels[common.GetWorkloadIdentifier()] = "foo2"

	testCases := []struct {
		name                              string
		routingPolicy                     *admiralV1.RoutingPolicy
		expectedFilterCacheKey            string
		expectedFilterCount               int
		expectedEnvoyFilterConfigPatchVal map[string]interface{}
	}{
		{
			name:                   "If dependent deployment exists, should fetch filter from cache",
			routingPolicy:          routingPolicyFooTest,
			expectedFilterCacheKey: "rpfoofoodev",
			expectedFilterCount:    1,
			expectedEnvoyFilterConfigPatchVal: map[string]interface{}{"name": "dynamicRoutingFilterPatch", "typed_config": map[string]interface{}{
				"@type": "type.googleapis.com/udpa.type.v1.TypedStruct", "type_url": "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm",
				"value": map[string]interface{}{
					"config": map[string]interface{}{
						"configuration": map[string]interface{}{
							"@type": "type.googleapis.com/google.protobuf.StringValue",
							"value": "routingServiceUrl: e2e.test.routing.service.mesh\nhosts: e2e.testservice.mesh\nplugin: test"},
						"vm_config": map[string]interface{}{"code": map[string]interface{}{"local": map[string]interface{}{"filename": ""}}, "runtime": "envoy.wasm.runtime.v8", "vm_id": "test-dr-532221909d5db54fe5f5-f6ce3712830af1b15625-1.13"}}}}},
		},
		{
			name:                   "If dependent deployment does not exist, the filter should not be created ",
			routingPolicy:          routingPolicyFoo1,
			expectedFilterCacheKey: "rpfoofoodev",
			expectedFilterCount:    0,
		},
		{
			name:                   "If dependent deployment exists in a different cluster, the filter should not be created in cluster where dependency isnt there",
			routingPolicy:          routingPolicyFoo2,
			expectedFilterCacheKey: "rpfoofoodev",
			expectedFilterCount:    0,
		},
	}

	ctx := context.Background()

	time.Sleep(time.Second * 30)
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			handler.Added(ctx, c.routingPolicy)
			if c.expectedFilterCount > 0 {
				filterCacheValue := registry.AdmiralCache.RoutingPolicyFilterCache.Get(c.expectedFilterCacheKey)
				assert.NotNil(t, filterCacheValue)
				routingPolicyNameSha, _ := getSha1(c.routingPolicy.Name + common.GetRoutingPolicyEnv(c.routingPolicy) + common.GetRoutingPolicyIdentity(c.routingPolicy))
				dependentIdentitySha, _ := getSha1("bar")
				envoyFilterName := fmt.Sprintf("%s-dr-%s-%s-%s", strings.ToLower(c.routingPolicy.Spec.Plugin), routingPolicyNameSha, dependentIdentitySha, "1.13")

				filterMap := filterCacheValue[remoteController.ClusterID]
				assert.NotNil(t, filterMap)
				assert.NotNil(t, filterMap[envoyFilterName])

				filter, err := remoteController.RoutingPolicyController.IstioClient.NetworkingV1alpha3().
					EnvoyFilters("istio-system").Get(ctx, envoyFilterName, metaV1.GetOptions{})
				assert.Nil(t, err)
				assert.NotNil(t, filter)
			}
			//get envoyfilters from all namespaces
			list1, _ := remoteController.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").List(ctx, metaV1.ListOptions{})
			assert.Equal(t, c.expectedFilterCount, len(list1.Items))
			if c.expectedFilterCount > 0 {
				receivedEnvoyFilter, _ := remoteController.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").Get(ctx, "test-dr-532221909d5db54fe5f5-f6ce3712830af1b15625-1.13", metaV1.GetOptions{})
				eq := reflect.DeepEqual(c.expectedEnvoyFilterConfigPatchVal, receivedEnvoyFilter.Spec.ConfigPatches[0].Patch.Value.AsMap())
				assert.True(t, eq)
			}

			// once the routing policy is deleted, the corresponding filter should also be deleted
			handler.Deleted(ctx, c.routingPolicy)
			assert.Nil(t, registry.AdmiralCache.RoutingPolicyFilterCache.Get(c.expectedFilterCacheKey))
		})
	}

	// ignore the routing policy
	annotations := routingPolicyFoo.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[common.AdmiralIgnoreAnnotation] = "true"
	routingPolicyFoo.SetAnnotations(annotations)

	handler.Updated(ctx, routingPolicyFoo)
	assert.Nil(t, registry.AdmiralCache.RoutingPolicyFilterCache.Get("rpfoofoodev"))
}

func TestRoutingPolicyReadOnly(t *testing.T) {
	p := common.AdmiralParams{
		KubeconfigPath:             "testdata/fake.config",
		LabelSet:                   &common.LabelSet{},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheReconcileDuration:     time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace:      "default",
		EnableRoutingPolicy:        true,
		EnvoyFilterVersion:         "1.13",
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"

	handler := RoutingPolicyHandler{}

	testcases := []struct {
		name      string
		rp        *admiralV1.RoutingPolicy
		readOnly  bool
		doesError bool
	}{
		{
			name:      "Readonly test - Routing Policy",
			rp:        &admiralV1.RoutingPolicy{},
			readOnly:  true,
			doesError: true,
		},
		{
			name:      "Readonly false test - Routing Policy",
			rp:        &admiralV1.RoutingPolicy{},
			readOnly:  false,
			doesError: false,
		},
	}

	ctx := context.Background()

	for _, c := range testcases {
		t.Run(c.name, func(t *testing.T) {
			if c.readOnly {
				commonUtil.CurrentAdmiralState.ReadOnly = true
			} else {
				commonUtil.CurrentAdmiralState.ReadOnly = false
			}
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer func() {
				log.SetOutput(os.Stderr)
			}()
			// Add routing policy test
			handler.Added(ctx, c.rp)
			t.Log(buf.String())
			val := strings.Contains(buf.String(), "skipping read-only mode")
			assert.Equal(t, c.doesError, val)

			// Update routing policy test
			handler.Updated(ctx, c.rp)
			t.Log(buf.String())
			val = strings.Contains(buf.String(), "skipping read-only mode")
			assert.Equal(t, c.doesError, val)

			// Delete routing policy test
			handler.Deleted(ctx, c.rp)
			t.Log(buf.String())
			val = strings.Contains(buf.String(), "skipping read-only mode")
			assert.Equal(t, c.doesError, val)
		})
	}
}
