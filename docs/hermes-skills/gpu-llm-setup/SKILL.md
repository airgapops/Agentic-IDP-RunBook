# GPU + LLM Stack Setup Skill

## Descripción
Este skill guía la instalación y verificación del stack completo GPU + Ollama + Kubernetes Device Plugin en un nodo con GPU NVIDIA. Diseñado para infraestructura soberana / airgap.

**Cuándo usar este skill:**
- Al agregar un nodo con GPU NVIDIA al cluster
- Para diagnosticar problemas con Ollama o el device plugin de K8s
- Para verificar que la GPU está correctamente integrada en Kubernetes

---

## Pasos del Skill

### 1. Verificar GPU detectada a nivel hardware

```bash
# El agente debe ejecutar esto PRIMERO para confirmar hardware
lspci | grep -i nvidia
cat /proc/driver/nvidia/gpus/*/information 2>/dev/null
ls /dev/nvidia* 2>/dev/null
```

**Interpretar resultado:**
- Si no aparece nada en `lspci` → problema de hardware/PCIe
- Si aparece en `lspci` pero no en `/dev/nvidia*` → driver no cargado

---

### 2. Verificar driver NVIDIA

```bash
nvidia-smi
```

**Troubleshooting si falla:**

```bash
# Ver qué módulos están cargados
lsmod | grep nvidia

# Ver si DKMS tiene el módulo compilado
dkms status

# CRÍTICO para GPUs Blackwell (RTX 5000 series): usar open module
# El módulo propietario NO funciona con Blackwell
dkms autoinstall --verbose
modprobe nvidia && modprobe nvidia-uvm
nvidia-smi
```

> ⚠️ **RTX 5060 Ti y serie Blackwell**: requieren OBLIGATORIAMENTE el módulo abierto de NVIDIA (`nvidia-open` / `kmod-nvidia-open-dkms`). El módulo propietario deja la GPU en estado `Video BIOS: ??.??.??.??.??` y `nvidia-smi` devuelve "No devices were found".

---

### 3. Verificar Ollama usando GPU

```bash
# Ver logs de inicio de Ollama
journalctl -u ollama -n 20 | grep -iE "cuda|gpu|library|compute"
```

**Resultado esperado:**
```
inference compute id=GPU-xxxx library=CUDA name=CUDA0 description="NVIDIA GeForce RTX ..." total="15.9 GiB"
```

**Si Ollama usa CPU en lugar de GPU:**
```bash
# Ollama incluye backend ROCm (AMD) que crashea en NVIDIA
# Solución: deshabilitar ROCm
mv /usr/local/lib/ollama/rocm /usr/local/lib/ollama/rocm.disabled

# Configurar variables de entorno
cat > /etc/systemd/system/ollama.service.d/override.conf << 'EOF'
[Service]
Environment="OLLAMA_HOST=0.0.0.0:11434"
Environment="CUDA_VISIBLE_DEVICES=0"
Environment="ROCR_VISIBLE_DEVICES="
Environment="HIP_VISIBLE_DEVICES="
EOF

systemctl daemon-reload && systemctl restart ollama
journalctl -u ollama -n 10 | grep -i cuda
```

---

### 4. Verificar Kubernetes Device Plugin

```bash
# Verificar que kubelet.sock existe (ruta ESTÁNDAR K8s, no la de K3s)
ls /var/lib/kubelet/device-plugins/kubelet.sock

# Ver estado del device plugin
kubectl get pods -n kube-system | grep nvidia

# Verificar GPU allocatable en el nodo
kubectl get node -o jsonpath='{.items[*].status.allocatable}' | python3 -m json.tool | grep nvidia
```

**Resultado esperado:** `"nvidia.com/gpu": "1"`

**Si el device plugin no registra GPU:**
```bash
# CAUSA COMÚN: DaemonSet montando ruta incorrecta de K3s
# K3s usa la ruta ESTÁNDAR de K8s:
#   ✅ /var/lib/kubelet/device-plugins/  ← kubelet.sock está aquí
#   ❌ /var/lib/rancher/k3s/agent/kubelet/device-plugins/  ← NO usar

kubectl describe daemonset nvidia-device-plugin-daemonset -n kube-system | grep -A5 "Volumes"
# Si muestra /var/lib/rancher/k3s... → hay que redeploy con la ruta correcta
```

---

### 5. Test de inferencia con GPU

```bash
# Ejecutar modelo y monitorear GPU simultáneamente
/usr/local/bin/ollama run hermes3:8b "escribe un haiku sobre kubernetes" &
sleep 3
nvidia-smi --query-gpu=memory.used,memory.total,utilization.gpu --format=csv,noheader
# Debe mostrar: ~5000 MiB usados, utilization > 0%
```

---

### 6. Verificar estado completo del stack

```bash
echo "=== nvidia-smi ==="
nvidia-smi --query-gpu=name,memory.total,driver_version --format=csv,noheader

echo "=== Ollama modelos disponibles ==="
/usr/local/bin/ollama list

echo "=== GPU en Kubernetes ==="
kubectl get node -o custom-columns="NODE:.metadata.name,GPU:.status.allocatable.nvidia\.com/gpu"

echo "=== Device Plugin ==="
kubectl get pods -n kube-system -l name=nvidia-device-plugin-ds
```

---

## Recomendaciones de modelos por caso de uso

| Caso de uso | Modelo recomendado | VRAM | Calidad tool calling |
|---|---|---|---|
| Agente K8s (mejor calidad) | `qwen2.5:14b` | 9 GB | ⭐⭐⭐⭐ |
| Chat / Q&A general | `hermes3:8b` | 5 GB | ⭐⭐⭐ |
| Razonamiento complejo | `deepseek-r1:14b` | 9 GB | ⭐⭐⭐⭐ |
| Código | `qwen2.5-coder:14b` | 9 GB | ⭐⭐⭐⭐ |

> 💡 Con 16GB VRAM (RTX 5060 Ti) puedes correr modelos de hasta 14B con contexto de 32K sin problema.

---

## Configurar Hermes con Ollama local (sin API key)

```bash
# En el nodo con Hermes instalado:
hermes model
# → Seleccionar: "Custom endpoint (enter URL manually)"
# URL: http://<IP-NODO-GPU>:11434/v1
# API key: ollama
# Modo: Chat Completions (opción 2)
# Modelo: qwen2.5:14b
# Context length: 131072
# Nombre: ollama-local

# Agregar override de contexto (Hermes requiere mínimo 64K):
echo 'model.context_length: 131072' >> /root/.hermes/config.yaml
```

> ⚠️ **Limitación actual**: Los modelos locales de 7-14B parámetros tienen soporte parcial de tool-calling en Hermes Agent. Para uso en producción con herramientas kubectl complejas, se recomienda GPT-4o vía OpenRouter.

---

## Referencias
- Playbook completo: [PB-06-gpu-llm-stack.md](../playbooks/PB-06-gpu-llm-stack.md)
- NVIDIA open kernel module: https://github.com/NVIDIA/open-gpu-kernel-modules
- Ollama: https://ollama.com
- NVIDIA Device Plugin: https://github.com/NVIDIA/k8s-device-plugin
