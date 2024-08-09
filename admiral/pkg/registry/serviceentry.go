package registry

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"github.com/sirupsen/logrus"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
)

// IstioSEBuilder is an interface to construct Service Entry objects
// from IdentityConfig objects. It can construct multiple Service Entries
// from an IdentityConfig or construct just one given a IdentityConfigEnvironment.
type IstioSEBuilder interface {
	BuildServiceEntriesFromIdentityConfig(ctxLogger *logrus.Entry, ctx context.Context, event admiral.EventType, identityConfig IdentityConfig) ([]*networkingV1Alpha3.ServiceEntry, error)
}

type ServiceEntryBuilder struct {
	OperatorCluster string
}

// BuildServiceEntriesFromIdentityConfig builds service entries to write to the operator cluster
// by looping through the IdentityConfig clusters and environments to get spec information. It
// builds one SE per environment per cluster the identity is deployed in.
func (b *ServiceEntryBuilder) BuildServiceEntriesFromIdentityConfig(ctxLogger *logrus.Entry, ctx context.Context, event admiral.EventType, identityConfig IdentityConfig) ([]*networkingV1Alpha3.ServiceEntry, error) {
	identity := identityConfig.Assetname
	serviceEntries := []*networkingV1Alpha3.ServiceEntry{}
	var err error
	if event == admiral.Add || event == admiral.Update {
		ctxLogger.Infof(common.CtxLogFormat, "buildServiceEntry", identity, common.GetSyncNamespace(), b.OperatorCluster, "Beginning to build the SE spec")
		ingressEndpoints, ingressErr := getIngressEndpoints(identityConfig.Clusters)
		if ingressErr != nil {
			err = ingressErr
			return serviceEntries, err
		}
		for i, identityConfigCluster := range identityConfig.Clusters {
			sourceCluster := identityConfigCluster.Name
			for _, identityConfigEnvironment := range identityConfigCluster.Environment {
				se, buildErr := buildServiceEntryForClusterByEnv(ctxLogger, ctx, b.OperatorCluster, sourceCluster, identity, identityConfigCluster.ClientAssets, ingressEndpoints, ingressEndpoints[i].Address, identityConfigEnvironment)
				if buildErr != nil {
					err = buildErr
				}
				serviceEntries = append(serviceEntries, se)
			}
		}
		return serviceEntries, err
	}
	return serviceEntries, err
}

// buildServiceEntryForClusterByEnv builds a service entry based on cluster and IdentityConfigEnvironment information
// to be written to the operator cluster.
func buildServiceEntryForClusterByEnv(ctxLogger *logrus.Entry, ctx context.Context, operatorCluster string, sourceCluster string, identity string, clientAssets []map[string]string, ingressEndpoints []*networkingV1Alpha3.WorkloadEntry, remoteEndpointAddress string, identityConfigEnvironment IdentityConfigEnvironment) (*networkingV1Alpha3.ServiceEntry, error) {
	ctxLogger.Infof(common.CtxLogFormat, "buildServiceEntry", identity, common.GetSyncNamespace(), operatorCluster, "build the SE spec from IdentityConfigEnvironment")
	env := identityConfigEnvironment.Name
	fqdn := common.GetCnameVal([]string{env, strings.ToLower(identity), common.GetHostnameSuffix()})
	san := common.SpiffePrefix + common.GetSANPrefix() + common.Slash + identity
	ports, err := getServiceEntryPorts(identityConfigEnvironment)
	if err != nil {
		return nil, err
	}
	endpoints, err := getServiceEntryEndpoints(ctxLogger, operatorCluster, sourceCluster, ingressEndpoints, remoteEndpointAddress, identityConfigEnvironment)
	if err != nil {
		return nil, err
	}
	dependentNamespaces, err := getSortedDependentNamespaces(ctxLogger, ctx, operatorCluster, sourceCluster, fqdn, env, clientAssets)
	if err != nil {
		return nil, err
	}
	return &networkingV1Alpha3.ServiceEntry{
		Hosts:           []string{fqdn},
		Ports:           ports,
		Location:        networkingV1Alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      networkingV1Alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{san},
		Endpoints:       endpoints,
		ExportTo:        dependentNamespaces,
	}, err
}

// getIngressEndpoint constructs the endpoint of the ingress gateway/remote endpoint for an identity
// by reading the information directly from the IdentityConfigCluster.
func getIngressEndpoints(clusters []IdentityConfigCluster) ([]*networkingV1Alpha3.WorkloadEntry, error) {
	ingressEndpoints := []*networkingV1Alpha3.WorkloadEntry{}
	var err error
	for _, cluster := range clusters {
		portNumber, parseErr := strconv.ParseInt(cluster.IngressPort, 10, 64)
		if parseErr != nil {
			err = parseErr
			continue
		}
		ingressEndpoint := &networkingV1Alpha3.WorkloadEntry{
			Address:  cluster.IngressEndpoint,
			Locality: cluster.Locality,
			Ports:    map[string]uint32{cluster.IngressPortName: uint32(portNumber)},
			Labels:   map[string]string{"security.istio.io/tlsMode": "istio"},
		}
		ingressEndpoints = append(ingressEndpoints, ingressEndpoint)
	}
	return ingressEndpoints, err
}

// getServiceEntryPorts constructs the ServicePorts of the service entry that should be built
// for the given identityConfigEnvironment.
func getServiceEntryPorts(identityConfigEnvironment IdentityConfigEnvironment) ([]*networkingV1Alpha3.ServicePort, error) {
	//TODO: Verify this is how ports should be set
	//Find Port with targetPort that matches inbound common.SidecarEnabledPorts
	//Set port name and protocol based on that
	port := &networkingV1Alpha3.ServicePort{Number: uint32(common.DefaultServiceEntryPort), Name: util.Http, Protocol: util.Http}
	var err error
	if len(identityConfigEnvironment.Ports) == 0 {
		err = errors.New("identityConfigEnvironment had no ports for: " + identityConfigEnvironment.Name)
	}
	for _, servicePort := range identityConfigEnvironment.Ports {
		//TODO: 8090 is supposed to be set as the common.SidecarEnabledPorts (includeInboundPorts), and we check that in the rollout, but we don't have that information here
		if servicePort.TargetPort.IntValue() == 8090 {
			protocol := util.GetPortProtocol(servicePort.Name)
			port.Name = protocol
			port.Protocol = protocol
		}
	}
	ports := []*networkingV1Alpha3.ServicePort{port}
	return ports, err
}

// getServiceEntryEndpoints constructs the remote or local endpoint of the service entry that
// should be built for the given identityConfigEnvironment.
func getServiceEntryEndpoints(ctxLogger *logrus.Entry, operatorCluster string, sourceCluster string, ingressEndpoints []*networkingV1Alpha3.WorkloadEntry, remoteEndpointAddress string, identityConfigEnvironment IdentityConfigEnvironment) ([]*networkingV1Alpha3.WorkloadEntry, error) {
	//TODO: Verify Local and Remote Endpoints are constructed correctly
	endpoints := []*networkingV1Alpha3.WorkloadEntry{}
	var err error
	for _, endpoint := range ingressEndpoints {
		tmpEp := endpoint.DeepCopy()
		tmpEp.Labels["type"] = identityConfigEnvironment.Type
		if operatorCluster == sourceCluster && tmpEp.Address == remoteEndpointAddress {
			//Local Endpoint Address if the identity is deployed on the same cluster as it's client and the endpoint is the remote endpoint for the cluster
			tmpEp.Address = identityConfigEnvironment.ServiceName + common.Sep + identityConfigEnvironment.Namespace + common.GetLocalDomainSuffix()
			for _, servicePort := range identityConfigEnvironment.Ports {
				//There should only be one mesh port here (http-service-mesh), but we are preserving ability to have multiple ports
				protocol := util.GetPortProtocol(servicePort.Name)
				if _, ok := tmpEp.Ports[protocol]; ok {
					tmpEp.Ports[protocol] = uint32(servicePort.Port)
					ctxLogger.Infof(common.CtxLogFormat, "LocalMeshPort", servicePort.Port, "", sourceCluster, "Protocol: "+protocol)
				} else {
					err = errors.New("failed to get Port for protocol: " + protocol)
				}
			}
		}
		endpoints = append(endpoints, tmpEp)
	}
	return endpoints, err
}

// getSortedDependentNamespaces constructs a sorted list of unique namespaces for a given cluster, client assets,
// and cname, where each namespace is where a client asset of the cname is deployed on the cluster. If the cname
// is also deployed on the cluster then the istio-system namespace is also in the list.
func getSortedDependentNamespaces(ctxLogger *logrus.Entry, ctx context.Context, operatorCluster string, sourceCluster string, cname string, env string, clientAssets []map[string]string) ([]string, error) {
	clientNamespaces := []string{}
	var err error
	var clientIdentityConfig IdentityConfig
	for _, clientAsset := range clientAssets {
		//TODO: Need to do registry client initialization better, maybe pass it in
		registryClient := NewRegistryClient(WithRegistryEndpoint("endpoint"), WithOperatorCluster(operatorCluster))
		// For each client asset of cname, we fetch its identityConfig
		clientIdentityConfig, err = registryClient.GetByIdentityName(clientAsset["name"], ctx)
		if err != nil {
			ctxLogger.Infof(common.CtxLogFormat, "buildServiceEntry", cname, common.GetSyncNamespace(), clientAsset["name"], "Failed to fetch IdentityConfig: "+err.Error())
			continue
		}
		for _, clientIdentityConfigCluster := range clientIdentityConfig.Clusters {
			// For each cluster the client asset is deployed on, we check if that cluster is the operator cluster we are writing to
			if operatorCluster == clientIdentityConfigCluster.Name {
				for _, clientIdentityConfigEnvironment := range clientIdentityConfigCluster.Environment {
					// For each environment of the client asset on the operator cluster, we add the namespace to our list
					if clientIdentityConfigEnvironment.Name == env {
						//Do we need to check if ENV matches here for exportTo?
						clientNamespaces = append(clientNamespaces, clientIdentityConfigEnvironment.Namespace)
					}
				}
			}
		}
	}
	if operatorCluster == sourceCluster {
		clientNamespaces = append(clientNamespaces, common.NamespaceIstioSystem)
	}
	if len(clientNamespaces) > common.GetExportToMaxNamespaces() {
		clientNamespaces = []string{"*"}
		ctxLogger.Infof("exceeded max namespaces for cname=%s in cluster=%s", cname, operatorCluster)
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
