# 🧠 SKILL: AirgapOps — Infraestructura Multi-Cluster Soberana

## Contexto del Proyecto

Plataforma soberana air-gapped con dos clusters físicos independientes gestionados via Crossplane + ArgoCD + vcluster.

**Repo local:** `/Users/jonnhy/Documents/airgapOps`

---

## 🗺️ Topología de Red

| Nodo | IP | Rol | OS | Acceso SSH |
|---|---|---|---|---|
| Raspberry Pi | `192.168.0.9` | Tier 0 — PKI (Step-CA) + DNS (PowerDNS) | Raspbian | `ubuntu@192.168.0.9` / pass: `Panama2028#...` |
| ThinkPad | `192.168.0.10` | Tier 1 — Management Cluster (K3s) | — | SSH bloqueado, usar ArgoCD API |
| Ryzen | `192.168.0.13` | Tier 2 — Compute Cluster (K3s) + GPU RTX 5060 Ti | Rocky Linux 10 | `admin@192.168.0.13` / pass: `ADMIN123` |

> [!IMPORTANT]
> **ArgoCD** es accesible en `https://argocd.zerops.internal` — usuario: `admin` / contraseña: `uU285kuivWXq95qD`  
> **Gitea** en `https://gitea.zerops.internal` — usuario: `jadamson` / contraseña: `Panama2028#...` (repo: `jadamson/airgap-ops`)  
> **Step-CA ACME** en `https://192.168.0.9:9000/acme/acme/directory`  
> **PowerDNS API** en `http://192.168.0.9:8081` — API Key: `super-secret-api-key-zerops`  
> **OpenRouter API Key:** `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`

---

## 🏗️ Arquitectura de Clusters

```
ThinkPad (192.168.0.10) — Management Cluster
├── ArgoCD          → GitOps engine, sincroniza desde Gitea (jadamson/airgap-ops)
├── Crossplane      → Control plane remoto
│   ├── Provider Kubernetes → ryzen-compute-cluster (ProviderConfig)
│   └── Provider Helm       → ryzen-helm-cluster (ProviderConfig)
├── Gitea           → Source of truth (git interno)
└── Backstage       → Developer portal

Ryzen (192.168.0.13) — Compute Cluster
├── K3s v1.35.5+k3s1  (Service CIDR: 10.43.0.0/16, Pod CIDR: 10.42.0.0/16)
├── MetalLB         → Pool: 192.168.0.240-245, Traefik en 192.168.0.240
├── Traefik v2.11   → IngressController con hostNetwork:true (CRÍTICO — ver abajo)
├── cert-manager    → ClusterIssuer "step-ca" → Step-CA ACME
├── NVIDIA RTX 5060 Ti (Blackwell GB206, 16 GB VRAM)
└── vclusters (namespaces en Ryzen, gestionados por Crossplane desde ThinkPad)
    ├── vcluster-mock      → api.zerops.internal  (mock JSON API)
    ├── vcluster-postgres  → db.zerops.internal   (PostgreSQL + pgAdmin)
    └── vcluster-rabbitmq  → queue.zerops.internal (RabbitMQ + Management UI)
```

---

## 🌐 Stack de Networking Completo

### DNS (PowerDNS en Pi)
- **Zona:** `zerops.internal`
- Registros: `api`, `db`, `queue` → `192.168.0.240` (MetalLB VIP de Traefik)
- Para agregar registros vía API:
```bash
curl -X PATCH http://192.168.0.9:8081/api/v1/servers/localhost/zones/zerops.internal. \
  -H "X-API-Key: super-secret-api-key-zerops" \
  -H "Content-Type: application/json" \
  -d '{"rrsets": [{"name": "nuevo.zerops.internal.", "type": "A", "ttl": 300,
       "changetype": "REPLACE", "records": [{"content": "192.168.0.240", "disabled": false}]}]}'
```

### MetalLB
- **Pool:** `192.168.0.240-245`
- **Archivo:** `infrastructure/ryzen/metallb-config.yaml`
- ⚠️ `192.168.0.200` está ocupado por router Belkin — no usar

### Traefik (CRÍTICO — hostNetwork)
- **Archivo:** `infrastructure/ryzen/traefik-helm.yaml`
- **Versión:** v2.11.0 (chart v26.1.0)
- **IP externa:** `192.168.0.240` (MetalLB LoadBalancer)
- **`hostNetwork: true`** — OBLIGATORIO. Sin esto, Traefik usa pod IPs directas
  que son inalcanzables por el bug de kube-router (chains iptables obsoletas
  post-reboot bloquean pod-to-pod entre namespaces)
- **Sin redirect HTTP→HTTPS global** — necesario para que cert-manager ACME HTTP-01 funcione
- Para re-aplicar si ArgoCD revierte:
```bash
# Via SSH en Ryzen:
sudo -S -k bash <<< 'ADMIN123
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl patch deploy traefik -n traefik-system --type=json \
  -p='"'"'[{"op":"add","path":"/spec/template/spec/hostNetwork","value":true},
          {"op":"add","path":"/spec/template/spec/dnsPolicy","value":"ClusterFirstWithHostNet"}]'"'"'
'
```

### CoreDNS (CRÍTICO — stub zone para cert-manager)
- **Archivo:** `infrastructure/ryzen/coredns-zerops-stub.yaml`
- Agrega `zerops.internal → 192.168.0.9` al CoreDNS del Ryzen K3s
- Sin esto, cert-manager ACME HTTP-01 self-check falla con `no such host`
- Para verificar: `kubectl run dns-test --rm -it --restart=Never --image=busybox -- nslookup api.zerops.internal`

### TLS (cert-manager + Step-CA)
- **ClusterIssuer:** `step-ca`
- **Archivo:** `infrastructure/ryzen/cert-manager-config.yaml`
- Los certificados se emiten automáticamente via ACME HTTP-01 cuando un Ingress
  tiene la annotation `cert-manager.io/cluster-issuer: step-ca`
- **Root CA** del Pi: `/home/ubuntu/zerops-network/root_ca.crt`
- Para confiar en el Mac: `sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain /tmp/zerops-root-ca.crt`

---

## 🏠 vclusters — Stack Actual

### Configuración común (vcluster 0.15.5)

```yaml
# En cada HelmRelease de vcluster:
values:
  serviceCIDR: "10.43.0.0/16"   # OBLIGATORIO — sin esto los vclusters crashean
  vcluster:
    extraArgs:
    - --tls-san=192.168.0.13
  syncer:
    extraArgs:
    - --name=vcluster-<nombre>
    - --service-account=vc-workload-vcluster-<nombre>
    - --kube-config-context-name=my-vcluster
    - --leader-elect=false
    - --sync=ingresses          # sync Ingresses al host K3s
    - --tls-san=192.168.0.13
    # ⚠️ NO incluir --enable-ingress-classes → NO soportado en v0.15.5, causa CrashLoop
  sync:
    ingresses:
      enabled: true
```

### Namespaces en el host K3s

| vcluster | Namespace host | Workloads synced |
|---|---|---|
| vcluster-mock | `vcluster-mock` | mock-api pod + Ingress → api.zerops.internal |
| vcluster-postgres | `vcluster-postgres` | postgres + pgadmin pods + Ingress → db.zerops.internal |
| vcluster-rabbitmq | `vcluster-rabbitmq` | rabbitmq pod + Ingress → queue.zerops.internal |

### Acceder a un vcluster desde el Mac

```bash
# Obtener kubeconfig del vcluster (via kubectl en Ryzen)
sshpass -p 'ADMIN123' ssh admin@192.168.0.13 "
  echo 'ADMIN123' | sudo -S -k sh -c '
    export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
    /usr/local/bin/kubectl get secret vc-vcluster-mock -n vcluster-mock \
      -o jsonpath=\"{.data.config}\" | base64 -d
  '
"
```

### Endpoints HTTPS en producción

| Servicio | URL | Tipo | Credenciales |
|---|---|---|---|
| Mock API | `https://api.zerops.internal/` | JSON echo | — |
| pgAdmin | `https://db.zerops.internal/` | Web UI | admin@zerops.io / zerops123 |
| RabbitMQ UI | `https://queue.zerops.internal/` | Web UI | zerops / zerops123 |

```bash
# Test rápido desde Mac (agregar al /etc/hosts: 192.168.0.240 api.zerops.internal db.zerops.internal queue.zerops.internal)
curl -sk https://api.zerops.internal/ | python3 -m json.tool | head -10
curl -sk -o /dev/null -w "%{http_code}" https://db.zerops.internal/
curl -sk -o /dev/null -w "%{http_code}" https://queue.zerops.internal/
```

---

## 🐛 Problemas Conocidos y Fixes

### vcluster CrashLoopBackOff — `--enable-ingress-classes` inválido
**Síntoma:** `vcluster-*-0` en CrashLoopBackOff  
**Causa:** Flag `--enable-ingress-classes` no existe en vcluster 0.15.5  
**Fix:** Remover el flag del StatefulSet syncer args

### vcluster CIDR mismatch
**Síntoma:** `Service "kubernetes" is invalid: spec.clusterIPs: failed to allocate IP 10.43.x.x: not in range 10.96.0.0/12`  
**Fix:** `serviceCIDR: "10.43.0.0/16"` a nivel top del helm values (NO en syncer.extraArgs)

### cert-manager ACME HTTP-01 self-check falla — `no such host`
**Síntoma:** Challenge stuck en `pending` con `dial tcp: lookup api.zerops.internal on 10.43.0.10:53: no such host`  
**Causa:** CoreDNS no reenvía consultas de `zerops.internal` a PowerDNS  
**Fix:** CoreDNS stub zone (ya aplicado): `infrastructure/ryzen/coredns-zerops-stub.yaml`

### cert-manager ACME HTTP-01 — `502 Bad Gateway`
**Síntoma:** Challenge `pending` con `wrong status code '502', expected '200'`  
**Causa:** Traefik usa pod IPs directas (via Endpoints) que son inalcanzables por kube-router
iptables chains obsoletas (KUBE-ROUTER-FORWARD contiene KUBE-POD-FW-* chains que bloquean
tráfico cross-namespace entre traefik-system y los namespaces vcluster-*)  
**Fix:** `hostNetwork: true` en el Deployment de Traefik (corre en el network namespace del host
donde los pod IPs sí son alcanzables)

### Traefik revierte a pod network (ArgoCD re-sync)
**Síntoma:** `spec.hostNetwork` es false, pod IP vuelve a `10.42.0.X`  
**Causa:** La Crossplane HelmRelease reconcilia y el helm chart no aplica hostNetwork vía values  
**Fix manual:**
```bash
sshpass -p 'ADMIN123' ssh admin@192.168.0.13 "sudo -S -k bash" <<< 'ADMIN123
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
/usr/local/bin/kubectl patch deploy traefik -n traefik-system --type=json \
  -p='"'"'[{"op":"add","path":"/spec/template/spec/hostNetwork","value":true},
          {"op":"add","path":"/spec/template/spec/dnsPolicy","value":"ClusterFirstWithHostNet"}]'"'"'
'
```
> [!IMPORTANT]
> Este fix es temporal — ArgoCD/Crossplane puede revertirlo. La solución permanente es un
> post-sync hook en ArgoCD o configurar Crossplane para ignorar este campo.

### MetalLB IP conflicto
**Síntoma:** LoadBalancer IP `192.168.0.200` no responde  
**Causa:** `192.168.0.200` está ocupado por router Belkin externo  
**Fix:** Pool MetalLB usa `192.168.0.240-245`

### NVIDIA Device Plugin — ruta kubelet
**Síntoma:** GPU no aparece como allocatable  
**Causa:** K3s usa `/var/lib/kubelet/device-plugins/` (ruta estándar K8s)  
**Fix:** Montar `/var/lib/kubelet/device-plugins` en el DaemonSet.

### Ollama — CPU en lugar de GPU
**Síntoma:** Ollama no usa CUDA  
**Causa:** Backend ROCm interfiere con detección NVIDIA  
**Fix:** `mv /usr/local/lib/ollama/rocm /usr/local/lib/ollama/rocm.disabled && systemctl restart ollama`

---

## 🔄 Flujo de Trabajo Operacional

### Aplicar cambios via GitOps (correcto)
```bash
# 1. Editar archivos en Mac
# 2. Push a Gitea via HTTPS (SSL skip)
cd /Users/jonnhy/Documents/airgapOps
git add .
git commit -m "feat: descripción"
git push  # push al repo local → Gitea → ArgoCD detecta → Crossplane aplica
```

### Cambios urgentes directos en Ryzen (bypass GitOps)
```bash
# SIEMPRE via SSH con sshpass:
sshpass -p 'ADMIN123' ssh -o StrictHostKeyChecking=no admin@192.168.0.13 \
  "echo 'ADMIN123' | sudo -S -k bash -c 'export KUBECONFIG=/etc/rancher/k3s/k3s.yaml && <comando>'"
```

### Copiar scripts al Ryzen y ejecutar
```bash
# Para scripts complejos con Python:
sshpass -p 'ADMIN123' scp -o StrictHostKeyChecking=no /tmp/script.py admin@192.168.0.13:/tmp/
sshpass -p 'ADMIN123' ssh -o StrictHostKeyChecking=no admin@192.168.0.13 \
  "echo 'ADMIN123' | sudo -S -k python3 /tmp/script.py"
```

### Verificar estado del stack completo
```bash
sshpass -p 'ADMIN123' ssh admin@192.168.0.13 "sudo bash" <<< 'ADMIN123
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
K=/usr/local/bin/kubectl
echo "=== vclusters ===" && $K get pods -A | grep "vcluster-.*-0"
echo "=== Traefik ===" && $K get svc traefik -n traefik-system
echo "=== Certs ===" && $K get certificates -A
echo "=== Ingresses ===" && $K get ingress -A
'
```

### Forzar re-emisión de certificados TLS
```bash
# Borrar CertificateRequests → cert-manager los recrea y vuelve a pedir certs
sshpass -p 'ADMIN123' ssh admin@192.168.0.13 "sudo bash" <<< 'ADMIN123
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
K=/usr/local/bin/kubectl
for ns in vcluster-mock vcluster-postgres vcluster-rabbitmq; do
  $K delete certificaterequest --all -n $ns
done
'
```

---

## 📂 Archivos Clave del Repo

```
infrastructure/
├── ryzen/
│   ├── metallb-config.yaml          → Pool 192.168.0.240-245
│   ├── traefik-helm.yaml            → Traefik IngressController (hostNetwork!)
│   ├── cert-manager-config.yaml     → ClusterIssuer step-ca
│   └── coredns-zerops-stub.yaml     → CoreDNS stub zone zerops.internal → PowerDNS
├── vclusters/
│   ├── vcluster-mock.yaml           → HelmRelease vcluster-mock
│   ├── vcluster-postgres.yaml       → HelmRelease vcluster-postgres
│   └── vcluster-rabbitmq.yaml       → HelmRelease vcluster-rabbitmq
└── compositions/
    ├── platform-composition.yaml    → Crossplane Composition para vclusters
    └── platform-xrd.yaml            → XRD del CompositePlatform
```

---

## 📂 Playbooks Disponibles

| Playbook | Descripción |
|---|---|
| `PB-00` | Corporate network identity |
| `PB-01` | Management cluster (ThinkPad) |
| `PB-02` | Compute cluster (Ryzen) |
| `PB-03` | Primera platform via Crossplane |
| `PB-04` | Developer workflow |
| `PB-05` | Firewall y hardening |
| `PB-06` | GPU + LLM stack (NVIDIA Blackwell + Ollama + Hermes) |
