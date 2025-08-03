package clusters

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/istio-ecosystem/admiral/admiral/apis/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"slices"
	"strings"
	"time"
)

// TODO: make this more generic to handle new dynamoDB tables
type WorkloadDatabaseClient struct {
	dynamoClient *DynamoClient
	database     *v1.DynamoDB
}

type DynamicConfigDatabaseClient struct {
	dynamoClient *DynamoClient
	database     *v1.DynamoDB
}

type DummyDatabaseClient struct{}

type AdmiralDatabaseManager interface {
	Update(data interface{}, logger *log.Entry) error
	Delete(data interface{}, logger *log.Entry) error
	Get(env, identity string) (interface{}, error)
}

func (workloadDatabaseClient *WorkloadDatabaseClient) Update(data interface{}, ctxLogger *log.Entry) error {
	workloadData := data.(WorkloadData)

	err := checkIfDatabaseClientIsInitialize(workloadDatabaseClient)
	if err != nil {
		return err
	}

	return workloadDatabaseClient.dynamoClient.updateWorkloadDataItem(&workloadData, workloadDatabaseClient.database.TableName, ctxLogger)
}

func (workloadDatabaseClient *WorkloadDatabaseClient) Get(env, identity string) (interface{}, error) {

	err := checkIfDatabaseClientIsInitialize(workloadDatabaseClient)
	if err != nil {
		return nil, err
	}

	return workloadDatabaseClient.dynamoClient.getWorkloadDataItemByIdentityAndEnv(env, identity, workloadDatabaseClient.database.TableName)
}

func (workloadDatabaseClient *WorkloadDatabaseClient) Delete(data interface{}, ctxLogger *log.Entry) error {
	workloadData := data.(WorkloadData)

	err := checkIfDatabaseClientIsInitialize(workloadDatabaseClient)
	if err != nil {
		return err
	}
	return workloadDatabaseClient.dynamoClient.deleteWorkloadDataItem(&workloadData, workloadDatabaseClient.database.TableName)
}

func checkIfDatabaseClientIsInitialize(workloadDatabaseClient *WorkloadDatabaseClient) error {
	if workloadDatabaseClient == nil || workloadDatabaseClient.dynamoClient == nil {
		return fmt.Errorf("dynamoClient is not initialized")
	}

	if workloadDatabaseClient.database == nil {
		return fmt.Errorf("database is not initialized")
	}

	return nil
}

func (dynamicConfigDatabaseClient *DynamicConfigDatabaseClient) Update(data interface{}, logger *log.Entry) error {
	//TODO implement me
	//At point of release there is no plan to support push config to dyanmic config storage
	panic("implement me")
}

func (dynamicConfigDatabaseClient *DynamicConfigDatabaseClient) Delete(data interface{}, logger *log.Entry) error {
	//TODO implement me
	//At point of release there is no plan to support delete config to dyanmic config storage
	panic("implement me")
}

func (dynamicConfigDatabaseClient *DynamicConfigDatabaseClient) Get(env, identity string) (interface{}, error) {
	//Variable renaming is done to re-purpose existing interface
	err := checkIfDynamicConfigDatabaseClientIsInitialized(dynamicConfigDatabaseClient)

	if err != nil {
		return nil, err
	}

	return dynamicConfigDatabaseClient.dynamoClient.getDynamicConfig(env, identity, dynamicConfigDatabaseClient.database.TableName)
}

func checkIfDynamicConfigDatabaseClientIsInitialized(dynamicConfigDatabaseClient *DynamicConfigDatabaseClient) error {
	if dynamicConfigDatabaseClient == nil || dynamicConfigDatabaseClient.dynamoClient == nil {
		return fmt.Errorf("task=%s, dynamoClient is not initialized", common.DynamicConfigUpdate)
	}

	if dynamicConfigDatabaseClient.database == nil {
		return fmt.Errorf("task=%s, database is not initialized", common.DynamicConfigUpdate)
	}

	return nil
}

func (databaseClient *DummyDatabaseClient) Update(data interface{}, logger *log.Entry) error {
	return nil
}

func (databaseClient *DummyDatabaseClient) Delete(data interface{}, logger *log.Entry) error {
	return nil
}

func (databaseClient *DummyDatabaseClient) Get(env, identity string) (interface{}, error) {
	return nil, nil
}

func NewAdmiralDatabaseClient(admiralConfigPath string, dynamoClientInitFunc func(string, string) (*DynamoClient, error)) (*WorkloadDatabaseClient, error) {
	var workloadDatabaseClient = &WorkloadDatabaseClient{}

	admiralConfig, err := ReadDynamoConfigForDynamoDB(admiralConfigPath)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling admiral config file, err: %v", err)
	}

	workloadDatabaseClient.database = &admiralConfig.WorkloadDatabase
	workloadDatabaseClient.dynamoClient, err = dynamoClientInitFunc(
		admiralConfig.WorkloadDatabase.Role,
		admiralConfig.WorkloadDatabase.Region,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate dynamo client, err: %v", err)
	}
	return workloadDatabaseClient, nil
}

func ReadDynamoConfigForDynamoDB(path string) (*v1.AdmiralConfig, error) {
	var admiralConfig *v1.AdmiralConfig

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading admiral config file, err: %v", err)
	}

	err = yaml.Unmarshal(data, &admiralConfig)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling admiral config file, err: %v", err)
	}

	return admiralConfig, nil
}

func NewDynamicConfigDatabaseClient(path string, dynamoClientInitFunc func(role string, region string) (*DynamoClient, error)) (*DynamicConfigDatabaseClient, error) {
	var dynamicConfigClient = DynamicConfigDatabaseClient{}

	admiralConfig, err := ReadDynamoConfigForDynamoDB(path)
	if err != nil {
		return nil, fmt.Errorf("task=%v, error unmarshalling admiral config file, err: %v", common.DynamicConfigUpdate, err)
	}

	dynamicConfigClient.database = &admiralConfig.DynamicConfigDatabase
	dynamicConfigClient.database.TableName = common.GetAdmiralParams().DynamicConfigDynamoDBTableName
	dynamicConfigClient.dynamoClient, err = dynamoClientInitFunc(
		admiralConfig.WorkloadDatabase.Role,
		admiralConfig.WorkloadDatabase.Region,
	)
	if err != nil {
		return &dynamicConfigClient, fmt.Errorf("task=%v, unable to instantiate dynamo client for DynamicConfig, err: %v", common.DynamicConfigUpdate, err)
	}
	return &dynamicConfigClient, nil
}

func UpdateASyncAdmiralConfig(ctxDynamicConfig context.Context, rr *RemoteRegistry, syncTime int) {
	//pull once on startup
	ReadAndUpdateSyncAdmiralConfig(rr)
	ticker := time.NewTicker(time.Minute * time.Duration(syncTime))
	defer ticker.Stop()

	for {
		select {
		case <-ctxDynamicConfig.Done():
			log.Infof("task=%v, context done stopping ticker", common.DynamicConfigUpdate)
			return
		case <-ticker.C:
			ReadAndUpdateSyncAdmiralConfig(rr)
		}
	}
}

func ReadAndUpdateSyncAdmiralConfig(rr *RemoteRegistry) error {
	dbRawData, err := rr.DynamicConfigDatabaseClient.Get("EnableDynamicConfig", common.Admiral)
	if err != nil {
		log.Errorf("task=%s, error getting EnableDynamicConfig admiral config, err: %v", common.DynamicConfigUpdate, err)
		return err
	}

	configData, ok := dbRawData.(DynamicConfigData)
	if !ok {
		return errors.New(fmt.Sprintf("task=%s, failed to parse DynamicConfigData", common.DynamicConfigUpdate))
	}
	UpdateSyncAdmiralConfig(configData)
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(common.CtxLogFormat, "DynamicConfig", "", "", "", "processing skipped as Admiral is in Read-only mode")
		return nil
	}
	if IsCacheWarmupTime(rr) {
		log.Infof("task=%s, NeedToUpdateConfig=false, processing skipped during cache warm up state", common.DynamicConfigUpdate)
		return nil
	}

	if IsDynamicConfigChanged(configData) {
		log.Infof(fmt.Sprintf("task=%s, NeedToUpdateConfig=true", common.DynamicConfigUpdate))
		ctx := context.Background()
		//Process NLB Cluster
		processLBMigration(ctx, rr, common.GetAdmiralParams().NLBEnabledClusters, &rr.AdmiralCache.NLBEnabledCluster, common.GetAdmiralParams().NLBIngressLabel)
		//Process CLB Cluster
		processLBMigration(ctx, rr, common.GetAdmiralParams().CLBEnabledClusters, &rr.AdmiralCache.CLBEnabledCluster, common.GetAdmiralParams().CLBIngressLabel)
		// Process InitiateClientInitiatedProcessingFor
		var c ClientDependencyRecordProcessor
		err := triggerClientInitiatedProcessing(ctx, c, rr, common.GetInitiateClientInitiatedProcessingFor())
		if err != nil {
			log.Errorf(fmt.Sprintf("task=%s, Error=%v", common.DynamicConfigUpdate, err))
		}
	} else {
		log.Infof(fmt.Sprintf("task=%s, NeedToUpdateConfig=false", common.DynamicConfigUpdate))
	}

	return nil
}

func triggerClientInitiatedProcessing(ctx context.Context, c ProcessClientDependencyRecord, remoteRegistry *RemoteRegistry, processingForMap map[string]string) error {
	if !common.ClientInitiatedProcessingEnabledForDynamicConfig() {
		return nil
	}

	txId := uuid.NewString()
	ctxLogger := log.WithField("task", common.ClientInitiatedProcessing)
	ctx = context.WithValue(ctx, "txId", txId)
	bypassPrefix := "bypass:"

	var processedGlobalIdentifiers []string
	for _, globalIdentifier := range processingForMap {
		var err error
		var bypass bool
		if strings.HasPrefix(globalIdentifier, bypassPrefix) {
			globalIdentifier = strings.TrimPrefix(globalIdentifier, bypassPrefix)
			bypass = true
		}
		clusterNamespaces := remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Get(globalIdentifier)
		if clusterNamespaces == nil {
			return nil
		}
		clusterNamespaces.Range(func(cluster string, nsMap *common.Map) {
			if nsMap == nil {
				return
			}
			nsMap.Range(func(ns string, v string) {
				ctxLogger.Infof(LogFormat, "DynamicConfig", "", ns, cluster, "Client initiated processing started for "+globalIdentifier+" txId="+txId)
				processingErr := c.processClientDependencyRecord(ctx, remoteRegistry, globalIdentifier, cluster, ns, bypass)

				if processingErr != nil {
					ctxLogger.Errorf(common.CtxLogFormat, common.ClientInitiatedProcessing, globalIdentifier, ns, cluster, fmt.Errorf("Client initiated processing error for identity=%s txId=%s, got error=%v", globalIdentifier, txId, processingErr))
				}
				err = common.AppendError(processingErr, err)
			})
		})
		if err == nil {
			processedGlobalIdentifiers = append(processedGlobalIdentifiers, globalIdentifier)
			ctxLogger.Infof(LogFormat, "DynamicConfig", "", "", "", "Client initiated processing completed for "+globalIdentifier+" txId="+txId)
		}
	}

	//Remove processed global identifiers
	unprocessedCount := removeProcessedIdentities(processedGlobalIdentifiers, processingForMap)
	if unprocessedCount > 0 {
		return fmt.Errorf("Failed to process %v identities as a client", unprocessedCount)
	}
	return nil
}

func removeProcessedIdentities(processedGlobalIdentifiers []string, processingForMap map[string]string) int {
	for _, globalIdentifier := range processedGlobalIdentifiers {
		delete(processingForMap, globalIdentifier)
	}
	var unprocessed []string
	for k := range processingForMap {
		unprocessed = append(unprocessed, k)
	}
	currentParams := common.GetAdmiralParams()
	currentParams.InitiateClientInitiatedProcessingFor = unprocessed
	common.UpdateAdmiralParams(currentParams)
	return len(unprocessed)
}

func processLBMigration(ctx context.Context, rr *RemoteRegistry, updatedLBs []string, existingCache *[]string, lbLabel string) {

	ctxLogger := log.WithField("task", common.LBUpdateProcessor)

	clusterToProcess := getLBToProcess(updatedLBs, existingCache)
	ctxLogger.Infof("ClusterToProccess=%s, LBLabel=%s", clusterToProcess, lbLabel)
	for _, cluster := range clusterToProcess {
		//Cover NLB Usecase for asset migration by cluster
		if len(strings.Split(cluster, ":")) == 2 {
			cluster = strings.Split(cluster, ":")[1]
		}
		err := isServiceControllerInitialized(rr.remoteControllers[cluster])
		if err == nil {
			for _, fetchService := range rr.remoteControllers[cluster].ServiceController.Cache.Get(common.NamespaceIstioSystem) {
				if fetchService.Labels[common.App] == lbLabel {
					start := time.Now()
					ctxLogger.Infof("Cluster=%s, Processing LB migration for Cluster.", cluster)

					//Trigger Service event explicitly for migration
					ctx = context.WithValue(ctx, common.EventType, admiral.Update)
					err := handleEventForService(ctx, fetchService, rr, cluster)
					if err != nil {
						util.LogElapsedTimeSinceTask(ctxLogger, common.LBUpdateProcessor,
							lbLabel, "", cluster, "Error="+err.Error(), start)
					} else {
						util.LogElapsedTimeSinceTask(ctxLogger, common.LBUpdateProcessor,
							lbLabel, "", cluster, "Completed", start)
					}
				}
			}
		} else {
			ctxLogger.Infof("Cluster=%s, Service Controller not initialize. Skipped LB migration for Cluster. Err: %s", cluster, err.Error())
		}
	}
}

func getLBToProcess(updatedLB []string, cache *[]string) []string {

	clusersToProcess := make([]string, 0)
	if cache == nil || len(*cache) == 0 {
		*cache = updatedLB
		return updatedLB
	}

	if len(updatedLB) == 0 {
		clusersToProcess = *cache
		*cache = updatedLB
		return clusersToProcess
	}
	//Validate if New ClusterAdded
	for _, clusterFromAdmiralParam := range updatedLB {
		if !slices.Contains(*cache, clusterFromAdmiralParam) {
			clusersToProcess = append(clusersToProcess, clusterFromAdmiralParam)
			*cache = append(*cache, clusterFromAdmiralParam)
		}
	}

	//Validate if cluster Removed
	for _, clusterFromCache := range *cache {
		if !slices.Contains(updatedLB, clusterFromCache) {
			clusersToProcess = append(clusersToProcess, clusterFromCache)
		}
	}
	//Final dynamoDB data as cache
	*cache = updatedLB
	return clusersToProcess
}

func IsDynamicConfigChanged(config DynamicConfigData) bool {

	if config.EnableDynamicConfig != common.Admiral {
		return false
	}

	if DynamicConfigCheckSum == md5.Sum([]byte(fmt.Sprintf("%v", config))) {
		return false
	} else {
		DynamicConfigCheckSum = md5.Sum([]byte(fmt.Sprintf("%v", config)))
		return true
	}
}

func UpdateSyncAdmiralConfig(configData DynamicConfigData) {
	if configData.EnableDynamicConfig == common.Admiral {
		//Fetch Existing config and update which are changed.
		newAdmiralConfig := common.GetAdmiralParams()
		if len(configData.NLBEnabledClusters) > 0 {
			newAdmiralConfig.NLBEnabledClusters = configData.NLBEnabledClusters
		}

		if len(configData.CLBEnabledClusters) > 0 {
			newAdmiralConfig.CLBEnabledClusters = configData.CLBEnabledClusters
		}

		if len(configData.NLBEnabledIdentityList) > 0 {
			newAdmiralConfig.NLBEnabledIdentityList = configData.NLBEnabledIdentityList
		}

		if len(configData.InitiateClientInitiatedProcessingFor) > 0 {
			newAdmiralConfig.InitiateClientInitiatedProcessingFor = configData.InitiateClientInitiatedProcessingFor
		}
		common.UpdateAdmiralParams(newAdmiralConfig)
	}
}
