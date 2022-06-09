package clusters

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	log "github.com/sirupsen/logrus"
	"strconv"
	"time"
)

func sleep(sleepDuration time.Duration, sleepSeconds int){
	log.Info("Sleeping for ", sleepSeconds, " seconds")
	time.Sleep(sleepDuration)
}

func getLease(allLeases []ReadWriteLease, leaseName string) ReadWriteLease {
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

func NewDynamoClient() *DynamoClient {
	return &DynamoClient{
		svc: GetDynamoSvc(),
	}
}


func (client *DynamoClient) updatedReadWriteLease(lease ReadWriteLease) error {
	svc := client.svc
	tableName := "admiral-lease"
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

func GetDynamoSvc() *dynamodb.DynamoDB {
	dynamoArn :="arn:aws:iam::930875703956:role/IKSDynamoAccess"
	log.Info("dynamoArn: "+dynamoArn)
	sess := session.Must(session.NewSession())
	// Create the credentials from AssumeRoleProvider to assume the role
	// referenced by the "myRoleARN" ARN.
	creds := stscreds.NewCredentials(sess, dynamoArn)
	region := "us-west-2"
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
