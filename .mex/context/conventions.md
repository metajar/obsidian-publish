---
name: conventions
description: How code is written in this project — naming, structure, patterns, and style. Load when writing new code or reviewing existing code.
triggers:
  - "convention"
  - "pattern"
  - "naming"
  - "style"
  - "how should I"
  - "what's the right way"
edges:
  - target: context/architecture.md
    condition: when a convention depends on understanding the system structure
  - target: context/auth-security.md
    condition: when writing code that touches passwords, tokens, or sessions
  - target: context/stack.md
    condition: when checking which library to use for a task
# Add only nodes that embody the documented convention; do not ground examples broadly.
# grounds_to:
#   - node: "function:<tier-1-id>"
#     fingerprint: "mh:64:<hex>"
grounds_to: []
last_updated: 2026-09-10
---

# Conventions

<!-- Read broad, ground tight. Anchor concrete symbols while keeping prose readable:
```markdown
[`someFunction()`](mex://function:<tier-1-id>)
```
-->

## Naming

[TO BE DETERMINED — populate after first implementation. Anticipated: Go packages lowercase short names, handler files per resource (`pages.go`, `theme.go`); DB columns snake_case (`password_hash`, `created_at`); plugin files per Obsidian scaffold convention (`main.ts`, `styles.css`).]

## Structure

[TO BE DETERMINED — populate after first implementation. Anticipated: monorepo with `server/` (Go module, `cmd/server` entrypoint) and `plugin/` (Obsidian plugin) at the top level; reader-facing routes and the token-authenticated `/api/*` routes stay separate in the server.]

## Patterns

- Reader-facing endpoints and plugin API endpoints are fully separate. Never mix: `/api/*` requires the bearer token; `GET /{route}` and `POST /{route}/auth` are public (page password gate only). Do not add auth-dependent behavior to public routes or vice versa.
- The plugin never treats its local note↔route map as truth — anything the user sees about published state (settings list, modal pre-fill) is fetched live from the server.
- Password handling: plaintext passwords exist only transiently in the publish request and the reader auth form; everything stored is an argon2id hash.

## Verify Checklist

Before presenting any code:
- [ ] No page password is stored or logged in plaintext — argon2id hash only.
- [ ] Every new `/api/*` route is behind the bearer-token middleware.
- [ ] Protected-page content (and any hash) is unreachable before password verification — no leakage in error paths or redirects.
- [ ] Password-gated routes have rate limiting on auth attempts.
- [ ] New routes validate the slug format (URL-safe) before storage.
- [ ] Server changes keep persistence in SQLite (survives restart); no in-memory-only state for published pages.
