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

const (
	typeLabel         = "type"
	testServiceKey    = "canary"
	defaultServiceKey = "default"
	canaryPrefix      = "canary"
	previewPrefix     = "preview"
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
		seMap          = map[string]map[string]*networkingV1Alpha3.ServiceEntry{}
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
			if len(identityConfigEnvironment.Services) == 0 {
				return serviceEntries, fmt.Errorf("there were no services for the asset in namespace %s on cluster %s", identityConfigEnvironment.Namespace, serverCluster)
			}

			start = time.Now()
			meshHosts := getMeshHosts(identity, identityConfigEnvironment)
			for _, host := range meshHosts {
				var tmpSe *networkingV1Alpha3.ServiceEntry
				endpoints, err := getServiceEntryEndpoints(ctxLogger, b.ClientCluster, serverCluster, host, ingressEndpoints, identityConfigEnvironment)
				util.LogElapsedTimeSince("getServiceEntryEndpoint", identity, env, b.ClientCluster, start)
				if err != nil {
					return serviceEntries, err
				}
				if se, ok := seMap[env][host]; !ok {
					tmpSe = &networkingV1Alpha3.ServiceEntry{
						Hosts:           []string{host},
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
				seMap[env] = map[string]*networkingV1Alpha3.ServiceEntry{host: tmpSe}
			}
		}
	}
	for _, seForEnv := range seMap {
		for _, se := range seForEnv {
			serviceEntries = append(serviceEntries, se)
		}
	}
	return serviceEntries, err
}

func getMeshHosts(identity string, identityConfigEnvironment *registry.IdentityConfigEnvironment) []string {
	meshHosts := []string{}
	meshHosts = append(meshHosts, common.GetCnameVal([]string{identityConfigEnvironment.Name, strings.ToLower(identity), common.GetHostnameSuffix()}))
	if identityConfigEnvironment.Type[common.Rollout] != nil {
		strategy := identityConfigEnvironment.Type[common.Rollout].Strategy
		if strategy == bluegreenStrategy {
			meshHosts = append(meshHosts, common.GetCnameVal([]string{previewPrefix, strings.ToLower(identity), common.GetHostnameSuffix()}))
		}
		if strategy == canaryStrategy {
			meshHosts = append(meshHosts, common.GetCnameVal([]string{canaryPrefix, strings.ToLower(identity), common.GetHostnameSuffix()}))
		}
	}
	return meshHosts
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
	host string,
	ingressEndpoints map[string]*networkingV1Alpha3.WorkloadEntry,
	identityConfigEnvironment *registry.IdentityConfigEnvironment) ([]*networkingV1Alpha3.WorkloadEntry, error) {
	var err error
	services := identityConfigEnvironment.Services
	endpoint := ingressEndpoints[serverCluster]
	endpoints := []*networkingV1Alpha3.WorkloadEntry{}

	if services == nil {
		return endpoints, fmt.Errorf("services are nil for identityConfigEnvironment %s", identityConfigEnvironment.Name)
	}

	// Logic to determine which services should be against default (like whether have both rollout and deployment, and which service for which type) will move to state syncer
	// Also state syncer will be responsible for setting the weight of the services, and removing services without weights if one has a weight

	// Services will have 2 keys at max - default and testSvc
	// a. Non istio canary rollout will have only default - which will have root svc
	// b. Istio canary rollout:
	// 1. If weights present - Default key will have stable, canary svc with weights. The latter can be part of testSvc key as well
	// 2. If no weights present - Default key will have stable svc, testSvc will have canary svc
	// c. Blue green rollout will have default key with stable svc, testSvc with preview svc
	// d. Deployment will have default key with root svc

	// Service structure sample:
	/*
			"services": {
		            "default": [{
		              "name": "app-1-spk-stable-service",
		                "weight": 25,
		              "ports": {
		                "http": 8090
		              }
		            }],
		            "canary": [{
		              "name": "app-1-spk-desired-service",
		              "weight": 75,
		              "ports": {
		                "http": 8090
		              }
		            }]
		          },
	*/
	ep := endpoint.DeepCopy()
	if ep.Labels == nil {
		ep.Labels = make(map[string]string)
	}
	ep.Labels["security.istio.io/tlsMode"] = "istio"
	if clientCluster == serverCluster {
		if strings.HasPrefix(host, canaryPrefix) || strings.HasPrefix(host, previewPrefix) {
			if services[testServiceKey] != nil {
				ep.Address = services[testServiceKey][0].Name + common.Sep + identityConfigEnvironment.Namespace + common.GetLocalDomainSuffix()
				ep.Ports = services[testServiceKey][0].Ports
				endpoints = append(endpoints, ep)
			}
		} else {
			for _, service := range services[defaultServiceKey] {
				tempEp := ep.DeepCopy()
				tempEp.Address = service.Name + common.Sep + identityConfigEnvironment.Namespace + common.GetLocalDomainSuffix()
				tempEp.Ports = service.Ports
				if service.Weight > 0 {
					tempEp.Weight = uint32(service.Weight)
				}
				endpoints = append(endpoints, tempEp)
			}
			for _, service := range services[testServiceKey] {
				if service.Weight > 0 {
					tempEp := ep.DeepCopy()
					tempEp.Address = service.Name + common.Sep + identityConfigEnvironment.Namespace + common.GetLocalDomainSuffix()
					tempEp.Ports = service.Ports
					tempEp.Weight = uint32(service.Weight)
					endpoints = append(endpoints, tempEp)
				}
			}
		}
	} else {
		endpoints = append(endpoints, ep)
	}
	sort.Sort(WorkloadEntrySorted(endpoints))
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
