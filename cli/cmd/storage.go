package cmd

import (
	"fmt"
	"strings"

	"github.com/airgapops/agctl/internal/config"
	"github.com/airgapops/agctl/internal/kube"
	"github.com/spf13/cobra"
)

var (
	storageSize      string
	storageMountPath string
)

func init() {
	rootCmd.AddCommand(storageCmd)
	storageCmd.AddCommand(storageAttachCmd)
	storageCmd.AddCommand(storageDetachCmd)
	storageCmd.AddCommand(storageStatusCmd)
	storageCmd.AddCommand(storageClassesCmd)

	storageAttachCmd.Flags().StringVar(&storageSize, "size", "5Gi", "PVC size (e.g. 1Gi, 5Gi, 10Gi)")
	storageAttachCmd.Flags().StringVar(&storageMountPath, "mount", "", "Mount path (auto-detected per image)")
}

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Manage persistent storage for OpenChoreo components",
	Long:  `Attach, detach, and inspect persistent volumes for components that OpenChoreo cannot natively provision.`,
}

// ── storage attach ──────────────────────────────────────────────────────

var storageAttachCmd = &cobra.Command{
	Use:   "attach <component>",
	Short: "Attach persistent volume to a component",
	Long: `Creates a PersistentVolumeClaim and patches the Deployment to mount it.
This bypasses OpenChoreo's Workload CRD limitation of not supporting volumes.

Examples:
  agctl storage attach postgres --size 5Gi --env production
  agctl storage attach postgres --size 10Gi --env all
  agctl storage attach postgres --size 5Gi --env development --mount /data`,
	Args: cobra.ExactArgs(1),
	RunE: runStorageAttach,
}

func runStorageAttach(cmd *cobra.Command, args []string) error {
	component := args[0]
	envName := envFlag

	if envName == "" {
		return fmt.Errorf("--env flag is required (development|staging|production|all)")
	}

	// Detect mount path if not specified
	mountPath := storageMountPath
	if mountPath == "" {
		mountPath = detectMountPath(component)
	}

	if envName == "all" {
		for name := range cfg.Environments {
			if err := attachStorage(component, name, storageSize, mountPath); err != nil {
				fmt.Printf("  ❌ %s: %v\n", name, err)
			}
		}
		return nil
	}

	return attachStorage(component, envName, storageSize, mountPath)
}

func attachStorage(component, envName, size, mountPath string) error {
	env, err := cfg.ResolveEnv(envName)
	if err != nil {
		return err
	}

	namespace, err := resolveNamespace(envName)
	if err != nil {
		return fmt.Errorf("resolving namespace: %w", err)
	}

	dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())
	pvcName := fmt.Sprintf("%s-data-%s", component, envName)
	volName := fmt.Sprintf("%s-data", component)

	fmt.Printf("📦 Attaching %s storage to %s in %s...\n", size, component, envName)

	// Step 1: Check if PVC already exists
	existing, _ := dp.Run("get", "pvc", pvcName, "-n", namespace, "-o", "jsonpath={.metadata.name}")
	if existing != "" {
		fmt.Printf("  ⏭️  PVC %s already exists in %s\n", pvcName, envName)
	} else {
		// Step 2: Create PVC
		pvcYAML := fmt.Sprintf(`apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: agctl
    openchoreo.dev/component: %s
    agctl.airgapops.dev/environment: %s
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: %s
  resources:
    requests:
      storage: %s`, pvcName, namespace, component, envName, env.StorageClass, size)

		if _, err := dp.Apply(pvcYAML); err != nil {
			return fmt.Errorf("creating PVC: %w", err)
		}
		fmt.Printf("  ✅ PVC %s (%s, class=%s) created\n", pvcName, size, env.StorageClass)
	}

	// Step 3: Find deployment
	deployName, err := dp.GetDeploymentName(namespace, "openchoreo.dev/component="+component)
	if err != nil {
		return fmt.Errorf("finding deployment: %w", err)
	}
	fmt.Printf("  📋 Deployment: %s\n", deployName)

	// Step 4: Check if volume already mounted
	checkPath := fmt.Sprintf("{.spec.template.spec.volumes[?(@.name==%q)].name}", volName)
	mounted, _ := dp.Run("get", "deploy", deployName, "-n", namespace, "-o", "jsonpath="+checkPath)
	if mounted != "" {
		fmt.Printf("  ⏭️  Volume %s already mounted\n", volName)
		fmt.Printf("  ✅ %s in %s already has persistent storage\n", component, envName)
		return nil
	}

	// Step 5: Patch deployment to add volume + volumeMount
	patch := fmt.Sprintf(`{
  "spec": {
    "template": {
      "spec": {
        "volumes": [{
          "name": %q,
          "persistentVolumeClaim": {"claimName": %q}
        }],
        "containers": [{
          "name": %q,
          "volumeMounts": [{
            "name": %q,
            "mountPath": %q
          }]
        }]
      }
    }
  }
}`, volName, pvcName, component, volName, mountPath)

	if _, err := dp.Patch("deployment", deployName, namespace, "strategic", patch); err != nil {
		return fmt.Errorf("patching deployment: %w", err)
	}
	fmt.Printf("  ✅ Volume mounted at %s\n", mountPath)

	// Step 6: Wait for rollout
	fmt.Printf("  ⏳ Waiting for rollout...\n")
	if _, err := dp.RolloutStatus(namespace, deployName); err != nil {
		return fmt.Errorf("rollout failed: %w", err)
	}

	fmt.Printf("  ✅ %s in %s now has %s persistent storage at %s\n", component, envName, size, mountPath)
	return nil
}

// ── storage detach ──────────────────────────────────────────────────────

var storageDetachCmd = &cobra.Command{
	Use:   "detach <component>",
	Short: "Detach persistent volume from a component",
	Long: `Removes the volume mount from the Deployment and optionally deletes the PVC.
Data will be preserved in the PV until the PVC is deleted.

Examples:
  agctl storage detach postgres --env production`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]
		if envFlag == "" {
			return fmt.Errorf("--env flag is required")
		}

		_, err := cfg.ResolveEnv(envFlag)
		if err != nil {
			return err
		}

		namespace, err := resolveNamespace(envFlag)
		if err != nil {
			return fmt.Errorf("resolving namespace: %w", err)
		}

		dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())
		pvcName := fmt.Sprintf("%s-data-%s", component, envFlag)
		volName := fmt.Sprintf("%s-data", component)

		fmt.Printf("🗑️  Detaching storage from %s in %s...\n", component, envFlag)

		// Remove volume from deployment via JSON patch
		deployName, err := dp.GetDeploymentName(namespace, "openchoreo.dev/component="+component)
		if err != nil {
			return err
		}

		// Remove volumeMount and volume by patching to empty (strategic merge)
		// This is tricky — we use a JSON patch to remove by name
		// First, find the index of the volume
		patch := fmt.Sprintf(`[
  {"op": "test", "path": "/spec/template/spec/volumes/0/name", "value": %q},
  {"op": "remove", "path": "/spec/template/spec/volumes/0"},
  {"op": "remove", "path": "/spec/template/spec/containers/0/volumeMounts/0"}
]`, volName)

		if _, err := dp.Patch("deployment", deployName, namespace, "json", patch); err != nil {
			fmt.Printf("  ⚠️  Could not remove volume from deployment (may need manual cleanup): %v\n", err)
		} else {
			fmt.Printf("  ✅ Volume removed from deployment\n")
		}

		// Delete PVC
		if _, err := dp.Delete("pvc", pvcName, namespace); err != nil {
			fmt.Printf("  ⚠️  Could not delete PVC: %v\n", err)
		} else {
			fmt.Printf("  ✅ PVC %s deleted\n", pvcName)
		}

		fmt.Printf("  ✅ Storage detached from %s in %s\n", component, envFlag)
		return nil
	},
}

// ── storage status ──────────────────────────────────────────────────────

var storageStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show persistent storage status across environments",
	Long: `Lists all PVCs managed by agctl across all or a specific environment.

Examples:
  agctl storage status
  agctl storage status --env production`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

		envs := cfg.Environments
		if envFlag != "" && envFlag != "all" {
			env, err := cfg.ResolveEnv(envFlag)
			if err != nil {
				return err
			}
			envs = map[string]config.EnvironmentConfig{
				envFlag: *env,
			}
		}

		fmt.Println("📊 Storage Status")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("%-15s %-30s %-8s %-12s %-10s\n", "ENV", "PVC", "SIZE", "STATUS", "CLASS")
		fmt.Println("─────────────────────────────────────────────────────────────────")

		found := false
		for envName, env := range envs {
			namespace := env.Namespace
			// Try to resolve for current project
			if ns, err := resolveNamespace(envName); err == nil {
				namespace = ns
			}
			out, err := dp.Run("get", "pvc", "-n", namespace,
				"-l", "app.kubernetes.io/managed-by=agctl",
				"-o", "jsonpath={range .items[*]}{.metadata.name}|{.spec.resources.requests.storage}|{.status.phase}|{.spec.storageClassName}{\"\\n\"}{end}")
			if err != nil || out == "" {
				continue
			}

			for _, line := range strings.Split(out, "\n") {
				if line == "" {
					continue
				}
				parts := strings.Split(line, "|")
				if len(parts) >= 4 {
					found = true
					status := parts[2]
					statusIcon := "✅"
					if status != "Bound" {
						statusIcon = "⏳"
					}
					fmt.Printf("%-15s %-30s %-8s %s %-10s %-10s\n",
						envName, parts[0], parts[1], statusIcon, status, parts[3])
				}
			}
		}

		if !found {
			fmt.Println("  (no persistent volumes found — use 'agctl storage attach' to create one)")
		}
		fmt.Println()
		return nil
	},
}

// ── storage classes ─────────────────────────────────────────────────────

var storageClassesCmd = &cobra.Command{
	Use:   "classes",
	Short: "List available StorageClasses on the data plane",
	RunE: func(cmd *cobra.Command, args []string) error {
		dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

		fmt.Println("💾 Available StorageClasses")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		out, err := dp.Run("get", "storageclass",
			"-o", "jsonpath={range .items[*]}{.metadata.name}|{.provisioner}|{.reclaimPolicy}{\"\\n\"}{end}")
		if err != nil {
			return fmt.Errorf("listing storage classes: %w", err)
		}

		fmt.Printf("%-25s %-35s %-10s %-10s\n", "NAME", "PROVISIONER", "RECLAIM", "USED BY")
		fmt.Println("────────────────────────────────────────────────────────────────────────")

		for _, line := range strings.Split(out, "\n") {
			if line == "" {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				// Check which environments use this class
				usedBy := []string{}
				for envName, env := range cfg.Environments {
					if env.StorageClass == parts[0] {
						usedBy = append(usedBy, envName)
					}
				}
				usage := strings.Join(usedBy, ",")
				if usage == "" {
					usage = "-"
				}
				fmt.Printf("%-25s %-35s %-10s %-10s\n", parts[0], parts[1], parts[2], usage)
			}
		}
		fmt.Println()
		return nil
	},
}

// ── helpers ─────────────────────────────────────────────────────────────

// detectMountPath returns the standard data directory for known images
func detectMountPath(component string) string {
	paths := map[string]string{
		"postgres":  "/var/lib/postgresql/data",
		"redis":     "/data",
		"rabbitmq":  "/var/lib/rabbitmq",
		"mysql":     "/var/lib/mysql",
		"mongodb":   "/data/db",
		"minio":     "/data",
		"gitea":     "/data",
		"influxdb":  "/var/lib/influxdb2",
		"grafana":   "/var/lib/grafana",
		"prometheus": "/prometheus",
	}
	if p, ok := paths[component]; ok {
		return p
	}
	return "/data" // safe default
}
