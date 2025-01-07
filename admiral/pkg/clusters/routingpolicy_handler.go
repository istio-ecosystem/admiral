package clusters

import (
	"context"
	"errors"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"sync"

	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RoutingPolicyHandler struct {
	RemoteRegistry       *RemoteRegistry
	ClusterID            string
	RoutingPolicyService RoutingPolicyProcessor
}

func NewRoutingPolicyHandler(rr *RemoteRegistry, cId string, rpProcessor RoutingPolicyProcessor) *RoutingPolicyHandler {
	return &RoutingPolicyHandler{RemoteRegistry: rr, ClusterID: cId, RoutingPolicyService: rpProcessor}
}

type RoutingPolicyProcessor interface {
	ProcessAddOrUpdate(ctx context.Context, eventType admiral.EventType, newRP *v1.RoutingPolicy, oldRP *v1.RoutingPolicy, dependents map[string]string) error
	ProcessDependency(ctx context.Context, eventType admiral.EventType, dependency *v1.Dependency) error
	Delete(ctx context.Context, eventType admiral.EventType, routingPolicy *v1.RoutingPolicy) error
}

type RoutingPolicyService struct {
	RemoteRegistry *RemoteRegistry
}

func (r *RoutingPolicyService) ProcessAddOrUpdate(ctx context.Context, eventType admiral.EventType, newRP *v1.RoutingPolicy, oldRP *v1.RoutingPolicy, dependents map[string]string) error {
	id := common.GetRoutingPolicyIdentity(newRP)
	env := common.GetRoutingPolicyEnv(newRP)
	ctxLogger := common.GetCtxLogger(ctx, id, env)
	ctxLogger.Debugf(LogFormat, eventType, common.RoutingPolicyResourceType, newRP.Name, "-", "start of processing routing policy")
	var err error
	defer util.LogElapsedTime("ProcessAddOrUpdateRoutingPolicy", id, env, "-")
	for _, remoteController := range r.RemoteRegistry.remoteControllers {
		if !common.DoRoutingPolicyForCluster(remoteController.ClusterID) {
			ctxLogger.Warnf(LogFormat, eventType, common.RoutingPolicyResourceType, newRP.Name, remoteController.ClusterID, "processing disabled for cluster")
			continue
		}
		for _, dependent := range dependents {
			// Check if the dependent exists in this remoteCluster. If so, we create an envoyFilter with dependent identity as workload selector
			if _, ok := r.RemoteRegistry.AdmiralCache.IdentityClusterCache.Get(dependent).Copy()[remoteController.ClusterID]; ok {
				_, err1 := createOrUpdateEnvoyFilter(ctx, remoteController, newRP, eventType, dependent, r.RemoteRegistry.AdmiralCache)
				if err1 != nil {
					ctxLogger.Errorf(LogErrFormat, eventType, common.RoutingPolicyResourceType, newRP.Name, remoteController.ClusterID, err)
					err = common.AppendError(err, err1)
				} else {
					ctxLogger.Infof(LogFormat, eventType, common.RoutingPolicyResourceType, newRP.Name, remoteController.ClusterID, "created envoyfilters")
					if oldRP != nil {
						oldEnv := common.GetRoutingPolicyEnv(oldRP)
						r.RemoteRegistry.AdmiralCache.RoutingPolicyCache.Delete(id, oldEnv, oldRP.Name)
					}
					r.RemoteRegistry.AdmiralCache.RoutingPolicyCache.Put(id, env, newRP.Name, newRP)
				}
			}
		}
	}
	ctxLogger.Debugf(LogFormat, eventType, common.RoutingPolicyResourceType, newRP.Name, "-", "end of processing routing policy")
	return err
}

func (r *RoutingPolicyService) ProcessDependency(ctx context.Context, eventType admiral.EventType, dependency *v1.Dependency) error {
	newDestinations, _ := getDestinationsToBeProcessed(eventType, dependency, r.RemoteRegistry)
	env := common.GetEnvFromMetadata(dependency.Annotations, dependency.Labels, nil)
	ctxLogger := common.GetCtxLogger(ctx, dependency.Name, env)
	defer util.LogElapsedTime("ProcessDependencyUpdateForRoutingPolicy", dependency.Spec.Source, env, "-")
	ctxLogger.Debugf(LogFormat, eventType, common.RoutingPolicyResourceType, dependency.Spec.Source, "-", "start of processing dependency update for routing policy")
	// for each destination, identify the newRP.
	for _, dependent := range newDestinations {
		// if the dependent is in the mesh, then get the newRP
		policies := r.RemoteRegistry.AdmiralCache.RoutingPolicyCache.GetForIdentity(dependent)
		for _, rp := range policies {
			err := r.ProcessAddOrUpdate(ctx, eventType, rp, nil, map[string]string{dependent: dependency.Spec.Source})
			if err != nil {
				ctxLogger.Errorf(LogErrFormat, eventType, common.RoutingPolicyResourceType, rp.Name, "",
					fmt.Sprintf("failed to process routing policy for new destination=%s in delta update", dependent))
				return err
			}
			ctxLogger.Infof(LogFormat, eventType, common.RoutingPolicyResourceType, rp.Name, "",
				fmt.Sprintf("finished processing routing policy for new destination=%s in delta update", dependent))
		}
	}
	ctxLogger.Debugf(LogFormat, eventType, common.RoutingPolicyResourceType, dependency.Spec.Source, "-", "end of processing dependency update for routing policy")
	return nil
}

func (r *RoutingPolicyService) Delete(ctx context.Context, eventType admiral.EventType, routingPolicy *v1.RoutingPolicy) error {
	identity := common.GetRoutingPolicyIdentity(routingPolicy)
	env := common.GetRoutingPolicyEnv(routingPolicy)
	ctxLogger := common.GetCtxLogger(ctx, identity, env)
	defer util.LogElapsedTime("DeleteRoutingPolicy", identity, env, "-")
	ctxLogger.Debugf(LogFormat, eventType, common.RoutingPolicyResourceType, routingPolicy.Name, "-", "start of delete for routing policy")
	key := routingPolicy.Name + identity + env
	if r.RemoteRegistry == nil || r.RemoteRegistry.AdmiralCache == nil || r.RemoteRegistry.AdmiralCache.RoutingPolicyFilterCache == nil {
		ctxLogger.Infof(LogFormat, eventType, common.RoutingPolicyResourceType, routingPolicy.Name, "", "skipping delete event as cache is nil")
		return nil
	}
	// RoutingPolicyFilterCache key=rpname+rpidentity+environment of the routingPolicy, value is a map [clusterId -> map [filterName -> filterNameSpace]]
	clusterIdFilterMap := r.RemoteRegistry.AdmiralCache.RoutingPolicyFilterCache.Get(key)
	var err error
	for _, rc := range r.RemoteRegistry.remoteControllers {
		if !common.DoRoutingPolicyForCluster(rc.ClusterID) {
			ctxLogger.Warnf(LogFormat, eventType, common.RoutingPolicyResourceType, routingPolicy.Name, rc.ClusterID, "RoutingPolicy disabled for cluster")
			continue
		}
		if filterMap, ok := clusterIdFilterMap[rc.ClusterID]; ok {
			for filter, filterNs := range filterMap {
				ctxLogger.Infof(LogFormat, eventType, common.EnvoyFilterResourceType, filter, rc.ClusterID, "deleting")
				err1 := rc.RoutingPolicyController.IstioClient.NetworkingV1alpha3().EnvoyFilters(filterNs).Delete(ctx, filter, metaV1.DeleteOptions{})
				if err1 != nil {
					ctxLogger.Errorf(LogErrFormat, eventType, common.EnvoyFilterResourceType, filter, rc.ClusterID, err1)
					err = common.AppendError(err, err1)
				}
			}
		}
	}
	if err == nil {
		ctxLogger.Infof(LogFormat, eventType, common.RoutingPolicyResourceType, fmt.Sprintf("%s.%s.%s", identity, env, routingPolicy.Name), "", "deleting from cache")
		r.RemoteRegistry.AdmiralCache.RoutingPolicyCache.Delete(identity, env, routingPolicy.Name)
		r.RemoteRegistry.AdmiralCache.RoutingPolicyFilterCache.Delete(key)
	}
	ctxLogger.Debugf(LogFormat, eventType, common.RoutingPolicyResourceType, routingPolicy.Name, "-", "end of delete for routing policy")
	return err
}

func NewRoutingPolicyProcessor(remoteRegistry *RemoteRegistry) RoutingPolicyProcessor {
	return &RoutingPolicyService{RemoteRegistry: remoteRegistry}
}

type routingPolicyCacheEntry struct {
	// key: env, value: map [ key: name, value: routingPolicy]
	policiesByEnv map[string]map[string]*v1.RoutingPolicy
}

type RoutingPolicyCache struct {
	// key: identity of the asset
	// value: routingPolicyCacheEntry
	entries map[string]*routingPolicyCacheEntry
	mutex   *sync.Mutex
}

func NewRoutingPolicyCache() *RoutingPolicyCache {
	entries := make(map[string]*routingPolicyCacheEntry)
	return &RoutingPolicyCache{entries: entries, mutex: &sync.Mutex{}}
}

func (r *RoutingPolicyCache) GetForIdentity(identity string) []*v1.RoutingPolicy {
	defer r.mutex.Unlock()
	r.mutex.Lock()
	if r.entries[identity] == nil {
		return []*v1.RoutingPolicy{}
	}
	policies := make([]*v1.RoutingPolicy, 0)
	for _, envMap := range r.entries[identity].policiesByEnv {
		for _, rp := range envMap {
			policies = append(policies, rp)
		}
	}
	return policies
}

func (r *RoutingPolicyCache) Get(identity string, env string, name string) *v1.RoutingPolicy {

	defer r.mutex.Unlock()
	r.mutex.Lock()
	if (r.entries[identity] == nil) ||
		(r.entries[identity].policiesByEnv == nil) ||
		(r.entries[identity].policiesByEnv[env] == nil) ||
		(r.entries[identity].policiesByEnv[env][name] == nil) {
		return nil
	}
	return r.entries[identity].policiesByEnv[env][name]
}

func (r *RoutingPolicyCache) Put(identity string, env string, name string, rp *v1.RoutingPolicy) {
	if rp == nil {
		return
	}
	defer r.mutex.Unlock()
	r.mutex.Lock()
	if r.entries[identity] == nil {
		r.entries[identity] = &routingPolicyCacheEntry{policiesByEnv: make(map[string]map[string]*v1.RoutingPolicy)}
	}
	if r.entries[identity].policiesByEnv[env] == nil {
		r.entries[identity].policiesByEnv[env] = make(map[string]*v1.RoutingPolicy)
	}
	r.entries[identity].policiesByEnv[env][name] = rp
}

func (r *RoutingPolicyCache) Delete(identity string, env string, name string) {
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(LogFormat, admiral.Delete, "routingpolicy", fmt.Sprintf("%s.%s.%s", identity, env, name), "", "skipping read-only mode")
		return
	}
	if common.GetEnableRoutingPolicy() {
		defer r.mutex.Unlock()
		r.mutex.Lock()
		if r.entries[identity] != nil &&
			r.entries[identity].policiesByEnv != nil &&
			r.entries[identity].policiesByEnv[env] != nil &&
			r.entries[identity].policiesByEnv[env][name] != nil {
			delete(r.entries[identity].policiesByEnv[env], name)
			if len(r.entries[identity].policiesByEnv[env]) == 0 {
				delete(r.entries[identity].policiesByEnv, env)
			}
		}
	} else {
		log.Infof(LogFormat, admiral.Delete, "routingpolicy", fmt.Sprintf("%s.%s.%s", identity, env, name), "", "routingpolicy disabled")
	}
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
			log.Infof(LogFormat, "success", "routingpolicy", obj.Name, "", "Ignored the RoutingPolicy because of the annotation or label")
			return nil
		}
		dependents := getDependents(obj, r)
		if len(dependents) == 0 {
			log.Info("No dependents found for Routing Policy - ", obj.Name)
			return nil
		}
		err := r.RoutingPolicyService.ProcessAddOrUpdate(ctx, admiral.Add, obj, nil, dependents)
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

func (r RoutingPolicyHandler) Updated(ctx context.Context, newRP *v1.RoutingPolicy, oldRP *v1.RoutingPolicy) error {
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(LogFormat, admiral.Update, "routingpolicy", "", "", "skipping read-only mode")
		return nil
	}
	if common.GetEnableRoutingPolicy() {
		if common.ShouldIgnoreResource(newRP.ObjectMeta) {
			log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.RoutingPolicyResourceType,
				newRP.Name, newRP.Namespace, "", "Value=true")
			log.Infof(LogFormat, admiral.Update, "routingpolicy", newRP.Name, "", "Ignored the RoutingPolicy because of the annotation or label")
			// We need to process this as a delete event.
			r.Deleted(ctx, newRP)
			return nil
		}
		dependents := getDependents(newRP, r)
		if len(dependents) == 0 {
			return nil
		}
		err := r.RoutingPolicyService.ProcessAddOrUpdate(ctx, admiral.Update, newRP, oldRP, dependents)
		if err != nil {
			log.Errorf(LogErrFormat, admiral.Update, "routingpolicy", newRP.Name, "", "failed to process routing policy")
			return err
		}
		log.Infof(LogFormat, admiral.Update, "routingpolicy", newRP.Name, "", "updated routing policy")
	} else {
		log.Infof(LogFormat, admiral.Update, "routingpolicy", newRP.Name, "", "routingpolicy disabled")
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
	err := r.RoutingPolicyService.Delete(ctx, admiral.Delete, obj)
	if err != nil {
		log.Infof(LogFormat, admiral.Delete, "routingpolicy", obj.Name, "", "deleted envoy filter for routing policy")
	}
	return err
}
