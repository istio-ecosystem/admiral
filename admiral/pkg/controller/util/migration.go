package util

import (
	"fmt"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	networking "istio.io/api/networking/v1alpha3"
	k8sV1 "k8s.io/api/core/v1"
)

// UpdateEndpointsForDeployToRolloutMigration creates an SE with the endpoints for both the rollout and deployment
// This is for Deployment <-> Rollout migration
func UpdateEndpointsForDeployToRolloutMigration(serviceInstance map[string]*k8sV1.Service,
	serviceEntry *networking.ServiceEntry, meshPorts map[string]map[string]uint32, clusterIngress string,
	clusterAppDeleteMap map[string]string, clusterName string,
	clusterDeployRolloutPresent map[string]map[string]bool) ([]*k8sV1.Service, error) {
	if serviceInstance[common.Deployment] == nil || serviceInstance[common.Rollout] == nil {
		return nil, fmt.Errorf("serviceInstance for Deployment/Rollout is nil as the service cache has not updated yet")
	}

	deployLocalFqdn := serviceInstance[common.Deployment].Name + common.Sep + serviceInstance[common.Deployment].Namespace + common.GetLocalDomainSuffix()
	rolloutFqdn := serviceInstance[common.Rollout].Name + common.Sep + serviceInstance[common.Rollout].Namespace + common.GetLocalDomainSuffix()
	var requiredServices []*k8sV1.Service

	var uniqueEndpointsList []*networking.WorkloadEntry
	for _, ep := range serviceEntry.Endpoints {
		// only if the ep.Address is equal to clusterIngress do we append the deployment
		// and rollout endpoint for add and update events.
		// For delete events we check for which cluster did we get the event for and then
		// decide which cluster to remove the deployment or rollout endpoint for.
		if ep.Address == clusterIngress {
			if clusterAppDeleteMap[clusterName] != common.Deployment && clusterDeployRolloutPresent[clusterName][common.Deployment] {
				deployEp := &networking.WorkloadEntry{
					Address:  deployLocalFqdn,
					Locality: ep.Locality,
					Ports:    meshPorts[common.Deployment],
					Labels:   map[string]string{"type": common.Deployment, "security.istio.io/tlsMode": "istio"},
				}
				uniqueEndpointsList = append(uniqueEndpointsList, deployEp)
				requiredServices = append(requiredServices, serviceInstance[common.Deployment])
			}

			if clusterAppDeleteMap[clusterName] != common.Rollout && clusterDeployRolloutPresent[clusterName][common.Rollout] {
				rolloutEp := &networking.WorkloadEntry{
					Address:  rolloutFqdn,
					Locality: ep.Locality,
					Ports:    meshPorts[common.Rollout],
					Labels:   map[string]string{"type": common.Rollout, "security.istio.io/tlsMode": "istio"},
				}
				uniqueEndpointsList = append(uniqueEndpointsList, rolloutEp)
				requiredServices = append(requiredServices, serviceInstance[common.Rollout])
			}
		} else {
			// TODO: check when will this be applicable, and then
			// update the required service accordingly
			delete(ep.Labels, "type")
			uniqueEndpointsList = append(uniqueEndpointsList, ep)
		}
	}

	serviceEntry.Endpoints = uniqueEndpointsList

	return requiredServices, nil
}
