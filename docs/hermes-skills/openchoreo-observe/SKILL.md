---
name: openchoreo-observe
description: >-
  Observe and diagnose OpenChoreo platform health, component status,
  and pod logs. Use this skill when asked to: check status, view logs,
  diagnose errors, troubleshoot deployments, check health, monitor
  pods, find why a component is failing, check platform status.
version: 1.0.0
metadata:
  author: platform-team
  tags: [openchoreo, observability, logs, troubleshoot, diagnose]
---

# 🔍 SKILL: Observe & Diagnose OpenChoreo Platform

## When to Use
- User asks to check platform health or component status
- User reports an error or failure
- User wants to see logs
- User asks "why is X not working?" or "is X running?"

## Instructions

### Platform Health Check

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

echo "=== Control Plane ==="
kubectl get pods -n openchoreo-control-plane -o wide

echo "=== Data Plane ==="
kubectl get pods -n openchoreo-data-plane -o wide

echo "=== OpenChoreo Resources ==="
kubectl get project,component,workload,environment,releasebinding -A

echo "=== Node Status ==="
kubectl get nodes -o wide
```

### Component Health Check

```bash
# Component status
kubectl get component {name} -o yaml | grep -A10 "status:"

# Workload status
kubectl get workload {name} -o yaml | grep -A10 "status:"

# Pods for a component
kubectl get pods -l openchoreo.dev/component={name} -A -o wide

# Events (last 10 minutes)
kubectl get events -A --sort-by='.lastTimestamp' | tail -20
```

### View Logs

```bash
# Logs for a component's pods
kubectl logs -l openchoreo.dev/component={name} --tail=50 -A

# Logs for specific OpenChoreo system pods
kubectl logs -n openchoreo-control-plane deployment/controller-manager --tail=30
kubectl logs -n openchoreo-control-plane deployment/openchoreo-api --tail=30
kubectl logs -n openchoreo-control-plane deployment/backstage --tail=30

# Follow logs in real-time
kubectl logs -l openchoreo.dev/component={name} -f --tail=10 -A
```

### Diagnose Failures

```bash
# Pod describe (shows events, scheduling issues)
POD=$(kubectl get pods -l openchoreo.dev/component={name} -A -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
kubectl describe pod $POD -A

# Check for CrashLoopBackOff
kubectl get pods -A | grep -E "CrashLoop|Error|Pending|ImagePull"

# Check ReleaseBinding status
kubectl get releasebinding -A -o wide
```

### Network & Endpoints

```bash
# Check services
kubectl get svc -A | grep -v "kube-system"

# Check endpoints
kubectl get endpoints -A | grep -v "kube-system"

# Test internal DNS
kubectl run dns-test --rm -it --restart=Never --image=busybox:1.36 \
  -- nslookup {service}.default.svc.cluster.local 2>/dev/null

# Check Gateway routes
kubectl get httproute -A 2>/dev/null
kubectl get gateway -A 2>/dev/null
```

### Quick Status Summary

```bash
echo "=== PLATFORM STATUS $(date) ==="
echo ""
echo "Nodes:"
kubectl get nodes --no-headers | awk '{printf "  %-20s %-10s %s\n", $1, $2, $5}'
echo ""
echo "Control Plane: $(kubectl get pods -n openchoreo-control-plane --no-headers | grep -c Running)/$(kubectl get pods -n openchoreo-control-plane --no-headers | wc -l) pods running"
echo "Data Plane:    $(kubectl get pods -n openchoreo-data-plane --no-headers | grep -c Running)/$(kubectl get pods -n openchoreo-data-plane --no-headers | wc -l) pods running"
echo ""
echo "Components:"
kubectl get component -A --no-headers | awk '{printf "  %-20s project=%-10s type=%s\n", $2, $3, $4}'
echo ""
echo "Environments:"
kubectl get environment -A --no-headers | awk '{printf "  %-15s\n", $2}'
```

## Common Issues

| Symptom | Check | Fix |
|---------|-------|-----|
| Pod `CrashLoopBackOff` | `kubectl logs {pod}` | Fix config/image |
| Pod `ImagePullBackOff` | `kubectl describe pod {pod}` | Check image name/registry auth |
| Pod `Pending` | `kubectl describe pod {pod}` | Check resources/scheduling |
| Component not deploying | `kubectl get releasebinding -A` | Create ReleaseBinding |
| Gateway 404 | `kubectl get httproute -A` | Check HTTPRoute config |

## Rules & Constraints
- Always use `export KUBECONFIG=/etc/rancher/k3s/k3s.yaml` first
- Limit log output with `--tail=50` to avoid flooding context
- Check events FIRST when diagnosing — they usually reveal the cause
- Use `-o wide` for extra context on pod scheduling
