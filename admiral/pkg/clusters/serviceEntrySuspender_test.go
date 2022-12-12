package clusters

import (
	"sync"
	"testing"
)

func TestSuspendGeneration(t *testing.T) {
	testCases := []struct {
		name                   string
		ignoredIdentityCache   *IgnoredIdentityCache
		sourceIdentity         string
		sourceEnvironment      string
		expectedIgnoreDecision bool
	}{
		{
			name: "Given suspension is enabled, " +
				"And is configured to ignore all, " +
				"When SuspendGeneration is called with 'identity1' and 'environment1', " +
				"Then, it should return true",
			ignoredIdentityCache: &IgnoredIdentityCache{
				RWLock:  &sync.RWMutex{},
				Enabled: true,
				All:     true,
			},
			sourceIdentity:         "identity1",
			sourceEnvironment:      "environment1",
			expectedIgnoreDecision: true,
		},
		{
			name: "Given suspension is enabled, " +
				"And is configured to ignore none, " +
				"When SuspendGeneration is called with 'identity1' and 'environment1', " +
				"Then, it should return false",
			ignoredIdentityCache: &IgnoredIdentityCache{
				RWLock:  &sync.RWMutex{},
				Enabled: true,
			},
			sourceIdentity:         "identity1",
			sourceEnvironment:      "environment1",
			expectedIgnoreDecision: false,
		},
		{
			name: "Given suspension is enabled, " +
				"And suspension includes identity 'identity1' with no environments, " +
				"When SuspendGeneration is called with 'identity1' and 'environment1, " +
				"Then, it should return true",
			ignoredIdentityCache: &IgnoredIdentityCache{
				RWLock:  &sync.RWMutex{},
				Enabled: true,
				EnvironmentsByIdentity: map[string][]string{
					"identity1": {""},
				}},
			sourceIdentity:         "identity1",
			sourceEnvironment:      "environment1",
			expectedIgnoreDecision: true,
		},
		{
			name: "Given suspension is enabled, " +
				"And suspension includes 'identity1' for 'environment1', " +
				"When SuspendGeneration is called with 'identity2' and 'environment1, " +
				"Then, it should return false",
			ignoredIdentityCache: &IgnoredIdentityCache{
				RWLock:  &sync.RWMutex{},
				Enabled: true,
				EnvironmentsByIdentity: map[string][]string{
					"identity1": {"environment1"},
				},
			},
			sourceIdentity:         "identity2",
			sourceEnvironment:      "environment1",
			expectedIgnoreDecision: false,
		},
		{
			name: "Given suspension is enabled, " +
				"And suspension includes 'identity1' for 'environment1'" +
				"When ignoreProcessingIdentity is called with 'identity1' and 'environment1, " +
				"Then, it should return true",
			ignoredIdentityCache: &IgnoredIdentityCache{
				RWLock:  &sync.RWMutex{},
				Enabled: true,
				EnvironmentsByIdentity: map[string][]string{
					"identity1": {"environment1"},
				},
			},
			sourceIdentity:         "identity1",
			sourceEnvironment:      "environment1",
			expectedIgnoreDecision: true,
		},
		{
			name: "Given suspension is enabled, " +
				"And suspension includes 'identity1' for 'environment1'" +
				"When ignoreProcessingIdentity is called with 'identity2' and 'environment2, " +
				"Then, it should return false",
			ignoredIdentityCache: &IgnoredIdentityCache{
				RWLock:  &sync.RWMutex{},
				Enabled: true,
				EnvironmentsByIdentity: map[string][]string{
					"identity1": {"environment1"},
				},
			},
			sourceIdentity:         "identity2",
			sourceEnvironment:      "environment2",
			expectedIgnoreDecision: false,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			checker := &serviceEntrySuspender{
				ignoredIdentityCache: c.ignoredIdentityCache,
			}
			ignore := checker.SuspendUpdate(c.sourceIdentity, c.sourceEnvironment)
			if ignore != c.expectedIgnoreDecision {
				t.Errorf("expected ignore decision to be: %v, got: %v", c.expectedIgnoreDecision, ignore)
			}
		})
	}
}
