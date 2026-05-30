# 🏗️ Zerops Sovereign IDP — airgapOps

> Air-gapped Internal Developer Platform powered by **OpenChoreo** on bare-metal K3s clusters with Flux CD GitOps, Hermes AI, and vcluster isolation.

## 🌐 Architecture

```
┌─────────────────────┐     ┌──────────────────────────┐     ┌──────────────────────────┐
│  🍓 Raspberry Pi    │     │  💻 ThinkPad (Mgmt)      │     │  🖥️ Ryzen (Compute)      │
│  192.168.0.9        │     │  192.168.0.10             │     │  192.168.0.13             │
│                     │     │                           │     │                           │
│  ├── PowerDNS       │────▶│  OpenChoreo Control Plane │────▶│  OpenChoreo Data Plane    │
│  │   zerops.internal│     │  ├── Backstage (Portal)   │     │  ├── Cluster Agent        │
│  └── Step-CA (PKI)  │     │  ├── ThunderID (OIDC)     │     │  ├── Cilium (networking)  │
│      ACME TLS       │     │  ├── Flux CD (GitOps)     │     │  ├── Gateway (Envoy)      │
│                     │     │  ├── OpenChoreo API        │     │  └── Workloads            │
│                     │     │  ├── Argo Workflows (CI)   │     │      ├── mock-api         │
│                     │     │  ├── MCP Servers (AI)      │     │      ├── postgres         │
│                     │     │  └── Gateway API (Envoy)   │     │      └── rabbitmq         │
└─────────────────────┘     └──────────────────────────┘     └──────────────────────────┘
     Tier-0 (DNS/PKI)           Control Plane                     Data Plane
```

## 🔄 Platform Workflow

```
Developer ──▶ OpenChoreo Portal ──▶ Create Component ──▶ Flux CD (sync)
     │              │                                        │
     │              ▼                                        ▼
     │         Hermes AI (MCP) ──▶ Natural Language Ops    Data Plane
     │                                                    (auto-deploy)
     ▼
  Git Push ──▶ Argo Workflows (CI) ──▶ Build ──▶ Release ──▶ Deploy
```

1. **OpenChoreo Portal** provides self-service catalog and Golden Path templates
2. **Flux CD** (built-in) handles GitOps reconciliation automatically
3. **Argo Workflows** runs CI pipelines and builds
4. **Hermes AI** connects via MCP for natural language platform operations
5. **Data Plane** runs workloads with Cilium networking and Envoy gateway

## 📁 Repository Structure

```
airgapOps/
├── openchoreo/                    # Helm values for OpenChoreo installation
│   ├── values-cp.yaml             # Control Plane configuration
│   └── values-dp.yaml             # Data Plane configuration
├── platform/                      # OpenChoreo CRD declarations
│   ├── project.yaml               # Project definition
│   ├── environments/              # Environment definitions
│   │   ├── development.yaml
│   │   └── production.yaml
│   └── components/                # Component definitions
│       ├── mock-api.yaml          # HTTP echo service
│       ├── postgres.yaml          # PostgreSQL database
│       └── rabbitmq.yaml          # RabbitMQ message broker
├── docs/                          # Documentation
│   ├── playbooks/                 # PB-00 through PB-06
│   ├── hermes-skills/             # AI assistant skills
│   ├── bootstrap/                 # One-time setup scripts (reference)
│   ├── architecture-cookbook.md
│   └── multi-cluster-architecture.md
├── .gitignore
└── README.md
```

## 🔗 Key URLs

| Service | URL |
|---------|-----|
| OpenChoreo Portal | `https://choreo.zerops.internal` |
| OpenChoreo API | `https://api.choreo.zerops.internal` |
| ThunderID (OIDC) | `https://thunder.zerops.internal` |
| Gitea | `https://gitea.zerops.internal` |

## 🚀 Installation

### Prerequisites
- K3s cluster on ThinkPad (management) and Ryzen (compute)
- PowerDNS + Step-CA on Raspberry Pi
- Helm v3.12+ and kubectl v1.32+

### Install OpenChoreo

```bash
# 1. Install Gateway API CRDs
kubectl apply --server-side -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml

# 2. Install Control Plane
helm upgrade --install openchoreo-control-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
  --version 1.1.0 \
  --namespace openchoreo-control-plane --create-namespace \
  --values openchoreo/values-cp.yaml --wait --timeout 600s

# 3. Install Data Plane
helm upgrade --install openchoreo-data-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
  --version 1.1.0 \
  --namespace openchoreo-data-plane --create-namespace \
  --values openchoreo/values-dp.yaml --wait --timeout 300s

# 4. Apply Platform Resources
kubectl apply -f platform/
```

## 🤖 AI Integration (Hermes + MCP)

OpenChoreo exposes MCP servers for AI-driven operations:
- **Control Plane MCP**: Resource management (projects, components, deployments)
- **Observability MCP**: Logs, metrics, traces, incident analysis

Configure Hermes to connect via MCP for natural language platform ops.

## 📚 Documentation

- [PB-00: Corporate Network Identity](docs/playbooks/PB-00-corporate-network-identity.md)
- [PB-01: Management Cluster](docs/playbooks/PB-01-management-cluster.md)
- [PB-02: Compute Cluster](docs/playbooks/PB-02-compute-cluster.md)
- [PB-03: First Platform](docs/playbooks/PB-03-first-platform.md)
- [PB-04: Developer Workflow](docs/playbooks/PB-04-developer-workflow.md)
- [PB-05: Firewall & Hardening](docs/playbooks/PB-05-firewall-and-hardening.md)
- [PB-06: GPU LLM Stack](docs/playbooks/PB-06-gpu-llm-stack.md)

## 🛡️ Security

- All traffic uses **mTLS** via Step-CA ACME
- **ThunderID** provides OIDC (built-in to OpenChoreo)
- Secrets via **OpenBao** + External Secrets Operator
- Air-gapped: zero external internet dependencies
- CIS Benchmark hardening in [PB-05](docs/playbooks/PB-05-firewall-and-hardening.md)
