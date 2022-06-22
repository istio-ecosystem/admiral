package clusters

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strconv"
	"time"
)

/*
Utility function to block the go-routine for duration specified
*/
func sleep(sleepDuration time.Duration, sleepSeconds int){
	log.Info("Sleeping for ", sleepSeconds, " seconds")
	time.Sleep(sleepDuration)
}

/*
Utility function to filter lease from all the leases returned from DynamoDB
The DynamoDB table maybe used for multiple environments
*/
func filterOrCreateLeaseIfNotFound(allLeases []ReadWriteLease, leaseName string) ReadWriteLease {
	for _, readWriteLease := range  allLeases {
		if readWriteLease.LeaseName == leaseName {
			return  readWriteLease
		}
	}
	readWriteLease := ReadWriteLease{}
	readWriteLease.LeaseName = leaseName;
	readWriteLease.Notes ="Created at "+strconv.FormatInt(time.Now().UTC().Unix(), 10)
	return readWriteLease
}


type ReadWriteLease struct {
	LeaseName string `json:"leaseName"`
	LeaseOwner string `json:"leaseOwner"`
	UpdatedTime int64 `json:"updatedTime"`
	Notes string `json:"notes"`
}

type DynamoClient struct {
	svc dynamodbiface.DynamoDBAPI
}

func NewDynamoClient(dynamoDBConfig DynamoDBConfig) *DynamoClient {
	return &DynamoClient{
		svc: GetDynamoSvc(dynamoDBConfig.Role,dynamoDBConfig.Region),
	}
}

/*
Utility function to update lease duration .
This will be called in configured interval by Active instance
Passive instance calls this when it finds the existing Active instance has not udpated the lease within the duration specified.
*/
func (client *DynamoClient) updatedReadWriteLease(lease ReadWriteLease, tableName string) error {
	svc := client.svc
	av, err := dynamodbattribute.MarshalMap(lease)
	if err != nil {
		log.WithFields(log.Fields{
			"error" : err.Error(),
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
		"leaseName": lease.LeaseName,
		"leaseOwner": lease.LeaseOwner,
		"updatedTime": lease.UpdatedTime,
		"notes": lease.Notes,
	}).Info("Successfully added item to table " + tableName)

	return err
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
Utility function to initialize AWS seassion for DynamoDB connection
*/
func GetDynamoSvc(dynamoArn string,region string) *dynamodb.DynamoDB {
	log.Info("dynamoArn: "+dynamoArn)
	sess := session.Must(session.NewSession())
	// Create the credentials from AssumeRoleProvider to assume the role
	// referenced by the "myRoleARN" ARN.
	creds := stscreds.NewCredentials(sess, dynamoArn)
	// Create a Session with a custom region
	dynamoSession := session.Must(session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      &region,
	}))
	// Create service client value configured for credentials
	// from assumed role.
	svc := dynamodb.New(dynamoSession)
	return svc
}
/*
utility function to read the yaml file containing the DynamoDB configuration.
The file will be present inside the pod. File name should be provided as a program argument.
*/
func BuildDynamoDBConfig(configFile string) (DynamoDBConfig, error) {

	data, err := ioutil.ReadFile(configFile)
	dynamoDBConfigWrapper := &DynamoDBConfigWrapper{}

	if err != nil {
		return DynamoDBConfig{}, fmt.Errorf("error reading config file to build Dynamo DB config: %v", err)
	}

	err = yaml.Unmarshal(data, &dynamoDBConfigWrapper)

	if err != nil {
		return DynamoDBConfig{}, fmt.Errorf("error unmarshaling config file err: %v", err)
	}

	return dynamoDBConfigWrapper.DynamoDBConfig,nil
}
