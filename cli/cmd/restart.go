package cmd

import (
	"fmt"

	"github.com/airgapops/agctl/internal/kube"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(restartCmd)
}

var restartCmd = &cobra.Command{
	Use:   "restart <component>",
	Short: "Restart a component (rollout restart)",
	Long: `Triggers a rolling restart of a component's deployment.
Equivalent to: kubectl rollout restart deployment <name>

Examples:
  agctl restart postgres --env production
  agctl restart postgrest --env all --project api`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]

		return forEachEnv(func(envName, namespace string) error {
			dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

			fmt.Printf("\n🔄 Restarting %s in %s...\n", component, envName)

			// Find the deployment
			deployName, err := dp.GetDeploymentName(namespace, "openchoreo.dev/component="+component)
			if err != nil {
				return fmt.Errorf("deployment not found for %s in %s", component, envName)
			}

			// Rollout restart
			if _, err := dp.RolloutRestart(namespace, deployName); err != nil {
				return fmt.Errorf("restart failed: %w", err)
			}

			fmt.Printf("  ⏳ Waiting for rollout...")
			if _, err := dp.RolloutStatus(namespace, deployName); err != nil {
				fmt.Printf(" ⚠️  timeout (pod may still be starting)\n")
			} else {
				fmt.Printf(" ✅\n")
			}

			fmt.Printf("  ✅ %s restarted in %s\n", component, envName)
			return nil
		})
	},
}
