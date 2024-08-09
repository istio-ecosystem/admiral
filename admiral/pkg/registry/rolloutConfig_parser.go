package registry

func NewRolloutConfigSyncer(cacheHandler IdentityConfigCache) *rolloutConfigSyncer {
	return &rolloutConfigSyncer{
		cacheHandler: cacheHandler,
	}
}

type rolloutConfigSyncer struct {
	cacheHandler IdentityConfigCache
}

func (syncer *rolloutConfigSyncer) Sync(config interface{}) error {
	return nil
}
