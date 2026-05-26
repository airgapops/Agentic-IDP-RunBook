# 🧠 SKILL: AirgapOps — Infraestructura Multi-Cluster Soberana

## Contexto del Proyecto

Plataforma soberana air-gapped con dos clusters físicos independientes gestionados via Crossplane + ArgoCD + vcluster.

**Repo local:** `/Users/jonnhy/Documents/airgapOps`

---

## 🗺️ Topología de Red

| Nodo | IP | Rol | OS | Acceso SSH |
|---|---|---|---|---|
| Raspberry Pi | `192.168.0.9` | Tier 0 — PKI (Step-CA) + DNS (PowerDNS) | Raspbian | — |
| ThinkPad | `192.168.0.10` | Tier 1 — Management Cluster (K3s) | — | SSH bloqueado, usar ArgoCD API |
| Ryzen | `192.168.0.13` | Tier 2 — Compute Cluster (K3s) + GPU RTX 5060 Ti | Rocky Linux 10 | `ssh admin@192.168.0.13` / pass: `ADMIN123` |

> [!IMPORTANT]
> **ArgoCD** es accesible desde cualquier nodo en `https://argocd.zerops.internal` — usuario: `admin` / contraseña: `uU285kuivWXq95qD`  
> **ThinkPad SSH** no accesible remotamente (sin llave SSH configurada). Gestionar via ArgoCD API.  
> **OpenRouter API Key:** `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`

> [!IMPORTANT]
> El ThinkPad es `root@rooter` según prompt de shell. Acceso SSH al Ryzen: `ssh admin@192.168.0.13` con password `ADMIN123`.

---

## 🏗️ Arquitectura de Clusters

```
ThinkPad (192.168.0.10) — Management Cluster
├── ArgoCD          → GitOps engine, sincroniza desde Gitea
├── Crossplane      → Control plane remoto
│   ├── Provider Kubernetes → ryzen-compute-cluster (ProviderConfig)
│   └── Provider Helm       → ryzen-helm-cluster (ProviderConfig)
├── Gitea           → Source of truth (git interno)
└── OpenChoreo/Backstage → Developer portal

Ryzen (192.168.0.13) — Compute Cluster (independiente)
├── K3s             → Service CIDR: 10.43.0.0/16
├── NVIDIA RTX 5060 Ti (Blackwell GB206, 16 GB VRAM)
│   ├── Driver: 595.71.05 (open kernel module — obligatorio Blackwell)
│   ├── CUDA: 13.2
│   └── nvidia-device-plugin v0.17.0 (montado en /var/lib/kubelet/device-plugins/)
├── Ollama (CUDA, ROCm deshabilitado)
│   ├── hermes3:8b
│   ├── llama3.1:8b
│   └── qwen2.5:14b  ← mejor para function calling / agentes
└── vclusters (creados por Crossplane desde ThinkPad)
    └── ryzen-dev-platform (namespace: default-ns)
```

---

## 🔑 Credenciales y Kubeconfigs

| Recurso | Ubicación | Detalle |
|---|---|---|
| Kubeconfig Ryzen | `/Users/jonnhy/Documents/airgapOps/ryzen-kubeconfig.yaml` | Apunta a `192.168.0.13:6443` |
| Kubeconfig ThinkPad | En el ThinkPad en `~/.kube/config` o `/etc/rancher/k3s/k3s.yaml` | Context default |
| SSH Ryzen | `admin@192.168.0.13` | Password: `ADMIN123` |

---

## ⚙️ Crossplane — Compositions y CRDs

| Archivo | Descripción |
|---|---|
| `infrastructure/compositions/platform-composition.yaml` | Crea namespace + vcluster via Helm en Ryzen |
| `infrastructure/compositions/platform-xrd.yaml` | XRD del CompositePlatform |
| `infrastructure/compositions/llmkube-composition.yaml` | Composition para stacks LLM en GPU |
| `infrastructure/crossplane-providers.yaml` | Providers: Helm + Kubernetes |
| `infrastructure/crossplane-providerconfigs.yaml` | ProviderConfigs apuntando al Ryzen |

**Nota crítica:** Las Compositions son gestionadas por ArgoCD. Cambios manuales via `kubectl apply/patch` serán sobreescritos. **El flujo correcto es: editar en repo → git push → ArgoCD sync**.

---

## 🐛 Problemas Conocidos y Fixes

### vcluster CrashLoopBackOff — CIDR mismatch
**Síntoma:** `Service "kubernetes" is invalid: spec.clusterIPs: failed to allocate IP 10.43.x.x: not in range 10.96.0.0/12`  
**Causa raíz:** El syncer de vcluster falla en auto-detectar el Service CIDR del host (`10.43.0.0/16`) y cae al fallback `10.96.0.0/12`.

**Historia del fix (3 iteraciones):**
| Intento | Fix | Error | Por qué falló |
|---|---|---|---|
| 1 | `cidr-wrapper` init container en k3s binary | `IP not in range 10.96.0.0/12` | Wrapper arregla k3s pero el syncer tiene su propio CIDR |
| 2 | `syncer.extraArgs: [--service-cidr=10.43.0.0/16]` | `unknown flag: --service-cidr` | El syncer binary NO acepta ese flag |
| 3 ✅ | `serviceCIDR: "10.43.0.0/16"` (top-level helm) | — | Configura AMBOS: syncer interno y k3s |

**Fix correcto en platform-composition.yaml:**
```yaml
values:
  serviceCIDR: "10.43.0.0/16"   # ← top-level, NO en syncer.extraArgs
  vcluster:
    extraArgs:
    - --tls-san=192.168.0.13
  syncer:
    extraInitContainers: [...]   # cidr-wrapper como seguro adicional
```

**Limpieza requerida tras aplicar el fix:**
```bash
# En Ryzen vía SSH
echo 'ADMIN123' | sudo -S -k sh -c \
  'KUBECONFIG=/etc/rancher/k3s/k3s.yaml /usr/local/bin/kubectl delete statefulset -n default-ns <nombre-vcluster> && \
   KUBECONFIG=/etc/rancher/k3s/k3s.yaml /usr/local/bin/kubectl delete pvc -n default-ns data-<nombre-vcluster>-0'
```

### compositeDeletePolicy no soportado
**Síntoma:** `strict decoding error: unknown field "spec.compositeDeletePolicy"`  
**Fix:** Remover el campo de la Composition (campo de Crossplane >= v1.14, verificar versión instalada).

### NVIDIA Device Plugin — ruta kubelet
**Síntoma:** GPU no aparece como allocatable  
**Causa:** K3s usa `/var/lib/kubelet/device-plugins/` (ruta estándar K8s), no `/var/lib/rancher/k3s/...`  
**Fix:** Montar `/var/lib/kubelet/device-plugins` en el DaemonSet.

### Ollama — CPU en lugar de GPU
**Síntoma:** Ollama no usa CUDA  
**Causa:** Backend ROCm interfiere con detección NVIDIA  
**Fix:** `mv /usr/local/lib/ollama/rocm /usr/local/lib/ollama/rocm.disabled && systemctl restart ollama`

---

## 🔄 Flujo de Trabajo Operacional

### Aplicar cambio en Composition
```bash
# 1. Editar archivo en Mac
# 2. Commit y push
cd /Users/jonnhy/Documents/airgapOps
git add infrastructure/compositions/
git commit -m "fix: descripción del cambio"
git push

# 3. En ThinkPad — forzar sync de ArgoCD si no sincroniza automático
kubectl annotate app <app-name> -n argocd argocd.argoproj.io/refresh=hard --overwrite
# o: argocd app sync <app-name> --force
```

### Limpiar vcluster para reconciliación limpia (en Ryzen)
```bash
# Conectar al Ryzen
ssh admin@192.168.0.13
# Password: ADMIN123

# Borrar StatefulSet y PVC (Crossplane los recreará)
sudo kubectl delete statefulset -n default-ns <nombre-vcluster>
sudo kubectl delete pvc -n default-ns data-<nombre-vcluster>-0
```

### Verificar estado GPU en Ryzen
```bash
nvidia-smi
kubectl get node -o jsonpath='{.items[0].status.allocatable}' | python3 -m json.tool | grep nvidia
journalctl -u ollama -n 10 | grep -i cuda
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
