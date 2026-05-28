#!/bin/bash
# =============================================================================
# host-installer.sh — Se ejecuta dentro del namespace del host (nsenter)
# Instala NVIDIA drivers + Container Toolkit + Ollama en Rocky Linux 10
# =============================================================================
set -e

log() { echo "[$(date '+%H:%M:%S')] $*"; }

log "=== HOST: $(cat /etc/os-release | grep PRETTY_NAME | cut -d= -f2) ==="
log "Kernel: $(uname -r)"

# ─────────────────────────────────────────────────────────────────────────────
# 1. NVIDIA Drivers
# ─────────────────────────────────────────────────────────────────────────────
log "--- Verificando NVIDIA ---"
if nvidia-smi &>/dev/null; then
  log "NVIDIA ya instalado:"
  nvidia-smi --query-gpu=name,memory.total,driver_version --format=csv,noheader
else
  log "Instalando NVIDIA drivers para Rocky Linux 10..."
  dnf install -y epel-release 2>/dev/null || true

  # Repo CUDA de NVIDIA (RHEL9 funciona en Rocky 10)
  curl -s -L https://developer.download.nvidia.com/compute/cuda/repos/rhel9/x86_64/cuda-rhel9.repo \
    -o /etc/yum.repos.d/cuda-rhel9.repo

  # Instalar drivers open (más compatible con kernels nuevos)
  dnf module install -y nvidia-driver:open-dkms 2>/dev/null || \
  dnf install -y nvidia-open-dkms 2>/dev/null || \
  dnf install -y cuda-drivers-open 2>/dev/null || \
  dnf install -y nvidia-driver 2>/dev/null || \
    log "WARN: Drivers NVIDIA no disponibles — reinicia y verifica"

  log "Verificando nvidia-smi..."
  nvidia-smi 2>/dev/null && log "Drivers OK" || log "Necesita reinicio del host"
fi

# ─────────────────────────────────────────────────────────────────────────────
# 2. NVIDIA Container Toolkit
# ─────────────────────────────────────────────────────────────────────────────
log "--- NVIDIA Container Toolkit ---"
if rpm -q nvidia-container-toolkit &>/dev/null; then
  log "nvidia-container-toolkit ya instalado"
else
  curl -s -L https://nvidia.github.io/libnvidia-container/stable/rpm/nvidia-container-toolkit.repo \
    -o /etc/yum.repos.d/nvidia-container-toolkit.repo
  # Deshabilitar GPG check (la key de NVIDIA tiene problemas de fecha en 2026)
  sed -i 's/gpgcheck=1/gpgcheck=0/g' /etc/yum.repos.d/nvidia-container-toolkit.repo
  dnf install -y --nogpgcheck nvidia-container-toolkit
  log "nvidia-container-toolkit instalado"
fi

# ─────────────────────────────────────────────────────────────────────────────
# 3. Configurar containerd K3s para NVIDIA
# ─────────────────────────────────────────────────────────────────────────────
log "--- Configurando containerd K3s ---"
CONTAINERD_CONFIG="/var/lib/rancher/k3s/agent/etc/containerd/config.toml"
if [ -f "$CONTAINERD_CONFIG" ]; then
  nvidia-ctk runtime configure --runtime=containerd --config="$CONTAINERD_CONFIG" 2>/dev/null \
    && log "containerd configurado con NVIDIA runtime" \
    || log "WARN: nvidia-ctk runtime configure falló"
fi

# También configurar el containerd default
nvidia-ctk runtime configure --runtime=containerd 2>/dev/null || true

# Reiniciar K3s (el servicio que exista)
systemctl restart k3s 2>/dev/null || systemctl restart k3s-agent 2>/dev/null || true
sleep 5
log "K3s reiniciado"

# ─────────────────────────────────────────────────────────────────────────────
# 4. Instalar Ollama
# ─────────────────────────────────────────────────────────────────────────────
log "--- Instalando Ollama ---"
if command -v ollama &>/dev/null; then
  log "Ollama ya instalado: $(ollama --version)"
else
  log "Instalando dependencia zstd..."
  dnf install -y zstd 2>/dev/null || true
  log "Descargando e instalando Ollama..."
  curl -fsSL https://ollama.com/install.sh | sh
  log "Ollama instalado"
fi

# ─────────────────────────────────────────────────────────────────────────────
# 5. Configurar Ollama service (escuchar en 0.0.0.0)
# ─────────────────────────────────────────────────────────────────────────────
log "--- Configurando servicio Ollama ---"
mkdir -p /etc/systemd/system/ollama.service.d/
printf '[Service]\nEnvironment="OLLAMA_HOST=0.0.0.0:11434"\nEnvironment="OLLAMA_ORIGINS=*"\nEnvironment="OLLAMA_MAX_LOADED_MODELS=2"\n' \
  > /etc/systemd/system/ollama.service.d/override.conf

# Abrir firewall
firewall-cmd --permanent --add-port=11434/tcp 2>/dev/null || true
firewall-cmd --reload 2>/dev/null || true

systemctl daemon-reload
systemctl enable ollama
systemctl restart ollama
sleep 5

STATUS=$(systemctl is-active ollama)
log "Ollama status: $STATUS"

# ─────────────────────────────────────────────────────────────────────────────
# 6. Pull modelos
# ─────────────────────────────────────────────────────────────────────────────
log "--- Descargando modelos LLM ---"
log "Pulling hermes3:8b (5GB)..."
ollama pull hermes3:8b &
PULL1_PID=$!

log "Pulling llama3.1:8b (5GB)..."
ollama pull llama3.1:8b &
PULL2_PID=$!

wait $PULL1_PID && log "hermes3:8b OK" || log "WARN: hermes3:8b failed"
wait $PULL2_PID && log "llama3.1:8b OK" || log "WARN: llama3.1:8b failed"

# ─────────────────────────────────────────────────────────────────────────────
# 7. Verificación final
# ─────────────────────────────────────────────────────────────────────────────
log ""
log "=== VERIFICACIÓN FINAL ==="
nvidia-smi --query-gpu=name,memory.total,utilization.gpu --format=csv,noheader 2>/dev/null \
  && log "GPU: OK" || log "GPU: verificar drivers (puede necesitar reinicio)"

curl -sf http://localhost:11434/api/tags | python3 -c \
  "import sys,json; d=json.load(sys.stdin); [print(f'  - {m[\"name\"]}') for m in d['models']]; print(f'Ollama OK: {len(d[\"models\"])} modelos')" \
  2>/dev/null || log "Ollama API: verificar"

log "=== DONE ==="
