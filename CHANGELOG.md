# Changelog

All notable changes to **airgapOps — Sovereign IDP RunBook** will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.0.2] — 2026-06-11

### Added
- `CHANGELOG.md` — formal version history following Keep a Changelog spec
- `🔧 Build del CLI` section in README with `make`-based build instructions
  (`make build`, `make build-linux`, `make install`, `make deploy`)

### Changed
- `cli/Makefile`: aligned `VERSION` from `0.1.0` → `0.0.2` for consistency with git semver tags
- `README.md`: corrected CLI build section to use the canonical `Makefile` instead of raw `go build`
  commands; updated output path to `cli/bin/`

### Fixed
- Removed tracked compiled binaries (`cli/agctl`, `cli/agctl_linux` — 21 MB total) that were
  causing `send-pack: unexpected disconnect` on `git push`
- Added `cli/agctl` and `cli/agctl_linux` to `.gitignore` (previously only `cli/bin/` was covered)
- Added SSH keepalive config (`ServerAliveInterval 60`, `ServerAliveCountMax 10`) to prevent
  connection drops on large pushes

---

## [0.0.1] — 2026-06-10

### Added
- Initial release of the **airgapOps Agentic IDP RunBook**
- `agctl` CLI v0.1.0 — platform CLI for OpenChoreo deployments
  - Commands: `deploy`, `undeploy`, `promote`, `status`, `project`, `provision`,
    `restart`, `secret`, `storage`, `db`, `netpol`
  - Embedded version string via `-ldflags`
- Hermes AI MCP skills and configuration (`docs/hermes-skills/`, `docs/hermes-config/`)
- Onboarding guide and golden-path developer workflow documentation
- Gitea container registry integration (replaces external registry dependency)
- OpenChoreo-native stack — full ArgoCD removal
- Helm values for Control Plane (`openchoreo/values-cp.yaml`) and Data Plane (`openchoreo/values-dp.yaml`)
- Platform CRD declarations (`platform/project.yaml`, `platform/environments/`, `platform/components/`)
- Playbooks PB-00 through PB-06:
  - PB-00: Corporate Network Identity (PowerDNS + Step-CA)
  - PB-01: Management Cluster (ThinkPad / K3s)
  - PB-02: Compute Cluster (Ryzen / K3s)
  - PB-03: First Platform (OpenChoreo install)
  - PB-04: Developer Workflow (golden path CI/CD)
  - PB-05: Firewall & Hardening
  - PB-06: GPU LLM Stack (Hermes AI)
- Architecture cookbook and multi-cluster architecture documentation
- `.gitignore` covering secrets, kubeconfigs, PEM files, OS artifacts, IDE files

---

[0.0.2]: https://github.com/airgapops/Agentic-IDP-RunBook/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/airgapops/Agentic-IDP-RunBook/releases/tag/v0.0.1
