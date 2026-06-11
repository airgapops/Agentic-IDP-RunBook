package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/airgapops/agctl/internal/kube"
	"github.com/spf13/cobra"
)

var promoteToEnv string

func init() {
	rootCmd.AddCommand(promoteCmd)
	promoteCmd.Flags().StringVar(&promoteToEnv, "to", "", "Target environment (staging|production)")
	promoteCmd.MarkFlagRequired("to")
}

var promoteCmd = &cobra.Command{
	Use:   "promote <component>",
	Short: "Promote a release to the next environment",
	Long: `Promotes a ComponentRelease from one environment to the next,
following the DeploymentPipeline promotion path defined in OpenChoreo.

The DeploymentPipeline defines: development → staging → production.
Promote reuses the SAME ComponentRelease (same verified image),
instead of doing a fresh deploy.

Examples:
  agctl promote landing --to staging         # Promote from dev to staging
  agctl promote landing --to production      # Promote from staging to prod
  agctl promote postgres --to staging -p api # Promote in 'api' project`,
	Args: cobra.ExactArgs(1),
	RunE: runPromote,
}

func runPromote(cmd *cobra.Command, args []string) error {
	componentName := args[0]
	targetEnv := promoteToEnv

	if targetEnv == "" {
		return fmt.Errorf("--to flag is required (staging|production)")
	}

	// Determine source environment from the DeploymentPipeline
	sourceEnv, err := resolveSourceEnv(targetEnv)
	if err != nil {
		return err
	}

	cp := kube.NewControlPlaneClient()

	fmt.Printf("\n🔄 Promoting %s: %s → %s [project=%s]\n", componentName, sourceEnv, targetEnv, projectFlag)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// ── Step 1: Find current ReleaseBinding in source env ──────────────
	fmt.Printf("  ➊ Finding release in %s...", sourceEnv)
	bindingName := fmt.Sprintf("%s-%s", componentName, sourceEnv)
	releaseRef, err := cp.Run("get", "releasebinding", bindingName, "-n", projectFlag,
		"-o", "jsonpath={.spec.releaseName}")
	if err != nil || releaseRef == "" {
		return fmt.Errorf("no ReleaseBinding found for %s in %s. Deploy first: agctl deploy app %s --env %s",
			componentName, sourceEnv, componentName, sourceEnv)
	}
	fmt.Printf(" ✅ %s\n", releaseRef)

	// ── Step 2: Get the image from the ComponentRelease ────────────────
	fmt.Printf("  ➋ Reading release details...")
	image, err := cp.Run("get", "componentrelease", releaseRef, "-n", projectFlag,
		"-o", "jsonpath={.status.image}")
	if err != nil || image == "" {
		// Fallback: get image from workload
		image, _ = cp.Run("get", "workload", componentName, "-n", projectFlag,
			"-o", "jsonpath={.spec.container.image}")
	}
	if image == "" {
		image = "(same as source)"
	}
	fmt.Printf(" ✅ image=%s\n", image)

	// ── Step 3: Check if target ReleaseBinding exists ──────────────────
	fmt.Printf("  ➌ Checking %s environment...", targetEnv)
	targetBinding := fmt.Sprintf("%s-%s", componentName, targetEnv)
	existingRef, _ := cp.Run("get", "releasebinding", targetBinding, "-n", projectFlag,
		"-o", "jsonpath={.spec.releaseName}")

	if existingRef == releaseRef {
		fmt.Printf(" ⏭️  already at %s\n", releaseRef)
		fmt.Printf("\n✅ %s is already promoted to %s\n", componentName, targetEnv)
		return nil
	}
	if existingRef != "" {
		fmt.Printf(" 🔄 upgrading from %s\n", existingRef)
	} else {
		fmt.Printf(" 🆕 new binding\n")
	}

	// ── Step 4: Create/Update ReleaseBinding in target env ─────────────
	fmt.Printf("  ➍ Binding release to %s...", targetEnv)
	bindingYAML := fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: %s
  namespace: %s
  annotations:
    openchoreo.dev/display-name: "%s → %s"
    openchoreo.dev/promoted-from: "%s"
    openchoreo.dev/promoted-by: "agctl"
spec:
  owner:
    projectName: %s
    componentName: %s
  releaseName: "%s"
  environment: "%s"
  state: Active`, targetBinding, projectFlag, componentName, targetEnv, sourceEnv, projectFlag, componentName, releaseRef, targetEnv)

	if _, err := cp.Apply(bindingYAML); err != nil {
		return fmt.Errorf("creating ReleaseBinding: %w", err)
	}
	fmt.Printf(" ✅\n")

	// ── Step 5: Verify ────────────────────────────────────────────────
	fmt.Printf("  ➎ Verifying promotion...")
	verifyRef, _ := cp.Run("get", "releasebinding", targetBinding, "-n", projectFlag,
		"-o", "jsonpath={.spec.releaseName}")
	if verifyRef == releaseRef {
		fmt.Printf(" ✅\n")
	} else {
		fmt.Printf(" ⚠️  binding exists but ref mismatch\n")
	}

	// ── Summary ───────────────────────────────────────────────────────
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("✅ Promoted %s to %s\n", componentName, targetEnv)
	fmt.Printf("   Release: %s\n", releaseRef)
	fmt.Printf("   Path:    %s → %s\n", sourceEnv, targetEnv)
	if image != "" {
		fmt.Printf("   Image:   %s\n", image)
	}

	// Show next promotion if available
	nextEnv := getNextPromotion(targetEnv)
	if nextEnv != "" {
		fmt.Printf("\n💡 Next: agctl promote %s --to %s\n", componentName, nextEnv)
	}

	return nil
}

// resolveSourceEnv determines where to promote FROM based on the target
// following the DeploymentPipeline: development → staging → production
func resolveSourceEnv(targetEnv string) (string, error) {
	// Try reading from the DeploymentPipeline CRD
	cp := kube.NewControlPlaneClient()
	pipelineJSON, err := cp.Run("get", "deploymentpipeline", "default", "-n", projectFlag,
		"-o", "jsonpath={.spec.promotionPaths}")
	if err == nil && pipelineJSON != "" {
		source := parseSourceFromPipeline(pipelineJSON, targetEnv)
		if source != "" {
			return source, nil
		}
	}

	// Fallback: hardcoded promotion path
	switch targetEnv {
	case "staging":
		return "development", nil
	case "production":
		return "staging", nil
	default:
		return "", fmt.Errorf("cannot promote to %q. Valid targets: staging, production", targetEnv)
	}
}

// parseSourceFromPipeline extracts source env from DeploymentPipeline JSON
func parseSourceFromPipeline(jsonStr string, targetEnv string) string {
	// The JSON is an array of promotion paths
	var paths []struct {
		SourceEnvironmentRef struct {
			Name string `json:"name"`
		} `json:"sourceEnvironmentRef"`
		TargetEnvironmentRefs []struct {
			Name string `json:"name"`
		} `json:"targetEnvironmentRefs"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &paths); err != nil {
		return ""
	}

	for _, p := range paths {
		for _, t := range p.TargetEnvironmentRefs {
			if strings.EqualFold(t.Name, targetEnv) {
				return p.SourceEnvironmentRef.Name
			}
		}
	}
	return ""
}

// getNextPromotion returns the next environment after the given one
func getNextPromotion(currentEnv string) string {
	switch currentEnv {
	case "development":
		return "staging"
	case "staging":
		return "production"
	default:
		return ""
	}
}
