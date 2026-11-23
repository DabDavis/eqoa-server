# Hosting a public server on a VPS

This guide picks up where the README's [Installing a server](../README.md#installing-a-server)
ends: one Linux VPS (Debian or Ubuntu) runs the game server, the web relay for browser
players, and nginx (or Apache) in front of the HTTP parts with TLS. Every port and path here is
the default in [`config.yaml.example`](../config.yaml.example); if you change one there,
change it here too.

The names used throughout:

| Name | What |
|---|---|
| `example.com` | the game's host name: players' clients connect to it, and the browser client page is served from it |
| `account.example.com` | the account pages (register, sign in, characters, password, transfer) |
| `gm.example.com` | the GM panel |
| `203.0.113.10` | the VPS's public IPv4 address, which `example.com` resolves to |
| `/srv/eqoa-server` | the server's folder: the binary, `config.yaml`, `eqoa.db`, `patches/`, `worlds/` |
| `eqoa` | the system user the server runs as |

## Recommended VPS

Measured on a 2 vCPU / 4 GB VPS running **two shards at once** plus the web relay and nginx,
with a load average of about 1.5 to 1.7:

| | Players online | NPCs loaded | Server RSS | CPU (average, of one core) | Database |
|---|---|---|---|---|---|
| Shard A | ~36 | ~65,000 | ~670 MB | ~45% | 440 MB |
| Shard B | ~22 | similar | ~415 MB | ~11% | 160 MB |

A restart that keeps players in (`tools/swap_keep.sh`, below) pauses the game about 15 s on that
box; heavy database work at the same moment (a `VACUUM`) doubled the pause. Its disk was 75 GB,
most of the used part backups and old builds.

| Players (one shard) | vCPU | RAM | Disk | Traffic a month | Basis |
|---|---|---|---|---|---|
| up to 10 (friends) | 1 | 2 GB | 25 GB | 1 TB | below the measured range |
| up to 25 | 2 | 4 GB | 40 GB | 1 TB | measured |
| up to 50 | 2 | 4 GB | 60 GB | 2 TB | measured (~60 across two shards on this size) |
| up to 100 | 4 | 8 GB | 80 GB | 3 TB | **estimate** |
| 200+ | 8, fast cores | 16 GB | 160 GB | 5 TB+ | **estimate**; consider a second shard |

- **2 vCPU is the floor for a public server.** One core runs the tick; the other takes the
  database, the HTTP listeners, the relay and nginx, and keeps a restart's pause short.
- **The NPC world costs first, players second.** Every 100 ms tick visits the world's NPCs
  (tens of thousands with the open database), so an empty server already uses memory and a
  share of a core. Players add their view, their state channels and their packets, and each
  separately hunted area wakes the creatures around it; players spread over many areas cost
  more than the same number in one town.
- **The tick is one thread under one lock**, so a faster core helps more than more cores past
  four. Past about 100 players in many different places, a second shard (README, "Shards" in
  `config.yaml.example`) scales better than a bigger machine.
- **Bandwidth per player (an estimate from the code):** the server sends a player at most one
  packet of state-channel updates a tick, 1,328 bytes at most, 10 a second: about 13 KB/s
  (~100 kbit/s) at the ceiling, in a busy town; walking alone it is a small fraction of that.
  Plan on 5 to 15 KB/s a player while playing, a few GB a player a month. What players send is
  far less. A browser player's traffic crosses the box twice (its WebSocket to the relay, and
  the relay to the game on the same host, which is local).
- **Disk:** the binary and `worlds/` (about 100 MB of ground files, plus about 650 MB if you
  make `cmd/groundmap`'s maps), the database (it grows with players: 0.2 to 0.5 GB above), the
  log, and backups. Daily backups of a 0.5 GB database fill a disk in weeks: rotate them
  (below).
- `server.gc_percent: 25` and `server.no_huge_pages: true` (in the example, commented out)
  bring the RSS down on a small box.

## Ports and firewall

| Port | Proto | Listener | Open to the internet? |
|---|---|---|---|
| 10070 | UDP | the game (`server.game_port`; each further shard its own, e.g. 10071) | **yes** |
| 9735 | TCP | the login (`server.auth_port`) | **yes** |
| 7000 | TCP | the patch server, plain HTTP (`server.patch_port`): the game's patcher (PS2, native clients) | **yes** |
| 7003 | UDP | PS2 hard-disk world delivery (`patch.worlds_udp_port`) | only if PS2 players use the HDD setup |
| 80, 443 | TCP | nginx or Apache | **yes** |
| 8080 | TCP | the account pages (`server.account_port`) | **no**: through the proxy |
| 8081 | TCP | the GM panel (`gm.port`, bound to 127.0.0.1 by `gm.bind`) | no: through the proxy |
| 8790 | TCP | `wsrelay`'s WebSockets (its `-listen`, 127.0.0.1) | no: through the proxy |
| 8792 | TCP | `wsrelay -lookup` (`auth.relay_lookup`) | no: loopback only |
| 6061 | TCP | Go's profiler, if `server.pprof_addr` is set | no: loopback only |

The game, login and patch listeners bind `server.bind_address` (`0.0.0.0`). **The account
server listens on every address** whatever `bind_address` says, so the firewall is what keeps
port 8080 off the internet. With ufw:

```bash
sudo ufw default deny incoming
sudo ufw allow OpenSSH
sudo ufw allow 10070/udp     # the game (add 10071/udp, ... for more shards)
sudo ufw allow 9735/tcp      # login
sudo ufw allow 7000/tcp      # patch server
sudo ufw allow 7003/udp      # PS2 HDD world delivery (optional)
sudo ufw allow 'Nginx Full'  # 80 + 443 (Apache: 'Apache Full')
sudo ufw enable
```

In `config.yaml`, `server.server_ip` is `203.0.113.10`: the address clients are told to
connect to, which must be the public one (a dotted quad, for PS2 world downloads).

## systemd

### The game server

`/etc/systemd/system/eqoa-game.service`:

```ini
[Unit]
Description=EQOA game server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=eqoa
Group=eqoa
UMask=0077
WorkingDirectory=/srv/eqoa-server
ExecStart=/srv/eqoa-server/gameserver -config /srv/eqoa-server/config.yaml
Restart=on-failure
RestartSec=5
# A stop saves every character in the world, after world.restart_notice_wait.
TimeoutStopSec=60
LimitNOFILE=65536
StandardOutput=append:/srv/eqoa-server/gameserver.log
StandardError=append:/srv/eqoa-server/gameserver.log

[Install]
WantedBy=multi-user.target
```

The log file is the one `gm.log_path` (`gameserver.log`, relative to the working directory)
shows in the GM panel. Rotate it with logrotate's `copytruncate`, since the server keeps it
open.

### The web relay

Browser players reach the game through `cmd/wsrelay`: it holds their UDP and TCP sockets
behind WebSockets, and lets them reach only the ports it is told to. Build it beside the server
(`go build -o wsrelay ./cmd/wsrelay`), then `/etc/systemd/system/eqoa-relay.service`:

```ini
[Unit]
Description=EQOA WebSocket relay
After=network-online.target eqoa-game.service

[Service]
Type=simple
User=eqoa
ExecStart=/srv/eqoa-server/wsrelay \
    -listen 127.0.0.1:8790 \
    -allow-udp 203.0.113.10:10070 \
    -allow-tcp 203.0.113.10:9735,203.0.113.10:7000 \
    -origin https://example.com \
    -lookup 127.0.0.1:8792
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

- `-allow-udp` / `-allow-tcp` name the game, login and patch ports at the **public** address
  (the page asks the relay for the host name it was served from, which resolves to it). Add
  each further shard's game port (`203.0.113.10:10071`, or a range `10070-10071`).
- `-origin` is the page's origin; without it any site may use your relay.
- `-lookup` lets the game server see a browser player's own address (for `auth.one_login`
  and the login limits): set `auth.relay_lookup: "http://127.0.0.1:8792"` to match. Without
  it every browser player has the relay's address, which `server_ip` exempts.

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now eqoa-game eqoa-relay
journalctl -u eqoa-relay -f
```

### Restarting

`sudo systemctl restart eqoa-game` ends every session (players go back to the login). Two
scripts in `tools/` do better; each one's header has the details:

- `tools/swap_keep.sh`: swaps a staged binary in **without ending sessions** (SIGUSR2 to the
  running server; it saves, re-execs into the new binary and puts every session back; players
  see a pause of several seconds). It needs passwordless `sudo -n systemctl stop|start` of the
  unit for its rollback.
- `tools/swap_when_empty.sh`: from cron, swaps a staged binary in only when nobody is
  connected (README, "Deploying without kicking players").

## nginx

`sudo apt install nginx certbot python3-certbot-nginx`, then
`/etc/nginx/sites-available/example.com` (linked into `sites-enabled/`). Start with the
`listen 80` blocks only, run `sudo certbot --nginx -d example.com -d account.example.com -d
gm.example.com`, and certbot adds the certificate lines; the version below is the result.

```nginx
# Plain HTTP: certbot's challenges, then HTTPS.
server {
    listen 80;
    listen [::]:80;
    server_name example.com account.example.com gm.example.com;
    location /.well-known/acme-challenge/ { root /var/www/letsencrypt; }
    location / { return 301 https://$host$request_uri; }
}

# example.com: the browser client page, its relay, and the patch files it fetches.
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name example.com;
    ssl_certificate     /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;

    root /var/www/example.com;
    add_header Strict-Transport-Security "max-age=63072000" always;
    add_header X-Content-Type-Options "nosniff" always;

    # The browser client: the files its build produces (below), served as they are.
    location /eqoa/play/ {
        add_header Cache-Control "no-cache" always;
        add_header Strict-Transport-Security "max-age=63072000" always;
        add_header X-Content-Type-Options "nosniff" always;
        try_files $uri $uri/ =404;
    }
    location = /eqoa/play { return 301 /eqoa/play/$is_args$args; }

    # wsrelay: the page connects to wss://example.com/eqoa/relay/udp and /tcp.
    location ^~ /eqoa/relay/ {
        proxy_pass http://127.0.0.1:8790/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering off;
    }

    # The patch server, for the browser client's patcher, which fetches over HTTPS:
    # the manifest under /patch/ and each file as /<name>.gz at the root.
    location ^~ /patch/ {
        proxy_pass http://127.0.0.1:7000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        add_header Cache-Control "no-cache" always;
    }
    location ~ ^/[A-Za-z0-9._-]+\.gz$ {
        proxy_pass http://127.0.0.1:7000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        add_header Cache-Control "no-cache" always;
    }

    # The account server's JSON routes, for pages served from this host (the client
    # page's helpers ask /eqoa/account/api/...).
    location ^~ /eqoa/account/api/ {
        proxy_pass http://127.0.0.1:8080/api/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        client_max_body_size 64k;
    }
}

# account.example.com: the account pages, whole.
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name account.example.com;
    ssl_certificate     /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    add_header Strict-Transport-Security "max-age=63072000" always;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        client_max_body_size 64k;
    }
}

# gm.example.com: the GM panel, on a host of its own (its cookie is a __Host- one
# and no other page shares its origin). gm.public_url: "https://gm.example.com".
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name gm.example.com;
    ssl_certificate     /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    add_header Strict-Transport-Security "max-age=63072000" always;

    location / {
        proxy_pass http://127.0.0.1:8081;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 120s;
        client_max_body_size 4m;
    }
}
```

The account server trusts `X-Real-IP` only from loopback, so its login limits see the
player's address through the proxy. With the account pages on their own host, set in
`config.yaml`:

```yaml
gm:
  port: 8081
  public_url: "https://gm.example.com"
auth:
  bind_address: "127.0.0.1"
  password_url: "account.example.com/password"
  relay_lookup: "http://127.0.0.1:8792"
# and, only if you turn the account e-mail on:
mail:
  base_url: "https://account.example.com"
  page_path: "/password"
```

There is **no crash-report endpoint** to proxy: players' bug reports are filed in the game
(`/bug`, `#bug`, F3 on vanilla) and land in the GM panel's petitions.

### The browser client page

The page is built from the public client repository,
[eqoa-rust-client-vanilla](https://github.com/DabDavis/eqoa-rust-client-vanilla): its `web/`
folder has the build and deploy scripts. Serve what its build produces under
`/var/www/example.com/eqoa/play/` (the `location /eqoa/play/` above). Served from
`example.com`, the page by default connects through `wss://example.com/eqoa/relay` and plays
on `example.com` itself, so nothing needs configuring in it. Players bring their own disc
image; the page holds no game files.

## Apache

The same three hosts with `mod_ssl`, `mod_proxy`, `mod_proxy_http` and
`mod_proxy_wstunnel`:

```bash
sudo apt install apache2 certbot python3-certbot-apache
sudo a2enmod ssl proxy proxy_http proxy_wstunnel headers rewrite
```

`/etc/apache2/sites-available/eqoa.conf` (then `sudo a2ensite eqoa` and
`sudo certbot --apache -d example.com -d account.example.com -d gm.example.com`):

```apache
<VirtualHost *:80>
    ServerName example.com
    ServerAlias account.example.com gm.example.com
    RewriteEngine on
    RewriteCond %{REQUEST_URI} !^/\.well-known/acme-challenge/
    RewriteRule ^ https://%{HTTP_HOST}%{REQUEST_URI} [R=301,L]
</VirtualHost>

<VirtualHost *:443>
    ServerName example.com
    SSLEngine on
    SSLCertificateFile    /etc/letsencrypt/live/example.com/fullchain.pem
    SSLCertificateKeyFile /etc/letsencrypt/live/example.com/privkey.pem
    Header always set Strict-Transport-Security "max-age=63072000"
    DocumentRoot /var/www/example.com

    <Location /eqoa/play/>
        Header always set Cache-Control "no-cache"
    </Location>

    ProxyPreserveHost On
    ProxyRequests Off

    # wsrelay (WebSockets; long timeouts)
    ProxyPass        /eqoa/relay/ ws://127.0.0.1:8790/ timeout=3600
    ProxyPassReverse /eqoa/relay/ ws://127.0.0.1:8790/

    # the patch server, for the browser client's patcher
    ProxyPass        /patch/ http://127.0.0.1:7000/patch/
    ProxyPassReverse /patch/ http://127.0.0.1:7000/patch/
    ProxyPassMatch   ^/([A-Za-z0-9._-]+\.gz)$ http://127.0.0.1:7000/$1

    # the account server's JSON routes
    ProxyPass        /eqoa/account/api/ http://127.0.0.1:8080/api/
    ProxyPassReverse /eqoa/account/api/ http://127.0.0.1:8080/api/

    RequestHeader set X-Real-IP "expr=%{REMOTE_ADDR}"
    RequestHeader set X-Forwarded-Proto "https"
</VirtualHost>

<VirtualHost *:443>
    ServerName account.example.com
    SSLEngine on
    SSLCertificateFile    /etc/letsencrypt/live/example.com/fullchain.pem
    SSLCertificateKeyFile /etc/letsencrypt/live/example.com/privkey.pem
    Header always set Strict-Transport-Security "max-age=63072000"
    ProxyPreserveHost On
    RequestHeader set X-Real-IP "expr=%{REMOTE_ADDR}"
    RequestHeader set X-Forwarded-Proto "https"
    ProxyPass        / http://127.0.0.1:8080/
    ProxyPassReverse / http://127.0.0.1:8080/
</VirtualHost>

<VirtualHost *:443>
    ServerName gm.example.com
    SSLEngine on
    SSLCertificateFile    /etc/letsencrypt/live/example.com/fullchain.pem
    SSLCertificateKeyFile /etc/letsencrypt/live/example.com/privkey.pem
    Header always set Strict-Transport-Security "max-age=63072000"
    ProxyPreserveHost On
    RequestHeader set X-Real-IP "expr=%{REMOTE_ADDR}"
    RequestHeader set X-Forwarded-For "expr=%{REMOTE_ADDR}"
    RequestHeader set X-Forwarded-Proto "https"
    ProxyPass        / http://127.0.0.1:8081/ timeout=120
    ProxyPassReverse / http://127.0.0.1:8081/
</VirtualHost>
```

`RequestHeader ... expr=` needs Apache 2.4.10 or later. `ProxyPass ... ws://` (mod_proxy_wstunnel) carries the relay's WebSockets; on Apache 2.4.47
and later `ProxyPass /eqoa/relay/ http://127.0.0.1:8790/ upgrade=websocket` does the same.

## Backups

The database is SQLite in WAL mode: what is not yet checkpointed sits in `eqoa.db-wal`, so
**never `cp` the live `eqoa.db`**, which can copy a torn or stale file. Two safe ways:

- **The GM panel's backup** (an admin's tools, or its `create_backup` operation for an AI agent):
  a consistent copy (`VACUUM INTO`) into `gm.backup_dir` (`backups/`, beside the database).
- **`sqlite3 .backup`** from cron, while the server runs (a reader never blocks the server's
  writes in WAL mode):

  ```bash
  # /etc/cron.d/eqoa-backup: 04:30 every day, kept 14 days
  30 4 * * * eqoa sqlite3 /srv/eqoa-server/eqoa.db ".backup /srv/eqoa-server/backups/eqoa-$(date +\%Y\%m\%d).db" && gzip -f /srv/eqoa-server/backups/eqoa-$(date +\%Y\%m\%d).db && find /srv/eqoa-server/backups -name 'eqoa-*.db.gz' -mtime +14 -delete
  ```

Copy the backups off the machine too. Avoid running `VACUUM` or other long database work
while a restart is under way: it lengthens the pause players see.

## Updating

```bash
cd /srv/src/eqoa-server && git pull
(cd ../go-eqoa-esf-tools && git pull)        # the sibling checkout the build uses
go build -o gameserver ./cmd/gameserver
go build -o wsrelay ./cmd/wsrelay
```

Then either a plain restart:

```bash
sqlite3 /srv/eqoa-server/eqoa.db ".backup /srv/eqoa-server/backups/eqoa-pre-update.db"
sudo install -o eqoa -m 755 gameserver wsrelay /srv/eqoa-server/
sudo systemctl restart eqoa-game eqoa-relay
```

or stage `gameserver` and its `sha256` for `tools/swap_keep.sh` (players stay in) or
`tools/swap_when_empty.sh` (see Restarting). Set `world.restart_notice` so a plain restart warns
players first. Read the release notes before an update: a new database release
(`open-db-*`) is for a fresh server, not one with players.
