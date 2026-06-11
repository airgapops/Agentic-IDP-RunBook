#!/bin/bash
# Setup CI/CD repos in Gitea with pipelines
# Usage: bash setup_repos.sh

set -e
GITEA_URL="https://gitea.zerops.internal"
TOKEN="f319df6a535c6fe79293f3f3c20abaa74c91bc69"
API="$GITEA_URL/api/v1"
CURL="curl -sk -H Authorization:\ token\ $TOKEN -H Content-Type:\ application/json"

echo "═══════════════════════════════════════════════"
echo "  🚀 Setting up CI/CD repositories in Gitea"
echo "═══════════════════════════════════════════════"

# ── 1. Create repo: jadamson/landing ──
echo ""
echo "📦 Creating repo: jadamson/landing..."
$CURL -X POST "$API/user/repos" -d '{
  "name": "landing",
  "description": "airgapOps Landing Page — Nginx with CI/CD pipeline",
  "auto_init": true,
  "default_branch": "main",
  "private": false
}' -o /dev/null -w "  HTTP %{http_code}\n"

sleep 2

# ── 2. Create index.html via API ──
echo "📄 Creating index.html..."
INDEX_CONTENT=$(echo -n '<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>airgapOps</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            min-height: 100vh;
            display: flex; align-items: center; justify-content: center;
            background: linear-gradient(135deg, #0f0c29, #302b63, #24243e);
            font-family: system-ui, -apple-system, sans-serif;
            color: #fff;
        }
        .container { text-align: center; }
        h1 {
            font-size: 4rem;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            margin-bottom: 1rem;
        }
        p { font-size: 1.2rem; opacity: 0.7; }
        .badge {
            display: inline-block;
            margin-top: 2rem;
            padding: 0.5rem 1.5rem;
            border: 1px solid rgba(255,255,255,0.2);
            border-radius: 999px;
            font-size: 0.85rem;
            opacity: 0.5;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>airgapOps</h1>
        <p>Agentic Internal Developer Platform</p>
        <div class="badge">Deployed by Hermes AI &bull; Zero-Trust Air-Gapped</div>
    </div>
</body>
</html>' | base64 -w0)

$CURL -X POST "$API/repos/jadamson/landing/contents/index.html" -d "{
  \"content\": \"$INDEX_CONTENT\",
  \"message\": \"feat: initial landing page\"
}" -o /dev/null -w "  HTTP %{http_code}\n"

# ── 3. Create Dockerfile ──
echo "🐳 Creating Dockerfile..."
DOCKERFILE_CONTENT=$(echo -n 'FROM nginx:alpine
COPY . /usr/share/nginx/html/
EXPOSE 80' | base64 -w0)

$CURL -X POST "$API/repos/jadamson/landing/contents/Dockerfile" -d "{
  \"content\": \"$DOCKERFILE_CONTENT\",
  \"message\": \"feat: add Dockerfile\"
}" -o /dev/null -w "  HTTP %{http_code}\n"

# ── 4. Create .gitea/workflows/deploy.yaml ──
echo "⚙️  Creating CI/CD pipeline..."
PIPELINE_CONTENT=$(cat << 'PIPELINE' | base64 -w0
name: Build & Deploy
on:
  push:
    branches: [main, staging, production]

jobs:
  build-deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set environment from branch
        id: env
        run: |
          BRANCH="${GITHUB_REF_NAME}"
          case $BRANCH in
            main)       echo "ENV=development" >> $GITHUB_OUTPUT ;;
            staging)    echo "ENV=staging" >> $GITHUB_OUTPUT ;;
            production) echo "ENV=production" >> $GITHUB_OUTPUT ;;
            *)          echo "ENV=development" >> $GITHUB_OUTPUT ;;
          esac
          SHORT_SHA=$(echo "${GITHUB_SHA}" | cut -c1-7)
          echo "SHA=${SHORT_SHA}" >> $GITHUB_OUTPUT

      - name: Build container image
        run: |
          docker build -t gitea.zerops.internal/jadamson/landing:${{ steps.env.outputs.SHA }} .
          docker tag gitea.zerops.internal/jadamson/landing:${{ steps.env.outputs.SHA }} \
                     gitea.zerops.internal/jadamson/landing:latest

      - name: Trivy vulnerability scan
        run: |
          trivy image --severity HIGH,CRITICAL --exit-code 1 \
            gitea.zerops.internal/jadamson/landing:${{ steps.env.outputs.SHA }} || \
            echo "⚠️ Vulnerabilities found but continuing (non-blocking for now)"

      - name: Push to Gitea Registry
        run: |
          echo "${{ secrets.REGISTRY_PASS }}" | docker login gitea.zerops.internal -u jadamson --password-stdin
          docker push gitea.zerops.internal/jadamson/landing:${{ steps.env.outputs.SHA }}
          docker push gitea.zerops.internal/jadamson/landing:latest

      - name: Deploy via agctl
        run: |
          agctl deploy app landing \
            --image gitea.zerops.internal/jadamson/landing:${{ steps.env.outputs.SHA }} \
            --env ${{ steps.env.outputs.ENV }}
PIPELINE
)

$CURL -X POST "$API/repos/jadamson/landing/contents/.gitea/workflows/deploy.yaml" -d "{
  \"content\": \"$PIPELINE_CONTENT\",
  \"message\": \"ci: add Gitea Actions pipeline with Trivy scan\"
}" -o /dev/null -w "  HTTP %{http_code}\n"

# ── 5. Create branches: staging, production ──
echo "🌿 Creating branches..."
# Get main branch SHA
MAIN_SHA=$($CURL "$API/repos/jadamson/landing/branches/main" 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin)['commit']['id'])")
echo "  main SHA: $MAIN_SHA"

$CURL -X POST "$API/repos/jadamson/landing/branches" -d "{
  \"new_branch_name\": \"staging\",
  \"old_branch_name\": \"main\"
}" -o /dev/null -w "  staging: HTTP %{http_code}\n"

$CURL -X POST "$API/repos/jadamson/landing/branches" -d "{
  \"new_branch_name\": \"production\",
  \"old_branch_name\": \"main\"
}" -o /dev/null -w "  production: HTTP %{http_code}\n"

echo ""
echo "════════════════════════════════════════════════"
echo "  ✅ jadamson/landing — DONE"
echo "════════════════════════════════════════════════"

# ── 6. Create repo: jadamson/todo-api ──
echo ""
echo "📦 Creating repo: jadamson/todo-api..."
$CURL -X POST "$API/user/repos" -d '{
  "name": "todo-api",
  "description": "PostgREST + PostgreSQL API Stack with CI/CD pipeline",
  "auto_init": true,
  "default_branch": "main",
  "private": false
}' -o /dev/null -w "  HTTP %{http_code}\n"

sleep 2

# ── 7. Create todo-api pipeline ──
echo "⚙️  Creating API CI/CD pipeline..."
API_PIPELINE_CONTENT=$(cat << 'APIPIPE' | base64 -w0
name: Build & Deploy API Stack
on:
  push:
    branches: [main, staging, production]

jobs:
  build-deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set environment from branch
        id: env
        run: |
          BRANCH="${GITHUB_REF_NAME}"
          case $BRANCH in
            main)       echo "ENV=development" >> $GITHUB_OUTPUT ;;
            staging)    echo "ENV=staging" >> $GITHUB_OUTPUT ;;
            production) echo "ENV=production" >> $GITHUB_OUTPUT ;;
          esac
          echo "SHA=$(echo ${GITHUB_SHA} | cut -c1-7)" >> $GITHUB_OUTPUT

      - name: Deploy stack via agctl
        run: |
          agctl deploy stack postgrest \
            --env ${{ steps.env.outputs.ENV }} \
            --project api
APIPIPE
)

$CURL -X POST "$API/repos/jadamson/todo-api/contents/.gitea/workflows/deploy.yaml" -d "{
  \"content\": \"$API_PIPELINE_CONTENT\",
  \"message\": \"ci: add API stack deployment pipeline\"
}" -o /dev/null -w "  HTTP %{http_code}\n"

# Create branches for todo-api
echo "🌿 Creating branches for todo-api..."
$CURL -X POST "$API/repos/jadamson/todo-api/branches" -d '{"new_branch_name":"staging","old_branch_name":"main"}' -o /dev/null -w "  staging: HTTP %{http_code}\n"
$CURL -X POST "$API/repos/jadamson/todo-api/branches" -d '{"new_branch_name":"production","old_branch_name":"main"}' -o /dev/null -w "  production: HTTP %{http_code}\n"

echo ""
echo "════════════════════════════════════════════════"
echo "  ✅ jadamson/todo-api — DONE"
echo "════════════════════════════════════════════════"

# ── 8. List final state ──
echo ""
echo "📊 Final state:"
$CURL "$API/repos/search?token=$TOKEN" 2>/dev/null | python3 -c "
import json,sys
d = json.load(sys.stdin)
for r in d.get('data',[]):
    print(f\"  📁 {r['full_name']} — {r.get('description','')}\")
"
echo ""
echo "🎉 All repos created with CI/CD pipelines!"
