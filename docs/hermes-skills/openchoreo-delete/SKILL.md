---
name: openchoreo-delete
description: >-
  Delete an OpenChoreo component and all its resources across all environments.
  Use this skill when asked to: delete, remove, undeploy, destroy, or teardown a service.
  Cleans up: ReleaseBindings, Workload, Component, Services, HTTPRoutes,
  Certificates, Gateway listeners, DNS records, NetworkPolicies, and ConfigMaps.
version: 2.0.0
metadata:
  author: platform-team
  tags: [openchoreo, delete, cleanup, teardown]
---

# 🗑️ SKILL: Delete Component from OpenChoreo

## When to Use
- User says "delete X", "remove X", "undeploy X", "destroy X"

## CRITICAL RULES
1. **EXECUTE all commands** — never show steps
2. **Use `sudo kubectl`** on ThinkPad (passwordless)
3. **Use `kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml`** for Ryzen
4. **Delete in order**: ReleaseBindings → Workload → Component (ThinkPad first, then Ryzen cleanup)
5. **Delete custom NetworkPolicies** with `allow-gateway-{COMPONENT}` and `{COMPONENT}-isolation` names

## Variables
- `{COMPONENT}` — component name to delete (e.g. airgapops, nginx)

## EXECUTE ALL STEPS BELOW

### Step 1: Delete ReleaseBindings on ThinkPad

```bash
sudo kubectl delete releasebinding -n default -l openchoreo.dev/component={COMPONENT}
```

### Step 2: Delete Workload on ThinkPad

```bash
sudo kubectl delete workload {COMPONENT} -n default
```

### Step 3: Delete Component on ThinkPad

```bash
sudo kubectl delete component {COMPONENT} -n default
```

### Step 4: Wait for pods to terminate (10 seconds)

```bash
sleep 10
```

### Step 5: Cleanup Ryzen — Services, HTTPRoutes, ConfigMaps, NetworkPolicies

Find all namespaces that had this component:

```bash
for NS in $(kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get ns -o name | grep "dp-default" | sed 's|namespace/||'); do
  kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml delete svc {COMPONENT} -n $NS 2>/dev/null
  kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml delete httproute {COMPONENT}-https-route {COMPONENT}-staging-route {COMPONENT}-production-route {COMPONENT}-{ENV}-route -n $NS 2>/dev/null
  kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml delete configmap {COMPONENT}-html -n $NS 2>/dev/null
  kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml delete networkpolicy allow-gateway-{COMPONENT} {COMPONENT}-isolation allow-{COMPONENT}-to-postgres -n $NS 2>/dev/null
done
```

### Step 6: Cleanup Ryzen — Certificates

```bash
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml delete certificate {COMPONENT}-cert {COMPONENT}-staging-cert {COMPONENT}-production-cert -n openchoreo-data-plane 2>/dev/null
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml delete secret {COMPONENT}-tls-secret {COMPONENT}-staging-tls-secret {COMPONENT}-production-tls-secret -n openchoreo-data-plane 2>/dev/null
```

### Step 7: Cleanup Gateway listeners

Remove the HTTPS listeners for this component from the Gateway:

```bash
# Get current listeners, filter out the ones for this component, and replace
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get gateway gateway-default -n openchoreo-data-plane -o json | python3 -c "
import json,sys
gw=json.load(sys.stdin)
gw['spec']['listeners']=[l for l in gw['spec']['listeners'] if '{COMPONENT}' not in l.get('name','')]
del gw['metadata']['resourceVersion']
del gw['metadata']['uid']
del gw['metadata']['creationTimestamp']
if 'status' in gw: del gw['status']
if 'managedFields' in gw['metadata']: del gw['metadata']['managedFields']
json.dump(gw,sys.stdout)
" | kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml replace -f -
```

### Step 8: Cleanup DNS records

```bash
sshpass -p '<CP_PASSWORD>' ssh -o StrictHostKeyChecking=no ubuntu@192.168.0.9 \
  "curl -s -X PATCH 'http://localhost:8081/api/v1/servers/localhost/zones/zerops.internal.' \
  -H 'X-API-Key: <POWERDNS_API_KEY>' \
  -H 'Content-Type: application/json' \
  -d '{\"rrsets\": [
    {\"name\": \"{COMPONENT}.zerops.internal.\", \"type\": \"A\", \"changetype\": \"DELETE\"},
    {\"name\": \"{COMPONENT}-stg.zerops.internal.\", \"type\": \"A\", \"changetype\": \"DELETE\"},
    {\"name\": \"{COMPONENT}-prod.zerops.internal.\", \"type\": \"A\", \"changetype\": \"DELETE\"}
  ]}'"
```

### Step 9: Verify deletion

```bash
sudo kubectl get component,workload,releasebinding -n default | grep {COMPONENT} || echo "Component fully deleted from ThinkPad"
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get pods -A | grep {COMPONENT} || echo "No pods remaining on Ryzen"
```

### Step 10: Report to user

```
🗑️ {COMPONENT} deleted from all environments!

Cleaned up:
  ✅ Component + Workload + ReleaseBindings (ThinkPad)
  ✅ Pods terminated (Ryzen)
  ✅ Services, HTTPRoutes, ConfigMaps (Ryzen)
  ✅ TLS Certificates (Ryzen)
  ✅ Gateway HTTPS listeners (Ryzen)
  ✅ DNS records (Pi)

Next actions:
  • "Despliega {COMPONENT} de nuevo"
  • "Muéstrame qué servicios tenemos"
```
