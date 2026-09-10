---
name: decisions
description: Key architectural and technical decisions with reasoning. Load when making design choices or understanding why something is built a certain way.
triggers:
  - "why do we"
  - "why is it"
  - "decision"
  - "alternative"
  - "we chose"
edges:
  - target: context/architecture.md
    condition: when a decision relates to system structure
  - target: context/stack.md
    condition: when a decision relates to technology choice
  - target: context/auth-security.md
    condition: when a decision relates to auth or password handling
# Decisions usually ground sparsely; add only symbols that implement the decision.
# Entry shape: { node: "function:<tier-1-id>", fingerprint: "mh:64:<hex>" }
grounds_to: []
last_updated: 2026-09-10
---

# Decisions

<!-- If a decision names its concrete implementation point, link it as below;
     do not anchor vague concepts:
```markdown
[`someFunction()`](mex://function:<tier-1-id>)
```
-->

<!-- HOW TO USE THIS FILE:
     Each decision follows the format below.
     When a decision changes: DO NOT delete the old entry.
     Mark it as superseded, add the new entry above it.
     The history must be preserved — this is the event clock. -->

## Decision Log

### Self-host everything — no third-party publishing service
**Date:** 2026-09-10
**Status:** Active
**Decision:** The whole pipeline (plugin → server → readers) runs on infrastructure the user owns; no external publishing service.
**Reasoning:** "Publish once, share a link" with full ownership — zero notes leave the user's own infrastructure; privacy and control are the point of the project.
**Alternatives considered:** Obsidian Publish (rejected — third-party, subscription, content leaves the vault), sending files/screenshots manually (rejected — per-recipient toil).
**Consequences:** User operates the server themselves (systemd/container); simplicity and low ops burden are first-class requirements.

### Server in Go, single self-contained binary
**Date:** 2026-09-10
**Status:** Active
**Decision:** The publishing/serving backend is a Go program shipped as one static binary.
**Reasoning:** Cross-compiles to Linux ARM/x86 in one command, tiny memory footprint, trivial to run as a systemd service or container on a home server or droplet.
**Alternatives considered:** Node/Rust/Python backends (rejected — larger runtime or heavier deployment story for a personal home-server project).
**Consequences:** No CGo anywhere (keeps cross-compilation trivial); minimal external Go dependencies preferred.

### SQLite via modernc.org/sqlite for persistence
**Date:** 2026-09-10
**Status:** Active
**Decision:** All server state (route table, page content, password hashes, theme) persists to a single SQLite file using the pure-Go driver.
**Reasoning:** Survives restarts with zero external services; one file to back up; pure-Go driver avoids CGo so the binary stays statically cross-compilable.
**Alternatives considered:** Flat files on disk (rejected — route table + concurrent reads cleaner in SQL), PostgreSQL (rejected — external service contradicts single-binary minimal-ops goal), mattn/go-sqlite3 (rejected — CGo).
**Consequences:** Personal-scale traffic only; no DB migrations tooling beyond simple schema versioning.

### Echo for the HTTP layer
**Date:** 2026-09-10
**Status:** Active
**Decision:** Server uses the Echo framework for routing and middleware.
**Reasoning:** Token-auth and rate-limiting middleware with less boilerplate than stdlib, still lightweight.
**Alternatives considered:** stdlib `net/http` (rejected — more boilerplate for middleware chains), chi (rejected — Echo preferred for middleware + binder ergonomics), gin (rejected — no need for its size).
**Consequences:** Handlers follow Echo's signature and binder/validation idioms.

### goldmark for Markdown rendering
**Date:** 2026-09-10
**Status:** Active
**Decision:** Render Markdown to HTML with goldmark (CommonMark + GFM extensions).
**Reasoning:** Complete Markdown coverage is required; goldmark is the de-facto standard Go library, CommonMark-compliant, and extensible (tables, strikethrough, task lists, autolinks).
**Alternatives considered:** blackfriday (rejected — CommonMark compliance gaps), other languages' renderers (rejected — must run in the Go server).
**Consequences:** Server renders server-side; no client-side Markdown JS on published pages.

### argon2id for password hashing
**Date:** 2026-09-10
**Status:** Active
**Decision:** Page passwords are hashed with argon2id (golang.org/x/crypto/argon2), never stored in plaintext.
**Reasoning:** Memory-hard modern KDF; strongest practical protection for at-rest password hashes on a personally operated server.
**Alternatives considered:** bcrypt (rejected — argon2id preferred as the modern choice; GPU-resistance).
**Consequences:** Hash parameters (memory/time/parallelism) must be chosen and stored alongside the hash; verify with constant-time comparison.

### Deployment is flexible: Cloudflare Tunnel OR direct droplet
**Date:** 2026-09-10
**Status:** Active
**Decision:** The server must run equally well behind a Cloudflare Tunnel on a home server or directly reachable on a droplet (e.g. DigitalOcean).
**Reasoning:** The owner may use either; the app must not assume tunnel-specific behavior. HTTPS is required either way.
**Alternatives considered:** Tunnel-only assumption (superseded — inbound ports on a user-controlled droplet are acceptable).
**Consequences:** No code-level dependency on Cloudflare specifics; TLS termination strategy is a deployment concern, not an app concern.

### Theming: global site theme + per-page CSS override
**Date:** 2026-09-10
**Status:** Active (resolves PRD §14.1)
**Decision:** One global site theme (`GET/POST /api/theme`) plus an optional per-page `theme_css` override on publish/update; served precedence is per-page > global > built-in default, and the password-entry form always keeps the fixed default stylesheet.
**Reasoning:** The PRD's stated lean; one theme keeps the reading experience coherent while the override covers occasional special pages; keeping the password form unthemed prevents an owner's CSS from breaking the only gate standing between readers and protected content.
**Alternatives considered:** Per-page themes only (rejected — tedious for a personal site), separate theme objects referenced by id (rejected — YAGNI; a CSS string per page is enough at this scale).
**Consequences:** Global theme changes re-render all non-override pages; plugin UI currently exposes only the site theme (per-page override is API-only). Custom CSS replaces the default entirely — owner's responsibility.

### Session mechanism: route-scoped HMAC-signed cookie
**Date:** 2026-09-10
**Status:** Active
**Decision:** On successful password entry the server sets an `op_session` cookie — HMAC-SHA256-signed payload binding route + 1-hour expiry, HttpOnly, Secure, SameSite=Lax, cookie Path scoped to `/{route}` (implemented in `server/internal/session`).
**Reasoning:** Cookie gives natural re-reads without URL sharing; route binding in both the signed payload and the cookie Path keeps one unlock from opening other pages; signing (not server-side session state) keeps the server stateless.
**Alternatives considered:** Signed URL token (rejected — leaked when readers share the URL back; access should not be sticky to the link itself).
**Consequences:** Rotating `data/session-secret` revokes all outstanding sessions. Supersedes the open question in PRD §14.4.

### Route namespace: flat routes + reserved-route rejection
**Date:** 2026-09-10
**Status:** Active (resolves PRD §14.3)
**Decision:** Published routes stay flat at the root (`/{route}`); collisions with server features are prevented by rejecting a reserved list (`api`, `assets`, `favicon.ico`, `robots.txt`) at validation.
**Reasoning:** Flat URLs are what a personal publishing tool wants to share; the reserved list is a small, explicit surface that grows only when the server grows.
**Alternatives considered:** Fixed prefix like `/notes/{slug}` (rejected — uglier shared URLs, solves a problem the reserved list already covers).
**Consequences:** Any future server-owned top-level path MUST be added to the reserved list before it ships.

### Asset/image publishing: upload at publish time, rewrite outbound only
**Date:** 2026-09-10
**Status:** Active (resolves PRD §14.2; the v2 "fast-follow" was pulled forward and implemented)
**Decision:** On publish/re-publish the plugin uploads vault images (png/jpg/jpeg/gif/webp, ≤10 MB) via `POST /api/assets` and rewrites embeds to server asset URLs in the outbound payload only — the note on disk is never modified. SVG is rejected server-side (script-bearing XSS vector); non-image attachments are skipped with a summary notice; upload failures fail soft.
**Reasoning:** Publishing must not mutate the vault; server-side validation (extension + magic bytes must agree) is defense in depth; immutable content-hash-deduped assets (`/assets/{hash16-name}`, immutable cache) keep serving trivial.
**Alternatives considered:** Strip images (rejected — degrades notes), warn-only (rejected — that's what non-images do; images are the common case), rewriting the note file in place (rejected — mutates the vault).
**Consequences:** Notes republished after editing an image re-upload and get a new URL (hash changes); the note in the vault keeps its original embeds, so re-publish always re-resolves.

### Custom CSS: arbitrary, capped at 256 KB
**Date:** 2026-09-10
**Status:** Active (resolves PRD §14.6)
**Decision:** Themes accept arbitrary custom CSS (site-wide and per-page), capped at 256 KB per theme. The plugin ships 4 presets but custom CSS is first-class.
**Reasoning:** Self-hosted, single-owner — the only person the CSS can hurt is the owner. A preset-only list would fight the product's ownership promise.
**Alternatives considered:** Preset list only (rejected — arbitrary limits on the owner's own site), sanitizing CSS (rejected — no robust CSS sanitizer; the trust boundary is the owner).
**Consequences:** Raw HTML in Markdown stays disabled (goldmark unsafe off) — CSS is styling, HTML is code; only the former is allowed.
