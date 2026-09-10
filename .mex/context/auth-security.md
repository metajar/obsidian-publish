---
name: auth-security
description: Token auth, password gating, sessions, and rate limiting. Load when touching the plugin API auth, protected pages, or anything involving passwords/tokens.
triggers:
  - "auth"
  - "token"
  - "password"
  - "session"
  - "rate limit"
  - "security"
  - "brute force"
edges:
  - target: context/architecture.md
    condition: when understanding where auth sits in the request flow
  - target: context/decisions.md
    condition: when understanding why argon2id / token auth were chosen, or the pending session-mechanism decision
  - target: patterns/debug-plugin-server-connection.md
    condition: when diagnosing 401s or auth failures between plugin and server
# Ground only the concrete auth symbols (hash/verify, middleware) once they exist.
# Entry shape: { node: "function:<tier-1-id>", fingerprint: "mh:64:<hex>" }
grounds_to: []
last_updated: 2026-09-10
---

# Auth & Security

Two fully separate trust planes — never mix them:

1. **Plugin ↔ server (owner plane)** — every `/api/*` call carries `Authorization: Bearer <token>` over HTTPS. Long-lived token generated once on first server setup, pasted once into plugin settings. No token → 401. Rotation/revocation must be supported.
2. **Reader ↔ server (public plane)** — `GET /{route}` and `POST /{route}/auth` are unauthenticated except where an individual page has a password. The server must never trust the plugin for reader-facing behavior.

## Password-protected pages

- Passwords arrive in plaintext only transiently: the publish request and the reader auth form. Stored form is an **argon2id hash** (`golang.org/x/crypto/argon2`) with parameters (memory/time/parallelism) stored alongside the hash; verification is constant-time.
- Pre-auth, a protected route serves only the password-entry page — never content, never the hash, no leakage via error paths, redirects, or caching headers.
- On successful password entry, the server issues a **short-lived session scoped to that route**: an HMAC-SHA256-signed cookie (`op_session`, HttpOnly, Secure, SameSite=Lax, Path=/{route}, 1-hour expiry). The signed payload binds route + expiry, so a cookie for one route never unlocks another. It is not a permanent unlock.
- Password attempts are **rate-limited** to deter brute force. Rate limiting must survive the MVP — it is requirement S9, not a nice-to-have.

## Token rules

- The API token is scoped narrowly to this API — never reused elsewhere.
- Token rotation and revocation steps must be documented once implemented.
- Reject on any malformed/missing Authorization header — fail closed.

## Hard invariants

- No page password is ever stored or logged in plaintext (non-negotiable; see AGENTS.md).
- No protected content or hash is reachable before password verification succeeds.
- Reader endpoints never gain plugin-API behavior and vice versa.
- API PUT password semantics (agreed with plugin): omitted = keep, JSON null (or "") = remove, non-empty string = set/replace.
## Server implementation notes

Implemented in `server/internal/` (Go module `obsidian-publish/server`):

- **Bearer auth** — `httpapi.bearerAuth` middleware on the `/api/*` Echo group only; `hmac.Equal` constant-time compare; 401 JSON on missing/malformed/wrong header. Token: 32 random bytes, base64url, generated at first run into `data/api-token` (0600) and printed once; rotate by deleting the file and restarting.
- **argon2id** — `pwhash.Hash/Verify` (time=1, memory=64MB, threads=4, key=32, random 16-byte salt; PHC format stores params with the hash; verify is constant-time). Plaintext exists only in the bound request struct.
- **Session** — `session.Manager` signs `"<route>|<unix-expiry>"` with the 32-byte secret from `data/session-secret`; cookie Path is scoped to the route so browsers only send it back there. Rotating the secret file invalidates all sessions.
- **Rate limiting** — `ratelimit.Limiter`, sliding window, 5 attempts/min per IP+route on `POST /{route}/auth` only; over-limit returns 429 with the generic password form (no content). In-memory (resets on restart) — acceptable for MVP per S9.
- **Slug validation** — `store.ValidRoute`: 1–64 chars `[a-z0-9-]`, alphanumeric endpoints, reserved routes (`api`, `favicon.ico`, `robots.txt`) rejected; enforced in the handler AND defensively again in `store.Create`.
