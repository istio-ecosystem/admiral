package clusters

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/apis/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"slices"
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

func UpdateASyncAdmiralConfig(rr *RemoteRegistry, syncTime int) {

	ctxDynamicConfig, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	if IsDynamicConfigChanged(configData) {
		log.Infof(fmt.Sprintf("task=%s, updating DynamicConfigData with Admiral config", common.DynamicConfigUpdate))
		UpdateSyncAdmiralConfig(configData)

		ctx := context.Context(context.Background())
		//Process NLB Cluster
		processLBMigration(ctx, rr, common.GetAdmiralParams().NLBEnabledClusters, &rr.AdmiralCache.NLBEnabledCluster, common.GetAdmiralParams().NLBIngressLabel)
		//Process CLB Cluster
		processLBMigration(ctx, rr, common.GetAdmiralParams().CLBEnabledClusters, &rr.AdmiralCache.CLBEnabledCluster, common.GetAdmiralParams().LabelSet.GatewayApp)
	} else {
		log.Infof(fmt.Sprintf("task=%s, no need to update DynamicConfigData", common.DynamicConfigUpdate))
	}

	return nil
}

func processLBMigration(ctx context.Context, rr *RemoteRegistry, updatedLBs []string, existingCache *[]string, lbLabel string) {

	log.Infof("task=%s, Processing LB migration for %s. UpdateReceived=%s, ExistingCache=%s, ", common.LBUpdateProcessor, lbLabel, updatedLBs, existingCache)

	for _, cluster := range getLBToProcess(updatedLBs, existingCache) {
		err := isServiceControllerInitialized(rr.remoteControllers[cluster])
		if err == nil {
			for _, fetchService := range rr.remoteControllers[cluster].ServiceController.Cache.Get(common.NamespaceIstioSystem) {
				if fetchService.Labels[common.App] == lbLabel {
					log.Infof("task=%s, Cluster=%s, Processing LB migration for Cluster.", common.LBUpdateProcessor, cluster)
					go handleServiceEventForDeployment(ctx, fetchService, rr, cluster, rr.GetRemoteController(cluster).DeploymentController, rr.GetRemoteController(cluster).ServiceController, HandleEventForDeployment)
					go handleServiceEventForRollout(ctx, fetchService, rr, cluster, rr.GetRemoteController(cluster).RolloutController, rr.GetRemoteController(cluster).ServiceController, HandleEventForRollout)
				}
			}
		} else {
			log.Infof("task=%s, Cluster=%s, Service Controller not initializ. Skipped LB migration for Cluster.", common.LBUpdateProcessor, cluster)
		}
	}
}

func getLBToProcess(updatedLB []string, cache *[]string) []string {
	var clusersToProcess []string
	if cache == nil || len(*cache) == 0 {
		*cache = updatedLB
		return updatedLB
	}
	//Validate if New ClusterAdded
	for _, clusterFromAdmiralParam := range updatedLB {
		if !slices.Contains(*cache, clusterFromAdmiralParam) {
			clusersToProcess = append(clusersToProcess, clusterFromAdmiralParam)
		}
	}

	//Validate if cluster Removed
	for _, clusterFromCache := range *cache {
		if !slices.Contains(updatedLB, clusterFromCache) {
			clusersToProcess = append(clusersToProcess, clusterFromCache)
		}
	}

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

		common.UpdateAdmiralParams(newAdmiralConfig)

	}
}
