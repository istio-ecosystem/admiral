package clusters

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"time"
)

type DynamoDBBasedStateChecker struct {}

func (DynamoDBBasedStateChecker) shouldRunOnIndependentGoRoutine() bool{
	return true;
}

func (DynamoDBBasedStateChecker) stateCheckBasedDREnabled() bool {
	return true;
}

func (DynamoDBBasedStateChecker) getStateCheckerName() string{
	return "dynamodbbasedstatechecker"
}

func (DynamoDBBasedStateChecker) runStateCheck(as *AdmiralState){
	as.ReadOnly = READ_ONLY_ENABLED
	var DynamodbClient *DynamoClient
	DynamodbClient = NewDynamoClient()
	leaseName := "qal"
	podIdentifier := "pod1"
	waitDuration := 15 * time.Second
	waitTimeInSeconds :=15
	failureThreshold := 3
	const SKIP_LEASE_CHECK_POD_NAME = "SKIP-LEASE-POD"
	for {
		fmt.Println("Retrieving latest  value of read write value for leaseName :" , leaseName )
		log.Info("Memory location  from state checker of admiral state is = " ,&as.ReadOnly)
		readWriteLeases, err := DynamodbClient.getReadWriteLease()
		if nil!=err{
			log.WithFields(log.Fields{
				"error": err.Error(),
			}).Error("Error retrieving the latest lease")
		}
		readWriteLease := getLease(readWriteLeases,leaseName)
		currentTime := time.Now().UTC().Unix()
		if "" == readWriteLease.LeaseOwner {
			log.Info("Lease with name=" , leaseName, " does not exist. Creating a new lease with owner=" , podIdentifier)
			readWriteLease.LeaseOwner = podIdentifier
			readWriteLease.UpdatedTime = currentTime
			DynamodbClient.updatedReadWriteLease(readWriteLease)
			//Not updating read-write mode until we confirm this pod has the lease
		}else if SKIP_LEASE_CHECK_POD_NAME == readWriteLease.LeaseOwner {
			log.Info("Lease held by skip lease check pod. Setting Admiral to read only mode")
			as.ReadOnly = READ_ONLY_ENABLED;
		}else if podIdentifier == readWriteLease.LeaseOwner {
			as.ReadOnly = READ_WRITE_ENABLED
			log.Info("Lease with name=", leaseName, " is owned by current pod. Extending lease ownership till ", currentTime)
			readWriteLease.UpdatedTime = currentTime
			DynamodbClient.updatedReadWriteLease(readWriteLease)
		}else if readWriteLease.UpdatedTime < (currentTime - int64(waitTimeInSeconds*failureThreshold)){
			log.Info("Current time time is more than the failureInterval. Taking over the lease from ", readWriteLease.LeaseOwner)
			readWriteLease.LeaseOwner = podIdentifier
			readWriteLease.UpdatedTime = currentTime
			DynamodbClient.updatedReadWriteLease(readWriteLease)
			//Not updating read-write mode until we confirm this pod has the lease
		}else {
			log.Info("Lease held by ", readWriteLease.LeaseOwner, " till ", readWriteLease.UpdatedTime)
			as.ReadOnly = READ_ONLY_ENABLED;
		}
		sleep(waitDuration,waitTimeInSeconds)
	}
}