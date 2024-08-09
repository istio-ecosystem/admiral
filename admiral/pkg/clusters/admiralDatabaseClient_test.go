package clusters

/*
import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/istio-ecosystem/admiral/admiral/apis/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewAdmiralDatabaseClient(t *testing.T) {

	var dummyDynamoClientFunc = func(role, region string) (*DynamoClient, error) {
		return nil, nil
	}

	var dummyDynamoClientFuncWithError = func(role, region string) (*DynamoClient, error) {
		return nil, fmt.Errorf("failed to initialize client")
	}

	testCases := []struct {
		name              string
		admiralConfigPath string
		dynamoClientFunc  func(role, region string) (*DynamoClient, error)
		expectedErr       error
	}{
		{
			name: "Given admiral config contains all required configurations, " +
				"When NewAdmiralDatabseClient is called, " +
				"Then it should initialize admiralDatabaseClient and not return any error",
			admiralConfigPath: "testdata/admiralDatabaseClientConfig_is_valid.yaml",
			dynamoClientFunc:  dummyDynamoClientFunc,
			expectedErr:       nil,
		},
		{
			name: "Given admiral config does not contain valid configurations, " +
				"When NewAdmiralDatabseClient is called, " +
				"Then it should not initialize admiralDatabaseClient and return error",
			admiralConfigPath: "testdata/admiralDatabaseClientConfig_is_not_valid.yaml",
			dynamoClientFunc:  dummyDynamoClientFunc,
			expectedErr:       fmt.Errorf("error unmarshalling admiral config file, err: yaml: line 20: mapping values are not allowed in this context"),
		},
		{
			name: "Given admiral config contains all required configurations but fails to create dynamodb client, " +
				"When NewAdmiralDatabseClient is called, " +
				"Then it should not initialize admiralDatabaseClient and return error",
			admiralConfigPath: "testdata/admiralDatabaseClientConfig_is_valid.yaml",
			dynamoClientFunc:  dummyDynamoClientFuncWithError,
			expectedErr:       fmt.Errorf("unable to instantiate dynamo client, err: failed to initialize client"),
		},
		{
			name: "Given incorrect admiral config path, " +
				"When NewAdmiralDatabseClient is called, " +
				"Then it should not initialize admiralDatabaseClient and return error",
			admiralConfigPath: "testdata/admiralDatabaseClientConfig_is_invalid.yaml",
			dynamoClientFunc:  dummyDynamoClientFuncWithError,
			expectedErr:       fmt.Errorf("error reading admiral config file, err: open testdata/admiralDatabaseClientConfig_is_invalid.yaml: no such file or directory"),
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewAdmiralDatabaseClient(c.admiralConfigPath, c.dynamoClientFunc)
			if c.expectedErr != nil {
				assert.EqualError(t, err, c.expectedErr.Error())
			} else {
				if err != c.expectedErr {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
			}
		})
	}
}

func TestUpdateWorkloadData(t *testing.T) {

	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})

	var DynamodbClient = DynamoClient{
		&mockDynamoDBClient{},
	}

	var databaseClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: &DynamodbClient,
	}

	var databaseClientWithNilDynamoClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: nil,
	}

	var databaseClientWithNilDatabase = WorkloadDatabaseClient{
		database:     nil,
		dynamoClient: &DynamodbClient,
	}

	var DynamodbClientWithError = DynamoClient{
		&mockDynamoDBClientWithErrors{},
	}

	var databaseClientWithError = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: &DynamodbClientWithError,
	}

	var workloadDataWithoutGTP = WorkloadData{
		AssetAlias: "custom",
		Endpoint:   "dev.custom.global",
		Env:        "dev",
		Aliases:    []string{"dev.custom.testsuffix"},
	}

	var workloadDataWithFailoverGTP = WorkloadData{
		AssetAlias: "custom",
		Endpoint:   "dev.custom.global",
		Env:        "dev",
		Aliases:    []string{"dev.custom.testsuffix"},
		LbType:     model.TrafficPolicy_FAILOVER.String(),
		TrafficDistribution: map[string]int32{
			"us-west-2": 100,
		},
	}

	var workloadDataWithTopologyGTP = WorkloadData{
		AssetAlias: "custom",
		Endpoint:   "dev.custom.global",
		Env:        "dev",
		Aliases:    []string{"dev.custom.testsuffix"},
		LbType:     model.TrafficPolicy_TOPOLOGY.String(),
	}

	testCases := []struct {
		name           string
		workloadData   WorkloadData
		databaseClient WorkloadDatabaseClient
		expectedErr    error
	}{
		{
			name: "Given workload object and no globaltrafficpolicy configuration, " +
				"When UpdateWorkloadData is called, " +
				"Then it should not return any error",
			workloadData:   workloadDataWithoutGTP,
			databaseClient: databaseClient,
			expectedErr:    nil,
		},
		{
			name: "Given workload object with failover globaltrafficpolicy configuration, " +
				"When UpdateWorkloadData is called, " +
				"Then it should not return any error",
			workloadData:   workloadDataWithFailoverGTP,
			databaseClient: databaseClient,
			expectedErr:    nil,
		},
		{
			name: "Given workload object with topology globaltrafficpolicy configuration, " +
				"When UpdateWorkloadData is called, " +
				"Then it should not return any error",
			workloadData:   workloadDataWithTopologyGTP,
			databaseClient: databaseClient,
			expectedErr:    nil,
		},
		{
			name: "Given workload object with failover globaltrafficpolicy configuration and error occurs using dynamodb client " +
				"When UpdateWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadDataWithFailoverGTP,
			databaseClient: databaseClientWithError,
			expectedErr:    fmt.Errorf("error occurred adding record to dynamodb"),
		},
		{
			name: "Given dynamodb client is nil" +
				"When UpdateWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadDataWithFailoverGTP,
			databaseClient: databaseClientWithNilDynamoClient,
			expectedErr:    fmt.Errorf("dynamoClient is not initialized"),
		},
		{
			name: "Given database is nil" +
				"When UpdateWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadDataWithFailoverGTP,
			databaseClient: databaseClientWithNilDatabase,
			expectedErr:    fmt.Errorf("database is not initialized"),
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := c.databaseClient.Update(c.workloadData, ctxLogger)
			if c.expectedErr != nil {
				assert.EqualError(t, err, c.expectedErr.Error())
			} else {
				if err != c.expectedErr {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
			}
		})
	}
}

type mockDynamoDBClientForGet struct {
	dynamodbiface.DynamoDBAPI
}

func (m *mockDynamoDBClientForGet) Query(queryInput *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
	switch *queryInput.TableName {
	case workloadDataTableName:
		return &dynamodb.QueryOutput{
			Items: []map[string]*dynamodb.AttributeValue{
				{
					"assetAlias": {S: aws.String("intuit.test.asset")},
					"env":        {S: aws.String("testEnv")},
					"dnsPrefix":  {S: aws.String("west")},
					"lbType":     {S: aws.String("FAILOVER")},
					"trafficDistribution": {M: map[string]*dynamodb.AttributeValue{
						"us-west-2": {N: aws.String("100")},
						"us-east-2": {N: aws.String("0")},
					}},
					"gtpManagedBy": {S: aws.String("github")},
				},
			},
		}, nil
	default:
		return nil, nil
	}
}

func (m *mockDynamoDBClientForGet) Scan(input *dynamodb.ScanInput) (*dynamodb.ScanOutput, error) {
	return nil, nil
}

func (m *mockDynamoDBClientForGet) GetItem(input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
	return nil, nil
}

func (m *mockDynamoDBClientForGet) PutItem(input *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
	return &dynamodb.PutItemOutput{}, nil
}

func (m *mockDynamoDBClientForGet) DeleteItem(input *dynamodb.DeleteItemInput) (*dynamodb.DeleteItemOutput, error) {
	return &dynamodb.DeleteItemOutput{}, nil
}

func TestGetWorkloadDataOnDatabaseClient(t *testing.T) {

	var DynamodbClient = DynamoClient{
		&mockDynamoDBClientForGet{},
	}

	var databaseClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: &DynamodbClient,
	}

	var databaseClientWithNilDynamoClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: nil,
	}

	var databaseClientWithNilDatabase = WorkloadDatabaseClient{
		database:     nil,
		dynamoClient: &DynamodbClient,
	}

	testCases := []struct {
		name                  string
		identity              string
		env                   string
		workloadDataItems     []WorkloadData
		databaseClient        WorkloadDatabaseClient
		expectedErr           error
		expectedNumberOfItems int
	}{
		{
			name: "Given identity and env parameter, " +
				"And, client is able to query the given table, " +
				"And, one of the items has identity set to 'intuit.test.asset'," +
				"When Get is called, " +
				"Then, it should return expected items without any error",
			identity:              "intuit.test.asset",
			env:                   "testEnv",
			databaseClient:        databaseClient,
			expectedNumberOfItems: 1,
			expectedErr:           nil,
		},
		{
			name: "Given dynamodb client is nil" +
				"When Get is called on databaseClient, " +
				"Then it should return error",
			databaseClient: databaseClientWithNilDynamoClient,
			expectedErr:    fmt.Errorf("dynamoClient is not initialized"),
		},
		{
			name: "Given database is nil" +
				"When Get is called on databaseClient, " +
				"Then it should return error",
			databaseClient: databaseClientWithNilDatabase,
			expectedErr:    fmt.Errorf("database is not initialized"),
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.databaseClient.Get(c.identity, c.env)
			if c.expectedErr != nil {
				assert.EqualError(t, err, c.expectedErr.Error())
			} else {
				if err != c.expectedErr {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
			}
		})
	}
}

func TestDeleteWorkloadData(t *testing.T) {

	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})

	var DynamodbClient = DynamoClient{
		&mockDynamoDBClient{},
	}

	var databaseClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: &DynamodbClient,
	}

	var DynamodbClientWithError = DynamoClient{
		&mockDynamoDBClientWithErrors{},
	}

	var databaseClientWithNilDynamoClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: nil,
	}

	var databaseClientWithNilDatabase = WorkloadDatabaseClient{
		database:     nil,
		dynamoClient: &DynamodbClient,
	}

	var databaseClientWithError = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: &DynamodbClientWithError,
	}

	var workloadData = WorkloadData{
		Endpoint:   "dev.custom.global",
		AssetAlias: "custom",
	}

	testCases := []struct {
		name           string
		workloadData   WorkloadData
		databaseClient WorkloadDatabaseClient
		expectedErr    error
	}{
		{
			name: "Given workload object, " +
				"When deleteWorkloadData is called, " +
				"Then it should not return any error",
			workloadData:   workloadData,
			databaseClient: databaseClient,
			expectedErr:    nil,
		},
		{
			name: "Given worklaod object,and error occurs using dynamodb client " +
				"When deleteWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadData,
			databaseClient: databaseClientWithError,
			expectedErr:    fmt.Errorf("error occurred deleting the item"),
		},
		{
			name: "Given dynamodb client is nil" +
				"When deleteWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadData,
			databaseClient: databaseClientWithNilDynamoClient,
			expectedErr:    fmt.Errorf("dynamoClient is not initialized"),
		},
		{
			name: "Given database is nil" +
				"When deleteWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadData,
			databaseClient: databaseClientWithNilDatabase,
			expectedErr:    fmt.Errorf("database is not initialized"),
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := c.databaseClient.Delete(c.workloadData, ctxLogger)
			if c.expectedErr != nil {
				assert.EqualError(t, err, c.expectedErr.Error())
			} else {
				if err != c.expectedErr {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
			}
		})
	}
}
*/
