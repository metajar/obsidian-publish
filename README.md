# Obsidian Publish

Self-hosted publishing for your Obsidian notes: hit "Publish" inside a note
and it becomes a live web page on your own server — optionally
password-protected — with a shareable link.

## How it fits together

Two pieces, no third parties:

- **`plugin/`** — an Obsidian plugin (the control surface). It sends a note's
  Markdown plus a route (URL slug) and optional password to your server, lists
  your published pages, and unpublishes them.
- **`server/`** — a single static Go binary (the backend). It authenticates
  the plugin with a bearer token, renders Markdown to HTML, serves pages
  publicly, password-gates protected routes with argon2id + short-lived
  signed session cookies, and persists everything to SQLite.

```
Obsidian plugin --HTTPS + bearer token--> Go server --public HTTP--> readers
```

Deploy it behind a Cloudflare Tunnel on a home server (no open ports) or
directly on a droplet with a TLS reverse proxy — the server is agnostic.

## Build

Server (Go 1.26+):

```console
$ cd server
$ go build ./... && go test ./...
$ CGO_ENABLED=0 go build -o obsidian-publish-server ./cmd/server
```

Pure Go (CGO disabled, SQLite via modernc) — cross-compiles anywhere Go does:

```console
$ CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o obsidian-publish-server ./cmd/server
$ CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o obsidian-publish-server ./cmd/server
```

Plugin (Node.js + npm):

```console
$ cd plugin
$ npm install
$ npm run build   # typecheck + bundle to plugin/main.js
```

A container image build (`docker build -t obsidian-publish .` from the repo
root) is available too — see the root `Dockerfile`.

## Run locally

```console
$ cd server
$ go run ./cmd/server
```

On first run it creates a data dir (`server/data/`) with the SQLite DB,
session secret, and API token, and **prints the token once** — copy it now.
The server listens on `:8080`. Configuration is via flags (`-addr`, `-db`,
`-token-file`, `-secret-file`, `-base-url`) or `OBSPUB_*` env vars; defaults
live in `server/internal/config`.

## Install the plugin in Obsidian

1. In your vault, create `.obsidian/plugins/selfhosted-publish/`.
2. Copy `plugin/main.js`, `plugin/manifest.json`, and `plugin/styles.css`
   into it (or symlink the `plugin/` directory there).
3. Enable it under Settings → Community plugins.
4. In the plugin's settings, enter the server URL and the API token printed
   on first server start, then hit **Test connection**.

## Publish your first note

1. Open a note → command palette → **Publish** (also in the note context
   menu).
2. Confirm the route slug (pre-filled from the note title) and optionally
   enable password protection.
3. Publish — the toast shows the live URL with a copy button. Re-running
   Publish updates the same page; Settings → Published Pages lists and
   unpublishes everything.

## Deploying

- **Behind a Cloudflare Tunnel** (home server, no open inbound ports):
  [`deploy/cloudflared.md`](deploy/cloudflared.md)
- **Direct droplet** (systemd + ufw + TLS reverse proxy):
  [`deploy/droplet.md`](deploy/droplet.md) — includes the systemd unit
  [`deploy/obsidian-publish.service`](deploy/obsidian-publish.service)
  walkthrough, or use the root `Dockerfile` (distroless, non-root, `/data`
  volume).

## Security notes

- **API token**: plugin↔server calls require `Authorization: Bearer <token>`;
  anything else gets a 401. The token (32 random bytes) is generated on first
  server start, printed once, stored mode-0600 in the data dir — never commit
  it or paste it anywhere else. Rotate by deleting the token file and
  restarting; update plugin settings to match.
- **Page passwords** are never stored or logged in plaintext — argon2id
  hashes only. Password attempts are rate-limited (5/min per IP+route).
- **Reader sessions**: a correct password yields a short-lived (1 hour)
  HMAC-SHA256-signed cookie scoped to that one route — not a permanent
  unlock; rotating the session-secret file invalidates all sessions.
- Serve over HTTPS (via the tunnel or a reverse proxy); the app itself speaks
  plain HTTP on localhost by design.

## Backups

Back up the **data directory** — `server/data/` locally, `/var/lib/obsidian-publish/`
under systemd, or the `/data` volume in Docker. It is the whole state:
published pages, routes, password hashes, token, and session secret. Restore
it next to the binary and everything comes back.
