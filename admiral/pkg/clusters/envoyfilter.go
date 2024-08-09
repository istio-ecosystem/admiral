package clusters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	k8sErrors "k8s.io/apimachinery/pkg/api/errors"

	structPb "github.com/golang/protobuf/ptypes/struct"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/api/networking/v1alpha3"
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	getSha1 = common.GetSha1
)

const (
	envoyFilter                                           = "EnvoyFilter"
	hostsKey                                              = "hosts: "
	pluginKey                                             = "plugin: "
	envoyfilterAssociatedRoutingPolicyNameAnnotation      = "associated-routing-policy-name"
	envoyfilterAssociatedRoutingPolicyIdentityeAnnotation = "associated-routing-policy-identity"
)

// getEnvoyFilterNamespace returns the user namespace where envoy filter needs to be created.
func getEnvoyFilterNamespace() string {
	var namespace string
	namespace = common.NamespaceIstioSystem
	return namespace
}
func createOrUpdateEnvoyFilter(ctx context.Context, rc *RemoteController, routingPolicy *v1.RoutingPolicy, eventType admiral.EventType, workloadIdentityKey string, admiralCache *AdmiralCache) ([]*networking.EnvoyFilter, error) {

	var (
		filterNamespace string
		err             error
	)

	filterNamespace = getEnvoyFilterNamespace()
	routingPolicyNameSha, err := getSha1(routingPolicy.Name + common.GetRoutingPolicyEnv(routingPolicy) + common.GetRoutingPolicyIdentity(routingPolicy))
	if err != nil {
		log.Errorf(LogErrFormat, eventType, envoyFilter, routingPolicy.Name, rc.ClusterID, "error occurred while computing routingPolicy name sha1")
		return nil, err
	}
	dependentIdentitySha, err := getSha1(workloadIdentityKey)
	if err != nil {
		log.Errorf(LogErrFormat, eventType, envoyFilter, routingPolicy.Name, rc.ClusterID, "error occurred while computing dependentIdentity sha1")
		return nil, err
	}
	if len(common.GetEnvoyFilterVersion()) == 0 {
		log.Errorf(LogErrFormat, eventType, envoyFilter, routingPolicy.Name, rc.ClusterID, "envoy filter version not supplied")
		return nil, errors.New("envoy filter version not supplied")
	}

	var versionsArray = common.GetEnvoyFilterVersion() // e.g. 1.13,1.17
	env := common.GetRoutingPolicyEnv(routingPolicy)
	filterList := make([]*networking.EnvoyFilter, 0)

	for _, version := range versionsArray {
		envoyFilterName := fmt.Sprintf("%s-dr-%s-%s-%s", strings.ToLower(routingPolicy.Spec.Plugin), routingPolicyNameSha, dependentIdentitySha, version)
		envoyfilterSpec := constructEnvoyFilterStruct(routingPolicy, map[string]string{common.AssetAlias: workloadIdentityKey}, version, envoyFilterName)

		log.Infof(LogFormat, eventType, envoyFilter, envoyFilterName, rc.ClusterID, "version +"+version)

		envoyfilter := &networking.EnvoyFilter{
			TypeMeta: metaV1.TypeMeta{
				Kind:       "EnvoyFilter",
				APIVersion: "networking.istio.io/v1alpha3",
			},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      envoyFilterName,
				Namespace: filterNamespace,
				Annotations: map[string]string{
					envoyfilterAssociatedRoutingPolicyNameAnnotation:      routingPolicy.Name,
					envoyfilterAssociatedRoutingPolicyIdentityeAnnotation: common.GetRoutingPolicyIdentity(routingPolicy),
				},
			},
			//nolint
			Spec: *envoyfilterSpec,
		}

		// To maintain mapping of envoyfilters created for a routing policy, and to facilitate deletion of envoyfilters when routing policy is deleted
		admiralCache.RoutingPolicyFilterCache.Put(routingPolicy.Name+common.GetRoutingPolicyIdentity(routingPolicy)+env, rc.ClusterID, envoyFilterName, filterNamespace)

		//get the envoyfilter if it exists. If it exists, update it. Otherwise create it.
		if eventType == admiral.Add || eventType == admiral.Update {
			// We query the API server instead of getting it from cache because there could be potential condition where the filter exists in the cache but not on the cluster.
			var err2 error
			filter, err1 := rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().
				EnvoyFilters(filterNamespace).Get(ctx, envoyFilterName, metaV1.GetOptions{})

			if k8sErrors.IsNotFound(err1) {
				log.Infof(LogFormat, eventType, envoyFilter, envoyFilterName, rc.ClusterID, "creating the envoy filter")
				filter, err2 = rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().
					EnvoyFilters(filterNamespace).Create(ctx, envoyfilter, metaV1.CreateOptions{})
			} else if err1 == nil {
				log.Infof(LogFormat, eventType, envoyFilter, envoyFilterName, rc.ClusterID, "updating existing envoy filter")
				envoyfilter.ResourceVersion = filter.ResourceVersion
				filter, err2 = rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().
					EnvoyFilters(filterNamespace).Update(ctx, envoyfilter, metaV1.UpdateOptions{})
			} else {
				err = common.AppendError(err1, err)
				log.Errorf(LogErrFormat, eventType, envoyFilter, routingPolicy.Name, rc.ClusterID, err1)
			}

			if err2 == nil {
				filterList = append(filterList, filter)
			} else {
				err = common.AppendError(err2, err)
				log.Errorf(LogErrFormat, eventType, envoyFilter, routingPolicy.Name, rc.ClusterID, err2)
			}
		}
	}
	return filterList, err
}
func constructEnvoyFilterStruct(routingPolicy *v1.RoutingPolicy, workloadSelectorLabels map[string]string, filterVersion string, filterName string) *v1alpha3.EnvoyFilter {
	var envoyFilterStringConfig string
	var wasmPath string
	for key, val := range routingPolicy.Spec.Config {
		if key == common.WASMPath {
			wasmPath = common.WasmPathValue
			continue
		}
		envoyFilterStringConfig += fmt.Sprintf("%s: %s\n", key, val)
	}
	if len(common.GetEnvoyFilterAdditionalConfig()) != 0 {
		envoyFilterStringConfig += common.GetEnvoyFilterAdditionalConfig() + "\n"
	}
	envoyFilterStringConfig += getHosts(routingPolicy) + "\n"
	envoyFilterStringConfig += getPlugin(routingPolicy)

	log.Infof("msg=%s type=routingpolicy name=%s", "adding config", routingPolicy.Name)

	configuration := structPb.Struct{
		Fields: map[string]*structPb.Value{
			"@type": {Kind: &structPb.Value_StringValue{StringValue: "type.googleapis.com/google.protobuf.StringValue"}},
			"value": {Kind: &structPb.Value_StringValue{StringValue: envoyFilterStringConfig}},
		},
	}

	vmConfig := structPb.Struct{
		Fields: map[string]*structPb.Value{
			"runtime": {Kind: &structPb.Value_StringValue{StringValue: "envoy.wasm.runtime.v8"}},
			"vm_id":   {Kind: &structpb.Value_StringValue{StringValue: filterName}},
			"code": {Kind: &structPb.Value_StructValue{StructValue: &structPb.Struct{Fields: map[string]*structPb.Value{
				"local": {Kind: &structPb.Value_StructValue{StructValue: &structPb.Struct{Fields: map[string]*structPb.Value{
					"filename": {Kind: &structPb.Value_StringValue{StringValue: wasmPath}},
				}}}},
			}}}},
		},
	}

	typedConfigValue := structPb.Struct{
		Fields: map[string]*structPb.Value{
			"config": {
				Kind: &structPb.Value_StructValue{
					StructValue: &structPb.Struct{
						Fields: map[string]*structPb.Value{
							"configuration": {Kind: &structPb.Value_StructValue{StructValue: &configuration}},
							"vm_config":     {Kind: &structPb.Value_StructValue{StructValue: &vmConfig}},
						},
					},
				},
			},
		},
	}

	typedConfig := &structPb.Struct{
		Fields: map[string]*structPb.Value{
			"@type":    {Kind: &structPb.Value_StringValue{StringValue: "type.googleapis.com/udpa.type.v1.TypedStruct"}},
			"type_url": {Kind: &structPb.Value_StringValue{StringValue: "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"}},
			"value":    {Kind: &structPb.Value_StructValue{StructValue: &typedConfigValue}},
		},
	}

	envoyfilter := getEnvoyFilterSpec(workloadSelectorLabels, "dynamicRoutingFilterPatch", typedConfig, v1alpha3.EnvoyFilter_SIDECAR_OUTBOUND,
		&v1alpha3.EnvoyFilter_ListenerMatch_SubFilterMatch{Name: "envoy.filters.http.router"}, v1alpha3.EnvoyFilter_Patch_INSERT_BEFORE, filterVersion)
	return envoyfilter
}

func getEnvoyFilterSpec(workloadSelectorLabels map[string]string, filterName string, typedConfig *structPb.Struct,
	filterContext networkingv1alpha3.EnvoyFilter_PatchContext, subfilter *v1alpha3.EnvoyFilter_ListenerMatch_SubFilterMatch,
	insertPosition networkingv1alpha3.EnvoyFilter_Patch_Operation, filterVersion string) *v1alpha3.EnvoyFilter {
	return &v1alpha3.EnvoyFilter{
		WorkloadSelector: &v1alpha3.WorkloadSelector{Labels: workloadSelectorLabels},

		ConfigPatches: []*v1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{
			{
				ApplyTo: v1alpha3.EnvoyFilter_HTTP_FILTER,
				Match: &v1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
					Context: filterContext,
					// TODO: Figure out the possibility of using this for istio version upgrades. Can we add multiple filters with different proxy version Match here?
					Proxy: &v1alpha3.EnvoyFilter_ProxyMatch{ProxyVersion: "^" + strings.ReplaceAll(filterVersion, ".", "\\.") + ".*"},
					ObjectTypes: &v1alpha3.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
						Listener: &v1alpha3.EnvoyFilter_ListenerMatch{
							FilterChain: &v1alpha3.EnvoyFilter_ListenerMatch_FilterChainMatch{
								Filter: &v1alpha3.EnvoyFilter_ListenerMatch_FilterMatch{
									Name:      "envoy.filters.network.http_connection_manager",
									SubFilter: subfilter,
								},
							},
						},
					},
				},
				Patch: &v1alpha3.EnvoyFilter_Patch{
					Operation: insertPosition,
					Value: &structPb.Struct{
						Fields: map[string]*structPb.Value{
							"name": {Kind: &structPb.Value_StringValue{StringValue: filterName}},
							"typed_config": {
								Kind: &structPb.Value_StructValue{
									StructValue: typedConfig,
								},
							},
						},
					},
				},
			},
		},
	}
}

func getHosts(routingPolicy *v1.RoutingPolicy) string {
	hosts := ""
	for _, host := range routingPolicy.Spec.Hosts {
		hosts += host + ","
	}
	hosts = strings.TrimSuffix(hosts, ",")
	return hostsKey + hosts
}

func getPlugin(routingPolicy *v1.RoutingPolicy) string {
	plugin := routingPolicy.Spec.Plugin
	return pluginKey + plugin
}
