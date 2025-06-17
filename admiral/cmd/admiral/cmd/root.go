package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/routes"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/server"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/clusters"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	deploymentOrRolloutWorkerConcurrency = 5
	dependentClusterWorkerConcurrency    = 5
)

var (
	ctx, cancel = context.WithCancel(context.Background())
)

// GetRootCmd returns the root of the cobra command-tree.
func GetRootCmd(args []string) *cobra.Command {
	var ()

	params := common.AdmiralParams{LabelSet: &common.LabelSet{}}

	opts := routes.RouteOpts{}

	rootCmd := &cobra.Command{
		Use:          "Admiral",
		Short:        "Admiral is a control plane of control planes",
		Long:         "Admiral provides automatic configuration for multiple istio deployments to work as a single Mesh",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("%q is an invalid argument", args[0])
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.Level(params.LogLevel))
			if params.LogToFile {
				// open a file and rotate it at a certain size
				_, err := os.OpenFile(params.LogFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
				if err != nil {
					log.Error("error opening file for logging: " + err.Error() + " switching to stdout")
				} else {
					log.SetOutput(&lumberjack.Logger{
						Filename:   params.LogFilePath,
						MaxSize:    params.LogFileSizeInMBs, // megabytes
						MaxBackups: 10,
						MaxAge:     28, //days
					})
				}
			}
			log.Info("Starting Admiral")
			var (
				err            error
				remoteRegistry *clusters.RemoteRegistry
			)
			if params.AdmiralOperatorMode {
				remoteRegistry, err = clusters.InitAdmiralOperator(ctx, params)
			} else {
				remoteRegistry, err = clusters.InitAdmiral(ctx, params)
			}
			if err != nil {
				log.Fatalf("Error: %v", err)
			}

			if common.IsAdmiralDynamicConfigEnabled() {
				ctxDynamicConfig, cancel := context.WithCancel(context.Background())
				defer cancel()
				go clusters.UpdateASyncAdmiralConfig(ctxDynamicConfig, remoteRegistry, params.DynamicSyncPeriod)
			}

			// This is required for PERF tests only.
			// Perf tests requires remote registry object for validations.
			// There is no way to inject this object
			// There is no other away to propagate this object to perf suite
			if params.KubeconfigPath == loader.FakeKubeconfigPath {
				cmd.SetContext(context.WithValue(cmd.Context(), "remote-registry", remoteRegistry))
			}

			service := server.Service{}
			metricsService := server.Service{}
			opts.RemoteRegistry = remoteRegistry

			mainRoutes := routes.NewAdmiralAPIServer(&opts)
			metricRoutes := routes.NewMetricsServer()

			if err != nil {
				log.Error("Error setting up server:", err.Error())
			}

			wg := new(sync.WaitGroup)
			wg.Add(2)
			go func() {
				metricsService.Start(ctx, 6900, metricRoutes, routes.Filter, remoteRegistry)
				wg.Done()
			}()
			go func() {
				service.Start(ctx, 8080, mainRoutes, routes.Filter, remoteRegistry)
				wg.Done()
			}()
			wg.Wait()

			log.WithFields(log.Fields{
				"error": err.Error(),
			}).Fatal("Error setting up the server")

		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			shutdown(cancel)
		},
	}

	rootCmd.SetArgs(args)
	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	rootCmd.PersistentFlags().IntVar(&params.LogLevel, "log_level", int(log.InfoLevel),
		fmt.Sprintf("Set log verbosity, defaults to 'Info'. Must be between %v and %v", int(log.PanicLevel), int(log.TraceLevel)))
	rootCmd.PersistentFlags().BoolVar(&params.LogToFile, "log_to_file", false,
		"If enabled, use file to log instead of stdout")
	rootCmd.PersistentFlags().StringVar(&params.LogFilePath, "log_file_path", "/app/logs/admiral.log",
		"Path to log file. If not specified, defaults to /app/logs/admiral.log")
	rootCmd.PersistentFlags().IntVar(&params.LogFileSizeInMBs, "log_file_size_in_MBs", 200,
		"Size of the log file in Mbs. If not specified, defaults to 200 Mbs")
	rootCmd.PersistentFlags().StringVar(&params.KubeconfigPath, "kube_config", "",
		"Use a Kubernetes configuration file instead of in-cluster configuration")
	rootCmd.PersistentFlags().BoolVar(&params.ArgoRolloutsEnabled, "argo_rollouts", true,
		"Use argo rollout configurations")
	rootCmd.PersistentFlags().StringVar(&params.SecretFilterTags, "secret_filter_tags", "admiral/sync", "Filter tags for the specific admiral namespace secret to watch")
	rootCmd.PersistentFlags().StringVar(&params.ClusterRegistriesNamespace, "secret_namespace", "admiral",
		"Namespace to monitor for secrets defaults to admiral-secrets")
	rootCmd.PersistentFlags().StringVar(&params.DependenciesNamespace, "dependency_namespace", "admiral",
		"Namespace to monitor for changes to dependency objects")
	rootCmd.PersistentFlags().StringVar(&params.SyncNamespace, "sync_namespace", "admiral-sync",
		"Namespace in which Admiral will put its generated configurations")
	rootCmd.PersistentFlags().DurationVar(&params.CacheReconcileDuration, "sync_period", 5*time.Minute,
		"Interval for syncing Kubernetes resources, defaults to 5 min")
	rootCmd.PersistentFlags().DurationVar(&params.SeAndDrCacheReconcileDuration, "se_dr_sync_period", 5*time.Minute,
		"Interval for syncing ServiceEntries and DestinationRules resources, defaults to 5 min")
	rootCmd.PersistentFlags().BoolVar(&params.EnableSAN, "enable_san", false,
		"If SAN should be enabled for created Service Entries")
	rootCmd.PersistentFlags().StringVar(&params.SANPrefix, "san_prefix", "",
		"Prefix to use when creating SAN for Service Entries")
	rootCmd.PersistentFlags().StringVar(&params.Profile, "secret_resolver", common.AdmiralProfileDefault,
		"Type of resolver. Valid options - default|intuit")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.DeploymentAnnotation, "deployment_annotation", "sidecar.istio.io/inject",
		"The annotation, on a pod spec in a deployment, which must be set to \"true\" for Admiral to listen on the deployment")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.SubsetLabel, "subset_label", "subset",
		"The label, on a deployment, tells admiral which target group this deployment is a part of. Used for traffic splits via the Global Traffic Policy object")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.NamespaceSidecarInjectionLabel, "namespace_injected_label", "istio-injection",
		"The label key, on a namespace, which tells Istio to perform sidecar injection")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.NamespaceSidecarInjectionLabelValue, "namespace_injected_value", "enabled",
		"The label value, on a namespace or service, which tells Istio to perform sidecar injection")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.AdmiralIgnoreLabel, "admiral_ignore_label", "admiral-ignore",
		"The label value, on a namespace, which tells Istio to perform sidecar injection")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.PriorityKey, "priority_key", "priority",
		"The label value, on admiral resources, which tells admiral to give higher priority while processing admiral resource. Currently, this will be used for GlobalTrafficPolicy processing.")
	rootCmd.PersistentFlags().StringVar(&params.HostnameSuffix, "hostname_suffix", "global",
		"The hostname suffix to customize the cname generated by admiral. Default suffix value will be \"global\"")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.WorkloadIdentityKey, "workload_identity_key", "identity",
		"The workload identity  key, on deployment which holds identity value used to generate cname by admiral. Default label key will be \"identity\" Admiral will look for a label with this key. If present, that will be used. If not, it will try an annotation (for use cases where an identity is longer than 63 chars)")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.AdmiralCRDIdentityLabel, "admiral_crd_identity_label", "identity",
		"The label key which will be used to tie globaltrafficpolicy objects to deployments. Configured separately to the workload identity key because this one won't fall back to annotations.")
	rootCmd.PersistentFlags().StringVar(&params.WorkloadSidecarUpdate, "workload_sidecar_update", "disabled",
		"The parameter will be used to decide whether to update workload sidecar resource or not. By default these updates will be disabled.")
	rootCmd.PersistentFlags().StringVar(&params.WorkloadSidecarName, "workload_sidecar_name", "default",
		"Name of the sidecar resource in the workload namespace. By default sidecar resource will be named as \"default\".")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.EnvKey, "env_key", "admiral.io/env",
		"The annotation or label, on a pod spec in a deployment, which will be used to group deployments across regions/clusters under a single environment. Defaults to `admiral.io/env`. "+
			"The order would be to use annotation specified as `env_key`, followed by label specified as `env_key` and then fallback to the label `env`")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.GatewayApp, "gateway_app", "istio-ingressgateway",
		"The the value of the `app` label to use to match and find the service that represents the ingress for cross cluster traffic (AUTO_PASSTHROUGH mode)")
	rootCmd.PersistentFlags().StringVar(&params.AdmiralConfig, "secret_resolver_config_path", "/etc/config/resolver_config.yaml",
		"Path to the secret resolver config")
	rootCmd.PersistentFlags().BoolVar(&params.MetricsEnabled, "metrics", true, "Enable prometheus metrics collections")
	rootCmd.PersistentFlags().StringVar(&params.AdmiralStateCheckerName, "admiral_state_checker_name", "NoOPStateChecker", "The value of the admiral_state_checker_name label to configure the DR Strategy for Admiral")
	rootCmd.PersistentFlags().StringVar(&params.DRStateStoreConfigPath, "dr_state_store_config_path", "", "Location of config file which has details for data store. Ex:- Dynamo DB connection details")
	rootCmd.PersistentFlags().StringVar(&params.ServiceEntryIPPrefix, "se_ip_prefix", "240.0", "IP prefix for the auto generated IPs for service entries. Only the first two octets:  Eg- 240.0")
	rootCmd.PersistentFlags().StringVar(&params.EnvoyFilterVersion, "envoy_filter_version", "1.17,1.20",
		"The value of envoy filter version is used to match the proxy version for envoy filter created by routing policy")
	rootCmd.PersistentFlags().StringVar(&params.DeprecatedEnvoyFilterVersion, "deprecated_envoy_filter_version", "",
		"The value of envoy filter version which are deprecated and need to be removed from the clusters")
	rootCmd.PersistentFlags().StringVar(&params.EnvoyFilterAdditionalConfig, "envoy_filter_additional_config", "",
		"The value of envoy filter is to add additional config to the filter config section")
	rootCmd.PersistentFlags().BoolVar(&params.EnableRoutingPolicy, "enable_routing_policy", false,
		"If Routing Policy feature needs to be enabled")
	rootCmd.PersistentFlags().StringSliceVar(&params.RoutingPolicyClusters, "routing_policy_enabled_clusters", []string{}, "The destination clusters to enable routing policy filters on")
	rootCmd.PersistentFlags().StringSliceVar(&params.ExcludedIdentityList, "excluded_identity_list", []string{},
		"List of identities which should be excluded from getting processed")
	rootCmd.PersistentFlags().BoolVar(&params.EnableDiffCheck, "enable_diff_check", true, "Enable diff check")
	rootCmd.PersistentFlags().StringSliceVar(&params.AdditionalEndpointSuffixes, "additional_endpoint_suffixes", []string{},
		"Suffixes that Admiral should use to generate additional endpoints through VirtualServices")
	rootCmd.PersistentFlags().StringSliceVar(&params.AdditionalEndpointLabelFilters, "additional_endpoint_label_filters", []string{},
		"Labels that admiral will check on deployment/rollout before creating additional endpoints. '*' would indicate generating additional endpoints for all deployment/rollouts")
	rootCmd.PersistentFlags().BoolVar(&params.EnableWorkloadDataStorage, "enable_workload_data_storage", false,
		"When true, workload data will be stored in a persistent storage")
	rootCmd.PersistentFlags().BoolVar(&params.DisableDefaultAutomaticFailover, "disable_default_automatic_failover", false,
		"When set to true, automatic failover will only be enabled when there is a OutlierDetection CR or GTP defined with outlier configurations")
	rootCmd.PersistentFlags().BoolVar(&params.DisableIPGeneration, "disable_ip_generation", false, "When set to true, ips will not be generated and written to service entries")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.IdentityPartitionKey, "identity_partition_key", "admiral.io/identityPartition",
		"The annotation on a deployment/rollout spec, which will be used to divide an asset based on user-specified partition. Defaults to `admiral.io/identityPartition`.")
	rootCmd.PersistentFlags().StringSliceVar(&params.ExportToIdentityList, "exportto_identity_list", []string{"*"}, "List of identities to write ExportTo field for")
	rootCmd.PersistentFlags().IntVar(&params.ExportToMaxNamespaces, "exportto_max_namespaces", 35, "Max number of namespaces to write in ExportTo field before just replacing with *")

	// Admiral HA flags
	rootCmd.PersistentFlags().IntVar(&params.DNSRetries, "dns_retries", 3, "number of retries for dns resolution")
	rootCmd.PersistentFlags().IntVar(&params.DNSTimeoutMs, "dns_timeout_ms", 1000, "ttl for dns resolution timeout")
	rootCmd.PersistentFlags().StringVar(&params.DnsConfigFile, "dns_config_file", "/etc/resolv.conf", "the dns config file to use")
	rootCmd.PersistentFlags().BoolVar(&params.EnableProxyEnvoyFilter, "enable_proxy_envoy_filter", false,
		"When true, envoyfilter through dependency proxy will be generated")
	rootCmd.PersistentFlags().BoolVar(&params.EnableDependencyProcessing, "enable_dependency_processing", false,
		"When true, SE/DR/VS processing flow will be kicked in upon receiving any update event on dependency record")
	rootCmd.PersistentFlags().StringVar(&params.SeAddressConfigmap, "se_address_configmap", "se-address-configmap",
		"the confimap to use for generating se addresses (will be auto-created if does not exist)")
	rootCmd.PersistentFlags().BoolVar(&params.EnableOutlierDetection, "enable_outlierdetection", false, "Enable/Disable OutlierDetection")
	rootCmd.PersistentFlags().IntVar(&params.DeploymentOrRolloutWorkerConcurrency, "deployment_or_rollout_worker_concurrency", deploymentOrRolloutWorkerConcurrency,
		"Deployment/Rollout Controller worker concurrency")
	rootCmd.PersistentFlags().IntVar(&params.DependentClusterWorkerConcurrency, "dependent_cluster_worker_concurrency", dependentClusterWorkerConcurrency,
		"Dependent cluster worker concurrency")
	rootCmd.PersistentFlags().IntVar(&params.DependencyWarmupMultiplier, "dependency_warmup_multiplier", 2,
		"Dependency warmup multiplier is the time used for dependency proxy warmup time multiplied by cache warmup")
	rootCmd.PersistentFlags().Int32Var(&params.MaxRequestsPerConnection, "max_requests_per_connection", clusters.DefaultMaxRequestsPerConnection,
		"Maximum number of requests per connection to a backend. Setting this parameter to 1 disables keep alive. Default 100, can go up to 2^29.")
	rootCmd.PersistentFlags().BoolVar(&params.EnableServiceEntryCache, "enable_serviceentry_cache", false, "Enable/Disable Caching serviceentries")
	rootCmd.PersistentFlags().BoolVar(&params.EnableDestinationRuleCache, "enable_destinationrule_cache", false, "Enable/Disable Caching destinationrules")
	rootCmd.PersistentFlags().BoolVar(&params.EnableAbsoluteFQDN, "enable_absolute_fqdn", true, "Enable/Disable Absolute FQDN")
	rootCmd.PersistentFlags().StringSliceVar(&params.AlphaIdentityList, "alpha_identity_list", []string{},
		"Identities which can be used for testing of alpha features")
	rootCmd.PersistentFlags().BoolVar(&params.EnableAbsoluteFQDNForLocalEndpoints, "enable_absolute_fqdn_for_local_endpoints", false, "Enable/Disable Absolute FQDN for local endpoints")
	rootCmd.PersistentFlags().BoolVar(&params.EnableClientConnectionConfigProcessing, "enable_client_connection_config_processing", false, "Enable/Disable ClientConnectionConfig Processing")
	rootCmd.PersistentFlags().StringSliceVar(&params.GatewayAssetAliases, "gateway_asset_aliases", []string{"Intuit.platform.servicesgateway.servicesgateway"}, "The asset aliases used for API Gateway")
	rootCmd.PersistentFlags().BoolVar(&params.EnableActivePassive, "enable_active_passive", false, "Enable/Disable Active-Passive behavior")
	rootCmd.PersistentFlags().BoolVar(&params.EnableSWAwareNSCaches, "enable_sw_aware_ns_caches", false, "Enable/Disable SW Aware NS Caches")
	rootCmd.PersistentFlags().Int64Var(&params.DefaultWarmupDurationSecs, "default_warmup_duration_in_seconds", 45, "The default value for the warmupDurationSecs to be used on Destination Rules created by admiral")
	rootCmd.PersistentFlags().BoolVar(&params.EnableGenerationCheck, "enable_generation_check", true, "Enable/Disable Generation Check")
	rootCmd.PersistentFlags().BoolVar(&params.EnableIsOnlyReplicaCountChangedCheck, "enable_replica_count_check", true, "Enable/Disable Replica Count Check")
	rootCmd.PersistentFlags().BoolVar(&params.ClientInitiatedProcessingEnabledForControllers, "client_initiated_processing_enabled_for_controllers", false, "Enable/Disable Client Initiated Processing for controllers")
	rootCmd.PersistentFlags().BoolVar(&params.ClientInitiatedProcessingEnabledForDynamicConfig, "client_initiated_processing_enabled_for_dynamic_config", false, "Enable/Disable Client Initiated Processing via DB")
	rootCmd.PersistentFlags().StringSliceVar(&params.InitiateClientInitiatedProcessingFor, "initiate_client_initiated_processing_for", []string{}, "List of identities for which client initiated processing should be initiated")
	rootCmd.PersistentFlags().BoolVar(&params.PreventSplitBrain, "prevent_split_brain", true, "Enable/Disable Explicit Split Brain prevention logic")
	rootCmd.PersistentFlags().StringSliceVar(&params.IgnoreLabelsAnnotationsVSCopyList, "ignore_labels_annotations_vs_copy_list", []string{"applications.argoproj.io/app-name", "app.kubernetes.io/instance", "argocd.argoproj.io/tracking-id"}, "Labels and annotations that should not be preserved during VS copy")

	//Admiral 2.0 flags
	rootCmd.PersistentFlags().BoolVar(&params.AdmiralOperatorMode, "admiral_operator_mode", false, "Enable/Disable admiral operator functionality")
	rootCmd.PersistentFlags().BoolVar(&params.AdmiralStateSyncerMode, "admiral_state_syncer_mode", false, "Enable/Disable admiral to run as state syncer only")
	rootCmd.PersistentFlags().StringVar(&params.OperatorSyncNamespace, "operator_sync_namespace", "admiral-operator-sync",
		"Namespace in which Admiral Operator will put its generated configurations")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.ShardIdentityLabelKey, "shard_identity_label_key", "admiral.io/shardIdentity", "used to filter which shard Admiral Operator will watch")
	rootCmd.PersistentFlags().StringVar(&params.OperatorIdentityValue, "operator_identity_value", "", "Admiral operator should watch shards where operatorIdentityLabelKey == operatorIdentityValue")
	rootCmd.PersistentFlags().StringVar(&params.ShardIdentityValue, "shard_identity_value", "", "Admiral operator should watch shards where shardIdentityLabelKey == shardIdentityValue")
	rootCmd.PersistentFlags().StringVar(&params.OperatorSecretFilterTags, "operator_secret_filter_tags", "admiral/syncoperator",
		"Filter tags for the specific admiral operator namespace secret to watch")
	rootCmd.PersistentFlags().StringVar(&params.RegistryClientHost, "registry_client_host", "https://registry.com", "Host section of the url for a call to service registry")
	rootCmd.PersistentFlags().StringVar(&params.RegistryClientAppId, "registry_client_app_id", "admiral", "App Id for auth header")
	rootCmd.PersistentFlags().StringVar(&params.RegistryClientAppSecret, "registry_client_app_secret", "secrets/admiral/credentials/preprod", "Path to secret for auth header")
	rootCmd.PersistentFlags().StringVar(&params.RegistryClientBaseURI, "registry_client_base_uri", "v1", "Base URI section of of the url for a call to service registry")
	rootCmd.PersistentFlags().StringVar(&params.AdmiralAppEnv, "admiral_app_env", "preProd", "The env of admiral used for a call to service registry")
	rootCmd.PersistentFlags().StringSliceVar(&params.AdmiralStateSyncerClusters, "admiral_state_syncer_clusters", []string{}, "List of clusters that Admiral State Syncer syncs to registry for")

	// Flags pertaining to VS based routing
	rootCmd.PersistentFlags().BoolVar(&params.EnableVSRouting, "enable_vs_routing", false, "Enable/Disable VS Based Routing")
	rootCmd.PersistentFlags().StringSliceVar(&params.VSRoutingDisabledClusters, "vs_routing_disabled_clusters", []string{}, "The source clusters to disable VS based routing on")
	rootCmd.PersistentFlags().StringSliceVar(&params.VSRoutingSlowStartEnabledClusters, "vs_routing_slow_start_enabled_clusters", []string{}, "The source clusters to where VS routing is enabled and require slow start")
	rootCmd.PersistentFlags().StringSliceVar(&params.VSRoutingGateways, "vs_routing_gateways", []string{}, "The PASSTHROUGH gateways to use for VS based routing")
	rootCmd.PersistentFlags().StringSliceVar(&params.IngressVSExportToNamespaces, "ingress_vs_export_to_namespaces", []string{"istio-system"}, "List of namespaces where the ingress VS should be exported")
	rootCmd.PersistentFlags().StringVar(&params.IngressLBPolicy, "ingress_lb_policy", "round_robin", "loadbalancer policy for ingress destination rule (round_robin/random/passthrough/least_request)")

	// Flags pertaining to VS based routing in-cluster
	rootCmd.PersistentFlags().BoolVar(&params.EnableVSRoutingInCluster, "enable_vs_routing_in_cluster", false, "Enable/Disable VS Based Routing in cluster")
	// Usage:  --vs_routing_in_cluster_enabled_resources *=identity1,cluster2=*,cluster3="identity3, identity4" OR --vs_routing_in_cluster_enabled_resources *=* (enable for everything)
	rootCmd.PersistentFlags().StringToStringVarP(&params.VSRoutingInClusterEnabledResources, "vs_routing_in_cluster_enabled_resources", "e", map[string]string{}, "The source clusters and corresponding source identities to enable VS based routing in-cluster on")
	// Usage:  --vs_routing_in_cluster_disabled_resources *=identity1,cluster2=*,cluster3="identity3, identity4" OR --vs_routing_in_cluster_disabled_resources *=* (disable for everything)
	rootCmd.PersistentFlags().StringToStringVarP(&params.VSRoutingInClusterDisabledResources, "vs_routing_in_cluster_disabled_resources", "d", map[string]string{}, "The source clusters and corresponding source identities to disable VS based routing in-cluster on")
	rootCmd.PersistentFlags().BoolVar(&params.EnableCustomVSMerge, "enable_custom_vs_merge", false, "Enable/Disable custom VS merge with in cluster VS")
	rootCmd.PersistentFlags().StringVar(&params.ProcessVSCreatedBy, "process_vs_created_by", "", "process the VS that was createdBy. Add createdBy label and value provided here for admiral to process this VS")

	rootCmd.PersistentFlags().BoolVar(&params.EnableClientDiscovery, "enable_client_discovery", true, "Enable/Disable Client (mesh egress) Discovery")
	rootCmd.PersistentFlags().StringSliceVar(&params.ClientDiscoveryClustersForJobs, "client_discovery_clusters_for_jobs", []string{}, "List of clusters for client discovery for k8s jobs")
	rootCmd.PersistentFlags().StringSliceVar(&params.DiscoveryClustersForNumaflow, "client_discovery_clusters_for_numaflow", []string{}, "List of clusters for client discovery for numaflow types")

	//Parameter for DynamicConfigPush
	rootCmd.PersistentFlags().BoolVar(&params.EnableDynamicConfig, "enable_dynamic_config", true, "Enable/Disable Dynamic Configuration")
	rootCmd.PersistentFlags().StringVar(&params.DynamicConfigDynamoDBTableName, "dynamic_config_dynamodb_table_name", "dynamic-config-dev", "The name of the dynamic config dynamodb table")
	rootCmd.PersistentFlags().IntVar(&params.DynamicSyncPeriod, "dynamic_sync_period", 10, "Duration in min after which the dynamic sync get performed")

	//Parameter for NLB releated migration
	rootCmd.PersistentFlags().StringSliceVar(&params.NLBEnabledClusters, "nlb_enabled_clusters", []string{}, "Comma seperated list of enabled clusters to be enabled for NLB")
	rootCmd.PersistentFlags().StringSliceVar(&params.CLBEnabledClusters, "clb_enabled_clusters", []string{}, "Comma seperated list of enabled clusters to be enabled for CLB")
	rootCmd.PersistentFlags().StringSliceVar(&params.NLBEnabledIdentityList, "nlb_enabled_identity_list", []string{"intuit.services.mesh.meshhealthcheck"}, "Comma seperated list of enabled idenity list to be enabled for NLB in lower case")
	rootCmd.PersistentFlags().StringVar(&params.NLBIngressLabel, "nlb_ingress_label", common.NLBIstioIngressGatewayLabelValue, "The value of the `app` label to use to match and find the service that represents the NLB ingress for cross cluster traffic")
	rootCmd.PersistentFlags().StringVar(&params.CLBIngressLabel, "clb_ingress_label", common.IstioIngressGatewayLabelValue, "The value of the `app` label to use to match and find the service that represents the CLB ingress for cross cluster traffic")

	//Parameters for slow start
	rootCmd.PersistentFlags().BoolVar(&params.EnableTrafficConfigProcessingForSlowStart, "enable_traffic_config_processing_for_slow_start", false, "Enable/Disable TrafficConfig Processing for slowStart support")

	return rootCmd
}

func shutdown(cancelFunc context.CancelFunc) {

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Block until one of the signals above is received
	<-signalCh
	log.Info("Signal received, calling cancel func...")
	cancelFunc()
	// goodbye.
}
