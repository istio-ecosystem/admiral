# Disaster recovery

## Key terms

### Active Admiral/Read-Write Admiral 
Admiral instance which is constantly monitoring all the clusters for changes and is responsible for creating , updating and deleting the Istio custom objects 
like Service Entry, Destination rule, Virtual Service , Sidecar and others.

### Passive Admiral/Read-only Admiral 
Admiral instance which constantly monitors all the clusters for changes but does not manipulate any Istio objects. This instance does not create, update or delete Istio custom objects.

### Lock object 
A reference object that is used to define if an Admiral instance is active or passive.

### Lease 
Duration for which the usage rights are obtained on a lock object.

## What is the need for this ?

As of this writing, we can only run one instance of Admiral in a Service mesh environment. We risk running into data inconsistency issues if Admiral is scaled out.
Multiple Admiral instances can overwrite Service Entry and other Istio custom objects and cause non-determistic behaviour.
Running only one instance of Admiral exposes us to the risk of not being able to modify Istio custom objects in case the Admiral instance is unresponsive or has network issue due to infrastructure failures.

In a Service mesh spanning multiple availability zones/DR regions, we need the ability to run passive Admiral instance that can take over the responsibility of
updating and creating Istio custom objects whenever the active instance encounters any issues.

## Changes to health check API -
As of this writing, Admiral exposes REST APIs that can be used to check the current clusters being monitored and also data about various Istio custom objects.
As we consider data from Active state Admiral as the Source of truth, we need a way to direct all REST APIs to the active state Admiral. To support this, we have enhanced the health check endpoint to 
include an optional query param ```checkifreadonly```. If this query param is set to true, a passive state Admiral will return a 502 whereas an active state Admiral 
returns a successful response. This query param can be used to configure health check diagnostic URL to decide the active instance from a pool of Admiral instances.
Do not use this query param on [readiness or liveness check probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/).
![](Admiral-health-check.png)

## Creating custom DR solutions

To create your own implementation of DR, please create struct which implements below interface methods.

* ```runStateCheck ``` should have the logic for DR. This take AdmiralState type object as param and modifies the readonly flag to transition Admiral between Active and Passive modes
* ```shouldRunOnIndependentGoRoutine ``` should return true if you want the DR logic in ```runStateCheck``` method to run on a seperate GoRoutine.

```
type AdmiralStateChecker interface {
	runStateCheck(ctx context.Context)
	shouldRunOnIndependentGoRoutine() bool
}
```
* Once you have the Struct which implements above interface methods, update the ```startAdmiralStateChecker``` function to invoke your handler.
* Invoke your custom implementation by setting the program argument --admiral_state_checker_name to a value used in ```startAdmiralStateChecker``` function
* Please contribute your implementation to this project

## Sample implementations
### [Dynamo DB Based Admiral DR](DynamoDBBasedDR.md)


