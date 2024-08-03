package clusters

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/apis/v1"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

// TODO: make this more generic to handle new dynamoDB tables
type WorkloadDatabaseClient struct {
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
