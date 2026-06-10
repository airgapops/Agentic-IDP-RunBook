---
name: cert-management
description: >-
  Manage TLS certificates for the Zerops platform. Handles certificate creation,
  renewal, CA trust installation, and troubleshooting expired certs.
  Use when: HTTPS shows "Not Secure", certs expired, new service needs TLS,
  new laptop needs CA trust.
version: 1.0.0
metadata:
  author: platform-team
  tags: [tls, certificates, https, cert-manager, ca]
---

# 🔐 SKILL: Certificate & TLS Management

## When to Use
- Browser shows "No seguro" / "Not Secure"
- Certificate expired
- New service needs HTTPS
- New laptop/client needs CA trust

## Architecture

```
ThinkPad (CP):  zerops-direct-issuer → CA: "Zerops Internal Direct CA"
Ryzen (DP):     zerops-direct-issuer → CA: "Zerops Ryzen Direct CA"
```

Both use cert-manager with direct CA signing (no ACME, no challenges).

## Certificate Issuers

| Cluster | Issuer Name | CA Name | Secret |
|---------|-------------|---------|--------|
| ThinkPad | zerops-direct-issuer | Zerops Internal Direct CA | zerops-direct-ca-secret (ns: cert-manager) |
| Ryzen | zerops-direct-issuer | Zerops Ryzen Direct CA | zerops-ryzen-ca-secret (ns: cert-manager) |

## Create New Certificate

### On ThinkPad (for choreo, gitea, thunder, etc.)

```bash
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: {name}-cert
  namespace: default
spec:
  secretName: {name}-tls-secret
  duration: 2160h
  renewBefore: 360h
  dnsNames:
    - {name}.zerops.internal
  issuerRef:
    name: zerops-direct-issuer
    kind: ClusterIssuer
EOF
```

### On Ryzen (for postgrest, redis, etc.)

```bash
sudo /usr/local/bin/kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: {name}-cert
  namespace: openchoreo-data-plane
spec:
  secretName: {name}-tls-secret
  duration: 2160h
  renewBefore: 360h
  dnsNames:
    - {name}.zerops.internal
  issuerRef:
    name: zerops-direct-issuer
    kind: ClusterIssuer
EOF
```

## Install CA on New Client (Mac)

```bash
# Run the bootstrap script
bash /path/to/airgapOps/docs/bootstrap/install-ca-client.sh

# Or manually:
# 1. Copy CA cert from ThinkPad
scp rooter@192.168.0.10:/tmp/zerops-direct-ca.crt /tmp/

# 2. Copy CA cert from Ryzen
scp admin@192.168.0.13:/tmp/zerops-ryzen-ca.crt /tmp/

# 3. Import both
sudo security import /tmp/zerops-direct-ca.crt -k /Library/Keychains/System.keychain
sudo security import /tmp/zerops-ryzen-ca.crt -k /Library/Keychains/System.keychain

# 4. Open Keychain Access → find each CA → Trust → "Always Trust"
# 5. Restart Chrome (Cmd+Q, reopen)
```

## Troubleshoot Expired Certs

```bash
# Check all certs
kubectl get certificate -A

# Force renewal (delete secret, cert-manager will recreate)
kubectl delete secret {name}-tls-secret -n {namespace}

# Verify renewal
sleep 15
kubectl get certificate {name}-cert -n {namespace}
```

## Important Notes
- Certs are 90 days (2160h) with 15-day auto-renewal
- NO ACME challenges needed (direct CA signing)
- Two separate CAs: one per cluster (ThinkPad + Ryzen)
- Both CAs must be trusted on client machines
