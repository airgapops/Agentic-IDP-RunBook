# PB-03: Primer Entorno de Desarrollo (Platform)
> **"De un YAML a un cluster completo en minutos"**

Este playbook guía el aprovisionamiento del primer entorno de desarrollo aislado usando el CompositeResource `Platform`. Se demuestra el flujo completo: el operador aplica un CR en el ThinkPad, Crossplane reacciona, y un vcluster completo aparece en el Ryzen listo para ser usado por un equipo de desarrollo.

> ⚠️ **Prerequisito obligatorio:** El PB-02 debe estar completo. Crossplane debe poder crear objetos en el Ryzen (`READY=True`, `SYNCED=True` en el test de conectividad).

---

## 📋 Prerequisitos

| Elemento | Estado requerido |
|---|---|
| **ThinkPad** | K3s running, Crossplane con providers Healthy |
| **Ryzen** | K3s running, accesible desde ThinkPad via kubeconfig |
| **Crossplane ProviderConfigs** | `ryzen-helm-cluster` y `ryzen-compute-cluster` presentes |
| **XRDs y Compositions** | Aplicados en el ThinkPad (via ArgoCD o manualmente) |
| **Repo airgap-ops** | Con la Composition `platform` en `crossplane/compositions/` |

---

## 🏛️ Modelo Mental: Cómo Funciona una Platform

Antes de ejecutar cualquier comando, es crítico entender el flujo de aprovisionamiento:

```
Operador/Developer
      │
      │  kubectl apply platform.yaml
      ▼
┌─────────────────────────────────────┐
│    ThinkPad (Management Cluster)    │
│                                     │
│  Platform CR (kind: Platform)       │
│       │                             │
│       ▼                             │
│  XRC: CompositeResource             │
│  (zerops.internal/v1alpha1)         │
│       │                             │
│       ▼                             │
│  Composition → Crossplane           │
│       │ provider-helm               │
│       │ provider-kubernetes         │
└───────┼─────────────────────────────┘
        │
        │  Helm Release (vcluster chart)
        │  Namespace Object
        ▼
┌─────────────────────────────────────┐
│    Ryzen (Compute Cluster)          │
│                                     │
│  Namespace: <team>-platform         │
│  Pod: vcluster-0 (1/1 Running)      │
│  Service: vcluster (ClusterIP)      │
│                                     │
│  Dentro del vcluster:               │
│  - API Server propio                │
│  - etcd propio                      │
│  - CoreDNS propio                   │
│  - Nodo virtual "synced" del Ryzen  │
└─────────────────────────────────────┘
        │
        │  kubeconfig extraído del Secret
        ▼
   Team Developer
   kubectl --kubeconfig=vcluster.yaml get nodes
```

**Flujo de datos:**
1. `Platform` CR → **XRD valida** el schema
2. XRD → **Composition selecciona** los recursos compuestos
3. Composition → **provider-helm** crea un `Release` (chart de vcluster)
4. Composition → **provider-kubernetes** crea el `Namespace` y `ConfigMap` de contexto
5. Helm Release → **vcluster Pod** en el Ryzen (StatefulSet con 1 réplica)
6. vcluster → expone un **Secret** con el kubeconfig interno
7. Operador extrae el kubeconfig → Developer obtiene acceso

---

## 🚀 The Runbook

Los siguientes comandos se ejecutan en el **ThinkPad** (`192.168.0.10`) salvo indicación contraria.

### Fase 1: Verificar XRDs y Compositions

**1. Confirmar que los XRDs están instalados:**
```bash
kubectl get xrds
# NAME                                ESTABLISHED   OFFERED   AGE
# platforms.zerops.internal           True          True      5m
```

**2. Confirmar que la Composition está registrada:**
```bash
kubectl get compositions
# NAME                 AGE   XR-KIND    XR-APIVERSION
# platform-vcluster    5m    Platform   zerops.internal/v1alpha1
```

Si los XRDs no están presentes, aplicarlos manualmente desde el repo:
```bash
kubectl apply -f ~/airgap-ops/crossplane/xrds/
kubectl apply -f ~/airgap-ops/crossplane/compositions/
```

---

### Fase 2: Crear el Platform CR

**1. Crear el archivo `test-platform.yaml`:**
```yaml
# test-platform.yaml
apiVersion: zerops.internal/v1alpha1
kind: Platform
metadata:
  name: test-platform
  namespace: default
  labels:
    team: platform-engineering
    environment: development
spec:
  # Tamaño del vcluster: small | medium | large
  # small:  1 CPU, 2Gi RAM
  # medium: 2 CPU, 4Gi RAM
  # large:  4 CPU, 8Gi RAM
  size: small
  # Namespace en el Ryzen donde se creará el vcluster
  namespace: test-platform-ns
```

**2. Aplicar el Platform CR:**
```bash
kubectl apply -f test-platform.yaml

# Output esperado:
# platform.zerops.internal/test-platform created
```

---

### Fase 3: Monitorear el Aprovisionamiento

**1. Observar el estado del Platform CR:**
```bash
kubectl get platforms -A -w
# NAMESPACE   NAME            SYNCED   READY   COMPOSITION          AGE
# default     test-platform   True     False   platform-vcluster    10s
# default     test-platform   True     False   platform-vcluster    30s
# default     test-platform   True     True    platform-vcluster    2m
```

> ℹ️ El `READY=True` puede tardar entre 2 y 5 minutos dependiendo de la velocidad de descarga de la imagen de vcluster.

**2. Ver los Composite Resources generados por Crossplane:**
```bash
kubectl get compositeresources
# o abreviado:
kubectl get xplatforms

kubectl describe platform test-platform
# En la sección "Status > Conditions" ver:
# - Synced: True
# - Ready: True
```

**3. Ver los Helm Releases creados en el Ryzen:**
```bash
# Desde el ThinkPad, ver los Releases de Crossplane
kubectl get releases -A
# NAMESPACE            NAME                    CHART      VERSION   SYNCED   READY   AGE
# crossplane-system    test-platform-vcluster  vcluster   0.20.x    True     True    2m
```

**4. Ver el pod del vcluster en el Ryzen (SSH al Ryzen):**
```bash
ssh usuario@192.168.0.13

# Ver el namespace creado
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get ns
# Debe aparecer: test-platform-ns

# Ver el pod del vcluster
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get pods -n test-platform-ns
# NAME          READY   STATUS    RESTARTS   AGE
# vcluster-0    1/1     Running   0          2m
```

---

### Fase 4: Bug Conocido — Init Container `cidr-wrapper` en K3s 1.35+

> ⚠️ **Bug conocido:** En versiones de K3s 1.35 y superiores, el vcluster puede quedar atascado en `Init:0/1` debido al init container `cidr-wrapper`.

**Síntoma:**
```bash
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get pods -n test-platform-ns
# NAME          READY   STATUS     RESTARTS   AGE
# vcluster-0    0/1     Init:0/1   0          5m
```

**Por qué existe el `cidr-wrapper`:**  
El init container `cidr-wrapper` intenta detectar el CIDR del cluster host para configurar el routing de red del vcluster. En K3s 1.35+, la forma en que K3s expone esta información cambió, y el init container no puede leerla correctamente.

**Workaround — Especificar el CIDR explícitamente en la Composition:**
```yaml
# En crossplane/compositions/platform-vcluster.yaml
# Agregar en la sección values del Helm Release:
syncer:
  extraArgs:
    - --tls-san=192.168.0.13
vcluster:
  env:
    - name: K3S_CLUSTER_CIDR
      value: "10.52.0.0/16"   # El CIDR del Ryzen definido en PB-02
    - name: K3S_SERVICE_CIDR
      value: "10.53.0.0/16"
```

**Aplicar el fix y forzar reconciliación:**
```bash
# En el ThinkPad, actualizar la Composition en el repo y push
git add crossplane/compositions/platform-vcluster.yaml
git commit -m "fix: specify K3s CIDR for vcluster init container"
git push origin main

# ArgoCD sincronizará automáticamente. O forzar manualmente:
kubectl -n argocd app sync platform-root

# Eliminar y recrear el Platform CR para que Crossplane reconcilie
kubectl delete platform test-platform
kubectl apply -f test-platform.yaml
```

---

### Fase 5: Extraer el Kubeconfig del vcluster

Una vez que el pod está `1/1 Running`, el vcluster expone su kubeconfig en un Secret de Kubernetes.

**1. Encontrar el Secret con el kubeconfig (desde el ThinkPad):**
```bash
# El secret es creado por Crossplane/vcluster en el namespace del Ryzen
# y luego sincronizado de vuelta al ThinkPad por la Composition

kubectl get secrets -n default | grep test-platform
# test-platform-kubeconfig    Opaque   1   2m
```

**2. Extraer el kubeconfig:**
```bash
kubectl get secret test-platform-kubeconfig -n default \
  -o jsonpath='{.data.value}' | base64 -d > ~/vcluster-test.yaml

chmod 600 ~/vcluster-test.yaml
```

**3. Si el secret está en el Ryzen (alternativa via SSH):**
```bash
ssh usuario@192.168.0.13

kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml \
  get secret vc-test-platform-vcluster -n test-platform-ns \
  -o jsonpath='{.data.config}' | base64 -d > ~/vcluster-test.yaml

# Reemplazar el server interno por la IP del Ryzen
sed -i 's/localhost/192.168.0.13/g' ~/vcluster-test.yaml

# Copiar de vuelta al ThinkPad
scp ~/vcluster-test.yaml usuario@192.168.0.10:~/vcluster-test.yaml
```

**4. Verificar conectividad con el vcluster:**
```bash
# Desde el ThinkPad
kubectl --kubeconfig=~/vcluster-test.yaml get nodes
# NAME    STATUS   ROLES    AGE   VERSION
# ryzen   Ready    <none>   2m    v1.xx.x+k3s1

kubectl --kubeconfig=~/vcluster-test.yaml get pods -A
# NAMESPACE     NAME                       READY   STATUS    RESTARTS   AGE
# kube-system   coredns-xxx-yyy            1/1     Running   0          2m
```

---

### Fase 6: Verificación Completa del Entorno

**1. Verificar nodo virtual Ready:**
```bash
kubectl --kubeconfig=~/vcluster-test.yaml get nodes
# Debe mostrar el nodo virtual (synced desde el Ryzen) en estado Ready
```

**2. Verificar CoreDNS Running:**
```bash
kubectl --kubeconfig=~/vcluster-test.yaml get pods -n kube-system
# NAME                       READY   STATUS    RESTARTS   AGE
# coredns-xxx                1/1     Running   0          3m
```

**3. Verificar resolución DNS interna del vcluster:**
```bash
kubectl --kubeconfig=~/vcluster-test.yaml run test-dns \
  --image=busybox --rm -it --restart=Never -- nslookup kubernetes.default.svc.cluster.local
# Server: 10.53.x.x (Service CIDR del vcluster)
# Address: 10.53.x.x#53
# Name: kubernetes.default.svc.cluster.local
# Address: 10.53.0.1
```

---

## 🔧 Troubleshooting

### Problema 1: Namespace Terminating (no se puede eliminar)

**Síntoma:**
```bash
kubectl get ns test-platform-ns
# NAME               STATUS        AGE
# test-platform-ns   Terminating   10m
```

**Causa:** Quedan finalizers en recursos dentro del namespace que ya no pueden ser procesados.

**Solución — Force Finalize:**
```bash
# En el Ryzen
NAMESPACE="test-platform-ns"

# 1. Obtener el JSON del namespace
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get namespace $NAMESPACE -o json > /tmp/ns.json

# 2. Eliminar los finalizers
cat /tmp/ns.json | python3 -c "
import sys, json
data = json.load(sys.stdin)
data['spec']['finalizers'] = []
print(json.dumps(data))
" > /tmp/ns-no-finalizers.json

# 3. Aplicar via la subresource finalize
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml replace \
  --raw "/api/v1/namespaces/$NAMESPACE/finalize" \
  -f /tmp/ns-no-finalizers.json
```

---

### Problema 2: APIService Stale — `metrics.k8s.io` no disponible

**Síntoma:**
```bash
kubectl --kubeconfig=~/vcluster-test.yaml top nodes
# Error from server (ServiceUnavailable): the server is currently unable to handle the request (get nodes.metrics.k8s.io)

kubectl get apiservices | grep metrics
# v1beta1.metrics.k8s.io   False (ServiceUnavailable)   10m
```

**Causa:** El vcluster sincroniza el APIService `metrics.k8s.io` desde el cluster host (Ryzen), pero el metrics-server no está instalado en el Ryzen, dejando el APIService registrado pero no funcional.

**Solución A — Eliminar el APIService stale en el vcluster:**
```bash
kubectl --kubeconfig=~/vcluster-test.yaml delete apiservice v1beta1.metrics.k8s.io
```

**Solución B — Instalar metrics-server en el vcluster:**
```bash
kubectl --kubeconfig=~/vcluster-test.yaml apply -f \
  https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

# Parchear para deshabilitar verificación TLS (común en entornos self-signed)
kubectl --kubeconfig=~/vcluster-test.yaml patch deployment metrics-server \
  -n kube-system \
  --type='json' \
  -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}]'
```

---

### Problema 3: Platform CR atascado en `READY=False`

**Diagnóstico:**
```bash
kubectl describe platform test-platform
# Buscar en "Status > Conditions" el mensaje de error

kubectl -n crossplane-system logs -l app=crossplane --tail=50
# Ver logs del controller de Crossplane

kubectl get releases -A
# Ver si el Release está en error
kubectl describe release test-platform-vcluster -n crossplane-system
```

---

## ✅ Checklist de Validación (Exit Criteria)

- [ ] `kubectl get platforms -A` muestra `SYNCED=True` y `READY=True`
- [ ] `kubectl get releases -A` muestra el Release del vcluster con `READY=True`
- [ ] El pod `vcluster-0` está `1/1 Running` en el Ryzen (namespace `test-platform-ns`)
- [ ] `kubectl --kubeconfig=~/vcluster-test.yaml get nodes` retorna el nodo virtual en `Ready`
- [ ] `kubectl --kubeconfig=~/vcluster-test.yaml get pods -n kube-system` muestra CoreDNS `Running`
- [ ] El test de DNS interno del vcluster resuelve `kubernetes.default.svc.cluster.local`

---

## 🔗 Siguiente Paso

→ **[PB-04: Developer Workflow](./PB-04-developer-workflow.md)** — Flujo de trabajo para que los equipos de desarrollo soliciten y usen entornos Platform a través del Developer Portal.

---

*Parte del [Zerops Sovereign IDP](../../README.md) — Air-gapped Internal Developer Platform*
