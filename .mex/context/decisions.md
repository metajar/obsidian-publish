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

### [TO BE DETERMINED] Theming scope
Pending: global site theme vs. per-page theme override (PRD §14.1 — spec leans global with optional per-page override). Decide before implementing theme storage/API.

### [TO BE DETERMINED] Session mechanism for password-protected pages
Pending: cookie vs. signed URL token for the short-lived route-scoped session (PRD §14.4). Decide when implementing the password gate; affects `context/auth-security.md`.

### [TO BE DETERMINED] Route namespace
Pending: flat routes vs. a fixed prefix like `/notes/{slug}` to avoid collisions with future server features (PRD §14.3). Decide before freezing the route table schema.

### [TO BE DETERMINED] Asset/image handling
Pending: how the plugin treats embedded images in MVP (strip, warn, or queue as fast-follow) (PRD §14.2). Out of scope for MVP publishing.

### [TO BE DETERMINED] Custom CSS limits
Pending: arbitrary custom CSS in themes vs. preset list only (PRD §14.6 — low risk since self-hosted single-user).
