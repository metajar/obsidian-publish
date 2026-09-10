---
name: setup
description: Dev environment setup and commands. Load when setting up the project for the first time or when environment issues arise.
triggers:
  - "setup"
  - "install"
  - "environment"
  - "getting started"
  - "how do I run"
  - "local development"
edges:
  - target: context/stack.md
    condition: when specific technology versions or library details are needed
  - target: context/architecture.md
    condition: when understanding how components connect during setup
  - target: context/auth-security.md
    condition: when generating or rotating the API token
# Ground only setup behavior implemented by specific code symbols.
# Entry shape: { node: "function:<tier-1-id>", fingerprint: "mh:64:<hex>" }
grounds_to: []
last_updated: 2026-09-10
---

# Setup

<!-- Commands and environment facts need no code grounding. For a concrete symbol:
```markdown
[`someFunction()`](mex://function:<tier-1-id>)
```
-->

## Prerequisites

- Go [TO BE DETERMINED — pin minimum version at first build]
- Node.js + npm [TO BE DETERMINED — pin minimum version at first build]
- Obsidian (desktop) — for installing/running the plugin from `plugin/` in a vault

## First-time Setup

[TO BE DETERMINED — populate after first implementation. Anticipated:]
1. Server: `cd server && go build ./cmd/server` (or `go run ./cmd/server`)
2. Server generates an API token on first setup; set server URL + token in plugin settings
3. Plugin: `cd plugin && npm install && npm run build`, then load `plugin/` as an Obsidian vault plugin
4. Deploy: run the binary behind Cloudflare Tunnel OR directly on a droplet — either is supported

## Environment Variables

[TO BE DETERMINED — populate after first implementation. Anticipated server config: listen address, SQLite storage path, API token / token file. The token is generated on first server setup and pasted once into plugin settings. Do not commit real values.]

## Common Commands

[TO BE DETERMINED — populate after first implementation. Anticipated: `go run ./cmd/server`, `go test ./...`, `go build ./cmd/server`; plugin `npm install`, `npm run dev` (esbuild watch), `npm run build`.]

## Common Issues

[TO BE DETERMINED — only record issues that actually occur.]
