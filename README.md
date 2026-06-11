# 🏗️ airgapOps — Sovereign IDP on Bare Metal

> Air-gapped Internal Developer Platform powered by **OpenChoreo** on bare-metal K3s clusters, with Gitea CI/CD, Hermes AI, and a custom `agctl` CLI.

## 🌐 Architecture

```
┌─────────────────────┐     ┌──────────────────────────┐     ┌──────────────────────────┐
│  🍓 Raspberry Pi    │     │  💻 ThinkPad             │     │  🖥️ Ryzen               │
│  192.168.0.9        │     │  192.168.0.10             │     │  192.168.0.13             │
│                     │     │                           │     │                           │
│  ├── PowerDNS       │────▶│  OpenChoreo Control Plane │────▶│  OpenChoreo Data Plane    │
│  │   zerops.internal│     │  ├── OpenChoreo API        │     │  ├── Workloads (pods)     │
│  └── Step-CA (PKI)  │     │  ├── Gitea (repos + CI)   │     │  ├── Gateway (Envoy)      │
│      ACME TLS       │     │  ├── Gitea Actions Runner  │     │  └── Cilium eBPF ⏳       │
│                     │     │  ├── Hermes AI (MCP)       │     │      (Video 2 roadmap)    │
│                     │     │  └── agctl CLI             │     │                           │
└─────────────────────┘     └──────────────────────────┘     └──────────────────────────┘
     Tier-0 (DNS/PKI)              Control Plane                     Data Plane
                                  Ubuntu 24.04 / k3s             Rocky Linux 10.1 / k3s
```

## 🔄 Developer Workflow (Golden Path)

```
Developer
    │
    ├── git clone https://gitea.zerops.internal/<org>/<app>
    ├── edita código
    └── git push main
              │
              ▼
        Gitea Actions (CI)
        ├── git clone repo @ SHA exacto
        ├── docker build --no-cache
        ├── docker push → gitea.zerops.internal/<org>/<app>:<sha>
        └── agctl deploy app <name> --image <image> --env development
                    │
                    ▼
           OpenChoreo (CD)
           ├── crea/actualiza Workload CRD
           ├── provisiona namespace en Ryzen
           ├── crea Deployment + Service
           └── Gateway expone vía HTTPS (TLS automático)
                    │
                    ▼
        https://<app>-dev.zerops.internal ✅
```

## 🛠️ Stack Real

| Capa | Herramienta | Notas |
|------|-------------|-------|
| **DNS** | PowerDNS | `*.zerops.internal` resuelve en la red local |
| **PKI / TLS** | Step-CA | ACME automático, todos los servicios con HTTPS |
| **Portal / CD** | OpenChoreo | Self-service, Workload CRDs, reconciliación automática |
| **Repos + CI** | Gitea + Gitea Actions | Runner en ThinkPad, workflow sin dependencias externas |
| **CLI de plataforma** | `agctl` | Despliega apps vía OpenChoreo desde la línea de comandos |
| **AI Ops** | Hermes (MCP) | Natural language platform operations |
| **Secrets** | OpenBao | ⏳ Pendiente de instalación |
| **Network Policy** | Cilium + Hubble | ⏳ Video 2 — PostgREST + PostgreSQL con eBPF |
| **Registry** | Gitea Container Registry | `gitea.zerops.internal/<org>/<app>:<sha>` |

## 📁 Repository Structure

```
airgapOps/
├── cli/                           # agctl — CLI de plataforma
│   ├── cmd/                       # Comandos: deploy, project, env...
│   └── scripts/                   # Scripts de setup y mantenimiento
├── openchoreo/                    # Helm values para OpenChoreo
│   ├── values-cp.yaml             # Control Plane
│   └── values-dp.yaml             # Data Plane
├── platform/                      # OpenChoreo CRD declarations
│   ├── project.yaml
│   ├── environments/
│   └── components/
└── docs/
    ├── playbooks/                 # PB-00 al PB-06
    └── hermes-skills/             # Skills de Hermes AI
```

> **Nota:** Los binarios compilados (`cli/agctl`, `cli/agctl_linux`) **no se incluyen en el repositorio** — deben generarse localmente. Ver sección [Build del CLI](#️-build-del-cli-agctl) abajo.

## 🔧 Build del CLI (`agctl`)

Los binarios de `agctl` se excluyen del repositorio para evitar subir archivos pesados (>10MB cada uno). Deben compilarse localmente antes de usar.

```bash
cd cli/

# Para macOS (arquitectura local)
go build -o agctl .

# Para Linux (cross-compile — deploy en ThinkPad/Ryzen)
GOOS=linux GOARCH=amd64 go build -o agctl_linux .

# Verificar
./agctl --help
```

> ⚠️ **Requisito:** Tener [Go 1.21+](https://go.dev/dl/) instalado. Los archivos `cli/agctl` y `cli/agctl_linux` están en `.gitignore` para mantener el repositorio liviano.

## 🔗 URLs del Ambiente

| Servicio | URL |
|----------|-----|
| Gitea | `https://gitea.zerops.internal` |
| OpenChoreo API | `https://api.choreo.zerops.internal` |
| Demo Landing (dev) | `https://demo-landing-dev.zerops.internal` |

## 🚀 Deploy Rápido (Golden Path)

```bash
# 1. Clonar app desde Gitea
GIT_SSL_NO_VERIFY=true git clone \
  https://<user>:<pass>@gitea.zerops.internal/<org>/<app>.git

# 2. Hacer un cambio y pushear
git add . && git commit -m "mi cambio" && git push origin main

# 3. Gitea Actions corre automáticamente:
#    build → push → agctl deploy → pod en Ryzen ✅
```

## ⚙️ Workflow CI/CD (.gitea/workflows/deploy.yaml)

```yaml
name: Deploy
on:
  push:
    branches: [main, staging, production]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Build, Push & Deploy
        run: |
          set -e
          BRANCH="${GITHUB_REF##*/}"
          case "$BRANCH" in
            main)       DEPLOY_ENV=development ;;
            staging)    DEPLOY_ENV=staging ;;
            production) DEPLOY_ENV=production ;;
          esac

          # Clonar repo (sin actions/checkout — no existe en Gitea airgap)
          GIT_SSL_NO_VERIFY=true git clone --depth=1 \
            "https://user:pass@gitea.zerops.internal/org/app.git" /tmp/build
          cd /tmp/build

          IMAGE="gitea.zerops.internal/org/app:${GITHUB_SHA::8}"
          docker build --no-cache -t "$IMAGE" .
          echo "pass" | docker login gitea.zerops.internal -u user --password-stdin
          docker push "$IMAGE"

          agctl deploy app <name> --image "$IMAGE" --env "$DEPLOY_ENV"
```

> **Notas críticas:**
> - No usar `uses: actions/checkout@v4` — Gitea lo busca en sí mismo y falla (404)
> - No usar `ENV` como variable — es reservada del shell; usar `DEPLOY_ENV`

## 🛡️ Seguridad

- Todo el tráfico usa **TLS automático** vía Step-CA ACME
- Red completamente **air-gapped**: cero dependencias a internet externo
- **OpenBao** (pendiente): gestión de secrets para apps
- **Cilium NetworkPolicy** (roadmap Video 2): control de tráfico a nivel eBPF entre pods

## 📚 Playbooks

- [PB-00: Corporate Network Identity](docs/playbooks/PB-00-corporate-network-identity.md)
- [PB-01: Management Cluster](docs/playbooks/PB-01-management-cluster.md)
- [PB-02: Compute Cluster](docs/playbooks/PB-02-compute-cluster.md)
- [PB-03: First Platform](docs/playbooks/PB-03-first-platform.md)
- [PB-04: Developer Workflow](docs/playbooks/PB-04-developer-workflow.md)
- [PB-05: Firewall & Hardening](docs/playbooks/PB-05-firewall-and-hardening.md)
- [PB-06: GPU LLM Stack](docs/playbooks/PB-06-gpu-llm-stack.md)

## 🗺️ Roadmap

- [ ] **OpenBao** — gestión de secrets (próximo)
- [ ] **Cilium + Hubble UI** — eBPF networking + network policy visual (Video 2)
- [ ] **PostgREST + PostgreSQL** — API sobre base de datos con Cilium NetworkPolicy
