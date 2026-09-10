---
name: stack
description: Technology stack, library choices, and the reasoning behind them. Load when working with specific technologies or making decisions about libraries and tools.
triggers:
  - "library"
  - "package"
  - "dependency"
  - "which tool"
  - "technology"
edges:
  - target: context/decisions.md
    condition: when the reasoning behind a tech choice is needed
  - target: context/conventions.md
    condition: when understanding how to use a technology in this codebase
  - target: context/architecture.md
    condition: when understanding where each technology sits in the system
# Broad inventory: ground only claims embodied by a small number of symbols.
# Entry shape: { node: "function:<tier-1-id>", fingerprint: "mh:64:<hex>" }
grounds_to: []
last_updated: 2026-09-10
---

# Stack

<!-- Keep grounding sparse here. For a concrete wrapper or adapter mention, use:
```markdown
[`someFunction()`](mex://function:<tier-1-id>)
```
-->

## Core Technologies

- **Go** — server language; compiles to a single static binary for Linux ARM/x86 (systemd or container, home server or droplet).
- **TypeScript** — Obsidian plugin language, built with the standard Obsidian plugin API + esbuild scaffold.
- **SQLite** — embedded storage (route table, pages, password hashes, theme); no external DB service.

## Key Libraries

- **Echo** (not stdlib-only, not chi/gin) — HTTP framework for the server; middleware carries token auth and rate limiting.
- **modernc.org/sqlite** (not mattn/go-sqlite3) — pure-Go SQLite driver, no CGo, keeps cross-compilation one-command.
- **goldmark** — Markdown → HTML rendering; full CommonMark + GFM (tables, strikethrough, task lists) for complete Markdown coverage.
- **golang.org/x/crypto/argon2** (not bcrypt) — argon2id password hashing for protected pages.
- **esbuild + npm scripts** — plugin build tooling (standard Obsidian scaffold).

## What We Deliberately Do NOT Use

- No external database (Postgres, MySQL) — SQLite file only; minimal ops burden for a personal project.
- No CGo anywhere — the server must stay a trivially cross-compilable static binary.
- No third-party publishing services (Obsidian Publish, Notion, etc.) — zero notes leave the user's own infrastructure.
- No analytics/tracking libraries in v1.

## Version Constraints

- **Go 1.26** — server builds with the toolchain at first implementation; `CGO_ENABLED=0 go build` verified (pure-Go only).
- Server deps pinned in `server/go.mod`: Echo v4.15.4, modernc.org/sqlite v1.58.0, goldmark v1.8.6, golang.org/x/crypto v0.57.0.
- Node/esbuild versions: TBD at plugin build time.
