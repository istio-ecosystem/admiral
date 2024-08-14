package registry

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/sirupsen/logrus"
	"sync"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
)

const (
	serviceRegistryIdentityConfigMapName = "service-registry-identityconfig"
	admiralQaNs                          = "services-admiral-use2-qal"
)

type ConfigSyncer interface {
	UpdateEnvironmentConfigByCluster(ctxLogger *logrus.Entry, environment, cluster string, config IdentityConfig, writer ConfigWriter) error
}

type configSync struct {
	cache map[string]IdentityConfigCache
	lock  *sync.RWMutex
}

func NewConfigSync() ConfigSyncer {
	return &configSync{
		lock:  &sync.RWMutex{},
		cache: make(map[string]IdentityConfigCache),
	}
}

func (c *configSync) UpdateEnvironmentConfigByCluster(
	ctxLogger *logrus.Entry,
	environment,
	cluster string,
	config IdentityConfig,
	writer ConfigWriter) error {
	var (
		task          = "UpdateEnvironmentConfigByCluster"
		clusterConfig = config.Clusters[cluster]
	)
	defer util.LogElapsedTimeForTask(ctxLogger, task, config.IdentityName, "", "", "processingTime")()
	ctxLogger.Infof(common.CtxLogFormat, task, config.IdentityName, "", "", "received")
	defer c.lock.Unlock()
	c.lock.Lock()
	cache, ok := c.cache[config.IdentityName]
	if ok {
		ctxLogger.Infof(common.CtxLogFormat, task, config.IdentityName, "", cluster, "cache found")
		// update cluster configuration
		configCache, err := cache.Get(config.IdentityName)
		if err != nil {
			ctxLogger.Errorf(common.CtxLogFormat, task, config.IdentityName, "", cluster, "config not found in cache")
			return err
		}
		if configCache.Clusters[cluster] != nil {
			configCache.Clusters[cluster].IngressEndpoint = clusterConfig.IngressEndpoint
			configCache.Clusters[cluster].IngressPort = clusterConfig.IngressPort
			configCache.Clusters[cluster].IngressPortName = clusterConfig.IngressPortName
			configCache.Clusters[cluster].Locality = clusterConfig.Locality
			configCache.Clusters[cluster].Name = clusterConfig.Name
			if configCache.Clusters[cluster].Environment == nil {
				configCache.Clusters[cluster].Environment = map[string]*IdentityConfigEnvironment{}
			}
			configCache.Clusters[cluster].Environment[environment] = config.Clusters[cluster].Environment[environment]
			ctxLogger.Infof(common.CtxLogFormat, task, config.IdentityName, "", cluster, "updating the cache")
			return cache.Update(config.IdentityName, configCache, writer)
		}
		// cluster doesn't exist
		configCache.Clusters[cluster] = &IdentityConfigCluster{
			Name:            clusterConfig.Name,
			Locality:        clusterConfig.Locality,
			IngressEndpoint: clusterConfig.IngressEndpoint,
			IngressPort:     clusterConfig.IngressPort,
			IngressPortName: clusterConfig.IngressPortName,
			Environment: map[string]*IdentityConfigEnvironment{
				environment: clusterConfig.Environment[environment],
			},
		}
		ctxLogger.Infof(common.CtxLogFormat, task, config.IdentityName, "", cluster, "updating the cache")
		return cache.Update(config.IdentityName, configCache, writer)
	}
	ctxLogger.Infof(common.CtxLogFormat, task, config.IdentityName, "", cluster, "creating new cache")
	// entry doesn't exist
	c.cache[config.IdentityName] = NewConfigCache()
	ctxLogger.Infof(common.CtxLogFormat, task, config.IdentityName, "", cluster, "updating the cache")
	return c.cache[config.IdentityName].Update(config.IdentityName, &config, writer)
}
