package clusters

import (
	"context"
	"fmt"
	"sync"
	"testing"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	networkingAlpha3 "istio.io/api/networking/v1alpha3"
	apiMachineryMetaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandleEventForClientConnectionConfig(t *testing.T) {
	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.ResetSync()
	common.InitializeConfig(p)

	testCases := []struct {
		name                     string
		ctx                      context.Context
		clientConnectionSettings *v1.ClientConnectionConfig
		modifySE                 ModifySEFunc
		expectedError            error
	}{
		{
			name: "Given valid params to HandleEventForClientConnectionConfig func " +
				"When identity is not set on the ClientConnectionConfig " +
				"Then the func should return an error",
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:      "ccsName",
					Namespace: "testns",
					Labels: map[string]string{
						"admiral.io/env": "testEnv",
					},
				},
			},
			expectedError: fmt.Errorf(
				"op=Event type=ClientConnectionConfig name=ccsName cluster=testCluster message=skipped as label identity was not found, namespace=testns"),
			ctx:      context.Background(),
			modifySE: mockModifySE,
		},
		{
			name: "Given valid params to HandleEventForClientConnectionConfig func " +
				"When admiral.io/env is not set on the ClientConnectionConfig " +
				"Then the func should not return an error",
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
			ctx:           context.Background(),
			modifySE:      mockModifySE,
			expectedError: nil,
		},
		{
			name: "Given valid params to HandleEventForClientConnectionConfig func " +
				"When modifySE func returns an error " +
				"Then the func should return an error",
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
			ctx:           context.WithValue(context.Background(), "hasErrors", "modifySE failed"),
			modifySE:      mockModifySE,
			expectedError: fmt.Errorf("modifySE failed"),
		},
		{
			name: "Given valid params to HandleEventForClientConnectionConfig func " +
				"When modifySE func does not return any error " +
				"Then the func should not return any error either",
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
			ctx:           context.Background(),
			modifySE:      mockModifySE,
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actualError := HandleEventForClientConnectionConfig(tc.ctx, common.UPDATE, tc.clientConnectionSettings, nil, "testCluster", tc.modifySE)
			if tc.expectedError != nil {
				if actualError == nil {
					t.Fatalf("expected error %s but got nil", tc.expectedError.Error())
				}
				assert.Equal(t, tc.expectedError.Error(), actualError.Error())
			} else {
				if actualError != nil {
					t.Fatalf("expected error nil but got %s", actualError.Error())
				}
			}

		})
	}

}

func TestDelete(t *testing.T) {

	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)

	testCases := []struct {
		name                          string
		env                           string
		identity                      string
		clientConnectionSettingsCache *clientConnectionSettingsCache
		expectedError                 error
	}{
		{
			name: "Given clientConnectionSettingsCache " +
				"When Delete func is called with clientConnectionSettings " +
				"And the passed identity and env key is not in the cache " +
				"Then the func should return an error",
			env:      "foo",
			identity: "bar",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				identityCache: make(map[string]*v1.ClientConnectionConfig),
				mutex:         &sync.RWMutex{},
			},
			expectedError: fmt.Errorf(
				"clientConnectionSettings with key foo.bar not found in clientConnectionSettingsCache"),
		},
		{
			name: "Given clientConnectionSettingsCache " +
				"When Delete func is called " +
				"And the passed identity and env key is in the cache " +
				"Then the func should not return an error and should successfully delete the entry",
			env:      "testEnv",
			identity: "testId",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				identityCache: map[string]*v1.ClientConnectionConfig{
					"testEnv.testId": {
						ObjectMeta: apiMachineryMetaV1.ObjectMeta{
							Name:      "ccsName",
							Namespace: "testns",
							Labels: map[string]string{
								"admiral.io/env": "testEnv",
								"identity":       "testId",
							},
						},
					},
				},
				mutex: &sync.RWMutex{},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := tc.clientConnectionSettingsCache.Delete(tc.identity, tc.env)
			if tc.expectedError != nil {
				if err == nil {
					t.Fatalf("expected error %s but got nil", tc.expectedError.Error())
				}
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				if err != nil {
					t.Fatalf("expected nil error but got %s error", err.Error())
				}
				assert.Nil(t, tc.clientConnectionSettingsCache.identityCache[tc.env+"."+tc.identity])
			}

		})
	}

}

func TestPut(t *testing.T) {

	p := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(p)

	testCases := []struct {
		name                          string
		clientConnectionSettings      *v1.ClientConnectionConfig
		clientConnectionSettingsCache *clientConnectionSettingsCache
		expectedError                 error
	}{
		{
			name: "Given clientConnectionSettingsCache " +
				"When Put func is called with clientConnectionSettings " +
				"And the passed clientConnectionSettings is missing the name " +
				"Then the func should return an error",
			clientConnectionSettings: &v1.ClientConnectionConfig{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Namespace: "testns",
				},
			},
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				identityCache: make(map[string]*v1.ClientConnectionConfig),
				mutex:         &sync.RWMutex{},
			},
			expectedError: fmt.Errorf(
				"skipped adding to clientConnectionSettingsCache, missing name in clientConnectionSettings"),
		},
		{
			name: "Given clientConnectionSettingsCache " +
				"When Put func is called with clientConnectionSettings " +
				"And the passed clientConnectionSettings is missing the name " +
				"Then the func should not return any error and should successfully add the entry",
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
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				identityCache: make(map[string]*v1.ClientConnectionConfig),
				mutex:         &sync.RWMutex{},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := tc.clientConnectionSettingsCache.Put(tc.clientConnectionSettings)
			if tc.expectedError != nil {
				if err == nil {
					t.Fatalf("expected error %s but got nil", tc.expectedError.Error())
				}
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				if err != nil {
					t.Fatalf("expected nil error but got %s error", err.Error())
				}
				assert.Equal(t, tc.clientConnectionSettings, tc.clientConnectionSettingsCache.identityCache["testEnv.testId"])
			}

		})
	}

}

func TestGetFromIdentity(t *testing.T) {

	clientConnectionSettings := &v1.ClientConnectionConfig{
		ObjectMeta: apiMachineryMetaV1.ObjectMeta{
			Name:      "ccsName",
			Namespace: "testns",
			Labels: map[string]string{
				"admiral.io/env": "testEnv",
				"identity":       "testId",
			},
		},
	}

	testCases := []struct {
		name                          string
		identity                      string
		env                           string
		clientConnectionSettingsCache *clientConnectionSettingsCache
	}{
		{
			name: "Given clientConnectionSettingsCache " +
				"When GetFromIdentity func is called with valid identity and env " +
				"Then the func should return clientConnectionSettings from cache",
			identity: "testId",
			env:      "testEnv",
			clientConnectionSettingsCache: &clientConnectionSettingsCache{
				identityCache: map[string]*v1.ClientConnectionConfig{
					"testEnv.testId": clientConnectionSettings,
				},
				mutex: &sync.RWMutex{},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			actualClientConnectionConfig, err := tc.clientConnectionSettingsCache.GetFromIdentity(tc.identity, tc.env)
			assert.Nil(t, err)
			assert.Equal(t, clientConnectionSettings, actualClientConnectionConfig)

		})
	}

}

func mockModifySE(ctx context.Context, event admiral.EventType, env string,
	sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*networkingAlpha3.ServiceEntry, error) {

	if ctx.Value("hasErrors") != nil {
		return nil, fmt.Errorf(ctx.Value("hasErrors").(string))
	}

	return nil, nil
}
