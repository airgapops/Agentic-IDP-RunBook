---
name: openchoreo-deploy
description: >-
  Deploy applications to the Zerops Sovereign IDP using OpenChoreo CRDs.
  Use this skill when asked to: create a new service, deploy an application,
  launch a container, create a component, deploy from image, deploy to
  development or production environment. Handles Component + Workload +
  ReleaseBinding lifecycle on Kubernetes via kubectl.
version: 1.0.0
metadata:
  author: platform-team
  tags: [openchoreo, deploy, kubernetes, component, workload]
---

# 🚀 SKILL: Deploy Components on OpenChoreo

## When to Use
- User asks to deploy a new service, application, or container
- User wants to create a new Component in the platform
- User wants to run a Docker image on the cluster
- User says "deploy X", "create service X", "run container X"

## Architecture Context

```
ThinkPad (192.168.0.10) — Management Cluster
├── OpenChoreo Control Plane (namespace: openchoreo-control-plane)
├── OpenChoreo Data Plane (namespace: openchoreo-data-plane)
├── Gitea (Git + Container Registry: gitea.zerops.internal)
└── Hermes AI (this agent, running on host)

Kubeconfig: /etc/rancher/k3s/k3s.yaml
```

## Available ClusterComponentTypes

| Name | WorkloadType | Use For |
|------|-------------|---------|
| `deployment/service` | Deployment | APIs, backends, databases |
| `deployment/web-application` | Deployment | Frontend web apps |
| `deployment/worker` | Deployment | Background workers, queues |
| `cronjob/scheduled-task` | CronJob | Periodic batch jobs |

## Available Environments

| Name | Production? | Description |
|------|------------|-------------|
| `development` | No | Testing and iteration |
| `staging` | No | Pre-production validation |
| `production` | Yes | Stable workloads |

## Instructions

### Step 1: Create the Component + Workload YAML

```yaml
# File: /tmp/deploy-{name}.yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: {name}
  namespace: default
  annotations:
    openchoreo.dev/display-name: "{Display Name}"
    openchoreo.dev/description: "{Description}"
  labels:
    openchoreo.dev/name: {name}
spec:
  owner:
    projectName: default
  componentType:
    kind: ClusterComponentType
    name: deployment/service  # or deployment/web-application, deployment/worker
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: {name}
  namespace: default
spec:
  owner:
    projectName: default
    componentName: {name}
  container:
    image: {image}:{tag}
```

### Step 2: Apply to cluster

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl apply -f /tmp/deploy-{name}.yaml
```

### Step 3: Verify

```bash
kubectl get component {name} -o wide
kubectl get workload {name} -o wide
kubectl get pods -l openchoreo.dev/component={name} -A
```

### Step 4: Create ReleaseBinding to deploy to an environment

```bash
# Check if a ComponentRelease was auto-created
kubectl get componentrelease -l openchoreo.dev/component={name}

# If yes, create a ReleaseBinding
cat <<EOF | kubectl apply -f -
apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: {name}-development
  namespace: default
spec:
  componentReleaseRef:
    name: {release-name}  # from the componentrelease output
  environmentRef:
    name: development
EOF
```

## Using Gitea Container Registry

For air-gapped deployments, push images to Gitea's built-in OCI registry:

```bash
# Login to Gitea registry
docker login gitea.zerops.internal -u jadamson -p 'Panama2028#...'

# Tag and push
docker tag myapp:latest gitea.zerops.internal/jadamson/myapp:v1.0
docker push gitea.zerops.internal/jadamson/myapp:v1.0

# Use in Component Workload:
# image: gitea.zerops.internal/jadamson/myapp:v1.0
```

## Verification Checklist
- [ ] Component exists: `kubectl get component {name}`
- [ ] Workload exists: `kubectl get workload {name}`
- [ ] Component shows correct PROJECT and COMPONENTTYPE
- [ ] Workload shows correct PROJECT and COMPONENT

## Rules & Constraints
- All Components go in `namespace: default` and `projectName: default`
- Component names must be lowercase, alphanumeric with hyphens
- Always specify `componentType.kind: ClusterComponentType`
- Workload `owner` requires BOTH `projectName` AND `componentName`
- Container images from Gitea registry: `gitea.zerops.internal/jadamson/{image}:{tag}`
