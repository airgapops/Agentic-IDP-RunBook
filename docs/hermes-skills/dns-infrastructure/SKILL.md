# 🧠 SKILL: AirgapOps — Arquitectura DNS (PowerDNS Authoritative + Recursor)

## 📌 Contexto del Problema

Actualmente, el servidor DNS primario en la Raspberry Pi (`192.168.0.9`) ejecuta **PowerDNS Authoritative Server**. Este tipo de servidor es excelente para alojar y servir zonas locales (como `zerops.internal`), pero **no resuelve consultas externas** (como `google.com` o `github.com`).

**Síntomas:**
- Si un equipo (ej. la Mac) configura su DNS primario como `192.168.0.9`, podrá resolver `api.zerops.internal`, pero **perderá el acceso a internet**.
- El ThinkPad u otros equipos que logran resolver el dominio pueden estar utilizando configuraciones divididas, resolv.conf locales manipulados o interfaces secundarias.

**La Solución Estándar (Recomendada):**
Implementar una arquitectura combinada de **PowerDNS Recursor** (caché y resolución recursiva) + **PowerDNS Authoritative** (zonas internas).

---

## 🏗️ Arquitectura Propuesta

```text
Mac / ThinkPad / Nodos K3s
        │
        ▼ (Puerto 53)
┌───────────────────────────────────────────────┐
│              Raspberry Pi (192.168.0.9)       │
│                                               │
│  1️⃣ PowerDNS Recursor (Escucha en 0.0.0.0:53)  │
│      ├── Consulta para internet (google.com)  ───▶ 1.1.1.1 / 8.8.8.8
│      └── Consulta para `zerops.internal`      ───▶ (Forward a localhost:5300)
│                                               │
│  2️⃣ PowerDNS Authoritative (Escucha en :5300)  │
│      └── Base de datos (SQLite/Postgres) con  │
│          los registros A de *.zerops.internal │
└───────────────────────────────────────────────┘
```

Con este diseño:
1. Configuras el DHCP de tu router para que entregue `192.168.0.9` como servidor DNS a **toda la red** (Mac, móviles, etc.).
2. Toda la red tendrá internet.
3. Toda la red podrá resolver los subdominios locales automáticamente sin tocar el `/etc/hosts` de cada máquina.

---

## ⚙️ Pasos de Implementación para Hermes

Si el usuario (Tú) le pide a Hermes implementar esto, Hermes debe seguir este plan:

### 1. Reconfigurar PowerDNS Authoritative (Mover puerto)
Actualmente el Authoritative Server escucha en el puerto 53.
- Modificar el archivo de configuración `pdns.conf` o la variable de entorno de Docker.
- Cambiar `local-port=53` a `local-port=5300`.
- Cambiar `local-address=0.0.0.0` a `local-address=127.0.0.1`.
- Reiniciar el servicio/contenedor Authoritative.

### 2. Instalar y Configurar PowerDNS Recursor
- Instalar el recursor (ej. `apt install pdns-recursor` o usando Docker en el Pi).
- Editar `recursor.conf`:
  ```ini
  # Escuchar en la red local
  local-address=0.0.0.0
  local-port=53
  
  # Permitir consultas desde la red local (ej. 192.168.0.0/24)
  allow-from=127.0.0.0/8, 192.168.0.0/24
  
  # Reenviar consultas DNS de la zona interna al Authoritative Server local
  forward-zones=zerops.internal=127.0.0.1:5300
  
  # Reenviar el resto de consultas a DNS públicos (opcional, por defecto hace resolución recursiva directa a los root servers)
  forward-zones-recurse=.=1.1.1.1;8.8.8.8
  ```
- Reiniciar el recursor.

### 3. Ajustar el DNS del router o los clientes
- En la Mac: `Configuración de Red -> Wi-Fi -> Detalles -> DNS`, añadir `192.168.0.9`. Eliminar los demás o dejarlos como secundarios (aunque macOS usa el primario para casi todo).
- **Prueba desde la Mac**: 
  ```bash
  nslookup api.zerops.internal 192.168.0.9
  nslookup google.com 192.168.0.9
  ```
  Ambas deben devolver una IP válida sin errores.

---

## 🚀 Cómo proceder ahora mismo
Para que puedas usar la Mac inmediatamente sin problemas:

1. **Si quieres que yo (la IA actual) implemente este cambio en la Raspberry Pi ahora mismo**, dímelo y prepararé los comandos SSH y configuraciones de Docker/Systemd.
2. **Si prefieres usar `/etc/hosts` temporalmente** solo en la Mac para seguir probando los vclusters hoy, ejecuta:
   `echo "192.168.0.240 api.zerops.internal db.zerops.internal queue.zerops.internal" | sudo tee -a /etc/hosts`
