#!/bin/bash
set -e
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# Create openbao token secret
kubectl create secret generic openbao-token --from-literal=token=root -n openbao --dry-run=client -o yaml | kubectl apply -f -

# Create ClusterSecretStore
cat <<'CSSEOF' | kubectl apply -f -
apiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: default
spec:
  provider:
    vault:
      server: "http://openbao.openbao.svc.cluster.local:8200"
      path: "secret"
      version: "v2"
      auth:
        tokenSecretRef:
          name: openbao-token
          namespace: openbao
          key: token
CSSEOF

echo "ClusterSecretStore created successfully"

# Create openchoreo-control-plane namespace
kubectl create namespace openchoreo-control-plane --dry-run=client -o yaml | kubectl apply -f -

# Create Backstage ExternalSecrets
cat <<'ESEOF' | kubectl apply -f -
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: backstage-secrets
  namespace: openchoreo-control-plane
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: backstage-secrets
  data:
  - secretKey: backend-secret
    remoteRef:
      key: backstage-backend-secret
      property: value
  - secretKey: client-secret
    remoteRef:
      key: backstage-client-secret
      property: value
  - secretKey: jenkins-api-key
    remoteRef:
      key: backstage-jenkins-api-key
      property: value
ESEOF

echo "Backstage ExternalSecrets created"
echo "PHASE1_COMPLETE"
