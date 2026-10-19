# wsrelay — a browser page's sockets, held on the server

A web page can open neither a UDP socket nor a raw TCP connection, and an EQOA
client needs both: DRDP over UDP to the game server, TCP to the login server
(9735) and the patch server's HTTP (7000). `wsrelay` holds the real sockets;
each of the page's sockets is one WebSocket to it.

The wire protocol is eqoa-port's `tools/wsrelay.py`, unchanged, so eqoa-port's
web build (`platform/webnet.c`, `web/page.js`) and eqoa-rs's web build both
speak it. This is the one to run on a public host; the Python one is a
development tool.

## Protocol

| Path | What |
|---|---|
| `/udp` (WebSocket) | A UDP socket on an ephemeral port. Each binary message is one datagram, both ways, behind a 6-byte header: the far end's IPv4 address (first octet first) and port (big-endian). |
| `/tcp?host=A.B.C.D&port=N` (WebSocket) | A TCP connection; the binary messages are its bytes. When the server closes it, the WebSocket closes (status 1000). |
| first message on either | Text: `ok LOCALADDR LOCALPORT` (the relay's own end of the socket, which is what the far server sees), or `err REASON`, after which the WebSocket closes. |
| `GET /resolve?name=NAME` | The name's IPv4 address as text (404 if none). |
| `GET /local?to=A.B.C.D` | This host's own address toward that host, as text. |

`/resolve` and `/local` carry `Access-Control-Allow-Origin: *`.

## What it reaches, and from where

**Only the `host:port` pairs it is given.** `wsrelay.py` allows any port of an
allowed host; on a machine that runs other services on loopback that would make
the relay a way into them. A TCP request for anything else gets `err`; a
datagram to anything else is dropped, and so is one *from* anything else.

The `ok` line of `/udp` names the relay's address toward the first allowed host
(`-allow-udp`, else `-allow-tcp`), as `wsrelay.py` does: a DRDP client builds
its own endpoint from it, so it must be the address the game server sees.

Behind a proxy on loopback (nginx), a client is the address the proxy names:
`X-Real-IP`, else the **last** entry of `X-Forwarded-For` (nginx's
`$proxy_add_x_forwarded_for` appends to whatever the client sent, so the first
entry is the client's to choose). From any other peer the headers are ignored.

## Flags

| Flag | Default | |
|---|---|---|
| `-listen` | `127.0.0.1:8790` | Where to serve. Behind a TLS proxy, keep it on loopback. |
| `-allow-udp` | none | `HOST:PORT` or `HOST:LO-HI`, repeatable or comma-separated. HOST is an IPv4 address, a network (`203.0.113.0/24`) or a name, resolved once at start. |
| `-allow-tcp` | none | The same, for `/tcp`. |
| `-origin` | any | A page origin allowed to connect (`https://example.org`), repeatable. A handshake from another origin gets 403. One with no `Origin` header (not a browser page) is let through: an Origin check keeps other sites' pages off the relay, which is all it can do. |
| `-max-per-ip` | 8 | Sockets one client address may hold at once (429 past it). |
| `-udp-idle` | `120s` | A UDP socket with no datagram either way this long is closed. |
| `-tcp-idle` | `10m` | A TCP connection idle this long is closed. |
| `-ping` | `30s` | WebSocket pings that notice a client gone silent (a browser answers them itself; the page never sees them). `0` turns them off. |
| `-v` | off | Also log dropped datagrams and failed handshakes. |

It logs one line per socket opened, refused and closed: the client, the target
or port, the bytes (and for UDP, datagrams) each way, how long, and why it ended.

```bash
go build -o wsrelay ./cmd/wsrelay
./wsrelay -allow-udp 203.0.113.10:10070 \
          -allow-tcp 203.0.113.10:9735,203.0.113.10:7000 \
          -origin https://example.org
```

Use the address the server list names (`server.server_ip` in `config.yaml`):
the client sends its DRDP to whatever endpoint the list gives it, and the relay
must allow exactly that.

## Behind nginx (wss)

A page served over https may only open `wss://` sockets. The trailing slash on
`proxy_pass` strips the prefix, so the relay sees `/udp` and `/tcp`, and a page
connects to `wss://example.org/eqoa/relay/udp`:

```nginx
location /eqoa/relay/ {
    proxy_pass http://127.0.0.1:8790/;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

## systemd

```ini
[Unit]
Description=EQOA WebSocket relay for browser clients
After=network-online.target
Wants=network-online.target

[Service]
User=sdg
ExecStart=/home/sdg/eqoa-server/wsrelay -allow-udp 203.0.113.10:10070 -allow-tcp 203.0.113.10:9735,203.0.113.10:7000 -origin https://example.org
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
```

## What the game server sees

Every web client arrives from the relay's address: one IP, a port per socket.
That is fine for this server: a login ticket (`internal/auth/tickets.go`) is
keyed by user name, so the TCP login and the UDP session that follow it through
the relay pair up; and a new login reaps another pre-game session from the same
address only once that one has been silent for 10 s
(`RemoveStaleGameSessions`), so two web players logging in at once do not take
each other out. Anything new that keys on a client's IP alone will lump web
players together.

## Tests

`go test ./internal/wsrelay/` runs the relay in-process against UDP and TCP
peers on loopback: a datagram out and its echo back with its header, the `ok`
port being the socket the datagram really left from; the stream both ways
(300 KB) and the byte counts; the server's close ending the socket cleanly;
refusals (a listening port that is not allowed, a name, bad ports) and the
datagrams to and from a port that is not allowed; origins; the per-client limit,
behind a proxy too; the idle timeout; a client that stops answering pings. Each
check that can pass by doing nothing has a control that must fail.
