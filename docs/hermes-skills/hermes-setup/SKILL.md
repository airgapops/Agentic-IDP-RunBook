---
name: hermes-setup
description: >-
  How to install and configure Hermes skills on the ThinkPad.
  Covers skill directory structure, loading, and testing.
version: 1.0.0
metadata:
  author: platform-team
  tags: [hermes, setup, skills, configuration]
---

# ⚙️ SKILL: Hermes Setup & Configuration

## Hermes Architecture

```
Hermes CLI (/usr/local/bin/hermes)
  └── Python venv: /usr/local/lib/hermes-agent/venv/
  └── Config: /root/.hermes/cli-config.yaml
  └── Skills: /root/.hermes/skills/
  └── HERMES.md: /root/HERMES.md (system instructions)
```

> **CRITICAL**: Hermes runs as **root** via sudo. All files must be in `/root/`, NOT `/home/rooter/`.

## Skill Directory Structure

```
/root/.hermes/skills/
├── openchoreo-deploy/
│   └── SKILL.md          # Required: YAML frontmatter + instructions
├── expose-service/
│   └── SKILL.md
├── cert-management/
│   └── SKILL.md
├── ryzen-network-fix/
│   └── SKILL.md
├── openchoreo-secrets/
│   └── SKILL.md
├── openchoreo-promote/
│   └── SKILL.md
├── openchoreo-observe/
│   └── SKILL.md
├── dns-infrastructure/
│   └── SKILL.md
├── k8s-provisioning/
│   └── SKILL.md
├── gpu-llm-setup/
│   └── SKILL.md
└── ops-infra/
    └── SKILL.md
```

## SKILL.md Format

Each `SKILL.md` MUST have YAML frontmatter:

```markdown
---
name: skill-name           # Required, max 64 chars
description: >-            # Required, max 1024 chars
  What this skill does...
version: 1.0.0             # Optional
metadata:
  author: platform-team
  tags: [tag1, tag2]
---

# Skill Title

Instructions here...
```

## How to Load Skills

### Step 1: Create skill files on your Mac

```bash
# Skills live in the airgapOps repo
ls ~/Documents/airgapOps/docs/hermes-skills/
```

### Step 2: Copy to ThinkPad (/root/.hermes/skills/)

```bash
# From your Mac:
sshpass -p '<CP_PASSWORD>' scp -r \
  ~/Documents/airgapOps/docs/hermes-skills/* \
  rooter@192.168.0.10:/tmp/hermes-skills/

# Then SSH to ThinkPad and copy to /root/:
sshpass -p '<CP_PASSWORD>' ssh rooter@192.168.0.10 \
  "echo '<CP_PASSWORD>' | sudo -S -k bash -c '
    cp -r /tmp/hermes-skills/* /root/.hermes/skills/
    chmod -R 755 /root/.hermes/skills/
  '"
```

### Step 3: Copy HERMES.md (system instructions)

```bash
sshpass -p '<CP_PASSWORD>' scp \
  ~/Documents/airgapOps/docs/hermes-config/HERMES.md \
  rooter@192.168.0.10:/tmp/HERMES.md

sshpass -p '<CP_PASSWORD>' ssh rooter@192.168.0.10 \
  "echo '<CP_PASSWORD>' | sudo -S -k cp /tmp/HERMES.md /root/HERMES.md"
```

### Step 4: Copy cli-config.yaml

```bash
sshpass -p '<CP_PASSWORD>' scp \
  ~/Documents/airgapOps/docs/hermes-config/config.yaml \
  rooter@192.168.0.10:/tmp/cli-config.yaml

sshpass -p '<CP_PASSWORD>' ssh rooter@192.168.0.10 \
  "echo '<CP_PASSWORD>' | sudo -S -k cp /tmp/cli-config.yaml /root/.hermes/cli-config.yaml"
```

### Step 5: Restart Hermes and verify

```bash
# Exit Hermes (Ctrl+C)
hermes chat

# Check skills loaded:
/skills list --source local
# Should show 12 local skills
```

## How to Use Hermes

### First message in each session (loads context):

```
Lee ~/HERMES.md y sigue esas instrucciones para todas las acciones.
```

### Deploy a service:

```
Despliega Nginx con HTTPS en el Ryzen siguiendo la skill openchoreo-deploy
```

### View status:

```
Muéstrame el estado de todos los servicios en el Ryzen
```

### Delete a service:

```
Elimina el componente nginx del cluster
```

## Troubleshooting

| Problema | Causa | Fix |
|----------|-------|-----|
| Skills no aparecen | Wrong directory | Skills must be in `/root/.hermes/skills/` |
| Hermes instala bare metal | HERMES.md not loaded | Say "Lee ~/HERMES.md primero" |
| `SKILL.md not found` | Missing frontmatter | Add `---\nname:\ndescription:\n---` header |
| Permissions error | Root vs rooter | `sudo chmod -R 755 /root/.hermes/skills/` |

## One-Liner: Sync Skills from Mac to ThinkPad

```bash
# Run this from your Mac after editing any skill:
sshpass -p '<CP_PASSWORD>' ssh rooter@192.168.0.10 \
  "echo '<CP_PASSWORD>' | sudo -S -k rm -rf /root/.hermes/skills/openchoreo-*" && \
sshpass -p '<CP_PASSWORD>' scp -r \
  ~/Documents/airgapOps/docs/hermes-skills/* \
  rooter@192.168.0.10:/tmp/hs/ && \
sshpass -p '<CP_PASSWORD>' ssh rooter@192.168.0.10 \
  "echo '<CP_PASSWORD>' | sudo -S -k bash -c '
    cp -r /tmp/hs/* /root/.hermes/skills/
    chmod -R 755 /root/.hermes/skills/
    echo SYNCED: \$(ls /root/.hermes/skills/ | wc -l) skills
  '"
```
