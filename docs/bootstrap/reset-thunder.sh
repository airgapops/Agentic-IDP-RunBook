#!/bin/bash
set -e
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

echo "==> Scaling down Thunder to delete PVC..."
kubectl scale deployment/thunder-deployment -n thunder --replicas=0
sleep 3

echo "==> Deleting Thunder PVC (forces fresh bootstrap)..."
kubectl delete pvc thunder-database-pvc -n thunder --wait=true

echo "==> Scaling Thunder back up..."
kubectl scale deployment/thunder-deployment -n thunder --replicas=1

echo "==> Waiting for Thunder to be ready..."
kubectl wait -n thunder --for=condition=available --timeout=120s deployment/thunder-deployment

echo "==> Waiting for bootstrap to complete..."
sleep 15

echo "==> Checking if bootstrap created the OAuth app..."
kubectl exec -n thunder deployment/thunder-deployment -- curl -s http://localhost:8090/.well-known/openid-configuration | grep -o '"issuer":"[^"]*"'

echo ""
echo "THUNDER_RESET_COMPLETE"
