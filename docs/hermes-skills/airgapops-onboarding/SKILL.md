---
name: airgapops-onboarding
description: >-
  Master orchestrator skill for airgapOps platform setup.
  Use this skill when: a new buyer needs to install the platform,
  a user asks "how do I get started?", or someone needs to set up
  airgapOps from scratch. This skill guides through ALL phases:
  infrastructure, OpenChoreo, agctl CLI, first deployment, and verification.
  It delegates to sub-skills for each phase.
version: 1.0.0
metadata:
  author: platform-team
  tags: [onboarding, setup, installation, master, orchestrator]
---

# 🚀 SKILL: airgapOps — Complete Platform Setup

## When to Use
- New buyer asks "how do I install this?"
- User says "set up airgapOps from scratch"
- User asks "what do I need to get started?"
- User asks "walk me through the installation"
- User wants to replicate the airgapOps stack in their environment

## CRITICAL RULES
1. **FOLLOW THE PHASES IN ORDER** — each phase depends on the previous one
2. **VERIFY before moving to the next phase** — run the verification command
3. **ASK the user for their hardware details** before starting Phase 1
4. **DELEGATE to sub-skills** — this skill orchestrates, sub-skills execute
5. **NEVER skip Phase 0** — hardware requirements must be confirmed first

---

## Phase 0: Hardware Requirements Check

Before anything, confirm the user has:

### Minimum Hardware (3 nodes)

| Node | Role | Min Specs | Purpose |
|------|------|-----------|---------|
| **Node 1** — DNS/PKI | Tier 0 | 2GB RAM, any CPU | PowerDNS + Step-CA |
| **Node 2** — Control Plane | Tier 1 | 8GB RAM, 4 cores | OpenChoreo + Hermes + agctl |
| **Node 3** — Data Plane | Tier 2 | 16GB RAM, 8 cores | Workloads + GPU (optional) |

### Network Requirements
- [ ] All 3 nodes on the same LAN (or routable)
- [ ] Static IPs assigned to each node
- [ ] No internet required (air-gapped)
- [ ] Ports open between nodes: 6443 (K8s API), 8081 (DNS API), 22 (SSH)

### Software Requirements
- [ ] Ubuntu 22.04+ or similar Linux on all nodes
- [ ] SSH access between nodes
- [ ] A workstation (Mac/Linux) with Go 1.22+ for building agctl

**ASK the user:**
```
Before we start, I need to know:
1. What are the IPs of your 3 nodes? (DNS, Control Plane, Data Plane)
2. What OS are they running?
3. What are the SSH credentials for each?
4. Do you have a GPU on the Data Plane node? (optional)
```

---

## Phase 1: DNS & PKI Setup (Node 1)

**Sub-skill:** `dns-infrastructure`

### Steps
1. Install PowerDNS on the DNS node
2. Create the internal zone (e.g., `zerops.internal`)
3. Install Step-CA for private TLS certificates (optional)
4. Add initial DNS records for the other nodes

### Verification
```bash
# From any node:
dig @<DNS_NODE_IP> choreo.<DOMAIN> +short
# Should return the Control Plane IP
```

### Key DNS Records to Create

| Record | Points To | Purpose |
|--------|----------|---------|
| `choreo.<domain>` | Control Plane IP | OpenChoreo portal |
| `gitea.<domain>` | Control Plane IP | Git + Container Registry |
| `*.<domain>` | Data Plane MetalLB IP | All deployed apps |

---

## Phase 2: Control Plane Setup (Node 2)

### 2a. Install K3s
**Sub-skill:** `k8s-provisioning`

```bash
# On the Control Plane node:
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="server" sh -
```

> In air-gapped: download K3s binary and images tarball beforehand

### 2b. Install Container Registry (Gitea)

```bash
# Deploy Gitea for Git + Container Registry
# Configure K3s to use it as a private registry
```

### 2c. Install OpenChoreo
**Sub-skill:** `openchoreo-deploy`

```bash
# Install via Helm
helm install openchoreo ./openchoreo-chart \
  --namespace openchoreo-control-plane --create-namespace \
  --values openchoreo/values-control-plane.yaml
```

### 2d. Install Hermes Agent

```bash
# Install Hermes
npm install -g @anthropic/hermes  # or local install
# Configure with OpenRouter API key
```

### 2e. Install agctl CLI

```bash
# On the build machine (Mac/Linux with Go):
cd cli/
GOOS=linux GOARCH=amd64 go build -o bin/agctl-linux .

# Deploy to Control Plane:
scp bin/agctl-linux <CP_USER>@<CP_IP>:/usr/local/bin/agctl
chmod +x /usr/local/bin/agctl

# Deploy config:
scp configs/agctl.yaml <CP_USER>@<CP_IP>:/etc/agctl/config.yaml
```

### 2f. Deploy Hermes Skills

```bash
# Copy all skills to the Hermes skills directory
cp -r docs/hermes-skills/* /root/.hermes/skills/
```

### Verification
```bash
# OpenChoreo running?
sudo kubectl get pods -n openchoreo-control-plane
# All pods should be Running

# agctl working?
agctl version
# Should print: agctl v0.1.0

# agctl can reach both clusters?
agctl status
```

---

## Phase 3: Data Plane Setup (Node 3)

### 3a. Install K3s
```bash
# On the Data Plane node:
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="server" sh -
```

### 3b. Configure Private Registry
```bash
# Point to Gitea container registry
cat > /etc/rancher/k3s/registries.yaml <<EOF
mirrors:
  "gitea.<domain>":
    endpoint:
      - "https://gitea.<domain>"
configs:
  "gitea.<domain>":
    auth:
      username: <user>
      password: <password>
    tls:
      insecure_skip_verify: true
EOF
systemctl restart k3s
```

### 3c. Install MetalLB (LoadBalancer)
```bash
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/main/config/manifests/metallb-native.yaml
# Configure IP pool for services
```

### 3d. Connect Data Plane to Control Plane
**Sub-skill:** `openchoreo-deploy`

```bash
# Register the Data Plane cluster in OpenChoreo
sudo kubectl apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterDataPlane
metadata:
  name: <data-plane-name>
spec:
  planeID: <data-plane-name>
  ...
EOF
```

### 3e. Export Kubeconfig for agctl
```bash
# Copy Data Plane kubeconfig to Control Plane
scp /etc/rancher/k3s/k3s.yaml <CP_USER>@<CP_IP>:/etc/agctl/dp-kubeconfig.yaml
# Edit the server URL to point to Data Plane IP
sed -i "s|127.0.0.1|<DP_IP>|" /etc/agctl/dp-kubeconfig.yaml
```

### 3f. GPU Setup (optional)
**Sub-skill:** `gpu-llm-setup`

### Verification
```bash
# From Control Plane:
agctl status
# Should show all 3 environments in the Data Plane

kubectl --kubeconfig=/etc/agctl/dp-kubeconfig.yaml get nodes
# Should show the Data Plane node
```

---

## Phase 4: Platform Configuration

### 4a. Create Environments
**Sub-skill:** `openchoreo-deploy`

```bash
sudo kubectl apply -f platform/environments/development.yaml
sudo kubectl apply -f platform/environments/production.yaml
```

### 4b. Update agctl Config
Edit `/etc/agctl/config.yaml` with the actual namespace IDs:
```bash
# Get namespace IDs
sudo kubectl get ns | grep dp-default
```

### Verification
```bash
agctl status
# Should show environments with their namespaces
```

---

## Phase 5: First Deployment

### 5a. Deploy PostgreSQL + PostgREST
**Sub-skill:** `openchoreo-stack-postgrest`

Tell Hermes:
```
Despliega PostgreSQL y PostgREST en development
```

### 5b. Attach Persistent Storage
**Sub-skill:** `agctl-storage`

```bash
agctl storage attach postgres --size 5Gi --env development
```

### 5c. Seed Database
**Sub-skill:** `agctl-platform`

```bash
agctl db seed postgres --env development
```

### 5d. Configure Secrets
**Sub-skill:** `agctl-secrets`

```bash
agctl secret set postgrest PGRST_DB_URI="postgres://zerops:zerops123@postgres:5432/app" --env development
```

### 5e. Expose with HTTPS
**Sub-skill:** `expose-service`, `cert-management`

### Verification
```bash
# End-to-end test
agctl status
agctl db exec postgres "SELECT count(*) FROM todos" --env development
curl -sk https://postgrest-dev.<domain>/todos
```

---

## Phase 6: Promote to Production

**Sub-skill:** `openchoreo-promote`

```bash
# Promote PostgREST and Postgres to production
# This includes: DNS, TLS, Gateway, HTTPRoute, NetworkPolicy

# Then:
agctl storage attach postgres --size 10Gi --env production
agctl db seed postgres --env production
agctl secret sync postgrest --from development --to production
```

### Final Verification
```bash
agctl status
# All environments green ✅
# PVCs bound ✅
# Gateway listeners within limit ✅

agctl db exec postgres "SELECT count(*) FROM todos" --env all
# All environments have data ✅

curl -sk https://postgrest-prod.<domain>/todos
# API responding ✅
```

---

## Phase Map — Skills Index

```
Phase 0: Requirements          → (this skill)
Phase 1: DNS/PKI               → dns-infrastructure
Phase 2: Control Plane         → k8s-provisioning, openchoreo-deploy, hermes-setup
Phase 3: Data Plane            → k8s-provisioning, ryzen-network-fix, gpu-llm-setup
Phase 4: Configuration         → openchoreo-deploy, ops-infra
Phase 5: First Deployment      → openchoreo-stack-postgrest, agctl-storage,
                                  agctl-secrets, agctl-platform, expose-service
Phase 6: Promotion             → openchoreo-promote, cert-management
```

## Troubleshooting Quick Reference

| Problem | Likely Cause | Fix Skill |
|---------|-------------|-----------|
| Pods not starting | Image not in registry | ops-infra (Gitea section) |
| 503 errors | NetworkPolicy blocking | openchoreo-deploy (rule #5) |
| Data lost on restart | No PVC | agctl-storage |
| `***` in secrets | Webhook masking | agctl-secrets |
| DNS not resolving | PowerDNS config | dns-infrastructure |
| TLS errors | Certificate issue | cert-management |
| Gateway at 7/7 | Listener limit | openchoreo-promote (rule #10) |
| API returns empty | Tables missing | agctl-platform (db seed) |
