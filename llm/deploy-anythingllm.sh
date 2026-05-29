#!/bin/bash
# =============================================================================
# deploy-anythingllm.sh — Script de despliegue de AnythingLLM
# Ejecutar desde el Mac: bash llm/deploy-anythingllm.sh <OPENROUTER_API_KEY>
# =============================================================================

set -e

KUBECONFIG_THINKPAD="$(dirname "$0")/../ryzen-kubeconfig.yaml"
# Usar el kubeconfig del ThinkPad para desplegar ahí
KUBECONFIG_MGMT="/Users/jonnhy/Documents/airgapOps/thinkpad-kubeconfig.yaml"

API_KEY="${1:-}"
DOMAIN="llm.zerops.internal"
PI_IP="192.168.0.9"
THINKPAD_IP="192.168.0.10"
PDNS_API_KEY="super-secret-api-key-zerops"

if [ -z "$API_KEY" ]; then
  echo "❌ Uso: bash llm/deploy-anythingllm.sh <TU_OPENROUTER_API_KEY>"
  exit 1
fi

echo "🚀 Desplegando AnythingLLM en zerops.internal..."

# 1. Crear namespace
echo "📦 Creando namespace llm-system..."
sshpass -p 'Panama2028#...' ssh -o StrictHostKeyChecking=no rooter@${THINKPAD_IP} \
  "echo 'Panama2028#...' | sudo -S kubectl apply -f - << 'EOF'
apiVersion: v1
kind: Namespace
metadata:
  name: llm-system
EOF"

# 2. Crear secret con API key
echo "🔑 Creando secret con OpenRouter API Key..."
sshpass -p 'Panama2028#...' ssh -o StrictHostKeyChecking=no rooter@${THINKPAD_IP} \
  "echo 'Panama2028#...' | sudo -S kubectl create secret generic anythingllm-secrets \
  --namespace=llm-system \
  --from-literal=OPENROUTER_API_KEY='${API_KEY}' \
  --dry-run=client -o yaml | sudo kubectl apply -f -"

# 3. Aplicar manifiestos
echo "📋 Aplicando manifiestos..."
sshpass -p 'Panama2028#...' ssh -o StrictHostKeyChecking=no rooter@${THINKPAD_IP} \
  "cat > /tmp/anythingllm.yaml" < "$(dirname "$0")/anythingllm-deployment.yaml"

# 4. Registrar DNS en PowerDNS
echo "🌐 Registrando ${DOMAIN} en PowerDNS..."
curl -s -X PATCH \
  -H "X-API-Key: ${PDNS_API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"rrsets\":[{\"name\":\"llm.zerops.internal.\",\"type\":\"A\",\"ttl\":300,\"changetype\":\"REPLACE\",\"records\":[{\"content\":\"${THINKPAD_IP}\",\"disabled\":false}]}]}" \
  "http://${PI_IP}:8081/api/v1/servers/localhost/zones/zerops.internal." && echo "DNS ✅"

echo "⏳ Esperando que AnythingLLM arranque (puede tardar 60-90s en descargar imagen)..."
echo "✅ Cuando esté listo: https://${DOMAIN}"
echo ""
echo "🔐 Token de acceso: zerops-llm-2028 (cámbialo desde Settings)"
echo "📡 OpenRouter modelo default: openai/gpt-4o"
