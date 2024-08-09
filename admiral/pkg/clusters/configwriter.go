package clusters

import (
	"sort"
	"strconv"
	"strings"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	"github.com/sirupsen/logrus"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
)

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
		err            error
	)
	ctxLogger.Infof(common.CtxLogFormat, "buildServiceEntry", identity, common.GetSyncNamespace(), b.ClientCluster, "Beginning to build the SE spec")
	ingressEndpoints, err := getIngressEndpoints(identityConfig.Clusters)
	if err != nil {
		return serviceEntries, err
	}
	_, isServerOnClientCluster := ingressEndpoints[b.ClientCluster]
	dependentNamespaces, err := getExportTo(ctxLogger, b.RemoteRegistry.RegistryClient, b.ClientCluster, isServerOnClientCluster, identityConfig.ClientAssets)
	if err != nil {
		return serviceEntries, err
	}
	for _, identityConfigCluster := range identityConfig.Clusters {
		serverCluster := identityConfigCluster.Name
		for _, identityConfigEnvironment := range identityConfigCluster.Environment {
			env := identityConfigEnvironment.Name
			var tmpSe *networkingV1Alpha3.ServiceEntry
			ep, err := getServiceEntryEndpoint(ctxLogger, b.ClientCluster, serverCluster, ingressEndpoints, identityConfigEnvironment)
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
					Endpoints:       []*networkingV1Alpha3.WorkloadEntry{ep},
					ExportTo:        dependentNamespaces,
				}
			} else {
				tmpSe = se
				tmpSe.Endpoints = append(tmpSe.Endpoints, ep)
			}
			sort.Sort(WorkloadEntrySorted(tmpSe.Endpoints))
			serviceEntries = append(serviceEntries, tmpSe)
		}
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
func getServiceEntryEndpoint(
	ctxLogger *logrus.Entry,
	clientCluster string,
	serverCluster string,
	ingressEndpoints map[string]*networkingV1Alpha3.WorkloadEntry,
	identityConfigEnvironment *registry.IdentityConfigEnvironment) (*networkingV1Alpha3.WorkloadEntry, error) {
	//TODO: Verify Local and Remote Endpoints are constructed correctly
	var err error
	endpoint := ingressEndpoints[serverCluster]
	tmpEp := endpoint.DeepCopy()
	tmpEp.Labels["type"] = identityConfigEnvironment.Type
	if clientCluster == serverCluster {
		for _, service := range identityConfigEnvironment.Services {
			if service.Weight == -1 {
				// its not a weighted service, which means we should have only one service endpoint
				//Local Endpoint Address if the identity is deployed on the same cluster as it's client and the endpoint is the remote endpoint for the cluster
				tmpEp.Address = service.Name + common.Sep + identityConfigEnvironment.Namespace + common.GetLocalDomainSuffix()
				tmpEp.Ports = service.Ports
				break
			}
			// TODO: this needs fixing, because there can be multiple services when we have weighted endpoints, and this
			// will only choose one service
			tmpEp.Address = service.Name + common.Sep + identityConfigEnvironment.Namespace + common.GetLocalDomainSuffix()
			tmpEp.Ports = service.Ports
		}
	}
	return tmpEp, err
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
			// TODO: this should return an error.
			ctxLogger.Infof(common.CtxLogFormat, "buildServiceEntry", clientAsset, common.GetSyncNamespace(), "", "could not fetch IdentityConfig: "+err.Error())
			continue
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
