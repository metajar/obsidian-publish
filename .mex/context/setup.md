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

- Go 1.26+ (server; pure-Go build verified with `CGO_ENABLED=0`)
- Node.js + npm (plugin; esbuild + vitest via `plugin/package.json`)
- Obsidian desktop — for loading `plugin/` as a vault plugin

## First-time Setup

1. Server: `cd server && go run ./cmd/server` — first run generates the API token (printed once) and session secret into the data dir
2. Copy the printed token into plugin settings (Server URL + API token), use "Test connection"
3. Plugin dev: `cd plugin && npm install && npm run build`, then enable the plugin in an Obsidian vault pointing at the built `main.js` (or symlink `plugin/` into `.obsidian/plugins/selfhosted-publish/`)
4. Deploy: run the binary behind Cloudflare Tunnel OR directly on a droplet — either is supported; rotate the token by deleting the token file and restarting

## Environment Variables

Server config (flags > env > defaults; flags: `-addr`, `-db`, `-token-file`, `-secret-file`, `-base-url`):
- `OBSPUB_ADDR` (optional) — listen address, default `:8080`
- `OBSPUB_DB` (optional) — SQLite path, default `data/obsidian-publish.db`
- `OBSPUB_TOKEN_FILE` (optional) — API token file, default `data/api-token` (0600)
- `OBSPUB_SECRET_FILE` (optional) — session-signing secret, default `data/session-secret` (0600)
- `OBSPUB_BASE_URL` (optional) — base for live URLs returned on publish; defaults to request Host

## Common Commands

- `cd server && go run ./cmd/server` — run server (first run prints the API token once)
- `cd server && go build ./... && go vet ./... && go test ./...` — build/lint/test server
- `CGO_ENABLED=0 go build -o server ./cmd/server` — static cross-compilable binary
- `cd plugin && npm install` — install plugin deps
- `cd plugin && npm run build` — typecheck (tsc) + production bundle (esbuild)
- `cd plugin && npm test` — vitest suite (slug + API client)
- `cd plugin && npm run dev` — esbuild watch

## Common Issues

**401 from every plugin call:** token mismatch — check the token file on the server matches plugin settings exactly (constant-time compare rejects prefix errors silently). Rotate by deleting the token file and restarting the server.
**Rate-limited while testing the password flow:** the limiter allows 5 auth attempts/min per IP+route and counts successes too — wait a minute or test from another route.
