---
name: agctl-deploy
description: >-
  Full-lifecycle deployment via agctl CLI.
  Use this skill when: a user asks to deploy a new application,
  provision an app from scratch, deploy a stack, create/manage projects,
  promote releases between environments, test NetworkPolicies,
  restart components, or remove a deployed application.
version: 3.0.0
metadata:
  author: platform-team
  tags: [deploy, agctl, openchoreo, automation, netpol, project, provision, promote]
---

# 🚀 SKILL: agctl — Full-Lifecycle Platform Operations

## When to Use
- User says "deploy an application" / "deploy nginx" / "deploy a web app"
- User says "provision an app" / "create everything from scratch" / "set up nginx in all environments"
- User says "promote to staging/production" / "move this to prod"
- User says "deploy postgres and postgrest" / "deploy the API stack"
- User says "create a new project" / "separate namespace"
- User says "test NetworkPolicy" / "can X reach Y?"
- User says "remove/delete/undeploy an application"
- User says "restart a component"

## CRITICAL RULES
1. **For new apps from scratch, use `agctl provision`** — creates repo + branches + CI/CD + deploy in ONE command
2. **For existing apps, use `agctl deploy`** — deploys to a specific environment
3. **To move releases between environments, use `agctl promote`** — follows the DeploymentPipeline
4. **For PostgREST stack, use `agctl deploy stack postgrest`**
5. **Use --project for namespace isolation** — `--project api` puts workloads in separate namespaces
6. **To remove, use `agctl undeploy`** — do not manually delete resources
7. **NEVER delete Workloads or Components manually** — use agctl undeploy

---

## Natural Language → agctl Mapping

When the user speaks in natural language, map their intent to the correct command:

| User says (natural language) | Command to execute |
|:--|:--|
| "Quiero una app nginx en todos los ambientes" | `agctl provision <name>` |
| "Crea una landing page" | `agctl provision <name>` |
| "Aprovisiona una app para dev, staging y producción" | `agctl provision <name>` |
| "Despliega esta imagen en development" | `agctl deploy app <name> --image <img> --env development` |
| "Mueve esto a staging" / "Promueve a staging" | `agctl promote <name> --to staging` |
| "Pasa a producción" | `agctl promote <name> --to production` |
| "Crea un proyecto para la API" | `agctl project create <name>` |
| "Elimina la app" / "Borra la app" | `agctl undeploy <name> --env all` |
| "¿Puede la landing alcanzar la API?" | `agctl netpol test <src> --target <dst> --env <env>` |
| "Reinicia postgres" | `agctl restart postgres --env <env>` |
| "¿Cómo está el cluster?" | `agctl status --env all` |
| "Escanea la imagen" | `trivy image --skip-db-update --severity HIGH,CRITICAL <image>` |

## Post-Action Response Template

**AFTER every agctl command**, always include the output AND suggest next available actions.
Use this format:

```
[command output here]

📋 ¿Qué te gustaría hacer ahora?
  • Promover a staging → "promueve <app> a staging"
  • Promover a producción → "promueve <app> a producción"
  • Ver el estado → "muéstrame el estado"
  • Escanear vulnerabilidades → "escanea la imagen"
  • Clonar el repo → "dame la URL para clonar"
  • Eliminar la app → "elimina <app>"
```

---

## Provision (create everything from scratch)

This is the **Golden Path** command. It creates everything from zero:
- Gitea repository with source code
- Branches: main → development, staging, production
- CI/CD pipeline (.gitea/workflows/deploy.yaml)
- Deploys to all environments via OpenChoreo CRDs

```bash
# Provision a static nginx site
agctl provision landing

# Provision a REST API
agctl provision my-api --template api --port 3000

# Provision in a separate project (isolated namespaces)
agctl provision my-api --template api --project api

# Provision a background worker
agctl provision processor --template worker --expose=false
```

### Templates

| Template | Description | Includes |
|----------|-------------|----------|
| `nginx` | Static site (default) | index.html, Dockerfile, pipeline |
| `api` | REST API in Go | main.go, go.mod, Dockerfile, pipeline |
| `worker` | Background process | main.go, go.mod, Dockerfile, pipeline |

---

## Promote (move releases between environments)

Follows the DeploymentPipeline: development → staging → production.
Reuses the SAME ComponentRelease (verified image).

```bash
# Promote from development to staging
agctl promote landing --to staging

# Promote from staging to production
agctl promote landing --to production

# Promote in a specific project
agctl promote my-api --to staging --project api
```

---

## Project Management

```bash
# Create a new project (creates isolated namespaces per environment)
agctl project create api

# List all projects
agctl project list

# Show project status with pods and services
agctl project status api
```

---

## Deploy a single application

```bash
# Deploy nginx to default project
agctl deploy app landing --image nginx:alpine --port 80 --env all

# Deploy to a specific project (separate namespace)
agctl deploy app myapi --image myapi:v1 --port 3000 --env all --project api

# Deploy with custom HTML
agctl deploy app landing --image nginx:alpine --html "airgapOps" --env development

# Deploy without HTTPS exposure (internal only)
agctl deploy app worker --image myworker:v1 --port 8080 --env production --expose=false
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--image` | `nginx:alpine` | Container image |
| `--port` | `80` | Container port |
| `--expose` | `true` | Create DNS + TLS + Gateway + HTTPRoute |
| `--html` | (none) | Custom HTML content (nginx only) |
| `--env` | (required) | Target environment |
| `--project` | `default` | OpenChoreo project (namespace isolation) |
| `--type` | (auto) | ComponentType: service\|web-application\|worker\|scheduled-task |

---

## Deploy a stack

```bash
# PostgreSQL + PostgREST in separate project
agctl deploy stack postgrest --env all --project api

# After stack deploy, seed the database:
agctl db seed postgres --env development --project api
```

---

## NetworkPolicy Management

```bash
# Test if one component can reach another
agctl netpol test landing --target postgres --target-project api --env development
# Result: ❌ BLOCKED (cross-namespace denied by default)

# Test same-namespace connectivity
agctl netpol test postgrest --target postgres --project api --env development
# Result: ✅ ALLOWED

# Create an allow rule
agctl netpol allow postgrest --target postgres --port 5432 --env development

# List all policies
agctl netpol list --env development --project api
```

---

## Restart / Undeploy

```bash
# Restart a component
agctl restart postgres --env production --project api

# Undeploy from one environment
agctl undeploy myapp --env development

# Undeploy from all environments
agctl undeploy myapp --env all
```

---

## URL Pattern

Apps are exposed at: `{name}-{env_short}.{domain}`

Examples:
- `landing-dev.zerops.internal` (development)
- `postgrest-prod.zerops.internal` (production)

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| "pod not ready after 60s" | Check image exists in Gitea registry |
| "Gateway at max listeners" | Free a listener with `agctl undeploy` |
| "no namespace found" | Create project first: `agctl project create <name>` |
| DNS not resolving | Verify SSH to DNS node works |
| 503 after deploy | Check NetworkPolicy: `agctl netpol list --env <env>` |
| "gitea.url not configured" | Add gitea section to /etc/agctl/config.yaml |

