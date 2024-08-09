package clusters

import (
	"context"
	"errors"
	"sync"

	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RoutingPolicyHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

type routingPolicyCache struct {
	// map of routing policies key=environment.identity, value: RoutingPolicy object
	// only one routing policy per identity + env is allowed
	identityCache map[string]*v1.RoutingPolicy
	mutex         *sync.Mutex
}

func (r *routingPolicyCache) Delete(identity string, environment string) {
	defer r.mutex.Unlock()
	r.mutex.Lock()
	key := common.ConstructRoutingPolicyKey(environment, identity)
	if _, ok := r.identityCache[key]; ok {
		log.Infof("deleting RoutingPolicy with key=%s from global RoutingPolicy cache", key)
		delete(r.identityCache, key)
	}
}

func (r *routingPolicyCache) GetFromIdentity(identity string, environment string) *v1.RoutingPolicy {
	defer r.mutex.Unlock()
	r.mutex.Lock()
	return r.identityCache[common.ConstructRoutingPolicyKey(environment, identity)]
}

func (r *routingPolicyCache) Put(rp *v1.RoutingPolicy) error {
	if rp == nil || rp.Name == "" {
		// no RoutingPolicy, throw error
		return errors.New("cannot add an empty RoutingPolicy to the cache")
	}
	if rp.Labels == nil {
		return errors.New("labels empty in RoutingPolicy")
	}
	defer r.mutex.Unlock()
	r.mutex.Lock()
	var rpIdentity = rp.Labels[common.GetRoutingPolicyLabel()]
	var rpEnv = common.GetRoutingPolicyEnv(rp)

	log.Infof("Adding RoutingPolicy with name %v to RoutingPolicy cache. LabelMatch=%v env=%v", rp.Name, rpIdentity, rpEnv)
	key := common.ConstructRoutingPolicyKey(rpEnv, rpIdentity)
	r.identityCache[key] = rp

	return nil
}

type routingPolicyFilterCache struct {
	// map of envoyFilters key=routingpolicyName+identity+environment of the routingPolicy, value is a map [clusterId -> map [filterName -> filterNameSpace]]
	filterCache map[string]map[string]map[string]string
	mutex       *sync.Mutex
}

/*
Get - returns the envoyFilters for a given identity(rpName+identity)+env key
*/
func (r *routingPolicyFilterCache) Get(identityEnvKey string) (filters map[string]map[string]string) {
	defer r.mutex.Unlock()
	r.mutex.Lock()
	return r.filterCache[identityEnvKey]
}

/*
Put - updates the cache for filters, where it uses identityEnvKey, clusterID, and filterName as the key, and filterNamespace as the value
*/
func (r *routingPolicyFilterCache) Put(identityEnvKey string, clusterId string, filterName string, filterNamespace string) {
	defer r.mutex.Unlock()
	r.mutex.Lock()
	if r.filterCache[identityEnvKey] == nil {
		r.filterCache[identityEnvKey] = make(map[string]map[string]string)
	}

	if r.filterCache[identityEnvKey][clusterId] == nil {
		r.filterCache[identityEnvKey][clusterId] = make(map[string]string)
	}
	r.filterCache[identityEnvKey][clusterId][filterName] = filterNamespace
}

func (r *routingPolicyFilterCache) Delete(identityEnvKey string) {
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(LogFormat, admiral.Delete, "routingpolicy", identityEnvKey, "", "skipping read-only mode")
		return
	}
	if common.GetEnableRoutingPolicy() {
		defer r.mutex.Unlock()
		r.mutex.Lock()
		// delete all envoyFilters for a given identity+env key
		delete(r.filterCache, identityEnvKey)
	} else {
		log.Infof(LogFormat, admiral.Delete, "routingpolicy", identityEnvKey, "", "routingpolicy disabled")
	}
}
func (r RoutingPolicyHandler) Added(ctx context.Context, obj *v1.RoutingPolicy) error {
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(LogFormat, admiral.Add, "routingpolicy", "", "", "skipping read-only mode")
		return nil
	}
	if common.GetEnableRoutingPolicy() {
		if common.ShouldIgnoreResource(obj.ObjectMeta) {
			log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.RoutingPolicyResourceType,
				obj.Name, obj.Namespace, "", "Value=true")
			log.Infof(LogFormat, "success", "routingpolicy", obj.Name, "", "Ignored the RoutingPolicy because of the annotation")
			return nil
		}
		dependents := getDependents(obj, r)
		if len(dependents) == 0 {
			log.Info("No dependents found for Routing Policy - ", obj.Name)
			return nil
		}
		err := r.processroutingPolicy(ctx, dependents, obj, admiral.Add)
		if err != nil {
			log.Errorf(LogErrFormat, admiral.Update, "routingpolicy", obj.Name, "", "failed to process routing policy")
			return err
		}
		log.Infof(LogFormat, admiral.Add, "routingpolicy", obj.Name, "", "finished processing routing policy")
	} else {
		log.Infof(LogFormat, admiral.Add, "routingpolicy", obj.Name, "", "routingpolicy disabled")
	}
	return nil
}

func (r RoutingPolicyHandler) processroutingPolicy(ctx context.Context, dependents map[string]string, routingPolicy *v1.RoutingPolicy, eventType admiral.EventType) error {
	var err error
	for _, remoteController := range r.RemoteRegistry.remoteControllers {
		for _, dependent := range dependents {
			// Check if the dependent exists in this remoteCluster. If so, we create an envoyFilter with dependent identity as workload selector
			if _, ok := r.RemoteRegistry.AdmiralCache.IdentityClusterCache.Get(dependent).Copy()[remoteController.ClusterID]; ok {
				_, err1 := createOrUpdateEnvoyFilter(ctx, remoteController, routingPolicy, eventType, dependent, r.RemoteRegistry.AdmiralCache)
				if err1 != nil {
					log.Errorf(LogErrFormat, eventType, "routingpolicy", routingPolicy.Name, remoteController.ClusterID, err)
					err = common.AppendError(err, err1)
				} else {
					log.Infof(LogFormat, eventType, "routingpolicy	", routingPolicy.Name, remoteController.ClusterID, "created envoyfilters")
				}
			}
		}
	}
	return err
}

func (r RoutingPolicyHandler) Updated(ctx context.Context, obj *v1.RoutingPolicy) error {
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(LogFormat, admiral.Update, "routingpolicy", "", "", "skipping read-only mode")
		return nil
	}
	if common.GetEnableRoutingPolicy() {
		if common.ShouldIgnoreResource(obj.ObjectMeta) {
			log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.RoutingPolicyResourceType,
				obj.Name, obj.Namespace, "", "Value=true")
			log.Infof(LogFormat, admiral.Update, "routingpolicy", obj.Name, "", "Ignored the RoutingPolicy because of the annotation")
			// We need to process this as a delete event.
			r.Deleted(ctx, obj)
			return nil
		}
		dependents := getDependents(obj, r)
		if len(dependents) == 0 {
			return nil
		}
		err := r.processroutingPolicy(ctx, dependents, obj, admiral.Update)
		if err != nil {
			log.Errorf(LogErrFormat, admiral.Update, "routingpolicy", obj.Name, "", "failed to process routing policy")
			return err
		}
		log.Infof(LogFormat, admiral.Update, "routingpolicy", obj.Name, "", "updated routing policy")
	} else {
		log.Infof(LogFormat, admiral.Update, "routingpolicy", obj.Name, "", "routingpolicy disabled")
	}
	return nil
}

// getDependents - Returns the client dependents for the destination service with routing policy
// Returns a list of asset ID's of the client services or nil if no dependents are found
func getDependents(obj *v1.RoutingPolicy, r RoutingPolicyHandler) map[string]string {
	sourceIdentity := common.GetRoutingPolicyIdentity(obj)
	if len(sourceIdentity) == 0 {
		err := errors.New("identity label is missing")
		log.Warnf(LogErrFormat, "add", "RoutingPolicy", obj.Name, r.ClusterID, err)
		return nil
	}

	dependents := r.RemoteRegistry.AdmiralCache.IdentityDependencyCache.Get(sourceIdentity).Copy()
	return dependents
}

/*
Deleted - deletes the envoyFilters for the routingPolicy when delete event received for routing policy
*/
func (r RoutingPolicyHandler) Deleted(ctx context.Context, obj *v1.RoutingPolicy) error {
	err := r.deleteEnvoyFilters(ctx, obj, admiral.Delete)
	if err != nil {
		log.Infof(LogFormat, admiral.Delete, "routingpolicy", obj.Name, "", "deleted envoy filter for routing policy")
	}
	return err
}

func (r RoutingPolicyHandler) deleteEnvoyFilters(ctx context.Context, obj *v1.RoutingPolicy, eventType admiral.EventType) error {
	key := obj.Name + common.GetRoutingPolicyIdentity(obj) + common.GetRoutingPolicyEnv(obj)
	if r.RemoteRegistry == nil || r.RemoteRegistry.AdmiralCache == nil || r.RemoteRegistry.AdmiralCache.RoutingPolicyFilterCache == nil {
		log.Infof(LogFormat, eventType, "routingpolicy", obj.Name, "", "skipping delete event as cache is nil")
		return nil
	}
	clusterIdFilterMap := r.RemoteRegistry.AdmiralCache.RoutingPolicyFilterCache.Get(key) // RoutingPolicyFilterCache key=rpname+rpidentity+environment of the routingPolicy, value is a map [clusterId -> map [filterName -> filterNameSpace]]
	var err error
	for _, rc := range r.RemoteRegistry.remoteControllers {
		if rc != nil {
			if filterMap, ok := clusterIdFilterMap[rc.ClusterID]; ok {
				for filter, filterNs := range filterMap {
					log.Infof(LogFormat, eventType, "envoyfilter", filter, rc.ClusterID, "deleting")
					err1 := rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters(filterNs).Delete(ctx, filter, metaV1.DeleteOptions{})
					if err1 != nil {
						log.Errorf(LogErrFormat, eventType, "envoyfilter", filter, rc.ClusterID, err1)
						err = common.AppendError(err, err1)
					} else {
						log.Infof(LogFormat, eventType, "envoyfilter", filter, rc.ClusterID, "deleting from cache")
					}
				}
			}
		}
	}
	if err == nil {
		r.RemoteRegistry.AdmiralCache.RoutingPolicyFilterCache.Delete(key)
	}
	return err
}
