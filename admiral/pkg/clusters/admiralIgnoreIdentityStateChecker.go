package clusters

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	v1 "github.com/istio-ecosystem/admiral/admiral/apis/v1"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type ignoreIdentityStateChecker struct {
	stateCache    *IgnoredIdentityCache
	admiralConfig *v1.AdmiralConfig
	dynamoClient  *DynamoClient
	stateFetcher  func(client *DynamoClient, tableName, clusterEnvironment string) ([]IgnoredIdentityCache, error)
}

func NewIgnoreIdentityStateChecker(configFile string, dynamoClientInitFunc func(string, string) (*DynamoClient, error)) (*ignoreIdentityStateChecker, error) {
	var (
		checker       = &ignoreIdentityStateChecker{}
		admiralConfig *v1.AdmiralConfig
	)
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return checker, fmt.Errorf("error reading admiral config file, err: %v", err)
	}
	err = yaml.Unmarshal(data, &admiralConfig)
	if err != nil {
		return checker, fmt.Errorf("error unmarshalling admiral config file, err: %v", err)
	}
	err = validateAdmiralConfig(admiralConfig)
	if err != nil {
		return checker, fmt.Errorf("failed validating admiral config, err: %v", err)
	}
	checker.admiralConfig = admiralConfig
	checker.stateFetcher = getIgnoreIdentityListItem
	checker.dynamoClient, err = dynamoClientInitFunc(
		checker.admiralConfig.IgnoreIdentityList.DynamoDB.Role,
		checker.admiralConfig.IgnoreIdentityList.DynamoDB.Region,
	)
	if err != nil {
		return checker, fmt.Errorf("unable to instantiate dynamo client, err: %v", err)
	}
	return checker, nil
}

func validateAdmiralConfig(admiralConfig *v1.AdmiralConfig) error {
	if admiralConfig == nil {
		return fmt.Errorf("admiralConfig cannot be nil")
	}
	if admiralConfig.IgnoreIdentityList == (v1.IgnoreIdentityList{}) {
		return fmt.Errorf("ignoreIdentityList cannot be empty")
	}
	if admiralConfig.IgnoreIdentityList.DynamoDB == (v1.DynamoDB{}) {
		return fmt.Errorf("ignoreIdentityList.dynamoDB cannot be empty")
	}
	if admiralConfig.IgnoreIdentityList.DynamoDB.Role == "" ||
		admiralConfig.IgnoreIdentityList.DynamoDB.Region == "" ||
		admiralConfig.IgnoreIdentityList.DynamoDB.TableName == "" ||
		admiralConfig.IgnoreIdentityList.DynamoDB.ClusterEnvironment == "" {
		return fmt.Errorf("ignoreIdentityList.dynamoDB is not configured correctly. expect all properties to be present, got: %+v", admiralConfig.IgnoreIdentityList.DynamoDB)
	}
	if admiralConfig.IgnoreIdentityList.StateCheckerPeriodInSeconds == 0 {
		return fmt.Errorf("ignoreIdentityList.stateCheckerPeriodInSeconds is either not set, or is set to 0. It should be set to a value greater than 0")
	}
	return nil
}

func (checker *ignoreIdentityStateChecker) initStateCache(cache interface{}) error {
	ignoredIdentityStateCache, ok := cache.(*IgnoredIdentityCache)
	if !ok {
		return fmt.Errorf("unable to set cache store")
	}
	defer ignoredIdentityStateCache.RWLock.RUnlock()
	ignoredIdentityStateCache.RWLock.RLock()
	checker.stateCache = ignoredIdentityStateCache
	return nil
}

func (checker *ignoreIdentityStateChecker) shouldRunOnIndependentGoRoutine() bool {
	return true
}

func (checker *ignoreIdentityStateChecker) runStateCheck(ctx context.Context) {
	period := time.Duration(checker.admiralConfig.IgnoreIdentityList.StateCheckerPeriodInSeconds) * time.Second
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	state, err := checker.getState()
	if err != nil {
		log.Info(err)
	} else {
		checker.syncState(state)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state, err := checker.getState()
			if err != nil {
				log.Info(err)
			} else {
				checker.syncState(state)
			}
		}
	}
}

func (checker *ignoreIdentityStateChecker) syncState(state IgnoredIdentityCache) {
	checker.stateCache.RWLock.Lock()
	defer checker.stateCache.RWLock.Unlock()
	state.RWLock.RLock()
	defer state.RWLock.RUnlock()
	checker.stateCache.All = state.All
	checker.stateCache.Enabled = state.Enabled
	checker.stateCache.ClusterEnvironment = state.ClusterEnvironment
	checker.stateCache.EnvironmentsByIdentity = state.EnvironmentsByIdentity
}

func (checker ignoreIdentityStateChecker) getState() (IgnoredIdentityCache, error) {
	var currentItem IgnoredIdentityCache
	items, err := checker.stateFetcher(
		checker.dynamoClient,
		checker.admiralConfig.IgnoreIdentityList.DynamoDB.TableName,
		checker.admiralConfig.IgnoreIdentityList.DynamoDB.ClusterEnvironment)
	if err != nil {
		return currentItem,
			fmt.Errorf("op=ignoreIdentityStateChecker message=error retrieving items, err: %v", err)
	}
	if len(items) == 0 {
		return currentItem,
			fmt.Errorf(
				"op=ignoreIdentityStateChecker message=expected table: '%s' to contain 1 item, got: %v",
				checker.admiralConfig.IgnoreIdentityList.DynamoDB.TableName, len(items))
	}
	if len(items) > 1 {
		log.Warnf("op=ignoreIdentityStateChecker message=expected %s to contain only one item, got: %v. will use the first item",
			checker.admiralConfig.IgnoreIdentityList.DynamoDB.TableName, len(items))
	}
	log.Infof("op=ignoreIdentityStateChecker message=successfully got ignore list item")
	return items[0], nil
}
