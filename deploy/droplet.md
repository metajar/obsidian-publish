# Direct droplet deployment (DigitalOcean, Hetzner, etc.)

Run the Obsidian Publish server directly on a small cloud VM. The app serves
plain HTTP on loopback; a reverse proxy in front of it provides TLS and is the
only thing exposed to the internet.

If you'd rather run behind a Cloudflare Tunnel from a home server instead, see
[`cloudflared.md`](cloudflared.md).

## 1. Build and upload the binary

From a machine with Go 1.26+ (or build on the droplet), from the `server/`
directory of this repo:

```console
# x86 droplet (most common):
$ CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -o obsidian-publish-server ./cmd/server

# ARM droplet (e.g. Ampere/Hetzner CAX):
$ CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
    -o obsidian-publish-server ./cmd/server
```

The build is pure Go (CGO disabled, SQLite via modernc.org/sqlite), so the
result is a fully static binary. Upload it:

```console
$ scp obsidian-publish-server root@YOUR_DROPLET_IP:/usr/local/bin/
```

(Alternatively, build the Docker image from the repo root — see the root
`Dockerfile` — and run it with a `/data` volume.)

## 2. Create the service user and install systemd unit

```console
$ sudo useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin \
     --comment "Obsidian Publish server" obsidian-publish
$ sudo cp obsidian-publish.service /etc/systemd/system/
$ sudo systemctl daemon-reload
```

The unit ([`obsidian-publish.service`](obsidian-publish.service)) keeps all
state under `/var/lib/obsidian-publish` (created automatically), runs the
binary as the low-privilege `obsidian-publish` user, and hardens the service
(NoNewPrivileges, ProtectSystem=strict, PrivateTmp, ProtectHome, ...).

## 3. Firewall: allow SSH and web only

```console
$ sudo ufw default deny incoming
$ sudo ufw default allow outgoing
$ sudo ufw allow 22/tcp    # SSH — tighten to your IP if you can
$ sudo ufw allow 80/tcp    # HTTP (ACME challenges + redirect)
$ sudo ufw allow 443/tcp   # HTTPS
$ sudo ufw enable
```

The publish server itself listens on `127.0.0.1:8080` (the unit default), so
it is unreachable directly even if the firewall were wrong.

## 4. TLS via reverse proxy

Point DNS (an A record for e.g. `notes.example.com`) at the droplet, then:

**Caddy (recommended)** — automatic HTTPS via Let's Encrypt:

```console
$ sudo apt install -y caddy
$ sudo caddy reverse-proxy --from notes.example.com --to 127.0.0.1:8080
```

That one-liner obtains and renews the certificate and proxies
`notes.example.com` to the local service. To make it permanent, put the same
config in `/etc/caddy/Caddyfile`:

```
notes.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

**nginx** is a fine alternative: install `nginx` and `certbot --nginx`,
then `proxy_pass http://127.0.0.1:8080;` in the server block.

## 5. Start the service and capture the one-time token

```console
$ sudo systemctl enable --now obsidian-publish
```

On first start the server generates the API token and session secret and
prints the token **once**. Read it from the journal:

```console
$ journalctl -u obsidian-publish --no-pager | grep -A 5 "API token generated"
```

Paste the token into the Obsidian plugin settings along with the public URL
(`https://notes.example.com`). If you scrolled past it, rotate it: delete
`/var/lib/obsidian-publish/api-token` and restart — a new one is printed.

Set the public base URL so publish responses return correct links:

```console
$ sudo systemctl edit obsidian-publish
#   [Service]
#   Environment=OBSPUB_BASE_URL=https://notes.example.com
$ sudo systemctl restart obsidian-publish
```

## 6. Verify

- `https://notes.example.com` should 404 on an unused route (serving works).
- Publish a note from Obsidian → the returned link opens in a browser.
- `systemctl status obsidian-publish` should show active (running).

## Backups

Back up `/var/lib/obsidian-publish/` — the SQLite DB plus the token and
session-secret files are the whole server state. Restoring that directory (or
the `/data` volume if you used Docker) onto a fresh droplet plus the unit file
recovers everything.

## Token rotation / revocation

Rotation is: delete `/var/lib/obsidian-publish/api-token`, restart the
service, capture the newly printed token, update plugin settings, and treat
the old token as revoked (it no longer matches). The session secret rotates
the same way (deleting `/var/lib/obsidian-publish/session-secret` also
invalidates every outstanding reader page session).
