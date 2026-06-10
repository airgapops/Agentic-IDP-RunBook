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
DNS Node (<DNS_IP>) — Tier 0: DNS + PKI
├── PowerDNS        → <YOUR_DOMAIN> zone (Split-Horizon)
└── Step-CA          → ACME TLS (private CA)

Control Plane (<CP_IP>) — Tier 1: Control Plane
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

Data Plane (<DP_IP>) — Tier 2: Compute
├── K3s v1.35
├── NVIDIA GPU (optional)
├── MetalLB (<GATEWAY_IP_RANGE>)
├── Traefik (ingress, hostNetwork: true)
└── Application workloads
```

## Access Credentials

| Service | URL | User | Password |
|---------|-----|------|----------|
| OpenChoreo Portal | `https://choreo.<YOUR_DOMAIN>` | admin@openchoreo.dev | `<CHOREO_PASSWORD>` |
| Gitea | `https://gitea.<YOUR_DOMAIN>` | `<GITEA_USER>` | `<GITEA_PASSWORD>` |
| Gitea Container Registry | `gitea.<YOUR_DOMAIN>` | `<GITEA_USER>` | `<GITEA_PASSWORD>` |
| PowerDNS API | `http://<DNS_IP>:8081` | — | API Key: `<POWERDNS_API_KEY>` |
| DNS Node SSH | `<DNS_USER>@<DNS_IP>` | `<DNS_USER>` | `<DNS_PASSWORD>` |
| Control Plane SSH | `<CP_USER>@<CP_IP>` | `<CP_USER>` | `<CP_PASSWORD>` |
| Data Plane SSH | `<DP_USER>@<DP_IP>` | `<DP_USER>` | `<DP_PASSWORD>` |

## Kubeconfig

```bash
# ThinkPad (management cluster — where OpenChoreo runs)
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
```

## DNS Management (PowerDNS)

```bash
# Add/update a DNS record
curl -X PATCH http://<DNS_IP>:8081/api/v1/servers/localhost/zones/<YOUR_DOMAIN>. \
  -H "X-API-Key: <POWERDNS_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"rrsets": [{"name": "{name}.<YOUR_DOMAIN>.", "type": "A", "ttl": 300,
       "changetype": "REPLACE", "records": [{"content": "<CP_IP>", "disabled": false}]}]}'
```

## Gitea Container Registry

```bash
# Push image to Gitea (on-premise, air-gapped)
docker login gitea.<YOUR_DOMAIN> -u <GITEA_USER> -p '<GITEA_PASSWORD>'
docker tag myapp:latest gitea.<YOUR_DOMAIN>/<GITEA_USER>/myapp:v1.0
docker push gitea.<YOUR_DOMAIN>/<GITEA_USER>/myapp:v1.0
```

## OpenChoreo Resources (current state)

| Resource | Items |
|----------|-------|
| Project | `default` |
| Components | (your deployed components) |
| Environments | `development`, `staging`, `production` |
| ClusterComponentTypes | `service`, `web-application`, `worker`, `scheduled-task` |
| DeploymentPipeline | `default` |

## Git Repository

```bash
# Clone from Gitea
git clone https://gitea.<YOUR_DOMAIN>/<GITEA_USER>/airgap-ops.git

# Repository structure
airgapOps/
├── cli/             # agctl CLI source code
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
