package clusters

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	admiralapiv1 "github.com/istio-ecosystem/admiral-api/pkg/apis/admiral/v1"
	admiralapi "github.com/istio-ecosystem/admiral-api/pkg/client/clientset/versioned"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
)

type ShardHandler struct {
	RemoteRegistry *RemoteRegistry
}

type ConfigWriterData struct {
	IdentityConfig *registry.IdentityConfig
	ClusterName    string
	Error          error
}

func (sh *ShardHandler) Added(ctx context.Context, obj *admiralapiv1.Shard, cc admiralapi.Interface) error {
	err := HandleEventForShard(ctx, admiral.Add, obj, sh.RemoteRegistry, cc)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Add, common.ShardResourceType, obj.Name, "", err)
	}
	return nil
}

func (sh *ShardHandler) Deleted(ctx context.Context, obj *admiralapiv1.Shard) error {
	ctxLogger := common.GetCtxLogger(ctx, obj.Name, "")
	ctxLogger.Warnf(common.CtxLogFormat, "ShardHandlerDeleted", obj.Name, obj.Namespace, "", "", "Shard object was deleted")
	return nil
}

// HandleEventForShardFunc is a handler function for shard events
type HandleEventForShardFunc func(
	ctx context.Context, event admiral.EventType, obj *admiralapiv1.Shard,
	remoteRegistry *RemoteRegistry, clusterName string) error

// helper function to handle add and delete for ShardHandler
func HandleEventForShard(ctx context.Context, event admiral.EventType, obj *admiralapiv1.Shard, remoteRegistry *RemoteRegistry, cc admiralapi.Interface) error {
	var err error
	ctxLogger := common.GetCtxLogger(ctx, obj.Name, "")
	tmpShard := obj.DeepCopy()
	ctxLogger.Infof(common.CtxLogFormat, "HandleEventForShard", obj.Name, "", "", "beginning to handle shard event")
	tmpShardStatusCondition := admiralapiv1.ShardStatusCondition{
		Message:         "Starting to handle shard",
		Reason:          admiralapiv1.Processing,
		Status:          admiralapiv1.FalseConditionStatus,
		LastUpdatedTime: v1.Time{},
	}
	if len(tmpShard.Status.Conditions) > 0 {
		tmpShard.Status.Conditions = slices.Insert(tmpShard.Status.Conditions, 0, tmpShardStatusCondition)
	} else {
		tmpShard.Status.Conditions = []admiralapiv1.ShardStatusCondition{tmpShardStatusCondition}
	}
	err = updateShardStatus(ctx, ctxLogger, tmpShard, obj, cc)
	if err != nil {
		ctxLogger.Warnf(common.CtxLogFormat, "updateShardStatus", obj.Name, obj.Namespace, "", "failed to update shard status to processing")
	}
	var consumerWG, producerWG, resultsWG sync.WaitGroup
	configWriterData := make(chan *ConfigWriterData, 1000)
	configWriterDataResults := make(chan *ConfigWriterData, 1000)
	for i := 0; i < 5; i++ {
		consumerWG.Add(1)
		go ConsumeIdentityConfigs(ctxLogger, ctx, configWriterData, configWriterDataResults, remoteRegistry, &consumerWG)
	}
	// Get all ICs from shard and put into channel
	producerWG.Add(1)
	go ProduceIdentityConfigsFromShard(ctxLogger, *obj, configWriterData, remoteRegistry, &producerWG)
	// Start processing results
	resultsWG.Add(1)
	go ProcessResults(ctx, ctxLogger, configWriterDataResults, &resultsWG, tmpShard, cc)
	// wait for all consumers to finish
	producerWG.Wait()
	consumerWG.Wait()
	// all consumers done,no more values sent to results
	close(configWriterDataResults)
	// wait for all results to be processed
	resultsWG.Wait()
	//TODO: choose what errors we want to retry shard processing very carefully - this error is only for writing shard to cluster, can choose what error to return in an error channel from processresults
	return nil
}

// ProduceIdentityConfigsFromShard creates a registry client and uses it to get the identity configs
// of the assets on the shard, and puts those into configWriterData which go into the job channel
func ProduceIdentityConfigsFromShard(ctxLogger *log.Entry, shard admiralapiv1.Shard, configWriterData chan<- *ConfigWriterData, rr *RemoteRegistry, producerWG *sync.WaitGroup) {
	cnames := make(map[string]string)
	defer util.LogElapsedTime("ProduceIdentityConfigsFromShard", shard.Name, shard.Namespace, "")
	for _, clusterShard := range shard.Spec.Clusters {
		for _, identityItem := range clusterShard.Identities {
			start := time.Now()
			identityConfig, err := rr.RegistryClient.GetIdentityConfigByIdentityName(identityItem.Name, ctxLogger)
			if err != nil {
				ctxLogger.Warnf(common.CtxLogFormat, "ProduceIdentityConfig", identityItem.Name, shard.Namespace, clusterShard.Name, err)
				configWriterData <- &ConfigWriterData{
					IdentityConfig: &registry.IdentityConfig{IdentityName: identityItem.Name},
					ClusterName:    clusterShard.Name,
					Error:          err,
				}
				continue
			}
			ctxLogger.Infof(common.CtxLogFormat, "ProduceIdentityConfig", identityConfig.IdentityName, shard.Namespace, clusterShard.Name, "successfully produced IdentityConfig")
			// Fill the IdentityDependencyCache
			for _, clientAsset := range identityConfig.ClientAssets {
				rr.AdmiralCache.IdentityDependencyCache.Put(identityConfig.IdentityName, clientAsset, clientAsset)
			}
			// Fill the GTP, OD, and CCC caches
			for _, identityConfigCluster := range identityConfig.Clusters {
				for _, identityConfigEnv := range identityConfigCluster.Environment {
					err = rr.AdmiralCache.GlobalTrafficCache.Put(&identityConfigEnv.TrafficPolicy.GlobalTrafficPolicy)
					if err != nil {
						ctxLogger.Warnf(common.CtxLogFormat, "ProduceIdentityConfigGTPPut", identityItem.Name, shard.Namespace, clusterShard.Name, err)
					}
					err = rr.AdmiralCache.OutlierDetectionCache.Put(&identityConfigEnv.TrafficPolicy.OutlierDetection)
					if err != nil {
						ctxLogger.Warnf(common.CtxLogFormat, "ProduceIdentityConfigODPut", identityItem.Name, shard.Namespace, clusterShard.Name, err)
					}
					err = rr.AdmiralCache.ClientConnectionConfigCache.Put(&identityConfigEnv.TrafficPolicy.ClientConnectionConfig)
					if err != nil {
						ctxLogger.Warnf(common.CtxLogFormat, "ProduceIdentityConfigCCCPut", identityItem.Name, shard.Namespace, clusterShard.Name, err)
					}
					// Fill the DependencyNamespaceCache
					for _, clientAsset := range identityConfig.ClientAssets {
						//TODO: How to deal with multiple services here?
						cname := common.GetCnameVal([]string{identityConfigEnv.Name, strings.ToLower(identityConfig.IdentityName), common.GetHostnameSuffix()})
						cnames[cname] = "1"
						localFqdn := identityConfigEnv.ServiceName + common.Sep + identityConfigEnv.Namespace + common.GetLocalDomainSuffix()
						rr.AdmiralCache.DependencyNamespaceCache.Put(clientAsset, identityConfigEnv.Namespace, localFqdn, cnames)
					}
				}
				// Fill the ClusterLocalityCache
				rr.AdmiralCache.ClusterLocalityCache.Put(identityConfigCluster.Name, identityConfigCluster.Name, identityConfigCluster.Locality)
			}
			configWriterData <- &ConfigWriterData{
				IdentityConfig: &identityConfig,
				ClusterName:    clusterShard.Name,
				Error:          err,
			}
			util.LogElapsedTimeSince("ProduceIdentityConfigsFillCaches", identityItem.Name, "", clusterShard.Name, start)
		}
	}
	producerWG.Done()
	close(configWriterData)
}

// ConsumeIdentityConfigs takes a configWriterData from the data channel and produces the networking resources for the
// identity in the config. It then returns the result to the results channel.
func ConsumeIdentityConfigs(ctxLogger *log.Entry, ctx context.Context, configWriterData <-chan *ConfigWriterData, configWriterDataResults chan<- *ConfigWriterData, rr *RemoteRegistry, wg *sync.WaitGroup) {
	defer wg.Done()
	for data := range configWriterData {
		if data.Error != nil {
			ctxLogger.Warnf(common.CtxLogFormat, "ConsumeIdentityConfig", "", "", data.ClusterName, "received configWriterData with error from producer")
			configWriterDataResults <- data
			continue
		}
		identityConfig := data.IdentityConfig
		assetName := identityConfig.IdentityName
		clientCluster := data.ClusterName
		ctxLogger.Infof(common.CtxLogFormat, "ConsumeIdentityConfig", assetName, "", clientCluster, "starting to consume identityConfig")
		start := time.Now()
		serviceEntryBuilder := ServiceEntryBuilder{ClientCluster: clientCluster, RemoteRegistry: rr}
		serviceEntries, err := serviceEntryBuilder.BuildServiceEntriesFromIdentityConfig(ctxLogger, *identityConfig)
		if err != nil {
			ctxLogger.Warnf(common.CtxLogFormat, "ConsumeIdentityConfigBuildSEs", assetName, "", clientCluster, err)
			data.Error = err
			configWriterDataResults <- data
			continue
		}
		isServiceEntryModifyCalledForSourceCluster := false
		sourceClusterEnvironmentNamespaces := map[string]string{}
		for _, cluster := range identityConfig.Clusters {
			if cluster.Name == clientCluster {
				isServiceEntryModifyCalledForSourceCluster = true
				for _, environment := range cluster.Environment {
					sourceClusterEnvironmentNamespaces[environment.Name] = environment.Namespace
				}
				break
			}
		}
		// Get any type from the identityConfig
		for _, cv := range identityConfig.Clusters {
			for _, ev := range cv.Environment {
				for typeKey := range ev.Type {
					ctx = context.WithValue(ctx, common.EventResourceType, typeKey)
					break
				}
				break
			}
			break
		}
		for _, se := range serviceEntries {
			isServiceEntryModifyCalledForSourceClusterAndEnv := false
			env := strings.Split(se.Hosts[0], common.Sep)[0]
			if _, ok := sourceClusterEnvironmentNamespaces[env]; ok && isServiceEntryModifyCalledForSourceCluster {
				isServiceEntryModifyCalledForSourceClusterAndEnv = true
			}
			clusters := make(chan string, 1)
			errors := make(chan error, 1)
			go AddServiceEntriesWithDrWorker(ctxLogger, ctx, rr,
				true, //TODO: doGenerateAdditionalEndpoints()
				isServiceEntryModifyCalledForSourceClusterAndEnv,
				assetName,
				strings.Split(se.Hosts[0], common.Sep)[0],
				"",
				copyServiceEntry(se),
				clusters,
				errors)
			clusters <- clientCluster
			close(clusters)
			err := <-errors
			if err != nil {
				ctxLogger.Errorf(common.CtxLogFormat, "ConsumeIdentityConfigAddSEWithDRWorker", strings.ToLower(se.Hosts[0])+"-se", "", clientCluster, err)
				data.Error = err
			}
			if isServiceEntryModifyCalledForSourceClusterAndEnv {
				ctxLogger.Infof(common.CtxLogFormat, "ConsumeIdentityConfigModifySidecar", strings.ToLower(se.Hosts[0])+"-se", "", clientCluster, "modifying Sidecar for local cluster communication")
				err = modifySidecarForLocalClusterCommunication(
					ctxLogger,
					ctx, sourceClusterEnvironmentNamespaces[env], assetName,
					rr.AdmiralCache.DependencyNamespaceCache, rr.GetRemoteController(clientCluster))
				if err != nil {
					ctxLogger.Errorf(common.CtxLogFormat, "ConsumeIdentityConfigModifySidecarErr", assetName, sourceClusterEnvironmentNamespaces[env], "", err)
					data.Error = err
				}
			}
		}
		configWriterDataResults <- data
		util.LogElapsedTimeSince("ConsumeIdentityConfigs", assetName, "", clientCluster, start)
	}
}

// ProcessResults reads the data object from the results channel and updates the original shard object with the proper result.
func ProcessResults(ctx context.Context, ctxLogger *log.Entry, results <-chan *ConfigWriterData, resultswg *sync.WaitGroup, shard *admiralapiv1.Shard, cc admiralapi.Interface) error {
	defer resultswg.Done()
	updatedShard := shard.DeepCopy()
	updatedShard.Status.ClustersMonitored = len(shard.Spec.Clusters)
	updatedShardStatusCondition := admiralapiv1.ShardStatusCondition{
		Message:         "Shard handling complete",
		Reason:          admiralapiv1.Processed,
		Status:          admiralapiv1.TrueConditionStatus,
		Type:            admiralapiv1.SyncComplete,
		LastUpdatedTime: v1.Time{Time: time.Now()},
	}
	updatedShard.Status.FailureDetails = admiralapiv1.FailureDetails{
		LastUpdatedTime: v1.Time{Time: time.Now()},
		FailedClusters:  []admiralapiv1.FailedCluster{},
	}
	clusterFailedIdentitiesMap := make(map[string][]admiralapiv1.FailedIdentity)
	for data := range results {
		if data.Error != nil {
			ctxLogger.Warnf(common.CtxLogFormat, "ProcessResults", shard.Name, common.GetOperatorSyncNamespace(), data.ClusterName, data.Error.Error())
			failedIdentityList := clusterFailedIdentitiesMap[data.ClusterName]
			failedIdentity := admiralapiv1.FailedIdentity{
				Name:    data.IdentityConfig.IdentityName,
				Message: data.Error.Error(),
			}
			if failedIdentityList != nil {
				clusterFailedIdentitiesMap[data.ClusterName] = slices.Insert(failedIdentityList, 0, failedIdentity)
			} else {
				clusterFailedIdentitiesMap[data.ClusterName] = []admiralapiv1.FailedIdentity{failedIdentity}
			}
			updatedShardStatusCondition.Reason = admiralapiv1.ErrorOccurred
			updatedShardStatusCondition.Type = admiralapiv1.SyncFailed
			updatedShardStatusCondition.LastUpdatedTime = v1.Time{Time: time.Now()}
			updatedShard.Status.FailureDetails.LastUpdatedTime = v1.Time{Time: time.Now()}
		}
	}
	for cluster, identities := range clusterFailedIdentitiesMap {
		updatedShard.Status.FailureDetails.FailedClusters = append(updatedShard.Status.FailureDetails.FailedClusters, admiralapiv1.FailedCluster{
			Name:             cluster,
			FailedIdentities: identities,
		})
	}
	//Just overwrite the first one because we already created a new condition entry in HandleEventForShard
	updatedShard.Status.Conditions[0] = updatedShardStatusCondition
	updatedShard.Status.LastUpdatedTime = v1.Time{Time: time.Now()}
	err := updateShardStatus(ctx, ctxLogger, updatedShard, shard, cc)
	if err != nil {
		ctxLogger.Errorf(common.CtxLogFormat, "ProcessResults", shard.Name, shard.Namespace, "", err)
	}
	return err
}

func updateShardStatus(ctx context.Context, ctxLogger *log.Entry, obj *admiralapiv1.Shard, exist *admiralapiv1.Shard, cc admiralapi.Interface) error {
	var err error
	ctxLogger.Infof(common.CtxLogFormat, "UpdateShardStatus", obj.Namespace, "", "Updating Shard="+obj.Name)
	exist, err = cc.AdmiralV1().Shards(obj.Namespace).Get(ctx, obj.Name, v1.GetOptions{})
	if err != nil {
		exist = obj
		ctxLogger.Warnf(common.CtxLogFormat, "UpdateShard", exist.Name, exist.Namespace, "", "got error on fetching shard, will retry updating")
	}
	exist.Labels = obj.Labels
	exist.Annotations = obj.Annotations
	exist.Spec = obj.Spec
	_, err = cc.AdmiralV1().Shards(exist.Namespace).Update(ctx, exist, v1.UpdateOptions{})
	if err != nil {
		ctxLogger.Infof(common.CtxLogFormat, "UpdateShard", exist.Name, exist.Namespace, "", "Failed to initially update shard="+obj.Name)
		err = retryUpdatingShard(ctx, ctxLogger, obj, exist, cc, err)
	}
	if err != nil {
		ctxLogger.Errorf(LogErrFormat, "UpdateShardStatus", common.ShardResourceType, obj.Name, "", err)
		return err
	} else {
		ctxLogger.Infof(LogFormat, "UpdateShardStatus", common.ShardResourceType, obj.Name, "", "Success")
	}
	return nil
}

func retryUpdatingShard(ctx context.Context, ctxLogger *log.Entry, obj *admiralapiv1.Shard, exist *admiralapiv1.Shard, cc admiralapi.Interface, err error) error {
	numRetries := 5
	if err != nil && k8sErrors.IsConflict(err) {
		for i := 0; i < numRetries; i++ {
			ctxLogger.Errorf(common.CtxLogFormat, "Update", obj.Name, obj.Namespace, "", err.Error()+". retry Shard update "+strconv.Itoa(i+1)+"/"+strconv.Itoa(numRetries))
			updatedShard, err := cc.AdmiralV1().Shards(exist.Namespace).Get(ctx, exist.Name, v1.GetOptions{})
			// if old shard not found, move on
			if err != nil {
				ctxLogger.Infof(common.CtxLogFormat, "Update", exist.Name, exist.Namespace, "", err.Error()+fmt.Sprintf(". Error getting old shard"))
				continue
			}
			ctxLogger.Infof(common.CtxLogFormat, "Update", obj.Name, obj.Namespace, "", fmt.Sprintf("existingResourceVersion=%s resourceVersionUsedForUpdate=%s", updatedShard.ResourceVersion, obj.ResourceVersion))
			updatedShard.Spec = obj.Spec
			updatedShard.Status = obj.Status
			updatedShard.Annotations = obj.Annotations
			updatedShard.Labels = obj.Labels
			sh, err := cc.AdmiralV1().Shards(exist.Namespace).Update(ctx, updatedShard, v1.UpdateOptions{})
			if err == nil || sh == nil {
				return nil
			}
		}
	}
	return err
}
