#!/bin/bash
set -e
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

echo "==> Creating openchoreo-data-plane namespace..."
kubectl create namespace openchoreo-data-plane --dry-run=client -o yaml | kubectl apply -f -

echo "==> Waiting for cluster-gateway CA certificate..."
kubectl wait -n openchoreo-control-plane \
  --for=condition=Ready certificate/cluster-gateway-ca --timeout=120s 2>/dev/null || true

echo "==> Copying cluster-gateway CA to data plane namespace..."
CA_CRT=$(kubectl get secret cluster-gateway-ca \
  -n openchoreo-control-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)

kubectl create configmap cluster-gateway-ca \
  --from-literal=ca.crt="$CA_CRT" \
  -n openchoreo-data-plane --dry-run=client -o yaml | kubectl apply -f -

echo "==> Installing OpenChoreo Data Plane..."
helm upgrade --install openchoreo-data-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
  --version 1.1.0 \
  --namespace openchoreo-data-plane \
  --values https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/install/k3d/single-cluster/values-dp.yaml \
  --wait --timeout 300s

echo "==> Waiting for cluster-agent TLS secret..."
kubectl wait -n openchoreo-data-plane \
  --for=jsonpath='{.data.ca\.crt}' secret/cluster-agent-tls --timeout=120s

echo "==> Registering Data Plane with Control Plane..."
AGENT_CA=$(kubectl get secret cluster-agent-tls \
  -n openchoreo-data-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)

cat <<DPEOF | kubectl apply -f -
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterDataPlane
metadata:
  name: default
spec:
  planeID: default
  clusterAgent:
    clientCA:
      value: |
$(echo "$AGENT_CA" | sed 's/^/        /')
  secretStoreRef:
    name: default
  gateway:
    ingress:
      external:
        http:
          host: openchoreoapis.localhost
          listenerName: http
          port: 19080
        name: gateway-default
        namespace: openchoreo-data-plane
DPEOF

echo "DATA_PLANE_COMPLETE"
