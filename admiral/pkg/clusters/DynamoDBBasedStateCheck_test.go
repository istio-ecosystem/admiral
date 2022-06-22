package clusters

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/stretchr/testify/assert"
	"testing"
)

type mockDynamoClient struct {
	dynamodbiface.DynamoDBAPI
}

func (m *mockDynamoClient) Scan(input *dynamodb.ScanInput) (*dynamodb.ScanOutput, error){
	output :=  &dynamodb.ScanOutput{
		Items: []map[string]*dynamodb.AttributeValue{
			{"leaseName": { S: aws.String("testLease1"),}, "leaseOwner": {S: aws.String("testPod"),},"notes": {S: aws.String("test1"),},"updatedTime": {N: aws.String("1655875287"),},},
			{"leaseName": { S: aws.String("testLease2"),}, "leaseOwner": {S: aws.String("someotherPod"),},"notes": {S: aws.String("test2"),},"updatedTime": {N: aws.String("9999999999"),},},
			{"leaseName": { S: aws.String("testLease3"),}, "leaseOwner": {S: aws.String("someOtherPod"),},"notes": {S: aws.String("test3"),},"updatedTime": {N: aws.String("11111"),},},
			{"leaseName": { S: aws.String("skipLease"),}, "leaseOwner": {S: aws.String("SKIP-LEASE-POD"),},"notes": {S: aws.String("test3"),},"updatedTime": {N: aws.String("11111"),},},
		},
	}

	return output, nil
}


func (m *mockDynamoClient) GetItem(input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error){
	output :=  &dynamodb.GetItemOutput{
		Item: map[string]*dynamodb.AttributeValue{"accountNumber": { S: aws.String("123123123"),}, "roleName": {S: aws.String("PowerUser"),},},
	}
	return output, nil
}

func (m *mockDynamoClient) PutItem(input *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
	return &dynamodb.PutItemOutput{}, nil
}


func Test_RunStateCheckReadOnlyToReadWriteTransition(t *testing.T) {
	dynamoDbConfig:=DynamoDBConfig{
		LeaseName: "testLease1",
		PodIdentifier: "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold: 3,
		TableName: "admiral-lease",
		Role: "dummyRole",
		Region: "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	as:= &AdmiralState{
		ReadOnly: READ_ONLY_ENABLED,
	}
	ExecuteStateCheck(dynamoDbConfig,&dynamodbClient,as)
	assert.Equal(t, as.ReadOnly,READ_WRITE_ENABLED)
}

func Test_RunStateCheckReadWriteToReadOnlyTransition(t *testing.T) {
	dynamoDbConfig:=DynamoDBConfig{
		LeaseName: "testLease2",
		PodIdentifier: "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold: 3,
		TableName: "admiral-lease",
		Role: "dummyRole",
		Region: "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	as:= &AdmiralState{
		ReadOnly: READ_WRITE_ENABLED,
	}
	ExecuteStateCheck(dynamoDbConfig,&dynamodbClient,as)
	assert.Equal(t, as.ReadOnly,READ_ONLY_ENABLED)
}

func Test_RunStateCheckReadWriteToReadWrite(t *testing.T) {
	dynamoDbConfig:=DynamoDBConfig{
		LeaseName: "testLease1",
		PodIdentifier: "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold: 3,
		TableName: "admiral-lease",
		Role: "dummyRole",
		Region: "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	as:= &AdmiralState{
		ReadOnly: READ_WRITE_ENABLED,
	}
	ExecuteStateCheck(dynamoDbConfig,&dynamodbClient,as)
	assert.Equal(t, as.ReadOnly,READ_WRITE_ENABLED)
}

func Test_RunStateCheckReadOnlyToReadOnly(t *testing.T) {
	dynamoDbConfig:=DynamoDBConfig{
		LeaseName: "testLease2",
		PodIdentifier: "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold: 3,
		TableName: "admiral-lease",
		Role: "dummyRole",
		Region: "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	as:= &AdmiralState{
		ReadOnly: READ_ONLY_ENABLED,
	}
	ExecuteStateCheck(dynamoDbConfig,&dynamodbClient,as)
	assert.Equal(t, as.ReadOnly,READ_ONLY_ENABLED)
}

func Test_RunStateCheckReadOnlyModeGrabbingLock(t *testing.T){
	dynamoDbConfig:=DynamoDBConfig{
		LeaseName: "testLease3",
		PodIdentifier: "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold: 3,
		TableName: "admiral-lease",
		Role: "dummyRole",
		Region: "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	as:= &AdmiralState{
		ReadOnly: READ_ONLY_ENABLED,
	}
	ExecuteStateCheck(dynamoDbConfig,&dynamodbClient,as)
	assert.Equal(t, as.ReadOnly,READ_ONLY_ENABLED)
}

func Test_RunStateCheckNewLockUseCase(t *testing.T){
	dynamoDbConfig:=DynamoDBConfig{
		LeaseName: "testnewlease",
		PodIdentifier: "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold: 3,
		TableName: "admiral-lease",
		Role: "dummyRole",
		Region: "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	as:= &AdmiralState{
		ReadOnly: READ_ONLY_ENABLED,
	}
	ExecuteStateCheck(dynamoDbConfig,&dynamodbClient,as)
	assert.Equal(t, as.ReadOnly,READ_ONLY_ENABLED)
}

func Test_RunStateCheckReadWriteModeSkipLeaseTransition(t *testing.T){
	dynamoDbConfig:=DynamoDBConfig{
		LeaseName: "skipLease",
		PodIdentifier: "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold: 3,
		TableName: "admiral-lease",
		Role: "dummyRole",
		Region: "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	as:= &AdmiralState{
		ReadOnly: READ_WRITE_ENABLED,
	}
	ExecuteStateCheck(dynamoDbConfig,&dynamodbClient,as)
	assert.Equal(t, as.ReadOnly,READ_ONLY_ENABLED)
}

func Test_RunStateCheckReadOnlyModeSkipLeaseNoChange(t *testing.T){
	dynamoDbConfig:=DynamoDBConfig{
		LeaseName: "skipLease",
		PodIdentifier: "testPod",
		WaitTimeInSeconds: 15,
		FailureThreshold: 3,
		TableName: "admiral-lease",
		Role: "dummyRole",
		Region: "us-west-2",
	}
	dynamodbClient := DynamoClient{
		&mockDynamoClient{},
	}
	as:= &AdmiralState{
		ReadOnly: READ_ONLY_ENABLED,
	}
	ExecuteStateCheck(dynamoDbConfig,&dynamodbClient,as)
	assert.Equal(t, as.ReadOnly,READ_ONLY_ENABLED)
}