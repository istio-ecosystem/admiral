package clusters

import (
	"context"
	"sync"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
)

const (
	serviceEntrySuspenderLogPrefix = "op=serviceEntrySuspender message="
	// Alert logs
	alertMsgSuspensionEnabled = serviceEntrySuspenderLogPrefix + "service entry update suspension is enabled. " +
		"this does not mean that service entry updates will not happen. " +
		"it will depend on the suspension list, which can include all identities " +
		"for all environments, OR certain identities for all or certain environments"
	alertMsgSuspensionForAll                           = serviceEntrySuspenderLogPrefix + "service entry update is suspended for all"
	alertMsgSuspensionForIdentityInAllEnvironments     = serviceEntrySuspenderLogPrefix + "service entry update is suspended for identity across all environments"
	alertMsgSuspensionForIdentityInMatchingEnvironment = serviceEntrySuspenderLogPrefix + "service entry update is suspended for identity for given environment"
)

type serviceEntrySuspender struct {
	ignoredIdentityCache *IgnoredIdentityCache
}

func NewDynamicServiceEntrySuspender(ctx context.Context, params common.AdmiralParams) *serviceEntrySuspender {
	var cache = &IgnoredIdentityCache{
		RWLock: &sync.RWMutex{},
	}
	stateChecker := initAdmiralStateChecker(ctx, ignoreIdentityChecker, params.AdmiralConfig)
	stateChecker.initStateCache(cache)
	RunAdmiralStateCheck(ctx, ignoreIdentityChecker, stateChecker)
	return &serviceEntrySuspender{ignoredIdentityCache: cache}
}

func NewDefaultServiceEntrySuspender(items []string) *serviceEntrySuspender {
	var (
		enabled               bool
		environmentByIdentity = make(map[string][]string)
	)
	if len(items) > 0 {
		enabled = true
	}
	for _, item := range items {
		environmentByIdentity[item] = []string{""}
	}
	return &serviceEntrySuspender{ignoredIdentityCache: &IgnoredIdentityCache{
		RWLock:                 &sync.RWMutex{},
		All:                    false,
		Enabled:                enabled,
		EnvironmentsByIdentity: environmentByIdentity,
	}}
}

func NewDummyServiceEntrySuspender() *serviceEntrySuspender {
	return &serviceEntrySuspender{
		ignoredIdentityCache: &IgnoredIdentityCache{
			RWLock: &sync.RWMutex{},
		},
	}
}

func (des *serviceEntrySuspender) SuspendUpdate(identity, environment string) bool {
	return des.enabled() && (des.all() || des.identityByEnvironment(identity, environment))
}

func (des *serviceEntrySuspender) enabled() bool {
	if des.ignoredIdentityCache.Enabled {
		log.Println(alertMsgSuspensionEnabled)
	} else {
		log.Println(serviceEntrySuspenderLogPrefix + "service entry update suspension is not enabled")
	}
	return des.ignoredIdentityCache.Enabled
}

func (des *serviceEntrySuspender) all() bool {
	if des.ignoredIdentityCache.All {
		log.Println(alertMsgSuspensionForAll)
	} else {
		log.Println(serviceEntrySuspenderLogPrefix + "service entry update suspension for 'all' identities is not enabled")
	}
	return des.ignoredIdentityCache.All
}

func (des *serviceEntrySuspender) identityByEnvironment(identity, environment string) bool {
	log.Printf(serviceEntrySuspenderLogPrefix+"checking if identity %s in environment %s is in the suspension list",
		identity, environment)
	des.ignoredIdentityCache.RWLock.RLock()
	defer des.ignoredIdentityCache.RWLock.RUnlock()
	if des.ignoredIdentityCache.EnvironmentsByIdentity[identity] != nil {
		identityEnvironments := des.ignoredIdentityCache.EnvironmentsByIdentity[identity]
		if len(identityEnvironments) == 0 || (len(identityEnvironments) == 1 && identityEnvironments[0] == "") {
			log.Printf("%s, identity: %s", alertMsgSuspensionForIdentityInAllEnvironments, identity)
			return true
		}
		for _, identityEnvironment := range identityEnvironments {
			if identityEnvironment == environment {
				log.Printf("%s, identity: %s, environment: %s",
					alertMsgSuspensionForIdentityInMatchingEnvironment, identity, environment,
				)
				return true
			}
		}
	}
	return false
}
