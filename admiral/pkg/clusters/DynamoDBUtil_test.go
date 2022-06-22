package clusters

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/stretchr/testify/assert"
	"testing"
)

type mockDynamoDBClient struct {
	dynamodbiface.DynamoDBAPI
}

func (m *mockDynamoDBClient) Scan(input *dynamodb.ScanInput) (*dynamodb.ScanOutput, error){
	output :=  &dynamodb.ScanOutput{
		Items: []map[string]*dynamodb.AttributeValue{
			{"leaseName": { S: aws.String("qal"),}, "leaseOwner": {S: aws.String("qal-west"),},"notes": {S: aws.String("test1"),},"updatedTime": {N: aws.String("1655875287"),},},
			{"leaseName": { S: aws.String("e2e"),}, "leaseOwner": {S: aws.String("e2e-west"),},"notes": {S: aws.String("test2"),},"updatedTime": {N: aws.String("1655875287"),},},
		},
	}

	return output, nil
}


func (m *mockDynamoDBClient) GetItem(input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error){
	output :=  &dynamodb.GetItemOutput{
		Item: map[string]*dynamodb.AttributeValue{"accountNumber": { S: aws.String("123123123"),}, "roleName": {S: aws.String("PowerUser"),},},
	}
	return output, nil
}

func (m *mockDynamoDBClient) PutItem(input *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
	return &dynamodb.PutItemOutput{}, nil
}


type mockDynamoDBClientWithErrors struct {
	dynamodbiface.DynamoDBAPI
}

func (m *mockDynamoDBClientWithErrors) GetItem(input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error){
	return &dynamodb.GetItemOutput{}, errors.New("error occurred retrieving the item")
}

func (m *mockDynamoDBClientWithErrors) Scan(input *dynamodb.ScanInput) (*dynamodb.ScanOutput, error){
	return &dynamodb.ScanOutput{}, errors.New("dynamodb scan error")
}

func (m *mockDynamoDBClientWithErrors) PutItem(input *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
	return &dynamodb.PutItemOutput{}, errors.New("error occured adding record to dynamodb")
}

func Test_GetReadWriteLease(t *testing.T) {

	DynamodbClient := DynamoClient{
		&mockDynamoDBClient{},
	}
	readWriteRecords, err := DynamodbClient.getReadWriteLease()

	assert.Nil(t, err)
	assert.Equal(t, len(readWriteRecords), 2)
}

func Test_GetReadWriteLeaseError(t *testing.T) {

	DynamodbClient := DynamoClient{
		&mockDynamoDBClientWithErrors{},
	}
	whitelistRecords, err := DynamodbClient.getReadWriteLease()

	assert.NotNil(t, err)
	assert.Equal(t, len(whitelistRecords), 0)
	assert.Equal(t, err.Error(), "dynamodb scan error")
}

func Test_WriteWhitelistRecord(t *testing.T) {

	lease := ReadWriteLease{
		LeaseName: "qal",
		LeaseOwner: "qal-west",
		Notes: "tester",
		UpdatedTime: 1655875287,
	}
	DynamodbClient := DynamoClient{
		&mockDynamoDBClient{},
	}

	err := DynamodbClient.updatedReadWriteLease(lease,"test-table")

	assert.Nil(t, err)
}

func Test_WriteWhitelistRecordWithError(t *testing.T) {

	lease := ReadWriteLease{
		LeaseName: "qal",
		LeaseOwner: "qal-west",
		Notes: "tester",
		UpdatedTime: 1655875287,
	}
	DynamodbClient := DynamoClient{
		&mockDynamoDBClientWithErrors{},
	}

	err := DynamodbClient.updatedReadWriteLease(lease,"test-table")

	assert.NotNil(t, err)
}

func Test_BuildDynamoDBConfig(t *testing.T)  {
	dynamoDBConfig,err := BuildDynamoDBConfig("testdata/fake-dynamodb-config.yaml")
	assert.Nil(t, err)
	assert.NotNil(t, dynamoDBConfig)
	assert.Equal(t,"somelease" ,dynamoDBConfig.LeaseName)
	assert.Equal(t,"arn:aws:iam::someaccount:role/somerole" ,dynamoDBConfig.Role)
	assert.Equal(t,"sometable" ,dynamoDBConfig.TableName)
	assert.Equal(t,3 ,dynamoDBConfig.FailureThreshold)
	assert.Equal(t,"somename" ,dynamoDBConfig.PodIdentifier)
	assert.Equal(t,"us-west-2" ,dynamoDBConfig.Region)
	assert.Equal(t,15 ,dynamoDBConfig.WaitTimeInSeconds)
}