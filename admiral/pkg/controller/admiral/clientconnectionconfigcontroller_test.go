package admiral

import (
	"context"
	"fmt"
	"sync"
	"testing"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	admiralv1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned/typed/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	apiMachineryMetaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

func TestNewClientConnectionConfigController(t *testing.T) {

	testCases := []struct {
		name                            string
		clientConnectionSettingsHandler ClientConnectionConfigHandlerInterface
		configPath                      *rest.Config
		expectedError                   error
	}{
		{
			name: "Given valid params " +
				"When NewClientConnectionConfigController func is called " +
				"Then func should return ClientConnectionConfigController and no  error",
			configPath:                      &rest.Config{},
			clientConnectionSettingsHandler: &MockClientConnectionHandler{},
			expectedError:                   nil,
		},
	}
	stop := make(chan struct{})
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actualClientConnectionConfigController, actualError := NewClientConnectionConfigController(
				stop, tc.clientConnectionSettingsHandler, tc.configPath, 0, loader.GetFakeClientLoader())
			if tc.expectedError != nil {
				if actualError == nil {
					t.Fatalf("expected %s error got nil error", tc.expectedError)
				}
				assert.Equal(t, tc.expectedError.Error(), actualError.Error())
			} else {
				assert.NotNil(t, actualClientConnectionConfigController)
			}

		})
	}

}

func TestGetClientConnectionConfigController(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)

	testCases := []struct {
		name                               string
		clientConnectionSettings           interface{}
		ctx                                context.Context
		clientConnectionSettingsController *ClientConnectionConfigController
		expectedError                      error
	}{
		{
			name: "Given a ClientConnectionConfigController " +
				"When invalid object is passed to Get func " +
				"Then then the func should return an error",
			clientConnectionSettings: &struct{}{},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{},
					mutex: &sync.RWMutex{},
				},
			},
			ctx:           context.WithValue(context.Background(), "txId", "999"),
			expectedError: fmt.Errorf("type assertion failed, &{} is not of type *v1.ClientConnectionConfig"),
		},
		{
			name: "Given a ClientConnectionConfigController " +
				"When valid ClientConnectionConfig object is passed to Get func " +
				"And crdClient is nil on the ClientConnectionConfigController " +
				"Then then the func should return an error",
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{},
					mutex: &sync.RWMutex{},
				},
			},
			ctx:           context.WithValue(context.Background(), "txId", "999"),
			expectedError: fmt.Errorf("crd client is not initialized, txId=999"),
		},
		{
			name: "Given a ClientConnectionConfigController " +
				"When valid ClientConnectionConfig object is passed to Get func " +
				"Then then the func should not return any errors",
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				crdClient: &MockCRDClient{},
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
						"testEnv.testId": {
							"testns": {"ccsName": &clientConnectionSettingsItem{
								clientConnectionSettings: &v1.ClientConnectionConfig{
									ObjectMeta: apiMachineryMetaV1.ObjectMeta{
										Name:      "ccsName",
										Namespace: "testns",
										Labels: map[string]string{
											"admiral.io/env": "testEnv",
											"identity":       "testId",
										},
									},
								},
								status: common.ProcessingInProgress,
							}},
						},
					},
					mutex: &sync.RWMutex{},
				},
			},
			ctx: context.WithValue(context.Background(), "ClientConnectionConfig",
				&v1.ClientConnectionConfig{
					ObjectMeta: apiMachineryMetaV1.ObjectMeta{
						Name:      "ccsName",
						Namespace: "testns",
						Labels: map[string]string{
							"admiral.io/env": "testEnv",
							"identity":       "testId",
						},
					},
				}),
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, actualError := tc.clientConnectionSettingsController.Get(
				tc.ctx, false, tc.clientConnectionSettings)
			if actualError != nil {
				assert.Equal(t, tc.expectedError, actualError)
			} else {
				assert.NotNil(t, actual.(*v1.ClientConnectionConfig))
			}

		})
	}

}

func TestGetProcessItemStatusClientConnectionConfigController(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)

	testCases := []struct {
		name                               string
		clientConnectionSettings           interface{}
		clientConnectionSettingsController *ClientConnectionConfigController
		expectedstatus                     string
		expectedError                      error
	}{
		{
			name: "Given a ClientConnectionConfigController " +
				"When invalid object is passed to GetProcessItemStatus func " +
				"Then then the func should return an error",
			clientConnectionSettings: &struct{}{},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{},
					mutex: &sync.RWMutex{},
				},
			},
			expectedError: fmt.Errorf("type assertion failed, &{} is not of type *v1.ClientConnectionConfig"),
		},
		{
			name: "Given a ClientConnectionConfigController " +
				"When valid ClientConnectionConfig and status is passed to GetProcessItemStatus func " +
				"Then then the func should not return any errors and return the status",
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
						"testEnv.testId": {
							"testns": {"ccsName": &clientConnectionSettingsItem{
								clientConnectionSettings: &v1.ClientConnectionConfig{
									ObjectMeta: apiMachineryMetaV1.ObjectMeta{
										Name:      "ccsName",
										Namespace: "testns",
										Labels: map[string]string{
											"admiral.io/env": "testEnv",
											"identity":       "testId",
										},
									},
								},
								status: common.ProcessingInProgress,
							}},
						},
					},
					mutex: &sync.RWMutex{},
				},
			},
			expectedError:  nil,
			expectedstatus: common.ProcessingInProgress,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualStatus, actualError := tc.clientConnectionSettingsController.GetProcessItemStatus(tc.clientConnectionSettings)
			if actualError != nil {
				assert.Equal(t, tc.expectedError, actualError)
			} else {
				assert.Equal(t, tc.expectedstatus, actualStatus)
			}

		})
	}

}

func TestUpdateProcessItemStatusClientConnectionConfigController(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)

	testCases := []struct {
		name                               string
		clientConnectionSettings           interface{}
		clientConnectionSettingsController *ClientConnectionConfigController
		status                             string
		expectedError                      error
	}{
		{
			name: "Given a ClientConnectionConfigController " +
				"When invalid object is passed to UpdateProcessItemStatus func " +
				"Then then the func should return an error",
			clientConnectionSettings: &struct{}{},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{},
					mutex: &sync.RWMutex{},
				},
			},
			expectedError: fmt.Errorf("type assertion failed, &{} is not of type *v1.ClientConnectionConfig"),
		},
		{
			name: "Given a ClientConnectionConfigController " +
				"When valid ClientConnectionConfig and status is passed to UpdateProcessItemStatus func " +
				"Then then the func should not return any errors and update the status",
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
						"testEnv.testId": {
							"testns": {"ccsName": &clientConnectionSettingsItem{
								clientConnectionSettings: &v1.ClientConnectionConfig{
									ObjectMeta: apiMachineryMetaV1.ObjectMeta{
										Name:      "ccsName",
										Namespace: "testns",
										Labels: map[string]string{
											"admiral.io/env": "testEnv",
											"identity":       "testId",
										},
									},
								},
								status: common.ProcessingInProgress,
							}},
						},
					},
					mutex: &sync.RWMutex{},
				},
			},
			expectedError: nil,
			status:        common.Processed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualError := tc.clientConnectionSettingsController.UpdateProcessItemStatus(
				tc.clientConnectionSettings, tc.status)
			if actualError != nil {
				assert.Equal(t, tc.expectedError, actualError)
			} else {
				actualStatus := tc.clientConnectionSettingsController.Cache.GetStatus(
					tc.clientConnectionSettings.(*v1.ClientConnectionConfig))
				assert.Equal(t, tc.status, actualStatus)
			}

		})
	}

}

func TestDeletedClientConnectionConfigController(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)

	testCases := []struct {
		name                               string
		clientConnectionSettings           interface{}
		clientConnectionSettingsController *ClientConnectionConfigController
		expectedError                      error
	}{
		{
			name: "Given a ClientConnectionConfigController " +
				"When invalid object is passed to Deleted func " +
				"Then then the func should return an error",
			clientConnectionSettings: &struct{}{},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				clientConnectionSettingsHandler: &MockClientConnectionHandler{},
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{},
					mutex: &sync.RWMutex{},
				},
			},
			expectedError: fmt.Errorf("type assertion failed, &{} is not of type *v1.ClientConnectionConfig"),
		},
		{
			name: "Given a ClientConnectionConfigController " +
				"When valid ClientConnectionConfig is passed to Deleted func " +
				"Then then the func should not return any errors",
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				clientConnectionSettingsHandler: &MockClientConnectionHandler{},
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{},
					mutex: &sync.RWMutex{},
				},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actualError := tc.clientConnectionSettingsController.Deleted(context.Background(),
				tc.clientConnectionSettings)
			assert.Equal(t, tc.expectedError, actualError)

		})
	}

}

func TestUpdatedClientConnectionConfigController(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)

	testCases := []struct {
		name                               string
		clientConnectionSettings           interface{}
		clientConnectionSettingsController *ClientConnectionConfigController
		expectedError                      error
	}{
		{
			name: "Given a ClientConnectionConfigController " +
				"When invalid object is passed to Updated func " +
				"Then then the func should return an error",
			clientConnectionSettings: &struct{}{},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				clientConnectionSettingsHandler: &MockClientConnectionHandler{},
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{},
					mutex: &sync.RWMutex{},
				},
			},
			expectedError: fmt.Errorf("type assertion failed, &{} is not of type *v1.ClientConnectionConfig"),
		},
		{
			name: "Given a ClientConnectionConfigController " +
				"When valid ClientConnectionConfig is passed to Updated func " +
				"Then then the func should not return any errors",
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				clientConnectionSettingsHandler: &MockClientConnectionHandler{},
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{},
					mutex: &sync.RWMutex{},
				},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actualError := tc.clientConnectionSettingsController.Updated(context.Background(),
				tc.clientConnectionSettings, nil)
			assert.Equal(t, tc.expectedError, actualError)

		})
	}

}

func TestAddedClientConnectionConfigController(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)

	testCases := []struct {
		name                               string
		clientConnectionSettings           interface{}
		clientConnectionSettingsController *ClientConnectionConfigController
		expectedError                      error
	}{
		{
			name: "Given a ClientConnectionConfigController " +
				"When invalid object is passed to Added func " +
				"Then then the func should return an error",
			clientConnectionSettings: &struct{}{},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				clientConnectionSettingsHandler: &MockClientConnectionHandler{},
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{},
					mutex: &sync.RWMutex{},
				},
			},
			expectedError: fmt.Errorf("type assertion failed, &{} is not of type *v1.ClientConnectionConfig"),
		},
		{
			name: "Given a ClientConnectionConfigController " +
				"When valid ClientConnectionConfig is passed to Added func " +
				"Then then the func should not return any errors",
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			clientConnectionSettingsController: &ClientConnectionConfigController{
				clientConnectionSettingsHandler: &MockClientConnectionHandler{},
				Cache: &clientConnectionSettingsCache{
					cache: map[string]map[string]map[string]*clientConnectionSettingsItem{},
					mutex: &sync.RWMutex{},
				},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actualError := tc.clientConnectionSettingsController.Added(context.Background(), tc.clientConnectionSettings)
			assert.Equal(t, tc.expectedError, actualError)

		})
	}

}

func TestUpdateStatus(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)

	testCases := []struct {
		name                          string
		clientConnectionSettingsCache *clientConnectionSettingsCache
		clientConnectionSettings      *v1.ClientConnectionConfig
		status                        string
		expectedError                 error
	}{
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a ClientConnectionConfig, and status is passed to the UpdateStatus func " +
				"And the key does not exists in the cache " +
				"Then the func should return an error",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"testns": {"ccsName": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccsName",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "bar",
					Namespace: "barns",
					Labels: map[string]string{
						"admiral.io/env": "foo",
						"identity":       "bar",
					},
				},
			},
			status: common.NotProcessed,
			expectedError: fmt.Errorf(
				"op=Update type=ClientConnectionConfig name=bar namespace=barns cluster= " +
					"message=skipped updating status in cache, clientConnectionSettings not found in cache"),
		},
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a ClientConnectionConfig, and status is passed to the UpdateStatus func " +
				"And the matching namespace does not exists in the cache " +
				"Then the func should return an error",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"testns": {"ccsName": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccsName",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "barns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			status: common.NotProcessed,
			expectedError: fmt.Errorf(
				"op=Update type=ClientConnectionConfig name=ccsName namespace=barns cluster= " +
					"message=skipped updating status in cache, clientConnectionSettings namespace not found in cache"),
		},
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a ClientConnectionConfig, and status is passed to the UpdateStatus func " +
				"And the matching name does not exists in the cache " +
				"Then the func should return an error",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"testns": {"ccsName": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccsName",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccs0",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			status: common.NotProcessed,
			expectedError: fmt.Errorf(
				"op=Update type=ClientConnectionConfig name=ccs0 namespace=testns cluster= " +
					"message=skipped updating status in cache, clientConnectionSettings not found in cache with the specified name"),
		},
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a valid ClientConnectionConfig, and status is passed to the UpdateStatus func " +
				"Then the func should updated the status and not return an error",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"testns": {"ccsName": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccsName",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			status:        common.NotProcessed,
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actualError := tc.clientConnectionSettingsCache.UpdateStatus(tc.clientConnectionSettings, tc.status)

			if actualError != nil {
				assert.Equal(t, tc.expectedError.Error(), actualError.Error())
			} else {
				actualStatus := tc.clientConnectionSettingsCache.GetStatus(tc.clientConnectionSettings)
				assert.Equal(t, tc.status, actualStatus)
			}

		})
	}

}

func TestGetStatus(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)

	testCases := []struct {
		name                          string
		clientConnectionSettingsCache *clientConnectionSettingsCache
		clientConnectionSettings      *v1.ClientConnectionConfig
		expectedStatus                string
	}{
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a ClientConnectionConfig is passed to the GetStatus func " +
				"And the key does not exists in the cache " +
				"Then the func should return NotProcessed as the status",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"testns": {"ccsName": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccsName",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "bar",
					Namespace: "barns",
					Labels: map[string]string{
						"admiral.io/env": "foo",
						"identity":       "bar",
					},
				},
			},
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a ClientConnectionConfig is passed to the GetStatus func " +
				"And there is no matching clientConnectionSetting in the cache for the given NS" +
				"Then the func should return NotProcessed as the status",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"testns": {"ccsName": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccsName",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "testId",
					Namespace: "barns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a ClientConnectionConfig is passed to the GetStatus func " +
				"And there is no matching clientConnectionSetting in the cache for the given name" +
				"Then the func should return NotProcessed as the status",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"testns": {"ccsName": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccsName",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "testId",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a ClientConnectionConfig is passed to the GetStatus func " +
				"And there is a matching clientConnectionSetting in the cache " +
				"Then the func should return the correct status",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"testns": {"ccsName": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccsName",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			expectedStatus: common.ProcessingInProgress,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actual := tc.clientConnectionSettingsCache.GetStatus(tc.clientConnectionSettings)

			assert.Equal(t, tc.expectedStatus, actual)

		})
	}

}

func TestDeleteClientConnectionConfigCache(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)
	testCases := []struct {
		name                          string
		clientConnectionSettingsCache *clientConnectionSettingsCache
		clientConnectionSettings      *v1.ClientConnectionConfig
		expectedCache                 map[string]map[string]map[string]*clientConnectionSettingsItem
	}{
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a ClientConnectionConfig is passed to the Delete func " +
				"And the ClientConnectionConfig does not exists in the cache " +
				"Then the func should not delete anything from the cache",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"testns": {"ccsName": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccsName",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "bar",
					Namespace: "barns",
					Labels: map[string]string{
						"admiral.io/env": "foo",
						"identity":       "bar",
					},
				},
			},
			expectedCache: map[string]map[string]map[string]*clientConnectionSettingsItem{
				"testEnv.testId": {
					"testns": {"ccsName": &clientConnectionSettingsItem{
						clientConnectionSettings: &v1.ClientConnectionConfig{
							ObjectMeta: apiMachineryMetaV1.ObjectMeta{
								Name:      "ccsName",
								Namespace: "testns",
								Labels: map[string]string{
									"admiral.io/env": "testEnv",
									"identity":       "testId",
								},
							},
						},
						status: common.ProcessingInProgress,
					}},
				},
			},
		},
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a valid ClientConnectionConfig is passed to the Delete func " +
				"Then the func should delete the ClientConnectionConfig fromthe cache",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"testns": {"ccs0": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccs0",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccs0",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			expectedCache: map[string]map[string]map[string]*clientConnectionSettingsItem{
				"testEnv.testId": {
					"testns": {},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			tc.clientConnectionSettingsCache.Delete(tc.clientConnectionSettings)

			assert.Equal(t, tc.expectedCache, tc.clientConnectionSettingsCache.cache)

		})
	}

}

func TestPutClientConnectionConfigCache(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)

	testCases := []struct {
		name                          string
		clientConnectionSettingsCache *clientConnectionSettingsCache
		clientConnectionSettings      *v1.ClientConnectionConfig
		expectedCache                 map[string]map[string]map[string]*clientConnectionSettingsItem
	}{
		{
			name: "Given an empty clientConnectionSettingsCache " +
				"When a valid ClientConnectionConfig is passed to the Put func " +
				"Then the func should add the ClientConnectionConfig to the cache",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: make(map[string]map[string]map[string]*clientConnectionSettingsItem),
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			expectedCache: map[string]map[string]map[string]*clientConnectionSettingsItem{
				"testEnv.testId": {
					"testns": {"ccsName": &clientConnectionSettingsItem{
						clientConnectionSettings: &v1.ClientConnectionConfig{
							ObjectMeta: apiMachineryMetaV1.ObjectMeta{
								Name:      "ccsName",
								Namespace: "testns",
								Labels: map[string]string{
									"admiral.io/env": "testEnv",
									"identity":       "testId",
								},
							},
						},
						status: common.ProcessingInProgress,
					}},
				},
			},
		},
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a valid ClientConnectionConfig is passed to the Put func " +
				"And the ClientConnectionConfig is in a different namespace " +
				"Then the func should add the ClientConnectionConfig to the cache",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"someotherns": {"ccsName": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccsName",
									Namespace: "someotherns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			expectedCache: map[string]map[string]map[string]*clientConnectionSettingsItem{
				"testEnv.testId": {
					"testns": {"ccsName": &clientConnectionSettingsItem{
						clientConnectionSettings: &v1.ClientConnectionConfig{
							ObjectMeta: apiMachineryMetaV1.ObjectMeta{
								Name:      "ccsName",
								Namespace: "testns",
								Labels: map[string]string{
									"admiral.io/env": "testEnv",
									"identity":       "testId",
								},
							},
						},
						status: common.ProcessingInProgress,
					}},
					"someotherns": {"ccsName": &clientConnectionSettingsItem{
						clientConnectionSettings: &v1.ClientConnectionConfig{
							ObjectMeta: apiMachineryMetaV1.ObjectMeta{
								Name:      "ccsName",
								Namespace: "someotherns",
								Labels: map[string]string{
									"admiral.io/env": "testEnv",
									"identity":       "testId",
								},
							},
						},
						status: common.ProcessingInProgress,
					}},
				},
			},
		},
		{
			name: "Given an clientConnectionSettingsCache " +
				"When a valid ClientConnectionConfig is passed to the Put func " +
				"And another ClientConnectionConfig is in same namespace " +
				"Then the func should add the ClientConnectionConfig to the cache",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"testEnv.testId": {
						"testns": {"ccs0": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccs0",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccs1",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
						"identity":       "testId",
					},
				},
			},
			expectedCache: map[string]map[string]map[string]*clientConnectionSettingsItem{
				"testEnv.testId": {
					"testns": {
						"ccs0": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccs0",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						},
						"ccs1": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name:      "ccs1",
									Namespace: "testns",
									Labels: map[string]string{
										"admiral.io/env": "testEnv",
										"identity":       "testId",
									},
								},
							},
							status: common.ProcessingInProgress,
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			tc.clientConnectionSettingsCache.Put(tc.clientConnectionSettings)

			assert.Equal(t, tc.expectedCache, tc.clientConnectionSettingsCache.cache)

		})
	}

}

func TestGetClientConnectionConfigCache(t *testing.T) {

	testCases := []struct {
		name                               string
		clientConnectionSettingsCache      *clientConnectionSettingsCache
		key                                string
		namespace                          string
		expectedClientConnectionConfigList []*v1.ClientConnectionConfig
	}{
		{
			name: "Given an empty clientConnectionSettingsCache " +
				"When Get func is called on it " +
				"Then the func should return an empty slice of clientConnectionSettingsItem",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: make(map[string]map[string]map[string]*clientConnectionSettingsItem),
				mutex: &sync.RWMutex{},
			},
			key:                                "doesNotExists",
			namespace:                          "testns",
			expectedClientConnectionConfigList: []*v1.ClientConnectionConfig{},
		},
		{
			name: "Given an clientConnectionSettingsCache " +
				"When Get func is called with a key and namespace param " +
				"And the passed namespace does not match the key " +
				"Then the func should return an empty slice of clientConnectionSettingsItem",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"ccskey": {"someotherns": map[string]*clientConnectionSettingsItem{}},
				},
				mutex: &sync.RWMutex{},
			},
			key:                                "ccskey",
			namespace:                          "testns",
			expectedClientConnectionConfigList: []*v1.ClientConnectionConfig{},
		},
		{
			name: "Given an clientConnectionSettingsCache " +
				"When Get func is called with a key and namespace param " +
				"And the passed namespace does match the key " +
				"Then the func should return a slice of clientConnectionSettingsItem",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				cache: map[string]map[string]map[string]*clientConnectionSettingsItem{
					"ccskey": {
						"testns": {"ccsName": &clientConnectionSettingsItem{
							clientConnectionSettings: &v1.ClientConnectionConfig{
								ObjectMeta: apiMachineryMetaV1.ObjectMeta{
									Name: "ccsName",
								},
							},
							status: common.ProcessingInProgress,
						}},
					},
				},
				mutex: &sync.RWMutex{},
			},
			key:       "ccskey",
			namespace: "testns",
			expectedClientConnectionConfigList: []*v1.ClientConnectionConfig{
				{
					ObjectMeta: apiMachineryMetaV1.ObjectMeta{
						Name: "ccsName",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actual := tc.clientConnectionSettingsCache.Get(tc.key, tc.namespace)

			assert.NotNil(t, actual)
			assert.Equal(t, tc.expectedClientConnectionConfigList, actual)

		})
	}

}

type MockClientConnectionHandler struct {
}

func (m *MockClientConnectionHandler) Added(ctx context.Context, obj *v1.ClientConnectionConfig) error {
	return nil
}

func (m *MockClientConnectionHandler) Updated(ctx context.Context, obj *v1.ClientConnectionConfig) error {
	return nil
}

func (m *MockClientConnectionHandler) Deleted(ctx context.Context, obj *v1.ClientConnectionConfig) error {
	return nil
}

type MockCRDClient struct {
}

func (m MockCRDClient) Discovery() discovery.DiscoveryInterface {
	return nil
}

func (m MockCRDClient) AdmiralV1alpha1() admiralv1.AdmiralV1alpha1Interface {
	return MockAdmiralV1{}
}

type MockAdmiralV1 struct {
}

func (m MockAdmiralV1) RESTClient() rest.Interface {
	return nil
}

func (m MockAdmiralV1) ClientConnectionConfigs(namespace string) admiralv1.ClientConnectionConfigInterface {
	return MockClientConnectionConfig{}
}

func (m MockAdmiralV1) Dependencies(namespace string) admiralv1.DependencyInterface {
	return nil
}

func (m MockAdmiralV1) DependencyProxies(namespace string) admiralv1.DependencyProxyInterface {
	return nil
}

func (m MockAdmiralV1) GlobalTrafficPolicies(namespace string) admiralv1.GlobalTrafficPolicyInterface {
	return nil
}

func (m MockAdmiralV1) OutlierDetections(namespace string) admiralv1.OutlierDetectionInterface {
	return nil
}

func (m MockAdmiralV1) RoutingPolicies(namespace string) admiralv1.RoutingPolicyInterface {
	return nil
}

func (m MockAdmiralV1) TrafficConfigs(namespace string) admiralv1.TrafficConfigInterface {
	return nil
}

type MockClientConnectionConfig struct {
}

func (m MockClientConnectionConfig) Create(ctx context.Context, clientConnectionSettings *v1.ClientConnectionConfig, opts apiMachineryMetaV1.CreateOptions) (*v1.ClientConnectionConfig, error) {
	return nil, nil
}

func (m MockClientConnectionConfig) Update(ctx context.Context, clientConnectionSettings *v1.ClientConnectionConfig, opts apiMachineryMetaV1.UpdateOptions) (*v1.ClientConnectionConfig, error) {
	return nil, nil
}

func (m MockClientConnectionConfig) UpdateStatus(ctx context.Context, clientConnectionSettings *v1.ClientConnectionConfig, opts apiMachineryMetaV1.UpdateOptions) (*v1.ClientConnectionConfig, error) {
	return nil, nil
}

func (m MockClientConnectionConfig) Delete(ctx context.Context, name string, opts apiMachineryMetaV1.DeleteOptions) error {
	return nil
}

func (m MockClientConnectionConfig) DeleteCollection(ctx context.Context, opts apiMachineryMetaV1.DeleteOptions, listOpts apiMachineryMetaV1.ListOptions) error {
	return nil
}

func (m MockClientConnectionConfig) Get(ctx context.Context, name string, opts apiMachineryMetaV1.GetOptions) (*v1.ClientConnectionConfig, error) {
	return ctx.Value("ClientConnectionConfig").(*v1.ClientConnectionConfig), nil
}

func (m MockClientConnectionConfig) List(ctx context.Context, opts apiMachineryMetaV1.ListOptions) (*v1.ClientConnectionConfigList, error) {
	return nil, nil
}

func (m MockClientConnectionConfig) Watch(ctx context.Context, opts apiMachineryMetaV1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (m MockClientConnectionConfig) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts apiMachineryMetaV1.PatchOptions, subresources ...string) (result *v1.ClientConnectionConfig, err error) {
	return nil, nil
}
