---
name: openchoreo-secrets
description: >-
  Manage secrets and environment variables for OpenChoreo Components.
  Use this skill when asked to: create secrets, set environment variables,
  configure database credentials, manage API keys, set passwords,
  configure per-environment secrets for development or production.
version: 1.0.0
metadata:
  author: platform-team
  tags: [openchoreo, secrets, environment-variables, security]
---

# 🔐 SKILL: Manage Secrets for OpenChoreo Components

## When to Use
- User asks to create/update secrets for a component
- User wants to set environment variables (DATABASE_URL, API_KEY, etc.)
- User needs different secrets per environment (dev vs prod)
- User asks about secret management or credentials

## Instructions

### Step 1: Create a Kubernetes Secret

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# Create secret for a component
kubectl create secret generic {component-name}-secrets \
  --from-literal=DATABASE_URL="postgresql://user:pass@host:5432/db" \
  --from-literal=API_KEY="sk-..." \
  --dry-run=client -o yaml | kubectl apply -f -
```

Or from a YAML file:

```yaml
# File: /tmp/{component-name}-secrets.yaml
apiVersion: v1
kind: Secret
metadata:
  name: {component-name}-secrets
  namespace: default
  labels:
    openchoreo.dev/component: {component-name}
type: Opaque
stringData:
  DATABASE_URL: "postgresql://user:pass@postgres.default.svc:5432/mydb"
  API_KEY: "your-api-key"
  REDIS_URL: "redis://redis.default.svc:6379"
```

### Step 2: Reference secrets in Workload

Update the Workload to mount the secret as environment variables:

```bash
kubectl patch workload {component-name} --type=merge -p '{
  "spec": {
    "container": {
      "env": [
        {"name": "DATABASE_URL", "valueFrom": {"secretKeyRef": {"name": "{component-name}-secrets", "key": "DATABASE_URL"}}},
        {"name": "API_KEY", "valueFrom": {"secretKeyRef": {"name": "{component-name}-secrets", "key": "API_KEY"}}}
      ]
    }
  }
}'
```

### Step 3: Per-Environment Secrets

Create separate secrets for each environment:

```bash
# Development secrets
kubectl create secret generic {component-name}-dev-secrets \
  --from-literal=DATABASE_URL="postgresql://dev:dev123@postgres-dev:5432/devdb" \
  --dry-run=client -o yaml | kubectl apply -f -

# Production secrets
kubectl create secret generic {component-name}-prod-secrets \
  --from-literal=DATABASE_URL="postgresql://prod:SecureP@ss!@postgres-prod:5432/proddb" \
  --dry-run=client -o yaml | kubectl apply -f -
```

### Step 4: Verify

```bash
# List secrets for a component
kubectl get secrets -l openchoreo.dev/component={component-name}

# View secret keys (NOT values)
kubectl get secret {component-name}-secrets -o jsonpath='{.data}' | python3 -c "
import json,sys
data=json.load(sys.stdin)
for k in data: print(f'  {k}: ****')
"

# Check if Workload has env vars
kubectl get workload {component-name} -o yaml | grep -A5 "env:"
```

## Common Secret Patterns

| Pattern | Secret Name | Keys |
|---------|------------|------|
| PostgreSQL | `postgres-secrets` | `POSTGRES_PASSWORD`, `POSTGRES_USER`, `POSTGRES_DB` |
| RabbitMQ | `rabbitmq-secrets` | `RABBITMQ_DEFAULT_USER`, `RABBITMQ_DEFAULT_PASS` |
| API Service | `{name}-secrets` | `API_KEY`, `JWT_SECRET`, `DATABASE_URL` |
| Redis | `redis-secrets` | `REDIS_PASSWORD`, `REDIS_URL` |

## Rules & Constraints
- NEVER echo/print secret VALUES in logs or output
- Use `stringData` (plain text) in YAML, NOT `data` (base64)
- Label secrets with `openchoreo.dev/component` for traceability
- Production secrets should use strong random passwords
- Use `kubectl create secret --dry-run=client -o yaml | kubectl apply -f -` for idempotency

## 🚨 CRITICAL: OpenChoreo Webhook Password Masking

> **OpenChoreo has a mutating webhook that MASKS passwords in URLs.**
> When you save a Workload with `PGRST_DB_URI=postgres://zerops:zerops123@postgres:5432/app`,
> the webhook replaces the password: `postgres://zerops:***@postgres:5432/app`
>
> **THIS IS NORMAL BEHAVIOR. DO NOT TRY TO FIX IT.**
>
> The REAL password still reaches the data plane ConfigMap correctly.
> The masking only affects what you see in `kubectl get workload -o yaml`.

### What to do if you see `***` in a Workload env var:
1. **DO NOTHING** — it is working correctly
2. **Verify on the data plane** — check the ConfigMap in the data plane namespace:
   ```bash
   kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get configmap -n {NAMESPACE} -o yaml | grep PGRST_DB_URI
   ```
3. The ConfigMap will have the REAL value

### What NEVER to do:
- **🚨 NEVER delete a Workload** to "fix" masked values
- **🚨 NEVER delete ConfigMaps** in data plane namespaces
- **🚨 NEVER patch Workloads** repeatedly to try to set the password
- **🚨 NEVER delete and recreate resources** to work around masking
- These actions break the entire deployment and all environments
