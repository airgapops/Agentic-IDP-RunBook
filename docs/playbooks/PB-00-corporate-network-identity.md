# PB-00: Corporate Network & Identity Tier
> **"El día cero antes del día uno"**

Este playbook aprovisiona la capa fundacional de red (DNS Split-Horizon) y la Infraestructura de Llave Pública (PKI) privada. Sin esto, un clúster de Kubernetes corporativo no puede enrutar tráfico interno ni asegurar comunicaciones mTLS.

---

## 📋 Prerequisitos de Hardware y OS

| Elemento | Especificación |
|---|---|
| **Hardware** | Nodo dedicado (ej. Raspberry Pi 4/5) aislado del clúster principal |
| **OS** | Ubuntu Server Minimal 22.04 LTS / 24.04 LTS — Arquitectura ARM64 |
| **Red** | IP local estática asignada (IP real: `192.168.0.9`) |
| **Dominio Base** | `zerops.internal` |

### Hardware de referencia en este IDP
| Nodo | IP | Rol |
|---|---|---|
| Raspberry Pi 4 | `192.168.0.9`  | DNS + PKI (PB-00) |
| ThinkPad | `192.168.0.10` | Management Cluster (PB-01) |
| Ryzen | `192.168.0.13` | Compute Node (PB-02) |

---

## 🏛️ ADR — Architecture Decision Records

### ADR-1: Podman (Rootless) en lugar de Docker
**Decisión:** Usar Podman en modo rootless para el nodo de infraestructura de red.  
**Rationale:** Mitiga ataques de escalada de privilegios. Los demonios de red no corren como administrador. El namespace de red de Linux aísla los procesos contenedorizados sin requerir un daemon privilegiado.

### ADR-2: Step-CA en lugar de Let's Encrypt
**Decisión:** Usar Smallstep Step-CA como CA ACME privada interna.  
**Rationale:** Los dominios `.internal` no son resolubles públicamente. Step-CA provee un servidor ACME 100% privado para entornos Air-gapped, compatible con cert-manager de Kubernetes.

### ADR-3: PowerDNS con SQLite en lugar de CoreDNS o Bind9
**Decisión:** PowerDNS Auth con backend SQLite como servidor DNS autoritativo.  
**Rationale:** API REST nativa para gestión dinámica de registros. CoreDNS no tiene API de escritura. Bind9 requiere edición de archivos de zona. PowerDNS permite automatización completa.

---

## 🚀 The Runbook

### Fase 1: Liberación de Red y Preparación del OS

Ubuntu viene con un caché de DNS que ocupa el puerto 53. Debemos liberarlo.

**1. Desactivar el stub DNS de systemd-resolved:**
```bash
sudo mkdir -p /etc/systemd/resolved.conf.d
echo -e "[Resolve]\nDNSStubListener=no" | sudo tee /etc/systemd/resolved.conf.d/disable-stub.conf
sudo systemctl restart systemd-resolved
```

**2. Enlazar el DNS maestro del host y permitir puertos no privilegiados:**
```bash
sudo rm /etc/resolv.conf
sudo ln -s /run/systemd/resolve/resolv.conf /etc/resolv.conf
echo "net.ipv4.ip_unprivileged_port_start=53" | sudo tee /etc/sysctl.d/99-podman-ports.conf
sudo sysctl --system
```

---

### Fase 2: Motor Rootless y Estructura de Datos

**1. Instalar paquetes core:**
```bash
sudo apt-get update
sudo apt-get install -y podman podman-compose sqlite3 curl
```

**2. Crear árbol de directorios (ejecutar como usuario normal):**
```bash
mkdir -p ~/zerops-network/pdns/conf.d
mkdir -p ~/zerops-network/pdns/data
mkdir -p ~/zerops-network/step-ca

# Forzar mapeo de UID 1000 para el contenedor de la CA
podman unshare chown -R 1000:1000 ~/zerops-network/step-ca
cd ~/zerops-network
```

---

### Fase 3: Inyección de la Base de Datos (PowerDNS)

Para evitar problemas de permisos de volumen subuid en Rootless, creamos la BD en el host:

```bash
# Descargar esquema oficial y crear la base de datos
curl -s https://raw.githubusercontent.com/PowerDNS/pdns/master/modules/gsqlite3backend/schema.sqlite3.sql \
  | sqlite3 ~/zerops-network/pdns/data/pdns.sqlite3

# Otorgar permisos amplios a la ruta de datos (Solo dentro del contexto Rootless)
chmod 777 ~/zerops-network/pdns/data
chmod 666 ~/zerops-network/pdns/data/pdns.sqlite3
```

---

### Fase 4: Configuración Declarativa

**1. Crear `pdns/conf.d/pdns.conf`:**
```bash
nano ~/zerops-network/pdns/conf.d/pdns.conf
```

Contenido:
```ini
launch=gsqlite3
gsqlite3-database=/var/lib/powerdns/pdns.sqlite3
gsqlite3-dnssec=yes
api=yes
api-key=<POWERDNS_API_KEY>
webserver=yes
webserver-address=0.0.0.0
webserver-allow-from=0.0.0.0/0
webserver-port=8081
log-dns-details=yes
loglevel=4
```

**2. Crear `docker-compose.yml`:**

> ⚠️ Reemplazar `[TU_IP_RASPBERRY]` por la IP real del nodo (ej: `192.168.0.9`)

```yaml
version: '3.8'

services:
  powerdns:
    image: docker.io/powerdns/pdns-auth-48:latest
    container_name: powerdns
    restart: always
    ports:
      - "53:53/tcp"
      - "53:53/udp"
      - "8081:8081/tcp"
    cap_add:
      - NET_BIND_SERVICE
    volumes:
      - ./pdns/conf.d:/etc/powerdns/pdns.d
      - ./pdns/data:/var/lib/powerdns
    command: --config-dir=/etc/powerdns --include-dir=/etc/powerdns/pdns.d

  step-ca:
    image: docker.io/smallstep/step-ca:latest
    container_name: step-ca
    restart: always
    ports:
      - "9000:9000"
    volumes:
      - ./step-ca:/home/step
    environment:
      - DOCKER_STEPCA_INIT_NAME=Zerops Internal Root CA
      - DOCKER_STEPCA_INIT_DNS_NAMES=localhost,step-ca,step-ca.zerops.internal,[TU_IP_RASPBERRY]
      - DOCKER_STEPCA_INIT_PROVISIONER_NAME=admin
      - DOCKER_STEPCA_INIT_PASSWORD=ChangeThisStrongPassword123!
```

---

### Fase 5: Despliegue y Activación

**1. Desplegar los servicios:**
```bash
podman-compose up -d
# Validar con: podman ps (ambos contenedores deben estar 'Up')
```

**2. Crear la Zona Corporativa DNS:**
```bash
curl -X POST -H "X-API-Key: <POWERDNS_API_KEY>" \
  -d '{"name":"zerops.internal.", "kind": "Native"}' \
  http://localhost:8081/api/v1/servers/localhost/zones
```

**3. Extraer el Certificado Raíz:**
```bash
podman exec -it step-ca cat /home/step/certs/root_ca.crt > root_ca.crt
```

---

## ✅ Checklist de Validación (Exit Criteria)

- [ ] El puerto 53 (UDP/TCP) está abierto y asignado a Podman
- [ ] La API de PowerDNS devuelve JSON con la zona `zerops.internal`
- [ ] El archivo `root_ca.crt` existe en el directorio actual
- [ ] Guardar este archivo de forma segura — será inyectado en el PB-01

---

## 🔗 Siguiente Paso

→ **[PB-01: Management Cluster (ThinkPad)](./PB-01-management-cluster.md)** — Levantar el clúster de K3s en el ThinkPad, conectarlo a este DNS y hacer que reconozca el certificado de Step-CA.

---

*Parte del [Zerops Sovereign IDP](../../README.md) — Air-gapped Internal Developer Platform*
