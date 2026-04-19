---
title: Portainer MCP Safe
---

# Portainer MCP Safe

Portainer MCP Safe is an altered-source fork of Portainer MCP by Portainer.io.

- Original upstream source: [portainer/portainer-mcp](https://github.com/portainer/portainer-mcp)
- Fork repository: [Malaccamaxgit/portainer-mcp-safe](https://github.com/Malaccamaxgit/portainer-mcp-safe)

This fork keeps the upstream Portainer MCP model and tool surface, then adds a
safer default posture for AI-assisted use:

- secret-like stack env values are redacted by default
- compose file reads are redacted by default
- Docker proxy access is restricted to a small safe allowlist by default
- Kubernetes secret paths are blocked by default

## Start Here

- [Repository README](https://github.com/Malaccamaxgit/portainer-mcp-safe/blob/main/README.md)
- [Docker MCP Toolkit Usage](https://github.com/Malaccamaxgit/portainer-mcp-safe/blob/main/docker/USAGE.txt)
- [Client and Model Guide](clients_and_models.md)
- [Design Summary](design_summary.md)

## Repository Layout

- Go fork source lives at the repo root
- Docker MCP Toolkit packaging lives in `docker/`
- GitHub Pages content lives in `docs/`

## Licensing and Attribution

This repository keeps the upstream `LICENSE` unchanged and marks this
distribution as altered source. Fork attribution is captured in `NOTICE`.

## GitHub Pages URL

After Pages is enabled for this repository, the site should publish at:

`https://malaccamaxgit.github.io/portainer-mcp-safe/`
