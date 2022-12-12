package clusters

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

const (
	// Alert logs
	alertMsgSuspensionEnabled = "op=dynamicEndpointSuspension message=endpoint generation suspension is enabled." +
		"this does not mean that endpoint generation will be suspended. " +
		"it will depend on the suspension list, which can include all identities " +
		"for all environments, OR certain identities for all or certain environments"
	alertMsgSuspensionForAll                           = "op=dynamicEndpointSuspension message=endpoint generation suspended for all"
	alertMsgSuspensionForIdentityInAllEnvironments     = "op=dynamicEndpointSuspension message=endpoint generation suspended for identity across all environments"
	alertMsgSuspensionForIdentityInMatchingEnvironment = "op=dynamicEndpointSuspension message=endpoint generation suspended for identity for given environment"
)

type serviceEntrySuspender struct {
	ignoredIdentityCache *IgnoredIdentityCache
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
	}
	log.Println("op=dynamicEndpointSuspension message=endpoint generation suspension is not enabled")
	return des.ignoredIdentityCache.Enabled
}

func (des *serviceEntrySuspender) all() bool {
	if des.ignoredIdentityCache.All {
		log.Println(alertMsgSuspensionForAll)
	}
	log.Println("op=dynamicEndpointSuspension message=endpoint generation suspension for 'all' identities is not enabled")
	return des.ignoredIdentityCache.All
}

func (des *serviceEntrySuspender) identityByEnvironment(identity, environment string) bool {
	log.Printf("op=dynamicEndpointSuspension message=checking if identity %s in environment %s is in the suspension list",
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
