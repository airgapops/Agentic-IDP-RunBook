# PB-01: Management Cluster (ThinkPad)
> **"El plano de control de toda la plataforma"**

Este playbook convierte el ThinkPad en el Management Cluster de la plataforma Zerops Sovereign IDP. Aquí viven Crossplane, ArgoCD, cert-manager, Envoy Gateway, Gitea y Backstage. Este nodo es el cerebro del sistema: emite órdenes al Ryzen (PB-02) y expone el Developer Portal a los equipos.

> ⚠️ **Prerequisito obligatorio:** El PB-00 debe estar completo. El Raspberry Pi (`192.168.0.200`) debe estar respondiendo en el puerto 53 (DNS) y el archivo `root_ca.crt` debe estar disponible localmente.

---

## 📋 Prerequisitos de Hardware y OS

| Elemento | Especificación |
|---|---|
| **Hardware** | ThinkPad (o cualquier x86_64 con ≥ 16 GB RAM, ≥ 4 CPU cores) |
| **OS** | Ubuntu Server 22.04 LTS — Arquitectura x86_64 |
| **IP** | Estática: `192.168.0.10` |
| **Puerto de acceso** | SSH habilitado |
| **Dominio Base** | `zerops.internal` |

### Hardware de referencia en este IDP
| Nodo | IP | Rol |
|---|---|---|
| Raspberry Pi 4 | `192.168.0.200` | DNS + PKI (PB-00) ✅ |
| ThinkPad | `192.168.0.10` | Management Cluster (este PB) |
| Ryzen | `192.168.0.13` | Compute Node (PB-02) |

---

## 🏛️ ADR — Architecture Decision Records

### ADR-1: K3s en lugar de kubeadm o Kind
**Decisión:** Usar K3s como distribución de Kubernetes ligera.  
**Rationale:** K3s integra containerd, kube-proxy y los componentes del plano de control en un único binario. Ideal para hardware de laboratorio. Soporta kubeconfig multi-cluster y actualización in-place.

### ADR-2: Deshabilitar Traefik (usamos Envoy Gateway)
**Decisión:** Instalar K3s con `--disable=traefik`.  
**Rationale:** Envoy Gateway (basado en Envoy Proxy) es la implementación de referencia de la Gateway API de Kubernetes. Ofrece soporte nativo de HTTP/2, gRPC y L7 routing avanzado sin duplicar el ingress controller.

### ADR-3: cert-manager con ACME privado (Step-CA)
**Decisión:** cert-manager obtiene certificados TLS de Step-CA vía ACME.  
**Rationale:** Automatiza la rotación de certificados para todos los servicios internos. Compatible con los dominios `.internal` no resolubles públicamente. Zero-touch una vez configurado.

### ADR-4: Gitea con SQLite (no PostgreSQL)
**Decisión:** Gitea en modo SQLite para el repositorio de infraestructura.  
**Rationale:** El repositorio `airgap-ops` es el único repositorio crítico de plataforma. SQLite es suficiente para este caso de uso. Elimina la dependencia de una BD externa y simplifica el backup (un solo archivo).

### ADR-5: ArgoCD App-of-Apps pattern
**Decisión:** Un único Application raíz en ArgoCD que apunta a `apps/` en el repo.  
**Rationale:** Permite gestionar el ciclo de vida de todas las aplicaciones de plataforma desde Git. Un solo `kubectl apply` inicial despliega toda la plataforma.

---

## 🚀 The Runbook

### Fase 1: Preparación del OS y Red

**1. Verificar IP estática y conectividad al Raspberry Pi:**
```bash
ip addr show | grep 192.168.0.10
ping -c 3 192.168.0.200
```

**2. Configurar DNS apuntando al Raspberry Pi:**
```bash
sudo mkdir -p /etc/systemd/resolved.conf.d
sudo tee /etc/systemd/resolved.conf.d/zerops-dns.conf <<EOF
[Resolve]
DNS=192.168.0.200
Domains=zerops.internal
DNSStubListener=no
EOF

sudo systemctl restart systemd-resolved
```

**3. Verificar resolución DNS:**
```bash
dig @192.168.0.200 step-ca.zerops.internal
# Debe retornar 192.168.0.200
```

---

### Fase 2: Inyección del Certificado Raíz de Step-CA

El archivo `root_ca.crt` debe ser copiado desde el Raspberry Pi.

**1. Copiar el certificado raíz:**
```bash
# Opción A: desde el Raspberry Pi directamente
scp pi@192.168.0.200:~/zerops-network/root_ca.crt ~/root_ca.crt

# Opción B: si se tiene acceso al contenedor
# (ejecutar en el Raspberry Pi y copiar el output)
# podman exec -it step-ca cat /home/step/certs/root_ca.crt > root_ca.crt
```

**2. Inyectar el certificado en el trust store del OS (Ubuntu):**
```bash
sudo cp ~/root_ca.crt /usr/local/share/ca-certificates/zerops-root-ca.crt
sudo update-ca-certificates
# Debe mostrar: "1 added"
```

**3. Verificar que curl confía en el certificado:**
```bash
curl https://step-ca.zerops.internal:9000/health
# Debe retornar: {"status":"ok"}
```

---

### Fase 3: Instalación de K3s

**1. Instalar K3s deshabilitando Traefik:**
```bash
curl -sfL https://get.k3s.io | sh -s - \
  --disable=traefik \
  --node-ip=192.168.0.10 \
  --cluster-cidr=10.42.0.0/16 \
  --service-cidr=10.43.0.0/16 \
  --tls-san=192.168.0.10 \
  --tls-san=k3s.zerops.internal
```

**2. Configurar kubectl para el usuario:**
```bash
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown $USER:$USER ~/.kube/config
chmod 600 ~/.kube/config

# Verificar
kubectl get nodes
# NAME       STATUS   ROLES                  AGE   VERSION
# thinkpad   Ready    control-plane,master   1m    v1.xx.x+k3s1
```

**3. Inyectar el certificado raíz en K3s/containerd:**
```bash
# K3s usa containerd. Inyectar el cert en su trust store
sudo mkdir -p /var/lib/rancher/k3s/agent/etc/containerd/certs.d/step-ca.zerops.internal:9000
sudo tee /var/lib/rancher/k3s/agent/etc/containerd/certs.d/step-ca.zerops.internal:9000/hosts.toml <<EOF
server = "https://step-ca.zerops.internal:9000"

[host."https://step-ca.zerops.internal:9000"]
  ca = "/usr/local/share/ca-certificates/zerops-root-ca.crt"
EOF

sudo systemctl restart k3s
```

---

### Fase 4: Instalar cert-manager

**1. Agregar el repo Helm y crear el namespace:**
```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update

kubectl create namespace cert-manager
```

**2. Instalar cert-manager con CRDs:**
```bash
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --version v1.14.5 \
  --set installCRDs=true \
  --set global.leaderElection.namespace=cert-manager

# Esperar a que los pods estén Ready
kubectl -n cert-manager rollout status deployment/cert-manager
kubectl -n cert-manager rollout status deployment/cert-manager-webhook
```

**3. Crear el Secret con el certificado raíz (para que cert-manager confíe en Step-CA):**
```bash
kubectl -n cert-manager create secret generic step-ca-root \
  --from-file=ca.crt=~/root_ca.crt
```

**4. Obtener el fingerprint de Step-CA:**
```bash
# Ejecutar en el Raspberry Pi y copiar el output
# podman exec -it step-ca step certificate fingerprint /home/step/certs/root_ca.crt
STEP_CA_FINGERPRINT="<output-del-comando-anterior>"
```

**5. Crear el ClusterIssuer apuntando a Step-CA ACME:**
```bash
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: step-ca-issuer
spec:
  acme:
    server: https://step-ca.zerops.internal:9000/acme/acme/directory
    email: admin@zerops.internal
    privateKeySecretRef:
      name: step-ca-acme-account-key
    caBundle: $(base64 -w0 ~/root_ca.crt)
    solvers:
    - http01:
        ingress:
          class: envoy
EOF
```

**6. Verificar el ClusterIssuer:**
```bash
kubectl get clusterissuers
# NAME             READY   AGE
# step-ca-issuer   True    30s
```

---

### Fase 5: Instalar Envoy Gateway

**1. Agregar repo y crear namespace:**
```bash
helm repo add envoy-gateway https://charts.envoyproxy.io
helm repo update

kubectl create namespace envoy-gateway-system
```

**2. Instalar Envoy Gateway:**
```bash
helm install eg envoy-gateway/gateway \
  --namespace envoy-gateway-system \
  --version v1.1.2

kubectl -n envoy-gateway-system rollout status deployment/envoy-gateway
```

**3. Crear la GatewayClass:**
```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: zerops-gateway
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
EOF
```

**4. Crear el Gateway principal:**
```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: zerops-main
  namespace: envoy-gateway-system
spec:
  gatewayClassName: zerops-gateway
  listeners:
  - name: https
    port: 443
    protocol: HTTPS
    tls:
      mode: Terminate
      certificateRefs:
      - kind: Secret
        name: zerops-wildcard-tls
  - name: http
    port: 80
    protocol: HTTP
EOF
```

---

### Fase 6: Instalar Gitea

**1. Crear namespace y configurar Gitea con SQLite:**
```bash
helm repo add gitea-charts https://dl.gitea.com/charts/
helm repo update

kubectl create namespace gitea
```

**2. Instalar Gitea:**
```bash
helm install gitea gitea-charts/gitea \
  --namespace gitea \
  --set gitea.admin.username=jadamson \
  --set gitea.admin.password=ChangeThisPassword123! \
  --set gitea.admin.email=jadamson@zerops.internal \
  --set persistence.size=10Gi \
  --set postgresql.enabled=false \
  --set postgresql-ha.enabled=false \
  --set redis-cluster.enabled=false \
  --set gitea.config.database.DB_TYPE=sqlite3 \
  --set gitea.config.server.DOMAIN=gitea.zerops.internal \
  --set gitea.config.server.ROOT_URL=https://gitea.zerops.internal

kubectl -n gitea rollout status deployment/gitea
```

**3. Crear el HTTPRoute para exponer Gitea:**
```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: gitea
  namespace: gitea
spec:
  parentRefs:
  - name: zerops-main
    namespace: envoy-gateway-system
  hostnames:
  - "gitea.zerops.internal"
  rules:
  - backendRefs:
    - name: gitea-http
      port: 3000
EOF
```

**4. Registrar DNS para Gitea en PowerDNS:**
```bash
curl -X PATCH -H "X-API-Key: super-secret-api-key-zerops" \
  -H "Content-Type: application/json" \
  -d '{"rrsets": [{"name": "gitea.zerops.internal.", "type": "A", "ttl": 300, "changetype": "REPLACE", "records": [{"content": "192.168.0.10", "disabled": false}]}]}' \
  http://192.168.0.200:8081/api/v1/servers/localhost/zones/zerops.internal.
```

**5. Crear el repositorio airgap-ops en Gitea:**
```bash
# Una vez que Gitea esté accesible en https://gitea.zerops.internal
curl -k -X POST https://gitea.zerops.internal/api/v1/user/repos \
  -H "Content-Type: application/json" \
  -u "jadamson:ChangeThisPassword123!" \
  -d '{"name": "airgap-ops", "description": "Zerops Sovereign IDP - Infrastructure as Code", "private": false, "auto_init": true}'

# Push del repo local
cd ~/airgap-ops
git remote add origin https://gitea.zerops.internal/jadamson/airgap-ops.git
git push -u origin main
```

---

### Fase 7: Instalar ArgoCD

**1. Crear namespace e instalar ArgoCD:**
```bash
kubectl create namespace argocd

kubectl apply -n argocd -f \
  https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

kubectl -n argocd rollout status deployment/argocd-server
```

**2. Configurar ArgoCD para confiar en el certificado de Gitea (Step-CA):**
```bash
kubectl -n argocd create configmap argocd-tls-certs-cm \
  --from-file=gitea.zerops.internal=~/root_ca.crt \
  --dry-run=client -o yaml | kubectl apply -f -
```

**3. Obtener la contraseña inicial de admin:**
```bash
kubectl -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath="{.data.password}" | base64 -d && echo
```

**4. Exponer ArgoCD vía HTTPRoute:**
```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: argocd
  namespace: argocd
spec:
  parentRefs:
  - name: zerops-main
    namespace: envoy-gateway-system
  hostnames:
  - "argocd.zerops.internal"
  rules:
  - backendRefs:
    - name: argocd-server
      port: 80
EOF
```

**5. Registrar DNS para ArgoCD:**
```bash
curl -X PATCH -H "X-API-Key: super-secret-api-key-zerops" \
  -H "Content-Type: application/json" \
  -d '{"rrsets": [{"name": "argocd.zerops.internal.", "type": "A", "ttl": 300, "changetype": "REPLACE", "records": [{"content": "192.168.0.10", "disabled": false}]}]}' \
  http://192.168.0.200:8081/api/v1/servers/localhost/zones/zerops.internal.
```

**6. Crear el App-of-Apps raíz:**
```bash
kubectl apply -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: platform-root
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://gitea.zerops.internal/jadamson/airgap-ops.git
    targetRevision: HEAD
    path: apps/
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
EOF
```

---

### Fase 8: Instalar Crossplane

**1. Agregar repo e instalar Crossplane:**
```bash
helm repo add crossplane-stable https://charts.crossplane.io/stable
helm repo update

kubectl create namespace crossplane-system

helm install crossplane crossplane-stable/crossplane \
  --namespace crossplane-system \
  --version 1.15.0 \
  --set args='{--enable-composition-revisions}'

kubectl -n crossplane-system rollout status deployment/crossplane
```

**2. Instalar provider-helm y provider-kubernetes:**
```bash
kubectl apply -f - <<EOF
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-helm
spec:
  package: xpkg.upbound.io/crossplane-contrib/provider-helm:v0.18.1
---
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-kubernetes
spec:
  package: xpkg.upbound.io/crossplane-contrib/provider-kubernetes:v0.13.0
EOF

# Esperar a que los providers estén Healthy
kubectl get providers -w
# NAME                  INSTALLED   HEALTHY   PACKAGE                            AGE
# provider-helm         True        True      ...                                2m
# provider-kubernetes   True        True      ...                                2m
```

**3. Crear el Secret con el kubeconfig del Ryzen:**
```bash
# El kubeconfig del Ryzen se obtiene en el PB-02.
# Una vez disponible, crear el secret:
kubectl -n crossplane-system create secret generic ryzen-kubeconfig \
  --from-file=kubeconfig=/path/to/ryzen-kubeconfig.yaml
```

**4. Crear los ProviderConfig para el Ryzen:**
```bash
kubectl apply -f - <<EOF
apiVersion: helm.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: ryzen-helm-cluster
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: ryzen-kubeconfig
      key: kubeconfig
---
apiVersion: kubernetes.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: ryzen-compute-cluster
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: ryzen-kubeconfig
      key: kubeconfig
EOF
```

**5. Aplicar XRDs y Compositions (vía ArgoCD o directamente):**
```bash
# Los XRDs y Compositions viven en el repo airgap-ops
# ArgoCD los aplicará automáticamente. Para aplicarlos manualmente:
kubectl apply -f ~/airgap-ops/crossplane/xrds/
kubectl apply -f ~/airgap-ops/crossplane/compositions/
```

---

## ✅ Checklist de Validación (Exit Criteria)

- [ ] `kubectl get nodes` muestra el ThinkPad en estado `Ready`
- [ ] `kubectl get clusterissuers` muestra `step-ca-issuer` con `READY=True`
- [ ] `curl https://gitea.zerops.internal` responde sin errores TLS
- [ ] `curl https://argocd.zerops.internal` responde con la UI de ArgoCD
- [ ] `kubectl -n argocd get apps` muestra `platform-root` en `Synced`
- [ ] `kubectl -n crossplane-system get providers` muestra ambos providers `HEALTHY=True`
- [ ] ArgoCD muestra ≥ 5 apps en estado `Synced+Healthy`
- [ ] `https://choreo.zerops.internal` responde (requiere completar Backstage, ver PB-03)

---

## 🔗 Siguiente Paso

→ **[PB-02: Compute Cluster (Ryzen)](./PB-02-compute-cluster.md)** — Levantar el nodo Ryzen, registrarlo como destino de Crossplane y prepararlo para alojar vclusters.

---

*Parte del [Zerops Sovereign IDP](../../README.md) — Air-gapped Internal Developer Platform*
