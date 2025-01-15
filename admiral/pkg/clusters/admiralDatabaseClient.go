package clusters

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/apis/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
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

func (dynamicConfigDatabaseClient *DynamicConfigDatabaseClient) Update(data interface{}, ctxLogger *log.Entry) error {
	//workloadData := data.(WorkloadData)
	//
	//err := checkIfDatabaseClientIsInitialize(workloadDatabaseClient)
	//if err != nil {
	//	return err
	//}
	//
	//return workloadDatabaseClient.dynamoClient.updateWorkloadDataItem(&workloadData, workloadDatabaseClient.database.TableName, ctxLogger)
	panic("Implement me!")
}

func (dynamicConfigDatabaseClient *DynamicConfigDatabaseClient) Get(env, identity string) (interface{}, error) {

	//err := checkIfDatabaseClientIsInitialize(dynamicConfigDatabaseClient)
	//if err != nil {
	//	return nil, err
	//}
	//
	return dynamicConfigDatabaseClient.dynamoClient.getDynamicConfig(dynamicConfigDatabaseClient.database.TableName)
}

func (dynamicConfigDatabaseClient *DynamicConfigDatabaseClient) Delete(data interface{}, ctxLogger *log.Entry) error {
	//workloadData := data.(WorkloadData)
	//
	//err := checkIfDatabaseClientIsInitialize(workloadDatabaseClient)
	//if err != nil {
	//	return err
	//}
	//return workloadDatabaseClient.dynamoClient.deleteWorkloadDataItem(&workloadData, workloadDatabaseClient.database.TableName)
	panic("Implement me!")
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
	var (
		workloadDatabaseClient = &WorkloadDatabaseClient{}
		admiralConfig          *v1.AdmiralConfig
	)

	data, err := ioutil.ReadFile(admiralConfigPath)
	if err != nil {
		return nil, fmt.Errorf("error reading admiral config file, err: %v", err)
	}

	err = yaml.Unmarshal(data, &admiralConfig)
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

func NewDynamicConfigDatabaseClient(path string, dynamoClientInitFunc func(role string, region string) (*DynamoClient, error)) (*DynamicConfigDatabaseClient, error) {
	var (
		admiralConfig       *v1.AdmiralConfig
		dynamicConfigClient = &DynamicConfigDatabaseClient{}
	)

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading admiral config file for DynamicConfig, err: %v", err)
	}

	err = yaml.Unmarshal(data, &admiralConfig)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling admiral config file for DynamicConfig, err: %v", err)
	}

	dynamicConfigClient.database = &admiralConfig.DynamicConfigDatabase
	dynamicConfigClient.database.TableName = common.GetAdmiralParams().DynamicConfigDynamoDBTableName
	dynamicConfigClient.dynamoClient, err = dynamoClientInitFunc(
		admiralConfig.WorkloadDatabase.Role,
		admiralConfig.WorkloadDatabase.Region,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate dynamo client for DynamicConfig, err: %v", err)
	}
	return dynamicConfigClient, nil
}

func UpdateASyncAdmiralConfig(dbClient AdmiralDatabaseManager, syncTime int) {

	for range time.Tick(time.Minute * time.Duration(syncTime)) {
		UpdateSyncAdmiralConfig(dbClient)
	}
}

func UpdateSyncAdmiralConfig(dbClient AdmiralDatabaseManager) {
	dbClient.Get("", "")
}
