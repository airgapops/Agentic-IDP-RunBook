# Guía de Replicación de Plataforma Soberana (PB-00 a PB-06)

Esta guía técnica de nivel empresarial documenta detalladamente la implementación, las configuraciones y los comandos necesarios para replicar y operar nuestra **Plataforma Soberana e Infraestructura de Autoservicio (IDP)**. El ecosistema abarca desde la capa de confianza criptográfica inicial hasta el aprovisionamiento multi-clúster remoto e inteligente y el portal de desarrollo unificado.

---

## 1. Arquitectura de Referencia de la Plataforma Soberana

La plataforma opera bajo un modelo distribuido con roles físicos y virtuales claramente definidos y desacoplados:

```mermaid
graph TD
    subgraph Tier 0: Confianza y Red (Raspberry Pi - 192.168.0.9)
        DNS[PowerDNS / Servidor DNS Interno]
        StepCA[Step-CA / ACME Private PKI]
    end

    subgraph Tier 1: Clúster de Gestión y Borde (ThinkPad - 192.168.0.10)
        K3s_Mgmt[K3s Management Cluster]
        MetalLB[MetalLB LoadBalancer / IP: 192.168.0.203]
        Envoy[Envoy Gateway / Gateway API]
        CertManager[cert-manager / ACME Client]
        ExtDNS[ExternalDNS / pdns dynamic sync]
        ArgoCD[ArgoCD / Motor de GitOps]
        Gitea[Gitea / Repositorio Git Local]
        OpenChoreo[OpenChoreo / Portal de Desarrollador]
        Crossplane[Crossplane Control Plane]
    end

    subgraph Tier 2: Clúster de Cómputo Independiente (Ryzen - 192.168.0.13)
        K3s_Ryzen[K3s Autonomous Compute Cluster]
        NvidiaGPU[NVIDIA GPU Runtime & Operator]
        
        subgraph Clústeres Virtuales (Orquestados por Crossplane)
            vcluster_dev[vcluster-ia-desarrollo / Namespace: env-ia]
            vcluster_prod[vcluster-web-prod / Namespace: env-web]
        end
    end

    %% Relaciones y Direcciones del Tráfico
    DNS -.->|Resolución Wildcard *.zerops.internal| MetalLB
    StepCA -.->|Emisión Automática de TLS (ACME)| CertManager
    CertManager -->|Auto-inyección de Certificados| Envoy
    ExtDNS -->|Registro automático de HTTPRoutes| DNS
    ArgoCD -->|Reconciliación GitOps| Gitea
    OpenChoreo -->|Autoservicio / Gitea Commits| Gitea
    Crossplane -->|Llamadas API seguras (Puerto 6443)| K3s_Ryzen
    K3s_Ryzen -->|Creación de Clústeres Virtuales| vcluster_dev
```

---

## PB-00: Prerrequisitos de Infraestructura (Tier 0)

### 1. Servidor de DNS Privado: PowerDNS (Raspberry Pi)
PowerDNS corre en un contenedor Podman en la Raspberry Pi, proporcionando la zona privada local `zerops.internal`.

*   **Configuración del API**: Puerto `8081` habilitado con la API Key `super-secret-api-key-zerops`.
*   **Registro Wildcard Principal**:
    *   **Registro**: `*.zerops.internal`
    *   **Tipo**: `A`
    *   **Dirección IP**: `192.168.0.10` (Apunta directamente a la IP física del ThinkPad que aloja el Gateway Envoy).

### 2. Autoridad Certificadora (CA) Criptográfica: Step-CA (Raspberry Pi)
Step-CA opera en el puerto `9000` como un servidor PKI privado compatible con el protocolo ACME.
*   **Certificado Raíz (`root_ca.crt`)**: Emite el anclaje criptográfico de confianza para todos los equipos.
*   **Aprovisionador ACME**: Configurado para resolver y firmar certificados de dominios de la red interna `zerops.internal`.

---

## PB-01: Fundación de Nube Privada (Tier 1 - ThinkPad)

### 1. Instalación de K3s de Gestión
Instalado en el ThinkPad Ubuntu (`192.168.0.10`). El control administrativo se realiza mediante SSH utilizando la autenticación por contraseña bypass:
```bash
sshpass -p 'Panama2028#...' ssh -o PubkeyAuthentication=no -o StrictHostKeyChecking=no rooter@192.168.0.10 "sudo kubectl get nodes"
```

### 2. Balanceador de Carga: MetalLB
Permite asignar direcciones IP reales de la red LAN a los servicios y gateways del clúster físico.
*   **IP activa asignada**: `192.168.0.203` (Reservada exclusivamente para el Gateway Envoy vía pool MetalLB `192.168.0.201-250`).

### 3. Puerta de Enlace Envoy: Gateway API
Reemplaza el clásico controlador Ingress con la especificación moderna de Kubernetes Gateway API.
*   **Manifiesto del Gateway (`gateway/zerops-gateway.yaml`)**:
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: zerops-gateway
  namespace: default
  annotations:
    cert-manager.io/cluster-issuer: zerops-ca-issuer
spec:
  gatewayClassName: eg
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
    - name: https-gitea
      protocol: HTTPS
      port: 443
      hostname: gitea.zerops.internal
      tls:
        mode: Terminate
        certificateRefs:
          - group: ""
            kind: Secret
            name: gitea-tls-secret
      allowedRoutes:
        namespaces:
          from: All
    - name: https-argocd
      protocol: HTTPS
      port: 443
      hostname: argocd.zerops.internal
      tls:
        mode: Terminate
        certificateRefs:
          - group: ""
            kind: Secret
            name: argocd-tls-secret
      allowedRoutes:
        namespaces:
          from: All
    - name: https-choreo
      protocol: HTTPS
      port: 443
      hostname: choreo.zerops.internal
      tls:
        mode: Terminate
        certificateRefs:
          - group: ""
            kind: Secret
            name: choreo-tls-secret
      allowedRoutes:
        namespaces:
          from: All
```

### 4. Automatización de Certificados: cert-manager
*   **ClusterIssuer de Step-CA**:
```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: zerops-ca-issuer
spec:
  acme:
    server: https://192.168.0.9:9000/acme/acme/directory
    privateKeySecretRef:
      name: step-ca-acme-key
    solvers:
      - http01:
          ingress:
            class: eg
```

### 5. Sincronización Dinámica de DNS: ExternalDNS
ExternalDNS monitorea las rutas Envoy del tipo `HTTPRoute` e inyecta automáticamente los registros dinámicos en PowerDNS.

> [!IMPORTANT]
> **Permisos de Namespaces en ClusterRole**: Para evitar fallos de sincronización y reinicios (CrashLoopBackOff), el `ClusterRole` de `external-dns` debe incluir acceso explícito para listar y observar (`list`, `watch`, `get`) recursos de tipo `namespaces`.

*   **ClusterRole Configurado (`gateway/external-dns.yaml`)**:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: external-dns
rules:
  - apiGroups: [""]
    resources: ["services","endpoints","pods","namespaces"]
    verbs: ["get","watch","list"]
  - apiGroups: ["extensions","apps"]
    resources: ["deployments"]
    verbs: ["get","watch","list"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["ingresses"]
    verbs: ["get","watch","list"]
  - apiGroups: ["gateway.networking.k8s.io"]
    resources: ["gateways", "httproutes"]
    verbs: ["get","watch","list"]
```

---

## PB-02: El Corazón del AI-IDP (Tier 2 - GitOps)

### 1. Repositorio de la Verdad: Gitea
Expuesto a través de Envoy en `https://gitea.zerops.internal`. Toda la configuración se guarda en el repositorio `airgap-ops` del usuario `jadamson`.

### 2. Motor GitOps y Configuración Insecure en ArgoCD
ArgoCD reside en el namespace `argocd` y opera bajo `https://argocd.zerops.internal`. Para evitar redirecciones TLS infinitas debido a la terminación TLS en Envoy, se activa el modo inseguro:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cmd-params-cm
  namespace: argocd
data:
  server.insecure: "true"
```

### 3. Registro del Repositorio y Salto de Verificación SSL
Dado que Gitea utiliza certificados autofirmados por nuestra Step-CA privada, instruimos a ArgoCD para omitir la validación SSL exclusivamente en este origen:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: gitea-repo-secret
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
type: Opaque
stringData:
  url: https://gitea.zerops.internal/jadamson/airgap-ops.git
  insecure: "true"
```

---

## PB-03: Aprovisionamiento de Cómputo Independiente (Ryzen)

Para garantizar la potencia de cómputo libre de interferencias, el host Ryzen (`192.168.0.13`) se mantiene como un clúster físico autónomo.

### 1. Establecimiento de Fideicomiso PKI (Rocky Linux 10)
Para que el host Ryzen confíe en los certificados emitidos por la Step-CA (como el de Gitea):
1.  Copia el certificado raíz:
    ```bash
    sudo cp root_ca.crt /etc/pki/ca-trust/source/anchors/zerops-root.crt
    ```
2.  Actualiza el almacén del sistema:
    ```bash
    sudo update-ca-trust
    ```

### 2. Instalación de K3s Autónomo
Instalado en el Ryzen desactivando el balanceador Traefik integrado para permitir la orquestación remota limpia:
```bash
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable traefik" sh -
```

### 3. Apertura de Red y Cortafuegos
Para habilitar el control remoto, abrimos de forma permanente el puerto de la API de K3s en Rocky Linux:
```bash
sudo firewall-cmd --zone=public --add-port=6443/tcp --permanent
sudo firewall-cmd --reload
```

### 4. Puente de Secretos entre Clústeres (Secret Bridge)
Para enlazar el ThinkPad de gestión con el Ryzen de ejecución:
1.  Extrae el archivo `/etc/rancher/k3s/k3s.yaml` del Ryzen.
2.  Reemplaza `127.0.0.1` por la IP fija `192.168.0.13`.
3.  Crea el secreto en el clúster del ThinkPad:
    ```bash
    kubectl create secret generic ryzen-kubeconfig-secret \
      --namespace=crossplane-system \
      --from-file=kubeconfig=ryzen-modified-kubeconfig.yaml
    ```

---

## PB-04: Motor de Orquestación Declarativa (Crossplane)

Crossplane actúa como el motor que traduce los requisitos lógicos en recursos físicos en el Ryzen.

### 1. Aplicación de Crossplane en ArgoCD (`argocd/crossplane.yaml`)
Despliega la versión estable y activa las definiciones de recursos personalizados (CRDs).

### 2. Registro de los Proveedores Kubernetes y Helm
Instalamos los proveedores requeridos en el ThinkPad (`infrastructure/crossplane-providers.yaml`):
```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-kubernetes
spec:
  package: xpkg.upbound.io/crossplane-contrib/provider-kubernetes:v0.11.0
---
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-helm
spec:
  package: xpkg.upbound.io/crossplane-contrib/provider-helm:v0.19.0
```

### 3. Configuración de Acceso Remoto al Ryzen (`infrastructure/crossplane-providerconfigs.yaml`)
Enlazamos el motor de Crossplane con el Ryzen a través de `ryzen-kubeconfig-secret`:
```yaml
apiVersion: kubernetes.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: ryzen-compute-cluster
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: ryzen-kubeconfig-secret
      key: kubeconfig
---
apiVersion: helm.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: ryzen-helm-cluster
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: ryzen-kubeconfig-secret
      key: kubeconfig
```

---

## PB-05: Abstracciones del Desarrollador (Dos Niveles de Control)

### Nivel 1: Definición de Plataformas (vclusters)
Cuando un desarrollador crea una `Platform`, Crossplane aprovisiona de forma aislada un namespace y un clúster de K3s virtual ligero (`vcluster`) en el Ryzen.

*   ** XRD de Plataformas (`platform-xrd.yaml`)**:
```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: CompositeResourceDefinition
metadata:
  name: compositeplatforms.zerops.internal
spec:
  group: zerops.internal
  names:
    kind: CompositePlatform
    plural: compositeplatforms
  claimNames:
    kind: Platform
    plural: platforms
  versions:
    - name: v1alpha1
      served: true
      referenceable: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                size:
                  type: string
                  default: small
                namespace:
                  type: string
              required:
                - namespace
```

*   **Composición de Plataformas (`platform-composition.yaml`)**:
    Instala Loft `vcluster` mediante Helm de manera remota en el host Ryzen configurando la dirección SAN para aceptar llamadas SSL seguras del DNS wildcard.

---

### Nivel 2: Servicios de Respaldo e Integración de Middleware (Dapr)
La infraestructura aprovisionada (Bases de datos, colas) se inyecta automáticamente con abstracciones de **Dapr Components** en el namespace de ejecución para que el código de la aplicación interactúe sin conocer las credenciales físicas de red.

#### 1. Bases de Datos (`Database`)
*   **Descripción**: Provisiona motores Redis/Postgres en el Ryzen.
*   **Composición (`database-composition.yaml`)**: Despliega el chart Helm de `bitnami/redis` y auto-inyecta un Dapr Component de tipo `state.redis` en el namespace remoto:
```yaml
apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: statestore
  namespace: <app-namespace>
spec:
  type: state.redis
  version: v1
  metadata:
  - name: redisHost
    value: <service-name>-db-redis-master:6379
  - name: redisPassword
    value: ""
```

#### 2. Mensajería y Colas (`Queue`)
*   **Descripción**: Provisiona RabbitMQ de forma aislada.
*   **Composición (`queue-composition.yaml`)**: Registra la release Helm de RabbitMQ y expone un componente de Dapr de tipo `pubsub.rabbitmq` para habilitar flujos reactivos desacoplados.

#### 3. LLMKube Gateway AI (Acceso GPU)
*   **Descripción**: Mapea el acceso seguro a modelos LLM locales de inferencia (vLLM / Ollama) alojados directamente en la potente GPU del Ryzen.

---

## PB-06: Despliegue del Portal OpenChoreo (Backstage)

El portal centraliza el acceso a las plantillas y diagramas del IDP, corriendo en el clúster ThinkPad.

### 1. Base de Datos Persistente local en ThinkPad (`openchoreo/openchoreo-db.yaml`)
Despliega PostgreSQL en el namespace `openchoreo-system` con un PersistentVolumeClaim local para salvaguardar el estado de catálogos y plantillas de desarrollo.

### 2. Portal OpenChoreo (`openchoreo/openchoreo-app.yaml`)
Utiliza la imagen de la comunidad soportada `roadiehq/community-backstage-image:latest`.

> [!IMPORTANT]
> **Puerto de Escucha de Backstage de la Comunidad**: A diferencia de la imagen de desarrollo tradicional de Backstage que expone en el puerto `7007`, la imagen comunitaria `roadiehq/community-backstage-image` está compilada y optimizada para servir las peticiones HTTP de forma nativa en el puerto **`7000`**. Los servicios de Kubernetes deben apuntar el `targetPort` a este puerto.

*   **Configuración del Servicio y Despliegue**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: openchoreo-app
  namespace: openchoreo-system
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: backstage
        image: roadiehq/community-backstage-image:latest
        ports:
        - containerPort: 7000
          name: http
---
apiVersion: v1
kind: Service
metadata:
  name: openchoreo-service
  namespace: openchoreo-system
spec:
  selector:
    app: openchoreo-app
  ports:
  - port: 80
    targetPort: 7000
```

### 3. Exposición HTTPS Criptográficamente Segura (Envoy + cert-manager)
Configuramos la ruta L7 y la auto-generación de certificados válidos mediante Step-CA (`openchoreo/openchoreo-app.yaml`):
```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: choreo-cert
  namespace: default
spec:
  commonName: choreo.zerops.internal
  dnsNames:
    - choreo.zerops.internal
  issuerRef:
    kind: ClusterIssuer
    name: zerops-ca-issuer
  secretName: choreo-tls-secret
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: openchoreo-route
  namespace: openchoreo-system
spec:
  parentRefs:
    - name: zerops-gateway
      namespace: default
  hostnames:
    - "choreo.zerops.internal"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: openchoreo-service
          port: 80
```

---

## 2. Pruebas de Validación y Bitácora de Éxito

### 1. Resolución Dinámica y Handshake TLS Seguro
Ejecutando un curl con mapeo SNI resuelto directamente a la IP del LoadBalancer local de Envoy (`192.168.0.203`) se obtiene una conexión segura validada correctamente por Step-CA sin requerir parámetros inseguros:

```bash
rooter@192.168.0.10:~$ curl -vI https://choreo.zerops.internal --resolve choreo.zerops.internal:443:192.168.0.203

* Connected to choreo.zerops.internal (192.168.0.203) port 443
* ALPN: server accepted h2
* Server certificate:
*  subject: CN=choreo.zerops.internal
*  start date: May 22 03:09:35 2026 GMT
*  expire date: May 23 03:10:35 2026 GMT
*  subjectAltName: host "choreo.zerops.internal" matched cert's "choreo.zerops.internal"
*  issuer: O=Zerops Internal Root CA; CN=Zerops Internal Root CA Intermediate CA
*  SSL certificate verify ok.
> HEAD / HTTP/2
> Host: choreo.zerops.internal

< HTTP/2 200 
< content-type: text/html; charset=UTF-8
< content-length: 3427
< date: Fri, 22 May 2026 03:27:26 GMT
```

### 2. Bitácora de Sincronización Automática en PowerDNS
Al desplegarse y validarse la ruta de OpenChoreo, el componente `external-dns` recuperó el estado de namespaces de manera segura y actualizó los registros A correspondientes en la Raspberry Pi de forma instantánea:

```bash
rooter@192.168.0.10:~$ kubectl logs -n kube-system deployment/external-dns --tail 5
time="2026-05-22T03:27:37Z" level=info msg="CREATE: choreo.zerops.internal 0 IN A  192.168.0.203 []"
time="2026-05-22T03:27:37Z" level=info msg="CREATE: argocd.zerops.internal 0 IN A  192.168.0.203 []"
time="2026-05-22T03:27:37Z" level=info msg="CREATE: gitea.zerops.internal 0 IN A  192.168.0.203 []"
time="2026-05-22T03:27:37Z" level=info msg="Changes pushed out to PowerDNS in 35.642884ms"
```
