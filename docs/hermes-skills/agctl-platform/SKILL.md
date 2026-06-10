---
name: agctl-platform
description: >-
  Platform health monitoring and database operations using agctl CLI.
  Use this skill when asked to: check status, view platform health,
  run SQL queries, seed databases, list tables, or check overall
  infrastructure status.
version: 1.0.0
metadata:
  author: platform-team
  tags: [agctl, status, database, health, monitoring]
---

# 📊 SKILL: Platform Status & Database Operations with agctl

## When to Use
- User asks to check platform health or status
- User asks to run SQL queries
- User asks to seed/populate databases
- User asks to list tables or check data
- User asks "is everything running?"

## CRITICAL RULES
1. **ALWAYS use agctl** — it provides unified output across all tiers
2. **EXECUTE commands** — never show steps to the user
3. **agctl runs on ThinkPad** at `/usr/local/bin/agctl`

## Platform Status

### Full platform overview
```bash
agctl status
```

Shows:
- Control Plane: pods, components
- Per-environment: pods, PVCs, services
- Gateway: listener count (warns at 6/7 and 7/7)
- Certificates: status and expiry

### Single environment
```bash
agctl status --env production
```

## Database Operations

### Execute SQL
```bash
# Single environment
agctl db exec postgres "SELECT * FROM todos" --env production

# All environments
agctl db exec postgres "SELECT count(*) FROM todos" --env all
```

### Seed database (create tables + sample data)
```bash
# Single environment
agctl db seed postgres --env production

# All environments at once
agctl db seed postgres --env all

# Custom SQL file
agctl db seed postgres --env production --file /path/to/custom.sql
```

The default seed creates:
- `categories` table (id, name, color) with 3 rows
- `todos` table (id, title, description, completed, priority, category_id, created_at) with 5 rows
- Grants permissions to `zerops` user
- Automatically restarts PostgREST to reload schema

### List tables
```bash
agctl db tables postgres --env production
agctl db tables postgres --env all
```

## Typical Workflows

### "¿Está todo corriendo?"
```bash
agctl status
```

### "La API no devuelve datos"
```bash
# Check if tables exist
agctl db tables postgres --env production

# If empty, seed
agctl db seed postgres --env production

# Verify
agctl db exec postgres "SELECT count(*) FROM todos" --env production
```

### "Necesito ver los datos de staging"
```bash
agctl db exec postgres "SELECT * FROM todos ORDER BY id" --env staging
```

### "Recrea las tablas en todos los ambientes"
```bash
agctl db seed postgres --env all
```

## Rules & Constraints
- `agctl db exec` uses `psql` inside the pod — limited to PostgreSQL
- Default credentials: user=`zerops`, password=`zerops123`, db=`app`
- `agctl db seed` automatically restarts PostgREST after seeding
- Always use `--env` flag — no default environment
