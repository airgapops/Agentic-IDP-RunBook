---
name: expose-service
description: >-
  Expose a service on the Ryzen Data Plane via HTTPS with a custom subdomain.
  Creates DNS record, TLS certificate, Gateway HTTPS listener, and HTTPRoute.
  Use when asked to: expose a service, create HTTPS endpoint, add subdomain,
  make service accessible from browser.
version: 1.0.0
metadata:
  author: platform-team
  tags: [https, tls, dns, gateway, expose, kgateway]
---

# 🔒 SKILL: Expose Service via HTTPS on Ryzen

## When to Use
- User asks to expose a service with HTTPS
- User wants `https://{name}.zerops.internal` for a Ryzen service
- User says "make X accessible", "add HTTPS to X", "create subdomain for X"

## Architecture

```
Browser → DNS (Pi) → kgateway (Ryzen:443) → HTTPRoute → Service → Pod
```

## Pre-requisites
- Service running on Ryzen (deployed via openchoreo-deploy skill)
- kgateway installed on Ryzen (namespace: kgateway-system)
- cert-manager with `zerops-direct-issuer` ClusterIssuer on Ryzen
- PowerDNS API accessible on 192.168.0.9:8081

## Instructions

### Step 1: Create DNS Record (PowerDNS API)

```bash
# Execute from any machine with network access to Pi
GATEWAY_IP="192.168.0.242"  # Ryzen kgateway LoadBalancer IP
SERVICE_NAME="{name}"

curl -s -X PATCH http://192.168.0.9:8081/api/v1/servers/localhost/zones/zerops.internal. \
  -H "X-API-Key: <POWERDNS_API_KEY>" \
  -H "Content-Type: application/json" \
  -d "{\"rrsets\": [{
    \"name\": \"${SERVICE_NAME}.zerops.internal.\",
    \"type\": \"A\",
    \"ttl\": 300,
    \"changetype\": \"REPLACE\",
    \"records\": [{\"content\": \"${GATEWAY_IP}\", \"disabled\": false}]
  }]}"
echo "DNS_CREATED: ${SERVICE_NAME}.zerops.internal → ${GATEWAY_IP}"
```

### Step 2: Create TLS Certificate (Ryzen)

```bash
# Execute on Ryzen (192.168.0.13)
sudo /usr/local/bin/kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: {name}-cert
  namespace: openchoreo-data-plane
spec:
  secretName: {name}-tls-secret
  duration: 2160h       # 90 days
  renewBefore: 360h     # Renew 15 days before
  dnsNames:
    - {name}.zerops.internal
  issuerRef:
    name: zerops-direct-issuer
    kind: ClusterIssuer
EOF

# Verify
sleep 5
sudo /usr/local/bin/kubectl get certificate {name}-cert -n openchoreo-data-plane
```

### Step 3: Add HTTPS Listener to Gateway (Ryzen)

```bash
# Execute on Ryzen
sudo /usr/local/bin/kubectl patch gateway gateway-default -n openchoreo-data-plane \
  --type=json -p="[{
    \"op\": \"add\",
    \"path\": \"/spec/listeners/-\",
    \"value\": {
      \"name\": \"https-{name}\",
      \"port\": 443,
      \"protocol\": \"HTTPS\",
      \"hostname\": \"{name}.zerops.internal\",
      \"tls\": {
        \"mode\": \"Terminate\",
        \"certificateRefs\": [{\"name\": \"{name}-tls-secret\"}]
      },
      \"allowedRoutes\": {\"namespaces\": {\"from\": \"All\"}}
    }
  }]"
```

### Step 4: Create HTTPRoute (Ryzen)

```bash
# Execute on Ryzen
sudo /usr/local/bin/kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: {name}-https-route
  namespace: dp-default-default-development-f8e58905
spec:
  parentRefs:
    - name: gateway-default
      namespace: openchoreo-data-plane
      sectionName: https-{name}
  hostnames:
    - {name}.zerops.internal
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: {name}
          port: {port}
EOF
```

### Step 5: Verify

```bash
# Test HTTPS
curl -sk https://{name}.zerops.internal/ -w "%{http_code}\n" -o /dev/null

# Verify DNS
nslookup {name}.zerops.internal 192.168.0.9
```

## Key Values

| Parameter | Value |
|-----------|-------|
| PowerDNS API | `http://192.168.0.9:8081` |
| PowerDNS API Key | `<POWERDNS_API_KEY>` |
| Gateway Name | `gateway-default` |
| Gateway Namespace | `openchoreo-data-plane` |
| Gateway IP | `192.168.0.242` |
| TLS Issuer | `zerops-direct-issuer` (ClusterIssuer) |
| App Namespace | `dp-default-default-development-f8e58905` |
| Cert Duration | 90 days (auto-renew) |

## CA Trust
For browsers to show 🔒 (not "No seguro"), the client Mac must trust:
- **Zerops Ryzen Direct CA** → installed via `docs/bootstrap/install-ca-client.sh`

## Rules & Constraints
- Gateway listener names must be unique (use `https-{name}`)
- Certificate secrets go in `openchoreo-data-plane` namespace
- HTTPRoutes go in the app namespace (`dp-default-*`)
- One listener per hostname in the gateway
