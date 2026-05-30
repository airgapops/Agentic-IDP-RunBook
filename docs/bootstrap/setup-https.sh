#!/bin/bash
set -e
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

echo "==> Step 1: Create TLS certificates for API and Thunder..."

# Certificate for api.choreo.zerops.internal
kubectl apply -f - <<'EOF'
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: api-choreo-cert
  namespace: default
spec:
  secretName: api-choreo-tls-secret
  dnsNames:
    - api.choreo.zerops.internal
  issuerRef:
    name: zerops-ca-issuer
    kind: ClusterIssuer
EOF

# Certificate for thunder.zerops.internal
kubectl apply -f - <<'EOF'
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: thunder-cert
  namespace: default
spec:
  secretName: thunder-tls-secret
  dnsNames:
    - thunder.zerops.internal
  issuerRef:
    name: zerops-ca-issuer
    kind: ClusterIssuer
EOF

echo "Waiting for certificates to be ready..."
sleep 10
kubectl get certificates -n default

echo ""
echo "==> Step 2: Add HTTPS listeners to zerops-gateway..."

kubectl patch gateway zerops-gateway -n default --type='json' -p='[
  {"op": "add", "path": "/spec/listeners/-", "value": {
    "name": "https-api-choreo",
    "hostname": "api.choreo.zerops.internal",
    "port": 443,
    "protocol": "HTTPS",
    "tls": {
      "mode": "Terminate",
      "certificateRefs": [{"kind": "Secret", "name": "api-choreo-tls-secret"}]
    },
    "allowedRoutes": {"namespaces": {"from": "All"}}
  }},
  {"op": "add", "path": "/spec/listeners/-", "value": {
    "name": "https-thunder",
    "hostname": "thunder.zerops.internal",
    "port": 443,
    "protocol": "HTTPS",
    "tls": {
      "mode": "Terminate",
      "certificateRefs": [{"kind": "Secret", "name": "thunder-tls-secret"}]
    },
    "allowedRoutes": {"namespaces": {"from": "All"}}
  }}
]'

echo ""
echo "==> Step 3: Create HTTPRoutes for OpenChoreo services through zerops-gateway..."

# Route for choreo.zerops.internal -> backstage
kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: choreo-https-route
  namespace: default
spec:
  parentRefs:
    - name: zerops-gateway
      namespace: default
      sectionName: https-choreo
  hostnames:
    - choreo.zerops.internal
  rules:
    - backendRefs:
        - name: backstage
          namespace: openchoreo-control-plane
          port: 7007
EOF

# Route for api.choreo.zerops.internal -> openchoreo-api
kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: api-choreo-https-route
  namespace: default
spec:
  parentRefs:
    - name: zerops-gateway
      namespace: default
      sectionName: https-api-choreo
  hostnames:
    - api.choreo.zerops.internal
  rules:
    - backendRefs:
        - name: openchoreo-api
          namespace: openchoreo-control-plane
          port: 8080
EOF

# Route for thunder.zerops.internal -> thunder
kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: thunder-https-route
  namespace: default
spec:
  parentRefs:
    - name: zerops-gateway
      namespace: default
      sectionName: https-thunder
  hostnames:
    - thunder.zerops.internal
  rules:
    - backendRefs:
        - name: thunder
          namespace: thunder
          port: 8090
EOF

echo ""
echo "==> Step 4: Create ReferenceGrants for cross-namespace routing..."

# Grant for openchoreo-control-plane
kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-zerops-gw-to-cp
  namespace: openchoreo-control-plane
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      namespace: default
  to:
    - group: ""
      kind: Service
EOF

# Grant for thunder
kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-zerops-gw-to-thunder
  namespace: thunder
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      namespace: default
  to:
    - group: ""
      kind: Service
EOF

echo ""
echo "==> Step 5: Verify..."
kubectl get httproute -n default --no-headers
kubectl get gateway zerops-gateway -n default -o jsonpath='{range .spec.listeners[*]}{.name}{" -> "}{.hostname}{"\n"}{end}'

echo ""
echo "HTTPS_ROUTES_COMPLETE"
