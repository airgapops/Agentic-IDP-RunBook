# Skill: K8s Resource Provisioning (Ryzen Direct)

## Contexto
Aprovisionar recursos Kubernetes directamente en el cluster K3s del Ryzen.
No requiere vcluster ni Crossplane — acceso directo via kubeconfig.

## Cluster
- **Endpoint:** `https://192.168.0.13:6443`
- **Kubeconfig:** `/Users/jonnhy/Documents/airgapOps/ryzen-kubeconfig.yaml`
- **GPU:** RTX 5060 Ti — `nvidia.com/gpu: 1` disponible
- **K3s:** v1.35.5+k3s1

## Alias útil
```bash
export RYZEN="KUBECONFIG=/Users/jonnhy/Documents/airgapOps/ryzen-kubeconfig.yaml /usr/local/bin/kubectl"
# O desde el Ryzen por SSH:
# KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl
```

---

## Recetas comunes

### Crear namespace para un proyecto
```bash
KUBECONFIG=ryzen-kubeconfig.yaml kubectl create namespace <nombre>
KUBECONFIG=ryzen-kubeconfig.yaml kubectl label namespace <nombre> \
  app.kubernetes.io/managed-by=hermes \
  zerops.io/project=<nombre>
```

### Deployar workload LLM con GPU
```bash
# Ollama con NVIDIA GPU (namespace: ollama-system)
KUBECONFIG=ryzen-kubeconfig.yaml kubectl apply -f llm/ollama-ryzen-nvidia.yaml

# Verificar que el pod tomó la GPU
KUBECONFIG=ryzen-kubeconfig.yaml kubectl describe pod -n ollama-system -l app=ollama | grep -A5 "Limits:"
```

### Verificar GPU disponible
```bash
KUBECONFIG=ryzen-kubeconfig.yaml kubectl describe nodes | grep -A5 "Allocatable:" | grep nvidia
# Debe mostrar: nvidia.com/gpu: 1

# Capacidad actual usada
KUBECONFIG=ryzen-kubeconfig.yaml kubectl get pods -A -o json | \
  python3 -c "import sys,json; pods=json.load(sys.stdin)['items']; \
  [print(p['metadata']['namespace'], p['metadata']['name']) \
   for p in pods for c in p['spec']['containers'] \
   if c.get('resources',{}).get('limits',{}).get('nvidia.com/gpu')]"
```

### Pull modelo en Ollama
```bash
# Via Job de Kubernetes (dentro del cluster)
kubectl -n ollama-system exec -it deploy/ollama -- ollama pull hermes3:8b

# Via REST desde el host (si Ollama corre como systemd)
curl -X POST http://192.168.0.13:11434/api/pull \
  -H "Content-Type: application/json" \
  -d '{"name":"hermes3:8b"}' --no-buffer
```

### Consultar LLM
```bash
# Ollama REST API (systemd service en host)
curl http://192.168.0.13:11434/api/generate \
  -H "Content-Type: application/json" \
  -d '{"model":"hermes3:8b","prompt":"Hola, describe el cluster en una línea.","stream":false}'

# Lista de modelos disponibles
curl http://192.168.0.13:11434/api/tags | python3 -m json.tool | grep name
```

### Status general del cluster
```bash
KUBECONFIG=ryzen-kubeconfig.yaml kubectl get nodes,pods -A \
  --field-selector=status.phase!=Succeeded 2>/dev/null | \
  grep -v Completed
```

---

## Namespaces del cluster

| Namespace | Propósito | Creado por |
|---|---|---|
| `kube-system` | Core K3s + nvidia-device-plugin | K3s |
| `ollama-system` | Servicio Ollama con GPU | Hermes |
| `llm-system` | AnythingLLM frontend | ArgoCD |
| `default-ns` | vcluster (reservado) | Crossplane |
| `ryzen-dev-apps` | vcluster synced workloads | vcluster |

---

## Arquitectura de acceso Hermes → K8s

```
Hermes (Mac / ThinkPad)
  │
  ├── kubectl + ryzen-kubeconfig.yaml
  │     └── https://192.168.0.13:6443
  │           └── K3s Ryzen
  │                 ├── ollama-system  (GPU workloads)
  │                 ├── llm-system     (AnythingLLM)
  │                 └── [namespaces por proyecto]
  │
  └── Ollama REST API directa
        └── http://192.168.0.13:11434
              ├── hermes3:8b
              ├── llama3.1:8b
              └── [otros modelos]
```

---

## Problemas conocidos

### nvidia-device-plugin no expone GPU en pods
```bash
# Verificar que el plugin está Running
kubectl get ds -n kube-system nvidia-device-plugin-daemonset

# Verificar containerd runtime class
kubectl get runtimeclass nvidia 2>/dev/null || echo "RuntimeClass 'nvidia' no existe"

# Si no existe, crear:
kubectl apply -f - <<EOF
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: nvidia
handler: nvidia
EOF
```

### Pod con GPU queda en Pending
```bash
# Ver por qué no se schedulea
kubectl describe pod <pod> -n <ns> | grep -A10 "Events:"
# Si dice "Insufficient nvidia.com/gpu" → GPU ocupada por otro pod
# Si dice "RuntimeClass not found" → ver solución arriba
```
