package clusters

import (
	"context"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"strconv"
	"time"
)

type TrafficConfigProcessor interface {
	Process(ctx context.Context, tc *v1.TrafficConfig,
		remoteRegistry *RemoteRegistry, eventType admiral.EventType,
		modifySE ModifySEFunc) error
}

// ProcessTrafficConfigService is a struct implementing processing logic
type ProcessTrafficConfigService struct {
}

// TrafficConfigHandler is responsible for handling changes to TrafficConfig
type TrafficConfigHandler struct {
	RemoteRegistry          *RemoteRegistry
	TrafficConfigController *admiral.TrafficConfigController
	TrafficConfigProcessor  TrafficConfigProcessor
}

// Added method to handle new TrafficConfig additions
func (th *TrafficConfigHandler) Added(ctx context.Context, obj *v1.TrafficConfig) error {
	log.Debugf(LogFormat, common.Add, common.TrafficConfigResourceType, obj.Name, "", common.ReceivedStatus)
	slowStartConfigs, err := th.populateCaches(obj, admiral.Add)
	if err != nil {
		return err
	}
	return th.HandleTrafficConfigRecord(ctx, obj, th.RemoteRegistry, admiral.Add, slowStartConfigs)
}

// Updated method to handle updates to existing TrafficConfig
func (th *TrafficConfigHandler) Updated(ctx context.Context, obj *v1.TrafficConfig) error {
	log.Debugf(LogFormat, common.Update, common.TrafficConfigResourceType, obj.Name, "", common.ReceivedStatus)
	slowStartConfigs, err := th.populateCaches(obj, admiral.Update)
	if err != nil {
		return err
	}
	return th.HandleTrafficConfigRecord(ctx, obj, th.RemoteRegistry, admiral.Update, slowStartConfigs)
}

// Deleted method to handle deletions of TrafficConfig
func (th *TrafficConfigHandler) Deleted(ctx context.Context, obj *v1.TrafficConfig) error {
	log.Debugf(LogFormat, common.Delete, common.TrafficConfigResourceType, obj.Name, "", common.ReceivedStatus)
	slowStartConfigs, err := th.populateCaches(obj, admiral.Delete)
	if err != nil {
		return err
	}
	return th.HandleTrafficConfigRecord(ctx, obj, th.RemoteRegistry, admiral.Update, slowStartConfigs)
}

// HandleTrafficConfigRecord processes TrafficConfig records
func (th *TrafficConfigHandler) HandleTrafficConfigRecord(ctx context.Context, obj *v1.TrafficConfig, remoteRegistry *RemoteRegistry, eventType admiral.EventType, slowStartConfigs *common.Map) error {
	ctx = context.WithValue(ctx, common.EventResourceType, common.TrafficConfig)
	if IsCacheWarmupTime(remoteRegistry) {
		log.Debugf(LogFormat, string(eventType), common.TrafficConfigResourceType, obj.Name, "", "processing skipped during cache warm up state")
		return nil
	}

	if !common.IsTrafficConfigProcessingEnabledForSlowStart() {
		log.Infof(LogFormat, string(eventType), common.TrafficConfigResourceType, obj.Name, "", "trafficConfig processing is disabled")
		return nil
	}

	assetAlias := getTrafficConfigLabel(obj.Labels, common.TrafficConfigAssetLabelKey)
	assetEnv := getTrafficConfigLabel(obj.Labels, common.TrafficConfigEnvLabelKey)
	// Update the warmupDuration in the .local destinationRule
	for _, cachedWorkloadEnv := range slowStartConfigs.GetKeys() {
		// Find source clusters where this identity exists
		sourceClusters := remoteRegistry.AdmiralCache.IdentityClusterCache.Get(assetAlias).GetValues()

		if sourceClusters == nil || len(sourceClusters) == 0 {
			log.Warnf("No source clusters found for identity %s in environment %s", assetAlias, cachedWorkloadEnv)
			continue
		}

		// pass in the first sourceCluster in the list. This is required as modifySE relies on
		sourceCluster := sourceClusters[0]
		ctx := context.WithValue(ctx, common.ClusterName, sourceCluster)
		ctx = context.WithValue(ctx, common.TrafficConfigContextWorkloadEnvKey, cachedWorkloadEnv)

		log.Infof("Applying TrafficConfig %s to identity %s in environment %s for cluster %s",
			obj.Name, assetAlias, cachedWorkloadEnv, sourceCluster)

		// Call modify service entry for each cluster where this identity exists
		_, err := modifyServiceEntryForNewServiceOrPod(ctx, eventType, cachedWorkloadEnv, assetAlias, remoteRegistry)
		if err != nil {
			log.Warnf("Failed to update destinationRule for TrafficConfig %s in cluster %s: %v",
				obj.Name, sourceCluster, err)
			continue
		}

		// After the processing has completed, remove the workloadEnv from cache if the slowStartValue is blank.
		if remoteRegistry.AdmiralCache.SlowStartConfigCache.Get(assetAlias).Get(assetEnv).Get(cachedWorkloadEnv) == "" {
			remoteRegistry.AdmiralCache.SlowStartConfigCache.Get(assetAlias).Get(assetEnv).Delete(cachedWorkloadEnv)
		}
	}

	log.Infof("Successfully processed TrafficConfig %s", obj.Name)

	return nil
}

func (th *TrafficConfigHandler) populateCaches(obj *v1.TrafficConfig, eventType admiral.EventType) (*common.Map, error) {
	remoteRegistry := th.RemoteRegistry
	assetAlias := getTrafficConfigLabel(obj.Labels, common.TrafficConfigAssetLabelKey)
	assetEnv := getTrafficConfigLabel(obj.Labels, common.TrafficConfigEnvLabelKey)
	//
	//ctx = context.WithValue(ctx, common.TrafficConfigContextAssetKey, assetAlias)
	//ctx = context.WithValue(ctx, common.TrafficConfigContextEnvKey, assetEnv)
	// Implement logic here. Placeholder for now.
	slowStartConfigs := obj.Spec.EdgeService.SlowStartConfig
	if slowStartConfigs == nil {
		log.Warnf("No slowStartConfig found for TrafficConfig %s", obj.Name)
		return nil, nil
	}

	// Create a map of workloadEnvSelectors for quick lookup
	workloadEnvSelectorsMap := make(map[string]bool)
	// Iterate over each slowStartConfig
	for _, slowStartConfig := range slowStartConfigs {
		// Retrieve the workload environment selectors
		workloadEnvSelectors := slowStartConfig.WorkloadEnvSelectors

		// Retrieve the duration for the slow start
		duration, err := time.ParseDuration(slowStartConfig.Duration)
		if err != nil {
			log.Errorf("Failed to parse warmup duration specified for TrafficConfig %s: %v", obj.Name, err)
			return nil, err
		}

		secondsInt := int64(duration.Seconds())
		for _, workloadEnv := range workloadEnvSelectors {
			workloadEnvSelectorsMap[workloadEnv] = true
			if eventType == admiral.Delete {
				remoteRegistry.AdmiralCache.SlowStartConfigCache.Get(assetAlias).Get(assetEnv).Put(workloadEnv, "")
			} else {
				if obj.Annotations["isSlowStartDisabled"] == "true" {
					remoteRegistry.AdmiralCache.SlowStartConfigCache.Put(assetAlias, assetEnv, workloadEnv, "")
				} else {
					remoteRegistry.AdmiralCache.SlowStartConfigCache.Put(assetAlias, assetEnv, workloadEnv, strconv.FormatInt(secondsInt, 10))
				}
			}
		}
	}

	var workloadEnvsNotFoundInTrafficConfig []string
	slowStartConfigsForAllWorkloadEnvs := remoteRegistry.AdmiralCache.SlowStartConfigCache.Get(assetAlias).Get(assetEnv)
	if slowStartConfigsForAllWorkloadEnvs != nil {
		// Iterate through cached workload environments
		slowStartConfigsForAllWorkloadEnvs.Range(func(cachedEnv, _ string) {
			// If this env is not in the current TrafficConfig's selectors, add it to missing list
			if _, exists := workloadEnvSelectorsMap[cachedEnv]; !exists {
				workloadEnvsNotFoundInTrafficConfig = append(workloadEnvsNotFoundInTrafficConfig, cachedEnv)
				log.Infof("Found workload environment in cache that is not in TrafficConfig: asset=%s, env=%s",
					assetAlias, cachedEnv)
				// set the value for slowStart for this workloadEnv to blank
				remoteRegistry.AdmiralCache.SlowStartConfigCache.Put(assetEnv, assetEnv, cachedEnv, "")
			}
		})
	}
	return slowStartConfigsForAllWorkloadEnvs, nil
}

func getTrafficConfigLabel(labels map[string]string, key string) string {
	if asset, exists := labels[key]; exists {
		return asset
	}
	return ""
}
