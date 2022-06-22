package clusters

import (
	log "github.com/sirupsen/logrus"
	"time"
)

/*
The skip lease pod can be used for testing DynamoDB based DR.
Update the podname field to "SKIP-LEASE-POD" to test Admiral pods in passive mode.
*/
const SKIP_LEASE_CHECK_POD_NAME = "SKIP-LEASE-POD"

type DynamoDBBasedStateChecker struct {
	drConfigFileLocation string
}

func (DynamoDBBasedStateChecker) shouldRunOnIndependentGoRoutine() bool{
	return true;
}

/*
This method has the logic to update the ReadOnly field within the AdmiralState object based on the lease obtained on the shared lock object
The AdmiralState object is referenced everywhere in the code before trying to create/update/delete Istio custom objects

Below is the logic for Admiral instance in Active state
1. Get the latest lease information from DynamoDB table
2. If the current pod owns the lease, update the last updated field with current timestamp
3. Update ReadOnly field to false.
4. Sleep for configured duration
5. Admiral instance which is constantly monitoring all the clusters for changes and is responsible to creating , updating and deleting the Istio custom objects
like Service Entry, Destination rule, Virtual Service , Sidecar and others.

Below is the logic for Admiral instance in Passive state
1. Get the latest lease information from DynamoDB table
2. If the current pod does not own the lease, check if the last updated time field is within the configured wait threshold.
3. If the last updated time field is older than the computed threshold, update self as the owner of the lease with current timestamp as last updated time
4. If the last updated time field is within the computed threshold,mark current pod as read only
5. Sleep for configured duration
*/
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