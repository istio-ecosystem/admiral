package common

import (
	"sort"
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

//Returns the list of deployments to which this GTP should apply. It is assumed that all inputs already are an identity match
//If the GTP has an identity label, it should match all deployments which share that label
//If the GTP does not have an identity label, it should return all deployments without an identity label
//IMPORTANT: If an environment label is specified on either the GTP or the deployment, the same value must be specified on the other for them to match
func MatchDeploymentsToGTP(gtp *v1.GlobalTrafficPolicy, deployments []k8sAppsV1.Deployment) []k8sAppsV1.Deployment {
	if gtp == nil || gtp.Name == "" {
		log.Warn("Nil or empty GlobalTrafficPolicy provided for deployment match. Returning nil.")
		return nil
	}

	gtpEnv := GetGtpEnv(gtp)

	if len(deployments) == 0 {
		return nil
	}

	var envMatchedDeployments []k8sAppsV1.Deployment

	for _, deployment := range deployments {
		deploymentEnvironment := GetEnv(&deployment)
		if deploymentEnvironment == gtpEnv {
			envMatchedDeployments = append(envMatchedDeployments, deployment)
		}
	}

	if len(envMatchedDeployments) == 0 {
		return nil
	}

	for _, deployment := range deployments {
		log.Infof("Newly added GTP with name=%v matched with Deployment %v in namespace %v. Env=%v", gtp.Name, deployment.Name, deployment.Namespace, gtpEnv)
	}
	return envMatchedDeployments
}

//Find the GTP that best matches the deployment.
//It's assumed that the set of GTPs passed in has already been matched via the GtpDeploymentLabel. Now it's our job to choose the best one.
//In order:
// - If one and only one GTP matches the env label of the deployment - use that one. Use "default" as the default env label for all GTPs and deployments.
// - If multiple GTPs match the deployment label, use the oldest one (Using an old one has less chance of new behavior which could impact workflows)
//IMPORTANT: If an environment label is specified on either the GTP or the deployment, the same value must be specified on the other for them to match
func MatchGTPsToDeployment(gtpList []v1.GlobalTrafficPolicy, deployment *k8sAppsV1.Deployment) *v1.GlobalTrafficPolicy {
	if deployment == nil || deployment.Name == "" {
		log.Warn("Nil or empty GlobalTrafficPolicy provided for deployment match. Returning nil.")
		return nil
	}
	deploymentEnvironment := GetEnv(deployment)

	//If one and only one GTP matches the env label of the deployment - use that one
	if len(gtpList) == 1 {
		gtpEnv := GetGtpEnv(&gtpList[0])
		if gtpEnv == deploymentEnvironment {
			log.Infof("Newly added deployment with name=%v matched with GTP %v in namespace %v. Env=%v", deployment.Name, gtpList[0].Name, deployment.Namespace, gtpEnv)
			return &gtpList[0]
		} else {
			return nil
		}
	}

	if len(gtpList) == 0 {
		return nil
	}

	var envMatchedGTPList []v1.GlobalTrafficPolicy

	for _, gtp := range gtpList {
		gtpEnv := GetGtpEnv(&gtp)
		if gtpEnv == deploymentEnvironment {
			envMatchedGTPList = append(envMatchedGTPList, gtp)
		}
	}

	//if one matches the environment from the gtp, return it
	if len(envMatchedGTPList) == 1 {
		log.Infof("Newly added deployment with name=%v matched with GTP %v in namespace %v. Env=%v", deployment.Name, envMatchedGTPList[0].Name, deployment.Namespace, deploymentEnvironment)
		return &envMatchedGTPList[0]
	}

	//No GTPs matched the environment label
	if len(envMatchedGTPList) == 0 {
		return nil
	}

	//Using age as a tiebreak
	sort.Slice(envMatchedGTPList, func(i, j int) bool {
		iTime := envMatchedGTPList[i].CreationTimestamp.Nanosecond()
		jTime := envMatchedGTPList[j].CreationTimestamp.Nanosecond()
		return iTime < jTime
	})

	log.Warnf("Multiple GTPs found that match the deployment with name=%v in namespace %v. Using the oldest one, you may want to clean up your configs to prevent this in the future", deployment.Name, deployment.Namespace)
	//return oldest gtp
	log.Infof("Newly added deployment with name=%v matched with GTP %v in namespace %v. Env=%v", deployment.Name, envMatchedGTPList[0].Name, deployment.Namespace, deploymentEnvironment)
	return &envMatchedGTPList[0]

}

func GetGtpEnv(gtp *v1.GlobalTrafficPolicy) string {
	var environment = gtp.Annotations[GetEnvKey()]
	if len(environment) == 0 {
		environment = gtp.Labels[GetEnvKey()]
	}
	if len(environment) == 0 {
		environment = gtp.Labels[Env]
		log.Warnf("Using deprecated approach to use env label for GTP, name=%v in namespace=%v", gtp.Name, gtp.Namespace)
	}
	if len(environment) == 0 {
		environment = Default
	}
	return environment
}
