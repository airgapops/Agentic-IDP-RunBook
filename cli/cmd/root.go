package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/airgapops/agctl/internal/config"
	"github.com/airgapops/agctl/internal/kube"
	"github.com/spf13/cobra"
)

var (
	cfgFile     string
	envFlag     string
	projectFlag string
	cfg         *config.Config
	Version     = "0.3.0"
)

var rootCmd = &cobra.Command{
	Use:   "agctl",
	Short: "airgapOps CLI — Manage OpenChoreo workloads in airgap environments",
	Long: `agctl extends OpenChoreo with capabilities it doesn't natively support:
  • Full-lifecycle deployment (Component → HTTPS in 1 command)
  • Multi-project namespace isolation
  • Persistent storage (PVC/PV) for stateful workloads
  • Environment-aware secrets that bypass webhook masking
  • NetworkPolicy testing and management
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
		// Default project from config
		if projectFlag == "" {
			projectFlag = cfg.Defaults.Project
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
	rootCmd.PersistentFlags().StringVarP(&projectFlag, "project", "p", "", "OpenChoreo project (default: from config)")

	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print agctl version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("agctl v%s — airgapOps CLI\n", Version)
	},
}

// resolveNamespace resolves the data plane namespace for a given environment
// and the current project. For non-default projects, it discovers namespaces
// by querying kubectl for namespaces matching the pattern.
func resolveNamespace(envName string) (string, error) {
	ns, err := cfg.ResolveNamespace(projectFlag, envName)
	if err != nil {
		return "", err
	}

	// If we got a discovery marker, query kubectl for the actual namespace
	if strings.HasPrefix(ns, "PROJECT_DISCOVER:") {
		parts := strings.SplitN(ns, ":", 3)
		project := parts[1]
		env := parts[2]
		return discoverNamespace(project, env)
	}

	return ns, nil
}

// discoverNamespace finds the data plane namespace for a non-default project
func discoverNamespace(project, envName string) (string, error) {
	dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())
	prefix := fmt.Sprintf("dp-%s-default-%s-", project, envName)

	out, err := dp.Run("get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}")
	if err != nil {
		return "", fmt.Errorf("listing namespaces: %w", err)
	}

	for _, ns := range strings.Fields(out) {
		if strings.HasPrefix(ns, prefix) {
			return ns, nil
		}
	}

	return "", fmt.Errorf("no namespace found for project=%q env=%q (looking for %s*)", project, envName, prefix)
}

// forEachEnv runs a function for each environment when --env=all, or just the specified one
func forEachEnv(fn func(envName, namespace string) error) error {
	if envFlag == "" {
		return fmt.Errorf("--env flag is required (development|staging|production|all)")
	}

	if envFlag == "all" {
		var lastErr error
		for envName := range cfg.Environments {
			ns, err := resolveNamespace(envName)
			if err != nil {
				fmt.Printf("  ⚠️  %s: %v\n", envName, err)
				lastErr = err
				continue
			}
			if err := fn(envName, ns); err != nil {
				fmt.Printf("  ❌ %s: %v\n", envName, err)
				lastErr = err
			}
		}
		return lastErr
	}

	ns, err := resolveNamespace(envFlag)
	if err != nil {
		return err
	}
	return fn(envFlag, ns)
}
