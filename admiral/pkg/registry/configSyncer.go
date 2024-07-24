package registry

type ConfigSyncer interface {
	SyncDeployment() error
	SyncService() error

	// argo custom resources
	SyncArgoRollout() error

	// admiral custom resources
	SyncGlobalTrafficPolicy() error
	SyncClientConnectionConfigurations() error
	SyncOutlierDetectionConfigurations() error
}

type configSync struct{}

func NewConfigSync() *configSync {
	return &configSync{}
}

func (c *configSync) SyncDeployment() error {
	return nil
}

func (c *configSync) SyncService() error {
	return nil
}
func (c *configSync) SyncArgoRollout() error {
	return nil
}
func (c *configSync) SyncGlobalTrafficPolicy() error {
	return nil
}
func (c *configSync) SyncClientConnectionConfigurations() error {
	return nil
}
func (c *configSync) SyncOutlierDetectionConfigurations() error {
	return nil
}
