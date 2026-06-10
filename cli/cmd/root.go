package cmd

import (
	"fmt"
	"os"

	"github.com/airgapops/agctl/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	envFlag string
	cfg     *config.Config
	Version = "0.1.0"
)

var rootCmd = &cobra.Command{
	Use:   "agctl",
	Short: "airgapOps CLI — Manage OpenChoreo workloads in airgap environments",
	Long: `agctl extends OpenChoreo with capabilities it doesn't natively support:
  • Persistent storage (PVC/PV) for stateful workloads
  • Environment-aware secrets that bypass webhook masking
  • Cross-environment promotion with full HTTPS
  • Database operations (exec, seed, backup)
  • Platform health monitoring

Built for air-gapped Kubernetes clusters running OpenChoreo.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config loading for version/help
		if cmd.Name() == "version" || cmd.Name() == "help" {
			return nil
		}
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: /etc/agctl/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&envFlag, "env", "e", "", "target environment (development|staging|production|all)")

	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print agctl version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("agctl v%s — airgapOps CLI\n", Version)
	},
}
