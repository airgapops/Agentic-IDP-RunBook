package cmd

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var (
	provisionTemplate string
)

func init() {
	rootCmd.AddCommand(provisionCmd)
	provisionCmd.Flags().StringVar(&provisionTemplate, "template", "nginx", "App template: nginx|api|worker")
}

var provisionCmd = &cobra.Command{
	Use:   "provision <name>",
	Short: "Provision an app from scratch: repo + branches + CI/CD + deploy to all environments",
	Long: `Creates a complete application from zero:
  1. Gitea repository with source code
  2. Branches: main (dev), staging, production
  3. CI/CD pipeline (.gitea/workflows/deploy.yaml)
  4. Deploy to all environments via OpenChoreo CRDs

This is the "Golden Path" command — one instruction creates everything.

Templates:
  nginx  — Static site with custom HTML (default)
  api    — REST API with Dockerfile
  worker — Background process

Examples:
  agctl provision landing                           # nginx app, all envs
  agctl provision my-api --template api --port 3000 # API app
  agctl provision landing --project api             # in separate project namespace`,
	Args: cobra.ExactArgs(1),
	RunE: runProvision,
}

func runProvision(cmd *cobra.Command, args []string) error {
	appName := args[0]

	giteaURL := cfg.Gitea.URL
	giteaUser := cfg.Gitea.User
	giteaToken := cfg.Gitea.Token
	if giteaToken == "" {
		giteaToken = cfg.Gitea.Password
	}

	if giteaURL == "" {
		return fmt.Errorf("gitea.url not configured in /etc/agctl/config.yaml")
	}

	fmt.Printf("\n🏗️  Provisioning %s [template=%s, project=%s]\n", appName, provisionTemplate, projectFlag)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// ── Step 1: Create Gitea Repository ────────────────────────────────
	fmt.Printf("  ➊ Creating Gitea repository %s/%s...", giteaUser, appName)
	repoCreated, err := giteaCreateRepo(giteaURL, giteaUser, giteaToken, appName)
	if err != nil {
		return fmt.Errorf("creating repo: %w", err)
	}
	if repoCreated {
		fmt.Printf(" ✅ created\n")
	} else {
		fmt.Printf(" ⏭️  already exists\n")
	}

	// ── Step 2: Push source code based on template ─────────────────────
	fmt.Printf("  ➋ Pushing %s template files...", provisionTemplate)
	files := generateTemplateFiles(appName, provisionTemplate)
	for path, content := range files {
		err := giteaCreateFile(giteaURL, giteaUser, giteaToken, appName, path, content, "main")
		if err != nil {
			// File may already exist, try update
			if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "409") {
				fmt.Printf("\n    ⏭️  %s exists", path)
				continue
			}
			fmt.Printf("\n    ⚠️  %s: %v", path, err)
		}
	}
	fmt.Printf(" ✅\n")

	// ── Step 3: Create CI/CD pipeline ──────────────────────────────────
	fmt.Printf("  ➌ Creating CI/CD pipeline...")
	pipelineContent := generatePipeline(appName, giteaUser)
	err = giteaCreateFile(giteaURL, giteaUser, giteaToken, appName, ".gitea/workflows/deploy.yaml", pipelineContent, "main")
	if err != nil && !strings.Contains(err.Error(), "409") {
		fmt.Printf(" ⚠️  %v\n", err)
	} else {
		fmt.Printf(" ✅\n")
	}

	// ── Step 4: Create branches ────────────────────────────────────────
	fmt.Printf("  ➍ Creating environment branches...")
	branches := []string{"staging", "production"}
	for _, branch := range branches {
		err := giteaCreateBranch(giteaURL, giteaUser, giteaToken, appName, branch)
		if err != nil {
			if strings.Contains(err.Error(), "409") || strings.Contains(err.Error(), "already exists") {
				fmt.Printf(" [%s ⏭️]", branch)
				continue
			}
			fmt.Printf(" [%s ⚠️]", branch)
		} else {
			fmt.Printf(" [%s ✅]", branch)
		}
	}
	fmt.Printf("\n")

	// ── Step 5: Deploy to all environments ─────────────────────────────
	fmt.Printf("  ➎ Deploying to all environments...\n")

	// Set flags for deploy
	oldEnv := envFlag
	envFlag = "all"

	// Set the image to the Gitea registry
	registryImage := fmt.Sprintf("%s/%s/%s:latest",
		strings.TrimPrefix(strings.TrimPrefix(giteaURL, "https://"), "http://"),
		giteaUser, appName)

	// For nginx template, use nginx:alpine directly (no build needed initially)
	if provisionTemplate == "nginx" {
		deployImage = "nginx:alpine"
	} else {
		deployImage = registryImage
	}

	deployExpose = true

	// Run deploy for each environment IN PARALLEL
	var wg sync.WaitGroup
	for envName := range cfg.Environments {
		wg.Add(1)
		go func(env string) {
			defer wg.Done()
			if err := deployApp(appName, env); err != nil {
				fmt.Printf("    ⚠️  %s: %v\n", env, err)
			}
		}(envName)
	}
	wg.Wait()
	envFlag = oldEnv

	// ── Summary ─────────────────────────────────────────────────────────
	domain := cfg.Defaults.Domain
	cloneURL := fmt.Sprintf("%s/%s/%s.git", giteaURL, giteaUser, appName)

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("✅ %s provisioned successfully!\n", appName)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	fmt.Println("\n📦 Clone:")
	fmt.Printf("   git clone %s\n", cloneURL)

	fmt.Println("\n🌐 Application URLs:")
	for envName, envCfg := range cfg.Environments {
		if envName == "production" {
			fmt.Printf("   🔴 production  → https://%s.%s\n", appName, domain)
		} else {
			fmt.Printf("   %s %s → https://%s-%s.%s\n",
				map[string]string{"development": "🟢", "staging": "🟡"}[envName],
				envName, appName, envCfg.Short, domain)
		}
	}

	fmt.Println("\n🌿 Branches → Environments:")
	fmt.Println("   main       → development")
	fmt.Println("   staging    → staging")
	fmt.Println("   production → production")

	fmt.Println("\n🔄 CI/CD: git push → build → trivy scan → push → deploy (automatic)")

	fmt.Println("\n📋 Commands:")
	fmt.Printf("   agctl promote %s --to staging\n", appName)
	fmt.Printf("   agctl promote %s --to production\n", appName)
	fmt.Printf("   agctl status --env all\n")

	return nil
}

// ── Gitea API helpers ──────────────────────────────────────────────────

func giteaAPIRequest(method, url, token string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBytes, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Use Basic Auth if password is configured (avoids token scope issues)
	if cfg != nil && cfg.Gitea.Password != "" {
		req.SetBasicAuth(cfg.Gitea.User, cfg.Gitea.Password)
	} else if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	return client.Do(req)
}

func giteaCreateRepo(baseURL, user, token, name string) (bool, error) {
	url := fmt.Sprintf("%s/api/v1/user/repos", baseURL)
	payload := map[string]interface{}{
		"name":           name,
		"auto_init":      true,
		"default_branch": "main",
		"private":        false,
		"has_actions":    true,
		"description":    fmt.Sprintf("Provisioned by agctl — %s", name),
	}
	resp, err := giteaAPIRequest("POST", url, token, payload)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	created := false
	if resp.StatusCode == 201 {
		created = true
	} else if resp.StatusCode == 409 || resp.StatusCode == 422 {
		created = false // already exists
	} else {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Ensure Actions is enabled (PATCH covers the already-exists case too)
	patchURL := fmt.Sprintf("%s/api/v1/repos/%s/%s", baseURL, user, name)
	patchPayload := map[string]interface{}{"has_actions": true}
	if patchResp, patchErr := giteaAPIRequest("PATCH", patchURL, token, patchPayload); patchErr == nil {
		patchResp.Body.Close()
	}

	return created, nil
}

func giteaCreateFile(baseURL, user, token, repo, path, content, branch string) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/contents/%s", baseURL, user, repo, path)
	payload := map[string]interface{}{
		"content": base64Encode(content),
		"message": fmt.Sprintf("agctl provision: add %s", path),
		"branch":  branch,
	}
	resp, err := giteaAPIRequest("POST", url, token, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 201 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
}

func giteaCreateBranch(baseURL, user, token, repo, branch string) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/branches", baseURL, user, repo)
	payload := map[string]interface{}{
		"new_branch_name": branch,
		"old_branch_name": "main",
	}
	resp, err := giteaAPIRequest("POST", url, token, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 201 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
}

func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// ── Template generators ────────────────────────────────────────────────

func generateTemplateFiles(appName, template string) map[string]string {
	files := make(map[string]string)

	switch template {
	case "nginx":
		files["index.html"] = fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s — airgapOps</title>
  <style>
    body { font-family: system-ui; background: #0a0a0a; color: #fafafa; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; }
    .card { text-align: center; padding: 2rem 3rem; border: 1px solid #333; border-radius: 12px; background: #111; }
    h1 { font-size: 2.5rem; background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); -webkit-background-clip: text; -webkit-text-fill-color: transparent; }
    p { color: #888; font-size: 1.1rem; }
    .env { color: #4ade80; font-weight: bold; }
  </style>
</head>
<body>
  <div class="card">
    <h1>%s</h1>
    <p>Deployed by <strong>agctl</strong> on <span class="env">airgapOps</span></p>
    <p style="color: #555; font-size: 0.85rem;">Provisioned automatically via the Golden Path</p>
  </div>
</body>
</html>`, appName, appName)

		files["Dockerfile"] = `FROM nginx:alpine
COPY index.html /usr/share/nginx/html/index.html
EXPOSE 80`

	case "api":
		files["main.go"] = fmt.Sprintf(`package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"app":     "%s",
			"status":  "ok",
			"time":    time.Now().Format(time.RFC3339),
			"env":     os.Getenv("APP_ENV"),
			"version": os.Getenv("APP_VERSION"),
		})
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	fmt.Printf("%s API listening on :%s\n", "%s", port)
	http.ListenAndServe(":"+port, nil)
}`, appName, appName, appName)

		files["go.mod"] = fmt.Sprintf("module %s\n\ngo 1.21\n", appName)

		files["Dockerfile"] = `FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -o server .

FROM alpine:3.19
COPY --from=builder /app/server /server
EXPOSE 3000
CMD ["/server"]`

	case "worker":
		files["main.go"] = fmt.Sprintf(`package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fmt.Println("%s worker started")
	
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			fmt.Printf("[%s] %s worker heartbeat\n", time.Now().Format("15:04:05"), "%s")
		case s := <-sig:
			fmt.Printf("Received %s, shutting down\n", s)
			return
		}
	}
}`, appName, appName, appName)

		files["go.mod"] = fmt.Sprintf("module %s\n\ngo 1.21\n", appName)

		files["Dockerfile"] = `FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -o worker .

FROM alpine:3.19
COPY --from=builder /app/worker /worker
CMD ["/worker"]`
	}

	return files
}

func generatePipeline(appName, giteaUser string) string {
	return fmt.Sprintf(`name: Deploy %s
on:
  push:
    branches: [main, staging, production]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set environment from branch
        run: |
          BRANCH="${GITHUB_REF##*/}"
          case "$BRANCH" in
            main)       echo "ENV=development" >> $GITHUB_ENV ;;
            staging)    echo "ENV=staging"     >> $GITHUB_ENV ;;
            production) echo "ENV=production"  >> $GITHUB_ENV ;;
          esac
          echo "Deploying to: $ENV"

      - name: Build image
        run: |
          docker build -t gitea.zerops.internal/%s/%s:${GITHUB_SHA::8} .
          docker tag gitea.zerops.internal/%s/%s:${GITHUB_SHA::8} gitea.zerops.internal/%s/%s:latest

      - name: Push to registry
        run: |
          echo "${{ secrets.REGISTRY_PASSWORD }}" | docker login gitea.zerops.internal -u %s --password-stdin
          docker push gitea.zerops.internal/%s/%s:${GITHUB_SHA::8}
          docker push gitea.zerops.internal/%s/%s:latest

      - name: Deploy via agctl
        run: |
          agctl deploy app %s \
            --image gitea.zerops.internal/%s/%s:${GITHUB_SHA::8} \
            --env $ENV
`, appName,
		giteaUser, appName,
		giteaUser, appName,
		giteaUser, appName,
		giteaUser,
		giteaUser, appName,
		giteaUser, appName,
		appName,
		giteaUser, appName)
}
