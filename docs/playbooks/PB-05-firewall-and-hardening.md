# 🔥 PB-05 — Firewall Interno y Hardening del OS

> **Zerops Sovereign IDP** · Playbook Series  
> ← [PB-04 GitOps & ArgoCD](./PB-04-gitops-argocd.md) | [Índice General](../index.md) | PB-06 Observability →

---

| Campo            | Valor                                      |
|------------------|--------------------------------------------|
| **ID**           | PB-05                                      |
| **Título**       | Firewall Interno y Hardening del OS        |
| **Dominio**      | `zerops.internal` (air-gapped)             |
| **Estado**       | ✅ Activo                                   |
| **Versión**      | 1.0.0                                      |
| **Fecha**        | 2026-05-22                                 |
| **Autor**        | Equipo Zerops Ops                          |
| **Referencias**  | CIS Ubuntu 22.04/24.04 LTS, CIS K8s, K3s Hardening Guide |

---

## Tabla de Contenido

1. [Diagrama de Zonas de Confianza](#1-diagrama-de-zonas-de-confianza)
2. [Matriz de Comunicación Inter-Nodo](#2-matriz-de-comunicación-inter-nodo)
3. [Reglas UFW por Nodo](#3-reglas-ufw-por-nodo)
4. [Hardening del OS](#4-hardening-del-os)
5. [CIS Benchmark References](#5-cis-benchmark-references)
6. [Audit Commands Rápidos](#6-audit-commands-rápidos)
7. [Checklist de Validación (Exit Criteria)](#7-checklist-de-validación-exit-criteria)

---

## 1. Diagrama de Zonas de Confianza

El entorno está segmentado en cuatro zonas de confianza. El tráfico únicamente puede fluir entre zonas según las reglas definidas en la [Sección 2](#2-matriz-de-comunicación-inter-nodo) y las reglas UFW de la [Sección 3](#3-reglas-ufw-por-nodo). Todo egreso a Internet está bloqueado a nivel de red (air-gapped físico).

```
╔══════════════════════════════════════════════════════════════════════════════════╗
║                         RED LAN: 192.168.0.0/24                                ║
║                                                                                  ║
║  ┌───────────────────────────────────────────────────────────────────────────┐  ║
║  │  ZONA 0 — Tier-0: Infraestructura Raíz (Trust: MÁXIMO)                   │  ║
║  │                                                                           │  ║
║  │  ┌──────────────────────────────────────────────────────────────────┐    │  ║
║  │  │  🍓 Raspberry Pi — 192.168.0.200                                 │    │  ║
║  │  │     Ubuntu ARM64                                                  │    │  ║
║  │  │     • PowerDNS  :53 TCP/UDP  (DNS autoritativo zerops.internal)  │    │  ║
║  │  │     • Step-CA   :9000        (PKI / ACME endpoint)               │    │  ║
║  │  │     • PDNS API  :8081        (solo 127.0.0.1 — NUNCA exponer)   │    │  ║
║  │  └──────────────────────────────────────────────────────────────────┘    │  ║
║  └───────────────────────────────────────────────────────────────────────────┘  ║
║           │ DNS(53)          │ DNS(53)                                           ║
║           │ Step-CA(9000)    │                                                   ║
║           ▼                  ▼                                                   ║
║  ┌────────────────────┐   ┌──────────────────────────────────────────────────┐  ║
║  │ ZONA 1 — MGMT      │   │ ZONA 2 — COMPUTE (Trust: MEDIO)                  │  ║
║  │ (Trust: ALTO)      │   │                                                   │  ║
║  │                    │   │  ⚙️  Ryzen — 192.168.0.13                        │  ║
║  │ 💻 ThinkPad        │   │     Ubuntu x86_64                                 │  ║
║  │  192.168.0.10      │◄──┤     • K3s Compute Cluster (workloads/vclusters)   │  ║
║  │  Ubuntu x86_64     │   │     • K8s API  :6443  (← solo ThinkPad)          │  ║
║  │  • K3s Mgmt CP     │──►│     • kubelet  :10250 (← solo ThinkPad)          │  ║
║  │  • K8s API  :6443  │   │     • Flannel  :8472 UDP ↔ ThinkPad              │  ║
║  │  • Envoy GW :443   │   │     • NodePorts:30000-32767 (← solo ThinkPad)    │  ║
║  │  • SSH      :22    │   │     • SSH      :22                                │  ║
║  │  • Flannel  :8472  │   └──────────────────────────────────────────────────┘  ║
║  └────────────────────┘                                                          ║
║           ▲                                                                      ║
║           │ SSH(22), HTTPS(443), kubectl(6443-mgmt)                             ║
║           │                                                                      ║
║  ┌────────────────────────────────────────────────────────────────────────────┐  ║
║  │  ZONA DEV — Operador Mac (192.168.0.x) — Trust: BAJO / Acceso Controlado  │  ║
║  └────────────────────────────────────────────────────────────────────────────┘  ║
║                                                                                  ║
║  ╔══════════════════════════════════════════════════════════════════════════╗    ║
║  ║  🚫 INTERNET — BLOQUEADO (air-gapped físico). Sin egreso ni ingreso.    ║    ║
║  ╚══════════════════════════════════════════════════════════════════════════╝    ║
╚══════════════════════════════════════════════════════════════════════════════════╝
```

### Principio de mínimo privilegio por zona

| Zona      | Nodo                  | Nivel de confianza | Puede iniciar conexión hacia |
|-----------|-----------------------|--------------------|------------------------------|
| Zona 0    | Raspberry Pi          | Máximo             | Solo responde (DNS/CA)       |
| Zona 1    | ThinkPad              | Alto               | Zona 0, Zona 2               |
| Zona 2    | Ryzen                 | Medio              | Zona 0 (solo DNS)            |
| Zona Dev  | Mac del operador      | Bajo               | Zona 1 (SSH+HTTPS), Zona 2 (SSH) |
| Internet  | —                     | Ninguno (blocked)  | —                            |

---

## 2. Matriz de Comunicación Inter-Nodo

> **Convención**: `✅ Requerido` = flujo necesario para el funcionamiento del sistema. `⚠️ Condicional` = solo bajo circunstancias específicas. `❌ Denegado` = bloqueado por política.

### 2.1 Flujos de Red Aprobados

| Origen                     | Destino                     | Puerto / Protocolo     | Propósito                                              | ¿Requerido? |
|----------------------------|-----------------------------|------------------------|--------------------------------------------------------|-------------|
| Mac (192.168.0.x)          | ThinkPad (192.168.0.10)     | `22/TCP`               | SSH — administración del nodo                          | ✅           |
| Mac (192.168.0.x)          | ThinkPad (192.168.0.10)     | `443/TCP`              | HTTPS — Envoy Gateway (choreo/argocd/gitea)            | ✅           |
| Mac (192.168.0.x)          | ThinkPad (192.168.0.10)     | `6443/TCP`             | kubectl al Management Cluster K3s                      | ✅           |
| Mac (192.168.0.x)          | Ryzen (192.168.0.13)        | `22/TCP`               | SSH — administración del nodo                          | ✅           |
| ThinkPad (192.168.0.10)    | Raspberry Pi (192.168.0.200)| `53/TCP+UDP`           | Resolución DNS interna (zerops.internal)               | ✅           |
| ThinkPad (192.168.0.10)    | Raspberry Pi (192.168.0.200)| `9000/TCP`             | Step-CA ACME — cert-manager obtiene certificados TLS   | ✅           |
| ThinkPad (192.168.0.10)    | Ryzen (192.168.0.13)        | `6443/TCP`             | K8s API — Crossplane + provider-helm gestionan Ryzen   | ✅           |
| ThinkPad (192.168.0.10)    | Ryzen (192.168.0.13)        | `8472/UDP`             | Flannel VXLAN — overlay de red entre nodos K3s         | ✅           |
| ThinkPad (192.168.0.10)    | Ryzen (192.168.0.13)        | `10250/TCP`            | Metrics Server / kubelet read-only (si se necesita)    | ⚠️           |
| Ryzen (192.168.0.13)       | Raspberry Pi (192.168.0.200)| `53/TCP+UDP`           | Resolución DNS interna desde pods y nodo               | ✅           |
| Ryzen (192.168.0.13)       | ThinkPad (192.168.0.10)     | `8472/UDP`             | Flannel VXLAN — overlay de red (bidireccional)         | ✅           |

### 2.2 Flujos Internos al Nodo (loopback only)

| Nodo       | Servicio           | Puerto / Protocolo | Scope          | Propósito                                 | ¿Exponer a LAN? |
|------------|--------------------|--------------------|----------------|-------------------------------------------|-----------------|
| ThinkPad   | etcd               | `2379-2380/TCP`    | `127.0.0.1`    | Almacén de estado K3s                     | ❌ Nunca         |
| Ryzen      | etcd (embedded)    | `2379-2380/TCP`    | `127.0.0.1`    | Almacén de estado K3s                     | ❌ Nunca         |
| Raspberry Pi| PowerDNS API      | `8081/TCP`         | `127.0.0.1`    | API REST de gestión de zonas DNS          | ❌ Nunca         |
| ThinkPad   | kubelet            | `10250/TCP`        | nodo local     | Health checks internos                    | ⚠️ Solo ThinkPad |
| Ryzen      | kubelet            | `10250/TCP`        | nodo local     | Health checks internos + metrics server   | ⚠️ Solo ThinkPad |

### 2.3 Flujos Denegados (política explícita)

| Origen       | Destino         | Puerto            | Razón de denegación                                |
|--------------|-----------------|-------------------|----------------------------------------------------|
| Cualquiera   | Raspberry Pi    | `8081/TCP`        | API DNS — solo localhost, nunca exponer a LAN      |
| Cualquiera   | ThinkPad        | `2379-2380/TCP`   | etcd — solo loopback                               |
| Cualquiera   | Ryzen           | `2379-2380/TCP`   | etcd — solo loopback                               |
| Internet     | Cualquier nodo  | cualquier puerto  | Air-gapped — sin egreso/ingreso a Internet         |
| Ryzen        | ThinkPad        | `443/TCP`         | Workloads no acceden al plano de gestión           |
| Mac          | Ryzen           | `6443/TCP`        | kubectl al compute cluster va via Management CP    |

---

## 3. Reglas UFW por Nodo

> **Prerrequisito**: UFW instalado y `iptables` disponible en todos los nodos.
> ```bash
> sudo apt-get install -y ufw
> ```

> **⚠️ Advertencia**: Aplicar estas reglas en una sesión SSH activa. UFW no interrumpe conexiones existentes al activar, pero sí las nuevas. Verificar siempre la regla SSH antes de `ufw enable`.

---

### 3.1 🍓 Raspberry Pi (192.168.0.200) — DNS + PKI

```bash
#!/usr/bin/env bash
# =============================================================================
# UFW Rules — Raspberry Pi (192.168.0.200)
# Rol: DNS autoritativo (PowerDNS) + PKI (Step-CA)
# =============================================================================

# --- Políticas por defecto ---
sudo ufw default deny incoming        # Rechazar todo ingreso no explícito
sudo ufw default allow outgoing       # Permitir todo egreso (LAN interna)
sudo ufw default deny forward         # No enrutar tráfico

# --- Loopback (siempre permitir) ---
sudo ufw allow in on lo               # Loopback completo

# --- SSH: solo desde LAN ---
sudo ufw allow from 192.168.0.0/24 to any port 22 proto tcp \
  comment 'SSH - LAN management only'

# --- DNS: TCP + UDP desde LAN ---
# PowerDNS escucha en ambos protocolos (queries grandes van por TCP)
sudo ufw allow from 192.168.0.0/24 to any port 53 proto tcp \
  comment 'DNS TCP - PowerDNS desde LAN'
sudo ufw allow from 192.168.0.0/24 to any port 53 proto udp \
  comment 'DNS UDP - PowerDNS desde LAN'

# --- Step-CA ACME: solo desde ThinkPad (cert-manager) ---
# El ACME challenge va desde 192.168.0.10 (ThinkPad / cert-manager pods)
sudo ufw allow from 192.168.0.10 to any port 9000 proto tcp \
  comment 'Step-CA ACME - ThinkPad cert-manager only'

# --- PowerDNS API: BLOQUEO EXPLÍCITO desde exterior ---
# 8081 solo debe ser accesible via 127.0.0.1 en el mismo host
# UFW no interfiere con loopback, pero sí bloquea el acceso desde LAN
sudo ufw deny from 192.168.0.0/24 to any port 8081 proto tcp \
  comment 'DENY PowerDNS API desde LAN - solo localhost'

# --- Activar y verificar ---
sudo ufw --force enable
echo ""
echo "=== UFW Status — Raspberry Pi ==="
sudo ufw status verbose
```

**Resultado esperado de `ufw status verbose`:**

```
Status: active
Logging: on (low)
Default: deny (incoming), allow (outgoing), deny (forward)

To                         Action      From
--                         ------      ----
22/tcp                     ALLOW IN    192.168.0.0/24  # SSH - LAN management only
53/tcp                     ALLOW IN    192.168.0.0/24  # DNS TCP - PowerDNS desde LAN
53/udp                     ALLOW IN    192.168.0.0/24  # DNS UDP - PowerDNS desde LAN
9000/tcp                   ALLOW IN    192.168.0.10    # Step-CA ACME - ThinkPad cert-manager only
8081/tcp                   DENY IN     192.168.0.0/24  # DENY PowerDNS API desde LAN
```

---

### 3.2 💻 ThinkPad (192.168.0.10) — Management Cluster K3s

```bash
#!/usr/bin/env bash
# =============================================================================
# UFW Rules — ThinkPad (192.168.0.10)
# Rol: Management Cluster K3s — ArgoCD, Gitea, Choreo, Crossplane, Envoy GW
# =============================================================================

# --- Políticas por defecto ---
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw default deny forward

# --- Loopback ---
sudo ufw allow in on lo

# --- SSH: solo desde LAN ---
sudo ufw allow from 192.168.0.0/24 to any port 22 proto tcp \
  comment 'SSH - LAN management only'

# --- HTTPS (Envoy Gateway): desde LAN ---
# Expone: choreo.zerops.internal, argocd.zerops.internal, gitea.zerops.internal
sudo ufw allow from 192.168.0.0/24 to any port 443 proto tcp \
  comment 'HTTPS Envoy Gateway - choreo/argocd/gitea LAN access'

# --- K8s API (Management Cluster): desde LAN ---
# Permite kubectl desde el Mac del operador
sudo ufw allow from 192.168.0.0/24 to any port 6443 proto tcp \
  comment 'K8s API Management Cluster - kubectl desde LAN'

# --- Flannel VXLAN: SOLO desde Ryzen ---
# Tráfico de overlay de red entre nodos K3s
sudo ufw allow from 192.168.0.13 to any port 8472 proto udp \
  comment 'Flannel VXLAN UDP - desde Ryzen (compute cluster)'

# --- Kubelet metrics: SOLO desde Ryzen (si metrics-server lo requiere) ---
# Comentar si no se usa metrics-server cruzado
# sudo ufw allow from 192.168.0.13 to any port 10250 proto tcp \
#   comment 'kubelet - desde Ryzen (metrics-server, condicional)'

# --- etcd: DENEGACIÓN EXPLÍCITA desde exterior ---
# etcd solo debe escuchar en loopback (configurar K3s con --etcd-expose-metrics=false)
sudo ufw deny from 192.168.0.0/24 to any port 2379 proto tcp \
  comment 'DENY etcd client desde LAN - solo loopback'
sudo ufw deny from 192.168.0.0/24 to any port 2380 proto tcp \
  comment 'DENY etcd peer desde LAN - solo loopback'

# --- Activar y verificar ---
sudo ufw --force enable
echo ""
echo "=== UFW Status — ThinkPad ==="
sudo ufw status verbose
```

**Resultado esperado de `ufw status verbose`:**

```
Status: active
Logging: on (low)
Default: deny (incoming), allow (outgoing), deny (forward)

To                         Action      From
--                         ------      ----
22/tcp                     ALLOW IN    192.168.0.0/24  # SSH - LAN management only
443/tcp                    ALLOW IN    192.168.0.0/24  # HTTPS Envoy Gateway
6443/tcp                   ALLOW IN    192.168.0.0/24  # K8s API Management Cluster
8472/udp                   ALLOW IN    192.168.0.13    # Flannel VXLAN desde Ryzen
2379/tcp                   DENY IN     192.168.0.0/24  # DENY etcd client desde LAN
2380/tcp                   DENY IN     192.168.0.0/24  # DENY etcd peer desde LAN
```

> **Nota sobre Crossplane**: Crossplane y provider-helm corren en el ThinkPad y realizan llamadas *salientes* hacia Ryzen:6443. El firewall del ThinkPad (`allow outgoing`) permite esto. El firewall del **Ryzen** es quien restringe quién puede conectar a su 6443 (solo ThinkPad).

---

### 3.3 ⚙️ Ryzen (192.168.0.13) — Compute Cluster K3s

```bash
#!/usr/bin/env bash
# =============================================================================
# UFW Rules — Ryzen (192.168.0.13)
# Rol: Compute Cluster K3s — workloads de vclusters, pods de usuario
# =============================================================================

# --- Políticas por defecto ---
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw default deny forward

# --- Loopback ---
sudo ufw allow in on lo

# --- SSH: solo desde LAN ---
sudo ufw allow from 192.168.0.0/24 to any port 22 proto tcp \
  comment 'SSH - LAN management only'

# --- K8s API: SOLO desde ThinkPad (Crossplane + provider-helm) ---
# El Mac no accede directamente al API del Ryzen
sudo ufw allow from 192.168.0.10 to any port 6443 proto tcp \
  comment 'K8s API Compute Cluster - SOLO desde ThinkPad (Crossplane)'

# --- kubelet: SOLO desde ThinkPad ---
# Metrics Server, health checks y logs del Management Cluster
sudo ufw allow from 192.168.0.10 to any port 10250 proto tcp \
  comment 'kubelet - SOLO desde ThinkPad (metrics-server / log aggregation)'

# --- Flannel VXLAN: SOLO desde ThinkPad ---
sudo ufw allow from 192.168.0.10 to any port 8472 proto udp \
  comment 'Flannel VXLAN UDP - SOLO desde ThinkPad (management cluster)'

# --- NodePorts (vcluster internos): SOLO desde ThinkPad ---
# Los vclusters exponen sus APIs K8s en NodePorts del rango 30000-32767.
# Solo el Management Cluster (ThinkPad) necesita acceder a estos ports.
sudo ufw allow from 192.168.0.10 to any port 30000:32767 proto tcp \
  comment 'NodePorts K8s (vclusters) - SOLO desde ThinkPad'

# --- etcd: DENEGACIÓN EXPLÍCITA desde exterior ---
sudo ufw deny from 192.168.0.0/24 to any port 2379 proto tcp \
  comment 'DENY etcd client desde LAN - solo loopback'
sudo ufw deny from 192.168.0.0/24 to any port 2380 proto tcp \
  comment 'DENY etcd peer desde LAN - solo loopback'

# --- Activar y verificar ---
sudo ufw --force enable
echo ""
echo "=== UFW Status — Ryzen ==="
sudo ufw status verbose
```

**Resultado esperado de `ufw status verbose`:**

```
Status: active
Logging: on (low)
Default: deny (incoming), allow (outgoing), deny (forward)

To                         Action      From
--                         ------      ----
22/tcp                     ALLOW IN    192.168.0.0/24  # SSH - LAN management only
6443/tcp                   ALLOW IN    192.168.0.10    # K8s API - SOLO ThinkPad
10250/tcp                  ALLOW IN    192.168.0.10    # kubelet - SOLO ThinkPad
8472/udp                   ALLOW IN    192.168.0.10    # Flannel VXLAN - SOLO ThinkPad
30000:32767/tcp            ALLOW IN    192.168.0.10    # NodePorts vclusters - SOLO ThinkPad
2379/tcp                   DENY IN     192.168.0.0/24  # DENY etcd client
2380/tcp                   DENY IN     192.168.0.0/24  # DENY etcd peer
```

---

## 4. Hardening del OS

> Aplicar en **todos los nodos** salvo que se indique nodo específico. Estas medidas son independientes del firewall y complementan la postura de seguridad global.

### 4.1 Deshabilitar IPv6 (si no se usa)

El entorno Zerops usa exclusivamente IPv4 en la LAN. Deshabilitar IPv6 reduce la superficie de ataque.

```bash
# /etc/sysctl.d/99-disable-ipv6.conf
sudo tee /etc/sysctl.d/99-disable-ipv6.conf <<'EOF'
# Deshabilitar IPv6 en todas las interfaces
net.ipv6.conf.all.disable_ipv6     = 1
net.ipv6.conf.default.disable_ipv6 = 1
net.ipv6.conf.lo.disable_ipv6      = 1
EOF

sudo sysctl --system
# Verificar
cat /proc/sys/net/ipv6/conf/all/disable_ipv6  # debe retornar 1
```

> **Nota K3s**: Si K3s fue instalado sin `--disable-ipv6`, verificar que la configuración de Flannel no dependa de IPv6 antes de deshabilitar.

---

### 4.2 Deshabilitar Módulos de Kernel Innecesarios

```bash
# /etc/modprobe.d/zerops-hardening.conf
sudo tee /etc/modprobe.d/zerops-hardening.conf <<'EOF'
# Sistemas de archivos innecesarios
install cramfs  /bin/false
install freevxfs /bin/false
install jffs2   /bin/false
install hfs     /bin/false
install hfsplus /bin/false
install squashfs /bin/false  # Nota: comentar si se usan snaps
install udf     /bin/false

# Protocolos de red no usados
install dccp    /bin/false
install sctp    /bin/false
install rds     /bin/false
install tipc    /bin/false

# USB (si el nodo no necesita almacenamiento USB)
# install usb-storage /bin/false  # Descomentar si aplica
EOF

sudo update-initramfs -u 2>/dev/null || true
```

---

### 4.3 SSH Hardening

Aplicar en **todos los nodos** (Raspberry Pi, ThinkPad, Ryzen).

```bash
# Hacer backup de la configuración actual
sudo cp /etc/ssh/sshd_config /etc/ssh/sshd_config.bak.$(date +%Y%m%d)

# Aplicar configuración hardened
sudo tee /etc/ssh/sshd_config.d/99-zerops-hardening.conf <<'EOF'
# === Zerops SSH Hardening ===

# Autenticación: solo claves públicas
PasswordAuthentication no
ChallengeResponseAuthentication no
KbdInteractiveAuthentication no
UsePAM yes

# Sin acceso root directo
PermitRootLogin no

# Usuarios permitidos (ajustar según nodo)
# AllowUsers zerops-admin tu-usuario

# Reducir superficie
X11Forwarding no
AllowAgentForwarding yes    # necesario para kubectl port-forward proxies
AllowTcpForwarding yes      # necesario para kubectl port-forward
PrintMotd no
Banner /etc/issue.net

# Timeouts y límites
LoginGraceTime 30
MaxAuthTries 3
MaxSessions 5
ClientAliveInterval 300
ClientAliveCountMax 2

# Algoritmos modernos (OpenSSH 8.x+)
KexAlgorithms curve25519-sha256,curve25519-sha256@libssh.org,diffie-hellman-group16-sha512,diffie-hellman-group18-sha512
Ciphers chacha20-poly1305@openssh.com,aes256-gcm@openssh.com,aes128-gcm@openssh.com
MACs hmac-sha2-512-etm@openssh.com,hmac-sha2-256-etm@openssh.com

# Logging
LogLevel VERBOSE
SyslogFacility AUTH
EOF

# Validar configuración antes de recargar
sudo sshd -t && echo "Configuración SSH válida"

# Recargar SSH (sin cerrar sesiones activas)
sudo systemctl reload ssh
```

**Configurar banner de advertencia legal:**

```bash
sudo tee /etc/issue.net <<'EOF'
*******************************************************************************
*                      ZEROPS SOVEREIGN IDP — ACCESO RESTRINGIDO              *
*                                                                              *
* Este sistema es de uso exclusivo para personal autorizado.                   *
* Todo acceso es monitoreado y registrado.                                     *
* El acceso no autorizado está prohibido y puede ser perseguido legalmente.   *
*******************************************************************************
EOF
```

---

### 4.4 Fail2ban para SSH

```bash
# Instalar fail2ban
sudo apt-get install -y fail2ban

# Configuración para SSH
sudo tee /etc/fail2ban/jail.d/zerops-ssh.conf <<'EOF'
[sshd]
enabled   = true
port      = ssh
filter    = sshd
backend   = systemd
logpath   = /var/log/auth.log
maxretry  = 3
bantime   = 3600      ; 1 hora de ban
findtime  = 600       ; ventana de 10 minutos
ignoreip  = 127.0.0.1/8 192.168.0.0/24  ; no banear la LAN interna (ajustar si se requiere)
EOF

sudo systemctl enable fail2ban
sudo systemctl restart fail2ban

# Verificar estado
sudo fail2ban-client status sshd
```

> **Nota**: `ignoreip = 192.168.0.0/24` evita lockouts accidentales desde la LAN. En entornos de mayor seguridad, eliminar esa excepción.

---

### 4.5 sysctl Hardening

```bash
# /etc/sysctl.d/99-zerops-security.conf
sudo tee /etc/sysctl.d/99-zerops-security.conf <<'EOF'
# =============================================================================
# Zerops Sovereign IDP — sysctl Security Hardening
# Ref: CIS Ubuntu 22.04/24.04 LTS Benchmark, Sección 3 (Network Parameters)
# =============================================================================

# --- IP Forwarding ---
# IMPORTANTE: K3s requiere ip_forward=1 para el routing de pods.
# Dejar en 1 para nodos K3s (ThinkPad y Ryzen).
# Raspberry Pi no necesita forwarding: cambiar a 0 en ese nodo.
net.ipv4.ip_forward = 1

# --- Prevención de ataques de red ---
# No aceptar paquetes con rutas fuente (source routing)
net.ipv4.conf.all.accept_source_route    = 0
net.ipv4.conf.default.accept_source_route = 0

# No aceptar ICMP redirect (previene MITM en la LAN)
net.ipv4.conf.all.accept_redirects       = 0
net.ipv4.conf.default.accept_redirects   = 0
net.ipv4.conf.all.secure_redirects       = 0
net.ipv4.conf.default.secure_redirects   = 0

# No enviar ICMP redirects
net.ipv4.conf.all.send_redirects         = 0
net.ipv4.conf.default.send_redirects     = 0

# --- Reverse Path Filtering (anti-spoofing) ---
# Modo estricto: verificar que paquetes entrantes tienen ruta de retorno válida
# Nota: K3s/Flannel puede requerir rp_filter=2 (loose) en lugar de 1 (strict)
# Verificar con 'ip route' si hay rutas asimétricas antes de aplicar strict
net.ipv4.conf.all.rp_filter              = 1
net.ipv4.conf.default.rp_filter          = 1

# --- SYN Flood Protection ---
net.ipv4.tcp_syncookies                  = 1

# --- Log de paquetes sospechosos ---
net.ipv4.conf.all.log_martians           = 1
net.ipv4.conf.default.log_martians       = 1

# --- Ignorar broadcasts (Smurf attack mitigation) ---
net.ipv4.icmp_echo_ignore_broadcasts     = 1

# --- Ignorar errores ICMP falsos ---
net.ipv4.icmp_ignore_bogus_error_responses = 1

# --- Protección de memoria ---
kernel.randomize_va_space                = 2   # ASLR completo
kernel.dmesg_restrict                    = 1   # Solo root lee dmesg
kernel.kptr_restrict                     = 2   # Ocultar punteros del kernel
fs.protected_hardlinks                   = 1
fs.protected_symlinks                    = 1
EOF

# Aplicar cambios
sudo sysctl --system

# Verificar valores críticos
echo "=== Verificación sysctl ==="
sysctl net.ipv4.ip_forward
sysctl net.ipv4.conf.all.accept_redirects
sysctl net.ipv4.conf.all.rp_filter
sysctl kernel.randomize_va_space
```

> **⚠️ K3s y `rp_filter`**: Flannel VXLAN puede generar rutas asimétricas. Si K3s falla después de aplicar `rp_filter=1`, cambiar a `rp_filter=2` (modo loose) solo en los nodos K3s (ThinkPad y Ryzen).

---

## 5. CIS Benchmark References

### 5.1 Ubuntu OS Hardening

| Recurso | URL | Versión Aplicable |
|---------|-----|-------------------|
| CIS Ubuntu Linux 22.04 LTS Benchmark | https://www.cisecurity.org/cis-benchmarks#ubuntu_linux | Raspberry Pi, ThinkPad, Ryzen |
| CIS Ubuntu Linux 24.04 LTS Benchmark | https://www.cisecurity.org/cis-benchmarks#ubuntu_linux | Si actualización a 24.04 |
| Ubuntu Security Guide (USG) — herramienta oficial | https://ubuntu.com/security/certifications/docs/usg | Todos los nodos Ubuntu |
| OpenSCAP para Ubuntu | https://www.open-scap.org/ | Auditoría automatizada |

**Aplicar CIS Benchmark Nivel 1 con Ubuntu Security Guide (USG):**

```bash
# 1. Habilitar USG via Ubuntu Advantage / Pro
sudo apt-get install -y ubuntu-advantage-tools
sudo pro attach <TOKEN>           # Requiere cuenta Ubuntu Pro (gratuita para personal use)
sudo pro enable usg

# 2. Instalar ubuntu-security-guide
sudo apt-get install -y ubuntu-security-guide

# 3. Auditar el sistema contra CIS Level 1 Server
sudo usg audit cis_level1_server

# 4. Aplicar remediaciones automáticas (revisar el reporte antes de aplicar)
sudo usg fix cis_level1_server

# 5. Generar reporte HTML
sudo usg audit cis_level1_server --html-file /tmp/usg-report-$(hostname)-$(date +%Y%m%d).html
```

**Alternativamente, con OpenSCAP:**

```bash
# Instalar OpenSCAP
sudo apt-get install -y openscap-scanner libopenscap8 ssg-debderived

# Auditar contra CIS Ubuntu 22.04 Level 1
sudo oscap xccdf eval \
  --profile xccdf_org.ssgproject.content_profile_cis_level1_server \
  --report /tmp/openscap-report-$(hostname).html \
  /usr/share/xml/scap/ssg/content/ssg-ubuntu2204-ds.xml

echo "Reporte generado en /tmp/openscap-report-$(hostname).html"
```

---

### 5.2 Kubernetes / K3s Hardening

| Recurso | URL | Propósito |
|---------|-----|-----------|
| CIS Kubernetes Benchmark (v1.9+) | https://www.cisecurity.org/benchmark/kubernetes | Referencia primaria para K8s |
| K3s CIS Hardening Guide (oficial Rancher) | https://docs.k3s.io/security/hardening-guide | Configuración específica K3s |
| K3s CIS Self-Assessment | https://docs.k3s.io/security/self-assessment | Verificación manual control por control |
| kube-bench (herramienta de auditoría) | https://github.com/aquasecurity/kube-bench | Auditoría automatizada CIS K8s |

**Auditar K3s con kube-bench:**

```bash
# Ejecutar en el nodo a auditar (ThinkPad o Ryzen)
# Opción A: via Docker (si Docker está disponible)
docker run --pid=host \
  -v /etc:/etc:ro \
  -v /var:/var:ro \
  -v /usr/bin/k3s:/usr/bin/k3s:ro \
  aquasec/kube-bench:latest \
  --benchmark cis-1.9

# Opción B: binario directo (recomendado en air-gapped)
# Descargar kube-bench desde un entorno con Internet y transferir via SCP
# https://github.com/aquasecurity/kube-bench/releases
chmod +x kube-bench
sudo ./kube-bench --benchmark cis-1.9 --config-dir /etc/kube-bench/cfg \
  --output json > /tmp/kube-bench-$(hostname)-$(date +%Y%m%d).json

# Ver solo los FAILs
sudo ./kube-bench --benchmark cis-1.9 | grep -E "^\[FAIL\]"
```

**Configuraciones críticas de K3s para cumplir CIS:**

```bash
# En el archivo de configuración K3s: /etc/rancher/k3s/config.yaml
# (Aplicar según el rol del nodo)

# Servidor (ThinkPad — Management Cluster):
# kube-apiserver:
#   - --anonymous-auth=false
#   - --audit-log-path=/var/lib/rancher/k3s/server/audit.log
#   - --audit-log-maxage=30
#   - --audit-log-maxbackup=10
#   - --audit-log-maxsize=100
#   - --tls-min-version=VersionTLS12
#
# kube-controller-manager:
#   - --terminated-pod-gc-threshold=10
#
# Referencia completa: https://docs.k3s.io/security/hardening-guide
```

---

### 5.3 Container / Runtime Hardening

| Recurso | URL | Propósito |
|---------|-----|-----------|
| CIS Docker Benchmark | https://www.cisecurity.org/benchmark/docker | Base para hardening de runtime de contenedores |
| CIS Podman | Usar CIS Docker como base — Podman es drop-in replacement | Si se usa Podman en lugar de Docker |
| Falco (Runtime Security) | https://falco.org/ | Detección de comportamiento anómalo en tiempo real |

**Instalar Falco para detección de runtime:**

```bash
# Añadir repositorio Falco (ejecutar desde Mac con acceso, luego transferir paquete)
# En entorno air-gapped: descargar paquete .deb y transferir via SCP

# Instalación desde paquete local
sudo dpkg -i falco_<version>_amd64.deb
sudo systemctl enable falco
sudo systemctl start falco

# Ver alertas en tiempo real
sudo journalctl -fu falco

# Reglas críticas que Falco detecta:
# - Escritura en /etc, /bin, /usr/bin desde un proceso no esperado
# - Ejecución de shell en un contenedor
# - Acceso a /proc desde un contenedor
# - Cambios de privilegios (setuid, sudo) desde pods
```

---

### 5.4 Networking

| Recurso | URL | Aplicación |
|---------|-----|------------|
| CIS Benchmark para Dispositivos de Red | https://www.cisecurity.org/cis-benchmarks#network_devices | Router/switch de la LAN 192.168.0.0/24 |

> **Nota**: Para el switch/router que gestiona la LAN `192.168.0.0/24`, aplicar el CIS Benchmark correspondiente al fabricante (Cisco, MikroTik, etc.). Los controles mínimos incluyen: deshabilitar servicios no usados (Telnet, SNMP v1/v2), usar SSH v2 para gestión, VLANs para segmentación si es posible, y deshabilitar puertos físicos no usados.

---

## 6. Audit Commands Rápidos

Comandos para verificar el estado de seguridad de cada nodo sin necesidad de herramientas adicionales.

```bash
#!/usr/bin/env bash
# =============================================================================
# Zerops Sovereign IDP — Security Audit Quick Commands
# Ejecutar en cada nodo para verificar postura de seguridad
# =============================================================================

# --- 1. Estado de UFW ---
echo "=== [1] UFW Status ==="
sudo ufw status numbered
echo ""

# --- 2. Puertos en escucha activos ---
echo "=== [2] Puertos en escucha (ss -tulpn) ==="
sudo ss -tulpn
echo ""

# --- 3. Verificar que etcd NO está expuesto externamente ---
echo "=== [3] etcd exposure check (debe estar solo en 127.0.0.1) ==="
sudo ss -tulpn | grep -E ':2379|:2380' || echo "OK: etcd no detectado en puertos externos"
# Desde otro nodo de la LAN (sustituir IP del nodo a verificar):
# nmap -p 2379,2380 192.168.0.10
# nmap -p 2379,2380 192.168.0.13
echo ""

# --- 4. Verificar que PowerDNS API no está expuesta externamente ---
echo "=== [4] PowerDNS API exposure check (solo desde el Raspberry Pi) ==="
sudo ss -tulpn | grep ':8081' | grep -v '127.0.0.1' \
  && echo "⚠️  ALERTA: PowerDNS API escucha en interfaz pública!" \
  || echo "OK: PowerDNS API solo en localhost"
# Desde otro nodo de la LAN:
# nmap -p 8081 192.168.0.200  # debe aparecer como filtered/closed
echo ""

# --- 5. Auditar K3s con kube-bench (en nodos K3s) ---
echo "=== [5] kube-bench CIS K8s Audit ==="
echo "Ejecutar manualmente:"
echo "  sudo ./kube-bench --benchmark cis-1.9 | grep -E '^\[FAIL\]|^\[WARN\]'"
echo ""

# --- 6. Verificar SSH hardening ---
echo "=== [6] SSH Configuration Check ==="
sudo sshd -T | grep -E "passwordauthentication|permitrootlogin|x11forwarding|maxauthtries"
echo ""

# --- 7. Verificar fail2ban ---
echo "=== [7] Fail2ban Status ==="
sudo fail2ban-client status sshd 2>/dev/null || echo "fail2ban no activo o sin jail sshd"
echo ""

# --- 8. Verificar sysctl críticos ---
echo "=== [8] sysctl Security Parameters ==="
for param in \
  net.ipv4.ip_forward \
  net.ipv4.conf.all.accept_redirects \
  net.ipv4.conf.all.rp_filter \
  net.ipv4.tcp_syncookies \
  kernel.randomize_va_space \
  kernel.dmesg_restrict; do
  echo "  $param = $(sysctl -n $param)"
done
echo ""

# --- 9. Verificar módulos de kernel restringidos ---
echo "=== [9] Kernel Modules Check ==="
for mod in dccp sctp rds tipc cramfs; do
  lsmod | grep -q "^$mod " \
    && echo "  ⚠️  ALERTA: módulo $mod está cargado" \
    || echo "  OK: $mod no cargado"
done
echo ""

# --- 10. Conexiones activas ---
echo "=== [10] Active Connections ==="
sudo ss -tnp state established
```

---

## 7. Checklist de Validación (Exit Criteria)

Completar esta checklist en todos los nodos antes de considerar el hardening como aplicado. Marcar con `[x]` cuando el control es verificado.

### 7.1 Firewall (UFW)

- [ ] **UFW activo en Raspberry Pi** — `sudo ufw status` retorna `Status: active`
- [ ] **UFW activo en ThinkPad** — `sudo ufw status` retorna `Status: active`
- [ ] **UFW activo en Ryzen** — `sudo ufw status` retorna `Status: active`
- [ ] **Puerto 8081 (PowerDNS API) solo en localhost** — `nmap -p 8081 192.168.0.200` desde LAN retorna `filtered` o `closed`
- [ ] **Puerto 2379-2380 (etcd) solo en loopback en ThinkPad** — `nmap -p 2379,2380 192.168.0.10` desde LAN retorna `filtered` o `closed`
- [ ] **Puerto 2379-2380 (etcd) solo en loopback en Ryzen** — `nmap -p 2379,2380 192.168.0.13` desde LAN retorna `filtered` o `closed`
- [ ] **K8s API del Ryzen (6443) solo accesible desde ThinkPad** — `nmap -p 6443 192.168.0.13` desde Mac retorna `filtered`
- [ ] **Flannel (8472 UDP) solo entre ThinkPad y Ryzen** — verificado en reglas UFW de ambos nodos
- [ ] **NodePorts (30000-32767) del Ryzen solo accesibles desde ThinkPad** — regla UFW confirmada

### 7.2 SSH Hardening

- [ ] **PasswordAuthentication no** en los 3 nodos — `sudo sshd -T | grep passwordauthentication` retorna `no`
- [ ] **PermitRootLogin no** en los 3 nodos — `sudo sshd -T | grep permitrootlogin` retorna `no`
- [ ] **MaxAuthTries ≤ 3** en los 3 nodos
- [ ] **fail2ban activo** en los 3 nodos — `sudo fail2ban-client status sshd`
- [ ] **Banner de advertencia** configurado en `/etc/issue.net`

### 7.3 Hardening del OS

- [ ] **IPv6 deshabilitado** — `cat /proc/sys/net/ipv6/conf/all/disable_ipv6` retorna `1`
- [ ] **Módulos de red innecesarios bloqueados** (dccp, sctp, rds, tipc) — `/etc/modprobe.d/zerops-hardening.conf` presente
- [ ] **ASLR activado** — `sysctl kernel.randomize_va_space` retorna `2`
- [ ] **TCP SYN Cookies** — `sysctl net.ipv4.tcp_syncookies` retorna `1`
- [ ] **ICMP redirects deshabilitados** — `sysctl net.ipv4.conf.all.accept_redirects` retorna `0`
- [ ] **rp_filter activo** — `sysctl net.ipv4.conf.all.rp_filter` retorna `1` (o `2` en nodos K3s si se requiere)

### 7.4 CIS Benchmark

- [ ] **CIS Ubuntu Benchmark Nivel 1** aplicado en los 3 nodos — reporte USG/OpenSCAP sin FAILs críticos
- [ ] **kube-bench** pasa los controles críticos del CIS Kubernetes Benchmark en ThinkPad y Ryzen
  - Sin FAILs en categorías: `[master]` API Server, Controller Manager, Scheduler, etcd
  - Sin FAILs en categoría: `[node]` kubelet configuration
- [ ] **Falco** instalado y activo en nodos K3s (ThinkPad y Ryzen)

### 7.5 ADR (Architecture Decision Record)

| ID     | Decisión                                                        | Razón                                                                     |
|--------|-----------------------------------------------------------------|---------------------------------------------------------------------------|
| ADR-05-01 | UFW como firewall principal en lugar de nftables directo     | UFW provee abstracción más simple con semántica per-application, menor riesgo de error de configuración |
| ADR-05-02 | etcd solo en loopback (no cluster etcd externo)               | K3s embedded etcd. Menos superficie de ataque, suficiente para la escala del proyecto |
| ADR-05-03 | PowerDNS API solo en 127.0.0.1                                | La API no requiere autenticación fuerte por defecto; nunca exponer a LAN |
| ADR-05-04 | rp_filter=1 por defecto, excepciones documentadas para K3s    | Flannel VXLAN puede requerir loose mode; documentar y verificar caso a caso |
| ADR-05-05 | fail2ban ignoreip incluye 192.168.0.0/24                      | Evitar lockouts accidentales en entorno de laboratorio; revisar para producción |

---

## Referencias Cruzadas

| Documento | Relación |
|-----------|----------|
| [PB-04 GitOps & ArgoCD](./PB-04-gitops-argocd.md) | ← Anterior: despliegue del Management Cluster que este PB protege |
| [PB-06 Observability](./PB-06-observability.md) | → Siguiente: monitoreo y alertas de seguridad (Prometheus, Loki, Falco alerts) |
| [docs/network/topology.md](../network/topology.md) | Topología de red completa |
| [docs/adr/](../adr/) | Architecture Decision Records del proyecto |

---

> **Siguiente Paso → PB-06 Observability**: Una vez aplicadas y verificadas las reglas de firewall y hardening, el siguiente playbook cubre la pila de observabilidad: Prometheus + Grafana para métricas, Loki para logs centralizados, y la integración de alertas de Falco con el sistema de notificaciones del operador.

---

*Documento generado para Zerops Sovereign IDP — 2026-05-22 · Ambiente: air-gapped, dominio zerops.internal*
