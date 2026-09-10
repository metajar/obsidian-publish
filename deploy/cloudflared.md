# Behind a Cloudflare Tunnel

The server is designed to run on a home server with **no inbound ports
opened**: outbound `cloudflared` connects to Cloudflare's edge, and Cloudflare
routes your hostname to it. The Obsidian plugin and your readers both reach
the server through the tunnel over HTTPS.

Key point: **TLS is terminated at Cloudflare's edge.** The Obsidian Publish
server itself serves plain HTTP on localhost — that is intentional and safe,
because the only exposure to the internet is the tunnel.

## 1. Run the server

Install the systemd unit from [`obsidian-publish.service`](obsidian-publish.service)
(after building the binary and creating the `obsidian-publish` user as
documented in the unit header). The unit's default `OBSPUB_ADDR=127.0.0.1:8080`
is exactly right for this setup — nothing outside the machine can reach the
port directly.

Capture the one-time API token from the first start:

```console
$ journalctl -u obsidian-publish --no-pager | grep -A 5 "API token generated"
```

Paste that token into the Obsidian plugin settings together with your public
URL (the tunnel hostname).

## 2. Create the tunnel

On the same machine, install `cloudflared` and log in:

```console
$ sudo cloudflared tunnel login
$ cloudflared tunnel create obsidian-publish
```

Note the tunnel UUID and the credentials file path it prints, then add a DNS
record pointing your hostname at the tunnel:

```console
$ sudo cloudflared tunnel route dns obsidian-publish notes.example.com
```

## 3. Route the hostname to the service

Create `/etc/cloudflared/config.yml`:

```yaml
# /etc/cloudflared/config.yml — PLACEHOLDER values: replace the tunnel UUID
# and hostname with your own.
tunnel: 00000000-0000-0000-0000-000000000000
credentials-file: /etc/cloudflared/00000000-0000-0000-0000-000000000000.json

ingress:
  # Public hostname -> local plain-HTTP service port.
  - hostname: notes.example.com
    service: http://127.0.0.1:8080
  # Required catch-all: anything else gets a 404.
  - service: http_status:404
```

Cloudflare terminates HTTPS at the edge and forwards to `http://127.0.0.1:8080`
over the loopback interface — do not configure `originRequest` TLS options or
point the service at `https://`, the app has no TLS of its own.

Finally, tell the publish server its public URL so generated page links are
correct (the tunnel hostname), then restart:

```console
$ sudo systemctl edit obsidian-publish
#   [Service]
#   Environment=OBSPUB_BASE_URL=https://notes.example.com
$ sudo systemctl restart obsidian-publish
```

## 4. Run cloudflared as a service

```console
$ sudo cloudflared service install
$ sudo systemctl enable --now cloudflared
```

Visit `https://notes.example.com` — an unpublished route should 404, which
means the path is working. Publish a note from Obsidian to see a live page.

## Notes

- **Firewall:** keep it closed. No inbound ports need to be opened for the
  tunnel; cloudflared makes outbound connections only.
- **Token rotation:** delete the token file
  (`/var/lib/obsidian-publish/api-token`) and restart the service — a new
  token is generated and printed once. Update the plugin settings to match.
- The same setup works for a droplet instead of a home server; if you prefer
  a direct droplet without Cloudflare, see [`droplet.md`](droplet.md).
