---
name: router
description: Session bootstrap and navigation hub. Read at the start of every session before any task. Contains project state, routing table, and behavioural contract.
edges:
  - target: context/architecture.md
    condition: when working on system design, integrations, or understanding how components connect
  - target: context/stack.md
    condition: when working with specific technologies, libraries, or making tech decisions
  - target: context/conventions.md
    condition: when writing new code, reviewing code, or unsure about project patterns
  - target: context/decisions.md
    condition: when making architectural choices or understanding why something is built a certain way
  - target: context/setup.md
    condition: when setting up the dev environment or running the project for the first time
  - target: context/auth-security.md
    condition: when touching token auth, password gating, sessions, or rate limiting
  - target: patterns/INDEX.md
    condition: when starting a task — check the pattern index for a matching pattern file
last_updated: 2026-09-10 (plugin: theme settings + image publishing)
---

# Session Bootstrap

If you haven't already read `AGENTS.md`, read it now — it contains the project identity, non-negotiables, and commands.

Then read this file fully before doing anything else in this session.

## Current Project State

**Working:**
- `PRD.md` — complete product spec (scope, flows, requirements, API sketch)
- Go server (`server/`, Go module `obsidian-publish/server`): publish/update/delete/list + route-availability APIs behind bearer-token middleware, public page serving with password gate, argon2id hashing, route-scoped HMAC session cookies, per-IP+route rate limiting, SQLite (modernc, pure-Go) persistence, goldmark rendering at publish time. `go build/vet/test ./...` all pass from `server/`.
- Obsidian plugin (`plugin/`): publish command (palette + editor context menu), publish modal (slug prefill, debounced availability check, CSPRNG password generation, update-mode password semantics), settings tab (server URL + masked token, test connection, live published-pages table with confirmed unpublish + copy link, theme section with 4 built-in presets + custom CSS pushed via POST /api/theme, loaded live via GET /api/theme), image publishing on publish/re-publish (wiki `![[img.png|400]]` and local `![alt](path.png)` embeds uploaded via POST /api/assets and rewritten to server URLs in the outbound payload only — the note on disk is never modified; remote images, non-image attachments reported via Notice, upload failures fail soft, dedupe per vault file). `npm run build` clean (zero TS errors), 85/85 vitest tests pass.
- End-to-end smoke (2026-09-10, `main` after merge): 401 without token → publish → serve → password gate (form-only pre-auth, 401 wrong password, 303+cookie correct, content only with cookie) → list (no hash exposure) → delete → 404. All passed.
- API contract frozen between plugin and server: bare-array `GET /api/pages`, `DELETE`→204, PUT password omitted=keep / null=remove / string=set, 401/409/400/404 error mapping.

**Not yet built:**
- Theming + asset endpoints on the server (`GET/POST /api/theme`, `POST /api/assets`) — plugin side is built against the frozen contract; server implementation is in progress in parallel
- Deployment setup (Cloudflare Tunnel or droplet — owner's choice at deploy time; systemd unit/container not yet written)
- Plugin↔server integration test inside Obsidian itself (plugin built against the frozen contract; verified by unit tests + server-side e2e only)

**Known issues:**
- Open decisions in `context/decisions.md`: theming scope, route namespace, asset handling, custom CSS limits (session mechanism is now decided: route-scoped cookie)

## Routing Table

Load the relevant file based on the current task. Always load `context/architecture.md` first if not already in context this session.

| Task type | Load |
|-----------|------|
| Understanding how the system works | `context/architecture.md` |
| Working with a specific technology | `context/stack.md` |
| Writing or reviewing code | `context/conventions.md` |
| Making a design decision | `context/decisions.md` |
| Setting up or running the project | `context/setup.md` |
| Auth, tokens, passwords, sessions, rate limiting | `context/auth-security.md` |
| Any specific task | Check `patterns/INDEX.md` for a matching pattern |

## Behavioural Contract

For every task, follow this loop:

1. **CONTEXT** — Load the relevant context file(s) from the routing table above. Check `patterns/INDEX.md` for a matching pattern. If one exists, follow it.
2. **BUILD** — Do the work. If a pattern exists, follow its Steps. If you are about to deviate from an established pattern, say so before writing any code — state the deviation and why.
3. **VERIFY** — Load `context/conventions.md` and run the Verify Checklist item by item. State each item and whether the output passes. Do not summarise — enumerate explicitly.
4. **DEBUG** — If verification fails or something breaks, check `patterns/INDEX.md` for a debug pattern. Follow it. Fix the issue and re-run VERIFY.
5. **GROW** — After meaningful work, run this binary checklist:
   - **Ground:** What changed in reality? Name the changed behavior, system, command, dependency, or workflow.
   - **Record:** If project state changed, update the "Current Project State" section above. If documented facts changed, update the relevant `context/` file surgically.
   - **Orient:** If this task can recur and no pattern exists, create one in `patterns/` using `patterns/README.md`, then add it to `patterns/INDEX.md`. If a pattern exists but you learned a gotcha, update it.
   - **Write:** Bump `last_updated` in every scaffold file you changed. Read `mex logging --json` before optional `mex log` notes: `significant` records material rationale, `checkpoints` batches useful notes at task/session boundaries, and `manual` avoids unsolicited notes. Honor explicit user log requests in every mode; mandatory workflow Activity and recovery audits remain required.
