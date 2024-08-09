package admiral

import (
	"context"
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
