package common

import (
	"github.com/sirupsen/logrus"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	"strings"
)

const (
	NamespaceKubeSystem        = "kube-system"
	NamespaceIstioSystem       = "istio-system"
	Env                        = "env"
	Identity                   = "identity"
	Http                       = "http"
	DefaultMtlsPort            = 15443
	DefaultHttpPort            = 80
	Sep                        = "."
	Dash                       = "-"
	Slash                      = "/"
	DotLocalDomainSuffix       = ".svc.cluster.local"
	Mesh                       = "mesh"
	MulticlusterIngressGateway = "istio-multicluster-ingressgateway"
	LocalAddressPrefix         = "240.0"
	NodeRegionLabel            = "failure-domain.beta.kubernetes.io/region"
	SpiffePrefix               = "spiffe://"
	SidecarEnabledPorts        = "traffic.sidecar.istio.io/includeInboundPorts"
	Default                    = "default"
)

func GetPodGlobalIdentifier(pod *k8sV1.Pod) string {
	identity := pod.Labels[DefaultGlobalIdentifier()]
	if len(identity) == 0 {
		identity = pod.Annotations[DefaultGlobalIdentifier()]
	}
	return identity
}

func GetDeploymentGlobalIdentifier(deployment *k8sAppsV1.Deployment) string {
	identity := deployment.Spec.Template.Labels[DefaultGlobalIdentifier()]
	if len(identity) == 0 {
		//TODO can this be removed now? This was for backward compatibility
		identity = deployment.Spec.Template.Annotations[DefaultGlobalIdentifier()]
	}
	return identity
}

func DefaultGlobalIdentifier() string {
	return Identity
}

// GetCname returns cname in the format <env>.<service identity>.global, Ex: stage.Admiral.services.registry.global
func GetCname(deployment *k8sAppsV1.Deployment, identifier string, nameSuffix string) string {
	var environment = GetEnv(deployment)
	alias := deployment.Spec.Template.Labels[identifier]
	if len(alias) == 0 {
		logrus.Warnf("%v label missing on service %v in namespace %v. Falling back to annotation to create cname.", identifier, deployment.Name, deployment.Namespace)
		alias = deployment.Spec.Template.Annotations[identifier]
	}
	if len(alias) == 0 {
		logrus.Errorf("Unable to get cname for service with name %v in namespace %v as it doesn't have the %v annotation", deployment.Name, deployment.Namespace, identifier)
		return ""
	}
	return environment + Sep + alias + Sep + nameSuffix
}

func GetEnv(deployment *k8sAppsV1.Deployment) string {
	var environment = deployment.Spec.Template.Labels[Env]
	if len(environment) == 0 {
		environment = deployment.Spec.Template.Annotations[Env]
	}
	if len(environment) == 0 {
		splitNamespace := strings.Split(deployment.Namespace, Dash)
		environment = splitNamespace[len(splitNamespace) - 1]
	}
	if len(environment) == 0 {
		environment = Default
	}
	return environment
}

// GetSAN returns SAN for a service entry in the format spiffe://<domain>/<identifier>, Ex: spiffe://subdomain.domain.com/Admiral.platform.mesh.server
func GetSAN(domain string, deployment *k8sAppsV1.Deployment, identifier string) string {
	identifierVal := deployment.Spec.Template.Labels[identifier]
	if len(identifierVal) == 0 {
		return ""
	}
	if len(domain) > 0 {
		return SpiffePrefix + domain + Slash + identifierVal
	} else {
		return SpiffePrefix + identifierVal
	}
}

func GetNodeLocality(node *k8sV1.Node) string {
	region, _ := node.Labels[NodeRegionLabel]
	return region
}
