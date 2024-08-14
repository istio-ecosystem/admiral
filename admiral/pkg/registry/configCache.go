package registry

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
)

type IdentityConfigCache interface {
	Get(name string) (*IdentityConfig, error)
	Update(name string, config *IdentityConfig, writer ConfigWriter) error
	Remove(name string) error
}
type cacheHandler struct {
	lock  *sync.RWMutex
	cache map[string]*IdentityConfig
}

func NewConfigCache() *cacheHandler {
	return &cacheHandler{
		lock:  &sync.RWMutex{},
		cache: map[string]*IdentityConfig{},
	}
}

func (handler *cacheHandler) Get(identityName string) (*IdentityConfig, error) {
	defer handler.lock.RUnlock()
	handler.lock.RLock()
	config, ok := handler.cache[identityName]
	if ok {
		return config, nil
	}
	return nil, fmt.Errorf("record not found")
}

func (handler *cacheHandler) Update(identityName string, config *IdentityConfig, writer ConfigWriter) error {
	defer handler.lock.Unlock()
	handler.lock.Lock()
	handler.cache[identityName] = config
	return writeToFileLocal(writer, config)
}

func writeToFileLocal(writer ConfigWriter, config *IdentityConfig) error {
	fmt.Printf("[state syncer] writing config to file assetname=%s", config.IdentityName)
	shortAlias := strings.Split(config.IdentityName, ".")
	currentDir, err := filepath.Abs("./")
	if err != nil {
		return err
	}
	pathName := currentDir + "/testdata/" + shortAlias[len(shortAlias)-1] + "IdentityConfiguration.json"
	if common.GetSecretFilterTags() == common.GetOperatorSecretFilterTags() && common.GetOperatorSecretFilterTags() != "" {
		pathName = "/etc/serviceregistry/config/" + shortAlias[len(shortAlias)-1] + "IdentityConfiguration.json"
	}
	fmt.Printf("[state syncer] file path=%s", pathName)
	bytes, _ := json.MarshalIndent(config, "", "  ")
	return writer.Write(pathName, bytes)
}

func (handler *cacheHandler) Remove(identityName string) error {
	defer handler.lock.Unlock()
	handler.lock.Lock()
	_, ok := handler.cache[identityName]
	if ok {
		delete(handler.cache, identityName)
		return nil
	}
	return fmt.Errorf("config for %s does not exist", identityName)
}
