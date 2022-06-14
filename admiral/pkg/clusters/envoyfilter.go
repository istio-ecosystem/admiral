package clusters

import (
	"fmt"
	"github.com/gogo/protobuf/types"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"istio.io/api/networking/v1alpha3"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

func createOrUpdateEnvoyFilter( rc *RemoteController, routingPolicy *v1.RoutingPolicy, eventType admiral.EventType, workloadIdentityKey string, admiralCache *AdmiralCache) (*networking.EnvoyFilter, error) {

	workloadSelectors := make(map[string]string)
	workloadSelectors[common.GetWorkloadIdentifier()] = workloadIdentityKey
	workloadSelectors[common.GetEnvKey()] = common.GetRoutingPolicyEnv(routingPolicy)

	envoyfilterSpec := constructEnvoyFilterStruct(routingPolicy, workloadSelectors)

	selectorLabelsSha, err := common.GetSha1(workloadIdentityKey+common.GetRoutingPolicyEnv(routingPolicy))
	if err != nil {
		log.Error("Error ocurred while computing workload Labels sha1")
		return nil, err
	}
	envoyFilterName := fmt.Sprintf("%s-dynamicrouting-%s-%s", strings.ToLower(routingPolicy.Spec.Plugin), selectorLabelsSha, "1.10")
	envoyfilter := &networking.EnvoyFilter{
		TypeMeta: metaV1.TypeMeta{
			Kind:       "EnvoyFilter",
			APIVersion: "networking.istio.io/v1alpha3",
		},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      envoyFilterName,
			Namespace: "istio-system",
		},
		Spec: *envoyfilterSpec,
	}

	admiralCache.RoutingPolicyFilterCache.Put(workloadIdentityKey+common.GetRoutingPolicyEnv(routingPolicy), rc.ClusterID, envoyFilterName)
	var filter *networking.EnvoyFilter
	//get the envoyfilter if it exists. If it exists, update it. Otherwise create it.
	if eventType == admiral.Add || eventType == admiral.Update {
		filter, err = rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").Get(envoyFilterName, metaV1.GetOptions{})
		if err != nil {
			log.Infof("msg=%s filtername=%s clustername=%s", "creating the envoy filter", envoyFilterName, rc.ClusterID)
			filter, err = rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").Create(envoyfilter)
			if err != nil {
				log.Infof("Error creating filter: %v", filter)
			}
		} else {
			log.Infof("msg=%s filtername=%s clustername=%s", "updating existing envoy filter", envoyFilterName, rc.ClusterID)
			envoyfilter.ResourceVersion = filter.ResourceVersion
			filter, err = rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters("istio-system").Update(envoyfilter)
		}
	}


	return filter, err
}

func constructEnvoyFilterStruct(routingPolicy *v1.RoutingPolicy, workloadSelectorLabels map[string]string) *v1alpha3.EnvoyFilter {
	var envoyFilterStringConfig string
	for key, val := range routingPolicy.Spec.Config {
		envoyFilterStringConfig += fmt.Sprintf("%s: %s\n", key, val)
	}

	configuration := types.Struct{
		Fields: map[string]*types.Value{
			"@type": {Kind: &types.Value_StringValue{StringValue: "type.googleapis.com/google.protobuf.StringValue"}},
			"value": {Kind: &types.Value_StringValue{StringValue: envoyFilterStringConfig}},
		},
	}

	vmConfig := types.Struct{
		Fields: map[string]*types.Value{
			"runtime": {Kind: &types.Value_StringValue{StringValue: "envoy.wasm.runtime.v8"}},
			"code": {Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: map[string]*types.Value{
				"local": {Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: map[string]*types.Value{
					"filename": {Kind: &types.Value_StringValue{StringValue: "/etc/istio/extensions/dynamicrouter.wasm"}},
				}}}},
			}}}},
		},
	}

	typedConfigValue := types.Struct{
		Fields: map[string]*types.Value{
			"config": {
				Kind: &types.Value_StructValue{
					StructValue: &types.Struct{
						Fields: map[string]*types.Value{
							"configuration": {Kind: &types.Value_StructValue{StructValue: &configuration}},
							"vm_config":     {Kind: &types.Value_StructValue{StructValue: &vmConfig}},
						},
					},
				},
			},
		},
	}

	typedConfig := types.Struct{
		Fields: map[string]*types.Value{
			"@type":    {Kind: &types.Value_StringValue{StringValue: "type.googleapis.com/udpa.type.v1.TypedStruct"}},
			"type_url": {Kind: &types.Value_StringValue{StringValue: "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"}},
			"value":    {Kind: &types.Value_StructValue{StructValue: &typedConfigValue}},
		},
	}

	envoyfilterSpec := &v1alpha3.EnvoyFilter{
		WorkloadSelector: &v1alpha3.WorkloadSelector{Labels: workloadSelectorLabels},

		ConfigPatches: []*v1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{
			{
				ApplyTo: v1alpha3.EnvoyFilter_HTTP_FILTER,
				Match: &v1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
					Context: v1alpha3.EnvoyFilter_SIDECAR_OUTBOUND,
					// TODO: Figure out the possibility of using this for istio version upgrades. Can we add multiple filters with different proxy version Match here?
					Proxy: &v1alpha3.EnvoyFilter_ProxyMatch{ProxyVersion: `^1\.10.*`},
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
					//https://pkg.go.dev/github.com/gogo/protobuf/types#Value
					Value: &types.Struct{
						Fields: map[string]*types.Value{
							"name": {Kind: &types.Value_StringValue{StringValue: "dynamicRoutingFilterPatch"}},
							"typed_config": {
								Kind: &types.Value_StructValue{
									StructValue: &typedConfig,
								},
							},
						},
					},
				},
			},
		},
	}
	return envoyfilterSpec
}

