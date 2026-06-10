---
name: agctl-secrets
description: >-
  Manage secrets and environment variables using agctl CLI.
  Use this skill when asked to: set secrets, configure credentials,
  manage environment variables, inject database URLs, sync secrets
  between environments, compare secrets, or fix webhook masking issues.
  This skill REPLACES direct kubectl secret management.
version: 1.0.0
metadata:
  author: platform-team
  tags: [agctl, secrets, environment-variables, configmap, webhook-bypass]
---

# 🔐 SKILL: Manage Secrets with agctl

## When to Use
- User asks to set/update secrets or environment variables
- User asks to inject database credentials
- User asks to compare secrets between environments
- User asks to sync/copy secrets from one environment to another
- User sees `***` in a Workload env var and asks about it
- User asks about webhook masking

## CRITICAL RULES
1. **ALWAYS use agctl** — never manually patch ConfigMaps or Secrets
2. **EXECUTE commands** — never show steps to the user
3. **agctl bypasses webhook masking** — it patches ConfigMaps directly on the data plane
4. **NEVER modify Workload CRD env vars** to fix masking — use agctl instead
5. **NEVER delete ConfigMaps** — agctl patches them in-place

## 🚨 OpenChoreo Webhook Masking (Background)

OpenChoreo has a mutating webhook that masks passwords in URLs.
When you set `PGRST_DB_URI=postgres://zerops:zerops123@postgres:5432/app`,
the webhook stores it as `postgres://zerops:***@postgres:5432/app`.

**agctl solves this** by patching the ConfigMap directly on the data plane,
bypassing the webhook entirely.

## Commands

### Set environment variables
```bash
# Single variable
agctl secret set postgrest PGRST_DB_URI="postgres://zerops:zerops123@postgres:5432/app" --env production

# Multiple variables at once
agctl secret set postgrest PGRST_DB_ANON_ROLE=zerops PGRST_DB_SCHEMA=public --env production

# All environments
agctl secret set postgrest PGRST_DB_URI="postgres://zerops:zerops123@postgres:5432/app" --env all
```

### Get a specific value
```bash
# Masked (safe for logs)
agctl secret get postgrest PGRST_DB_URI --env production

# Revealed (careful!)
agctl secret get postgrest PGRST_DB_URI --env production --reveal
```

### List all variables for a component
```bash
agctl secret list postgrest --env production
agctl secret list postgrest --env all
```

### Compare between environments
```bash
agctl secret diff postgrest --from development --to production
```

Output:
```
Secret diff for postgrest: development → production
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  PGRST_DB_URI        = post****                 (same)
  PGRST_DB_ANON_ROLE  = zerops                   (same)
  PGRST_DB_SCHEMA     = public                   (same)
  ✅ No differences found
```

### Sync secrets between environments
```bash
# Copy all secrets from dev to staging
agctl secret sync postgrest --from development --to staging
```

## Typical Workflows

### "Inyecta las credenciales de la DB en staging"
```bash
agctl secret sync postgrest --from development --to staging
```

### "El PostgREST no puede conectar a la DB en producción"
```bash
# Check current value
agctl secret get postgrest PGRST_DB_URI --env production --reveal

# Fix it
agctl secret set postgrest PGRST_DB_URI="postgres://zerops:zerops123@postgres:5432/app" --env production
```

### "Muéstrame las diferencias entre dev y prod"
```bash
agctl secret diff postgrest --from development --to production
```

## Rules & Constraints
- Values with PASSWORD, SECRET, KEY, TOKEN, URI, URL in the key name are auto-masked in output
- Use `--reveal` flag explicitly to see actual values
- agctl restarts the deployment automatically after setting secrets
- **NEVER echo/print secret values in plain text** unless user explicitly requests `--reveal`
