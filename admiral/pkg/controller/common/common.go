package common

import (
	"fmt"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	log "github.com/sirupsen/logrus"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
)

const (
	NamespaceKubeSystem           = "kube-system"
	NamespaceIstioSystem          = "istio-system"
	Env                           = "env"
	Http                          = "http"
	Grpc                          = "grpc"
	GrpcWeb                       = "grpc-web"
	Http2                         = "http2"
	DefaultMtlsPort               = 15443
	DefaultServiceEntryPort       = 80
	Sep                           = "."
	Dash                          = "-"
	Slash                         = "/"
	DotLocalDomainSuffix          = ".svc.cluster.local"
	Mesh                          = "mesh"
	MulticlusterIngressGateway    = "istio-multicluster-ingressgateway"
	LocalAddressPrefix            = "240.0"
	NodeRegionLabel               = "failure-domain.beta.kubernetes.io/region"
	SpiffePrefix                  = "spiffe://"
	SidecarEnabledPorts           = "traffic.sidecar.istio.io/includeInboundPorts"
	Default                       = "default"
	AdmiralIgnoreAnnotation       = "admiral.io/ignore"
	AdmiralCnameCaseSensitive     = "admiral.io/cname-case-sensitive"
	BlueGreenRolloutPreviewPrefix = "preview"
)

type Event int

const (
	Add    Event = 0
	Update Event = 1
	Delete Event = 2
)

type ResourceType string

const (
	VirtualService  ResourceType = "VirtualService"
	DestinationRule ResourceType = "DestinationRule"
	ServiceEntry    ResourceType = "ServiceEntry"
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
	return identity
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
		log.Infof("admiral.io/cname-case-sensitive annotation enabled on deployment with name %v", deployment.Name)
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
	var environment = gtp.Annotations[GetEnvKey()]
	if len(environment) == 0 {
		environment = gtp.Labels[GetEnvKey()]
	}
	if len(environment) == 0 {
		environment = gtp.Spec.Selector[GetEnvKey()]
	}
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
	identity := gtp.Labels[GetGlobalTrafficDeploymentLabel()]
	if len(identity) == 0 {
		identity = gtp.Spec.Selector[GetGlobalTrafficDeploymentLabel()]
	}
	return identity
}

func GetGtpKey(gtp *v1.GlobalTrafficPolicy) string {
	return ConstructGtpKey(GetGtpEnv(gtp), GetGtpIdentity(gtp))
}

func ConstructGtpKey(env, identity string) string {
	return fmt.Sprintf("%s.%s", env, identity)
}

func ShouldIgnoreResource(metadata v12.ObjectMeta) bool {
	return  metadata.Annotations[AdmiralIgnoreAnnotation] == "true" || metadata.Labels[AdmiralIgnoreAnnotation] == "true"
}
