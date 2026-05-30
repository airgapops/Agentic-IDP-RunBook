---
name: openchoreo-promote
description: >-
  Promote application releases between environments in OpenChoreo.
  Use this skill when asked to: promote a service to production,
  move a component from dev to staging, rollback a deployment,
  create a release binding, promote between environments.
version: 1.0.0
metadata:
  author: platform-team
  tags: [openchoreo, promote, release, environment, rollback]
---

# 🔄 SKILL: Promote Releases Between Environments

## When to Use
- User asks to promote/move a component to another environment
- User says "promote X to production", "move X to staging"
- User wants to rollback a deployment
- User asks about release management or environment promotion

## Key Concepts

```
ComponentRelease (immutable snapshot)
       │
       ├──► ReleaseBinding (dev)     → deploys to development
       ├──► ReleaseBinding (staging) → deploys to staging
       └──► ReleaseBinding (prod)    → deploys to production

Promotion = Create a new ReleaseBinding pointing same ComponentRelease
            to a different Environment.
```

## Instructions

### Step 1: Find the ComponentRelease to promote

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# List all releases for a component
kubectl get componentrelease -l openchoreo.dev/component={component-name}

# Get the specific release name
RELEASE_NAME=$(kubectl get componentrelease \
  -l openchoreo.dev/component={component-name} \
  -o jsonpath='{.items[0].metadata.name}')
echo "Release to promote: $RELEASE_NAME"
```

### Step 2: Check current ReleaseBindings

```bash
# See where this component is currently deployed
kubectl get releasebinding -l openchoreo.dev/component={component-name}
```

### Step 3: Create ReleaseBinding for target environment

```bash
cat <<EOF | kubectl apply -f -
apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: {component-name}-{target-environment}
  namespace: default
  annotations:
    openchoreo.dev/display-name: "{Component} → {Environment}"
    openchoreo.dev/description: "Promoted from {source-env} on $(date -Iseconds)"
spec:
  componentReleaseRef:
    name: $RELEASE_NAME
  environmentRef:
    name: {target-environment}
EOF
```

### Step 4: Verify promotion

```bash
# Check the ReleaseBinding was created
kubectl get releasebinding {component-name}-{target-environment}

# Check pods are running in the target environment
kubectl get pods -l openchoreo.dev/component={component-name} -A

# Wait for ready
kubectl wait --for=condition=Ready \
  releasebinding/{component-name}-{target-environment} \
  --timeout=120s 2>/dev/null || echo "Check status manually"
```

### Rollback

To rollback, either:
1. **Delete the ReleaseBinding**: `kubectl delete releasebinding {component-name}-{environment}`
2. **Point to previous release**: Update the `componentReleaseRef.name` to an older release

```bash
# Find previous releases (sorted by age)
kubectl get componentrelease -l openchoreo.dev/component={component-name} \
  --sort-by=.metadata.creationTimestamp

# Patch to rollback
kubectl patch releasebinding {component-name}-{environment} \
  --type=merge \
  -p '{"spec":{"componentReleaseRef":{"name":"{previous-release-name}"}}}'
```

## Promotion Matrix

| Source | Target | Command |
|--------|--------|---------|
| development → staging | `{component}-staging` | Standard promotion |
| staging → production | `{component}-production` | Requires verification |
| production → development | NOT recommended | Use rollback instead |

## Rules & Constraints
- ComponentReleases are IMMUTABLE — never modify, always create new
- ReleaseBinding naming convention: `{component}-{environment}`
- Always verify the source environment is healthy before promoting
- Production promotions should be verified with health checks
- To rollback: delete the ReleaseBinding or point to a previous release
