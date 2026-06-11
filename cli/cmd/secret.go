package cmd

import (
	"fmt"
	"strings"

	"github.com/airgapops/agctl/internal/kube"
	"github.com/spf13/cobra"
)

var (
	revealFlag bool
	fromEnv    string
	toEnv      string
)

func init() {
	rootCmd.AddCommand(secretCmd)
	secretCmd.AddCommand(secretSetCmd)
	secretCmd.AddCommand(secretGetCmd)
	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretDiffCmd)
	secretCmd.AddCommand(secretSyncCmd)

	secretGetCmd.Flags().BoolVar(&revealFlag, "reveal", false, "Show actual secret values (use with caution)")
	secretDiffCmd.Flags().StringVar(&fromEnv, "from", "development", "Source environment")
	secretDiffCmd.Flags().StringVar(&toEnv, "to", "production", "Target environment")
	secretSyncCmd.Flags().StringVar(&fromEnv, "from", "", "Source environment")
	secretSyncCmd.Flags().StringVar(&toEnv, "to", "", "Target environment")
}

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage secrets and environment variables",
	Long: `Manage environment variables and secrets for OpenChoreo components.
Bypasses the OpenChoreo webhook password masking by patching ConfigMaps
directly on the data plane.`,
}

// ── secret set ──────────────────────────────────────────────────────────

var secretSetCmd = &cobra.Command{
	Use:   "set <component> <KEY=VALUE> [KEY=VALUE...]",
	Short: "Set environment variables for a component",
	Long: `Sets environment variables by patching the ConfigMap on the data plane.
This BYPASSES the OpenChoreo webhook that masks passwords in URLs.

Examples:
  agctl secret set postgrest PGRST_DB_URI="postgres://zerops:zerops123@postgres:5432/app" --env production
  agctl secret set postgrest PGRST_DB_ANON_ROLE=zerops PGRST_DB_SCHEMA=public --env all`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]
		kvPairs := args[1:]

		if envFlag == "" {
			return fmt.Errorf("--env flag is required")
		}

		// Parse KEY=VALUE pairs
		secrets := map[string]string{}
		for _, kv := range kvPairs {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid format %q, expected KEY=VALUE", kv)
			}
			secrets[parts[0]] = parts[1]
		}

		if envFlag == "all" {
			for name := range cfg.Environments {
				fmt.Printf("🔐 Setting secrets for %s in %s...\n", component, name)
				if err := setSecrets(component, name, secrets); err != nil {
					fmt.Printf("  ❌ %v\n", err)
				}
			}
			return nil
		}

		fmt.Printf("🔐 Setting secrets for %s in %s...\n", component, envFlag)
		return setSecrets(component, envFlag, secrets)
	},
}

func setSecrets(component, envName string, secrets map[string]string) error {
	_, err := cfg.ResolveEnv(envName)
	if err != nil {
		return err
	}
	namespace, err := resolveNamespace(envName)
	if err != nil {
		return fmt.Errorf("resolving namespace: %w", err)
	}
	dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

	// Find the ConfigMap for this component
	cmPrefix := fmt.Sprintf("%s-%s-env-configs", component, envName)
	cmName, err := dp.FindConfigMap(namespace, cmPrefix)
	if err != nil {
		// Try alternate naming: component-envName-env-configs-hash
		cmPrefix = component
		cmName, err = dp.FindConfigMap(namespace, cmPrefix)
		if err != nil {
			return fmt.Errorf("no ConfigMap found for %s in %s. Is the component deployed?", component, envName)
		}
	}

	fmt.Printf("  📋 ConfigMap: %s\n", cmName)

	// Patch each secret
	for key, value := range secrets {
		if _, err := dp.PatchConfigMap(namespace, cmName, key, value); err != nil {
			return fmt.Errorf("setting %s: %w", key, err)
		}
		maskedValue := maskValue(key, value)
		fmt.Printf("  ✅ %s = %s\n", key, maskedValue)
	}

	// Restart deployment to pick up new values
	deployName, err := dp.GetDeploymentName(namespace, "openchoreo.dev/component="+component)
	if err == nil {
		fmt.Printf("  🔄 Restarting %s...\n", deployName)
		dp.RolloutRestart(namespace, deployName)
	}

	fmt.Printf("  ✅ Secrets updated (bypassed webhook masking)\n")
	return nil
}

// ── secret get ──────────────────────────────────────────────────────────

var secretGetCmd = &cobra.Command{
	Use:   "get <component> <KEY>",
	Short: "Get an environment variable value",
	Long: `Reads the actual value from the data plane ConfigMap.
By default, values are masked. Use --reveal to see actual values.

Examples:
  agctl secret get postgrest PGRST_DB_URI --env production
  agctl secret get postgrest PGRST_DB_URI --env production --reveal`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]
		key := args[1]

		if envFlag == "" {
			return fmt.Errorf("--env flag is required")
		}

		env, err := cfg.ResolveEnv(envFlag)
		if err != nil {
			return err
		}
		_ = env
		namespace, err := resolveNamespace(envFlag)
		if err != nil {
			return fmt.Errorf("resolving namespace: %w", err)
		}
		dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

		cmName, err := findComponentConfigMap(dp, namespace, component)
		if err != nil {
			return err
		}

		value, err := dp.GetConfigMapValue(namespace, cmName, key)
		if err != nil {
			return fmt.Errorf("key %s not found: %w", key, err)
		}

		if revealFlag {
			fmt.Printf("%s = %s\n", key, value)
		} else {
			fmt.Printf("%s = %s\n", key, maskValue(key, value))
		}
		return nil
	},
}

// ── secret list ─────────────────────────────────────────────────────────

var secretListCmd = &cobra.Command{
	Use:   "list <component>",
	Short: "List all environment variables for a component",
	Long: `Shows all keys from the data plane ConfigMap. Values are masked by default.

Examples:
  agctl secret list postgrest --env production
  agctl secret list postgrest --env all`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]
		if envFlag == "" {
			return fmt.Errorf("--env flag is required")
		}

		if envFlag == "all" {
			for name := range cfg.Environments {
				fmt.Printf("═══ %s ═══\n", strings.ToUpper(name))
				if err := listSecrets(component, name); err != nil {
					fmt.Printf("  ❌ %v\n", err)
				}
				fmt.Println()
			}
			return nil
		}
		return listSecrets(component, envFlag)
	},
}

func listSecrets(component, envName string) error {
	_, err := cfg.ResolveEnv(envName)
	if err != nil {
		return err
	}
	namespace, err := resolveNamespace(envName)
	if err != nil {
		return fmt.Errorf("resolving namespace: %w", err)
	}
	dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

	cmName, err := findComponentConfigMap(dp, namespace, component)
	if err != nil {
		return err
	}

	out, err := dp.Run("get", "configmap", cmName, "-n", namespace,
		"-o", "jsonpath={range .data}{@.key}: {@.value}{\"\\n\"}{end}")
	if err != nil {
		// Fallback: get all data as JSON
		out, err = dp.Run("get", "configmap", cmName, "-n", namespace,
			"-o", "jsonpath={.data}")
		if err != nil {
			return err
		}
	}

	fmt.Printf("📋 ConfigMap: %s\n", cmName)
	if out != "" {
		// Parse the data
		for _, line := range strings.Split(out, "\n") {
			if line != "" {
				fmt.Printf("  %s\n", line)
			}
		}
	}
	return nil
}

// ── secret diff ─────────────────────────────────────────────────────────

var secretDiffCmd = &cobra.Command{
	Use:   "diff <component>",
	Short: "Compare secrets between two environments",
	Long: `Shows differences in environment variables between two environments.

Examples:
  agctl secret diff postgrest --from development --to production`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]
		dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

		fromNS, err := resolveNamespace(fromEnv)
		if err != nil {
			return fmt.Errorf("resolving from namespace: %w", err)
		}
		toNS, err := resolveNamespace(toEnv)
		if err != nil {
			return fmt.Errorf("resolving to namespace: %w", err)
		}

		// Get ConfigMaps from both envs
		fromCM, err := findComponentConfigMap(dp, fromNS, component)
		if err != nil {
			return fmt.Errorf("source: %w", err)
		}
		toCM, err := findComponentConfigMap(dp, toNS, component)
		if err != nil {
			return fmt.Errorf("target: %w", err)
		}

		fromData := getConfigMapData(dp, fromNS, fromCM)
		toData := getConfigMapData(dp, toNS, toCM)

		fmt.Printf("Secret diff for %s: %s → %s\n", component, fromEnv, toEnv)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		allKeys := map[string]bool{}
		for k := range fromData {
			allKeys[k] = true
		}
		for k := range toData {
			allKeys[k] = true
		}

		diffs := 0
		for key := range allKeys {
			fv, fok := fromData[key]
			tv, tok := toData[key]

			if fok && tok && fv == tv {
				fmt.Printf("  %-30s = %-20s (same)\n", key, maskValue(key, fv))
			} else if fok && tok {
				fmt.Printf("  %-30s ≠ %s → %s\n", key, maskValue(key, fv), maskValue(key, tv))
				diffs++
			} else if fok && !tok {
				fmt.Printf("  %-30s + only in %s\n", key, fromEnv)
				diffs++
			} else {
				fmt.Printf("  %-30s + only in %s\n", key, toEnv)
				diffs++
			}
		}

		if diffs == 0 {
			fmt.Println("  ✅ No differences found")
		} else {
			fmt.Printf("  ⚠️  %d difference(s) found\n", diffs)
		}

		return nil
	},
}

// ── secret sync ─────────────────────────────────────────────────────────

var secretSyncCmd = &cobra.Command{
	Use:   "sync <component>",
	Short: "Sync secrets from one environment to another",
	Long: `Copies all environment variables from one environment to another.

Examples:
  agctl secret sync postgrest --from development --to staging`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]

		if fromEnv == "" || toEnv == "" {
			return fmt.Errorf("--from and --to flags are required")
		}

		dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

		fromNS, err := resolveNamespace(fromEnv)
		if err != nil {
			return fmt.Errorf("resolving from namespace: %w", err)
		}

		fromCM, err := findComponentConfigMap(dp, fromNS, component)
		if err != nil {
			return fmt.Errorf("source: %w", err)
		}

		// Get all data from source
		data := getConfigMapData(dp, fromNS, fromCM)
		if len(data) == 0 {
			return fmt.Errorf("no secrets found in %s for %s", fromEnv, component)
		}

		fmt.Printf("🔄 Syncing %d secrets from %s → %s for %s\n", len(data), fromEnv, toEnv, component)

		return setSecrets(component, toEnv, data)
	},
}

// ── helpers ─────────────────────────────────────────────────────────────

func findComponentConfigMap(dp *kube.Client, namespace, component string) (string, error) {
	out, err := dp.Run("get", "configmap", "-n", namespace, "-o", "name")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimPrefix(line, "configmap/")
		if strings.Contains(name, component) && strings.Contains(name, "env-configs") {
			return name, nil
		}
	}
	return "", fmt.Errorf("no ConfigMap found for component %s in namespace %s", component, namespace)
}

func getConfigMapData(dp *kube.Client, namespace, cmName string) map[string]string {
	data := map[string]string{}
	out, err := dp.Run("get", "configmap", cmName, "-n", namespace,
		"-o", "jsonpath={range .data}{.key}={.value}|||{end}")
	if err != nil {
		return data
	}
	// Parse key=value pairs
	for _, pair := range strings.Split(out, "|||") {
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			data[parts[0]] = parts[1]
		}
	}
	return data
}

func maskValue(key, value string) string {
	// Mask sensitive values
	sensitive := []string{"PASSWORD", "SECRET", "KEY", "TOKEN", "URI", "URL", "PASS"}
	for _, s := range sensitive {
		if strings.Contains(strings.ToUpper(key), s) {
			if len(value) <= 4 {
				return "****"
			}
			return value[:4] + strings.Repeat("*", len(value)-4)
		}
	}
	return value
}
