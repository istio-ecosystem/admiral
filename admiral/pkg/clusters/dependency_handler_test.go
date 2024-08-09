package clusters

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	"istio.io/api/networking/v1alpha3"
	k8sAppsV1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MockDestinationServiceProcessor struct {
	invocation int
}

func (m *MockDestinationServiceProcessor) Process(ctx context.Context, dependency *v1.Dependency,
	remoteRegistry *RemoteRegistry, eventType admiral.EventType, modifySE ModifySEFunc) error {
	m.invocation++
	return nil
}

func TestProcessDestinationService(t *testing.T) {

	admiralParams := common.AdmiralParams{
		CacheReconcileDuration: 10 * time.Minute,
		LabelSet: &common.LabelSet{
			EnvKey: "env",
		},
	}
	identityClusterCache := common.NewMapOfMaps()
	identityClusterCache.Put("foo", "testCluster", "testCluster")
	identityClusterCache.Put("bar", "testCluster", "testCluster")
	identityClusterCache.Put("testSource", "testCluster", "testCluster")
	identityClusterCache.Put("testSource", "testCluster1", "testCluster1")

	identityClusterCacheWithOnlyTestSource := common.NewMapOfMaps()
	identityClusterCacheWithOnlyTestSource.Put("testSource", "testCluster", "testCluster")

	identityClusterCacheWithServicesGateway := common.NewMapOfMaps()
	identityClusterCacheWithServicesGateway.Put("foo", "testCluster", "testCluster")
	identityClusterCacheWithServicesGateway.Put("bar", "testCluster", "testCluster")
	identityClusterCacheWithServicesGateway.Put("testSource", "testCluster", "testCluster")
	identityClusterCacheWithServicesGateway.Put(common.ServicesGatewayIdentity, "testCluster", "testCluster")
	identityClusterCacheWithServicesGateway.Put(strings.ToLower(common.ServicesGatewayIdentity), "testCluster", "testCluster")

	identityClusterCacheWithServicesGatewayAndFoo := common.NewMapOfMaps()
	identityClusterCacheWithServicesGatewayAndFoo.Put("foo", "testCluster", "testCluster")
	identityClusterCacheWithServicesGatewayAndFoo.Put("testSource", "testCluster", "testCluster")
	identityClusterCacheWithServicesGatewayAndFoo.Put(common.ServicesGatewayIdentity, "testCluster", "testCluster")

	deploymentCache := admiral.NewDeploymentCache()
	deploymentCache.UpdateDeploymentToClusterCache("foo", &k8sAppsV1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"env": "stage"},
		},
	})
	deploymentCache.UpdateDeploymentToClusterCache("bar", &k8sAppsV1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"env": "stage"},
		},
	})

	rolloutCache := admiral.NewRolloutCache()
	rolloutCache.UpdateRolloutToClusterCache("foo", &v1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"env": "stage"},
		},
	})
	rolloutCache.UpdateRolloutToClusterCache("bar", &v1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"env": "stage"},
		},
	})

	testCases := []struct {
		name                          string
		dependency                    *v1.Dependency
		modifySEFunc                  ModifySEFunc
		remoteRegistry                *RemoteRegistry
		isDependencyProcessingEnabled bool
		expectedError                 error
	}{
		{
			name: "Given valid params " +
				"When admiral is in cache warmup state " +
				"Then the func should just return without processing and no errors",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now(),
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				return nil, nil
			},
			expectedError: nil,
		},
		{
			name: "Given valid params " +
				"When dependency processing is disabled " +
				"Then the func should just return without processing and no errors",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 15),
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				return nil, nil
			},
			isDependencyProcessingEnabled: false,
			expectedError:                 nil,
		},
		{
			name: "Given valid params " +
				"When destination identity is not in IdentityClusterCache " +
				"Then the func should not return an error",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					IdentityClusterCache: identityClusterCacheWithOnlyTestSource,
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"testSource": {"foo"}},
						mutex:              &sync.Mutex{},
					},
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar"},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				return nil, nil
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 nil,
		},
		{
			name: "Given valid params " +
				"When destination identity's cluster is not in remote controller cache " +
				"Then the func should NOT return an error",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					IdentityClusterCache: identityClusterCache,
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"testSource": {"foo", "bar"}},
						mutex:              &sync.Mutex{},
					},
				},
				remoteControllers: map[string]*RemoteController{},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo"},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				return nil, nil
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 nil,
		},
		{
			name: "Given valid params " +
				"When destination identity is in the deployment controller cache " +
				"And the modifySE func returns an error " +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					IdentityClusterCache: identityClusterCache,
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"testSource": {"foo"}},
						mutex:              &sync.Mutex{},
					},
				},
				remoteControllers: map[string]*RemoteController{
					"testCluster": {
						DeploymentController: &admiral.DeploymentController{
							Cache: deploymentCache,
						},
					},
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar"},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				return nil, fmt.Errorf("error occurred while processing the deployment")
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 fmt.Errorf("error occurred while processing the deployment"),
		},
		{
			name: "Given valid params " +
				"When destination identity is in the rollout controller cache " +
				"And the modifySE func returns an error " +
				"Then the func should return an error",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					IdentityClusterCache: identityClusterCache,
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"testSource": {"foo"}},
						mutex:              &sync.Mutex{},
					},
				},
				remoteControllers: map[string]*RemoteController{
					"testCluster": {
						DeploymentController: &admiral.DeploymentController{
							Cache: admiral.NewDeploymentCache(),
						},
						RolloutController: &admiral.RolloutController{
							Cache: rolloutCache,
						},
					},
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar"},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				return nil, fmt.Errorf("error occurred while processing the rollout")
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 fmt.Errorf("error occurred while processing the rollout"),
		},
		{
			name: "Given valid params " +
				"When destination identity is in the rollout controller cache " +
				"And the modifySE func returns successfully " +
				"Then the func should not return an error",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					IdentityClusterCache: identityClusterCache,
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"testSource": {"foo", "bar"}},
						mutex:              &sync.Mutex{},
					},
				},
				remoteControllers: map[string]*RemoteController{
					"testCluster": {
						DeploymentController: &admiral.DeploymentController{
							Cache: admiral.NewDeploymentCache(),
						},
						RolloutController: &admiral.RolloutController{
							Cache: rolloutCache,
						},
					},
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testSource"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo"},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				return nil, nil
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 nil,
		},
		{
			name: "Given valid params " +
				"When destination identity is in the deployment controller cache " +
				"And the modifySE func returns successfully " +
				"Then the func should not return an error",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					IdentityClusterCache: identityClusterCache,
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"testSource": {"foo"}},
						mutex:              &sync.Mutex{},
					},
				},
				remoteControllers: map[string]*RemoteController{
					"testCluster": {
						DeploymentController: &admiral.DeploymentController{
							Cache: deploymentCache,
						},
					},
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testSource"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar"},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				return nil, nil
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 nil,
		},
		{
			name: "Given valid params " +
				"When a new destination is in the dependency record " +
				"Then the func should process only the new destination and not return an error",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"testSource": {"foo"}},
						mutex:              &sync.Mutex{},
					},
					IdentityClusterCache: identityClusterCache,
				},
				remoteControllers: map[string]*RemoteController{
					"testCluster": {
						DeploymentController: &admiral.DeploymentController{
							Cache: deploymentCache,
						},
					},
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar"},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				return nil, nil
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 nil,
		},
		{
			name: "Given valid params " +
				"When a new dependency record event is received, " +
				"When all destinations are MESH enabled, " +
				"Then, modifySE is not called for " + common.ServicesGatewayIdentity + " identity",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{},
						mutex:              &sync.Mutex{},
					},
					IdentityClusterCache: identityClusterCacheWithServicesGateway,
				},
				remoteControllers: map[string]*RemoteController{
					"testCluster": {
						DeploymentController: &admiral.DeploymentController{
							Cache: deploymentCache,
						},
					},
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar", common.ServicesGatewayIdentity},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				if sourceIdentity == common.ServicesGatewayIdentity {
					return nil, fmt.Errorf("did not expect to be called for %s", common.ServicesGatewayIdentity)
				}
				return nil, nil
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 nil,
		},
		{
			name: "Given valid params " +
				"When a new dependency record event is received, " +
				"When all destinations are MESH enabled, " +
				"When " + common.ServicesGatewayIdentity + " is in lower case in the dependency record, " +
				"Then, modifySE is not called for " + strings.ToLower(common.ServicesGatewayIdentity) + " identity",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{},
						mutex:              &sync.Mutex{},
					},
					IdentityClusterCache: identityClusterCacheWithServicesGateway,
				},
				remoteControllers: map[string]*RemoteController{
					"testCluster": {
						DeploymentController: &admiral.DeploymentController{
							Cache: deploymentCache,
						},
					},
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar", strings.ToLower(common.ServicesGatewayIdentity)},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				if strings.EqualFold(sourceIdentity, common.ServicesGatewayIdentity) {
					return nil, fmt.Errorf("did not expect to be called for %s", common.ServicesGatewayIdentity)
				}
				return nil, nil
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 nil,
		},
		{
			name: "Given valid params " +
				"When a new dependency record event is received, " +
				"When one destination is NOT MESH enabled, " +
				"Then, modifySE is called for " + common.ServicesGatewayIdentity + " identity",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"testSource": {"foo", "bar"}},
						mutex:              &sync.Mutex{},
					},
					IdentityClusterCache: identityClusterCacheWithServicesGatewayAndFoo,
				},
				remoteControllers: map[string]*RemoteController{
					"testCluster": {
						DeploymentController: &admiral.DeploymentController{
							Cache: deploymentCache,
						},
					},
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar", common.ServicesGatewayIdentity},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				if sourceIdentity == common.ServicesGatewayIdentity {
					return nil, nil
				}
				return nil, fmt.Errorf("was not called for %s", common.ServicesGatewayIdentity)
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 nil,
		},
		{
			name: "Given valid params " +
				"When a new dependency record event is received, " +
				"When dependency source does not have a cluster in cache, " +
				"Then, do not return an error",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{},
						mutex:              &sync.Mutex{},
					},
					IdentityClusterCache: identityClusterCache,
				},
				remoteControllers: map[string]*RemoteController{
					"testCluster": {
						DeploymentController: &admiral.DeploymentController{
							Cache: deploymentCache,
						},
					},
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar", common.ServicesGatewayIdentity},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				return nil, nil
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 nil,
		},
		{
			name: "Given valid params " +
				"When a new dependency record event is received, " +
				"When source does not have a cluster in cache, " +
				"Then, do not return an error, " +
				"And do not call modifySE",
			remoteRegistry: &RemoteRegistry{
				StartTime: time.Now().Add(-time.Minute * 30),
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{},
						mutex:              &sync.Mutex{},
					},
					IdentityClusterCache: common.NewMapOfMaps(),
				},
				remoteControllers: map[string]*RemoteController{
					"testCluster": {
						DeploymentController: &admiral.DeploymentController{
							Cache: deploymentCache,
						},
					},
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar", common.ServicesGatewayIdentity},
				},
			},
			modifySEFunc: func(ctx context.Context, event admiral.EventType, env string,
				sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*v1alpha3.ServiceEntry, error) {
				return nil, fmt.Errorf("this should not be called")
			},
			isDependencyProcessingEnabled: true,
			expectedError:                 nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			common.ResetSync()
			admiralParams.EnableDependencyProcessing = tc.isDependencyProcessingEnabled
			common.InitializeConfig(admiralParams)

			processDestinationService := &ProcessDestinationService{}

			actualErr := processDestinationService.Process(context.TODO(), tc.dependency, tc.remoteRegistry, admiral.Add, tc.modifySEFunc)

			if tc.expectedError != nil {
				assert.NotNil(t, actualErr)
				assert.Equal(t, tc.expectedError.Error(), actualErr.Error())
			} else {
				assert.Nil(t, actualErr)
			}
		})
	}

}

func TestGetDestinationDiff(t *testing.T) {
	var (
		identityClusterCacheWithOnlyFoo        = common.NewMapOfMaps()
		identityClusterCacheWithAllMeshEnabled = common.NewMapOfMaps()
	)
	identityClusterCacheWithOnlyFoo.Put("foo", "cluster1", "cluster1")
	identityClusterCacheWithAllMeshEnabled.Put("foo", "cluster1", "cluster1")
	identityClusterCacheWithAllMeshEnabled.Put("bar", "cluster1", "cluster1")
	testCases := []struct {
		name                     string
		remoteRegistry           *RemoteRegistry
		dependency               *v1.Dependency
		expectedDestinations     []string
		expectedIsNonMeshEnabled bool
	}{
		{
			name: "Given valid params " +
				"When the cache is empty" +
				"Then the func should return all the destinations as is",
			remoteRegistry: &RemoteRegistry{
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{},
						mutex:              &sync.Mutex{},
					},
					IdentityClusterCache: identityClusterCacheWithAllMeshEnabled,
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar"},
				},
			},
			expectedDestinations: []string{"foo", "bar"},
		},
		{
			name: "Given valid params" +
				"When all the destinations are already in the cache" +
				"Then the func should return an empty list",
			remoteRegistry: &RemoteRegistry{
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"testSource": {"foo", "bar"}},
						mutex:              &sync.Mutex{},
					},
					IdentityClusterCache: identityClusterCacheWithAllMeshEnabled,
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar"},
				},
			},
			expectedDestinations: []string{},
		},
		{
			name: "Given valid params" +
				"When there is an additional destination that is not in the cache" +
				"Then the func should return only the one that is missing in the cache",
			remoteRegistry: &RemoteRegistry{
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"testSource": {"foo"}},
						mutex:              &sync.Mutex{},
					},
					IdentityClusterCache: identityClusterCacheWithAllMeshEnabled,
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar"},
				},
			},
			expectedDestinations: []string{"bar"},
		},
		{
			name: "Given valid params" +
				"When there is a NON mesh enabled service" +
				"Then the function should return new services, and true",
			remoteRegistry: &RemoteRegistry{
				AdmiralCache: &AdmiralCache{
					SourceToDestinations: &sourceToDestinations{
						sourceDestinations: map[string][]string{"testSource": {"foo"}},
						mutex:              &sync.Mutex{},
					},
					IdentityClusterCache: identityClusterCacheWithOnlyFoo,
				},
			},
			dependency: &v1.Dependency{
				ObjectMeta: metav1.ObjectMeta{Name: "testDepRec"},
				Spec: model.Dependency{
					Source:       "testSource",
					Destinations: []string{"foo", "bar"},
				},
			},
			expectedDestinations:     []string{"bar"},
			expectedIsNonMeshEnabled: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualDestinations, nonMeshEnabledExists := getDestinationsToBeProcessed(tc.dependency, tc.remoteRegistry)
			assert.Equal(t, tc.expectedDestinations, actualDestinations)
			assert.Equal(t, tc.expectedIsNonMeshEnabled, nonMeshEnabledExists)
		})
	}

}
