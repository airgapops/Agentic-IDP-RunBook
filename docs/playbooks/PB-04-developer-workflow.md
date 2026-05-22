# PB-04: Developer Workflow
> **"De la petición al cluster en 3 minutos"**

Este playbook describe el flujo de trabajo del día a día para los equipos de desarrollo que usan la plataforma Zerops Sovereign IDP. Los desarrolladores interactúan con el Developer Portal (Backstage) o directamente con kubectl para solicitar entornos de desarrollo aislados, desplegar aplicaciones, y gestionar el ciclo de vida completo de sus Platforms.

> ⚠️ **Prerequisito:** Los PB-01, PB-02 y PB-03 deben estar completos. La plataforma base debe estar operativa.

---

## 📋 Acceso Inicial — Lo Que Necesitas

| Recurso | URL | Credenciales |
|---|---|---|
| **Developer Portal (Backstage)** | https://choreo.zerops.internal | SSO / cuenta Gitea |
| **ArgoCD** | https://argocd.zerops.internal | admin / (ver ThinkPad) |
| **Gitea** | https://gitea.zerops.internal | tu usuario |
| **ThinkPad (kubectl)** | SSH `192.168.0.10` | tu cuenta de OS |

### Configurar tu entorno local
```bash
# 1. Agregar el certificado raíz de la plataforma a tu máquina local
sudo cp zerops-root-ca.crt /usr/local/share/ca-certificates/
sudo update-ca-certificates

# 2. Agregar entradas DNS en tu /etc/hosts (si no tienes DNS corporativo)
echo "192.168.0.10  choreo.zerops.internal" | sudo tee -a /etc/hosts
echo "192.168.0.10  argocd.zerops.internal" | sudo tee -a /etc/hosts
echo "192.168.0.10  gitea.zerops.internal" | sudo tee -a /etc/hosts

# 3. Obtener acceso kubectl al ThinkPad
ssh usuario@192.168.0.10
```

---

## 🏛️ ADR — Architecture Decision Records

### ADR-1: Developer Portal como punto de entrada único
**Decisión:** Backstage en `choreo.zerops.internal` es la interfaz principal para developers.  
**Rationale:** Abstrae la complejidad de Crossplane, Kubernetes y GitOps. El developer no necesita entender la infraestructura subyacente — solo pide un entorno con un nombre y un tamaño.

### ADR-2: Kubectl como interfaz de poder
**Decisión:** Los desarrolladores avanzados y los operadores de plataforma usan kubectl directamente.  
**Rationale:** El Developer Portal es una conveniencia, no una restricción. El acceso directo permite debugging, automatización en CI/CD y flujos de trabajo más ágiles.

### ADR-3: Un vcluster por equipo/proyecto
**Decisión:** Cada Platform CR crea un vcluster aislado para un equipo o proyecto.  
**Rationale:** Aislamiento fuerte de red, RBAC y recursos. Un equipo no puede afectar a otro. El ciclo de vida (crear/escalar/eliminar) es independiente.

---

## 🚀 The Runbook

### Fase 1: Acceder al Developer Portal

**1. Navegar a Backstage:**
```
https://choreo.zerops.internal
```

**2. Iniciar sesión con tu cuenta de Gitea:**
- Click en "Sign in with Gitea"
- Ingresar usuario y contraseña de `gitea.zerops.internal`
- Autorizar la aplicación Backstage

**3. Explorar el Service Catalog:**
- Click en "Catalog" en el menú lateral
- Ver los componentes registrados:
  - `zerops-platform` — La plataforma IDP principal
  - `crossplane-core` — Motor de aprovisionamiento
  - `vcluster-template` — Template de entorno de desarrollo
  - Otros componentes de infraestructura

**4. Ver la documentación técnica:**
- En el Catalog, click en un componente → pestaña "Docs"
- Los playbooks de esta misma serie están indexados aquí

---

### Fase 2: Crear un Nuevo Entorno (Platform)

#### Opción A: Via Developer Portal (Backstage)

**1. Ir a "Create" en el menú lateral**

**2. Seleccionar el template "Platform Environment"**

**3. Completar el formulario:**
   - **Nombre del equipo:** `mi-equipo-dev`
   - **Tamaño:** `small` / `medium` / `large`
   - **Namespace objetivo:** `mi-equipo-apps` (se creará en el Ryzen)

**4. Click "Create" → Backstage genera y hace commit del Platform CR al repo**

**5. ArgoCD detecta el commit y aplica el Platform CR automáticamente**

#### Opción B: Via kubectl (directo)

Crear el archivo del Platform CR y aplicarlo directamente:

```yaml
# mi-equipo-platform.yaml
apiVersion: zerops.internal/v1alpha1
kind: Platform
metadata:
  name: mi-equipo-dev
  namespace: default
  labels:
    team: mi-equipo
    environment: development
    owner: mi-usuario
spec:
  # Tamaños disponibles:
  # small:  1 vCPU, 2Gi RAM  — Ideal para 1-2 desarrolladores
  # medium: 2 vCPU, 4Gi RAM  — Ideal para equipos de 3-5
  # large:  4 vCPU, 8Gi RAM  — Proyectos con múltiples servicios
  size: small
  namespace: mi-equipo-apps
```

```bash
# Aplicar el Platform CR (en el ThinkPad)
kubectl apply -f mi-equipo-platform.yaml

# Output esperado:
# platform.zerops.internal/mi-equipo-dev created
```

---

### Fase 3: Monitorear el Aprovisionamiento

**1. Observar en tiempo real desde el ThinkPad:**
```bash
kubectl get platforms -w
# NAMESPACE   NAME           SYNCED   READY   COMPOSITION          AGE
# default     mi-equipo-dev  True     False   platform-vcluster    15s
# default     mi-equipo-dev  True     False   platform-vcluster    45s
# default     mi-equipo-dev  True     True    platform-vcluster    2m30s
```

**2. Ver el progreso detallado:**
```bash
kubectl describe platform mi-equipo-dev
# Buscar la sección:
# Status:
#   Conditions:
#     Type:    Synced
#     Status:  True
#     Type:    Ready
#     Status:  True  ← cuando aparezca esto, el entorno está listo
```

**3. Ver los recursos creados en el Ryzen:**
```bash
ssh usuario@192.168.0.13

kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get ns
# Debe aparecer: mi-equipo-apps

kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get pods -n mi-equipo-apps
# NAME                  READY   STATUS    RESTARTS   AGE
# vcluster-0            1/1     Running   0          2m
```

**4. Ver en ArgoCD (opcional):**
```
https://argocd.zerops.internal
```
- Buscar la Application `platform-root` → ver los recursos sincronizados
- El Platform CR aparecerá como un recurso gestionado

---

### Fase 4: Obtener Acceso al vcluster

**1. Extraer el kubeconfig del Secret generado:**
```bash
# En el ThinkPad
kubectl get secret mi-equipo-dev-kubeconfig -n default \
  -o jsonpath='{.data.value}' | base64 -d > ~/mi-equipo-vcluster.yaml

chmod 600 ~/mi-equipo-vcluster.yaml
```

**2. Si el secret no está disponible en el ThinkPad, buscarlo en el Ryzen:**
```bash
ssh usuario@192.168.0.13

# Listar secrets en el namespace del vcluster
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml \
  get secrets -n mi-equipo-apps | grep vc-

# Extraer el kubeconfig
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml \
  get secret vc-mi-equipo-dev-vcluster -n mi-equipo-apps \
  -o jsonpath='{.data.config}' | base64 -d > ~/mi-equipo-vcluster.yaml

# Ajustar el server URL (localhost → IP del Ryzen)
sed -i 's/localhost/192.168.0.13/g' ~/mi-equipo-vcluster.yaml

# Copiar al ThinkPad o a tu máquina local
scp ~/mi-equipo-vcluster.yaml usuario@192.168.0.10:~/
# o directamente a tu laptop (si tienes acceso SSH al Ryzen)
scp usuario@192.168.0.13:~/mi-equipo-vcluster.yaml ~/.kube/mi-equipo-vcluster.yaml
```

**3. Verificar conectividad:**
```bash
kubectl --kubeconfig=~/mi-equipo-vcluster.yaml get nodes
# NAME    STATUS   ROLES    AGE   VERSION
# ryzen   Ready    <none>   3m    v1.xx.x+k3s1

kubectl --kubeconfig=~/mi-equipo-vcluster.yaml get pods -A
# NAMESPACE     NAME                       READY   STATUS    AGE
# kube-system   coredns-xxx-yyy            1/1     Running   3m
```

**4. (Opcional) Agregar al kubeconfig global:**
```bash
# Hacer merge en ~/.kube/config
KUBECONFIG=~/.kube/config:~/mi-equipo-vcluster.yaml \
  kubectl config view --flatten > ~/.kube/config.merged

mv ~/.kube/config.merged ~/.kube/config

# Cambiar de contexto
kubectl config get-contexts
kubectl config use-context mi-equipo-dev  # nombre del contexto en el kubeconfig
```

---

### Fase 5: Desplegar una Aplicación de Prueba

**1. Desplegar una aplicación `nginx` de prueba en el vcluster:**
```bash
# Crear namespace para la aplicación
kubectl --kubeconfig=~/mi-equipo-vcluster.yaml create namespace mi-app

# Desplegar nginx
kubectl --kubeconfig=~/mi-equipo-vcluster.yaml apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-zerops
  namespace: mi-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: hello-zerops
  template:
    metadata:
      labels:
        app: hello-zerops
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: hello-zerops
  namespace: mi-app
spec:
  selector:
    app: hello-zerops
  ports:
  - port: 80
    targetPort: 80
EOF
```

**2. Verificar que el deployment está Running:**
```bash
kubectl --kubeconfig=~/mi-equipo-vcluster.yaml -n mi-app rollout status deployment/hello-zerops
# deployment "hello-zerops" successfully rolled out

kubectl --kubeconfig=~/mi-equipo-vcluster.yaml -n mi-app get pods
# NAME                            READY   STATUS    RESTARTS   AGE
# hello-zerops-xxx-yyy            1/1     Running   0          30s
```

**3. Hacer port-forward para probar localmente:**
```bash
kubectl --kubeconfig=~/mi-equipo-vcluster.yaml -n mi-app \
  port-forward svc/hello-zerops 8080:80 &

curl http://localhost:8080
# <!DOCTYPE html><html><head><title>Welcome to nginx!</title>...
```

---

### Fase 6: Ver los Recursos en ArgoCD

**1. Navegar a ArgoCD:**
```
https://argocd.zerops.internal
```
- Login: `admin` / contraseña obtenida en PB-01

**2. Explorar las Applications:**
- `platform-root` — App-of-Apps raíz
- `crossplane-system` — Crossplane y providers
- `cert-manager` — Gestión de certificados
- `gitea` — Repositorio de código
- `platform-mi-equipo-dev` — La Platform recién creada (si ArgoCD la gestiona)

**3. Ver el árbol de recursos:**
- Click en la Application → ver el grafo de recursos en tiempo real
- Los nodos verdes = Healthy, los amarillos = Progressing, los rojos = Degraded

**4. Forzar sincronización si es necesario:**
```bash
# Via CLI (argocd CLI debe estar instalado)
argocd login argocd.zerops.internal --username admin
argocd app sync platform-root
argocd app sync platform-mi-equipo-dev
```

---

### Fase 7: Gestión del Ciclo de Vida

#### Actualizar una Platform (cambiar tamaño)

```bash
# Editar el Platform CR para cambiar de small a medium
kubectl patch platform mi-equipo-dev -n default \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/size", "value": "medium"}]'

# Crossplane detecta el cambio y actualiza el Helm Release
# El vcluster reiniciará con más recursos
kubectl get platforms -w
# Esperar a READY=True nuevamente
```

#### Escalar réplicas de una aplicación dentro del vcluster

```bash
kubectl --kubeconfig=~/mi-equipo-vcluster.yaml -n mi-app \
  scale deployment hello-zerops --replicas=3

kubectl --kubeconfig=~/mi-equipo-vcluster.yaml -n mi-app get pods
# NAME                    READY   STATUS    RESTARTS   AGE
# hello-zerops-xxx-aaa    1/1     Running   0          30s
# hello-zerops-xxx-bbb    1/1     Running   0          30s
# hello-zerops-xxx-ccc    1/1     Running   0          30s
```

#### Eliminar una Platform

> ⚠️ **Acción destructiva.** Esto eliminará el vcluster y todos los recursos dentro de él. Asegúrate de hacer backup de lo que necesites.

```bash
# 1. Hacer backup del kubeconfig (por si acaso)
cp ~/mi-equipo-vcluster.yaml ~/backups/mi-equipo-vcluster-$(date +%Y%m%d).yaml

# 2. Eliminar el Platform CR
kubectl delete platform mi-equipo-dev -n default

# 3. Crossplane eliminará el Helm Release → el vcluster se destruirá
# 4. El namespace en el Ryzen será eliminado automáticamente
# 5. Monitorear la eliminación
kubectl get platforms -w
# (la entrada desaparecerá cuando sea eliminada)

# 6. Verificar en el Ryzen que el namespace fue eliminado
ssh usuario@192.168.0.13
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get ns | grep mi-equipo
# (no debe aparecer)
```

---

## 📖 Referencia de Comandos del Día a Día

### Comandos en el ThinkPad (Management)

```bash
# Ver todos los entornos (Platforms) activos
kubectl get platforms -A

# Ver estado detallado de un entorno
kubectl describe platform <nombre> -n <namespace>

# Ver todos los Helm Releases gestionados por Crossplane
kubectl get releases -A

# Ver todos los Objects gestionados por Crossplane
kubectl get objects -A

# Ver logs de Crossplane (para debugging)
kubectl -n crossplane-system logs -l app=crossplane --tail=100 -f

# Ver logs de provider-helm
kubectl -n crossplane-system logs -l pkg.crossplane.io/revision=provider-helm --tail=50 -f

# Ver certificados TLS gestionados por cert-manager
kubectl get certificates -A
kubectl get certificaterequests -A

# Ver apps en ArgoCD
kubectl -n argocd get applications

# Forzar sync de una app en ArgoCD
kubectl -n argocd patch application <nombre-app> \
  --type merge -p '{"operation": {"sync": {"revision": "HEAD"}}}'
```

### Comandos en el Ryzen (Compute)

```bash
# Ver todos los namespaces (cada vcluster tiene su propio namespace)
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get ns

# Ver todos los pods de vclusters
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get pods --all-namespaces

# Ver recursos consumidos por un vcluster específico
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml top pods -n <namespace-del-vcluster>

# Ver logs de un vcluster
kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml logs \
  -n <namespace-del-vcluster> vcluster-0 -f
```

### Comandos dentro del vcluster (Developer)

```bash
# Shortcut: exportar la variable KUBECONFIG
export KUBECONFIG=~/mi-equipo-vcluster.yaml

# Ver nodos virtuales
kubectl get nodes

# Ver todos los pods
kubectl get pods -A

# Ver recursos disponibles
kubectl describe node

# Desplegar desde un archivo
kubectl apply -f mi-aplicacion.yaml

# Ver logs de una aplicación
kubectl -n mi-namespace logs deployment/mi-app -f

# Port-forward para acceso local
kubectl -n mi-namespace port-forward svc/mi-servicio 8080:80

# Limpiar todo en un namespace
kubectl -n mi-namespace delete all --all
```

### Comandos de PowerDNS (Raspberry Pi)

```bash
# Registrar un nuevo hostname
curl -X PATCH -H "X-API-Key: super-secret-api-key-zerops" \
  -H "Content-Type: application/json" \
  -d '{"rrsets": [{"name": "<hostname>.zerops.internal.", "type": "A", "ttl": 300, "changetype": "REPLACE", "records": [{"content": "<IP>", "disabled": false}]}]}' \
  http://192.168.0.200:8081/api/v1/servers/localhost/zones/zerops.internal.

# Listar todos los registros DNS
curl -H "X-API-Key: super-secret-api-key-zerops" \
  http://192.168.0.200:8081/api/v1/servers/localhost/zones/zerops.internal. | python3 -m json.tool

# Eliminar un registro DNS
curl -X PATCH -H "X-API-Key: super-secret-api-key-zerops" \
  -H "Content-Type: application/json" \
  -d '{"rrsets": [{"name": "<hostname>.zerops.internal.", "type": "A", "changetype": "DELETE"}]}' \
  http://192.168.0.200:8081/api/v1/servers/localhost/zones/zerops.internal.
```

---

## ✅ Checklist de Validación (Exit Criteria)

- [ ] Puedes acceder a `https://choreo.zerops.internal` y el Catalog muestra los componentes
- [ ] Puedes crear una Platform CR y `kubectl get platforms -w` muestra `READY=True` en < 5 min
- [ ] Puedes extraer el kubeconfig del vcluster y ejecutar `kubectl get nodes`
- [ ] El nodo virtual está en estado `Ready`
- [ ] Puedes desplegar una aplicación de prueba dentro del vcluster
- [ ] `https://argocd.zerops.internal` muestra las Applications en `Synced+Healthy`
- [ ] Puedes eliminar una Platform y el namespace desaparece del Ryzen limpiamente

---

## 📚 Referencia Rápida de URLs

| Servicio | URL | Propósito |
|---|---|---|
| Developer Portal | https://choreo.zerops.internal | Backstage — entrada para developers |
| ArgoCD | https://argocd.zerops.internal | GitOps — ver estado de deployments |
| Gitea | https://gitea.zerops.internal | Repositorio — código e IaC |
| Step-CA | https://step-ca.zerops.internal:9000 | PKI — emisión de certificados |
| PowerDNS API | http://192.168.0.200:8081 | DNS — gestión de registros |

---

## 🔗 Navegación de Playbooks

| PB | Descripción | Link |
|---|---|---|
| PB-00 | Red e Identidad Corporativa (Raspberry Pi) | [PB-00](./PB-00-corporate-network-identity.md) |
| PB-01 | Management Cluster (ThinkPad) | [PB-01](./PB-01-management-cluster.md) |
| PB-02 | Compute Cluster (Ryzen) | [PB-02](./PB-02-compute-cluster.md) |
| PB-03 | Primer Entorno de Desarrollo (Platform) | [PB-03](./PB-03-first-platform.md) |
| PB-04 | Developer Workflow (este documento) | — |

---

*Parte del [Zerops Sovereign IDP](../../README.md) — Air-gapped Internal Developer Platform*
