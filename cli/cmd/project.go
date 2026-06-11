package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/airgapops/agctl/internal/kube"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectStatusCmd)
}

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage OpenChoreo projects",
	Long: `Projects provide namespace isolation in OpenChoreo.
Each project gets its own set of namespaces per environment.

Examples:
  agctl project create api
  agctl project list
  agctl project status api`,
}

// ── project create ──────────────────────────────────────────────────────

var projectCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new OpenChoreo project",
	Long: `Creates a new Project CRD in OpenChoreo.
OpenChoreo will automatically create data plane namespaces for each environment.

Examples:
  agctl project create api
  agctl project create monitoring`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cp := kube.NewControlPlaneClient()

		fmt.Printf("\n📂 Creating project %q\n", name)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Check if exists
		existing, _ := cp.Run("get", "project", name, "-o", "jsonpath={.metadata.name}")
		if existing != "" {
			fmt.Printf("  ⏭️  Project %q already exists\n\n", name)
			return nil
		}

		// Create Project CRD
		fmt.Printf("  ➊ Creating Project CRD...")
		projectYAML := fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: %s
  annotations:
    openchoreo.dev/display-name: "%s"
    openchoreo.dev/description: "Created by agctl"
spec:
  deploymentPipeline:
    name: default`, name, name)

		if _, err := cp.Apply(projectYAML); err != nil {
			return fmt.Errorf("creating Project: %w", err)
		}
		fmt.Printf(" ✅\n")

		// Wait for namespaces to be created
		fmt.Printf("  ➋ Waiting for namespaces...")
		dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())
		prefix := fmt.Sprintf("dp-%s-", name)

		var foundNS []string
		for i := 0; i < 30; i++ {
			time.Sleep(2 * time.Second)
			out, err := dp.Run("get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}")
			if err != nil {
				continue
			}
			foundNS = nil
			for _, ns := range strings.Fields(out) {
				if strings.HasPrefix(ns, prefix) {
					foundNS = append(foundNS, ns)
				}
			}
			if len(foundNS) >= len(cfg.Environments) {
				break
			}
		}

		if len(foundNS) == 0 {
			fmt.Printf(" ⚠️  No namespaces found yet (may take longer)\n")
		} else {
			fmt.Printf(" ✅ %d namespaces\n", len(foundNS))
			for _, ns := range foundNS {
				fmt.Printf("    📁 %s\n", ns)
			}
		}

		fmt.Printf("\n  ✅ Project %q created\n", name)
		fmt.Printf("  💡 Deploy: agctl deploy app myapp --project %s --env all\n\n", name)
		return nil
	},
}

// ── project list ────────────────────────────────────────────────────────

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all OpenChoreo projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		cp := kube.NewControlPlaneClient()

		fmt.Println()
		out, err := cp.Run("get", "projects", "-o",
			"custom-columns=NAME:.metadata.name,PIPELINE:.spec.deploymentPipeline.name,AGE:.metadata.creationTimestamp")
		if err != nil {
			return fmt.Errorf("listing projects: %w", err)
		}

		fmt.Println(out)

		// Also show namespaces per project
		dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())
		nsOut, _ := dp.Run("get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}")
		namespaces := strings.Fields(nsOut)

		fmt.Println("\n📁 Data Plane Namespaces:")
		for _, ns := range namespaces {
			if strings.HasPrefix(ns, "dp-") {
				fmt.Printf("  %s\n", ns)
			}
		}
		fmt.Println()

		return nil
	},
}

// ── project status ──────────────────────────────────────────────────────

var projectStatusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show project status with environments and pods",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project := projectFlag
		if len(args) > 0 {
			project = args[0]
		}

		dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())
		prefix := fmt.Sprintf("dp-%s-", project)

		fmt.Printf("\n📂 Project: %s\n", project)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		nsOut, _ := dp.Run("get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}")

		for _, ns := range strings.Fields(nsOut) {
			if !strings.HasPrefix(ns, prefix) {
				continue
			}

			// Extract env name from namespace
			envName := "unknown"
			for name, env := range cfg.Environments {
				if strings.Contains(ns, name) || strings.Contains(ns, env.Short) {
					envName = name
					break
				}
			}

			fmt.Printf("\n  🌍 %s (%s)\n", envName, ns)

			// Pods
			pods, _ := dp.Run("get", "pods", "-n", ns, "--no-headers",
				"-o", "custom-columns=NAME:.metadata.name,STATUS:.status.phase,RESTARTS:.status.containerStatuses[0].restartCount,AGE:.metadata.creationTimestamp")
			if pods != "" {
				for _, line := range strings.Split(pods, "\n") {
					if line != "" {
						fmt.Printf("    🟢 %s\n", line)
					}
				}
			} else {
				fmt.Printf("    (no pods)\n")
			}

			// Services
			svcs, _ := dp.Run("get", "svc", "-n", ns, "--no-headers",
				"-o", "custom-columns=NAME:.metadata.name,TYPE:.spec.type,PORTS:.spec.ports[*].port")
			if svcs != "" {
				for _, line := range strings.Split(svcs, "\n") {
					if line != "" {
						fmt.Printf("    🔗 svc/%s\n", line)
					}
				}
			}

			// PVCs
			pvcs, _ := dp.Run("get", "pvc", "-n", ns, "--no-headers",
				"-o", "custom-columns=NAME:.metadata.name,SIZE:.spec.resources.requests.storage,STATUS:.status.phase")
			if pvcs != "" && !strings.Contains(pvcs, "No resources") {
				for _, line := range strings.Split(pvcs, "\n") {
					if line != "" {
						fmt.Printf("    💾 pvc/%s\n", line)
					}
				}
			}

			// NetworkPolicies
			nps, _ := dp.Run("get", "networkpolicy", "-n", ns, "--no-headers", "-o", "custom-columns=NAME:.metadata.name")
			if nps != "" && !strings.Contains(nps, "No resources") {
				count := len(strings.Split(strings.TrimSpace(nps), "\n"))
				fmt.Printf("    🔒 %d NetworkPolicies active\n", count)
			}
		}

		fmt.Println()
		return nil
	},
}
