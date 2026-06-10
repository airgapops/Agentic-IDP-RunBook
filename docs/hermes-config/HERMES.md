# Zerops Sovereign IDP — Hermes Instructions

## BEHAVIOR RULES (CRITICAL)
1. **EXECUTE commands directly** — NEVER just show steps or ask for confirmation
2. **NEVER install software on bare metal** (no apt, no pip, no yum)
3. **ALWAYS use OpenChoreo CRDs** via kubectl to deploy services
4. **After deploying, report**: URL, credentials, and next actions
5. **Run all commands yourself** using the terminal — do not ask the user to run them
6. **Read your skills** in ~/.hermes/skills/ for exact commands

## Architecture
- ThinkPad (this machine, 192.168.0.10): OpenChoreo Control Plane
- Ryzen (192.168.0.13): Data Plane (runs workloads)
- Pi (192.168.0.9): PowerDNS

## How to Deploy (EXECUTE ALL STEPS AUTOMATICALLY)
1. `sudo kubectl apply` Component + Workload CRDs on this machine
2. Sleep 30 seconds for cluster-agent to deploy to Ryzen
3. SSH to Ryzen: create Service + delete NetworkPolicies
4. Create DNS via curl to PowerDNS API
5. SSH to Ryzen: create TLS cert + Gateway listener + HTTPRoute
6. Test with curl and report URL + credentials to user

## kubectl (passwordless sudo configured)
```bash
sudo kubectl <command>
```

## SSH to Ryzen
```bash
sudo sshpass -p '<DP_PASSWORD>' ssh -o StrictHostKeyChecking=no admin@192.168.0.13 "echo '<DP_PASSWORD>' | sudo -S -k /usr/local/bin/kubectl <command>"
```

## PowerDNS API (create DNS record)
```bash
curl -s -X PATCH http://192.168.0.9:8081/api/v1/servers/localhost/zones/zerops.internal. \
  -H "X-API-Key: <POWERDNS_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"rrsets": [{"name": "SERVICE.zerops.internal.", "type": "A", "ttl": 300, "changetype": "REPLACE", "records": [{"content": "192.168.0.242", "disabled": false}]}]}'
```

## Namespaces
- This machine: default (for CRDs)
- Ryzen pods: dp-default-default-development-f8e58905

## Gateway (Ryzen)
- IP: 192.168.0.242
- Name: gateway-default (ns: openchoreo-data-plane)
- TLS issuer: zerops-direct-issuer (ClusterIssuer)

## Current Services
| Service | URL | User | Password |
|---------|-----|------|----------|
| PostgreSQL | postgres:5432 (internal) | zerops | zerops123 |
| Redis | redis:6379 | - | - |
| Redis Commander | https://redis.zerops.internal | admin | admin123 |
| PostgREST | https://postgrest.zerops.internal | - | - |
| OpenChoreo | https://choreo.zerops.internal | admin@openchoreo.dev | <CHOREO_PASSWORD> |
| Gitea | https://gitea.zerops.internal | <GITEA_USER> | <CP_PASSWORD> |
