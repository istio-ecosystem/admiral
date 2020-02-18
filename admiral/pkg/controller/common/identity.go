package common

import "istio.io/istio/pkg/log"

var workloadIdentifier = Identity

func GetWorkloadIdentifier() string {
	return workloadIdentifier
}

func OverrideDefaultWorkloadIdentifier(newIdentifier string) {
	if len(newIdentifier) >0 {
		workloadIdentifier = newIdentifier
		log.Infof("Workload identifier overridden to %v", workloadIdentifier)
	}
}