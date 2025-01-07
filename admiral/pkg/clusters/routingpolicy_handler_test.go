package clusters

import (
	"bytes"
	"context"
	"errors"
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

const envoyFilterVersion_1_13 = "1.13"

func TestNewRoutingPolicyProcessor(t *testing.T) {
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
		RoutingPolicyClusters:      []string{"*"},
		EnvoyFilterVersion:         envoyFilterVersion_1_13,
		Profile:                    common.AdmiralProfileDefault,
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"

	registry, _ := InitAdmiral(context.Background(), p)

	rpFilterCache := &routingPolicyFilterCache{}
	rpFilterCache.filterCache = make(map[string]map[string]map[string]string)
	rpFilterCache.mutex = &sync.Mutex{}

	routingPolicyController := &admiral.RoutingPolicyController{IstioClient: istiofake.NewSimpleClientset()}
	bar1RCtrl, _ := createMockRemoteController(func(i interface{}) {})

	bar1RCtrl.RoutingPolicyController = routingPolicyController
	registry.remoteControllers = map[string]*RemoteController{"cluster-1": bar1RCtrl}
	registry.AdmiralCache.RoutingPolicyFilterCache = rpFilterCache

	registry.AdmiralCache.IdentityClusterCache.Put("bar", bar1RCtrl.ClusterID, bar1RCtrl.ClusterID)
	registry.AdmiralCache.IdentityClusterCache.Put("bar2", "differentCluster", "differentCluster")

	// foo is dependent upon bar and bar has a deployment in the same cluster.
	registry.AdmiralCache.IdentityDependencyCache.Put("foo", "bar", "bar")

	// foo2 is also dependent upon bar2 but bar2 is in a different(non-existent) cluster, so this cluster should not have the envoyfilter created
	registry.AdmiralCache.IdentityDependencyCache.Put("foo2", "bar2", "bar2")

	// foo1 is dependent upon bar 1 but bar1 does not have a deployment so it is missing from identityClusterCache
	registry.AdmiralCache.IdentityDependencyCache.Put("foo1", "bar1", "bar1")

	type args struct {
		rr    *RemoteRegistry
		et    admiral.EventType
		newRP *admiralV1.RoutingPolicy
		oldRP *admiralV1.RoutingPolicy
		dp    map[string]string
	}
	type want struct {
		err                               error
		expectedFilterCacheKey            string
		expectedFilterCount               int
		expectedEnvoyFilterConfigPatchVal map[string]interface{}
	}
	update1FooRP := &admiralV1.RoutingPolicy{
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

	foo1RP := update1FooRP.DeepCopy()
	foo1RP.Labels[common.GetWorkloadIdentifier()] = "foo1"

	foo2RP := update1FooRP.DeepCopy()
	foo1RP.Labels[common.GetWorkloadIdentifier()] = "foo2"

	efnFooRp := envoyFilterName(update1FooRP, "bar")

	testCases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "New routing policy for an existing deployment of foo",
			args: args{
				rr:    registry,
				et:    admiral.Add,
				newRP: update1FooRP,
				dp:    map[string]string{"bar": "bar"},
			},
			want: want{
				expectedFilterCacheKey: "rpfoofoodev",
				expectedFilterCount:    1,
				expectedEnvoyFilterConfigPatchVal: map[string]interface{}{"name": "dynamicRoutingFilterPatch", "typed_config": map[string]interface{}{
					"@type": "type.googleapis.com/udpa.type.v1.TypedStruct", "type_url": "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm",
					"value": map[string]interface{}{
						"config": map[string]interface{}{
							"configuration": map[string]interface{}{
								"@type": "type.googleapis.com/google.protobuf.StringValue",
								"value": "routingServiceUrl: e2e.test.routing.service.mesh\nhosts: e2e.testservice.mesh\nplugin: test"},
							"vm_config": map[string]interface{}{"code": map[string]interface{}{"local": map[string]interface{}{"filename": ""}}, "runtime": "envoy.wasm.runtime.v8", "vm_id": efnFooRp}}}}},
			},
		},
		{
			name: "New routing policy but dependency cluster is missing",
			args: args{
				rr:    registry,
				et:    admiral.Add,
				newRP: foo1RP,
				dp:    map[string]string{"bar1": "bar1"},
			},
			want: want{
				expectedFilterCacheKey: "rpfoofoodev",
				expectedFilterCount:    0,
			},
		},
		{
			name: "New routing policy and known cluster containing the deployment",
			args: args{
				rr:    registry,
				et:    admiral.Add,
				newRP: foo2RP,
				dp:    map[string]string{"bar2": "bar2"},
			},
			want: want{
				expectedFilterCacheKey: "rpfoofoodev",
				expectedFilterCount:    0,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// routing policy cache is empty
			registry.AdmiralCache.RoutingPolicyCache = NewRoutingPolicyCache()
			// routing policy cache is empty
			if tc.args.oldRP != nil {
				id := common.GetRoutingPolicyIdentity(tc.args.oldRP)
				env := common.GetRoutingPolicyEnv(tc.args.oldRP)
				name := tc.args.oldRP.Name
				registry.AdmiralCache.RoutingPolicyCache.Put(id, env, name, tc.args.oldRP)
			}
			p := NewRoutingPolicyProcessor(tc.args.rr)
			ae := p.ProcessAddOrUpdate(ctx, tc.args.et, tc.args.newRP, tc.args.oldRP, tc.args.dp)
			if ae != nil && !errors.Is(ae, tc.want.err) {
				t.Errorf("NewRoutingPolicyProcessor() = %v, want %v", ae, tc.want.err)
				t.Fail()
			}
			list1, _ := bar1RCtrl.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").List(ctx, metaV1.ListOptions{})
			assert.Equal(t, tc.want.expectedFilterCount, len(list1.Items))

			if tc.want.expectedFilterCount > 0 {
				receivedEnvoyFilter, _ := bar1RCtrl.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").Get(ctx, efnFooRp, metaV1.GetOptions{})
				eq := reflect.DeepEqual(tc.want.expectedEnvoyFilterConfigPatchVal, receivedEnvoyFilter.Spec.ConfigPatches[0].Patch.Value.AsMap())
				assert.True(t, eq)
			}

			// once the routing policy is deleted, the corresponding filter should also be deleted
			p.Delete(ctx, admiral.Delete, tc.args.newRP)
			assert.Nil(t, registry.AdmiralCache.RoutingPolicyFilterCache.Get(tc.want.expectedFilterCacheKey))
			assert.Nil(t, registry.AdmiralCache.RoutingPolicyCache.Get("foo", "dev", tc.args.newRP.Name))

		})
	}
}

func TestRoutingPolicyProcess_DependencyUpdate_AddOrUpdate(t *testing.T) {
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
		RoutingPolicyClusters:      []string{"cluster-1"},
		EnvoyFilterVersion:         envoyFilterVersion_1_13,
		Profile:                    common.AdmiralProfileDefault,
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"

	registry, _ := InitAdmiral(context.Background(), p)
	registry.AdmiralCache.SourceToDestinations = &sourceToDestinations{
		sourceDestinations: map[string][]string{
			"foo": {"bar"},
		},
		mutex: &sync.Mutex{},
	}

	routingPolicyController := &admiral.RoutingPolicyController{IstioClient: istiofake.NewSimpleClientset()}
	allowedCluster, _ := createMockRemoteController(func(i interface{}) {})
	allowedCluster.ClusterID = "cluster-1"

	disabledCluster, _ := createMockRemoteController(func(i interface{}) {})
	disabledCluster.ClusterID = "cluster-2"

	rpFilterCache := &routingPolicyFilterCache{}
	rpFilterCache.filterCache = make(map[string]map[string]map[string]string)
	rpFilterCache.mutex = &sync.Mutex{}

	allowedCluster.RoutingPolicyController = routingPolicyController
	registry.remoteControllers = map[string]*RemoteController{"cluster-1": allowedCluster, "cluster-2": disabledCluster}
	registry.AdmiralCache.RoutingPolicyFilterCache = rpFilterCache

	registry.AdmiralCache.IdentityDependencyCache.Put("foo", "bar", "bar")
	registry.AdmiralCache.IdentityClusterCache.Put("bar", allowedCluster.ClusterID, allowedCluster.ClusterID)
	registry.AdmiralCache.IdentityClusterCache.Put("bar2", allowedCluster.ClusterID, allowedCluster.ClusterID)
	registry.AdmiralCache.IdentityClusterCache.Put("baz", disabledCluster.ClusterID, disabledCluster.ClusterID)

	fooRP := &admiralV1.RoutingPolicy{
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
	registry.AdmiralCache.RoutingPolicyCache.Put("foo", "dev", "foo", fooRP)

	type args struct {
		rr         *RemoteRegistry
		dependency *admiralV1.Dependency
	}
	type want struct {
		err                                         error
		expectedFilterCacheKey                      string
		filterCountInCluster1                       int
		filterCountInCluster2                       int
		expectedEnvoyFilterConfigPatchValInCluster1 map[string]interface{}
	}

	efn := envoyFilterName(fooRP, "bar2")

	testCases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "Adding a new destination to dependency in an enabled cluster adds the filter",
			args: args{
				rr: registry,
				dependency: &admiralV1.Dependency{
					TypeMeta: metaV1.TypeMeta{},
					ObjectMeta: metaV1.ObjectMeta{
						Name: "foo-dep",
						Labels: map[string]string{
							"identity":       "bar2",
							"admiral.io/env": "dev",
						},
					},
					Spec: model.Dependency{
						Source:        "bar2",
						IdentityLabel: "identity",
						Destinations:  []string{"foo"},
					},
				},
			},
			want: want{
				filterCountInCluster1:  1,
				expectedFilterCacheKey: "rpfoofoodev",
				expectedEnvoyFilterConfigPatchValInCluster1: map[string]interface{}{"name": "dynamicRoutingFilterPatch", "typed_config": map[string]interface{}{
					"@type": "type.googleapis.com/udpa.type.v1.TypedStruct", "type_url": "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm",
					"value": map[string]interface{}{
						"config": map[string]interface{}{
							"configuration": map[string]interface{}{
								"@type": "type.googleapis.com/google.protobuf.StringValue",
								"value": "routingServiceUrl: e2e.test.routing.service.mesh\nhosts: e2e.testservice.mesh\nplugin: test"},
							"vm_config": map[string]interface{}{"code": map[string]interface{}{"local": map[string]interface{}{"filename": ""}}, "runtime": "envoy.wasm.runtime.v8", "vm_id": efn}}}}},
			},
		},
		{
			name: "Adding a new destination to dependency in a disabled cluster does nothing",
			args: args{
				rr: registry,
				dependency: &admiralV1.Dependency{
					TypeMeta: metaV1.TypeMeta{},
					ObjectMeta: metaV1.ObjectMeta{
						Name: "foo-dep",
						Labels: map[string]string{
							"identity":       "baz",
							"admiral.io/env": "dev",
						},
					},
					Spec: model.Dependency{
						Source:        "baz",
						IdentityLabel: "identity",
						Destinations:  []string{"foo"},
					},
				},
			},
			want: want{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			processor := NewRoutingPolicyProcessor(tc.args.rr)
			err := processor.ProcessDependency(ctx, admiral.Update, tc.args.dependency)
			if err != nil && tc.want.err == nil {
				t.Errorf("ProcessDependency() unexpected error = %v", err)
			}

			// cluster 1
			list1, _ := allowedCluster.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").List(ctx, metaV1.ListOptions{})
			assert.Equal(t, tc.want.filterCountInCluster1, len(list1.Items))
			if tc.want.filterCountInCluster1 > 0 {

				receivedEnvoyFilter, _ := allowedCluster.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").Get(ctx, efn, metaV1.GetOptions{})
				eq := reflect.DeepEqual(tc.want.expectedEnvoyFilterConfigPatchValInCluster1, receivedEnvoyFilter.Spec.ConfigPatches[0].Patch.Value.AsMap())
				assert.True(t, eq)

				// cleanup
				processor.Delete(ctx, admiral.Delete, fooRP)
				assert.Nil(t, registry.AdmiralCache.RoutingPolicyFilterCache.Get(tc.want.expectedFilterCacheKey))
				assert.Nil(t, registry.AdmiralCache.RoutingPolicyCache.Get("foo", "dev", fooRP.Name))
			}

			// cluster 2
			list2, _ := disabledCluster.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").List(ctx, metaV1.ListOptions{})
			assert.Equal(t, tc.want.filterCountInCluster2, len(list2.Items))
		})
	}
}

func envoyFilterName(fooRP *admiralV1.RoutingPolicy, dependentIdentity string) string {
	name, _ := common.GetSha1(fooRP.Name + common.GetRoutingPolicyEnv(fooRP) + common.GetRoutingPolicyIdentity(fooRP))
	dependentIdentitySha, _ := common.GetSha1(dependentIdentity)
	envoyFilterName := fmt.Sprintf("%s-dr-%s-%s-%s", strings.ToLower(fooRP.Spec.Plugin), name, dependentIdentitySha, envoyFilterVersion_1_13)
	return envoyFilterName
}

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
		RoutingPolicyClusters:      []string{"*"},
		EnvoyFilterVersion:         envoyFilterVersion_1_13,
		Profile:                    common.AdmiralProfileDefault,
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"

	registry, _ := InitAdmiral(context.Background(), p)

	rpFilterCache := &routingPolicyFilterCache{}
	rpFilterCache.filterCache = make(map[string]map[string]map[string]string)
	rpFilterCache.mutex = &sync.Mutex{}

	routingPolicyController := &admiral.RoutingPolicyController{IstioClient: istiofake.NewSimpleClientset()}
	remoteController, _ := createMockRemoteController(func(i interface{}) {})

	remoteController.RoutingPolicyController = routingPolicyController
	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController}
	registry.AdmiralCache.RoutingPolicyFilterCache = rpFilterCache
	registry.AdmiralCache.RoutingPolicyCache = NewRoutingPolicyCache()

	// foo is dependent upon bar and bar has a deployment in the same cluster.
	registry.AdmiralCache.IdentityDependencyCache.Put("foo", "bar", "bar")
	registry.AdmiralCache.IdentityClusterCache.Put("bar", remoteController.ClusterID, remoteController.ClusterID)

	// foo2 is also dependent upon bar2 but bar2 is in a different cluster, so this cluster should not have the envoyfilter created
	registry.AdmiralCache.IdentityDependencyCache.Put("foo2", "bar2", "bar2")
	registry.AdmiralCache.IdentityClusterCache.Put("bar2", "differentCluster", "differentCluster")

	// foo1 is dependent upon bar 1 but bar1 does not have a deployment so it is missing from identityClusterCache
	registry.AdmiralCache.IdentityDependencyCache.Put("foo1", "bar1", "bar1")
	processor := &MockPolicyProcessor{}
	oops := errors.New("oops")
	errProcessor := &MockPolicyProcessor{err: oops}
	fooRP := &admiralV1.RoutingPolicy{
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
	type args struct {
		proc      *MockPolicyProcessor
		eventType admiral.EventType
		rp        *admiralV1.RoutingPolicy
	}
	type want struct {
		err         error
		addCount    int
		delCount    int
		updateCount int
	}
	time.Sleep(time.Second * 10)
	testCases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "Add event is propagated correctly",
			args: args{
				proc:      processor,
				eventType: admiral.Add,
				rp:        fooRP,
			},
			want: want{
				addCount: 1,
			},
		},
		{
			name: "Err on Add event",
			args: args{
				proc:      errProcessor,
				eventType: admiral.Add,
				rp:        fooRP,
			},
			want: want{
				err: oops,
			},
		},
		{
			name: "Update event is propagated correctly",
			args: args{
				proc:      processor,
				eventType: admiral.Update,
				rp:        fooRP,
			},
			want: want{
				updateCount: 1,
			},
		},
		{
			name: "Err on update event",
			args: args{
				proc:      errProcessor,
				eventType: admiral.Update,
				rp:        fooRP,
			},
			want: want{
				err: oops,
			},
		},
		{
			name: "Delete event is propagated correctly",
			args: args{
				proc:      processor,
				eventType: admiral.Delete,
				rp:        fooRP,
			},
			want: want{
				delCount: 1,
			},
		},
		{
			name: "Err on delete event",
			args: args{
				proc:      errProcessor,
				eventType: admiral.Delete,
				rp:        fooRP,
			},
			want: want{
				err: oops,
			},
		},
	}

	ctx := context.Background()

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			handler := NewRoutingPolicyHandler(registry, "", c.args.proc)
			switch c.args.eventType {
			case admiral.Add:
				err := handler.Added(ctx, c.args.rp)
				if c.want.err != nil && !errors.Is(err, c.want.err) {
					t.Errorf("RoutingPolicyHandler.Added() = %v, want %v", err, c.want.err)
					t.Fail()
				}
				if c.want.addCount > 0 {
					assert.Equal(t, c.args.proc.addCount, c.want.addCount)
				}
			case admiral.Update:
				err := handler.Updated(ctx, c.args.rp, nil)
				if c.want.err != nil && !errors.Is(err, c.want.err) {
					t.Errorf("RoutingPolicyHandler.Updated() = %v, want %v", err, c.want.err)
					t.Fail()
				}
				if c.want.addCount > 0 {
					assert.Equal(t, c.args.proc.addCount, c.want.addCount)
				}
			case admiral.Delete:
				err := handler.Deleted(ctx, c.args.rp)
				if c.want.err != nil && !errors.Is(err, c.want.err) {
					t.Errorf("RoutingPolicyHandler.Deleted() = %v, want %v", err, c.want.err)
					t.Fail()
				}
				if c.want.addCount > 0 {
					assert.Equal(t, c.args.proc.addCount, c.want.addCount)
				}
			}
		})
	}
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
		RoutingPolicyClusters:      []string{"*"},
		EnvoyFilterVersion:         envoyFilterVersion_1_13,
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"

	registry, _ := InitAdmiral(context.Background(), p)
	registry.AdmiralCache.RoutingPolicyCache = NewRoutingPolicyCache()

	handler := NewRoutingPolicyHandler(registry, "", NewRoutingPolicyProcessor(registry))

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
			handler.Updated(ctx, c.rp, nil)
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

func TestRoutingPolicyIgnored(t *testing.T) {
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
		RoutingPolicyClusters:      []string{"*"},
		EnvoyFilterVersion:         envoyFilterVersion_1_13,
		Profile:                    common.AdmiralProfileDefault,
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"

	registry, _ := InitAdmiral(context.Background(), p)
	processor := &MockPolicyProcessor{}
	handler := NewRoutingPolicyHandler(registry, "", processor)

	type want struct {
		err         error
		addCount    int
		delCount    int
		updateCount int
	}
	testcases := []struct {
		name string
		rp   *admiralV1.RoutingPolicy
		want want
	}{
		{
			name: "Ignore Routing Policy - Annotation",
			rp: &admiralV1.RoutingPolicy{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: map[string]string{
						"admiral.io/ignore": "true",
					},
				},
			},
			want: want{},
		},
		{
			name: "Ignore Routing Policy - Label",
			rp: &admiralV1.RoutingPolicy{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: map[string]string{
						"admiral.io/ignore": "true",
					},
				},
			},
			want: want{},
		},
	}

	ctx := context.Background()

	for _, c := range testcases {
		t.Run(c.name, func(t *testing.T) {
			handler.Added(ctx, c.rp)
			assert.Equal(t, c.want.addCount, 0)

			// Update routing policy test
			handler.Updated(ctx, c.rp, nil)
			assert.Equal(t, c.want.updateCount, 0)

			// Delete routing policy test
			handler.Deleted(ctx, c.rp)
			assert.Equal(t, c.want.delCount, 0)
		})
	}
}

func TestRoutingPolicyProcessingDisabled(t *testing.T) {
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
		EnableRoutingPolicy:        false,
		RoutingPolicyClusters:      []string{"*"},
		EnvoyFilterVersion:         envoyFilterVersion_1_13,
		Profile:                    common.AdmiralProfileDefault,
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"

	registry, _ := InitAdmiral(context.Background(), p)

	processor := &MockPolicyProcessor{}
	handler := NewRoutingPolicyHandler(registry, "", processor)

	type want struct {
		err         error
		addCount    int
		delCount    int
		updateCount int
	}
	testcases := []struct {
		name string
		rp   *admiralV1.RoutingPolicy
		want want
	}{
		{
			name: "Disabled",
			rp: &admiralV1.RoutingPolicy{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: map[string]string{
						"admiral.io/ignore": "true",
					},
				},
			},
			want: want{},
		},
	}

	ctx := context.Background()

	for _, c := range testcases {
		t.Run(c.name, func(t *testing.T) {
			handler.Added(ctx, c.rp)
			assert.Equal(t, c.want.addCount, 0)

			// Update routing policy test
			handler.Updated(ctx, c.rp, nil)
			assert.Equal(t, c.want.updateCount, 0)

			// Delete routing policy test
			handler.Deleted(ctx, c.rp)
			assert.Equal(t, c.want.delCount, 0)
		})
	}
}

func TestRoutingPolicyCache_Put(t *testing.T) {
	rp := NewRoutingPolicyCache()

	rp.Put("foo", "dev", "rpfoo", nil)
	assert.Nil(t, rp.Get("foo", "dev", "rpfoo"))

	rp.Put("foo", "dev", "rpfoo", &admiralV1.RoutingPolicy{})
	assert.NotNil(t, rp.Get("foo", "dev", "rpfoo"))
}

func TestRoutingPolicyCache_Delete(t *testing.T) {
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
		EnvoyFilterVersion:         envoyFilterVersion_1_13,
		Profile:                    common.AdmiralProfileDefault,
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"

	type args struct {
		readOnly            bool
		enableRoutingPolicy bool
		identity            string
		env                 string
		name                string
	}
	testCases := []struct {
		name string
		args args
	}{
		{
			name: "Admiral should delete from cache",
			args: args{
				readOnly:            false,
				enableRoutingPolicy: true,
				identity:            "foo",
				env:                 "dev",
				name:                "rpfoo",
			},
		},
		{
			name: "Admiral should not delete from cache during readOnly",
			args: args{
				readOnly:            true,
				enableRoutingPolicy: true,
				identity:            "foo",
				env:                 "dev",
				name:                "rpfoo",
			},
		},
		{
			name: "Admiral should not delete from cache when disabled",
			args: args{
				readOnly:            false,
				enableRoutingPolicy: false,
				identity:            "foo",
				env:                 "dev",
				name:                "rpfoo",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			common.ResetSync()
			p.EnableRoutingPolicy = tc.args.enableRoutingPolicy
			InitAdmiral(context.Background(), p)
			commonUtil.CurrentAdmiralState.ReadOnly = tc.args.readOnly
			cache := NewRoutingPolicyCache()
			rp := &admiralV1.RoutingPolicy{}
			cache.Put(tc.args.identity, tc.args.env, tc.args.name, rp)
			cache.Delete(tc.args.identity, tc.args.env, tc.args.name)
			if tc.args.readOnly {
				assert.NotNil(t, cache.Get(tc.args.identity, tc.args.env, tc.args.name))
			} else {
				if tc.args.enableRoutingPolicy {
					assert.Nil(t, cache.Get(tc.args.identity, tc.args.env, tc.args.name))
				} else {
					assert.NotNil(t, cache.Get(tc.args.identity, tc.args.env, tc.args.name))
				}
			}
		})
	}
}

type MockPolicyProcessor struct {
	addCount    int
	depCount    int
	deleteCount int
	err         error
}

func (m *MockPolicyProcessor) ProcessAddOrUpdate(ctx context.Context, eventType admiral.EventType, newRP *admiralV1.RoutingPolicy, oldRP *admiralV1.RoutingPolicy, dependents map[string]string) error {
	m.addCount++
	return m.err
}

func (m *MockPolicyProcessor) ProcessDependency(ctx context.Context, eventType admiral.EventType, dependency *admiralV1.Dependency) error {
	m.depCount++
	return m.err
}

func (m *MockPolicyProcessor) Delete(ctx context.Context, eventType admiral.EventType, routingPolicy *admiralV1.RoutingPolicy) error {
	m.deleteCount++
	return m.err
}
