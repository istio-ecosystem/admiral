package clusters

// admiralReadWriteLeaseStateChecker_test.go

import (
	"context"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/stretchr/testify/assert"
)

type mockDynamoClient struct {
	dynamodbiface.DynamoDBAPI
}

func (m *mockDynamoClient) Scan(input *dynamodb.ScanInput) (*dynamodb.ScanOutput, error) {
	return &dynamodb.ScanOutput{
		Items: []map[string]*dynamodb.AttributeValue{
			{"leaseName": {S: aws.String("testLease1")}, "leaseOwner": {S: aws.String("testPod")}, "notes": {S: aws.String("test1")}, "updatedTime": {N: aws.String("1655875287")}},
			{"leaseName": {S: aws.String("testLease2")}, "leaseOwner": {S: aws.String("someotherPod")}, "notes": {S: aws.String("test2")}, "updatedTime": {N: aws.String("9999999999")}},
			{"leaseName": {S: aws.String("testLease3")}, "leaseOwner": {S: aws.String("someOtherPod")}, "notes": {S: aws.String("test3")}, "updatedTime": {N: aws.String("11111")}},
			{"leaseName": {S: aws.String("skipLease")}, "leaseOwner": {S: aws.String("SKIP-LEASE-POD")}, "notes": {S: aws.String("test3")}, "updatedTime": {N: aws.String("11111")}},
		},
	}, nil
}

func (m *mockDynamoClient) GetItem(input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
	output := &dynamodb.GetItemOutput{
		Item: map[string]*dynamodb.AttributeValue{"accountNumber": {S: aws.String("123123123")}, "roleName": {S: aws.String("PowerUser")}},
	}
	return output, nil
}

func (m *mockDynamoClient) PutItem(input *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
	return &dynamodb.PutItemOutput{}, nil
}

func setupMockContext() context.Context {
	rr := &RemoteRegistry{
		StartTime: time.Now(),
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "remoteRegistry", rr)
	return ctx
}

func Test_RunStateCheckReadOnlyToReadWriteTransition(t *testing.T) {
	dynamoDbConfig := DynamoDBConfig{
		LeaseName:         "testLease1",
		PodIdentifier:     "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold:  3,
		TableName:         "admiral-lease",
		Role:              "dummyRole",
		Region:            "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	ExecuteStateCheck(setupMockContext(), dynamoDbConfig, &dynamodbClient)
	assert.Equal(t, commonUtil.CurrentAdmiralState.ReadOnly, ReadWriteEnabled)
}

func Test_RunStateCheckReadWriteToReadOnlyTransition(t *testing.T) {
	dynamoDbConfig := DynamoDBConfig{
		LeaseName:         "testLease2",
		PodIdentifier:     "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold:  3,
		TableName:         "admiral-lease",
		Role:              "dummyRole",
		Region:            "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	ExecuteStateCheck(setupMockContext(), dynamoDbConfig, &dynamodbClient)
	assert.Equal(t, commonUtil.CurrentAdmiralState.ReadOnly, ReadOnlyEnabled)
}

func Test_RunStateCheckReadWriteToReadWrite(t *testing.T) {
	dynamoDbConfig := DynamoDBConfig{
		LeaseName:         "testLease1",
		PodIdentifier:     "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold:  3,
		TableName:         "admiral-lease",
		Role:              "dummyRole",
		Region:            "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	ExecuteStateCheck(setupMockContext(), dynamoDbConfig, &dynamodbClient)
	assert.Equal(t, commonUtil.CurrentAdmiralState.ReadOnly, ReadWriteEnabled)
}

func Test_RunStateCheckReadOnlyToReadOnly(t *testing.T) {
	dynamoDbConfig := DynamoDBConfig{
		LeaseName:         "testLease2",
		PodIdentifier:     "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold:  3,
		TableName:         "admiral-lease",
		Role:              "dummyRole",
		Region:            "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	ExecuteStateCheck(setupMockContext(), dynamoDbConfig, &dynamodbClient)
	assert.Equal(t, commonUtil.CurrentAdmiralState.ReadOnly, ReadOnlyEnabled)
}

func Test_RunStateCheckReadOnlyModeGrabbingLock(t *testing.T) {
	dynamoDbConfig := DynamoDBConfig{
		LeaseName:         "testLease3",
		PodIdentifier:     "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold:  3,
		TableName:         "admiral-lease",
		Role:              "dummyRole",
		Region:            "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	ExecuteStateCheck(setupMockContext(), dynamoDbConfig, &dynamodbClient)
	assert.Equal(t, commonUtil.CurrentAdmiralState.ReadOnly, ReadOnlyEnabled)
}

func Test_RunStateCheckNewLockUseCase(t *testing.T) {
	dynamoDbConfig := DynamoDBConfig{
		LeaseName:         "testnewlease",
		PodIdentifier:     "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold:  3,
		TableName:         "admiral-lease",
		Role:              "dummyRole",
		Region:            "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	ExecuteStateCheck(setupMockContext(), dynamoDbConfig, &dynamodbClient)
	assert.Equal(t, commonUtil.CurrentAdmiralState.ReadOnly, ReadOnlyEnabled)
}

func Test_RunStateCheckReadWriteModeSkipLeaseTransition(t *testing.T) {
	dynamoDbConfig := DynamoDBConfig{
		LeaseName:         "skipLease",
		PodIdentifier:     "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold:  3,
		TableName:         "admiral-lease",
		Role:              "dummyRole",
		Region:            "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	ExecuteStateCheck(setupMockContext(), dynamoDbConfig, &dynamodbClient)
	assert.Equal(t, commonUtil.CurrentAdmiralState.ReadOnly, ReadOnlyEnabled)
}

func Test_RunStateCheckReadOnlyModeSkipLeaseNoChange(t *testing.T) {
	dynamoDbConfig := DynamoDBConfig{
		LeaseName:         "skipLease",
		PodIdentifier:     "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold:  3,
		TableName:         "admiral-lease",
		Role:              "dummyRole",
		Region:            "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	ExecuteStateCheck(setupMockContext(), dynamoDbConfig, &dynamodbClient)
	assert.Equal(t, commonUtil.CurrentAdmiralState.ReadOnly, ReadOnlyEnabled)
}

func TestRunStateCheck(t *testing.T) {
	dynamoDbConfig := DynamoDBConfig{
		LeaseName:         "testLease1",
		PodIdentifier:     "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold:  3,
		TableName:         "admiral-lease",
		Role:              "dummyRole",
		Region:            "us-west-2",
	}

	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}

	// Mocking a warmup scenario
	admiralParams := common.AdmiralParams{
		CacheReconcileDuration: 10 * time.Minute,
	}
	common.InitializeConfig(admiralParams)

	tests := []struct {
		name                       string
		ctx                        context.Context
		expectedReadOnly           bool
		expectedIsStateInitialized bool
	}{
		{
			name: "Given that the remoteregistry is nil," +
				"Then the function should return," +
				"And keep the Read-Only and StateInitialized set to false",
			ctx:                        context.TODO(),
			expectedReadOnly:           false,
			expectedIsStateInitialized: false,
		},
		{
			name: "Given that the pod is in warmup phase," +
				"And the remoteregistry is not nil," +
				"Then the pod should stop sending updates to dynamoDB," +
				"And set the Read-Only and StateInitialized setting as true",
			ctx:                        setupMockContext(),
			expectedReadOnly:           ReadOnlyEnabled,
			expectedIsStateInitialized: ReadOnlyEnabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commonUtil.CurrentAdmiralState.ReadOnly = false
			commonUtil.CurrentAdmiralState.IsStateInitialized = false
			ExecuteStateCheck(tt.ctx, dynamoDbConfig, &dynamodbClient)
			assert.Equal(t, tt.expectedReadOnly, commonUtil.CurrentAdmiralState.ReadOnly)
			assert.Equal(t, tt.expectedIsStateInitialized, commonUtil.CurrentAdmiralState.IsStateInitialized)
		})
	}

}
