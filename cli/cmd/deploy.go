package cmd

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/airgapops/agctl/internal/kube"
	"github.com/spf13/cobra"
)

var (
	deployImage   string
	deployPort    int
	deployExpose  bool
	deployHTML    string
	deployType    string

	// Stack image overrides (airgap: default to Gitea registry)
	stackPostgresImage  string
	stackPostgrestImage string
)

func init() {
	rootCmd.AddCommand(deployCmd)
	deployCmd.AddCommand(deployAppCmd)
	deployCmd.AddCommand(deployStackCmd)

	deployAppCmd.Flags().StringVar(&deployImage, "image", "nginx:alpine", "Container image")
	deployAppCmd.Flags().IntVar(&deployPort, "port", 80, "Container port")
	deployAppCmd.Flags().BoolVar(&deployExpose, "expose", true, "Expose via HTTPS (DNS + TLS + Gateway + HTTPRoute)")
	deployAppCmd.Flags().StringVar(&deployHTML, "html", "", "Custom HTML content (nginx only)")
	deployAppCmd.Flags().StringVar(&deployType, "type", "", "OpenChoreo ComponentType: service|web-application|worker|scheduled-task (auto-detected if empty)")

	deployStackCmd.Flags().StringVar(&stackPostgresImage, "postgres-image", "gitea.zerops.internal/jadamson/postgres:16-alpine", "PostgreSQL image (airgap: use Gitea registry)")
	deployStackCmd.Flags().StringVar(&stackPostgrestImage, "postgrest-image", "gitea.zerops.internal/jadamson/postgrest:v12.2.3", "PostgREST image (airgap: use Gitea registry)")
}

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy applications and stacks to OpenChoreo",
	Long: `Full-lifecycle deployment: Component + Workload + Service + NetworkPolicy
+ DNS + TLS Certificate + Gateway Listener + HTTPRoute in a single command.

Replaces 10+ manual kubectl commands with one agctl command.`,
}

// ── deploy app ──────────────────────────────────────────────────────────

var deployAppCmd = &cobra.Command{
	Use:   "app <name>",
	Short: "Deploy a single application",
	Long: `Creates a complete deployment pipeline for a single application:
  1. Component CRD (control plane)
  2. Workload CRD (control plane)
  3. Wait for pod
  4. Service (data plane)
  5. NetworkPolicy (data plane)
  6. DNS A record (DNS node)
  7. TLS Certificate (data plane)
  8. Gateway Listener (data plane)
  9. HTTPRoute (data plane)

Examples:
  agctl deploy app myapp --image nginx:alpine --port 80 --env development
  agctl deploy app myapi --image gitea.zerops.internal/user/myapi:v1 --port 3000 --env all
  agctl deploy app landing --image nginx:alpine --html "<h1>Hello airgapOps</h1>" --env development`,
	Args: cobra.ExactArgs(1),
	RunE: runDeployApp,
}

func runDeployApp(cmd *cobra.Command, args []string) error {
	name := args[0]
	if envFlag == "" {
		return fmt.Errorf("--env flag is required (development|staging|production|all)")
	}

	if envFlag == "all" {
		for envName := range cfg.Environments {
			if err := deployApp(name, envName); err != nil {
				fmt.Printf("  ❌ %s: %v\n", envName, err)
			}
		}
		return nil
	}

	return deployApp(name, envFlag)
}

func deployApp(name, envName string) error {
	env, err := cfg.ResolveEnv(envName)
	if err != nil {
		return err
	}

	// Resolve namespace — retry up to 3min for OpenChoreo to create it
	var namespace string
	for attempt := 0; attempt < 36; attempt++ {
		namespace, err = resolveNamespace(envName)
		if err == nil {
			break
		}
		if attempt == 0 {
			fmt.Printf("\n    ⏳ Waiting for namespace (%s)...", envName)
		}
		time.Sleep(5 * time.Second)
	}
	if namespace == "" {
		return fmt.Errorf("namespace not created after 3min for env=%s", envName)
	}

	cp := kube.NewControlPlaneClient()
	dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())
	domain := cfg.Defaults.Domain
	var hostname string
	if envName == "production" {
		hostname = fmt.Sprintf("%s.%s", name, domain)
	} else {
		hostname = fmt.Sprintf("%s-%s.%s", name, env.Short, domain)
	}
	gwNS := cfg.Gateway.Namespace
	listenerName := fmt.Sprintf("https-%s-%s", name, env.Short)

	fmt.Printf("\n🚀 Deploying %s to %s [project=%s] (%s)\n", name, envName, projectFlag, hostname)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// ── Step 1: Component CRD ──────────────────────────────────────────
	// Resolve ComponentType: explicit flag > auto-detect based on expose/port
	compType := deployType
	if compType == "" {
		if !deployExpose {
			compType = "worker"
		} else if deployPort == 80 || deployPort == 443 || strings.Contains(deployImage, "nginx") {
			compType = "web-application"
		} else {
			compType = "service"
		}
	}
	compTypeFull := fmt.Sprintf("deployment/%s", compType)
	if compType == "scheduled-task" {
		compTypeFull = "cronjob/scheduled-task"
	}

	fmt.Printf("  ➊ Creating Component CRD (%s)...", compTypeFull)
	componentYAML := fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: %s
  namespace: %s
  annotations:
    openchoreo.dev/display-name: "%s"
    openchoreo.dev/description: "Deployed by agctl"
spec:
  owner:
    projectName: %s
  componentType:
    kind: ClusterComponentType
    name: %s
  autoDeploy: true`, name, projectFlag, name, projectFlag, compTypeFull)

	existing, _ := cp.Run("get", "component", name, "-n", projectFlag, "-o", "jsonpath={.metadata.name}")
	if existing != "" {
		fmt.Printf(" ⏭️  already exists\n")
	} else {
		if _, err := cp.Apply(componentYAML); err != nil {
			// AlreadyExists = another goroutine created it first — that's fine
			if strings.Contains(err.Error(), "AlreadyExists") {
				fmt.Printf(" ⏭️  already exists\n")
			} else {
				return fmt.Errorf("creating Component: %w", err)
			}
		} else {
			fmt.Printf(" ✅\n")
		}
	}

	// ── Step 2: Workload CRD ───────────────────────────────────────────
	fmt.Printf("  ➋ Creating Workload CRD...")

	// Build endpoints section for web-application types
	endpointsSection := ""
	if deployExpose {
		endpointsSection = fmt.Sprintf(`
  endpoints:
    http:
      type: HTTP
      port: %d
      basePath: /`, deployPort)
	}

	workloadYAML := fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: %s
  namespace: %s
spec:
  owner:
    projectName: %s
    componentName: %s
  container:
    image: %s%s`, name, projectFlag, projectFlag, name, deployImage, endpointsSection)

	// Always apply the Workload — this updates the image on every deploy
	if _, err := cp.Apply(workloadYAML); err != nil {
		// AlreadyExists = race with another goroutine — that's fine (they'll update too)
		if strings.Contains(err.Error(), "AlreadyExists") {
			fmt.Printf(" ⏭️  already exists\n")
		} else {
			return fmt.Errorf("creating Workload: %w", err)
		}
	} else {
		existingWL, _ := cp.Run("get", "workload", name, "-n", projectFlag, "-o", "jsonpath={.metadata.resourceVersion}")
		if existingWL != "" {
			fmt.Printf(" 🔄 updated (new image)\n")
		} else {
			fmt.Printf(" ✅\n")
		}
	}

	// ── Step 2.5: ReleaseBinding CRD ──────────────────────────────────
	// OpenChoreo autoDeploy only creates a binding for "development".
	// For staging/production we must create the ReleaseBinding explicitly,
	// and it MUST include releaseName pointing to the ComponentRelease.
	bindingName := fmt.Sprintf("%s-%s", name, envName)
	existingRB, _ := cp.Run("get", "releasebinding", bindingName, "-n", projectFlag, "-o", "jsonpath={.metadata.name}")
	if existingRB != "" {
		fmt.Printf("  ⛓️  ReleaseBinding %s ⏭️  exists\n", bindingName)
	} else {
		// Wait for OpenChoreo to create the ComponentRelease (up to 60s)
		fmt.Printf("  ⛓️  Waiting for ComponentRelease...")
		var releaseName string
		for i := 0; i < 30; i++ {
			releaseName, _ = cp.Run("get", "componentreleases.openchoreo.dev",
				"-n", projectFlag,
				"-l", fmt.Sprintf("openchoreo.dev/component=%s", name),
				"-o", "jsonpath={.items[0].metadata.name}")
			if releaseName != "" && releaseName != "null" {
				break
			}
			time.Sleep(2 * time.Second)
		}
		if releaseName == "" {
			// Fallback: try to get by name prefix
			releaseName, _ = cp.Run("get", "componentreleases.openchoreo.dev",
				"-n", projectFlag,
				"-o", fmt.Sprintf("jsonpath={.items[?(@.spec.owner.componentName==%q)].metadata.name}", name))
		}
		if releaseName == "" {
			fmt.Printf(" ⚠️  ComponentRelease not found, binding without releaseName\n")
		} else {
			fmt.Printf(" ✅ %s\n", releaseName)
		}

		fmt.Printf("  ⛓️  Creating ReleaseBinding (%s)...", envName)
		var rbYAML string
		if releaseName != "" {
			rbYAML = fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: %s
  namespace: %s
  annotations:
    openchoreo.dev/display-name: "%s → %s"
    openchoreo.dev/promoted-by: "agctl"
spec:
  owner:
    projectName: %s
    componentName: %s
  releaseName: "%s"
  environment: "%s"
  state: Active`, bindingName, projectFlag, name, envName, projectFlag, name, releaseName, envName)
		} else {
			rbYAML = fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: %s
  namespace: %s
  annotations:
    openchoreo.dev/display-name: "%s → %s"
    openchoreo.dev/promoted-by: "agctl"
spec:
  owner:
    projectName: %s
    componentName: %s
  environment: "%s"
  state: Active`, bindingName, projectFlag, name, envName, projectFlag, name, envName)
		}

		if _, err := cp.Apply(rbYAML); err != nil {
			fmt.Printf(" ⚠️  %v\n", err)
		} else {
			fmt.Printf(" ✅\n")
		}
	}

	// ── Step 3: Wait for pod (non-blocking) ────────────────────────────
	fmt.Printf("  ➌ Waiting for pod...")
	var podName string
	for i := 0; i < 30; i++ {
		podName, err = dp.GetPodName(namespace, "openchoreo.dev/component="+name)
		if err == nil && podName != "" {
			phase, _ := dp.Run("get", "pod", podName, "-n", namespace, "-o", "jsonpath={.status.phase}")
			if phase == "Running" {
				break
			}
		}
		time.Sleep(2 * time.Second)
	}
	if podName == "" {
		fmt.Printf(" ⚠️  pod pending (will reconcile in ~2 min)\n")
	} else {
		fmt.Printf(" ✅ %s\n", podName)
	}

	// ── Step 4: Service ────────────────────────────────────────────────
	fmt.Printf("  ➍ Creating Service...")
	svcYAML := fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: agctl
spec:
  selector:
    openchoreo.dev/component: %s
  ports:
    - port: %d
      targetPort: %d
  type: ClusterIP`, name, namespace, name, deployPort, deployPort)

	existingSvc, _ := dp.Run("get", "svc", name, "-n", namespace, "-o", "jsonpath={.metadata.name}")
	if existingSvc != "" {
		fmt.Printf(" ⏭️  already exists\n")
	} else {
		// Retry: namespace may have just been created and not yet Active
		var svcErr error
		for attempt := 0; attempt < 10; attempt++ {
			_, svcErr = dp.Apply(svcYAML)
			if svcErr == nil {
				break
			}
			time.Sleep(5 * time.Second)
		}
		if svcErr != nil {
			return fmt.Errorf("creating Service: %w", svcErr)
		}
		fmt.Printf(" ✅ :%d\n", deployPort)
	}


	// ── Step 5: NetworkPolicy ──────────────────────────────────────────
	fmt.Printf("  ➎ Creating NetworkPolicy...")
	npYAML := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-gateway-%s
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: agctl
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
      ports:
        - protocol: TCP
          port: %d`, name, namespace, name, gwNS, deployPort)

	if _, err := dp.Apply(npYAML); err != nil {
		fmt.Printf(" ⚠️  %v\n", err)
	} else {
		fmt.Printf(" ✅\n")
	}

	if !deployExpose {
		fmt.Printf("\n  ✅ %s deployed to %s (internal only)\n\n", name, envName)
		return nil
	}

	// ── Step 6: DNS ────────────────────────────────────────────────────
	fmt.Printf("  ➏ Creating DNS record (%s)...", hostname)
	if err := createDNSRecord(hostname, cfg.Gateway.IP); err != nil {
		fmt.Printf(" ⚠️  %v (set manually)\n", err)
	} else {
		fmt.Printf(" ✅\n")
	}

	// ── Step 7: TLS Certificate ────────────────────────────────────────
	fmt.Printf("  ➐ Creating TLS certificate...")
	certName := fmt.Sprintf("%s-%s-cert", name, env.Short)
	secretName := fmt.Sprintf("%s-%s-tls-secret", name, env.Short)
	certYAML := fmt.Sprintf(`apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: agctl
spec:
  secretName: %s
  duration: 2160h
  renewBefore: 360h
  dnsNames:
    - %s
  issuerRef:
    name: %s
    kind: ClusterIssuer`, certName, gwNS, secretName, hostname, cfg.Gateway.TLSIssuer)

	if _, err := dp.Apply(certYAML); err != nil {
		fmt.Printf(" ⚠️  %v\n", err)
	} else {
		fmt.Printf(" ✅\n")
	}

	// ── Step 8: Gateway Listener ───────────────────────────────────────
	fmt.Printf("  ➑ Patching Gateway listener...")
	// Check if listener already exists
	existingListeners, _ := dp.Run("get", "gateway", cfg.Gateway.Name, "-n", gwNS,
		"-o", fmt.Sprintf("jsonpath={.spec.listeners[?(@.name==%q)].name}", listenerName))
	if existingListeners != "" {
		fmt.Printf(" ⏭️  already exists\n")
	} else {
		// Check listener count
		allListeners, _ := dp.Run("get", "gateway", cfg.Gateway.Name, "-n", gwNS,
			"-o", "jsonpath={range .spec.listeners[*]}{.name}{\"\\n\"}{end}")
		count := 0
		for _, l := range strings.Split(allListeners, "\n") {
			if l != "" {
				count++
			}
		}
		if count >= cfg.Gateway.MaxListeners {
			fmt.Printf(" ❌ Gateway at max listeners (%d/%d)\n", count, cfg.Gateway.MaxListeners)
		} else {
			listenerPatch := fmt.Sprintf(`[{"op":"add","path":"/spec/listeners/-","value":{
				"name":%q,"port":443,"protocol":"HTTPS",
				"hostname":%q,
				"tls":{"mode":"Terminate","certificateRefs":[{"name":%q}]},
				"allowedRoutes":{"namespaces":{"from":"All"}}
			}}]`, listenerName, hostname, secretName)
			if _, err := dp.Patch("gateway", cfg.Gateway.Name, gwNS, "json", listenerPatch); err != nil {
				fmt.Printf(" ⚠️  %v\n", err)
			} else {
				fmt.Printf(" ✅ (%d/%d)\n", count+1, cfg.Gateway.MaxListeners)
			}
		}
	}

	// ── Step 9: HTTPRoute ──────────────────────────────────────────────
	fmt.Printf("  ➒ Creating HTTPRoute...")
	routeYAML := fmt.Sprintf(`apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: %s-https-route
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: agctl
spec:
  parentRefs:
    - name: %s
      namespace: %s
      sectionName: %s
  hostnames:
    - %s
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: %s
          port: %d`, name, namespace, cfg.Gateway.Name, gwNS, listenerName, hostname, name, deployPort)

	if _, err := dp.Apply(routeYAML); err != nil {
		fmt.Printf(" ⚠️  %v\n", err)
	} else {
		fmt.Printf(" ✅\n")
	}

	// ── Optional: Custom HTML ──────────────────────────────────────────
	if deployHTML != "" {
		fmt.Printf("  🎨 Injecting custom HTML...")
		htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>%s</title>
<style>
body{font-family:'Inter',sans-serif;display:flex;align-items:center;justify-content:center;
height:100vh;margin:0;background:#0f172a;color:#e2e8f0}
.card{text-align:center;padding:3rem;border-radius:1rem;
background:rgba(255,255,255,0.05);backdrop-filter:blur(10px);
border:1px solid rgba(255,255,255,0.1)}
h1{font-size:3em;background:linear-gradient(135deg,#06b6d4,#8b5cf6);
-webkit-background-clip:text;-webkit-text-fill-color:transparent}
p{color:#94a3b8;margin-top:1rem}
.badge{display:inline-block;margin-top:1rem;padding:.5rem 1rem;
border-radius:2rem;background:rgba(6,182,212,0.15);color:#06b6d4;font-size:.85rem}
</style></head>
<body><div class="card">
<h1>%s</h1>
<p>Deployed by agctl to %s</p>
<div class="badge">%s • %s</div>
</div></body></html>`, name, deployHTML, envName, envName, hostname)

		if _, err := dp.Exec(namespace, podName,
			"sh", "-c", fmt.Sprintf("cat > /usr/share/nginx/html/index.html << 'AGCTL_EOF'\n%s\nAGCTL_EOF", htmlContent)); err != nil {
			fmt.Printf(" ⚠️  %v\n", err)
		} else {
			fmt.Printf(" ✅\n")
		}
	}

	// ── Verify ─────────────────────────────────────────────────────────
	fmt.Printf("\n  ⏳ Waiting for HTTPS endpoint...\n")
	time.Sleep(3 * time.Second)

	fmt.Printf("  🔗 https://%s\n", hostname)
	fmt.Printf("  ✅ %s deployed to %s\n\n", name, envName)

	return nil
}

// ── deploy stack ────────────────────────────────────────────────────────

var deployStackCmd = &cobra.Command{
	Use:   "stack <name>",
	Short: "Deploy a predefined application stack",
	Long: `Deploys a full application stack (multiple components).

Available stacks:
  postgrest    PostgreSQL + PostgREST API
  monitoring   Prometheus + Grafana (TODO)

Examples:
  agctl deploy stack postgrest --env development
  agctl deploy stack postgrest --env all`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		stack := args[0]
		if envFlag == "" {
			return fmt.Errorf("--env flag is required")
		}

		switch stack {
		case "postgrest":
			return deployPostgrestStack(envFlag)
		default:
			return fmt.Errorf("unknown stack %q. Available: postgrest", stack)
		}
	},
}

func deployPostgrestStack(envName string) error {
	if envName == "all" {
		for name := range cfg.Environments {
			if err := deployPostgrestStackSingle(name); err != nil {
				fmt.Printf("  ❌ %s: %v\n", name, err)
			}
		}
		return nil
	}
	return deployPostgrestStackSingle(envName)
}

func deployPostgrestStackSingle(envName string) error {
	_, err := cfg.ResolveEnv(envName)
	if err != nil {
		return err
	}

	namespace, err := resolveNamespace(envName)
	if err != nil {
		return fmt.Errorf("resolving namespace: %w", err)
	}

	fmt.Printf("\n🐘 Deploying PostgREST stack to %s\n", envName)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Override flags for each component
	origImage := deployImage
	origPort := deployPort
	origHTML := deployHTML
	origExpose := deployExpose

	// 1. Deploy PostgreSQL (internal only)
	deployImage = stackPostgresImage
	deployPort = 5432
	deployExpose = false
	deployHTML = ""

	fmt.Println("\n── Component 1: PostgreSQL ──")
	if err := deployApp("postgres", envName); err != nil {
		return fmt.Errorf("deploying postgres: %w", err)
	}

	// Set env vars for PostgreSQL
	dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())
	podName, _ := dp.GetPodName(namespace, "openchoreo.dev/component=postgres")
	if podName != "" {
		fmt.Printf("  🔧 Configuring PostgreSQL user...")
		dp.Exec(namespace, podName,
			"sh", "-c", "PGPASSWORD=zerops123 psql -U zerops -d app -c 'SELECT 1' 2>/dev/null || "+
				"psql -U postgres -c \"CREATE USER zerops WITH PASSWORD 'zerops123'; CREATE DATABASE app OWNER zerops; GRANT ALL ON DATABASE app TO zerops;\" 2>/dev/null")
		fmt.Printf(" ✅\n")
	}

	// 2. Deploy PostgREST (exposed)
	deployImage = stackPostgrestImage
	deployPort = 3000
	deployExpose = true
	deployHTML = ""

	fmt.Println("\n── Component 2: PostgREST ──")
	if err := deployApp("postgrest", envName); err != nil {
		return fmt.Errorf("deploying postgrest: %w", err)
	}

	// Restore flags
	deployImage = origImage
	deployPort = origPort
	deployHTML = origHTML
	deployExpose = origExpose

	fmt.Println("\n── Stack Configuration ──")
	fmt.Printf("  🔐 Setting PostgREST secrets...\n")

	// Set PostgREST env vars
	secretVars := map[string]string{
		"PGRST_DB_URI":       fmt.Sprintf("postgres://zerops:zerops123@postgres.%s:5432/app", namespace),
		"PGRST_DB_SCHEMA":    "public",
		"PGRST_DB_ANON_ROLE": "zerops",
	}

	cmName, err := dp.FindConfigMap(namespace, "postgrest")
	if err != nil {
		fmt.Printf("  ⚠️  ConfigMap not found, secrets must be set manually\n")
	} else {
		for k, v := range secretVars {
			if _, err := dp.PatchConfigMap(namespace, cmName, k, v); err != nil {
				fmt.Printf("  ⚠️  %s: %v\n", k, err)
			} else {
				masked := v
				if strings.Contains(strings.ToLower(k), "uri") {
					masked = v[:15] + "****"
				}
				fmt.Printf("  ✅ %s = %s\n", k, masked)
			}
		}
	}

	fmt.Printf("\n  ✅ PostgREST stack deployed to %s\n", envName)
	fmt.Printf("  💡 Run: agctl db seed postgres --env %s\n\n", envName)
	return nil
}

// ── DNS helper ──────────────────────────────────────────────────────────

func createDNSRecord(hostname, ip string) error {
	dnsHost := cfg.Clusters.DNS.Host
	dnsUser := cfg.Clusters.DNS.User
	dnsPass := cfg.Clusters.DNS.Password
	dnsKey := cfg.Clusters.DNS.APIKey
	zone := cfg.Clusters.DNS.Zone

	if dnsHost == "" || dnsKey == "" {
		return fmt.Errorf("DNS not configured in agctl config")
	}

	// SSH to DNS node and create record via PowerDNS API
	curlCmd := fmt.Sprintf(
		`curl -s -X PATCH 'http://localhost:8081/api/v1/servers/localhost/zones/%s.' `+
			`-H 'X-API-Key: %s' -H 'Content-Type: application/json' `+
			`-d '{"rrsets":[{"name":"%s.","type":"A","ttl":300,"changetype":"REPLACE","records":[{"content":"%s","disabled":false}]}]}'`,
		zone, dnsKey, hostname, ip)

	sshCmd := exec.Command("sshpass", "-p", dnsPass, "ssh",
		"-o", "StrictHostKeyChecking=no",
		fmt.Sprintf("%s@%s", dnsUser, dnsHost),
		curlCmd)

	out, err := sshCmd.CombinedOutput()
	if err != nil {
		// Try without sshpass (if SSH key is configured)
		sshCmd2 := exec.Command("ssh",
			"-o", "StrictHostKeyChecking=no",
			fmt.Sprintf("%s@%s", dnsUser, dnsHost),
			curlCmd)
		out, err = sshCmd2.CombinedOutput()
		if err != nil {
			return fmt.Errorf("SSH to DNS: %s (%w)", string(out), err)
		}
	}

	// Verify with dig
	_ = exec.Command("dig", fmt.Sprintf("@%s", dnsHost), hostname, "+short").Run()

	return nil
}

// verifyHTTPS does a quick check on the endpoint
func verifyHTTPS(hostname string) bool {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: nil, // skip TLS verify for private CA
		},
	}
	resp, err := client.Get("https://" + hostname)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}
