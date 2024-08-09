package clusters

import (
	"context"
	"errors"
	"fmt"
	"sync"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
)

type OutlierDetectionHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type OutlierDetectionCache interface {
	GetFromIdentity(identity string, environment string) (*v1.OutlierDetection, error)
	Put(od *v1.OutlierDetection) error
	Delete(identity string, env string) error
}

type outlierDetectionCache struct {

	//Map of OutlierDetection key=environment.identity, value:OutlierDetection
	identityCache map[string]*v1.OutlierDetection
	mutex         *sync.Mutex
}

func (cache *outlierDetectionCache) GetFromIdentity(identity string, environment string) (*v1.OutlierDetection, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	return cache.identityCache[common.ConstructKeyWithEnvAndIdentity(environment, identity)], nil
}

func (cache *outlierDetectionCache) Put(od *v1.OutlierDetection) error {
	if od.Name == "" {
		return errors.New("Cannot add an empty outlierdetection to the cache")
	}

	defer cache.mutex.Unlock()
	cache.mutex.Lock()

	identity := common.GetODIdentity(od)
	env := common.GetODEnv(od)

	log.Infof("Adding OutlierDetection with name=%s to OutlierDetectionCache. LabelMatch=%v env=%v", od.Name, identity, env)
	key := common.ConstructKeyWithEnvAndIdentity(env, identity)
	cache.identityCache[key] = od
	return nil
}

func (cache *outlierDetectionCache) Delete(identity string, env string) error {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	key := common.ConstructKeyWithEnvAndIdentity(env, identity)
	if _, ok := cache.identityCache[key]; ok {
		log.Infof("Deleting OutlierDetection with key=%s from OutlierDetection cache.", key)
		delete(cache.identityCache, key)
	} else {
		return fmt.Errorf("OutlierDetection with key %s not found in cache", key)
	}
	return nil
}

func (od OutlierDetectionHandler) Added(ctx context.Context, obj *v1.OutlierDetection) error {
	log.Infof(LogFormat, common.Add, common.OutlierDetection, obj.Name, od.ClusterID, common.ReceivedStatus)
	err := HandleEventForOutlierDetection(ctx, admiral.EventType(common.Add), obj, od.RemoteRegistry, od.ClusterID, modifyServiceEntryForNewServiceOrPod)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Add, common.OutlierDetection, obj.Name, od.ClusterID, err.Error())
	}
	return nil
}

func (od OutlierDetectionHandler) Updated(ctx context.Context, obj *v1.OutlierDetection) error {
	log.Infof(LogFormat, common.Update, common.OutlierDetection, obj.Name, od.ClusterID, common.ReceivedStatus)
	err := HandleEventForOutlierDetection(ctx, admiral.Update, obj, od.RemoteRegistry, od.ClusterID, modifyServiceEntryForNewServiceOrPod)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Update, common.OutlierDetection, obj.Name, od.ClusterID, err.Error())
	}
	return nil
}

func (od OutlierDetectionHandler) Deleted(ctx context.Context, obj *v1.OutlierDetection) error {
	log.Infof(LogFormat, common.Delete, common.OutlierDetection, obj.Name, od.ClusterID, common.ReceivedStatus)
	err := HandleEventForOutlierDetection(ctx, admiral.Update, obj, od.RemoteRegistry, od.ClusterID, modifyServiceEntryForNewServiceOrPod)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Delete, common.OutlierDetection, obj.Name, od.ClusterID, err.Error())
	}
	return nil
}

func HandleEventForOutlierDetection(ctx context.Context, event admiral.EventType, od *v1.OutlierDetection, registry *RemoteRegistry,
	clusterName string, modifySE ModifySEFunc) error {

	identity := common.GetODIdentity(od)
	if len(identity) <= 0 {
		return fmt.Errorf(LogFormat, "Event", common.OutlierDetection, od.Name, clusterName, "Skipped as label "+common.GetAdmiralCRDIdentityLabel()+" was not found, namespace="+od.Namespace)
	}

	env := common.GetODEnv(od)
	if len(env) <= 0 {
		return fmt.Errorf(LogFormat, "Event", common.OutlierDetection, od.Name, clusterName, "Skipped as env "+env+" was not found, namespace="+od.Namespace)
	}

	ctx = context.WithValue(ctx, common.ClusterName, clusterName)
	ctx = context.WithValue(ctx, common.EventResourceType, common.OutlierDetection)

	_, err := modifySE(ctx, admiral.Update, env, identity, registry)

	return err
}

func NewOutlierDetectionCache() *outlierDetectionCache {
	odCache := &outlierDetectionCache{}
	odCache.identityCache = make(map[string]*v1.OutlierDetection)
	odCache.mutex = &sync.Mutex{}
	return odCache
}
