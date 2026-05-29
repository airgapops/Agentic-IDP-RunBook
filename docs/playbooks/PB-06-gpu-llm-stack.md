# 🖥️ PB-06 — Stack GPU + LLM en Ryzen (Rocky Linux 10 / RTX 5060 Ti Blackwell)

> **Playbook:** PB-06  
> **Versión:** 1.0  
> **Fecha:** 2026-05-25  
> **Autor:** Ops Team  
> **Estado:** ✅ Validado en producción

---

## 📋 Tabla de Resumen

| Componente | Versión / Detalle | Estado |
|---|---|---|
| Hardware | AMD Ryzen + NVIDIA RTX 5060 Ti 16 GB (Blackwell GB206) | ✅ |
| Sistema Operativo | Rocky Linux 10 (RHEL 10 compatible) | ✅ |
| NVIDIA Driver | 595.71.05 (open kernel module) | ✅ |
| CUDA | 13.2 | ✅ |
| Ollama | Latest (con backend ROCm deshabilitado) | ✅ |
| Modelos LLM | hermes3:8b · llama3.1:8b · qwen2.5:14b | ✅ |
| NVIDIA Device Plugin | v0.17.0 (ruta kubelet estándar) | ✅ |
| K3s | Ya instalado (cluster Ryzen) | ✅ |

| Hermes Agent | OpenRouter (GPT-4o) / Ollama local (qwen2.5:14b) | ✅ |

---

## 🔰 Prerrequisitos

Antes de comenzar, verificar que se cumplen **todos** los siguientes puntos:

- **Hardware:** Nodo Ryzen con GPU NVIDIA RTX 5060 Ti 16 GB (arquitectura Blackwell GB206)
- **Sistema Operativo:** Rocky Linux 10 (o equivalente RHEL 10). **No usar RHEL 9.**
- **K3s:** Instalado y funcionando en el nodo (`kubectl get nodes` responde correctamente)
- **Acceso:** Sesión activa como `root` o con `sudo` sin contraseña
- **Conectividad:** Acceso a Internet para descargar repos y modelos (o mirrors internos configurados)
- **Kernel headers:** Disponibles para la versión de kernel en ejecución (`uname -r`)

> [!IMPORTANT]
> La RTX 5060 Ti (Blackwell GB206) **NO es compatible** con el módulo de kernel NVIDIA propietario (`kmod-nvidia`). Únicamente el módulo abierto (`kmod-nvidia-open-dkms`) inicializa correctamente la GPU. Ignorar este punto resulta en `nvidia-smi: No devices were found`.

---

## 🗃️ Paso 1 — Repositorios CUDA

Agregar el repositorio CUDA para RHEL 10. Es crítico usar la variante `rhel10`, no `rhel9`.

> [!WARNING]
> Si tienes habilitado el repositorio `cuda-rhel9-x86_64` por alguna instalación anterior, **deshabilitarlo explícitamente** antes de continuar. Mezclar repositorios RHEL9/RHEL10 provoca conflictos de dependencias y puede instalar drivers incompatibles.

```bash
# Agregar repositorio CUDA para RHEL 10
dnf config-manager --add-repo \
  https://developer.download.nvidia.com/compute/cuda/repos/rhel10/x86_64/cuda-rhel10.repo
# Deshabilitar repo RHEL 9 si existiera
dnf config-manager --disable cuda-rhel9-x86_64 2>/dev/null || true

# Limpiar caché y regenerar metadatos
dnf clean all && dnf makecache
```

**Verificación:**

```bash
dnf repolist enabled | grep cuda
# Debe mostrar únicamente: cuda-rhel10-x86_64
```

---

## 🔧 Paso 2 — Instalar NVIDIA Driver y Kernel Module

> [!CAUTION]
> La RTX 5060 Ti (Blackwell) **requiere obligatoriamente el módulo de kernel abierto**. El módulo propietario (`kmod-nvidia`) no inicializa esta GPU y resultará en un sistema sin aceleración GPU. Seguir exactamente estos pasos.

### 2.1 — Instalar kernel-devel y headers

```bash
# Instalar headers y devel para el kernel en ejecución
dnf install -y kernel-devel-$(uname -r) kernel-headers-$(uname -r)
```

### 2.2 — Instalar driver y módulo abierto (Blackwell obligatorio)

```bash
# Instalar driver NVIDIA, soporte CUDA y firmware
# --nogpgcheck: necesario hasta que las claves RHEL10 estén en el repo oficial
dnf install -y --nogpgcheck \
  --enablerepo=cuda-rhel10-x86_64 \
  nvidia-driver \
  nvidia-driver-cuda \
  libnvidia-cfg \
  nvidia-gpu-firmware
```

### 2.3 — Compilar módulo DKMS

```bash
# Compilar e instalar el módulo de kernel automáticamente
dkms autoinstall --verbose
```

> [!NOTE]
> DKMS compilará el módulo `nvidia-open` para el kernel activo. Este proceso puede tardar 2–5 minutos dependiendo de la CPU. Si se instala un nuevo kernel en el futuro, DKMS recompilará el módulo automáticamente al siguiente arranque.

### 2.4 — Reiniciar el sistema

```bash
reboot
```

---

## ✅ Paso 3 — Verificar Driver NVIDIA

Tras el reinicio, comprobar que el driver cargó correctamente:

```bash
nvidia-smi
```

**Salida esperada:**

```
+-----------------------------------------------------------------------------------------+
| NVIDIA-SMI 595.71.05              Driver Version: 595.71.05      CUDA Version: 13.2     |
|-----------------------------------------+------------------------+----------------------+
| GPU  Name                 Persistence-M | Bus-Id          Disp.A | Volatile Uncorr. ECC |
| Fan  Temp   Perf          Pwr:Usage/Cap |           Memory-Usage | GPU-Util  Compute M. |
|                                         |                        |               MIG M. |
|=========================================+========================+======================|
|   0  NVIDIA GeForce RTX 5060 Ti     Off |   00000000:XX:00.0 Off |                  N/A |
|  0%   35C    P8              10W /  180W |       1MiB / 16311MiB  |      0%      Default |
+-----------------------------------------------------------------------------------------+
```

Los campos críticos a verificar:
- **Nombre:** `NVIDIA GeForce RTX 5060 Ti`
- **Memoria:** `16311 MiB`
- **Driver:** `595.71.05`
- **CUDA:** `13.2`

---

## 🦙 Paso 4 — Instalar Ollama con CUDA

### 4.1 — Instalación base

```bash
curl -fsSL https://ollama.com/install.sh | sh
```

### 4.2 — Deshabilitar backend ROCm

> [!WARNING]
> Ollama incluye un backend ROCm (para GPUs AMD) que **crashea en sistemas NVIDIA** e impide que Ollama detecte la GPU correctamente. Es obligatorio deshabilitarlo antes de iniciar el servicio.

```bash
# Renombrar directorio ROCm para deshabilitarlo
mv /usr/local/lib/ollama/rocm /usr/local/lib/ollama/rocm.disabled
```

### 4.3 — Configurar servicio para CUDA

```bash
# Crear directorio de override de systemd
mkdir -p /etc/systemd/system/ollama.service.d

# Escribir configuración de entorno
cat > /etc/systemd/system/ollama.service.d/override.conf << 'EOF'
[Service]
Environment="OLLAMA_HOST=0.0.0.0:11434"
Environment="CUDA_VISIBLE_DEVICES=0"
Environment="ROCR_VISIBLE_DEVICES="
Environment="HIP_VISIBLE_DEVICES="
EOF
```

### 4.4 — Aplicar configuración e iniciar

```bash
systemctl daemon-reload
systemctl enable ollama
systemctl restart ollama
```

### 4.5 — Verificar detección de GPU

```bash
# Ollama debe reportar CUDA0 y ~15.9 GiB disponibles
journalctl -u ollama -n 10 | grep -i cuda
```

**Salida esperada:**

```
ollama[XXXXX]: msg="inference compute" id=GPU-XXXXXXXX library=cuda compute=12.x ...
ollama[XXXXX]: msg="inference compute" name=CUDA0 total="15.9 GiB" available="15.9 GiB"
```

> [!NOTE]
> Si `grep cuda` no devuelve resultados, revisar `journalctl -u ollama -n 50` para identificar el error. El problema más común en este punto es ROCm no deshabilitado correctamente.

---

## 📦 Paso 5 — Descargar Modelos LLM

```bash
# Nous Research Hermes 3 — Optimizado para agentes y function calling
ollama pull hermes3:8b

# Meta Llama 3.1 — Modelo de propósito general
ollama pull llama3.1:8b

# Alibaba Qwen 2.5 — Mejor soporte de tool calling local (requiere ~9 GB VRAM)
ollama pull qwen2.5:14b
```

**Verificar modelos descargados:**

```bash
ollama list
```

**Salida esperada:**

```
NAME              ID              SIZE    MODIFIED
hermes3:8b        XXXXXXXXXXXX    4.9 GB  ...
llama3.1:8b       XXXXXXXXXXXX    4.9 GB  ...
qwen2.5:14b       XXXXXXXXXXXX    9.0 GB  ...
```

> [!TIP]
> Para correr `qwen2.5:14b`, Ollama necesita ~9 GB de VRAM. Con 16 GB disponibles en la RTX 5060 Ti, el modelo carga completamente en GPU sin offloading a RAM del sistema.

---

## ⚙️ Paso 6 — NVIDIA Device Plugin para Kubernetes

Este paso expone la GPU al cluster K3s para que los pods puedan solicitar `nvidia.com/gpu`.

### 6.1 — Generar CDI spec

```bash
# Generar especificación CDI (Container Device Interface)
nvidia-ctk cdi generate --output=/etc/cdi/nvidia.yaml
```

### 6.2 — Verificar ruta del socket kubelet

> [!IMPORTANT]
> K3s expone el socket de kubelet en la **ruta estándar de Kubernetes** (`/var/lib/kubelet/device-plugins/`), **NO** en la ruta propia de K3s (`/var/lib/rancher/k3s/agent/kubelet/device-plugins/`). El DaemonSet debe montar la ruta estándar o el device plugin no podrá registrarse.

```bash
# Verificar que el socket existe en la ruta estándar
ls /var/lib/kubelet/device-plugins/kubelet.sock
```

### 6.3 — Desplegar NVIDIA Device Plugin DaemonSet

Crear el archivo `nvidia-device-plugin-ds.yaml`:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: nvidia-device-plugin-daemonset
  namespace: kube-system
spec:
  selector:
    matchLabels:
      name: nvidia-device-plugin-ds
  updateStrategy:
    type: RollingUpdate
  template:
    metadata:
      labels:
        name: nvidia-device-plugin-ds
    spec:
      tolerations:
      - key: nvidia.com/gpu
        operator: Exists
        effect: NoSchedule
      priorityClassName: system-node-critical
      runtimeClassName: nvidia
      containers:
      - image: nvcr.io/nvidia/k8s-device-plugin:v0.17.0
        name: nvidia-device-plugin-ctr
        env:
        - name: FAIL_ON_INIT_ERROR
          value: "false"
        securityContext:
          privileged: true
        volumeMounts:
        - name: device-plugin
          mountPath: /var/lib/kubelet/device-plugins
      volumes:
      - name: device-plugin
        hostPath:
          # Ruta estándar K8s — K3s también la usa, NO usar la ruta de rancher
          path: /var/lib/kubelet/device-plugins
          type: Directory
```

Aplicar el DaemonSet:

```bash
kubectl apply -f nvidia-device-plugin-ds.yaml

# Esperar a que el pod esté Running
kubectl get pod -n kube-system -l name=nvidia-device-plugin-ds -w
```

### 6.4 — Verificar GPU allocatable en el nodo

```bash
kubectl get node -o jsonpath='{.items[0].status.allocatable}' \
  | python3 -m json.tool \
  | grep nvidia
```

**Salida esperada:**

```json
"nvidia.com/gpu": "1"
```

---

## 🤖 Paso 7 — Configurar Hermes Agent con OpenRouter

Esta configuración se realiza en el **ThinkPad (management cluster)**, no en el nodo Ryzen.

### 7.1 — Configurar API Key de OpenRouter

```bash
# Crear directorio de configuración de Hermes
mkdir -p /root/.hermes

# Escribir archivo de variables de entorno
cat > /root/.hermes/.env << 'EOF'
OPENROUTER_API_KEY=<tu-api-key-de-openrouter>
EOF

# Restringir permisos del archivo
chmod 600 /root/.hermes/.env
```

### 7.2 — Crear configuración base de Hermes

```bash
cat > /root/.hermes/config.yaml << 'EOF'
# Hermes Agent — Configuración base
# Provider: OpenRouter → GPT-4o
model: openai/gpt-4o

gateway:
  type: cli

tools:
  shell:
    enabled: true
    timeout: 120

memory:
  enabled: true

agent:
  name: <nombre-de-tu-agente>
EOF
```

### 7.3 — Verificar conexión

```bash
hermes "hola, ¿estás conectado?"
# Debe responder usando GPT-4o vía OpenRouter
```

---

## 🔌 Paso 8 — Opcional: Hermes con Ollama Local (sin API Key)

Esta configuración permite usar Hermes Agent apuntando al servidor Ollama del Ryzen, sin coste por API.

### 8.1 — Configurar endpoint personalizado

Salir de Hermes si está activo y ejecutar desde el shell:

```bash
# IMPORTANTE: si estás dentro de una sesión de Hermes, salir primero
# Escribir /exit dentro de Hermes, luego ejecutar en el shell:
hermes model
```

Seguir el menú interactivo con estas opciones:

```
→ Custom endpoint (enter URL manually)
URL:     http://<IP-RYZEN>:11434/v1
API key: ollama
Modo:    Chat Completions (opción 2)
Modelo:  qwen2.5:14b
Context: 131072
Nombre:  ollama-local
```

### 8.2 — Override de context length en config.yaml

```bash
# Agregar override de contexto al config existente
echo 'model.context_length: 131072' >> /root/.hermes/config.yaml
```

### 8.3 — Limitaciones conocidas de LLMs locales para agentes

> [!WARNING]
> Los modelos de **8B parámetros** (hermes3:8b, llama3.1:8b) tienden a generar tool calls como texto plano en lugar del formato JSON estructurado que Hermes Agent espera para ejecutar herramientas. Esto puede causar que el agente "hable sobre hacer cosas" en lugar de ejecutarlas.
>
> **Recomendación:** Usar `qwen2.5:14b` para tareas de agente que requieran function calling. Qwen 2.5 tiene mejor adherencia al formato estructurado de tool calls.

---

## 🔥 Troubleshooting

### ❌ `nvidia-smi: No devices were found`

**Causa:** La RTX 5060 Ti (arquitectura Blackwell) requiere el módulo de kernel abierto. El módulo propietario (`kmod-nvidia`) no inicializa esta GPU.

**Solución:**

```bash
# Verificar qué módulo está cargado
lsmod | grep nvidia

# Si está el módulo propietario, eliminar e instalar el open
dnf remove kmod-nvidia
dnf install -y kmod-nvidia-open-dkms
dkms autoinstall --verbose
reboot
```

---

### ❌ `Module nvidia not found`

**Causa:** DKMS no compiló el módulo correctamente, generalmente por falta de kernel-devel.

**Solución:**

```bash
# Verificar que kernel-devel coincide con el kernel en ejecución
dnf install -y kernel-devel-$(uname -r)

# Recompilar módulo
dkms autoinstall --verbose

# Ver log detallado si falla
dkms status
```

---

### ❌ Ollama usa CPU en lugar de GPU

**Causa:** El backend ROCm incluido en Ollama interfiere con la detección NVIDIA y crashea el proceso de inferencia.

**Solución:**

```bash
systemctl stop ollama
mv /usr/local/lib/ollama/rocm /usr/local/lib/ollama/rocm.disabled
systemctl start ollama

# Verificar
journalctl -u ollama -n 10 | grep -i "cuda\|gpu"
```

---

### ❌ Device Plugin no registra GPU en Kubernetes

**Causa:** El DaemonSet monta la ruta de K3s (`/var/lib/rancher/k3s/agent/kubelet/device-plugins/`) en lugar de la ruta estándar que K3s realmente usa.

**Diagnóstico:**

```bash
# Verificar ruta real del socket
ls -la /var/lib/kubelet/device-plugins/kubelet.sock

# Revisar logs del device plugin pod
kubectl logs -n kube-system -l name=nvidia-device-plugin-ds
```

**Solución:** Asegurarse de que el `hostPath` en el DaemonSet apunta a `/var/lib/kubelet/device-plugins` (ver Paso 6.3).

---

### ❌ Hermes con modelo local da error 401

**Causa:** El `provider` en `config.yaml` no coincide con el nombre de la configuración guardada por `hermes model`. Editar `config.yaml` manualmente puede dejar inconsistencias en el estado interno de Hermes.

**Solución:**

```bash
# No editar config.yaml manualmente para el provider custom.
# Usar siempre hermes model para gestionar providers.
hermes model
# Seleccionar el provider "ollama-local" existente o crear uno nuevo
```

---

### ❌ `hermes model` ejecutado como mensaje dentro de Hermes

**Causa:** Se intenta ejecutar `hermes model` estando dentro de una sesión interactiva de Hermes, y el agente lo interpreta como un mensaje.

**Solución:**

```bash
# Dentro de la sesión de Hermes, escribir:
/exit

# Una vez en el shell, ejecutar:
hermes model
```

---

## 🔌 Paso 9 — Integración de IA con el Portal OpenChoreo (Backstage Assistant)

OpenChoreo cuenta con un asistente inteligente integrado a nivel de frontend de Backstage, controlado por dos variables del Control Plane:
*   `OPENCHOREO_FEATURES_ASSISTANT_ENABLED`: Habilita/deshabilita la pestaña del asistente de chat en la UI.
*   `OPENCHOREO_PORTAL_ASSISTANT_URL`: Apunta al backend `portal-assistant` que actúa como puente / proxy hacia el LLM.

### 9.1 — Arquitectura de Integración con el Motor Local (Ryzen GPU)

Para lograr inferencia offline soberana 100% air-gapped en la plataforma, diseñamos dos flujos de integración:

#### Opción A: Enrutamiento Directo a Ollama Local (Inferencia Directa)
El servicio `portal-assistant` se conecta directamente al endpoint compatible con OpenAI expuesto por Ollama en el Ryzen:

```text
┌─────────────────────────────────┐
│     Consola OpenChoreo (UI)     │
└─────────────────────────────────┘
                 │
                 ▼ (FEATURE_ASSISTANT habilitado)
┌─────────────────────────────────┐
│    portal-assistant backend     │
└─────────────────────────────────┘
                 │
                 ▼ (OpenAI-compatible API URL)
┌─────────────────────────────────┐
│   Ollama en Ryzen (Puerto 11434)│
│          qwen2.5:14b            │
└─────────────────────────────────┘
```

**Configuración en `openchoreo-app.yaml`:**
Modificar el deployment de `portal-assistant` para inyectar las siguientes variables:
```yaml
        env:
        - name: OPENCHOREO_FEATURES_ASSISTANT_ENABLED
          value: "true"
        - name: OPENCHOREO_PORTAL_ASSISTANT_URL
          value: "http://portal-assistant.openchoreo-control-plane.svc.cluster.local:8080"
        - name: OPENAI_API_BASE
          value: "http://192.168.0.13:11434/v1"
        - name: OPENAI_API_KEY
          value: "ollama" # Requerido por la librería cliente pero ignorado por Ollama
        - name: OPENAI_MODEL
          value: "qwen2.5:14b"
```

#### Opción B: Integración Agéntica Avanzada con Hermes Agent
En lugar de inferencia estática, enrutamos las peticiones al **Hermes Agent** corriendo en la infraestructura. Esto permite al asistente de Backstage **ejecutar herramientas** (ej. consultar estado del clúster, listar namespaces de vcluster, consultar catálogos) a través de las capacidades de Tool Calling de `qwen2.5:14b`:

```text
┌─────────────────────────────────┐
│     Consola OpenChoreo (UI)     │
└─────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────┐
│    portal-assistant backend     │
└─────────────────────────────────┘
                 │
                 ▼ (Custom agent connector)
┌─────────────────────────────────┐
│          Hermes Agent           │ (Conexión al kube-api y logs del cluster)
└─────────────────────────────────┘
                 │
                 ▼ (Inferencia de Tool Calling)
┌─────────────────────────────────┐
│        Ollama en Ryzen          │
│          qwen2.5:14b            │
└─────────────────────────────────┘
```

**Configuración del Conector Hermes:**
Configure `portal-assistant` para que se comunique con el endpoint de servicio de Hermes:
```yaml
        env:
        - name: OPENAI_API_BASE
          value: "http://hermes-agent-service.openchoreo-control-plane.svc.cluster.local:8080/v1"
        - name: OPENAI_MODEL
          value: "hermes-agent"
```

---

## 🏁 Estado Esperado Final

Tras completar todos los pasos, el sistema debe mostrar el siguiente estado:

```bash
# Driver NVIDIA
nvidia-smi
# → NVIDIA GeForce RTX 5060 Ti | 16311 MiB | Driver 595.71.05 | CUDA 13.2

# Modelos Ollama disponibles
ollama list
# → hermes3:8b   llama3.1:8b   qwen2.5:14b

# GPU expuesta en Kubernetes
kubectl get node -o jsonpath='{.items[0].status.allocatable}' | python3 -m json.tool | grep nvidia
# → "nvidia.com/gpu": "1"

# Hermes Agent (management — ThinkPad)
hermes
# → gpt-4o │ openrouter   (configuración por defecto)
# → qwen2.5:14b │ ollama-local   (configuración local opcional)
```

| Verificación | Comando | Resultado esperado |
|---|---|---|
| Driver NVIDIA | `nvidia-smi` | RTX 5060 Ti, 16311 MiB, Driver 595.71.05 |
| CUDA disponible | `nvidia-smi \| grep CUDA` | CUDA Version: 13.2 |
| Ollama en GPU | `journalctl -u ollama -n5 \| grep cuda` | CUDA0, 15.9 GiB available |
| Modelos cargados | `ollama list` | hermes3:8b, llama3.1:8b, qwen2.5:14b |
| GPU en K8s | `kubectl get node -o json \| grep nvidia` | nvidia.com/gpu: "1" |
| Hermes conectado | `hermes "ping"` | Respuesta de GPT-4o vía OpenRouter |

---

*Playbook generado y validado el 2026-05-25. Última ejecución exitosa en nodo Ryzen con Rocky Linux 10 + RTX 5060 Ti (Blackwell GB206).*
