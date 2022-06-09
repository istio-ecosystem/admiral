package clusters

import (
	log "github.com/sirupsen/logrus"
	"time"
)
const SKIP_LEASE_CHECK_POD_NAME = "SKIP-LEASE-POD"

type DynamoDBBasedStateChecker struct {
	drConfigFileLocation string
}

func (DynamoDBBasedStateChecker) shouldRunOnIndependentGoRoutine() bool{
	return true;
}

func (DynamoDBBasedStateChecker) stateCheckBasedDREnabled() bool {
	return true;
}

func (DynamoDBBasedStateChecker) getStateCheckerName() string{
	return "dynamodbbasedstatechecker"
}

func (dr DynamoDBBasedStateChecker) runStateCheck(as *AdmiralState){
	as.ReadOnly = READ_ONLY_ENABLED
	var DynamodbClient *DynamoClient
	dynamoDBConfig,err := BuildDynamoDBConfig(dr.drConfigFileLocation)
	if nil!= err {
		log.Error("DynamoDR: Could not start DynamoDBBasedStateChecker ", err)
		panic("Could not start DynamoDBBasedStateChecker")
	}
	DynamodbClient = NewDynamoClient(dynamoDBConfig)
	leaseName := dynamoDBConfig.LeaseName
	podIdentifier := dynamoDBConfig.PodIdentifierPrefix
	waitDuration := time.Duration(dynamoDBConfig.WaitTimeInSeconds) * time.Second
	waitTimeInSeconds :=dynamoDBConfig.WaitTimeInSeconds
	failureThreshold := dynamoDBConfig.FailureThreshold
	log.Infof("DynamoDR: CurrentPod = %v LeaseName = %v WaitTime= %v sec tableName= %v role= %v region= %v" ,podIdentifier, leaseName, waitTimeInSeconds, dynamoDBConfig.TableName,dynamoDBConfig.Role,dynamoDBConfig.Region)

	for {
		currentTime := time.Now().UTC().Unix()
		log.Infof("DynamoDR: Retrieving latest value of read write value for leaseName : %v , timestamp : %v " , leaseName,currentTime )
		readWriteLeases, err := DynamodbClient.getReadWriteLease()
		if nil!=err{
			log.WithFields(log.Fields{
				"error": err.Error(),
			}).Error("DynamoDR: Error retrieving the latest lease")
		}
		readWriteLease := getLease(readWriteLeases,leaseName)
		if "" == readWriteLease.LeaseOwner {
			log.Infof("DynamoDR: Lease with name=%v does not exist. Creating a new lease with owner=%v" , leaseName,podIdentifier)
			readWriteLease.LeaseOwner = podIdentifier
			readWriteLease.UpdatedTime = currentTime
			DynamodbClient.updatedReadWriteLease(readWriteLease,dynamoDBConfig.TableName)
			//Not updating read-write mode until we confirm this pod has the lease
		}else if SKIP_LEASE_CHECK_POD_NAME == readWriteLease.LeaseOwner {
			log.Info("DynamoDR: Lease held by skip lease check pod. Setting Admiral to read only mode")
			as.ReadOnly = READ_ONLY_ENABLED;
		}else if podIdentifier == readWriteLease.LeaseOwner {
			as.ReadOnly = READ_WRITE_ENABLED
			log.Infof("DynamoDR: Lease with name=%v is owned by the current pod. Extending lease ownership till %v. Admiral will write",leaseName, currentTime)
			readWriteLease.UpdatedTime = currentTime
			DynamodbClient.updatedReadWriteLease(readWriteLease,dynamoDBConfig.TableName)
		}else if readWriteLease.UpdatedTime < (currentTime - int64(waitTimeInSeconds*failureThreshold)){
			diffSecs := currentTime -readWriteLease.UpdatedTime
			log.Infof("DynamoDR: Current time %v is more than the lastUpdated time of lease %v by %v sec. Taking over the lease from %v.",currentTime, readWriteLease.UpdatedTime,diffSecs, readWriteLease.LeaseOwner)
			readWriteLease.LeaseOwner = podIdentifier
			readWriteLease.UpdatedTime = currentTime
			DynamodbClient.updatedReadWriteLease(readWriteLease,dynamoDBConfig.TableName)
			//Not updating read-write mode until we confirm this pod has the lease
		}else {
			log.Infof("DynamoDR: Lease held by %v till %v . Admiral will not write ", readWriteLease.LeaseOwner,readWriteLease.UpdatedTime)
			as.ReadOnly = READ_ONLY_ENABLED;
		}
		sleep(waitDuration,waitTimeInSeconds)
	}
}