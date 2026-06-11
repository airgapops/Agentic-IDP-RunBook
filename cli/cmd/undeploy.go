package cmd

import (
	"fmt"
	"strings"

	"github.com/airgapops/agctl/internal/kube"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(undeployCmd)
}

var undeployCmd = &cobra.Command{
	Use:   "undeploy <name>",
	Short: "Remove a deployed application completely",
	Long: `Removes all resources created by 'agctl deploy':
  - HTTPRoute
  - Gateway Listener
  - TLS Certificate + Secret
  - NetworkPolicy
  - Service
  - DNS record
  - Workload CRD (control plane)
  - Component CRD (control plane)

🚨 This will NOT delete ConfigMaps or PVCs.

Examples:
  agctl undeploy myapp --env development
  agctl undeploy myapp --env all`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if envFlag == "" {
			return fmt.Errorf("--env flag is required")
		}

		if envFlag == "all" {
			for envName := range cfg.Environments {
				undeployApp(name, envName)
			}
			// Remove Component + Workload from control plane (once)
			removeControlPlaneResources(name)
			return nil
		}

		undeployApp(name, envFlag)
		removeControlPlaneResources(name)
		return nil
	},
}

func undeployApp(name, envName string) {
	env, err := cfg.ResolveEnv(envName)
	if err != nil {
		fmt.Printf("  ❌ %s: %v\n", envName, err)
		return
	}

	dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())
	gwNS := cfg.Gateway.Namespace
	listenerName := fmt.Sprintf("https-%s-%s", name, env.Short)

	fmt.Printf("\n🗑️  Removing %s from %s\n", name, envName)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// HTTPRoute
	fmt.Printf("  ➊ Removing HTTPRoute...")
	if _, err := dp.Delete("httproute", name+"-https-route", env.Namespace); err != nil {
		fmt.Printf(" ⏭️  not found\n")
	} else {
		fmt.Printf(" ✅\n")
	}

	// Gateway Listener (remove from array)
	fmt.Printf("  ➋ Removing Gateway listener...")
	listenersJSON, _ := dp.Run("get", "gateway", cfg.Gateway.Name, "-n", gwNS,
		"-o", "jsonpath={range .spec.listeners[*]}{.name}{\"\\n\"}{end}")
	idx := -1
	for i, l := range strings.Split(listenersJSON, "\n") {
		if l == listenerName {
			idx = i
			break
		}
	}
	if idx >= 0 {
		patch := fmt.Sprintf(`[{"op":"remove","path":"/spec/listeners/%d"}]`, idx)
		if _, err := dp.Patch("gateway", cfg.Gateway.Name, gwNS, "json", patch); err != nil {
			fmt.Printf(" ⚠️  %v\n", err)
		} else {
			fmt.Printf(" ✅\n")
		}
	} else {
		fmt.Printf(" ⏭️  not found\n")
	}

	// Certificate
	certName := fmt.Sprintf("%s-%s-cert", name, env.Short)
	secretName := fmt.Sprintf("%s-%s-tls-secret", name, env.Short)
	fmt.Printf("  ➌ Removing TLS certificate...")
	dp.Delete("certificate", certName, gwNS)
	dp.Delete("secret", secretName, gwNS)
	fmt.Printf(" ✅\n")

	// NetworkPolicy
	fmt.Printf("  ➍ Removing NetworkPolicy...")
	dp.Delete("networkpolicy", "allow-gateway-"+name, env.Namespace)
	fmt.Printf(" ✅\n")

	// Service
	fmt.Printf("  ➎ Removing Service...")
	dp.Delete("service", name, env.Namespace)
	fmt.Printf(" ✅\n")

	fmt.Printf("  ✅ Data plane resources removed for %s\n", envName)
}

func removeControlPlaneResources(name string) {
	cp := kube.NewControlPlaneClient()

	fmt.Printf("\n  ➏ Removing Workload CRD...")
	if _, err := cp.Run("delete", "workload", name, "-n", cfg.Defaults.Project, "--ignore-not-found"); err != nil {
		fmt.Printf(" ⚠️  %v\n", err)
	} else {
		fmt.Printf(" ✅\n")
	}

	fmt.Printf("  ➐ Removing Component CRD...")
	if _, err := cp.Run("delete", "component", name, "-n", cfg.Defaults.Project, "--ignore-not-found"); err != nil {
		fmt.Printf(" ⚠️  %v\n", err)
	} else {
		fmt.Printf(" ✅\n")
	}

	fmt.Printf("\n  🗑️  Component fully removed\n\n")
}
