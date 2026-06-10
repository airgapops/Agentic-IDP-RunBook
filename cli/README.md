# agctl — airgapOps CLI

> Extends OpenChoreo with capabilities it doesn't natively support:
> persistent storage, environment-aware secrets, database operations,
> and unified platform monitoring.

## Why agctl?

OpenChoreo's Workload CRD only supports `args`, `command`, `env`, `files`, `image`.
No volumes. No PVCs. No secret management. No cross-environment tooling.

`agctl` fills these gaps with a single binary that understands your entire
3-tier airgap architecture.

```
┌──────────────────────────────────────────────┐
│               agctl (3.8MB binary)           │
│                                              │
│   storage    secret    db    status           │
│   attach     set       exec  (full platform) │
│   detach     get       seed                  │
│   status     list      tables                │
│   classes    diff                            │
│              sync                            │
│                                              │
│   ┌──────────────────────────────────────┐   │
│   │    Control Plane  │  Data Plane      │   │
│   │    (ThinkPad)     │  (Ryzen)         │   │
│   │    sudo kubectl   │  --kubeconfig    │   │
│   └──────────────────────────────────────┘   │
└──────────────────────────────────────────────┘
```

## Quick Start

```bash
# Check platform health
agctl status

# Attach persistent storage to PostgreSQL
agctl storage attach postgres --size 5Gi --env production

# Seed database with tables and data
agctl db seed postgres --env all

# Set secrets (bypasses OpenChoreo webhook masking)
agctl secret set postgrest PGRST_DB_URI="postgres://zerops:zerops123@postgres:5432/app" --env production

# Run SQL queries
agctl db exec postgres "SELECT * FROM todos" --env production
```

## Installation

### Build (on Mac, cross-compile for Linux)

```bash
cd cli/
export PATH=/usr/local/go/bin:$PATH
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/agctl-linux .
```

### Deploy to ThinkPad

```bash
scp bin/agctl-linux rooter@192.168.0.10:/tmp/agctl
ssh rooter@192.168.0.10 "sudo cp /tmp/agctl /usr/local/bin/agctl && sudo chmod +x /usr/local/bin/agctl"
scp configs/agctl.yaml rooter@192.168.0.10:/tmp/agctl.yaml
ssh rooter@192.168.0.10 "sudo mkdir -p /etc/agctl && sudo cp /tmp/agctl.yaml /etc/agctl/config.yaml"
```

Or with `make`:
```bash
make deploy
```

## Configuration

Config file: `/etc/agctl/config.yaml`

```yaml
clusters:
  control-plane:
    kubeconfig: /etc/rancher/k3s/k3s.yaml
  data-plane:
    kubeconfig: /etc/agctl/ryzen-kubeconfig.yaml

environments:
  development:
    namespace: dp-default-default-development-f8e58905
    storageClass: local-path
    short: dev
  staging:
    namespace: dp-default-default-staging-1362f732
    storageClass: local-path
    short: stg
  production:
    namespace: dp-default-default-production-86697425
    storageClass: local-path
    short: prod

gateway:
  name: gateway-default
  namespace: openchoreo-data-plane
  maxListeners: 7
```

## Commands Reference

### `agctl status`
Unified platform health view.

| Flag | Description |
|------|-------------|
| `--env` | Filter to single environment |

### `agctl storage attach <component>`
Creates PVC and patches deployment.

| Flag | Default | Description |
|------|---------|-------------|
| `--size` | `5Gi` | PVC size |
| `--env` | (required) | Target environment or `all` |
| `--mount` | auto-detect | Mount path override |

### `agctl storage detach <component>`
Removes volume and deletes PVC.

### `agctl storage status`
Lists all agctl-managed PVCs.

### `agctl storage classes`
Lists available StorageClasses.

### `agctl db exec <component> <sql>`
Runs SQL via kubectl exec.

| Flag | Description |
|------|-------------|
| `--env` | Target environment or `all` |

### `agctl db seed <component>`
Seeds database with tables and data.

| Flag | Description |
|------|-------------|
| `--env` | Target environment or `all` |
| `--file` | Custom SQL file (default: built-in) |

### `agctl db tables <component>`
Lists tables with column and row counts.

### `agctl secret set <component> <KEY=VALUE...>`
Sets environment variables on the data plane ConfigMap.

| Flag | Description |
|------|-------------|
| `--env` | Target environment or `all` |

### `agctl secret get <component> <KEY>`
Gets a specific variable value.

| Flag | Description |
|------|-------------|
| `--env` | Target environment |
| `--reveal` | Show unmasked value |

### `agctl secret list <component>`
Lists all variables for a component.

### `agctl secret diff <component>`
Compares variables between environments.

| Flag | Default | Description |
|------|---------|-------------|
| `--from` | `development` | Source environment |
| `--to` | `production` | Target environment |

### `agctl secret sync <component>`
Copies all variables from one environment to another.

| Flag | Description |
|------|-------------|
| `--from` | Source environment |
| `--to` | Target environment |

## Architecture

```
Mac (build machine)              ThinkPad (runtime)
┌─────────────────┐              ┌─────────────────────────────┐
│ Go source code  │   SCP 3.8MB  │ /usr/local/bin/agctl        │
│ go build ───────────────────►  │ /etc/agctl/config.yaml      │
│ GOOS=linux      │              │ /etc/agctl/ryzen-kubeconfig │
└─────────────────┘              └──────────┬──────────────────┘
                                            │
                                 ┌──────────┴──────────┐
                                 │                     │
                          sudo kubectl          kubectl --kubeconfig
                          (Control Plane)       (Data Plane / Ryzen)
```

## Key Design Decisions

1. **kubectl wrapper, not client-go** — simpler, faster to build, easier to debug.
   Can migrate to client-go later for MCP Server (Serie 2).

2. **Static binary** — no runtime dependencies. Go cross-compilation produces
   a self-contained executable. No Go needed on ThinkPad.

3. **Config file, not flags** — namespace mappings, kubeconfig paths, and gateway
   settings are infrastructure-specific. Config file avoids repetitive flags.

4. **Webhook bypass** — agctl patches ConfigMaps on the data plane directly,
   avoiding OpenChoreo's mutating webhook that masks passwords in URLs.

5. **Label-based tracking** — all resources created by agctl are labeled with
   `app.kubernetes.io/managed-by: agctl` for inventory and cleanup.

## Hermes Integration

agctl is designed to be invoked by Hermes Agent. Skills are at:
- `docs/hermes-skills/agctl-storage/SKILL.md`
- `docs/hermes-skills/agctl-secrets/SKILL.md`
- `docs/hermes-skills/agctl-platform/SKILL.md`

Example Hermes conversation:
```
User: "Dale almacenamiento persistente a postgres en producción"
Hermes: agctl storage attach postgres --size 5Gi --env production
        agctl db seed postgres --env production
        → ✅ postgres in production now has 5Gi persistent storage
```

## Roadmap

- **v0.1.0** (current) — storage, secrets, db, status
- **v0.2.0** — `agctl promote` (full environment promotion)
- **v0.3.0** — `agctl mcp serve` (MCP Server for Serie 2)
- **v0.4.0** — Multi-agent support
