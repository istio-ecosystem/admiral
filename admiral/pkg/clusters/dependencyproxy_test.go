package clusters

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateVSFromDependencyProxy(t *testing.T) {

	admiralParams := common.AdmiralParams{
		LabelSet:       &common.LabelSet{},
		HostnameSuffix: "global",
		SyncNamespace:  "testns",
	}

	admiralParams.LabelSet.EnvKey = "admiral.io/env"

	common.ResetSync()
	common.InitializeConfig(admiralParams)

	validDependencyProxyObj := &v1.DependencyProxy{
		Spec: model.DependencyProxy{
			Destination: &model.Destination{
				Identity: "test",
				DnsPrefixes: []string{
					"prefix00",
					"prefix01",
				},
				DnsSuffix: "xyz",
			},
			Proxy: &model.Proxy{
				Identity: "testproxy",
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"admiral.io/env": "stage",
			},
		},
	}

	testcases := []struct {
		name                                    string
		expectedError                           error
		dependencyProxyObj                      *v1.DependencyProxy
		dependencyProxyDefaultHostNameGenerator DependencyProxyDefaultHostNameGenerator
		expectedCacheKey                        string
		expectedCachedVSHosts                   []string
		expectedCachedVSDestinationHost         string
	}{
		{
			name:          "Given dependency proxy, when dependencyProxyDefaultHostNameGenerator is nil, func should return an error",
			expectedError: fmt.Errorf("failed to generate proxy VirtualService due to error: dependencyProxyDefaultHostNameGenerator is nil"),
		},
		{
			name:                                    "Given dependency proxy, when dependencyProxyObj is nil, func should return an error",
			expectedError:                           fmt.Errorf("dependencyProxyObj is nil"),
			dependencyProxyDefaultHostNameGenerator: &dependencyProxyDefaultHostNameGenerator{},
		},
		{
			name:                                    "Given dependency proxy, when valid dependencyProxy object is passed, func should add it to cache and not return an error",
			expectedError:                           nil,
			dependencyProxyObj:                      validDependencyProxyObj,
			dependencyProxyDefaultHostNameGenerator: &dependencyProxyDefaultHostNameGenerator{},
			expectedCacheKey:                        "test",
			expectedCachedVSHosts:                   []string{"stage.test.xyz", "prefix00.test.xyz", "prefix01.test.xyz"},
			expectedCachedVSDestinationHost:         "stage.testproxy.global",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {

			vs, err := generateVSFromDependencyProxy(context.Background(), tc.dependencyProxyObj, tc.dependencyProxyDefaultHostNameGenerator)

			if err != nil && tc.expectedError != nil {
				if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("expected error %s got %s", tc.expectedError.Error(), err.Error())
					return
				}
			} else {
				if err != tc.expectedError {
					t.Errorf("expected error %v got %v", tc.expectedError.Error(), err.Error())
					return
				}
			}

			if err == nil {
				if !reflect.DeepEqual(vs.Spec.Hosts, tc.expectedCachedVSHosts) {
					t.Errorf("expected %v got %v", vs.Spec.Hosts, tc.expectedCachedVSHosts)
					return
				}
				if vs.Spec.Http[0].Route[0].Destination.Host != tc.expectedCachedVSDestinationHost {
					t.Errorf("expected %v got %v", vs.Spec.Http[0].Route[0].Destination.Host, tc.expectedCachedVSDestinationHost)
					return
				}
			}

		})
	}

}

func TestUpdateIdentityDependencyProxyCache(t *testing.T) {

	admiralParams := common.AdmiralParams{
		LabelSet:       &common.LabelSet{},
		HostnameSuffix: "global",
		SyncNamespace:  "testns",
	}

	admiralParams.LabelSet.EnvKey = "admiral.io/env"

	common.ResetSync()
	common.InitializeConfig(admiralParams)

	validDependencyProxyObj := &v1.DependencyProxy{
		Spec: model.DependencyProxy{
			Destination: &model.Destination{
				Identity: "test",
				DnsPrefixes: []string{
					"prefix00",
					"prefix01",
				},
				DnsSuffix: "xyz",
			},
			Proxy: &model.Proxy{
				Identity: "testproxy",
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"admiral.io/env": "stage",
			},
		},
	}

	testcases := []struct {
		name                                    string
		expectedError                           error
		cache                                   *dependencyProxyVirtualServiceCache
		dependencyProxyObj                      *v1.DependencyProxy
		dependencyProxyDefaultHostNameGenerator DependencyProxyDefaultHostNameGenerator
		expectedCacheKey                        string
		expectedCachedVSHosts                   []string
		expectedCachedVSDestinationHost         string
	}{
		{
			name:          "Given identityDependencyCache, when dependencyProxyVirtualServiceCache is nil, func should return an error",
			expectedError: fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyVirtualServiceCache is nil"),
		},
		{
			name:          "Given identityDependencyCache, when dependencyProxyVirtualServiceCache.identityVSCache is nil, func should return an error",
			expectedError: fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyVirtualServiceCache.identityVSCache is nil"),
			cache: &dependencyProxyVirtualServiceCache{
				identityVSCache: nil,
			},
		},
		{
			name:          "Given identityDependencyCache, when dependency proxy is nil, func should return an error",
			expectedError: fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyObj is nil"),
			cache: &dependencyProxyVirtualServiceCache{
				identityVSCache: make(map[string]map[string]*v1alpha3.VirtualService),
				mutex:           &sync.Mutex{},
			},
		},
		{
			name:          "Given identityDependencyCache, when dependency proxy's annotation is nil, func should return an error",
			expectedError: fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyObj.Annotations is nil"),
			cache: &dependencyProxyVirtualServiceCache{
				identityVSCache: make(map[string]map[string]*v1alpha3.VirtualService),
				mutex:           &sync.Mutex{},
			},
			dependencyProxyObj: &v1.DependencyProxy{
				Spec: model.DependencyProxy{
					Destination: nil,
				},
			},
		},
		{
			name:          "Given identityDependencyCache, when dependency proxy's destination is nil, func should return an error",
			expectedError: fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyObj.Spec.Destination is nil"),
			cache: &dependencyProxyVirtualServiceCache{
				identityVSCache: make(map[string]map[string]*v1alpha3.VirtualService),
				mutex:           &sync.Mutex{},
			},
			dependencyProxyObj: &v1.DependencyProxy{
				Spec: model.DependencyProxy{
					Destination: nil,
				},
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"admiral.io/env": "stage"}},
			},
		},
		{
			name:          "Given identityDependencyCache, when dependency proxy's destination identity is empty, func should return an error",
			expectedError: fmt.Errorf("update dependency proxy cache failed with error: dependencyProxyObj.Spec.Destination.Identity is empty"),
			cache: &dependencyProxyVirtualServiceCache{
				identityVSCache: make(map[string]map[string]*v1alpha3.VirtualService),
				mutex:           &sync.Mutex{},
			},
			dependencyProxyObj: &v1.DependencyProxy{
				Spec: model.DependencyProxy{
					Destination: &model.Destination{
						Identity: "",
					},
				},
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"admiral.io/env": "stage"}},
			},
		},
		{
			name:          "Given identityDependencyCache, when valid dependencyProxy object is passed, func should add it to cache and not return an error",
			expectedError: nil,
			cache: &dependencyProxyVirtualServiceCache{
				identityVSCache: make(map[string]map[string]*v1alpha3.VirtualService),
				mutex:           &sync.Mutex{},
			},
			dependencyProxyObj:                      validDependencyProxyObj,
			dependencyProxyDefaultHostNameGenerator: &dependencyProxyDefaultHostNameGenerator{},
			expectedCacheKey:                        "test",
			expectedCachedVSHosts:                   []string{"stage.test.xyz", "prefix00.test.xyz", "prefix01.test.xyz"},
			expectedCachedVSDestinationHost:         "stage.testproxy.global",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {

			err := updateIdentityDependencyProxyCache(context.Background(), tc.cache, tc.dependencyProxyObj, tc.dependencyProxyDefaultHostNameGenerator)

			if err != nil && tc.expectedError != nil {
				if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("expected error %s got %s", tc.expectedError.Error(), err.Error())
					return
				}
			} else {
				if err != tc.expectedError {
					t.Errorf("expected error %v got %v", tc.expectedError, err)
					return
				}
			}

			if err == nil {
				vsMap, ok := tc.cache.identityVSCache[tc.expectedCacheKey]
				if !ok {
					t.Errorf("expected cache with key %s", tc.expectedCacheKey)
					return
				}
				for _, vs := range vsMap {
					if !reflect.DeepEqual(vs.Spec.Hosts, tc.expectedCachedVSHosts) {
						t.Errorf("expected %v got %v", vs.Spec.Hosts, tc.expectedCachedVSHosts)
						return
					}
					if vs.Spec.Http[0].Route[0].Destination.Host != tc.expectedCachedVSDestinationHost {
						t.Errorf("expected %v got %v", vs.Spec.Http[0].Route[0].Destination.Host, tc.expectedCachedVSDestinationHost)
						return
					}
				}
			}

		})
	}
}
