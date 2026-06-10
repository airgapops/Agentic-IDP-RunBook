---
name: ryzen-network-fix
description: >-
  Fix networking issues on the Ryzen Data Plane. Handles NetworkPolicy,
  firewall configuration, SELinux, and service creation for worker components.
  Use when: pods can't communicate, "connection refused", services unreachable.
version: 2.0.0
metadata:
  author: platform-team
  tags: [ryzen, network, networkpolicy, firewall, troubleshooting]
---

# 🔧 SKILL: Ryzen Network Troubleshooting & Fix

## When to Use
- Pods on Ryzen can't communicate with each other
- "Connection refused" or "No route to host" errors
- After Ryzen reboot, services stop working
- New component deployed but can't reach other services

## CRITICAL RULES
1. **NEVER delete all NetworkPolicies** — OpenChoreo policies are managed by the controller
2. **Use additive `allow-gateway-*` policies** to permit gateway traffic
3. **Use `kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml`** for Ryzen (from ThinkPad)

## Common Issues & Fixes

### Issue 1: NetworkPolicies Blocking Traffic

OpenChoreo creates restrictive NetworkPolicies per component that deny all ingress.
**DO NOT delete them.** Create additive policies instead.

```bash
# Check current NetworkPolicies
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get networkpolicy -n dp-default-default-development-f8e58905

# Create additive policy to allow gateway traffic for a component
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-gateway-{COMPONENT}
  namespace: dp-default-default-development-f8e58905
spec:
  podSelector:
    matchLabels:
      openchoreo.dev/component: {COMPONENT}
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: openchoreo-data-plane
      ports:
        - protocol: TCP
          port: {PORT}
EOF
```

> **Why this works**: Kubernetes NetworkPolicies are ADDITIVE.
> If ANY policy allows traffic, it passes — even if another policy denies it.

### Issue 2: Firewalld Blocking Pod Traffic

```bash
# Check firewalld status
sudo firewall-cmd --state

# If running, add pod networks to trusted zone
sudo firewall-cmd --permanent --zone=trusted --add-source=10.42.0.0/16
sudo firewall-cmd --permanent --zone=trusted --add-source=10.43.0.0/16
sudo firewall-cmd --reload

# Or disable entirely (simpler for air-gapped environments)
sudo systemctl stop firewalld
sudo systemctl disable firewalld
```

### Issue 3: SELinux Blocking Connections

```bash
# Check SELinux
getenforce

# Set to permissive
sudo setenforce 0
sudo sed -i 's/SELINUX=enforcing/SELINUX=permissive/' /etc/selinux/config
```

### Issue 4: Worker Component Missing Service

Worker-type components don't get automatic Kubernetes services.

```bash
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: {name}
  namespace: dp-default-default-development-f8e58905
spec:
  selector:
    openchoreo.dev/component: {name}
  ports:
    - port: {port}
      targetPort: {port}
  type: ClusterIP
EOF
```

### Issue 5: Too Many Open Files

```bash
sudo sysctl fs.inotify.max_user_watches=524288
sudo sysctl fs.inotify.max_user_instances=512
echo "fs.inotify.max_user_watches=524288" | sudo tee -a /etc/sysctl.conf
echo "fs.inotify.max_user_instances=512" | sudo tee -a /etc/sysctl.conf
```

## Diagnostic Commands

```bash
# Full diagnostic on Ryzen
echo "=== PODS ==="
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get pods -n dp-default-default-development-f8e58905 -o wide
echo "=== SERVICES ==="
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get svc -n dp-default-default-development-f8e58905
echo "=== ENDPOINTS ==="
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get endpoints -n dp-default-default-development-f8e58905
echo "=== NETPOL ==="
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get networkpolicy -n dp-default-default-development-f8e58905
echo "=== FIREWALL ==="
sudo firewall-cmd --state 2>/dev/null || echo "NOT_INSTALLED"
echo "=== SELINUX ==="
getenforce
```

## Post-Reboot Checklist

After Ryzen reboots, verify:
1. ✅ K3s running: `systemctl status k3s`
2. ✅ Pods running: `kubectl get pods -A`
3. ✅ Allow-gateway-* policies exist (create if missing)
4. ✅ cluster-agent connected (check ThinkPad logs)
5. ✅ Services accessible
