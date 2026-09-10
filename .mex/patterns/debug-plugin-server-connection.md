---
name: debug-plugin-server-connection
description: Diagnosing plugin↔server failures — 401s, unreachable server, route conflicts, publish/verify mismatches.
triggers:
  - "401"
  - "unauthorized"
  - "server unreachable"
  - "can't publish"
  - "connection failed"
  - "route taken"
edges:
  - target: context/auth-security.md
    condition: for the token/session rules that govern most failures here
  - target: context/setup.md
    condition: when the cause may be deployment/env (wrong URL, token not set)
grounds_to: []
last_updated: 2026-09-10
---

# Debug plugin↔server connection

## Context
All plugin→server calls ride HTTPS with a bearer token. Failures cluster at four boundaries: transport (URL/TLS), auth (token), validation (route), and state drift (plugin map vs server truth).

## Steps
1. **Identify the failure class from the symptom:** network error → transport; 401 → auth; 4xx validation → payload/route; wrong list in settings → state drift.
2. **Transport:** verify server URL in plugin settings (scheme + host + port), confirm the server is running and reachable — remember deployment may be Cloudflare Tunnel OR a direct droplet; try the base URL from a browser/curl first.
3. **Auth (401):** confirm token matches what the server generated on setup; check the header format (`Authorization: Bearer <token>`); after rotation, the plugin must be re-pasted.
4. **Route conflict:** the availability check (`GET /api/routes/{route}/available`) runs pre-submit — if the server says taken, the modal should surface it, not overwrite silently.
5. **State drift:** settings list must be re-fetched from `GET /api/pages`; never reconcile by trusting the plugin's local map.

## Gotchas
- [VERIFY AFTER FIRST IMPLEMENTATION] Where the server logs land for correlating a rejected request.
- A 200 HTML page in response to an API call means the request hit a public route, not `/api/*` — path bug, not auth bug.
- Tunnel vs droplet: don't debug tunnel config when the deployment is direct (or vice versa) — establish which mode is in use first.

## Verify
- [ ] Failure class identified before changing anything
- [ ] Token never pasted into logs, screenshots, or the transcript
- [ ] After the fix, publish → list → unpublish round-trips cleanly

## Debug
- Everything 401s including list: token middleware is failing closed — re-check token value and header.
- Publish succeeds but settings list is stale: the list isn't being fetched live (pattern violation, see `patterns/add-plugin-command.md`).

## Update Scaffold
- [ ] Update `.mex/ROUTER.md` "Current Project State" if what's working/not built has changed
- [ ] Update any `.mex/context/` files that are now out of date
- [ ] If this is a new task type without a pattern, create one in `.mex/patterns/` and add to `INDEX.md`
