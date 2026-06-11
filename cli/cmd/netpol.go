package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/airgapops/agctl/internal/kube"
	"github.com/spf13/cobra"
)

var (
	netpolTarget        string
	netpolTargetProject string
	netpolPort          int
)

func init() {
	rootCmd.AddCommand(netpolCmd)
	netpolCmd.AddCommand(netpolTestCmd)
	netpolCmd.AddCommand(netpolListCmd)
	netpolCmd.AddCommand(netpolAllowCmd)

	netpolTestCmd.Flags().StringVar(&netpolTarget, "target", "", "Target component to test connectivity to")
	netpolTestCmd.Flags().StringVar(&netpolTargetProject, "target-project", "", "Project of the target (default: same as source)")
	netpolTestCmd.Flags().IntVar(&netpolPort, "port", 0, "Target port to test")

	netpolAllowCmd.Flags().StringVar(&netpolTarget, "target", "", "Target component to allow")
	netpolAllowCmd.Flags().StringVar(&netpolTargetProject, "target-project", "", "Project of the target")
	netpolAllowCmd.Flags().IntVar(&netpolPort, "port", 0, "Port to allow (required)")
}

var netpolCmd = &cobra.Command{
	Use:   "netpol",
	Short: "Manage and test NetworkPolicies",
	Long: `Test cross-namespace connectivity, list policies, and create allow rules.

Examples:
  agctl netpol test landing --target postgres --target-project api --env development
  agctl netpol list --env development
  agctl netpol allow postgrest --target postgres --port 5432 --env development`,
}

// ── netpol test ─────────────────────────────────────────────────────────

var netpolTestCmd = &cobra.Command{
	Use:   "test <source-component>",
	Short: "Test if one component can reach another",
	Long: `Tests network connectivity between two components, optionally across projects.
Launches a temporary pod to attempt a TCP connection.

Examples:
  agctl netpol test landing --target postgres --target-project api --env development
  agctl netpol test postgrest --target postgres --port 5432 --env development`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]
		if netpolTarget == "" {
			return fmt.Errorf("--target is required")
		}
		if envFlag == "" {
			return fmt.Errorf("--env is required")
		}

		// Resolve source namespace
		sourceNS, err := resolveNamespace(envFlag)
		if err != nil {
			return fmt.Errorf("resolving source namespace: %w", err)
		}

		// Resolve target namespace
		targetProject := netpolTargetProject
		if targetProject == "" {
			targetProject = projectFlag
		}

		var targetNS string
		if targetProject == projectFlag {
			targetNS = sourceNS
		} else {
			// Save and restore project flag
			origProject := projectFlag
			projectFlag = targetProject
			targetNS, err = resolveNamespace(envFlag)
			projectFlag = origProject
			if err != nil {
				return fmt.Errorf("resolving target namespace: %w", err)
			}
		}

		// Determine target port
		port := netpolPort
		if port == 0 {
			// Auto-detect from service
			dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())
			portStr, _ := dp.Run("get", "svc", netpolTarget, "-n", targetNS,
				"-o", "jsonpath={.spec.ports[0].port}")
			if portStr != "" {
				fmt.Sscanf(portStr, "%d", &port)
			}
			if port == 0 {
				port = 80
			}
		}

		targetSvc := fmt.Sprintf("%s.%s.svc.cluster.local", netpolTarget, targetNS)
		crossNS := sourceNS != targetNS

		fmt.Printf("\n🔒 NetworkPolicy Test\n")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("  Source:  %s/%s (project: %s)\n", sourceNS, source, projectFlag)
		fmt.Printf("  Target:  %s/%s:%d (project: %s)\n", targetNS, netpolTarget, port, targetProject)
		fmt.Printf("  Cross-namespace: %v\n\n", crossNS)

		// Run test: exec from source pod to target
		dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

		// Find source pod
		sourcePod, err := dp.GetPodName(sourceNS, "openchoreo.dev/component="+source)
		if err != nil {
			// Try with a temp pod
			fmt.Printf("  ⚠️  Source pod not found, using test pod...\n")
			return runNetpolTestPod(dp, sourceNS, source, targetSvc, port)
		}

		// Test connectivity with timeout
		fmt.Printf("  🔍 Testing TCP connection...")
		testCmd := fmt.Sprintf("timeout 5 sh -c 'cat < /dev/null > /dev/tcp/%s/%d' 2>&1 && echo CONNECTED || echo BLOCKED", targetSvc, port)
		result, err := dp.Exec(sourceNS, sourcePod, "sh", "-c", testCmd)

		// If /dev/tcp not supported, try wget/curl
		if err != nil || strings.Contains(result, "not found") || strings.Contains(result, "syntax") {
			testCmd = fmt.Sprintf("timeout 5 wget -q -O /dev/null http://%s:%d/ 2>&1; echo EXIT:$?", targetSvc, port)
			result, _ = dp.Exec(sourceNS, sourcePod, "sh", "-c", testCmd)
		}

		connected := strings.Contains(result, "CONNECTED") || strings.Contains(result, "EXIT:0")

		if connected {
			fmt.Printf(" ✅ ALLOWED\n")
			fmt.Printf("\n  Result: ✅ %s CAN reach %s:%d\n", source, netpolTarget, port)
			if crossNS {
				fmt.Printf("  ⚠️  Cross-namespace traffic is ALLOWED (review NetworkPolicies)\n")
			}
		} else {
			fmt.Printf(" ❌ BLOCKED\n")
			fmt.Printf("\n  Result: ❌ %s CANNOT reach %s:%d\n", source, netpolTarget, port)
			if crossNS {
				fmt.Printf("  ✅ Zero-Trust: cross-namespace traffic is denied by default\n")
			}
			fmt.Printf("  💡 To allow: agctl netpol allow %s --target %s --target-project %s --port %d --env %s\n",
				source, netpolTarget, targetProject, port, envFlag)
		}

		fmt.Println()
		return nil
	},
}

func runNetpolTestPod(dp *kube.Client, sourceNS, source, targetSvc string, port int) error {
	testPodName := fmt.Sprintf("agctl-test-%s-%d", source, time.Now().Unix()%10000)

	// Create a temp pod in the source namespace with the same labels
	testYAML := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
  labels:
    openchoreo.dev/component: %s
    app.kubernetes.io/managed-by: agctl-test
spec:
  restartPolicy: Never
  containers:
    - name: test
      image: busybox:latest
      command: ["sh", "-c", "sleep 30"]`, testPodName, sourceNS, source)

	fmt.Printf("  🔧 Creating test pod %s...", testPodName)
	if _, err := dp.Apply(testYAML); err != nil {
		return fmt.Errorf("creating test pod: %w", err)
	}

	// Wait for ready
	for i := 0; i < 15; i++ {
		phase, _ := dp.Run("get", "pod", testPodName, "-n", sourceNS, "-o", "jsonpath={.status.phase}")
		if phase == "Running" {
			break
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Printf(" ✅\n")

	// Test
	fmt.Printf("  🔍 Testing TCP connection...")
	testCmd := fmt.Sprintf("timeout 5 wget -q -O /dev/null http://%s:%d/ 2>&1; echo EXIT:$?", targetSvc, port)
	result, _ := dp.Exec(sourceNS, testPodName, "sh", "-c", testCmd)

	connected := strings.Contains(result, "EXIT:0")
	if connected {
		fmt.Printf(" ✅ ALLOWED\n")
	} else {
		fmt.Printf(" ❌ BLOCKED\n")
		fmt.Printf("  ✅ Zero-Trust enforced\n")
	}

	// Cleanup
	fmt.Printf("  🧹 Cleaning up test pod...")
	dp.Delete("pod", testPodName, sourceNS)
	fmt.Printf(" ✅\n\n")

	return nil
}

// ── netpol list ─────────────────────────────────────────────────────────

var netpolListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all NetworkPolicies",
	RunE: func(cmd *cobra.Command, args []string) error {
		if envFlag == "" {
			return fmt.Errorf("--env is required")
		}

		return forEachEnv(func(envName, namespace string) error {
			dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

			fmt.Printf("\n🔒 NetworkPolicies in %s (%s)\n", envName, namespace)
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			out, err := dp.Run("get", "networkpolicy", "-n", namespace,
				"-o", "custom-columns=NAME:.metadata.name,POD-SELECTOR:.spec.podSelector.matchLabels,POLICY-TYPES:.spec.policyTypes[*]")
			if err != nil {
				fmt.Printf("  (no policies)\n")
				return nil
			}
			fmt.Println(out)
			return nil
		})
	},
}

// ── netpol allow ────────────────────────────────────────────────────────

var netpolAllowCmd = &cobra.Command{
	Use:   "allow <source-component>",
	Short: "Create a NetworkPolicy to allow cross-namespace traffic",
	Long: `Creates a NetworkPolicy that allows traffic from a source component
in one namespace to a target component in another namespace.

Examples:
  agctl netpol allow postgrest --target postgres --port 5432 --env development
  agctl netpol allow frontend --target api --target-project backend --port 8080 --env all`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]
		if netpolTarget == "" || netpolPort == 0 {
			return fmt.Errorf("--target and --port are required")
		}

		targetProject := netpolTargetProject
		if targetProject == "" {
			targetProject = projectFlag
		}

		return forEachEnv(func(envName, sourceNS string) error {
			// Resolve target namespace
			var targetNS string
			var err error
			if targetProject == projectFlag {
				targetNS = sourceNS
			} else {
				origProject := projectFlag
				projectFlag = targetProject
				targetNS, err = resolveNamespace(envName)
				projectFlag = origProject
				if err != nil {
					return err
				}
			}

			dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())
			policyName := fmt.Sprintf("allow-%s-to-%s", source, netpolTarget)

			fmt.Printf("\n🔓 Creating allow rule: %s → %s:%d (%s)\n", source, netpolTarget, netpolPort, envName)

			npYAML := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: agctl
  annotations:
    agctl.airgapops.dev/source: "%s/%s"
    agctl.airgapops.dev/target: "%s/%s"
spec:
  podSelector:
    matchLabels:
      openchoreo.dev/component: %s
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: %s
          podSelector:
            matchLabels:
              openchoreo.dev/component: %s
      ports:
        - protocol: TCP
          port: %d`,
				policyName, targetNS,
				sourceNS, source,
				targetNS, netpolTarget,
				netpolTarget,
				sourceNS, source,
				netpolPort)

			if _, err := dp.Apply(npYAML); err != nil {
				return fmt.Errorf("creating NetworkPolicy: %w", err)
			}

			fmt.Printf("  ✅ %s created in %s\n", policyName, targetNS)
			return nil
		})
	},
}
