# Arquitectura Multi-Clúster Soberana Independiente (PB-02 Avanzado)
## Clúster de Gestión (ThinkPad) + Clúster de Cómputo Físico Independiente (Ryzen/GPU) + vcluster + Fideicomiso de CA Soberana

Este documento redefine la arquitectura para asegurar la **independencia física y de control** de nuestros sistemas. La máquina potente Ryzen **NO** se une al clúster del ThinkPad como un nodo worker clásico. En su lugar, opera como un **clúster de Kubernetes independiente y autónomo** en la red. El clúster del ThinkPad actúa estrictamente como el **Clúster de Gestión (Management Cluster)**, gestionando el ciclo de vida de los clústeres virtuales (`vclusters`) de manera remota dentro del clúster de la máquina Ryzen a través de **Crossplane**.

---

## 1. Topología de Red y Conectividad Física

Nuestra red interna se distribuye de la siguiente manera:

*   **Tier 0 (Confianza y Red):** Raspberry Pi (`192.168.0.9`) — Aloja PowerDNS y Step-CA.
*   **Tier 1 (Gestión / Cerebro):** ThinkPad (`192.168.0.10`) — Corre el clúster de K3s de gestión, ArgoCD, Gitea, Crossplane y el Developer Portal.
*   **Tier 2 (Cómputo / Músculo):** Host Ryzen (`192.168.0.13`) — Clúster de K3s físico independiente. Aloja y ejecuta los `vclusters` (clústeres virtuales independientes) con workloads reales y acceso a GPU. Acceso SSH: `admin` / `ADMIN123`.

---

## 2. Mapa de Arquitectura Multi-Clúster Real

El siguiente diagrama muestra cómo coexisten los dos clústeres físicos independientes y cómo el clúster de gestión orquesta los recursos remotos mediante APIs cifradas y seguras:

```mermaid
flowchart TD
    subgraph Tier_0["Tier 0: Confianza (Raspberry Pi - 192.168.0.9)"]
        ca["Step-CA (ACME Private PKI)"]
        dns["PowerDNS (Wildcard *.zerops.internal)"]
    end

    subgraph Tier_1["Tier 1: Cerebro (ThinkPad - 192.168.0.10)"]
        subgraph K3s_Management["K3s Clúster de Gestión (Físico)"]
            portal["OpenChoreo / Backstage"]
            gitea["Gitea (Layer of Truth)"]
            argocd["ArgoCD (GitOps Engine)"]
            crossplane["Crossplane Control Plane"]
            prov_k8s["Crossplane Provider Kubernetes / Helm"]
        end
    end

    subgraph Tier_2["Tier 2: Músculo (Host Ryzen - 192.168.0.13)"]
        subgraph K3s_Compute_Host["K3s Clúster de Cómputo (Físico Independiente)"]
            ca_trust["Almacén de Confianza (Root CA importada)"]
            nvidia["NVIDIA GPU Operator / Device Plugin"]
            
            subgraph VCluster_A["vcluster-ia-desarrollo (Namespace: env-ia)"]
                vc_api_a["vcluster API Server"]
                app_ia["Workloads de IA (Acceso a GPU)"]
            end

            subgraph VCluster_B["vcluster-web-prod (Namespace: env-web)"]
                vc_api_b["vcluster API Server"]
                app_web["Workloads de Producción"]
            end
        end
    end

    %% Fideicomiso de Certificados
    ca -.->|Importación de root_ca.crt| ca_trust
    ca -.->|Firma de certificados ACME| K3s_Management
    
    %% Flujo de Aprovisionamiento Remoto
    portal -->|1. Commit de Estado| gitea
    argocd -->|2. Sincroniza Manifiesto| crossplane
    crossplane -->|3. Procesa Recurso Declarativo| prov_k8s
    prov_k8s -->|4. Llamada API Kubernetes Remota (puerto 6443)| K3s_Compute_Host
    K3s_Compute_Host -->|5. Despliega y Orquesta| VCluster_A
```

---

## 3. Estrategia de Confianza PKI (Importación de la CA Raíz)

Como el clúster de la máquina Ryzen (`192.168.0.13`) es totalmente independiente, los pods y los clústeres virtuales que corran en ella no confiarán por defecto en los certificados emitidos por la Step-CA de nuestra red interna (como `https://gitea.zerops.internal`).

Para resolver esto de raíz y lograr un fideicomiso completo en la red interna:

1.  **Importación del Certificado Raíz en el Sistema Operativo del Host Ryzen:**
    Copiaremos el certificado raíz `root_ca.crt` desde la Raspberry Pi al host Ryzen y lo registraremos en el almacén de certificados del sistema:
    ```bash
    # En el Host Ryzen (192.168.0.13)
    sudo cp root_ca.crt /usr/local/share/ca-certificates/zerops-root-ca.crt
    sudo update-ca-certificates
    ```

2.  **Configuración de K3s para Confiar en la CA Raíz:**
    K3s utiliza su propio contenedor de certificados en algunos componentes y hereda del sistema en otros. Para garantizar la confianza total en todo el motor de K3s y sus contenedores, configuraremos el motor de K3s para que monte y confíe en la CA raíz del sistema. Esto permite que servicios como `containerd`, `cert-manager` local y los vclusters que se creen hereden la confianza automáticamente.

---

## 4. Cómo Crossplane Gestiona el Clúster Remoto

1.  **Aislamiento de Seguridad:**
    El ThinkPad **no** tiene acceso a nivel de sistema operativo directo del Ryzen para sus workloads. En su lugar, el ThinkPad interactúa únicamente a través de la **API de Kubernetes segura y cifrada de K3s (puerto 6443)** del host Ryzen.
2.  **El Kubeconfig Secreto:**
    Extraemos el archivo `kubeconfig` de la máquina Ryzen (`/etc/rancher/k3s/k3s.yaml`, reemplazando `127.0.0.1` por `192.168.0.13`) y lo guardamos como un Kubernetes Secret en el ThinkPad en el namespace `crossplane-system`.
3.  **ProviderConfig de Crossplane:**
    Configuramos un `ProviderConfig` en Crossplane que le dice: *"Cuando quieras aprovisionar recursos de tipo vcluster o namespaces de desarrollo, utiliza el Secret que contiene el Kubeconfig de la máquina Ryzen"*.
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
    ```

---

## 5. Plan de Acción Ajustado (Fases Correctas)

### Fase 1: Inicialización del Clúster de Cómputo Independiente (Ryzen - 192.168.0.13)
1.  **SSH y Acceso:** Conectarse vía SSH a la máquina potente (`admin` / `ADMIN123` en `192.168.0.13`).
2.  **Confianza de Red:** Descargar el certificado raíz `root_ca.crt` de la Raspberry Pi y registrarlo en el sistema operativo del Ryzen.
3.  **Instalar K3s Autónomo:** Instalar K3s de forma independiente en la máquina Ryzen sin unirse a ningún clúster existente.
4.  **Habilitar GPUs:** Instalar los controladores de NVIDIA y el plugin de GPU para Kubernetes en este K3s autónomo.

### Fase 2: Configurar la Orquestación Remota en el ThinkPad
1.  Descargar el `kubeconfig` del Ryzen, modificar la IP a `192.168.0.13` y subirlo al ThinkPad como un Secret de Kubernetes.
2.  Desplegar **Crossplane** en el ThinkPad vía ArgoCD.
3.  Configurar el `Provider-Kubernetes` de Crossplane en el ThinkPad apuntando al Secret del Ryzen.

### Fase 3: Diseñar las Composiciones de vcluster
1.  Definir las plantillas de infraestructura en el ThinkPad que le indiquen a Crossplane cómo levantar un `vcluster` (un namespace, un despliegue Helm de vcluster y recursos asociados) de forma remota en la máquina Ryzen.

### Fase 4: Integración del Portal OpenChoreo
1.  Desplegar OpenChoreo en el ThinkPad.
2.  Configurar la interfaz/plano conversacional para que genere manifiestos que utilicen los templates de Crossplane definidos en la Fase 3, logrando el despliegue automático bajo demanda en la máquina potente.
