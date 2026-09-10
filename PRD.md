# PRD: Obsidian Publish

## 1. Summary

Obsidian Publish is a self-hosted alternative to sending notes to individuals or using third-party sharing tools. A user writes a note in Obsidian, runs a "Publish" command from within the note, chooses a route (URL path) and optionally a password, and the note becomes a live web page served from a self-hosted Go server — sitting behind a Cloudflare Tunnel on the user's home server. The system has two components: an **Obsidian plugin** (authoring/control surface) and a **Go server** (publishing/serving backend), communicating over an authenticated API.

## 2. Problem Statement

Sharing notes today means either:
- Sending files/screenshots to individuals one at a time, or
- Using a third-party publishing service that doesn't fit a self-hosted, privacy-conscious workflow.

The user wants a **"publish once, share a link"** workflow that they fully own and control end-to-end — no external service, no per-recipient sending, optional password gating for sensitive notes.

## 3. Goals

- Publish any note from Obsidian to a public (or password-protected) URL in a few clicks.
- Choose the route/slug for each published page at publish time.
- Optionally password-protect a page.
- View all currently published pages from Obsidian settings, and unpublish any of them.
- Support a user-defined site theme, set in the plugin and pushed to the server.
- Keep the Go server simple — it renders whatever Markdown it's given and manages routes/auth; it doesn't need to understand Obsidian's vault structure.
- Secure, token-based communication between plugin and server.
- Run comfortably behind a Cloudflare Tunnel with no inbound ports opened on the home network.

## 4. Non-Goals (v1)

- Real-time collaborative editing or comments on published pages.
- Automatic re-publishing on every save (publish is an explicit, user-initiated action; see Open Questions for a possible "sync on save" toggle in v2).
- Multi-user / multi-vault support — this is a single-owner personal publishing tool.
- Analytics/visitor tracking (could be a v2 addition).
- Asset/attachment publishing (images, embeds) — flagged as a fast-follow, not blocking MVP, see §11.
- Custom domains beyond what Cloudflare Tunnel + DNS already provides (out of scope for the app itself).

## 5. Users

Single persona: **the note owner** — technically comfortable, self-hosts infrastructure, wants a lightweight personal publishing pipeline, values not depending on third parties, comfortable running a Go binary and a Cloudflare Tunnel.

## 6. System Architecture

```
┌────────────────────────┐          HTTPS (token auth)          ┌──────────────────────────┐
│   Obsidian Plugin       │ ───────────────────────────────────▶ │   Go Server (home server) │
│  - Publish command      │ ◀─────────────────────────────────── │  - Route table             │
│  - Route/password modal │            responses                 │  - Renders MD → HTML       │
│  - Settings: page list, │                                       │  - Password gate           │
│    unpublish, theme     │                                       │  - Theme storage           │
└────────────────────────┘                                       └──────────────────────────┘
                                                                           │
                                                                  Cloudflare Tunnel
                                                                           │
                                                                        Internet
                                                                           │
                                                                    Reader's browser
```

- **Obsidian plugin**: runs locally in the user's vault. Never stores the note content anywhere except sending it to the Go server on publish.
- **Go server**: single self-contained binary, runs on the user's home server, holds the route table, rendered pages, password hashes, and theme config. Exposed to the internet only via Cloudflare Tunnel (no open inbound ports).
- **Auth**: plugin and server share a long-lived API token (generated on first server setup, pasted once into plugin settings). All plugin→server calls are authenticated with this token over HTTPS (provided by the tunnel). Reader traffic to published pages is unauthenticated unless the page has a password.

## 7. Core User Flows

### 7.1 Publish a note
1. User opens a note, runs **"Publish"** (command palette or note context menu).
2. Plugin opens a modal:
   - **Route** — text field, pre-filled with a slugified version of the note title (editable). Plugin validates the route is URL-safe and checks with the server whether it's already taken.
   - **Password protect?** — toggle. If on, a password field appears (or "generate a password" option).
3. On confirm, plugin sends the note's Markdown content + route + password (if any) + theme reference to the Go server via the publish API.
4. Server stores/updates the route, hashes the password if provided, renders the page, and returns the live URL.
5. Plugin shows a success toast with the URL and a "Copy link" action.

### 7.2 Re-publish (update) a page
- If a note is already published (plugin tracks this via frontmatter or an internal map of note↔route), running "Publish" again updates the existing route's content instead of creating a new one. Modal shows the current route/password state, editable.

### 7.3 Unpublish
1. User opens plugin **Settings → Published Pages**.
2. Sees a table: route, source note, published date, password-protected (yes/no), last updated.
3. Clicks "Unpublish" next to any entry → confirmation prompt → plugin calls the server's delete API → route stops resolving (server can either 404 or return a "no longer available" page).

### 7.4 Viewing a password-protected page
1. Reader hits the route on the Go server.
2. Server detects the page requires a password, returns a page with a password entry form/modal (not the content).
3. Reader submits password → server validates against stored hash → on success, sets a short-lived session (cookie/signed token) scoped to that route and serves the content; on failure, shows an error and allows retry.

### 7.5 Setting a theme
1. In plugin settings, user picks/configures a theme (e.g., choose from a small built-in set, or supply custom CSS).
2. Plugin pushes the theme config to the server (site-wide, or per-page — see §9.5 for decision needed).
3. Server applies it to rendered pages going forward.

## 8. Functional Requirements — Obsidian Plugin

| # | Requirement |
|---|---|
| P1 | "Publish" command available from the command palette and note context menu. |
| P2 | Publish modal collects route (editable, pre-filled slug) and password toggle + value. |
| P3 | Plugin validates route format client-side and asks the server if the route is available before submitting. |
| P4 | Plugin tracks publish state per note (published/unpublished, route, password-protected) — stored in frontmatter and/or a local plugin data file, kept in sync with server truth. |
| P5 | Settings page lists all published pages (route, note, dates, password state) sourced live from the server. |
| P6 | Settings page supports unpublishing a page, with confirmation. |
| P7 | Settings page includes server connection config: server URL, API token. |
| P8 | Settings page includes theme selection/configuration, pushed to server on change. |
| P9 | Clear success/error feedback for publish, unpublish, and theme-push actions (e.g. token invalid, route taken, server unreachable). |
| P10 | Copy-to-clipboard for a published page's URL. |

## 9. Functional Requirements — Go Server

| # | Requirement |
|---|---|
| S1 | Exposes an authenticated API (token in header) for: publish/update page, delete (unpublish) page, list pages, check route availability, set theme. |
| S2 | Accepts a Markdown payload + metadata (route, password optional, theme reference) and renders it to HTML for serving. |
| S3 | Maintains a route table mapping route → page record (content, password hash, theme, timestamps). |
| S4 | For password-protected routes, serves a password-entry page instead of content until the correct password is supplied; issues a short-lived signed session token/cookie scoped to that route on success. |
| S5 | Passwords are never stored in plaintext — hashed with a strong algorithm (e.g. bcrypt/argon2). |
| S6 | Unpublish removes/disables the route immediately; server returns 404 (or a configurable "unpublished" page) for that route thereafter. |
| S7 | Persists route table, page content, and theme config to disk (survive restarts) — simple embedded storage (e.g. SQLite or flat files) rather than an external DB dependency. |
| S8 | Single self-contained binary, minimal external dependencies, simple to run as a systemd service or in a container behind the tunnel. |
| S9 | Rejects any plugin API call without a valid token (401); rate-limits password attempts on protected pages to deter brute force. |
| S10 | Applies the current theme (CSS) when rendering pages. |
| S11 | Logs publish/unpublish/access events at a basic level for the owner's own visibility (not full analytics — just operational logging). |

## 10. API Sketch (plugin ↔ server)

All requests authenticated via `Authorization: Bearer <token>` over HTTPS (through the Cloudflare Tunnel).

| Method & Path | Purpose |
|---|---|
| `GET /api/routes/{route}/available` | Check if a route is free. |
| `POST /api/pages` | Publish a new page: `{route, markdown, password?, theme?}`. |
| `PUT /api/pages/{route}` | Update an existing published page. |
| `DELETE /api/pages/{route}` | Unpublish a page. |
| `GET /api/pages` | List all published pages with metadata. |
| `POST /api/theme` | Set/update the site (or per-page) theme. |
| `GET /{route}` | Public: serve the rendered page (or password prompt). |
| `POST /{route}/auth` | Public: submit password for a protected route. |

## 11. Data Model (server-side, per page)

- `route` (string, unique, URL-safe)
- `title`
- `content_html` / `content_md`
- `password_hash` (nullable)
- `theme` (reference or inline override)
- `created_at`, `updated_at`
- `source_note_id` (optional, for plugin's own bookkeeping — server doesn't need to care about Obsidian internals)

## 12. Security Considerations

- Plugin↔server traffic authenticated with a bearer token, generated once on server setup, entered once into plugin settings; support token rotation.
- All traffic (plugin↔server and reader↔server) rides over HTTPS via the Cloudflare Tunnel — no plaintext exposure, no open inbound ports on the home network.
- Password-protected pages: hashed storage, rate-limited attempts, short-lived scoped session after successful auth (not a permanent unlock).
- Server should not trust the plugin blindly for anything beyond the authenticated API — reader-facing endpoints are fully separate and unauthenticated except where a page requires a password.
- Consider a token scoped narrowly to this API (not reused elsewhere) and documented rotation/revocation steps.

## 13. Non-Functional Requirements

- **Simplicity**: Go server should be a single binary with minimal ops burden — this is a home-server project, not a production SaaS.
- **Resilience**: server restarts should not lose published pages (persisted storage per S7).
- **Performance**: trivial for personal-scale traffic; no specific scaling target needed for v1.
- **Portability**: server should run on common home-server setups (Linux, ARM or x86) with straightforward deployment (single binary + config file).

## 14. Open Questions

1. **Theming scope**: is theme global (one theme for the whole site) or configurable per-page? (Spec currently leans global with optional per-page override — needs a decision.)
2. **Assets**: images/embeds in notes — out of scope for MVP, but how should the plugin handle a note with an embedded image? (Strip it, warn the user, or queue as fast-follow?)
3. **Route collisions**: should routes be scoped under a fixed prefix (e.g. `/notes/{slug}`) to avoid collisions with future server features, or fully flat?
4. **Session mechanism for password pages**: cookie vs. signed URL token — needs a decision based on how "sticky" access should be per reader.
5. **Sync-on-save**: should there be an optional mode where publishing auto-updates on every save, vs. always being an explicit action? (Proposed as v2, but worth confirming it's not wanted for MVP.)
6. **Custom CSS risk**: if themes allow arbitrary custom CSS, is that acceptable given it's self-hosted and single-user (low risk), or should it be limited to a preset list?

## 15. Milestones

**MVP**
- Plugin: Publish command + modal (route, password), settings page with list + unpublish, server connection config.
- Server: publish/update/delete/list APIs, route serving, password gate, token auth, persisted storage.
- Manual server deployment behind Cloudflare Tunnel (user already handles this).

**V2 candidates**
- Theming (built-in theme picker + custom CSS).
- Asset/image publishing.
- Sync-on-save / auto-republish option.
- Basic access logging visible in plugin settings.
- Token rotation UI in plugin.

## 16. Success Criteria

- User can go from "note open in Obsidian" to "live shareable link" in under 30 seconds, without leaving Obsidian.
- Zero notes ever leave the user's own infrastructure (plugin → home server only).
- Published pages list in settings always matches server truth (no drift).
- Password-protected pages reliably block unauthenticated access and never expose password hashes or content pre-auth.
