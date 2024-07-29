package clusters

import (
	"context"
	"fmt"
	admiralapiv1 "github.com/istio-ecosystem/admiral-api/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
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
	var consumerwg, resultswg sync.WaitGroup
	configWriterData := make(chan *ConfigWriterData, 1000)
	configWriterDataResults := make(chan *ConfigWriterData, 1000)
	for i := 0; i < 5; i++ {
		consumerwg.Add(1)
		go ConsumeIdentityConfigs(ctxLogger, ctx, configWriterData, configWriterDataResults, remoteRegistry, &consumerwg)
	}
	// Get all ICs from shard and put into channel
	go ProduceIdentityConfigsFromShard(ctxLogger, *obj, configWriterData, remoteRegistry)
	// Start processing results
	resultswg.Add(1)
	go UpdateShard(ctxLogger, configWriterDataResults, &resultswg, tmpShard)
	// wait for all consumers to finish
	consumerwg.Wait()
	// all consumers done,no more values sent to results
	close(configWriterDataResults)
	// wait for all results to be processed
	resultswg.Wait()
	//TODO: Need to write the new tmpShard with all the results to the cluster + return error for the item to be requeued
	return nil
}

// ProduceIdentityConfigsFromShard creates a registry client and uses it to get the identity configs
// of the assets on the shard, and puts those into configWriterData which go into the job channel
func ProduceIdentityConfigsFromShard(ctxLogger *log.Entry, shard admiralapiv1.Shard, configWriterData chan<- *ConfigWriterData, rr *RemoteRegistry) {
	for _, clusterShard := range shard.Spec.Clusters {
		for _, identityItem := range clusterShard.Identities {
			identityConfig, err := rr.RegistryClient.GetIdentityConfigByIdentityName(identityItem.Name, ctxLogger)
			if err != nil {
				ctxLogger.Warnf(common.CtxLogFormat, "ProduceIdentityConfig", identityItem.Name, shard.Namespace, clusterShard.Name, err)
			}
			ctxLogger.Infof(common.CtxLogFormat, "ProduceIdentityConfig", identityConfig.IdentityName, shard.Namespace, clusterShard.Name, "successfully produced IdentityConfig")
			//TODO: Fill rr.AdmiralCache
			//1. IdentityDependencyCache (identityConfig.IdentityName -> clientAssets)
			//2. GlobalTrafficCache (id + env -> gtp)
			//3. OutlierDetectionCache (id + env -> od)
			//4. ClientConnectionConfigCache (id + env -> ccc)
			//5. ClusterLocalityCache (cluster -> cluster -> locality) (don't care about set functionality, only one locality per cluster)
			configWriterData <- &ConfigWriterData{
				IdentityConfig: &identityConfig,
				ClusterName:    clusterShard.Name,
				Error:          err,
			}
		}
	}
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
		//TODO: doesn't make much sense to have this as a struct, easier to just pass in the cluster and remote registry
		serviceEntryBuilder := ServiceEntryBuilder{ClientCluster: clientCluster, RemoteRegistry: rr}
		serviceEntries, err := serviceEntryBuilder.BuildServiceEntriesFromIdentityConfig(ctxLogger, *identityConfig)
		if err != nil {
			ctxLogger.Warnf(common.CtxLogFormat, "ConsumeIdentityConfig", assetName, "", clientCluster, err)
			data.Result = err.Error()
		}
		// service deployed in cluster 1 with 2 env qal, e2e, cluster 2 with 3 env qal, e2e, prod
		// write SEs to cluster 1
		// env -> list of cluster
		// env -> se
		// check if any of the clusters are a source cluster -> rethink this, won't work if one env is on a cluster but not on another
		//isServiceEntryModifyCalledForSourceCluster := false
		//for _, cluster := range identityConfig.Clusters {
		//	if cluster.Name == clientCluster {
		//		isServiceEntryModifyCalledForSourceCluster = true
		//		break
		//	}
		//}
		//ctx = context.WithValue(ctx, common.EventResourceType, identityConfig.Clusters[0].Environment[0].Type)
		for _, se := range serviceEntries {
			//clusters := make(chan string, 1)
			//errors := make(chan error, 1)
			//clusters <- clientCluster
			//AddServiceEntriesWithDrWorker(ctxLogger, ctx, rr,
			//	true, //doGenerateAdditionalEndpoints()
			//	isServiceEntryModifyCalledForSourceCluster,
			//	assetName,
			//	strings.Split(se.Hosts[0], common.Sep)[0],
			//	se,
			//	clusters,
			//	errors)
			rc := rr.GetRemoteController(clientCluster)
			seName := strings.ToLower(se.Hosts[0]) + "-se"
			sec := rc.ServiceEntryController
			//TODO: se reconciliation cache
			oldServiceEntry := sec.Cache.Get(seName, clientCluster)
			if oldServiceEntry == nil {
				ctxLogger.Infof(common.CtxLogFormat, "ConsumeIdentityConfig", seName, "", clientCluster, "starting to write se to cluster")
				oldServiceEntry, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(common.GetOperatorSyncNamespace()).Get(ctx, seName, metav1.GetOptions{})
				// if old service entry not find, just create a new service entry instead
				if err != nil && k8sErrors.IsNotFound(err) {
					ctxLogger.Infof(common.CtxLogFormat, "ConsumeIdentityConfig", seName, "", clientCluster, fmt.Sprintf("failed fetching old service entry, error=%v", err))
					oldServiceEntry = nil
				}
			}
			newServiceEntry := createServiceEntrySkeleton(*se, seName, common.GetOperatorSyncNamespace())
			err = addUpdateServiceEntry(ctxLogger, ctx, newServiceEntry, oldServiceEntry, common.GetOperatorSyncNamespace(), rc)
			if err != nil {
				ctxLogger.Warnf(common.CtxLogFormat, "ConsumeIdentityConfig", seName, "", clientCluster, err)
				data.Result = err.Error()
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
