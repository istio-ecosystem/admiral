package admiral

import (
	"context"
	"errors"
	"fmt"
	admiralapiv1 "github.com/istio-ecosystem/admiral-api/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
)

func GetMockShard() *admiralapiv1.Shard {
	identityItem := admiralapiv1.IdentityItem{
		Name:        "sample",
		Environment: "e2e",
	}
	clusterShard := admiralapiv1.ClusterShards{
		Name:       "cg-tax-ppd-usw2-k8s",
		Locality:   "us-west-2",
		Identities: []admiralapiv1.IdentityItem{identityItem},
	}
	shardStatusCondition := admiralapiv1.ShardStatusCondition{
		Message:         "sync not started",
		Reason:          "notStarted",
		Status:          "false",
		Type:            "SyncComplete",
		LastUpdatedTime: v1.Now(),
	}
	shard := admiralapiv1.Shard{
		ObjectMeta: v1.ObjectMeta{Name: "shard-sample", Namespace: "admiral-sync"},
		Spec:       admiralapiv1.ShardSpec{Clusters: []admiralapiv1.ClusterShards{clusterShard}},
		Status: admiralapiv1.ShardStatus{
			ClustersMonitored: 1,
			Conditions:        []admiralapiv1.ShardStatusCondition{shardStatusCondition},
			FailureDetails:    admiralapiv1.FailureDetails{},
			LastUpdatedTime:   v1.Time{},
		},
	}
	return &shard
}

func TestShardController_Added(t *testing.T) {
	shard := GetMockShard()
	shardController, _ := getNewMockShardController()
	err := shardController.Added(context.Background(), shard)
	if err != nil {
		t.Errorf("err: %v", err)
	}
}

func TestShardController_Deleted(t *testing.T) {
	shardController, _ := getNewMockShardController()
	shard := admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Name: "test"}}
	shard2 := admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Name: "test2"}}
	shard3 := admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Name: "test3"}}
	shardController.Cache.UpdateShardToClusterCache("test", &shard)
	shardController.Cache.UpdateShardToClusterCache("test3", &shard2)

	testCases := []struct {
		name             string
		shard            *admiralapiv1.Shard
		expectedErr      error
		expectedCacheLen int
	}{
		{
			name:             "Expects shard to be deleted from the cache",
			shard:            &shard,
			expectedErr:      nil,
			expectedCacheLen: 1,
		},
		{
			name:             "Given shard to be deleted name doesn't match shard name in cache, no delete or err",
			shard:            &shard3,
			expectedErr:      nil,
			expectedCacheLen: 1,
		},
		{
			name:             "Expects no error thrown if calling delete on a shard not in the cache",
			shard:            &shard2,
			expectedErr:      nil,
			expectedCacheLen: 1,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := shardController.Deleted(context.Background(), c.shard)
			if !errors.Is(err, c.expectedErr) {
				t.Errorf("Got err: %v but expected to get err: %v", err, c.expectedErr)
			}
			if c.expectedCacheLen != len(shardController.Cache.cache) {
				t.Errorf("Expected cache to have len: %v, but it had len: %v", c.expectedCacheLen, len(shardController.Cache.cache))
			}
		})
	}
}

func getNewMockShardController() (*ShardController, error) {
	shardHandler := test.MockShardHandler{}
	shardController, err := NewShardController(context.Background().Done(), &shardHandler, "../../test/resources/admins@fake-cluster.k8s.local", "test-ns", 0, loader.GetFakeClientLoader())
	return shardController, err
}

func TestNewShardController(t *testing.T) {
	shardController, _ := getNewMockShardController()
	if shardController == nil {
		t.Errorf("Shard controller should not be nil")
	}
}

func TestShardCache_Get(t *testing.T) {
	shardController, _ := getNewMockShardController()
	shard := admiralapiv1.Shard{
		ObjectMeta: v1.ObjectMeta{Name: "test"},
	}
	shardController.Cache.UpdateShardToClusterCache("test", &shard)
	testCases := []struct {
		name          string
		key           string
		expectedShard *admiralapiv1.Shard
	}{
		{
			name:          "Given the shard exists in the cache with matching key, returns the shard",
			key:           "test",
			expectedShard: &shard,
		},
		{
			name:          "Given there is no shard in the cache with matching key, returns nil",
			key:           "test2",
			expectedShard: nil,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			resultShard := shardController.Cache.Get(c.key)
			if c.expectedShard != resultShard {
				t.Errorf("Expected shard: %v, but got %v", c.expectedShard, resultShard)
			}
		})
	}
}

func TestUpdateProcessItemStatusShard(t *testing.T) {
	shardController, _ := getNewMockShardController()
	shardController.Cache.UpdateShardToClusterCache("test", &admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Name: "test"}})
	testCases := []struct {
		name           string
		obj            interface{}
		statusToSet    string
		expectedErr    error
		expectedStatus string
	}{
		{
			name: "Given shard cache has a valid shard in its cache, " +
				"Then, the status for the valid shard should be updated to processed",
			obj:            &admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Name: "test"}},
			statusToSet:    common.Processed,
			expectedErr:    nil,
			expectedStatus: common.Processed,
		},
		{
			name: "Given shard cache has a valid shard in its cache, " +
				"Then, the status for the valid shard should be updated to not processed",
			obj:            &admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Name: "test"}},
			statusToSet:    common.NotProcessed,
			expectedErr:    nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given shard cache does not have a valid shard in its cache, " +
				"Then, the status for the valid shard should be not processed, " +
				"And an error should be returned with the shard not found message",
			obj:            &admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Name: "test2", Namespace: "test-ns"}},
			statusToSet:    common.NotProcessed,
			expectedErr:    fmt.Errorf(LogCacheFormat, "Update", "Shard", "test2", "test-ns", "", "nothing to update, shard not found in cache"),
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given non-shard obj is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:            "not a shard",
			expectedErr:    fmt.Errorf("type assertion failed"),
			expectedStatus: common.NotProcessed,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := shardController.UpdateProcessItemStatus(c.obj, c.statusToSet)
			if c.expectedErr != nil && err == nil || c.expectedErr == nil && err != nil {
				t.Errorf("expected error: %v but got error: %v", c.expectedErr, err)
			}
			status, _ := shardController.GetProcessItemStatus(c.obj)
			assert.Equal(t, c.expectedStatus, status)
		})
	}
}

func TestGetProcessItemStatusShard(t *testing.T) {
	shardController, _ := getNewMockShardController()
	shard := admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Name: "test"}}
	shardController.Cache.UpdateShardToClusterCache("test", &shard)
	shardController.Cache.UpdateShardProcessStatus(&shard, common.Processed)
	testCases := []struct {
		name           string
		obj            interface{}
		expectedErr    error
		expectedResult string
	}{
		{
			name: "Given shard cache has a valid Shard in its cache, " +
				"And the Shard is processed" +
				"Then, we should be able to get the status as processed",
			obj:            &admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Name: "test"}},
			expectedResult: common.Processed,
		},
		{
			name: "Given shard cache does not have a valid shard in its cache, " +
				"Then, the returned status should be not processed",
			obj:            &admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Name: "test2"}},
			expectedResult: common.NotProcessed,
		},
		{
			name: "Given non-shard obj is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:            "not a shard",
			expectedErr:    fmt.Errorf("type assertion failed"),
			expectedResult: common.NotProcessed,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			res, err := shardController.GetProcessItemStatus(c.obj)
			if c.expectedErr != nil && err == nil || c.expectedErr == nil && err != nil {
				t.Errorf("expected error: %v but got error: %v", c.expectedErr, err)
			}
			assert.Equal(t, c.expectedResult, res)
		})
	}
}

func TestShardLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a Shard object
	s := &ShardController{}
	s.LogValueOfAdmiralIoIgnore("not a shard")
	// No error should occur

	// Test case 2: K8sClient is nil
	s = &ShardController{}
	s.LogValueOfAdmiralIoIgnore(&admiralapiv1.Shard{})
	// No error should occur

	// Test case 3: Namespace is not found
	mockClient := fake.NewSimpleClientset()
	s = &ShardController{K8sClient: mockClient}
	s.LogValueOfAdmiralIoIgnore(&admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Namespace: "test-ns"}})
	// No error should occur

	// Test case 4: AdmiralIgnoreAnnotation is not set
	mockClient = fake.NewSimpleClientset(&coreV1.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test-ns"}})
	s = &ShardController{K8sClient: mockClient}
	s.LogValueOfAdmiralIoIgnore(&admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Namespace: "test-ns"}})
	// No error should occur

	// Test case 5: AdmiralIgnoreAnnotation is set in Shard object
	mockClient = fake.NewSimpleClientset(&coreV1.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test-ns"}})
	s = &ShardController{K8sClient: mockClient}
	shard := &admiralapiv1.Shard{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "test-ns",
			Annotations: map[string]string{
				common.AdmiralIgnoreAnnotation: "true",
			},
		},
	}
	s.LogValueOfAdmiralIoIgnore(shard)
	// No error should occur

	// Test case 6: AdmiralIgnoreAnnotation is set in Namespace object
	mockClient = fake.NewSimpleClientset(&coreV1.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-ns",
			Annotations: map[string]string{
				common.AdmiralIgnoreAnnotation: "true",
			},
		},
	})
	s = &ShardController{K8sClient: mockClient}
	shard = &admiralapiv1.Shard{ObjectMeta: v1.ObjectMeta{Namespace: "test-ns"}}
	s.LogValueOfAdmiralIoIgnore(shard)
	// No error should occur
}
