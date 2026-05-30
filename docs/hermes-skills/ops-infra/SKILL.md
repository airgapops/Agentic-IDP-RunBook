---
name: ops-infra
description: >-
  Master reference for the Zerops Sovereign IDP infrastructure.
  Use this skill for: network topology, node access, DNS management,
  TLS certificates, Gitea credentials, OpenChoreo architecture,
  cluster administration, and general platform context.
version: 2.0.0
metadata:
  author: platform-team
  tags: [infrastructure, networking, dns, tls, openchoreo, gitea]
---

# 🧠 SKILL: AirgapOps — Zerops Sovereign IDP Infrastructure

## Platform Architecture

```
Raspberry Pi (192.168.0.9) — Tier 0: DNS + PKI
├── PowerDNS        → zerops.internal zone (Split-Horizon)
└── Step-CA          → ACME TLS (private CA)

ThinkPad (192.168.0.10) — Tier 1: Control Plane
├── OpenChoreo Control Plane (6 pods)
│   ├── Backstage      (Developer Portal)
│   ├── OpenChoreo API (REST API + MCP servers)
│   ├── ThunderID      (OIDC identity provider)
│   ├── Flux CD        (GitOps engine — built-in)
│   ├── Controller     (CRD reconciler)
│   └── Gateway        (Envoy)
├── OpenChoreo Data Plane (2 pods)
│   ├── Cluster Agent  (connects to Control Plane)
│   └── Gateway        (Envoy)
├── Gitea              (Git server + Container Registry)
├── Hermes AI          (running on host)
└── K3s v1.35          (kubeconfig: /etc/rancher/k3s/k3s.yaml)

Ryzen (192.168.0.13) — Tier 2: Compute (currently off)
├── K3s v1.35
├── NVIDIA RTX 5060 Ti (16GB VRAM)
├── MetalLB (192.168.0.240-245)
├── Traefik (ingress, hostNetwork: true)
└── vclusters (mock, postgres, rabbitmq)
```

## Access Credentials

| Service | URL | User | Password |
|---------|-----|------|----------|
| OpenChoreo Portal | `https://choreo.zerops.internal` | admin@openchoreo.dev | Admin@123 |
| Gitea | `https://gitea.zerops.internal` | jadamson | Panama2028#... |
| Gitea Container Registry | `gitea.zerops.internal` | jadamson | Panama2028#... |
| PowerDNS API | `http://192.168.0.9:8081` | — | API Key: `super-secret-api-key-zerops` |
| Raspberry Pi SSH | `ubuntu@192.168.0.9` | ubuntu | Panama2028#... |
| ThinkPad SSH | `rooter@192.168.0.10` | rooter | Panama2028#... |
| Ryzen SSH | `admin@192.168.0.13` | admin | ADMIN123 |

## Kubeconfig

```bash
# ThinkPad (management cluster — where OpenChoreo runs)
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
```

## DNS Management (PowerDNS)

```bash
# Add/update a DNS record
curl -X PATCH http://192.168.0.9:8081/api/v1/servers/localhost/zones/zerops.internal. \
  -H "X-API-Key: super-secret-api-key-zerops" \
  -H "Content-Type: application/json" \
  -d '{"rrsets": [{"name": "{name}.zerops.internal.", "type": "A", "ttl": 300,
       "changetype": "REPLACE", "records": [{"content": "192.168.0.10", "disabled": false}]}]}'
```

## Gitea Container Registry

```bash
# Push image to Gitea (on-premise, air-gapped)
docker login gitea.zerops.internal -u jadamson -p 'Panama2028#...'
docker tag myapp:latest gitea.zerops.internal/jadamson/myapp:v1.0
docker push gitea.zerops.internal/jadamson/myapp:v1.0
```

## OpenChoreo Resources (current state)

| Resource | Items |
|----------|-------|
| Project | `default` |
| Components | `mock-api`, `postgres`, `rabbitmq` |
| Workloads | `mock-api`, `postgres`, `rabbitmq` |
| Environments | `development`, `staging`, `production` |
| ClusterComponentTypes | `service`, `web-application`, `worker`, `scheduled-task` |
| DeploymentPipeline | `default` |

## Git Repository

```bash
# Clone from Gitea
git clone https://gitea.zerops.internal/jadamson/airgap-ops.git

# Repository structure
airgapOps/
├── openchoreo/      # Helm values (reference)
├── platform/        # OpenChoreo CRDs
└── docs/            # Playbooks + Hermes skills
```

## Related Skills
- **openchoreo-deploy**: Create and deploy Components
- **openchoreo-promote**: Promote between environments
- **openchoreo-secrets**: Manage secrets and env vars
- **openchoreo-observe**: Diagnose and monitor
- **dns-infrastructure**: DNS zone management
- **gpu-llm-setup**: NVIDIA + Ollama on Ryzen
