package clusters

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"sync"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	awsSession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

const (
	dynamoDbMaxRetries       = 3
	dynamoDbRetryBackoffTime = 10 * time.Second
	updateWorkloadDataItem   = "updateWorkloadDataItem"

	AssetAliasKey      = "assetAlias"
	EndpointKey        = "endpoint"
	SuccessClustersKey = "successClusters"
	FailedClustersKey  = "failedClusters"
)

type DynamoDBConfigWrapper struct {
	DynamoDBConfig DynamoDBConfig `yaml:"dynamoDB,omitempty"`
}

// Store last known CheckSum of DynamoDB config
var DynamicConfigCheckSum [16]byte

/*
Reference struct used to unmarshall the DynamoDB config present in the yaml config file
*/
type DynamoDBConfig struct {
	LeaseName         string `yaml:"leaseName,omitempty"`
	PodIdentifier     string `yaml:"podIdentifier,omitempty"`
	WaitTimeInSeconds int    `yaml:"waitTimeInSeconds,omitempty"`
	FailureThreshold  int    `yaml:"failureThreshold,omitempty"`
	TableName         string `yaml:"tableName,omitempty"`
	Role              string `yaml:"role,omitempty"`
	Region            string `yaml:"region,omitempty"`
}

type ReadWriteLease struct {
	LeaseName   string `json:"leaseName"`
	LeaseOwner  string `json:"leaseOwner"`
	UpdatedTime int64  `json:"updatedTime"`
	Notes       string `json:"notes"`
}

// workload data struct holds mesh endpoint related information, which includes endpoint, asset alias, env and  gtp details to be persisted in dynamoDb
type WorkloadData struct {
	AssetAlias          string           `json:"assetAlias"`
	Endpoint            string           `json:"endpoint"`
	Env                 string           `json:"env"`
	DnsPrefix           string           `json:"dnsPrefix"`
	LbType              string           `json:"lbType"`
	TrafficDistribution map[string]int32 `json:"trafficDistribution"`
	Aliases             []string         `json:"aliases"`
	GtpManagedBy        string           `json:"gtpManagedBy"`
	GtpId               string           `json:"gtpId"`
	LastUpdatedAt       string           `json:"lastUpdatedAt"` // GTP updation time in RFC3339 format
	SuccessCluster      []string         `json:"successClusters"`
	FailedClusters      []string         `json:"failedClusters"`
}

type DynamicConfigData struct {
	/*
		DynamoDB primary key only support string, binary, int.
		Conversation from binary to bool is not straight forward hence choosing string
	*/
	EnableDynamicConfig                  string   `json:"enableDynamicConfig"`
	NLBEnabledClusters                   []string `json:"nlbEnabledClusters"`
	NLBEnabledIdentityList               []string `json:"nlbEnabledAssetAlias"`
	CLBEnabledClusters                   []string `json:"clbEnabledClusters"`
	InitiateClientInitiatedProcessingFor []string `json:"initiateClientInitiatedProcessingFor"`
}

type DynamoClient struct {
	svc dynamodbiface.DynamoDBAPI
}

func NewDynamoClient(role, region string) (*DynamoClient, error) {
	svc, err := GetDynamoSvc(role, region)
	if err != nil {
		return nil, err
	}
	return &DynamoClient{
		svc: svc,
	}, nil
}

/*
Utility function to update lease duration .
This will be called in configured interval by Active instance
Passive instance calls this when it finds the existing Active instance has not updated the lease within the duration specified.
*/
func (client *DynamoClient) updatedReadWriteLease(lease ReadWriteLease, tableName string) error {
	svc := client.svc
	av, err := dynamodbattribute.MarshalMap(lease)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("Error marshalling readWriteLease item.")
		return err
	}

	input := &dynamodb.PutItemInput{
		Item:      av,
		TableName: aws.String(tableName),
	}
	_, err = svc.PutItem(input)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("Got error calling PutItem:")
		return err
	}
	log.WithFields(log.Fields{
		"leaseName":   lease.LeaseName,
		"leaseOwner":  lease.LeaseOwner,
		"updatedTime": lease.UpdatedTime,
		"notes":       lease.Notes,
	}).Info("Successfully added item to table " + tableName)

	return err
}

/*
Utility function to update workload data item.
This will be called by Active admiral instance on every update to serviceentry.
*/
func (client *DynamoClient) updateWorkloadDataItem(workloadDataEntry *WorkloadData, tableName string, ctxLogger *log.Entry) error {
	expr, err := generateUpdateExpression(workloadDataEntry)
	if err != nil {
		err = fmt.Errorf("failed to generate update expression : %+v", err)
		ctxLogger.Errorf(common.CtxLogFormat, updateWorkloadDataItem, tableName, "", "", err)
		return err
	}

	for i := 0; i < dynamoDbMaxRetries; i++ {
		_, err = client.svc.UpdateItem(&dynamodb.UpdateItemInput{
			TableName:                 aws.String(tableName),
			ReturnValues:              aws.String("NONE"), // NONE as we are ignoring the return value
			UpdateExpression:          expr.Update(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			Key: map[string]*dynamodb.AttributeValue{
				"assetAlias": {S: aws.String(workloadDataEntry.AssetAlias)},
				"endpoint":   {S: aws.String(workloadDataEntry.Endpoint)},
			},
		})

		if err != nil {
			ctxLogger.Errorf(common.CtxLogFormat, updateWorkloadDataItem, tableName, "", "", fmt.Sprintf("failed to update dynamoDB item: %v. Retrying in %v seconds", err, dynamoDbRetryBackoffTime.String()))
			time.Sleep(dynamoDbRetryBackoffTime)
		} else {
			ctxLogger.Infof(common.CtxLogFormat, updateWorkloadDataItem, tableName, "", "", fmt.Sprintf("successfully updated workload data for endpoint=%s", workloadDataEntry.Endpoint))
			return nil
		}
	}

	ctxLogger.Errorf(common.CtxLogFormat+" maxAttempts=%v", updateWorkloadDataItem, tableName, "", "", dynamoDbMaxRetries,
		fmt.Sprintf("exhausted all retry attempts, failed to update workload record for endpoint %s", workloadDataEntry.Endpoint))
	return err
}

func (client *DynamoClient) getWorkloadDataItemByIdentityAndEnv(env, identity, tableName string) ([]WorkloadData, error) {
	var (
		workloadDataItems = []WorkloadData{}
	)

	keyCond := expression.KeyEqual(expression.Key("assetAlias"), expression.Value(identity))
	filt := expression.Name("env").Equal(expression.Value(env))
	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCond).
		WithFilter(filt).
		Build()

	if err != nil {
		return nil, err
	}

	items, err := client.svc.Query(&dynamodb.QueryInput{
		TableName:                 aws.String(tableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query items from workload data table for identity %s and env %s, err: %v", identity, env, err)
	}

	if items == nil {
		return workloadDataItems, nil
	}

	for _, item := range items.Items {
		var workloadDataItem WorkloadData
		err = dynamodbattribute.UnmarshalMap(item, &workloadDataItem)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal table items, err: %v", err)
		}
		workloadDataItems = append(workloadDataItems, workloadDataItem)
	}

	return workloadDataItems, nil
}

func (client *DynamoClient) getDynamicConfig(key string, value string, tableName string) (DynamicConfigData, error) {

	configData := DynamicConfigData{}

	keyCond := expression.KeyEqual(expression.Key(key), expression.Value(value))

	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCond).
		Build()

	if err != nil {
		return configData, err
	}

	dbQuery := dynamodb.QueryInput{
		TableName:                 aws.String(tableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
	}

	items, err := client.svc.Query(&dbQuery)

	if err != nil {
		return configData, fmt.Errorf("task=%s, Failed to fetch dynamic config : %s", common.DynamicConfigUpdate, err)
	}

	if items == nil {
		log.Infof("Failed to fetch dynamic config : %s", tableName)
		return configData, nil
	}

	if items.Count != nil && *items.Count == 1 {
		err = dynamodbattribute.UnmarshalMap(items.Items[0], &configData)
		if err != nil {
			return configData, fmt.Errorf("task=%s, failed to unmarshal table items, err: %v", common.DynamicConfigUpdate, err)
		}

	} else {
		return configData, fmt.Errorf("task=%s, Expected only 1 row but got %d for tableName : %s", common.DynamicConfigUpdate, items.Count, tableName)
	}

	return configData, nil
}

/*
Utility function to update workload data item.
This will be called by Active admiral instance on every update to serviceentry.
*/
func (client *DynamoClient) deleteWorkloadDataItem(workloadDataEntry *WorkloadData, tableName string) error {
	svc := client.svc

	var (
		err error
		av  map[string]*dynamodb.AttributeValue
	)

	keys := make(map[string]string)
	keys["assetAlias"] = workloadDataEntry.AssetAlias
	keys["endpoint"] = workloadDataEntry.Endpoint

	av, err = dynamodbattribute.MarshalMap(keys)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("error marshalling keys while deleting workload data.")
		return err
	}

	input := &dynamodb.DeleteItemInput{
		Key:       av,
		TableName: aws.String(tableName),
	}

	for i := 0; i < dynamoDbMaxRetries; i++ {
		_, err = svc.DeleteItem(input)

		if err != nil {
			log.Info("failed to delete dynamoDB item, retying again in " + dynamoDbRetryBackoffTime.String())
			time.Sleep(dynamoDbRetryBackoffTime)
		} else {
			log.WithFields(log.Fields{
				"workloadEndpoint": workloadDataEntry.Endpoint,
				"assetAlias":       workloadDataEntry.AssetAlias,
			}).Infof("Successfully deleted workload data for endpoint %s to table %s", workloadDataEntry.Endpoint, tableName)

			return nil
		}
	}

	alertMsgWhenFailedToDeleteEndpointData := fmt.Sprintf("exhausted all retry attempts, failed to delete workload record for endpoint %s", workloadDataEntry.Endpoint)
	log.WithFields(log.Fields{
		"error":       err.Error(),
		"tableName":   tableName,
		"maxAttempts": dynamoDbMaxRetries,
	}).Error(alertMsgWhenFailedToDeleteEndpointData)

	return err

}

func getIgnoreIdentityListItem(client *DynamoClient, tableName, clusterEnvironment string) ([]IgnoredIdentityCache, error) {
	var (
		items []IgnoredIdentityCache
	)
	table, err := client.svc.Scan(&dynamodb.ScanInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		return items, fmt.Errorf("failed to scan table: '%s', err: %v", tableName, err)
	}
	for _, item := range table.Items {
		var currentStore = IgnoredIdentityCache{
			RWLock: &sync.RWMutex{},
		}
		err = dynamodbattribute.UnmarshalMap(item, &currentStore)
		if err != nil {
			return items, fmt.Errorf("failed to unmarshal table items, err: %v", err)
		}
		if currentStore.ClusterEnvironment == clusterEnvironment {
			items = append(items, currentStore)
		}
	}
	return items, nil
}

/*
Utility function to get all the entries from the Dynamo DB table
*/
func (client *DynamoClient) getReadWriteLease() ([]ReadWriteLease, error) {
	var readWriteLeases []ReadWriteLease
	svc := client.svc
	log.Info("Fetching existing readWrite entries...")
	readWriteLeaseEntries, err := svc.Scan(&dynamodb.ScanInput{
		TableName: aws.String("admiral-lease"),
	})
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("Failed to scan dynamo table")
		return nil, err
	}

	log.WithFields(log.Fields{
		"readWriteLeaseEntries": readWriteLeaseEntries,
	}).Debug("retrieved records...")

	item := ReadWriteLease{}

	for _, v := range readWriteLeaseEntries.Items {
		err = dynamodbattribute.UnmarshalMap(v, &item)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err.Error(),
			}).Panic("Failed to unmarshall record")
		}
		readWriteLeases = append(readWriteLeases, item)
	}
	return readWriteLeases, nil
}

/*
Utility function to initialize AWS session for DynamoDB connection
*/
func GetDynamoSvc(dynamoArn string, region string) (*dynamodb.DynamoDB, error) {
	log.Info("dynamoArn: " + dynamoArn)
	session := awsSession.Must(awsSession.NewSession())
	// Create the credentials from AssumeRoleProvider to assume the role
	// referenced by the "myRoleARN" ARN.
	creds := stscreds.NewCredentials(session, dynamoArn)
	_, err := creds.Get()
	if err != nil {
		log.Printf("aws credentials are invalid, err: %v", err)
		return nil, err
	}
	// Create a Session with a custom region
	dynamoSession := awsSession.Must(awsSession.NewSession(&aws.Config{
		Credentials: creds,
		Region:      &region,
	}))
	// Create service client value configured for credentials
	// from assumed role.
	svc := dynamodb.New(dynamoSession)
	return svc, nil
}

/*
utility function to read the yaml file containing the DynamoDB configuration.
The file will be present inside the pod. File name should be provided as a program argument.
*/
func BuildDynamoDBConfig(configFile string) (DynamoDBConfig, error) {
	dynamoDBConfigWrapper := &DynamoDBConfigWrapper{}
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return DynamoDBConfig{}, fmt.Errorf("error reading config file to build Dynamo DB config: %v", err)
	}
	err = yaml.Unmarshal(data, &dynamoDBConfigWrapper)
	if err != nil {
		return DynamoDBConfig{}, fmt.Errorf("error unmarshalling config file err: %v", err)
	}
	return dynamoDBConfigWrapper.DynamoDBConfig, nil
}

/*
Utility function to filter lease from all the leases returned from DynamoDB
The DynamoDB table maybe used for multiple environments
*/
func filterOrCreateLeaseIfNotFound(allLeases []ReadWriteLease, leaseName string) ReadWriteLease {
	for _, readWriteLease := range allLeases {
		if readWriteLease.LeaseName == leaseName {
			return readWriteLease
		}
	}
	readWriteLease := ReadWriteLease{}
	readWriteLease.LeaseName = leaseName
	readWriteLease.Notes = "Created at " + strconv.FormatInt(time.Now().UTC().Unix(), 10)
	return readWriteLease
}

func generateUpdateExpression(workloadDataEntry *WorkloadData) (expression.Expression, error) {
	av, err := dynamodbattribute.MarshalMap(workloadDataEntry)
	if err != nil {
		return expression.Expression{}, fmt.Errorf("error marshalling workload data: %v", err)
	}

	update := handleClusterListUpdate(workloadDataEntry)

	for key, value := range av {
		// setting of primary keys is not allowed in UpdateItem
		// skip success and failed cluster keys as they are handled above
		if key == AssetAliasKey || key == EndpointKey || key == FailedClustersKey || key == SuccessClustersKey {
			continue
		}

		// set other keys as it is from workloadDataEntry
		update = update.Set(expression.Name(key), expression.Value(value))
	}

	return expression.NewBuilder().WithUpdate(update).Build()
}

func handleClusterListUpdate(workloadDataEntry *WorkloadData) expression.UpdateBuilder {
	var update expression.UpdateBuilder
	successClusters := (&dynamodb.AttributeValue{}).SetSS(aws.StringSlice(workloadDataEntry.SuccessCluster))
	failedClusters := (&dynamodb.AttributeValue{}).SetSS(aws.StringSlice(workloadDataEntry.FailedClusters))

	// clear success and failure list when there is no gtp in place
	if workloadDataEntry.SuccessCluster == nil && workloadDataEntry.FailedClusters == nil {
		update = update.Remove(expression.Name(SuccessClustersKey))
		update = update.Remove(expression.Name(FailedClustersKey))
		return update
	}

	// this case handles handleDynamoDbUpdateForOldGtp
	if workloadDataEntry.SuccessCluster != nil && workloadDataEntry.FailedClusters != nil {
		update = update.Set(expression.Name(SuccessClustersKey), expression.Value(successClusters))
		update = update.Set(expression.Name(FailedClustersKey), expression.Value(failedClusters))
		return update
	}

	// if destination rule update is successful in cluster, add to success cluster list and remove from failed cluster list
	if workloadDataEntry.SuccessCluster != nil && workloadDataEntry.FailedClusters == nil {
		update = update.Delete(expression.Name(FailedClustersKey), expression.Value(successClusters))
		update = update.Add(expression.Name(SuccessClustersKey), expression.Value(successClusters))
		return update
	}

	// if destination rule update failed in cluster, add to failed cluster list and remove from success cluster list
	if workloadDataEntry.FailedClusters != nil && workloadDataEntry.SuccessCluster == nil {
		update = update.Delete(expression.Name(SuccessClustersKey), expression.Value(failedClusters))
		update = update.Add(expression.Name(FailedClustersKey), expression.Value(failedClusters))
		return update
	}

	return update
}
