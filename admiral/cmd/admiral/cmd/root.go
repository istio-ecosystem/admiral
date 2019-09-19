package cmd

import (
	"context"
	"flag"
	"fmt"
	"github.com/admiral/admiral/pkg/clusters"
	"istio.io/istio/pkg/log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var (
	loggingOptions = log.DefaultOptions()
	ctx, cancel    = context.WithCancel(context.Background())
)

// GetRootCmd returns the root of the cobra command-tree.
func GetRootCmd(args []string) *cobra.Command {

	var ()

	params := clusters.AdmiralParams{}

	rootCmd := &cobra.Command{
		Use:          "Admiral",
		Short:        "Admiral is a control plane of control planes",
		Long:         "Admiral provides automatic configuration for multiple istio deployments to work as a single Mesh",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("%q is an invalid argument", args[0])
			}
			err := log.Configure(loggingOptions)
			return err
		},
		Run: func(cmd *cobra.Command, args []string) {
			log.Info("Starting Admiral")
			_, err := clusters.InitAdmiral(ctx, params)

			if err != nil {
				log.Fatalf("Error: %v", err)
			}

		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			shutdown(cancel)

		},
	}

	rootCmd.SetArgs(args)
	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	rootCmd.PersistentFlags().StringVar(&params.KubeconfigPath, "kube_config", "",
		"Use a Kubernetes configuration file instead of in-cluster configuration")
	rootCmd.PersistentFlags().StringVar(&params.ClusterRegistriesNamespace, "secret_namespace", "default",
		"Namespace to monitor for secrets defaults to admiral-secrets")
	rootCmd.PersistentFlags().StringVar(&params.DependenciesNamespace, "dependency_namespace", "default",
		"Namespace to monitor for secrets defaults to admiral-secrets")
	rootCmd.PersistentFlags().StringVar(&params.SyncNamespace, "sync_namespace", "admiral-sync",
		"Namespace to monitor for secrets defaults to admiral-secrets")
	rootCmd.PersistentFlags().DurationVar(&params.CacheRefreshDuration, "sync_period", 5*time.Minute,
		"Interval for syncing Kubernetes resources, defaults to 5 min")
	rootCmd.PersistentFlags().BoolVar(&params.EnableSAN, "enable_san", false,
		"If SAN should be enabled for created Service Entries")
	rootCmd.PersistentFlags().StringVar(&params.SANPrefix, "san_prefix", "",
		"Prefix to use when creating SAN for Service Entries")
	rootCmd.PersistentFlags().StringVar(&params.SecretResolver, "secret_resolver", "",
		"Type of resolver to use to fetch kubeconfig for monitored clusters")
	loggingOptions.AttachCobraFlags(rootCmd)

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
