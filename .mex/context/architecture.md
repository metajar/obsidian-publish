---
name: architecture
description: How the major pieces of this project connect and flow. Load when working on system design, integrations, or understanding how components interact.
triggers:
  - "architecture"
  - "system design"
  - "how does X connect to Y"
  - "integration"
  - "flow"
edges:
  - target: context/stack.md
    condition: when specific technology details are needed
  - target: context/decisions.md
    condition: when understanding why the architecture is structured this way
  - target: context/auth-security.md
    condition: when touching token auth, password gating, sessions, or rate limiting
  - target: context/setup.md
    condition: when deploying or running the server/plugin for the first time
# Broad overview: keep this empty unless a claim depends on a few specific symbols.
# Entry shape: { node: "function:<tier-1-id>", fingerprint: "mh:64:<hex>" }
grounds_to: []
last_updated: 2026-09-10
---

# Architecture

<!-- Read broad, ground tight. Architecture usually grounds sparsely. When a
     specific symbol is worth navigating to, use this inline form:
```markdown
[`someFunction()`](mex://function:<tier-1-id>)
```
-->

## System Overview

Two components, one repo: an **Obsidian plugin** (TypeScript, runs in the user's vault) and a **Go server** (single binary, self-hosted).

Publish flow: user runs "Publish" command in a note → plugin opens a modal (route slug, optional password) → plugin checks route availability with the server → plugin sends Markdown + route + optional password + theme over the authenticated API (`Authorization: Bearer <token>`, HTTPS) → server persists the page (SQLite), hashes the password (argon2id), renders Markdown → HTML (goldmark), applies theme CSS, returns the live URL → plugin shows a toast with copy-link.

Read flow: reader hits `GET /{route}` → if unprotected, server serves rendered HTML; if protected, server serves a password-entry page → reader submits password → server verifies against the argon2id hash → on success issues a short-lived route-scoped session and serves the content.

Unpublish flow: plugin Settings → Published Pages (sourced live from `GET /api/pages`) → Unpublish → server deletes/disables the route immediately → route 404s thereafter.

## Key Components

- **Obsidian plugin** — publish command + modal, settings (server URL, token, page list, unpublish, theme), tracks note↔route state locally but server is always the source of truth. Never persists note content beyond sending it on publish.
- **Go server** — Echo-based HTTP API + public page serving; holds route table, rendered pages, password hashes, theme config; persists everything to SQLite so restarts lose nothing.
- **Password gate** — reader-facing; serves password-entry page instead of content until verification succeeds; rate-limited attempts (see `context/auth-security.md`).
- **Render pipeline** — goldmark (CommonMark + GFM) Markdown → HTML, wrapped in the active theme's CSS.

## External Dependencies

- **Cloudflare Tunnel** — one supported deployment path for the home server (no inbound ports). NOT a requirement: a direct-to-droplet deployment (e.g. DigitalOcean) is equally valid; the app is deployment-agnostic.
- **SQLite (modernc.org/sqlite)** — embedded storage for route table, page content, password hashes, theme config. No external database service.
- **Obsidian** — host application for the plugin; the server deliberately knows nothing about vault structure.

## What Does NOT Exist Here

- No multi-user / multi-vault support — single-owner personal tool.
- No analytics or visitor tracking — only basic operational logging of publish/unpublish/access events.
- No asset/attachment publishing (images, embeds) in MVP — fast-follow.
- No auto-republish on save — publishing is always an explicit user action (possible v2 toggle).
- No real-time collaboration or comments on published pages.
