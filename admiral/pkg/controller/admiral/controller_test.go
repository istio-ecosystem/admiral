package admiral

import (
	"context"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	k8sAppsV1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckIfResourceVersionIsChanged(t *testing.T) {
	var (
		ctxLogger = log.WithFields(log.Fields{
			"txId": "abc",
		})
		ctx                = context.Background()
		resourceName1      = "resource-name-1"
		resourceNameSpace1 = "resource-namespace-1"
	)
	testCases := []struct {
		name                  string
		oldObj                interface{}
		newObj                interface{}
		delegator             *MockDelegator
		resourceName          string
		resourceNamespace     string
		latestObjInKubernetes interface{}
		expectedResult        bool
	}{
		{
			name: "Given an update event " +
				"When the resource version for both the object is the same " +
				"Then func should return false",
			oldObj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1111",
				},
			},
			newObj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1111",
				},
			},
			expectedResult: false,
		},
		{
			name: "Given an update event " +
				"When the resource version of new object is greater than resource version of old object, " +
				"Then func should return true",
			oldObj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1111",
				},
			},
			newObj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "9999",
				},
			},
			expectedResult: true,
		},
		{
			name: "Given an update event " +
				"When the resource version for the new object is smaller than the old object, " +
				"When the new object was added because it was a retry event, " +
				"When delegator is not initialized, " +
				"Then func should return true",
			oldObj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "2",
				},
			},
			newObj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1",
				},
			},
			delegator:      nil,
			expectedResult: true,
		},
		{
			name: "Given an update event " +
				"When the resource version for the new object is smaller than the old object, " +
				"When the new object was added because it was a retry event, " +
				"When delegator is initialized, " +
				"Then func should return false",
			oldObj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:            resourceName1,
					Namespace:       resourceNameSpace1,
					ResourceVersion: "2",
				},
			},
			newObj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:            resourceName1,
					Namespace:       resourceNameSpace1,
					ResourceVersion: "1",
				},
			},
			delegator:         NewMockDelegator(),
			resourceName:      resourceName1,
			resourceNamespace: resourceNameSpace1,
			latestObjInKubernetes: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:            resourceName1,
					Namespace:       resourceNameSpace1,
					ResourceVersion: "2",
				},
			},
			expectedResult: false,
		},
		{
			name: "Given an update event " +
				"When the resource version for the new object is smaller than the old object, " +
				"When the new object was added because the resource version was reset , " +
				"When delegator is initialized, " +
				"Then func should return true",
			oldObj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:            resourceName1,
					Namespace:       resourceNameSpace1,
					ResourceVersion: "2",
				},
			},
			newObj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:            resourceName1,
					Namespace:       resourceNameSpace1,
					ResourceVersion: "1",
				},
			},
			delegator:         NewMockDelegator(),
			resourceName:      resourceName1,
			resourceNamespace: resourceNameSpace1,
			latestObjInKubernetes: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:            resourceName1,
					Namespace:       resourceNameSpace1,
					ResourceVersion: "1",
				},
			},
			expectedResult: true,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.delegator != nil {
				c.delegator.SetGetReturn(c.latestObjInKubernetes, nil)
			}
			var result bool
			_, result = checkIfResourceVersionHasIncreased(ctxLogger, ctx, c.oldObj, c.newObj, c.delegator)
			assert.Equal(t, c.expectedResult, result)
		})
	}
}

func TestShouldRetry(t *testing.T) {
	var (
		ctxLogger = log.WithFields(log.Fields{
			"txId": "abc",
		})
		ctx                = context.Background()
		resourceName1      = "resource-name-1"
		resourceNameSpace1 = "resource-namespace-1"
		serviceSlice       = []*v1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName1 + "-canary",
					Namespace: resourceNameSpace1,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName1,
					Namespace: resourceNameSpace1,
				},
			},
		}
	)
	testCases := []struct {
		name                  string
		obj                   interface{}
		delegator             *MockDelegator
		resourceName          string
		resourceNamespace     string
		latestObjInKubernetes interface{}
		expectedResult        bool
	}{
		{
			name: "Given an update event " +
				"When the resource version for both the object is the same " +
				"When the new object was added because it was a retry event " +
				"Then func should return true",
			obj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1111",
				},
			},
			delegator:         nil,
			expectedResult:    true,
			resourceName:      resourceName1,
			resourceNamespace: resourceNameSpace1,
			latestObjInKubernetes: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1111",
				},
			},
		},
		{
			name: "Given an update event " +
				"When the resource version for the new object is smaller than the old object " +
				"When the new object was added because it was a retry event " +
				"When delegator is initialized" +
				"Then func should return false",
			obj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:            resourceName1,
					Namespace:       resourceNameSpace1,
					ResourceVersion: "2",
				},
			},
			delegator:         NewMockDelegator(),
			resourceName:      resourceName1,
			resourceNamespace: resourceNameSpace1,
			latestObjInKubernetes: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:            resourceName1,
					Namespace:       resourceNameSpace1,
					ResourceVersion: "3",
				},
			},
			expectedResult: false,
		},
		{
			name: "Given an update event " +
				"When the resource version for the new object is smaller than the old object, " +
				"When the new object was added because of a retry " +
				"When delegator is nil, " +
				"Then func should return true",
			obj: &k8sAppsV1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:            resourceName1,
					Namespace:       resourceNameSpace1,
					ResourceVersion: "2",
				},
			},
			delegator:      nil,
			expectedResult: true,
		},
		{
			name: "Given an update event, " +
				"When the controller is a service controller, " +
				"When the controller returns a slice of services from the cache, " +
				"Then the func should look through the cache for the matching service.",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName1,
					Namespace: resourceNameSpace1,
				},
			},
			delegator:             NewMockDelegator(),
			latestObjInKubernetes: interface{}(serviceSlice),
			expectedResult:        true,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.delegator != nil {
				c.delegator.SetGetReturn(c.latestObjInKubernetes, nil)
			}
			var result bool
			result = shouldRetry(ctxLogger, ctx, c.obj, c.delegator)
			assert.Equal(t, c.expectedResult, result)
		})
	}
}

// Successfully creates a new controller with given parameters
/*func TestNewControllerCreation(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	delegator := new(MockDelegator)
	informer := new(MockSharedIndexInformer)

	controller := NewController("test-controller", "test-cluster", stopCh, delegator, informer)

	if controller.name != "test-controller" {
		t.Errorf("expected controller name to be 'test-controller', got %s", controller.name)
	}
	if controller.cluster != "test-cluster" {
		t.Errorf("expected cluster to be 'test-cluster', got %s", controller.cluster)
	}
	if controller.delegator != delegator {
		t.Errorf("expected delegator to be set")
	}
	if controller.informer != informer {
		t.Errorf("expected informer to be set")
	}
}*/

// Handles nil newObj or latestObj gracefully
func TestUpdateTxIdHandlesNilObjectsGracefully(t *testing.T) {
	ctx := context.Background()
	var latestObj metav1.Object = nil
	newObj := &metav1.ObjectMeta{ResourceVersion: "2"}
	txId := "initialTxId"
	ctxLogger := log.WithFields(log.Fields{})
	controller := Controller{name: "testController", cluster: "testCluster"}

	updatedTxId, updatedLogger := updateTxId(ctx, newObj, latestObj, txId, ctxLogger, controller)

	if updatedTxId != txId {
		t.Errorf("Expected txId to remain unchanged, got %s", updatedTxId)
	}

	if updatedLogger != ctxLogger {
		t.Errorf("Expected logger to remain unchanged")
	}
}

func TestUpdateTxIdWhenResourceVersionsDiffer(t *testing.T) {
	ctx := context.Background()
	latestObj := &metav1.ObjectMeta{ResourceVersion: "1"}
	newObj := &metav1.ObjectMeta{ResourceVersion: "2"}
	txId := "initialTxId"
	ctxLogger := log.WithFields(log.Fields{})
	controller := Controller{name: "testController", cluster: "testCluster"}

	updatedTxId, updatedLogger := updateTxId(ctx, newObj, latestObj, txId, ctxLogger, controller)

	expectedTxIdPrefix := "1-initialTxId"
	if !strings.HasPrefix(updatedTxId, expectedTxIdPrefix) {
		t.Errorf("Expected txId to start with %s, got %s", expectedTxIdPrefix, updatedTxId)
	}

	if updatedLogger.Data["txId"] != updatedTxId {
		t.Errorf("Expected logger txId to be %s, got %s", updatedTxId, updatedLogger.Data["txId"])
	}
}

// Successfully adds an object to the queue when a valid object is passed
func TestAddFuncImplAddsValidObjectToQueue(t *testing.T) {
	// Arrange
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	informer := cache.NewSharedIndexInformer(nil, &v1.Pod{}, 0, cache.Indexers{})
	controller := &Controller{
		name:     "test-controller",
		cluster:  "test-cluster",
		queue:    queue,
		informer: informer,
	}
	obj := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pod",
			Namespace:       "default",
			ResourceVersion: "1",
		},
	}

	// Act
	controller.AddFuncImpl(obj)

	// Assert
	if queue.Len() != 1 {
		t.Errorf("Expected queue length to be 1, got %d", queue.Len())
	}
}

// Processes new object if resource version has changed
func TestUpdateFuncImplProcessesNewObjectOnResourceVersionChange(t *testing.T) {
	// Arrange
	controller := &Controller{
		name:      "test-controller",
		cluster:   "test-cluster",
		delegator: &MockDelegator{},
		queue:     workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}
	oldObj := &v1.Pod{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"}}
	newObj := &v1.Pod{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "2"}}

	// Act
	controller.UpdateFuncImpl(oldObj, newObj)

	// Assert
	if controller.queue.Len() != 1 {
		t.Errorf("Expected queue length to be 1, got %d", controller.queue.Len())
	}
}

// Successfully adds a delete event to the queue when a valid object is passed
func TestDeleteFuncImplAddsDeleteEventToQueue(t *testing.T) {

	// Create a mock object that implements the necessary interface
	mockObj := &metav1.ObjectMeta{
		Name:            "test-object",
		Namespace:       "test-namespace",
		ResourceVersion: "12345",
	}

	// Create a mock queue
	mockQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Initialize the Controller with the mock queue
	controller := &Controller{
		name:    "test-controller",
		cluster: "test-cluster",
		queue:   mockQueue,
	}

	// Call the DeleteFuncImpl method with the mock object
	controller.DeleteFuncImpl(mockObj)

	// Check if the event was added to the queue
	if mockQueue.Len() != 1 {
		t.Errorf("Expected queue length of 1, but got %d", mockQueue.Len())
	}

	// Retrieve the item from the queue and verify its contents
	item, _ := mockQueue.Get()
	informerCacheObj, ok := item.(InformerCacheObj)
	if !ok {
		t.Errorf("Expected item of type InformerCacheObj, but got %T", item)
	}

	if informerCacheObj.eventType != Delete {
		t.Errorf("Expected event type 'Delete', but got %s", informerCacheObj.eventType)
	}

	if informerCacheObj.key == "" {
		t.Error("Expected a non-empty key")
	}

	if informerCacheObj.obj != mockObj {
		t.Error("Expected obj to be the same as the passed object")
	}
}

func TestController_ProcessNextItem(t *testing.T) {
	// Arrange
	mockQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	mockDelegator := &MockDelegator{}
	controller := &Controller{
		name:      "test-controller",
		cluster:   "test-cluster",
		delegator: mockDelegator,
		queue:     mockQueue,
	}

	// Add an item to the queue
	mockQueue.Add("test-key")

	// Act
	controller.processNextItem()
}
