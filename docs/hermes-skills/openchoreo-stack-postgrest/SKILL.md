---
name: openchoreo-stack-postgrest
description: >-
  Deploy PostgREST + PostgreSQL stack via OpenChoreo.
  Use this skill when asked to: deploy postgrest, deploy a REST API, deploy
  a database with API, or set up postgrest + postgres.
  Handles: Components, Workloads, Services, Additive NetworkPolicies,
  DNS, TLS, Gateway, HTTPRoutes for PostgREST.
  PostgreSQL is internal-only (no HTTPS). NO pgAdmin/Adminer (use kubectl exec).
version: 5.0.0
metadata:
  author: platform-team
  tags: [openchoreo, postgrest, postgresql, stack]
---

# 🗄️ SKILL: Deploy PostgREST + PostgreSQL Stack

## When to Use
- User asks to deploy PostgREST, a REST API, a database API
- User asks for PostgreSQL + PostgREST

## CRITICAL RULES
1. **EXECUTE all commands** — never show steps
2. **Use `sudo kubectl`** on ThinkPad (passwordless)
3. **Use `kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml`** for Ryzen
4. **DNS: SSH to Pi** — curl to 192.168.0.9 does NOT work from ThinkPad
5. **ALWAYS HTTPS** for PostgREST
6. **PostgreSQL is internal only** — no HTTPS, no external access
7. componentType MUST be `deployment/worker` — no other types exist
8. **NEVER delete NetworkPolicies** — create additive `allow-gateway-*` policies
9. **HTTPRoute sectionName MUST match listener name** — use `https-postgrest`, NEVER `https`
10. **NEVER create wildcard HTTPS listeners** — always use specific hostnames
11. **All HTTPS listeners MUST use port 443**
12. **NO pgAdmin/Adminer** — use `kubectl exec` to access PostgreSQL directly
13. **Max 7 HTTPS listeners** per Gateway — kgateway limit
14. **🚨 NEVER delete Workloads, Components, or ConfigMaps** — this destroys ALL environments
15. **Password masking is NORMAL** — OpenChoreo's webhook masks passwords in `PGRST_DB_URI` (shows `***`). The real values reach the data plane. DO NOT try to fix this.
16. **Services MUST use simple selectors** — only `openchoreo.dev/component: {name}`. NEVER include `pod-template-hash` in selectors.

## Default Credentials
- PostgreSQL: user=`zerops`, password=`zerops123`, db=`app`

## Namespace
- Development: `dp-default-default-development-f8e58905`

## Database Access (instead of pgAdmin/Adminer)

To query PostgreSQL directly:
```bash
PG_POD=$(kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get pods -n dp-default-default-development-f8e58905 -l openchoreo.dev/component=postgres -o jsonpath='{.items[0].metadata.name}')
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml exec -n dp-default-default-development-f8e58905 $PG_POD -- psql -U zerops -d app -c "YOUR SQL HERE"
```

## EXECUTE ALL STEPS BELOW

### Step 1: Create 2 Components + 2 Workloads on ThinkPad (single apply)

```bash
sudo kubectl apply -f - <<'EOF'
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: postgres
  namespace: default
  annotations:
    openchoreo.dev/display-name: "PostgreSQL"
    openchoreo.dev/description: "PostgreSQL 16 database"
spec:
  owner:
    projectName: default
  componentType:
    kind: ClusterComponentType
    name: deployment/worker
  autoDeploy: true
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: postgres
  namespace: default
spec:
  owner:
    projectName: default
    componentName: postgres
  container:
    image: postgres:16-alpine
    env:
      - key: POSTGRES_USER
        value: zerops
      - key: POSTGRES_PASSWORD
        value: zerops123
      - key: POSTGRES_DB
        value: app
---
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: postgrest
  namespace: default
  annotations:
    openchoreo.dev/display-name: "PostgREST"
    openchoreo.dev/description: "Auto-REST API for PostgreSQL"
spec:
  owner:
    projectName: default
  componentType:
    kind: ClusterComponentType
    name: deployment/worker
  autoDeploy: true
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: postgrest
  namespace: default
spec:
  owner:
    projectName: default
    componentName: postgrest
  container:
    image: postgrest/postgrest:latest
    env:
      - key: PGRST_DB_URI
        value: postgres://zerops:zerops123@postgres:5432/app
      - key: PGRST_DB_ANON_ROLE
        value: zerops
      - key: PGRST_DB_SCHEMA
        value: public
EOF
```

### Step 2: Wait for pods (15 seconds)

```bash
sleep 15 && kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get pods -n dp-default-default-development-f8e58905 | grep -E "postgres|postgrest"
```

If pods not ready, wait 10 more seconds.

### Step 3: Create Services on Ryzen

```bash
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml apply -f - <<'EOF'
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: dp-default-default-development-f8e58905
spec:
  selector:
    openchoreo.dev/component: postgres
  ports:
    - port: 5432
      targetPort: 5432
  type: ClusterIP
---
apiVersion: v1
kind: Service
metadata:
  name: postgrest
  namespace: dp-default-default-development-f8e58905
spec:
  selector:
    openchoreo.dev/component: postgrest
  ports:
    - port: 3000
      targetPort: 3000
  type: ClusterIP
EOF
```

### Step 4: Create ADDITIVE NetworkPolicies (DO NOT delete OpenChoreo policies)

```bash
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml apply -f - <<'EOF'
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-gateway-postgrest
  namespace: dp-default-default-development-f8e58905
spec:
  podSelector:
    matchLabels:
      openchoreo.dev/component: postgrest
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: openchoreo-data-plane
      ports:
        - protocol: TCP
          port: 3000
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-postgrest-to-postgres
  namespace: dp-default-default-development-f8e58905
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
  namespace: dp-default-default-development-f8e58905
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

### Step 5: DNS records (SSH to Pi)

```bash
sshpass -p '<CP_PASSWORD>' ssh -o StrictHostKeyChecking=no ubuntu@192.168.0.9 "curl -s -X PATCH 'http://localhost:8081/api/v1/servers/localhost/zones/zerops.internal.' -H 'X-API-Key: <POWERDNS_API_KEY>' -H 'Content-Type: application/json' -d '{\"rrsets\": [{\"name\": \"postgrest.zerops.internal.\", \"type\": \"A\", \"ttl\": 300, \"changetype\": \"REPLACE\", \"records\": [{\"content\": \"192.168.0.242\", \"disabled\": false}]}]}'"
```

### Step 6: TLS + Gateway + HTTPRoute for PostgREST

```bash
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml apply -f - <<'EOF'
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: postgrest-cert
  namespace: openchoreo-data-plane
spec:
  secretName: postgrest-tls-secret
  duration: 2160h
  renewBefore: 360h
  dnsNames:
    - postgrest.zerops.internal
  issuerRef:
    name: zerops-direct-issuer
    kind: ClusterIssuer
EOF

kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml patch gateway gateway-default -n openchoreo-data-plane --type=json -p='[{"op":"add","path":"/spec/listeners/-","value":{"name":"https-postgrest","port":443,"protocol":"HTTPS","hostname":"postgrest.zerops.internal","tls":{"mode":"Terminate","certificateRefs":[{"name":"postgrest-tls-secret"}]},"allowedRoutes":{"namespaces":{"from":"All"}}}}]'

kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: postgrest-https-route
  namespace: dp-default-default-development-f8e58905
spec:
  parentRefs:
    - name: gateway-default
      namespace: openchoreo-data-plane
      sectionName: https-postgrest
  hostnames:
    - postgrest.zerops.internal
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: postgrest
          port: 3000
EOF
```

### Step 7: Verify

```bash
sleep 10
echo "=== PostgREST ===" && curl -sk https://postgrest.zerops.internal/ -o /dev/null -w "HTTPS: %{http_code}\n"
```

### Step 8: Report

```
✅ PostgREST + PostgreSQL stack deployed!

📍 URLs:
  • PostgREST API: https://postgrest.zerops.internal

🔐 Credentials:
  • PostgreSQL: zerops / zerops123 (database: app)

🔒 NetworkPolicy:
  • PostgreSQL: only accepts connections from PostgREST
  • PostgREST: receives from Gateway, connects to PostgreSQL only
  • Other pods: BLOCKED from PostgreSQL

📋 Database Access (no pgAdmin needed):
  • Use Hermes: "Muéstrame las tablas" or "Crea una tabla de productos"
  • Uses kubectl exec into postgres pod directly

Next actions:
  • "Crea una tabla de productos con columnas: id, nombre, precio, stock"
  • "Promueve postgrest a staging"
  • "Muéstrame los endpoints de PostgREST"
```
