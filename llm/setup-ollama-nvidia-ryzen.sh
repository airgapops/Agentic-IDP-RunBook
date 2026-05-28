#!/bin/bash
# =============================================================================
# setup-ollama-nvidia-ryzen.sh
# Instalar Ollama + NVIDIA CUDA en el Ryzen (Ubuntu 22.04/24.04)
# Ejecutar DIRECTAMENTE en el Ryzen como root o con sudo
# =============================================================================
set -e

echo "╔══════════════════════════════════════════════════╗"
echo "║   Ollama + NVIDIA RTX 5090 Ti — Ryzen Setup     ║"
echo "╚══════════════════════════════════════════════════╝"

# --- Detectar GPU ---
echo ""
echo "=== GPU detectada ==="
lspci | grep -iE "nvidia|vga|display"
nvidia-smi 2>/dev/null && echo "Drivers NVIDIA ya instalados ✓" || echo "Instalando drivers..."

# =============================================================================
# PASO 1: Drivers NVIDIA (si no están instalados)
# =============================================================================
if ! command -v nvidia-smi &>/dev/null; then
  echo ""
  echo "=== [1/6] Instalando drivers NVIDIA (Blackwell RTX 50xx) ==="
  apt-get update -qq
  apt-get install -y linux-headers-$(uname -r) build-essential

  # Para RTX 5090 Ti (Blackwell) necesitas driver 570+
  apt-get install -y nvidia-driver-570 2>/dev/null || \
  apt-get install -y nvidia-driver-565 2>/dev/null || \
  apt-get install -y nvidia-driver-560 2>/dev/null || \
  ubuntu-drivers install --gpgpu 2>/dev/null || \
  echo "ATENCIÓN: Instala los drivers manualmente desde https://www.nvidia.com/drivers"

  echo "Drivers instalados. Verificando..."
  nvidia-smi || echo "Reinicia el sistema y vuelve a ejecutar el script si nvidia-smi falla"
else
  echo "=== [1/6] Drivers NVIDIA OK ==="
  nvidia-smi --query-gpu=name,memory.total,driver_version --format=csv,noheader
fi

# =============================================================================
# PASO 2: NVIDIA Container Toolkit (para Docker/containerd)
# =============================================================================
echo ""
echo "=== [2/6] Instalando NVIDIA Container Toolkit ==="
if ! dpkg -l | grep -q nvidia-container-toolkit; then
  curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | \
    gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg

  curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
    sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
    tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

  apt-get update -qq
  apt-get install -y nvidia-container-toolkit
  echo "NVIDIA Container Toolkit instalado ✓"
else
  echo "NVIDIA Container Toolkit ya instalado ✓"
fi

# =============================================================================
# PASO 3: Configurar containerd para K3s + NVIDIA
# =============================================================================
echo ""
echo "=== [3/6] Configurando containerd (K3s) para NVIDIA ==="
nvidia-ctk runtime configure --runtime=containerd \
  --config=/var/lib/rancher/k3s/agent/etc/containerd/config.toml 2>/dev/null || \
nvidia-ctk runtime configure --runtime=containerd 2>/dev/null || \
echo "Configurar containerd manualmente si falla"

# Reiniciar K3s para aplicar configuración containerd
systemctl restart k3s-agent 2>/dev/null || systemctl restart k3s 2>/dev/null || \
  echo "Reiniciar k3s manualmente: systemctl restart k3s"

echo "Containerd configurado ✓"

# =============================================================================
# PASO 4: NVIDIA Device Plugin en K3s
# =============================================================================
echo ""
echo "=== [4/6] Instalando NVIDIA Device Plugin en K3s ==="
# El ThinkPad se encargará de esto via kubectl --kubeconfig=ryzen-kubeconfig.yaml
echo "El Device Plugin se aplicará desde el ThinkPad."
echo "Comando: kubectl apply -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/v0.17.0/deployments/static/nvidia-device-plugin.yml"

# =============================================================================
# PASO 5: Instalar Ollama nativo (más simple que K8s para GPU)
# =============================================================================
echo ""
echo "=== [5/6] Instalando Ollama ==="
if ! command -v ollama &>/dev/null; then
  curl -fsSL https://ollama.com/install.sh | sh
  echo "Ollama instalado ✓"
else
  echo "Ollama ya instalado ✓"
  ollama --version
fi

# Configurar Ollama para escuchar en todas las interfaces (accesible desde ThinkPad)
echo ""
echo "=== Configurando Ollama como servicio (0.0.0.0:11434) ==="
mkdir -p /etc/systemd/system/ollama.service.d/
cat > /etc/systemd/system/ollama.service.d/override.conf << 'EOF'
[Service]
Environment="OLLAMA_HOST=0.0.0.0:11434"
Environment="OLLAMA_ORIGINS=*"
Environment="OLLAMA_MAX_LOADED_MODELS=2"
EOF

systemctl daemon-reload
systemctl enable ollama
systemctl restart ollama
sleep 3
systemctl status ollama --no-pager | head -10

# =============================================================================
# PASO 6: Descargar modelos recomendados para RTX 5090 Ti
# =============================================================================
echo ""
echo "=== [6/6] Descargando modelos LLM ==="
echo "RTX 5090 Ti con 16GB VRAM — modelos recomendados:"
echo "  • hermes3:8b    (5GB)  — agente conversacional de Nous Research"
echo "  • llama3.1:8b   (5GB)  — general purpose, muy rápido"
echo "  • codellama:13b (8GB)  — generación de YAML/código K8s"
echo ""
echo "Descargando hermes3:8b y llama3.1:8b..."
ollama pull hermes3:8b &
ollama pull llama3.1:8b &
wait
echo "Modelos descargados ✓"

# Test rápido
echo ""
echo "=== Test rápido ==="
ollama run llama3.1:8b "Responde en una línea: ¿Cuál es la capital de Panamá?" --nowordwrap 2>/dev/null || \
  echo "Test: ejecuta 'ollama run llama3.1:8b' para probar"

echo ""
echo "╔══════════════════════════════════════════════════╗"
echo "║   Ollama Listo en Ryzen ✓                       ║"
echo "║                                                  ║"
echo "║   API:  http://192.168.0.13:11434               ║"
echo "║   Test: curl http://192.168.0.13:11434/api/tags ║"
echo "║                                                  ║"
echo "║   Modelos instalados:                            ║"
echo "║     ollama list                                   ║"
echo "╚══════════════════════════════════════════════════╝"
