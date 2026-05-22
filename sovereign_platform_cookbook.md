# Sovereign Platform Replication Cookbook (PB-00, PB-01, PB-02)

This enterprise-grade Cookbook documents the step-by-step implementation, configurations, and commands required to replicate the **Sovereign Infrastructure / Internal Developer Platform (IDP)**. It covers the complete build-out across the prerequisites, network foundation, and GitOps core layers.

---

## Architecture Overview

The following diagram maps the structural flow of the sovereign platform:

```mermaid
graph TD
    subgraph Tier 0: Red y Confianza (Raspberry Pi - 192.168.0.9)
        DNS[PowerDNS / Wildcard *.zerops.internal]
        StepCA[Step-CA / ACME Private PKI]
    end

    subgraph Tier 1: Cómputo y Borde (ThinkPad - 192.168.0.10)
        K3s[K3s Single Node Cluster]
        MetalLB[MetalLB / IP: 192.168.0.200]
        Envoy[Envoy Gateway / Gateway API]
        CertManager[cert-manager / ACME Issuer]
        ExtDNS[ExternalDNS / pdns sync]
    end

    subgraph Tier 2: Capa GitOps (K3s Cluster)
        Gitea[Gitea / gitea.zerops.internal]
        ArgoCD[ArgoCD / argocd.zerops.internal]
    end

    %% Flow lines
    DNS -->|Resolves wildcard| MetalLB
    StepCA -->|ACME TLS Certificate authority| CertManager
    CertManager -->|Auto-provisions Secrets| Envoy
    ArgoCD -->|Pulls manifests| Gitea
    ExtDNS -->|Registers routes| DNS
```

---

## PB-00: Infrastructure Prerequisites (Tier 0)

### 1. PowerDNS Setup (Raspberry Pi - `192.168.0.9`)
PowerDNS runs inside a Podman container on the Raspberry Pi. It serves the `zerops.internal` private zone.

*   **API Configuration**: Enabled on port `8081` with API key `super-secret-api-key-zerops`.
*   **Wildcard DNS Record**:
    *   Record: `*.zerops.internal`
    *   Type: `A`
    *   IP: `192.168.0.10` (Points directly to K3s node)

### 2. Step-CA Setup (Raspberry Pi - `192.168.0.9`)
Step-CA operates on port `9000` as a private ACME-compatible CA.
*   **Root CA Certificate**: Exposes root cert `root_ca.crt` to bootstrap trust.
*   **ACME Provisioner**: Configured to auto-verify DNS names in the `zerops.internal` namespace.

---

## PB-01: Private Cloud Foundation (Tier 1)

### 1. K3s Installation & Access
Installed on the Ubuntu ThinkPad host (`192.168.0.10`).
*   **Administrative Access**: SSH as `rooter` with password authentication bypassed locally using `-o PubkeyAuthentication=no` (to prevent interactive keypass prompts):
    ```bash
    sshpass -p 'Panama2028#...' ssh -o PubkeyAuthentication=no -o StrictHostKeyChecking=no rooter@192.168.0.10 "kubectl get nodes"
    ```

### 2. MetalLB Configuration
Allows allocation of real LAN IPs within the bare metal cluster.
*   **IP Pool**: Configured in range `192.168.0.200-192.168.0.200` to bind to Envoy Gateway.

### 3. Envoy Gateway & Gateway API
Replaces traditional ingress with Envoy-based L4/L7 routing.
*   **Gateway Manifest** (`gateway/zerops-gateway.yaml`):
    ```yaml
    apiVersion: gateway.networking.k8s.io/v1
    kind: Gateway
    metadata:
      name: zerops-gateway
      namespace: default
      annotations:
        cert-manager.io/cluster-issuer: zerops-ca-issuer
    spec:
      gatewayClassName: eg
      listeners:
        - name: http
          protocol: HTTP
          port: 80
          allowedRoutes:
            namespaces:
              from: All
        - name: https-gitea
          protocol: HTTPS
          port: 443
          hostname: gitea.zerops.internal
          tls:
            mode: Terminate
            certificateRefs:
              - group: ""
                kind: Secret
                name: gitea-tls-secret
          allowedRoutes:
            namespaces:
              from: All
        - name: https-argocd
          protocol: HTTPS
          port: 443
          hostname: argocd.zerops.internal
          tls:
            mode: Terminate
            certificateRefs:
              - group: ""
                kind: Secret
                name: argocd-tls-secret
          allowedRoutes:
            namespaces:
              from: All
    ```

### 4. cert-manager ACME Trust Integration
*   **ClusterIssuer Configuration**: Points to Step-CA's private ACME endpoint on the Raspberry Pi:
    ```yaml
    apiVersion: cert-manager.io/v1
    kind: ClusterIssuer
    metadata:
      name: zerops-ca-issuer
    spec:
      acme:
        server: https://192.168.0.9:9000/acme/acme/directory
        privateKeySecretRef:
          name: step-ca-acme-key
        solvers:
          - http01:
              ingress:
                class: eg
    ```

### 5. ExternalDNS Integration (Dynamic DNS Synchronization)
ExternalDNS runs in the `kube-system` namespace, automatically publishing routes from Envoy's `HTTPRoute` into PowerDNS.
*   **Deployment Configuration** (`gateway/external-dns.yaml`):
    ```yaml
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: external-dns
      namespace: kube-system
    spec:
      strategy:
        type: Recreate
      selector:
        matchLabels:
          app: external-dns
      template:
        metadata:
          labels:
            app: external-dns
        spec:
          containers:
          - name: external-dns
            image: registry.k8s.io/external-dns/external-dns:v0.14.0
            args:
            - --source=gateway-httproute
            - --provider=pdns
            - --pdns-server=http://192.168.0.9:8081
            - --pdns-api-key=super-secret-api-key-zerops
            - --txt-owner-id=k3s-sovereign
            - --domain-filter=zerops.internal
    ```

---

## PB-02: The AI-IDP Core (Tier 2)

### 1. Gitea (The Layer of Truth)
Runs in the `gitea` namespace, exposed at `https://gitea.zerops.internal`.
*   **API Repo Creation**:
    ```bash
    curl -X POST -H "Content-Type: application/json" \
      -u "jadamson:Panama2028#..." \
      -d '{"name":"airgap-ops","private":false}' \
      http://localhost:3000/api/v1/user/repos
    ```

### 2. ArgoCD (The GitOps Engine)
Runs in the `argocd` namespace, exposed at `https://argocd.zerops.internal`.

*   **ArgoCD Insecure Mode CM**: Required when terminating TLS at Envoy to avoid redirect loops:
    ```yaml
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: argocd-cmd-params-cm
      namespace: argocd
    data:
      server.insecure: "true"
    ```
*   **HTTPRoute Manifest** (`argocd/argocd-route.yaml`):
    ```yaml
    apiVersion: gateway.networking.k8s.io/v1
    kind: HTTPRoute
    metadata:
      name: argocd-route
      namespace: argocd
    spec:
      parentRefs:
        - name: zerops-gateway
          namespace: default
      hostnames:
        - "argocd.zerops.internal"
      rules:
        - matches:
            - path:
                type: PathPrefix
                value: /
          backendRefs:
            - name: argocd-server
              port: 80
    ```
*   **cert-manager Certificate** (`argocd/argocd-cert.yaml`):
    ```yaml
    apiVersion: cert-manager.io/v1
    kind: Certificate
    metadata:
      name: argocd-cert
      namespace: default
    spec:
      commonName: argocd.zerops.internal
      dnsNames:
        - argocd.zerops.internal
      issuerRef:
        kind: ClusterIssuer
        name: zerops-ca-issuer
      secretName: argocd-tls-secret
    ```

---

## The Bootstrap Loop (Closing the Circle)

1.  **Configure local workspace remote & push**:
    ```bash
    git init
    git remote add origin https://gitea.zerops.internal/jadamson/airgap-ops.git
    git add .
    git commit -m "feat: bootstrap platform state"
    git -c http.sslVerify=false push -u origin main
    ```
2.  **Add Gitea Repository configuration to ArgoCD** (`argocd/gitea-repo.yaml`):
    Since Gitea uses the private CA, we skip SSL verification specifically for this repo using `insecure: "true"`:
    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: gitea-repo-secret
      namespace: argocd
      labels:
        argocd.argoproj.io/secret-type: repository
    type: Opaque
    stringData:
      url: https://gitea.zerops.internal/jadamson/airgap-ops.git
      insecure: "true"
    ```
3.  **Create Root App-of-Apps** (`argocd/root-app.yaml`):
    ```yaml
    apiVersion: argoproj.io/v1alpha1
    kind: Application
    metadata:
      name: root-app
      namespace: argocd
    spec:
      project: default
      source:
        repoURL: https://gitea.zerops.internal/jadamson/airgap-ops.git
        targetRevision: HEAD
        path: argocd
      destination:
        server: https://kubernetes.default.svc
        namespace: argocd
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
          - CreateNamespace=true
    ```
4.  **Execute the loop**:
    Apply both manifests to the cluster. ArgoCD will instantly establish connection to Gitea, read its own cluster configuration, and synchronize the entire platform state automatically!
    ```bash
    kubectl apply -f gitea-repo.yaml -f root-app.yaml
    ```

---

## DNS & Access Automation for Panama Developers

To achieve true zero-touch DNS resolution without manually editing local `/etc/hosts` for every new service:
1.  **Set Mac DNS Server**: Add your Raspberry Pi IP `192.168.0.9` as the primary DNS server in macOS network settings.
2.  **Wildcard Resolution**: Since PowerDNS hosts a `*.zerops.internal` wildcard pointing to your ThinkPad `192.168.0.10`, any new routing declared in Git and deployed in the cluster (e.g. `choreo.zerops.internal`) will instantly resolve and open in your browser automatically!
