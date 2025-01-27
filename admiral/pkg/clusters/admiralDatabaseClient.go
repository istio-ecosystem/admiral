package clusters

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/apis/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
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
	return dynamicConfigDatabaseClient.dynamoClient.getDynamicConfig(env, identity, dynamicConfigDatabaseClient.database.TableName)
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
	return dynamicConfigDatabaseClient.dynamoClient.getDynamicConfig(env, identity, dynamicConfigDatabaseClient.database.TableName)
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

func UpdateASyncAdmiralConfig(dbClient AdmiralDatabaseManager, syncTime int) {

	for range time.Tick(time.Minute * time.Duration(syncTime)) {
		ReadAndUpdateSyncAdmiralConfig(dbClient)
	}
}

func ReadAndUpdateSyncAdmiralConfig(dbClient AdmiralDatabaseManager) error {

	dbRawData, err := dbClient.Get("EnableDynamicConfig", common.Admiral)
	if err != nil {
		log.Errorf("task=%s, Error getting EnableDynamicConfig admiral config, err: %v", common.DynamicConfigUpdate, err)
		return err
	}

	configData, ok := dbRawData.(DynamicConfigData)
	if !ok {
		return errors.New(fmt.Sprintf("task=%s, Failed to parse DynamicConfigData", common.DynamicConfigUpdate))
	}

	if IsDynamicConfigChanged(configData) {
		log.Infof(fmt.Sprintf("task=%s, Updating DynamicConfigData with Admiral config", common.DynamicConfigUpdate))
		UpdateSyncAdmiralConfig(configData)
	} else {
		log.Infof(fmt.Sprintf("task=%s, No need to update DynamicConfigData", common.DynamicConfigUpdate))
	}

	return nil
}

func IsDynamicConfigChanged(config DynamicConfigData) bool {

	if config.EnableDynamicConfig != common.Admiral {
		return false
	}

	if DynamicConfigCheckSum == sha256.Sum256([]byte(fmt.Sprintf("%v", config))) {
		return false
	} else {
		DynamicConfigCheckSum = sha256.Sum256([]byte(fmt.Sprintf("%v", config)))
		return true
	}
}

func UpdateSyncAdmiralConfig(configData DynamicConfigData) {
	if configData.EnableDynamicConfig == common.Admiral {
		//Fetch Existing config and update which are changed.
		newAdmiralConfig := common.GetAdmiralParams()
		if len(strings.Join(configData.NLBEnabledClusters, ",")) > 0 {
			newAdmiralConfig.NLBEnabledClusters = strings.Join(configData.NLBEnabledClusters, ",")
		}

		if len(strings.Join(configData.CLBEnabledClusters, ",")) > 0 {
			newAdmiralConfig.CLBEnabledClusters = strings.Join(configData.CLBEnabledClusters, ",")
		}

		if len(strings.Join(configData.NLBEnabledIdentityList, ",")) > 0 {
			newAdmiralConfig.NLBEnabledIdentityList = strings.Join(configData.NLBEnabledIdentityList, ",")
		}

		common.UpdateAdmiralParams(newAdmiralConfig)

	}
}
