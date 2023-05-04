package clusters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	structpb "github.com/golang/protobuf/ptypes/struct"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"istio.io/api/networking/v1alpha3"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	getSha1 = common.GetSha1
)

const hostsKey = "hosts: "
const pluginKey = "plugin: "

func createOrUpdateEnvoyFilter(ctx context.Context, rc *RemoteController, routingPolicy *v1.RoutingPolicy, eventType admiral.EventType, workloadIdentityKey string, admiralCache *AdmiralCache, workloadSelectorMap map[string]string) (*networking.EnvoyFilter, error) {

	envoyfilterSpec, err := constructEnvoyFilterStruct(routingPolicy, workloadSelectorMap)
	if err != nil {
		log.Error("error occurred while constructing envoy filter struct")
		return nil, err
	}

	selectorLabelsSha, err := getSha1(workloadIdentityKey + common.GetRoutingPolicyEnv(routingPolicy))
	if err != nil {
		log.Error("error occurred while computing workload labels sha1")
		return nil, err
	}
	if len(common.GetEnvoyFilterVersion()) == 0 {
		log.Error("envoy filter version not supplied")
		return nil, errors.New("envoy filter version not supplied")
	}
	envoyFilterName := fmt.Sprintf("%s-dynamicrouting-%s-%s", strings.ToLower(routingPolicy.Spec.Plugin), selectorLabelsSha, common.GetEnvoyFilterVersion())
	envoyfilter := &networking.EnvoyFilter{
		TypeMeta: metaV1.TypeMeta{
			Kind:       "EnvoyFilter",
			APIVersion: "networking.istio.io/v1alpha3",
		},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      envoyFilterName,
			Namespace: common.NamespaceIstioSystem,
		},
		//nolint
		Spec: *envoyfilterSpec,
	}

	admiralCache.RoutingPolicyFilterCache.Put(workloadIdentityKey+common.GetRoutingPolicyEnv(routingPolicy), rc.ClusterID, envoyFilterName)
	var filter *networking.EnvoyFilter
	//get the envoyfilter if it exists. If it exists, update it. Otherwise create it.
	if eventType == admiral.Add || eventType == admiral.Update {
		// We query the API server instead of getting it from cache because there could be potential condition where the filter exists in the cache but not on the cluster.
		filter, err = rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().
			EnvoyFilters(common.NamespaceIstioSystem).Get(ctx, envoyFilterName, metaV1.GetOptions{})
		if err != nil {
			log.Infof("msg=%s filtername=%s clustername=%s", "creating the envoy filter", envoyFilterName, rc.ClusterID)
			filter, err = rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().
				EnvoyFilters(common.NamespaceIstioSystem).Create(ctx, envoyfilter, metaV1.CreateOptions{})
			if err != nil {
				log.Infof("error creating filter: %v", err)
			}
		} else {
			log.Infof("msg=%s filtername=%s clustername=%s", "updating existing envoy filter", envoyFilterName, rc.ClusterID)
			envoyfilter.ResourceVersion = filter.ResourceVersion
			filter, err = rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().
				EnvoyFilters(common.NamespaceIstioSystem).Update(ctx, envoyfilter, metaV1.UpdateOptions{})
		}
	}

	return filter, err
}

func constructEnvoyFilterStruct(routingPolicy *v1.RoutingPolicy, workloadSelectorLabels map[string]string) (*v1alpha3.EnvoyFilter, error) {
	var envoyFilterStringConfig string
	var wasmPath string
	for key, val := range routingPolicy.Spec.Config {
		if key == common.WASMPath {
			wasmPath = val
			continue
		}
		envoyFilterStringConfig += fmt.Sprintf("%s: %s\n", key, val)
	}
	if len(common.GetEnvoyFilterAdditionalConfig()) != 0 {
		envoyFilterStringConfig += common.GetEnvoyFilterAdditionalConfig() + "\n"
	}
	hosts, err := getHosts(routingPolicy)
	if err != nil {
		return nil, err
	}
	envoyFilterStringConfig += hosts + "\n"
	plugin, err := getPlugin(routingPolicy)
	if err != nil {
		return nil, err
	}
	envoyFilterStringConfig += plugin

	configuration := structpb.Struct{
		Fields: map[string]*structpb.Value{
			"@type": {Kind: &structpb.Value_StringValue{StringValue: "type.googleapis.com/google.protobuf.StringValue"}},
			"value": {Kind: &structpb.Value_StringValue{StringValue: envoyFilterStringConfig}},
		},
	}
	
	vmConfig := structpb.Struct{
		Fields: map[string]*structpb.Value{
			"runtime": {Kind: &structpb.Value_StringValue{StringValue: "envoy.wasm.runtime.v8"}},
			"vm_id":   {Kind: &structpb.Value_StringValue{StringValue: routingPolicy.Spec.Plugin}},
			"code": {Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{Fields: map[string]*structpb.Value{
				"local": {Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{Fields: map[string]*structpb.Value{
					"filename": {Kind: &structpb.Value_StringValue{StringValue: wasmPath}},
				}}}},
			}}}},
		},
	}

	typedConfigValue := structpb.Struct{
		Fields: map[string]*structpb.Value{
			"config": {
				Kind: &structpb.Value_StructValue{
					StructValue: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"configuration": {Kind: &structpb.Value_StructValue{StructValue: &configuration}},
							"vm_config":     {Kind: &structpb.Value_StructValue{StructValue: &vmConfig}},
						},
					},
				},
			},
		},
	}

	typedConfig := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"@type":    {Kind: &structpb.Value_StringValue{StringValue: "type.googleapis.com/udpa.type.v1.TypedStruct"}},
			"type_url": {Kind: &structpb.Value_StringValue{StringValue: "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"}},
			"value":    {Kind: &structpb.Value_StructValue{StructValue: &typedConfigValue}},
		},
	}

	envoyfilterSpec := getEnvoyFilterSpec(workloadSelectorLabels, typedConfig)
	return envoyfilterSpec, nil
}

func getEnvoyFilterSpec(workloadSelectorLabels map[string]string, typedConfig *structpb.Struct) *v1alpha3.EnvoyFilter {
	return &v1alpha3.EnvoyFilter{
		WorkloadSelector: &v1alpha3.WorkloadSelector{Labels: workloadSelectorLabels},

		ConfigPatches: []*v1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{
			{
				ApplyTo: v1alpha3.EnvoyFilter_HTTP_FILTER,
				Match: &v1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
					Context: v1alpha3.EnvoyFilter_SIDECAR_OUTBOUND,
					// TODO: Figure out the possibility of using this for istio version upgrades. Can we add multiple filters with different proxy version Match here?
					Proxy: &v1alpha3.EnvoyFilter_ProxyMatch{ProxyVersion: "^" + strings.ReplaceAll(common.GetEnvoyFilterVersion(), ".", "\\.") + ".*"},
					ObjectTypes: &v1alpha3.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
						Listener: &v1alpha3.EnvoyFilter_ListenerMatch{
							FilterChain: &v1alpha3.EnvoyFilter_ListenerMatch_FilterChainMatch{
								Filter: &v1alpha3.EnvoyFilter_ListenerMatch_FilterMatch{
									Name: "envoy.filters.network.http_connection_manager",
									SubFilter: &v1alpha3.EnvoyFilter_ListenerMatch_SubFilterMatch{
										Name: "envoy.filters.http.router",
									},
								},
							},
						},
					},
				},
				Patch: &v1alpha3.EnvoyFilter_Patch{
					Operation: v1alpha3.EnvoyFilter_Patch_INSERT_BEFORE,
					Value: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"name": {Kind: &structpb.Value_StringValue{StringValue: "dynamicRoutingFilterPatch"}},
							"typed_config": {
								Kind: &structpb.Value_StructValue{
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

func getHosts(routingPolicy *v1.RoutingPolicy) (string, error) {
	hosts := ""
	for _, host := range routingPolicy.Spec.Hosts {
		hosts += host + ","
	}
	if len(hosts) == 0 {
		log.Error("routing policy hosts cannot be empty")
		return "", errors.New("routing policy hosts cannot be empty")
	}
	hosts = strings.TrimSuffix(hosts, ",")
	return hostsKey + hosts, nil
}

func getPlugin(routingPolicy *v1.RoutingPolicy) (string, error) {
	plugin := routingPolicy.Spec.Plugin
	if len(plugin) == 0 {
		log.Error("routing policy plugin cannot be empty")
		return "", errors.New("routing policy plugin cannot be empty")
	}
	return pluginKey + plugin, nil
}
