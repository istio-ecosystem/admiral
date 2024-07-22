package clusters

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"gopkg.in/yaml.v2"

	"go.opentelemetry.io/otel/attribute"
	api "go.opentelemetry.io/otel/metric"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	model "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/sirupsen/logrus"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SeDrTuple struct {
	SeName                      string
	DrName                      string
	ServiceEntry                *networking.ServiceEntry
	DestinationRule             *networking.DestinationRule
	SeDnsPrefix                 string
	SeDrGlobalTrafficPolicyName string
}

const (
	intuitHostSuffix                             = "intuit"
	resourceCreatedByAnnotationLabel             = "app.kubernetes.io/created-by"
	resourceCreatedByAnnotationValue             = "admiral"
	resourceCreatedByAnnotationCartographerValue = "cartographer"
	dnsPrefixAnnotationLabel                     = "dns-prefix"
	serviceEntryAssociatedGtpAnnotationLabel     = "associated-gtp"
	gtpManagedByGithub                           = "github"
	gtpManagedByMeshAgent                        = "mesh-agent"
	gtpManagerMeshAgentFieldValue                = "ewok-mesh-agent"
)

func createServiceEntryForDeployment(ctxLogger *logrus.Entry, ctx context.Context, event admiral.EventType, rc *RemoteController, admiralCache *AdmiralCache,
	meshPorts map[string]uint32, destDeployment *k8sAppsV1.Deployment, serviceEntries map[string]*networking.ServiceEntry) (*networking.ServiceEntry, error) {
	defer util.LogElapsedTimeForModifySE(ctxLogger, "createServiceEntryForDeployment", "", "", "", "")()
	workloadIdentityKey := common.GetWorkloadIdentifier()
	globalFqdn := common.GetCname(destDeployment, workloadIdentityKey, common.GetHostnameSuffix())

	//Handling retries for getting/putting service entries from/in cache
	start := time.Now()
	address, err := getUniqueAddress(ctxLogger, ctx, admiralCache, globalFqdn)
	if err != nil {
		return nil, err
	}
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "GetUniqueAddress",
		"", "", rc.ClusterID, "", start)

	if !common.DisableIPGeneration() && len(address) == 0 {
		ctxLogger.Errorf(common.CtxLogFormat, "createServiceEntryForDeployment", destDeployment.Name, destDeployment.Namespace, "", "Failed because address is empty while DisableIPGeneration is disabled")
		return nil, nil
	}
	if len(globalFqdn) == 0 {
		ctxLogger.Errorf(common.CtxLogFormat, "createServiceEntryForDeployment", destDeployment.Name, destDeployment.Namespace, "", "Failed because fqdn is empty")
		return nil, nil
	}

	san := getSanForDeployment(destDeployment, workloadIdentityKey)
	return generateServiceEntry(ctxLogger, event, admiralCache, meshPorts, globalFqdn, rc, serviceEntries, address, san, common.Deployment), nil
}

// modifyServiceEntryForNewServiceOrPod creates/updates
// multiple resources for the passed identity and environment
func modifyServiceEntryForNewServiceOrPod(
	ctx context.Context, event admiral.EventType, env string,
	sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*networking.ServiceEntry, error) {
	ctxLogger := common.GetCtxLogger(ctx, sourceIdentity, env)
	ctxLogger.Infof(common.CtxLogFormat, "event", "", "", "", "received")
	defer util.LogElapsedTimeForModifySE(ctxLogger, "event", "", "", "", "TotalModifySETime")()
	var modifySEerr error
	var isServiceEntryModifyCalledForSourceCluster bool
	totalConfigWriterEvents.Increment(api.WithAttributes(
		attribute.Key("identity").String(sourceIdentity),
		attribute.Key("environment").String(env),
	))
	// Assigns sourceIdentity, which could have the partition prefix or might not, to the partitionedIdentity
	// Then, gets the non-partitioned identity and assigns it to sourceIdentity. sourceIdentity will always have the original/non-partitioned identity
	partitionedIdentity := sourceIdentity
	sourceIdentity = getNonPartitionedIdentity(remoteRegistry.AdmiralCache, sourceIdentity)
	if remoteRegistry.ServiceEntrySuspender.SuspendUpdate(sourceIdentity, env) {
		ctxLogger.Infof(common.CtxLogFormat, event, "", "", sourceIdentity, env, "",
			"processing skipped as service entry update is suspended for identity")
		return nil, fmt.Errorf("processing skipped as service entry update is suspended for identity %s in environment %s", sourceIdentity, env)
	}
	if commonUtil.IsAdmiralReadOnly() {
		ctxLogger.Infof(common.CtxLogFormat, event, "", "", "", "processing skipped as Admiral is in Read-only mode")
		return nil, nil
	}

	// Should not return early here for TrafficConfig persona, as cache should build up during warm up time
	if IsCacheWarmupTime(remoteRegistry) && !common.IsPersonaTrafficConfig() {
		ctxLogger.Infof(common.CtxLogFormat, event, "", "", "", "processing skipped during cache warm up state")
		return nil, fmt.Errorf(common.CtxLogFormat, event, env, sourceIdentity, "", "processing skipped during cache warm up state for env="+env+" identity="+sourceIdentity)
	}
	ctxLogger.Infof(common.CtxLogFormat, event, "", "", "", "processing")
	var (
		cname                                 string
		namespace                             string
		deploymentOrRolloutName               string
		deploymentOrRolloutNS                 string
		serviceInstance                       *k8sV1.Service
		rollout                               *argo.Rollout
		deployment                            *k8sAppsV1.Deployment
		start                                 = time.Now()
		identityKey                           = common.ConstructKeyWithEnvAndIdentity(env, sourceIdentity)
		gtpIdentityKey                        = common.ConstructKeyWithEnvAndIdentity(env, partitionedIdentity)
		clusters                              = remoteRegistry.GetClusterIds()
		outlierDetections                     = make(map[string][]*v1.OutlierDetection)
		clientConnectionSettings              = make(map[string][]*v1.ClientConnectionConfig)
		gtps                                  = make(map[string][]*v1.GlobalTrafficPolicy)
		weightedServices                      = make(map[string]*WeightedService)
		cnames                                = make(map[string]string)
		sourceServices                        = make(map[string]map[string]*k8sV1.Service)
		sourceWeightedServices                = make(map[string]map[string]*WeightedService)
		sourceDeployments                     = make(map[string]*k8sAppsV1.Deployment)
		sourceRollouts                        = make(map[string]*argo.Rollout)
		appType                               = make(map[string]string)
		serviceEntries                        = make(map[string]*networking.ServiceEntry)
		clustersToDeleteSE                    = make(map[string]bool)
		clusterAppDeleteMap                   = make(map[string]string)
		clusterDeployRolloutPresent           = make(map[string]map[string]bool)
		sourceClusters                        []string
		isAdditionalEndpointGenerationEnabled bool
		deployRolloutMigration                = make(map[string]bool)
	)

	clusterName, ok := ctx.Value(common.ClusterName).(string)
	if !ok {
		ctxLogger.Errorf(AlertLogMsg, ctx.Value(common.ClusterName))
		return nil, nil
	}

	eventResourceType, ok := ctx.Value(common.EventResourceType).(string)
	if !ok {
		ctxLogger.Errorf(AlertLogMsg, ctx.Value(common.EventResourceType))
		return nil, nil
	}

	var createResourcesOnlyInDependentOverrideClusters bool
	dependentClusterOverride, ok := ctx.Value(common.DependentClusterOverride).(*common.Map)
	if !ok {
		ctxLogger.Warnf(common.CtxLogFormat, "event", "", "", "", "dependent cluster override not passed")
	} else {
		if dependentClusterOverride != nil && len(dependentClusterOverride.GetKeys()) > 0 {
			ctxLogger.Infof(common.CtxLogFormat, "modifyServiceEntryForNewServiceOrPod", "", "", "", "dependent cluster override passed")
			createResourcesOnlyInDependentOverrideClusters = true
		}
	}

	// build service entry spec
	for _, clusterId := range clusters {
		rc := remoteRegistry.GetRemoteController(clusterId)
		if rc == nil {
			ctxLogger.Warnf(common.CtxLogFormat, "Event", "", "", clusterId, "remote controller not available/initialized for the cluster")
			continue
		}

		if rc.DeploymentController != nil {
			deployment = rc.DeploymentController.Cache.Get(partitionedIdentity, env)
		}

		if rc.RolloutController != nil {
			rollout = rc.RolloutController.Cache.Get(partitionedIdentity, env)
		}

		if deployment == nil && rollout == nil {
			ctxLogger.Infof(common.CtxLogFormat, "event", "", "", clusterId, "neither deployment nor rollouts found")
			continue
		}

		// For Deployment <-> Rollout migration
		// Check the type of the application and set the required variables.
		// It can be a deployment, rollout or both (during migration).
		if deployment != nil {
			appType[rc.ClusterID] = common.Deployment
			if rollout != nil {
				if event == admiral.Delete {
					clusterAppDeleteMap[clusterName] = eventResourceType
				}
				deployRolloutMigration[rc.ClusterID] = true
			}
		} else if rollout != nil {
			appType[rc.ClusterID] = common.Rollout
		}

		// For Deployment <-> Rollout migration
		// sourceServices will also include the services for
		// both deployment and rollout for a particular cluster
		if _, ok := sourceServices[rc.ClusterID]; !ok {
			sourceServices[rc.ClusterID] = make(map[string]*k8sV1.Service)
		}

		if _, ok := clusterDeployRolloutPresent[rc.ClusterID]; !ok {
			clusterDeployRolloutPresent[rc.ClusterID] = make(map[string]bool)
		}

		remoteRegistry.AdmiralCache.IdentityClusterCache.Put(partitionedIdentity, rc.ClusterID, rc.ClusterID)
		util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheIdentityClusterCachePut",
			deploymentOrRolloutName, deploymentOrRolloutNS, rc.ClusterID, "", start)

		if deployment != nil {
			if eventResourceType == common.Deployment {
				deploymentOrRolloutName = deployment.Name
				deploymentOrRolloutNS = deployment.Namespace
			}
			ctxLogger.Infof(common.CtxLogFormat, "BuildServiceEntry", deploymentOrRolloutName, deploymentOrRolloutNS, clusterId, "building service entry for deployment")
			ctxLogger.Infof(common.CtxLogFormat, "AdmiralCacheIdentityClusterCachePut", deploymentOrRolloutName,
				deploymentOrRolloutNS, rc.ClusterID, "updating identity<->cluster mapping")
			clusterDeployRolloutPresent[rc.ClusterID][common.Deployment] = true
			var err error
			serviceInstance, err = getServiceForDeployment(rc, deployment)
			if err != nil {
				ctxLogger.Warnf(common.CtxLogFormat, "GetServiceForDeployment", deploymentOrRolloutName, deploymentOrRolloutNS, clusterId, err)
				continue
			}
			sourceServices[rc.ClusterID][common.Deployment] = serviceInstance

			namespace = deployment.Namespace
			localMeshPorts := GetMeshPortsForDeployments(rc.ClusterID, serviceInstance, deployment)
			cname = common.GetCname(deployment, common.GetWorkloadIdentifier(), common.GetHostnameSuffix())
			sourceDeployments[rc.ClusterID] = deployment
			sourceClusters = append(sourceClusters, clusterId)

			if common.IsPersonaTrafficConfig() {
				continue
			}

			// Decide if we want to add, update or delete the SE endpoints for the current cluster being processed.
			eventType, deleteCluster := removeSeEndpoints(clusterName, event, clusterId, deployRolloutMigration[rc.ClusterID], common.Deployment, clusterAppDeleteMap)
			clustersToDeleteSE[clusterId] = deleteCluster

			start = time.Now()
			_, errCreateSE := createServiceEntryForDeployment(ctxLogger, ctx, eventType, rc, remoteRegistry.AdmiralCache, localMeshPorts, deployment, serviceEntries)
			ctxLogger.Infof(common.CtxLogFormat, "BuildServiceEntry",
				deploymentOrRolloutName, deploymentOrRolloutNS, clusterId, "total service entries built="+strconv.Itoa(len(serviceEntries)))
			util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheCreateServiceEntryForDeployment",
				deploymentOrRolloutName, deploymentOrRolloutNS, rc.ClusterID, "", start)
			modifySEerr = common.AppendError(modifySEerr, errCreateSE)
		}

		if rollout != nil {
			if eventResourceType == common.Rollout {
				deploymentOrRolloutName = rollout.Name
				deploymentOrRolloutNS = rollout.Namespace
			}
			ctxLogger.Infof(common.CtxLogFormat, "BuildServiceEntry", deploymentOrRolloutName, deploymentOrRolloutNS, clusterId, "building service entry for rollout")
			ctxLogger.Infof(common.CtxLogFormat, "CacheUpdate", deploymentOrRolloutName,
				deploymentOrRolloutNS, rc.ClusterID, "updating identity<->cluster mapping")
			clusterDeployRolloutPresent[rc.ClusterID][common.Rollout] = true
			cname = common.GetCnameForRollout(rollout, common.GetWorkloadIdentifier(), common.GetHostnameSuffix())
			cnames[cname] = "1"
			sourceRollouts[rc.ClusterID] = rollout
			sourceClusters = append(sourceClusters, clusterId)
			namespace = rollout.Namespace
			if common.IsPersonaTrafficConfig() {
				continue
			}
			weightedServices = getServiceForRollout(ctx, rc, rollout)
			if len(weightedServices) == 0 {
				ctxLogger.Warnf(common.CtxLogFormat, "GetServiceForRollout", deploymentOrRolloutName, deploymentOrRolloutNS, rc.ClusterID, "No matching service instance found")
				continue
			}

			//use any service within the weightedServices for determining ports etc.
			for _, sInstance := range weightedServices {
				serviceInstance = sInstance.Service
				break
			}
			sourceServices[rc.ClusterID][common.Rollout] = serviceInstance

			localMeshPorts := GetMeshPortsForRollout(rc.ClusterID, serviceInstance, rollout)

			// Decide if we want to add, update or delete the SE endpoints for the current cluster being processed.
			eventType, deleteCluster := removeSeEndpoints(clusterName, event, clusterId, deployRolloutMigration[rc.ClusterID], common.Rollout, clusterAppDeleteMap)
			clustersToDeleteSE[clusterId] = deleteCluster

			start = time.Now()
			_, errCreateSE := createServiceEntryForRollout(ctxLogger, ctx, eventType, rc, remoteRegistry.AdmiralCache, localMeshPorts, rollout, serviceEntries)
			ctxLogger.Infof(common.CtxLogFormat, "BuildServiceEntry", deploymentOrRolloutName, deploymentOrRolloutNS, clusterId, "total service entries built="+strconv.Itoa(len(serviceEntries)))
			util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheCreateServiceEntryForRollout",
				deploymentOrRolloutName, deploymentOrRolloutNS, rc.ClusterID, "", start)
			modifySEerr = common.AppendError(modifySEerr, errCreateSE)
		}

		start = time.Now()
		remoteRegistry.AdmiralCache.CnameClusterCache.Put(cname, rc.ClusterID, rc.ClusterID)
		remoteRegistry.AdmiralCache.CnameIdentityCache.Store(cname, partitionedIdentity)
		util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheCnameClusterCachePutAndCnameIdentityCacheStore",
			deploymentOrRolloutName, deploymentOrRolloutNS, rc.ClusterID, "", start)
		sourceWeightedServices[rc.ClusterID] = weightedServices

		if common.IsPersonaTrafficConfig() {
			continue
		}

		gtpsInNamespace := rc.GlobalTraffic.Cache.Get(gtpIdentityKey, namespace)
		if len(gtpsInNamespace) > 0 {
			ctxLogger.Infof(common.CtxLogFormat, "GetGlobalTrafficCache", deploymentOrRolloutName, deploymentOrRolloutNS, clusterId, "found GTP in cache")
			gtps[rc.ClusterID] = gtpsInNamespace
		} else {
			ctxLogger.Infof(common.CtxLogFormat, "GetGlobalTrafficCache", deploymentOrRolloutName, deploymentOrRolloutNS, clusterId, "No GTPs found")
		}

		if rc.OutlierDetectionController != nil && rc.OutlierDetectionController.GetCache() != nil {
			odInNamespace := rc.OutlierDetectionController.GetCache().Get(identityKey, namespace)
			if len(odInNamespace) > 0 {
				ctxLogger.Infof(common.CtxLogFormat, "GetOutlierDetectionCache", deploymentOrRolloutName, deploymentOrRolloutNS, clusterId, "found OutlierDetection in cache")
				outlierDetections[rc.ClusterID] = odInNamespace
			} else {
				ctxLogger.Infof(common.CtxLogFormat, "GetOutlierDetectionCache", deploymentOrRolloutName, deploymentOrRolloutNS, clusterId, "No OutlierDetections found")
			}
		} else {
			ctxLogger.Infof(common.CtxLogFormat, "GetOutlierDetectionCache", deploymentOrRolloutName, deploymentOrRolloutNS, clusterId, "No OutlierDetections found")
		}

		if common.IsClientConnectionConfigProcessingEnabled() {
			err := populateClientConnectionConfigCache(rc, identityKey, namespace, clientConnectionSettings)
			if err != nil {
				ctxLogger.Errorf(
					common.CtxLogFormat, "populateClientConnectionConfigCache", deploymentOrRolloutName,
					deploymentOrRolloutNS, clusterId, err.Error())
			}
			ctxLogger.Infof(
				common.CtxLogFormat, "populateClientConnectionConfigCache", deploymentOrRolloutName,
				deploymentOrRolloutNS, clusterId, "Success")
		} else {
			ctxLogger.Infof(
				common.CtxLogFormat, "populateClientConnectionConfigCache", deploymentOrRolloutName,
				deploymentOrRolloutNS, clusterId, "Skipped as ClientConnectionConfig processing is disabled")
		}
	}

	if common.IsAdmiralStateSyncerMode() {
		ctxLogger.Infof(
			common.CtxLogFormat, "AdmiralStateSyncer", deploymentOrRolloutName,
			deploymentOrRolloutNS, "", "Running in admiral state syncer mode")
		var sourceClusters []string
		// fetch all clusters where a deployment
		// for the identity is present
		for cluster := range sourceDeployments {
			sourceClusters = append(sourceClusters, cluster)
		}
		// fetch all clusters where a rollout
		// for the identity is present
		for cluster := range sourceRollouts {
			sourceClusters = append(sourceClusters, cluster)
		}
		return nil, updateClusterIdentityCache(remoteRegistry, sourceClusters, sourceIdentity)
	}

	//PID: use partitionedIdentity because IdentityDependencyCache is filled using the partitionedIdentity - DONE
	dependents := remoteRegistry.AdmiralCache.IdentityDependencyCache.Get(partitionedIdentity).Copy()
	// updates CnameDependentClusterCache and CnameDependentClusterNamespaceCache
	cname = strings.TrimSpace(cname)
	if cname == "" {
		ctxLogger.Infof(common.CtxLogFormat, "UpdateCnameDependentClusterNamespaceCache", deploymentOrRolloutName, deploymentOrRolloutNS, "", "Skipping processing as cname is empty")
		return nil, common.AppendError(modifySEerr, errors.New("skipped processing as cname is empty"))
	}
	start = time.Now()
	updateCnameDependentClusterNamespaceCache(ctxLogger, remoteRegistry, dependents, deploymentOrRolloutName, deploymentOrRolloutNS, cname, sourceServices)
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheCnameDependentClusterNamespaceCachePut",
		deploymentOrRolloutName, deploymentOrRolloutNS, "", "", start)
	dependentClusters := make(map[string]string)
	if remoteRegistry.AdmiralCache.CnameDependentClusterCache != nil && remoteRegistry.AdmiralCache.CnameDependentClusterCache.Get(cname) != nil {
		dependentClusters = remoteRegistry.AdmiralCache.CnameDependentClusterCache.Get(cname).Copy()
	}

	if common.IsPersonaTrafficConfig() {
		ctxLogger.Info(common.CtxLogFormat, deploymentOrRolloutName, deploymentOrRolloutNS, "", "NOT Generating Service Entry in Traffic Config Persona")
		for sourceCluster, _ := range sourceServices {
			resourceLabels := fetchResourceLabel(sourceDeployments, sourceRollouts, sourceCluster)
			if resourceLabels != nil {
				// check if additional endpoint generation is required
				doGenerateAdditionalEndpoints(ctxLogger, resourceLabels, partitionedIdentity, remoteRegistry.AdmiralCache)
			} else {
				ctxLogger.Warnf(common.CtxLogFormat, "BuildServiceEntry", deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, "unable to find label for rollout or deployment in source cluster: "+sourceCluster)
			}
		}
		return nil, nil
	}

	util.LogElapsedTimeSinceForModifySE(ctxLogger, "BuildServiceEntry",
		deploymentOrRolloutName, deploymentOrRolloutNS, "", "", start)

	//cache the latest GTP in global cache to be reused during DR creation
	start = time.Now()
	err := updateGlobalGtpCache(remoteRegistry, partitionedIdentity, env, gtps, clusterName, ctxLogger)
	if err != nil {
		modifySEerr = common.AppendError(modifySEerr, err)
	}
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheUpdateGlobalGtpCache",
		deploymentOrRolloutName, deploymentOrRolloutNS, "", "", start)

	start = time.Now()
	updateGlobalOutlierDetectionCache(ctxLogger, remoteRegistry.AdmiralCache, sourceIdentity, env, outlierDetections)
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheUpdateGlobalOutlierDetectionCache",
		deploymentOrRolloutName, deploymentOrRolloutNS, "", "", start)

	start = time.Now()
	err = updateGlobalClientConnectionConfigCache(ctxLogger, remoteRegistry.AdmiralCache, sourceIdentity, env, clientConnectionSettings)
	if err != nil {
		ctxLogger.Warnf(common.CtxLogFormat, "UpdateGlobalClientConnectionConfigCache", "", "", "", err.Error())
	}
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheUpdateGlobalClientConnectionConfigCache",
		deploymentOrRolloutName, deploymentOrRolloutNS, "", "", start)

	//handle local updates (source clusters first)
	//update the address to local fqdn for service entry in a cluster local to the service instance
	start = time.Now()

	for sourceCluster, serviceInstance := range sourceServices {
		resourceLabels := fetchResourceLabel(sourceDeployments, sourceRollouts, sourceCluster)
		if resourceLabels != nil {
			// check if additional endpoint generation is required
			ctxLogger.Infof(common.CtxLogFormat, "DoGenerateAdditionalEndpoints",
				deploymentOrRolloutName, deploymentOrRolloutNS, "", "checking if we need additional endpoints. Resource label length:"+strconv.Itoa(len(resourceLabels)))
			isAdditionalEndpointGenerationEnabled = doGenerateAdditionalEndpoints(ctxLogger, resourceLabels, partitionedIdentity, remoteRegistry.AdmiralCache)
			ctxLogger.Infof(common.CtxLogFormat, "DoGenerateAdditionalEndpoints",
				deploymentOrRolloutName, deploymentOrRolloutNS, "", "additional endpoint generation is="+strconv.FormatBool(isAdditionalEndpointGenerationEnabled))
		} else {
			ctxLogger.Warnf(common.CtxLogFormat, "Event", deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, "unable to find label for rollout or deployment in source cluster: "+sourceCluster)
		}
		if createResourcesOnlyInDependentOverrideClusters {
			continue
		}
		// For Deployment <-> Rollout migration
		// This is maintaining the behavior like before if there was no serviceInstance
		// for a sourceCluster the for loop would just move to the next entry
		if serviceInstance[appType[sourceCluster]] == nil {
			continue
		}

		isServiceEntryModifyCalledForSourceCluster = true

		// For Deployment <-> Rollout migration
		// Ignore local fqdn computation if the application is being migrated
		// This is computed in the UpdateEndpointsForDeployToRolloutMigration function
		var localFqdn string
		if !deployRolloutMigration[sourceCluster] {
			localFqdn = serviceInstance[appType[sourceCluster]].Name + common.Sep + serviceInstance[appType[sourceCluster]].Namespace + common.GetLocalDomainSuffix()
		}

		rc := remoteRegistry.GetRemoteController(sourceCluster)
		blueGreenStrategy := isBlueGreenStrategy(sourceRollouts[sourceCluster])
		canaryIstioStrategy := IsCanaryIstioStrategy(sourceRollouts[sourceCluster])

		// For Deployment <-> Rollout migration:
		// Initializing meshDeployAndRolloutPorts Map
		meshDeployAndRolloutPorts := make(map[string]map[string]uint32)
		if _, ok := meshDeployAndRolloutPorts[common.Deployment]; !ok {
			meshDeployAndRolloutPorts[common.Deployment] = make(map[string]uint32)
		}

		if _, ok := meshDeployAndRolloutPorts[common.Rollout]; !ok {
			meshDeployAndRolloutPorts[common.Rollout] = make(map[string]uint32)
		}

		var meshPorts map[string]uint32
		if len(sourceDeployments) > 0 {
			if deployRolloutMigration[sourceCluster] {
				meshPorts = GetMeshPortsForRollout(sourceCluster, serviceInstance[common.Rollout], sourceRollouts[sourceCluster])
				meshDeployAndRolloutPorts[common.Rollout] = meshPorts
			}
			meshPorts = GetMeshPortsForDeployments(sourceCluster, serviceInstance[common.Deployment], sourceDeployments[sourceCluster])
			meshDeployAndRolloutPorts[common.Deployment] = meshPorts
		} else {
			meshPorts = GetMeshPortsForRollout(sourceCluster, serviceInstance[common.Rollout], sourceRollouts[sourceCluster])
			meshDeployAndRolloutPorts[common.Rollout] = meshPorts
		}

		for key, serviceEntry := range serviceEntries {
			if len(serviceEntry.Endpoints) == 0 || (!deployRolloutMigration[sourceCluster] && clustersToDeleteSE[sourceCluster]) {
				ctxLogger.Infof(common.CtxLogFormat, "WriteServiceEntryToSourceClusters", deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, "writing to cluster="+sourceCluster)
				err := AddServiceEntriesWithDrToAllCluster(ctxLogger,
					ctx, remoteRegistry, map[string]string{sourceCluster: sourceCluster},
					map[string]*networking.ServiceEntry{key: serviceEntry}, isAdditionalEndpointGenerationEnabled, isServiceEntryModifyCalledForSourceCluster,
					partitionedIdentity, env)
				if err != nil {
					ctxLogger.Errorf(common.CtxLogFormat, "Event", deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, err.Error())
					modifySEerr = common.AppendError(modifySEerr, err)
				}
			}

			clusterIngress, _ := rc.ServiceController.Cache.GetLoadBalancer(common.GetAdmiralParams().LabelSet.GatewayApp, common.NamespaceIstioSystem)
			for _, ep := range serviceEntry.Endpoints {
				//replace istio ingress-gateway address with local fqdn, note that ingress-gateway can be empty (not provisioned, or is not up)
				if ep.Address == clusterIngress || ep.Address == "" {
					// Update endpoints with locafqdn for active and preview se of bluegreen rollout
					if blueGreenStrategy {
						ctxLogger.Infof(common.CtxLogFormat, "WriteServiceEntryToSourceClusters",
							deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, "Updating ServiceEntry with blue/green endpoints")
						oldPorts := ep.Ports
						updateEndpointsForBlueGreen(sourceRollouts[sourceCluster], sourceWeightedServices[sourceCluster], cnames, ep, sourceCluster, key)
						err := AddServiceEntriesWithDrToAllCluster(
							ctxLogger, ctx, remoteRegistry, map[string]string{sourceCluster: sourceCluster},
							map[string]*networking.ServiceEntry{key: serviceEntry}, isAdditionalEndpointGenerationEnabled, isServiceEntryModifyCalledForSourceCluster, partitionedIdentity, env)
						if err != nil {
							ctxLogger.Errorf(common.CtxLogFormat, "WriteServiceEntryToSourceClusters",
								deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, err.Error())
							modifySEerr = common.AppendError(modifySEerr, err)
						}
						//swap it back to use for next iteration
						ep.Address = clusterIngress
						ep.Ports = oldPorts
						// see if we have weighted services (rollouts with canary strategy)
						// need to add checks for nil here
					} else if canaryIstioStrategy && strings.HasPrefix(key, common.CanaryRolloutCanaryPrefix) {
						ctxLogger.Infof(common.CtxLogFormat, "WriteServiceEntryToSourceClusters",
							deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, "Updating ServiceEntry for canary endpoints")
						//Nil check for canary service is done in iscanaryIstioStrategy function
						canaryService := sourceRollouts[sourceCluster].Spec.Strategy.Canary.CanaryService
						// use only canary service for fqdn
						fqdn := canaryService + common.Sep + serviceInstance[appType[sourceCluster]].Namespace + common.GetLocalDomainSuffix()
						ep.Address = fqdn
						oldPorts := ep.Ports
						ep.Ports = meshPorts
						err := AddServiceEntriesWithDrToAllCluster(
							ctxLogger, ctx, remoteRegistry, map[string]string{sourceCluster: sourceCluster},
							map[string]*networking.ServiceEntry{key: serviceEntry}, isAdditionalEndpointGenerationEnabled, isServiceEntryModifyCalledForSourceCluster, partitionedIdentity, env)
						if err != nil {
							ctxLogger.Errorf(common.CtxLogFormat, "WriteServiceEntryToSourceClusters",
								deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, err.Error())
							modifySEerr = common.AppendError(modifySEerr, err)
						}
						// swap it back to use for next iteration
						ep.Address = clusterIngress
						ep.Ports = oldPorts

					} else if len(sourceWeightedServices[sourceCluster]) > 1 {
						ctxLogger.Infof(common.CtxLogFormat, "WriteServiceEntryToSourceClusters",
							deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, "Updating ServiceEntry with weighted endpoints")
						var se = copyServiceEntry(serviceEntry)
						updateEndpointsForWeightedServices(se, sourceWeightedServices[sourceCluster], clusterIngress, meshPorts)
						err := AddServiceEntriesWithDrToAllCluster(
							ctxLogger, ctx, remoteRegistry, map[string]string{sourceCluster: sourceCluster},
							map[string]*networking.ServiceEntry{key: se}, isAdditionalEndpointGenerationEnabled, isServiceEntryModifyCalledForSourceCluster, partitionedIdentity, env)
						if err != nil {
							ctxLogger.Errorf(common.CtxLogFormat, "WriteServiceEntryToSourceClusters",
								deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, err.Error())
							modifySEerr = common.AppendError(modifySEerr, err)
						}
					} else if deployRolloutMigration[sourceCluster] {
						ctxLogger.Infof(common.CtxLogFormat, "WriteServiceEntryToSourceClusters",
							deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, "Updating ServiceEntry for Deployment to Rollout migration")
						var err error
						var se = copyServiceEntry(serviceEntry)
						err = util.UpdateEndpointsForDeployToRolloutMigration(serviceInstance, se, meshDeployAndRolloutPorts, clusterIngress, clusterAppDeleteMap, sourceCluster, clusterDeployRolloutPresent)
						// If the previous function returned an error that means the endpoints were not updated
						// we should retry updating the endpoints and not apply the non modified SE to the cluster
						if err != nil {
							ctxLogger.Errorf(common.CtxLogFormat, "WriteServiceEntryToSourceClusters",
								deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, err.Error())
							modifySEerr = common.AppendError(modifySEerr, err)
							break
						}
						err = AddServiceEntriesWithDrToAllCluster(
							ctxLogger, ctx, remoteRegistry, map[string]string{sourceCluster: sourceCluster},
							map[string]*networking.ServiceEntry{key: se}, isAdditionalEndpointGenerationEnabled, isServiceEntryModifyCalledForSourceCluster, partitionedIdentity, env)
						if err != nil {
							ctxLogger.Errorf(common.CtxLogFormat, "WriteServiceEntryToSourceClusters",
								deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, err.Error())
							modifySEerr = common.AppendError(modifySEerr, err)
						}
					} else {
						ctxLogger.Infof(common.CtxLogFormat, "WriteServiceEntryToSourceClusters",
							deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, "Updating ServiceEntry regular endpoints")
						ep.Address = localFqdn
						oldPorts := ep.Ports
						ep.Ports = meshPorts
						err := AddServiceEntriesWithDrToAllCluster(
							ctxLogger, ctx, remoteRegistry, map[string]string{sourceCluster: sourceCluster},
							map[string]*networking.ServiceEntry{key: serviceEntry}, isAdditionalEndpointGenerationEnabled, isServiceEntryModifyCalledForSourceCluster, partitionedIdentity, env)
						if err != nil {
							ctxLogger.Errorf(common.CtxLogFormat, "WriteServiceEntryToSourceClusters",
								deploymentOrRolloutName, deploymentOrRolloutNS, sourceCluster, err.Error())
							modifySEerr = common.AppendError(modifySEerr, err)
						}
						// swap it back to use for next iteration
						ep.Address = clusterIngress
						ep.Ports = oldPorts
					}
				}
			}
		}

		start = time.Now()
		if common.GetWorkloadSidecarUpdate() == "enabled" {
			err := modifySidecarForLocalClusterCommunication(
				ctxLogger,
				ctx, serviceInstance[appType[sourceCluster]].Namespace, sourceIdentity,
				remoteRegistry.AdmiralCache.DependencyNamespaceCache, rc)
			if err != nil {
				ctxLogger.Errorf(common.CtxLogFormat, "modifySidecarForLocalClusterCommunication",
					deploymentOrRolloutName, deploymentOrRolloutNS, "", err)
			}
		}

		for _, val := range dependents {
			remoteRegistry.AdmiralCache.DependencyNamespaceCache.Put(val, serviceInstance[appType[sourceCluster]].Namespace, localFqdn, cnames)
		}
	}

	util.LogElapsedTimeSinceForModifySE(ctxLogger, "WriteServiceEntryToSourceClusters",
		deploymentOrRolloutName, deploymentOrRolloutNS, sourceIdentity, "", start)

	//Write to dependent clusters
	start = time.Now()
	isServiceEntryModifyCalledForSourceCluster = false
	if createResourcesOnlyInDependentOverrideClusters {
		var clusters = make(map[string]string, 0)
		dependentClusterOverride.Range(func(k string, v string) {
			clusters[k] = v
		})
		ctxLogger.Infof(common.CtxLogFormat, "WriteServiceEntryToDependentClusters", deploymentOrRolloutName, deploymentOrRolloutNS, "", fmt.Sprintf("Using override values of dependent clusters: %v, count: %v", clusters, len(clusters)))
		dependentClusters = clusters
	}
	err = AddServiceEntriesWithDrToAllCluster(ctxLogger, ctx, remoteRegistry, dependentClusters, serviceEntries, isAdditionalEndpointGenerationEnabled, isServiceEntryModifyCalledForSourceCluster, partitionedIdentity, env)
	if err != nil {
		ctxLogger.Errorf(common.CtxLogFormat, "Event", deploymentOrRolloutName, deploymentOrRolloutNS, "", err.Error())
		modifySEerr = common.AppendError(modifySEerr, err)
	}

	util.LogElapsedTimeSinceForModifySE(ctxLogger, "WriteServiceEntryToDependentClusters",
		deploymentOrRolloutName, deploymentOrRolloutNS, "", "", start)

	return serviceEntries, modifySEerr
}

// Given an identity with a partition prefix, returns the identity without the prefix that is stored in the PartitionIdentityCache
// If the identity did not have a partition prefix, returns the passed in identity
func getNonPartitionedIdentity(admiralCache *AdmiralCache, sourceIdentity string) string {
	if common.EnableSWAwareNSCaches() && admiralCache.PartitionIdentityCache != nil {
		nonPartitionedIdentity := admiralCache.PartitionIdentityCache.Get(sourceIdentity)
		if len(nonPartitionedIdentity) > 0 {
			return nonPartitionedIdentity
		}
	}
	return sourceIdentity
}

func populateClientConnectionConfigCache(rc *RemoteController, identityKey string, namespace string,
	clientConnectionSettings map[string][]*v1.ClientConnectionConfig) error {
	if rc.ClientConnectionConfigController == nil || rc.ClientConnectionConfigController.Cache == nil {
		return fmt.Errorf("clientConnectionSettings controller is not initialized")
	}
	namespacesWithClientConnectionConfig := rc.ClientConnectionConfigController.Cache.Get(identityKey, namespace)
	if len(namespacesWithClientConnectionConfig) == 0 {
		return fmt.Errorf("clientConnectionSettings not found in controller cache")
	}
	clientConnectionSettings[rc.ClusterID] = namespacesWithClientConnectionConfig
	return nil
}

func fetchResourceLabel(sourceDeployments map[string]*k8sAppsV1.Deployment,
	sourceRollouts map[string]*argo.Rollout, cluster string) map[string]string {
	if len(sourceDeployments) > 0 && sourceDeployments[cluster] != nil {
		return parseLabels(sourceDeployments[cluster].Labels)
	}
	if len(sourceRollouts) > 0 && sourceRollouts[cluster] != nil {
		return parseLabels(sourceRollouts[cluster].Labels)
	}
	return nil
}

func parseLabels(labels map[string]string) map[string]string {
	newLabels := make(map[string]string, len(labels))
	for k, v := range labels {
		newLabels[k] = v
	}
	return newLabels
}

func getExistingVS(ctxLogger *logrus.Entry, ctx context.Context, rc *RemoteController, vsName string) (*v1alpha3.VirtualService, error) {
	existingVS, err := rc.VirtualServiceController.IstioClient.NetworkingV1alpha3().VirtualServices(common.GetSyncNamespace()).Get(ctx, vsName, v12.GetOptions{})
	if err != nil && k8sErrors.IsNotFound(err) {
		ctxLogger.Debugf(LogFormat, "get", common.VirtualServiceResourceType, vsName, rc.ClusterID, "virtualservice not found")
		return nil, err
	}
	return existingVS, nil
}

func getAdmiralGeneratedVirtualService(ctx context.Context, remoteController *RemoteController, vsName string,
	namespace string) (*v1alpha3.VirtualService, error) {
	if remoteController == nil {
		return nil, fmt.Errorf("error fetching admiral generated virtualservice as remote controller not initialized")
	}
	if remoteController.VirtualServiceController == nil {
		return nil, fmt.Errorf("error fetching admiral generated virtualservice as VirtualServiceController controller not initialized")
	}

	existingVS, err := remoteController.VirtualServiceController.IstioClient.NetworkingV1alpha3().
		VirtualServices(namespace).Get(ctx, vsName, v12.GetOptions{})
	if err != nil {
		return nil, err
	}
	if existingVS == nil {
		return nil, fmt.Errorf("no virtualservice found with name %s", vsName)
	}

	var result *v1alpha3.VirtualService
	if isGeneratedByAdmiral(existingVS.Annotations) {
		result = existingVS
	}
	return result, nil
}

func updateGlobalClientConnectionConfigCache(ctxLogger *logrus.Entry, cache *AdmiralCache, identity string,
	env string, clientConnectionSettings map[string][]*v1.ClientConnectionConfig) error {

	if !common.IsClientConnectionConfigProcessingEnabled() {
		ctxLogger.Infof(common.CtxLogFormat, "UpdateGlobalClientConnectionConfigCache",
			"", "", "", "skipped as ClientConnectionConfig processing is disabled")
		return nil
	}

	defer util.LogElapsedTime("updateGlobalClientConnectionConfigCache", identity, env, "")()
	clientConnectionSettingsOrdered := make([]*v1.ClientConnectionConfig, 0)
	for _, clientConnectionSettingsInCluster := range clientConnectionSettings {
		clientConnectionSettingsOrdered = append(clientConnectionSettingsOrdered, clientConnectionSettingsInCluster...)
	}

	if len(clientConnectionSettingsOrdered) == 0 {
		ctxLogger.Infof(common.CtxLogFormat, "UpdateGlobalClientConnectionConfigCache", "", "", "", fmt.Sprintf(
			"no %s found for identity=%s in env=%s. Deleting global cache entries if any",
			common.ClientConnectionConfig, identity, env))
		cache.ClientConnectionConfigCache.Delete(identity, env)
		return nil
	}
	if len(clientConnectionSettingsOrdered) > 1 {
		ctxLogger.Infof(common.CtxLogFormat, "UpdateGlobalClientConnectionConfigCache", "", "", "", fmt.Sprintf(
			"more than one %s found for identity=%s in env=%s.", common.ClientConnectionConfig, identity, env))
		sortClientConnectionConfigByCreationTime(clientConnectionSettingsOrdered, identity, env)
	}

	mostRecentClientConnectionConfig := clientConnectionSettingsOrdered[0]

	err := cache.ClientConnectionConfigCache.Put(mostRecentClientConnectionConfig)

	if err != nil {
		return fmt.Errorf("error in updating %s global cache with name=%s in namespace=%s as actively used for identity=%s with err=%w",
			common.ClientConnectionConfig, mostRecentClientConnectionConfig.Name, mostRecentClientConnectionConfig.Namespace,
			identity, err)
	}
	ctxLogger.Infof(common.CtxLogFormat, "UpdateGlobalClientConnectionConfigCache", "", "", "",
		fmt.Sprintf("%s with name=%s in namespace=%s is actively used for identity=%s",
			common.ClientConnectionConfig, mostRecentClientConnectionConfig.Name, mostRecentClientConnectionConfig.Namespace, identity))
	return nil
}

func updateGlobalOutlierDetectionCache(ctxLogger *logrus.Entry, cache *AdmiralCache, identity string, env string, outlierDetections map[string][]*v1.OutlierDetection) {
	defer util.LogElapsedTime("updateGlobalOutlierDetectionCache", identity, env, "")()
	odOrder := make([]*v1.OutlierDetection, 0)
	for _, odsInCluster := range outlierDetections {
		odOrder = append(odOrder, odsInCluster...)
	}

	if len(odOrder) == 0 {
		ctxLogger.Infof("No %s found for identity=%s in env=%s. Deleting global cache entries if any", common.OutlierDetection, identity, env)
		cache.OutlierDetectionCache.Delete(identity, env)
		return
	} else if len(odOrder) > 0 {
		//TODO : Test with multiple outlier detection in use case of env alias qa, qa-west etc
		ctxLogger.Infof("More than one %s found for identity=%s in env=%s.", common.OutlierDetection, identity, env)
		sortOutlierDetectionByCreationTime(odOrder, identity, env)
	}

	mostRecentOd := odOrder[0]

	err := cache.OutlierDetectionCache.Put(mostRecentOd)

	if err != nil {
		ctxLogger.Errorf("Error in updating %s with name=%s in namespace=%s as actively used for identity=%s with err=%v", common.OutlierDetection, mostRecentOd.Name, mostRecentOd.Namespace,
			common.ConstructKeyWithEnvAndIdentity(common.GetODEnv(mostRecentOd), common.GetODIdentity(mostRecentOd)), err)
	} else {
		ctxLogger.Infof("%s with name=%s in namespace=%s is actively used for identity=%s", common.OutlierDetection, mostRecentOd.Name, mostRecentOd.Namespace,
			common.ConstructKeyWithEnvAndIdentity(common.GetODEnv(mostRecentOd), common.GetODIdentity(mostRecentOd)))
	}

}

// Does two things;
// i)  Picks the GTP that was created most recently from the passed in GTP list based on GTP priority label (GTPs from all clusters)
// ii) Updates the global GTP cache with the selected GTP in i)
func updateGlobalGtpCache(remoteRegistry *RemoteRegistry, identity, env string, gtps map[string][]*v1.GlobalTrafficPolicy, clusterName string, ctxLogger *logrus.Entry) error {
	defer util.LogElapsedTimeForModifySE(ctxLogger, "updateGlobalGtpCache", "", "", "", "")()
	gtpsOrdered := make([]*v1.GlobalTrafficPolicy, 0)
	for _, gtpsInCluster := range gtps {
		gtpsOrdered = append(gtpsOrdered, gtpsInCluster...)
	}
	if len(gtpsOrdered) == 0 {
		ctxLogger.Debugf("No GTPs found for identity=%s in env=%s. Deleting global cache entries if any", identity, env)
		oldGTP, _ := remoteRegistry.AdmiralCache.GlobalTrafficCache.GetFromIdentity(identity, env)
		if oldGTP != nil {
			err := handleDynamoDbUpdateForOldGtp(oldGTP, remoteRegistry, clusterName, env, identity, ctxLogger)
			if err != nil {
				ctxLogger.Errorf("failed to update dynamodb data when GTP was deleted for identity=%s and env=%s, err=%v", identity, env, err.Error())
				return fmt.Errorf("failed to update dynamodb data when GTP was deleted for identity=%s and env=%s, err=%v", identity, env, err.Error())
			}
		}
		remoteRegistry.AdmiralCache.GlobalTrafficCache.Delete(identity, env)
		return nil
	} else if len(gtpsOrdered) > 1 {
		ctxLogger.Infof("More than one GTP found for identity=%s in env=%s.", identity, env)
		//sort by creation time and priority, gtp with highest priority and most recent at the beginning
		common.SortGtpsByPriorityAndCreationTime(gtpsOrdered, identity, env)
	}

	mostRecentGtp := gtpsOrdered[0]

	err := remoteRegistry.AdmiralCache.GlobalTrafficCache.Put(mostRecentGtp)

	if err != nil {
		ctxLogger.Errorf("Error in updating GTP with name=%s in namespace=%s as actively used for identity=%s with err=%v", mostRecentGtp.Name, mostRecentGtp.Namespace, common.GetGtpKey(mostRecentGtp), err)
		return fmt.Errorf("error in updating GTP with name=%s in namespace=%s as actively used for identity=%s with err=%v", mostRecentGtp.Name, mostRecentGtp.Namespace, common.GetGtpKey(mostRecentGtp), err)
	} else {
		ctxLogger.Infof("GTP with name=%s in namespace=%s is actively used for identity=%s", mostRecentGtp.Name, mostRecentGtp.Namespace, common.GetGtpKey(mostRecentGtp))
	}
	return nil
}

func sortOutlierDetectionByCreationTime(ods []*v1.OutlierDetection, identity string, env string) {
	sort.Slice(ods, func(i, j int) bool {
		iTime := ods[i].CreationTimestamp
		jTime := ods[j].CreationTimestamp
		logrus.Debugf("%s sorting identity=%s env=%s name1=%s creationTime1=%v name2=%s creationTime2=%v", common.OutlierDetection, identity, env, ods[i].Name, iTime, ods[j].Name, jTime)
		return iTime.After(jTime.Time)
	})
}

func sortClientConnectionConfigByCreationTime(ods []*v1.ClientConnectionConfig, identity string, env string) {
	sort.Slice(ods, func(i, j int) bool {
		iTime := ods[i].CreationTimestamp
		jTime := ods[j].CreationTimestamp
		logrus.Debugf(
			"%s sorting identity=%s env=%s name1=%s creationTime1=%v name2=%s creationTime2=%v",
			common.ClientConnectionConfig, identity, env, ods[i].Name, iTime, ods[j].Name, jTime)
		return iTime.After(jTime.Time)
	})
}

func updateEndpointsForBlueGreen(rollout *argo.Rollout, weightedServices map[string]*WeightedService, cnames map[string]string,
	ep *networking.WorkloadEntry, sourceCluster string, meshHost string) {
	activeServiceName := rollout.Spec.Strategy.BlueGreen.ActiveService
	previewServiceName := rollout.Spec.Strategy.BlueGreen.PreviewService

	if previewService, ok := weightedServices[previewServiceName]; strings.HasPrefix(meshHost, common.BlueGreenRolloutPreviewPrefix+common.Sep) && ok {
		previewServiceInstance := previewService.Service
		localFqdn := previewServiceInstance.Name + common.Sep + previewServiceInstance.Namespace + common.GetLocalDomainSuffix()
		cnames[localFqdn] = "1"
		ep.Address = localFqdn
		ep.Ports = GetMeshPortsForRollout(sourceCluster, previewServiceInstance, rollout)
	} else if activeService, ok := weightedServices[activeServiceName]; ok {
		activeServiceInstance := activeService.Service
		localFqdn := activeServiceInstance.Name + common.Sep + activeServiceInstance.Namespace + common.GetLocalDomainSuffix()
		cnames[localFqdn] = "1"
		ep.Address = localFqdn
		ep.Ports = GetMeshPortsForRollout(sourceCluster, activeServiceInstance, rollout)
	}
}

// update endpoints for Argo rollouts specific Service Entries to account for traffic splitting (Canary strategy)
func updateEndpointsForWeightedServices(serviceEntry *networking.ServiceEntry, weightedServices map[string]*WeightedService, clusterIngress string, meshPorts map[string]uint32) {
	var endpoints = make([]*networking.WorkloadEntry, 0)
	var endpointToReplace *networking.WorkloadEntry

	//collect all endpoints except the one to replace
	for _, ep := range serviceEntry.Endpoints {
		if ep.Address == clusterIngress || ep.Address == "" {
			endpointToReplace = ep
		} else {
			endpoints = append(endpoints, ep)
		}
	}

	if endpointToReplace == nil {
		return
	}

	//create endpoints based on weightedServices
	for _, serviceInstance := range weightedServices {
		//skip service instances with 0 weight
		if serviceInstance.Weight <= 0 {
			continue
		}
		var ep = copyEndpoint(endpointToReplace)
		ep.Ports = meshPorts
		ep.Address = serviceInstance.Service.Name + common.Sep + serviceInstance.Service.Namespace + common.GetLocalDomainSuffix()
		ep.Weight = uint32(serviceInstance.Weight)
		endpoints = append(endpoints, ep)
	}
	serviceEntry.Endpoints = endpoints
}

func modifySidecarForLocalClusterCommunication(
	ctxLogger *logrus.Entry,
	ctx context.Context, sidecarNamespace, sourceIdentity string,
	sidecarEgressMap *common.SidecarEgressMap, rc *RemoteController) error {
	if rc == nil {
		return fmt.Errorf("skipped modifying sidecar resource as remoteController object is nil")
	}
	//get existing sidecar from the cluster
	sidecarConfig := rc.SidecarController
	sidecarEgressMap.Range(func(k string, v map[string]common.SidecarEgress) {
		if k == sourceIdentity {
			sidecarEgress := v
			if sidecarConfig == nil || sidecarEgress == nil {
				return
			}

			sidecar, err := sidecarConfig.IstioClient.NetworkingV1alpha3().Sidecars(sidecarNamespace).Get(ctx, common.GetWorkloadSidecarName(), v12.GetOptions{})
			if err != nil {
				return
			}
			if sidecar == nil || (sidecar.Spec.Egress == nil) {
				return
			}

			//copy and add our new local FQDN
			newSidecar := copySidecar(sidecar)
			egressHosts := make(map[string]string)
			for _, sidecarEgress := range sidecarEgress {
				egressHost := sidecarEgress.Namespace + "/" + sidecarEgress.FQDN
				egressHosts[egressHost] = egressHost
				sidecarEgress.CNAMEs.Range(func(k, v string) {
					scopedCname := sidecarEgress.Namespace + "/" + k
					egressHosts[scopedCname] = scopedCname
				})
			}

			for egressHost := range egressHosts {
				if !util.Contains(newSidecar.Spec.Egress[0].Hosts, egressHost) {
					newSidecar.Spec.Egress[0].Hosts = append(newSidecar.Spec.Egress[0].Hosts, egressHost)
				}
			}

			//nolint
			newSidecarConfig := createSidecarSkeleton(newSidecar.Spec, common.GetWorkloadSidecarName(), sidecarNamespace)

			//insert into cluster
			if newSidecarConfig != nil {
				addUpdateSidecar(ctxLogger, ctx, newSidecarConfig, sidecar, sidecarNamespace, rc)
			}
		}
	})
	return nil
}

func addUpdateSidecar(ctxLogger *logrus.Entry, ctx context.Context, obj *v1alpha3.Sidecar, exist *v1alpha3.Sidecar, namespace string, rc *RemoteController) {
	var err error
	exist.Labels = obj.Labels
	exist.Annotations = obj.Annotations
	exist.Spec = obj.Spec
	if commonUtil.IsAdmiralReadOnly() {
		ctxLogger.Infof(LogErrFormat, "Update", "Sidecar", obj.Name, rc.ClusterID, "Skipped as Admiral pod is in read only mode")
		return
	}
	_, err = rc.SidecarController.IstioClient.NetworkingV1alpha3().Sidecars(namespace).Update(ctx, obj, v12.UpdateOptions{})
	if err != nil {
		ctxLogger.Infof(LogErrFormat, "Update", "Sidecar", obj.Name, rc.ClusterID, err)
	} else {
		ctxLogger.Infof(LogErrFormat, "Update", "Sidecar", obj.Name, rc.ClusterID, "Success")
	}
}

func copySidecar(sidecar *v1alpha3.Sidecar) *v1alpha3.Sidecar {
	newSidecarObj := &v1alpha3.Sidecar{}
	newSidecarObj.Spec.WorkloadSelector = sidecar.Spec.WorkloadSelector
	newSidecarObj.Spec.Ingress = sidecar.Spec.Ingress
	newSidecarObj.Spec.Egress = sidecar.Spec.Egress
	return newSidecarObj
}

// AddServiceEntriesWithDrToAllCluster will create the default service entries and also additional ones specified in GTP
func AddServiceEntriesWithDrToAllCluster(ctxLogger *logrus.Entry, ctx context.Context, rr *RemoteRegistry, sourceClusters map[string]string,
	serviceEntries map[string]*networking.ServiceEntry, isAdditionalEndpointsEnabled bool, isServiceEntryModifyCalledForSourceCluster bool,
	identityId, env string) error {
	if identityId == "" {
		return fmt.Errorf("failed to process service entry as identity passed was empty")
	}

	if env == "" {
		return fmt.Errorf("failed to process service entry as env passed was empty for identity %s", identityId)
	}

	clustersLen := len(sourceClusters)

	var addSEorDRerror error
	for _, se := range serviceEntries {
		clusters := make(chan string, clustersLen)
		errors := make(chan error, clustersLen)

		if len(se.Hosts) == 0 {
			return fmt.Errorf("failed to process service entry for identity %s and env %s as it is nil or has empty hosts", identityId, env)
		}

		ctxLogger.Infof("DependentClusterWorkerConcurrency: %v", common.DependentClusterWorkerConcurrency())

		for w := 1; w <= common.DependentClusterWorkerConcurrency(); w++ {
			go AddServiceEntriesWithDrWorker(ctxLogger, ctx, rr, isAdditionalEndpointsEnabled, isServiceEntryModifyCalledForSourceCluster,
				identityId, env, copyServiceEntry(se), clusters, errors)
		}

		for _, c := range sourceClusters {
			clusters <- c
		}
		close(clusters)

		for i := 1; i <= clustersLen; i++ {
			addSEorDRerror = common.AppendError(addSEorDRerror, <-errors)
		}
	}

	return addSEorDRerror
}

// add logs before sending to channel
func AddServiceEntriesWithDrWorker(
	ctxLogger *logrus.Entry,
	ctx context.Context,
	rr *RemoteRegistry,
	isAdditionalEndpointsEnabled bool,
	isServiceEntryModifyCalledForSourceCluster bool,
	identityId,
	env string,
	seObj *networking.ServiceEntry,
	clusters <-chan string,
	errors chan<- error) {

	// TODO: Check if we are missing assigning error when an error happens
	// TODO: Check if we return silently
	//partitionedIdentity holds the originally passed in identity which could have a partition prefix
	partitionedIdentity := identityId
	//identityId is guaranteed to have the non-partitioned identity
	identityId = getNonPartitionedIdentity(rr.AdmiralCache, identityId)
	for cluster := range clusters { // TODO log cluster / service entry
		se := copyServiceEntry(seObj)
		var (
			start                    = time.Now()
			cache                    = rr.AdmiralCache
			syncNamespace            = common.GetSyncNamespace()
			addSEorDRToAClusterError error
		)

		rc := rr.GetRemoteController(cluster)
		if rc == nil || rc.NodeController == nil || rc.NodeController.Locality == nil {
			ctxLogger.Warnf(common.CtxLogFormat, "AddServiceEntriesWithDrWorker", "", "", cluster, "remote controller not found for the cluster") // TODO: add service entry name
			errors <- nil
			continue
		}

		//this get is within the loop to avoid race condition when one event could update destination rule on stale data
		globalTrafficPolicy, err := cache.GlobalTrafficCache.GetFromIdentity(partitionedIdentity, env)
		if err != nil {
			ctxLogger.Errorf(LogErrFormat, "GlobalTrafficCache", "", "", cluster, err.Error())
		}
		util.LogElapsedTimeSince("AdmiralCacheGlobalTrafficCacheGetFromIdentity", identityId, env, cluster, start)

		if globalTrafficPolicy != nil {
			ctxLogger.Infof(common.CtxLogFormat, "AddServiceEntriesWithDrWorker", "", "", cluster, fmt.Sprintf("creating dr for cluster %v with gtp name %v and in namespace %v", cluster, globalTrafficPolicy.Name, globalTrafficPolicy.Namespace))
		} else {
			ctxLogger.Infof("creating dr for cluster %v without gtp", cluster)
			ctxLogger.Infof(common.CtxLogFormat, "AddServiceEntriesWithDrWorker", "", "", cluster, "cluster, creating dr without gtp")
		}
		outlierDetection, err := cache.OutlierDetectionCache.GetFromIdentity(identityId, env)
		if err != nil {
			ctxLogger.Warnf(common.CtxLogFormat, "AddServiceEntriesWithDrWorker", "", "", cluster, "failed to load OutlierDetection cache")
		}

		if outlierDetection != nil {
			ctxLogger.Infof(common.CtxLogFormat, "AddServiceEntriesWithDrWorker", outlierDetection.Name, outlierDetection.Namespace, cluster, "creating DR with outlier detection")
		} else {
			ctxLogger.Infof(common.CtxLogFormat, "AddServiceEntriesWithDrWorker", "", "", cluster, "creating DR without outlier detection")
		}
		clientConnectionSettings, err := cache.ClientConnectionConfigCache.GetFromIdentity(identityId, env)
		if err != nil {
			ctxLogger.Warnf(common.CtxLogFormat, "AddServiceEntriesWithDrWorker", "", "", cluster, "failed to load clientConnectionSettings cache")
		}

		//check if there is a gtp and add additional hosts/destination rules
		start = time.Now()
		currentDR := getCurrentDRForLocalityLbSetting(rr, isServiceEntryModifyCalledForSourceCluster, cluster, se, partitionedIdentity)
		ctxLogger.Infof("currentDR set for dr=%v cluster=%v", getIstioResourceName(se.Hosts[0], "-default-dr"), cluster)
		var seDrSet = createSeAndDrSetFromGtp(ctxLogger, ctx, env, rc.NodeController.Locality.Region, cluster, se,
			globalTrafficPolicy, outlierDetection, clientConnectionSettings, cache, currentDR)
		util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheCreateSeAndDrSetFromGtp", "", "", cluster, "", start)

		for _, seDr := range seDrSet {
			var (
				oldServiceEntry     *v1alpha3.ServiceEntry
				oldDestinationRule  *v1alpha3.DestinationRule
				newServiceEntry     *v1alpha3.ServiceEntry
				additionalEndpoints []string
				skipSEUpdate        bool
				skipDRUpdate        bool
			)
			start := time.Now()

			oldServiceEntry = rc.ServiceEntryController.Cache.Get(seDr.SeName, cluster)
			if oldServiceEntry == nil {
				ctxLogger.Infof(common.CtxLogFormat, "AddServiceEntriesWithDrWorker")
				oldServiceEntry, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(syncNamespace).Get(ctx, seDr.SeName, v12.GetOptions{})
				// if old service entry not find, just create a new service entry instead
				if err != nil && k8sErrors.IsNotFound(err) {
					ctxLogger.Infof(common.CtxLogFormat, "AddServiceEntriesWithDrWorker", seDr.SeName, "", cluster, fmt.Sprintf("failed fetching old service entry, error=%v", err))
					oldServiceEntry = nil
				} else {

				}
			}

			// check if the existing service entry was created outside of admiral
			// if it was, then admiral will not take any action on this SE
			if oldServiceEntry != nil && !isGeneratedByAdmiral(oldServiceEntry.Annotations) {
				ctxLogger.Infof(common.CtxLogFormat, "AddServiceEntriesWithDrWorker", oldServiceEntry.Name, syncNamespace, cluster, "skipped updating the SE as there exists a custom SE with the same name")
				skipSEUpdate = true
			}
			drReconciliationRequired := reconcileDestinationRule(
				ctxLogger,
				common.EnableDestinationRuleCache(),
				rc,
				seDr.DestinationRule.DeepCopy(),
				seDr.DrName,
				cluster)
			util.LogElapsedTimeSinceForModifySE(ctxLogger, "ReconcileDestinationRule", "", "", cluster, "", start)
			if drReconciliationRequired {
				oldDestinationRule, err = rc.DestinationRuleController.IstioClient.NetworkingV1alpha3().DestinationRules(syncNamespace).Get(ctx, seDr.DrName, v12.GetOptions{})
				if err != nil {
					ctxLogger.Errorf(common.CtxLogFormat, "AddServiceEntriesWithDrWorker", seDr.DrName, syncNamespace, cluster, fmt.Sprintf("failed getting old DestinationRule, error=%v", err))
					oldDestinationRule = nil
				}
				// check if the existing destination rule was created outside of admiral
				// if it was, then admiral will not take any action on this DR
				if oldDestinationRule != nil && !isGeneratedByAdmiral(oldDestinationRule.Annotations) {
					ctxLogger.Warnf(LogFormat, "update", "DestinationRule", oldDestinationRule.Name, cluster, "skipped updating the DR as there exists a custom DR with the same name in "+syncNamespace+" namespace")
					skipDRUpdate = true
				}
			} else {
				ctxLogger.Infof(LogFormat, "update", "DestinationRule", seDr.DrName, cluster, "skipped updating the DR as there is no diff")
				skipDRUpdate = true
			}

			if skipSEUpdate && skipDRUpdate {
				errors <- nil
				continue
			}

			var deleteOldServiceEntry = false
			if oldServiceEntry != nil && !skipSEUpdate {
				areEndpointsValid := validateAndProcessServiceEntryEndpoints(oldServiceEntry)
				if !areEndpointsValid && len(oldServiceEntry.Spec.Endpoints) == 0 {
					deleteOldServiceEntry = true
				}
			}

			//clean service entry in case no endpoints are configured or if all the endpoints are invalid
			if (len(seDr.ServiceEntry.Endpoints) == 0) || deleteOldServiceEntry {
				if !skipSEUpdate {
					start = time.Now()
					err := deleteServiceEntry(ctx, oldServiceEntry, syncNamespace, rc) // [TODO] (needs fix): what happens if it was not able to get the old service entry even though it existed
					util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheDeleteServiceEntry", "", "", cluster, "", start)
					addSEorDRToAClusterError = common.AppendError(addSEorDRToAClusterError, err)

					if isServiceEntryModifyCalledForSourceCluster {
						start = time.Now()
						err = deleteWorkloadData(cluster, env, oldServiceEntry, rr, ctxLogger)
						util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheDeleteWorkloadData", "", "", cluster, "", start)
						if err != nil {
							addSEorDRToAClusterError = common.AppendError(addSEorDRToAClusterError, err)
							ctxLogger.Errorf(LogErrFormat, "Delete", "dynamoDbWorkloadData", env+"."+identityId, cluster, err.Error())
						}
					} else {
						ctxLogger.Infof(LogFormat, "Delete", "dynamoDbWorkloadData", env+"."+identityId, cluster, "skipped deleting workload data as this is not source cluster")
					}

					start = time.Now()
					cache.SeClusterCache.Delete(seDr.ServiceEntry.Hosts[0])
					util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheSeClusterCache Delete", "", "", cluster, "", start)

					// Delete additional endpoints if any
					if isAdditionalEndpointsEnabled {
						vsDNSPrefix := getDNSPrefixFromServiceEntry(seDr)
						start = time.Now()
						// if env contains -air suffix remove it else return original string
						trimmedAirEnv := strings.TrimSuffix(env, common.AIREnvSuffix)
						err = deleteAdditionalEndpoints(ctxLogger, ctx, rc, identityId, trimmedAirEnv, syncNamespace, vsDNSPrefix)
						util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheDeleteWorkloadData", "", "", cluster, "", start)
						if err != nil {
							ctxLogger.Errorf(LogErrFormat, "Delete", "VirtualService", trimmedAirEnv+"."+identityId, cluster, err.Error())
							addSEorDRToAClusterError = common.AppendError(addSEorDRToAClusterError, err)
						}
					} else {
						ctxLogger.Infof(LogFormat, "Delete", "VirtualService", env+"."+identityId, cluster, "skipped deleting additional endpoints through VirtualService in "+syncNamespace+" namespace")
					}
				}
				if !skipDRUpdate {
					start = time.Now()
					// after deleting the service entry, destination rule also need to be deleted if the service entry host no longer exists
					err = deleteDestinationRule(ctx, oldDestinationRule, syncNamespace, rc)
					util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheDeleteDestinationRule", "", "", cluster, "", start)
					addSEorDRToAClusterError = common.AppendError(addSEorDRToAClusterError, err)
				}
			} else {
				if !skipSEUpdate {
					ctxLogger.Infof(common.CtxLogFormat, "CreateServiceEntrySkeleton", seDr.SeName, syncNamespace, cluster, "creating service entry skeleton")
					//nolint
					newServiceEntry = createServiceEntrySkeleton(*seDr.ServiceEntry, seDr.SeName, syncNamespace)
					if newServiceEntry != nil {
						var compareAnnotations, compareLabels []string
						newServiceEntry.Annotations = map[string]string{common.GetWorkloadIdentifier(): fmt.Sprintf("%v", identityId)}
						compareAnnotations = append(compareAnnotations, common.GetWorkloadIdentifier())
						newServiceEntry.Labels = map[string]string{
							common.GetEnvKey(): fmt.Sprintf("%v", env),
						}
						compareLabels = append(compareLabels, common.GetEnvKey())
						if seDr.SeDnsPrefix != "" && seDr.SeDnsPrefix != common.Default {
							newServiceEntry.Annotations[dnsPrefixAnnotationLabel] = seDr.SeDnsPrefix
							compareAnnotations = append(compareAnnotations, dnsPrefixAnnotationLabel)
						}
						if seDr.SeDrGlobalTrafficPolicyName != "" {
							newServiceEntry.Annotations[serviceEntryAssociatedGtpAnnotationLabel] = seDr.SeDrGlobalTrafficPolicyName
							compareAnnotations = append(compareAnnotations, serviceEntryAssociatedGtpAnnotationLabel)
						}

						start = time.Now()
						seReconciliationRequired := reconcileServiceEntry(
							ctxLogger,
							common.EnableServiceEntryCache(),
							rc,
							newServiceEntry.DeepCopy(),
							seDr.SeName,
							cluster,
							compareAnnotations,
							compareLabels)
						util.LogElapsedTimeSinceForModifySE(ctxLogger, "ReconcileServiceEntry", "", "", cluster, "", start)

						if seReconciliationRequired {
							err = addUpdateServiceEntry(ctxLogger, ctx, newServiceEntry, oldServiceEntry, syncNamespace, rc)
							addSEorDRToAClusterError = common.AppendError(addSEorDRToAClusterError, err)
						}
						util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheAddUpdateServiceEntry", "", "", cluster, "", start) // TODO: log service entry name

						start = time.Now()
						cache.SeClusterCache.Put(newServiceEntry.Spec.Hosts[0], rc.ClusterID, rc.ClusterID)
						util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheSeClusterCachePut", "", "", cluster, "", start)
						// Create additional endpoints if necessary
						if isAdditionalEndpointsEnabled {
							ctxLogger.Infof("gatewayAliases=%v", common.GetGatewayAssetAliases())
							// build list of gateway clusters
							gwClusters := []string{}
							for _, gwAlias := range common.GetGatewayAssetAliases() {
								dependents := rr.AdmiralCache.IdentityDependencyCache.Get(partitionedIdentity)
								if dependents != nil && dependents.Len() > 0 {
									dependents.Range(func(_ string, dependent string) {
										if strings.Contains(strings.ToLower(dependent), strings.ToLower(gwAlias)) {
											gwClustersMap := rr.AdmiralCache.IdentityClusterCache.Get(dependent)
											if gwClustersMap != nil {
												for _, cluster := range gwClustersMap.GetKeys() {
													gwClusters = append(gwClusters, cluster)
												}
											}
										}
									})
								}
							}
							ctxLogger.Infof("gatewayClusters=%v", gwClusters)
							vsDNSPrefix := getDNSPrefixFromServiceEntry(seDr)
							// if env contains -air suffix remove it else return original string
							trimmedAirEnv := strings.TrimSuffix(env, common.AIREnvSuffix)
							additionalEndpoints, err = getAdditionalEndpoints(identityId, trimmedAirEnv, vsDNSPrefix)
							if err != nil {
								ctxLogger.Errorf(LogErrFormat, "Create", "VirtualService", trimmedAirEnv+"."+identityId, cluster, err.Error())
								addSEorDRToAClusterError = common.AppendError(addSEorDRToAClusterError, err)
							} else {
								start = time.Now()
								err = createAdditionalEndpoints(
									ctxLogger,
									ctx, rc, rr,
									additionalEndpoints, partitionedIdentity, trimmedAirEnv,
									newServiceEntry.Spec.Hosts[0], syncNamespace, vsDNSPrefix,
									gwClusters, env)
								util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheCreateAdditionalEndpoints", "", "", cluster, "", start)
								if err != nil {
									ctxLogger.Errorf(LogErrFormat, "Create", "VirtualService", trimmedAirEnv+"."+identityId, cluster, err.Error())
									addSEorDRToAClusterError = common.AppendError(addSEorDRToAClusterError, err)
								}
							}
						} else {
							ctxLogger.Infof(LogFormat, "Create", "VirtualService", env+"."+identityId, cluster, "skipped creating additional endpoints through VirtualService in "+syncNamespace+" namespace")
						}

						//update worklaodEndpoint entry to dynamoDB workloadData table only for source entry
						if isServiceEntryModifyCalledForSourceCluster {
							start = time.Now()
							err = storeWorkloadData(cluster, newServiceEntry, globalTrafficPolicy, additionalEndpoints, rr, ctxLogger, *seDr.DestinationRule, true)
							util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheStoreWorkloadData", "", "", cluster, "", start)
							if err != nil {
								addSEorDRToAClusterError = common.AppendError(addSEorDRToAClusterError, err)
								ctxLogger.Errorf(LogErrFormat, "Create", "dynamoDbWorkloadData", env+"."+identityId, cluster, err.Error())
							}
						} else {
							ctxLogger.Infof(LogFormat, "Create", "dynamoDbWorkloadData", env+"."+identityId, cluster, "skipped updating workload data as this is not source cluster")
						}
					}
				}
				if !skipDRUpdate {
					//nolint
					newDestinationRule := createDestinationRuleSkeleton(*seDr.DestinationRule, seDr.DrName, syncNamespace)
					// if event was deletion when this function was called, then GlobalTrafficCache should already deleted the cache globalTrafficPolicy is an empty shell object
					start = time.Now()
					err = addUpdateDestinationRule(ctxLogger, ctx, newDestinationRule, oldDestinationRule, syncNamespace, rc, rr)
					util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheAddUpdateDestinationRule", "", "", cluster, "", start)
					addSEorDRToAClusterError = common.AppendError(addSEorDRToAClusterError, err)

					isSuccess := err == nil

					// update the cluster processing status in dynamodb for each assetAlias and endpoint pair
					// this is only required if GTP is in place
					if globalTrafficPolicy != nil {
						start = time.Now()
						err = storeWorkloadData(cluster, newServiceEntry, globalTrafficPolicy, additionalEndpoints, rr, ctxLogger, *seDr.DestinationRule, isSuccess)
						util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheStoreWorkloadData", "", "", cluster, "", start)
						if err != nil {
							addSEorDRToAClusterError = common.AppendError(addSEorDRToAClusterError, err)
							ctxLogger.Errorf(LogErrFormat, "Update", "dynamoDbWorkloadData", env+"."+identityId, cluster, err.Error())
						}
					}
				}
			}
		}

		errors <- addSEorDRToAClusterError
	}
}

// getDNSPrefixFromServiceEntry returns DNSPrefix set on SE DR Tuple,
// if nothing is set, then it returns default
func getDNSPrefixFromServiceEntry(seDR *SeDrTuple) string {
	if seDR.SeDnsPrefix != "" && seDR.SeDnsPrefix != common.Default {
		return seDR.SeDnsPrefix
	}
	return common.Default
}

func deleteWorkloadData(clusterName, env string, serviceEntry *v1alpha3.ServiceEntry, rr *RemoteRegistry, ctxLogger *logrus.Entry) error {
	start := time.Now()

	if serviceEntry == nil {
		return fmt.Errorf("provided service entry is nil")
	}

	if reflect.DeepEqual(serviceEntry.Spec, networking.ServiceEntry{}) {
		return fmt.Errorf("serviceentry %s has a nil spec", serviceEntry.ObjectMeta.Name)
	}

	if serviceEntry.Spec.Hosts == nil {
		return fmt.Errorf("hosts are not defined in serviceentry: %s", serviceEntry.ObjectMeta.Name)
	}

	if len(serviceEntry.Spec.Hosts) == 0 {
		return fmt.Errorf("0 hosts found in serviceentry: %s", serviceEntry.ObjectMeta.Name)
	}

	if rr.AdmiralDatabaseClient == nil {
		return fmt.Errorf("dynamodb client for workload data table is not initialized")
	}

	workloadDataToDelete := WorkloadData{
		AssetAlias: serviceEntry.Annotations[common.GetWorkloadIdentifier()],
		Endpoint:   serviceEntry.Spec.Hosts[0],
	}

	err := rr.AdmiralDatabaseClient.Delete(workloadDataToDelete, ctxLogger)
	if err != nil {
		return err
	}

	_, ok := rr.AdmiralCache.DynamoDbEndpointUpdateCache.Load(workloadDataToDelete.Endpoint)
	if ok {
		rr.AdmiralCache.DynamoDbEndpointUpdateCache.Delete(workloadDataToDelete.Endpoint)
	}

	util.LogElapsedTimeSince("DeleteEndpointRecord", serviceEntry.Spec.Hosts[0], env, clusterName, start)
	return nil
}

func handleDynamoDbUpdateForOldGtp(oldGtp *v1.GlobalTrafficPolicy, remoteRegistry *RemoteRegistry, clusterName string, env string, identity string, ctxLogger *logrus.Entry) error {

	if oldGtp == nil {
		return fmt.Errorf("provided globaltrafficpolicy is nil")
	}

	if reflect.DeepEqual(oldGtp.Spec, model.GlobalTrafficPolicy{}) {
		return fmt.Errorf("globaltrafficpolicy %s has a nil spec", oldGtp.ObjectMeta.Name)
	}

	if oldGtp.Spec.Policy == nil {
		return fmt.Errorf("policies are not defined in globaltrafficpolicy : %s", oldGtp.ObjectMeta.Name)
	}

	if len(oldGtp.Spec.Policy) == 0 {
		return fmt.Errorf("0 policies configured on globaltrafficpolicy: %s", oldGtp.ObjectMeta.Name)
	}

	if remoteRegistry.AdmiralDatabaseClient == nil {
		return fmt.Errorf("dynamodb client for workload data table is not initialized")
	}

	defer util.LogElapsedTimeForModifySE(ctxLogger, "handleDynamoDbUpdateForOldGtp", oldGtp.Name, oldGtp.Namespace, clusterName, "")()

	workloadData, err := remoteRegistry.AdmiralDatabaseClient.Get(env, identity)

	if err != nil {
		return err
	}

	if workloadData == nil {
		ctxLogger.Infof("got nil workload data when get on admiral database client was called")
		return nil
	}

	for _, existingWorkloadDataItems := range workloadData.([]WorkloadData) {
		if existingWorkloadDataItems.DnsPrefix != "default" {
			workloadDataToUpdate := existingWorkloadDataItems
			workloadDataToUpdate.GtpManagedBy = ""

			err = pushWorkloadDataToDynamodbTable(workloadDataToUpdate, existingWorkloadDataItems.Endpoint, clusterName, remoteRegistry, ctxLogger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func pushWorkloadDataToDynamodbTable(workloadDataToUpdate WorkloadData, endpoint, clusterName string, remoteRegistry *RemoteRegistry, ctxLogger *logrus.Entry) error {
	start := time.Now()
	//calculate sha256sum for fetched workloadData
	newWorkloadDataShasum := calculateShasumForWorkloadData(workloadDataToUpdate)

	//validate if there is diff between new endpoint data and existing data in dynamoDB table
	if !verifyIfEndpointRecordNeedsUpdate(ctxLogger, remoteRegistry.AdmiralCache, endpoint, newWorkloadDataShasum) {
		return nil
	}
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheVerifyIfEndpointRecordNeedsUpdate", endpoint, "", clusterName, "", start)

	//call put operation on dynamoDB workloadData table in case this is new record or has diffs compared to existing record
	start = time.Now()
	err := remoteRegistry.AdmiralDatabaseClient.Update(workloadDataToUpdate, ctxLogger)
	if err != nil {
		return err
	}
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheAdmiralDatabaseClientUpdate", endpoint, "", clusterName, "", start)

	start = time.Now()
	remoteRegistry.AdmiralCache.DynamoDbEndpointUpdateCache.Store(endpoint, fmt.Sprint(newWorkloadDataShasum))
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "AdmiralCacheDynamoDbEndpointUpdateCacheStore", endpoint, "", clusterName, "", start)

	return nil
}

func storeWorkloadData(clusterName string, serviceEntry *v1alpha3.ServiceEntry,
	globalTrafficPolicy *v1.GlobalTrafficPolicy, additionalEndpoints []string, rr *RemoteRegistry, ctxLogger *logrus.Entry, dr networking.DestinationRule, isSuccess bool) error {

	start := time.Now()

	if serviceEntry == nil {
		return fmt.Errorf("provided service entry is nil")
	}

	if reflect.DeepEqual(serviceEntry.Spec, networking.ServiceEntry{}) {
		return fmt.Errorf("serviceentry %s has a nil spec", serviceEntry.ObjectMeta.Name)
	}

	if serviceEntry.Spec.Hosts == nil {
		return fmt.Errorf("hosts are not defined in serviceentry: %s", serviceEntry.ObjectMeta.Name)
	}

	if len(serviceEntry.Spec.Hosts) == 0 {
		return fmt.Errorf("0 hosts found in serviceentry: %s", serviceEntry.ObjectMeta.Name)
	}

	if rr.AdmiralDatabaseClient == nil {
		return fmt.Errorf("dynamodb client for workload data table is not initialized")
	}

	//get worklaod data based on service entry, globaltrafficpolicy and additional endpoints
	workloadData := getWorkloadData(ctxLogger, serviceEntry, globalTrafficPolicy, additionalEndpoints, dr, clusterName, isSuccess)

	err := pushWorkloadDataToDynamodbTable(workloadData, serviceEntry.Spec.Hosts[0], clusterName, rr, ctxLogger)
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "UpdateEndpointRecord", serviceEntry.Spec.Hosts[0], "", clusterName, "", start)
	if err != nil {
		return err
	}
	return nil
}

func calculateShasumForWorkloadData(workloadData WorkloadData) []byte {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%v", workloadData)))
	return h.Sum(nil)
}

func verifyIfEndpointRecordNeedsUpdate(ctxLogger *logrus.Entry, cache *AdmiralCache, serviceEntryHost string, newWorkloadDataShasum []byte) bool {
	existingShaSum, ok := cache.DynamoDbEndpointUpdateCache.Load(serviceEntryHost)
	if ok && (fmt.Sprint(existingShaSum) == fmt.Sprint(newWorkloadDataShasum)) {
		ctxLogger.Infof("no diff between new workload data and existing data for endpoint %v, hence not updating dynamoDB record", serviceEntryHost)
		return false
	}
	return true
}

func getWorkloadData(ctxLogger *logrus.Entry, serviceEntry *v1alpha3.ServiceEntry, globalTrafficPolicy *v1.GlobalTrafficPolicy,
	additionalEndpoints []string, dr networking.DestinationRule, clusterName string, isSuccess bool) WorkloadData {

	var lbType, dnsPrefix, managedBy, gtpId, lastUpdatedAt string
	var trafficDistribution = make(map[string]int32)
	var successClusters, failedClusters []string

	if globalTrafficPolicy != nil {
		lbType, dnsPrefix, trafficDistribution, managedBy, gtpId, lastUpdatedAt = getGTPDetails(ctxLogger, serviceEntry, globalTrafficPolicy)

		if isSuccess {
			successClusters = append(successClusters, clusterName)
		} else {
			failedClusters = append(failedClusters, clusterName)
		}
	} else {
		// If Mesh is Active-Passive and a new application is being onboarded
		// update dynamoDB trafficDistribution to include the primary region
		// with 100% traffic going there. This will be used to reflect the
		// Active-Passive state and the primary region in DevPortal UI
		if common.EnableActivePassive() {
			if &dr != (&networking.DestinationRule{}) &&
				dr.TrafficPolicy != nil &&
				dr.TrafficPolicy.LoadBalancer != nil &&
				dr.TrafficPolicy.LoadBalancer.LocalityLbSetting != nil &&
				dr.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute != nil &&
				len(dr.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute) == 1 &&
				dr.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute[0].From == "*" {
				for region, weight := range dr.TrafficPolicy.LoadBalancer.LocalityLbSetting.Distribute[0].To {
					trafficDistribution[region] = int32(weight)
				}
			}
		}
	}

	workloadData := WorkloadData{
		AssetAlias:          serviceEntry.Annotations[common.GetWorkloadIdentifier()],
		Endpoint:            serviceEntry.Spec.Hosts[0],
		Env:                 serviceEntry.Labels[common.GetEnvKey()],
		DnsPrefix:           dnsPrefix,
		LbType:              lbType,
		TrafficDistribution: trafficDistribution,
		GtpManagedBy:        managedBy,
		GtpId:               gtpId,
		LastUpdatedAt:       lastUpdatedAt,
		SuccessCluster:      successClusters,
		FailedClusters:      failedClusters,
	}

	if len(additionalEndpoints) > 0 {
		workloadData.Aliases = additionalEndpoints
	}

	return workloadData
}

func getGTPDetails(ctxLogger *logrus.Entry, serviceEntry *v1alpha3.ServiceEntry, globalTrafficPolicy *v1.GlobalTrafficPolicy) (string, string, map[string]int32, string, string, string) {
	var lbType, dnsPrefix, gtpManagedBy, gtpId, lastUpdatedAt string
	var trafficDistribution = make(map[string]int32)

	gtpId = globalTrafficPolicy.Annotations[common.IntuitTID]
	if gtpId == "" {
		gtpId = globalTrafficPolicy.ResourceVersion
	}

	lastUpdatedAt = globalTrafficPolicy.Annotations[common.LastUpdatedAt]
	if lastUpdatedAt == "" {
		lastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	if globalTrafficPolicy != nil && globalTrafficPolicy.Spec.Policy != nil {
		gtpManagedBy = gtpManagedByGithub
		for _, managedField := range globalTrafficPolicy.ManagedFields {
			if managedField.Manager == gtpManagerMeshAgentFieldValue {
				gtpManagedBy = gtpManagedByMeshAgent
				break
			}
		}

		for _, globalTrafficPolicy := range globalTrafficPolicy.Spec.Policy {
			if serviceEntry.Annotations != nil && len(serviceEntry.Annotations) != 0 && ((globalTrafficPolicy.DnsPrefix == serviceEntry.Annotations["dns-prefix"]) || (serviceEntry.Annotations["dns-prefix"] == "" && globalTrafficPolicy.DnsPrefix == "default")) {
				lbType = globalTrafficPolicy.LbType.String()
				for _, trafficEntry := range globalTrafficPolicy.Target {
					trafficDistribution[trafficEntry.Region] = trafficEntry.Weight
				}
				dnsPrefix = globalTrafficPolicy.DnsPrefix
				break
			}
		}
	} else {
		ctxLogger.Infof("creating workload entry without gtp details, as gtp or gtp policy is not configured for asset - %v", serviceEntry.Annotations[common.GetWorkloadIdentifier()])
	}

	return lbType, dnsPrefix, trafficDistribution, gtpManagedBy, gtpId, lastUpdatedAt
}

// This func returns a bool to indicate if additional endpoints generation is needed
// based on the following conditions.
//  1. Additional endpoint suffixes have been configured in the admiral params
//  2. The rollout/deployment labels passed contains any of the allowed labels
//     configured in the admiral params 'additional_endpoint_label_filters'
func doGenerateAdditionalEndpoints(ctxLogger *logrus.Entry, labels map[string]string, identity string, admiralCache *AdmiralCache) bool {
	additionalEndpointSuffixes := common.GetAdditionalEndpointSuffixes()
	if len(additionalEndpointSuffixes) <= 0 {
		ctxLogger.Infof(common.CtxLogFormat, "DoGenerateAdditionalEndpoints", "", "", "No additional endpoint suffixes found")
		return false
	}
	// Check if admiral configured allowed labels are in the passed labels map
	additionalEndpointAnnotationFilters := common.GetAdditionalEndpointLabelFilters()
	if util.Contains(additionalEndpointAnnotationFilters, "*") {
		ctxLogger.Infof(common.CtxLogFormat, "DoGenerateAdditionalEndpoints", "", "", "additional endpoints contains *")
		return true
	}
	if doesContainLabel(ctxLogger, labels, additionalEndpointAnnotationFilters) {
		// Store it in the map only if the labels match
		ctxLogger.Infof(common.CtxLogFormat,
			"DoGenerateAdditionalEndpoints", "", "", fmt.Sprintf("labels contains additionalEndpointAnnotationFilters=%v", additionalEndpointAnnotationFilters))
		admiralCache.IdentitiesWithAdditionalEndpoints.Store(identity, identity)
		return true
	}

	// When A -> B is a client -> service pair and additional endpoints generation is enabled for A,
	// we want to generate additional endpoints for B as well even if B does not have the associated labels in admiralParams.AdditionalEndpointLabelFilters.
	// However, we do not store B's identity in admiralCache.IdentitiesWithAdditionalEndpoints.
	dependents := admiralCache.IdentityDependencyCache.Get(identity)
	if dependents != nil {
		for _, dependent := range dependents.GetKeys() {
			_, ok := admiralCache.IdentitiesWithAdditionalEndpoints.Load(dependent)
			if ok {
				ctxLogger.Infof(common.CtxLogFormat, "DoGenerateAdditionalEndpoints", "", "", fmt.Sprintf("dependentAssetWithAdditionalEndpoints=%s", dependent))
				return true
			}
		}
	}
	ctxLogger.Infof(common.CtxLogFormat,
		"DoGenerateAdditionalEndpoints", "", "", "no dependents found, additional endpoints creation=false")
	return false
}

// doesContainLabel returns true if any of the allowedLabels are part of the
// resources's label's map
func doesContainLabel(ctxLogger *logrus.Entry, labels map[string]string, allowedLabels []string) bool {
	if labels == nil {
		ctxLogger.Infof(common.CtxLogFormat, "doesContainLabel", "", "", "", "no labels found")
		return false
	}
	for _, filter := range allowedLabels {
		if _, ok := labels[filter]; ok {
			ctxLogger.Infof(common.CtxLogFormat, "doesContainLabel", "", "", "", "found matching label")
			return true
		}
		ctxLogger.Infof(common.CtxLogFormat, "doesContainLabel", "", "", "", "labels does not contain filter="+filter)
	}
	ctxLogger.Infof(common.CtxLogFormat, "doesContainLabel", "", "", "", "no matching label found")
	return false
}

func validateAdditionalEndpointParams(identity, env string) error {
	if identity == "" {
		return fmt.Errorf("identity passed is empty")
	}
	if env == "" {
		return fmt.Errorf("env passed is empty")
	}
	return nil
}

func getAdditionalEndpointVirtualServiceNames(identity, env, vsDNSPrefix string) (vsNames []string) {
	for _, additionalEndpointSuffix := range common.GetAdditionalEndpointSuffixes() {
		vsName := fmt.Sprintf("%s.%s.%s-vs", env, identity, additionalEndpointSuffix)
		if len(vsDNSPrefix) != 0 {
			vsName = fmt.Sprintf("%s.%s", vsDNSPrefix, vsName)
		}
		vsNames = append(vsNames, vsName)
	}
	return
}

// deleteAdditionalEndpoints deletes all the additional endpoints that were generated for this
// ServiceEntry.
func deleteAdditionalEndpoints(ctxLogger *logrus.Entry, ctx context.Context, rc *RemoteController,
	identity, env, namespace, vsDNSPrefix string) error {

	err := validateAdditionalEndpointParams(identity, env)
	if err != nil {
		return fmt.Errorf("failed deleting additional endpoints due to error %w", err)
	}

	vsNames := getAdditionalEndpointVirtualServiceNames(identity, env, vsDNSPrefix)

	for _, vsName := range vsNames {
		vsToDelete, err := getAdmiralGeneratedVirtualService(ctx, rc, vsName, namespace)
		if err != nil {
			return err
		}

		if vsToDelete == nil {
			ctxLogger.Debug("skipped additional endpoints cleanup as no virtualservice was found to delete")
			return nil
		}

		err = deleteVirtualService(ctx, vsToDelete, namespace, rc)
		if err != nil {
			ctxLogger.Errorf(LogErrFormat, "Delete", "VirtualService", vsToDelete.Name, rc.ClusterID, err)
			return err
		}
		ctxLogger.Infof(LogFormat, "Delete", "VirtualService", vsToDelete.Name, rc.ClusterID, "Success")
	}
	return nil
}

func getAdditionalEndpoints(identity, env, vsDNSPrefix string) ([]string, error) {

	virtualServiceHostnames := make([]string, 0)

	err := validateAdditionalEndpointParams(identity, env)
	if err != nil {
		return virtualServiceHostnames, fmt.Errorf("failed validating additional endpoint parameters due to error %w", err)
	}

	additionalEndpointSuffixes := common.GetAdditionalEndpointSuffixes()
	trimmedAirEnv := strings.TrimSuffix(env, common.AIREnvSuffix)
	for _, suffix := range additionalEndpointSuffixes {
		hostName := strings.ToLower(common.GetCnameVal([]string{trimmedAirEnv, identity, suffix}))
		if vsDNSPrefix != "" && vsDNSPrefix != common.Default {
			hostName = strings.ToLower(common.GetCnameVal([]string{vsDNSPrefix, hostName}))
		}
		virtualServiceHostnames = append(virtualServiceHostnames, hostName)
	}

	return virtualServiceHostnames, nil
}

// createAdditionalEndpoints creates additional endpoints of service defined in the ServiceEntry.
// The list suffixes defined in admiralparams.AdditionalEndpointSuffixes will used to generate the endpoints
func createAdditionalEndpoints(
	ctxLogger *logrus.Entry,
	ctx context.Context,
	rc *RemoteController,
	rr *RemoteRegistry,
	virtualServiceHostnames []string,
	identity, env, destinationHostName, namespace, vsDNSPrefix string,
	gatewayClusters []string, originalEnvLabel string) error {

	err := validateAdditionalEndpointParams(identity, env)
	if err != nil {
		return fmt.Errorf("failed generating additional endpoints due to error %w", err)
	}

	if len(virtualServiceHostnames) == 0 {
		return fmt.Errorf("failed generating additional endpoints for suffixes %s", common.GetAdditionalEndpointSuffixes())
	}
	partitionedIdentity := identity
	identity = getNonPartitionedIdentity(rr.AdmiralCache, identity)

	defaultVSName := getIstioResourceName(virtualServiceHostnames[0], "-vs")

	existingVS, err := getExistingVS(ctxLogger, ctx, rc, defaultVSName)
	if err != nil {
		ctxLogger.Warn(err.Error())
	}
	// Donot update the VirtualService if it already exists as it might overlap with Cartographer.
	// TODO: Plan to move it to Cartographer as a single point of control over VirtualService to avoid conflicts

	eventResourceType, ok := ctx.Value(common.EventResourceType).(string)
	if !ok {
		ctxLogger.Errorf(AlertLogMsg, ctx.Value(common.EventResourceType))
		return errors.New("error occurred trying to get eventResourceType")
	}

	if existingVS != nil {
		if common.IsPresent(gatewayClusters, rc.ClusterID) && eventResourceType == common.Rollout && common.IsAirEnv(originalEnvLabel) {
			ctxLogger.Infof(common.CtxLogFormat, "updateAdditionalEndpointInGWCluster", "admiral-sync", rc.ClusterID, "event for updating existing VS in Gateway cluster received. will be updating the VS.")
		} else {
			ctxLogger.Infof("VirtualService for additional endpoint already exists, skipping. name=%s cluster=%s", defaultVSName, rc.ClusterID)
			return nil
		}
	}

	defaultVSRoute := networking.HTTPRoute{
		Route: []*networking.HTTPRouteDestination{
			{
				Destination: &networking.Destination{
					Host: destinationHostName,
					Port: &networking.PortSelector{
						Number: common.DefaultServiceEntryPort,
					},
				},
			},
		},
	}

	vs := networking.VirtualService{
		Hosts: virtualServiceHostnames,
		Http:  []*networking.HTTPRoute{&defaultVSRoute},
	}
	if common.EnableSWAwareNSCaches() && rr.AdmiralCache.CnameDependentClusterNamespaceCache != nil {
		//This is the .mesh cname that is used as the key for the CnameDependentClusterNamespaceCache and CnameDependentClusterCache
		defaultCname := common.GetCnameVal([]string{env, identity, common.GetHostnameSuffix()})
		defaultCname = strings.TrimSpace(strings.ToLower(defaultCname))
		dependentClusterNamespaces := rr.AdmiralCache.CnameDependentClusterNamespaceCache.Get(defaultCname)
		if dependentClusterNamespaces != nil && dependentClusterNamespaces.Len() > 0 {
			for _, vshostname := range virtualServiceHostnames {
				rr.AdmiralCache.CnameDependentClusterNamespaceCache.PutMapofMaps(strings.ToLower(vshostname), dependentClusterNamespaces)
				ctxLogger.Infof("clusterNamespaces for vs hostname %v  was empty, replacing with clusterNamespaces for %v", vshostname, defaultCname)
				rr.AdmiralCache.CnameIdentityCache.Store(vshostname, partitionedIdentity)
			}
		}
	}

	//nolint
	virtualService := createVirtualServiceSkeleton(vs, defaultVSName, namespace)
	// Add labels and create/update VS
	vsLabels := map[string]string{
		common.GetEnvKey():       env,
		dnsPrefixAnnotationLabel: vsDNSPrefix,
	}
	virtualService.Labels = vsLabels

	vsAnnotations := map[string]string{
		common.GetWorkloadIdentifier(): identity,
	}
	virtualService.Annotations = vsAnnotations

	err = addUpdateVirtualService(ctxLogger, ctx, virtualService, existingVS, namespace, rc, rr)
	if err != nil {
		return fmt.Errorf("failed generating additional endpoints from serviceentry due to error: %w", err)
	}

	return nil
}

func isGeneratedByAdmiral(annotations map[string]string) bool {
	seAnnotationVal, ok := annotations[resourceCreatedByAnnotationLabel]
	if !ok || seAnnotationVal != resourceCreatedByAnnotationValue {
		return false
	}
	return true
}

func isGeneratedByCartographer(annotations map[string]string) bool {
	seAnnotationVal, ok := annotations[resourceCreatedByAnnotationLabel]
	if !ok || seAnnotationVal != resourceCreatedByAnnotationCartographerValue {
		return false
	}
	return true
}

func createSeAndDrSetFromGtp(ctxLogger *logrus.Entry, ctx context.Context, env, region string, cluster string,
	se *networking.ServiceEntry, globalTrafficPolicy *v1.GlobalTrafficPolicy, outlierDetection *v1.OutlierDetection,
	clientConnectionSettings *v1.ClientConnectionConfig, cache *AdmiralCache, currentDR *v1alpha3.DestinationRule) map[string]*SeDrTuple {
	var (
		defaultDrName = getIstioResourceName(se.Hosts[0], "-default-dr")
		defaultSeName = getIstioResourceName(se.Hosts[0], "-se")
		seDrSet       = make(map[string]*SeDrTuple)
	)

	eventResourceType, ok := ctx.Value(common.EventResourceType).(string)
	if !ok {
		ctxLogger.Errorf(AlertLogMsg, ctx.Value(common.EventResourceType))
		return nil
	}

	event := admiral.Add
	if eventResourceType == common.GTP {
		event, ok = ctx.Value(common.EventType).(admiral.EventType)
		if !ok {
			ctxLogger.Errorf(AlertLogMsg, ctx.Value(common.EventType))
			return nil
		}
	}

	if common.EnableExportTo(se.Hosts[0]) && se != nil {
		sortedDependentNamespaces := getSortedDependentNamespaces(cache, se.Hosts[0], cluster, ctxLogger)
		se.ExportTo = sortedDependentNamespaces
	}

	if globalTrafficPolicy != nil {
		gtp := globalTrafficPolicy.Spec
		for _, gtpTrafficPolicy := range gtp.Policy {
			ctxLogger.Infof("Processing dnsPrefix=%s, lbType=%s, "+
				"outlier_detection=%s on gtp=%s from namespace=%s for identity=%s in env=%s",
				gtpTrafficPolicy.DnsPrefix,
				gtpTrafficPolicy.LbType, gtpTrafficPolicy.OutlierDetection.String(),
				globalTrafficPolicy.Name, globalTrafficPolicy.Namespace, common.GetGtpIdentity(globalTrafficPolicy), env)
			var modifiedSe = se
			var host = se.Hosts[0]
			var drName, seName = defaultDrName, defaultSeName
			if gtpTrafficPolicy.Dns != "" {
				ctxLogger.Warnf("Using the deprecated field `dns` in gtp=%v in namespace=%v", globalTrafficPolicy.Name, globalTrafficPolicy.Namespace)
			}
			if gtpTrafficPolicy.DnsPrefix != env && gtpTrafficPolicy.DnsPrefix != common.Default &&
				gtpTrafficPolicy.Dns != host {
				host = common.GetCnameVal([]string{gtpTrafficPolicy.DnsPrefix, se.Hosts[0]})
				drName, seName = getIstioResourceName(host, "-dr"), getIstioResourceName(host, "-se")
				modifiedSe = copyServiceEntry(se)
				modifiedSe.Hosts[0] = host
				// Get appropriate Address for the SE. If Address is empty string and Address gen is disabled,
				// we need to set Addresses to be empty array rather than array with 1 entry of empty string.
				// If we are not disabling Address gen then set Addresses to be array with one entry of Address as usual
				// Even if that Address is somehow empty string, that will throw an error later on which is expected.
				var newAddress, addressErr = getUniqueAddress(ctxLogger, ctx, cache, host)
				if addressErr != nil {
					ctxLogger.Errorf("failed while getting address for %v with error %v", seName, addressErr)
					return nil
				}
				if common.DisableIPGeneration() && len(newAddress) == 0 {
					modifiedSe.Addresses = []string{}
				} else {
					modifiedSe.Addresses = []string{newAddress}
				}
			}
			var seDr = &SeDrTuple{
				DrName:                      drName,
				SeName:                      seName,
				DestinationRule:             getDestinationRule(modifiedSe, region, gtpTrafficPolicy, outlierDetection, clientConnectionSettings, currentDR, eventResourceType, ctxLogger, event),
				ServiceEntry:                modifiedSe,
				SeDnsPrefix:                 gtpTrafficPolicy.DnsPrefix,
				SeDrGlobalTrafficPolicyName: globalTrafficPolicy.Name,
			}
			if strings.HasPrefix(se.Hosts[0], common.CanaryRolloutCanaryPrefix) && len(seDr.SeDnsPrefix) > 0 {
				if seDr.SeDnsPrefix != common.Default {
					seDr.SeDnsPrefix = seDr.SeDnsPrefix + common.Sep + common.CanaryRolloutCanaryPrefix
				} else {
					seDr.SeDnsPrefix = common.CanaryRolloutCanaryPrefix
				}
			}
			seDrSet[host] = seDr
		}
	}
	//create a destination rule for default hostname if that wasn't overridden in gtp
	if _, ok := seDrSet[se.Hosts[0]]; !ok {
		var seDr = &SeDrTuple{
			DrName:          defaultDrName,
			SeName:          defaultSeName,
			DestinationRule: getDestinationRule(se, region, nil, outlierDetection, clientConnectionSettings, currentDR, eventResourceType, ctxLogger, event),
			ServiceEntry:    se,
		}
		if strings.HasPrefix(se.Hosts[0], common.CanaryRolloutCanaryPrefix) {
			seDr.SeDnsPrefix = common.CanaryRolloutCanaryPrefix
		}
		seDrSet[se.Hosts[0]] = seDr
	}

	return seDrSet
}

func makeRemoteEndpointForServiceEntry(address string, locality string, portName string, portNumber int, appType string) *networking.WorkloadEntry {
	return &networking.WorkloadEntry{
		Address:  address,
		Locality: locality,
		Ports:    map[string]uint32{portName: uint32(portNumber)},
		Labels:   map[string]string{"type": appType, "security.istio.io/tlsMode": "istio"},
	}
}

func copyServiceEntry(se *networking.ServiceEntry) *networking.ServiceEntry {
	var newSe = &networking.ServiceEntry{}
	se.DeepCopyInto(newSe)
	return newSe
}

func copyDestinationRule(dr *v1alpha3.DestinationRule) *v1alpha3.DestinationRule {
	var newDr = &v1alpha3.DestinationRule{}
	dr.DeepCopyInto(newDr)
	return newDr
}

func loadServiceEntryCacheData(ctxLogger *logrus.Entry, ctx context.Context, c admiral.ConfigMapControllerInterface,
	admiralCache *AdmiralCache) {
	configmap, err := c.GetConfigMap(ctx)
	if err != nil {
		ctxLogger.Warnf("Failed to refresh configmap state Error: %v", err)
		return //No need to invalidate the cache
	}

	entryCache := GetServiceEntryStateFromConfigmap(configmap)

	if entryCache != nil {
		*admiralCache.ServiceEntryAddressStore = *entryCache
		ctxLogger.Infof("Successfully updated service entry cache state")
	}

}

// GetLocalAddressForSe gets a guarenteed unique local address for a serviceentry. Returns the address, True iff the configmap was updated false otherwise, and an error if any
// Any error coupled with an empty string address means the method should be retried
func GetLocalAddressForSe(ctxLogger *logrus.Entry, ctx context.Context, seName string, seAddressCache *ServiceEntryAddressStore,
	configMapController admiral.ConfigMapControllerInterface) (string, bool, error) {
	var address = seAddressCache.EntryAddresses[seName]
	if address != "" {
		return address, false, nil
	}
	if common.DisableIPGeneration() {
		return "", false, nil
	}
	address, err := GenerateNewAddressAndAddToConfigMap(ctxLogger, ctx, seName, configMapController)
	return address, true, err
}

func GetServiceEntriesByCluster(ctxLogger *logrus.Entry, ctx context.Context, clusterID string,
	remoteRegistry *RemoteRegistry) ([]*v1alpha3.ServiceEntry, error) {
	remoteController := remoteRegistry.GetRemoteController(clusterID)

	if remoteController != nil {
		serviceEntries, err := remoteController.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(common.GetSyncNamespace()).List(ctx, v12.ListOptions{})

		if err != nil {
			ctxLogger.Errorf(LogFormat, "Get", "ServiceEntries", "", clusterID, err)
			return nil, err
		}

		return serviceEntries.Items, nil
	} else {
		return nil, fmt.Errorf("admiral is not monitoring cluster %s", clusterID)
	}
}

// GenerateNewAddressAndAddToConfigMap an atomic fetch and update operation against the configmap
// (using K8s built in optimistic consistency mechanism via resource version)
func GenerateNewAddressAndAddToConfigMap(ctxLogger *logrus.Entry, ctx context.Context, seName string,
	configMapController admiral.ConfigMapControllerInterface) (string, error) {
	//1. get cm, see if there.
	//2. gen new unique address.
	//3. put configmap. RETURN SUCCESSFULLY IF CONFIGMAP PUT SUCCEEDS
	cm, err := configMapController.GetConfigMap(ctx)
	if err != nil {
		return "", err
	}

	newAddressState := GetServiceEntryStateFromConfigmap(cm)

	if newAddressState == nil {
		return "", errors.New("could not unmarshall configmap yaml")
	}

	if val, ok := newAddressState.EntryAddresses[seName]; ok { //Someone else updated the address state, so we'll use that
		return val, nil
	}
	if common.DisableIPGeneration() {
		return "", nil
		// This is the deepest point where Address is set to "" for SEs not in CM
		// If we have reached this point without returning, then we know that seName is not in the CM
		// This means that if are disabling ip gen we want to return empty string here. Otherwise, proceed as usual.
	}

	secondIndex := (len(newAddressState.Addresses) / 255) + 10
	firstIndex := (len(newAddressState.Addresses) % 255) + 1
	address := configMapController.GetIPPrefixForServiceEntries() + common.Sep + strconv.Itoa(secondIndex) + common.Sep + strconv.Itoa(firstIndex)

	for util.Contains(newAddressState.Addresses, address) {
		if firstIndex < 255 {
			firstIndex++
		} else {
			secondIndex++
			firstIndex = 0
		}
		address = configMapController.GetIPPrefixForServiceEntries() + common.Sep + strconv.Itoa(secondIndex) + common.Sep + strconv.Itoa(firstIndex)
	}
	newAddressState.Addresses = append(newAddressState.Addresses, address)
	newAddressState.EntryAddresses[seName] = address

	err = putServiceEntryStateFromConfigmap(ctxLogger, ctx, configMapController, cm, newAddressState)

	if err != nil {
		return "", err
	}
	return address, nil
}

// puts new data into an existing configmap. Providing the original is necessary to prevent fetch and update race conditions
func putServiceEntryStateFromConfigmap(ctxLogger *logrus.Entry, ctx context.Context, c admiral.ConfigMapControllerInterface,
	originalConfigmap *k8sV1.ConfigMap, data *ServiceEntryAddressStore) error {
	if originalConfigmap == nil {
		return errors.New("configmap must not be nil")
	}

	bytes, err := yaml.Marshal(data)

	if err != nil {
		ctxLogger.Errorf("Failed to put service entry state into the configmap. %v", err)
		return err
	}

	if originalConfigmap.Data == nil {
		originalConfigmap.Data = map[string]string{}
	}

	originalConfigmap.Data["serviceEntryAddressStore"] = string(bytes)

	err = ValidateConfigmapBeforePutting(originalConfigmap)
	if err != nil {
		ctxLogger.Errorf("Configmap failed validation. Something is wrong. Error: %v", err)
		return err
	}

	return c.PutConfigMap(ctx, originalConfigmap)
}

func createServiceEntryForRollout(ctxLogger *logrus.Entry, ctx context.Context, event admiral.EventType, rc *RemoteController,
	admiralCache *AdmiralCache, meshPorts map[string]uint32, destRollout *argo.Rollout,
	serviceEntries map[string]*networking.ServiceEntry) (*networking.ServiceEntry, error) {
	workloadIdentityKey := common.GetWorkloadIdentifier()
	globalFqdn := common.GetCnameForRollout(destRollout, workloadIdentityKey, common.GetHostnameSuffix())

	//Handling retries for getting/putting service entries from/in cache

	address, err := getUniqueAddress(ctxLogger, ctx, admiralCache, globalFqdn)
	if err != nil {
		return nil, err
	}

	if !common.DisableIPGeneration() && len(address) == 0 {
		ctxLogger.Errorf(common.CtxLogFormat, "createServiceEntryForRollout", destRollout.Name, destRollout.Namespace, "", "Failed because address is empty while DisableIPGeneration is disabled")
		return nil, nil
	}
	if len(globalFqdn) == 0 {
		ctxLogger.Errorf(common.CtxLogFormat, "createServiceEntryForRollout", destRollout.Name, destRollout.Namespace, "", "Failed because fqdn is empty")
		return nil, nil
	}

	san := getSanForRollout(destRollout, workloadIdentityKey)

	if destRollout.Spec.Strategy.BlueGreen != nil && destRollout.Spec.Strategy.BlueGreen.PreviewService != "" {
		ctxLogger.Infof(common.CtxLogFormat,
			"createServiceEntryForRollout", destRollout.Name, destRollout.Namespace, "", "Building ServiceEntry for BlueGreen")
		rolloutServices := getServiceForRollout(ctx, rc, destRollout)
		if _, ok := rolloutServices[destRollout.Spec.Strategy.BlueGreen.PreviewService]; ok {
			previewGlobalFqdn := common.BlueGreenRolloutPreviewPrefix + common.Sep + common.GetCnameForRollout(destRollout, workloadIdentityKey, common.GetHostnameSuffix())
			admiralCache.CnameIdentityCache.Store(previewGlobalFqdn, common.GetRolloutGlobalIdentifier(destRollout))
			previewAddress, _ := getUniqueAddress(ctxLogger, ctx, admiralCache, previewGlobalFqdn)
			if len(previewGlobalFqdn) != 0 && (common.DisableIPGeneration() || len(previewAddress) != 0) {
				ctxLogger.Infof(common.CtxLogFormat,
					"createServiceEntryForRollout", destRollout.Name, destRollout.Namespace, "", "ServiceEntry previewGlobalFqdn="+previewGlobalFqdn+". previewAddress="+previewAddress)
				generateServiceEntry(ctxLogger, event, admiralCache, meshPorts, previewGlobalFqdn, rc, serviceEntries, previewAddress, san, common.Rollout)
			}
		}
	}

	// Works for istio canary only, creates an additional SE for canary service
	ctxLogger.Infof(common.CtxLogFormat,
		"createServiceEntryForRollout", destRollout.Name, destRollout.Namespace, "", "Generating service entry for canary")
	err = GenerateServiceEntryForCanary(ctxLogger, ctx, event, rc, admiralCache, meshPorts, destRollout, serviceEntries, workloadIdentityKey, san)
	if err != nil {
		ctxLogger.Errorf(common.CtxLogFormat,
			"createServiceEntryForRollout", destRollout.Name, destRollout.Namespace, "", err.Error())
		return nil, err
	}

	tmpSe := generateServiceEntry(ctxLogger, event, admiralCache, meshPorts, globalFqdn, rc, serviceEntries, address, san, common.Rollout)
	ctxLogger.Infof(common.CtxLogFormat,
		"createServiceEntryForRollout", destRollout.Name, destRollout.Namespace, "", "service entry generated")
	return tmpSe, nil
}

func getSanForDeployment(destDeployment *k8sAppsV1.Deployment, workloadIdentityKey string) (san []string) {
	if common.GetEnableSAN() {
		tmpSan := common.GetSAN(common.GetSANPrefix(), destDeployment, workloadIdentityKey)
		if len(tmpSan) > 0 {
			return []string{common.GetSAN(common.GetSANPrefix(), destDeployment, workloadIdentityKey)}
		}
	}
	return nil

}

func getSanForRollout(destRollout *argo.Rollout, workloadIdentityKey string) (san []string) {
	if common.GetEnableSAN() {
		tmpSan := common.GetSANForRollout(common.GetSANPrefix(), destRollout, workloadIdentityKey)
		if len(tmpSan) > 0 {
			return []string{common.GetSANForRollout(common.GetSANPrefix(), destRollout, workloadIdentityKey)}
		}
	}
	return nil

}

func getUniqueAddress(ctxLogger *logrus.Entry, ctx context.Context, admiralCache *AdmiralCache, globalFqdn string) (string, error) {
	start := time.Now()
	defer util.LogElapsedTimeSinceForModifySE(ctxLogger, "GetUniqueAddress",
		"", "", "", "", start)
	//initializations
	var err error = nil
	maxRetries := 3
	counter := 0
	address := ""
	needsCacheUpdate := false

	for counter < maxRetries {
		address, needsCacheUpdate, err = GetLocalAddressForSe(ctxLogger, ctx, getIstioResourceName(globalFqdn, "-se"), admiralCache.ServiceEntryAddressStore, admiralCache.ConfigMapController)

		if len(address) > 0 {
			break
		}
		if len(address) == 0 && err == nil && common.DisableIPGeneration() {
			break
		}
		if err != nil {
			ctxLogger.Errorf("error getting local address for service entry. err: %v", err)
		}

		//random expo backoff
		timeToBackoff := rand.Intn(int(math.Pow(100.0, float64(counter)))) //get a random number between 0 and 100^counter. Will always be 0 the first time, will be 0-100 the second, and 0-1000 the third
		time.Sleep(time.Duration(timeToBackoff) * time.Millisecond)

		counter++
	}

	if err != nil {
		return address, fmt.Errorf("could not get unique address after %v retries. Failing to create serviceentry name=%v", maxRetries, globalFqdn)
	}

	if needsCacheUpdate {
		loadServiceEntryCacheData(ctxLogger, ctx, admiralCache.ConfigMapController, admiralCache)
	}

	return address, nil
}

func generateServiceEntry(ctxLogger *logrus.Entry, event admiral.EventType, admiralCache *AdmiralCache, meshPorts map[string]uint32,
	globalFqdn string, rc *RemoteController, serviceEntries map[string]*networking.ServiceEntry,
	address string, san []string, appType string) *networking.ServiceEntry {
	start := time.Now()
	defer util.LogElapsedTimeSinceForModifySE(ctxLogger, "GenerateServiceEntry", "", "", rc.ClusterID, "", start)
	admiralCache.CnameClusterCache.Put(globalFqdn, rc.ClusterID, rc.ClusterID)
	start = time.Now()
	defer util.LogElapsedTimeSinceForModifySE(ctxLogger, "GenerateServiceEntry", "", "", rc.ClusterID, "", start)
	tmpSe := serviceEntries[globalFqdn]

	var finalProtocol = commonUtil.Http
	var sePorts = []*networking.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
		Name: finalProtocol, Protocol: finalProtocol}}

	for protocol := range meshPorts {
		sePorts = []*networking.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: protocol, Protocol: protocol}}
		finalProtocol = protocol
	}

	if tmpSe == nil {
		tmpSe = &networking.ServiceEntry{
			Hosts:           []string{globalFqdn},
			Ports:           sePorts,
			Location:        networking.ServiceEntry_MESH_INTERNAL,
			Resolution:      networking.ServiceEntry_DNS,
			Addresses:       []string{address}, //It is possible that the address is an empty string. That is fine as the se creation will fail and log an error
			SubjectAltNames: san,
			Endpoints:       []*networking.WorkloadEntry{},
		}
		if common.DisableIPGeneration() && address == "" {
			tmpSe.Addresses = []string{} // If we have reached this stage without throwing an error, then it means we want to create an SE without an address
		}
	}

	start = time.Now()
	endpointAddress, port := rc.ServiceController.Cache.
		GetLoadBalancer(common.GetAdmiralParams().LabelSet.GatewayApp, common.NamespaceIstioSystem)
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "GetLoadBalancer", "", "", rc.ClusterID, "", start)
	var locality string
	if rc.NodeController.Locality != nil {
		locality = rc.NodeController.Locality.Region
	}

	seEndpoint := makeRemoteEndpointForServiceEntry(endpointAddress,
		locality, finalProtocol, port, appType)

	// if the action is deleting an endpoint from service entry, loop through the list and delete matching ones

	if event == admiral.Add || event == admiral.Update {
		match := false
		for _, ep := range tmpSe.Endpoints {
			if ep.Address == seEndpoint.Address {
				match = true
			}
		}

		if !match {
			tmpSe.Endpoints = append(tmpSe.Endpoints, seEndpoint)
		}
	} else if event == admiral.Delete {
		// create a tmp endpoint list to store all the endpoints that we intend to keep
		remainEndpoints := []*networking.WorkloadEntry{}

		// if the endpoint is not equal to the endpoint we intend to delete, append it to remainEndpoint list
		for _, existingEndpoint := range tmpSe.Endpoints {
			if !reflect.DeepEqual(existingEndpoint, seEndpoint) {
				remainEndpoints = append(remainEndpoints, existingEndpoint)
			}
		}

		// If no endpoints left for particular SE, we can delete the service entry object itself later inside function
		// AddServiceEntriesWithDrToAllCluster when updating SE, leave an empty shell skeleton here
		tmpSe.Endpoints = remainEndpoints
	}

	serviceEntries[globalFqdn] = tmpSe

	return tmpSe
}

func isBlueGreenStrategy(rollout *argo.Rollout) bool {
	if rollout != nil && &rollout.Spec != (&argo.RolloutSpec{}) && rollout.Spec.Strategy != (argo.RolloutStrategy{}) {
		if rollout.Spec.Strategy.BlueGreen != nil {
			return true
		}
	}
	return false
}

func reconcileServiceEntry(
	ctxLogger *logrus.Entry,
	enableSECache bool,
	rc *RemoteController,
	desiredSE *v1alpha3.ServiceEntry,
	seName,
	cluster string,
	annotationsKeyToCompare []string,
	labelKeysToCompare []string,
) bool {
	if !enableSECache {
		ctxLogger.Infof(common.CtxLogFormat, "ReconcileServiceEntry", "", "", cluster, "serviceEntryCache processing is disabled")
		return true
	}
	ctxLogger.Infof(common.CtxLogFormat, "ReconcileServiceEntry", seName, "", cluster, "Checking if ServiceEntry requires reconciliation")
	start := time.Now()
	currentSE := rc.ServiceEntryController.Cache.Get(seName, cluster)
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "ReconcileServiceEntry=Get", seName, "", cluster, "", start)
	if currentSE != nil {
		// compare annotations
		result := compareMapsOnKeys(annotationsKeyToCompare, desiredSE.Annotations, currentSE.Annotations)
		if !result {
			return true
		}
		// compare labels
		result = compareMapsOnKeys(labelKeysToCompare, desiredSE.Labels, currentSE.Labels)
		if !result {
			return true
		}
		// compare spec
		desiredSESpec := desiredSE.Spec.DeepCopy()
		currentSESpec := currentSE.Spec.DeepCopy()
		desiredSESpec.Addresses = []string{}
		currentSESpec.Addresses = []string{}

		sort.Sort(WorkloadEntrySorted(desiredSESpec.Endpoints))
		sort.Sort(WorkloadEntrySorted(currentSESpec.Endpoints))

		start = time.Now()
		result = reflect.DeepEqual(desiredSESpec, currentSESpec)
		// compare annotations and labels
		ctxLogger.Infof(common.CtxLogFormat, "ReconcileServiceEntry", seName, "", cluster, "reconcile="+strconv.FormatBool(!result))
		util.LogElapsedTimeSinceForModifySE(ctxLogger, "ReconcileServiceEntry=DeepEqual", seName, "", cluster, "", start)
		return !result
	}
	ctxLogger.Infof(common.CtxLogFormat, "ReconcileServiceEntry", seName, "", cluster, "reconcile=true")
	return true
}

func compareMapsOnKeys(keys []string, desired map[string]string, current map[string]string) bool {
	for _, v := range keys {
		if len(desired[v]) > 0 && desired[v] != current[v] {
			return false
		}
	}
	return true
}

func reconcileDestinationRule(
	ctxLogger *logrus.Entry,
	enableDRCache bool,
	rc *RemoteController,
	dr *networking.DestinationRule,
	drName,
	cluster string) bool {
	if !enableDRCache {
		ctxLogger.Infof(common.CtxLogFormat, "ReconcileDestinationRule", drName, "", cluster, "destinationRuleCache processing is disabled")
		return true
	}
	ctxLogger.Infof(common.CtxLogFormat, "ReconcileDestinationRule", drName, "", cluster, "Checking if DestinationRule requires reconciliation")
	start := time.Now()
	currentDR := rc.DestinationRuleController.Cache.Get(drName, common.GetSyncNamespace())
	util.LogElapsedTimeSinceForModifySE(ctxLogger, "ReconcileDestinationRule=Get", drName, "", cluster, "", start)
	if currentDR != nil {
		drSpec := dr.DeepCopy()
		currentDRSpec := currentDR.Spec.DeepCopy()
		start = time.Now()
		result := reflect.DeepEqual(drSpec, currentDRSpec)
		ctxLogger.Infof(common.CtxLogFormat, "ReconcileDestinationRule", drName, "", cluster, "reconcile="+strconv.FormatBool(!result))
		util.LogElapsedTimeSinceForModifySE(ctxLogger, "ReconcileDestinationRule=DeepEqual", "", "", cluster, "", start)
		return !result
	}
	return true
}

func getCurrentDRForLocalityLbSetting(rr *RemoteRegistry, isServiceEntryModifyCalledForSourceCluster bool, cluster string, se *networking.ServiceEntry, identityId string) *v1alpha3.DestinationRule {
	var (
		syncNamespace = common.GetSyncNamespace()
		cache         = rr.AdmiralCache
		rc            = rr.GetRemoteController(cluster)
		currentDR     *v1alpha3.DestinationRule
	)

	// Check if the DR is already present in the cache
	if rc != nil {
		currentDR = rc.DestinationRuleController.Cache.Get(getIstioResourceName(se.Hosts[0], "-default-dr"), syncNamespace)
	}

	// If the DR is not present in the cache and the processing is for a source cluster
	// we need to check if this is another region where the application is being deployed to.
	// If it is another region where the application is being deployed to we will pick up
	// the DR from the first region to maintain the LocalityLbSetting.
	if currentDR == nil && isServiceEntryModifyCalledForSourceCluster {
		//Needs to be partitionedIdentity - DONE
		clustersMap := cache.IdentityClusterCache.Get(identityId)
		var sourceClusters []string
		if clustersMap != nil {
			sourceClusters = clustersMap.GetKeys()
		}
		for _, clusterID := range sourceClusters {
			sourceRC := rr.GetRemoteController(clusterID)
			if sourceRC != nil {
				currentDR = sourceRC.DestinationRuleController.Cache.Get(getIstioResourceName(se.Hosts[0], "-default-dr"), syncNamespace)
				// When we have found another cluster where the DR exists we will break from the loop
				if currentDR != nil {
					break
				}
			}
		}
	}

	return currentDR
}

func updateCnameDependentClusterNamespaceCache(
	ctxLogger *logrus.Entry,
	remoteRegistry *RemoteRegistry,
	dependents map[string]string,
	deploymentOrRolloutName string,
	deploymentOrRolloutNS string,
	cname string,
	clusterResourceTypeServiceMap map[string]map[string]*k8sV1.Service) {
	//dependentClusterNamespaceCache includes source cluster while dependentClusterCache does not
	if dependents == nil {
		return
	}
	dependentClusterCounter := 0
	for dependentId := range dependents {
		ctxLogger.Infof(common.CtxLogFormat, "Dependent",
			deploymentOrRolloutName, deploymentOrRolloutNS, "", "dependent="+dependentId)
		if remoteRegistry.AdmiralCache == nil || remoteRegistry.AdmiralCache.IdentityClusterCache == nil {
			continue
		}
		identityClusters := remoteRegistry.AdmiralCache.IdentityClusterCache.Get(dependentId)
		var clusterIds []string
		if identityClusters != nil {
			clusterIds = identityClusters.GetKeys()
		}
		if len(clusterIds) > 0 {
			if remoteRegistry.AdmiralCache.CnameDependentClusterCache == nil {
				continue
			}
			for _, clusterId := range clusterIds {
				_, ok := clusterResourceTypeServiceMap[clusterId]
				if !ok {
					dependentClusterCounter++
					ctxLogger.Infof(common.CtxLogFormat, "DependentClusters",
						deploymentOrRolloutName, deploymentOrRolloutNS, "", "cname="+cname+" dependent cluster="+clusterId)
					remoteRegistry.AdmiralCache.CnameDependentClusterCache.Put(cname, clusterId, clusterId)
				} else {
					remoteRegistry.AdmiralCache.CnameDependentClusterCache.DeleteMap(cname, clusterId)
				}
				if !common.EnableSWAwareNSCaches() || remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache == nil {
					continue
				}
				identityClusterNamespaces := remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Get(dependentId)
				var clusterNamespaces *common.Map
				if identityClusterNamespaces != nil {
					clusterNamespaces = identityClusterNamespaces.Get(clusterId)
				}
				var namespaceIds []string
				if clusterNamespaces != nil {
					namespaceIds = clusterNamespaces.GetKeys()
				}
				if len(namespaceIds) > 0 && remoteRegistry.AdmiralCache.CnameDependentClusterNamespaceCache != nil {
					for _, namespaceId := range namespaceIds {
						remoteRegistry.AdmiralCache.CnameDependentClusterNamespaceCache.Put(cname, clusterId, namespaceId, namespaceId)
					}
					ctxLogger.Infof(common.CtxLogFormat, "CnameDependentClusterNamespaceCachePut", deploymentOrRolloutName,
						deploymentOrRolloutNS, clusterId, "cname: "+cname+" put cluster: "+clusterId+" put namespaces: "+strings.Join(namespaceIds, ","))
				} else {
					ctxLogger.Infof(common.CtxLogFormat, "DependentClusterNamespaces", deploymentOrRolloutName,
						deploymentOrRolloutNS, clusterId, "dependent: "+dependentId+" in cluster: "+clusterId+" had no dependent namespaces")
				}

			}

		} else {
			ctxLogger.Infof(common.CtxLogFormat, "DependentClusterNamespaces", deploymentOrRolloutName, deploymentOrRolloutNS, dependentId,
				"dependent: "+dependentId+" had no dependent clusters")
		}
	}
	ctxLogger.Infof(common.CtxLogFormat, "DependentClusters",
		deploymentOrRolloutName, deploymentOrRolloutNS, "", "total dependent clusters="+strconv.Itoa(dependentClusterCounter))
}
