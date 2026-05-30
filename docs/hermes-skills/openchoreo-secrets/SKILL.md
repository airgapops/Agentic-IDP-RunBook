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
