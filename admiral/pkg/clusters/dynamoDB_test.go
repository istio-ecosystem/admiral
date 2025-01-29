package clusters

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	"testing"
)

type mockDynamoDBClient struct {
	dynamodbiface.DynamoDBAPI
}

func (m *mockDynamoDBClient) Query(queryInput *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {

	if *queryInput.TableName == "validConfig" {
		return &dynamodb.QueryOutput{
			Items: []map[string]*dynamodb.AttributeValue{
				{
					"EnableDynamicConfig": {S: aws.String(common.Admiral)},
				},
			},
		}, nil
	} else if *queryInput.TableName == "nilConfig" {
		return nil, nil
	} else if *queryInput.TableName == "itemsWithAdmiral" {
		configData := DynamicConfigData{
			EnableDynamicConfig:    common.Admiral,
			NLBEnabledClusters:     []string{"cluster1", "cluster2"},
			NLBEnabledIdentityList: nil,
			CLBEnabledClusters:     nil,
		}
		testDynamoDBAttribute, _ := dynamodbattribute.MarshalMap(&configData)

		queryOutput := &dynamodb.QueryOutput{
			Items: []map[string]*dynamodb.AttributeValue{},
		}
		var queryCount int64
		queryCount = 1
		queryOutput.Items = append(queryOutput.Items, testDynamoDBAttribute)
		queryOutput.Count = &queryCount

		return queryOutput, nil
	} else if *queryInput.TableName == "throwError" {
		return nil, errors.New("test error")
	}
	return nil, nil

}

func TestDynamoClient_getDynamicConfig(t *testing.T) {

	testDynamoDBClient := DynamoClient{}
	testDynamoDBClient.svc = &mockDynamoDBClient{}

	type args struct {
		key       string
		value     string
		tableName string
	}

	testArgs := args{
		key:       "id",
		value:     "id",
		tableName: "nilConfig",
	}

	testArgs1 := args{
		key:       "test",
		value:     "test",
		tableName: "validConfig",
	}

	testArgs2 := args{
		key:       "test",
		value:     "test",
		tableName: "itemsWithAdmiral",
	}

	testArgs3 := args{
		key:       "test",
		value:     "test",
		tableName: "throwError",
	}

	tests := []struct {
		name    string
		fields  *DynamoClient
		args    args
		want    DynamicConfigData
		wantErr error
	}{
		{"When dynamodb return empty object then expect no error", &testDynamoDBClient, testArgs, DynamicConfigData{}, nil},
		{"When dynamodb return non object then expect non DynamicConfigData", &testDynamoDBClient, testArgs1, DynamicConfigData{}, errors.New("Expected only 1 row but got 0 for tableName")},
		{"When dynamodb return object with NLBCluster", &testDynamoDBClient, testArgs2, DynamicConfigData{EnableDynamicConfig: common.Admiral, NLBEnabledClusters: []string{"cluster1", "cluster2"}}, nil},
		{"When dynamodb return error then error needs to be bubbled up", &testDynamoDBClient, testArgs3, DynamicConfigData{}, errors.New("Failed to fetch dynamic config")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.getDynamicConfig(tt.args.key, tt.args.value, tt.args.tableName)
			if tt.wantErr != nil {
				assert.Contains(t, err.Error(), tt.wantErr.Error())
			} else {
				assert.Nil(t, err, "GetDynamicConfig() should not return error")
				assert.Equalf(t, tt.want, got, "getDynamicConfig(%v, %v, %v)", tt.args.key, tt.args.value, tt.args.tableName)
			}
		})
	}
}
