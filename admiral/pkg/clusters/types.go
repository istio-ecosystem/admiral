package clusters

import (
	"context"
	"regexp"
	"sync"
	"time"

	admiralV1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"

	log "github.com/sirupsen/logrus"
	networking "istio.io/api/networking/v1alpha3"
	k8s "k8s.io/client-go/kubernetes"
)

type ServiceEntrySuspender interface {
	SuspendUpdate(identity string, environment string) bool
}

type IgnoredIdentityCache struct {
	RWLock                 *sync.RWMutex
	Enabled                bool                `json:"enabled"`
	All                    bool                `json:"all"`
	ClusterEnvironment     string              `json:"clusterEnvironment"`
	EnvironmentsByIdentity map[string][]string `json:"environmentsByIdentities"`
}

type RemoteController struct {
	ClusterID                        string
	ApiServer                        string
	StartTime                        time.Time
	GlobalTraffic                    *admiral.GlobalTrafficController
	DeploymentController             *admiral.DeploymentController
	ServiceController                *admiral.ServiceController
	NodeController                   *admiral.NodeController
	ServiceEntryController           *istio.ServiceEntryController
	DestinationRuleController        *istio.DestinationRuleController
	VirtualServiceController         *istio.VirtualServiceController
	SidecarController                *istio.SidecarController
	RolloutController                *admiral.RolloutController
	RoutingPolicyController          *admiral.RoutingPolicyController
	EnvoyFilterController            *admiral.EnvoyFilterController
	OutlierDetectionController       *admiral.OutlierDetectionController
	ClientConnectionConfigController *admiral.ClientConnectionConfigController
	stop                             chan struct{}
	//listener for normal types
}

type AdmiralCache struct {
	CnameClusterCache                   *common.MapOfMaps
	CnameDependentClusterCache          *common.MapOfMaps
	CnameIdentityCache                  *sync.Map
	IdentityClusterCache                *common.MapOfMaps
	ClusterLocalityCache                *common.MapOfMaps
	IdentityDependencyCache             *common.MapOfMaps
	ServiceEntryAddressStore            *ServiceEntryAddressStore
	ConfigMapController                 admiral.ConfigMapControllerInterface //todo this should be in the remotecontrollers map once we expand it to have one configmap per cluster
	GlobalTrafficCache                  GlobalTrafficCache                   //The cache needs to live in the handler because it needs access to deployments
	OutlierDetectionCache               OutlierDetectionCache
	ClientConnectionConfigCache         ClientConnectionConfigCache
	DependencyNamespaceCache            *common.SidecarEgressMap
	SeClusterCache                      *common.MapOfMaps
	RoutingPolicyFilterCache            *routingPolicyFilterCache
	SourceToDestinations                *sourceToDestinations //This cache is to fetch list of all dependencies for a given source identity,
	TrafficConfigIgnoreAssets           []string
	GatewayAssets                       []string
	argoRolloutsEnabled                 bool
	DynamoDbEndpointUpdateCache         *sync.Map
	TrafficConfigWorkingScope           []*regexp.Regexp // regex of assets that are visible to Cartographer
	IdentitiesWithAdditionalEndpoints   *sync.Map
	IdentityClusterNamespaceCache       *common.MapOfMapOfMaps
	CnameDependentClusterNamespaceCache *common.MapOfMapOfMaps
	PartitionIdentityCache              *common.Map
}

type RemoteRegistry struct {
	sync.Mutex
	remoteControllers           map[string]*RemoteController
	SecretController            *secret.Controller
	secretClient                k8s.Interface
	ctx                         context.Context
	AdmiralCache                *AdmiralCache
	StartTime                   time.Time
	ServiceEntrySuspender       ServiceEntrySuspender
	AdmiralDatabaseClient       AdmiralDatabaseManager
	DependencyController        *admiral.DependencyController
	ClientLoader                loader.ClientLoader
	ClusterShardHandler         registry.ClusterShardStore
	ClusterIdentityStoreHandler registry.ClusterIdentityStore
}

// ModifySEFunc is a function that follows the dependency injection pattern which is used by HandleEventForGlobalTrafficPolicy
type ModifySEFunc func(ctx context.Context, event admiral.EventType, env string, sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*networking.ServiceEntry, error)

// TODO - Write a new function to prepare a new Map.

func NewRemoteRegistry(ctx context.Context, params common.AdmiralParams) *RemoteRegistry {
	var serviceEntrySuspender ServiceEntrySuspender
	var admiralDatabaseClient AdmiralDatabaseManager
	var err error

	gtpCache := &globalTrafficCache{}
	gtpCache.identityCache = make(map[string]*admiralV1.GlobalTrafficPolicy)
	gtpCache.mutex = &sync.Mutex{}

	//Initialize OutlierDetection Cache
	odCache := NewOutlierDetectionCache()

	clientConnectionSettingsCache := &clientConnectionSettingsCache{
		identityCache: make(map[string]*admiralV1.ClientConnectionConfig),
		mutex:         &sync.RWMutex{},
	}

	rpFilterCache := &routingPolicyFilterCache{}
	rpFilterCache.filterCache = make(map[string]map[string]map[string]string)
	rpFilterCache.mutex = &sync.Mutex{}
	admiralCache := &AdmiralCache{
		IdentityClusterCache:        common.NewMapOfMaps(),
		CnameClusterCache:           common.NewMapOfMaps(),
		CnameDependentClusterCache:  common.NewMapOfMaps(),
		IdentityDependencyCache:     common.NewMapOfMaps(),
		RoutingPolicyFilterCache:    rpFilterCache,
		DependencyNamespaceCache:    common.NewSidecarEgressMap(),
		CnameIdentityCache:          &sync.Map{},
		ServiceEntryAddressStore:    &ServiceEntryAddressStore{EntryAddresses: map[string]string{}, Addresses: []string{}},
		GlobalTrafficCache:          gtpCache,
		OutlierDetectionCache:       odCache,
		ClientConnectionConfigCache: clientConnectionSettingsCache,
		SeClusterCache:              common.NewMapOfMaps(),
		DynamoDbEndpointUpdateCache: &sync.Map{},
		argoRolloutsEnabled:         params.ArgoRolloutsEnabled,
		SourceToDestinations: &sourceToDestinations{
			sourceDestinations: make(map[string][]string),
			mutex:              &sync.Mutex{},
		},
		IdentitiesWithAdditionalEndpoints:   &sync.Map{},
		IdentityClusterNamespaceCache:       common.NewMapOfMapOfMaps(),
		CnameDependentClusterNamespaceCache: common.NewMapOfMapOfMaps(),
		PartitionIdentityCache:              common.NewMap(),
	}
	if common.GetAdmiralProfile() == common.AdmiralProfileDefault || common.GetAdmiralProfile() == common.AdmiralProfilePerf {
		serviceEntrySuspender = NewDefaultServiceEntrySuspender(params.ExcludedIdentityList)
	} else if common.GetAdmiralProfile() == common.AdmiralProfileIntuit {
		serviceEntrySuspender = NewDynamicServiceEntrySuspender(ctx, params)
	} else {
		serviceEntrySuspender = NewDummyServiceEntrySuspender()
	}

	if common.GetEnableWorkloadDataStorage() {
		admiralDatabaseClient, err = NewAdmiralDatabaseClient(common.GetAdmiralConfigPath(), NewDynamoClient)
		if err != nil {
			alertMsgWhenFailedToConfigureDatabaseClient := "failed to configure admiral database client"
			log.WithField("error", err.Error()).Error(alertMsgWhenFailedToConfigureDatabaseClient)
		}
	} else {
		admiralDatabaseClient = &DummyDatabaseClient{}
	}

	var clientLoader loader.ClientLoader
	if common.GetAdmiralProfile() == common.AdmiralProfilePerf {
		clientLoader = loader.GetFakeClientLoader()
	} else {
		clientLoader = loader.GetKubeClientLoader()
	}

	return &RemoteRegistry{
		ctx:                   ctx,
		StartTime:             time.Now(),
		remoteControllers:     make(map[string]*RemoteController),
		AdmiralCache:          admiralCache,
		ServiceEntrySuspender: serviceEntrySuspender,
		AdmiralDatabaseClient: admiralDatabaseClient,
		ClientLoader:          clientLoader,
	}
}

// NewRemoteRegistryForHAController - creates an instance of RemoteRegistry
// which initializes properties relevant to database builder functionality
func NewRemoteRegistryForHAController(ctx context.Context) *RemoteRegistry {
	return &RemoteRegistry{
		ctx:               ctx,
		StartTime:         time.Now(),
		remoteControllers: make(map[string]*RemoteController),
		ClientLoader:      loader.GetKubeClientLoader(),
		AdmiralCache: &AdmiralCache{
			IdentityClusterCache:    common.NewMapOfMaps(),
			IdentityDependencyCache: common.NewMapOfMaps(),
		},
	}
}

type sourceToDestinations struct {
	sourceDestinations map[string][]string
	mutex              *sync.Mutex
}

func (d *sourceToDestinations) put(dependencyObj *admiralV1.Dependency) {
	if dependencyObj.Spec.Source == "" {
		return
	}
	if dependencyObj.Spec.Destinations == nil {
		return
	}
	if len(dependencyObj.Spec.Destinations) <= 0 {
		return
	}
	d.mutex.Lock()
	d.sourceDestinations[dependencyObj.Spec.Source] = dependencyObj.Spec.Destinations
	d.mutex.Unlock()
}

func (d *sourceToDestinations) Get(key string) []string {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.sourceDestinations[key]
}

func (r *RemoteRegistry) GetRemoteController(clusterId string) *RemoteController {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	return r.remoteControllers[clusterId]
}

func (r *RemoteRegistry) PutRemoteController(clusterId string, rc *RemoteController) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	r.remoteControllers[clusterId] = rc
}

func (r *RemoteRegistry) DeleteRemoteController(clusterId string) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	delete(r.remoteControllers, clusterId)
}

func (r *RemoteRegistry) RangeRemoteControllers(fn func(k string, v *RemoteController)) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	for k, v := range r.remoteControllers {
		fn(k, v)
	}
}

func (r *RemoteRegistry) GetClusterIds() []string {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	var clusters = make([]string, 0, len(r.remoteControllers))
	for k := range r.remoteControllers {
		clusters = append(clusters, k)
	}
	return clusters
}

func (r *RemoteRegistry) shutdown() {

	done := r.ctx.Done()
	//wait for the context to close
	<-done

	//close the remote controllers stop channel
	for _, v := range r.remoteControllers {
		close(v.stop)
	}
}

type ServiceEntryAddressStore struct {
	EntryAddresses map[string]string `yaml:"entry-addresses,omitempty"`
	Addresses      []string          `yaml:"addresses,omitempty"` //trading space for efficiency - this will give a quick way to validate that the address is unique
}

type RouteConfig struct {
	ServicesConfig []*ServiceRouteConfig `json:"servicesRouteConfig"`
}

type ServiceRouteConfig struct {
	WorkloadEnvRevision map[string]string `json:"workloadEnvRevision,omitempty"`
	ServiceAssetAlias   string            `json:"serviceAssetAlias,omitempty"`
	Routes              []*Route          `json:"routes,omitempty"`
}

type Route struct {
	Name                 string   `json:"name"`
	Inbound              string   `json:"inbound"`
	Outbound             string   `json:"outbound"`
	WorkloadEnvSelectors []string `json:"workloadEnvSelectors"`
	OutboundEndpoints    []string
}

type NodeHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}
