package cmd

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/airgapops/agctl/internal/config"
	"github.com/airgapops/agctl/internal/kube"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show platform health across all environments",
	Long: `Displays a unified view of the platform:
  • Control Plane health (ThinkPad)
  • Data Plane pods per environment (Ryzen)
  • Gateway listener count
  • Certificate expiry
  • Storage volumes

Examples:
  agctl status
  agctl status --env production`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	cp := kube.NewControlPlaneClient()
	dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║              agctl — Platform Status                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Control Plane
	fmt.Println("🖥️  Control Plane (ThinkPad)")
	fmt.Println("───────────────────────────────────────────────────────────────")
	cpPods, err := cp.Run("get", "pods", "-n", "openchoreo-control-plane", "--no-headers")
	if err != nil {
		fmt.Printf("  ❌ Cannot reach control plane: %v\n", err)
	} else {
		running := 0
		total := 0
		for _, line := range strings.Split(cpPods, "\n") {
			if line == "" {
				continue
			}
			total++
			if strings.Contains(line, "Running") {
				running++
			}
		}
		icon := "✅"
		if running < total {
			icon = "⚠️"
		}
		fmt.Printf("  %s Pods: %d/%d running\n", icon, running, total)
	}

	// Components
	components, _ := cp.Run("get", "component", "-n", "default", "--no-headers",
		"-o", "custom-columns=NAME:.metadata.name")
	if components != "" {
		fmt.Printf("  📋 Components: %s\n", strings.Join(strings.Fields(components), ", "))
	}
	fmt.Println()

	// Environments
	envs := cfg.Environments
	if envFlag != "" && envFlag != "all" {
		env, err := cfg.ResolveEnv(envFlag)
		if err != nil {
			return err
		}
		envs = map[string]config.EnvironmentConfig{envFlag: *env}
	}

	for envName, env := range envs {
		fmt.Printf("🌍 %s (%s)\n", capitalize(envName), env.Short)
		fmt.Println("───────────────────────────────────────────────────────────────")

		// Pods
		pods, err := dp.Run("get", "pods", "-n", env.Namespace, "--no-headers",
			"-o", "custom-columns=NAME:.metadata.name,STATUS:.status.phase,COMPONENT:.metadata.labels.openchoreo\\.dev/component")
		if err != nil {
			fmt.Printf("  ❌ Cannot reach namespace %s\n", env.Namespace)
			continue
		}

		for _, line := range strings.Split(pods, "\n") {
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				icon := "✅"
				if fields[1] != "Running" {
					icon = "❌"
				}
				fmt.Printf("  %s %-40s %s [%s]\n", icon, fields[0], fields[1], fields[2])
			}
		}

		// PVCs
		pvcs, _ := dp.Run("get", "pvc", "-n", env.Namespace, "--no-headers",
			"-l", "app.kubernetes.io/managed-by=agctl",
			"-o", "custom-columns=NAME:.metadata.name,SIZE:.spec.resources.requests.storage,STATUS:.status.phase")
		if pvcs != "" {
			for _, line := range strings.Split(pvcs, "\n") {
				if line == "" {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) >= 3 {
					icon := "💾"
					fmt.Printf("  %s %-40s %s %s\n", icon, fields[0], fields[1], fields[2])
				}
			}
		}

		// Services
		svcs, _ := dp.Run("get", "svc", "-n", env.Namespace, "--no-headers",
			"-o", "custom-columns=NAME:.metadata.name,PORT:.spec.ports[0].port")
		if svcs != "" {
			svcList := []string{}
			for _, line := range strings.Split(svcs, "\n") {
				if line == "" {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					svcList = append(svcList, fmt.Sprintf("%s:%s", fields[0], fields[1]))
				}
			}
			fmt.Printf("  🔌 Services: %s\n", strings.Join(svcList, ", "))
		}
		fmt.Println()
	}

	// Gateway
	fmt.Println("🌐 Gateway")
	fmt.Println("───────────────────────────────────────────────────────────────")
	listeners, err := dp.Run("get", "gateway", cfg.Gateway.Name, "-n", cfg.Gateway.Namespace,
		"-o", "jsonpath={range .spec.listeners[*]}{.name}{\"\\n\"}{end}")
	if err != nil {
		fmt.Printf("  ❌ Cannot read gateway: %v\n", err)
	} else {
		count := 0
		for _, l := range strings.Split(listeners, "\n") {
			if l != "" {
				count++
			}
		}
		icon := "✅"
		warning := ""
		if count >= cfg.Gateway.MaxListeners {
			icon = "🚨"
			warning = " ← AT LIMIT!"
		} else if count >= cfg.Gateway.MaxListeners-1 {
			icon = "⚠️"
			warning = " ← 1 slot remaining"
		}
		fmt.Printf("  %s Listeners: %d/%d%s\n", icon, count, cfg.Gateway.MaxListeners, warning)
		fmt.Printf("  📍 IP: %s\n", cfg.Gateway.IP)
	}

	// Certs
	fmt.Println()
	fmt.Println("🔐 Certificates")
	fmt.Println("───────────────────────────────────────────────────────────────")
	certs, err := dp.Run("get", "certificate", "-n", cfg.Gateway.Namespace, "--no-headers",
		"-o", "custom-columns=NAME:.metadata.name,READY:.status.conditions[0].status,EXPIRY:.status.notAfter")
	if err != nil {
		fmt.Printf("  ⚠️  cert-manager not accessible\n")
	} else {
		for _, line := range strings.Split(certs, "\n") {
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				icon := "✅"
				if fields[1] != "True" {
					icon = "❌"
				}
				fmt.Printf("  %s %-35s ready=%s  expires=%s\n", icon, fields[0], fields[1], fields[2])
			}
		}
	}

	fmt.Println()
	return nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
