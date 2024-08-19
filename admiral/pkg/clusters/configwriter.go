package clusters

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	"github.com/sirupsen/logrus"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
)

const typeLabel = "type"

// IstioSEBuilder is an interface to construct Service Entry objects
// from IdentityConfig objects. It can construct multiple Service Entries
// from an IdentityConfig or construct just one given a IdentityConfigEnvironment.
type IstioSEBuilder interface {
	BuildServiceEntriesFromIdentityConfig(ctxLogger *logrus.Entry, event admiral.EventType, identityConfig registry.IdentityConfig) ([]*networkingV1Alpha3.ServiceEntry, error)
}

type ServiceEntryBuilder struct {
	RemoteRegistry *RemoteRegistry
	ClientCluster  string
}

// BuildServiceEntriesFromIdentityConfig builds service entries to write to the client cluster
// by looping through the IdentityConfig clusters and environments to get spec information. It
// builds one SE per environment per cluster the identity is deployed in.
func (b *ServiceEntryBuilder) BuildServiceEntriesFromIdentityConfig(ctxLogger *logrus.Entry, identityConfig registry.IdentityConfig) ([]*networkingV1Alpha3.ServiceEntry, error) {
	var (
		identity       = identityConfig.IdentityName
		seMap          = map[string]*networkingV1Alpha3.ServiceEntry{}
		serviceEntries = []*networkingV1Alpha3.ServiceEntry{}
		start          = time.Now()
		err            error
	)
	defer util.LogElapsedTime("BuildServiceEntriesFromIdentityConfig", identity, common.GetOperatorSyncNamespace(), b.ClientCluster)
	ctxLogger.Infof(common.CtxLogFormat, "BuildServiceEntriesFromIdentityConfig", identity, common.GetOperatorSyncNamespace(), b.ClientCluster, "Beginning to build the SE spec")
	ingressEndpoints, err := getIngressEndpoints(identityConfig.Clusters)
	util.LogElapsedTimeSince("getIngressEndpoints", identity, "", b.ClientCluster, start)
	if err != nil || len(ingressEndpoints) == 0 {
		return serviceEntries, err
	}
	start = time.Now()
	_, isServerOnClientCluster := ingressEndpoints[b.ClientCluster]
	dependentNamespaces, err := getExportTo(ctxLogger, b.RemoteRegistry.RegistryClient, b.ClientCluster, isServerOnClientCluster, identityConfig.ClientAssets)
	util.LogElapsedTimeSince("getExportTo", identity, "", b.ClientCluster, start)
	if err != nil {
		return serviceEntries, err
	}
	for _, identityConfigCluster := range identityConfig.Clusters {
		serverCluster := identityConfigCluster.Name
		for _, identityConfigEnvironment := range identityConfigCluster.Environment {
			env := identityConfigEnvironment.Name
			var tmpSe *networkingV1Alpha3.ServiceEntry
			start = time.Now()
			endpoints, err := getServiceEntryEndpoints(ctxLogger, b.ClientCluster, serverCluster, ingressEndpoints, identityConfigEnvironment)
			util.LogElapsedTimeSince("getServiceEntryEndpoint", identity, env, b.ClientCluster, start)
			if err != nil {
				return serviceEntries, err
			}
			if se, ok := seMap[env]; !ok {
				tmpSe = &networkingV1Alpha3.ServiceEntry{
					Hosts:           []string{common.GetCnameVal([]string{env, strings.ToLower(identity), common.GetHostnameSuffix()})},
					Ports:           identityConfigEnvironment.Ports,
					Location:        networkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution:      networkingV1Alpha3.ServiceEntry_DNS,
					SubjectAltNames: []string{common.SpiffePrefix + common.GetSANPrefix() + common.Slash + identity},
					Endpoints:       endpoints,
					ExportTo:        dependentNamespaces,
				}
			} else {
				tmpSe = se
				tmpSe.Endpoints = append(tmpSe.Endpoints, endpoints...)
			}
			sort.Sort(WorkloadEntrySorted(tmpSe.Endpoints))
			seMap[env] = tmpSe
		}
	}
	for _, se := range seMap {
		serviceEntries = append(serviceEntries, se)
	}
	return serviceEntries, err
}

// getIngressEndpoints constructs the endpoint of the ingress gateway/remote endpoint for an identity
// by reading the information directly from the IdentityConfigCluster.
func getIngressEndpoints(clusters map[string]*registry.IdentityConfigCluster) (map[string]*networkingV1Alpha3.WorkloadEntry, error) {
	ingressEndpoints := map[string]*networkingV1Alpha3.WorkloadEntry{}
	var err error
	for _, cluster := range clusters {
		portNumber, err := strconv.ParseInt(cluster.IngressPort, 10, 64)
		if err != nil {
			return ingressEndpoints, err
		}
		ingressEndpoint := &networkingV1Alpha3.WorkloadEntry{
			Address:  cluster.IngressEndpoint,
			Locality: cluster.Locality,
			Ports:    map[string]uint32{cluster.IngressPortName: uint32(portNumber)},
			Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
		}
		ingressEndpoints[cluster.Name] = ingressEndpoint
	}
	return ingressEndpoints, err
}

// getServiceEntryEndpoint constructs the remote or local endpoints of the service entry that
// should be built for the given identityConfigEnvironment.
func getServiceEntryEndpoints(
	ctxLogger *logrus.Entry,
	clientCluster string,
	serverCluster string,
	ingressEndpoints map[string]*networkingV1Alpha3.WorkloadEntry,
	identityConfigEnvironment *registry.IdentityConfigEnvironment) ([]*networkingV1Alpha3.WorkloadEntry, error) {
	if len(identityConfigEnvironment.Services) == 0 {
		return nil, fmt.Errorf("there were no services for the asset in namespace %s on cluster %s", identityConfigEnvironment.Namespace, serverCluster)
	}
	var err error
	endpoint := ingressEndpoints[serverCluster]
	endpoints := []*networkingV1Alpha3.WorkloadEntry{}
	tmpEp := endpoint.DeepCopy()
	tmpEp.Labels[typeLabel] = identityConfigEnvironment.Type
	services := []*registry.RegistryServiceConfig{}
	for _, service := range identityConfigEnvironment.Services {
		services = append(services, service)
	}
	sort.Sort(registry.RegistryServiceConfigSorted(services))
	// Deployment won't have weights, so just sort and take the first service to use as the endpoint
	if identityConfigEnvironment.Type == common.Deployment {
		if clientCluster == serverCluster {
			tmpEp.Address = services[0].Name + common.Sep + identityConfigEnvironment.Namespace + common.GetLocalDomainSuffix()
			tmpEp.Ports = services[0].Ports
		}
		endpoints = append(endpoints, tmpEp)
	}
	// Rollout without weights is treated the same as deployment so sort and take first service
	// If any of the services have weights then add them to the list of endpoints
	if identityConfigEnvironment.Type == common.Rollout {
		for _, service := range services {
			if service.Weight > 0 {
				weightedEp := tmpEp.DeepCopy()
				weightedEp.Weight = uint32(service.Weight)
				if clientCluster == serverCluster {
					weightedEp.Address = service.Name + common.Sep + identityConfigEnvironment.Namespace + common.GetLocalDomainSuffix()
					weightedEp.Ports = service.Ports
				}
				endpoints = append(endpoints, weightedEp)
			}
		}
		// If we go through all the services associated with the rollout and none have applicable weights then endpoints is empty
		// Treat the rollout like a deployment and sort and take the first service
		if len(endpoints) == 0 {
			if clientCluster == serverCluster {
				tmpEp.Address = services[0].Name + common.Sep + identityConfigEnvironment.Namespace + common.GetLocalDomainSuffix()
				tmpEp.Ports = services[0].Ports
			}
			endpoints = append(endpoints, tmpEp)
		}
	}
	// TODO: type is rollout, strategy is bluegreen, need a way to know which service is preview/desired, trigger another SE
	// TODO: type is rollout, strategy is canary, need a way to know which service is stable/root/desired, trigger another SE
	// TODO: two types in the environment, deployment to rollout migration
	return endpoints, err
}

// getExportTo constructs a sorted list of unique namespaces for a given cluster, client assets,
// and cname, where each namespace is where a client asset of the cname is deployed on the cluster. If the cname
// is also deployed on the cluster then the istio-system namespace is also in the list.
func getExportTo(ctxLogger *logrus.Entry, registryClient registry.IdentityConfiguration, clientCluster string, isServerOnClientCluster bool, clientAssets map[string]string) ([]string, error) {
	clientNamespaces := []string{}
	var err error
	var clientIdentityConfig registry.IdentityConfig
	for clientAsset := range clientAssets {
		// For each client asset of cname, we fetch its identityConfig
		clientIdentityConfig, err = registryClient.GetIdentityConfigByIdentityName(clientAsset, ctxLogger)
		if err != nil {
			ctxLogger.Infof(common.CtxLogFormat, "buildServiceEntry", clientAsset, common.GetSyncNamespace(), "", "could not fetch IdentityConfig: "+err.Error())
			return clientNamespaces, err
		}
		for _, clientIdentityConfigCluster := range clientIdentityConfig.Clusters {
			// For each cluster the client asset is deployed on, we check if that cluster is the client cluster we are writing to
			if clientCluster == clientIdentityConfigCluster.Name {
				for _, clientIdentityConfigEnvironment := range clientIdentityConfigCluster.Environment {
					// For each environment of the client asset on the client cluster, we add the namespace to our list
					//Do we need to check if ENV matches here for exportTo? Currently we don't, but we could
					clientNamespaces = append(clientNamespaces, clientIdentityConfigEnvironment.Namespace)
				}
			}
		}
	}
	if isServerOnClientCluster {
		clientNamespaces = append(clientNamespaces, common.NamespaceIstioSystem)
	}
	if len(clientNamespaces) > common.GetExportToMaxNamespaces() {
		clientNamespaces = []string{"*"}
	}
	sort.Strings(clientNamespaces)
	var dedupClientNamespaces []string
	for i := 0; i < len(clientNamespaces); i++ {
		if i == 0 || clientNamespaces[i] != clientNamespaces[i-1] {
			dedupClientNamespaces = append(dedupClientNamespaces, clientNamespaces[i])
		}
	}
	return clientNamespaces, err
}
