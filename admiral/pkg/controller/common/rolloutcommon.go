package common

import (
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	log "github.com/sirupsen/logrus"
	"sort"
	"strings"
)

// GetCname returns cname in the format <env>.<service identity>.global, Ex: stage.Admiral.services.registry.global
func GetCnameForRollout(rollout *argo.Rollout, identifier string, nameSuffix string) string {
	var environment = GetEnvForRollout(rollout)
	alias := GetValueForKeyFromRollout(identifier, rollout)
	if len(alias) == 0 {
		log.Warnf("%v label missing on deployment %v in namespace %v. Falling back to annotation to create cname.", identifier, rollout.Name, rollout.Namespace)
		alias = rollout.Spec.Template.Annotations[identifier]
	}
	if len(alias) == 0 {
		log.Errorf("Unable to get cname for deployment with name %v in namespace %v as it doesn't have the %v annotation", rollout.Name, rollout.Namespace, identifier)
		return ""
	}
	cname := environment + Sep + alias + Sep + nameSuffix
	if rollout.Spec.Template.Annotations[AdmiralCnameCaseSensitive] == "true" {
		log.Infof("admiral.io/cname-case-sensitive annotation enabled on rollout with name %v", rollout.Name)
		return cname
	}
	return strings.ToLower(cname)
}

// GetSAN returns SAN for a service entry in the format spiffe://<domain>/<identifier>, Ex: spiffe://subdomain.domain.com/Admiral.platform.mesh.server
func GetSANForRollout(domain string, rollout *argo.Rollout, identifier string) string {
	identifierVal := GetValueForKeyFromRollout(identifier, rollout)
	if len(identifierVal) == 0 {
		log.Errorf("Unable to get SAN for rollout with name %v in namespace %v as it doesn't have the %v annotation or label", rollout.Name, rollout.Namespace, identifier)
		return ""
	}
	if len(domain) > 0 {
		return SpiffePrefix + domain + Slash + identifierVal
	} else {
		return SpiffePrefix + identifierVal
	}
}

func GetValueForKeyFromRollout(key string, rollout *argo.Rollout) string {
	value := rollout.Spec.Template.Labels[key]
	if len(value) == 0 {
		log.Warnf("%v label missing on deployment %v in namespace %v. Falling back to annotation.", key, rollout.Name, rollout.Namespace)
		value = rollout.Spec.Template.Annotations[key]
	}
	return value
}

//Returns the list of rollouts to which this GTP should apply. It is assumed that all inputs already are an identity match
//If the GTP has an identity label, it should match all rollouts which share that label
//If the GTP does not have an identity label, it should return all rollouts without an identity label
//IMPORTANT: If an environment label is specified on either the GTP or the rollout, the same value must be specified on the other for them to match
func MatchRolloutsToGTP(gtp *v1.GlobalTrafficPolicy, rollouts []argo.Rollout) []argo.Rollout {
	if gtp == nil || gtp.Name == "" {
		log.Warn("Nil or empty GlobalTrafficPolicy provided for rollout match. Returning nil.")
		return nil
	}

	gtpEnv := gtp.Labels[GetEnvLabel()]
	if gtpEnv == "" {
		gtpEnv = Default
	}

	if len(rollouts) == 0 {
		return nil
	}

	var envMatchedRollouts []argo.Rollout

	for _, rollout := range rollouts {
		rolloutEnvironment := rollout.Spec.Template.Labels[GetEnvLabel()]
		if rolloutEnvironment == "" {
			//No environment label, use default value
			rolloutEnvironment = Default
		}
		if rolloutEnvironment == gtpEnv {
			envMatchedRollouts = append(envMatchedRollouts, rollout)
		}
	}

	if len(envMatchedRollouts) == 0 {
		return nil
	}

	for _, rollout := range rollouts {
		log.Infof("Newly added GTP with name=%v matched with Rollout %v in namespace %v. Env=%v", gtp.Name, rollout.Name, rollout.Namespace, gtpEnv)
	}
	return envMatchedRollouts
}

func GetRolloutGlobalIdentifier(rollout *argo.Rollout) string {
	identity := rollout.Spec.Template.Labels[GetWorkloadIdentifier()]
	if len(identity) == 0 {
		//TODO can this be removed now? This was for backward compatibility
		identity = rollout.Spec.Template.Annotations[GetWorkloadIdentifier()]
	}
	return identity
}

//Find the GTP that best matches the rollout.
//It's assumed that the set of GTPs passed in has already been matched via the GtprolloutLabel. Now it's our job to choose the best one.
//In order:
// - If one and only one GTP matches the env label of the rollout - use that one. Use "default" as the default env label for all GTPs and rollout.
// - If multiple GTPs match the rollout label, use the oldest one (Using an old one has less chance of new behavior which could impact workflows)
//IMPORTANT: If an environment label is specified on either the GTP or the rollout, the same value must be specified on the other for them to match
func MatchGTPsToRollout(gtpList []v1.GlobalTrafficPolicy, rollout *argo.Rollout) *v1.GlobalTrafficPolicy {
	if rollout == nil || rollout.Name == "" {
		log.Warn("Nil or empty GlobalTrafficPolicy provided for rollout match. Returning nil.")
		return nil
	}
	rolloutEnvironment := rollout.Spec.Template.Labels[GetEnvLabel()]
	if rolloutEnvironment == "" {
		//No environment label, use default value
		rolloutEnvironment = Default
	}

	//If one and only one GTP matches the env label of the rollout - use that one
	if len(gtpList) == 1 {
		gtpEnv := gtpList[0].Labels[GetEnvLabel()]
		if gtpEnv == "" {
			gtpEnv = Default
		}
		if gtpEnv == rolloutEnvironment {
			log.Infof("Newly added rollout with name=%v matched with GTP %v in namespace %v. Env=%v", rollout.Name, gtpList[0].Name, rollout.Namespace, gtpEnv)
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
		gtpEnv := gtp.Labels[GetEnvLabel()]
		if gtpEnv == "" {
			gtpEnv = Default
		}
		if gtpEnv == rolloutEnvironment {
			envMatchedGTPList = append(envMatchedGTPList, gtp)
		}
	}

	//if one matches the environment from the gtp, return it
	if len(envMatchedGTPList) == 1 {
		log.Infof("Newly added rollout with name=%v matched with GTP %v in namespace %v. Env=%v", rollout.Name, envMatchedGTPList[0].Name, rollout.Namespace, rolloutEnvironment)
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

	log.Warnf("Multiple GTPs found that match the rollout with name=%v in namespace %v. Using the oldest one, you may want to clean up your configs to prevent this in the future", rollout.Name, rollout.Namespace)
	//return oldest gtp
	log.Infof("Newly added rollout with name=%v matched with GTP %v in namespace %v. Env=%v", rollout.Name, envMatchedGTPList[0].Name, rollout.Namespace, rolloutEnvironment)
	return &envMatchedGTPList[0]

}

func GetEnvForRollout(rollout *argo.Rollout) string {
	var environment = rollout.Spec.Template.Labels[GetEnvLabel()]
	if len(environment) == 0 {
		environment = rollout.Spec.Template.Annotations[GetEnvLabel()]
	}
	if len(environment) == 0 {
		splitNamespace := strings.Split(rollout.Namespace, Dash)
		if len(splitNamespace) > 1 {
			environment = splitNamespace[len(splitNamespace)-1]
		}
	}
	if len(environment) == 0 {
		environment = Default
	}
	return environment
}
