# 🏗️ Zerops Sovereign IDP — airgapOps

> Air-gapped Internal Developer Platform running on bare-metal K3s clusters with ArgoCD GitOps, OpenChoreo, and vcluster isolation.

## 🌐 Architecture

```
┌─────────────────────┐     ┌──────────────────────────┐     ┌──────────────────────────┐
│  🍓 Raspberry Pi    │     │  💻 ThinkPad (Mgmt)      │     │  🖥️ Ryzen (Compute)      │
│  192.168.0.9        │     │  192.168.0.10             │     │  192.168.0.13             │
│                     │     │                           │     │                           │
│  ├── PowerDNS       │────▶│  ├── ArgoCD (GitOps)      │────▶│  ├── MetalLB              │
│  │   zerops.internal│     │  ├── Gitea (Git server)   │     │  ├── Traefik (Ingress)    │
│  └── Step-CA (PKI)  │     │  ├── OpenChoreo (Portal)  │     │  ├── cert-manager         │
│      ACME TLS       │     │  ├── ThunderID (OIDC)     │     │  ├── vcluster-mock        │
│                     │     │  ├── Envoy Gateway        │     │  ├── vcluster-postgres    │
│                     │     │  └── cert-manager          │     │  └── vcluster-rabbitmq    │
└─────────────────────┘     └──────────────────────────┘     └──────────────────────────┘
     Tier-0 (DNS/PKI)           Control Plane                     Data Plane (Compute)
```

## 🔄 GitOps Workflow

```
Developer ──▶ OpenChoreo UI ──▶ Golden Path Template ──▶ Gitea (push YAML)
                                                              │
                                                              ▼
                                                         ArgoCD (detect)
                                                              │
                                                              ▼
                                                     Ryzen Cluster (deploy)
                                                         └── vcluster ready
```

1. **OpenChoreo** provides a self-service catalog with Golden Path templates
2. Templates generate **ArgoCD Application** manifests and push to **Gitea**
3. **ArgoCD** detects changes and deploys directly to the **Ryzen** compute cluster
4. **vclusters** (Loft) run as isolated Kubernetes environments on Ryzen

## 📁 Repository Structure

```
airgapOps/
├── argocd/                    # App-of-Apps pattern
│   ├── root-app.yaml          # Root application (syncs this directory)
│   ├── gateway.yaml           # → gateway/
│   ├── infrastructure.yaml    # → infrastructure/ryzen/
│   ├── vclusters-app.yaml     # → manifests/vclusters/
│   ├── tenant-apps.yaml       # → manifests/tenants/
│   ├── ryzen-metallb.yaml     # Helm → MetalLB on Ryzen
│   ├── ryzen-traefik.yaml     # Helm → Traefik on Ryzen
│   └── ryzen-cert-manager.yaml# Helm → cert-manager on Ryzen
├── gateway/                   # Management cluster networking
│   ├── zerops-gateway.yaml    # Gateway API resource
│   └── external-dns.yaml     # ExternalDNS → PowerDNS
├── infrastructure/ryzen/      # Post-install configs for Ryzen
│   ├── metallb-config.yaml    # IP address pool
│   ├── cert-manager-config.yaml # Step-CA ClusterIssuer
│   └── coredns-zerops-stub.yaml # DNS stub zone
├── manifests/
│   ├── vclusters/             # ArgoCD Apps → Loft vcluster Helm charts
│   └── tenants/               # ArgoCD Apps → workload deployments
├── apps/                      # Workload manifests (K8s Deployments/Services/Ingress)
│   ├── mock-api/
│   ├── postgres/
│   └── rabbitmq/
├── backstage-catalog/         # OpenChoreo catalog entities & templates
│   ├── infrastructure-catalog.yaml
│   └── templates/vcluster-template/
├── docs/                      # Documentation
│   ├── playbooks/             # PB-00 through PB-06
│   ├── hermes-skills/         # AI assistant skills
│   └── bootstrap/             # One-time setup scripts (reference)
└── llm/                       # Ollama/AnythingLLM GPU stack (manual)
```

## 🔗 Key URLs

| Service | URL |
|---------|-----|
| ArgoCD | `https://argocd.zerops.internal` |
| OpenChoreo (Backstage) | `https://choreo.zerops.internal` |
| Gitea | `https://gitea.zerops.internal` |
| ThunderID (OIDC) | `https://thunder.zerops.internal` |
| Mock API | `https://api.zerops.internal` |
| pgAdmin | `https://db.zerops.internal` |
| RabbitMQ Management | `https://queue.zerops.internal` |

## 🚀 Quick Start

### Prerequisites
- Three nodes on the same LAN (192.168.0.x)
- K3s installed on ThinkPad (management) and Ryzen (compute)
- PowerDNS + Step-CA running on Raspberry Pi

### Bootstrap Sequence
1. **Raspberry Pi** — DNS + PKI (see [PB-00](docs/playbooks/PB-00-corporate-network-identity.md))
2. **ThinkPad** — K3s + ArgoCD + Gitea (see [PB-01](docs/playbooks/PB-01-management-cluster.md))
3. **Ryzen** — K3s compute node (see [PB-02](docs/playbooks/PB-02-compute-cluster.md))
4. **Apply root-app** — `kubectl apply -f argocd/root-app.yaml`
5. **OpenChoreo** — Install via Helm (see [bootstrap scripts](docs/bootstrap/))

## 📚 Documentation

- [PB-00: Corporate Network Identity](docs/playbooks/PB-00-corporate-network-identity.md)
- [PB-01: Management Cluster](docs/playbooks/PB-01-management-cluster.md)
- [PB-02: Compute Cluster](docs/playbooks/PB-02-compute-cluster.md)
- [PB-03: First Platform](docs/playbooks/PB-03-first-platform.md)
- [PB-04: Developer Workflow](docs/playbooks/PB-04-developer-workflow.md)
- [PB-05: Firewall & Hardening](docs/playbooks/PB-05-firewall-and-hardening.md)
- [PB-06: GPU LLM Stack](docs/playbooks/PB-06-gpu-llm-stack.md)
- [Architecture Cookbook](docs/architecture-cookbook.md)
- [Multi-Cluster Architecture](docs/multi-cluster-architecture.md)

## 🛡️ Security

- All internal traffic uses **mTLS** via Step-CA ACME certificates
- **ThunderID** provides OIDC authentication for OpenChoreo
- Secrets managed via **OpenBao** + External Secrets Operator
- Air-gapped: no external internet dependencies at runtime
- CIS Benchmark hardening documented in [PB-05](docs/playbooks/PB-05-firewall-and-hardening.md)
