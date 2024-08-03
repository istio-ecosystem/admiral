package clusters

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	v1 "github.com/istio-ecosystem/admiral/admiral/apis/v1"
	"github.com/stretchr/testify/assert"
)

func TestNewIgnoreIdentityStateChecker(t *testing.T) {
	var dummyDynamoClientFunc = func(role, region string) (*DynamoClient, error) {
		return nil, nil
	}
	testCases := []struct {
		name              string
		admiralConfigPath string
		expectedErr       error
	}{
		{
			name: "Given admiral config contains all required configurations, " +
				"When NewIgnoreIdentityStateChecker is called, " +
				"Then it should initialize ignoreIdentityStateChecker and not return any error",
			admiralConfigPath: "testdata/admiralIgnoreIdentityStateChecker_admiralConfig_is_valid.yaml",
			expectedErr:       nil,
		},
		{
			name: "Given admiral config is empty, " +
				"When NewIgnoreIdentityStateChecker is called, " +
				"Then it should fail with the expected error",
			admiralConfigPath: "testdata/admiralIgnoreIdentityStateChecker_admiralConfig_is_nil.yaml",
			expectedErr:       fmt.Errorf("failed validating admiral config, err: admiralConfig cannot be nil"),
		},
		{
			name: "Given ignoreIdentityList configuration in admiral config is empty, " +
				"When NewIgnoreIdentityStateChecker is called, " +
				"Then it should fail with the expected error",
			admiralConfigPath: "testdata/admiralIgnoreIdentityStateChecker_ignoreIdentityList_is_empty.yaml",
			expectedErr:       fmt.Errorf("failed validating admiral config, err: ignoreIdentityList cannot be empty"),
		},
		{
			name: "Given ignoreIdentityList.dynamoDB configuration in admiral config is empty, " +
				"When NewIgnoreIdentityStateChecker is called, " +
				"Then it should fail with the expected error",
			admiralConfigPath: "testdata/admiralIgnoreIdentityStateChecker_ignoreIdentityList.dynamoDB_is_empty.yaml",
			expectedErr:       fmt.Errorf("failed validating admiral config, err: ignoreIdentityList.dynamoDB cannot be empty"),
		},
		{
			name: "Given ignoreIdentityList.dynamoDB.Role configuration in admiral is empty, " +
				"When NewIgnoreIdentityStateChecker is called, " +
				"Then it should fail with the expected error",
			admiralConfigPath: "testdata/admiralIgnoreIdentityStateChecker_ignoreIdentityList.dynamoDB.Role_is_empty.yaml",
			expectedErr: fmt.Errorf("failed validating admiral config, err: ignoreIdentityList.dynamoDB is not configured correctly. expect all properties to be present, got: %+v",
				v1.DynamoDB{
					TableName: "test-db-1", Region: "us-east-2", Role: "", ClusterEnvironment: "dev",
				}),
		},
		{
			name: "Given ignoreIdentityList.dynamoDB.Region configuration in admiral is empty, " +
				"When NewIgnoreIdentityStateChecker is called, " +
				"Then it should fail with the expected error",
			admiralConfigPath: "testdata/admiralIgnoreIdentityStateChecker_ignoreIdentityList.dynamoDB.Region_is_empty.yaml",
			expectedErr: fmt.Errorf("failed validating admiral config, err: ignoreIdentityList.dynamoDB is not configured correctly. expect all properties to be present, got: %+v",
				v1.DynamoDB{
					TableName: "test-db-1", Region: "", Role: "arn:aws:iam::1111111:role/Admiral-IKS-Dynamo-Read-Access", ClusterEnvironment: "dev",
				}),
		},
		{
			name: "Given ignoreIdentityList.dynamoDB.TableName configuration in admiral is empty, " +
				"When NewIgnoreIdentityStateChecker is called, " +
				"Then it should fail with the expected error",
			admiralConfigPath: "testdata/admiralIgnoreIdentityStateChecker_ignoreIdentityList.dynamoDB.TableName_is_empty.yaml",
			expectedErr: fmt.Errorf("failed validating admiral config, err: ignoreIdentityList.dynamoDB is not configured correctly. expect all properties to be present, got: %+v",
				v1.DynamoDB{
					TableName: "", Region: "us-east-2", Role: "arn:aws:iam::1111111:role/Admiral-IKS-Dynamo-Read-Access", ClusterEnvironment: "dev",
				}),
		},
		{
			name: "Given ignoreIdentityList.dynamoDB.ClusterEnvironment configuration in admiral is empty, " +
				"When NewIgnoreIdentityStateChecker is called, " +
				"Then it should fail with the expected error",
			admiralConfigPath: "testdata/admiralIgnoreIdentityStateChecker_ignoreIdentityList.dynamoDB.ClusterEnvironment_is_empty.yaml",
			expectedErr: fmt.Errorf("failed validating admiral config, err: ignoreIdentityList.dynamoDB is not configured correctly. expect all properties to be present, got: %+v",
				v1.DynamoDB{
					TableName: "admiral-ignore-identity-state", Region: "us-east-2", Role: "arn:aws:iam::1111111:role/Admiral-IKS-Dynamo-Read-Access",
				}),
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewIgnoreIdentityStateChecker(c.admiralConfigPath, dummyDynamoClientFunc)
			if c.expectedErr != nil {
				assert.EqualError(t, err, c.expectedErr.Error())
			} else {
				if err != c.expectedErr {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
			}
		})
	}
}

func TestIgnoreIdentityStateCheckerGetState(t *testing.T) {
	var (
		dummyDynamoClientFunc = func(role, region string) (*DynamoClient, error) {
			return nil, nil
		}
		dummyIdentityListItemState = IgnoredIdentityCache{
			Enabled:                false,
			All:                    false,
			EnvironmentsByIdentity: map[string][]string{},
		}
		failedToFetchStateErr = errors.New("unable to fetch state")
		validAdmiralConfig    = "testdata/admiralIgnoreIdentityStateChecker_admiralConfig_is_valid.yaml"
	)
	checker, err := NewIgnoreIdentityStateChecker(validAdmiralConfig, dummyDynamoClientFunc)
	if err != nil {
		t.Errorf("failed to initialized ignore identity state checker, err: %v", err)
	}
	testCases := []struct {
		name          string
		stateFunc     func(client *DynamoClient, tableName, clusterEnvironment string) ([]IgnoredIdentityCache, error)
		checker       *ignoreIdentityStateChecker
		expectedState IgnoredIdentityCache
		expectedErr   error
	}{
		{
			name: "Given ignore identity state is valid" +
				"When getState is called, " +
				"Then, it should not return any errors",
			stateFunc: func(client *DynamoClient, tableName, clusterEnvironment string) ([]IgnoredIdentityCache, error) {
				return []IgnoredIdentityCache{dummyIdentityListItemState}, nil
			},
			checker:       checker,
			expectedState: dummyIdentityListItemState,
			expectedErr:   nil,
		},
		{
			name: "Given fetching ignore identity state results in an error" +
				"When getState is called, " +
				"Then, it should return the expected error",
			stateFunc: func(client *DynamoClient, tableName, clusterEnvironment string) ([]IgnoredIdentityCache, error) {
				return []IgnoredIdentityCache{}, failedToFetchStateErr
			},
			checker:     checker,
			expectedErr: fmt.Errorf("op=ignoreIdentityStateChecker message=error retrieving items, err: %v", failedToFetchStateErr),
		},
		{
			name: "Given ignore identity state is empty" +
				"When getState is called, " +
				"Then, it should return the expected error",
			stateFunc: func(client *DynamoClient, tableName, clusterEnvironment string) ([]IgnoredIdentityCache, error) {
				return []IgnoredIdentityCache{}, nil
			},
			checker: checker,
			expectedErr: fmt.Errorf(
				"op=ignoreIdentityStateChecker message=expected table: 'admiral-ignore-identity-state' to contain 1 item, got: %v",
				0,
			),
		},
		{
			name: "Given ignore identity state has more than 1 item " +
				"When setState is called, " +
				"Then, it should use the first element to set ",
			stateFunc: func(client *DynamoClient, tableName, clusterEnvironment string) ([]IgnoredIdentityCache, error) {
				return []IgnoredIdentityCache{
					{Enabled: true}, {Enabled: false},
				}, nil
			},
			checker: checker,
			expectedState: IgnoredIdentityCache{
				Enabled: true,
			},
			expectedErr: nil,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			c.checker.stateFetcher = c.stateFunc
			state, err := c.checker.getState()
			if c.expectedErr == nil {
				if err != nil {
					t.Errorf("expected error to be: nil, got: %v", err)
				}
			}
			if c.expectedErr != nil {
				if err == nil {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
				if err.Error() != c.expectedErr.Error() {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
			}
			if !reflect.DeepEqual(state, c.expectedState) {
				t.Errorf("expected state to be: %v, got: %v", c.expectedState, state)
			}
		})
	}
}

func TestIgnoreIdentityStateCheckerRunStateCheck(t *testing.T) {
	var (
		dummyDynamoClientFunc = func(role, region string) (*DynamoClient, error) {
			return nil, nil
		}
		cache = &IgnoredIdentityCache{
			RWLock: &sync.RWMutex{},
		}
		validAdmiralConfig             = "testdata/admiralIgnoreIdentityStateChecker_admiralConfig_is_valid.yaml"
		expectedIgnoreIdentityListItem = IgnoredIdentityCache{
			RWLock:  &sync.RWMutex{},
			Enabled: true,
			All:     false,
			EnvironmentsByIdentity: map[string][]string{
				"identity2": {"environment2"},
			},
		}
	)
	checker, err := NewIgnoreIdentityStateChecker(validAdmiralConfig, dummyDynamoClientFunc)
	if err != nil {
		t.Errorf("failed to initialized ignore identity state checker, err: %v", err)
	}
	checker.stateFetcher = func(client *DynamoClient, tableName, clusterEnvironment string) ([]IgnoredIdentityCache, error) {
		return []IgnoredIdentityCache{expectedIgnoreIdentityListItem}, nil
	}
	checker.initStateCache(cache)
	testCases := []struct {
		name                               string
		checker                            *ignoreIdentityStateChecker
		expectedCurrentIgnoreIdentityState IgnoredIdentityCache
	}{
		{
			name: "Given valid admiral config, " +
				"When runStateCheck is called, " +
				"Then it should set stateCache should be set to expected value",
			checker:                            checker,
			expectedCurrentIgnoreIdentityState: expectedIgnoreIdentityListItem,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			go c.checker.runStateCheck(ctx)
			count := 0
			max := 10
			for count <= max {
				time.Sleep(1 * time.Millisecond)
				checker.stateCache.RWLock.RLock()
				defer checker.stateCache.RWLock.RUnlock()
				if checker.stateCache.Enabled != c.expectedCurrentIgnoreIdentityState.Enabled {
					if count == max {
						t.Errorf("expected state cache.Enabled to be: %v, got: %v",
							c.expectedCurrentIgnoreIdentityState.Enabled, checker.stateCache.Enabled)
					}
				}
				if checker.stateCache.All != c.expectedCurrentIgnoreIdentityState.All {
					if count == max {
						t.Errorf("expected state cache.All to be: %v, got: %v",
							c.expectedCurrentIgnoreIdentityState.All, checker.stateCache.All)
					}
				}
				if !reflect.DeepEqual(checker.stateCache.EnvironmentsByIdentity, c.expectedCurrentIgnoreIdentityState.EnvironmentsByIdentity) {
					if count == max {
						t.Errorf("expected state cache.IdentitiesByEnvironment to be: %v, got: %v",
							c.expectedCurrentIgnoreIdentityState.EnvironmentsByIdentity, checker.stateCache.EnvironmentsByIdentity)
					}
				}
				count++
			}
			cancel()
		})
	}
}
