package clusters

import (
	"context"
	"fmt"
	"strings"
	"sync"

	admiralapiv1 "github.com/istio-ecosystem/admiral-api/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"

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
	// TODO: Could keep this result field or derive it from the passed along error, also could be Shard.Status type instead of string
	Result string
	Error  error
}

func (sh *ShardHandler) Added(ctx context.Context, obj *admiralapiv1.Shard) error {
	err := HandleEventForShard(ctx, admiral.Add, obj, sh.RemoteRegistry)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Add, common.ShardResourceType, obj.Name, "", err)
	}
	return nil
}

func (sh *ShardHandler) Deleted(ctx context.Context, obj *admiralapiv1.Shard) error {
	// TODO: Not yet implemented
	//err := HandleEventForShard(ctx, admiral.Delete, obj, sh.RemoteRegistry)
	//if err != nil {
	//	return fmt.Errorf(LogErrFormat, common.Delete, common.ShardResourceType, obj.Name, "", err)
	//}
	return nil
}

// HandleEventForShardFunc is a handler function for shard events
type HandleEventForShardFunc func(
	ctx context.Context, event admiral.EventType, obj *admiralapiv1.Shard,
	remoteRegistry *RemoteRegistry, clusterName string) error

// helper function to handle add and delete for ShardHandler
func HandleEventForShard(ctx context.Context, event admiral.EventType, obj *admiralapiv1.Shard,
	remoteRegistry *RemoteRegistry) error {
	ctxLogger := common.GetCtxLogger(ctx, obj.Name, "")
	tmpShard := obj.DeepCopy()
	ctxLogger.Infof(common.CtxLogFormat, "HandleEventForShard", obj.Name, "", "", "")
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
	go UpdateShard(ctxLogger, configWriterDataResults, &resultsWG, tmpShard)
	// wait for all consumers to finish
	producerWG.Wait()
	consumerWG.Wait()
	// all consumers done,no more values sent to results
	close(configWriterDataResults)
	// wait for all results to be processed
	resultsWG.Wait()
	//TODO: Need to write the new tmpShard with all the results to the cluster + return error for the item to be requeued
	return nil
}

// ProduceIdentityConfigsFromShard creates a registry client and uses it to get the identity configs
// of the assets on the shard, and puts those into configWriterData which go into the job channel
func ProduceIdentityConfigsFromShard(ctxLogger *log.Entry, shard admiralapiv1.Shard, configWriterData chan<- *ConfigWriterData, rr *RemoteRegistry, producerWG *sync.WaitGroup) {
	cnames := make(map[string]string)
	for _, clusterShard := range shard.Spec.Clusters {
		for _, identityItem := range clusterShard.Identities {
			identityConfig, err := rr.RegistryClient.GetIdentityConfigByIdentityName(identityItem.Name, ctxLogger)
			if err != nil {
				ctxLogger.Warnf(common.CtxLogFormat, "ProduceIdentityConfig", identityItem.Name, shard.Namespace, clusterShard.Name, err)
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
		identityConfig := data.IdentityConfig
		assetName := identityConfig.IdentityName
		clientCluster := data.ClusterName
		ctxLogger.Infof(common.CtxLogFormat, "ConsumeIdentityConfig", assetName, "", clientCluster, "starting to consume identityConfig")
		serviceEntryBuilder := ServiceEntryBuilder{ClientCluster: clientCluster, RemoteRegistry: rr}
		serviceEntries, err := serviceEntryBuilder.BuildServiceEntriesFromIdentityConfig(ctxLogger, *identityConfig)
		if err != nil {
			ctxLogger.Warnf(common.CtxLogFormat, "ConsumeIdentityConfig", assetName, "", clientCluster, err)
			data.Result = err.Error()
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
				ctx = context.WithValue(ctx, common.EventResourceType, ev.Type)
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
				copyServiceEntry(se),
				clusters,
				errors)
			clusters <- clientCluster
			close(clusters)
			err := <-errors
			if err != nil {
				ctxLogger.Warnf(common.CtxLogFormat, "ConsumeIdentityConfig", strings.ToLower(se.Hosts[0])+"-se", "", clientCluster, err)
				data.Result = err.Error()
			}
			if isServiceEntryModifyCalledForSourceClusterAndEnv {
				ctxLogger.Infof(common.CtxLogFormat, "ConsumeIdentityConfig", strings.ToLower(se.Hosts[0])+"-se", "", clientCluster, "modifying Sidecar for local cluster communication")
				err = modifySidecarForLocalClusterCommunication(
					ctxLogger,
					ctx, sourceClusterEnvironmentNamespaces[env], assetName,
					rr.AdmiralCache.DependencyNamespaceCache, rr.GetRemoteController(clientCluster))
				if err != nil {
					ctxLogger.Errorf(common.CtxLogFormat, "modifySidecarForLocalClusterCommunication",
						assetName, sourceClusterEnvironmentNamespaces[env], "", err)
				}
			}
		}
		configWriterDataResults <- data
	}
}

// UpdateShard reads the job object from the results channel and updates the original shard object with the proper result.
func UpdateShard(ctxLogger *log.Entry, results <-chan *ConfigWriterData, resultswg *sync.WaitGroup, shard *admiralapiv1.Shard) {
	defer resultswg.Done()
	for job := range results {
		ctxLogger.Infof(common.CtxLogFormat, "UpdateShard", shard.Name, "", job.ClusterName, job.Result)
		//ctxLogger.Infof(common.CtxLogFormat, "UpdateShard", shard.Name, "", job.ClusterName, shard.Status.Conditions[0].Message)
		//TODO: need to get updated shard crd spec and set status here
	}
}
