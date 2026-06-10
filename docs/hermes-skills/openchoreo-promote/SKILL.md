---
name: openchoreo-promote
description: >-
  Promote any OpenChoreo component to a different environment (dev, staging, production).
  Use this skill when asked to: promote, deploy to staging, deploy to production,
  move to next environment. Always provisions HTTPS for components that need it.
version: 7.0.0
metadata:
  author: platform-team
  tags: [openchoreo, promote, environments, staging, production]
---

# 🔄 SKILL: Promote Component to Environment

## When to Use
- User says "promote X to staging/production"
- User says "deploy X in production/staging"

## CRITICAL RULES
1. **EXECUTE all commands** — never show steps
2. **Use `sudo kubectl`** on ThinkPad (passwordless)
3. **Use `kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml`** for Ryzen commands
4. **DNS: SSH to Pi** — curl to 192.168.0.9 does NOT work from ThinkPad
5. **🚨 NEVER EVER delete ANY NetworkPolicy** — especially `openchoreo-*` policies. They are managed by the OpenChoreo reconciler and WILL be recreated, breaking your deployment. If traffic is blocked, the ONLY correct fix is to CREATE an ADDITIVE policy named `allow-gateway-{COMPONENT}`. NEVER use `kubectl delete networkpolicy`.
6. **NEVER hardcode releaseName** — always look it up dynamically
7. **HTTPRoute sectionName MUST match listener name** — use `https-{COMPONENT}-{ENV}`, NEVER `https`
8. **NEVER create wildcard HTTPS listeners** — always use specific hostnames
9. **All HTTPS listeners MUST use port 443**
10. **Max 7 HTTPS listeners** per Gateway — check count before adding
11. **If you get 503 or "upstream connect error"** — DO NOT delete NetworkPolicies. Instead, CREATE an additive `allow-gateway-{COMPONENT}` NetworkPolicy and restart the pod.
12. **🚨 NEVER delete Workloads, Components, or ConfigMaps** — this destroys ALL environments. The reconciler manages these.
13. **Password masking is NORMAL** — OpenChoreo's webhook masks passwords in URLs (shows `***`). The real values reach the data plane. DO NOT try to fix this.
14. **Services MUST use simple selectors** — only `openchoreo.dev/component: {name}`. NEVER include `pod-template-hash` in selectors.

## ⚠️ COMPONENT CLASSIFICATION — READ BEFORE EXECUTING

> **THIS TABLE DETERMINES WHICH STEPS TO RUN. CHECK IT FIRST.**

| Component | Port | Steps to Run | Notes |
|-----------|------|-------------|-------|
| airgapops | 80 | ALL 6 STEPS (1→2→3→4→5→6) | Full HTTPS promotion |
| postgrest | 3000 | ALL 6 STEPS (1→2→3→4→5→6) + postgres deps | Full HTTPS promotion + create postgres Service/NetPol |
| postgres | 5432 | ONLY Steps 1→2→3 | **INTERNAL ONLY — SKIP Steps 4,5,6 — NO DNS, NO TLS, NO Gateway** |

### PostgreSQL Exceptions:
- **SKIP Step 4** (no DNS record)
- **SKIP Step 5** (no TLS certificate, no Gateway listener, no HTTPRoute)
- **SKIP Step 6** (no curl verification — there is no HTTPS endpoint)
- **ONLY** create: ReleaseBinding + Service + NetworkPolicy

### PostgREST Requirements:
- **RUN ALL 6 STEPS** including DNS, TLS, Gateway, HTTPRoute
- **ALSO** create postgres Service + NetworkPolicy in the same namespace (see Step 3b)
- After promotion, remind user to create tables with postgrest-tables skill

## Variables
- `{COMPONENT}` — component name (e.g. airgapops, postgrest, postgres)
- `{ENV}` — target environment: staging | production
- `{ENV_SHORT}` — stg | prod
- `{PORT}` — service port (see table above)

## Subdomain Convention
- staging: `{COMPONENT}-stg.zerops.internal`
- production: `{COMPONENT}-prod.zerops.internal`

---

## EXECUTE ALL APPLICABLE STEPS BELOW

### Step 1: Create ReleaseBinding (ALL components)

> **⚠️ CRITICAL: Run this EXACT single command. It looks up the release name automatically.
> DO NOT hardcode the release name. DO NOT skip the RELEASE= line.
> The releaseName is NOT the component name — it has a hash suffix like `airgapops-8c8f49c57`.**

```bash
RELEASE=$(sudo kubectl get releasebinding -n default -l openchoreo.dev/component={COMPONENT} -o jsonpath='{.items[0].spec.releaseName}') && echo "Using release: $RELEASE" && sudo kubectl apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: {COMPONENT}-{ENV}
  namespace: default
spec:
  owner:
    projectName: default
    componentName: {COMPONENT}
  environment: {ENV}
  releaseName: $RELEASE
  state: Active
EOF
```

### Step 2: Wait 15s, find namespace, verify pod (ALL components)

```bash
sleep 15 && NS=$(kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get ns -o name | grep "{ENV}" | head -1 | sed 's|namespace/||') && echo "Namespace: $NS" && kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get pods -n $NS | grep {COMPONENT}
```

### Step 3: Service + Gateway NetworkPolicy (ALL components)

> **DO NOT delete NetworkPolicies. Create an ADDITIVE policy instead.**

```bash
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: {COMPONENT}
  namespace: $NS
spec:
  selector:
    openchoreo.dev/component: {COMPONENT}
  ports:
    - port: {PORT}
      targetPort: {PORT}
  type: ClusterIP
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-gateway-{COMPONENT}
  namespace: $NS
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

### Step 3b: PostgreSQL dependencies (ONLY when promoting PostgREST)

> **⚠️ When promoting PostgREST, you MUST also create postgres Service + NetworkPolicy
> in the SAME namespace so PostgREST can connect to the database.**

```bash
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: $NS
spec:
  selector:
    openchoreo.dev/component: postgres
  ports:
    - port: 5432
      targetPort: 5432
  type: ClusterIP
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-postgrest-to-postgres
  namespace: $NS
spec:
  podSelector:
    matchLabels:
      openchoreo.dev/component: postgrest
  policyTypes:
    - Egress
  egress:
    - to:
        - podSelector:
            matchLabels:
              openchoreo.dev/component: postgres
      ports:
        - protocol: TCP
          port: 5432
    - to: []
      ports:
        - protocol: UDP
          port: 53
        - protocol: TCP
          port: 53
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: postgres-isolation
  namespace: $NS
spec:
  podSelector:
    matchLabels:
      openchoreo.dev/component: postgres
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              openchoreo.dev/component: postgrest
      ports:
        - protocol: TCP
          port: 5432
EOF
```

### Step 4: DNS via SSH to Pi (SKIP for postgres)

> **⚠️ SKIP THIS STEP if {COMPONENT} is `postgres`. PostgreSQL has NO DNS record.**

```bash
sshpass -p '<CP_PASSWORD>' ssh -o StrictHostKeyChecking=no ubuntu@192.168.0.9 "curl -s -X PATCH 'http://localhost:8081/api/v1/servers/localhost/zones/zerops.internal.' -H 'X-API-Key: <POWERDNS_API_KEY>' -H 'Content-Type: application/json' -d '{\"rrsets\": [{\"name\": \"{COMPONENT}-{ENV_SHORT}.zerops.internal.\", \"type\": \"A\", \"ttl\": 300, \"changetype\": \"REPLACE\", \"records\": [{\"content\": \"192.168.0.242\", \"disabled\": false}]}]}'"
```

> Use `stg` for staging, `prod` for production as {ENV_SHORT}.

### Step 5: TLS + Gateway + HTTPRoute (SKIP for postgres)

> **⚠️ SKIP THIS STEP if {COMPONENT} is `postgres`. PostgreSQL has NO HTTPS endpoint.**
> **⚠️ DO NOT SKIP for PostgREST or any other component. PostgREST NEEDS HTTPS.**

First check listener count:
```bash
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get gateway gateway-default -n openchoreo-data-plane -o jsonpath='{range .spec.listeners[*]}{.name}{"\n"}{end}' | wc -l
```
If 7 or more, report to user and ask which to remove.

Then create TLS + Gateway listener + HTTPRoute:
```bash
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: {COMPONENT}-{ENV}-cert
  namespace: openchoreo-data-plane
spec:
  secretName: {COMPONENT}-{ENV}-tls-secret
  duration: 2160h
  renewBefore: 360h
  dnsNames:
    - {COMPONENT}-{ENV_SHORT}.zerops.internal
  issuerRef:
    name: zerops-direct-issuer
    kind: ClusterIssuer
EOF
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml patch gateway gateway-default -n openchoreo-data-plane --type=json -p='[{"op":"add","path":"/spec/listeners/-","value":{"name":"https-{COMPONENT}-{ENV}","port":443,"protocol":"HTTPS","hostname":"{COMPONENT}-{ENV_SHORT}.zerops.internal","tls":{"mode":"Terminate","certificateRefs":[{"name":"{COMPONENT}-{ENV}-tls-secret"}]},"allowedRoutes":{"namespaces":{"from":"All"}}}}]'
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: {COMPONENT}-{ENV}-route
  namespace: $NS
spec:
  parentRefs:
    - name: gateway-default
      namespace: openchoreo-data-plane
      sectionName: https-{COMPONENT}-{ENV}
  hostnames:
    - {COMPONENT}-{ENV_SHORT}.zerops.internal
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: {COMPONENT}
          port: {PORT}
EOF
```

### Step 6: Verify HTTPS and report (SKIP for postgres)

> **⚠️ SKIP THIS STEP if {COMPONENT} is `postgres`.**

```bash
sleep 5 && curl -sk https://{COMPONENT}-{ENV_SHORT}.zerops.internal/ -o /dev/null -w "HTTPS: %{http_code}\n"
```

Report:
```
✅ {COMPONENT} promoted to {ENV}!
📍 URL: https://{COMPONENT}-{ENV_SHORT}.zerops.internal
🔒 HTTPS: ✓
```

For postgres, report:
```
✅ postgres promoted to {ENV}!
📍 Internal: postgres:5432 (no external URL — internal only)
🔒 Access: via PostgREST API or kubectl exec
```
