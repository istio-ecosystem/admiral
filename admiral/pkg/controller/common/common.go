package common

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	v1alpha4 "istio.io/client-go/pkg/apis/networking/v1alpha3"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"

	"github.com/google/uuid"

	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"

	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
)

var (
	CtxLogFormat         = "task=%v name=%v namespace=%s cluster=%s message=%v"
	CtxLogFormatWithTime = "task=%v name=%v namespace=%s cluster=%s message=%v txTime=%v"
	LogFormatAdv         = "op=%v type=%v name=%v namespace=%s cluster=%s message=%v"
	LogErrFormat         = "op=%v type=%v name=%v cluster=%v error=%v"
	ConfigWriter         = "ConfigWriter"
)

const (
	Admiral                          = "admiral"
	NamespaceKubeSystem              = "kube-system"
	NamespaceIstioSystem             = "istio-system"
	IstioIngressGatewayLabelValue    = "istio-ingressgateway"
	NLBIstioIngressGatewayLabelValue = "istio-ingressgateway-nlb"
	NLBIdleTimeoutAnnotation         = "service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout" // service.beta.kubernetes.io/aws-load-balancer-listener-attributes.TCP-15543: tcp.idle_timeout.seconds=350 (needs to change to this once aws lb controller updated)
	NLBDefaultTimeoutSeconds         = 350
	Env                              = "env"
	DefaultMtlsPort                  = 15443
	DefaultServiceEntryPort          = 80
	Sep                              = "."
	Dash                             = "-"
	Slash                            = "/"
	DotLocalDomainSuffix             = ".svc.cluster.local"
	AssetAlias                       = "assetAlias"
	Mesh                             = "mesh"
	MulticlusterIngressGateway       = "istio-multicluster-ingressgateway"
	LocalAddressPrefix               = "240.0"
	NodeRegionLabel                  = "failure-domain.beta.kubernetes.io/region"
	SpiffePrefix                     = "spiffe://"
	SidecarEnabledPorts              = "traffic.sidecar.istio.io/includeInboundPorts"
	Default                          = "default"
	SidecarInjectAnnotation          = "sidecar.istio.io/inject"
	AdmiralIgnoreAnnotation          = "admiral.io/ignore"
	AdmiralEnvAnnotation             = "admiral.io/env"
	AdmiralCnameCaseSensitive        = "admiral.io/cname-case-sensitive"
	BlueGreenRolloutPreviewPrefix    = "preview"
	RolloutPodHashLabel              = "rollouts-pod-template-hash"
	RolloutActiveServiceSuffix       = "active-service"
	RolloutStableServiceSuffix       = "stable-service"
	RolloutRootServiceSuffix         = "root-service"
	CanaryRolloutCanaryPrefix        = "canary"
	WASMPath                         = "wasmPath"
	AdmiralProfileIntuit             = "intuit"
	AdmiralProfileDefault            = "default"
	AdmiralProfilePerf               = "perf"
	Cartographer                     = "cartographer"
	CreatedBy                        = "createdBy"
	CreatedFor                       = "createdFor"
	CreatedType                      = "createdType"
	CreatedForEnv                    = "createdForEnv"
	IsDisabled                       = "isDisabled"
	TransactionID                    = "transactionID"
	RevisionNumber                   = "revisionNumber"
	EnvoyKind                        = "EnvoyFilter"
	EnvoyApiVersion                  = "networking.istio.io/v1alpha3"
	SlashSTARRule                    = "/*"
	AppThrottleConfigVersion         = "v1"
	EnvoyFilterLogLevel              = "info"
	EnvoyFilterLogLocation           = "proxy"
	EnvoyFilterLogFormat             = "json"
	ADD                              = "Add"
	UPDATE                           = "Update"
	DELETE                           = "Delete"
	AssetLabel                       = "asset"
	WestLocality                     = "us-west-2"
	EastLocality                     = "us-east-2"

	RollingWindow = "ROLLING_WINDOW"

	MeshService              = "MESH_SERVICE"
	Deployment               = "deployment"
	Rollout                  = "rollout"
	Job                      = "job"
	Vertex                   = "vertex"
	MonoVertex               = "monovertex"
	GTP                      = "gtp"
	EventType                = "eventType"
	ProcessingInProgress     = "ProcessingInProgress"
	NotProcessed             = "NotProcessed"
	Processed                = "Processed"
	ServicesGatewayIdentity  = "Intuit.platform.servicesgateway.servicesgateway"
	DependentClusterOverride = "dependentClusterOverride"
	Received                 = "Received"
	Retry                    = "Retry"
	Forget                   = "Forget"

	ClusterName            = "clusterName"
	GtpPreferenceRegion    = "gtpPreferenceRegion"
	EventResourceType      = "eventResourceType"
	OutlierDetection       = "OutlierDetection"
	ClientConnectionConfig = "ClientConnectionConfig"
	TrafficConfig          = "TrafficConfig"
	WasmPathValue          = "/etc/istio/extensions/dynamicrouter.wasm"
	AIREnvSuffix           = "-air"
	MESHSUFFIX             = ".mesh"

	LastUpdatedAt = "lastUpdatedAt"
	IntuitTID     = "intuit_tid"
	GTPCtrl       = "gtp-ctrl"

	App                       = "app"
	DynamicConfigUpdate       = "DynamicConfigUpdate"
	LBUpdateProcessor         = "LBUpdateProcessor"
	ClientInitiatedProcessing = "ClientInitiatedProcessing"

	DummyAdmiralGlobal = "dummy.admiral.global"

	VSRoutingLabel = "admiral.io/vs-routing"
	// VSRoutingType This label has been added in order to make the API call efficient
	VSRoutingType                      = "admiral.io/vs-routing-type"
	InclusterVSNameSuffix              = "incluster-vs"
	VSRoutingTypeInCluster             = "incluster"
	CreatedByLabel                     = "createdBy"
	CreatedForLabel                    = "createdFor"
	WarmupDurationValueInSeconds       = "warmupDurationValueInSeconds"
	TrafficConfigAssetLabelKey         = "asset"
	TrafficConfigEnvLabelKey           = "env"
	TrafficConfigContextCacheKey       = "trafficConfigContextCacheKey"
	TrafficConfigContextWorkloadEnvKey = "trafficConfigWorkloadEnvKey"
	TrafficConfigIdentity              = "trafficConfigIdentity"
	RoutingDRSuffix                    = "routing-dr"
	InclusterDRSuffix                  = "incluster-dr"
)

type Event string

const (
	Add    Event = "Add"
	Update Event = "Update"
	Delete Event = "Delete"
)

type ResourceType string

const (
	// Kubernetes/ARGO Resource Types
	DeploymentResourceType ResourceType = "Deployment"
	RolloutResourceType    ResourceType = "Rollout"
	ServiceResourceType    ResourceType = "Service"
	ConfigMapResourceType  ResourceType = "ConfigMap"
	SecretResourceType     ResourceType = "Secret"
	NodeResourceType       ResourceType = "Node"
	JobResourceType        ResourceType = "Job"

	// Admiral Resource Types
	DependencyResourceType          ResourceType = "Dependency"
	DependencyProxyResourceType     ResourceType = "DependencyProxy"
	GlobalTrafficPolicyResourceType ResourceType = "GlobalTrafficPolicy"
	RoutingPolicyResourceType       ResourceType = "RoutingPolicy"
	ShardResourceType               ResourceType = "Shard"
	TrafficConfigResourceType       ResourceType = "TrafficConfig"

	// Istio Resource Types
	VirtualServiceResourceType  ResourceType = "VirtualService"
	DestinationRuleResourceType ResourceType = "DestinationRule"
	ServiceEntryResourceType    ResourceType = "ServiceEntry"
	EnvoyFilterResourceType     ResourceType = "EnvoyFilter"
	SidecarResourceType         ResourceType = "Sidecar"

	// Status
	ReceivedStatus = "Received"
)

func GetPodGlobalIdentifier(pod *k8sV1.Pod) string {
	identity := pod.Labels[GetWorkloadIdentifier()]
	if len(identity) == 0 {
		identity = pod.Annotations[GetWorkloadIdentifier()]
	}
	return identity
}

func GetDeploymentGlobalIdentifier(deployment *k8sAppsV1.Deployment) string {
	identity := deployment.Spec.Template.Labels[GetWorkloadIdentifier()]
	if len(identity) == 0 {
		//TODO can this be removed now? This was for backward compatibility
		identity = deployment.Spec.Template.Annotations[GetWorkloadIdentifier()]
	}
	if EnableSWAwareNSCaches() && len(identity) > 0 && len(GetDeploymentIdentityPartition(deployment)) > 0 {
		identity = GetDeploymentIdentityPartition(deployment) + Sep + strings.ToLower(identity)

	}
	return identity
}

func GetDeploymentOriginalIdentifier(deployment *k8sAppsV1.Deployment) string {
	identity := deployment.Spec.Template.Labels[GetWorkloadIdentifier()]
	if len(identity) == 0 {
		//TODO can this be removed now? This was for backward compatibility
		identity = deployment.Spec.Template.Annotations[GetWorkloadIdentifier()]
	}
	return identity
}

func GetDeploymentIdentityPartition(deployment *k8sAppsV1.Deployment) string {
	identityPartition := deployment.Spec.Template.Annotations[GetPartitionIdentifier()]
	if len(identityPartition) == 0 {
		//In case partition is accidentally applied as Label
		identityPartition = deployment.Spec.Template.Labels[GetPartitionIdentifier()]
	}
	return identityPartition
}

func GetLocalDomainSuffix() string {
	if IsAbsoluteFQDNEnabled() && IsAbsoluteFQDNEnabledForLocalEndpoints() {
		return DotLocalDomainSuffix + Sep
	}
	return DotLocalDomainSuffix
}

// GetCname returns cname in the format <env>.<service identity>.global, Ex: stage.Admiral.services.registry.global
func GetCname(deployment *k8sAppsV1.Deployment, identifier string, nameSuffix string) string {
	var environment = GetEnv(deployment)
	alias := GetValueForKeyFromDeployment(identifier, deployment)
	if len(alias) == 0 {
		log.Warnf("%v label missing on deployment %v in namespace %v. Falling back to annotation to create cname.", identifier, deployment.Name, deployment.Namespace)
		alias = deployment.Spec.Template.Annotations[identifier]
	}
	if len(alias) == 0 {
		log.Errorf("Unable to get cname for deployment with name %v in namespace %v as it doesn't have the %v annotation", deployment.Name, deployment.Namespace, identifier)
		return ""
	}
	cname := GetCnameVal([]string{environment, alias, nameSuffix})
	if deployment.Spec.Template.Annotations[AdmiralCnameCaseSensitive] == "true" {
		return cname
	}
	return strings.ToLower(cname)
}

func GetCnameVal(vals []string) string {
	return strings.Join(vals, Sep)
}

func GetEnv(deployment *k8sAppsV1.Deployment) string {
	var environment = deployment.Spec.Template.Annotations[GetEnvKey()]
	if len(environment) == 0 {
		environment = deployment.Spec.Template.Labels[GetEnvKey()]
	}
	if len(environment) == 0 {
		environment = deployment.Spec.Template.Labels[Env]
	}
	if len(environment) == 0 {
		splitNamespace := strings.Split(deployment.Namespace, Dash)
		if len(splitNamespace) > 1 {
			environment = splitNamespace[len(splitNamespace)-1]
		}
		log.Warnf("Using deprecated approach to deduce env from namespace for deployment, name=%v in namespace=%v", deployment.Name, deployment.Namespace)
	}
	if len(environment) == 0 {
		environment = Default
	}
	return environment
}

// GetSAN returns SAN for a service entry in the format spiffe://<domain>/<identifier>, Ex: spiffe://subdomain.domain.com/Admiral.platform.mesh.server
func GetSAN(domain string, deployment *k8sAppsV1.Deployment, identifier string) string {
	identifierVal := GetValueForKeyFromDeployment(identifier, deployment)
	if len(identifierVal) == 0 {
		log.Errorf("Unable to get SAN for deployment with name %v in namespace %v as it doesn't have the %v annotation or label", deployment.Name, deployment.Namespace, identifier)
		return ""
	}
	if len(domain) > 0 {
		return SpiffePrefix + domain + Slash + identifierVal
	} else {
		return SpiffePrefix + identifierVal
	}
}

func GetNodeLocality(node *k8sV1.Node) string {
	region := node.Labels[NodeRegionLabel]
	return region
}

func GetValueForKeyFromDeployment(key string, deployment *k8sAppsV1.Deployment) string {
	value := deployment.Spec.Template.Labels[key]
	if len(value) == 0 {
		log.Warnf("%v label missing on deployment %v in namespace %v. Falling back to annotation.", key, deployment.Name, deployment.Namespace)
		value = deployment.Spec.Template.Annotations[key]
	}
	return value
}

func GetGtpEnv(gtp *v1.GlobalTrafficPolicy) string {
	environment := GetEnvFromMetadata(gtp.Annotations, gtp.Labels, gtp.Spec.Selector)
	if len(environment) == 0 {
		environment = gtp.Labels[Env]
		log.Warnf("Using deprecated approach to use env label for GTP, name=%v in namespace=%v", gtp.Name, gtp.Namespace)
	}
	if len(environment) == 0 {
		environment = gtp.Spec.Selector[Env]
		log.Warnf("Using deprecated approach to use env label for GTP, name=%v in namespace=%v", gtp.Name, gtp.Namespace)
	}
	if len(environment) == 0 {
		environment = Default
	}
	return environment
}

func GetGtpIdentity(gtp *v1.GlobalTrafficPolicy) string {
	identity := GetIdentity(gtp.Labels, gtp.Spec.Selector)

	if EnableSWAwareNSCaches() && len(identity) > 0 && len(GetGtpIdentityPartition(gtp)) > 0 {
		identity = GetGtpIdentityPartition(gtp) + Sep + strings.ToLower(identity)
	}
	return identity
}

func GetGtpIdentityPartition(gtp *v1.GlobalTrafficPolicy) string {
	identityPartition := gtp.ObjectMeta.Annotations[GetPartitionIdentifier()]
	if len(identityPartition) == 0 {
		//In case partition is accidentally applied as Label
		identityPartition = gtp.ObjectMeta.Labels[GetPartitionIdentifier()]
	}
	return identityPartition
}

func GetGtpKey(gtp *v1.GlobalTrafficPolicy) string {
	return ConstructKeyWithEnvAndIdentity(GetGtpEnv(gtp), GetGtpIdentity(gtp))
}

func GetGtpPreferenceRegion(existingGtp, newGtp *v1.GlobalTrafficPolicy) string {
	existingGtpDnsRegionMapping := makeDnsPrefixRegionMapping(existingGtp)
	newGtpDnsRegionMapping := makeDnsPrefixRegionMapping(newGtp)

	for dnsprefix, region := range newGtpDnsRegionMapping {
		if existingRegion, ok := existingGtpDnsRegionMapping[dnsprefix]; ok {
			if existingRegion != region {
				return region
			}
		}
	}
	return ""
}

func makeDnsPrefixRegionMapping(gtp *v1.GlobalTrafficPolicy) map[string]string {
	dnsRegionMapping := make(map[string]string)
	for _, tp := range gtp.Spec.Policy {
		if tp.LbType == model.TrafficPolicy_FAILOVER {
			for _, tg := range tp.Target {
				if tg.Weight == int32(100) {
					dnsRegionMapping[tp.DnsPrefix] = tg.Region
				}
			}
		}
	}
	return dnsRegionMapping
}

func ConstructKeyWithEnvAndIdentity(env, identity string) string {
	return fmt.Sprintf("%s.%s", env, identity)
}

func ShouldIgnoreResource(metadata v12.ObjectMeta) bool {
	return metadata.Annotations[AdmiralIgnoreAnnotation] == "true" || metadata.Labels[AdmiralIgnoreAnnotation] == "true"
}

func IsServiceMatch(serviceSelector map[string]string, selector *v12.LabelSelector) bool {
	if selector == nil || len(selector.MatchLabels) == 0 || len(serviceSelector) == 0 {
		return false
	}
	var match = true
	for lkey, lvalue := range serviceSelector {
		// Rollouts controller adds a dynamic label with name rollouts-pod-template-hash to both active and passive replicasets.
		// This dynamic label is not available on the rollout template. Hence ignoring the label with name rollouts-pod-template-hash
		if lkey == RolloutPodHashLabel {
			continue
		}
		value, ok := selector.MatchLabels[lkey]
		if !ok || value != lvalue {
			match = false
			break
		}
	}
	return match
}

func GetRoutingPolicyEnv(rp *v1.RoutingPolicy) string {
	var environment = rp.Annotations[GetEnvKey()]
	if len(environment) == 0 {
		environment = rp.Labels[GetEnvKey()]
	}
	if len(environment) == 0 {
		environment = Default
	}
	return environment
}

func GetRoutingPolicyIdentity(rp *v1.RoutingPolicy) string {
	identity := rp.Labels[GetRoutingPolicyLabel()]
	return identity
}

func GetRoutingPolicyKey(rp *v1.RoutingPolicy) string {
	return ConstructKeyWithEnvAndIdentity(GetRoutingPolicyEnv(rp), GetRoutingPolicyIdentity(rp))
}

// this function is exactly same as ConstructKeyWithEnvAndIdentity.
// Not reusing the same function to keep the methods associated with these two objects separate.
func ConstructRoutingPolicyKey(env, identity string) string {
	return ConstructKeyWithEnvAndIdentity(env, identity)
}

func IsTrafficConfigDisabled(tc *v1.TrafficConfig) bool {
	labelValue := tc.Labels["isDisabled"]
	annotationValue := tc.Annotations["isDisabled"]
	return labelValue == "true" || annotationValue == "true"
}

func GetTrafficConfigEnv(tc *v1.TrafficConfig) string {
	identity := tc.Labels["env"]
	if len(identity) == 0 {
		identity = tc.Annotations["env"]
	}
	return identity
}

func GetTrafficConfigIdentity(tc *v1.TrafficConfig) string {
	identity := tc.Labels[GetTrafficConfigIdentifier()]
	if len(identity) == 0 {
		identity = tc.Annotations[GetTrafficConfigIdentifier()]
	}
	return identity
}

func CheckIFEnvLabelIsPresent(tc *v1.TrafficConfig) error {
	if tc.Labels == nil {
		err := errors.New("no labels found of traffic config object - " + tc.Name + " in namespace - " + tc.Namespace)
		return err
	}
	if len(tc.Labels["env"]) == 0 {
		err := errors.New("mandatory label env is not present on the traffic config object=" + tc.Name + " in namespace=" + tc.Namespace)
		return err
	}
	return nil
}

func GetTrafficConfigRevision(tc *v1.TrafficConfig) string {
	identity := tc.Labels["revisionNumber"]
	if len(identity) == 0 {
		identity = tc.Annotations["revisionNumber"]
	}
	return identity
}

func GetTrafficConfigTransactionID(tc *v1.TrafficConfig) string {
	tid := tc.Labels["transactionID"]
	if len(tid) == 0 {
		tid = tc.Annotations["transactionID"]
	}
	return tid

}

func GetSha1(key interface{}) (string, error) {
	bv, err := GetBytes(key)
	if err != nil {
		return "", err
	}
	hasher := sha1.New()
	hasher.Write(bv)
	sha := hex.EncodeToString(hasher.Sum(nil))
	if len(sha) >= 20 {
		return sha[0:20], nil
	} else {
		return sha, nil
	}
}

func GetBytes(key interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(key)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func AppendError(err error, newError error) error {
	if newError != nil {
		if err == nil {
			err = newError
		} else {
			err = fmt.Errorf("%s; %s", err.Error(), newError.Error())
		}
	}
	return err
}

func IsIstioIngressGatewayService(svc *k8sV1.Service, key string) bool {

	if svc != nil && len(svc.Labels) > 0 {
		return svc.Namespace == NamespaceIstioSystem && svc.Labels["app"] == key
	}
	return false
}

func FetchTxIdOrGenNew(ctx context.Context) string {
	txId, ok := ctx.Value("txId").(string)
	if !ok {
		log.Errorf("unable to fetch txId from context, will recreate one")
		id := uuid.New()
		txId = id.String()
	}
	return txId
}

func GetCtxLogger(ctx context.Context, identity, env string) *log.Entry {
	controllerName, ok := ctx.Value("controller").(string)
	if ok {
		triggeringCluster, ok := ctx.Value("cluster").(string)
		if ok {
			return log.WithFields(log.Fields{
				"op":                "ConfigWriter",
				"triggeringCluster": triggeringCluster,
				"identity":          identity,
				"txId":              FetchTxIdOrGenNew(ctx),
				"controller":        controllerName,
				"env":               env,
			})
		}
		return log.WithFields(log.Fields{
			"op":         "ConfigWriter",
			"identity":   identity,
			"txId":       FetchTxIdOrGenNew(ctx),
			"controller": controllerName,
			"env":        env,
		})
	}
	return log.WithFields(log.Fields{
		"op":       "ConfigWriter",
		"identity": identity,
		"txId":     FetchTxIdOrGenNew(ctx),
		"env":      env,
	})
}

func GetClientConnectionConfigIdentity(clientConnectionSettings *v1.ClientConnectionConfig) string {
	return GetIdentity(clientConnectionSettings.Labels, map[string]string{})
}

func GetClientConnectionConfigEnv(clientConnectionSettings *v1.ClientConnectionConfig) string {
	env := GetEnvFromMetadata(clientConnectionSettings.Annotations,
		clientConnectionSettings.Labels, map[string]string{})
	if len(env) == 0 {
		env = clientConnectionSettings.Labels[Env]
		log.Warnf("Using deprecated approach to use env label for %s, name=%v in namespace=%v",
			ClientConnectionConfig, clientConnectionSettings.Name, clientConnectionSettings.Namespace)
	}
	if len(env) == 0 {
		env = Default
	}
	return env
}

func GetIdentity(labels, selectors map[string]string) string {
	identity := labels[GetAdmiralCRDIdentityLabel()]
	if len(identity) == 0 {
		identity = selectors[GetAdmiralCRDIdentityLabel()]
	}
	return identity
}

func GetEnvFromMetadata(annotations, labels, selectors map[string]string) string {
	var env = annotations[GetEnvKey()]
	if len(env) == 0 {
		env = labels[GetEnvKey()]
	}
	if len(env) == 0 {
		env = selectors[GetEnvKey()]
	}
	return env
}

func GetODIdentity(od *v1.OutlierDetection) string {
	return GetIdentity(od.Labels, od.Spec.Selector)
}

func GetODEnv(od *v1.OutlierDetection) string {
	env := GetEnvFromMetadata(od.Annotations, od.Labels, od.Spec.Selector)
	if len(env) == 0 {
		env = od.Labels[Env]
		log.Warnf("Using deprecated approach to use env label for %s, name=%v in namespace=%v", OutlierDetection, od.Name, od.Namespace)
	}
	if len(env) == 0 {
		env = od.Spec.Selector[Env]
		log.Warnf("Using deprecated approach to use env label for %s, name=%v in namespace=%v", OutlierDetection, od.Name, od.Namespace)
	}
	if len(env) == 0 {
		env = Default
	}
	return env
}

func RetryWithBackOff(ctx context.Context, callback func() error, retryCount int) error {
	sleep := 10 * time.Second
	var err error
	for i := 0; i < retryCount; i++ {
		if i > 0 {
			log.Infof("retrying after sleeping %v, txId=%v", sleep, ctx.Value("txId"))
			time.Sleep(sleep)
			sleep *= 2
		}
		err = callback()
		if err == nil {
			break
		}
		log.Infof("retrying with error %v, txId=%v", err, ctx.Value("txId"))
	}
	return err
}

func SortGtpsByPriorityAndCreationTime(gtpsToOrder []*v1.GlobalTrafficPolicy, identity string, env string) {
	sort.Slice(gtpsToOrder, func(i, j int) bool {
		iPriority := getGtpPriority(gtpsToOrder[i])
		jPriority := getGtpPriority(gtpsToOrder[j])

		iCreationTime := gtpsToOrder[i].CreationTimestamp
		jCreationTime := gtpsToOrder[j].CreationTimestamp
		if iPriority != jPriority {
			log.Debugf("GTP sorting identity=%s env=%s name1=%s creationTime1=%v priority1=%d name2=%s creationTime2=%v priority2=%d", identity, env, gtpsToOrder[i].Name, iCreationTime, iPriority, gtpsToOrder[j].Name, jCreationTime, jPriority)
			return iPriority > jPriority
		}

		if gtpsToOrder[i].Annotations != nil && gtpsToOrder[j].Annotations != nil {
			if iUpdateTime, ok := gtpsToOrder[i].Annotations["lastUpdatedAt"]; ok {
				if jUpdateTime, ok := gtpsToOrder[j].Annotations["lastUpdatedAt"]; ok {
					log.Debugf("GTP sorting identity=%s env=%s name1=%s updateTime1=%v priority1=%d name2=%s updateTime2=%v priority2=%d", identity, env, gtpsToOrder[i].Name, iUpdateTime, iPriority, gtpsToOrder[j].Name, jUpdateTime, jPriority)
					return iUpdateTime > jUpdateTime
				}
			}
		}

		log.Debugf("GTP sorting identity=%s env=%s name1=%s creationTime1=%v priority1=%d name2=%s creationTime2=%v priority2=%d", identity, env, gtpsToOrder[i].Name, iCreationTime, iPriority, gtpsToOrder[j].Name, jCreationTime, jPriority)
		return iCreationTime.After(jCreationTime.Time)
	})
}

func getGtpPriority(gtp *v1.GlobalTrafficPolicy) int {
	if val, ok := gtp.ObjectMeta.Labels[GetAdmiralParams().LabelSet.PriorityKey]; ok {
		if convertedValue, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return convertedValue
		}
	}
	return 0
}

func GenerateTxId(meta v12.Object, ctrlName string, id string) string {
	if meta != nil {
		if ctrlName == GTPCtrl {
			annotations := meta.GetAnnotations()
			if len(annotations[IntuitTID]) > 0 {
				id = annotations[IntuitTID] + "-" + id
			}
		}
		if len(meta.GetResourceVersion()) > 0 {
			id = meta.GetResourceVersion() + "-" + id
		}
	}
	return id
}

func IsPresent(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func IsAirEnv(originalEnvLabel string) bool {
	return strings.HasSuffix(originalEnvLabel, AIREnvSuffix)
}

func GetMeshPortsForDeployments(clusterName string, destService *k8sV1.Service,
	destDeployment *k8sAppsV1.Deployment) map[string]uint32 {

	if destService == nil || destDeployment == nil {
		logrus.Warnf("Deployment or Service is nil cluster=%s", clusterName)
		return nil
	}

	var meshPorts string
	if destDeployment.Spec.Template.Annotations == nil {
		meshPorts = ""
	} else {
		meshPorts = destDeployment.Spec.Template.Annotations[SidecarEnabledPorts]
	}
	ports := GetMeshPortsHelper(meshPorts, destService, clusterName)
	return ports
}

func GetMeshPortsHelper(meshPorts string, destService *k8sV1.Service, clusterName string) map[string]uint32 {
	var ports = make(map[string]uint32)

	if destService == nil {
		return ports
	}
	if len(meshPorts) == 0 {
		logrus.Infof(LogFormatAdv, "GetMeshPorts", "service", destService.Name, destService.Namespace,
			clusterName, "No mesh ports present, defaulting to first port")
		if destService.Spec.Ports != nil && len(destService.Spec.Ports) > 0 {
			var protocol = util.GetPortProtocol(destService.Spec.Ports[0].Name)
			ports[protocol] = uint32(destService.Spec.Ports[0].Port)
		}
		return ports
	}

	meshPortsSplit := strings.Split(meshPorts, ",")

	if len(meshPortsSplit) > 1 {
		logrus.Warnf(LogErrFormat, "Get", "MeshPorts", "", clusterName,
			"Multiple inbound mesh ports detected, admiral generates service entry with first matched port and protocol")
	}

	//fetch the first valid port if there is more than one mesh port
	var meshPortMap = make(map[uint32]uint32)
	for _, meshPort := range meshPortsSplit {
		port, err := strconv.ParseUint(meshPort, 10, 32)
		if err == nil {
			meshPortMap[uint32(port)] = uint32(port)
			break
		}
	}
	for _, servicePort := range destService.Spec.Ports {
		//handling relevant protocols from here:
		// https://istio.io/latest/docs/ops/configuration/traffic-management/protocol-selection/#manual-protocol-selection
		//use target port if present to match the annotated mesh port
		targetPort := uint32(servicePort.Port)
		if servicePort.TargetPort.StrVal != "" {
			port, err := strconv.Atoi(servicePort.TargetPort.StrVal)
			if err != nil {
				logrus.Warnf(LogErrFormat, "GetMeshPorts", "Failed to parse TargetPort", destService.Name, clusterName, err)
			}
			if port > 0 {
				targetPort = uint32(port)
			}

		}
		if servicePort.TargetPort.IntVal != 0 {
			targetPort = uint32(servicePort.TargetPort.IntVal)
		}
		if _, ok := meshPortMap[targetPort]; ok {
			var protocol = util.GetPortProtocol(servicePort.Name)
			logrus.Infof(LogFormatAdv, "MeshPort", servicePort.Port, destService.Name, destService.Namespace,
				clusterName, "Protocol: "+protocol)
			ports[protocol] = uint32(servicePort.Port)
			break
		}
	}
	return ports
}

func ShouldIgnore(annotations map[string]string, labels map[string]string) bool {
	labelSet := GetLabelSet()

	//if admiral ignore label set
	if labels[labelSet.AdmiralIgnoreLabel] == "true" { //if we should ignore, do that and who cares what else is there
		return true
	}

	//if sidecar not injected
	if annotations[labelSet.DeploymentAnnotation] != "true" { //Not sidecar injected, we don't want to inject
		return true
	}

	if annotations[AdmiralIgnoreAnnotation] == "true" {
		return true
	}

	return false //labels are fine, we should not ignore
}

func GetIdentityPartition(annotations map[string]string, labels map[string]string) string {
	identityPartition := annotations[GetPartitionIdentifier()]
	if len(identityPartition) == 0 {
		//In case partition is accidentally applied as Label
		identityPartition = labels[GetPartitionIdentifier()]
	}
	return identityPartition
}

func GetGlobalIdentifier(annotations map[string]string, labels map[string]string) string {
	identity := labels[GetWorkloadIdentifier()]
	if len(identity) == 0 {
		//TODO can this be removed now? This was for backward compatibility
		identity = annotations[GetWorkloadIdentifier()]
	}
	if EnableSWAwareNSCaches() && len(identity) > 0 && len(GetIdentityPartition(annotations, labels)) > 0 {
		identity = GetIdentityPartition(annotations, labels) + Sep + strings.ToLower(identity)
	}
	return identity
}

func GetOriginalIdentifier(annotations map[string]string, labels map[string]string) string {
	identity := labels[GetWorkloadIdentifier()]
	if len(identity) == 0 {
		//TODO can this be removed now? This was for backward compatibility
		identity = annotations[GetWorkloadIdentifier()]
	}
	return identity
}

func GenerateUniqueNameForVS(originNamespace string, vsName string) string {

	if originNamespace == "" && vsName == "" {
		return ""
	}

	if originNamespace == "" {
		return vsName
	}

	if vsName == "" {
		return originNamespace
	}

	newVSName := originNamespace + "-" + vsName
	if len(newVSName) > 250 {
		newVSName = newVSName[:250]
	}
	//"op=%v type=%v name=%v namespace=%s cluster=%s message=%v"
	logrus.Debugf(LogFormatAdv, "VirtualService", newVSName, originNamespace, "", "New VS name generated")

	return newVSName

}

func IsAGateway(item string) bool {
	gwAssetAliases := GetGatewayAssetAliases()
	for _, gw := range gwAssetAliases {
		if strings.HasSuffix(strings.ToLower(item), Sep+strings.ToLower(gw)) {
			return true
		}
	}
	return false
}

func GetPartitionAndOriginalIdentifierFromPartitionedIdentifier(partitionedIdentifier string) (string, string) {
	// Given gwAssetAliases = [Org.platform.servicesgateway.servicesgateway], partitionedIdentifier = swx.org.platform.servicesgateway.servicesgateway
	// returns swx, Org.platform.servicesgateway.servicesgateway

	// Given gwAssetAliases = [Org.platform.servicesgateway.servicesgateway], partitionedIdentifier = Org.platform.servicesgateway.servicesgateway
	// returns "", Org.platform.servicesgateway.servicesgateway

	// Given gwAssetAliases = [Org.platform.servicesgateway.servicesgateway], partitionedIdentifier = Abc.foo.bar
	// returns "", Abc.foo.bar
	gwAssetAliases := GetGatewayAssetAliases()
	for _, gw := range gwAssetAliases {
		if strings.HasSuffix(strings.ToLower(partitionedIdentifier), Sep+strings.ToLower(gw)) {
			return strings.TrimSuffix(partitionedIdentifier, Sep+strings.ToLower(gw)), gw
		}
	}
	return "", partitionedIdentifier
}

func IsVSRoutingEnabledVirtualService(vs *v1alpha4.VirtualService) bool {
	if vs == nil {
		return false
	}
	if vs.Labels == nil {
		return false
	}
	if _, ok := vs.Labels[VSRoutingLabel]; !ok {
		return false
	}
	return true
}

func IsVSRoutingInClusterVirtualService(vs *v1alpha4.VirtualService) bool {
	if vs == nil {
		return false
	}
	if vs.Labels == nil {
		return false
	}
	val, ok := vs.Labels[VSRoutingType]
	if !ok {
		return false
	}
	if val != VSRoutingTypeInCluster {
		return false
	}
	return true
}

// IsDefaultFQDN return true if the passed fqdn starts with the env
func IsDefaultFQDN(fqdn, env string) bool {
	if strings.HasPrefix(fqdn, env) {
		return true
	}
	return false
}

func SliceIntersection[T comparable](a []T, b []T) []T {
	set := make([]T, 0)
	hash := make(map[T]struct{})
	for _, v := range a {
		hash[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := hash[v]; ok {
			set = append(set, v)
		}
	}
	return set
}
