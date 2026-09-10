---
name: add-api-endpoint
description: Adding or changing a plugin↔server API endpoint (Echo route + handler + storage + plugin client method together).
triggers:
  - "new endpoint"
  - "add api"
  - "new route"
  - "plugin api"
  - "handler"
edges:
  - target: context/architecture.md
    condition: to see the full publish/read flow the endpoint joins
  - target: context/auth-security.md
    condition: to confirm the endpoint sits on the right trust plane (token-authed /api/* vs public)
  - target: context/conventions.md
    condition: for the verify checklist before presenting the code
grounds_to: []
last_updated: 2026-09-10
---

# Add an API endpoint

## Context
The API contract lives in `PRD.md` §10. Endpoints come in pairs by trust plane: `/api/*` (bearer-token, owner plane) and public reader routes. An endpoint change almost always touches three places: Echo route + handler (server), SQLite persistence, and the plugin's API client + UI wiring.

## Steps
1. Decide the trust plane: `/api/*` behind token middleware, or public. If public + password-relevant, re-read `context/auth-security.md` first.
2. Server: add the Echo route in the appropriate group (authed vs public), then the handler. Validate input (route slugs must be URL-safe) before touching storage.
3. Server: extend the SQLite schema/persistence only if the page record needs new fields.
4. Plugin: add the client method (same auth header as all other calls), then wire it into the command/modal/settings surface.
5. Update both sides' error handling so the plugin can surface server errors (route taken, invalid token, unreachable).

## Gotchas
- [VERIFY AFTER FIRST IMPLEMENTATION] Exact handler/persistence file layout.
- Never add a plugin-API behavior to a public route or vice versa (hard invariant).
- Route slug validation must run on the server too — the plugin's client-side check is UX, not security.
- Reader-facing route handling must not leak protected content in error paths.

## Verify
- [ ] Endpoint is on the correct trust plane with middleware applied
- [ ] Route slug validated server-side
- [ ] Persistence survives restart (no in-memory-only state)
- [ ] Plugin surfaces distinct errors for 401 / unreachable / validation failure
- [ ] No password or token logged

## Debug
- 401 on every call: token middleware ordering or wrong header format.
- Plugin gets HTML instead of JSON: request likely hit a public route, not `/api/*` — check path.

## Update Scaffold
- [ ] Update `.mex/ROUTER.md` "Current Project State" if what's working/not built has changed
- [ ] Update any `.mex/context/` files that are now out of date
- [ ] If this is a new task type without a pattern, create one in `.mex/patterns/` and add to `INDEX.md`
