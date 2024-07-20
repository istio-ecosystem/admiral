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

type GlobalTrafficHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type GlobalTrafficCache interface {
	GetFromIdentity(identity string, environment string) (*v1.GlobalTrafficPolicy, error)
	Put(gtp *v1.GlobalTrafficPolicy) error
	Delete(identity string, environment string) error
}

type globalTrafficCache struct {
	//map of global traffic policies key=environment.identity, value:GlobalTrafficCache GlobalTrafficPolicy object
	identityCache map[string]*v1.GlobalTrafficPolicy

	mutex *sync.Mutex
}

func (g *globalTrafficCache) GetFromIdentity(identity string, environment string) (*v1.GlobalTrafficPolicy, error) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return g.identityCache[common.ConstructKeyWithEnvAndIdentity(environment, identity)], nil
}

func (g *globalTrafficCache) Put(gtp *v1.GlobalTrafficPolicy) error {
	if gtp.Name == "" {
		//no GTP, throw error
		return errors.New("cannot add an empty globaltrafficpolicy to the cache")
	}
	defer g.mutex.Unlock()
	g.mutex.Lock()
	var gtpIdentity = common.GetGtpIdentity(gtp)
	var gtpEnv = common.GetGtpEnv(gtp)

	log.Infof("adding GTP with name %v to GTP cache. LabelMatch=%v env=%v", gtp.Name, gtpIdentity, gtpEnv)
	key := common.ConstructKeyWithEnvAndIdentity(gtpEnv, gtpIdentity)
	g.identityCache[key] = gtp
	return nil
}

func (g *globalTrafficCache) Delete(identity string, environment string) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	key := common.ConstructKeyWithEnvAndIdentity(environment, identity)
	if _, ok := g.identityCache[key]; ok {
		log.Infof("deleting gtp with key=%s from global GTP cache", key)
		delete(g.identityCache, key)
		return nil
	}
	return fmt.Errorf("gtp with key %s not found in cache", key)
}

func (gtp *GlobalTrafficHandler) Added(ctx context.Context, obj *v1.GlobalTrafficPolicy) error {
	log.Infof(LogFormat, "Added", "globaltrafficpolicy", obj.Name, gtp.ClusterID, "received")
	err := HandleEventForGlobalTrafficPolicy(ctx, admiral.Add, obj, gtp.RemoteRegistry, gtp.ClusterID, modifyServiceEntryForNewServiceOrPod)
	if err != nil {
		return fmt.Errorf(LogErrFormat, "Added", "globaltrafficpolicy", obj.Name, gtp.ClusterID, err.Error())
	}
	return nil
}

func (gtp *GlobalTrafficHandler) Updated(ctx context.Context, obj *v1.GlobalTrafficPolicy) error {
	log.Infof(LogFormat, "Updated", "globaltrafficpolicy", obj.Name, gtp.ClusterID, "received")
	err := HandleEventForGlobalTrafficPolicy(ctx, admiral.Update, obj, gtp.RemoteRegistry, gtp.ClusterID, modifyServiceEntryForNewServiceOrPod)
	if err != nil {
		return fmt.Errorf(LogErrFormat, "Updated", "globaltrafficpolicy", obj.Name, gtp.ClusterID, err.Error())
	}
	return nil
}

func (gtp *GlobalTrafficHandler) Deleted(ctx context.Context, obj *v1.GlobalTrafficPolicy) error {
	log.Infof(LogFormat, "Deleted", "globaltrafficpolicy", obj.Name, gtp.ClusterID, "received")
	err := HandleEventForGlobalTrafficPolicy(ctx, admiral.Delete, obj, gtp.RemoteRegistry, gtp.ClusterID, modifyServiceEntryForNewServiceOrPod)
	if err != nil {
		return fmt.Errorf(LogErrFormat, "Deleted", "globaltrafficpolicy", obj.Name, gtp.ClusterID, err.Error())
	}
	return nil
}

// HandleEventForGlobalTrafficPolicy processes all the events related to GTPs
func HandleEventForGlobalTrafficPolicy(ctx context.Context, event admiral.EventType, gtp *v1.GlobalTrafficPolicy,
	remoteRegistry *RemoteRegistry, clusterName string, modifySE ModifySEFunc) error {
	globalIdentifier := common.GetGtpIdentity(gtp)
	if len(globalIdentifier) == 0 {
		return fmt.Errorf(LogFormat, "Event", "globaltrafficpolicy", gtp.Name, clusterName, "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+gtp.Namespace)
	}

	env := common.GetGtpEnv(gtp)

	// For now we're going to force all the events to update only in order to prevent
	// the endpoints from being deleted.
	// TODO: Need to come up with a way to prevent deleting default endpoints so that this hack can be removed.
	// Use the same function as added deployment function to update and put new service entry in place to replace old one

	ctx = context.WithValue(ctx, "clusterName", clusterName)
	ctx = context.WithValue(ctx, "eventResourceType", common.GTP)
	ctx = context.WithValue(ctx, common.EventType, event)

	_, err := modifySE(ctx, admiral.Update, env, globalIdentifier, remoteRegistry)
	return err
}
