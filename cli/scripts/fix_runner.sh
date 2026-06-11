#!/bin/bash
# Fix act_runner config for host-only mode (no Docker)
cd /opt/act_runner

# 1. Remove docker labels from config, keep only host
python3 << 'PYEOF'
lines = open("config.yaml").readlines()
out = []
skip_docker_labels = False
for line in lines:
    if 'docker://' in line:
        continue  # skip docker label lines
    if 'docker_host: ""' in line:
        out.append(line.replace('docker_host: ""', 'docker_host: "-"'))
    else:
        out.append(line)
open("config.yaml","w").writelines(out)
print("config.yaml fixed")
PYEOF

# 2. Fix .runner labels
python3 << 'PYEOF'
import json
d = json.load(open(".runner"))
d["labels"] = ["ubuntu-latest:host", "self-hosted:host"]
json.dump(d, open(".runner","w"), indent=2)
print(".runner labels fixed:", d["labels"])
PYEOF

echo "=== Config labels ==="
grep -A3 "labels:" config.yaml
echo "=== Docker host ==="
grep "docker_host" config.yaml
echo "=== Runner labels ==="
grep labels .runner
