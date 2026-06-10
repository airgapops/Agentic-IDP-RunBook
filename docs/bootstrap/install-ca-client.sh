#!/bin/bash
# =============================================================================
# Install Zerops CA on Client Machines
# Run this on any laptop/workstation that needs to access *.zerops.internal
# =============================================================================
set -e

THINKPAD_IP="${1:-192.168.0.10}"
CA_FILE="/tmp/zerops-direct-ca.crt"

echo "===> Downloading CA from ThinkPad ($THINKPAD_IP)..."
scp -o StrictHostKeyChecking=no rooter@${THINKPAD_IP}:/tmp/zerops-direct-ca.crt "$CA_FILE" 2>/dev/null || {
  echo "SCP failed. Paste the CA cert content below (Ctrl+D when done):"
  cat > "$CA_FILE"
}

echo "===> CA Certificate Info:"
openssl x509 -in "$CA_FILE" -noout -subject -dates

OS=$(uname -s)
case "$OS" in
  Darwin)
    echo "===> Installing CA on macOS..."
    sudo security import "$CA_FILE" -k /Library/Keychains/System.keychain
    echo ""
    echo "IMPORTANT: Open Keychain Access, find 'Zerops Internal Direct CA',"
    echo "double-click → Trust → 'Always Trust'"
    echo "Then restart Chrome."
    ;;
  Linux)
    echo "===> Installing CA on Linux..."
    sudo cp "$CA_FILE" /usr/local/share/ca-certificates/zerops-direct-ca.crt
    sudo update-ca-certificates
    echo "Restart your browser."
    ;;
  *)
    echo "Unsupported OS: $OS. Manually import $CA_FILE into your trust store."
    ;;
esac

echo ""
echo "===> Test:"
echo "  curl -s https://choreo.zerops.internal/ -o /dev/null -w '%{http_code}'"
echo ""
echo "DONE"
