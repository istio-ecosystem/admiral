package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	log "github.com/sirupsen/logrus"
	"time"
)




func main() {

	var DynamodbClient *DynamoClient
	DynamodbClient = NewDynamoClient()
//	whiteListRecords, err := DynamodbClient.GetWhitelistRecords()
	whiteListRecords, err := DynamodbClient.GetReadWriteLease()

	if len(whiteListRecords) ==2{
	 record:=ReadWriteLease{}
	 record.LeaseName ="e2e"
	 record.LeaseOwner="me"
	 record.UpdatedTime = time.Now().String()
	 DynamodbClient.addUpdateRecord(record)
	}
	if err != nil {
		fmt.Println("Errored!")
	}

	fmt.Println(whiteListRecords);


/*	rootCmd := cmd.GetRootCmd(os.Args[1:])

	if err := rootCmd.Execute(); err != nil {
		os.Exit(-1)
	}*/
}


type ReadWriteLease struct {
	LeaseName string `json:"leaseName"`
	LeaseOwner string `json:"leaseOwner"`
	UpdatedTime string `json:"updatedTime"`
}

type WhitelistRecord struct {
	AccountNumber string `json:"accountNumber"`
	RoleName string `json:"roleName"`
	Labels string `json:"labels"`
}

type DynamoClient struct {
	svc dynamodbiface.DynamoDBAPI
}

func NewDynamoClient() *DynamoClient {
	return &DynamoClient{
		svc: GetDynamoSvc(),
	}
}

func (client *DynamoClient) updatedReadWriteLease(lease ReadWriteLease){
	log.Info("Updating read write lease ", lease)

}

func (client *DynamoClient) addUpdateRecord(lease ReadWriteLease) error {
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
	}).Info("Successfully added item to table " + tableName)

	return err
}


func (client *DynamoClient) GetReadWriteLease() ([]ReadWriteLease, error) {
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

func (client *DynamoClient) GetWhitelistRecords() ([]WhitelistRecord, error) {
	var whitelistRecords []WhitelistRecord
	svc := client.svc
	log.Info("Fetching existing whitelist rules...")
	whitelistRules, err := svc.Scan(&dynamodb.ScanInput{
		TableName: aws.String("aviso-whitelist"),
	})
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("Failed to scan dynamo table")
		return nil, err
	}

	log.WithFields(log.Fields{
		"whitelistRules": whitelistRules,
	}).Debug("retrieved records...")

	item := WhitelistRecord{}

	for _, v := range whitelistRules.Items {
		err = dynamodbattribute.UnmarshalMap(v, &item)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err.Error(),
			}).Panic("Failed to unmarshall record")
		}
		whitelistRecords = append(whitelistRecords, item)
	}
	return whitelistRecords, nil
}

/*func (client *DynamoClient) QueryDynamo(accountNumber string, roleName string) (bool, error) {
	svc := client.svc
	result, err := svc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(Config.DynamoConfig.Table),
		Key: map[string]*dynamodb.AttributeValue{
			"accountNumber": {
				S: aws.String(accountNumber),
			},
			"roleName": {
				S: aws.String(roleName),
			},
		},
	})

	if result.Item != nil {
		return true, nil
	}

	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("Error occured retrieving record from dynamoDB")
		return false, err
	}

	return false, nil
}*/

/*func (client *DynamoClient) WriteWhitelistRecord(rule v1.Rule) error {
	svc := client.svc
	whitelistRecord := WhitelistRecord {
		AccountNumber: rule.Account,
		RoleName: rule.Role,
		Labels:  "cluster",
	}

	return client.addRecord(whitelistRecord, svc)
}*/


/*func (client *DynamoClient) addRecord(item WhitelistRecord, svc dynamodbiface.DynamoDBAPI) error {
	tableName := Config.DynamoConfig.Table
	av, err := dynamodbattribute.MarshalMap(item)
	if err != nil {
		log.WithFields(log.Fields{
			"error" : err.Error(),
		}).Error("Error marshalling new whitelist item.")
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
		"role": item.RoleName,
		"account": item.AccountNumber,
	}).Info("Successfully added item to table " + tableName)

	return err
}
*/

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

