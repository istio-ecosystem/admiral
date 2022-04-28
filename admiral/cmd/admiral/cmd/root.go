package cmd

import (
	"context"
	"flag"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/routes"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/server"
	"github.com/istio-ecosystem/admiral/admiral/pkg/clusters"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
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
			log.Info("Starting Admiral")
			remoteRegistry, err := clusters.InitAdmiral(ctx, params)

			if err != nil {
				log.Fatalf("Error: %v", err)
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
	rootCmd.PersistentFlags().StringVar(&params.KubeconfigPath, "kube_config", "",
		"Use a Kubernetes configuration file instead of in-cluster configuration")
	rootCmd.PersistentFlags().BoolVar(&params.ArgoRolloutsEnabled, "argo_rollouts", false,
		"Use argo rollout configurations")
	rootCmd.PersistentFlags().StringVar(&params.ClusterRegistriesNamespace, "secret_namespace", "admiral",
		"Namespace to monitor for secrets defaults to admiral-secrets")
	rootCmd.PersistentFlags().StringVar(&params.DependenciesNamespace, "dependency_namespace", "admiral",
		"Namespace to monitor for changes to dependency objects")
	rootCmd.PersistentFlags().StringVar(&params.SyncNamespace, "sync_namespace", "admiral-sync",
		"Namespace in which Admiral will put its generated configurations")
	rootCmd.PersistentFlags().DurationVar(&params.CacheRefreshDuration, "sync_period", 5*time.Minute,
		"Interval for syncing Kubernetes resources, defaults to 5 min")
	rootCmd.PersistentFlags().BoolVar(&params.EnableSAN, "enable_san", false,
		"If SAN should be enabled for created Service Entries")
	rootCmd.PersistentFlags().StringVar(&params.SANPrefix, "san_prefix", "",
		"Prefix to use when creating SAN for Service Entries")
	rootCmd.PersistentFlags().StringVar(&params.SecretResolver, "secret_resolver", "",
		"Type of resolver to use to fetch kubeconfig for monitored clusters")
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
	rootCmd.PersistentFlags().StringVar(&params.HostnameSuffix, "hostname_suffix", "global",
		"The hostname suffix to customize the cname generated by admiral. Default suffix value will be \"global\"")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.WorkloadIdentityKey, "workload_identity_key", "identity",
		"The workload identity  key, on deployment which holds identity value used to generate cname by admiral. Default label key will be \"identity\" Admiral will look for a label with this key. If present, that will be used. If not, it will try an annotation (for use cases where an identity is longer than 63 chars)")
	rootCmd.PersistentFlags().StringVar(&params.LabelSet.GlobalTrafficDeploymentLabel, "globaltraffic_deployment_label", "identity",
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
	rootCmd.PersistentFlags().BoolVar(&params.MetricsEnabled, "metrics", true, "Enable prometheus metrics collections")

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
