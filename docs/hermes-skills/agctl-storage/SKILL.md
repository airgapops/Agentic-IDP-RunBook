---
name: agctl-storage
description: >-
  Attach persistent storage (PVC/PV) to OpenChoreo components using agctl CLI.
  Use this skill when asked to: add storage, attach volume, persist data,
  create PVC, add persistence, fix data loss after restarts,
  or when a database loses data on pod restart.
version: 1.0.0
metadata:
  author: platform-team
  tags: [agctl, storage, pvc, persistence, volumes]
---

# 💾 SKILL: Manage Persistent Storage with agctl

## When to Use
- User reports data lost after pod restart (PostgreSQL, Redis, etc.)
- User asks to add persistent storage / volumes
- User asks to make a database survive restarts
- User asks about PVC or PersistentVolumeClaim

## CRITICAL RULES
1. **ALWAYS use agctl** — never manually create PVCs or patch deployments
2. **EXECUTE commands** — never show steps to the user
3. **agctl runs on ThinkPad** — it's at `/usr/local/bin/agctl`
4. **Config is at** `/etc/agctl/config.yaml`
5. **After attaching storage, ALWAYS seed the database** with `agctl db seed`

## Commands

### Attach storage to a component
```bash
# Single environment
agctl storage attach postgres --size 5Gi --env production

# All environments at once
agctl storage attach postgres --size 5Gi --env all

# Custom mount path
agctl storage attach myapp --size 2Gi --env development --mount /app/data
```

### Check storage status
```bash
# All environments
agctl storage status

# Single environment
agctl storage status --env production
```

### List available StorageClasses
```bash
agctl storage classes
```

### Detach storage (destructive — asks confirmation)
```bash
agctl storage detach postgres --env production
```

## Auto-Detected Mount Paths

agctl automatically detects the correct mount path for known components:

| Component | Mount Path |
|-----------|-----------|
| postgres | `/var/lib/postgresql/data` |
| redis | `/data` |
| rabbitmq | `/var/lib/rabbitmq` |
| mysql | `/var/lib/mysql` |
| mongodb | `/data/db` |
| minio | `/data` |
| grafana | `/var/lib/grafana` |
| prometheus | `/prometheus` |
| (unknown) | `/data` |

## Typical Workflow

When user says "postgres keeps losing data" or "add persistence to postgres":

```bash
# 1. Attach storage
agctl storage attach postgres --size 5Gi --env production

# 2. Seed database (tables were lost)
agctl db seed postgres --env production

# 3. Verify
agctl db exec postgres "SELECT count(*) FROM todos" --env production

# 4. Show status
agctl storage status --env production
```

## Rules & Constraints
- PVCs are labeled with `app.kubernetes.io/managed-by: agctl` for tracking
- StorageClass is per-environment (configured in /etc/agctl/config.yaml)
- **NEVER manually patch deployments** — always use agctl
- **NEVER delete PVCs** without `agctl storage detach`
- OpenChoreo's reconciler may revert deployment patches — if this happens, re-run `agctl storage attach`
