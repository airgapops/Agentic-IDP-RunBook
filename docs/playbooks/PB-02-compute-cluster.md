# PB-02: Compute Cluster (Ryzen)
> **"El músculo donde viven los entornos de desarrollo"**

Este playbook convierte el Ryzen en el Compute Cluster de la plataforma Zerops. Aquí se aprovisionan los vclusters (entornos de desarrollo aislados) que los equipos solicitan vía Crossplane desde el ThinkPad. El Ryzen no gestiona nada por sí mismo — recibe órdenes del Management Cluster.

> ⚠️ **Prerequisito obligatorio:** El PB-01 debe estar completo. El ThinkPad (`192.168.0.10`) debe tener K3s, Crossplane y los providers instalados.

---

## 📋 Prerequisitos de Hardware y OS

| Elemento | Especificación |
|---|---|
| **Hardware** | Ryzen (o cualquier x86_64 con ≥ 32 GB RAM, ≥ 8 CPU cores recomendado) |
| **OS** | Ubuntu Server 22.04 LTS — Arquitectura x86_64 |
| **IP** | Estática: `192.168.0.13` |
| **Puerto de acceso** | SSH habilitado |
| **Dominio Base** | `zerops.internal` |

### Hardware de referencia en este IDP
| Nodo | IP | Rol |
|---|---|---|
| Raspberry Pi 4 | `192.168.0.9` | DNS + PKI (PB-00) ✅ |
| ThinkPad | `192.168.0.10` | Management Cluster (PB-01) ✅ |
| Ryzen | `192.168.0.13` | Compute Node (este PB) |

---

## 🏛️ ADR — Architecture Decision Records

### ADR-1: K3s standalone (no agente del ThinkPad)
**Decisión:** El Ryzen corre su propia instancia de K3s independiente, no como worker del ThinkPad.  
**Rationale:** Crossplane gestiona la federación de clusters vía kubeconfig, no via K3s agent join. Esto permite que el Ryzen sea un cluster completamente independiente con su propio plano de control, adecuado para alojar vclusters con aislamiento fuerte.

### ADR-2: Kubeconfig con IP/hostname explícito
**Decisión:** El kubeconfig generado para Crossplane usa la IP `192.168.0.13` como server, no `127.0.0.1`.  
**Rationale:** El ThinkPad necesita alcanzar la API del Ryzen desde la red local. El kubeconfig por defecto de K3s apunta a `127.0.0.1:6443` y no es usable remotamente sin modificación.

### ADR-3: No exponer la API del Ryzen públicamente
**Decisión:** El puerto 6443 del Ryzen solo es accesible desde la red local (`192.168.0.0/24`).  
**Rationale:** El Ryzen es exclusivamente un nodo de cómputo. Solo el ThinkPad (Crossplane) necesita acceso a su API. No hay motivo para exponerlo más allá de la red local.

---

## 🚀 The Runbook

### Fase 1: Preparación del OS y Red

**1. Verificar IP estática y conectividad:**
```bash
ip addr show | grep 192.168.0.13
ping -c 3 192.168.0.9   # Raspberry Pi (DNS)
ping -c 3 192.168.0.10    # ThinkPad (Management)
```

**2. Configurar DNS apuntando al Raspberry Pi:**
```bash
sudo mkdir -p /etc/systemd/resolved.conf.d
sudo tee /etc/systemd/resolved.conf.d/zerops-dns.conf <<EOF
[Resolve]
DNS=192.168.0.9
Domains=zerops.internal
DNSStubListener=no
EOF

sudo systemctl restart systemd-resolved
```

**3. Verificar resolución DNS:**
```bash
dig @192.168.0.9 step-ca.zerops.internal
# Debe retornar 192.168.0.9
```

---

### Fase 2: Inyección del Certificado Raíz de Step-CA

**1. Copiar el certificado raíz desde el Raspberry Pi:**
```bash
scp pi@192.168.0.9:~/zerops-network/root_ca.crt ~/root_ca.crt
```

**2. Inyectar en el trust store del OS:**
```bash
sudo cp ~/root_ca.crt /usr/local/share/ca-certificates/zerops-root-ca.crt
sudo update-ca-certificates
# Debe mostrar: "1 added"
```

**3. Verificar:**
```bash
curl https://step-ca.zerops.internal:9000/health
# {"status":"ok"}
```

---

### Fase 3: Instalación de K3s en el Ryzen

**1. Instalar K3s como servidor standalone (sin Traefik):**
```bash
curl -sfL https://get.k3s.io | sh -s - \
  --disable=traefik \
  --node-ip=192.168.0.13 \
  --cluster-cidr=10.52.0.0/16 \
  --service-cidr=10.53.0.0/16 \
  --tls-san=192.168.0.13 \
  --tls-san=ryzen.zerops.internal
```

> ℹ️ **Nota:** Se usan rangos CIDR distintos al ThinkPad (`10.42/10.43`) para evitar colisiones si en el futuro se configura una VPN entre clusters.

**2. Verificar que K3s está corriendo:**
```bash
sudo systemctl status k3s
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get nodes
# NAME    STATUS   ROLES                  AGE   VERSION
# ryzen   Ready    control-plane,master   1m    v1.xx.x+k3s1
```

---

### Fase 4: Generar Kubeconfig para el ThinkPad

El kubeconfig por defecto de K3s usa `127.0.0.1` como server. Debemos generar uno con la IP real del Ryzen para que Crossplane (en el ThinkPad) pueda conectarse.

**1. Extraer y modificar el kubeconfig:**
```bash
sudo cat /etc/rancher/k3s/k3s.yaml \
  | sed 's/127.0.0.1/192.168.0.13/g' \
  > ~/ryzen-kubeconfig.yaml

chmod 600 ~/ryzen-kubeconfig.yaml
```

**2. Verificar conectividad local con el kubeconfig modificado:**
```bash
kubectl --kubeconfig=~/ryzen-kubeconfig.yaml get nodes
# Debe retornar el nodo ryzen en Ready
```

**3. Copiar el kubeconfig al ThinkPad:**
```bash
scp ~/ryzen-kubeconfig.yaml usuario@192.168.0.10:~/ryzen-kubeconfig.yaml
```

---

### Fase 5: Crear el Secret en el ThinkPad

Los siguientes comandos se ejecutan **en el ThinkPad** (`192.168.0.10`).

**1. SSH al ThinkPad:**
```bash
ssh usuario@192.168.0.10
```

**2. Crear el secret con el kubeconfig del Ryzen:**
```bash
kubectl -n crossplane-system create secret generic ryzen-kubeconfig \
  --from-file=kubeconfig=~/ryzen-kubeconfig.yaml

# Verificar
kubectl -n crossplane-system get secret ryzen-kubeconfig
# NAME               TYPE     DATA   AGE
# ryzen-kubeconfig   Opaque   1      5s
```

---

### Fase 6: Crear los ProviderConfig en el ThinkPad

Los siguientes comandos se ejecutan **en el ThinkPad**.

**1. Crear ProviderConfig para provider-helm (desplegar Helm releases en el Ryzen):**
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
EOF
```

**2. Crear ProviderConfig para provider-kubernetes (crear objetos K8s en el Ryzen):**
```bash
kubectl apply -f - <<EOF
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

**3. Verificar que los ProviderConfigs están registrados:**
```bash
kubectl get providerconfigs
# NAME                    AGE   CONFIGREF   READY
# ryzen-helm-cluster      30s
# ryzen-compute-cluster   30s
```

---

### Fase 7: Verificar Conectividad Crossplane → Ryzen

Este es el test de aceptación más importante del PB-02: confirmar que Crossplane puede crear recursos en el Ryzen.

**1. Crear un Namespace de prueba en el Ryzen via Crossplane (desde el ThinkPad):**
```bash
kubectl apply -f - <<EOF
apiVersion: kubernetes.crossplane.io/v1alpha1
kind: Object
metadata:
  name: test-namespace-ryzen
spec:
  providerConfigRef:
    name: ryzen-compute-cluster
  forProvider:
    manifest:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: crossplane-connectivity-test
EOF
```

**2. Verificar que el objeto fue creado:**
```bash
# En el ThinkPad: ver si el Object está synced
kubectl get objects test-namespace-ryzen
# NAME                     READY   SYNCED   AGE
# test-namespace-ryzen     True    True     30s

# En el Ryzen (SSH): confirmar que el namespace existe
ssh usuario@192.168.0.13 "kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get ns crossplane-connectivity-test"
# NAME                         STATUS   AGE
# crossplane-connectivity-test Active   30s
```

**3. Limpiar el objeto de prueba:**
```bash
kubectl delete object test-namespace-ryzen
```

---

### Fase 8: Registrar Ryzen en DNS

**Desde cualquier nodo, registrar el hostname del Ryzen en PowerDNS:**
```bash
curl -X PATCH -H "X-API-Key: <POWERDNS_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"rrsets": [{"name": "ryzen.zerops.internal.", "type": "A", "ttl": 300, "changetype": "REPLACE", "records": [{"content": "192.168.0.13", "disabled": false}]}]}' \
  http://192.168.0.9:8081/api/v1/servers/localhost/zones/zerops.internal.
```

---

## ✅ Checklist de Validación (Exit Criteria)

- [ ] `kubectl --kubeconfig=~/ryzen-kubeconfig.yaml get nodes` retorna `Ready` desde el ThinkPad
- [ ] `kubectl -n crossplane-system get secret ryzen-kubeconfig` existe en el ThinkPad
- [ ] `kubectl get providerconfigs` muestra `ryzen-helm-cluster` y `ryzen-compute-cluster`
- [ ] El Object de prueba `test-namespace-ryzen` mostró `READY=True` y `SYNCED=True`
- [ ] El namespace `crossplane-connectivity-test` apareció en el Ryzen
- [ ] Crossplane puede crear `Release` (Helm) y `Object` (K8s) en el Ryzen sin errores

---

## 🔗 Siguiente Paso

→ **[PB-03: Primer Entorno de Desarrollo (Platform)](./PB-03-first-platform.md)** — Aprovisionar el primer vcluster usando el CompositeResource `Platform` y verificar el ciclo de vida completo.

---

*Parte del [Zerops Sovereign IDP](../../README.md) — Air-gapped Internal Developer Platform*
