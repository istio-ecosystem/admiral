package clusters

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"sync"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"

	k8s "k8s.io/client-go/kubernetes"
)

const (
	LogFormat    = "op=%s type=%v name=%v cluster=%s message=%s"
	LogErrFormat = "op=%s type=%v name=%v cluster=%s, e=%v"
)

type AdmiralParams struct {
	KubeconfigPath             string
	CacheRefreshDuration       time.Duration
	ClusterRegistriesNamespace string
	DependenciesNamespace      string
	SyncNamespace              string
	EnableSAN                  bool
	SANPrefix                  string
	SecretResolver             string
	LabelSet                   *common.LabelSet
	HostnameSuffix             string
	WorkloadIdentityLabel      string
}

func (b AdmiralParams) String() string {
	return fmt.Sprintf("KubeconfigPath=%v ", b.KubeconfigPath) +
		fmt.Sprintf("CacheRefreshDuration=%v ", b.CacheRefreshDuration) +
		fmt.Sprintf("ClusterRegistriesNamespace=%v ", b.ClusterRegistriesNamespace) +
		fmt.Sprintf("DependenciesNamespace=%v ", b.DependenciesNamespace) +
		fmt.Sprintf("EnableSAN=%v ", b.EnableSAN) +
		fmt.Sprintf("SANPrefix=%v ", b.SANPrefix) +
		fmt.Sprintf("SecretResolver=%v ", b.SecretResolver)
}

type RemoteController struct {
	ClusterID            string
	GlobalTraffic        *admiral.GlobalTrafficController
	DeploymentController *admiral.DeploymentController
	ServiceController    *admiral.ServiceController
	PodController        *admiral.PodController
	NodeController       *admiral.NodeController
	ServiceEntryController *istio.ServiceEntryController
	DestinationRuleController * istio.DestinationRuleController
	VirtualServiceController * istio.VirtualServiceController
	stop                 chan struct{}
	//listener for normal types
}

type AdmiralCache struct {
	CnameClusterCache               *common.MapOfMaps
	CnameDependentClusterCache      *common.MapOfMaps
	CnameIdentityCache              *sync.Map
	IdentityClusterCache            *common.MapOfMaps
	ClusterLocalityCache            *common.MapOfMaps
	IdentityDependencyCache         *common.MapOfMaps
	SubsetServiceEntryIdentityCache *sync.Map
	ServiceEntryAddressStore        *common.ServiceEntryAddressStore
	ConfigMapController             admiral.ConfigMapControllerInterface //todo this should be in the remotecontrollers map once we expand it to have one configmap per cluster
}

//This will handle call backs from the secret controller to provision
//controllers watching config in remote clusters
type RemoteRegistry struct {
	config AdmiralParams
	sync.Mutex
	remoteControllers map[string]*RemoteController
	secretClient      k8s.Interface
	ctx               context.Context
	AdmiralCache      *AdmiralCache
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

func InitAdmiral(ctx context.Context, params AdmiralParams) (*RemoteRegistry, error) {

	logrus.Infof("Initializing Admiral with params: %v", params)

	w := RemoteRegistry{
		ctx: ctx,
	}

	wd := DependencyHandler{
		RemoteRegistry: &w,
	}

	var err error
	wd.DepController, err = admiral.NewDependencyController(ctx.Done(), &wd, params.KubeconfigPath, params.DependenciesNamespace, params.CacheRefreshDuration)
	if err != nil {
		return nil, fmt.Errorf(" Error with dependency controller init: %v", err)
	}

	w.config = params
	w.remoteControllers = make(map[string]*RemoteController)

	w.AdmiralCache = &AdmiralCache{
		IdentityClusterCache:            common.NewMapOfMaps(),
		CnameClusterCache:               common.NewMapOfMaps(),
		CnameDependentClusterCache:      common.NewMapOfMaps(),
		ClusterLocalityCache:            common.NewMapOfMaps(),
		IdentityDependencyCache:         common.NewMapOfMaps(),
		CnameIdentityCache:              &sync.Map{},
		SubsetServiceEntryIdentityCache: &sync.Map{},
		ServiceEntryAddressStore:        &common.ServiceEntryAddressStore{EntryAddresses: map[string]string{}, Addresses: []string{}}}

	configMapController, err := admiral.NewConfigMapController(w.config.KubeconfigPath, w.config.SyncNamespace)
	if err != nil {
		return nil, fmt.Errorf(" Error with configmap controller init: %v", err)
	}
	w.AdmiralCache.ConfigMapController = configMapController
	loadServiceEntryCacheData(w.AdmiralCache.ConfigMapController, w.AdmiralCache)

	err = createSecretController(ctx, &w, params)
	if err != nil {
		return nil, fmt.Errorf(" Error with secret control init: %v", err)
	}

	go w.shutdown()

	return &w, nil
}

func createSecretController(ctx context.Context, w *RemoteRegistry, params AdmiralParams) error {
	var err error

	w.secretClient, err = admiral.K8sClientFromPath(params.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("could not create K8s client: %v", err)
	}

	err = secret.StartSecretController(w.secretClient,
		w.createCacheController,
		w.deleteCacheController,
		w.config.ClusterRegistriesNamespace,
		ctx, params.SecretResolver)

	if err != nil {
		return fmt.Errorf("could not start secret controller: %v", err)
	}

	return nil
}

func (r *RemoteRegistry) createCacheController(clientConfig *rest.Config, clusterID string, resyncPeriod time.Duration) error {

	stop := make(chan struct{})

	rc := RemoteController{
		stop:      stop,
		ClusterID: clusterID,
	}

	var err error

	logrus.Infof("starting global traffic policy controller custerID: %v", clusterID)
	rc.GlobalTraffic, err = admiral.NewGlobalTrafficController(stop, &GlobalTrafficHandler{RemoteRegistry: r}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with GlobalTrafficController controller init: %v", err)
	}

	logrus.Infof("starting deployment controller custerID: %v", clusterID)
	rc.DeploymentController, err = admiral.NewDeploymentController(stop, &DeploymentHandler{RemoteRegistry: r}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with DeploymentController controller init: %v", err)
	}

	logrus.Infof("starting pod controller custerID: %v", clusterID)
	rc.PodController, err = admiral.NewPodController(stop, &PodHandler{RemoteRegistry: r}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with PodController controller init: %v", err)
	}

	logrus.Infof("starting node controller custerID: %v", clusterID)
	rc.NodeController, err = admiral.NewNodeController(stop, &NodeHandler{RemoteRegistry: r}, clientConfig)

	if err != nil {
		return fmt.Errorf(" Error with NodeController controller init: %v", err)
	}

	logrus.Infof("starting service controller custerID: %v", clusterID)
	rc.ServiceController, err = admiral.NewServiceController(stop, &ServiceHandler{RemoteRegistry: r}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with ServiceController controller init: %v", err)
	}

	logrus.Infof("starting service entry controller for custerID: %v", clusterID)
	rc.ServiceEntryController, err = istio.NewServiceEntryController(stop, &ServiceEntryHandler{RemoteRegistry: r, ClusterID:clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with ServiceEntryController init: %v", err)
	}

	logrus.Infof("starting destination rule controller for custerID: %v", clusterID)
	rc.DestinationRuleController, err = istio.NewDestinationRuleController(stop, &DestinationRuleHandler{RemoteRegistry: r, ClusterID:clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with DestinationRuleController init: %v", err)
	}

	logrus.Infof("starting virtual service controller for custerID: %v", clusterID)
	rc.VirtualServiceController, err = istio.NewVirtualServiceController(stop, &VirtualServiceHandler{RemoteRegistry: r, ClusterID:clusterID}, clientConfig, resyncPeriod)

	if err != nil {
		return fmt.Errorf(" Error with VirtualServiceController init: %v", err)
	}

	r.Lock()
	defer r.Unlock()
	r.remoteControllers[clusterID] = &rc

	logrus.Infof("Create Controller %s", clusterID)

	return nil
}

func (r *RemoteRegistry) deleteCacheController(clusterID string) error {

	controller, ok := r.remoteControllers[clusterID]

	if ok {
		close(controller.stop)
	}

	r.Lock()
	defer r.Unlock()
	delete(r.remoteControllers, clusterID)

	logrus.Infof(LogFormat, "Delete", "remote-controller", clusterID, clusterID, "success")
	return nil
}

func loadServiceEntryCacheData(c admiral.ConfigMapControllerInterface, admiralCache *AdmiralCache) {
	configmap, err := c.GetConfigMap()
	if err != nil {
		log.Warnf("Failed to refresh configmap state Error: %v", err)
		return //No need to invalidate the cache
	}

	entryCache := admiral.GetServiceEntryStateFromConfigmap(configmap)

	if entryCache != nil {
		*admiralCache.ServiceEntryAddressStore = *entryCache
		log.Infof("Successfully updated service entry cache state")
	}

}

//Gets a guarenteed unique local address for a serviceentry. Returns the address, True iff the configmap was updated false otherwise, and an error if any
//Any error coupled with an empty string address means the method should be retried
func GetLocalAddressForSe(seName string, seAddressCache *common.ServiceEntryAddressStore, configMapController admiral.ConfigMapControllerInterface) (string, bool, error) {
	var address = seAddressCache.EntryAddresses[seName]
	if len(address) == 0 {
		address, err := GenerateNewAddressAndAddToConfigMap(seName, configMapController)
		return address, true, err
	}
	return address, false, nil
}

//an atomic fetch and update operation against the configmap (using K8s build in optimistic consistency mechanism via resource version)
func GenerateNewAddressAndAddToConfigMap(seName string, configMapController admiral.ConfigMapControllerInterface) (string, error) {
	//1. get cm, see if there. 2. gen new uq address. 3. put configmap. RETURN SUCCESSFULLY IFF CONFIGMAP PUT SUCCEEDS
	cm, err := configMapController.GetConfigMap()
	if err != nil {
		return "", err
	}

	newAddressState := admiral.GetServiceEntryStateFromConfigmap(cm)

	if newAddressState == nil {
		return "", errors.New("could not unmarshall configmap yaml")
	}

	if val, ok := newAddressState.EntryAddresses[seName]; ok { //Someone else updated the address state, so we'll use that
		return val, nil
	}

	secondIndex := (len(newAddressState.Addresses) / 255) + 10
	firstIndex := (len(newAddressState.Addresses) % 255) + 1
	address := common.LocalAddressPrefix + common.Sep + strconv.Itoa(secondIndex) + common.Sep + strconv.Itoa(firstIndex)

	for util.Contains(newAddressState.Addresses, address) {
		if firstIndex < 255 {
			firstIndex++
		} else {
			secondIndex++
			firstIndex = 0
		}
		address = common.LocalAddressPrefix + common.Sep + strconv.Itoa(secondIndex) + common.Sep + strconv.Itoa(firstIndex)
	}
	newAddressState.Addresses = append(newAddressState.Addresses, address)
	newAddressState.EntryAddresses[seName] = address

	err = putServiceEntryStateFromConfigmap(configMapController, cm, newAddressState)

	if err != nil {
		return "", err
	}
	return address, nil
}

//puts new data into an existing configmap. Providing the original is necessary to prevent fetch and update race conditions
func putServiceEntryStateFromConfigmap(c admiral.ConfigMapControllerInterface, originalConfigmap *k8sV1.ConfigMap, data *common.ServiceEntryAddressStore) error {
	if originalConfigmap == nil {
		return errors.New("configmap must not be nil")
	}

	bytes, err := yaml.Marshal(data)

	if err != nil {
		logrus.Errorf("Failed to put service entry state into the configmap. %v", err)
		return nil
	}

	if originalConfigmap.Data == nil {
		originalConfigmap.Data = map[string]string{}
	}

	originalConfigmap.Data["serviceEntryAddressStore"] = string(bytes)

	return c.PutConfigMap(originalConfigmap)
}
