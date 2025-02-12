package clusters

import (
	"context"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"time"

	log "github.com/sirupsen/logrus"
)

/*
The skip lease pod can be used for testing DynamoDB based DR.
Update the podname field to "SKIP-LEASE-POD" to test Admiral pods in passive mode.
*/
const skipLeaseCheckPodName = "SKIP-LEASE-POD"

type admiralReadWriteLeaseStateChecker struct {
	drConfigFileLocation string
}

func (admiralReadWriteLeaseStateChecker) shouldRunOnIndependentGoRoutine() bool {
	return true
}

func (admiralReadWriteLeaseStateChecker) initStateCache(cache interface{}) error {
	return nil
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
func (dr admiralReadWriteLeaseStateChecker) runStateCheck(ctx context.Context) {
	commonUtil.CurrentAdmiralState.ReadOnly = ReadOnlyEnabled
	var dynamodbClient *DynamoClient
	dynamoDBConfig, err := BuildDynamoDBConfig(dr.drConfigFileLocation)
	if nil != err {
		log.Error("dynamoDR: Could not start DynamoDBBasedStateChecker ", err)
		log.Panic("could not start DynamoDBBasedStateChecker")
	}
	dynamodbClient, err = NewDynamoClient(dynamoDBConfig.Role, dynamoDBConfig.Region)
	if err != nil {
		log.Errorf("unable to instantiate dynamo client, err: %v", err)
	}
	waitDuration := time.Duration(dynamoDBConfig.WaitTimeInSeconds) * time.Second
	ticker := time.NewTicker(waitDuration)
	defer ticker.Stop()
	// Call Execute State Check explicitly to speed up initialization. Without this the initialization will be delayed by waitDuration
	ExecuteStateCheck(ctx, dynamoDBConfig, dynamodbClient)
	for {
		select {
		case <-ctx.Done():
			log.Infoln("dynamoDR: context done stopping ticker")
			return
		case <-ticker.C:
			ExecuteStateCheck(ctx, dynamoDBConfig, dynamodbClient)
		}
	}
}

func ExecuteStateCheck(ctx context.Context, dynamoDBConfig DynamoDBConfig, dynamodbClient *DynamoClient) {
	var (
		leaseName         = dynamoDBConfig.LeaseName
		podIdentifier     = dynamoDBConfig.PodIdentifier
		waitTimeInSeconds = dynamoDBConfig.WaitTimeInSeconds
		failureThreshold  = dynamoDBConfig.FailureThreshold
		currentTime       = time.Now().UTC().Unix()
	)

	log.Infof("DynamoDR: CurrentPod = %v LeaseName = %v WaitTime= %v sec tableName= %v role= %v region= %v", podIdentifier, leaseName, waitTimeInSeconds, dynamoDBConfig.TableName, dynamoDBConfig.Role, dynamoDBConfig.Region)
	log.Infof("DynamoDR: Retrieving latest value of read write value for leaseName : %v , timestamp : %v ", leaseName, currentTime)

	remoteRegistry, ok := ctx.Value("remoteRegistry").(*RemoteRegistry)
	if !ok {
		log.Errorf(AssertionLogMsg, ctx.Value("remoteRegistry"))
		return
	}

	readWriteLeases, err := dynamodbClient.getReadWriteLease()
	if nil != err {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("DynamoDR: Error retrieving the latest lease")
		//Transition Admiral to Read-only mode in case of issue connecting to Dynamo DB
		commonUtil.CurrentAdmiralState.ReadOnly = ReadOnlyEnabled
		log.Error("DynamoDR: Error retrieving the latest lease. Admiral will not write")
		return
	}

	readWriteLease := filterOrCreateLeaseIfNotFound(readWriteLeases, leaseName)
	if readWriteLease.LeaseOwner == "" {
		log.Infof("DynamoDR: Lease with name=%v does not exist. Creating a new lease with owner=%v", leaseName, podIdentifier)
		readWriteLease.LeaseOwner = podIdentifier
		readWriteLease.UpdatedTime = currentTime
		dynamodbClient.updatedReadWriteLease(readWriteLease, dynamoDBConfig.TableName)
		//Not updating read-write mode until we confirm this pod has the lease
	} else if skipLeaseCheckPodName == readWriteLease.LeaseOwner {
		commonUtil.CurrentAdmiralState.ReadOnly = ReadOnlyEnabled
		commonUtil.CurrentAdmiralState.IsStateInitialized = StateInitialized
	} else if remoteRegistry != nil && podIdentifier == readWriteLease.LeaseOwner && IsCacheWarmupTime(remoteRegistry) {
		// If the Active Admiral pod is in warmup phase we skip sending the updates to DynamoDB thus allowing the Passive Admiral instance to take over the lease.
		log.Infof("DynamoDR: Lease with name=%v is owned by the current pod. No updates are sent to dynamoDB in warmup state. Will allow the other instance to take over the lease.", leaseName)
		commonUtil.CurrentAdmiralState.ReadOnly = ReadOnlyEnabled
		commonUtil.CurrentAdmiralState.IsStateInitialized = StateInitialized
	} else if podIdentifier == readWriteLease.LeaseOwner {
		commonUtil.CurrentAdmiralState.ReadOnly = ReadWriteEnabled
		commonUtil.CurrentAdmiralState.IsStateInitialized = StateInitialized
		log.Infof("DynamoDR: Lease with name=%v is owned by the current pod. Extending lease ownership till %v. Admiral will write", leaseName, currentTime)
		readWriteLease.UpdatedTime = currentTime
		dynamodbClient.updatedReadWriteLease(readWriteLease, dynamoDBConfig.TableName)
	} else if readWriteLease.UpdatedTime < (currentTime-int64(waitTimeInSeconds*failureThreshold)) && remoteRegistry != nil && !IsCacheWarmupTime(remoteRegistry) {
		diffSecs := currentTime - readWriteLease.UpdatedTime
		log.Infof("DynamoDR: Current time %v is more than the lastUpdated time of lease %v by %v sec. Taking over the lease from %v.", currentTime, readWriteLease.UpdatedTime, diffSecs, readWriteLease.LeaseOwner)
		readWriteLease.LeaseOwner = podIdentifier
		readWriteLease.UpdatedTime = currentTime
		dynamodbClient.updatedReadWriteLease(readWriteLease, dynamoDBConfig.TableName)
		//Not updating read-write mode until we confirm this pod has the lease
	} else {
		log.Infof("DynamoDR: Lease held by %v till %v . Admiral will not write ", readWriteLease.LeaseOwner, readWriteLease.UpdatedTime)
		commonUtil.CurrentAdmiralState.ReadOnly = ReadOnlyEnabled
		commonUtil.CurrentAdmiralState.IsStateInitialized = StateInitialized
	}
}
